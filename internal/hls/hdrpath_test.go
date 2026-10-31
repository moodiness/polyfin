package hls

import (
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"
)

// A GPU whose HDR chain converts its sample slower than real time has it
// timed again to 720p, through the chain conversions take: libplacebo on
// Vulkan frames on NVIDIA's GPU, the device's own tone mapping on
// VideoToolbox and Intel's GPU, from an x265 sample carrying its
// mastering display. A GPU keeping up is timed once.
func TestASlowGPUHDRChainIsTimedLower(t *testing.T) {
	nvidia := Hardware{Method: "cuda", Encoders: []string{"h264_nvenc", "hevc_nvenc"}, ToneMapping: true}
	videoToolbox := Hardware{Method: "videotoolbox", Encoders: []string{"h264_videotoolbox", "hevc_videotoolbox"}, ToneMapping: true}
	intel := Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi"}, ToneMapping: true}
	for _, tc := range []struct {
		name      string
		hw        Hardware
		times     chainTimes
		speed     float64
		low       float64
		lowFilter string
	}{
		{"NVIDIA, slow", nvidia, chainTimes{hdrMemory: 5 * time.Second, hdrGPU: 4 * time.Second, hdrLow: 1600 * time.Millisecond}, 0.5, 1.25,
			"libplacebo=w=1280:h=720:"},
		{"NVIDIA, slow through memory", nvidia,
			chainTimes{hdrMemory: 4 * time.Second, hdrGPU: 5 * time.Second, hdrLow: 2500 * time.Millisecond}, 0.5, 0.8, "libplacebo=w=1280:h=720:"},
		{"NVIDIA, Vulkan not decoding", nvidia, chainTimes{hdrMemory: 4 * time.Second, checkFails: true, hdrLow: time.Second}, 0.5, 2,
			"libplacebo=w=1280:h=720:"},
		{"NVIDIA, fast", nvidia, chainTimes{hdrMemory: 600 * time.Millisecond, hdrGPU: 250 * time.Millisecond}, 8, 0, ""},
		{"VideoToolbox, slow", videoToolbox, chainTimes{hdrDevice: 2500 * time.Millisecond, hdrLow: 1600 * time.Millisecond}, 0.8, 1.25,
			"scale_vt=w=1280:h=720:color_matrix=bt709"},
		{"Intel, slow", intel, chainTimes{hdrDevice: 4 * time.Second, hdrLow: 2500 * time.Millisecond}, 0.5, 0.8,
			"scale_vaapi=w=1280:h=720:format=p010,tonemap_vaapi"},
		{"Intel, fast", intel, chainTimes{hdrDevice: time.Second}, 2, 0, ""},
		{"Intel, failing", intel, chainTimes{}, 0, 0, ""},
	} {
		m, f := measuringManager(t, tc.times)
		got := m.measure(t.Context(), tc.hw)
		if math.Abs(got.HDRSpeed-tc.speed) > 0.01 || math.Abs(got.LowHDRSpeed-tc.low) > 0.01 {
			t.Errorf("%s: HDR at %.2fx, %.2fx lower, want %.2fx and %.2fx", tc.name, got.HDRSpeed, got.LowHDRSpeed, tc.speed, tc.low)
		}
		low := f.runs["hdr low"]
		if tc.lowFilter == "" {
			if low != nil {
				t.Errorf("%s: timed lower: %q", tc.name, low)
			}
			continue
		}
		// The lower timing takes the chain conversions take.
		v := VideoEncoding{ToneMap: true, Hardware: &got}
		if !contains(low, append(v.inputs(), "-i")...) || !strings.Contains(low[len(low)-6], tc.lowFilter) {
			t.Errorf("%s: the lower timing %q", tc.name, low)
		}
		if tc.hw.Method != "cuda" && !strings.Contains(strings.Join(f.runs["hdr sample"], " "), "master-display=") {
			t.Errorf("%s: the sample %q", tc.name, f.runs["hdr sample"])
		}
	}
}

// HDR goes where the timings say: on a GPU keeping up, or not timed, as
// before; from a slower GPU to the processor when the processor keeps up,
// else to the GPU at 720p when it keeps up there, else to the faster of
// the two, the GPU then at 720p.
func TestWhereHDRGoesFollowsTheTimings(t *testing.T) {
	for _, tc := range []struct {
		name                string
		gpu, low, processor float64
		onProcessor         bool
		height              int
		reason              string
	}{
		{"a fast GPU", 2, 0, 1.2, false, 0, HDRKeepsUp},
		{"a GPU at real time", 1, 0, 3, false, 0, HDRKeepsUp},
		{"not timed", 0, 0, 1.2, false, 0, HDRNotTimed},
		{"a slow GPU, the processor faster", 0.5, 1.1, 1.2, true, 720, HDRProcessorFaster},
		{"a slow GPU, the processor at real time", 0.5, 0.8, 1, true, 720, HDRProcessorFaster},
		{"both slow, the GPU keeping up lower", 0.5, 1.1, 0.4, false, 720, HDRLowerHeight},
		{"both slow, the processor not timed", 0.5, 1.1, 0, false, 720, HDRLowerHeight},
		{"none keeping up, the processor faster", 0.5, 0.8, 0.9, true, 720, HDRNoneKeepsUp},
		{"none keeping up, the GPU faster lower", 0.5, 0.8, 0.6, false, 720, HDRNoneKeepsUp},
	} {
		got := chooseHDRPath(tc.gpu, tc.low, tc.processor)
		if got.Processor != tc.onProcessor || got.Height != tc.height || got.Reason != tc.reason {
			t.Errorf("%s: %+v", tc.name, got)
		}
	}
	m := &Manager{}
	if got := m.HDRPath(nil); got != (HDRPath{}) {
		t.Errorf("no GPU: %+v", got)
	}
	if got := m.HDRPath(&Hardware{Method: "vaapi", HDRSpeed: 0.5}); got != (HDRPath{}) {
		t.Errorf("a GPU that does not tone map: %+v", got)
	}
}

// The reporter's NVIDIA GPU: its HDR chain timed at 0.5x, then 0.8x at
// 720p, and the processor's tone mapping at 1.25x once the GPU is timed.
// HDR then goes to the processor, the GPU still encoding.
func TestASlowGPUHandsHDRToAFasterProcessor(t *testing.T) {
	r := &recordingRunner{answer: func(args []string) (time.Duration, error) {
		if strings.HasSuffix(args[len(args)-1], ".mkv") {
			return 5 * time.Second, nil
		}
		return 1600 * time.Millisecond, nil
	}}
	m := &Manager{dir: t.TempDir(), done: make(chan struct{}), logger: slog.New(slog.DiscardHandler), detected: map[string]*Hardware{}, run: r.run,
		can: capabilities{encoders: []string{"libx264", "libx265"}, filters: []string{"zscale", "tonemap"}}}
	hw := &Hardware{Method: "cuda", Encoders: []string{"h264_nvenc", "hevc_nvenc"}, ToneMapping: true, HDRSpeed: 0.5, LowHDRSpeed: 0.8}
	m.hardware.Store(hw)
	if got := m.HDRPath(hw); got.Processor || got.Height != 720 || got.Reason != HDRNoneKeepsUp {
		t.Errorf("before the processor is timed: %+v", got)
	}
	m.timeToneMappingLater()
	m.measuring.Wait()
	got := m.HDRPath(m.Hardware())
	if !got.Processor || got.Reason != HDRProcessorFaster || math.Abs(got.ProcessorSpeed-1.25) > 0.01 || m.ToneMappedHeight() != 720 {
		t.Errorf("after: %+v at %dp", got, m.ToneMappedHeight())
	}
	if !r.ran("-c:v h264_nvenc") {
		t.Errorf("the processor timed without the GPU's encoder: %q", r.runs)
	}
}
