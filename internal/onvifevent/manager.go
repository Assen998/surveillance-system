package onvifevent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yourorg/surveillance-system/internal/config"
	"github.com/yourorg/surveillance-system/internal/database"
	"github.com/yourorg/surveillance-system/internal/models"
	"github.com/yourorg/surveillance-system/pkg/onvif"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// Manager ONVIF 事件订阅管理器：为协议为 onvif 的摄像头订阅事件服务，
// 周期拉取摄像头主动上报的报警（移动侦测/越线/入侵等），转换为系统报警记录。
type Manager struct {
	cfg    *config.Config
	db     *gorm.DB
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// onAlert 可选回调，用于将报警转发给推送管理器（webhook/email 等）
	onAlert func(*models.Alert)

	// 去重：cameraID -> topic -> 最近落库时间，避免同一事件的重复拉取造成刷屏
	mu         sync.Mutex
	lastRecord map[uint]map[string]time.Time
}

func NewManager(cfg *config.Config, onAlert func(*models.Alert)) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		cfg:        cfg,
		db:         database.GetDB(),
		ctx:        ctx,
		cancel:     cancel,
		onAlert:    onAlert,
		lastRecord: make(map[uint]map[string]time.Time),
	}
}

// pollInterval 拉取间隔（秒），配置为空时用默认值 10
func (m *Manager) pollInterval() int {
	if m.cfg.Camera.OnvifEvent.PollInterval > 0 {
		return m.cfg.Camera.OnvifEvent.PollInterval
	}
	return 10
}

// Start 启动事件订阅；加载所有 protocol=onvif 的摄像头并各自订阅
func (m *Manager) Start() error {
	if !m.cfg.Camera.OnvifEvent.Enabled {
		logrus.Info("ONVIF 事件订阅未启用，跳过启动")
		return nil
	}

	var cams []models.Camera
	if err := m.db.Where("protocol = ? AND deleted_at IS NULL", "onvif").Find(&cams).Error; err != nil {
		return fmt.Errorf("查询 onvif 摄像头失败: %w", err)
	}

	if len(cams) == 0 {
		logrus.Info("没有 onvif 协议摄像头，跳过事件订阅")
		return nil
	}

	for _, cam := range cams {
		if cam.OnvifAddress == "" {
			logrus.Warnf("摄像头 %s 未配置 ONVIF 地址，跳过事件订阅", cam.Name)
			continue
		}
		cam := cam
		m.wg.Add(1)
		go m.subscribeLoop(cam)
	}
	logrus.Infof("ONVIF 事件订阅已启动，共 %d 个摄像头", len(cams))
	return nil
}

func (m *Manager) Stop() error {
	m.cancel()
	m.wg.Wait()
	logrus.Info("ONVIF 事件订阅管理器已停止")
	return nil
}

// subscribeLoop 单个摄像头的事件订阅循环：订阅 → 拉取 → 到期/失败重建
func (m *Manager) subscribeLoop(cam models.Camera) {
	defer m.wg.Done()

	client := onvif.NewClient(m.cfg.Camera.DiscoveryTimeout)
	if cam.Username != "" && cam.Password != "" {
		client.SetCredentials(cam.Username, cam.Password)
	}

	for {
		if m.ctx.Err() != nil {
			return
		}

		sub, err := client.CreatePullPointSubscription(cam.OnvifAddress)
		if err != nil {
			// 兜底：设备可能将 Events 服务独立于 device_service
			if eventsAddr := client.ResolveEventsXAddr(cam.OnvifAddress); eventsAddr != cam.OnvifAddress {
				sub, err = client.CreatePullPointSubscription(eventsAddr)
			}
		}
		if err != nil {
			logrus.Warnf("摄像头 %s 事件订阅失败: %v，%d 秒后重试", cam.Name, err, m.pollInterval())
			if !sleepCtx(m.ctx, time.Duration(m.pollInterval())*time.Second) {
				return
			}
			continue
		}

		logrus.Infof("摄像头 %s 事件订阅成功: %s", cam.Name, sub.Address)

		// 拉取循环
		active := m.pullLoop(client, cam, sub)
		// 取消订阅（忽略错误）
		_ = client.Unsubscribe(sub.Address)

		if !active {
			// 正常退出（ctx 取消）
			return
		}
		// active == true 表示因错误退出，短暂等待后重新订阅
		if !sleepCtx(m.ctx, time.Duration(m.pollInterval())*time.Second) {
			return
		}
	}
}

// pullLoop 持续阻塞拉取事件，返回 false 表示因 ctx 取消而退出（正常），
// true 表示因错误退出（需重新订阅）。
func (m *Manager) pullLoop(client *onvif.Client, cam models.Camera, sub *onvif.EventSubscription) bool {
	poll := m.pollInterval()

	// 订阅终止时间：到点主动重建，避免设备端失联
	var deadline time.Time
	if !sub.TerminationTime.IsZero() {
		deadline = sub.TerminationTime
	} else if m.cfg.Camera.OnvifEvent.SubscriptionTimeout > 0 {
		deadline = time.Now().Add(time.Duration(m.cfg.Camera.OnvifEvent.SubscriptionTimeout) * time.Minute)
	}

	for {
		if m.ctx.Err() != nil {
			return false
		}

		events, err := client.PullMessages(sub.Address, poll)
		if err != nil {
			if m.ctx.Err() != nil {
				return false
			}
			logrus.Warnf("摄像头 %s 拉取事件失败: %v", cam.Name, err)
			return true
		}

		for _, ev := range events {
			m.handleEvent(cam, ev)
		}

		// 订阅到期：返回 true 触发重新订阅
		if !deadline.IsZero() && time.Now().After(deadline) {
			logrus.Infof("摄像头 %s 事件订阅到期，重新订阅", cam.Name)
			return true
		}
	}
}

// handleEvent 将 ONVIF 事件转换为报警记录并落库
func (m *Manager) handleEvent(cam models.Camera, ev onvif.OnvifEvent) {
	alertType, level, isAlarm := mapTopicToAlert(ev.Topic)
	// 非报警类事件（监控/状态上报等）直接忽略，避免误报刷屏
	if !isAlarm {
		logrus.Debugf("忽略非报警 ONVIF 事件: Camera=%s topic=%s", cam.Name, ev.Topic)
		return
	}

	// 去重：同一摄像头同类事件在冷却期内只记录一条
	if m.duplicated(cam.ID, alertType) {
		return
	}

	msg := buildMessage(alertType, ev)
	details := buildDetails(ev)

	alert := &models.Alert{
		CameraID: cam.ID,
		Type:     alertType,
		Level:    level,
		Message:  msg,
		Details:  details,
		Status:   models.AlertStatusNew,
	}

	if err := m.db.Create(alert).Error; err != nil {
		logrus.Errorf("保存 ONVIF 报警失败 camera=%d: %v", cam.ID, err)
		return
	}
	logrus.Infof("ONVIF 报警: Camera=%s(%d), Type=%s, Level=%s, Topic=%s", cam.Name, cam.ID, alertType, level, ev.Topic)

	// 转发给推送管理器（可选）
	if m.onAlert != nil {
		m.onAlert(alert)
	}
}

// duplicated 冷却去重：同一摄像头同一类型 30 秒内只记录一条
func (m *Manager) duplicated(cameraID uint, alertType string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	byType, ok := m.lastRecord[cameraID]
	if !ok {
		byType = make(map[string]time.Time)
		m.lastRecord[cameraID] = byType
	}
	last, exists := byType[alertType]
	now := time.Now()
	if exists && now.Sub(last) < 30*time.Second {
		return true
	}
	byType[alertType] = now
	return false
}

// sleepCtx 带 ctx 取消的睡眠，返回 false 表示 ctx 已取消
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// mapTopicToAlert 将 ONVIF 事件主题映射为系统报警类型与等级。
// 第三个返回值 isAlarm 表示该主题是否为"报警"事件；false 表示监控/状态类事件，应忽略。
func mapTopicToAlert(topic string) (alertType string, level string, isAlarm bool) {
	t := strings.ToLower(topic)

	// 先判断是否为非报警类主题（监控、心跳、统计等），直接忽略
	if strings.HasPrefix(t, "monitoring/") ||
		strings.Contains(t, "processorusage") ||
		strings.Contains(t, "memoryusage") ||
		strings.Contains(t, "device/io") ||
		strings.Contains(t, "systemlog") {
		return "", "", false
	}

	switch {
	case strings.Contains(t, "linecross"), strings.Contains(t, "linedetector"), strings.Contains(t, "crossed"):
		return models.AlertTypeLineCross, models.AlertLevelHigh, true
	case strings.Contains(t, "intrusion"), strings.Contains(t, "objectsinside"),
		strings.Contains(t, "fielddetector"), strings.Contains(t, "enter"), strings.Contains(t, "invasion"):
		return models.AlertTypeIntrusion, models.AlertLevelHigh, true
	case strings.Contains(t, "tamper"), strings.Contains(t, "spray"), strings.Contains(t, "covered"):
		return models.AlertTypeObjectDetect, models.AlertLevelHigh, true
	case strings.Contains(t, "motion"):
		// motion / motionregion / videomotion / motionalarm 等
		return models.AlertTypeMotion, models.AlertLevelMedium, true
	case strings.Contains(t, "digitalinput"), strings.Contains(t, "trigger"):
		return models.AlertTypeIntrusion, models.AlertLevelMedium, true
	case strings.Contains(t, "disconnect"), strings.Contains(t, "removed"), strings.Contains(t, "networklost"):
		return models.AlertTypeOffline, models.AlertLevelCritical, true
	default:
		// 未识别的主题：不当作报警，避免误报
		return "", "", false
	}
}

// buildMessage 生成人类可读的报警描述
func buildMessage(alertType string, ev onvif.OnvifEvent) string {
	if ev.Topic != "" {
		return fmt.Sprintf("摄像头上报事件: %s", ev.Topic)
	}
	switch alertType {
	case models.AlertTypeMotion:
		return "摄像头上报：检测到移动"
	case models.AlertTypeLineCross:
		return "摄像头上报：越线报警"
	case models.AlertTypeIntrusion:
		return "摄像头上报：区域入侵报警"
	case models.AlertTypeObjectDetect:
		return "摄像头上报：设备破坏/遮挡报警"
	case models.AlertTypeOffline:
		return "摄像头上报：设备断开报警"
	default:
		return "摄像头上报报警事件"
	}
}

// buildDetails 将事件键值对序列化为 JSON 字符串
func buildDetails(ev onvif.OnvifEvent) string {
	if len(ev.Items) == 0 {
		return fmt.Sprintf(`{"topic": %q}`, ev.Topic)
	}
	var sb strings.Builder
	_, _ = fmt.Fprintf(&sb, `{"topic": %q, "items": {`, ev.Topic)
	i := 0
	for k, v := range ev.Items {
		if i > 0 {
			sb.WriteByte(',')
		}
		_, _ = fmt.Fprintf(&sb, "%q: %q", k, v)
		i++
	}
	sb.WriteString(`}}`)
	return sb.String()
}