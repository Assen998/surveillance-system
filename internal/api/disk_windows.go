//go:build windows

package api

// diskUsageMB Windows 下暂不读取磁盘用量（返回 0）
func diskUsageMB(path string) (totalMB, usedMB int64) {
	return 0, 0
}
