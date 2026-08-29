package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yourorg/surveillance-system/internal/config"
	"github.com/yourorg/surveillance-system/internal/database"
	"github.com/yourorg/surveillance-system/internal/models"
	"github.com/yourorg/surveillance-system/pkg/webdav"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Manager struct {
	cfg      *config.Config
	db       *gorm.DB
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.Mutex
	stats    StorageStats
	runtime  *RuntimeStorage
	cleanupMu sync.Mutex // 防止定时清理与手动清理并发执行
}

type StorageStats struct {
	TotalSpace     int64 `json:"total_space"`
	UsedSpace      int64 `json:"used_space"`
	FreeSpace      int64 `json:"free_space"`
	RecordingCount int64 `json:"recording_count"`
	SnapshotCount  int64 `json:"snapshot_count"`
	CameraStats    map[uint]CameraStorageStat `json:"camera_stats"`
}

type CameraStorageStat struct {
	CameraID       uint   `json:"camera_id"`
	CameraName     string `json:"camera_name"`
	RecordingCount int64  `json:"recording_count"`
	TotalSize      int64  `json:"total_size"`
	OldestRecording *time.Time `json:"oldest_recording"`
	LatestRecording *time.Time `json:"latest_recording"`
}

func NewManager(cfg *config.Config) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		cfg:    cfg,
		db:     database.GetDB(),
		ctx:    ctx,
		cancel: cancel,
		stats:  StorageStats{CameraStats: make(map[uint]CameraStorageStat)},
	}
}

// SetRuntimeStorage 注入运行时存储设置（设置页热更新）
func (m *Manager) SetRuntimeStorage(r *RuntimeStorage) {
	m.runtime = r
}

// local 返回当前生效的本地存储配置（运行时优先，未注入则用启动配置）
func (m *Manager) local() config.LocalStorageConfig {
	if m.runtime != nil {
		return m.runtime.GetLocal()
	}
	return m.cfg.Storage.Local
}

// webdav 返回当前生效的 WebDAV 配置（运行时优先，未注入则用启动配置）
func (m *Manager) webdav() config.WebdavConfig {
	if m.runtime != nil {
		return m.runtime.GetWebdav()
	}
	return m.cfg.Storage.Webdav
}

func (m *Manager) Start() error {
	if !m.cfg.Storage.Local.Enabled {
		logrus.Info("本地存储未启用，跳过存储管理器启动")
		return nil
	}

	// 确保根目录存在
	if err := os.MkdirAll(m.cfg.Storage.Local.RootPath, 0755); err != nil {
		return fmt.Errorf("创建存储根目录失败: %w", err)
	}

	m.wg.Add(1)
	go m.cleanupLoop()

	m.wg.Add(1)
	go m.statsLoop()

	logrus.Info("存储管理器启动完成")
	return nil
}

func (m *Manager) Stop() error {
	m.cancel()
	m.wg.Wait()
	logrus.Info("存储管理器已停止")
	return nil
}

func (m *Manager) cleanupLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(time.Duration(m.cfg.Storage.Local.CleanupInterval) * time.Second)
	defer ticker.Stop()

	// 启动时执行一次清理
	m.cleanup()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.cleanup()
		}
	}
}

func (m *Manager) cleanup() {
	m.cleanupMu.Lock()
	defer m.cleanupMu.Unlock()
	m.doCleanup()
}

// TriggerCleanup 手动触发一次完整清理（含容量上限检查），存储页"立即清理"按钮调用
func (m *Manager) TriggerCleanup() {
	m.cleanup()
}

func (m *Manager) doCleanup() {
	loc := m.local()
	if loc.Enabled {
		m.doCleanupLocal(loc)
	}
	// WebDAV 独立清理（远程保留天数 / 容量上限），与本地清理相互独立
	m.doCleanupWebdav()
}

func (m *Manager) doCleanupLocal(loc config.LocalStorageConfig) {
	cutoff := time.Now().AddDate(0, 0, -loc.MaxDays)

	// 1. 数据库层面：查找过期录像（按保留天数）
	var expiredRecordings []models.Recording
	if err := m.db.Where("end_time < ? AND storage_type = 'local' AND deleted_at IS NULL", cutoff).
		Find(&expiredRecordings).Error; err != nil {
		logrus.Errorf("查询过期录像失败: %v", err)
		return
	}

	deletedCount := 0
	deletedSize := int64(0)

	for _, rec := range expiredRecordings {
		if size := m.deleteRecordingLocalFiles(&rec); size > 0 {
			deletedSize += size
		}
		// 标记数据库记录为已删除
		if err := m.db.Delete(&rec).Error; err != nil {
			logrus.Errorf("删除录像记录失败: %v", err)
		} else {
			deletedCount++
		}
	}

	// 2. 清理孤儿文件（数据库中没有记录但文件存在的）
	m.cleanupOrphanFiles()

	// 3. 清理过期抓拍
	var expiredSnapshots []models.Snapshot
	if err := m.db.Where("timestamp < ? AND storage_type = 'local' AND deleted_at IS NULL", cutoff).
		Find(&expiredSnapshots).Error; err != nil {
		logrus.Errorf("查询过期抓拍失败: %v", err)
	} else {
		for _, snap := range expiredSnapshots {
			if err := os.Remove(snap.FilePath); err != nil && !os.IsNotExist(err) {
				logrus.Warnf("删除抓拍文件失败 %s: %v", snap.FilePath, err)
			}
			m.db.Delete(&snap)
		}
	}

	if deletedCount > 0 {
		logrus.Infof("存储清理完成: 删除 %d 个录像片段, 释放 %s", deletedCount, formatBytes(deletedSize))
	}

	// 4. 存储占用上限（与保留天数并行，谁先达到先清理）
	if loc.MaxStorageGB > 0 {
		m.enforceMaxStorage(loc)
	}
}

// doCleanupWebdav WebDAV 远程存储独立清理：
// - 按 WebDAV 独立的保留天数删除过期远程文件
// - 按 WebDAV 独立的容量上限（GB）从最旧文件开始删除
// 二者为 0 时跳过对应检查；均不启用则直接返回。
// 说明：本地清理删除的是本地文件与数据库记录，不影响远端的 WebDAV 副本；
// 因此 WebDAV 端必须用它自己的保留策略做独立清理。
func (m *Manager) doCleanupWebdav() {
	wd := m.webdav()
	if !wd.Enabled || wd.URL == "" {
		return
	}
	if wd.MaxDays <= 0 && wd.MaxStorageGB <= 0 {
		return
	}

	client := webdav.NewClient(wd.URL, wd.Username, wd.Password)
	base := strings.Trim(strings.TrimSpace(wd.BasePath), "/")

	type remoteFile struct {
		path    string
		size    int64
		modTime time.Time
	}

	var files []remoteFile
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > 6 {
			return
		}
		entries, err := client.List(dir)
		if err != nil {
			logrus.Warnf("WebDAV 清理：列目录失败 %s: %v", dir, err)
			return
		}
		for _, e := range entries {
			p := dir
			if p == "" {
				p = e.Name
			} else {
				p = p + "/" + e.Name
			}
			if e.IsDir {
				walk(p, depth+1)
				continue
			}
			files = append(files, remoteFile{path: p, size: e.Size, modTime: e.ModTime})
		}
	}
	walk(base, 0)
	if len(files) == 0 {
		return
	}

	now := time.Now()
	// 保护窗口：刚上传的文件（模时间在 10 分钟内）不参与清理，避免误删仍在写入的分段
	protectWindow := 10 * time.Minute

	// 1. 按保留天数清理
	deleted := 0
	var released int64
	deletedPaths := make(map[string]bool)
	if wd.MaxDays > 0 {
		cutoff := now.AddDate(0, 0, -wd.MaxDays)
		for _, f := range files {
			if !f.modTime.IsZero() && f.modTime.Before(cutoff) && now.Sub(f.modTime) > protectWindow {
				if err := client.Delete(f.path); err == nil {
					deleted++
					released += f.size
					deletedPaths[f.path] = true
				} else {
					logrus.Warnf("WebDAV 清理：删除过期文件失败 %s: %v", f.path, err)
				}
			}
		}
	}

	// 2. 按容量上限清理（从最旧开始删，直到回到上限以内）
	if wd.MaxStorageGB > 0 {
		var total int64
		for _, f := range files {
			total += f.size
		}
		limit := int64(wd.MaxStorageGB * 1024 * 1024 * 1024)
		if total-released > limit {
			sort.SliceStable(files, func(i, j int) bool {
				return files[i].modTime.Before(files[j].modTime)
			})
			remaining := total - released
			for _, f := range files {
				if remaining <= limit {
					break
				}
				if deletedPaths[f.path] {
					continue
				}
				if now.Sub(f.modTime) <= protectWindow {
					continue
				}
				if err := client.Delete(f.path); err == nil {
					deleted++
					released += f.size
					remaining -= f.size
					logrus.Debugf("WebDAV 容量清理：删除 %s（%s）", f.path, formatBytes(f.size))
				} else {
					logrus.Warnf("WebDAV 容量清理：删除失败 %s: %v", f.path, err)
				}
			}
		}
	}

	if deleted > 0 {
		logrus.Infof("WebDAV 清理完成: 删除 %d 个远程文件, 释放约 %s", deleted, formatBytes(released))
	}
}

// deleteRecordingLocalFiles 删除录像的本地文件（主文件+索引文件），返回主文件大小
func (m *Manager) deleteRecordingLocalFiles(rec *models.Recording) int64 {
	size := int64(0)
	if rec.FilePath != "" {
		if err := os.Remove(rec.FilePath); err == nil {
			size = rec.FileSize
		} else if !os.IsNotExist(err) {
			logrus.Warnf("删除文件失败 %s: %v", rec.FilePath, err)
		}
	}
	if rec.IndexPath != "" {
		if err := os.Remove(rec.IndexPath); err != nil && !os.IsNotExist(err) {
			logrus.Warnf("删除文件失败 %s: %v", rec.IndexPath, err)
		}
	}
	return size
}

// enforceMaxStorage 存储占用上限：根目录总占用超过上限时，从最旧的已完成录像开始删除，
// 直到占用回落到上限以内。正在写入的分段尚未入库，不会被误删。
func (m *Manager) enforceMaxStorage(loc config.LocalStorageConfig) {
	limitBytes := int64(loc.MaxStorageGB * 1024 * 1024 * 1024)
	if limitBytes <= 0 {
		return
	}

	used := dirSize(loc.RootPath)
	if used <= limitBytes {
		return
	}

	logrus.Warnf("存储占用 %s 超过上限 %s，开始容量清理（从最旧录像开始删除）",
		formatBytes(used), formatBytes(limitBytes))

	var recordings []models.Recording
	if err := m.db.Where("storage_type = 'local' AND status = 'completed' AND deleted_at IS NULL").
		Order("start_time ASC").Find(&recordings).Error; err != nil {
		logrus.Errorf("容量清理查询录像失败: %v", err)
		return
	}

	released := int64(0)
	count := 0
	for i := range recordings {
		if used-released <= limitBytes {
			break
		}
		rec := &recordings[i]
		if size := m.deleteRecordingLocalFiles(rec); size > 0 {
			released += size
		}
		if err := m.db.Delete(rec).Error; err != nil {
			logrus.Errorf("容量清理删除录像记录失败 id=%d: %v", rec.ID, err)
			continue
		}
		count++
	}

	// 仍超限：删除孤儿分段文件（流中断遗留、无数据库记录的 mp4/idx）
	if used-released > limitBytes {
		if orphanReleased, orphanCount := m.cleanupOrphanSegments(loc.RootPath, 5*time.Minute); orphanCount > 0 {
			released += orphanReleased
			count += orphanCount
		}
	}

	logrus.Infof("容量清理完成: 删除 %d 个录像片段, 释放约 %s", count, formatBytes(released))
}

// cleanupOrphanSegments 删除无数据库记录的分段文件（mp4/idx）。
// protectWindow 内修改过的文件受保护——正在写入的分段或刚写完尚未入库的分段。
func (m *Manager) cleanupOrphanSegments(root string, protectWindow time.Duration) (int64, int) {
	var released int64
	count := 0
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		name := filepath.Base(path)
		if !strings.HasSuffix(name, ".mp4") && !strings.HasSuffix(name, ".idx") {
			return nil
		}
		if time.Since(info.ModTime()) < protectWindow {
			return nil
		}
		var n int64
		m.db.Model(&models.Recording{}).Where("file_path = ?", filepath.Clean(path)).Count(&n)
		if n > 0 {
			return nil
		}
		if err := os.Remove(path); err == nil {
			released += info.Size()
			count++
			logrus.Debugf("容量清理: 删除孤儿分段文件 %s", path)
		}
		return nil
	})
	return released, count
}

// dirSize 递归统计目录下普通文件总大小
func dirSize(dir string) int64 {
	var total int64
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

func (m *Manager) cleanupOrphanFiles() {
	loc := m.local()
	root := loc.RootPath
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		cameraDir := filepath.Join(root, entry.Name())
		files, _ := os.ReadDir(cameraDir)

		for _, f := range files {
			if f.IsDir() {
				continue
			}

			filePath := filepath.Join(cameraDir, f.Name())

			// 检查数据库中是否存在
			var count int64
			m.db.Model(&models.Recording{}).Where("file_path = ?", filePath).Count(&count)
			if count == 0 {
				m.db.Model(&models.Snapshot{}).Where("file_path = ?", filePath).Count(&count)
			}

			if count == 0 {
				// 孤儿文件，检查修改时间是否超过保留期
				info, _ := f.Info()
				if time.Since(info.ModTime()) > time.Duration(loc.MaxDays)*24*time.Hour {
					os.Remove(filePath)
					logrus.Debugf("删除孤儿文件: %s", filePath)
				}
			}
		}
	}
}

func (m *Manager) statsLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	m.updateStats()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.updateStats()
		}
	}
}

func (m *Manager) updateStats() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 磁盘空间（跨平台：unix 用 Statfs，windows 用 GetDiskFreeSpaceEx）
	total, free, err := diskUsage(m.local().RootPath)
	if err != nil {
		logrus.Warnf("获取磁盘空间失败: %v", err)
	} else {
		m.stats.TotalSpace = int64(total)
		m.stats.FreeSpace = int64(free)
		m.stats.UsedSpace = m.stats.TotalSpace - m.stats.FreeSpace
	}

	// 录像统计
	var recordings []models.Recording
	m.db.Where("storage_type = 'local' AND deleted_at IS NULL").Find(&recordings)
	m.stats.RecordingCount = int64(len(recordings))

	var snapshots []models.Snapshot
	m.db.Where("storage_type = 'local' AND deleted_at IS NULL").Find(&snapshots)
	m.stats.SnapshotCount = int64(len(snapshots))

	// 按摄像头统计
	cameraStats := make(map[uint]CameraStorageStat)
	for _, rec := range recordings {
		stat := cameraStats[rec.CameraID]
		stat.CameraID = rec.CameraID
		stat.RecordingCount++
		stat.TotalSize += rec.FileSize

		if stat.OldestRecording == nil || rec.StartTime.Before(*stat.OldestRecording) {
			t := rec.StartTime
			stat.OldestRecording = &t
		}
		if stat.LatestRecording == nil || rec.EndTime.After(*stat.LatestRecording) {
			t := rec.EndTime
			stat.LatestRecording = &t
		}
		cameraStats[rec.CameraID] = stat
	}

	// 补充摄像头名称
	var cameras []models.Camera
	m.db.Find(&cameras)
	for _, cam := range cameras {
		if stat, ok := cameraStats[cam.ID]; ok {
			stat.CameraName = cam.Name
			cameraStats[cam.ID] = stat
		}
	}

	m.stats.CameraStats = cameraStats
}

func (m *Manager) GetStats() StorageStats {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stats
}

// RecordingManager 录像查询管理
type RecordingManager struct {
	db *gorm.DB
}

func NewRecordingManager() *RecordingManager {
	return &RecordingManager{db: database.GetDB()}
}

func (rm *RecordingManager) QueryRecordings(cameraID uint, start, end time.Time, recordType string, page, pageSize int) ([]models.Recording, int64, error) {
	query := rm.db.Model(&models.Recording{})
	if cameraID != 0 {
		query = query.Where("camera_id = ?", cameraID)
	}

	if !start.IsZero() {
		query = query.Where("end_time >= ?", start)
	}
	if !end.IsZero() {
		query = query.Where("start_time <= ?", end)
	}
	if recordType != "" {
		query = query.Where("record_type = ?", recordType)
	}

	var total int64
	query.Count(&total)

	var recordings []models.Recording
	offset := (page - 1) * pageSize
	err := query.Order("start_time DESC").Offset(offset).Limit(pageSize).Find(&recordings).Error

	return recordings, total, err
}

func (rm *RecordingManager) GetRecordingByID(id uint) (*models.Recording, error) {
	var rec models.Recording
	err := rm.db.First(&rec, id).Error
	return &rec, err
}

// DeleteRecording 软删除录像记录
func (rm *RecordingManager) DeleteRecording(id uint) error {
	return rm.db.Delete(&models.Recording{}, id).Error
}

func (rm *RecordingManager) GetRecordingSegments(cameraID uint, start, end time.Time) ([]models.Recording, error) {
	var recordings []models.Recording
	err := rm.db.Where("camera_id = ? AND start_time <= ? AND end_time >= ? AND status = 'completed' AND deleted_at IS NULL",
		cameraID, end, start).
		Order("start_time ASC").
		Find(&recordings).Error
	return recordings, err
}

func (rm *RecordingManager) GetLatestRecording(cameraID uint) (*models.Recording, error) {
	var rec models.Recording
	err := rm.db.Where("camera_id = ? AND status = 'completed' AND deleted_at IS NULL", cameraID).
		Order("start_time DESC").First(&rec).Error
	return &rec, err
}

// SnapshotManager 抓拍查询管理
type SnapshotManager struct {
	db *gorm.DB
}

func NewSnapshotManager() *SnapshotManager {
	return &SnapshotManager{db: database.GetDB()}
}

func (sm *SnapshotManager) QuerySnapshots(cameraID uint, start, end time.Time, snapType string, page, pageSize int) ([]models.Snapshot, int64, error) {
	query := sm.db.Model(&models.Snapshot{}).Where("camera_id = ? AND deleted_at IS NULL", cameraID)

	if !start.IsZero() {
		query = query.Where("timestamp >= ?", start)
	}
	if !end.IsZero() {
		query = query.Where("timestamp <= ?", end)
	}
	if snapType != "" {
		query = query.Where("type = ?", snapType)
	}

	var total int64
	query.Count(&total)

	var snapshots []models.Snapshot
	offset := (page - 1) * pageSize
	err := query.Order("timestamp DESC").Offset(offset).Limit(pageSize).Find(&snapshots).Error

	return snapshots, total, err
}

func (sm *SnapshotManager) GetSnapshotByID(id uint) (*models.Snapshot, error) {
	var snap models.Snapshot
	err := sm.db.First(&snap, id).Error
	return &snap, err
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}