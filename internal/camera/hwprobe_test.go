package camera

import (
	"net"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/yourorg/surveillance-system/internal/config"
	"github.com/yourorg/surveillance-system/internal/models"
	"github.com/yourorg/surveillance-system/pkg/hwcodec"
)

// newTestMgr 构造最小可用的 CameraManager（不依赖数据库）
func newTestMgr(t *testing.T) *CameraManager {
	t.Helper()
	t.Cleanup(func() { detectHWReport = hwcodec.Detect; runHWDecodeProbe = realHWDecodeProbe })
	m := &CameraManager{
		cfg:     &config.Config{},
		cameras: make(map[uint]*CameraInstance),
	}
	inst := &CameraInstance{
		Model: &models.Camera{
			Name: "probe-test", Protocol: "rtsp", IP: "127.0.0.1", Port: 8556,
			Path: "/x", Codec: "h264", Width: 640, Height: 480, FPS: 15, Bitrate: 1024,
			RecordEnabled: true,
		},
		RecordRTSPURL:  "rtsp://127.0.0.1:8556/x",
		PreviewRTSPURL: "rtsp://127.0.0.1:8556/x",
		StopChan:       make(chan struct{}),
		loopDone:       make(chan struct{}),
		previewMu:      sync.Mutex{},
		running:        true,
	}
	m.cameras[1] = inst
	return m
}

func fakeV4L2Report() hwcodec.Report {
	return hwcodec.Report{
		Decode: hwcodec.Capability{
			Available: true, Compiled: true, Feature: "V4L2 M2M",
			Codecs: []string{"H.264", "H.265"},
		},
	}
}

// holdRTSP 假 RTSP 服务器：接受连接但永不响应（复现 .231 挂死场景）
func holdRTSP(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c // 永久持有连接，不响应
			select {}
		}
	}()
	return "rtsp://" + ln.Addr().String() + "/x"
}

// TestProbeCameraHWHangFails：真实 ffmpeg + 挂死 RTSP → 自检超时 → hwFailed
func TestProbeCameraHWHangFails(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	m := newTestMgr(t)
	r := fakeV4L2Report()
	m.hwReport = &r

	inst := m.cameras[1]
	url := holdRTSP(t)
	inst.RecordRTSPURL = url
	inst.PreviewRTSPURL = url

	oldTO := hwProbeTimeout
	hwProbeTimeout = 3 * time.Second
	t.Cleanup(func() { hwProbeTimeout = oldTO })

	m.probeCameraHW(1, "sub")

	inst.mu.Lock()
	failed, probed, inFlight := inst.hwFailed, inst.hwProbedOK, inst.hwProbeInFlight
	inst.mu.Unlock()
	if !failed || probed || inFlight {
		t.Fatalf("expected hwFailed=true after hung probe, got failed=%v probed=%v inFlight=%v", failed, probed, inFlight)
	}
}

// TestProbeCameraHWPass：自检通过 → hwProbedOK（无运行中预览时 swap 应安全跳过）
func TestProbeCameraHWPass(t *testing.T) {
	m := newTestMgr(t)
	r := fakeV4L2Report()
	m.hwReport = &r

	runHWDecodeProbe = func(args []string, url string) bool { return true }

	m.probeCameraHW(1, "sub")

	inst := m.cameras[1]
	inst.mu.Lock()
	failed, probed, inFlight := inst.hwFailed, inst.hwProbedOK, inst.hwProbeInFlight
	inst.mu.Unlock()
	if failed || !probed || inFlight {
		t.Fatalf("expected probedOK=true after passing probe, got failed=%v probed=%v inFlight=%v", failed, probed, inFlight)
	}
}

// TestProbeCameraHWSkippedWhenUnavailable：能力不可用时不发起自检
func TestProbeCameraHWSkippedWhenUnavailable(t *testing.T) {
	m := newTestMgr(t)
	r := hwcodec.Report{} // 无能力
	m.hwReport = &r

	calls := 0
	runHWDecodeProbe = func(args []string, url string) bool { calls++; return true }

	m.probeCameraHW(1, "sub")

	inst := m.cameras[1]
	inst.mu.Lock()
	probed := inst.hwProbedOK
	inFlight := inst.hwProbeInFlight
	inst.mu.Unlock()
	if calls != 0 || probed || inFlight {
		t.Fatalf("probe should be skipped without capability, calls=%d probed=%v inFlight=%v", calls, probed, inFlight)
	}
}
