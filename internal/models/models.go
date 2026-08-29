package models

import (
	"time"

	"gorm.io/gorm"
)

// Camera 摄像头模型
type Camera struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	Name        string `gorm:"size:100;not null;index" json:"name" validate:"required,max=100"`
	Description string `gorm:"size:500" json:"description"`

	// 连接信息
	Protocol    string `gorm:"size:20;not null;default:'rtsp'" json:"protocol" validate:"oneof=rtsp onvif gb28181"` // rtsp, onvif, gb28181
	IP          string `gorm:"size:45;not null" json:"ip" validate:"required,ip"`
	Port        int    `gorm:"default:554" json:"port" validate:"min=1,max=65535"`
	Username    string `gorm:"size:50" json:"username"`
	Password    string `gorm:"size:100" json:"-"` // 不序列化返回
	Path        string `gorm:"size:200" json:"path"` // RTSP 路径，如 /stream1

	// ONVIF/GB28181 特有
	OnvifAddress        string  `gorm:"size:200" json:"onvif_address"`          // ONVIF 设备地址
	OnvifProfileToken   string  `gorm:"size:100" json:"onvif_profile_token"`    // 用户指定的 Profile Token
	DiscoveredStreamUri string  `gorm:"size:500" json:"discovered_stream_uri"`  // 缓存的发现流地址
	StreamUriUpdatedAt  *time.Time `json:"stream_uri_updated_at"`                // 缓存更新时间
	DeviceID            *string `gorm:"size:50;uniqueIndex" json:"device_id"`   // GB28181 设备 ID，指针类型允许 NULL

	// 状态
	Status      string `gorm:"size:20;default:'offline'" json:"status"` // online, offline, error
	LastOnline  *time.Time `json:"last_online"`
	ErrorMsg    string `gorm:"size:500" json:"error_msg"`

	// 录像配置
	RecordEnabled   bool `gorm:"default:true" json:"record_enabled"`
	RecordSchedule  string `gorm:"size:100;default:'0-23'" json:"record_schedule"` // 录像时间段，如 0-23, 9-18
	RecordType      string `gorm:"size:20;default:'continuous'" json:"record_type"` // continuous, motion, schedule

	// 画面配置
	Width       int `gorm:"default:1920" json:"width"`
	Height      int `gorm:"default:1080" json:"height"`
	FPS         int `gorm:"default:25" json:"fps"`
	Bitrate     int `gorm:"default:4096" json:"bitrate"` // kbps
	Codec       string `gorm:"size:20;default:'h264'" json:"codec"` // h264, h265

	// PTZ
	PTZEnabled bool `gorm:"default:false" json:"ptz_enabled"`

	// 关联
	Recordings []Recording `gorm:"foreignKey:CameraID" json:"-"`
	Alerts     []Alert     `gorm:"foreignKey:CameraID" json:"-"`
	Snapshots  []Snapshot  `gorm:"foreignKey:CameraID" json:"-"`
}

// Recording 录像片段模型
type Recording struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	CameraID  uint   `gorm:"not null;index" json:"camera_id"`
	Camera    Camera `gorm:"foreignKey:CameraID" json:"-"`

	StartTime time.Time `gorm:"not null;index" json:"start_time"`
	EndTime   time.Time `gorm:"not null;index" json:"end_time"`
	Duration  int       `json:"duration"` // 秒

	FilePath  string `gorm:"size:500;not null" json:"file_path"`
	FileSize  int64  `json:"file_size"` // 字节
	SegmentIndex int `json:"segment_index"` // 分段索引

	RecordType string `gorm:"size:20" json:"record_type"` // continuous, motion, manual
	Status     string `gorm:"size:20;default:'completed'" json:"status"` // recording, completed, error

	// 索引文件（用于快速定位）
	IndexPath string `gorm:"size:500" json:"index_path"`

	// 存储位置
	StorageType string `gorm:"size:20;default:'local'" json:"storage_type"` // local, minio
	StoragePath string `gorm:"size:500" json:"storage_path"`
}

// Snapshot 抓拍图片模型
type Snapshot struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	CameraID uint   `gorm:"not null;index" json:"camera_id"`
	Camera   Camera `gorm:"foreignKey:CameraID" json:"-"`

	Timestamp time.Time `gorm:"not null;index" json:"timestamp"`
	FilePath  string    `gorm:"size:500;not null" json:"file_path"`
	FileSize  int64     `json:"file_size"`
	Type      string    `gorm:"size:20;default:'schedule'" json:"type"` // schedule, motion, alert, manual
	StorageType string  `gorm:"size:20;default:'local'" json:"storage_type"`
}

// Alert 报警记录模型
type Alert struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	CameraID uint   `gorm:"not null;index" json:"camera_id"`
	Camera   Camera `gorm:"foreignKey:CameraID" json:"-"`

	Type       string `gorm:"size:30;not null;index" json:"type"` // motion, intrusion, line_cross, object_detect, offline
	Level      string `gorm:"size:10;default:'medium'" json:"level"` // low, medium, high, critical
	Message    string `gorm:"size:500" json:"message"`
	Details    string `gorm:"type:text" json:"details"` // JSON 详情

	SnapshotPath string `gorm:"size:500" json:"snapshot_path"` // 报警时刻抓拍
	VideoPath    string `gorm:"size:500" json:"video_path"`    // 报警关联视频片段

	Status     string `gorm:"size:20;default:'new'" json:"status"` // new, acknowledged, resolved
	AckedBy    uint   `json:"acked_by"`
	AckedAt    *time.Time `json:"acked_at"`
	ResolvedBy uint   `json:"resolved_by"`
	ResolvedAt *time.Time `json:"resolved_at"`

	Notified   bool `gorm:"default:false" json:"notified"`
	NotifyAt   *time.Time `json:"notify_at"`
}

// User 用户模型
type User struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	Username string `gorm:"size:50;uniqueIndex;not null" json:"username" validate:"required,min=3,max=50"`
	Password string `gorm:"size:100;not null" json:"-"` // bcrypt hash
	Email    string `gorm:"size:100;uniqueIndex" json:"email" validate:"email"`
	Phone    string `gorm:"size:20" json:"phone"`

	Role     string `gorm:"size:20;default:'viewer'" json:"role"` // admin, operator, viewer
	Status   string `gorm:"size:20;default:'active'" json:"status"` // active, disabled, locked
	LastLogin *time.Time `json:"last_login"`

	// 权限
	CameraPermissions []CameraPermission `gorm:"foreignKey:UserID" json:"camera_permissions,omitempty"`
}

// CameraPermission 用户-摄像头权限
type CameraPermission struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`

	UserID   uint   `gorm:"not null;index" json:"user_id"`
	User     User   `gorm:"foreignKey:UserID" json:"-"`

	CameraID uint   `gorm:"not null;index" json:"camera_id"`
	Camera   Camera `gorm:"foreignKey:CameraID" json:"-"`

	Permission string `gorm:"size:20;not null" json:"permission"` // view, control, config, admin
}

// SystemConfig 系统配置模型（键值对）
type SystemConfig struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`

	Key   string `gorm:"size:100;uniqueIndex;not null" json:"key"`
	Value string `gorm:"type:text" json:"value"`
	Desc  string `gorm:"size:200" json:"desc"`
}

// TableName 自定义表名
func (Camera) TableName() string       { return "cameras" }
func (Recording) TableName() string    { return "recordings" }
func (Snapshot) TableName() string     { return "snapshots" }
func (Alert) TableName() string        { return "alerts" }
func (User) TableName() string         { return "users" }
func (CameraPermission) TableName() string { return "camera_permissions" }
func (SystemConfig) TableName() string { return "system_configs" }

// CameraStatus 摄像头状态常量
const (
	CameraStatusOnline  = "online"
	CameraStatusOffline = "offline"
	CameraStatusError   = "error"
)

// RecordType 录像类型常量
const (
	RecordTypeContinuous = "continuous"
	RecordTypeMotion     = "motion"
	RecordTypeSchedule   = "schedule"
	RecordTypeManual     = "manual"
)

// AlertType 报警类型常量
const (
	AlertTypeMotion       = "motion"
	AlertTypeIntrusion    = "intrusion"
	AlertTypeLineCross    = "line_cross"
	AlertTypeObjectDetect = "object_detect"
	AlertTypeOffline      = "offline"
	AlertTypeStorageFull  = "storage_full"
	AlertTypeError        = "error"
)

// AlertLevel 报警等级常量
const (
	AlertLevelLow      = "low"
	AlertLevelMedium   = "medium"
	AlertLevelHigh     = "high"
	AlertLevelCritical = "critical"
)

// AlertStatus 报警状态常量
const (
	AlertStatusNew         = "new"
	AlertStatusAcknowledged = "acknowledged"
	AlertStatusResolved    = "resolved"
)

// UserRole 用户角色常量
const (
	UserRoleAdmin    = "admin"
	UserRoleOperator = "operator"
	UserRoleViewer   = "viewer"
)

// Permission 权限常量
const (
	PermView    = "view"
	PermControl = "control"
	PermConfig  = "config"
	PermAdmin   = "admin"
)