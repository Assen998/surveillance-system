package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server" yaml:"server"`
	Database DatabaseConfig `mapstructure:"database" yaml:"database"`
	Redis    RedisConfig    `mapstructure:"redis" yaml:"redis"`
	Storage  StorageConfig  `mapstructure:"storage" yaml:"storage"`
	Camera   CameraConfig   `mapstructure:"camera" yaml:"camera"`
	Alert    AlertConfig    `mapstructure:"alert" yaml:"alert"`
	GB28181  GB28181Config  `mapstructure:"gb28181" yaml:"gb28181"`
	Logging  LoggingConfig  `mapstructure:"logging" yaml:"logging"`
}

type ServerConfig struct {
	Host     string `mapstructure:"host" yaml:"host"`
	HTTPPort int    `mapstructure:"http_port" yaml:"http_port"`
	WSPort   int    `mapstructure:"ws_port" yaml:"ws_port"`
	GRPCPort int    `mapstructure:"grpc_port" yaml:"grpc_port"`
	Mode     string `mapstructure:"mode" yaml:"mode"`
}

type DatabaseConfig struct {
	Type     string         `mapstructure:"type" yaml:"type"`
	SQLite   SQLiteConfig   `mapstructure:"sqlite" yaml:"sqlite"`
	Postgres PostgresConfig `mapstructure:"postgres" yaml:"postgres"`
}

type SQLiteConfig struct {
	Path string `mapstructure:"path" yaml:"path"`
}

type PostgresConfig struct {
	Host     string `mapstructure:"host" yaml:"host"`
	Port     int    `mapstructure:"port" yaml:"port"`
	User     string `mapstructure:"user" yaml:"user"`
	Password string `mapstructure:"password" yaml:"password"`
	DBName   string `mapstructure:"dbname" yaml:"dbname"`
	SSLMode  string `mapstructure:"sslmode" yaml:"sslmode"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host" yaml:"host"`
	Port     int    `mapstructure:"port" yaml:"port"`
	Password string `mapstructure:"password" yaml:"password"`
	DB       int    `mapstructure:"db" yaml:"db"`
}

type StorageConfig struct {
	Local  LocalStorageConfig `mapstructure:"local" yaml:"local"`
	MinIO  MinIOConfig        `mapstructure:"minio" yaml:"minio"`
	Webdav WebdavConfig       `mapstructure:"webdav" yaml:"webdav"`
}

type LocalStorageConfig struct {
	Enabled         bool   `mapstructure:"enabled" yaml:"enabled"`
	RootPath        string `mapstructure:"root_path" yaml:"root_path"`
	SegmentDuration int    `mapstructure:"segment_duration" yaml:"segment_duration"`
	MaxDays         int    `mapstructure:"max_days" yaml:"max_days"`
	// MaxStorageGB 存储占用上限（GB），与保留天数并行：谁先达到先清理；0 = 不限制
	MaxStorageGB    float64 `mapstructure:"max_storage_gb" yaml:"max_storage_gb"`
	CleanupInterval int    `mapstructure:"cleanup_interval" yaml:"cleanup_interval"`
}

type WebdavConfig struct {
	Enabled  bool   `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	URL      string `mapstructure:"url" yaml:"url" json:"url"`
	Username string `mapstructure:"username" yaml:"username" json:"username"`
	Password string `mapstructure:"password" yaml:"password" json:"password"`
	BasePath string `mapstructure:"base_path" yaml:"base_path" json:"base_path"`
	// MaxDays 远程保留天数（独立于本地保留）；0 = 不按时间自动删除
	MaxDays int `mapstructure:"max_days" yaml:"max_days" json:"max_days"`
	// MaxStorageGB 远程占用上限（GB）；0 = 不限制；超限时从最旧录像开始删除
	MaxStorageGB float64 `mapstructure:"max_storage_gb" yaml:"max_storage_gb" json:"max_storage_gb"`
	// Only WebDAV 独占模式：true 时录像上传成功后立即删除本地副本，
	// 本地仅作为上传前的临时缓冲（上传失败则保留本地文件防丢失）。
	// 旧录像回放自动走 WebDAV 流式播放。
	Only bool `mapstructure:"only" yaml:"only" json:"only"`
}

type MinIOConfig struct {
	Enabled    bool   `mapstructure:"enabled" yaml:"enabled"`
	Endpoint   string `mapstructure:"endpoint" yaml:"endpoint"`
	AccessKey  string `mapstructure:"access_key" yaml:"access_key"`
	SecretKey  string `mapstructure:"secret_key" yaml:"secret_key"`
	Bucket     string `mapstructure:"bucket" yaml:"bucket"`
	UseSSL     bool   `mapstructure:"use_ssl" yaml:"use_ssl"`
}

type CameraConfig struct {
	DiscoveryTimeout  int `mapstructure:"discovery_timeout" yaml:"discovery_timeout"`
	StreamTimeout     int `mapstructure:"stream_timeout" yaml:"stream_timeout"`
	ReconnectInterval int `mapstructure:"reconnect_interval" yaml:"reconnect_interval"`
	MaxReconnect      int `mapstructure:"max_reconnect" yaml:"max_reconnect"`
	SnapshotEnabled   bool `mapstructure:"snapshot_enabled" yaml:"snapshot_enabled"`
	SnapshotInterval  int  `mapstructure:"snapshot_interval" yaml:"snapshot_interval"`
	// OnvifEvent ONVIF 事件报警（摄像头主动上报的移动侦测/越线/入侵等）
	OnvifEvent OnvifEventConfig `mapstructure:"onvif_event" yaml:"onvif_event"`
	// MotionRecord 移动侦测触发录像（事件型录像：平时不录，检测到移动时才录）
	MotionRecord MotionRecordConfig `mapstructure:"motion_record" yaml:"motion_record"`
}

// MotionRecordConfig 移动侦测触发录像配置（RecordType=motion 的摄像头生效）
type MotionRecordConfig struct {
	// Duration 每次移动触发后录制的时长（秒）
	Duration int `mapstructure:"duration" yaml:"duration"`
	// PreRecord 录像开始前回溯的预录时长（秒）；需要常驻环形缓冲，预留字段，暂未实现
	PreRecord int `mapstructure:"pre_record" yaml:"pre_record"`
	// Cooldown 相邻两次触发的冷却时间（秒）：冷却期内重复报警不会重复触发录像
	Cooldown int `mapstructure:"cooldown" yaml:"cooldown"`
}

type OnvifEventConfig struct {
	// Enabled 是否启用 ONVIF 事件订阅（摄像头主动上报报警）
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`
	// PollInterval 拉取事件间隔（秒）；建议 5~30，太大报警延迟高，太小增加负载
	PollInterval int `mapstructure:"poll_interval" yaml:"poll_interval"`
	// SubscriptionTimeout 订阅超时（分钟）；到期自动重新订阅
	SubscriptionTimeout int `mapstructure:"subscription_timeout" yaml:"subscription_timeout"`
}

type AlertConfig struct {
	Enabled  bool          `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	Channels AlertChannels `mapstructure:"channels" yaml:"channels" json:"channels"`
}

type AlertChannels struct {
	Webhook WebhookAlertConfig `mapstructure:"webhook" yaml:"webhook" json:"webhook"`
	Email   EmailAlertConfig   `mapstructure:"email" yaml:"email" json:"email"`
	SMS     SMSAlertConfig     `mapstructure:"sms" yaml:"sms" json:"sms"`
}

type WebhookAlertConfig struct {
	Enabled bool   `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	URL     string `mapstructure:"url" yaml:"url" json:"url"`
	// Type 推送格式类型：generic（默认，通用 JSON）| gotify（Gotify /message 接口，
	// URL 填形如 https://gotify.example.com/message?token=xxx）
	Type string `mapstructure:"type" yaml:"type" json:"type"`
}

type EmailAlertConfig struct {
	Enabled    bool     `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	SMTPHost   string   `mapstructure:"smtp_host" yaml:"smtp_host" json:"smtp_host"`
	SMTPPort   int      `mapstructure:"smtp_port" yaml:"smtp_port" json:"smtp_port"`
	Username   string   `mapstructure:"username" yaml:"username" json:"username"`
	Password   string   `mapstructure:"password" yaml:"password" json:"password"`
	From       string   `mapstructure:"from" yaml:"from" json:"from"`
	To         []string `mapstructure:"to" yaml:"to" json:"to"`
}

type SMSAlertConfig struct {
	Enabled       bool   `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	Provider      string `mapstructure:"provider" yaml:"provider" json:"provider"`
	AccessKey     string `mapstructure:"access_key" yaml:"access_key" json:"access_key"`
	SecretKey     string `mapstructure:"secret_key" yaml:"secret_key" json:"secret_key"`
	SignName      string `mapstructure:"sign_name" yaml:"sign_name" json:"sign_name"`
	TemplateCode  string `mapstructure:"template_code" yaml:"template_code" json:"template_code"`
}

type GB28181Config struct {
	Enabled     bool   `mapstructure:"enabled" yaml:"enabled"`
	SIPID       string `mapstructure:"sip_id" yaml:"sip_id"`
	SIPDomain   string `mapstructure:"sip_domain" yaml:"sip_domain"`
	SIPPassword string `mapstructure:"sip_password" yaml:"sip_password"`
	SIPPort     int    `mapstructure:"sip_port" yaml:"sip_port"`
	MediaPort   int    `mapstructure:"media_port" yaml:"media_port"`
	Expires     int    `mapstructure:"expires" yaml:"expires"`
}

type LoggingConfig struct {
	Level      string `mapstructure:"level" yaml:"level"`
	Format     string `mapstructure:"format" yaml:"format"`
	Output     string `mapstructure:"output" yaml:"output"`
	MaxSize    int    `mapstructure:"max_size" yaml:"max_size"`
	MaxBackups int    `mapstructure:"max_backups" yaml:"max_backups"`
	MaxAge     int    `mapstructure:"max_age" yaml:"max_age"`
	Compress   bool   `mapstructure:"compress" yaml:"compress"`
}

var GlobalConfig *Config

func Load(configPath string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// 环境变量覆盖
	v.AutomaticEnv()
	v.SetEnvPrefix("SURVEILLANCE")

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// 规范化路径
	cfg.Storage.Local.RootPath = expandPath(cfg.Storage.Local.RootPath)
	cfg.Database.SQLite.Path = expandPath(cfg.Database.SQLite.Path)
	cfg.Logging.Output = expandPath(cfg.Logging.Output)

	GlobalConfig = &cfg
	return &cfg, nil
}

func expandPath(path string) string {
	if path == "" {
		return path
	}
	if path[0] == '~' {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return path
}

// GetConfig 获取全局配置（线程安全）
func GetConfig() *Config {
	return GlobalConfig
}

// Save 保存配置到文件
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}