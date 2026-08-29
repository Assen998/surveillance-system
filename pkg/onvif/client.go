package onvif

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Client ONVIF 客户端
type Client struct {
	timeout    time.Duration
	httpClient *http.Client
	username   string
	password   string
	// Digest 认证状态
	digestAuth *digestAuthState
}

type digestAuthState struct {
	realm     string
	nonce     string
	qop       string
	algorithm string
	opaque    string
	nc        int
}

type DeviceInfo struct {
	IP            string    `json:"ip"`
	Port          int       `json:"port"`
	Name          string    `json:"name"`
	Manufacturer  string    `json:"manufacturer"`
	Model         string    `json:"model"`
	Firmware      string    `json:"firmware"`
	SerialNumber  string    `json:"serialNumber"`
	HardwareId    string    `json:"hardwareId"`
	MAC           string    `json:"mac"`
	Profiles      []Profile `json:"profiles"`
	XAddr         string    `json:"xaddr"`
	AuthRequired  bool      `json:"auth_required"`
}

type Profile struct {
	Token               string `json:"token"`
	Name                string `json:"name"`
	Width               int    `json:"width"`
	Height              int    `json:"height"`
	FPS                 int    `json:"fps"`
	Bitrate             int    `json:"bitrate"`
	Codec               string `json:"codec"`
	RTSPUri             string `json:"rtspUri"`
	VideoSourceTok      string `json:"videoSourceTok"`
	VideoEncoderTok     string `json:"videoEncoderTok"`
	PTZConfigurationToken string `json:"ptzConfigurationToken"`
}

// StreamTransport 流传输协议
type StreamTransport string

const (
	TransportUDP      StreamTransport = "UDP"
	TransportTCP      StreamTransport = "TCP"
	TransportHTTP     StreamTransport = "HTTP"
	TransportRTSP     StreamTransport = "RTSP"
)

// StreamProfile 流配置
type StreamProfile struct {
	ProfileToken  string
	Transport     StreamTransport
	StreamType    string // "RTP-Unicast", "RTP-Multicast"
}

// NewClient 创建 ONVIF 客户端
func NewClient(timeoutSec int) *Client {
	return &Client{
		timeout: time.Duration(timeoutSec) * time.Second,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSec) * time.Second,
		},
	}
}

// SetCredentials 设置认证凭据
func (c *Client) SetCredentials(username, password string) {
	c.username = username
	c.password = password
	c.digestAuth = nil // 重置认证状态
}

// Discover 发现网络中的 ONVIF 设备
func (c *Client) Discover(network string) ([]*DeviceInfo, error) {
	devices, err := c.wsDiscovery(network)
	if err != nil {
		return nil, err
	}

	var results []*DeviceInfo
	for _, dev := range devices {
		info, err := c.GetDeviceInfo(dev.XAddr)
		if err != nil {
			continue
		}
		results = append(results, info)
	}

	return results, nil
}

func (c *Client) wsDiscovery(network string) ([]*DeviceInfo, error) {
	var devices []*DeviceInfo

	ip, ipNet, err := net.ParseCIDR(network)
	if err != nil {
		if dev, _ := c.probeSingle(network); dev != nil {
			devices = append(devices, dev)
		}
		return devices, nil
	}

	for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); incIP(ip) {
		if ip.Equal(ipNet.IP) || ip.Equal(broadcastIP(ipNet)) {
			continue
		}
		if dev, _ := c.probeSingle(ip.String()); dev != nil {
			devices = append(devices, dev)
		}
	}

	return devices, nil
}

// ProbeSingle 探测单个 IP 的 ONVIF 设备（公开方法，用于获取配置文件）
func (c *Client) ProbeSingle(ip string) *DeviceInfo {
	info, _ := c.probeSingle(ip)
	return info
}

// ProbeSingleEx 探测单个 IP，返回 (设备信息, 是否检测到设备但要求认证)
func (c *Client) ProbeSingleEx(ip string) (*DeviceInfo, bool) {
	return c.probeSingle(ip)
}

// probeSingle 探测单个 IP 的 ONVIF 设备（并发尝试多个端口）
// 返回 (设备信息, authRequired)。authRequired=true 表示设备可达但要求用户名/密码
func (c *Client) probeSingle(ip string) (*DeviceInfo, bool) {
	ports := []int{80, 8000, 8080, 5000, 8899} // 调整顺序：80, 8000 最常用
	
	type result struct {
		port int
		info *DeviceInfo
		authRequired bool
	}
	
	resultChan := make(chan result, len(ports))
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	
	// 并发探测所有端口
	for _, port := range ports {
		go func(p int) {
			addr := fmt.Sprintf("http://%s:%d/onvif/device_service", ip, p)
			select {
			case <-ctx.Done():
				resultChan <- result{port: p}
			default:
				info, authRequired := c.getDeviceInfo(addr)
				if info != nil {
					// 如果有认证凭据，尝试获取配置文件。
					// GetProfiles 属于 Media 服务，端点可能不同于 device_service，先解析或回退。
					if c.username != "" && c.password != "" {
						mediaAddr := c.ResolveMediaXAddr(addr)
						profiles, err := c.GetProfiles(mediaAddr)
						if (err != nil || len(profiles) == 0) && mediaAddr != addr {
							// 兜底：部分设备 media 服务合并到 device_service
							profiles, err = c.GetProfiles(addr)
						}
						if err == nil && len(profiles) > 0 {
							info.Profiles = profiles
						}
					}
					resultChan <- result{port: p, info: info}
				} else {
					resultChan <- result{port: p, authRequired: authRequired}
				}
			}
		}(port)
	}
	
	// 等待第一个成功结果，或全部完成
	sawAuthRequired := false
	completed := 0
	for completed < len(ports) {
		select {
		case r := <-resultChan:
			completed++
			if r.info != nil {
				cancel() // 取消其他 goroutine
				return r.info, false
			}
			if r.authRequired {
				sawAuthRequired = true
			}
		case <-ctx.Done():
			// 超时，收集已完成的结果
			for completed < len(ports) {
				select {
				case r := <-resultChan:
					if r.info != nil {
						return r.info, false
					}
					if r.authRequired {
						sawAuthRequired = true
					}
					completed++
				default:
					goto done
				}
			}
		}
	}
done:
	return nil, sawAuthRequired
}

// doRequest 执行 HTTP 请求，自动处理 Digest 认证
func (c *Client) doRequest(ctx context.Context, xaddr, soapAction, body string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", xaddr, bytes.NewReader([]byte(body)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	req.Header.Set("SOAPAction", soapAction)

	// 添加认证
	if c.username != "" && c.password != "" {
		if c.digestAuth != nil && c.digestAuth.nonce != "" {
			// 使用 Digest 认证
			authHeader := c.buildDigestAuth(req.Method, xaddr)
			req.Header.Set("Authorization", authHeader)
		} else {
			// 首次请求用 Basic 认证（某些设备支持），失败后会自动切换 Digest
			req.SetBasicAuth(c.username, c.password)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// 处理 401 认证挑战
	if resp.StatusCode == 401 && c.username != "" && c.password != "" {
		resp.Body.Close()
		return c.doRequestWithDigest(ctx, xaddr, soapAction, body, resp.Header.Get("WWW-Authenticate"))
	}

	return resp, nil
}

func (c *Client) doRequestWithDigest(ctx context.Context, xaddr, soapAction, body, wwwAuth string) (*http.Response, error) {
	// 解析 WWW-Authenticate 头
	c.parseDigestChallenge(wwwAuth)

	req, err := http.NewRequestWithContext(ctx, "POST", xaddr, bytes.NewReader([]byte(body)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	req.Header.Set("SOAPAction", soapAction)

	authHeader := c.buildDigestAuth(req.Method, xaddr)
	req.Header.Set("Authorization", authHeader)

	return c.httpClient.Do(req)
}

func (c *Client) parseDigestChallenge(challenge string) {
	// 解析 Digest 认证挑战
	// 格式: Digest realm="...", nonce="...", qop="...", algorithm="...", opaque="..."
	c.digestAuth = &digestAuthState{}
	
	// 去掉 "Digest " 前缀
	challenge = strings.TrimPrefix(challenge, "Digest ")
	
	parts := strings.Split(challenge, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		// 去掉 key=value 中 value 的引号
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.Trim(kv[1], "\"")
		
		switch key {
		case "realm":
			c.digestAuth.realm = val
		case "nonce":
			c.digestAuth.nonce = val
		case "qop":
			c.digestAuth.qop = val
		case "algorithm":
			c.digestAuth.algorithm = val
		case "opaque":
			c.digestAuth.opaque = val
		}
	}
	
	if c.digestAuth.algorithm == "" {
		c.digestAuth.algorithm = "MD5"
	}
}

func (c *Client) buildDigestAuth(method, uri string) string {
	if c.digestAuth == nil || c.digestAuth.nonce == "" {
		return ""
	}

	c.digestAuth.nc++
	nc := fmt.Sprintf("%08x", c.digestAuth.nc)
	cnonce := generateCnonce()

	// HA1 = MD5(username:realm:password)
	ha1 := md5.Sum([]byte(fmt.Sprintf("%s:%s:%s", c.username, c.digestAuth.realm, c.password)))
	ha1Str := hex.EncodeToString(ha1[:])

	// HA2 = MD5(method:uri)
	ha2 := md5.Sum([]byte(fmt.Sprintf("%s:%s", method, uri)))
	ha2Str := hex.EncodeToString(ha2[:])

	// Response = MD5(HA1:nonce:nc:cnonce:qop:HA2)
	response := md5.Sum([]byte(fmt.Sprintf("%s:%s:%s:%s:%s:%s", 
		ha1Str, c.digestAuth.nonce, nc, cnonce, c.digestAuth.qop, ha2Str)))
	responseStr := hex.EncodeToString(response[:])

	auth := fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", algorithm=%s, qop=%s, nc=%s, cnonce="%s", response="%s"`,
		c.username, c.digestAuth.realm, c.digestAuth.nonce, uri, c.digestAuth.algorithm, c.digestAuth.qop, nc, cnonce, responseStr)

	if c.digestAuth.opaque != "" {
		auth += fmt.Sprintf(`, opaque="%s"`, c.digestAuth.opaque)
	}

	return auth
}

func generateCnonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// fallback to timestamp-based if crypto/rand fails
		return fmt.Sprintf("%x", time.Now().UnixNano())[:16]
	}
	return hex.EncodeToString(b)[:16]
}

// Capabilities 各 ONVIF 服务的真实端点地址（GetCapabilities 返回）
type Capabilities struct {
	DeviceXAddr string
	MediaXAddr  string
	EventsXAddr string
	PTZXAddr    string
	ImagingXAddr string
}

// GetCapabilities 获取设备能力与各服务端点地址。xaddr 为 Device 服务地址（device_service）。
// GetProfiles/GetStreamUri 等 Media 接口必须发往 MediaXAddr，而非 device_service。
func (c *Client) GetCapabilities(xaddr string) (*Capabilities, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	resp, err := c.doRequest(ctx, xaddr,
		`"http://www.onvif.org/ver10/device/wsdl/GetCapabilities"`,
		getCapabilitiesBody())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	raw := string(body)

	caps := &Capabilities{
		DeviceXAddr:  extractServiceXAddr(raw, "Device"),
		MediaXAddr:   extractServiceXAddr(raw, "Media"),
		EventsXAddr:  extractServiceXAddr(raw, "Events"),
		PTZXAddr:     extractServiceXAddr(raw, "PTZ"),
		ImagingXAddr: extractServiceXAddr(raw, "Imaging"),
	}
	// 兜底：很多设备 Media/Events 服务与 device_service 同地址；拿不到时回退 device_service
	if caps.DeviceXAddr == "" {
		caps.DeviceXAddr = xaddr
	}
	if caps.MediaXAddr == "" {
		caps.MediaXAddr = xaddr
	}
	if caps.EventsXAddr == "" {
		caps.EventsXAddr = xaddr
	}
	return caps, nil
}

// ResolveMediaXAddr 解析 Media 服务端点：优先 GetCapabilities，失败则返回 device_service（兜底）
func (c *Client) ResolveMediaXAddr(deviceXAddr string) string {
	caps, err := c.GetCapabilities(deviceXAddr)
	if err == nil && caps.MediaXAddr != "" {
		return caps.MediaXAddr
	}
	return deviceXAddr
}

// ResolveEventsXAddr 解析 Events 服务端点：优先 GetCapabilities，失败则返回 device_service（兜底）
func (c *Client) ResolveEventsXAddr(deviceXAddr string) string {
	caps, err := c.GetCapabilities(deviceXAddr)
	if err == nil && caps.EventsXAddr != "" {
		return caps.EventsXAddr
	}
	return deviceXAddr
}

// extractServiceXAddr 从 GetCapabilities 响应中提取指定服务的 XAddr（兼容命名空间前缀差异）
func extractServiceXAddr(raw, service string) string {
	re := regexp.MustCompile(`(?s)<(?:[^>]*:)?` + service + `[^>]*>\s*<(?:[^>]*:)?XAddr[^>]*>([^<]+)</(?:[^>]*:)?XAddr>`)
	if m := re.FindStringSubmatch(raw); len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// getDeviceInfo 获取设备信息，返回 (设备信息, 是否要求认证)
func (c *Client) getDeviceInfo(xaddr string) (*DeviceInfo, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	resp, err := c.doRequest(ctx, xaddr, 
		`"http://www.onvif.org/ver10/device/wsdl/GetDeviceInformation"`,
		getDeviceInfoBody())
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()

	// 401: 设备可达但要求认证（未提供凭据或凭据错误）
	if resp.StatusCode == 401 {
		return nil, true
	}

	body, _ := io.ReadAll(resp.Body)
	return parseDeviceInfo(xaddr, string(body)), false
}

func (c *Client) GetDeviceInfo(xaddr string) (*DeviceInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	resp, err := c.doRequest(ctx, xaddr,
		`"http://www.onvif.org/ver10/device/wsdl/GetDeviceInformation"`,
		getDeviceInfoBody())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	info := parseDeviceInfo(xaddr, string(body))
	if info == nil {
		return nil, fmt.Errorf("解析设备信息失败")
	}
	return info, nil
}

// GetProfiles 获取所有配置文件
func (c *Client) GetProfiles(xaddr string) ([]Profile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	resp, err := c.doRequest(ctx, xaddr,
		`"http://www.onvif.org/ver10/media/wsdl/GetProfiles"`,
		getProfilesBody())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return parseProfiles(string(body))
}

// GetStreamUri 获取流地址，支持指定传输协议
func (c *Client) GetStreamUri(xaddr, profileToken string, transport StreamTransport) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	body := fmt.Sprintf(getStreamUriBody(transport), profileToken)
	resp, err := c.doRequest(ctx, xaddr,
		`"http://www.onvif.org/ver10/media/wsdl/GetStreamUri"`,
		body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return parseStreamUri(string(respBody))
}

// GetStreamUriWithRetry 重试获取流地址，优先尝试 preferredToken 指定的 Profile
func (c *Client) GetStreamUriWithRetry(xaddr string, profiles []Profile, preferredToken string, transport StreamTransport, maxRetries int) (string, *Profile, error) {
	// 重排顺序：优先的 Profile 放最前，其余保持原顺序
	order := make([]Profile, 0, len(profiles))
	if preferredToken != "" {
		for _, p := range profiles {
			if p.Token == preferredToken {
				order = append(order, p)
			}
		}
	}
	for _, p := range profiles {
		if p.Token != preferredToken {
			order = append(order, p)
		}
	}

	var lastErr error

	for i := 0; i < maxRetries && i < len(order); i++ {
		profile := order[i]
		uri, err := c.GetStreamUri(xaddr, profile.Token, transport)
		if err == nil && uri != "" {
			return uri, &profile, nil
		}
		lastErr = err
		// 尝试下一个 profile
	}

	return "", nil, fmt.Errorf("所有配置文件尝试失败: %v", lastErr)
}

func (c *Client) PTZControl(xaddr, command string, speed float64) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	body := fmt.Sprintf(getPTZBody(command, speed))
	resp, err := c.doRequest(ctx, xaddr,
		`"http://www.onvif.org/ver20/ptz/wsdl/ContinuousMove"`,
		body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// SOAP 消息体
func getDeviceInfoBody() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema">
    <GetDeviceInformation xmlns="http://www.onvif.org/ver10/device/wsdl"/>
  </s:Body>
</s:Envelope>`
}

func getProfilesBody() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema">
    <GetProfiles xmlns="http://www.onvif.org/ver10/media/wsdl"/>
  </s:Body>
</s:Envelope>`
}

func getCapabilitiesBody() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema">
    <GetCapabilities xmlns="http://www.onvif.org/ver10/device/wsdl">
      <Category>All</Category>
    </GetCapabilities>
  </s:Body>
</s:Envelope>`
}

// getStreamUriBody 支持指定传输协议
func getStreamUriBody(transport StreamTransport) string {
	proto := "RTSP"
	switch transport {
	case TransportUDP:
		proto = "RTP-Unicast"
	case TransportTCP:
		proto = "RTP-Unicast"
	case TransportHTTP:
		proto = "RTP-Unicast"
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema">
    <GetStreamUri xmlns="http://www.onvif.org/ver10/media/wsdl">
      <StreamSetup>
        <Stream xmlns="http://www.onvif.org/ver10/schema">%s</Stream>
        <Transport xmlns="http://www.onvif.org/ver10/schema">
          <Protocol>%s</Protocol>
        </Transport>
      </StreamSetup>
      <ProfileToken>%%s</ProfileToken>
    </GetStreamUri>
  </s:Body>
</s:Envelope>`, proto, strings.ToUpper(string(transport)))
}

func getPTZBody(command string, speed float64) string {
	var x, y, z float64
	switch command {
	case "up":
		y = speed
	case "down":
		y = -speed
	case "left":
		x = -speed
	case "right":
		x = speed
	case "zoom_in":
		z = speed
	case "zoom_out":
		z = -speed
	case "stop":
		x, y, z = 0, 0, 0
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema">
    <ContinuousMove xmlns="http://www.onvif.org/ver20/ptz/wsdl">
      <ProfileToken>%s</ProfileToken>
      <Velocity>
        <PanTilt x="%f" y="%f" xmlns="http://www.onvif.org/ver10/schema"/>
        <Zoom x="%f" xmlns="http://www.onvif.org/ver10/schema"/>
      </Velocity>
    </ContinuousMove>
  </s:Body>
</s:Envelope>`, "profile_token_placeholder", x, y, z)
}

// XML 解析
type soapEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    soapBody `xml:"Body"`
}

type soapBody struct {
	XMLName      xml.Name       `xml:"Body"`
	DeviceInfo   *devInfoResp   `xml:"GetDeviceInformationResponse"`
	Profiles     *profilesResp  `xml:"GetProfilesResponse"`
	StreamUri    *streamUriResp `xml:"GetStreamUriResponse"`
	Fault        *soapFault     `xml:"Fault"`
}

type devInfoResp struct {
	Manufacturer string `xml:"Manufacturer"`
	Model        string `xml:"Model"`
	Firmware     string `xml:"FirmwareVersion"`
	SerialNumber string `xml:"SerialNumber"`
	HardwareId   string `xml:"HardwareId"`
}

type profilesResp struct {
	Profiles []profileXml `xml:"Profiles"`
}

type profileXml struct {
	Token string `xml:"token,attr"`
	Name  string `xml:"Name"`
	VideoSourceConfiguration struct {
		Token string `xml:"token,attr"`
	} `xml:"VideoSourceConfiguration"`
	VideoEncoderConfiguration struct {
		Token     string `xml:"token,attr"`
		Name      string `xml:"Name"`
		Encoding  string `xml:"Encoding"`
		Resolution struct {
			Width  int `xml:"Width"`
			Height int `xml:"Height"`
		} `xml:"Resolution"`
		RateControl struct {
			BitrateLimit int `xml:"BitrateLimit"`
		} `xml:"RateControl"`
		Quality float64 `xml:"Quality"`
	} `xml:"VideoEncoderConfiguration"`
	PTZConfiguration struct {
		Token string `xml:"token,attr"`
	} `xml:"PTZConfiguration"`
}

type streamUriResp struct {
	MediaUri struct {
		Uri string `xml:"Uri"`
	} `xml:"MediaUri"`
}

type soapFault struct {
	Code   string `xml:"Code>Value"`
	String string `xml:"Reason>Text"`
}

func parseDeviceInfo(xaddr, body string) *DeviceInfo {
	var env soapEnvelope
	if err := xml.Unmarshal([]byte(body), &env); err != nil {
		return nil
	}

	if env.Body.Fault != nil {
		return nil
	}

	if env.Body.DeviceInfo == nil {
		return nil
	}

	info := &DeviceInfo{
		XAddr:        xaddr,
		Manufacturer: env.Body.DeviceInfo.Manufacturer,
		Model:        env.Body.DeviceInfo.Model,
		Firmware:     env.Body.DeviceInfo.Firmware,
		SerialNumber: env.Body.DeviceInfo.SerialNumber,
		HardwareId:   env.Body.DeviceInfo.HardwareId,
	}

	if strings.HasPrefix(xaddr, "http://") {
		hostPart := strings.TrimPrefix(xaddr, "http://")
		hostPart = strings.Split(hostPart, "/")[0]
		parts := strings.Split(hostPart, ":")
		info.IP = parts[0]
		if len(parts) > 1 {
			fmt.Sscanf(parts[1], "%d", &info.Port)
		} else {
			info.Port = 80
		}
	}

	return info
}

func parseProfiles(body string) ([]Profile, error) {
	var env soapEnvelope
	if err := xml.Unmarshal([]byte(body), &env); err != nil {
		return nil, err
	}

	if env.Body.Profiles == nil {
		return nil, nil
	}

	var profiles []Profile
	for _, p := range env.Body.Profiles.Profiles {
		profile := Profile{
			Token:            p.Token,
			Name:             p.Name,
			Width:            p.VideoEncoderConfiguration.Resolution.Width,
			Height:           p.VideoEncoderConfiguration.Resolution.Height,
			Codec:            strings.ToLower(p.VideoEncoderConfiguration.Encoding),
			Bitrate:          p.VideoEncoderConfiguration.RateControl.BitrateLimit,
			VideoSourceTok:   p.VideoSourceConfiguration.Token,
			VideoEncoderTok:  p.VideoEncoderConfiguration.Token,
			PTZConfigurationToken: p.PTZConfiguration.Token,
		}
		profiles = append(profiles, profile)
	}

	return profiles, nil
}

func parseStreamUri(body string) (string, error) {
	var env soapEnvelope
	if err := xml.Unmarshal([]byte(body), &env); err != nil {
		return "", err
	}

	if env.Body.StreamUri != nil {
		return env.Body.StreamUri.MediaUri.Uri, nil
	}

	return "", fmt.Errorf("未找到流地址")
}

// 工具函数
func incIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}

func broadcastIP(ipNet *net.IPNet) net.IP {
	ip := make(net.IP, len(ipNet.IP))
	copy(ip, ipNet.IP)
	for i := range ip {
		ip[i] |= ^ipNet.Mask[i]
	}
	return ip
}

type probeMatch struct {
	XAddr        string
	Manufacturer string
	Model        string
	Firmware     string
	SerialNumber string
	HardwareId   string
	IP           string
	Port         int
}

// WSDiscover 真正的 WS-Discovery 组播探测（ONVIF 标准）
// 向 239.255.255.250:3702 发送 Probe，收集 ProbeMatches 响应
// 返回发现的设备列表（含 IP、端口、XAddr、厂商、型号、配置文件）
func (c *Client) WSDiscover(timeoutSec int) ([]*DeviceInfo, error) {
	// 组播地址
	mcastAddr := &net.UDPAddr{
		IP:   net.ParseIP("239.255.255.250"),
		Port: 3702,
	}

	// 监听所有接口的 UDP 响应
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("监听 UDP 失败: %w", err)
	}
	defer conn.Close()

	// 设置读取超时
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	conn.SetReadDeadline(deadline)

	// 构造 WS-Discovery Probe 消息（SOAP over UDP）
	messageID := fmt.Sprintf("uuid:%s", generateMessageID())
	probeMsg := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope"
            xmlns:w="http://schemas.xmlsoap.org/ws/2005/04/discovery"
            xmlns:dn="http://www.onvif.org/ver10/network/wsdl">
  <e:Header>
    <w:MessageID>%s</w:MessageID>
    <w:To e:mustUnderstand="true">urn:schemas-xmlsoap-org:ws:2005:04:discovery</w:To>
    <w:Action e:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</w:Action>
  </e:Header>
  <e:Body>
    <w:Probe>
      <w:Types>dn:NetworkVideoTransmitter</w:Types>
    </w:Probe>
  </e:Body>
</e:Envelope>`, messageID)

	// 发送组播 Probe
	if _, err := conn.WriteTo([]byte(probeMsg), mcastAddr); err != nil {
		return nil, fmt.Errorf("发送组播 Probe 失败: %w", err)
	}

	// 收集响应
	matches := make(map[string]*probeMatch) // key: XAddr 去重
	buf := make([]byte, 8192)

	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break // 超时结束
			}
			continue
		}
		if n == 0 {
			continue
		}

		resp := string(buf[:n])
		// 解析 ProbeMatches
		if strings.Contains(resp, "ProbeMatches") {
			pm := c.parseProbeMatch(resp)
			if pm != nil {
				key := pm.XAddr
				if _, exists := matches[key]; !exists {
					matches[key] = pm
					logrus.Debugf("WS-Discovery 发现设备: %s (%s)", pm.Manufacturer, pm.XAddr)
				}
			}
		}
	}

	// 转换为 DeviceInfo。组播发现阶段没有凭据：设备即使返回 401，
	// 也必须保留（发现的目标是找到设备 IP/XAddr），详情交给选中后的带凭据探测。
	var results []*DeviceInfo
	for _, pm := range matches {
		if pm.IP == "" || pm.Port == 0 {
			continue
		}
		dev := &DeviceInfo{
			IP:           pm.IP,
			Port:         pm.Port,
			XAddr:        pm.XAddr,
			Manufacturer: pm.Manufacturer,
			Model:        pm.Model,
			Firmware:     pm.Firmware,
			SerialNumber: pm.SerialNumber,
			HardwareId:   pm.HardwareId,
		}
		if dev.XAddr == "" {
			dev.XAddr = fmt.Sprintf("http://%s:%d/onvif/device_service", dev.IP, dev.Port)
		}
		// 设备未启用认证时可以顺手补充设备信息；401/网络失败都不影响发现结果
		if info, _ := c.getDeviceInfo(dev.XAddr); info != nil {
			dev.Name = info.Name
			dev.Manufacturer = firstNonEmpty(pm.Manufacturer, info.Manufacturer)
			dev.Model = firstNonEmpty(pm.Model, info.Model)
			dev.Firmware = firstNonEmpty(pm.Firmware, info.Firmware)
			dev.SerialNumber = firstNonEmpty(pm.SerialNumber, info.SerialNumber)
			dev.HardwareId = firstNonEmpty(pm.HardwareId, info.HardwareId)
		}
		results = append(results, dev)
	}

	return results, nil
}

var xaddrsRe = regexp.MustCompile(`<[^>]*XAddrs[^>]*>([^<]+)</[^>]*XAddrs>`)

// parseProbeMatch 解析 WS-Discovery ProbeMatch 响应
func (c *Client) parseProbeMatch(body string) *probeMatch {
	// 简易 XML 解析（避免依赖具体命名空间前缀）
	pm := &probeMatch{}

	// 提取 XAddrs
	if m := xaddrsRe.FindStringSubmatch(body); len(m) > 1 {
		for _, x := range strings.Fields(m[1]) {
			if strings.HasPrefix(x, "http://") || strings.HasPrefix(x, "https://") {
				pm.XAddr = strings.TrimSpace(x)
				break
			}
		}
	}

	// 从 XAddr 推导 IP 和端口
	if pm.XAddr != "" {
		if strings.HasPrefix(pm.XAddr, "http://") {
			hostPart := strings.TrimPrefix(pm.XAddr, "http://")
			hostPart = strings.Split(hostPart, "/")[0]
			parts := strings.Split(hostPart, ":")
			pm.IP = parts[0]
			if len(parts) > 1 {
				fmt.Sscanf(parts[1], "%d", &pm.Port)
			} else {
				pm.Port = 80
			}
		}
	}

	// 提取 Metadata 中的设备信息
	extract := func(tag string) string {
		start := strings.Index(body, "<"+tag+">")
		if start < 0 {
			start = strings.Index(body, "<dn:"+tag+">")
		}
		if start < 0 {
			return ""
		}
		start += len(tag) + 2
		end := strings.Index(body[start:], "</"+tag+">")
		if end < 0 {
			end = strings.Index(body[start:], "</dn:"+tag+">")
		}
		if end < 0 {
			return ""
		}
		return strings.TrimSpace(body[start : start+end])
	}

	pm.Manufacturer = extract("Manufacturer")
	pm.Model = extract("Model")
	pm.Firmware = extract("FirmwareVersion")
	pm.SerialNumber = extract("SerialNumber")
	pm.HardwareId = extract("HardwareId")

	return pm
}

// generateMessageID 生成简单的消息 ID
func generateMessageID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// ==================== 网段快速扫描（WS-Discovery 不可用时的回退方案） ====================

// quickONVIFCheck 对单个 IP 的常用 ONVIF 端口做快速探测。
// 判定依据：/onvif/device_service 端点返回 401 认证挑战（需登录的 ONVIF 设备）、
// 200 SOAP 响应（免认证设备）或 405/400（端点存在但方法/报文不被接受）。
func (c *Client) quickONVIFCheck(ip string, timeout time.Duration) *DeviceInfo {
	client := &http.Client{Timeout: timeout}
	type found struct {
		dev *DeviceInfo
	}
	ch := make(chan *DeviceInfo, 3)

	for _, port := range []int{80, 8000, 8080} {
		go func(port int) {
			xaddr := fmt.Sprintf("http://%s:%d/onvif/device_service", ip, port)
			req, err := http.NewRequest("POST", xaddr, bytes.NewReader([]byte(getDeviceInfoBody())))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
			req.Header.Set("SOAPAction", `"http://www.onvif.org/ver10/device/wsdev/GetDeviceInformation"`)
			resp, err := client.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			switch {
			case resp.StatusCode == http.StatusUnauthorized:
				// 401 + WWW-Authenticate：ONVIF 端点存在，需要凭据
				if resp.Header.Get("WWW-Authenticate") != "" {
					ch <- &DeviceInfo{IP: ip, Port: port, XAddr: xaddr, Name: ip, AuthRequired: true}
				}
			case resp.StatusCode == http.StatusOK:
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
				info := parseDeviceInfo(xaddr, string(body))
				if info != nil {
					info.IP = ip
					info.Port = port
					info.XAddr = xaddr
					ch <- info
				}
			case resp.StatusCode == http.StatusMethodNotAllowed:
				// 405：ONVIF 端点存在但不接受该方法
				ch <- &DeviceInfo{IP: ip, Port: port, XAddr: xaddr, Name: ip, AuthRequired: true}
			case resp.StatusCode == http.StatusBadRequest:
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
				if strings.Contains(string(body), "Fault") || strings.Contains(string(body), "onvif") {
					ch <- &DeviceInfo{IP: ip, Port: port, XAddr: xaddr, Name: ip, AuthRequired: true}
				}
			}
		}(port)
	}

	select {
	case dev := <-ch:
		return dev
	case <-time.After(timeout):
		return nil
	}
}

// SweepCIDR 对指定网段做高并发 ONVIF 快速扫描
func (c *Client) SweepCIDR(cidr string, perProbe time.Duration) []*DeviceInfo {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil
	}
	var ips []string
	for cur := ip.Mask(ipNet.Mask); ipNet.Contains(cur); {
		inc := cur
		if !inc.Equal(ipNet.IP) && !inc.Equal(broadcastIP(ipNet)) {
			ips = append(ips, cur.String())
		}
		next := make(net.IP, len(cur))
		copy(next, cur)
		for i := len(next) - 1; i >= 0; i-- {
			next[i]++
			if next[i] != 0 {
				break
			}
		}
		if !ipNet.Contains(next) {
			break
		}
		cur = next
	}

	results := make([]*DeviceInfo, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 128)

	for _, ipStr := range ips {
		wg.Add(1)
		sem <- struct{}{}
		go func(ipStr string) {
			defer wg.Done()
			defer func() { <-sem }()
			if dev := c.quickONVIFCheck(ipStr, perProbe); dev != nil {
				mu.Lock()
				results = append(results, dev)
				mu.Unlock()
			}
		}(ipStr)
	}
	wg.Wait()
	return results
}

// LocalScanCIDRs 返回本机所有活动接口所在 /24 网段（排除回环/虚拟网桥/Docker/Tailscale）
func LocalScanCIDRs() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var cidrs []string
	seen := make(map[string]bool)
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipn.IP.To4()
			if ip4 == nil {
				continue
			}
			// 排除：回环、Docker 网桥(172.16-31/12)、Tailscale CGNAT(100.64/10)
			if ip4[0] == 127 {
				continue
			}
			if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
				continue
			}
			if ip4[0] == 100 && (ip4[1]&0x3c) == 64 {
				continue
			}
			cidr24 := fmt.Sprintf("%d.%d.%d.0/24", ip4[0], ip4[1], ip4[2])
			if !seen[cidr24] {
				seen[cidr24] = true
				cidrs = append(cidrs, cidr24)
			}
		}
	}
	return cidrs
}

// SweepLocalSubnets 扫描本机所有网段的 ONVIF 设备（多网段并行，整体超时控制）
func (c *Client) SweepLocalSubnets(overall time.Duration) ([]*DeviceInfo, error) {
	cidrs := LocalScanCIDRs()
	if len(cidrs) == 0 {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), overall)
	defer cancel()

	perProbe := 1000 * time.Millisecond
	if perProbe > overall/3 {
		perProbe = overall / 3
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	results := make([]*DeviceInfo, 0)
	seen := make(map[string]bool)

	for _, cidr := range cidrs {
		wg.Add(1)
		go func(cidr string) {
			defer wg.Done()
			if ctx.Err() != nil {
				return
			}
			devs := c.SweepCIDR(cidr, perProbe)
			mu.Lock()
			for _, dev := range devs {
				if !seen[dev.IP] {
					seen[dev.IP] = true
					results = append(results, dev)
				}
			}
			mu.Unlock()
		}(cidr)
	}
	wg.Wait()
	return results, nil
}

// ==================== 事件订阅（EventService / Pull-Point） ====================

// EventSubscription 事件订阅句柄（Pull-Point 订阅返回的拉取地址）
type EventSubscription struct {
	// Address 拉取消息所用的端点地址（SubscriptionReference/Address）
	Address string
	// TerminationTime 订阅终止时间（到期前需 Renew/重新订阅）
	TerminationTime time.Time
}

// OnvifEvent 解析后的 ONVIF 通知事件
type OnvifEvent struct {
	// Topic 去掉命名空间前缀的事件主题，如 "RuleEngine/MotionRegionDetector/Motion"
	Topic string
	// UtcTime 事件发生时间（tt:Message UtcTime），零值表示未解析到
	UtcTime time.Time
	// Items SimpleItem 键值对（如 IsMotion, Source 等）
	Items map[string]string
}

// 事件服务 SOAPAction
const (
	soapActionCreatePullPoint = `"http://www.onvif.org/ver10/events/wsdl/PullPointSubscription/CreatePullPointSubscriptionRequest"`
	soapActionPullMessages    = `"http://www.onvif.org/ver10/events/wsdl/PullPointSubscription/PullMessagesRequest"`
	soapActionUnsubscribe     = `"http://www.onvif.org/ver10/events/wsdl/PullPointSubscription/UnsubscribeRequest"`
)

// CreatePullPointSubscription 创建 Pull-Point 订阅。
// xaddr 为设备服务地址（通常与 device_service 相同）。返回订阅句柄。
// 注意：设备可能不支持 Pull-Point，返回 error 时调用方应降级跳过事件订阅。
func (c *Client) CreatePullPointSubscription(xaddr string) (*EventSubscription, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	body := getPullPointSubscriptionBody()
	resp, err := c.doRequest(ctx, xaddr, soapActionCreatePullPoint, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("创建事件订阅失败 (HTTP %d)", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseSubscription(string(raw))
}

// PullMessages 从订阅地址拉取事件（阻塞式）。timeoutSec 为等待时间。
// 返回本次拉取到的事件列表（可能为空，表示超时无事件）。
// 注意：PullMessages 是长轮询——设备在无事件时会阻塞到 timeoutSec 才返回空响应，
// 因此 HTTP 客户端超时必须显著大于 timeoutSec，否则会提前超时导致频繁重订阅。
func (c *Client) PullMessages(subscriptionURL string, timeoutSec int) ([]OnvifEvent, error) {
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec+10)*time.Second)
	defer cancel()

	// 长轮询需要独立、更长的 HTTP 超时（+10s 缓冲），临时放宽共享 client 的超时，
	// 调用结束恢复。本 client 实例仅被单个订阅 goroutine 串行使用，故安全。
	origTimeout := c.httpClient.Timeout
	c.httpClient.Timeout = time.Duration(timeoutSec+10) * time.Second
	defer func() { c.httpClient.Timeout = origTimeout }()

	body := getPullMessagesBody(timeoutSec)
	resp, err := c.doRequest(ctx, subscriptionURL, soapActionPullMessages, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("拉取事件失败 (HTTP %d)", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseNotificationMessages(string(raw)), nil
}

// Unsubscribe 取消订阅并释放设备端资源（失败不报致命错误）
func (c *Client) Unsubscribe(subscriptionURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	body := getUnsubscribeBody()
	resp, err := c.doRequest(ctx, subscriptionURL, soapActionUnsubscribe, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return nil
}

// SOAP 消息体
func getPullPointSubscriptionBody() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema">
    <CreatePullPointSubscription xmlns="http://www.onvif.org/ver10/events/wsdl">
      <InitialTerminationTime>PT1H</InitialTerminationTime>
    </CreatePullPointSubscription>
  </s:Body>
</s:Envelope>`
}

func getPullMessagesBody(timeoutSec int) string {
	// PTxxS：ISO-8601 时长格式
	dur := fmt.Sprintf("PT%dS", timeoutSec)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema">
    <PullMessages xmlns="http://www.onvif.org/ver10/events/wsdl">
      <Timeout>%s</Timeout>
      <MessageLimit>128</MessageLimit>
    </PullMessages>
  </s:Body>
</s:Envelope>`, dur)
}

func getUnsubscribeBody() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema">
    <Unsubscribe xmlns="http://www.onvif.org/ver10/events/wsdl"/>
  </s:Body>
</s:Envelope>`
}

// 解析工具：用正则容错提取（兼容各厂商命名空间前缀差异）

var (
	reSubscriptionAddress = regexp.MustCompile(`<[^>]*(?:wsa5?:)?Address[^>]*>\s*([^<\s]+)\s*</[^>]*(?:wsa5?:)?Address>`)
	reTerminationTime     = regexp.MustCompile(`<[^>]*TerminationTime[^>]*>\s*([^<]+?)\s*</[^>]*TerminationTime>`)
	reCurrentTime         = regexp.MustCompile(`<[^>]*CurrentTime[^>]*>\s*([^<]+?)\s*</[^>]*CurrentTime>`)
	// 提取 Topic 文本（去掉命名空间前缀）
	reTopic = regexp.MustCompile(`<[^>]*Topic[^>]*>\s*([^<]+?)\s*</[^>]*Topic>`)
	// NotificationMessage 块（一个事件）
	reNotificationMessage = regexp.MustCompile(`(?s)<[^>]*NotificationMessage[^>]*>.*?</[^>]*NotificationMessage>`)
	// SimpleItem Name/Value
	reSimpleItem = regexp.MustCompile(`(?s)<[^>]*SimpleItem\s+Name="([^"]+)"\s+Value="([^"]*)"`)
	// tt:Message 的 UtcTime
	reUtcTime = regexp.MustCompile(`(?s)<[^>]*Message[^>]*UtcTime="([^"]+)"`)
)

func parseSubscription(raw string) (*EventSubscription, error) {
	sub := &EventSubscription{}
	m := reSubscriptionAddress.FindStringSubmatch(raw)
	if len(m) < 2 || m[1] == "" {
		return nil, fmt.Errorf("订阅响应中未找到 SubscriptionReference Address")
	}
	sub.Address = m[1]
	if tm := reTerminationTime.FindStringSubmatch(raw); len(tm) >= 2 {
		if t, err := parseOnvifTime(tm[1]); err == nil {
			sub.TerminationTime = t
		}
	}
	return sub, nil
}

// parseOnvifTime 解析 ONVIF 时间字符串（ISO8601/XSD dateTime）
func parseOnvifTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("空时间")
	}
	layouts := []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04:05Z0700", "2006-01-02T15:04:05.000Z"}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析时间 %q", s)
}

func parseNotificationMessages(raw string) []OnvifEvent {
	var events []OnvifEvent
	blocks := reNotificationMessage.FindAllString(raw, -1)
	for _, block := range blocks {
		ev := OnvifEvent{Items: make(map[string]string)}
		if m := reTopic.FindStringSubmatch(block); len(m) >= 2 {
			ev.Topic = normalizeTopic(m[1])
		}
		if m := reUtcTime.FindStringSubmatch(block); len(m) >= 2 {
			if t, err := parseOnvifTime(m[1]); err == nil {
				ev.UtcTime = t
			}
		}
		for _, sm := range reSimpleItem.FindAllStringSubmatch(block, -1) {
			if len(sm) >= 3 {
				ev.Items[sm[1]] = sm[2]
			}
		}
		// 即使 Topic 为空，只要有 SimpleItem 也算一个事件
		if ev.Topic != "" || len(ev.Items) > 0 {
			events = append(events, ev)
		}
	}
	return events
}

// normalizeTopic 去掉命名空间前缀，返回形如 "RuleEngine/MotionRegionDetector/Motion"
func normalizeTopic(topic string) string {
	s := strings.TrimSpace(topic)
	// 去掉前半部分命名空间（形如 tns1: 或 Axis: 或 urn:...）
	if idx := strings.IndexByte(s, ':'); idx >= 0 && idx < 8 {
		s = s[idx+1:]
	}
	return s
}