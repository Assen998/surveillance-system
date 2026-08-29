package storage

import (
	"sync"

	"github.com/yourorg/surveillance-system/internal/config"
)

// RuntimeStorage 运行时存储设置。
// 设置页保存后立即生效（分段时长对新连接/重连的摄像头生效，
// 清理与上传使用最新值），无需重启服务。
type RuntimeStorage struct {
	mu     sync.RWMutex
	local  config.LocalStorageConfig
	webdav config.WebdavConfig
}

func NewRuntimeStorage(cfg *config.Config) *RuntimeStorage {
	return &RuntimeStorage{
		local:  cfg.Storage.Local,
		webdav: cfg.Storage.Webdav,
	}
}

func (r *RuntimeStorage) GetLocal() config.LocalStorageConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.local
}

func (r *RuntimeStorage) GetWebdav() config.WebdavConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.webdav
}

func (r *RuntimeStorage) SetLocal(l config.LocalStorageConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.local = l
}

func (r *RuntimeStorage) SetWebdav(w config.WebdavConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.webdav = w
}
