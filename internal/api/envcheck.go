package api

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourorg/surveillance-system/internal/config"
	"github.com/yourorg/surveillance-system/internal/database"
	"github.com/yourorg/surveillance-system/internal/models"
	"github.com/yourorg/surveillance-system/pkg/minio"
	"github.com/yourorg/surveillance-system/pkg/webdav"

	"golang.org/x/crypto/bcrypt"
)

const (
	envOK   = "ok"
	envWarn = "warn"
	envFail = "fail"
)

type CheckItem struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	Fix      string `json:"fix"`
	Required bool   `json:"required"`
}

type EnvReport struct {
	OK         bool        `json:"ok"`
	CriticalOK bool        `json:"critical_ok"`
	Checks     []CheckItem `json:"checks"`
}

func (r *EnvReport) compute() {
	r.OK = true
	r.CriticalOK = true
	for _, c := range r.Checks {
		if c.Status == envFail {
			r.OK = false
			if c.Required {
				r.CriticalOK = false
			}
		} else if c.Status == envWarn {
			r.OK = false
		}
	}
}

func (s *Server) runEnvChecks() EnvReport {
	return s.runEnvChecksL(localeZH)
}

func (s *Server) runEnvChecksL(l string) EnvReport {
	var report EnvReport
	report.Checks = []CheckItem{}

	if v, err := runVersionCmd("ffmpeg"); err != nil {
		report.Checks = append(report.Checks, CheckItem{
			Name: "ffmpeg", Status: envFail, Required: true,
			Detail: TEnv(l, "env.ffmpeg.fail"),
			Fix:    TEnv(l, "env.ffmpeg.fix"),
		})
	} else {
		report.Checks = append(report.Checks, CheckItem{
			Name: "ffmpeg", Status: envOK, Detail: v,
		})
	}

	if v, err := runVersionCmd("ffprobe"); err != nil {
		report.Checks = append(report.Checks, CheckItem{
			Name: "ffprobe", Status: envFail, Required: true,
			Detail: TEnv(l, "env.ffprobe.fail"),
			Fix:    TEnv(l, "env.ffprobe.fix"),
		})
	} else {
		report.Checks = append(report.Checks, CheckItem{
			Name: "ffprobe", Status: envOK, Detail: v,
		})
	}

	if s.cfg.Database.Type == "sqlite" {
		dbDir := filepath.Dir(s.cfg.Database.SQLite.Path)
		if err := checkDirWritable(dbDir); err != nil {
			report.Checks = append(report.Checks, CheckItem{
				Name: TEnv(l, "env.dbdir.name"), Status: envFail, Required: true,
				Detail: TEnv(l, "env.dbdir.fail", dbDir, err),
				Fix:    TEnv(l, "env.dbdir.fix", dbDir),
			})
		} else {
			report.Checks = append(report.Checks, CheckItem{
				Name: TEnv(l, "env.dbdir.name"), Status: envOK, Detail: dbDir,
			})
		}
	}

	recRoot := s.currentStorageSettings().RootPath
	if recRoot == "" {
		recRoot = "./recordings"
	}
	if err := os.MkdirAll(recRoot, 0755); err != nil || checkDirWritable(recRoot) != nil {
		report.Checks = append(report.Checks, CheckItem{
			Name: TEnv(l, "env.recdir.name"), Status: envFail, Required: true,
			Detail: TEnv(l, "env.recdir.fail", recRoot),
			Fix:    TEnv(l, "env.recdir.fix", recRoot, recRoot),
		})
	} else {
		report.Checks = append(report.Checks, CheckItem{
			Name: TEnv(l, "env.recdir.name"), Status: envOK, Detail: recRoot,
		})
	}

	logPath := s.cfg.Logging.Output
	if logPath != "" {
		logDir := filepath.Dir(logPath)
		if err := os.MkdirAll(logDir, 0755); err != nil || checkDirWritable(logDir) != nil {
			report.Checks = append(report.Checks, CheckItem{
				Name: TEnv(l, "env.logdir.name"), Status: envFail, Required: true,
				Detail: TEnv(l, "env.logdir.fail", logDir),
				Fix:    TEnv(l, "env.logdir.fix", logDir, logDir),
			})
		} else {
			report.Checks = append(report.Checks, CheckItem{
				Name: TEnv(l, "env.logdir.name"), Status: envOK, Detail: logDir,
			})
		}
	}

	if free, err := diskFreeBytes(recRoot); err == nil {
		const gb = 1024 * 1024 * 1024
		switch {
		case free < 500*1024*1024:
			report.Checks = append(report.Checks, CheckItem{
				Name: TEnv(l, "env.disk.name"), Status: envFail, Required: true,
				Detail: TEnv(l, "env.disk.fail", formatBytes(free)),
				Fix:    TEnv(l, "env.disk.failfix"),
			})
		case free < 2*gb:
			report.Checks = append(report.Checks, CheckItem{
				Name: TEnv(l, "env.disk.name"), Status: envWarn,
				Detail: TEnv(l, "env.disk.warn", formatBytes(free)),
				Fix:    TEnv(l, "env.disk.warnfix"),
			})
		default:
			report.Checks = append(report.Checks, CheckItem{
				Name: TEnv(l, "env.disk.name"), Status: envOK, Detail: TEnv(l, "env.disk.ok", formatBytes(free)),
			})
		}
	}

	if wd := s.currentWebdavSettings(); wd.Enabled && wd.URL != "" {
		if err := checkWebdavReachable(wd.URL, wd.Username, wd.Password, wd.BasePath); err != nil {
			report.Checks = append(report.Checks, CheckItem{
				Name: TEnv(l, "env.webdav.name"), Status: envWarn,
				Detail: err.Error(),
				Fix:    TEnv(l, "env.webdav.fix"),
			})
		} else {
			report.Checks = append(report.Checks, CheckItem{
				Name: TEnv(l, "env.webdav.name"), Status: envOK, Detail: wd.URL,
			})
		}
	}

	if mn := s.currentMinIOSettings(); mn.Enabled && mn.Endpoint != "" {
		if err := checkMinioReachable(mn); err != nil {
			report.Checks = append(report.Checks, CheckItem{
				Name: TEnv(l, "env.minio.name"), Status: envWarn,
				Detail: err.Error(),
				Fix:    TEnv(l, "env.minio.fix"),
			})
		} else {
			report.Checks = append(report.Checks, CheckItem{
				Name: TEnv(l, "env.minio.name"), Status: envOK, Detail: mn.Endpoint + " / " + mn.Bucket,
			})
		}
	}

	report.compute()
	return report
}

func checkWebdavReachable(url, username, password, basePath string) error {
	c := webdav.NewClient(url, username, password)
	return c.CheckWithTimeout(basePath, 5*time.Second)
}

func checkMinioReachable(mnCfg config.MinIOConfig) error {
	client, err := minio.NewClient(mnCfg.Endpoint, mnCfg.AccessKey, mnCfg.SecretKey, mnCfg.Bucket, mnCfg.UseSSL)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return client.EnsureBucket(ctx)
}

func (s *Server) needSetup() bool {
	var count int64
	database.DB.Model(&models.User{}).Where("role = ?", models.UserRoleAdmin).Count(&count)
	return count == 0
}

func (s *Server) getSetupStatus(c *gin.Context) {
	c.JSON(200, gin.H{
		"need_setup": s.needSetup(),
		"env":        s.runEnvChecksL(LocaleFromContext(c)),
	})
}

type setupRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (s *Server) postSetup(c *gin.Context) {
	l := LocaleFromContext(c)
	if !s.needSetup() {
		c.JSON(403, gin.H{"error": TEnv(l, "setup.done403")})
		return
	}
	var req setupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": TEnv(l, "setup.param", err)})
		return
	}
	username := strings.TrimSpace(req.Username)
	if len(username) < 3 || len(username) > 50 {
		c.JSON(400, gin.H{"error": TEnv(l, "setup.username.len")})
		return
	}
	if len(req.Password) < 6 {
		c.JSON(400, gin.H{"error": TEnv(l, "setup.password.min")})
		return
	}

	var count int64
	database.DB.Model(&models.User{}).Count(&count)
	if count > 0 {
		c.JSON(403, gin.H{"error": TEnv(l, "setup.done403")})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(500, gin.H{"error": TEnv(l, "setup.hash")})
		return
	}

	admin := &models.User{
		Username: username,
		Password: string(hash),
		Email:    username + "@localhost",
		Role:     models.UserRoleAdmin,
		Status:   "active",
	}
	if err := database.DB.Create(admin).Error; err != nil {
		c.JSON(500, gin.H{"error": TEnv(l, "setup.create", err)})
		return
	}

	token := makeToken(admin.Username, admin.Role, tokenTTL)
	c.JSON(200, gin.H{
		"token": token,
		"user": gin.H{
			"username": admin.Username,
			"role":     admin.Role,
		},
	})
}

func (s *Server) getEnvCheck(c *gin.Context) {
	c.JSON(200, s.runEnvChecksL(LocaleFromContext(c)))
}

func runVersionCmd(name string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, "-version").Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) != "" {
			return strings.TrimSpace(line), nil
		}
	}
	return "", fmt.Errorf("cannot parse version output")
}

func checkDirWritable(dir string) error {
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("directory does not exist: %w", err)
	}
	f, err := os.CreateTemp(dir, ".envcheck_*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
