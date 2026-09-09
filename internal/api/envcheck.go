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

// ========== 运行环境检测 ==========

const (
	envOK   = "ok"
	envWarn = "warn"
	envFail = "fail"
)

// CheckItem 单项环境检测结果
type CheckItem struct {
	Name     string `json:"name"`
	Status   string `json:"status"` // ok | warn | fail
	Detail   string `json:"detail"`
	Fix      string `json:"fix"`      // 修复建议（warn/fail 时展示）
	Required bool   `json:"required"` // 关键项：fail 时阻止首次设置
}

// EnvReport 环境检测报告
type EnvReport struct {
	OK         bool        `json:"ok"`          // 全部通过（无 fail/warn）
	CriticalOK bool        `json:"critical_ok"` // 关键项全部通过（无关键 fail）
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

// runEnvChecks 执行全部环境检测
func (s *Server) runEnvChecks() EnvReport {
	var report EnvReport
	report.Checks = []CheckItem{}

	// 1. ffmpeg（关键：拉流/录像/预览全部依赖）
	if v, err := runVersionCmd("ffmpeg"); err != nil {
		report.Checks = append(report.Checks, CheckItem{
			Name: "ffmpeg", Status: envFail, Required: true,
			Detail: "未找到 ffmpeg，拉流/录像/预览均不可用",
			Fix: "安装：Debian/Armbian: sudo apt update && sudo apt install -y ffmpeg；" +
				"RHEL/Fedora: sudo dnf install -y ffmpeg；" +
				"其他系统可下载对应平台的静态编译版并放入 PATH",
		})
	} else {
		report.Checks = append(report.Checks, CheckItem{
			Name: "ffmpeg", Status: envOK, Detail: v,
		})
	}

	// 2. ffprobe（关键：摄像头连接测试/流探测）
	if v, err := runVersionCmd("ffprobe"); err != nil {
		report.Checks = append(report.Checks, CheckItem{
			Name: "ffprobe", Status: envFail, Required: true,
			Detail: "未找到 ffprobe，摄像头连接测试不可用",
			Fix:    "与 ffmpeg 同一软件包：sudo apt install -y ffmpeg（或 dnf install -y ffmpeg）",
		})
	} else {
		report.Checks = append(report.Checks, CheckItem{
			Name: "ffprobe", Status: envOK, Detail: v,
		})
	}

	// 3. 数据库目录可写（关键：SQLite WAL 模式需要写临时文件）
	if s.cfg.Database.Type == "sqlite" {
		dbDir := filepath.Dir(s.cfg.Database.SQLite.Path)
		if err := checkDirWritable(dbDir); err != nil {
			report.Checks = append(report.Checks, CheckItem{
				Name: "数据库目录", Status: envFail, Required: true,
				Detail: fmt.Sprintf("%s 不可写: %v", dbDir, err),
				Fix:    fmt.Sprintf("检查目录权限：sudo chmod -R u+w %s，并确认服务运行用户拥有写权限", dbDir),
			})
		} else {
			report.Checks = append(report.Checks, CheckItem{
				Name: "数据库目录", Status: envOK, Detail: dbDir,
			})
		}
	}

	// 4. 录像目录可写（关键）
	recRoot := s.currentStorageSettings().RootPath
	if recRoot == "" {
		recRoot = "./recordings"
	}
	if err := os.MkdirAll(recRoot, 0755); err != nil || checkDirWritable(recRoot) != nil {
		report.Checks = append(report.Checks, CheckItem{
			Name: "录像目录", Status: envFail, Required: true,
			Detail: fmt.Sprintf("%s 无法创建或不可写", recRoot),
			Fix: fmt.Sprintf("创建并授权：sudo mkdir -p %s && sudo chown -R <服务运行用户> %s，" +
				"或修改 config.yaml 的 storage.local.root_path 指向可写路径", recRoot, recRoot),
		})
	} else {
		report.Checks = append(report.Checks, CheckItem{
			Name: "录像目录", Status: envOK, Detail: recRoot,
		})
	}

	// 5. 日志目录可写（关键）
	logPath := s.cfg.Logging.Output
	if logPath != "" {
		logDir := filepath.Dir(logPath)
		if err := os.MkdirAll(logDir, 0755); err != nil || checkDirWritable(logDir) != nil {
			report.Checks = append(report.Checks, CheckItem{
				Name: "日志目录", Status: envFail, Required: true,
				Detail: fmt.Sprintf("%s 无法创建或不可写", logDir),
				Fix:    fmt.Sprintf("创建并授权：sudo mkdir -p %s && sudo chown -R <服务运行用户> %s", logDir, logDir),
			})
		} else {
			report.Checks = append(report.Checks, CheckItem{
				Name: "日志目录", Status: envOK, Detail: logDir,
			})
		}
	}

	// 6. 磁盘空间（录像所在分区）
	if free, err := diskFreeBytes(recRoot); err == nil {
		const gb = 1024 * 1024 * 1024
		switch {
		case free < 500*1024*1024:
			report.Checks = append(report.Checks, CheckItem{
				Name: "磁盘空间", Status: envFail, Required: true,
				Detail: fmt.Sprintf("录像分区剩余 %s，低于 500MB，录像将频繁失败", formatBytes(free)),
				Fix:    "清理磁盘或扩容：df -h 查看占用，删除旧录像/临时文件，或更换更大存储",
			})
		case free < 2*gb:
			report.Checks = append(report.Checks, CheckItem{
				Name: "磁盘空间", Status: envWarn,
				Detail: fmt.Sprintf("录像分区剩余 %s，空间偏少", formatBytes(free)),
				Fix:    "建议保留 2GB 以上可用空间；可调小录像保留天数（系统设置→存储设置）",
			})
		default:
			report.Checks = append(report.Checks, CheckItem{
				Name: "磁盘空间", Status: envOK, Detail: fmt.Sprintf("剩余 %s", formatBytes(free)),
			})
		}
	}

	// 7. WebDAV（可选：仅启用时检测）
	if wd := s.currentWebdavSettings(); wd.Enabled && wd.URL != "" {
		if err := checkWebdavReachable(wd.URL, wd.Username, wd.Password, wd.BasePath); err != nil {
			report.Checks = append(report.Checks, CheckItem{
				Name: "WebDAV 存储", Status: envWarn,
				Detail: err.Error(),
				Fix:  "检查 WebDAV 服务是否启动、URL/账号密码是否正确（系统设置→存储设置 可测试连接）；" +
					"若暂不使用可在存储设置中关闭 WebDAV",
			})
		} else {
			report.Checks = append(report.Checks, CheckItem{
				Name: "WebDAV 存储", Status: envOK, Detail: wd.URL,
			})
		}
	}

	// 8. MinIO/S3（可选：仅启用时检测）
	if mn := s.currentMinIOSettings(); mn.Enabled && mn.Endpoint != "" {
		if err := checkMinioReachable(mn); err != nil {
			report.Checks = append(report.Checks, CheckItem{
				Name: "MinIO 存储", Status: envWarn,
				Detail: err.Error(),
				Fix:  "检查 MinIO 服务是否启动、endpoint/密钥是否正确（系统设置→存储设置 可测试连接）；" +
					"若暂不使用可在存储设置中关闭 MinIO",
			})
		} else {
			report.Checks = append(report.Checks, CheckItem{
				Name: "MinIO 存储", Status: envOK, Detail: mn.Endpoint + " / " + mn.Bucket,
			})
		}
	}

	report.compute()
	return report
}

// checkWebdavReachable 5 秒超时的 WebDAV 连通性+凭据检测（PROPFIND）
func checkWebdavReachable(url, username, password, basePath string) error {
	c := webdav.NewClient(url, username, password)
	return c.CheckWithTimeout(basePath, 5*time.Second)
}

// checkMinioReachable 5 秒超时的 MinIO 连通性+凭据检测
func checkMinioReachable(mnCfg config.MinIOConfig) error {
	client, err := minio.NewClient(mnCfg.Endpoint, mnCfg.AccessKey, mnCfg.SecretKey, mnCfg.Bucket, mnCfg.UseSSL)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return client.EnsureBucket(ctx)
}

// ========== 首次设置（创建管理员） ==========

// needSetup 是否仍需首次设置：权威依据为「管理员用户数为 0」
func (s *Server) needSetup() bool {
	var count int64
	database.DB.Model(&models.User{}).Where("role = ?", models.UserRoleAdmin).Count(&count)
	return count == 0
}

// getSetupStatus GET /api/v1/setup/status（公开，无需登录）
func (s *Server) getSetupStatus(c *gin.Context) {
	c.JSON(200, gin.H{
		"need_setup": s.needSetup(),
		"env":        s.runEnvChecks(),
	})
}

type setupRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// postSetup POST /api/v1/setup（公开，仅首次设置未完成时可用）
func (s *Server) postSetup(c *gin.Context) {
	if !s.needSetup() {
		c.JSON(403, gin.H{"error": "已完成初始化设置，该接口不再可用"})
		return
	}
	var req setupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	username := strings.TrimSpace(req.Username)
	if len(username) < 3 || len(username) > 50 {
		c.JSON(400, gin.H{"error": "用户名长度需 3~50 个字符"})
		return
	}
	if len(req.Password) < 6 {
		c.JSON(400, gin.H{"error": "密码至少 6 位"})
		return
	}

	// 防止并发重复创建
	var count int64
	database.DB.Model(&models.User{}).Count(&count)
	if count > 0 {
		c.JSON(403, gin.H{"error": "已完成初始化设置，该接口不再可用"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(500, gin.H{"error": "密码加密失败"})
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
		c.JSON(500, gin.H{"error": "创建管理员失败: " + err.Error()})
		return
	}

	// 直接签发 JWT，前端无需再次登录
	token := makeToken(admin.Username, admin.Role, tokenTTL)
	c.JSON(200, gin.H{
		"token": token,
		"user": gin.H{
			"username": admin.Username,
			"role":     admin.Role,
		},
	})
}

// ========== 系统维护：环境检测（需登录） ==========

// getEnvCheck GET /api/v1/system/env（需登录，与首次设置页同一检测逻辑）
func (s *Server) getEnvCheck(c *gin.Context) {
	c.JSON(200, s.runEnvChecks())
}

// ========== 工具函数 ==========

// runVersionCmd 执行 <cmd> -version 并返回首行版本信息
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
	return "", fmt.Errorf("无法解析版本输出")
}

// checkDirWritable 在目录中写一个临时文件验证可写
func checkDirWritable(dir string) error {
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("目录不存在: %w", err)
	}
	f, err := os.CreateTemp(dir, ".envcheck_*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

// formatBytes 字节数转可读单位
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
