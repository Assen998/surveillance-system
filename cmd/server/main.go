package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/yourorg/surveillance-system/internal/alert"
	"github.com/yourorg/surveillance-system/internal/api"
	"github.com/yourorg/surveillance-system/internal/camera"
	"github.com/yourorg/surveillance-system/internal/config"
	"github.com/yourorg/surveillance-system/internal/database"
	"github.com/yourorg/surveillance-system/internal/models"
	"github.com/yourorg/surveillance-system/internal/onvifevent"
	"github.com/yourorg/surveillance-system/internal/storage"
	"github.com/yourorg/surveillance-system/pkg/ffmpeg"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// startTime 进程启动时间（系统信息接口展示用）
var startTime = time.Now()

func main() {
	// 解析命令行参数：可选传配置文件路径，未传时自动查找
	configPath := resolveConfigPath(os.Args)

	// 加载配置
	cfg, err := config.Load(configPath)
	if err != nil {
		logrus.Fatalf("加载配置失败: %v", err)
	}

	// 初始化日志
	initLogging(cfg)

	logrus.Infof("启动监控录像系统 v%s (build: %s, commit: %s)", Version, BuildTime, GitCommit)

	// 初始化数据库
	if err := database.Init(cfg); err != nil {
		logrus.Fatalf("初始化数据库失败: %v", err)
	}
	defer database.Close()

	// 初始化核心组件
	ffmpegMgr := ffmpeg.NewManager()

	// 运行时存储设置（设置页可热更新分段时长/存储路径/WebDAV）
	storageRT := storage.NewRuntimeStorage(cfg)

	cameraMgr := camera.NewCameraManager(cfg, ffmpegMgr)
	cameraMgr.SetRuntimeStorage(storageRT)
	storageMgr := storage.NewManager(cfg)
	storageMgr.SetRuntimeStorage(storageRT)
	alertMgr := alert.NewManager(cfg)

	// ONVIF 事件订阅（摄像头主动上报的报警），复用报警推送回调
	onvifEventMgr := onvifevent.NewManager(cfg, func(a *models.Alert) {
		alertMgr.OnAlert(a)
		if a.Type == models.AlertTypeMotion {
			cameraMgr.TriggerMotionRecording(a.CameraID)
		}
	})

	// 启动组件
	if err := cameraMgr.Start(); err != nil {
		logrus.Fatalf("启动摄像头管理器失败: %v", err)
	}

	if err := storageMgr.Start(); err != nil {
		logrus.Fatalf("启动存储管理器失败: %v", err)
	}

	if err := alertMgr.Start(); err != nil {
		logrus.Fatalf("启动报警管理器失败: %v", err)
	}

	if err := onvifEventMgr.Start(); err != nil {
		logrus.Fatalf("启动 ONVIF 事件订阅管理器失败: %v", err)
	}

	// 设置 Gin 模式
	gin.SetMode(cfg.Server.Mode)

	// 创建 API 服务器
	apiServer := api.NewServer(cfg, cameraMgr, storageMgr, alertMgr)
	apiServer.SetRuntimeStorage(storageRT)
	apiServer.SetConfigPath(configPath)
	apiServer.SetVersionInfo(Version, BuildTime, GitCommit, startTime)

	// 启动 HTTP 服务器
	httpServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.HTTPPort),
		Handler: apiServer.Router(),
	}

	go func() {
		logrus.Infof("HTTP 服务器启动在 %s:%d", cfg.Server.Host, cfg.Server.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logrus.Fatalf("HTTP 服务器启动失败: %v", err)
		}
	}()

	// WebSocket 服务器（可选，合并到同一端口）
	// 如果需要单独端口，取消注释下面代码
	// wsServer := &http.Server{
	// 	Addr:    fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.WSPort),
	// 	Handler: apiServer.WSRouter(),
	// }
	// go func() { ... }()

	// 优雅关闭（信号路径 与 页面「重启系统/在线更新」共用）
	shutdown := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			logrus.Errorf("HTTP 服务器关闭失败: %v", err)
		}
		cameraMgr.Stop()
		storageMgr.Stop()
		alertMgr.Stop()
		onvifEventMgr.Stop()
		ffmpegMgr.StopAll()
		database.Close()
	}

	// 原地重启：优雅关闭后 os.Exec 重新执行二进制（PID 不变，systemd 无感知；
	// 在线更新场景传入新二进制路径，重启即运行新版本）
	apiServer.SetRestartFunc(func(newBinary string) error {
		target := newBinary
		if target == "" {
			t, err := os.Executable()
			if err != nil {
				return err
			}
			if r, err := filepath.EvalSymlinks(t); err == nil {
				t = r
			}
			target = t
		}
		shutdown()
		return selfExec(target, append([]string{target}, os.Args[1:]...), os.Environ())
	})

	// 等待信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logrus.Info("正在关闭服务器...")
	shutdown()
	logrus.Info("服务器已关闭")
}

func initLogging(cfg *config.Config) {
	level, err := logrus.ParseLevel(cfg.Logging.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	logrus.SetLevel(level)

	if cfg.Logging.Format == "json" {
		logrus.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})
	} else {
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
	}

	// 输出到文件（lumberjack 轮转：max_size MB 触发、保留 max_backups 份），
	// 同时保留 stdout（systemd journal 仍可查）。
	if cfg.Logging.Output != "" {
		lj := &lumberjack.Logger{
			Filename:   cfg.Logging.Output,
			MaxSize:    cfg.Logging.MaxSize,
			MaxBackups: cfg.Logging.MaxBackups,
			MaxAge:     cfg.Logging.MaxAge,
			Compress:   cfg.Logging.Compress,
		}
		if lj.MaxSize <= 0 {
			lj.MaxSize = 100
		}
		if lj.MaxBackups <= 0 {
			lj.MaxBackups = 10
		}
		logrus.SetOutput(io.MultiWriter(os.Stdout, lj))
	}
}

// resolveConfigPath 解析配置文件路径：
// 1. 命令行显式指定（os.Args[1]）时优先使用；
// 2. 否则按顺序查找：同目录 config.yaml → configs/config.yaml。
// 这样 Release 单二进制（config.yaml 与二进制同目录）可直接运行，
// 而源码目录（configs/config.yaml）下 go run 也能正常工作。
func resolveConfigPath(args []string) string {
	if len(args) > 1 && args[1] != "" {
		return args[1]
	}
	for _, p := range []string{"config.yaml", "configs/config.yaml"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// 都不存在时仍返回默认路径，交由 config.Load 报出明确错误
	return "config.yaml"
}