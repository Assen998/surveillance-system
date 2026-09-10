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
	Update   UpdateConfig   `mapstructure:"update" yaml:"update"`
}


type UpdateConfig struct {


	Proxy string `mapstructure:"proxy" yaml:"proxy"`

	GitHubRepo string `mapstructure:"github_repo" yaml:"github_repo"`

	BaseURL string `mapstructure:"base_url" yaml:"base_url"`
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

	MaxStorageGB    float64 `mapstructure:"max_storage_gb" yaml:"max_storage_gb"`
	CleanupInterval int    `mapstructure:"cleanup_interval" yaml:"cleanup_interval"`
}

type WebdavConfig struct {
	Enabled  bool   `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	URL      string `mapstructure:"url" yaml:"url" json:"url"`
	Username string `mapstructure:"username" yaml:"username" json:"username"`
	Password string `mapstructure:"password" yaml:"password" json:"password"`
	BasePath string `mapstructure:"base_path" yaml:"base_path" json:"base_path"`

	MaxDays int `mapstructure:"max_days" yaml:"max_days" json:"max_days"`

	MaxStorageGB float64 `mapstructure:"max_storage_gb" yaml:"max_storage_gb" json:"max_storage_gb"`


	Only bool `mapstructure:"only" yaml:"only" json:"only"`
}

type MinIOConfig struct {
	Enabled    bool   `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	Endpoint   string `mapstructure:"endpoint" yaml:"endpoint" json:"endpoint"`
	AccessKey  string `mapstructure:"access_key" yaml:"access_key" json:"access_key"`
	SecretKey  string `mapstructure:"secret_key" yaml:"secret_key" json:"secret_key"`
	Bucket     string `mapstructure:"bucket" yaml:"bucket" json:"bucket"`
	UseSSL     bool   `mapstructure:"use_ssl" yaml:"use_ssl" json:"use_ssl"`

	BasePath string `mapstructure:"base_path" yaml:"base_path" json:"base_path"`

	MaxDays int `mapstructure:"max_days" yaml:"max_days" json:"max_days"`

	MaxStorageGB float64 `mapstructure:"max_storage_gb" yaml:"max_storage_gb" json:"max_storage_gb"`


	Only bool `mapstructure:"only" yaml:"only" json:"only"`
}

type CameraConfig struct {
	DiscoveryTimeout  int `mapstructure:"discovery_timeout" yaml:"discovery_timeout"`
	StreamTimeout     int `mapstructure:"stream_timeout" yaml:"stream_timeout"`
	ReconnectInterval int `mapstructure:"reconnect_interval" yaml:"reconnect_interval"`
	MaxReconnect      int `mapstructure:"max_reconnect" yaml:"max_reconnect"`
	SnapshotEnabled   bool `mapstructure:"snapshot_enabled" yaml:"snapshot_enabled"`
	SnapshotInterval  int  `mapstructure:"snapshot_interval" yaml:"snapshot_interval"`

	OnvifEvent OnvifEventConfig `mapstructure:"onvif_event" yaml:"onvif_event"`

	MotionRecord MotionRecordConfig `mapstructure:"motion_record" yaml:"motion_record"`

	PreviewStream string `mapstructure:"preview_stream" yaml:"preview_stream"`
}


type MotionRecordConfig struct {

	Duration int `mapstructure:"duration" yaml:"duration"`

	PreRecord int `mapstructure:"pre_record" yaml:"pre_record"`

	Cooldown int `mapstructure:"cooldown" yaml:"cooldown"`
}

type OnvifEventConfig struct {

	Enabled bool `mapstructure:"enabled" yaml:"enabled"`

	PollInterval int `mapstructure:"poll_interval" yaml:"poll_interval"`

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


	v.AutomaticEnv()
	v.SetEnvPrefix("SURVEILLANCE")

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}


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


func GetConfig() *Config {
	return GlobalConfig
}


func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
