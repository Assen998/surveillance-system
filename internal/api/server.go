package api

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"

	embedui "github.com/yourorg/surveillance-system"
	"github.com/yourorg/surveillance-system/internal/alert"
	"github.com/yourorg/surveillance-system/internal/camera"
	"github.com/yourorg/surveillance-system/internal/config"
	"github.com/yourorg/surveillance-system/internal/database"
	"github.com/yourorg/surveillance-system/internal/models"
	"github.com/yourorg/surveillance-system/internal/storage"
	"github.com/yourorg/surveillance-system/pkg/minio"
	"github.com/yourorg/surveillance-system/pkg/webdav"
)

type Server struct {
	cfg            *config.Config
	cameraMgr      *camera.CameraManager
	storageMgr     *storage.Manager
	alertMgr       *alert.Manager
	router         *gin.Engine
	wsUpgrader     websocket.Upgrader
	runtimeStorage *storage.RuntimeStorage
	cfgPath        string

	version   string
	buildTime string
	gitCommit string
	startTime time.Time

	restartFunc func(newBinary string) error
}

func (s *Server) SetVersionInfo(version, buildTime, gitCommit string, startTime time.Time) {
	s.version = version
	s.buildTime = buildTime
	s.gitCommit = gitCommit
	s.startTime = startTime
}

func (s *Server) SetRestartFunc(f func(newBinary string) error) {
	s.restartFunc = f
}

func (s *Server) SetRuntimeStorage(r *storage.RuntimeStorage) {
	s.runtimeStorage = r
}

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
			CheckOrigin:     func(r *http.Request) bool { return true },
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

	distFS, distErr := embedui.Dist()
	if distErr != nil {
		logrus.Warnf("failed to load embedded frontend assets (frontend build not run?): %v", distErr)
	}
	if distErr == nil {

		staticFS, _ := fs.Sub(distFS, "static")
		r.StaticFS("/static", http.FS(staticFS))

		indexData, _ := fs.ReadFile(distFS, "index.html")
		r.GET("/", func(c *gin.Context) {
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Data(http.StatusOK, "text/html; charset=utf-8", indexData)
		})

		if faviconData, err := fs.ReadFile(distFS, "favicon.ico"); err == nil {
			r.GET("/favicon.ico", func(c *gin.Context) {
				c.Data(http.StatusOK, "image/x-icon", faviconData)
			})
		}
	}

	r.GET("/health", s.healthCheck)
	r.GET("/api/version", s.getVersion)

	v1 := r.Group("/api/v1")
	{

		auth := v1.Group("/auth")
		{
			auth.POST("/login", s.login)
			auth.POST("/logout", s.logout)
			auth.GET("/me", s.authMiddleware(), s.getCurrentUser)
			auth.PUT("/password", s.authMiddleware(), s.changePassword)
		}

		v1.GET("/setup/status", s.getSetupStatus)
		v1.POST("/setup", s.postSetup)

		secured := v1.Group("")
		secured.Use(s.authMiddleware())
		{

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

			users := secured.Group("/users")
			users.Use(s.adminOnly())
			{
				users.GET("", s.listUsers)
				users.POST("", s.createUser)
				users.PUT("/:id", s.updateUser)
				users.DELETE("/:id", s.deleteUser)
				users.POST("/:id/reset-password", s.resetUserPassword)
				users.GET("/:id/permissions", s.listUserPermissions)
				users.PUT("/:id/permissions", s.setUserPermissions)
			}

			settings := secured.Group("/settings")
			{
				settings.GET("/storage", s.getStorageSettings)
				settings.PUT("/storage", s.updateStorageSettings)
				settings.GET("/camera", s.getCameraSettings)
				settings.PUT("/camera", s.updateCameraSettings)
				settings.POST("/webdav/test", s.testWebdav)
				settings.POST("/minio/test", s.testMinio)
			}

			recordings := secured.Group("/recordings")
			{
				recordings.GET("", s.listRecordings)
				recordings.GET("/:id", s.getRecording)
				recordings.DELETE("/:id", s.deleteRecording)
				recordings.GET("/camera/:cameraId", s.listCameraRecordings)
				recordings.GET("/camera/:cameraId/segments", s.getRecordingSegments)
			}

			snapshots := secured.Group("/snapshots")
			{
				snapshots.GET("", s.listAllSnapshots)
				snapshots.DELETE("", s.clearSnapshots)
				snapshots.DELETE("/:id", s.deleteSnapshot)
			}

			analytics := secured.Group("/analytics")
			{
				analytics.GET("/alerts", s.listAlerts)
				analytics.DELETE("/alerts", s.clearAlerts)
				analytics.GET("/alerts/:id", s.getAlert)
				analytics.PUT("/alerts/:id/ack", s.acknowledgeAlert)
				analytics.PUT("/alerts/:id/resolve", s.resolveAlert)
				analytics.DELETE("/alerts/:id", s.deleteAlert)
			}

			storage := secured.Group("/storage")
			{
				storage.GET("/stats", s.getStorageStats)
				storage.POST("/cleanup", s.triggerCleanup)
			}

			alerts := secured.Group("/alerts")
			{
				alerts.GET("/config", s.getAlertConfig)
				alerts.PUT("/config", s.updateAlertConfig)
				alerts.POST("/test", s.sendTestAlert)
			}

			system := secured.Group("/system")
			{
				system.GET("/config", s.getSystemConfig)
				system.PUT("/config", s.updateSystemConfig)
				system.GET("/info", s.getSystemInfo)
				system.POST("/restart", s.restartSystem)

				system.GET("/env", s.getEnvCheck)

				system.GET("/logs", s.getLogTail)
				system.GET("/logs/files", s.getLogFiles)
				system.POST("/logs/clear", s.clearLogs)

				system.POST("/backup", s.createBackup)
				system.GET("/backups", s.listBackups)
				system.GET("/backups/:name/download", s.downloadBackup)
				system.DELETE("/backups/:name", s.deleteBackup)

				system.GET("/update/check", s.checkUpdate)
				system.POST("/update", s.performUpdate)
				system.GET("/update/config", s.getUpdateConfig)
				system.PUT("/update/config", s.updateUpdateConfig)
			}
		}

		media := v1.Group("")
		media.Use(s.mediaAuthMiddleware())
		{

			media.GET("/recordings/:id/file", s.getRecordingFile)
			media.HEAD("/recordings/:id/file", s.getRecordingFile)
			media.GET("/recordings/:id/download", s.downloadRecording)

			media.GET("/webdav/list", s.listWebdavFiles)
			media.GET("/webdav/file", s.streamWebdavFile)

			media.GET("/minio/list", s.listMinIOFiles)
			media.GET("/minio/file", s.streamMinIOFile)

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

	r.GET("/ws", s.handleWebSocket)
	r.GET("/api/v1/ws/camera/:cameraId", s.handleCameraWS)

	r.NoRoute(func(c *gin.Context) {

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

var authSecret = []byte("surveillance-system-secret-change-in-production")

const tokenTTL = 24 * time.Hour

func makeToken(username, role string, ttl time.Duration) string {
	expires := time.Now().Add(ttl).Unix()
	payload := fmt.Sprintf("%s|%s|%d", username, role, expires)
	payloadB64 := base64.RawURLEncoding.EncodeToString([]byte(payload))
	mac := hmac.New(sha256.New, authSecret)
	mac.Write([]byte(payloadB64))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payloadB64 + "." + sig
}

func parseToken(token string) (string, string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid token format")
	}
	mac := hmac.New(sha256.New, authSecret)
	mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(expected), []byte(parts[1])) != 1 {
		return "", "", fmt.Errorf("invalid token signature")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "", fmt.Errorf("failed to decode token")
	}
	segs := strings.Split(string(payloadBytes), "|")
	if len(segs) != 3 {
		return "", "", fmt.Errorf("invalid token content")
	}
	expires, err := strconv.ParseInt(segs[2], 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return "", "", fmt.Errorf("token expired")
	}
	return segs[0], segs[1], nil
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenHeader := c.GetHeader("Authorization")
		if tokenHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not logged in or missing auth token"})
			c.Abort()
			return
		}
		tokenString := strings.TrimPrefix(tokenHeader, "Bearer ")
		username, role, err := parseToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token, please log in again"})
			c.Abort()
			return
		}
		c.Set("username", username)
		c.Set("role", role)
		c.Next()
	}
}

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
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not logged in or invalid token, please log in again"})
			c.Abort()
			return
		}
		c.Set("username", username)
		c.Set("role", role)
		c.Next()
	}
}

func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
		"version":   s.version,
	})
}

func (s *Server) getVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version":   s.version,
		"buildTime": s.buildTime,
		"gitCommit": s.gitCommit,
	})
}

func (s *Server) login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user models.User
	if err := database.GetDB().Where("username = ?", req.Username).First(&user).Error; err != nil {
		logrus.Warnf("login failed: user does not exist username=%s", req.Username)

		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$dummyhashdummyhashdummyhashdummyhas0000000000000000000000"), []byte(req.Password))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}
	if user.Status != "active" {
		c.JSON(http.StatusForbidden, gin.H{"error": "account is disabled"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		logrus.Warnf("login failed: wrong password username=%s", req.Username)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}

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
	c.JSON(http.StatusOK, gin.H{"message": "logged out successfully"})
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
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "old password is incorrect"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}
	if err := database.GetDB().Model(&user).Update("password", string(hash)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "password changed successfully"})
}

func (s *Server) adminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		if role != models.UserRoleAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin role required"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *Server) listUsers(c *gin.Context) {
	var users []models.User
	if err := database.GetDB().Order("id ASC").Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": users, "total": len(users)})
}

func (s *Server) createUser(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required,min=3,max=50"`
		Password string `json:"password" binding:"required,min=6"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
		Role     string `json:"role"`
		Status   string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Role == "" {
		req.Role = models.UserRoleViewer
	}
	if req.Status == "" {
		req.Status = "active"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}
	user := models.User{
		Username: req.Username,
		Password: string(hash),
		Email:    req.Email,
		Phone:    req.Phone,
		Role:     req.Role,
		Status:   req.Status,
	}
	if err := database.GetDB().Create(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to create: username or email already exists"})
		return
	}
	user.Password = ""
	c.JSON(http.StatusOK, user)
}

func (s *Server) updateUser(c *gin.Context) {
	id := parseUint(c.Param("id"))
	var req struct {
		Username    *string `json:"username"`
		Email       *string `json:"email"`
		Phone       *string `json:"phone"`
		Role        *string `json:"role"`
		Status      *string `json:"status"`
		NewPassword *string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var target models.User
	if err := database.GetDB().First(&target, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	updates := map[string]interface{}{}
	if req.Username != nil && *req.Username != "" {
		updates["username"] = *req.Username
	}
	if req.Email != nil {
		updates["email"] = *req.Email
	}
	if req.Phone != nil {
		updates["phone"] = *req.Phone
	}
	if req.Role != nil && *req.Role != "" {

		if *req.Role != models.UserRoleAdmin && target.Role == models.UserRoleAdmin {
			if me, _ := c.Get("username"); me == target.Username {
				c.JSON(http.StatusBadRequest, gin.H{"error": "cannot remove your own admin role"})
				return
			}
		}
		updates["role"] = *req.Role
	}
	if req.Status != nil && *req.Status != "" {
		if target.Role == models.UserRoleAdmin && *req.Status != "active" {
			if me, _ := c.Get("username"); me == target.Username {
				c.JSON(http.StatusBadRequest, gin.H{"error": "cannot disable your own admin account"})
				return
			}
			var adminCount int64
			database.GetDB().Model(&models.User{}).
				Where("role = ? AND status = ?", models.UserRoleAdmin, "active").
				Count(&adminCount)
			if adminCount <= 1 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "cannot disable the last admin"})
				return
			}
		}
		updates["status"] = *req.Status
	}
	if req.NewPassword != nil && *req.NewPassword != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(*req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}
		updates["password"] = string(hash)
	}
	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
		return
	}
	if res := database.GetDB().Model(&models.User{}).Where("id = ?", id).Updates(updates); res.Error != nil {
		if strings.Contains(res.Error.Error(), "UNIQUE constraint") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to update: username or email already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": res.Error.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "updated successfully"})
}

func (s *Server) deleteUser(c *gin.Context) {
	id := parseUint(c.Param("id"))
	me, _ := c.Get("username")

	var target models.User
	if err := database.GetDB().First(&target, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if target.Username == me {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete your own account"})
		return
	}
	if target.Role == models.UserRoleAdmin {
		var adminCount int64
		database.GetDB().Model(&models.User{}).
			Where("role = ? AND status = ?", models.UserRoleAdmin, "active").
			Count(&adminCount)
		if adminCount <= 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete the last admin"})
			return
		}
	}
	if err := database.GetDB().Delete(&models.User{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted successfully"})
}

func (s *Server) resetUserPassword(c *gin.Context) {
	id := parseUint(c.Param("id"))
	var req struct {
		Password string `json:"password" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}
	res := database.GetDB().Model(&models.User{}).Where("id = ?", id).Update("password", string(hash))
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": res.Error.Error()})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "password reset successfully"})
}

func (s *Server) listUserPermissions(c *gin.Context) {
	id := parseUint(c.Param("id"))
	var perms []models.CameraPermission
	if err := database.GetDB().Where("user_id = ?", id).Find(&perms).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": perms, "total": len(perms)})
}

func (s *Server) setUserPermissions(c *gin.Context) {
	id := parseUint(c.Param("id"))
	var req struct {
		Permissions []models.CameraPermission `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var user models.User
	if err := database.GetDB().First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	db := database.GetDB()
	if err := db.Where("user_id = ?", id).Delete(&models.CameraPermission{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for i := range req.Permissions {
		req.Permissions[i].ID = 0
		req.Permissions[i].UserID = id
		if req.Permissions[i].Permission == "" {
			req.Permissions[i].Permission = "view"
		}
		if err := db.Create(&req.Permissions[i]).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "permissions updated"})
}

func (s *Server) listCameras(c *gin.Context) {
	cameras, err := s.cameraMgr.ListCameras()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	previewDefault := s.cameraMgr.NormalizePreviewSrc("")
	for i := range cameras {
		cameras[i].Password = ""
		cameras[i].PTZSupported = s.cameraMgr.PTZCapability(cameras[i].ID)
		cameras[i].PreviewDefault = previewDefault
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  cameras,
		"total": len(cameras),
	})
}

type cameraRequest struct {
	Name              string `json:"name"`
	Description       string `json:"description"`
	Protocol          string `json:"protocol"`
	IP                string `json:"ip"`
	Port              int    `json:"port"`
	Username          string `json:"username"`
	Password          string `json:"password"`
	Path              string `json:"path"`
	OnvifAddress      string `json:"onvif_address"`
	OnvifProfileToken string `json:"onvif_profile_token"`

	Manufacturer string  `json:"manufacturer"`
	Model        string  `json:"model"`
	Firmware     string  `json:"firmware"`
	SerialNumber string  `json:"serial_number"`
	DeviceID     *string `json:"device_id"`

	RecordEnabled  *bool  `json:"record_enabled"`
	RecordSchedule string `json:"record_schedule"`
	RecordType     string `json:"record_type"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	FPS            int    `json:"fps"`
	Bitrate        int    `json:"bitrate"`
	Codec          string `json:"codec"`
	PTZEnabled     *bool  `json:"ptz_enabled"`
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
		Name:              r.Name,
		Description:       r.Description,
		Protocol:          r.Protocol,
		IP:                r.IP,
		Port:              r.Port,
		Username:          r.Username,
		Password:          r.Password,
		Path:              r.Path,
		OnvifAddress:      r.OnvifAddress,
		OnvifProfileToken: r.OnvifProfileToken,
		Manufacturer:      r.Manufacturer,
		Model:             r.Model,
		Firmware:          r.Firmware,
		SerialNumber:      r.SerialNumber,
		DeviceID:          r.DeviceID,
		RecordEnabled:     rec,
		RecordSchedule:    r.RecordSchedule,
		RecordType:        r.RecordType,
		Width:             r.Width,
		Height:            r.Height,
		FPS:               r.FPS,
		Bitrate:           r.Bitrate,
		Codec:             r.Codec,
		PTZEnabled:        ptz,
	}
}

func (s *Server) createCamera(c *gin.Context) {
	var req cameraRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cam := req.toModel()

	if cam.Protocol == "rtsp" && cam.RecordType == "motion" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "RTSP cameras do not support motion-detection recording (it relies on ONVIF event reporting); please choose continuous or scheduled recording"})
		return
	}

	if cam.Protocol != "gb28181" && (cam.DeviceID == nil || *cam.DeviceID == "") {
		cam.DeviceID = nil
	}

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

			rtspURL,
		)

		output, err := cmd.Output()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("camera connection test failed: %v, please check the IP/port/path/credentials are correct", err),
			})
			return
		}

		var probeResult map[string]interface{}
		if err := json.Unmarshal(output, &probeResult); err != nil || len(probeResult) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "camera connection test failed: unable to parse video stream information"})
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
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	cam.Password = ""
	cam.PTZSupported = s.cameraMgr.PTZCapability(parseUint(id))
	cam.PreviewDefault = s.cameraMgr.NormalizePreviewSrc("")
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

	existing, err := s.cameraMgr.GetCamera(camID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}

	cam := req.toModel()
	cam.ID = existing.ID
	cam.CreatedAt = existing.CreatedAt
	cam.UpdatedAt = existing.UpdatedAt
	cam.Status = existing.Status
	cam.LastOnline = existing.LastOnline
	cam.ErrorMsg = existing.ErrorMsg

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

	if req.RecordEnabled == nil {
		cam.RecordEnabled = existing.RecordEnabled
	}
	if req.PTZEnabled == nil {
		cam.PTZEnabled = existing.PTZEnabled
	}

	cam.DiscoveredStreamUri = existing.DiscoveredStreamUri
	cam.StreamUriUpdatedAt = existing.StreamUriUpdatedAt
	if cam.OnvifProfileToken == existing.OnvifProfileToken {
		cam.DiscoveredStreamUri = existing.DiscoveredStreamUri
	} else {

		cam.DiscoveredStreamUri = ""
		cam.StreamUriUpdatedAt = nil
	}

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
	c.JSON(http.StatusOK, gin.H{"message": "deleted successfully"})
}

func (s *Server) getCameraStatus(c *gin.Context) {
	id := c.Param("id")
	inst, ok := s.cameraMgr.GetCameraStatus(parseUint(id))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"camera_id":       inst.Model.ID,
		"name":            inst.Model.Name,
		"status":          inst.Status,
		"last_error":      inst.LastError,
		"reconnect_count": inst.ReconnectCnt,
		"is_streaming":    inst.Stream != nil && inst.Stream.IsRunning(),
	})
}

func (s *Server) startCamera(c *gin.Context) {
	id := parseUint(c.Param("id"))
	cam, err := s.cameraMgr.GetCamera(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	cam.RecordEnabled = true
	if err := s.cameraMgr.UpdateCamera(cam); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "started successfully"})
}

func (s *Server) stopCamera(c *gin.Context) {
	id := parseUint(c.Param("id"))
	cam, err := s.cameraMgr.GetCamera(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	cam.RecordEnabled = false
	if err := s.cameraMgr.UpdateCamera(cam); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "stopped successfully"})
}

func (s *Server) restartCamera(c *gin.Context) {
	id := parseUint(c.Param("id"))
	if err := s.cameraMgr.RestartCamera(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "restarted successfully"})
}

func (s *Server) takeSnapshot(c *gin.Context) {
	id := parseUint(c.Param("id"))

	path, err := s.cameraMgr.Snapshot(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := s.cameraMgr.SaveSnapshot(id, path, "manual"); err != nil {
		logrus.Warnf("failed to save snapshot record: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{"path": path})
}

func (s *Server) getSnapshotFile(c *gin.Context) {
	cameraID := c.Param("cameraId")
	file := strings.TrimPrefix(c.Param("file"), "/")

	if file == "" || strings.Contains(file, "..") ||
		!strings.HasPrefix(file, "snapshot_") || !strings.HasSuffix(file, ".jpg") {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Content-Type", "image/jpeg")
	c.File("./recordings/camera_" + cameraID + "/" + file)
}

type storageSettingsRequest struct {
	RootPath        string               `json:"root_path"`
	SegmentDuration int                  `json:"segment_duration"`
	MaxDays         int                  `json:"max_days"`
	MaxStorageGB    *float64             `json:"max_storage_gb"`
	CleanupInterval int                  `json:"cleanup_interval"`
	Webdav          *config.WebdavConfig `json:"webdav"`
	MinIO           *config.MinIOConfig  `json:"minio"`
}

type webdavTestRequest struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
	BasePath string `json:"base_path"`
}

type minioTestRequest struct {
	Endpoint  string `json:"endpoint"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Bucket    string `json:"bucket"`
	UseSSL    bool   `json:"use_ssl"`
	BasePath  string `json:"base_path"`
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

func (s *Server) currentMinIOSettings() config.MinIOConfig {
	if s.runtimeStorage != nil {
		return s.runtimeStorage.GetMinIO()
	}
	return s.cfg.Storage.MinIO
}

func (s *Server) listWebdavFiles(c *gin.Context) {
	wd := s.currentWebdavSettings()
	if !wd.Enabled || wd.URL == "" {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "files": []interface{}{}})
		return
	}

	cameraID := strings.TrimSpace(c.Query("camera_id"))
	rel := strings.Trim(strings.TrimSpace(wd.BasePath), "/")
	if cameraID != "" {

		rel = "camera_" + cameraID
		if rel2 := strings.Trim(wd.BasePath, "/"); rel2 != "" {
			rel = rel2 + "/" + rel
		}
	}

	client := webdav.NewClient(wd.URL, wd.Username, wd.Password)
	entries, err := client.List(rel)
	if err != nil {

		if strings.Contains(err.Error(), "404") {
			c.JSON(http.StatusOK, gin.H{"enabled": true, "camera_id": cameraID, "files": []interface{}{}})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read WebDAV directory: " + err.Error()})
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

	sort.Slice(files, func(i, j int) bool { return files[i].ModTime.After(files[j].ModTime) })
	if files == nil {
		files = []webdavFileItem{}
	}
	c.JSON(http.StatusOK, gin.H{"enabled": true, "camera_id": cameraID, "files": files})
}

func isMinioNoSuchKey(err error) bool {
	return err != nil && strings.Contains(err.Error(), "NoSuchKey")
}

func validateWebdavRelPath(basePath, rel string) error {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return fmt.Errorf("path must not be empty")
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." {
			return fmt.Errorf("invalid path")
		}
	}
	base := strings.Trim(strings.TrimSpace(basePath), "/")
	if base == "" {
		return nil
	}
	if rel != base && !strings.HasPrefix(rel, base+"/") {
		return fmt.Errorf("path must be within the WebDAV base directory")
	}
	return nil
}

func (s *Server) streamWebdavFile(c *gin.Context) {
	wd := s.currentWebdavSettings()
	if !wd.Enabled || wd.URL == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "WebDAV is not enabled"})
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
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read WebDAV file: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "file does not exist on WebDAV"})
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
	mn := s.currentMinIOSettings()

	wdSet := wd
	if wd.Password != "" {
		wdSet.Password = "********"
	}
	mnSet := mn
	if mn.SecretKey != "" {
		mnSet.SecretKey = "********"
	}
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"root_path":        loc.RootPath,
			"segment_duration": loc.SegmentDuration,
			"max_days":         loc.MaxDays,
			"max_storage_gb":   loc.MaxStorageGB,
			"cleanup_interval": loc.CleanupInterval,
			"webdav":           wdSet,
			"minio":            mnSet,
		},
	})
}

func (s *Server) updateStorageSettings(c *gin.Context) {
	var req storageSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters: " + err.Error()})
		return
	}

	loc := s.currentStorageSettings()
	wd := s.currentWebdavSettings()
	mio := s.currentMinIOSettings()

	if req.RootPath != "" {
		if strings.Contains(req.RootPath, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid storage path"})
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
			c.JSON(http.StatusBadRequest, gin.H{"error": "max storage usage must be between 0 and 100000 GB (0 means unlimited)"})
			return
		}
		loc.MaxStorageGB = *req.MaxStorageGB
	}
	if req.CleanupInterval >= 300 {
		loc.CleanupInterval = req.CleanupInterval
	}

	if req.Webdav != nil {
		w := *req.Webdav

		if w.Password == "" || w.Password == "********" {
			w.Password = wd.Password
		}

		if w.MaxDays < 0 || w.MaxDays > 3650 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "WebDAV remote retention days must be between 0 and 3650 (0 means no time-based auto deletion)"})
			return
		}
		if w.MaxStorageGB < 0 || w.MaxStorageGB > 100000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "WebDAV remote max usage must be between 0 and 100000 GB (0 means unlimited)"})
			return
		}
		wd = w
	}

	if req.MinIO != nil {
		mn := *req.MinIO

		if mn.SecretKey == "" || mn.SecretKey == "********" {
			mn.SecretKey = mio.SecretKey
		}

		mn.Endpoint = strings.TrimPrefix(strings.TrimPrefix(mn.Endpoint, "http://"), "https://")
		mn.Endpoint = strings.Trim(mn.Endpoint, "/")

		if mn.MaxDays < 0 || mn.MaxDays > 3650 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "MinIO remote retention days must be between 0 and 3650 (0 means no time-based auto deletion)"})
			return
		}
		if mn.MaxStorageGB < 0 || mn.MaxStorageGB > 100000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "MinIO remote max usage must be between 0 and 100000 GB (0 means unlimited)"})
			return
		}
		mio = mn
	}

	if s.runtimeStorage != nil {
		s.runtimeStorage.SetLocal(loc)
		s.runtimeStorage.SetWebdav(wd)
		s.runtimeStorage.SetMinIO(mio)
	}
	s.cfg.Storage.Local = loc
	s.cfg.Storage.Webdav = wd
	s.cfg.Storage.MinIO = mio

	if err := s.persistConfig(); err != nil {
		logrus.Warnf("failed to persist settings to config.yaml: %v (this change is only in effect in memory and will be reverted after restart)", err)
	}

	if err := os.MkdirAll(loc.RootPath, 0755); err != nil {
		logrus.Warnf("failed to create storage root directory %s: %v", loc.RootPath, err)
	}

	logrus.Infof("storage settings updated: root=%s segment=%ds maxdays=%d maxstorage=%vGB webdav.enabled=%v",
		loc.RootPath, loc.SegmentDuration, loc.MaxDays, loc.MaxStorageGB, wd.Enabled)

	c.JSON(http.StatusOK, gin.H{"message": "saved successfully (segment duration takes effect for new connections/reconnections)"})
}

func (s *Server) getCameraSettings(c *gin.Context) {
	enabled, interval := s.cameraMgr.GetSnapshotSettings()
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"snapshot_enabled":  enabled,
			"snapshot_interval": interval,
		},
	})
}

func (s *Server) updateCameraSettings(c *gin.Context) {
	var req struct {
		SnapshotEnabled  *bool `json:"snapshot_enabled"`
		SnapshotInterval *int  `json:"snapshot_interval"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters: " + err.Error()})
		return
	}

	enabled, interval := s.cameraMgr.GetSnapshotSettings()
	if req.SnapshotEnabled != nil {
		enabled = *req.SnapshotEnabled
	}
	if req.SnapshotInterval != nil {
		if *req.SnapshotInterval < 30 || *req.SnapshotInterval > 86400 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "snapshot interval must be between 30 and 86400 seconds"})
			return
		}
		interval = *req.SnapshotInterval
	}

	s.cameraMgr.SetSnapshotSettings(enabled, interval)
	s.cfg.Camera.SnapshotEnabled = enabled
	s.cfg.Camera.SnapshotInterval = interval
	if err := s.persistConfig(); err != nil {
		logrus.Warnf("failed to persist settings to config.yaml: %v (this change is only in effect in memory and will be reverted after restart)", err)
	}

	logrus.Infof("scheduled snapshot settings updated: enabled=%v interval=%ds", enabled, interval)
	c.JSON(http.StatusOK, gin.H{"message": "saved successfully (scheduled snapshots take effect immediately)"})
}

func (s *Server) testWebdav(c *gin.Context) {
	var req webdavTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters"})
		return
	}
	client := webdav.NewClient(req.URL, req.Username, req.Password)
	if err := client.TestAndUpload(req.BasePath); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "WebDAV connection successful, read and write work normally"})
}

func (s *Server) testMinio(c *gin.Context) {
	var req minioTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters"})
		return
	}
	endpoint := strings.TrimPrefix(strings.TrimPrefix(req.Endpoint, "http://"), "https://")
	endpoint = strings.Trim(endpoint, "/")
	if endpoint == "" || req.Bucket == "" {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "endpoint and bucket must not be empty"})
		return
	}
	client, err := minio.NewClient(endpoint, req.AccessKey, req.SecretKey, req.Bucket, req.UseSSL)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	if err := client.TestAndUpload(req.BasePath); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "MinIO connection successful, bucket is readable and writable"})
}

func (s *Server) listMinIOFiles(c *gin.Context) {
	mn := s.currentMinIOSettings()
	if !mn.Enabled || mn.Endpoint == "" || mn.Bucket == "" {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "files": []interface{}{}})
		return
	}

	cameraID := strings.TrimSpace(c.Query("camera_id"))
	prefix := strings.Trim(strings.TrimSpace(mn.BasePath), "/")
	if cameraID != "" {

		prefix = "camera_" + cameraID
		if b := strings.Trim(mn.BasePath, "/"); b != "" {
			prefix = b + "/" + prefix
		}
	}
	if prefix != "" {
		prefix += "/"
	}

	client, err := minio.NewClient(mn.Endpoint, mn.AccessKey, mn.SecretKey, mn.Bucket, mn.UseSSL)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to create MinIO client: " + err.Error()})
		return
	}
	entries, err := client.List(prefix)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to list MinIO objects: " + err.Error()})
		return
	}

	type minioFileItem struct {
		Name    string    `json:"name"`
		Path    string    `json:"path"`
		Size    int64     `json:"size"`
		ModTime time.Time `json:"mod_time"`
	}
	var files []minioFileItem
	for _, e := range entries {
		if !strings.HasSuffix(strings.ToLower(e.Key), ".mp4") {
			continue
		}
		files = append(files, minioFileItem{
			Name:    filepath.Base(e.Key),
			Path:    e.Key,
			Size:    e.Size,
			ModTime: e.ModTime,
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ModTime.After(files[j].ModTime) })
	if files == nil {
		files = []minioFileItem{}
	}
	c.JSON(http.StatusOK, gin.H{"enabled": true, "camera_id": cameraID, "files": files})
}

func (s *Server) streamMinIOFile(c *gin.Context) {
	mn := s.currentMinIOSettings()
	if !mn.Enabled || mn.Endpoint == "" || mn.Bucket == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MinIO is not enabled"})
		return
	}
	key := c.Query("path")
	if err := validateWebdavRelPath(mn.BasePath, key); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	client, err := minio.NewClient(mn.Endpoint, mn.AccessKey, mn.SecretKey, mn.Bucket, mn.UseSSL)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to create MinIO client: " + err.Error()})
		return
	}
	res, err := client.Get(c.Request.Context(), key, c.GetHeader("Range"))
	if err != nil {
		if isMinioNoSuchKey(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "file does not exist on MinIO"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read MinIO file: " + err.Error()})
		return
	}
	defer res.Body.Close()

	c.Status(http.StatusOK)
	if res.Start > 0 || res.End < res.TotalSize-1 {
		c.Status(http.StatusPartialContent)
		c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", res.Start, res.End, res.TotalSize))
	}
	c.Header("Content-Type", "video/mp4")
	c.Header("Content-Length", strconv.FormatInt(res.End-res.Start+1, 10))
	c.Header("Accept-Ranges", "bytes")
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filepath.Base(key)))
	io.Copy(c.Writer, res.Body)
}

func (s *Server) persistConfig() error {
	if s.cfgPath == "" {
		return fmt.Errorf("config path not configured")
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

		switch {
		case errors.Is(err, camera.ErrCameraNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, camera.ErrPTZNotSupported):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, camera.ErrCameraOffline):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusBadGateway, gin.H{"error": "PTZ control failed: " + err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "PTZ command sent"})
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

func (s *Server) deleteSnapshot(c *gin.Context) {
	id := parseUint(c.Param("id"))
	var snap models.Snapshot
	if err := database.GetDB().First(&snap, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "snapshot not found"})
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
		logrus.Warnf("failed to delete snapshot file %s: %v", full, err)
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

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
			logrus.Warnf("failed to delete snapshot file %s: %v", full, err)
		}
	}
	if err := database.GetDB().Unscoped().Where("id > 0").Delete(&models.Snapshot{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	logrus.Infof("all snapshots cleared: %d records, %d files deleted", len(snaps), deleted)
	c.JSON(http.StatusOK, gin.H{"message": "snapshots cleared", "deleted": len(snaps)})
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

func (s *Server) discoverLAN(c *gin.Context) {
	var req struct {
		Timeout int `json:"timeout"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {

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

func (s *Server) probeONVIFCamera(c *gin.Context) {
	ip := c.Query("ip")
	if ip == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing ip parameter"})
		return
	}

	username := c.Query("username")
	password := c.Query("password")

	device, err := s.cameraMgr.ProbeONVIFCamera(ip, username, password)
	if err != nil {

		authRequired := strings.Contains(err.Error(), "authentication is required")
		c.JSON(http.StatusOK, gin.H{"device": nil, "auth_required": authRequired, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"device": device, "auth_required": false, "error": ""})
}

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
		c.JSON(http.StatusNotFound, gin.H{"error": "recording not found"})
		return
	}
	c.JSON(http.StatusOK, rec)
}

func (s *Server) downloadRecording(c *gin.Context) {
	id := parseUint(c.Param("id"))
	rec, err := storage.NewRecordingManager().GetRecordingByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "recording not found"})
		return
	}
	full := rec.FilePath
	if !filepath.IsAbs(full) {
		full = "./" + full
	}
	if _, err := os.Stat(full); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "recording file does not exist"})
		return
	}
	c.FileAttachment(full, filepath.Base(full))
}

func (s *Server) getRecordingFile(c *gin.Context) {
	id := parseUint(c.Param("id"))
	rec, err := storage.NewRecordingManager().GetRecordingByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "recording not found"})
		return
	}
	full := rec.FilePath
	if !filepath.IsAbs(full) {
		full = "./" + full
	}
	if _, err := os.Stat(full); err == nil {

		c.Header("Accept-Ranges", "bytes")
		c.File(full)
		return
	}

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
						c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read recording from WebDAV"})
						return
					}
				} else {
					logrus.Warnf("WebDAV fallback playback failed id=%d: %v", id, gerr)
				}
			}
		}

		if mn := s.currentMinIOSettings(); mn.Enabled && mn.Endpoint != "" && mn.Bucket != "" {
			if err := validateWebdavRelPath(mn.BasePath, rec.StoragePath); err == nil {
				mc, merr := minio.NewClient(mn.Endpoint, mn.AccessKey, mn.SecretKey, mn.Bucket, mn.UseSSL)
				if merr == nil {
					res, gerr := mc.Get(c.Request.Context(), rec.StoragePath, c.GetHeader("Range"))
					if gerr == nil {
						defer res.Body.Close()
						if res.Start > 0 || res.End < res.TotalSize-1 {
							c.Status(http.StatusPartialContent)
							c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", res.Start, res.End, res.TotalSize))
						} else {
							c.Status(http.StatusOK)
						}
						c.Header("Content-Type", "video/mp4")
						c.Header("Content-Length", strconv.FormatInt(res.End-res.Start+1, 10))
						c.Header("Accept-Ranges", "bytes")
						io.Copy(c.Writer, res.Body)
						return
					}
					if !isMinioNoSuchKey(gerr) {
						c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read recording from MinIO"})
						return
					}
				} else {
					logrus.Warnf("MinIO fallback playback failed id=%d: %v", id, merr)
				}
			}
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "recording file does not exist"})
}

func (s *Server) deleteRecording(c *gin.Context) {
	id := parseUint(c.Param("id"))
	rm := storage.NewRecordingManager()
	rec, err := rm.GetRecordingByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "recording not found"})
		return
	}

	full := rec.FilePath
	if !filepath.IsAbs(full) {
		full = "./" + full
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		logrus.Warnf("failed to delete recording file %s: %v", full, err)
	}

	if rec.StoragePath != "" {
		if wd := s.runtimeStorage.GetWebdav(); wd.Enabled && wd.URL != "" {
			remotePath := filepath.Join(wd.BasePath, rec.StoragePath)
			if err := webdav.NewClient(wd.URL, wd.Username, wd.Password).Delete(remotePath); err == nil {
				logrus.Infof("deleted WebDAV remote recording: %s", remotePath)
			}
		}
	}

	if err := rm.DeleteRecording(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted successfully"})
}

func (s *Server) listCameraRecordings(c *gin.Context) {
	cameraID := parseUint(c.Param("cameraId"))
	recordType := c.Query("record_type")
	start, _ := parseTimeParam(c.Query("start"))
	end, _ := parseTimeParam(c.Query("end"))

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

func (s *Server) getHLSPlaylist(c *gin.Context) {
	cameraID := c.Param("cameraId")

	stream := c.Query("stream")
	if err := s.cameraMgr.EnsurePreview(parseUint(cameraID), stream); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to start preview stream: " + err.Error()})
		return
	}
	file := "./recordings/camera_" + cameraID + "/index.m3u8"

	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(file); err == nil {
			break
		}
		if time.Now().After(deadline) {
			c.JSON(http.StatusNotFound, gin.H{"error": "playlist does not exist, preview stream start timed out"})
			return
		}
		time.Sleep(200 * time.Millisecond)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "playlist does not exist, preview stream may not be ready"})
		return
	}

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

	c.Header("Content-Type", "video/mp4")
	c.File("./recordings/camera_" + cameraID + "/segment_latest.mp4")
}

func (s *Server) getLatestSnapshot(c *gin.Context) {
	cameraID := c.Param("cameraId")

	matches, _ := filepath.Glob("./recordings/camera_" + cameraID + "/snapshot_*.jpg")
	if len(matches) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no snapshots available"})
		return
	}

	sort.Strings(matches)
	c.Header("Content-Type", "image/jpeg")
	c.File(matches[len(matches)-1])
}

func (s *Server) getRecordingHLS(c *gin.Context) {

	c.Header("Content-Type", "application/vnd.apple.mpegurl")
	c.String(http.StatusOK, "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:10\n#EXT-X-ENDLIST\n")
}

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
	al := LocaleFromContext(c)
	for i := range alerts {
		alerts[i].Message = LocalizeAlert(al, alerts[i].Message)
	}

	c.JSON(http.StatusOK, gin.H{"data": alerts, "total": total})
}

func (s *Server) getAlert(c *gin.Context) {
	l := LocaleFromContext(c)
	id := parseUint(c.Param("id"))
	var alert models.Alert
	if err := database.GetDB().First(&alert, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": TEnv(l, "alert.notfound")})
		return
	}
	alert.Message = LocalizeAlert(l, alert.Message)
	c.JSON(http.StatusOK, alert)
}

func (s *Server) acknowledgeAlert(c *gin.Context) {
	l := LocaleFromContext(c)
	id := parseUint(c.Param("id"))
	var alert models.Alert
	if err := database.GetDB().First(&alert, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": TEnv(l, "alert.notfound")})
		return
	}
	now := time.Now()

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
	c.JSON(http.StatusOK, gin.H{"message": TEnv(l, "alert.acked")})
}

func (s *Server) resolveAlert(c *gin.Context) {
	l := LocaleFromContext(c)
	id := parseUint(c.Param("id"))
	var alert models.Alert
	if err := database.GetDB().First(&alert, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": TEnv(l, "alert.notfound")})
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
	c.JSON(http.StatusOK, gin.H{"message": TEnv(l, "alert.resolved")})
}

func (s *Server) deleteAlert(c *gin.Context) {
	l := LocaleFromContext(c)
	id := parseUint(c.Param("id"))
	if err := database.GetDB().Delete(&models.Alert{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": TEnv(l, "alert.deleted")})
}

func (s *Server) clearAlerts(c *gin.Context) {
	l := LocaleFromContext(c)
	result := database.GetDB().Unscoped().Where("id > 0").Delete(&models.Alert{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": TEnv(l, "alert.cleared"), "deleted": result.RowsAffected})
}

func (s *Server) getStorageStats(c *gin.Context) {
	stats := s.storageMgr.GetStats()
	c.JSON(http.StatusOK, stats)
}

func (s *Server) triggerCleanup(c *gin.Context) {

	s.storageMgr.TriggerCleanup()
	c.JSON(http.StatusOK, gin.H{"message": "cleanup completed (checked by retention days and max storage usage)"})
}

func (s *Server) getAlertConfig(c *gin.Context) {
	c.JSON(http.StatusOK, s.cfg.Alert)
}

func (s *Server) updateAlertConfig(c *gin.Context) {

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

	if err := s.persistConfig(); err != nil {
		logrus.Warnf("failed to persist alert config: %v (this change is only in effect in memory and will be reverted after restart)", err)
	}

	c.JSON(http.StatusOK, gin.H{"message": "alert config updated"})
}

func (s *Server) sendTestAlert(c *gin.Context) {

	var req struct {
		Channel string                     `json:"channel" binding:"required"`
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
		base := s.cfg.Alert
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

	c.JSON(http.StatusOK, gin.H{"message": "test alert sent"})
}

func (s *Server) getSystemConfig(c *gin.Context) {

	c.JSON(http.StatusOK, gin.H{
		"server":  s.cfg.Server,
		"storage": s.cfg.Storage,
		"camera":  s.cfg.Camera,
	})
}

func (s *Server) updateSystemConfig(c *gin.Context) {

	c.JSON(http.StatusOK, gin.H{"message": "config updated"})
}

func (s *Server) getSystemInfo(c *gin.Context) {

	var memTotalKB, memAvailKB int64
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			var v int64
			fmt.Sscanf(fields[1], "%d", &v)
			switch fields[0] {
			case "MemTotal:":
				memTotalKB = v
			case "MemAvailable:":
				memAvailKB = v
			}
		}
	}

	diskPath := s.cfg.Storage.Local.RootPath
	if diskPath == "" {
		diskPath = "."
	}
	diskTotalMB, diskUsedMB := diskUsageMB(diskPath)

	var dbSize int64
	if p := s.cfg.Database.SQLite.Path; p != "" {
		if fi, err := os.Stat(p); err == nil {
			dbSize = fi.Size()
		}
	}

	var cameraCount, recordingCount int64
	if db := database.GetDB(); db != nil {
		db.Model(&models.Camera{}).Count(&cameraCount)
		db.Model(&models.Recording{}).Count(&recordingCount)
	}

	var logSize int64
	if p := s.logFilePath(); p != "" {
		if fi, err := os.Stat(p); err == nil {
			logSize = fi.Size()
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"version":         s.version,
		"build_time":      s.buildTime,
		"git_commit":      s.gitCommit,
		"go_version":      runtime.Version(),
		"os":              runtime.GOOS,
		"arch":            runtime.GOARCH,
		"pid":             os.Getpid(),
		"start_time":      s.startTime.Unix(),
		"uptime":          int64(time.Since(s.startTime).Seconds()),
		"cpu_count":       runtime.NumCPU(),
		"mem_total_mb":    memTotalKB / 1024,
		"mem_used_mb":     (memTotalKB - memAvailKB) / 1024,
		"disk_total_mb":   diskTotalMB,
		"disk_used_mb":    diskUsedMB,
		"disk_path":       diskPath,
		"db_size":         dbSize,
		"camera_count":    cameraCount,
		"recording_count": recordingCount,
		"log_size":        logSize,
		"config_path":     s.cfgPath,
	})
}

func (s *Server) restartSystem(c *gin.Context) {
	if s.restartFunc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "this build does not support online restart"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "restarting system..."})
	go func() {
		time.Sleep(300 * time.Millisecond)
		if err := s.restartFunc(""); err != nil {
			logrus.Errorf("in-place restart failed: %v", err)
		}
	}()
}

func (s *Server) logFilePath() string {
	p := s.cfg.Logging.Output
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
	}
	return p
}

func (s *Server) getLogTail(c *gin.Context) {
	path := s.logFilePath()
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "log file not configured (logging.output)"})
		return
	}
	n, _ := strconv.Atoi(c.DefaultQuery("lines", "200"))
	if n <= 0 || n > 2000 {
		n = 200
	}
	keyword := strings.ToLower(c.Query("keyword"))

	var (
		allLines []string
		size     int64
	)
	if fi, err := os.Stat(path); err == nil {
		size = fi.Size()
		f, err := os.Open(path)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer f.Close()

		const tailBytes = int64(4 * 1024 * 1024)
		if size > tailBytes {
			if _, err := f.Seek(size-tailBytes, io.SeekStart); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		data, _ := io.ReadAll(f)
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		if len(lines) > 0 && lines[0] == "" {
			lines = lines[1:]
		}
		allLines = lines
	}
	if keyword != "" {
		filtered := allLines[:0]
		for _, l := range allLines {
			if strings.Contains(strings.ToLower(l), keyword) {
				filtered = append(filtered, l)
			}
		}
		allLines = filtered
	}
	if len(allLines) > n {
		allLines = allLines[len(allLines)-n:]
	}
	c.JSON(http.StatusOK, gin.H{
		"file":  path,
		"size":  size,
		"total": len(allLines),
		"lines": allLines,
	})
}

func (s *Server) getLogFiles(c *gin.Context) {
	path := s.logFilePath()
	type logFile struct {
		Name    string `json:"name"`
		Size    int64  `json:"size"`
		ModTime int64  `json:"mod_time"`
	}
	files := []logFile{}
	if path != "" {
		dir := filepath.Dir(path)
		base := filepath.Base(path)
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if name == base || strings.HasPrefix(name, base+".") {
				if fi, err := e.Info(); err == nil {
					files = append(files, logFile{Name: name, Size: fi.Size(), ModTime: fi.ModTime().Unix()})
				}
			}
		}
		sort.Slice(files, func(i, j int) bool {
			if files[i].Name == base {
				return true
			}
			if files[j].Name == base {
				return false
			}
			return files[i].Name < files[j].Name
		})
	}
	c.JSON(http.StatusOK, gin.H{"dir": filepath.Dir(path), "files": files})
}

func (s *Server) clearLogs(c *gin.Context) {
	path := s.logFilePath()
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "log file not configured (logging.output)"})
		return
	}
	var freed int64

	if f, err := os.OpenFile(path, os.O_TRUNC|os.O_WRONLY, 0644); err == nil {
		if fi, serr := f.Stat(); serr == nil {
			freed += fi.Size()
		}
		f.Close()
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open log file: " + err.Error()})
		return
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)
	removed := 0
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasPrefix(e.Name(), base+".") {
				continue
			}
			if fi, ierr := e.Info(); ierr == nil {
				freed += fi.Size()
			}
			if os.Remove(filepath.Join(dir, e.Name())) == nil {
				removed++
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("logs cleared (removed %d rotated files)", removed),
		"freed":   freed,
		"removed": removed,
	})
}

func (s *Server) backupDir() (string, error) {
	dbPath := s.cfg.Database.SQLite.Path
	if dbPath == "" {
		return "", fmt.Errorf("database path not configured")
	}
	return filepath.Join(filepath.Dir(dbPath), "backups"), nil
}

func isSafeBackupName(name string) bool {
	if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return false
	}
	return strings.HasPrefix(name, "surveillance-") && strings.HasSuffix(name, ".db")
}

func (s *Server) createBackup(c *gin.Context) {
	dir, err := s.backupDir()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create backup directory: " + err.Error()})
		return
	}
	name := "surveillance-" + time.Now().Format("20060102-150405") + ".db"
	full := filepath.Join(dir, name)

	usedMethod := ""
	if db := database.GetDB(); db != nil && s.cfg.Database.Type == "sqlite" {

		sql := "VACUUM INTO '" + strings.ReplaceAll(full, "'", "''") + "'"
		if db.Exec(sql).Error == nil {
			usedMethod = "vacuum_into"
		}
	}
	if usedMethod == "" {

		dbPath := s.cfg.Database.SQLite.Path
		if err := copyFileAll(dbPath, full); err != nil {
			os.Remove(full)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "backup failed: " + err.Error()})
			return
		}
		if _, err := os.Stat(dbPath + "-wal"); err == nil {
			copyFileAll(dbPath+"-wal", full+"-wal")
		}
		usedMethod = "file_copy"
	}
	var size int64
	if fi, err := os.Stat(full); err == nil {
		size = fi.Size()
	}
	c.JSON(http.StatusOK, gin.H{"message": "backup successful", "file": name, "size": size, "method": usedMethod})
}

func copyFileAll(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func (s *Server) listBackups(c *gin.Context) {
	type backupFile struct {
		Name    string `json:"name"`
		Size    int64  `json:"size"`
		ModTime int64  `json:"mod_time"`
	}
	files := []backupFile{}
	if dir, err := s.backupDir(); err == nil {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".db") {
					continue
				}
				if fi, ierr := e.Info(); ierr == nil {
					files = append(files, backupFile{Name: e.Name(), Size: fi.Size(), ModTime: fi.ModTime().Unix()})
				}
			}
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ModTime > files[j].ModTime })
	c.JSON(http.StatusOK, gin.H{"files": files})
}

func (s *Server) downloadBackup(c *gin.Context) {
	name := c.Param("name")
	if !isSafeBackupName(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file name"})
		return
	}
	dir, _ := s.backupDir()
	full := filepath.Join(dir, name)
	if fi, err := os.Stat(full); err != nil || fi.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}
	c.Header("Content-Disposition", "attachment; filename="+name)
	c.File(full)
}

func (s *Server) deleteBackup(c *gin.Context) {
	name := c.Param("name")
	if !isSafeBackupName(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file name"})
		return
	}
	dir, _ := s.backupDir()
	full := filepath.Join(dir, name)
	if _, err := os.Stat(full); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}
	if err := os.Remove(full); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted " + name})
}

const (
	defaultUpdateAPIBase = "https://api.github.com"
	defaultGitHubRepo    = "Assen998/surveillance-system"
)

func (s *Server) updateEndpoint() (string, string) {
	base := os.Getenv("SURVEILLANCE_UPDATE_BASE")
	if base == "" {
		base = strings.TrimSpace(s.cfg.Update.BaseURL)
	}
	if base == "" {
		base = defaultUpdateAPIBase
	}
	repo := os.Getenv("SURVEILLANCE_GITHUB_REPO")
	if repo == "" {
		repo = strings.TrimSpace(s.cfg.Update.GitHubRepo)
	}
	if repo == "" {
		repo = defaultGitHubRepo
	}
	return strings.TrimRight(base, "/"), repo
}

func validProxyAddr(p string) bool {
	u, err := url.Parse(p)
	if err != nil || u.Host == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "socks5"
}

func (s *Server) updateHTTPClient(total time.Duration) *http.Client {
	tr := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		MaxIdleConns:          4,
		IdleConnTimeout:       60 * time.Second,
		Proxy:                 http.ProxyFromEnvironment,
	}
	if p := strings.TrimSpace(s.cfg.Update.Proxy); p != "" && validProxyAddr(p) {
		if u, err := url.Parse(p); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Timeout: total, Transport: tr}
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
	URL                string `json:"url"`
}

type githubRelease struct {
	TagName     string               `json:"tag_name"`
	Body        string               `json:"body"`
	PublishedAt string               `json:"published_at"`
	Assets      []githubReleaseAsset `json:"assets"`
}

func (s *Server) fetchLatestRelease() (*githubRelease, error) {
	base, repo := s.updateEndpoint()
	url := fmt.Sprintf("%s/repos/%s/releases/latest", base, repo)
	client := s.updateHTTPClient(30 * time.Second)
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update source returned HTTP %d", resp.StatusCode)
	}
	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func (r *githubRelease) assetForCurrentPlatform() *githubReleaseAsset {
	ver := strings.TrimPrefix(r.TagName, "v")
	suffix := fmt.Sprintf("-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	wantName := fmt.Sprintf("surveillance-system-%s%s", ver, suffix)
	for i := range r.Assets {
		if r.Assets[i].Name == wantName {
			return &r.Assets[i]
		}
	}
	for i := range r.Assets {
		if strings.HasSuffix(r.Assets[i].Name, suffix) {
			return &r.Assets[i]
		}
	}
	if runtime.GOARCH == "arm" {
		prefix := fmt.Sprintf("surveillance-system-%s-%s-arm", ver, runtime.GOOS)
		for i := range r.Assets {
			if strings.HasPrefix(r.Assets[i].Name, prefix) && strings.HasSuffix(r.Assets[i].Name, ".tar.gz") {
				return &r.Assets[i]
			}
		}
	}
	return nil
}

func compareVersions(a, b string) int {
	pa := strings.Split(strings.TrimPrefix(strings.TrimSpace(a), "v"), ".")
	pb := strings.Split(strings.TrimPrefix(strings.TrimSpace(b), "v"), ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var xa, xb int
		if i < len(pa) {
			xa, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			xb, _ = strconv.Atoi(pb[i])
		}
		if xa < xb {
			return -1
		}
		if xa > xb {
			return 1
		}
	}
	return 0
}

func (s *Server) checkUpdate(c *gin.Context) {
	rel, err := s.fetchLatestRelease()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to check for update: " + err.Error()})
		return
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	asset := rel.assetForCurrentPlatform()
	resp := gin.H{
		"current_version": s.version,
		"latest_version":  latest,
		"has_update":      compareVersions(latest, s.version) > 0,
		"release_notes":   rel.Body,
		"published_at":    rel.PublishedAt,
	}
	if asset != nil {
		resp["asset_name"] = asset.Name
		resp["asset_size"] = asset.Size
	} else {
		resp["asset_name"] = ""
		resp["error"] = fmt.Sprintf("latest release has no asset for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	c.JSON(http.StatusOK, resp)
}

func extractBinaryFromTarGz(tarPath, dst string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != "surveillance-server" {
			continue
		}
		out, err := os.Create(dst)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	}
	return fmt.Errorf("surveillance-server binary not found in update package")
}

func (s *Server) performUpdate(c *gin.Context) {
	if s.restartFunc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "this build does not support online update"})
		return
	}
	rel, err := s.fetchLatestRelease()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch latest version: " + err.Error()})
		return
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	if compareVersions(latest, s.version) <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("already at the latest version (%s)", s.version)})
		return
	}
	asset := rel.assetForCurrentPlatform()
	if asset == nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("latest version %s has no asset for %s/%s", rel.TagName, runtime.GOOS, runtime.GOARCH)})
		return
	}

	exe, err := os.Executable()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to determine running directory: " + err.Error()})
		return
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)
	tmpTar := filepath.Join(dir, ".update-"+latest+".tar.gz")
	newBin := filepath.Join(dir, ".surveillance-server.new")
	defer os.Remove(tmpTar)
	defer os.Remove(newBin)

	client := s.updateHTTPClient(10 * time.Minute)
	resp, err := client.Get(asset.BrowserDownloadURL)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "download failed: " + err.Error()})
		return
	}
	f, err := os.Create(tmpTar)
	if err != nil {
		resp.Body.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create temporary file: " + err.Error()})
		return
	}
	n, copyErr := io.Copy(f, resp.Body)
	resp.Body.Close()
	f.Close()
	if copyErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "download interrupted: " + copyErr.Error()})
		return
	}
	if asset.Size > 0 && n != asset.Size {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("incomplete download (expected %d bytes, got %d bytes)", asset.Size, n)})
		return
	}

	if err := extractBinaryFromTarGz(tmpTar, newBin); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to extract update package: " + err.Error()})
		return
	}

	if fi, err := os.Stat(newBin); err != nil || fi.Size() < 512*1024 {
		c.JSON(http.StatusBadGateway, gin.H{"error": "new binary validation failed (file missing or too small)"})
		return
	}
	hdr := make([]byte, 4)
	if hf, err := os.Open(newBin); err == nil {
		io.ReadFull(hf, hdr)
		hf.Close()
	}
	if string(hdr) != "\x7fELF" {
		c.JSON(http.StatusBadGateway, gin.H{"error": "new binary validation failed (not an ELF executable)"})
		return
	}
	if err := os.Chmod(newBin, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set executable permission: " + err.Error()})
		return
	}

	bak := filepath.Join(dir, "surveillance-server.bak")
	os.Remove(bak)
	if err := os.Rename(exe, bak); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to back up old binary: " + err.Error()})
		return
	}
	if err := os.Rename(newBin, exe); err != nil {
		os.Rename(bak, exe)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to replace binary: " + err.Error()})
		return
	}

	logrus.Infof("program updated %s -> %s, preparing in-place restart", s.version, latest)
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("updated to %s, restarting system...", latest), "new_version": latest})
	go func() {
		time.Sleep(500 * time.Millisecond)
		if err := s.restartFunc(exe); err != nil {
			logrus.Errorf("restart after update failed: %v", err)
		}
	}()
}

func (s *Server) getUpdateConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"proxy":       s.cfg.Update.Proxy,
		"github_repo": s.cfg.Update.GitHubRepo,
		"base_url":    s.cfg.Update.BaseURL,
	})
}

func (s *Server) updateUpdateConfig(c *gin.Context) {
	var req struct {
		Proxy      *string `json:"proxy"`
		GitHubRepo *string `json:"github_repo"`
		BaseURL    *string `json:"base_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Proxy != nil {
		p := strings.TrimSpace(*req.Proxy)
		if p != "" && !validProxyAddr(p) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proxy address format, expected e.g. http://192.168.1.5:7890 (http/https/socks5 supported)"})
			return
		}
		s.cfg.Update.Proxy = p
	}
	if req.GitHubRepo != nil {
		s.cfg.Update.GitHubRepo = strings.TrimSpace(*req.GitHubRepo)
	}
	if req.BaseURL != nil {
		s.cfg.Update.BaseURL = strings.TrimRight(strings.TrimSpace(*req.BaseURL), "/")
	}
	if err := s.persistConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to persist config: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "update settings saved (effective on next check/update, no restart required)", "proxy": s.cfg.Update.Proxy})
}

func (s *Server) handleWebSocket(c *gin.Context) {
	conn, err := s.wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logrus.Errorf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	conn.WriteJSON(gin.H{"type": "welcome", "message": "Connected to surveillance system"})

	for {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}

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
	}
}

func parseUint(s string) uint {
	var id uint
	fmt.Sscanf(s, "%d", &id)
	return id
}
