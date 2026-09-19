package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/yourorg/surveillance-system/pkg/hwcodec"
)

// stubDevices 注入设备探测：nvidia/dri 布尔 + V4L2 节点名列表
func stubDevices(t *testing.T, nvidia, dri bool, v4l2Nodes ...string) {
	t.Helper()
	t.Cleanup(func() { hwcodec.ResetVerify() })
	byName := map[string]string{}
	for i, n := range v4l2Nodes {
		byName["/sys/class/video4linux/video"+string(rune('0'+i))+"/name"] = n
	}
	nodeList := make([]string, 0, len(v4l2Nodes))
	for i := range v4l2Nodes {
		nodeList = append(nodeList, "video"+string(rune('0'+i)))
	}
	hwcodec.SetDeviceCheckers(
		func(path string) bool { return path == "/dev/nvidia0" && nvidia },
		func(pattern string) bool { return strings.Contains(pattern, "renderD") && dri },
		func(dir string) []string {
			if dir == "/sys/class/video4linux" {
				return nodeList
			}
			return nil
		},
		func(path string) string { return byName[path] },
	)
}

func TestHwCodecNoDevice(t *testing.T) {
	stubDevices(t, false, false)
	s := &Server{}
	item := s.hwCodecCheck(localeZH)
	if item.Status != envOK {
		t.Fatalf("expected ok, got %s", item.Status)
	}
	if item.Name != "硬件编解码" {
		t.Fatalf("unexpected name: %s", item.Name)
	}
	// 无 GPU/无 VPU 节点：已编译但不可用
	if !strings.Contains(item.Detail, "已编译") {
		t.Fatalf("expected compiled message, got: %s", item.Detail)
	}
	// 码流顺序应为 H.264/H.265/AV1
	if i, j := strings.Index(item.Detail, "H.264"), strings.Index(item.Detail, "H.265"); i < 0 || j < 0 || i > j {
		t.Fatalf("codec order wrong: %s", item.Detail)
	}
}

func TestHwCodecNvidiaWarn(t *testing.T) {
	stubDevices(t, true, true)
	s := &Server{}
	item := s.hwCodecCheck(localeEN)
	if item.Status == envWarn {
		if !strings.Contains(item.Detail, "NVENC") {
			t.Fatalf("warn should mention NVENC: %s", item.Detail)
		}
		if item.Fix == "" {
			t.Fatal("warn should carry a fix hint")
		}
	}
}

func TestHwCodecDriReady(t *testing.T) {
	stubDevices(t, false, true)
	s := &Server{}
	item := s.hwCodecCheck(localeZH)
	if item.Status != envOK {
		t.Fatalf("expected ok, got %s", item.Status)
	}
	if !strings.Contains(item.Detail, "VAAPI") {
		t.Fatalf("expected VAAPI in detail: %s", item.Detail)
	}
}

func TestHwCodecAmlogicDecodeOnly(t *testing.T) {
	// .231 场景：Amlogic SoC 只有硬件解码节点（meson-video-decoder）
	stubDevices(t, false, false, "meson-video-decoder")
	r := hwcodec.Detect()
	if !r.Decode.Available {
		t.Fatalf("expected decode available, got %+v", r.Decode)
	}
	if r.Decode.Feature != "V4L2 M2M" {
		t.Fatalf("expected V4L2 M2M, got %s", r.Decode.Feature)
	}
	if r.Encode.Available {
		t.Fatalf("expected encode unavailable on decode-only SoC, got %+v", r.Encode)
	}
	if r.Encode.Reason != hwcodec.ReasonNoSOPCDevice {
		t.Fatalf("expected no_sopc_device reason, got %s", r.Encode.Reason)
	}
	args := r.DecodeInputArgs("h264")
	if len(args) != 2 || args[0] != "-c:v" || args[1] != "h264_v4l2m2m" {
		t.Fatalf("unexpected decode args: %v", args)
	}
}

func TestHWCodecNormalizeAndArgs(t *testing.T) {
	if got := hwcodec.NormalizeCodec("h265"); got != "hevc" {
		t.Fatalf("NormalizeCodec(h265) = %s", got)
	}
	if got := hwcodec.NormalizeCodec("H.264"); got != "h264" {
		t.Fatalf("NormalizeCodec(H.264) = %s", got)
	}
	stubDevices(t, true, false) // NVIDIA 设备
	r := hwcodec.Detect()
	// 若本机 ffmpeg 含 nvenc/cuvid（常见），编码参数应可用
	if r.Encode.Available {
		args := r.EncodeArgs("h264", 4096)
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "nvenc") {
			t.Fatalf("expected nvenc in args: %s", joined)
		}
		if !strings.Contains(joined, "4096k") {
			t.Fatalf("expected bitrate in args: %s", joined)
		}
	}
}

func TestHwCodecCheckVerifyFailed(t *testing.T) {
	// .231 场景完整版：解码节点存在，但真实自检失败（驱动/ffmpeg 不兼容）
	stubDevices(t, false, false, "meson-video-decoder")
	ctxStub := func(ctx context.Context, name string, args ...string) error {
		for _, a := range args {
			if a == "h264_v4l2m2m" {
				return errors.New("probe failed")
			}
		}
		return nil
	}
	hwcodec.SetProbeRunner(ctxStub)

	hwcodec.RunVerify()

	var s Server
	item := s.hwCodecCheck(localeZH)
	if item.Status != envWarn {
		t.Fatalf("expected warn status, got %s", item.Status)
	}
	if !strings.Contains(item.Detail, "自检未通过") {
		t.Fatalf("expected verify-failed detail, got %s", item.Detail)
	}
	if !strings.Contains(item.Detail, "V4L2 M2M") {
		t.Fatalf("expected feature name in detail, got %s", item.Detail)
	}
	// 探测失败的 V4L2 M2M 不应再出现在“已编译但未检测可用设备”列表里
	if strings.Contains(item.Detail, "另已编译支持") && strings.Count(item.Detail, "V4L2 M2M") > 1 {
		t.Fatalf("failed feature should not be listed twice: %s", item.Detail)
	}
}
