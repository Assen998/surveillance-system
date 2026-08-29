package onvif

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestEventServiceRoundTrip 用 httptest 模拟一个支持 Pull-Point 事件服务的 ONVIF 设备，
// 验证 CreatePullPointSubscription → PullMessages 的完整 SOAP 交互。
func TestEventServiceRoundTrip(t *testing.T) {
	mux := http.NewServeMux()

	// 订阅端点：返回 SubscriptionReference（指向 /events 端点）
	mux.HandleFunc("/onvif/device_service", func(w http.ResponseWriter, r *http.Request) {
		soapAction := r.Header.Get("SOAPAction")
		if strings.Contains(soapAction, "CreatePullPointSubscription") {
			resp := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
 <s:Body>
  <CreatePullPointSubscriptionResponse xmlns="http://www.onvif.org/ver10/events/wsdl">
   <SubscriptionReference xmlns="http://www.onvif.org/ver10/schema">
    <wsa5:Address xmlns:wsa5="http://www.w3.org/2005/08/addressing">/onvif/events?Idx=1</wsa5:Address>
   </SubscriptionReference>
   <CurrentTime xmlns="http://docs.oasis-open.org/wsn/b-2">2026-08-28T16:00:00Z</CurrentTime>
   <TerminationTime xmlns="http://docs.oasis-open.org/wsn/b-2">2026-08-28T17:00:00Z</TerminationTime>
  </CreatePullPointSubscriptionResponse>
 </s:Body>
</s:Envelope>`
			w.Header().Set("Content-Type", "application/soap+xml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(resp))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	// 拉取端点：返回一个运动事件
	mux.HandleFunc("/onvif/events", func(w http.ResponseWriter, r *http.Request) {
		soapAction := r.Header.Get("SOAPAction")
		if !strings.Contains(soapAction, "PullMessages") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.ReadAll(r.Body)
		resp := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
 <s:Body>
  <PullMessagesResponse xmlns="http://www.onvif.org/ver10/events/wsdl">
   <CurrentTime xmlns="http://docs.oasis-open.org/wsn/b-2">2026-08-28T16:00:10Z</CurrentTime>
   <TerminationTime xmlns="http://docs.oasis-open.org/wsn/b-2">2026-08-28T17:00:00Z</TerminationTime>
   <NotificationMessage xmlns="http://docs.oasis-open.org/wsn/b-2">
    <Topic Dialect="http://www.onvif.org/ver10/tev/topicExpression/ConcreteSet">
      tns1:RuleEngine/MotionRegionDetector/Motion
    </Topic>
    <Message>
      <tt:Message xmlns:tt="http://www.onvif.org/ver10/schema" UtcTime="2026-08-28T16:00:05Z" PropertyOperation="Changed">
       <tt:Data>
        <tt:SimpleItem Name="IsMotion" Value="true"/>
       </tt:Data>
      </tt:Message>
    </Message>
   </NotificationMessage>
  </PullMessagesResponse>
 </s:Body>
</s:Envelope>`
		w.Header().Set("Content-Type", "application/soap+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(resp))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewClient(5)
	client.SetCredentials("admin", "123456")

	sub, err := client.CreatePullPointSubscription(ts.URL + "/onvif/device_service")
	if err != nil {
		t.Fatalf("订阅失败: %v", err)
	}

	// 注意：模拟设备返回的相对地址，客户端需拼接 host。
	// 若 Address 是相对路径，这里修正为绝对 URL。
	addr := sub.Address
	if strings.HasPrefix(addr, "/") {
		addr = ts.URL + addr
	}
	t.Logf("订阅地址: %s", addr)

	events, err := client.PullMessages(addr, 1)
	if err != nil {
		t.Fatalf("拉取事件失败: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("应拉取到 1 个事件，实际 %d", len(events))
	}
	if events[0].Topic != "RuleEngine/MotionRegionDetector/Motion" {
		t.Errorf("Topic 错误: %q", events[0].Topic)
	}
	if events[0].Items["IsMotion"] != "true" {
		t.Errorf("IsMotion 错误: %q", events[0].Items["IsMotion"])
	}
	fmt.Println("roundtrip ok")
}