package ffmpeg

import (
	"net"
	"os/exec"
	"testing"
	"time"
)

// TestPreviewHWFallbackExit：硬件解码参数无效（本机无 v4l2m2m 设备）时，
// ffmpeg 快速退出（未被主动 Stop）→ 监控 goroutine 触发 OnHWFallback。
func TestPreviewHWFallbackExit(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	outDir := t.TempDir()
	ps := NewPreviewStream(999, "rtsp://127.0.0.1:19999/no-such-stream", outDir)
	ps.DecodeArgs = []string{"-c:v", "h264_v4l2m2m"} // 本机无 V4L2 M2M 设备 → ffmpeg 立即退出
	fired := make(chan struct{}, 1)
	ps.OnHWFallback = func() { fired <- struct{}{} }

	if err := ps.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case <-fired:
	case <-time.After(20 * time.Second):
		ps.Stop()
		t.Fatal("OnHWFallback not fired within 20s (exit path)")
	}
	ps.Stop()
}

// TestPreviewHWFallbackWatchdog：硬件路径进程挂死（RTSP 连接不响应，
// ffmpeg 存活但无输出，模拟 .231 Amlogic V4L2 M2M 挂起场景）→
// 看门狗在超时后强制结束进程并触发 OnHWFallback。
func TestPreviewHWFallbackWatchdog(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	old := hwWatchdogTimeout
	hwWatchdogTimeout = 3 * time.Second
	defer func() { hwWatchdogTimeout = old }()

	// 假 RTSP 服务器：接受 TCP 连接但永不响应 → ffmpeg 卡在握手阶段不退出
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	// 已接受的连接永久持有：不发送数据、不关闭 → ffmpeg 卡在 RTSP 握手
	held := make(chan net.Conn)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			held <- c
		}
	}()

	outDir := t.TempDir()
	ps := NewPreviewStream(997, "rtsp://"+ln.Addr().String()+"/hang", outDir)
	ps.DecodeArgs = []string{"-c:v", "h264_v4l2m2m"} // 启用硬件路径（看门狗生效）
	fired := make(chan struct{}, 1)
	ps.OnHWFallback = func() { fired <- struct{}{} }

	if err := ps.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case <-fired:
		// 看门狗应已杀掉挂死进程
		if ps.IsRunning() {
			t.Fatal("process should be killed by watchdog")
		}
	case <-time.After(15 * time.Second):
		ps.Stop()
		t.Fatal("watchdog fallback not fired within 15s")
	}
	ps.Stop()
}

// TestPreviewStopNoFallback：主动 Stop 触发的进程退出不应触发回退
func TestPreviewStopNoFallback(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	outDir := t.TempDir()
	ps := NewPreviewStream(998, "rtsp://127.0.0.1:19998/no-such-stream", outDir)
	ps.DecodeArgs = []string{"-c:v", "h264_v4l2m2m"}
	fired := make(chan struct{}, 1)
	ps.OnHWFallback = func() { fired <- struct{}{} }

	if err := ps.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// 立即主动 Stop：即使进程随后快速退出，也不应触发回退
	ps.Stop()
	select {
	case <-fired:
		t.Fatal("OnHWFallback fired after manual Stop")
	case <-time.After(5 * time.Second):
	}
}
