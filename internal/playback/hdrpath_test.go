package playback

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/media"
)

// The paths the timings at startup choose for HDR (see hls.HDRPath).
var (
	gpuKeepsUp       = hls.HDRPath{Reason: hls.HDRKeepsUp, GPUSpeed: 2}
	processorFaster  = hls.HDRPath{Processor: true, Height: 720, Reason: hls.HDRProcessorFaster, GPUSpeed: 0.5, LowSpeed: 0.8, ProcessorSpeed: 1.2}
	gpuKeepsUpLower  = hls.HDRPath{Height: 720, Reason: hls.HDRLowerHeight, GPUSpeed: 0.5, LowSpeed: 1.1, ProcessorSpeed: 0.4}
	noneKeepsUpOnGPU = hls.HDRPath{Height: 720, Reason: hls.HDRNoneKeepsUp, GPUSpeed: 0.5, LowSpeed: 0.8, ProcessorSpeed: 0.6}
)

// HDR a GPU would tone map follows the path its timings chose: on a GPU
// keeping up, as before; on the processor, at its own cap, when the GPU
// is slower than real time and the processor faster; on the GPU at 720p
// when only that keeps up. The GPU encodes either way, on every method
// that tone maps. Dolby Vision profile 5, which only NVIDIA's GPUs tone
// map, and HDR the processor cannot tone map stay on the GPU, at 720p.
func TestHDRFollowsThePathTheTimingsChose(t *testing.T) {
	nvidia := &hls.Hardware{Method: "cuda", Encoders: []string{"h264_nvenc"}, ToneMapping: true}
	intel := &hls.Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi"}, ToneMapping: true}
	qsv := &hls.Hardware{Method: "qsv", Encoders: []string{"h264_qsv"}, ToneMapping: true}
	videoToolbox := &hls.Hardware{Method: "videotoolbox", Encoders: []string{"h264_videotoolbox"}, ToneMapping: true}
	hdr10 := MediaStream{Codec: "hevc", VideoRange: "HDR", VideoRangeType: "HDR10", ColorTransfer: "smpte2084", Width: new(3840), Height: new(2160),
		MasteringDisplay: new(true)}
	profile5 := MediaStream{Codec: "hevc", VideoRange: "HDR", VideoRangeType: "DOVI", Width: new(3840), Height: new(2160)}
	const gpu, cpu = "gpu", "cpu"
	for _, test := range []struct {
		name      string
		hw        *hls.Hardware
		path      hls.HDRPath
		noFilters bool
		video     MediaStream
		want      string
		size      ConversionSize
		reason    string
	}{
		{"NVIDIA keeping up", nvidia, gpuKeepsUp, false, hdr10, gpu, ConversionSize{Reason: SizeSource}, ""},
		{"NVIDIA not timed", nvidia, hls.HDRPath{Reason: hls.HDRNotTimed}, false, hdr10, gpu, ConversionSize{Reason: SizeSource}, ""},
		{"NVIDIA slow, the processor faster", nvidia, processorFaster, false, hdr10, cpu, ConversionSize{Reason: SizeToneMapping, MaxHeight: 720},
			hls.ToneMapSlowGPU},
		{"NVIDIA slow, keeping up lower", nvidia, gpuKeepsUpLower, false, hdr10, gpu, ConversionSize{Reason: SizeGPUToneMapping, MaxHeight: 720},
			hls.ToneMapSlowGPU},
		{"NVIDIA, none keeping up, the GPU faster", nvidia, noneKeepsUpOnGPU, false, hdr10, gpu,
			ConversionSize{Reason: SizeGPUToneMapping, MaxHeight: 720}, hls.ToneMapSlowGPU},
		{"NVIDIA slow, Dolby Vision profile 5", nvidia, processorFaster, false, profile5, gpu,
			ConversionSize{Reason: SizeGPUToneMapping, MaxHeight: 720}, hls.ToneMapSlowGPU},
		{"NVIDIA slow, the processor without its filters", nvidia, processorFaster, true, hdr10, gpu,
			ConversionSize{Reason: SizeGPUToneMapping, MaxHeight: 720}, hls.ToneMapSlowGPU},
		{"Intel slow, the processor faster", intel, processorFaster, false, hdr10, cpu, ConversionSize{Reason: SizeToneMapping, MaxHeight: 720},
			hls.ToneMapSlowGPU},
		{"QSV slow, keeping up lower", qsv, gpuKeepsUpLower, false, hdr10, gpu, ConversionSize{Reason: SizeGPUToneMapping, MaxHeight: 720},
			hls.ToneMapSlowGPU},
		{"VideoToolbox slow, the processor faster", videoToolbox, processorFaster, false, hdr10, cpu,
			ConversionSize{Reason: SizeToneMapping, MaxHeight: 720}, hls.ToneMapSlowGPU},
		{"VideoToolbox keeping up", videoToolbox, gpuKeepsUp, false, hdr10, gpu, ConversionSize{Reason: SizeSource}, ""},
	} {
		can := Capabilities{Encoders: []string{"libx264"}, ToneMapping: !test.noFilters, Hardware: test.hw, HDR: test.path}
		got := ConvertVideo("h264", Limits{}, test.video, can)
		switch {
		case got == nil || !got.ToneMap || got.Hardware != test.hw || got.Encoder != test.hw.Encoders[0]:
			t.Errorf("%s: not encoded on the GPU: %+v", test.name, got)
		case got.ToneMapOnCPU != (test.want == cpu) || got.Size != test.size || got.ToneMapReason != test.reason:
			t.Errorf("%s: on the processor %v, %+v, %q", test.name, got.ToneMapOnCPU, got.Size, got.ToneMapReason)
		}
	}
	// Turned off in the settings, the processor tone maps whatever the
	// timings, and the reason says so.
	off := Capabilities{Encoders: []string{"libx264"}, ToneMapping: true, Hardware: nvidia, HDR: gpuKeepsUpLower, Tuning: Tuning{CPUToneMapping: true}}
	if got := ConvertVideo("h264", Limits{}, hdr10, off); got == nil || !got.ToneMapOnCPU || got.ToneMapReason != hls.ToneMapTurnedOff {
		t.Errorf("turned off: %+v", got)
	}
}

// The conversion's FFmpeg command line follows the path: libplacebo at 4K
// on a GPU keeping up, the processor's filters at 720p before the GPU's
// encoder, the GPU still decoding, when the processor is faster, and
// libplacebo at 720p when the GPU keeps up only there.
func TestFFmpegFollowsWhereHDRIsToneMapped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake FFmpeg is a shell script")
	}
	dir := t.TempDir()
	runs, script := filepath.Join(dir, "runs"), filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ncase \"$*\" in\n*-encoders*|*-filters*) exit 0 ;;\nesac\necho \"$*\" >> '"+runs+
		"'\necho 'no GPU here' >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	m, err := hls.NewManager(script, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	nvidia := &hls.Hardware{Method: "cuda", Encoders: []string{"h264_nvenc"}, ToneMapping: true}
	hdr := MediaStream{Codec: "hevc", VideoRange: "HDR", VideoRangeType: "HDR10", ColorTransfer: "smpte2084", Width: new(3840), Height: new(2160)}
	tuning := TuningOf(accounts.DefaultSettings())
	for _, tc := range []struct {
		name          string
		path          hls.HDRPath
		want, refused []string
	}{
		{"keeping up", gpuKeepsUp, []string{"libplacebo=w=3840:h=2160:", "-c:v h264_nvenc"}, []string{"zscale"}},
		{"the processor faster", processorFaster, []string{"-hwaccel cuda", "scale=w=1280:h=720,zscale=t=linear", "tonemap=tonemap=", "-c:v h264_nvenc"},
			[]string{"libplacebo", "vulkan"}},
		{"keeping up lower", gpuKeepsUpLower, []string{"libplacebo=w=1280:h=720:", "-c:v h264_nvenc"}, []string{"zscale"}},
	} {
		can := Capabilities{Encoders: []string{"libx264"}, ToneMapping: true, Hardware: nvidia, HDR: tc.path, Tuning: tuning}
		remux := Remux{Format: hls.FMP4, ConvertVideo: ConvertVideo("h264", Limits{}, hdr, can)}
		r := hls.Remux{Input: filepath.Join(dir, "hdr.mkv"), Video: 0, Audio: -1, Format: hls.FMP4,
			Plan: hls.NewPlan([]time.Duration{0}, 6*time.Second)}
		remux.convert(&r, media.Stream{Codec: "hevc", BitDepth: 10, AverageRate: 24}, tuning)
		_ = os.Remove(runs)
		open := func(context.Context) (hls.Remux, func(), error) { return r, func() {}, nil }
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		if f, err := m.Segment(ctx, hls.Key{Session: tc.name, Audio: -1, Format: hls.FMP4, Converts: true}, open, 0); err == nil {
			f.Close()
		}
		cancel()
		ran, _ := os.ReadFile(runs)
		line := string(ran)
		for _, part := range tc.want {
			if !strings.Contains(line, part) {
				t.Errorf("%s: no %q in %s", tc.name, part, line)
			}
		}
		for _, part := range tc.refused {
			if strings.Contains(line, part) {
				t.Errorf("%s: %q in %s", tc.name, part, line)
			}
		}
	}
}
