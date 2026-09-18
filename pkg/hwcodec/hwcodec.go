// Package hwcodec 检测 ffmpeg 的硬件编解码能力（编译支持 + 设备确认），
// 供环境检查、系统设置页开关、ffmpeg 参数构造共用。
package hwcodec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// 硬件解码器名 → 功能名（按 ffmpeg 已知命名规则匹配）
var hwDecodeFeatures = map[string]string{
	"h264_cuvid": "NVIDIA CUVID", "hevc_cuvid": "NVIDIA CUVID", "av1_cuvid": "NVIDIA CUVID",
	"h264_nvdec": "NVIDIA CUVID", "hevc_nvdec": "NVIDIA CUVID",
	"h264_vaapi": "VAAPI", "hevc_vaapi": "VAAPI", "av1_vaapi": "VAAPI",
	"h264_qsv": "Intel QSV", "hevc_qsv": "Intel QSV", "av1_qsv": "Intel QSV",
	"h264_v4l2m2m": "V4L2 M2M", "hevc_v4l2m2m": "V4L2 M2M",
	"h264_rkmpp": "Rockchip RKMPP", "hevc_rkmpp": "Rockchip RKMPP",
	"h264_videotoolbox": "VideoToolbox", "hevc_videotoolbox": "VideoToolbox",
	"h264_mmal": "Raspberry Pi MMAL", "hevc_mmal": "Raspberry Pi MMAL",
	"h264_d3d11va": "D3D11VA", "hevc_d3d11va": "D3D11VA",
	"h264_dxva2": "DXVA2", "hevc_dxva2": "DXVA2",
	"h264_amf": "AMD AMF", "hevc_amf": "AMD AMF",
}

// 硬件编码器名 → 功能名
var hwEncodeFeatures = map[string]string{
	"h264_nvenc": "NVIDIA NVENC", "hevc_nvenc": "NVIDIA NVENC", "av1_nvenc": "NVIDIA NVENC",
	"h264_vaapi": "VAAPI", "hevc_vaapi": "VAAPI", "av1_vaapi": "VAAPI",
	"h264_qsv": "Intel QSV", "hevc_qsv": "Intel QSV", "av1_qsv": "Intel QSV",
	"h264_v4l2m2m": "V4L2 M2M", "hevc_v4l2m2m": "V4L2 M2M",
	"h264_rkmpp": "Rockchip RKMPP", "hevc_rkmpp": "Rockchip RKMPP",
	"h264_videotoolbox": "VideoToolbox", "hevc_videotoolbox": "VideoToolbox",
	"h264_mmal": "Raspberry Pi MMAL",
	"h264_amf":  "AMD AMF", "hevc_amf": "AMD AMF", "mpeg4_amf": "AMD AMF",
	"h264_mf": "Windows MF", "hevc_mf": "Windows MF",
}

// Reason 码（前端按码 i18n 出提示文案）
const (
	ReasonNone         = ""
	ReasonNoSupport    = "no_ffmpeg_support"
	ReasonNvidiaNoCUDA = "nvidia_no_cuda"
	ReasonNoDevice     = "no_device"
	ReasonNoSOPCDevice = "no_sopc_device"
)

// Feature 一个硬件加速功能（如 NVIDIA NVENC）的检测结果
type Feature struct {
	Name     string   `json:"name"`
	Decode   []string `json:"decode"`
	Encode   []string `json:"encode"`
	DecodeOK bool     `json:"decode_ok"`
	EncodeOK bool     `json:"encode_ok"`
}

// DeviceOK 任一方向设备可用（供展示用）
func (f Feature) DeviceOK() bool { return f.DecodeOK || f.EncodeOK }

// Capability 某一方向（解码/编码）的最佳可用能力
type Capability struct {
	Available bool     `json:"available"`
	Compiled  bool     `json:"compiled"`
	Feature   string   `json:"feature"`
	Codecs    []string `json:"codecs"` // 短名 H.264/H.265/AV1
	Reason    string   `json:"reason"` // !Available 时的原因码
}

// Report 完整检测结果
type Report struct {
	Features []Feature  `json:"features"`
	Decode   Capability `json:"decode"`
	Encode   Capability `json:"encode"`
}

// ---- 可注入的设备探测（便于测试）----

var (
	fileExists = func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}
	globAny = func(pattern string) bool {
		matches, _ := filepath.Glob(pattern)
		return len(matches) > 0
	}
	readDirNames = func(dir string) []string {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return names
	}
	readSysfs = func(path string) string {
		b, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
)

// SetDeviceCheckers 替换设备探测函数（仅测试用）
func SetDeviceCheckers(fe func(string) bool, glob func(string) bool, dir func(string) []string, sysfs func(string) string) {
	fileExists = fe
	globAny = glob
	readDirNames = dir
	readSysfs = sysfs
}

func nvidiaDevice() bool { return fileExists("/dev/nvidia0") }
func driDevice() bool    { return globAny("/dev/dri/renderD*") }
func isDarwin() bool     { return runtime.GOOS == "darwin" }
func isWindows() bool    { return runtime.GOOS == "windows" }
func isARM() bool        { return runtime.GOARCH == "arm64" || runtime.GOARCH == "arm" }

// v4l2VPU 探测 V4L2 M2M 硬件视频节点（SoC VPU 编解码器）。
// 通过 /sys/class/video4linux/*/name 识别：meson-video-decoder（Amlogic）、
// rkvdec/rkvenc（Rockchip）等。USB 摄像头节点不含这些关键词，不会误判。
func v4l2VPU() (decodeNode, encodeNode bool) {
	for _, n := range readDirNames("/sys/class/video4linux") {
		name := strings.ToLower(readSysfs("/sys/class/video4linux/" + n + "/name"))
		if name == "" {
			continue
		}
		if strings.Contains(name, "encoder") || strings.Contains(name, "rkvenc") {
			encodeNode = true
		}
		if strings.Contains(name, "decoder") || strings.Contains(name, "rkvdec") ||
			strings.Contains(name, "meson") || strings.Contains(name, "vpu") {
			decodeNode = true
		}
	}
	return
}

// ffprobeCodecSet 返回 ffprobe -decoders/-encoders 中的全部编解码器名
func ffprobeCodecSet(which string) map[string]bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffprobe", "-hide_banner", which).Output()
	if err != nil {
		return nil
	}
	set := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		// 列表行格式："<6位标志> <名称> <描述...>"
		if len(fields) >= 2 && len(fields[0]) == 6 && strings.ContainsAny(fields[0], "VAS") {
			set[fields[1]] = true
		}
	}
	return set
}

// BaseCodec 从硬件编解码器名提取基础码流名（h264_vaapi → h264）
func BaseCodec(codec string) string {
	for _, b := range []string{"h264", "hevc", "av1", "mpeg4"} {
		if strings.HasPrefix(codec, b) {
			return b
		}
	}
	return codec
}

// NormalizeCodec 摄像头配置码流 → ffmpeg 码流名（h265 → hevc）
func NormalizeCodec(c string) string {
	switch strings.ToLower(strings.TrimSpace(c)) {
	case "h265", "h.265", "hevc":
		return "hevc"
	case "h264", "h.264":
		return "h264"
	case "av1":
		return "av1"
	}
	return "h264"
}

// ShortCodec 基础码流名 → 展示名
func ShortCodec(c string) string {
	switch c {
	case "h264":
		return "H.264"
	case "hevc":
		return "H.265"
	case "av1":
		return "AV1"
	case "mpeg4":
		return "MPEG-4"
	}
	return c
}

// MapShort 批量转换并按常见程度排序：H.264 → H.265 → AV1 → 其他
func MapShort(base []string) []string {
	order := []string{"h264", "hevc", "av1", "mpeg4"}
	in := make(map[string]bool, len(base))
	for _, b := range base {
		in[b] = true
	}
	sorted := make([]string, 0, len(base))
	extra := []string{}
	for _, o := range order {
		if in[o] {
			sorted = append(sorted, ShortCodec(o))
			delete(in, o)
		}
	}
	for b := range in {
		extra = append(extra, ShortCodec(b))
	}
	sort.Strings(extra)
	return append(sorted, extra...)
}

// deviceOKFor 按方向（isDecode）判断功能设备是否可用
func deviceOKFor(feat string, isDecode bool, nvidia, dri, vpuDecode, vpuEncode bool) bool {
	switch {
	case strings.Contains(feat, "NVIDIA"):
		return nvidia
	case feat == "VideoToolbox":
		return isDarwin()
	case feat == "D3D11VA" || feat == "DXVA2" || feat == "Windows MF":
		return isWindows()
	case feat == "VAAPI" || feat == "Intel QSV":
		// NVIDIA 独显也会创建 DRI render 节点（vaapi 实为不可用）；
		// ARM SoC 上 vaapi/qsv 基本不是正确路径（正确路径是 v4l2m2m/rkmpp）
		return dri && !nvidia && !isARM()
	case feat == "V4L2 M2M" || feat == "Rockchip RKMPP":
		// 解码能力要求解码节点，编码能力要求编码节点（Amlogic 只有解码节点）
		if isDecode {
			return vpuDecode
		}
		return vpuEncode
	case feat == "AMD AMF" || feat == "Raspberry Pi MMAL":
		return false // 无法可靠探测，不武断判定
	}
	return false
}

// Detect 执行完整检测（ffprobe 编解码器列表 + 设备启发式）
func Detect() Report {
	decSet := ffprobeCodecSet("-decoders")
	encSet := ffprobeCodecSet("-encoders")

	states := make(map[string]*struct{ decode, encode []string })
	collect := func(set map[string]bool, table map[string]string, isDecode bool) {
		for codec, feat := range table {
			if set == nil || !set[codec] {
				continue
			}
			st := states[feat]
			if st == nil {
				st = &struct{ decode, encode []string }{}
				states[feat] = st
			}
			if isDecode {
				st.decode = append(st.decode, BaseCodec(codec))
			} else {
				st.encode = append(st.encode, BaseCodec(codec))
			}
		}
	}
	collect(decSet, hwDecodeFeatures, true)
	collect(encSet, hwEncodeFeatures, false)

	nvidia := nvidiaDevice()
	dri := driDevice()
	vpuDec, vpuEnc := v4l2VPU()

	features := make([]Feature, 0, len(states))
	for feat, st := range states {
		features = append(features, Feature{
			Name:     feat,
			Decode:   MapShort(st.decode),
			Encode:   MapShort(st.encode),
			DecodeOK: deviceOKFor(feat, true, nvidia, dri, vpuDec, vpuEnc),
			EncodeOK: deviceOKFor(feat, false, nvidia, dri, vpuDec, vpuEnc),
		})
	}
	sort.Slice(features, func(i, j int) bool { return features[i].Name < features[j].Name })

	report := Report{Features: features}
	report.Decode = bestCapability(features, true, nvidia)
	report.Encode = bestCapability(features, false, nvidia)
	return report
}

var decodePriority = []string{
	"NVIDIA CUVID", "VAAPI", "Intel QSV", "VideoToolbox", "D3D11VA", "DXVA2",
	"V4L2 M2M", "Rockchip RKMPP", "AMD AMF", "Raspberry Pi MMAL",
}
var encodePriority = []string{
	"NVIDIA NVENC", "VAAPI", "Intel QSV", "VideoToolbox", "V4L2 M2M",
	"Rockchip RKMPP", "Windows MF", "AMD AMF", "Raspberry Pi MMAL",
}

func bestCapability(features []Feature, isDecode bool, nvidia bool) Capability {
	priority := decodePriority
	if !isDecode {
		priority = encodePriority
	}
	byName := make(map[string]Feature, len(features))
	for _, f := range features {
		byName[f.Name] = f
	}
	compiledAny := false
	sopcCompiled := false
	for _, name := range priority {
		f, ok := byName[name]
		if !ok {
			continue
		}
		compiledAny = true
		if (isDecode && len(f.Decode) == 0) || (!isDecode && len(f.Encode) == 0) {
			continue
		}
		deviceOK := f.DecodeOK
		if !isDecode {
			deviceOK = f.EncodeOK
		}
		if deviceOK {
			if isDecode {
				return Capability{Available: true, Compiled: true, Feature: name, Codecs: f.Decode}
			}
			return Capability{Available: true, Compiled: true, Feature: name, Codecs: f.Encode}
		}
		if name == "V4L2 M2M" || name == "Rockchip RKMPP" {
			sopcCompiled = true
		}
	}
	cap := Capability{Compiled: compiledAny}
	switch {
	case nvidia:
		// 有 NVIDIA GPU 但 ffmpeg 无对应支持（该方向无任何 NVIDIA 功能命中）
		cap.Reason = ReasonNvidiaNoCUDA
	case sopcCompiled:
		cap.Reason = ReasonNoSOPCDevice
	case compiledAny:
		cap.Reason = ReasonNoDevice
	default:
		cap.Reason = ReasonNoSupport
	}
	return cap
}

// DecodeInputArgs 返回输入侧硬件解码参数（插在 -i 之前）；不可用返回 nil
func (r Report) DecodeInputArgs(codec string) []string {
	if !r.Decode.Available {
		return nil
	}
	c := NormalizeCodec(codec)
	switch r.Decode.Feature {
	case "V4L2 M2M", "Rockchip RKMPP":
		return []string{"-c:v", c + "_v4l2m2m"}
	case "NVIDIA CUVID":
		return []string{"-c:v", c + "_cuvid"}
	case "VAAPI":
		return []string{"-hwaccel", "vaapi"}
	case "Intel QSV":
		return []string{"-hwaccel", "qsv"}
	case "VideoToolbox":
		return []string{"-hwaccel", "videotoolbox"}
	case "D3D11VA":
		return []string{"-hwaccel", "d3d11va"}
	case "DXVA2":
		return []string{"-hwaccel", "dxva2"}
	}
	return nil
}

// EncodeArgs 返回硬件编码参数（整体替换 libx264 块）；不可用返回 nil
func (r Report) EncodeArgs(codec string, bitrateKbps int) []string {
	if !r.Encode.Available {
		return nil
	}
	c := NormalizeCodec(codec)
	suffix := ""
	extra := []string{}
	switch r.Encode.Feature {
	case "NVIDIA NVENC":
		suffix, extra = "nvenc", []string{"-preset", "p1", "-tune", "hq"}
	case "VAAPI":
		suffix = "vaapi"
	case "Intel QSV":
		suffix, extra = "qsv", []string{"-preset", "veryfast"}
	case "V4L2 M2M":
		suffix = "v4l2m2m"
	case "Rockchip RKMPP":
		suffix = "rkmpp"
	case "VideoToolbox":
		suffix, extra = "videotoolbox", []string{"-realtime", "1"}
	case "Windows MF":
		suffix = "mf"
	case "AMD AMF":
		suffix = "amf"
	default:
		return nil
	}
	args := []string{"-c:v", c + "_" + suffix}
	args = append(args, extra...)
	if bitrateKbps > 0 {
		args = append(args, "-b:v", fmt.Sprintf("%dk", bitrateKbps))
	}
	args = append(args, "-pix_fmt", "yuv420p")
	return args
}
