// Package settings 基于 SystemConfig（key-value 表）的系统级设置存储。
package settings

import (
	"encoding/json"
	"errors"

	"github.com/yourorg/surveillance-system/internal/database"
	"github.com/yourorg/surveillance-system/internal/models"
)

const (
	KeyHWDecode       = "hw_decode"
	KeyHWEncode       = "hw_encode"
	KeyCameraDefaults = "camera_defaults"
)

// CameraDefaults 录像默认值（新摄像头预填）
type CameraDefaults struct {
	RecordEnabled bool   `json:"record_enabled"`
	RecordType    string `json:"record_type"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	FPS           int    `json:"fps"`
	Codec         string `json:"codec"`
	Bitrate       int    `json:"bitrate"`
}

// DefaultCameraDefaults 内置默认值（与 Camera 模型 gorm 默认值一致）
func DefaultCameraDefaults() CameraDefaults {
	return CameraDefaults{
		RecordEnabled: true,
		RecordType:    "continuous",
		Width:         1920,
		Height:        1080,
		FPS:           25,
		Codec:         "h264",
		Bitrate:       4096,
	}
}

// Get 读取字符串值
func Get(key string) (string, bool) {
	db := database.GetDB()
	if db == nil {
		return "", false
	}
	var sc models.SystemConfig
	if err := db.Where("`key` = ?", key).First(&sc).Error; err != nil {
		return "", false
	}
	return sc.Value, true
}

// Set 写入（upsert）
func Set(key, value, desc string) error {
	db := database.GetDB()
	if db == nil {
		return errors.New("database not initialized")
	}
	var sc models.SystemConfig
	err := db.Where("`key` = ?", key).First(&sc).Error
	if err != nil {
		return db.Create(&models.SystemConfig{Key: key, Value: value, Desc: desc}).Error
	}
	return db.Model(&sc).Updates(map[string]any{"value": value, "desc": desc}).Error
}

// GetBool 读取布尔值
func GetBool(key string, def bool) bool {
	v, ok := Get(key)
	if !ok {
		return def
	}
	return v == "true"
}

// GetCameraDefaults 读取录像默认值；未保存过返回内置默认
func GetCameraDefaults() CameraDefaults {
	v, ok := Get(KeyCameraDefaults)
	if !ok {
		return DefaultCameraDefaults()
	}
	cd := DefaultCameraDefaults()
	if err := json.Unmarshal([]byte(v), &cd); err != nil {
		return DefaultCameraDefaults()
	}
	return cd
}

// SetCameraDefaults 保存录像默认值
func SetCameraDefaults(cd CameraDefaults) error {
	b, err := json.Marshal(cd)
	if err != nil {
		return err
	}
	return Set(KeyCameraDefaults, string(b), "录像默认值（新摄像头预填）")
}
