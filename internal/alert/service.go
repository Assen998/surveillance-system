package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/yourorg/surveillance-system/internal/config"
	"github.com/yourorg/surveillance-system/internal/models"
	"github.com/sirupsen/logrus"
)

type Manager struct {
	cfg       *config.Config
	alertCh   chan *models.Alert
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	templates map[string]string
}

type WebhookPayload struct {
	Alert     *models.Alert `json:"alert"`
	Camera    *CameraInfo   `json:"camera,omitempty"`
	Timestamp string        `json:"timestamp"`
}

type CameraInfo struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	IP   string `json:"ip"`
}

func NewManager(cfg *config.Config) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		cfg:     cfg,
		alertCh: make(chan *models.Alert, 1000),
		ctx:     ctx,
		cancel:  cancel,
		templates: map[string]string{
			"motion":       "🚨 运动检测报警",
			"intrusion":    "🚨 区域入侵报警",
			"line_cross":   "🚨 越界报警",
			"object_detect": "🚨 目标检测报警",
			"offline":      "⚠️ 摄像头离线",
			"storage_full": "💾 存储空间不足",
			"error":        "❌ 系统错误",
		},
	}

	// 注册到 analytics
	// analytics.RegisterAlertHandler(m.OnAlert)

	return m
}

func (m *Manager) Start() error {
	if !m.cfg.Alert.Enabled {
		logrus.Info("报警推送未启用，跳过启动")
		return nil
	}

	m.wg.Add(1)
	go m.processLoop()

	logrus.Info("报警管理器启动完成")
	return nil
}

func (m *Manager) Stop() error {
	m.cancel()
	m.wg.Wait()
	logrus.Info("报警管理器已停止")
	return nil
}

func (m *Manager) OnAlert(alert *models.Alert) {
	select {
	case m.alertCh <- alert:
	default:
		logrus.Warn("报警队列已满，丢弃报警")
	}
}

func (m *Manager) processLoop() {
	defer m.wg.Done()

	for {
		select {
		case <-m.ctx.Done():
			return
		case alert := <-m.alertCh:
			m.sendAlert(alert)
		}
	}
}

func (m *Manager) sendAlert(alert *models.Alert) {
	var wg sync.WaitGroup

	// Webhook
	if m.cfg.Alert.Channels.Webhook.Enabled && m.cfg.Alert.Channels.Webhook.URL != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.sendWebhook(alert, &m.cfg.Alert.Channels.Webhook); err != nil {
				logrus.Errorf("Webhook 推送失败: %v", err)
			}
		}()
	}

	// Email
	if m.cfg.Alert.Channels.Email.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.sendEmail(alert, &m.cfg.Alert.Channels.Email); err != nil {
				logrus.Errorf("邮件推送失败: %v", err)
			}
		}()
	}

	// SMS
	if m.cfg.Alert.Channels.SMS.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.sendSMS(alert, &m.cfg.Alert.Channels.SMS); err != nil {
				logrus.Errorf("短信推送失败: %v", err)
			}
		}()
	}

	wg.Wait()

	// 标记已通知
	// 实际应用中应更新数据库
}

func (m *Manager) sendWebhook(alert *models.Alert, wh *config.WebhookAlertConfig) error {
	if wh.URL == "" {
		return fmt.Errorf("Webhook URL 未配置")
	}
	var data []byte
	var err error

	// 按 webhook 类型选择 payload 格式
	if strings.EqualFold(wh.Type, "gotify") {
		data, err = m.buildGotifyPayload(alert)
	} else {
		// generic（默认）：通用 JSON
		payload := WebhookPayload{
			Alert:     alert,
			Timestamp: time.Now().Format(time.RFC3339),
		}
		data, err = json.Marshal(payload)
	}
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", wh.URL, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	// 钉钉/飞书/企业微信签名验证可在此添加

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook 返回错误状态码: %d", resp.StatusCode)
	}

	return nil
}

// gotifyMessage Gotify /message 接口请求体
type gotifyMessage struct {
	Title    string `json:"title,omitempty"`
	Message  string `json:"message"`
	Priority int    `json:"priority"`
}

// buildGotifyPayload 将报警转换为 Gotify /message 格式
func (m *Manager) buildGotifyPayload(alert *models.Alert) ([]byte, error) {
	title := m.templates[alert.Type]
	if title == "" {
		title = "🚨 监控报警"
	}

	// 消息正文：优先用报警自带 message，否则按类型 + 摄像头生成
	msg := strings.TrimSpace(alert.Message)
	if msg == "" {
		msg = fmt.Sprintf("摄像头 %d 触发 %s 报警", alert.CameraID, alert.Type)
	}
	// 附带报警时间与详情，方便手机端直接查看
	detail := fmt.Sprintf("\n时间：%s", time.Now().Format("2006-01-02 15:04:05"))
	if alert.CameraID > 0 {
		detail = fmt.Sprintf("\n摄像头 ID：%d%s", alert.CameraID, detail)
	}

	body := gotifyMessage{
		Title:    title,
		Message:  msg + detail,
		Priority: levelToGotifyPriority(alert.Level),
	}
	return json.Marshal(body)
}

// levelToGotifyPriority 将报警等级映射到 Gotify 优先级（0~10）
func levelToGotifyPriority(level string) int {
	switch level {
	case models.AlertLevelLow:
		return 1
	case models.AlertLevelMedium:
		return 3
	case models.AlertLevelHigh:
		return 5
	case models.AlertLevelCritical:
		return 8
	default:
		return 3
	}
}

func (m *Manager) sendEmail(alert *models.Alert, em *config.EmailAlertConfig) error {
	cfg := em
	if len(cfg.To) == 0 {
		return fmt.Errorf("未配置收件人")
	}

	subject := fmt.Sprintf("[监控报警] %s - %s", m.templates[alert.Type], alert.Message)
	body := m.formatEmailBody(alert)

	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.SMTPHost)
	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)

	msg := []byte(fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n"+
		"\r\n"+
		"%s", cfg.From, strings.Join(cfg.To, ","), subject, body))

	return smtp.SendMail(addr, auth, cfg.From, cfg.To, msg)
}

func (m *Manager) formatEmailBody(alert *models.Alert) string {
	levelColor := map[string]string{
		models.AlertLevelLow:      "#28a745",
		models.AlertLevelMedium:   "#ffc107",
		models.AlertLevelHigh:     "#fd7e14",
		models.AlertLevelCritical: "#dc3545",
	}

	color := levelColor[alert.Level]

	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
</head>
<body style="font-family: Arial, sans-serif; line-height: 1.6; color: #333;">
    <div style="max-width: 600px; margin: 0 auto; padding: 20px;">
        <div style="background: %s; color: white; padding: 20px; border-radius: 8px 8px 0 0;">
            <h2 style="margin: 0;">%s</h2>
        </div>
        <div style="background: #f8f9fa; padding: 20px; border-radius: 0 0 8px 8px; border: 1px solid #dee2e6; border-top: none;">
            <table style="width: 100%%; border-collapse: collapse;">
                <tr>
                    <td style="padding: 8px 0; font-weight: bold; width: 120px;">报警类型:</td>
                    <td style="padding: 8px 0;">%s</td>
                </tr>
                <tr>
                    <td style="padding: 8px 0; font-weight: bold;">报警等级:</td>
                    <td style="padding: 8px 0;"><span style="background: %s; color: white; padding: 2px 8px; border-radius: 4px;">%s</span></td>
                </tr>
                <tr>
                    <td style="padding: 8px 0; font-weight: bold;">摄像头 ID:</td>
                    <td style="padding: 8px 0;">%d</td>
                </tr>
                <tr>
                    <td style="padding: 8px 0; font-weight: bold;">报警时间:</td>
                    <td style="padding: 8px 0;">%s</td>
                </tr>
                <tr>
                    <td style="padding: 8px 0; font-weight: bold;">详细信息:</td>
                    <td style="padding: 8px 0;">%s</td>
                </tr>
            </table>
            %s
        </div>
    </div>
</body>
</html>
`, color, m.templates[alert.Type], alert.Type, color, strings.ToUpper(alert.Level), alert.CameraID, alert.CreatedAt.Format("2006-01-02 15:04:05"), alert.Message, m.getSnapshotHTML(alert))
}

func (m *Manager) getSnapshotHTML(alert *models.Alert) string {
	if alert.SnapshotPath != "" {
		return fmt.Sprintf(`<div style="margin-top: 20px;">
            <p style="font-weight: bold;">现场抓拍:</p>
            <img src="cid:snapshot" alt="报警抓拍" style="max-width: 100%%; border: 1px solid #dee2e6; border-radius: 4px;">
        </div>`)
	}
	return ""
}

func (m *Manager) sendSMS(alert *models.Alert, sm *config.SMSAlertConfig) error {
	cfg := sm

	// 这里需要根据具体短信服务商实现
	// 阿里云、腾讯云等都有 Go SDK
	logrus.Infof("短信推送 (模拟): 发送给 %s, 内容: %s", cfg.SignName, alert.Message)

	// 示例：阿里云短信
	// client, _ := dysmsapi.NewClientWithAccessKey(...)
	// request := &dysmsapi.SendSmsRequest{...}
	// response, err := client.SendSms(request)

	return nil
}

// 手动发送测试报警
// SendTestAlert 向指定渠道发送测试报警。
// override 非 nil 时使用前端传入的渠道配置（当前表单值）进行测试，
// 支持「先测试、后保存」；为 nil 时使用当前生效配置。
func (m *Manager) SendTestAlert(channel string, override *config.AlertConfig) error {
	testAlert := &models.Alert{
		CameraID: 0,
		Type:     "test",
		Level:    models.AlertLevelLow,
		Message:  "这是一条测试报警消息",
		Details:  `{"test": true}`,
	}

	src := m.cfg.Alert
	if override != nil {
		src = *override
	}

	switch channel {
	case "webhook":
		return m.sendWebhook(testAlert, &src.Channels.Webhook)
	case "email":
		return m.sendEmail(testAlert, &src.Channels.Email)
	case "sms":
		return m.sendSMS(testAlert, &src.Channels.SMS)
	default:
		return fmt.Errorf("未知渠道: %s", channel)
	}
}

// 批量发送（用于定时汇总等）
func (m *Manager) SendBatch(alerts []*models.Alert) error {
	if len(alerts) == 0 {
		return nil
	}

	// 合并为一条汇总消息
	_ = fmt.Sprintf("监控系统报警汇总 (%d 条)", len(alerts))
	body := "<h3>报警详情:</h3><ul>"
	for _, a := range alerts {
		body += fmt.Sprintf("<li>[%s] %s - %s</li>", a.Level, a.Type, a.Message)
	}
	body += "</ul>"

	// 发送汇总
	return nil
}