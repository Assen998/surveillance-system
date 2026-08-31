package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"

	"github.com/yourorg/surveillance-system/internal/alert"
	"github.com/yourorg/surveillance-system/internal/camera"
	"github.com/yourorg/surveillance-system/internal/config"
	"github.com/yourorg/surveillance-system/internal/database"
	"github.com/yourorg/surveillance-system/internal/models"
	"github.com/yourorg/surveillance-system/internal/storage"
	embedui "github.com/yourorg/surveillance-system"
	"github.com/yourorg/surveillance-system/pkg/webdav"
)

type Server struct {
	cfg          *config.Config
	cameraMgr    *camera.CameraManager
	storageMgr   *storage.Manager
	alertMgr     *alert.Manager
	router       *gin.Engine
	wsUpgrader   websocket.Upgrader
	runtimeStorage *storage.RuntimeStorage
	cfgPath      string // config.yaml 路径，用于设置持久化
}

// SetRuntimeStorage 注入运行时存储设置
func (s *Server) SetRuntimeStorage(r *storage.RuntimeStorage) {
	s.runtimeStorage = r
}

// SetConfigPath 设置配置文件路径（设置保存时持久化用）
func (s *Server) SetConfigPath(p string) {
	s.cfgPath = p
}

func NewServer(cfg *config.Config, cameraMgr *camera.CameraManager, storageMgr *storage.Manager, alertMgr *alert.Manager) *Server {
	s := &Server{
		cfg:        cfg,
		cameraMgr:  cameraMgr,
		storageMgr: storageMgr,
		alertMgr:   alertMgr,
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}
	s.setupRoutes()
	return s
}

func (s *Server) Router() *gin.Engine {
	return s.router
}

func (s *Server) setupRoutes() {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(s.loggerMiddleware())
	r.Use(s.corsMiddleware())

	// 静态文件（前端构建产物，已通过 go:embed 嵌入单二进制）
	distFS, distErr := embedui.Dist()
	if distErr != nil {
		logrus.Warnf("加载内嵌前端资源失败（未执行前端构建？）: %v", distErr)
	}
	if distErr == nil {
		// /static 静态资源
		staticFS, _ := fs.Sub(distFS, "static")
		r.StaticFS("/static", http.FS(staticFS))
		// index.html 禁止缓存，确保前端发版后立即生效
		indexData, _ := fs.ReadFile(distFS, "index.html")
		r.GET("/", func(c *gin.Context) {
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Data(http.StatusOK, "text/html; charset=utf-8", indexData)
		})
		// favicon
		if faviconData, err := fs.ReadFile(distFS, "favicon.ico"); err == nil {
			r.GET("/favicon.ico", func(c *gin.Context) {
				c.Data(http.StatusOK, "image/x-icon", faviconData)
			})
		}
	}

	// 健康检查
	r.GET("/health", s.healthCheck)
	r.GET("/api/version", s.getVersion)

	// API v1
	v1 := r.Group("/api/v1")
	{
		// 认证接口（公开）
		auth := v1.Group("/auth")
		{
			auth.POST("/login", s.login)
			auth.POST("/logout", s.logout)
			auth.GET("/me", s.authMiddleware(), s.getCurrentUser)
			auth.PUT("/password", s.authMiddleware(), s.changePassword)
		}

		// 受保护的 API 组
		secured := v1.Group("")
		secured.Use(s.authMiddleware())
		{
			// 摄像头管理
			cameras := secured.Group("/cameras")
			{
				cameras.GET("", s.listCameras)
				cameras.POST("", s.createCamera)
				cameras.GET("/:id", s.getCamera)
				cameras.PUT("/:id", s.updateCamera)
				cameras.DELETE("/:id", s.deleteCamera)
				cameras.GET("/:id/status", s.getCameraStatus)
				cameras.POST("/:id/start", s.startCamera)
				cameras.POST("/:id/stop", s.stopCamera)
				cameras.POST("/:id/restart", s.restartCamera)
				cameras.POST("/:id/snapshot", s.takeSnapshot)
				cameras.POST("/:id/ptz", s.ptzControl)
				cameras.GET("/:id/snapshots", s.listSnapshots)
				cameras.GET("/discover", s.discoverCameras)
				cameras.POST("/discover/lan", s.discoverLAN)
				cameras.GET("/probe", s.probeONVIFCamera)
			}

			// 系统设置
			settings := secured.Group("/settings")
			{
				settings.GET("/storage", s.getStorageSettings)
				settings.PUT("/storage", s.updateStorageSettings)
				settings.GET("/camera", s.getCameraSettings)
				settings.PUT("/camera", s.updateCameraSettings)
				settings.POST("/webdav/test", s.testWebdav)
			}

			// 录像管理
			recordings := secured.Group("/recordings")
			{
				recordings.GET("", s.listRecordings)
				recordings.GET("/:id", s.getRecording)
				recordings.DELETE("/:id", s.deleteRecording)
				recordings.GET("/camera/:cameraId", s.listCameraRecordings)
				recordings.GET("/camera/:cameraId/segments", s.getRecordingSegments)
			}

			// 抓拍列表（全局管理）
			snapshots := secured.Group("/snapshots")
			{
				snapshots.GET("", s.listAllSnapshots)
				snapshots.DELETE("", s.clearSnapshots)
				snapshots.DELETE("/:id", s.deleteSnapshot)
			}

			// 智能分析
			analytics := secured.Group("/analytics")
			{
				analytics.GET("/alerts", s.listAlerts)
				analytics.DELETE("/alerts", s.clearAlerts)
				analytics.GET("/alerts/:id", s.getAlert)
				analytics.PUT("/alerts/:id/ack", s.acknowledgeAlert)
				analytics.PUT("/alerts/:id/resolve", s.resolveAlert)
				analytics.DELETE("/alerts/:id", s.deleteAlert)
			}

			// 存储管理
			storage := secured.Group("/storage")
			{
				storage.GET("/stats", s.getStorageStats)
				storage.POST("/cleanup", s.triggerCleanup)
			}

			// 报警配置
			alerts := secured.Group("/alerts")
			{
				alerts.GET("/config", s.getAlertConfig)
				alerts.PUT("/config", s.updateAlertConfig)
				alerts.POST("/test", s.sendTestAlert)
			}

			// 系统配置
			system := secured.Group("/system")
			{
				system.GET("/config", s.getSystemConfig)
				system.PUT("/config", s.updateSystemConfig)
				system.GET("/info", s.getSystemInfo)
				system.POST("/restart", s.restartSystem)
			}
		}

		// 媒体流组：浏览器通过 <video>/<img>/HLS.js 直接加载，无法携带 Authorization 头，
		// 改用 mediaAuthMiddleware（支持 ?token= 查询参数）。
		media := v1.Group("")
		media.Use(s.mediaAuthMiddleware())
		{
			// 录像文件（回放/下载）
			media.GET("/recordings/:id/file", s.getRecordingFile)
			media.HEAD("/recordings/:id/file", s.getRecordingFile)
			media.GET("/recordings/:id/download", s.downloadRecording)

			// WebDAV 远程录像（浏览 + 流式播放，?token= 鉴权）
			media.GET("/webdav/list", s.listWebdavFiles)
			media.GET("/webdav/file", s.streamWebdavFile)

			// 回放/流媒体
			stream := media.Group("/stream")
			{
				stream.GET("/camera/:cameraId/hls", s.getHLSPlaylist)
				stream.GET("/camera/:cameraId/hls/*file", s.getHLSSegment)
				stream.GET("/camera/:cameraId/mp4", s.getMP4Segment)
				stream.GET("/camera/:cameraId/snapshot", s.getLatestSnapshot)
				stream.GET("/camera/:cameraId/snapshots/*file", s.getSnapshotFile)
				stream.GET("/camera/:cameraId/recordings/:recordingId/hls", s.getRecordingHLS)
			}
		}
	}

	// WebSocket
	r.GET("/ws", s.handleWebSocket)
	r.GET("/api/v1/ws/camera/:cameraId", s.handleCameraWS)

	// SPA 回退：所有非 API、非静态、非 WebSocket 路由返回 index.html
	r.NoRoute(func(c *gin.Context) {
		// 排除 API、静态资源、WebSocket、健康检查
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") ||
			strings.HasPrefix(path, "/static/") ||
			strings.HasPrefix(path, "/ws") ||
			path == "/health" ||
			path == "/favicon.ico" {
			c.Status(http.StatusNotFound)
			return
		}
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		if distErr == nil {
			data, _ := fs.ReadFile(distFS, "index.html")
			c.Data(http.StatusOK, "text/html; charset=utf-8", data)
			return
		}
		c.File("./web/dist/index.html")
	})

	s.router = r
}

// 中间件
func (s *Server) loggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)
		logrus.WithFields(logrus.Fields{
			"method":   c.Request.Method,
			"path":     c.Request.URL.Path,
			"status":   c.Writer.Status(),
			"latency":  latency,
			"clientIP": c.ClientIP(),
		}).Info("HTTP Request")
	}
}

func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// ========== 认证：HMAC 签名令牌 ==========

// authSecret 令牌签名密钥（生产环境应通过配置注入并保密）
var authSecret = []byte("surveillance-system-secret-change-in-production")

const tokenTTL = 24 * time.Hour

// makeToken 生成 "payload.signature" 形式的令牌
// payload = base64(username|role|expiresUnix)
func makeToken(username, role string, ttl time.Duration) string {
	expires := time.Now().Add(ttl).Unix()
	payload := fmt.Sprintf("%s|%s|%d", username, role, expires)
	payloadB64 := base64.RawURLEncoding.EncodeToString([]byte(payload))
	mac := hmac.New(sha256.New, authSecret)
	mac.Write([]byte(payloadB64))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payloadB64 + "." + sig
}

// parseToken 校验令牌，返回 username, role
func parseToken(token string) (string, string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("令牌格式错误")
	}
	mac := hmac.New(sha256.New, authSecret)
	mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(expected), []byte(parts[1])) != 1 {
		return "", "", fmt.Errorf("令牌签名无效")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "", fmt.Errorf("令牌解码失败")
	}
	segs := strings.Split(string(payloadBytes), "|")
	if len(segs) != 3 {
		return "", "", fmt.Errorf("令牌内容错误")
	}
	expires, err := strconv.ParseInt(segs[2], 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return "", "", fmt.Errorf("令牌已过期")
	}
	return segs[0], segs[1], nil
}

// randomBytes 生成随机字节（用于重置密钥等场景）
func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

// 认证中间件：校验 Authorization: Bearer <token>
func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenHeader := c.GetHeader("Authorization")
		if tokenHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录或缺少认证令牌"})
			c.Abort()
			return
		}
		tokenString := strings.TrimPrefix(tokenHeader, "Bearer ")
		username, role, err := parseToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "令牌无效或已过期，请重新登录"})
			c.Abort()
			return
		}
		c.Set("username", username)
		c.Set("role", role)
		c.Next()
	}
}

// mediaAuthMiddleware 媒体流鉴权：浏览器通过 <video>/<img>/HLS.js 直接加载媒体文件时
// 无法自定义 Authorization 请求头，因此额外支持 ?token=<token> 查询参数携带令牌。
// 优先用 Authorization 头，其次用 query token。
func (s *Server) mediaAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := ""
		if h := c.GetHeader("Authorization"); h != "" {
			tokenString = strings.TrimPrefix(h, "Bearer ")
		}
		if tokenString == "" {
			tokenString = c.Query("token")
		}

		username, role, err := parseToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录或令牌无效，请重新登录"})
			c.Abort()
			return
		}
		c.Set("username", username)
		c.Set("role", role)
		c.Next()
	}
}

// ========== 健康检查 ==========
func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
		"version":   "1.0.0",
	})
}

func (s *Server) getVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version":   "1.0.0",
		"buildTime": "2024-01-01",
		"gitCommit": "dev",
	})
}

// ========== 认证 ==========
func (s *Server) login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 从数据库查询用户并校验密码（bcrypt）
	var user models.User
	if err := database.GetDB().Where("username = ?", req.Username).First(&user).Error; err != nil {
		logrus.Warnf("登录失败：用户不存在 username=%s", req.Username)
		// 执行一次 dummy 比对，避免时序探测用户名是否存在
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$dummyhashdummyhashdummyhashdummyhas0000000000000000000000"), []byte(req.Password))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}
	if user.Status != "active" {
		c.JSON(http.StatusForbidden, gin.H{"error": "账号已被禁用"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		logrus.Warnf("登录失败：密码错误 username=%s", req.Username)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}

	// 更新最后登录时间
	now := time.Now()
	database.GetDB().Model(&user).Update("last_login", &now)

	token := makeToken(user.Username, user.Role, tokenTTL)
	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user": gin.H{
			"username": user.Username,
			"role":     user.Role,
		},
	})
}

func (s *Server) logout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "登出成功"})
}

func (s *Server) getCurrentUser(c *gin.Context) {
	username, _ := c.Get("username")
	role, _ := c.Get("role")
	c.JSON(http.StatusOK, gin.H{
		"username":    username,
		"role":        role,
		"permissions": []string{"view", "control", "config", "admin"},
	})
}

func (s *Server) changePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	usernameVal, _ := c.Get("username")
	username, _ := usernameVal.(string)

	var user models.User
	if err := database.GetDB().Where("username = ?", username).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "原密码错误"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
		return
	}
	if err := database.GetDB().Model(&user).Update("password", string(hash)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "密码修改成功"})
}

// ========== 摄像头管理 ==========
func (s *Server) listCameras(c *gin.Context) {
	cameras, err := s.cameraMgr.ListCameras()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 隐藏密码
	for i := range cameras {
		cameras[i].Password = ""
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  cameras,
		"total": len(cameras),
	})
}

// cameraRequest 创建/更新摄像头的请求 DTO（包含 password；
// models.Camera.Password 带 json:"-" 无法通过 ShouldBindJSON 绑定）
type cameraRequest struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Protocol    string  `json:"protocol"`
	IP          string  `json:"ip"`
	Port        int     `json:"port"`
	Username    string  `json:"username"`
	Password    string  `json:"password"`
	Path        string  `json:"path"`
	OnvifAddress      string  `json:"onvif_address"`
	OnvifProfileToken string  `json:"onvif_profile_token"`
	// ONVIF 设备信息（可选：后端 ONVIF 连接时自动抓取入库；前端探测后也可传入）
	Manufacturer  string `json:"manufacturer"`
	Model         string `json:"model"`
	Firmware      string `json:"firmware"`
	SerialNumber  string `json:"serial_number"`
	DeviceID    *string `json:"device_id"`
	// 指针类型：nil = 未传（更新时保留原值），避免不完整 payload 翻转 bool 字段
	RecordEnabled   *bool   `json:"record_enabled"`
	RecordSchedule  string  `json:"record_schedule"`
	RecordType      string  `json:"record_type"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	FPS         int    `json:"fps"`
	Bitrate     int    `json:"bitrate"`
	Codec       string `json:"codec"`
	PTZEnabled  *bool  `json:"ptz_enabled"`
}

func (r *cameraRequest) toModel() *models.Camera {
	rec := false
	if r.RecordEnabled != nil {
		rec = *r.RecordEnabled
	}
	ptz := false
	if r.PTZEnabled != nil {
		ptz = *r.PTZEnabled
	}
	return &models.Camera{
		Name:                r.Name,
		Description:         r.Description,
		Protocol:            r.Protocol,
		IP:                  r.IP,
		Port:                r.Port,
		Username:            r.Username,
		Password:            r.Password,
		Path:                r.Path,
		OnvifAddress:        r.OnvifAddress,
		OnvifProfileToken:   r.OnvifProfileToken,
		Manufacturer:        r.Manufacturer,
		Model:               r.Model,
		Firmware:            r.Firmware,
		SerialNumber:        r.SerialNumber,
		DeviceID:            r.DeviceID,
		RecordEnabled:       rec,
		RecordSchedule:      r.RecordSchedule,
		RecordType:          r.RecordType,
		Width:               r.Width,
		Height:              r.Height,
		FPS:                 r.FPS,
		Bitrate:             r.Bitrate,
		Codec:               r.Codec,
		PTZEnabled:          ptz,
	}
}

func (s *Server) createCamera(c *gin.Context) {
	var req cameraRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cam := req.toModel()

	// RTSP/ONVIF 不需要 device_id，避免唯一索引冲突
	if cam.Protocol != "gb28181" && (cam.DeviceID == nil || *cam.DeviceID == "") {
		cam.DeviceID = nil
	}

	// 测试连接（仅 RTSP，ONVIF 由摄像头连接时自动发现流地址）
	if cam.Protocol == "rtsp" {
		rtspURL := camera.BuildRTSPURL(cam)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "ffprobe",
			"-v", "error",
			"-select_streams", "v:0",
			"-show_entries", "stream=codec_name,width,height,r_frame_rate",
			"-of", "json",
			"-rtsp_transport", "tcp",
			// 不加 -timeout/-stimeout（跨版本兼容；ffprobe 由上方 10s Go context 超时兜底）
			rtspURL,
		)

		output, err := cmd.Output()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("摄像头连接测试失败: %v，请检查 IP/端口/路径/账号密码是否正确", err),
			})
			return
		}

		// 解析 ffprobe 输出验证有视频流
		var probeResult map[string]interface{}
		if err := json.Unmarshal(output, &probeResult); err != nil || len(probeResult) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "摄像头连接测试失败: 无法解析视频流信息"})
			return
		}
	}

	if err := s.cameraMgr.CreateCamera(cam); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	cam.Password = ""
	c.JSON(http.StatusCreated, cam)
}

func (s *Server) getCamera(c *gin.Context) {
	id := c.Param("id")
	cam, err := s.cameraMgr.GetCamera(parseUint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "摄像头不存在"})
		return
	}
	cam.Password = ""
	c.JSON(http.StatusOK, cam)
}

func (s *Server) updateCamera(c *gin.Context) {
	id := c.Param("id")
	var req cameraRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	camID := parseUint(id)

	// 先取出现有摄像头，用于保留未被前端提交的状态/缓存字段
	existing, err := s.cameraMgr.GetCamera(camID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "摄像头不存在"})
		return
	}

	cam := req.toModel()
	cam.ID = existing.ID
	cam.CreatedAt = existing.CreatedAt
	cam.UpdatedAt = existing.UpdatedAt
	cam.Status = existing.Status
	cam.LastOnline = existing.LastOnline
	cam.ErrorMsg = existing.ErrorMsg

	// 防御：空值/零值字段回退到现有值，避免不完整 payload 清空关键配置
	// （前端正常传完整对象，此防护针对 API 直调或部分更新场景）
	if cam.Name == "" {
		cam.Name = existing.Name
	}
	if cam.Protocol == "" {
		cam.Protocol = existing.Protocol
	}
	if cam.Username == "" {
		cam.Username = existing.Username
	}
	if cam.IP == "" {
		cam.IP = existing.IP
	}
	if cam.Port == 0 {
		cam.Port = existing.Port
	}
	if cam.Path == "" {
		cam.Path = existing.Path
	}
	if req.OnvifAddress == "" {
		cam.OnvifAddress = existing.OnvifAddress
	}
	if req.OnvifProfileToken == "" {
		cam.OnvifProfileToken = existing.OnvifProfileToken
	}
	// ONVIF 设备信息由后端连接时自动抓取入库，未传时保留现有值
	if req.Manufacturer == "" {
		cam.Manufacturer = existing.Manufacturer
	}
	if req.Model == "" {
		cam.Model = existing.Model
	}
	if req.Firmware == "" {
		cam.Firmware = existing.Firmware
	}
	if req.SerialNumber == "" {
		cam.SerialNumber = existing.SerialNumber
	}
	if cam.RecordSchedule == "" {
		cam.RecordSchedule = existing.RecordSchedule
	}
	if cam.RecordType == "" {
		cam.RecordType = existing.RecordType
	}
	if cam.Codec == "" {
		cam.Codec = existing.Codec
	}
	if cam.Width == 0 {
		cam.Width = existing.Width
	}
	if cam.Height == 0 {
		cam.Height = existing.Height
	}
	if cam.FPS == 0 {
		cam.FPS = existing.FPS
	}
	if cam.Bitrate == 0 {
		cam.Bitrate = existing.Bitrate
	}
	// bool 字段：未传（nil）时保留原值
	if req.RecordEnabled == nil {
		cam.RecordEnabled = existing.RecordEnabled
	}
	if req.PTZEnabled == nil {
		cam.PTZEnabled = existing.PTZEnabled
	}
	// 保留 ONVIF 流地址缓存（除非前端显式更新 profile token）
	cam.DiscoveredStreamUri = existing.DiscoveredStreamUri
	cam.StreamUriUpdatedAt = existing.StreamUriUpdatedAt
	if cam.OnvifProfileToken == existing.OnvifProfileToken {
		cam.DiscoveredStreamUri = existing.DiscoveredStreamUri
	} else {
		// Profile 变更，清空缓存流地址，让连接逻辑重新发现
		cam.DiscoveredStreamUri = ""
		cam.StreamUriUpdatedAt = nil
	}
	// 密码为空表示不修改，保留原密码
	if req.Password == "" {
		cam.Password = existing.Password
	}

	if err := s.cameraMgr.UpdateCamera(cam); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	cam.Password = ""
	c.JSON(http.StatusOK, cam)
}

func (s *Server) deleteCamera(c *gin.Context) {
	id := c.Param("id")
	if err := s.cameraMgr.DeleteCamera(parseUint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

func (s *Server) getCameraStatus(c *gin.Context) {
	id := c.Param("id")
	inst, ok := s.cameraMgr.GetCameraStatus(parseUint(id))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "摄像头不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"camera_id":      inst.Model.ID,
		"name":           inst.Model.Name,
		"status":         inst.Status,
		"last_error":     inst.LastError,
		"reconnect_count": inst.ReconnectCnt,
		"is_streaming":   inst.Stream != nil && inst.Stream.IsRunning(),
	})
}

func (s *Server) startCamera(c *gin.Context) {
	id := parseUint(c.Param("id"))
	cam, err := s.cameraMgr.GetCamera(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "摄像头不存在"})
		return
	}
	cam.RecordEnabled = true
	if err := s.cameraMgr.UpdateCamera(cam); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "启动成功"})
}

func (s *Server) stopCamera(c *gin.Context) {
	id := parseUint(c.Param("id"))
	cam, err := s.cameraMgr.GetCamera(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "摄像头不存在"})
		return
	}
	cam.RecordEnabled = false
	if err := s.cameraMgr.UpdateCamera(cam); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "停止成功"})
}

func (s *Server) restartCamera(c *gin.Context) {
	id := parseUint(c.Param("id"))
	if err := s.cameraMgr.RestartCamera(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "重启成功"})
}

func (s *Server) takeSnapshot(c *gin.Context) {
	id := parseUint(c.Param("id"))

	path, err := s.cameraMgr.Snapshot(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 入库，便于列表展示
	if err := s.cameraMgr.SaveSnapshot(id, path, "manual"); err != nil {
		logrus.Warnf("保存抓拍记录失败: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{"path": path})
}

// getSnapshotFile 按文件名提供抓拍图片
func (s *Server) getSnapshotFile(c *gin.Context) {
	cameraID := c.Param("cameraId")
	file := strings.TrimPrefix(c.Param("file"), "/")
	// 仅允许 snapshot_ 开头的 jpg，防路径穿越
	if file == "" || strings.Contains(file, "..") ||
		!strings.HasPrefix(file, "snapshot_") || !strings.HasSuffix(file, ".jpg") {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Content-Type", "image/jpeg")
	c.File("./recordings/camera_" + cameraID + "/" + file)
}

// ========== 存储设置 ==========

// storageSettingsRequest 存储设置请求体
type storageSettingsRequest struct {
	RootPath        string `json:"root_path"`
	SegmentDuration int    `json:"segment_duration"`
	MaxDays         int    `json:"max_days"`
	MaxStorageGB    *float64 `json:"max_storage_gb"` // 存储占用上限（GB），0 = 不限制；不传 = 不修改
	CleanupInterval int    `json:"cleanup_interval"`
	Webdav          *config.WebdavConfig `json:"webdav"`
}

type webdavTestRequest struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
	BasePath string `json:"base_path"`
}

func (s *Server) currentStorageSettings() config.LocalStorageConfig {
	if s.runtimeStorage != nil {
		return s.runtimeStorage.GetLocal()
	}
	return s.cfg.Storage.Local
}

func (s *Server) currentWebdavSettings() config.WebdavConfig {
	if s.runtimeStorage != nil {
		return s.runtimeStorage.GetWebdav()
	}
	return s.cfg.Storage.Webdav
}

// listWebdavFiles 列出 WebDAV 基础目录下某摄像头（或全部）的录像文件
// GET /webdav/list?camera_id=11
func (s *Server) listWebdavFiles(c *gin.Context) {
	wd := s.currentWebdavSettings()
	if !wd.Enabled || wd.URL == "" {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "files": []interface{}{}})
		return
	}

	cameraID := strings.TrimSpace(c.Query("camera_id"))
	rel := strings.Trim(strings.TrimSpace(wd.BasePath), "/")
	if cameraID != "" {
		// 上传布局为 <base>/camera_<id>/xxx.mp4
		rel = "camera_" + cameraID
		if rel2 := strings.Trim(wd.BasePath, "/"); rel2 != "" {
			rel = rel2 + "/" + rel
		}
	}

	client := webdav.NewClient(wd.URL, wd.Username, wd.Password)
	entries, err := client.List(rel)
	if err != nil {
		// 目录不存在（该摄像头尚无上传）视为空列表
		if strings.Contains(err.Error(), "404") {
			c.JSON(http.StatusOK, gin.H{"enabled": true, "camera_id": cameraID, "files": []interface{}{}})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "读取 WebDAV 目录失败: " + err.Error()})
		return
	}

	type webdavFileItem struct {
		Name    string    `json:"name"`
		Path    string    `json:"path"`
		Size    int64     `json:"size"`
		ModTime time.Time `json:"mod_time"`
	}
	var files []webdavFileItem
	for _, e := range entries {
		if e.IsDir || !strings.HasSuffix(strings.ToLower(e.Name), ".mp4") {
			continue
		}
		full := e.Name
		if base := strings.Trim(wd.BasePath, "/"); base != "" {
			if cameraID != "" {
				full = base + "/camera_" + cameraID + "/" + e.Name
			} else {
				full = base + "/" + e.Name
			}
		}
		files = append(files, webdavFileItem{Name: e.Name, Path: full, Size: e.Size, ModTime: e.ModTime})
	}
	// 按修改时间倒序（最新在前）
	sort.Slice(files, func(i, j int) bool { return files[i].ModTime.After(files[j].ModTime) })
	if files == nil {
		files = []webdavFileItem{}
	}
	c.JSON(http.StatusOK, gin.H{"enabled": true, "camera_id": cameraID, "files": files})
}

// validateWebdavRelPath 确保请求的远程路径在基础目录内且不含 .. 片段
func validateWebdavRelPath(basePath, rel string) error {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return fmt.Errorf("路径不能为空")
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." {
			return fmt.Errorf("非法路径")
		}
	}
	base := strings.Trim(strings.TrimSpace(basePath), "/")
	if base == "" {
		return nil
	}
	if rel != base && !strings.HasPrefix(rel, base+"/") {
		return fmt.Errorf("路径必须在 WebDAV 基础目录内")
	}
	return nil
}

// streamWebdavFile 将 WebDAV 上的录像文件流式代理给浏览器（支持 Range 拖动进度）
// GET /webdav/file?path=surveillance/camera_11/segment_xxx.mp4
func (s *Server) streamWebdavFile(c *gin.Context) {
	wd := s.currentWebdavSettings()
	if !wd.Enabled || wd.URL == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "WebDAV 未启用"})
		return
	}
	rel := c.Query("path")
	if err := validateWebdavRelPath(wd.BasePath, rel); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	client := webdav.NewClient(wd.URL, wd.Username, wd.Password)
	resp, err := client.Get(c.Request.Context(), rel, c.GetHeader("Range"))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "读取 WebDAV 文件失败: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "WebDAV 上不存在该文件"})
		return
	}
	c.Status(resp.StatusCode)
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		c.Header("Content-Type", ct)
	} else {
		c.Header("Content-Type", "video/mp4")
	}
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		c.Header("Content-Range", cr)
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		c.Header("Content-Length", cl)
	}
	c.Header("Accept-Ranges", "bytes")
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filepath.Base(rel)))
	io.Copy(c.Writer, resp.Body)
}

func (s *Server) getStorageSettings(c *gin.Context) {
	loc := s.currentStorageSettings()
	wd := s.currentWebdavSettings()
	// 不返回密码明文，用是否已设置表示
	wdSet := wd
	if wd.Password != "" {
		wdSet.Password = "********"
	}
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"root_path":        loc.RootPath,
			"segment_duration": loc.SegmentDuration,
			"max_days":         loc.MaxDays,
			"max_storage_gb":   loc.MaxStorageGB,
			"cleanup_interval": loc.CleanupInterval,
			"webdav":           wdSet,
		},
	})
}

func (s *Server) updateStorageSettings(c *gin.Context) {
	var req storageSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}

	loc := s.currentStorageSettings()
	wd := s.currentWebdavSettings()

	// 校验与更新本地存储
	if req.RootPath != "" {
		if strings.Contains(req.RootPath, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "存储路径不合法"})
			return
		}
		loc.RootPath = req.RootPath
	}
	if req.SegmentDuration >= 30 && req.SegmentDuration <= 86400 {
		loc.SegmentDuration = req.SegmentDuration
	}
	if req.MaxDays >= 1 && req.MaxDays <= 365 {
		loc.MaxDays = req.MaxDays
	}
	if req.MaxStorageGB != nil {
		if *req.MaxStorageGB < 0 || *req.MaxStorageGB > 100000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "存储占用上限须在 0~100000 GB 之间（0 表示不限制）"})
			return
		}
		loc.MaxStorageGB = *req.MaxStorageGB
	}
	if req.CleanupInterval >= 300 {
		loc.CleanupInterval = req.CleanupInterval
	}

	// 更新 WebDAV
	if req.Webdav != nil {
		w := *req.Webdav
		// 密码传 "********" 或空表示不修改
		if w.Password == "" || w.Password == "********" {
			w.Password = wd.Password
		}
		// 校验 WebDAV 独立保留策略
		if w.MaxDays < 0 || w.MaxDays > 3650 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "WebDAV 远程保留天数须在 0~3650 之间（0 表示不按时间自动删除）"})
			return
		}
		if w.MaxStorageGB < 0 || w.MaxStorageGB > 100000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "WebDAV 远程占用上限须在 0~100000 GB 之间（0 表示不限制）"})
			return
		}
		wd = w
	}

	// 应用：运行时立即生效 + 内存配置 + 持久化到 config.yaml
	if s.runtimeStorage != nil {
		s.runtimeStorage.SetLocal(loc)
		s.runtimeStorage.SetWebdav(wd)
	}
	s.cfg.Storage.Local = loc
	s.cfg.Storage.Webdav = wd

	if err := s.persistConfig(); err != nil {
		logrus.Warnf("持久化设置到 config.yaml 失败: %v（本次修改仅在内存生效，重启后还原）", err)
	}

	// 确保存储根目录存在
	if err := os.MkdirAll(loc.RootPath, 0755); err != nil {
		logrus.Warnf("创建存储根目录失败 %s: %v", loc.RootPath, err)
	}

	logrus.Infof("存储设置已更新: root=%s segment=%ds maxdays=%d maxstorage=%vGB webdav.enabled=%v",
		loc.RootPath, loc.SegmentDuration, loc.MaxDays, loc.MaxStorageGB, wd.Enabled)

	c.JSON(http.StatusOK, gin.H{"message": "保存成功（分段时长对新连接/重连的摄像头生效）"})
}

// getCameraSettings 获取摄像头全局设置（定时抓拍）
func (s *Server) getCameraSettings(c *gin.Context) {
	enabled, interval := s.cameraMgr.GetSnapshotSettings()
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"snapshot_enabled":  enabled,
			"snapshot_interval": interval,
		},
	})
}

// updateCameraSettings 更新摄像头全局设置（定时抓拍开关/间隔，热更新立即生效）
func (s *Server) updateCameraSettings(c *gin.Context) {
	var req struct {
		SnapshotEnabled  *bool `json:"snapshot_enabled"`
		SnapshotInterval *int  `json:"snapshot_interval"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}

	enabled, interval := s.cameraMgr.GetSnapshotSettings()
	if req.SnapshotEnabled != nil {
		enabled = *req.SnapshotEnabled
	}
	if req.SnapshotInterval != nil {
		if *req.SnapshotInterval < 30 || *req.SnapshotInterval > 86400 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "抓拍间隔须在 30~86400 秒之间"})
			return
		}
		interval = *req.SnapshotInterval
	}

	// 运行时立即生效 + 内存配置 + 持久化到 config.yaml
	s.cameraMgr.SetSnapshotSettings(enabled, interval)
	s.cfg.Camera.SnapshotEnabled = enabled
	s.cfg.Camera.SnapshotInterval = interval
	if err := s.persistConfig(); err != nil {
		logrus.Warnf("持久化设置到 config.yaml 失败: %v（本次修改仅在内存生效，重启后还原）", err)
	}

	logrus.Infof("定时抓拍设置已更新: enabled=%v interval=%ds", enabled, interval)
	c.JSON(http.StatusOK, gin.H{"message": "保存成功（定时抓拍立即生效）"})
}

func (s *Server) testWebdav(c *gin.Context) {
	var req webdavTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
		return
	}
	client := webdav.NewClient(req.URL, req.Username, req.Password)
	if err := client.TestAndUpload(req.BasePath); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "WebDAV 连接成功，可正常读写"})
}

// persistConfig 将当前配置写回 config.yaml
func (s *Server) persistConfig() error {
	if s.cfgPath == "" {
		return fmt.Errorf("未配置 config 路径")
	}
	data, err := yaml.Marshal(s.cfg)
	if err != nil {
		return err
	}
	tmp := s.cfgPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.cfgPath)
}

func (s *Server) ptzControl(c *gin.Context) {
	id := parseUint(c.Param("id"))
	var req struct {
		Command string  `json:"command" binding:"required"`
		Speed   float64 `json:"speed"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.cameraMgr.PTZControl(id, req.Command, req.Speed); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "PTZ 指令已发送"})
}

func (s *Server) listSnapshots(c *gin.Context) {
	id := parseUint(c.Param("id"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "12"))

	snaps, total, err := s.cameraMgr.ListSnapshots(id, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if snaps == nil {
		snaps = []models.Snapshot{}
	}
	c.JSON(http.StatusOK, gin.H{"data": snaps, "total": total})
}

// listAllSnapshots 全局抓拍列表（录像管理-抓拍列表用）
// GET /api/v1/snapshots?camera_id=&start=&end=&page=&page_size=
func (s *Server) listAllSnapshots(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	if page < 1 {
		page = 1
	}

	db := database.GetDB().Model(&models.Snapshot{}).Preload("Camera")
	if cameraID := c.Query("camera_id"); cameraID != "" {
		if id, err := strconv.ParseUint(cameraID, 10, 64); err == nil {
			db = db.Where("camera_id = ?", id)
		}
	}
	if start := c.Query("start"); start != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", start, time.Local); err == nil {
			db = db.Where("timestamp >= ?", t)
		}
	}
	if end := c.Query("end"); end != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", end, time.Local); err == nil {
			db = db.Where("timestamp <= ?", t)
		}
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var snaps []models.Snapshot
	if err := db.Order("timestamp DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&snaps).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Camera 字段 json:"-"，包装一层补上摄像头名称
	type snapshotListItem struct {
		models.Snapshot
		CameraName string `json:"camera_name"`
	}
	items := make([]snapshotListItem, 0, len(snaps))
	for _, sn := range snaps {
		name := sn.Camera.Name
		items = append(items, snapshotListItem{Snapshot: sn, CameraName: name})
	}
	c.JSON(http.StatusOK, gin.H{"data": items, "total": total})
}

// deleteSnapshot 删除单张抓拍（数据库记录 + 本地文件）
// DELETE /api/v1/snapshots/:id
func (s *Server) deleteSnapshot(c *gin.Context) {
	id := parseUint(c.Param("id"))
	var snap models.Snapshot
	if err := database.GetDB().First(&snap, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "抓拍图片不存在"})
		return
	}
	if err := database.GetDB().Delete(&models.Snapshot{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	full := snap.FilePath
	if !filepath.IsAbs(full) {
		full = "./" + full
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		logrus.Warnf("删除抓拍文件失败 %s: %v", full, err)
	}
	c.JSON(http.StatusOK, gin.H{"message": "已删除"})
}

// clearSnapshots 一键删除全部抓拍（数据库记录 + 本地文件）
// DELETE /api/v1/snapshots
func (s *Server) clearSnapshots(c *gin.Context) {
	var snaps []models.Snapshot
	if err := database.GetDB().Unscoped().Where("id > 0").Find(&snaps).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	deleted := 0
	for _, snap := range snaps {
		full := snap.FilePath
		if !filepath.IsAbs(full) {
			full = "./" + full
		}
		if err := os.Remove(full); err == nil {
			deleted++
		} else if !os.IsNotExist(err) {
			logrus.Warnf("删除抓拍文件失败 %s: %v", full, err)
		}
	}
	if err := database.GetDB().Unscoped().Where("id > 0").Delete(&models.Snapshot{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	logrus.Infof("已清空全部抓拍：记录 %d 条，删除文件 %d 个", len(snaps), deleted)
	c.JSON(http.StatusOK, gin.H{"message": "已清空抓拍图片", "deleted": len(snaps)})
}

func (s *Server) discoverCameras(c *gin.Context) {
	network := c.DefaultQuery("network", "192.168.1.0/24")
	devices, err := s.cameraMgr.DiscoverONVIFCameras(network)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": devices})
}

// DiscoverLAN 局域网 WS-Discovery 组播扫描（真正的自动发现）
// POST /api/v1/cameras/discover/lan
// body: { "timeout": 10 } // 可选，默认 10 秒
func (s *Server) discoverLAN(c *gin.Context) {
	var req struct {
		Timeout int `json:"timeout"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		// 允许空 body，使用默认值
		req.Timeout = 10
	}
	if req.Timeout <= 0 {
		req.Timeout = 10
	}
	if req.Timeout > 30 {
		req.Timeout = 30
	}

	devices, err := s.cameraMgr.DiscoverLAN(req.Timeout)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": devices})
}

// ProbeONVIFCamera 单 IP 探测 ONVIF 设备（用于获取配置文件）
func (s *Server) probeONVIFCamera(c *gin.Context) {
	ip := c.Query("ip")
	if ip == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 ip 参数"})
		return
	}

	// 可选参数：用户名、密码、端口
	username := c.Query("username")
	password := c.Query("password")

	device, err := s.cameraMgr.ProbeONVIFCamera(ip, username, password)
	if err != nil {
		// 区分"设备需要认证"与"设备不存在"，前端可据此给出明确提示
		authRequired := strings.Contains(err.Error(), "需要认证")
		c.JSON(http.StatusOK, gin.H{"device": nil, "auth_required": authRequired, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"device": device, "auth_required": false, "error": ""})
}

// ========== 录像管理 ==========

// parseTimeParam 解析前端时间参数，支持 "2006-01-02 15:04:05" / RFC3339 / "2006-01-02"
func parseTimeParam(v string) (time.Time, bool) {
	if v == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", time.RFC3339, "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, v, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// listRecordings 录像列表（支持 camera_id / record_type / start / end 过滤）
// 注意：前端自行分页，这里返回全部匹配记录（上限 5000 条）
func (s *Server) listRecordings(c *gin.Context) {
	cameraID, _ := strconv.ParseUint(c.Query("camera_id"), 10, 32)
	recordType := c.Query("record_type")
	start, _ := parseTimeParam(c.Query("start"))
	end, _ := parseTimeParam(c.Query("end"))

	recs, total, err := storage.NewRecordingManager().QueryRecordings(uint(cameraID), start, end, recordType, 1, 5000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": recs, "total": total})
}

func (s *Server) getRecording(c *gin.Context) {
	id := parseUint(c.Param("id"))
	rec, err := storage.NewRecordingManager().GetRecordingByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "录像不存在"})
		return
	}
	c.JSON(http.StatusOK, rec)
}

func (s *Server) downloadRecording(c *gin.Context) {
	id := parseUint(c.Param("id"))
	rec, err := storage.NewRecordingManager().GetRecordingByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "录像不存在"})
		return
	}
	full := rec.FilePath
	if !filepath.IsAbs(full) {
		full = "./" + full
	}
	if _, err := os.Stat(full); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "录像文件不存在"})
		return
	}
	c.FileAttachment(full, filepath.Base(full))
}

// getRecordingFile 提供录像文件流（支持 Range，用于浏览器原生播放）。
// 本地文件不存在时（如 WebDAV 独占模式上传后已删除本地副本），
// 回退到 WebDAV 流式播放（透传 Range，可拖动进度）。
func (s *Server) getRecordingFile(c *gin.Context) {
	id := parseUint(c.Param("id"))
	rec, err := storage.NewRecordingManager().GetRecordingByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "录像不存在"})
		return
	}
	full := rec.FilePath
	if !filepath.IsAbs(full) {
		full = "./" + full
	}
	if _, err := os.Stat(full); err == nil {
		// 本地文件存在，直接服务
		c.Header("Accept-Ranges", "bytes")
		c.File(full)
		return
	}

	// 本地不存在 → 尝试 WebDAV 回退
	if rec.StoragePath != "" {
		if wd := s.currentWebdavSettings(); wd.Enabled && wd.URL != "" {
			if err := validateWebdavRelPath(wd.BasePath, rec.StoragePath); err == nil {
				client := webdav.NewClient(wd.URL, wd.Username, wd.Password)
				resp, gerr := client.Get(c.Request.Context(), rec.StoragePath, c.GetHeader("Range"))
				if gerr == nil {
					defer resp.Body.Close()
					if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent {
						c.Status(resp.StatusCode)
						if ct := resp.Header.Get("Content-Type"); ct != "" {
							c.Header("Content-Type", ct)
						} else {
							c.Header("Content-Type", "video/mp4")
						}
						if cr := resp.Header.Get("Content-Range"); cr != "" {
							c.Header("Content-Range", cr)
						}
						if cl := resp.Header.Get("Content-Length"); cl != "" {
							c.Header("Content-Length", cl)
						}
						c.Header("Accept-Ranges", "bytes")
						io.Copy(c.Writer, resp.Body)
						return
					}
					if resp.StatusCode != http.StatusNotFound {
						c.JSON(http.StatusBadGateway, gin.H{"error": "WebDAV 读取录像失败"})
						return
					}
				} else {
					logrus.Warnf("WebDAV 回退播放失败 id=%d: %v", id, gerr)
				}
			}
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "录像文件不存在"})
}

func (s *Server) deleteRecording(c *gin.Context) {
	id := parseUint(c.Param("id"))
	rm := storage.NewRecordingManager()
	rec, err := rm.GetRecordingByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "录像不存在"})
		return
	}

	// 删除本地文件
	full := rec.FilePath
	if !filepath.IsAbs(full) {
		full = "./" + full
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		logrus.Warnf("删除录像文件失败 %s: %v", full, err)
	}

	// 若已同步到 WebDAV，一并删除远程副本
	if rec.StoragePath != "" {
		if wd := s.runtimeStorage.GetWebdav(); wd.Enabled && wd.URL != "" {
			remotePath := filepath.Join(wd.BasePath, rec.StoragePath)
			if err := webdav.NewClient(wd.URL, wd.Username, wd.Password).Delete(remotePath); err == nil {
				logrus.Infof("已删除 WebDAV 远程录像: %s", remotePath)
			}
		}
	}

	if err := rm.DeleteRecording(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// listCameraRecordings 指定摄像头的录像列表
func (s *Server) listCameraRecordings(c *gin.Context) {
	cameraID := parseUint(c.Param("cameraId"))
	recordType := c.Query("record_type")
	start, _ := parseTimeParam(c.Query("start"))
	end, _ := parseTimeParam(c.Query("end"))

	// 尊重前端分页参数（此前写死 1/5000，前端请求 10 条也返回 5000 条）
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 50
	}

	recs, total, err := storage.NewRecordingManager().QueryRecordings(cameraID, start, end, recordType, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": recs, "total": total})
}

// getRecordingSegments 时间范围内的录像片段（回放页时间轴）
func (s *Server) getRecordingSegments(c *gin.Context) {
	cameraID := parseUint(c.Param("cameraId"))
	start, _ := parseTimeParam(c.Query("start"))
	end, _ := parseTimeParam(c.Query("end"))

	recs, err := storage.NewRecordingManager().GetRecordingSegments(cameraID, start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": recs})
}

// ========== 流媒体/回放 ==========
func (s *Server) getHLSPlaylist(c *gin.Context) {
	cameraID := c.Param("cameraId")
	// 按需启动预览流（子码流 HLS 转码），空闲超时后由管理器自动回收
	if err := s.cameraMgr.EnsurePreview(parseUint(cameraID)); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "预览流启动失败: " + err.Error()})
		return
	}
	file := "./recordings/camera_" + cameraID + "/index.m3u8"

	// 预览流刚按需启动，index.m3u8 生成有 1~3 秒延迟；短暂轮询等待其就绪，
	// 避免 HLS.js 首次请求即命中 404 走 fatal 分支。
	deadline := time.Now().Add(8 * time.Second)
	for {
		if _, err := os.Stat(file); err == nil {
			break
		}
		if time.Now().After(deadline) {
			c.JSON(http.StatusNotFound, gin.H{"error": "播放列表不存在，预览流启动超时"})
			return
		}
		time.Sleep(200 * time.Millisecond)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "播放列表不存在，预览流可能未就绪"})
		return
	}

	// 提取令牌（query 优先，其次 Authorization 头），用于注入到每个分段 URI，
	// 使 HLS.js 拉取 .ts 分段时也携带鉴权令牌。
	token := c.Query("token")
	if token == "" {
		if h := c.GetHeader("Authorization"); h != "" {
			token = strings.TrimPrefix(h, "Bearer ")
		}
	}
	tokenSuffix := ""
	if token != "" {
		tokenSuffix = "?token=" + url.QueryEscape(token)
	}

	// 重写分段路径：ffmpeg 写的分段名是 hls_segment_XXX.ts，需加 hls/ 前缀命中路由，
	// 并附加 token 供分段请求鉴权。
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			if !strings.HasPrefix(trimmed, "hls/") {
				trimmed = "hls/" + trimmed
			}
			lines[i] = trimmed + tokenSuffix
		}
	}
	c.Header("Content-Type", "application/vnd.apple.mpegurl")
	c.Data(http.StatusOK, "application/vnd.apple.mpegurl", []byte(strings.Join(lines, "\n")))
}

func (s *Server) getHLSSegment(c *gin.Context) {
	cameraID := c.Param("cameraId")
	// 分段请求说明预览仍在活跃状态，刷新空闲计时
	s.cameraMgr.TouchPreview(parseUint(cameraID))
	file := strings.TrimPrefix(c.Param("file"), "/")
	if file == "" || strings.Contains(file, "..") {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Content-Type", "video/mp2t")
	c.File("./recordings/camera_" + cameraID + "/" + file)
}

func (s *Server) getMP4Segment(c *gin.Context) {
	cameraID := c.Param("cameraId")
	// 返回最新的 MP4 分段用于直接播放
	c.Header("Content-Type", "video/mp4")
	c.File("./recordings/camera_" + cameraID + "/segment_latest.mp4")
}

func (s *Server) getLatestSnapshot(c *gin.Context) {
	cameraID := c.Param("cameraId")
	// 查找最新的快照文件（snapshot_<id>_<timestamp>.jpg）
	matches, _ := filepath.Glob("./recordings/camera_" + cameraID + "/snapshot_*.jpg")
	if len(matches) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "暂无快照"})
		return
	}
	// 文件名含时间戳，字典序即时间序
	sort.Strings(matches)
	c.Header("Content-Type", "image/jpeg")
	c.File(matches[len(matches)-1])
}

func (s *Server) getRecordingHLS(c *gin.Context) {
	// 历史录像的 HLS 回放
	c.Header("Content-Type", "application/vnd.apple.mpegurl")
	c.String(http.StatusOK, "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:10\n#EXT-X-ENDLIST\n")
}

// ========== 智能分析 ==========
func (s *Server) listAlerts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}

	query := database.GetDB().Model(&models.Alert{})

	// 过滤条件
	if v := c.Query("camera_id"); v != "" {
		if id, err := strconv.ParseUint(v, 10, 64); err == nil {
			query = query.Where("camera_id = ?", id)
		}
	}
	if v := c.Query("type"); v != "" {
		query = query.Where("type = ?", v)
	}
	if v := c.Query("level"); v != "" {
		query = query.Where("level = ?", v)
	}
	if v := c.Query("status"); v != "" {
		query = query.Where("status = ?", v)
	}
	start, hasStart := parseTimeParam(c.Query("start"))
	end, hasEnd := parseTimeParam(c.Query("end"))
	if hasStart {
		query = query.Where("created_at >= ?", start)
	}
	if hasEnd {
		query = query.Where("created_at <= ?", end)
	}

	var total int64
	query.Count(&total)

	var alerts []models.Alert
	if err := query.Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&alerts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if alerts == nil {
		alerts = []models.Alert{}
	}

	c.JSON(http.StatusOK, gin.H{"data": alerts, "total": total})
}

func (s *Server) getAlert(c *gin.Context) {
	id := parseUint(c.Param("id"))
	var alert models.Alert
	if err := database.GetDB().First(&alert, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "报警记录不存在"})
		return
	}
	c.JSON(http.StatusOK, alert)
}

func (s *Server) acknowledgeAlert(c *gin.Context) {
	id := parseUint(c.Param("id"))
	var alert models.Alert
	if err := database.GetDB().First(&alert, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "报警记录不存在"})
		return
	}
	now := time.Now()
	// 从认证中间件取当前用户（可选，未登录场景用 0）
	usernameVal, _ := c.Get("username")
	username, _ := usernameVal.(string)
	_ = username

	updates := map[string]interface{}{
		"status":   models.AlertStatusAcknowledged,
		"acked_at": &now,
	}
	if err := database.GetDB().Model(&alert).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已确认"})
}

func (s *Server) resolveAlert(c *gin.Context) {
	id := parseUint(c.Param("id"))
	var alert models.Alert
	if err := database.GetDB().First(&alert, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "报警记录不存在"})
		return
	}
	now := time.Now()
	updates := map[string]interface{}{
		"status":      models.AlertStatusResolved,
		"resolved_at": &now,
	}
	if err := database.GetDB().Model(&alert).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已解决"})
}

func (s *Server) deleteAlert(c *gin.Context) {
	id := parseUint(c.Param("id"))
	if err := database.GetDB().Delete(&models.Alert{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// clearAlerts 一键删除全部报警记录（物理删除，避免软删除行随运动报警持续增长而堆积）
func (s *Server) clearAlerts(c *gin.Context) {
	result := database.GetDB().Unscoped().Where("id > 0").Delete(&models.Alert{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已清空报警记录", "deleted": result.RowsAffected})
}

// ========== 存储管理 ==========
func (s *Server) getStorageStats(c *gin.Context) {
	stats := s.storageMgr.GetStats()
	c.JSON(http.StatusOK, stats)
}

func (s *Server) triggerCleanup(c *gin.Context) {
	// 同步执行一次完整清理（保留天数 + 存储占用上限），便于用户立即看到结果
	s.storageMgr.TriggerCleanup()
	c.JSON(http.StatusOK, gin.H{"message": "清理完成（已按保留天数与存储占用上限检查）"})
}

// ========== 报警配置 ==========
func (s *Server) getAlertConfig(c *gin.Context) {
	c.JSON(http.StatusOK, s.cfg.Alert)
}

func (s *Server) updateAlertConfig(c *gin.Context) {
	// 按「哪个渠道有值就更新哪个」做合并，避免前端只提交单个渠道时
	// 把 enabled / 其它渠道配置清零（指针用于区分「未提交」与「提交为 false」）。
	var req struct {
		Enabled  *bool `json:"enabled"`
		Channels *struct {
			Webhook *config.WebhookAlertConfig `json:"webhook"`
			Email   *config.EmailAlertConfig   `json:"email"`
			SMS     *config.SMSAlertConfig     `json:"sms"`
		} `json:"channels"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Enabled != nil {
		s.cfg.Alert.Enabled = *req.Enabled
	}
	if req.Channels != nil {
		if req.Channels.Webhook != nil {
			s.cfg.Alert.Channels.Webhook = *req.Channels.Webhook
		}
		if req.Channels.Email != nil {
			s.cfg.Alert.Channels.Email = *req.Channels.Email
		}
		if req.Channels.SMS != nil {
			s.cfg.Alert.Channels.SMS = *req.Channels.SMS
		}
	}

	// 持久化到 config.yaml（重启不丢失）
	if err := s.persistConfig(); err != nil {
		logrus.Warnf("持久化报警配置失败: %v（本次修改仅在内存生效，重启后还原）", err)
	}

	c.JSON(http.StatusOK, gin.H{"message": "报警配置已更新"})
}

func (s *Server) sendTestAlert(c *gin.Context) {
	// 前端可附带当前表单的渠道配置（webhook/email/sms），测试即按表单值发送，
	// 无需先保存（先测试后保存）；未附带时用当前生效配置。
	var req struct {
		Channel string `json:"channel" binding:"required"`
		Webhook *config.WebhookAlertConfig `json:"webhook"`
		Email   *config.EmailAlertConfig   `json:"email"`
		SMS     *config.SMSAlertConfig     `json:"sms"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var override *config.AlertConfig
	if req.Webhook != nil || req.Email != nil || req.SMS != nil {
		base := s.cfg.Alert // 值拷贝，测试不改动生效配置
		if req.Webhook != nil {
			base.Channels.Webhook = *req.Webhook
		}
		if req.Email != nil {
			base.Channels.Email = *req.Email
		}
		if req.SMS != nil {
			base.Channels.SMS = *req.SMS
		}
		override = &base
	}

	if err := s.alertMgr.SendTestAlert(req.Channel, override); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "测试报警已发送"})
}

// ========== 系统配置 ==========
func (s *Server) getSystemConfig(c *gin.Context) {
	// 返回非敏感配置
	c.JSON(http.StatusOK, gin.H{
		"server":  s.cfg.Server,
		"storage": s.cfg.Storage,
		"camera":  s.cfg.Camera,
	})
}

func (s *Server) updateSystemConfig(c *gin.Context) {
	// TODO: 更新配置并持久化
	c.JSON(http.StatusOK, gin.H{"message": "配置已更新"})
}

func (s *Server) getSystemInfo(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version":     "1.0.0",
		"go_version":  "go1.22",
		"start_time":  time.Now().Add(-time.Hour).Unix(), // 模拟
		"uptime":      3600,
	})
}

func (s *Server) restartSystem(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "系统重启中..."})
	// TODO: 触发重启
}

// ========== WebSocket ==========
func (s *Server) handleWebSocket(c *gin.Context) {
	conn, err := s.wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logrus.Errorf("WebSocket 升级失败: %v", err)
		return
	}
	defer conn.Close()

	// 发送欢迎消息
	conn.WriteJSON(gin.H{"type": "welcome", "message": "Connected to surveillance system"})

	for {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}
		// 处理消息
		s.handleWSMessage(conn, msg)
	}
}

func (s *Server) handleCameraWS(c *gin.Context) {
	cameraID := parseUint(c.Param("cameraId"))
	conn, err := s.wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// 订阅摄像头事件
	// TODO: 实现实时事件推送（状态变化、报警、新录像等）
	conn.WriteJSON(gin.H{"type": "subscribed", "camera_id": cameraID})

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (s *Server) handleWSMessage(conn *websocket.Conn, msg map[string]interface{}) {
	msgType, _ := msg["type"].(string)
	switch msgType {
	case "ping":
		conn.WriteJSON(gin.H{"type": "pong"})
	case "subscribe_camera":
		if camID, ok := msg["camera_id"].(float64); ok {
			conn.WriteJSON(gin.H{"type": "subscribed", "camera_id": uint(camID)})
		}
	case "unsubscribe_camera":
		// TODO
	}
}

// 辅助函数
func parseUint(s string) uint {
	var id uint
	fmt.Sscanf(s, "%d", &id)
	return id
}