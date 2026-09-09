//go:build !linux

package api

import "errors"

// diskFreeBytes 非 Linux 平台（Windows/macOS）暂不支持磁盘空间检测，
// 返回错误使该检查项被跳过（不影响其他检测项）。
func diskFreeBytes(dir string) (int64, error) {
	return 0, errors.New("当前平台不支持磁盘空间检测")
}
