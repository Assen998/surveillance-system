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

var startTime = time.Now()

func main() {

	configPath := resolveConfigPath(os.Args)

	cfg, err := config.Load(configPath)
	if err != nil {
		logrus.Fatalf("failed to load config: %v", err)
	}

	initLogging(cfg)

	logrus.Infof("starting surveillance system v%s (build: %s, commit: %s)", Version, BuildTime, GitCommit)

	if err := database.Init(cfg); err != nil {
		logrus.Fatalf("failed to initialize database: %v", err)
	}
	defer database.Close()

	ffmpegMgr := ffmpeg.NewManager()

	storageRT := storage.NewRuntimeStorage(cfg)

	cameraMgr := camera.NewCameraManager(cfg, ffmpegMgr)
	cameraMgr.SetRuntimeStorage(storageRT)
	storageMgr := storage.NewManager(cfg)
	storageMgr.SetRuntimeStorage(storageRT)
	alertMgr := alert.NewManager(cfg)

	onvifEventMgr := onvifevent.NewManager(cfg, func(a *models.Alert) {
		alertMgr.OnAlert(a)
		if a.Type == models.AlertTypeMotion {
			cameraMgr.TriggerMotionRecording(a.CameraID)
		}
	})

	if err := cameraMgr.Start(); err != nil {
		logrus.Fatalf("failed to start camera manager: %v", err)
	}

	if err := storageMgr.Start(); err != nil {
		logrus.Fatalf("failed to start storage manager: %v", err)
	}

	if err := alertMgr.Start(); err != nil {
		logrus.Fatalf("failed to start alert manager: %v", err)
	}

	if err := onvifEventMgr.Start(); err != nil {
		logrus.Fatalf("failed to start ONVIF event subscription manager: %v", err)
	}

	gin.SetMode(cfg.Server.Mode)

	apiServer := api.NewServer(cfg, cameraMgr, storageMgr, alertMgr)
	apiServer.SetRuntimeStorage(storageRT)
	apiServer.SetConfigPath(configPath)
	apiServer.SetVersionInfo(Version, BuildTime, GitCommit, startTime)

	httpServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.HTTPPort),
		Handler: apiServer.Router(),
	}

	go func() {
		logrus.Infof("HTTP server listening on %s:%d", cfg.Server.Host, cfg.Server.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logrus.Fatalf("HTTP server failed to start: %v", err)
		}
	}()

	shutdown := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			logrus.Errorf("HTTP server shutdown failed: %v", err)
		}
		cameraMgr.Stop()
		storageMgr.Stop()
		alertMgr.Stop()
		onvifEventMgr.Stop()
		ffmpegMgr.StopAll()
		database.Close()
	}

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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logrus.Info("shutting down server...")
	shutdown()
	logrus.Info("server stopped")
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

func resolveConfigPath(args []string) string {
	if len(args) > 1 && args[1] != "" {
		return args[1]
	}
	for _, p := range []string{"config.yaml", "configs/config.yaml"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return "config.yaml"
}
