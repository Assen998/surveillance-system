package api

import (
	"context"
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

type hwFeatureState struct {
	decode []string // 基础码流名（h264/hevc/av1...）
	encode []string
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

// baseCodec 从硬件编解码器名提取基础码流名（h264_vaapi → h264）
func baseCodec(codec string) string {
	for _, b := range []string{"h264", "hevc", "av1", "mpeg4"} {
		if strings.HasPrefix(codec, b) {
			return b
		}
	}
	return codec
}

func shortCodec(c string) string {
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

var fileExists = func(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

var globAny = func(pattern string) bool {
	matches, _ := filepath.Glob(pattern)
	return len(matches) > 0
}

// hwCodecCheck 检测硬件编解码支持（信息性检查，不影响通过/失败结论）
//
// 分两类：
//  1. ready   —— 编译支持 + 启发式确认设备存在（NVIDIA/DRI/VideoToolbox/Windows）
//  2. compiled —— 已编译但无法确认设备（V4L2 M2M/RKMPP/AMF/MMAL 取决于具体 SoC；
//     VAAPI/QSV 在无 DRI 节点或 ARM SoC 上不能确认）
//
// 特例：检测到 NVIDIA GPU 但 ffmpeg 无 NVENC/CUVID 支持时给 warn（用户有卡但用不上）。
func (s *Server) hwCodecCheck(l string) CheckItem {
	name := TEnv(l, "env.hw.name")
	sep := ", "
	if l == localeZH {
		sep = "、"
	}

	decSet := ffprobeCodecSet("-decoders")
	encSet := ffprobeCodecSet("-encoders")
	if decSet == nil && encSet == nil {
		return CheckItem{Name: name, Status: envOK, Detail: TEnv(l, "env.hw.none")}
	}

	states := make(map[string]*hwFeatureState)
	collect := func(set map[string]bool, table map[string]string, isDecode bool) {
		for codec, feat := range table {
			if set == nil || !set[codec] {
				continue
			}
			st := states[feat]
			if st == nil {
				st = &hwFeatureState{}
				states[feat] = st
			}
			if isDecode {
				st.decode = append(st.decode, baseCodec(codec))
			} else {
				st.encode = append(st.encode, baseCodec(codec))
			}
		}
	}
	collect(decSet, hwDecodeFeatures, true)
	collect(encSet, hwEncodeFeatures, false)

	if len(states) == 0 {
		return CheckItem{Name: name, Status: envOK, Detail: TEnv(l, "env.hw.none")}
	}

	// 设备存在性（启发式）
	nvidiaDev := fileExists("/dev/nvidia0")
	driDev := globAny("/dev/dri/renderD*")
	darwin := runtime.GOOS == "darwin"
	windows := runtime.GOOS == "windows"
	arm := runtime.GOARCH == "arm64" || runtime.GOARCH == "arm"

	deviceOK := func(feat string) bool {
		switch {
		case strings.Contains(feat, "NVIDIA"):
			return nvidiaDev
		case feat == "VideoToolbox":
			return darwin
		case feat == "D3D11VA" || feat == "DXVA2" || feat == "Windows MF":
			return windows
		case feat == "VAAPI" || feat == "Intel QSV":
			// NVIDIA 独显也会创建 DRI render 节点（vaapi 实为不可用）；
			// ARM SoC 上 vaapi/qsv 基本不是正确路径（正确路径是 v4l2m2m/rkmpp）
			return driDev && !nvidiaDev && !arm
		default:
			return false // V4L2 M2M / RKMPP / AMF / MMAL：取决于具体 SoC，不武断判定
		}
	}

	mapShort := func(base []string) []string {
		// 按常见程度排序：H.264 → H.265 → AV1 → 其他
		order := []string{"h264", "hevc", "av1", "mpeg4"}
		in := make(map[string]bool, len(base))
		for _, b := range base {
			in[b] = true
		}
		sorted := make([]string, 0, len(base))
		extra := []string{}
		for _, o := range order {
			if in[o] {
				sorted = append(sorted, shortCodec(o))
				delete(in, o)
			}
		}
		for b := range in {
			extra = append(extra, shortCodec(b))
		}
		sort.Strings(extra)
		return append(sorted, extra...)
	}
	featDesc := func(feat string, st *hwFeatureState) string {
		desc := feat
		if len(st.encode) > 0 {
			desc += " " + TEnv(l, "env.hw.encode") + " " +
				strings.Join(mapShort(st.encode), "/")
		}
		if len(st.decode) > 0 {
			desc += " " + TEnv(l, "env.hw.decode") + " " +
				strings.Join(mapShort(st.decode), "/")
		}
		return desc
	}

	ready, compiled := []string{}, []string{}
	for feat, st := range states {
		if deviceOK(feat) {
			ready = append(ready, featDesc(feat, st))
		} else {
			compiled = append(compiled, featDesc(feat, st))
		}
	}
	sort.Strings(ready)
	sort.Strings(compiled)

	// 有 NVIDIA GPU 但 ffmpeg 不含其硬件编解码 → warn（卡买了但用不上）
	if nvidiaDev {
		if _, hasEnc := states["NVIDIA NVENC"]; !hasEnc {
			if _, hasDec := states["NVIDIA CUVID"]; !hasDec {
				return CheckItem{
					Name: name, Status: envWarn,
					Detail: TEnv(l, "env.hw.nvidiaMissing"),
					Fix:    TEnv(l, "env.hw.nvidiaFix"),
				}
			}
		}
	}

	switch {
	case len(ready) > 0:
		detail := TEnv(l, "env.hw.found", strings.Join(ready, sep))
		if len(compiled) > 0 {
			detail += "；" + TEnv(l, "env.hw.also", strings.Join(compiled, sep))
		}
		return CheckItem{Name: name, Status: envOK, Detail: detail}
	case len(compiled) > 0:
		return CheckItem{
			Name: name, Status: envOK,
			Detail: TEnv(l, "env.hw.compiled", strings.Join(compiled, sep)),
		}
	default:
		return CheckItem{Name: name, Status: envOK, Detail: TEnv(l, "env.hw.none")}
	}
}
