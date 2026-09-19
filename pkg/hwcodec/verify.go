package hwcodec

// 自检（功能验证）
//
// 设备启发式（hwcodec.go）只回答「设备节点是否存在」。部分芯片/驱动/ffmpeg
// 组合下设备能打开但无数据输出（如 Amlogic meson-video-decoder 在特定环境
// 使 ffmpeg 挂死）。本文件在服务启动后做一次真实合成流自检：
//
//	解码：生成 2s H.264 测试流 → 硬件解码 → -f null
//	编码：lavfi 合成源 → 硬件编码 → -f null
//
// 自检失败（含 10s 超时杀死挂死进程）→ 该方向能力标记不可用
// （reason=hw_verify_failed）：设置页开关禁用并给出原因，环境检查措辞一致。
// 自检结果进程生命周期内有效（服务重启后重新自检）。
//
// 只自检 H.264（用户摄像头主流码流）；H.265 硬件路径的异常由预览看门狗
// （pkg/ffmpeg）兜底回退。

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const (
	probeTimeout  = 10 * time.Second
	probeCodec    = "h264"
	probeSize     = "640x480"
	probeRate     = 10
	probeDuration = 2
)

var (
	verifyMu     sync.Mutex
	verifyFailed = map[string]bool{} // "decode|V4L2 M2M" → 自检未通过
	verifyDone   = map[string]bool{} // "decode|V4L2 M2M" → 已执行（或未决，不重试）
	verifyBusy   atomic.Bool
)

// runProbe 可注入（测试用）
var runProbe = func(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

// SetProbeRunner 替换自检命令执行器（仅测试用）
func SetProbeRunner(fn func(ctx context.Context, name string, args ...string) error) {
	verifyMu.Lock()
	defer verifyMu.Unlock()
	runProbe = fn
}

// ResetVerify 清空自检状态（仅测试用）
func ResetVerify() {
	verifyMu.Lock()
	defer verifyMu.Unlock()
	verifyFailed = map[string]bool{}
	verifyDone = map[string]bool{}
	verifyBusy.Store(false)
}

// probeSourceDir 自检源文件目录（进程内共享一个文件）
var (
	probeSrcMu  sync.Mutex
	probeSrcMem string
)

func probeSourcePath() string {
	return filepath.Join(os.TempDir(), "dsh_hwprobe.h264")
}

// makeProbeSource 生成 2s H.264 裸流测试源（进程内缓存）
func makeProbeSource() (string, error) {
	probeSrcMu.Lock()
	defer probeSrcMu.Unlock()
	if probeSrcMem != "" {
		if _, err := os.Stat(probeSrcMem); err == nil {
			return probeSrcMem, nil
		}
		probeSrcMem = ""
	}
	path := probeSourcePath()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := runProbe(ctx, "ffmpeg",
		"-f", "lavfi",
		"-i", fmt.Sprintf("testsrc=duration=%d:size=%s:rate=%d", probeDuration, probeSize, probeRate),
		"-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-y", path)
	if err != nil {
		return "", err
	}
	probeSrcMem = path
	return path, nil
}

// probeDecodeArgs 硬件解码器自检命令（解码器在输入侧）
func probeDecodeArgs(feature, sourcePath string) (args []string, ok bool) {
	switch feature {
	case "V4L2 M2M", "Rockchip RKMPP":
		return []string{"-c:v", probeCodec + "_v4l2m2m", "-f", probeCodec, "-i", sourcePath, "-f", "null", "-"}, true
	case "NVIDIA CUVID":
		return []string{"-c:v", probeCodec + "_cuvid", "-f", probeCodec, "-i", sourcePath, "-f", "null", "-"}, true
	case "VAAPI", "Intel QSV", "VideoToolbox", "D3D11VA", "DXVA2":
		acc := map[string]string{
			"VAAPI": "vaapi", "Intel QSV": "qsv", "VideoToolbox": "videotoolbox",
			"D3D11VA": "d3d11va", "DXVA2": "dxva2",
		}[feature]
		return []string{"-f", probeCodec, "-hwaccel", acc, "-i", sourcePath, "-f", "null", "-"}, true
	}
	return nil, false
}

// probeEncodeArgs 硬件编码器自检命令（合成源直接硬件编码）
func probeEncodeArgs(feature string) (args []string, ok bool) {
	suffix := map[string]string{
		"NVIDIA NVENC": "nvenc", "VAAPI": "vaapi", "Intel QSV": "qsv",
		"V4L2 M2M": "v4l2m2m", "Rockchip RKMPP": "rkmpp",
		"VideoToolbox": "videotoolbox", "Windows MF": "mf", "AMD AMF": "amf",
	}[feature]
	if suffix == "" {
		return nil, false
	}
	return []string{
		"-f", "lavfi",
		"-i", fmt.Sprintf("testsrc=duration=%d:size=%s:rate=%d", probeDuration, probeSize, probeRate),
		"-c:v", probeCodec + "_" + suffix, "-pix_fmt", "yuv420p",
		"-f", "null", "-",
	}, true
}

// probeOne 对单个方向/功能执行自检并写入缓存。
// 无法构造自检命令（如 MMAL/AMF）→ 不下结论。
func probeOne(dir, feature string) {
	key := dir + "|" + feature
	verifyMu.Lock()
	if verifyDone[key] {
		verifyMu.Unlock()
		return
	}
	verifyMu.Unlock()

	pass := true
	switch dir {
	case "decode":
		src, err := makeProbeSource()
		if err != nil {
			pass = true // 源生成失败（如缺 libx264）→ 不下结论
		} else if args, ok := probeDecodeArgs(feature, src); ok {
			ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
			pass = runProbe(ctx, "ffmpeg", args...) == nil
			cancel()
		}
	case "encode":
		if args, ok := probeEncodeArgs(feature); ok {
			ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
			pass = runProbe(ctx, "ffmpeg", args...) == nil
			cancel()
		}
	}

	verifyMu.Lock()
	verifyDone[key] = true
	if !pass {
		verifyFailed[key] = true
	}
	verifyMu.Unlock()
}

// RunVerify 执行一次自检（幂等，可重复调用；仅探测当前最优能力）。
// 服务启动后由 main 在后台调用。
func RunVerify() {
	if !verifyBusy.CompareAndSwap(false, true) {
		return
	}
	defer verifyBusy.Store(false)

	report := Detect()
	if report.Decode.Available && report.Decode.Feature != "" {
		probeOne("decode", report.Decode.Feature)
	}
	if report.Encode.Available && report.Encode.Feature != "" {
		probeOne("encode", report.Encode.Feature)
	}
}

// downgradeFeatures 将自检未通过的功能从设备可用标记中剔除
func downgradeFeatures(features []Feature) {
	verifyMu.Lock()
	defer verifyMu.Unlock()
	for i := range features {
		if verifyFailed["decode|"+features[i].Name] {
			features[i].DecodeOK = false
		}
		if verifyFailed["encode|"+features[i].Name] {
			features[i].EncodeOK = false
		}
	}
}

// failedFeatureFor 返回该方向上「设备存在但自检未通过」的最高优先级功能名
func failedFeatureFor(features []Feature, isDecode bool) string {
	priority := decodePriority
	if !isDecode {
		priority = encodePriority
	}
	byName := make(map[string]Feature, len(features))
	for _, f := range features {
		byName[f.Name] = f
	}
	dir := "decode"
	if !isDecode {
		dir = "encode"
	}
	verifyMu.Lock()
	defer verifyMu.Unlock()
	for _, name := range priority {
		f, ok := byName[name]
		if !ok {
			continue
		}
		if (isDecode && len(f.Decode) == 0) || (!isDecode && len(f.Encode) == 0) {
			continue
		}
		if verifyFailed[dir+"|"+name] {
			return name
		}
	}
	return ""
}
