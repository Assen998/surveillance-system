package api

import (
	"sort"
	"strings"

	"github.com/yourorg/surveillance-system/pkg/hwcodec"
)

// hwCodecCheck 检测硬件编解码支持（信息性检查，不影响通过/失败结论）。
// 检测逻辑在 pkg/hwcodec（与系统设置页的硬件开关共用）。
func (s *Server) hwCodecCheck(l string) CheckItem {
	name := TEnv(l, "env.hw.name")
	sep := ", "
	if l == localeZH {
		sep = "、"
	}

	report := hwcodec.Detect()

	featDesc := func(f hwcodec.Feature) string {
		desc := f.Name
		if len(f.Encode) > 0 {
			desc += " " + TEnv(l, "env.hw.encode") + " " + strings.Join(f.Encode, "/")
		}
		if len(f.Decode) > 0 {
			desc += " " + TEnv(l, "env.hw.decode") + " " + strings.Join(f.Decode, "/")
		}
		return desc
	}

	ready, compiled := []string{}, []string{}
	for _, f := range report.Features {
		if f.DeviceOK() {
			ready = append(ready, featDesc(f))
		} else {
			compiled = append(compiled, featDesc(f))
		}
	}
	// ready 按能力优先级展示
	prio := map[string]int{}
	for i, p := range []string{
		"NVIDIA NVENC", "NVIDIA CUVID", "VAAPI", "Intel QSV", "VideoToolbox",
		"D3D11VA", "DXVA2", "V4L2 M2M", "Rockchip RKMPP", "AMD AMF",
		"Raspberry Pi MMAL", "Windows MF",
	} {
		prio[p] = i
	}
	sortByPrio := func(list []string) {
		for i := 0; i < len(list); i++ {
			for j := i + 1; j < len(list); j++ {
				pi, pj := prio[strings.Split(list[i], " ")[0]], prio[strings.Split(list[j], " ")[0]]
				if pj < pi {
					list[i], list[j] = list[j], list[i]
				}
			}
		}
	}
	sortByPrio(ready)
	sort.Strings(compiled)

	// 设备存在但真实自检未通过 → warn（.231 Amlogic 场景：节点在但驱动/ffmpeg 不兼容）
	if report.Decode.Reason == hwcodec.ReasonVerifyFailed || report.Encode.Reason == hwcodec.ReasonVerifyFailed {
		parts := []string{}
		compiled := []string{}
		for _, c := range report.Features {
			if c.DeviceOK() {
				continue
			}
			desc := featDesc(c)
			if report.Decode.Reason == hwcodec.ReasonVerifyFailed && c.Name == report.Decode.Feature {
				continue
			}
			if report.Encode.Reason == hwcodec.ReasonVerifyFailed && c.Name == report.Encode.Feature {
				continue
			}
			compiled = append(compiled, desc)
		}
		if report.Decode.Reason == hwcodec.ReasonVerifyFailed {
			parts = append(parts, TEnv(l, "env.hw.verifyFailed", report.Decode.Feature))
		}
		if report.Encode.Reason == hwcodec.ReasonVerifyFailed {
			parts = append(parts, TEnv(l, "env.hw.verifyFailed", report.Encode.Feature))
		}
		detail := strings.Join(parts, "；")
		if l != localeZH {
			detail = strings.Join(parts, "; ")
		}
		if len(compiled) > 0 {
			detail += "；" + TEnv(l, "env.hw.also", strings.Join(compiled, sep))
		}
		return CheckItem{
			Name: name, Status: envWarn,
			Detail: detail,
			Fix:    TEnv(l, "env.hw.verifyFix"),
		}
	}

	// 有 NVIDIA GPU 但 ffmpeg 不含其硬件编解码 → warn（卡买了但用不上）
	if report.Decode.Reason == hwcodec.ReasonNvidiaNoCUDA || report.Encode.Reason == hwcodec.ReasonNvidiaNoCUDA {
		return CheckItem{
			Name: name, Status: envWarn,
			Detail: TEnv(l, "env.hw.nvidiaMissing"),
			Fix:    TEnv(l, "env.hw.nvidiaFix"),
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
