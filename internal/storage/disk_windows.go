//go:build windows

package storage

import (
	"golang.org/x/sys/windows"
)

// diskUsage 返回路径所在文件系统的总空间与可用空间（字节）。
func diskUsage(path string) (total uint64, free uint64, err error) {
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	if err := windows.GetDiskFreeSpaceEx(pathPtr, &freeBytesAvailable, &totalBytes, &totalFreeBytes); err != nil {
		return 0, 0, err
	}
	return totalBytes, freeBytesAvailable, nil
}