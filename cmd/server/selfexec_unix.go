//go:build !windows

package main

import "syscall"

// selfExec 用 syscall.Exec 原地替换当前进程映像：
// 同一 PID 直接运行新二进制，systemd 无感知（不会触发 Restart），
// 也不会出现新旧进程抢端口的竞争。
func selfExec(path string, args []string, env []string) error {
	return syscall.Exec(path, args, env)
}
