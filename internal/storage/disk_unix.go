//go:build !windows

package storage

import (
	"golang.org/x/sys/unix"
)

// diskUsage 返回路径所在文件系统的总空间与可用空间（字节）。
func diskUsage(path string) (total uint64, free uint64, err error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	// Blocks*Bsize = 总空间；Bavail*Bsize = 可用空间（非特权用户可用）
	total = stat.Blocks * uint64(stat.Bsize)
	free = stat.Bavail * uint64(stat.Bsize)
	return total, free, nil
}