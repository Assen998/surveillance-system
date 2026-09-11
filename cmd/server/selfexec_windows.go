//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func selfExec(path string, args []string, env []string) error {
	cmd := exec.Command(path, args...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	const DETACHED_PROCESS = 0x00000008
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: DETACHED_PROCESS}
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
