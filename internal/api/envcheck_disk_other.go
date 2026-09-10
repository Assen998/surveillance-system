//go:build !linux

package api

import "errors"


func diskFreeBytes(dir string) (int64, error) {
	return 0, errors.New("当前平台不支持磁盘空间检测")
}
