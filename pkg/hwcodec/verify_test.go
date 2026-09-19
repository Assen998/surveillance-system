package hwcodec

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

// realProbeRunner 真实 ffmpeg 执行器
var realProbeRunner = func(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

// fakeSysfs 伪造 /sys/class/video4linux（模拟 .231：只有解码节点）
func fakeAmlogicSysfs() {
	SetDeviceCheckers(
		func(string) bool { return false }, // fileExists: 无 nvidia
		func(string) bool { return false }, // globAny: 无 dri
		func(dir string) []string {
			if dir == "/sys/class/video4linux" {
				return []string{"video0"}
			}
			return nil
		},
		func(path string) string {
			if path == "/sys/class/video4linux/video0/name" {
				return "meson-video-decoder"
			}
			return ""
		},
	)
}

func TestVerifyProbeFailDowngrades(t *testing.T) {
	t.Cleanup(func() { ResetVerify(); SetProbeRunner(func(context.Context, string, ...string) error { return nil }) })
	fakeAmlogicSysfs()
	ResetVerify()
	// 合成源生成放行（libx264），仅硬件解码探测失败（模拟 .231 挂死/报错）
	SetProbeRunner(func(ctx context.Context, name string, args ...string) error {
		for _, a := range args {
			if a == "h264_v4l2m2m" {
				return errFake
			}
		}
		return nil
	})

	// 自检前：设备启发式判定解码可用
	r1 := Detect()
	if !r1.Decode.Available || r1.Decode.Feature != "V4L2 M2M" {
		t.Fatalf("pre-verify: decode should be available V4L2 M2M, got %+v", r1.Decode)
	}

	RunVerify()

	// 自检后：降级为不可用 + verify_failed 理由码
	r2 := Detect()
	if r2.Decode.Available {
		t.Fatalf("post-verify: decode should be unavailable, got %+v", r2.Decode)
	}
	if r2.Decode.Reason != ReasonVerifyFailed {
		t.Fatalf("post-verify: reason = %q, want %q", r2.Decode.Reason, ReasonVerifyFailed)
	}
	if r2.Decode.Feature != "V4L2 M2M" {
		t.Fatalf("post-verify: feature = %q, want V4L2 M2M", r2.Decode.Feature)
	}
	// 功能级标记同步翻转（环境检查分桶一致）
	for _, f := range r2.Features {
		if f.Name == "V4L2 M2M" && f.DecodeOK {
			t.Fatal("post-verify: feature DecodeOK should be flipped to false")
		}
	}
	// 编码方向未受解码自检影响（.231 本来就没有编码节点）
	if r2.Encode.Available {
		t.Fatal("encode should remain unavailable")
	}
}

func TestVerifyProbePassKeepsAvailable(t *testing.T) {
	t.Cleanup(func() { ResetVerify(); SetProbeRunner(func(context.Context, string, ...string) error { return nil }) })
	fakeAmlogicSysfs()
	ResetVerify()
	SetProbeRunner(func(ctx context.Context, name string, args ...string) error { return nil })

	RunVerify()

	r := Detect()
	if !r.Decode.Available || r.Decode.Feature != "V4L2 M2M" {
		t.Fatalf("verify pass: decode should stay available, got %+v", r.Decode)
	}
	// 幂等：再次 RunVerify 不重复探测
	calls := 0
	SetProbeRunner(func(ctx context.Context, name string, args ...string) error { calls++; return nil })
	RunVerify()
	if calls != 0 {
		t.Fatalf("second RunVerify should not re-probe, calls=%d", calls)
	}
}

// TestVerifySkippedWithoutDevice：无可用设备时 RunVerify 不执行任何探测
func TestVerifySkippedWithoutDevice(t *testing.T) {
	t.Cleanup(func() { ResetVerify(); SetProbeRunner(func(context.Context, string, ...string) error { return nil }) })
	SetDeviceCheckers(
		func(string) bool { return false },
		func(string) bool { return false },
		func(string) []string { return nil },
		func(string) string { return "" },
	)
	ResetVerify()
	calls := 0
	SetProbeRunner(func(ctx context.Context, name string, args ...string) error {
		calls++
		return nil
	})

	RunVerify()
	if calls != 0 {
		t.Fatalf("no device: probe should be skipped, calls=%d", calls)
	}
}

func TestProbeArgsBuilders(t *testing.T) {
	dec, ok := probeDecodeArgs("V4L2 M2M", "/tmp/x.h264")
	if !ok || len(dec) != 9 {
		t.Fatalf("probeDecodeArgs V4L2 M2M: %v ok=%v", dec, ok)
	}
	if dec[0] != "-c:v" || dec[1] != "h264_v4l2m2m" {
		t.Fatalf("v4l2m2m decode args: %v", dec)
	}

	va, ok := probeDecodeArgs("VAAPI", "/tmp/x.h264")
	if !ok || va[2] != "-hwaccel" || va[3] != "vaapi" {
		t.Fatalf("vaapi decode args: %v ok=%v", va, ok)
	}

	cu, ok := probeDecodeArgs("NVIDIA CUVID", "/tmp/x.h264")
	if !ok || cu[1] != "h264_cuvid" {
		t.Fatalf("cuvid decode args: %v ok=%v", cu, ok)
	}

	if _, ok := probeDecodeArgs("AMD AMF", "/tmp/x.h264"); ok {
		t.Fatal("AMF should not have a probe")
	}

	enc, ok := probeEncodeArgs("NVIDIA NVENC")
	if !ok || enc[5] != "h264_nvenc" {
		t.Fatalf("nvenc encode args: %v ok=%v", enc, ok)
	}
	if _, ok := probeEncodeArgs("Raspberry Pi MMAL"); ok {
		t.Fatal("MMAL should not have an encode probe")
	}
}

var errFake = fakeErr("probe failed")

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

// cleanupProbeSource 清理自检源文件
func cleanupProbeSource() {
	os.Remove(probeSourcePath())
}

// TestVerifyRealFFmpegNoDevice：真实 ffmpeg 端到端——伪造 .231 设备节点，
// 真实探测 h264_v4l2m2m（本机无 /dev/video* → 必然失败）→ 降级 verify_failed。
func TestVerifyRealFFmpegNoDevice(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	t.Cleanup(func() { cleanupProbeSource(); ResetVerify(); SetProbeRunner(realProbeRunner) })
	cleanupProbeSource()
	fakeAmlogicSysfs()
	ResetVerify()
	// 使用真实 ffmpeg 执行探测
	SetProbeRunner(realProbeRunner)

	RunVerify()

	r := Detect()
	if r.Decode.Available {
		t.Fatalf("real probe: decode should be unavailable on a machine without V4L2 device, got %+v", r.Decode)
	}
	if r.Decode.Reason != ReasonVerifyFailed {
		t.Fatalf("real probe: reason = %q, want %q", r.Decode.Reason, ReasonVerifyFailed)
	}
}
