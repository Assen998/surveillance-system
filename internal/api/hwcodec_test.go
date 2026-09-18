package api

import (
	"strings"
	"testing"
)

func stubDevices(t *testing.T, nvidia, dri bool) {
	t.Helper()
	oldFE, oldGlob := fileExists, globAny
	fileExists = func(path string) bool {
		if path == "/dev/nvidia0" {
			return nvidia
		}
		return false
	}
	globAny = func(pattern string) bool {
		if strings.Contains(pattern, "renderD") {
			return dri
		}
		return false
	}
	t.Cleanup(func() {
		fileExists, globAny = oldFE, oldGlob
	})
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
	// 本机（无 GPU）：编解码器已编译但无设备 → compiled 文案
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
	// 有 NVIDIA 设备但 ffmpeg 无 NVENC/CUVID（若本机 ffmpeg 恰有支持则跳过断言）
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
	// 无 NVIDIA、有 DRI（x86 iGPU 场景）：VAAPI/QSV 应归入"可用"
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

func TestBaseCodec(t *testing.T) {
	cases := map[string]string{
		"h264_vaapi": "h264", "hevc_nvdec": "hevc", "av1_qsv": "av1",
		"mpeg4_amf": "mpeg4", "h264": "h264",
	}
	for in, want := range cases {
		if got := baseCodec(in); got != want {
			t.Errorf("baseCodec(%s) = %s, want %s", in, got, want)
		}
	}
}
