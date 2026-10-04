package hls

import (
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"time"
)

// Hardware is a GPU FFmpeg decodes and encodes video on. Decoded frames
// come back to memory for the filters every conversion shares, scaling,
// tone mapping and burned subtitles, and go to the GPU again to be
// encoded: a GPU that cannot decode a codec leaves it to FFmpeg's own
// decoder, and the conversion looks the same either way.
type Hardware struct {
	// Method is cuda, for NVIDIA GPUs, or vaapi, for AMD and Intel ones.
	Method string
	// Device is the render node VAAPI opens.
	Device string
	// Encoders are those that encoded on it at startup, such as h264_nvenc.
	Encoders []string
	// ToneMapping is set when the GPU also scales video and tone maps HDR
	// to SDR, through libplacebo on its Vulkan driver, which applies Dolby
	// Vision's metadata: NVIDIA GPUs only. On AMD GPUs, the frames libplacebo
	// imports from memory set off a fault in the Linux driver.
	ToneMapping bool
}

// hardwareMethods are the GPU methods by preference, named as
// POLYFIN_HWACCEL names them, with the encoders Polyfin uses.
var hardwareMethods = []struct {
	name, method string
	encoders     []string
}{
	{"nvenc", "cuda", []string{"h264_nvenc", "hevc_nvenc"}},
	{"vaapi", "vaapi", []string{"h264_vaapi", "hevc_vaapi"}},
}

// DetectHardware chooses the GPU video is converted on, by encoding a few
// frames with each encoder: with want auto, the first of NVIDIA and VAAPI
// that encodes, VAAPI on device or else on each render node in turn; with
// nvenc or vaapi, that one only; with none, none. Call it before encoding
// starts. It reports false when no GPU encodes.
func (m *Manager) DetectHardware(want, device string) (Hardware, bool) {
	for _, candidate := range hardwareMethods {
		if want != "auto" && want != candidate.name {
			continue
		}
		devices := []string{""}
		if candidate.method == "vaapi" {
			devices = renderNodes(device)
		}
		for _, node := range devices {
			hw := Hardware{Method: candidate.method, Device: node}
			for _, encoder := range candidate.encoders {
				if slices.Contains(m.can.encoders, encoder) && m.encodes(hw, encoder) {
					hw.Encoders = append(hw.Encoders, encoder)
				}
			}
			if len(hw.Encoders) > 0 {
				hw.ToneMapping = hw.Method == "cuda" && slices.Contains(m.can.filters, "libplacebo") && m.toneMaps(hw)
				m.hardware = &hw
				return hw, true
			}
		}
	}
	return Hardware{}, false
}

// toneMaps reports whether hw tone maps a quarter of a second of test
// pattern, tagged as HDR10, on its Vulkan device: NVIDIA's Vulkan driver
// may be missing from the container, or fail.
func (m *Manager) toneMaps(hw Hardware) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	hw.ToneMapping = true
	v := VideoEncoding{Width: 160, Height: 90, ToneMap: true, Hardware: &hw}
	args := append([]string{"-hide_banner", "-nostdin", "-loglevel", "error"}, toneMappingDevices...)
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=24", "-t", "0.25",
		"-vf", "format=yuv420p10le,setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc,"+v.filters(),
		"-c:v", hw.Encoders[0], "-f", "null", "-")
	return exec.CommandContext(ctx, m.ffmpeg, args...).Run() == nil
}

// toneMappingDevices open the NVIDIA GPU for CUDA and its Vulkan device
// derived from it, for filters: libplacebo runs on the GPU that decodes and
// encodes, whichever other GPUs Vulkan sees.
var toneMappingDevices = []string{"-init_hw_device", "cuda=cu", "-init_hw_device", "vulkan=vk@cu", "-filter_hw_device", "vk"}

// Hardware is the GPU DetectHardware chose, nil for none.
func (m *Manager) Hardware() *Hardware {
	return m.hardware
}

// renderNodes are the render nodes VAAPI may open: device if set, else
// those the system has, in order.
func renderNodes(device string) []string {
	if device != "" {
		return []string{device}
	}
	nodes, _ := filepath.Glob("/dev/dri/renderD*")
	slices.Sort(nodes)
	return nodes
}

// encodes reports whether encoder encodes a quarter of a second of test
// pattern on hw. A GPU or driver that fails makes FFmpeg fail, or abort.
func (m *Manager) encodes(hw Hardware, encoder string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	args := append([]string{"-hide_banner", "-nostdin", "-loglevel", "error"}, hw.devices()...)
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=24", "-t", "0.25")
	if hw.Method == "vaapi" {
		args = append(args, "-vf", "format=nv12,hwupload")
	}
	args = append(args, "-c:v", encoder, "-f", "null", "-")
	return exec.CommandContext(ctx, m.ffmpeg, args...).Run() == nil
}

// devices are FFmpeg's options opening the GPU for filters: VAAPI's
// render node, which the upload before the encoder needs. NVENC opens
// the NVIDIA GPU itself.
func (hw Hardware) devices() []string {
	if hw.Method == "vaapi" {
		return []string{"-init_hw_device", "vaapi=va:" + hw.Device, "-filter_hw_device", "va"}
	}
	return nil
}

// inputs are FFmpeg's input options of a conversion on a GPU: decoding on
// it, into memory, and, to tone map there, the devices libplacebo needs.
func (v *VideoEncoding) inputs() []string {
	hw := v.Hardware
	switch {
	case hw == nil:
		return nil
	case hw.Method == "vaapi":
		return append(hw.devices(), "-hwaccel", "vaapi", "-hwaccel_device", "va")
	case v.toneMapsOnGPU():
		return append(slices.Clone(toneMappingDevices), "-hwaccel", "cuda", "-hwaccel_device", "cu")
	}
	return []string{"-hwaccel", "cuda"}
}

// toneMapsOnGPU reports whether the conversion tone maps HDR on its GPU.
func (v *VideoEncoding) toneMapsOnGPU() bool {
	return v.ToneMap && v.Hardware != nil && v.Hardware.ToneMapping
}

// output ends a filter chain: frames as the encoder takes them, uploaded
// to the GPU for VAAPI; NVENC and the software encoders take them from
// memory.
func (hw *Hardware) output() string {
	if hw != nil && hw.Method == "vaapi" {
		return "format=nv12,hwupload"
	}
	return "format=yuv420p"
}

// DecodeInputs are FFmpeg's input options decoding on hw, nil for none,
// into memory, for work that only decodes, such as thumbnails: a codec the
// GPU cannot decode is left to FFmpeg's own decoder.
func (hw *Hardware) DecodeInputs() []string {
	switch {
	case hw == nil:
		return nil
	case hw.Method == "vaapi":
		return append(hw.devices(), "-hwaccel", "vaapi", "-hwaccel_device", "va")
	}
	return []string{"-hwaccel", "cuda"}
}
