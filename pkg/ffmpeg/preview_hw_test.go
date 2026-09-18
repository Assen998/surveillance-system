package ffmpeg

import (
	"os/exec"
	"testing"
	"time"
)

// TestPreviewHWFallback：硬件解码参数无效（本机无 v4l2m2m 设备）时，
// ffmpeg 快速退出（未被主动 Stop）→ 监控 goroutine 在 15s 窗口内触发 OnHWFallback。
func TestPreviewHWFallback(t *testing.T) {
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
		// 预期：快速死亡 + 未主动 Stop → 触发回退
	case <-time.After(20 * time.Second):
		ps.Stop()
		t.Fatal("OnHWFallback not fired within 20s")
	}
	ps.Stop()
}
