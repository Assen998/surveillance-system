package api

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	localeZH = "zh"
	localeEN = "en"
)

func LocaleFromContext(c *gin.Context) string {
	al := c.GetHeader("Accept-Language")
	if al == "" || strings.Contains(al, "zh") {
		return localeZH
	}
	return localeEN
}

var envMsgs = map[string]map[string]string{
	localeZH: {
		"env.ffmpeg.fail":      "未找到 ffmpeg，拉流/录像/预览均不可用",
		"env.ffmpeg.fix":       "安装：Debian/Armbian: sudo apt update && sudo apt install -y ffmpeg；RHEL/Fedora: sudo dnf install -y ffmpeg；其他系统可下载对应平台的静态编译版并放入 PATH",
		"env.ffprobe.fail":     "未找到 ffprobe，摄像头连接测试不可用",
		"env.ffprobe.fix":      "与 ffmpeg 同一软件包：sudo apt install -y ffmpeg（或 dnf install -y ffmpeg）",
		"env.hw.name":          "硬件编解码",
		"env.hw.encode":        "编码",
		"env.hw.decode":        "解码",
		"env.hw.found":         "可用硬件编解码：%s（录像走 copy 透传不受影响；预览转码是否启用硬件编解码由「录像默认配置」页开关控制）",
		"env.hw.also":          "另已编译支持：%s",
		"env.hw.compiled":      "ffmpeg 已编译以下硬件加速支持，但未检测到可用设备，当前使用 CPU 软件编解码：%s",
		"env.hw.none":          "未检测到硬件编解码支持，当前使用 CPU 软件编解码（录像走 copy 透传，不受影响）",
		"env.hw.nvidiaMissing": "检测到 NVIDIA GPU，但当前 ffmpeg 不含 NVENC/CUVID 硬件编解码支持",
		"env.hw.nvidiaFix":     "请更换带 CUDA 支持的 ffmpeg（社区静态编译版，如 BtbN/FFmpeg-Builds 的 cuda 版本），然后重启服务",
		"env.hw.verifyFailed":  "检测到硬件 %s 单元，但实际解码/编码自检未通过（当前 ffmpeg/驱动组合不兼容）",
		"env.hw.verifyFix":     "可升级 ffmpeg 或系统内核驱动后重启服务重试；当前预览使用 CPU 软件编解码，功能不受影响",
		"env.dbdir.name":       "数据库目录",
		"env.dbdir.fail":       "%s 不可写: %v",
		"env.dbdir.fix":        "检查目录权限：sudo chmod -R u+w %s，并确认服务运行用户拥有写权限",
		"env.recdir.name":      "录像目录",
		"env.recdir.fail":      "%s 无法创建或不可写",
		"env.recdir.fix":       "创建并授权：sudo mkdir -p %s && sudo chown -R <服务运行用户> %s，或修改 config.yaml 的 storage.local.root_path 指向可写路径",
		"env.logdir.name":      "日志目录",
		"env.logdir.fail":      "%s 无法创建或不可写",
		"env.logdir.fix":       "创建并授权：sudo mkdir -p %s && sudo chown -R <服务运行用户> %s",
		"env.disk.name":        "磁盘空间",
		"env.disk.fail":        "录像分区剩余 %s，低于 500MB，录像将频繁失败",
		"env.disk.failfix":     "清理磁盘或扩容：df -h 查看占用，删除旧录像/临时文件，或更换更大存储",
		"env.disk.warn":        "录像分区剩余 %s，空间偏少",
		"env.disk.warnfix":     "建议保留 2GB 以上可用空间；可调小录像保留天数（系统设置→存储设置）",
		"env.disk.ok":          "剩余 %s",
		"env.webdav.name":      "WebDAV 存储",
		"env.webdav.fix":       "检查 WebDAV 服务是否启动、URL/账号密码是否正确（系统设置→存储设置 可测试连接）；若暂不使用可在存储设置中关闭 WebDAV",
		"env.minio.name":       "MinIO 存储",
		"env.minio.fix":        "检查 MinIO 服务是否启动、endpoint/密钥是否正确（系统设置→存储设置 可测试连接）；若暂不使用可在存储设置中关闭 MinIO",
		"setup.done403":        "已完成初始化设置，该接口不再可用",
		"setup.param":          "参数错误: %v",
		"setup.username.len":   "用户名长度需 3~50 个字符",
		"setup.password.min":   "密码至少 6 位",
		"setup.hash":           "密码加密失败",
		"setup.create":         "创建管理员失败: %v",
		"alert.notfound":       "报警记录不存在",
		"alert.acked":          "已确认",
		"alert.resolved":       "已解决",
		"alert.cleared":        "已清空报警记录",
		"alert.deleted":        "删除成功",
		"alert.test":           "这是一条测试报警消息",
	},
	localeEN: {
		"env.ffmpeg.fail":      "ffmpeg not found — streaming, recording and preview are unavailable",
		"env.ffmpeg.fix":       "Install: Debian/Armbian: sudo apt update && sudo apt install -y ffmpeg; RHEL/Fedora: sudo dnf install -y ffmpeg; on other systems download a static build for your platform and add it to PATH",
		"env.ffprobe.fail":     "ffprobe not found — camera connection test is unavailable",
		"env.ffprobe.fix":      "Same package as ffmpeg: sudo apt install -y ffmpeg (or dnf install -y ffmpeg)",
		"env.hw.name":          "HW codecs",
		"env.hw.encode":        "encode",
		"env.hw.decode":        "decode",
		"env.hw.found":         "Available HW codecs: %s (recording uses copy passthrough; preview HW codec is controlled by the switches on the Recording Defaults page)",
		"env.hw.also":          "also compiled: %s",
		"env.hw.compiled":      "ffmpeg is compiled with these HW accelerations but no usable device was found, using CPU software codecs: %s",
		"env.hw.none":          "No hardware codec support detected, using CPU software codecs (recording uses copy passthrough)",
		"env.hw.nvidiaMissing": "NVIDIA GPU detected, but the current ffmpeg lacks NVENC/CUVID hardware codec support",
		"env.hw.nvidiaFix":     "Install an ffmpeg built with CUDA support (e.g. community static builds like the BtbN/FFmpeg-Builds cuda variant), then restart the service",
		"env.hw.verifyFailed":  "HW %s unit detected, but the real decode/encode self-test failed (incompatible with the current ffmpeg/driver)",
		"env.hw.verifyFix":     "Upgrade ffmpeg or the kernel driver and restart to retry; preview currently uses CPU software codecs, functionality unaffected",
		"env.dbdir.name":       "Database dir",
		"env.dbdir.fail":       "%s is not writable: %v",
		"env.dbdir.fix":        "Check directory permissions: sudo chmod -R u+w %s, and make sure the service user has write access",
		"env.recdir.name":      "Recording dir",
		"env.recdir.fail":      "%s cannot be created or is not writable",
		"env.recdir.fix":       "Create and authorize: sudo mkdir -p %s && sudo chown -R <service user> %s, or point storage.local.root_path in config.yaml to a writable path",
		"env.logdir.name":      "Log dir",
		"env.logdir.fail":      "%s cannot be created or is not writable",
		"env.logdir.fix":       "Create and authorize: sudo mkdir -p %s && sudo chown -R <service user> %s",
		"env.disk.name":        "Disk space",
		"env.disk.fail":        "Only %s left on the recording partition, below 500MB — recording will frequently fail",
		"env.disk.failfix":     "Free up disk space or expand storage: check usage with df -h, delete old recordings/temp files, or use larger storage",
		"env.disk.warn":        "Only %s left on the recording partition, low free space",
		"env.disk.warnfix":     "Keep at least 2GB free; you can reduce the retention days (Settings → Storage)",
		"env.disk.ok":          "%s free",
		"env.webdav.name":      "WebDAV storage",
		"env.webdav.fix":       "Check whether the WebDAV service is running and the URL/credentials are correct (test connection under Settings → Storage); disable WebDAV in storage settings if not used",
		"env.minio.name":       "MinIO storage",
		"env.minio.fix":        "Check whether the MinIO service is running and the endpoint/keys are correct (test connection under Settings → Storage); disable MinIO in storage settings if not used",
		"setup.done403":        "Initialization already completed, this endpoint is no longer available",
		"setup.param":          "Invalid parameters: %v",
		"setup.username.len":   "Username must be 3-50 characters",
		"setup.password.min":   "Password must be at least 6 characters",
		"setup.hash":           "Failed to hash password",
		"setup.create":         "Failed to create admin user: %v",
		"alert.notfound":       "Alert record not found",
		"alert.acked":          "Acknowledged",
		"alert.resolved":       "Resolved",
		"alert.cleared":        "Alert records cleared",
		"alert.deleted":        "Deleted",
		"alert.test":           "This is a test alert message",
	},
}

func TEnv(l, key string, args ...any) string {
	m, ok := envMsgs[l]
	if !ok {
		m = envMsgs[localeZH]
	}
	s, found := m[key]
	if !found {
		s, _ = envMsgs[localeZH][key]
	}
	if s == "" {
		return key
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

var alertMsgEn = map[string]string{
	"摄像头上报：检测到移动":     "Camera event: motion detected",
	"摄像头上报：越线报警":      "Camera event: line crossing detected",
	"摄像头上报：区域入侵报警":    "Camera event: zone intrusion detected",
	"摄像头上报：设备破坏/遮挡报警": "Camera event: camera tamper/occlusion detected",
	"摄像头上报：设备断开报警":    "Camera event: device disconnected",
	"摄像头上报报警事件":       "Camera reported an alert event",
	"这是一条测试报警消息":      "This is a test alert message",
}

const alertEventZhPrefix = "摄像头上报事件: "
const alertEventEnPrefix = "Camera event: "

func LocalizeAlert(l, msg string) string {
	if l == localeZH || msg == "" {
		return msg
	}
	if v, ok := alertMsgEn[msg]; ok {
		return v
	}
	if strings.HasPrefix(msg, alertEventZhPrefix) {
		return alertEventEnPrefix + strings.TrimPrefix(msg, alertEventZhPrefix)
	}
	return msg
}
