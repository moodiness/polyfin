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
				m.hardware = &hw
				return hw, true
			}
		}
	}
	return Hardware{}, false
}

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

// inputs are FFmpeg's input options decoding video on the GPU, into memory.
func (hw Hardware) inputs() []string {
	if hw.Method == "vaapi" {
		return append(hw.devices(), "-hwaccel", "vaapi", "-hwaccel_device", "va")
	}
	return []string{"-hwaccel", "cuda"}
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
