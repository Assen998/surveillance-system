package camera

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yourorg/surveillance-system/internal/config"
	"github.com/yourorg/surveillance-system/internal/database"
	"github.com/yourorg/surveillance-system/internal/models"
	"github.com/yourorg/surveillance-system/internal/storage"
	"github.com/yourorg/surveillance-system/pkg/ffmpeg"
	"github.com/yourorg/surveillance-system/pkg/onvif"
	"github.com/yourorg/surveillance-system/pkg/webdav"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// CameraManager 摄像头管理器
type CameraManager struct {
	cfg        *config.Config
	db         *gorm.DB
	cameras    map[uint]*CameraInstance
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	ffmpegMgr  *ffmpeg.Manager
	onvifClient *onvif.Client
	runtimeStorage *storage.RuntimeStorage // 运行时存储设置（设置页热更新）

	// 事件型录像触发防抖：冷却期内重复报警不重复录像
	motionRecordMu   sync.Mutex
	motionRecordLast map[uint]time.Time

	// 定时抓拍运行时设置（设置页热更新，无需重启）
	snapMu       sync.RWMutex
	snapEnabled  bool
	snapInterval int
	snapChanged  chan struct{} // 设置变更通知（容量 1），让抓拍循环立即重排
}

// CameraInstance 运行时摄像头实例
type CameraInstance struct {
	Model       *models.Camera
	Stream      *ffmpeg.Stream
	Preview     *ffmpeg.PreviewStream // 按需启动的 HLS 预览流（子码流，空闲自动停止）
	OnvifClient *onvif.Client         // 每摄像头独立 ONVIF 客户端（避免共享凭据状态竞争）
	Status      string
	LastError   string
	ReconnectCnt int
	StopChan    chan struct{}
	loopDone    chan struct{} // runCamera 循环退出时关闭（保证启停不重叠）

	// 预览按需启停：最后活跃时间 + 独占锁（防并发启停）
	previewLastActive time.Time
	previewMu         sync.Mutex

	// 录像用主码流 / 预览用子码流地址（ONVIF 多 Profile 解析结果）
	RecordRTSPURL  string // 主码流地址（录像用，高清 -c copy）
	PreviewRTSPURL string // 子码流地址（预览用，低分辨率转码）
	mu             sync.Mutex

	// running 表示 runCamera 运行循环是否活跃（独立于 Stream，事件型录像无常驻流）
	running bool
}

// NewCameraManager 创建摄像头管理器
func NewCameraManager(cfg *config.Config, ffmpegMgr *ffmpeg.Manager) *CameraManager {
	ctx, cancel := context.WithCancel(context.Background())

	// 定时抓拍运行时设置（设置页可热更新）；config.yaml 需显式给出 snapshot_enabled
	snapInterval := cfg.Camera.SnapshotInterval
	if snapInterval <= 0 {
		snapInterval = 300
	}

	return &CameraManager{
		cfg:        cfg,
		db:         database.GetDB(),
		cameras:    make(map[uint]*CameraInstance),
		ctx:        ctx,
		cancel:     cancel,
		ffmpegMgr:  ffmpegMgr,
		onvifClient: onvif.NewClient(cfg.Camera.DiscoveryTimeout),
		snapEnabled:  cfg.Camera.SnapshotEnabled,
		snapInterval: snapInterval,
		snapChanged:  make(chan struct{}, 1),
		motionRecordLast: make(map[uint]time.Time),
	}
}

// SetRuntimeStorage 注入运行时存储设置（设置页热更新分段时长/路径/WebDAV）
func (m *CameraManager) SetRuntimeStorage(r *storage.RuntimeStorage) {
	m.runtimeStorage = r
}

// GetSnapshotSettings 获取定时抓拍运行时设置
func (m *CameraManager) GetSnapshotSettings() (enabled bool, interval int) {
	m.snapMu.RLock()
	defer m.snapMu.RUnlock()
	return m.snapEnabled, m.snapInterval
}

// SetSnapshotSettings 热更新定时抓拍设置（设置页调用，立即生效）
func (m *CameraManager) SetSnapshotSettings(enabled bool, interval int) {
	if interval <= 0 {
		interval = 300
	}
	m.snapMu.Lock()
	m.snapEnabled = enabled
	m.snapInterval = interval
	m.snapMu.Unlock()

	// 非阻塞通知抓拍循环立即按新设置重排
	select {
	case m.snapChanged <- struct{}{}:
	default:
	}
}

// localStorage 返回当前生效的本地存储配置
func (m *CameraManager) localStorage() config.LocalStorageConfig {
	if m.runtimeStorage != nil {
		return m.runtimeStorage.GetLocal()
	}
	return m.cfg.Storage.Local
}

// webdavConfig 返回当前生效的 WebDAV 配置
func (m *CameraManager) webdavConfig() config.WebdavConfig {
	if m.runtimeStorage != nil {
		return m.runtimeStorage.GetWebdav()
	}
	return m.cfg.Storage.Webdav
}

// Start 启动管理器
func (m *CameraManager) Start() error {
	// 加载数据库中的摄像头
	if err := m.loadCameras(); err != nil {
		return err
	}

	// 启动所有启用的摄像头
	for id, inst := range m.cameras {
		if inst.Model.RecordEnabled {
			m.wg.Add(1)
			go m.runCamera(id)
		}
	}

	// 启动定时任务
	m.wg.Add(1)
	go m.healthCheckLoop()
	m.wg.Add(1)
	go m.snapshotLoop()
	m.wg.Add(1)
	go m.previewIdleLoop()

	logrus.Info("摄像头管理器启动完成")
	return nil
}

// Stop 停止管理器
func (m *CameraManager) Stop() error {
	m.cancel()
	m.wg.Wait()

	// 停止所有流
	m.mu.Lock()
	var streams []*ffmpeg.Stream
	for _, inst := range m.cameras {
		if s, p := inst.stop(); s != nil {
			streams = append(streams, s)
			if p != nil {
				p.Stop()
			}
		}
	}
	m.mu.Unlock()

	for _, s := range streams {
		s.Stop()
	}

	logrus.Info("摄像头管理器已停止")
	return nil
}

// loadCameras 从数据库加载摄像头
func (m *CameraManager) loadCameras() error {
	var cameras []models.Camera
	if err := m.db.Where("deleted_at IS NULL").Find(&cameras).Error; err != nil {
		return err
	}

	for _, cam := range cameras {
		cam := cam // 捕获循环变量
		m.cameras[cam.ID] = &CameraInstance{
			Model:    &cam,
			Status:   models.CameraStatusOffline,
			StopChan: make(chan struct{}),
			loopDone: make(chan struct{}),
		}
	}
	logrus.Infof("加载了 %d 个摄像头", len(cameras))
	return nil
}

// runCamera 运行单个摄像头
func (m *CameraManager) runCamera(id uint) {
	defer m.wg.Done()

	inst := m.cameras[id]
	cam := inst.Model

	inst.mu.Lock()
	inst.running = true
	inst.mu.Unlock()
	defer func() {
		inst.mu.Lock()
		inst.running = false
		inst.mu.Unlock()
	}()

	// 循环退出时关闭 loopDone（本循环独占的通道，避免与后续重启冲突）
	loopDone := inst.loopDone
	defer func() {
		if loopDone != nil {
			close(loopDone)
		}
	}()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-inst.StopChan:
			return
		default:
			if err := m.connectCamera(inst); err != nil {
				inst.setError(err.Error())
				inst.ReconnectCnt++

				if inst.ReconnectCnt >= m.cfg.Camera.MaxReconnect {
					logrus.Errorf("摄像头 %s 达到最大重连次数，放弃重连", cam.Name)
					m.updateCameraStatus(cam.ID, models.CameraStatusError, err.Error())
					return
				}

				logrus.Warnf("摄像头 %s 连接失败: %v, %d 秒后重试 (%d/%d)",
					cam.Name, err, m.cfg.Camera.ReconnectInterval, inst.ReconnectCnt, m.cfg.Camera.MaxReconnect)

				select {
				case <-m.ctx.Done():
					return
				case <-inst.StopChan:
					return
				case <-time.After(time.Duration(m.cfg.Camera.ReconnectInterval) * time.Second):
					continue
				}
			}

			// 连接成功，重置重连计数
			inst.ReconnectCnt = 0
			m.updateCameraStatus(cam.ID, models.CameraStatusOnline, "")

			// 等待流结束或停止信号
			select {
			case <-m.ctx.Done():
				return
			case <-inst.StopChan:
				return
			case <-inst.streamDone():
				logrus.Infof("摄像头 %s 流结束", cam.Name)
			}
		}
	}
}

// streamDone 返回录像流结束通道；事件型录像（motion）无常驻录像流时返回 nil，
// 调用方应仅等待 ctx/StopChan（移动触发录像与预览均按需、独立管理生命周期）。
func (inst *CameraInstance) streamDone() <-chan struct{} {
	if inst.Stream == nil {
		return nil
	}
	return inst.Stream.Done()
}

// connectCamera 连接摄像头并启动流
func (m *CameraManager) connectCamera(inst *CameraInstance) error {
	cam := inst.Model

	// 构建 RTSP URL（ONVIF 且无 path 时自动获取）
	rtspURL := BuildRTSPURL(cam)
	
	// ONVIF 协议且未指定 path 时，尝试通过 ONVIF 获取真实流地址
	if cam.Protocol == "onvif" && (cam.Path == "" || cam.Path == "/") {
		if cam.OnvifAddress != "" {
			logrus.Infof("摄像头 %s 尝试通过 ONVIF 获取流地址", cam.Name)

			// 为本次连接创建独立的 ONVIF 客户端（避免多摄像头共享凭据状态导致竞争）
			client := onvif.NewClient(m.cfg.Camera.DiscoveryTimeout)
			if cam.Username != "" && cam.Password != "" {
				client.SetCredentials(cam.Username, cam.Password)
			}
			inst.OnvifClient = client

			// === 缓存检查 ===
			// 事件型录像（motion）需要同时解析主/子码流，而缓存只存主码流地址，
			// 故 motion 模式跳过缓存、强制重新发现（保证拿到子码流预览地址）。
			cacheValid := false
			if cam.DiscoveredStreamUri != "" && cam.StreamUriUpdatedAt != nil {
				if time.Since(*cam.StreamUriUpdatedAt) < 24*time.Hour && false { // 所有模式都要解析子码流，故跳过仅存主码流的缓存
					rtspURL = injectRTSPAuth(cam.DiscoveredStreamUri, cam.Username, cam.Password)
					cacheValid = true
					logrus.Infof("摄像头 %s 使用缓存流地址: %s", cam.Name, rtspURL)
				}
			}

			if !cacheValid {
				// 需要重新发现
				logrus.Infof("摄像头 %s 缓存无效/过期/Profile 变更，重新发现流地址", cam.Name)

				// GetProfiles 属于 Media 服务，端点可能不同于 device_service，先解析 Media XAddr
				mediaAddr := client.ResolveMediaXAddr(cam.OnvifAddress)

				// 获取所有 Profile
				profiles, err := client.GetProfiles(mediaAddr)
				if (err != nil || len(profiles) == 0) && mediaAddr != cam.OnvifAddress {
					// 兜底：部分设备 media 服务合并到 device_service
					profiles, err = client.GetProfiles(cam.OnvifAddress)
				}
				if err != nil {
					logrus.Warnf("摄像头 %s 获取 ONVIF 配置失败: %v", cam.Name, err)
				} else if len(profiles) > 0 {
					logrus.Infof("摄像头 %s 发现 %d 个配置文件", cam.Name, len(profiles))
					
					// 选择 Profile：用户指定 > 自动选择
					var selectedProfile onvif.Profile
					if cam.OnvifProfileToken != "" {
						// 用户手动指定
						found := false
						for _, p := range profiles {
							if p.Token == cam.OnvifProfileToken {
								selectedProfile = p
								found = true
								break
							}
						}
						if !found {
							logrus.Warnf("摄像头 %s 指定的 Profile Token %s 不存在，回退自动选择", cam.Name, cam.OnvifProfileToken)
						} else {
							logrus.Infof("摄像头 %s 使用用户指定 Profile: %s", cam.Name, selectedProfile.Name)
						}
					}
					
					if selectedProfile.Token == "" {
						// 自动选择最佳
						selectedProfile = selectBestProfile(profiles)
						logrus.Infof("摄像头 %s 自动选择配置文件: %s (%dx%d)", cam.Name, selectedProfile.Name, selectedProfile.Width, selectedProfile.Height)
					}
					
					// 获取流地址（带重试、降级，优先使用选定的 Profile）
					streamUri, usedProfile, err := client.GetStreamUriWithRetry(
						mediaAddr, 
						profiles,
						selectedProfile.Token,
						onvif.TransportTCP,
						3,
					)
					if err != nil {
						logrus.Warnf("摄像头 %s TCP 获取失败: %v，尝试 UDP", cam.Name, err)
						streamUri, usedProfile, err = client.GetStreamUriWithRetry(
							mediaAddr,
							profiles,
							selectedProfile.Token,
							onvif.TransportUDP,
							3,
						)
						if err != nil {
							logrus.Warnf("摄像头 %s UDP 也失败: %v，使用拼接 URL", cam.Name, err)
						}
					}
					
					if err == nil && streamUri != "" {
						rtspURL = injectRTSPAuth(streamUri, cam.Username, cam.Password)
						// 主码流地址（录像用）；motion 模式下存储供触发录像时拉流
						inst.RecordRTSPURL = rtspURL

						// === 更新缓存 ===
						now := time.Now()
						cam.DiscoveredStreamUri = streamUri
						cam.StreamUriUpdatedAt = &now
						// 保留用户选择的 Profile Token，不用实际使用的覆盖（避免破坏用户意图）
						m.db.Model(cam).Updates(map[string]interface{}{
							"discovered_stream_uri": streamUri,
							"stream_uri_updated_at": now,
						})

						usedName := ""
						if usedProfile != nil {
							usedName = usedProfile.Name
						}
						logrus.Infof("摄像头 %s ONVIF 获取流地址成功 (Profile: %s): %s，已缓存",
							cam.Name, usedName, rtspURL)

						// === 所有模式：额外解析子码流作为预览地址（低分辨率省 CPU/内存） ===
						if len(profiles) > 1 {
							subProfile := selectSubProfile(profiles)
							if subProfile.Token != "" && subProfile.Token != selectedProfile.Token {
								if subUri, _, serr := client.GetStreamUriWithRetry(
									mediaAddr, profiles, subProfile.Token, onvif.TransportTCP, 2,
								); serr == nil && subUri != "" {
									inst.PreviewRTSPURL = injectRTSPAuth(subUri, cam.Username, cam.Password)
									logrus.Infof("摄像头 %s 子码流预览地址 (Profile: %s %dx%d): %s",
										cam.Name, subProfile.Name, subProfile.Width, subProfile.Height, inst.PreviewRTSPURL)
								}
							}
						}
					}
				}
			}
		} else {
			logrus.Warnf("摄像头 %s 未配置 ONVIF 地址，无法自动获取流", cam.Name)
		}
	}

	logrus.Infof("连接摄像头 %s: %s", cam.Name, rtspURL)

	// 主/子码流地址兜底：若未解析到（非 ONVIF 或解析失败），统一回退基础 rtspURL。
	if inst.RecordRTSPURL == "" {
		inst.RecordRTSPURL = rtspURL
	}
	if inst.PreviewRTSPURL == "" {
		inst.PreviewRTSPURL = rtspURL
	}

	motionMode := cam.RecordType == models.RecordTypeMotion

	// 事件型录像（motion）：不常驻录像流，也不常驻预览流；
	// 预览按需启动（子码流），录像由 ONVIF 移动事件临时触发（主码流）。
	// 仅需确认流地址已就绪，然后进入"等待事件"状态（由 runCamera 的 select 兜底）。
	if motionMode {
		inst.setError("")
		logrus.Infof("摄像头 %s 就绪（事件型录像模式：预览按需、移动触发时才录像）", cam.Name)
		return nil
	}

	// 连续/定时录像：常驻录像流（主码流 -c copy，纯录像不内嵌 HLS）。
	// 预览由独立的 PreviewStream（子码流、按需启动）承担，空闲时回收内存。
	stream, err := m.ffmpegMgr.CreateStream(inst.RecordRTSPURL, ffmpeg.StreamOptions{
		CameraID:        cam.ID,
		SegmentDuration: m.localStorage().SegmentDuration,
		OutputDir:       m.getCameraStoragePath(cam.ID),
		OnSegment:       m.onSegmentComplete,
		OnError:         func(err error) { inst.setError(err.Error()) },
		RecordOnly:      true,
	})
	if err != nil {
		return fmt.Errorf("创建录像流失败: %w", err)
	}

	inst.Stream = stream
	inst.setError("")

	if err := stream.Start(); err != nil {
		return fmt.Errorf("启动录像流失败: %w", err)
	}

	logrus.Infof("摄像头 %s 录像流启动成功（主码流 -c copy，预览按需走子码流）", cam.Name)

	return nil
}

// TriggerMotionRecording 移动侦测触发录像：为指定摄像头录制一段固定时长的视频。
// 仅供 RecordType=motion 的摄像头调用；内部做冷却去重，避免重复报警反复触发的重叠录像。
func (m *CameraManager) TriggerMotionRecording(cameraID uint) {
	m.mu.RLock()
	inst, ok := m.cameras[cameraID]
	m.mu.RUnlock()
	if !ok || inst == nil || inst.Model.RecordType != models.RecordTypeMotion {
		return
	}

	duration := m.cfg.Camera.MotionRecord.Duration
	if duration <= 0 {
		duration = 30
	}
	cooldown := m.cfg.Camera.MotionRecord.Cooldown
	if cooldown <= 0 {
		cooldown = 30
	}

	// 冷却去重：冷却期内重复报警不重复录像
	now := time.Now()
	m.motionRecordMu.Lock()
	if last, exists := m.motionRecordLast[cameraID]; exists && now.Sub(last) < time.Duration(cooldown)*time.Second {
		m.motionRecordMu.Unlock()
		logrus.Debugf("摄像头 %s 移动录像冷却中，跳过触发", inst.Model.Name)
		return
	}
	m.motionRecordLast[cameraID] = now
	m.motionRecordMu.Unlock()

	rtspURL := inst.RecordRTSPURL
	if rtspURL == "" {
		rtspURL = BuildRTSPURL(inst.Model)
	}

	go m.runMotionRecording(inst, rtspURL, duration)
}

// runMotionRecording 独立 ffmpeg 进程拉主码流录制固定时长 MP4，完成后入库为 motion 类型。
func (m *CameraManager) runMotionRecording(inst *CameraInstance, rtspURL string, duration int) {
	cam := inst.Model
	outDir := m.getCameraStoragePath(cam.ID)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		logrus.Errorf("创建录像目录失败 camera=%d: %v", cam.ID, err)
		return
	}

	startTime := time.Now()
	filename := fmt.Sprintf("motion_%s.mp4", startTime.Format("20060102_150405"))
	outPath := filepath.Join(outDir, filename)

	args := []string{
		"-y",
		"-rtsp_transport", "tcp",
		"-timeout", "5000000",
		"-i", rtspURL,
		"-c", "copy",        // 流拷贝，不转码（省 CPU）
		"-movflags", "+faststart", // moov 移到文件头：浏览器秒得时长、可拖动进度
		"-t", strconv.Itoa(duration),
		outPath,
	}

	logrus.Infof("摄像头 %s 移动侦测触发录像（%d 秒）: %s", cam.Name, duration, outPath)

	cmd := exec.CommandContext(m.ctx, "ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil && m.ctx.Err() == nil {
		logrus.Errorf("摄像头 %s 移动录像 ffmpeg 失败: %v (输出: %s)", cam.Name, err, tailString(string(output), 300))
		return
	}

	// 统计文件大小
	fi, err := os.Stat(outPath)
	if err != nil {
		logrus.Errorf("摄像头 %s 移动录像文件不存在: %v", cam.Name, err)
		return
	}

	endTime := time.Now()
	recording := &models.Recording{
		CameraID:     cam.ID,
		StartTime:    startTime,
		EndTime:      endTime,
		Duration:     int(endTime.Sub(startTime).Seconds()),
		FilePath:     outPath,
		FileSize:     fi.Size(),
		RecordType:   models.RecordTypeMotion,
		Status:       "completed",
		StorageType:  "local",
		StoragePath:  outPath,
	}

	if err := m.db.Create(recording).Error; err != nil {
		logrus.Errorf("保存移动录像记录失败 camera=%d: %v", cam.ID, err)
		return
	}
	logrus.Infof("移动侦测录像完成: Camera=%d, File=%s, Size=%d", cam.ID, outPath, fi.Size())

	// WebDAV 开启时异步上传
	if wd := m.webdavConfig(); wd.Enabled && wd.URL != "" {
		remoteRel := fmt.Sprintf("camera_%d/%s", cam.ID, filename)
		go m.uploadSegmentToWebdav(recording.ID, cam.ID, outPath, wd, remoteRel)
	}
}

// tailString 截取字符串尾部 n 字节（用于日志，避免刷屏）
func tailString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// onSegmentComplete 录像分段完成回调
func (m *CameraManager) onSegmentComplete(cameraID uint, segment *ffmpeg.SegmentInfo) {
	// 按该段真实录像开始时间重命名：segment_YYYYMMDD_HHMMSS.mp4
	// （原始名 segment_YYYYMMDD_HHMMSS_XX_NNNNNN.mp4 中时间戳为流启动时间，
	//   同一运行期的所有分段共用，不能反映每段实际录像时间）
	filePath := segment.FilePath
	timeStamp := segment.StartTime.Format("20060102_150405")
	newPath := filepath.Join(filepath.Dir(filePath), "segment_"+timeStamp+".mp4")
	if _, err := os.Stat(newPath); err == nil {
		// 目标名已存在（两段恰在同秒开始等极端情况）时追加序号，避免覆盖
		for i := 1; i <= 999; i++ {
			cand := filepath.Join(filepath.Dir(filePath), fmt.Sprintf("segment_%s_%d.mp4", timeStamp, i))
			if _, err := os.Stat(cand); os.IsNotExist(err) {
				newPath = cand
				break
			}
			if i == 999 {
				newPath = filePath // 极端情况放弃重命名
			}
		}
	}
	if newPath != filePath {
		if err := os.Rename(filePath, newPath); err != nil {
			logrus.Warnf("录像文件重命名失败（保留原名）: %v", err)
			newPath = filePath
		}
	}
	segment.FilePath = newPath

	recording := &models.Recording{
		CameraID:     cameraID,
		StartTime:    segment.StartTime,
		EndTime:      segment.EndTime,
		Duration:     int(segment.EndTime.Sub(segment.StartTime).Seconds()),
		FilePath:     segment.FilePath,
		FileSize:     segment.FileSize,
		SegmentIndex: segment.Index,
		RecordType:   models.RecordTypeContinuous,
		Status:       "completed",
		StorageType:  "local",
		StoragePath:  segment.FilePath,
		IndexPath:    segment.IndexPath,
	}

	if err := m.db.Create(recording).Error; err != nil {
		logrus.Errorf("保存录像记录失败: %v", err)
		return
	}
	logrus.Infof("录像分段完成: Camera=%d, File=%s, Size=%d", cameraID, segment.FilePath, segment.FileSize)

	// WebDAV 开启时异步上传该分段
	if wd := m.webdavConfig(); wd.Enabled && wd.URL != "" {
		remoteRel := fmt.Sprintf("camera_%d/%s", cameraID, filepath.Base(segment.FilePath))
		go m.uploadSegmentToWebdav(recording.ID, cameraID, segment.FilePath, wd, remoteRel)
	}
}

// uploadSegmentToWebdav 上传分段到 WebDAV，成功后回写录像记录的远程路径
func (m *CameraManager) uploadSegmentToWebdav(recordingID uint, cameraID uint, localPath string, wd config.WebdavConfig, remoteRel string) {
	client := webdav.NewClient(wd.URL, wd.Username, wd.Password)
	remotePath := wd.BasePath
	if remotePath != "" {
		remotePath += "/" + remoteRel
	} else {
		remotePath = remoteRel
	}

	// 最多重试 3 次
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if err := client.Upload(localPath, remotePath); err != nil {
			lastErr = err
			logrus.Warnf("WebDAV 上传重试 %d/3: camera=%d file=%s err=%v", attempt, cameraID, filepath.Base(localPath), err)
			time.Sleep(time.Duration(attempt*10) * time.Second)
			continue
		}
		// 上传成功，回写远程路径
		m.db.Model(&models.Recording{}).Where("id = ?", recordingID).
			Update("storage_path", remotePath)
		logrus.Infof("WebDAV 上传成功: camera=%d -> %s", cameraID, remotePath)

		// WebDAV 独占模式：上传成功后删除本地副本，本地仅作临时缓冲
		if wd.Only {
			if err := os.Remove(localPath); err == nil {
				logrus.Infof("WebDAV 独占模式: 已删除本地副本 %s", localPath)
			} else if !os.IsNotExist(err) {
				logrus.Warnf("WebDAV 独占模式: 删除本地副本失败 %s: %v", localPath, err)
			}
		}
		return
	}
	logrus.Errorf("WebDAV 上传失败（已重试 3 次）: camera=%d file=%s err=%v", cameraID, filepath.Base(localPath), lastErr)
}

// getCameraStoragePath 获取摄像头存储路径
func (m *CameraManager) getCameraStoragePath(cameraID uint) string {
	return fmt.Sprintf("%s/camera_%d", m.localStorage().RootPath, cameraID)
}

// Snapshot 抓拍一帧：优先复用常驻录像流（连续录像），无则瞬时 RTSP 会话抓帧（事件型录像）。
func (m *CameraManager) Snapshot(cameraID uint) (string, error) {
	m.mu.RLock()
	inst, ok := m.cameras[cameraID]
	m.mu.RUnlock()
	if !ok || inst == nil {
		return "", fmt.Errorf("摄像头不存在")
	}

	// 有常驻录像流时复用（continuous/schedule）
	if inst.Stream != nil && inst.Stream.IsRunning() {
		return inst.Stream.Snapshot()
	}

	// 无常驻流（motion）：瞬时拉主码流抓一帧
	rtspURL := inst.RecordRTSPURL
	if rtspURL == "" {
		rtspURL = inst.PreviewRTSPURL
	}
	if rtspURL == "" {
		rtspURL = BuildRTSPURL(inst.Model)
	}
	outDir := m.getCameraStoragePath(cameraID)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", err
	}
	snapshotPath := filepath.Join(outDir, fmt.Sprintf("snapshot_%d_%d.jpg", cameraID, time.Now().Unix()))
	ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y", "-rtsp_transport", "tcp", "-i", rtspURL, "-vframes", "1", "-q:v", "2", snapshotPath)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("抓拍失败: %w", err)
	}
	return snapshotPath, nil
}

// previewIdleTimeout 预览流空闲超时（秒）：超时后自动停止以回收内存
const previewIdleTimeout = 30 * time.Second

// EnsurePreview 按需启动预览流（子码流 HLS 转码）。前端请求 HLS 播放列表时调用。
// 若预览流未运行则启动；已运行则刷新空闲时间。返回是否可用（启动可能异步尚未产出 m3u8）。
func (m *CameraManager) EnsurePreview(cameraID uint) error {
	m.mu.RLock()
	inst, ok := m.cameras[cameraID]
	m.mu.RUnlock()
	if !ok || inst == nil {
		return fmt.Errorf("摄像头不存在")
	}
	if !inst.Model.RecordEnabled {
		return fmt.Errorf("摄像头未启用")
	}

	inst.previewMu.Lock()
	defer inst.previewMu.Unlock()

	// 已运行：刷新空闲时间即可
	if inst.Preview != nil && inst.Preview.IsRunning() {
		inst.previewLastActive = time.Now()
		return nil
	}

	// 子码流地址兜底（可能尚未解析，回退主码流）
	rtspURL := inst.PreviewRTSPURL
	if rtspURL == "" {
		rtspURL = inst.RecordRTSPURL
	}
	if rtspURL == "" {
		rtspURL = BuildRTSPURL(inst.Model)
	}

	outDir := m.getCameraStoragePath(cameraID)
	inst.Preview = ffmpeg.NewPreviewStream(cameraID, rtspURL, outDir)
	if err := inst.Preview.Start(); err != nil {
		inst.Preview = nil
		return fmt.Errorf("启动预览流失败: %w", err)
	}
	inst.previewLastActive = time.Now()
	logrus.Infof("摄像头 %s 预览流按需启动（子码流: %s）", inst.Model.Name, rtspURL)
	return nil
}

// TouchPreview 刷新预览流空闲时间（HLS 分段请求时调用，保持活跃）
func (m *CameraManager) TouchPreview(cameraID uint) {
	m.mu.RLock()
	inst, ok := m.cameras[cameraID]
	m.mu.RUnlock()
	if !ok || inst == nil {
		return
	}
	inst.previewMu.Lock()
	if inst.Preview != nil && inst.Preview.IsRunning() {
		inst.previewLastActive = time.Now()
	}
	inst.previewMu.Unlock()
}

// stopIdlePreviews 扫描并停止超时未活跃的预览流（由 previewIdleLoop 周期调用）
func (m *CameraManager) stopIdlePreviews() {
	m.mu.RLock()
	instances := make([]*CameraInstance, 0, len(m.cameras))
	for _, inst := range m.cameras {
		instances = append(instances, inst)
	}
	m.mu.RUnlock()

	now := time.Now()
	for _, inst := range instances {
		inst.previewMu.Lock()
		if inst.Preview != nil && inst.Preview.IsRunning() && now.Sub(inst.previewLastActive) > previewIdleTimeout {
			logrus.Infof("摄像头 %s 预览流空闲 %s，自动停止以回收内存", inst.Model.Name, previewIdleTimeout)
			p := inst.Preview
			inst.Preview = nil
			p.Stop()
		}
		inst.previewMu.Unlock()
	}
}

// previewIdleLoop 预览流空闲回收循环
func (m *CameraManager) previewIdleLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.stopIdlePreviews()
		}
	}
}

// healthCheckLoop 健康检查循环
func (m *CameraManager) healthCheckLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkCamerasHealth()
		}
	}
}

func (m *CameraManager) checkCamerasHealth() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for id, inst := range m.cameras {
		if inst.Stream != nil && !inst.Stream.IsHealthy() {
			logrus.Warnf("摄像头 %d 流不健康（数据流中断），触发重连", id)
			// 只杀 ffmpeg 进程，不取消流的 ctx——run 循环检测到退出后自动重连
			inst.Stream.Restart()
		}
	}
}

// snapshotLoop 定时抓拍循环
// 间隔/开关动态读取；设置变更时立即被唤醒重排（设置页热更新即时生效）
func (m *CameraManager) snapshotLoop() {
	defer m.wg.Done()

	for {
		enabled, interval := m.GetSnapshotSettings()
		if interval <= 0 {
			interval = 300
		}
		timer := time.NewTimer(time.Duration(interval) * time.Second)
		select {
		case <-m.ctx.Done():
			timer.Stop()
			return
		case <-m.snapChanged:
			// 设置已变更，按新值重新计时
			timer.Stop()
			continue
		case <-timer.C:
		}
		// 等待期间开关被关闭则跳过本轮
		if !enabled {
			continue
		}
		m.takeSnapshots()
	}
}

func (m *CameraManager) takeSnapshots() {
	m.mu.RLock()
	instances := make([]*CameraInstance, 0, len(m.cameras))
	for _, inst := range m.cameras {
		if inst.Model.RecordEnabled {
			instances = append(instances, inst)
		}
	}
	m.mu.RUnlock()

	for _, inst := range instances {
		go func(inst *CameraInstance) {
			path, err := m.Snapshot(inst.Model.ID)
			if err != nil {
				logrus.Errorf("摄像头 %d 抓拍失败: %v", inst.Model.ID, err)
				return
			}

			snapshot := &models.Snapshot{
				CameraID:    inst.Model.ID,
				Timestamp:   time.Now(),
				FilePath:    path,
				FileSize:    getFileSize(path),
				Type:        "schedule",
				StorageType: "local",
			}
			if err := m.db.Create(snapshot).Error; err != nil {
				logrus.Errorf("保存抓拍记录失败: %v", err)
			}
		}(inst)
	}
}

// GetCamera 获取摄像头信息
func (m *CameraManager) GetCamera(id uint) (*models.Camera, error) {
	var cam models.Camera
	if err := m.db.First(&cam, id).Error; err != nil {
		return nil, err
	}
	return &cam, nil
}

// ListCameras 列出所有摄像头
func (m *CameraManager) ListCameras() ([]models.Camera, error) {
	var cameras []models.Camera
	err := m.db.Where("deleted_at IS NULL").Order("id ASC").Find(&cameras).Error
	return cameras, err
}

// SaveSnapshot 保存一条抓拍记录（手动抓拍入库）
func (m *CameraManager) SaveSnapshot(cameraID uint, path string, fileType string) error {
	snapshot := &models.Snapshot{
		CameraID:    cameraID,
		Timestamp:   time.Now(),
		FilePath:    path,
		FileSize:    getFileSize(path),
		Type:        fileType,
		StorageType: "local",
	}
	return m.db.Create(snapshot).Error
}

// ListSnapshots 分页查询某摄像头的抓拍记录（按时间倒序）
func (m *CameraManager) ListSnapshots(cameraID uint, page, pageSize int) ([]models.Snapshot, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 12
	}
	var total int64
	q := m.db.Model(&models.Snapshot{}).Where("camera_id = ?", cameraID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var snaps []models.Snapshot
	err := q.Order("timestamp DESC, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&snaps).Error
	return snaps, total, err
}

// cloneCamera 复制摄像头模型（运行时实例与 API 请求模型隔离，
// 避免 handler 清空 Password 用于响应时影响正在连接的 goroutine）
func cloneCamera(cam *models.Camera) *models.Camera {
	c := *cam
	if cam.DeviceID != nil {
		d := *cam.DeviceID
		c.DeviceID = &d
	}
	if cam.StreamUriUpdatedAt != nil {
		t := *cam.StreamUriUpdatedAt
		c.StreamUriUpdatedAt = &t
	}
	if cam.LastOnline != nil {
		t := *cam.LastOnline
		c.LastOnline = &t
	}
	c.Recordings = nil
	c.Alerts = nil
	c.Snapshots = nil
	return &c
}

// CreateCamera 创建摄像头
func (m *CameraManager) CreateCamera(cam *models.Camera) error {
	if err := m.db.Create(cam).Error; err != nil {
		return err
	}

	// 添加到运行时（使用副本，与 API 层模型隔离）
	inst := &CameraInstance{
		Model:    cloneCamera(cam),
		Status:   models.CameraStatusOffline,
		StopChan: make(chan struct{}),
		loopDone: make(chan struct{}),
	}
	m.mu.Lock()
	m.cameras[cam.ID] = inst
	m.mu.Unlock()

	// 如果启用录像，启动摄像头
	if cam.RecordEnabled {
		m.wg.Add(1)
		go m.runCamera(cam.ID)
	}
	return nil
}

// UpdateCamera 更新摄像头
// 录像开关变化时，锁内判定状态迁移，锁外执行启停（避免持锁等待循环退出）
func (m *CameraManager) UpdateCamera(cam *models.Camera) error {
	if err := m.db.Save(cam).Error; err != nil {
		return err
	}

	m.mu.Lock()
	inst, ok := m.cameras[cam.ID]
	var needStart, needStop bool
	if ok {
		// 更新运行时实例（使用副本，与 API 层模型隔离）
		inst.Model = cloneCamera(cam)
		streamRunning := inst.running
		needStop = streamRunning && !cam.RecordEnabled
		needStart = !streamRunning && cam.RecordEnabled
	}
	m.mu.Unlock()

	if !ok {
		return nil
	}
	if needStop {
		if err := m.StopCamera(cam.ID); err != nil {
			logrus.Warnf("停止摄像头 %d 流失败: %v", cam.ID, err)
		}
	} else if needStart {
		if err := m.StartCamera(cam.ID); err != nil {
			logrus.Warnf("启动摄像头 %d 流失败: %v", cam.ID, err)
		}
	}
	return nil
}

// DeleteCamera 删除摄像头
func (m *CameraManager) DeleteCamera(id uint) error {
	m.mu.Lock()
	inst, ok := m.cameras[id]
	var stream *ffmpeg.Stream
	if ok {
		stream, _ = inst.stop() // 轻量摘除
		delete(m.cameras, id)
	}
	m.mu.Unlock()

	// 锁外停止流
	if stream != nil {
		stream.Stop()
	}

	return m.db.Delete(&models.Camera{}, id).Error
}

// GetCameraStatus 获取摄像头运行时状态
func (m *CameraManager) GetCameraStatus(id uint) (*CameraInstance, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inst, ok := m.cameras[id]
	return inst, ok
}

// StartCamera 启动指定摄像头的流（录像开关打开时调用）
func (m *CameraManager) StartCamera(id uint) error {
	cam, err := m.GetCamera(id)
	if err != nil {
		return fmt.Errorf("摄像头不存在")
	}
	m.mu.Lock()
	inst, ok := m.cameras[id]
	if ok {
		inst.Model = cloneCamera(cam)
		// 替换 StopChan 与 loopDone（旧的已关闭）
		inst.StopChan = make(chan struct{})
		inst.loopDone = make(chan struct{})
		inst.ReconnectCnt = 0
	} else {
		inst = &CameraInstance{
			Model:    cloneCamera(cam),
			Status:   models.CameraStatusOffline,
			StopChan: make(chan struct{}),
			loopDone: make(chan struct{}),
		}
		m.cameras[id] = inst
	}
	m.mu.Unlock()

	m.wg.Add(1)
	go m.runCamera(id)
	logrus.Infof("启动摄像头 %d 的流", id)
	return nil
}

// StopCamera 停止指定摄像头的流，并等待其运行循环完全退出
func (m *CameraManager) StopCamera(id uint) error {
	m.mu.Lock()
	inst, ok := m.cameras[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("摄像头不存在")
	}
	loopDone := inst.loopDone
	stream, preview := inst.stop() // 轻量摘除（关闭 StopChan + 摘出流）
	m.mu.Unlock()

	// 锁外停止流（可能阻塞数秒）
	if stream != nil {
		stream.Stop()
	}
	if preview != nil {
		preview.Stop()
	}

	// 等待 runCamera 循环退出，避免与新循环并存
	if loopDone != nil {
		select {
		case <-loopDone:
		case <-time.After(15 * time.Second):
			logrus.Warnf("摄像头 %d 运行循环未能在 15 秒内退出", id)
		}
	}
	logrus.Infof("已停止摄像头 %d 的流", id)
	return nil
}

// RestartCamera 重启指定摄像头的流（应用最新存储设置，如分段时长）
func (m *CameraManager) RestartCamera(id uint) error {
	if err := m.StopCamera(id); err != nil {
		return err
	}
	return m.StartCamera(id)
}

// DiscoverONVIFCameras 发现 ONVIF 摄像头
func (m *CameraManager) DiscoverONVIFCameras(network string) ([]*onvif.DeviceInfo, error) {
	return m.onvifClient.Discover(network)
}

// ProbeONVIFCamera 探测单个 IP 的 ONVIF 设备
func (m *CameraManager) ProbeONVIFCamera(ip, username, password string) (*onvif.DeviceInfo, error) {
	client := onvif.NewClient(10)
	if username != "" && password != "" {
		client.SetCredentials(username, password)
	}
	device, authRequired := client.ProbeSingleEx(ip)
	if device != nil {
		return device, nil
	}
	if authRequired {
		return nil, fmt.Errorf("检测到 ONVIF 设备，但需要认证：请提供正确的用户名/密码（若未填写凭据请先补充）")
	}
	return nil, fmt.Errorf("未发现 ONVIF 设备（设备可能离线、未开启 ONVIF 服务，或 IP 地址不正确）")
}

// DiscoverLAN 局域网 ONVIF 设备发现
// 首选 WS-Discovery 组播扫描；若组播不可用或无结果，自动回退到本机网段快速端点扫描。
// 全程无需用户预先输入 IP 或密码（401 认证挑战本身即证明设备为 ONVIF 设备）。
func (m *CameraManager) DiscoverLAN(timeoutSec int) ([]*onvif.DeviceInfo, error) {
	client := onvif.NewClient(timeoutSec)

	// 1) 先尝试 WS-Discovery 组播（使用的等待窗口不超过总超时的约 40%，且最多 5 秒）
	wsWindow := timeoutSec * 2 / 5
	if wsWindow <= 0 {
		wsWindow = 2
	}
	if wsWindow > 5 {
		wsWindow = 5
	}
	if devices, err := client.WSDiscover(wsWindow); err == nil && len(devices) > 0 {
		return devices, nil
	}

	// 2) 回退：本机所在 /24 网段的快速 ONVIF 端点并行扫描
	sweepWindow := time.Duration(timeoutSec) * time.Second
	if sweepWindow < 5*time.Second {
		sweepWindow = 5 * time.Second
	}
	devices, err := client.SweepLocalSubnets(sweepWindow)
	if err != nil {
		return nil, err
	}
	if len(devices) == 0 {
		return nil, fmt.Errorf("未在局域网内发现 ONVIF 设备：请确认摄像头已开启 ONVIF 服务，并与本机处于同一网段")
	}
	return devices, nil
}

// PTZControl PTZ 控制
func (m *CameraManager) PTZControl(cameraID uint, command string, speed float64) error {
	m.mu.RLock()
	inst, ok := m.cameras[cameraID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("摄像头不存在")
	}
	if !inst.Model.PTZEnabled {
		return fmt.Errorf("摄像头不支持 PTZ")
	}
	if inst.Status != models.CameraStatusOnline {
		return fmt.Errorf("摄像头未连接")
	}

	// 优先使用摄像头实例独立的 ONVIF 客户端（已带该摄像头凭据）
	if inst.OnvifClient != nil {
		return inst.OnvifClient.PTZControl(inst.Model.OnvifAddress, command, speed)
	}
	return m.onvifClient.PTZControl(inst.Model.OnvifAddress, command, speed)
}

// updateCameraStatus 更新摄像头状态
func (m *CameraManager) updateCameraStatus(id uint, status, errMsg string) {
	m.db.Model(&models.Camera{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":     status,
		"error_msg":  errMsg,
		"last_online": func() *time.Time { t := time.Now(); return &t }(),
	})

	m.mu.RLock()
	if inst, ok := m.cameras[id]; ok {
		inst.Status = status
		inst.LastError = errMsg
	}
	m.mu.RUnlock()
}

// CameraInstance 方法
// stop 在持有管理器锁的情况下轻量摘除实例：关闭 StopChan + 摘出录像流/预览流对象。
// 返回被摘出的流对象，调用方应在释放管理器锁之后调用 Stop()
// （Stream.Stop/PreviewStream.Stop 可能阻塞数秒等待 ffmpeg 退出，不能在持锁时调用）
func (inst *CameraInstance) stop() (*ffmpeg.Stream, *ffmpeg.PreviewStream) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	select {
	case <-inst.StopChan:
		// 已关闭
	default:
		close(inst.StopChan)
	}

	stream := inst.Stream
	inst.Stream = nil

	// 停止按需预览流
	preview := inst.Preview
	inst.Preview = nil

	inst.Status = models.CameraStatusOffline
	return stream, preview
}

func (inst *CameraInstance) setError(err string) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	inst.LastError = err
	if err != "" {
		inst.Status = models.CameraStatusError
	}
}

// selectBestProfile 选择最佳配置文件（优先主码流：分辨率最高、名称含 main/high/primary）
func selectBestProfile(profiles []onvif.Profile) onvif.Profile {
	if len(profiles) == 1 {
		return profiles[0]
	}

	// 评分选择
	bestIdx := 0
	bestScore := -1

	for i, p := range profiles {
		score := 0

		// 分辨率得分（主码流通常分辨率最高）
		score += p.Width * p.Height / 10000

		// 名称关键词加分
		name := strings.ToLower(p.Name)
		if strings.Contains(name, "main") || strings.Contains(name, "primary") || strings.Contains(name, "high") {
			score += 1000
		}
		if strings.Contains(name, "sub") || strings.Contains(name, "secondary") || strings.Contains(name, "low") {
			score -= 500
		}

		// H.264/H.265 编码加分
		if p.Codec == "h264" || p.Codec == "h265" {
			score += 100
		}

		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	return profiles[bestIdx]
}

// selectSubProfile 选择子码流（预览用，最低分辨率；名称含 sub/low/secondary 优先）
func selectSubProfile(profiles []onvif.Profile) onvif.Profile {
	if len(profiles) == 0 {
		return onvif.Profile{}
	}
	if len(profiles) == 1 {
		return profiles[0]
	}

	bestIdx := 0
	bestScore := int(^uint(0) >> 1) // 初始化为最大整数

	for i, p := range profiles {
		score := p.Width * p.Height

		name := strings.ToLower(p.Name)
		if strings.Contains(name, "sub") || strings.Contains(name, "secondary") || strings.Contains(name, "low") {
			score -= 1000000 // 名称明确是子码流，大幅降分使其优先
		}
		if strings.Contains(name, "main") || strings.Contains(name, "primary") || strings.Contains(name, "high") {
			continue // 明确是主码流，跳过
		}

		if score < bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	return profiles[bestIdx]
}

// injectRTSPAuth 若 RTSP 地址不含凭据则注入（ONVIF 发现的流地址通常不含账号密码）
func injectRTSPAuth(rtspURL, username, password string) string {
	if username == "" || rtspURL == "" {
		return rtspURL
	}
	// 已包含凭据（host 部分有 @）
	if idx := strings.Index(rtspURL, "://"); idx > 0 {
		rest := rtspURL[idx+3:]
		slash := strings.Index(rest, "/")
		host := rest
		if slash > 0 {
			host = rest[:slash]
		}
		if strings.Contains(host, "@") {
			return rtspURL
		}
	} else {
		return rtspURL
	}

	auth := username
	if password != "" {
		auth += ":" + password
	}
	idx := strings.Index(rtspURL, "://")
	return rtspURL[:idx+3] + auth + "@" + rtspURL[idx+3:]
}

// BuildRTSPURL 构建 RTSP 地址（导出供其他包使用）
func BuildRTSPURL(c *models.Camera) string {
	auth := ""
	if c.Username != "" && c.Password != "" {
		auth = fmt.Sprintf("%s:%s@", c.Username, c.Password)
	}
	path := c.Path
	if path == "" {
		path = "/stream1"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return fmt.Sprintf("rtsp://%s%s:%d%s", auth, c.IP, c.Port, path)
}

func getFileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}