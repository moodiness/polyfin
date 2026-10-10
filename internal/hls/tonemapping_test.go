package hls

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// recordingRunner stands in for FFmpeg: it records every command line and
// answers each as answer says.
type recordingRunner struct {
	mu     sync.Mutex
	runs   [][]string
	answer func(args []string) (time.Duration, error)
}

func (r *recordingRunner) run(_ context.Context, args []string) (time.Duration, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs = append(r.runs, args)
	return r.answer(args)
}

// ran reports whether a command line holding text ran.
func (r *recordingRunner) ran(text string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.ContainsFunc(r.runs, func(args []string) bool { return strings.Contains(strings.Join(args, " "), text) })
}

// count is how many command lines ran.
func (r *recordingRunner) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.runs)
}

// Only an Intel GPU, by its render node's PCI vendor, has its tone mapping
// tried: an AMD GPU never runs tonemap_vaapi nor anything with Vulkan. The
// check tone maps an x265 HDR10 sample carrying its mastering display, as
// tonemap_vaapi requires, decoded by VAAPI; without x265, or when the
// check fails, the Intel GPU leaves HDR to the processor.
func TestOnlyIntelGPUsTryVAAPIToneMapping(t *testing.T) {
	for _, tc := range []struct {
		name, vendor   string
		noX265, failed bool
		want, tried    bool
	}{
		{"Intel", "0x8086", false, false, true, true},
		{"Intel, the check failing", "0x8086", false, true, false, true},
		{"Intel without x265", "0x8086", true, false, false, false},
		{"AMD", "0x1002", false, false, false, false},
		{"an unknown vendor", "", false, false, false, false},
	} {
		r := &recordingRunner{answer: func(args []string) (time.Duration, error) {
			if tc.failed && slices.ContainsFunc(args, func(arg string) bool { return strings.Contains(arg, "tonemap_vaapi") }) {
				return 0, errors.New("failed")
			}
			return time.Second, nil
		}}
		encoders := []string{"h264_vaapi", "hevc_vaapi", "libx264", "libx265"}
		if tc.noX265 {
			encoders = encoders[:3]
		}
		var asked []string
		m := &Manager{dir: t.TempDir(), logger: slog.New(slog.DiscardHandler), detected: map[string]*Hardware{}, run: r.run,
			vendor: func(node string) string { asked = append(asked, node); return tc.vendor },
			can: capabilities{encoders: encoders, filters: []string{"hwupload", "hwdownload", "scale_vaapi", "tonemap_vaapi", "deinterlace_vaapi",
				"libplacebo", "zscale", "tonemap"}}}
		hw := m.detect("vaapi", "/dev/dri/renderD129")
		if hw == nil || hw.ToneMapping != tc.want || !slices.Equal(asked, []string{"/dev/dri/renderD129"}) {
			t.Errorf("%s: %+v, vendors asked of %v", tc.name, hw, asked)
			continue
		}
		if r.ran("tonemap_vaapi") != tc.tried || r.ran("master-display=") != tc.tried {
			t.Errorf("%s: tonemap_vaapi tried %v, want %v: %q", tc.name, r.ran("tonemap_vaapi"), tc.tried, r.runs)
		}
		if r.ran("vulkan") || r.ran("libplacebo") {
			t.Errorf("%s: Vulkan tried on a VAAPI GPU: %q", tc.name, r.runs)
		}
		if !tc.tried {
			continue
		}
		// The check runs the chain conversions use, decoded by VAAPI into
		// its memory, from the sample x265 wrote.
		i := slices.IndexFunc(r.runs, func(args []string) bool {
			return slices.ContainsFunc(args, func(a string) bool { return strings.Contains(a, "tonemap_vaapi") })
		})
		check := r.runs[i]
		if !contains(check, "-init_hw_device", "vaapi=va:/dev/dri/renderD129") || !contains(check, "-hwaccel_output_format", "vaapi") ||
			!contains(check, "-vf", "hwupload,scale_vaapi=w=160:h=90:format=p010,tonemap_vaapi=format=nv12:p=bt709:t=bt709:m=bt709") ||
			!contains(check, "-c:v", "h264_vaapi") || !strings.HasSuffix(check[slices.Index(check, "-i")+1], "hdr10.mkv") {
			t.Errorf("%s: the check %q", tc.name, check)
		}
	}
}

// An NVIDIA GPU keeps its libplacebo check, and never asks for a vendor.
func TestNVIDIAKeepsItsToneMappingCheck(t *testing.T) {
	r := &recordingRunner{answer: func([]string) (time.Duration, error) { return time.Second, nil }}
	m := &Manager{dir: t.TempDir(), logger: slog.New(slog.DiscardHandler), detected: map[string]*Hardware{}, run: r.run,
		vendor: func(string) string { t.Error("a vendor asked of an NVIDIA GPU"); return "" },
		can:    capabilities{encoders: []string{"h264_nvenc", "libx265"}, filters: []string{"libplacebo", "tonemap_vaapi", "scale_vaapi", "hwupload"}}}
	if hw := m.detect("nvenc", ""); hw == nil || !hw.ToneMapping || !r.ran("libplacebo") || r.ran("tonemap_vaapi") {
		t.Errorf("%+v: %q", hw, r.runs)
	}
}

// Automatic gives 1080p to HDR the processor tone maps when a 2 s 4K
// HDR10 sample converts to 1080p at 1.5 times real time or faster, and
// 720p when slower, when it fails, and before it ends. The sample and the
// conversion run as conversions do on the server: on the GPU's decoder
// and H.264 encoder when there is one, else in software.
func TestAutomaticToneMappedHeightFollowsTheTiming(t *testing.T) {
	failed := errors.New("failed")
	vaapi := &Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi", "hevc_vaapi"}}
	nvidia := &Hardware{Method: "cuda", Encoders: []string{"h264_nvenc", "hevc_nvenc"}, ToneMapping: true}
	for _, tc := range []struct {
		name       string
		hw         *Hardware
		conversion time.Duration
		convErr    error
		sampleErr  error
		want       int
	}{
		{"fast enough", nil, 1200 * time.Millisecond, nil, nil, 1080},
		{"exactly 1.5x", nil, 2 * time.Second * 2 / 3, nil, nil, 1080},
		{"too slow", nil, 1400 * time.Millisecond, nil, nil, 720},
		{"the conversion fails", nil, time.Millisecond, failed, nil, 720},
		{"the sample fails", nil, time.Millisecond, nil, failed, 720},
		{"VAAPI, fast enough", vaapi, time.Second, nil, nil, 1080},
		{"NVIDIA, on the processor all the same", nvidia, time.Second, nil, nil, 1080},
	} {
		r := &recordingRunner{answer: func(args []string) (time.Duration, error) {
			if strings.HasSuffix(args[len(args)-1], ".mkv") {
				return 5 * time.Second, tc.sampleErr
			}
			return tc.conversion, tc.convErr
		}}
		m := &Manager{dir: t.TempDir(), logger: slog.New(slog.DiscardHandler), detected: map[string]*Hardware{}, run: r.run,
			can: capabilities{encoders: []string{"libx264", "libx265"}, filters: []string{"zscale", "tonemap"}}}
		if got := m.timeToneMapping(t.Context(), tc.hw); got != tc.want {
			t.Errorf("%s: %dp, want %dp", tc.name, got, tc.want)
		}
		if tc.sampleErr != nil {
			if len(r.runs) != 1 {
				t.Errorf("%s: converted without a sample: %q", tc.name, r.runs)
			}
			continue
		}
		sample, conversion := r.runs[0], r.runs[1]
		if !contains(sample, "testsrc2=size=3840x2160:rate=24", "-t", "2") || !strings.Contains(strings.Join(sample, " "), "smpte2084") {
			t.Errorf("%s: the sample %q", tc.name, sample)
		}
		filters := conversion[slices.Index(conversion, "-vf")+1]
		if !strings.Contains(filters, "scale=w=1920:h=1080,zscale=t=linear") || strings.Contains(filters, "libplacebo") || strings.Contains(filters, "tonemap_vaapi") {
			t.Errorf("%s: the conversion's filters %q", tc.name, filters)
		}
		switch tc.hw {
		case nil:
			if !contains(sample, "-c:v", "libx265", "-preset", "ultrafast") || !contains(conversion, "-c:v", "libx264", "-preset", "veryfast") ||
				slices.Contains(conversion, "-hwaccel") {
				t.Errorf("%s: in software %q, %q", tc.name, sample, conversion)
			}
		case vaapi:
			if !contains(sample, "-c:v", "hevc_vaapi", "-profile:v", "main10") || !contains(conversion, "-hwaccel", "vaapi") ||
				!contains(conversion, "-c:v", "h264_vaapi") || !strings.HasSuffix(filters, "format=nv12,hwupload") {
				t.Errorf("%s: on VAAPI %q, %q", tc.name, sample, conversion)
			}
		case nvidia:
			if !contains(sample, "-c:v", "hevc_nvenc") || !contains(conversion, "-hwaccel", "cuda") || !contains(conversion, "-c:v", "h264_nvenc") {
				t.Errorf("%s: on NVIDIA %q, %q", tc.name, sample, conversion)
			}
		}
	}
}

// A GPU that encodes HEVC but refuses 10 bits leaves the sample to x265,
// and the processor is timed all the same.
func TestTheHDRSampleFallsBackToX265(t *testing.T) {
	vaapi := &Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi", "hevc_vaapi"}}
	r := &recordingRunner{answer: func(args []string) (time.Duration, error) {
		if slices.Contains(args, "hevc_vaapi") {
			return 0, errors.New("main10 refused")
		}
		return time.Second, nil
	}}
	m := &Manager{dir: t.TempDir(), logger: slog.New(slog.DiscardHandler), detected: map[string]*Hardware{}, run: r.run,
		can: capabilities{encoders: []string{"libx264", "libx265"}, filters: []string{"zscale", "tonemap"}}}
	if got := m.timeToneMapping(t.Context(), vaapi); got != 1080 {
		t.Errorf("%dp, want 1080p", got)
	}
	if len(r.runs) != 3 || !contains(r.runs[1], "-c:v", "libx265") || !contains(r.runs[2], "-c:v", "h264_vaapi") {
		t.Errorf("runs: %q", r.runs)
	}
}

// The height is 720p until the timing, which waits for the GPU's own,
// ends; then the one it chose. Without the processor's filters, nothing
// is timed.
func TestAutomaticToneMappedHeightIs720pUntilTimed(t *testing.T) {
	r := &recordingRunner{answer: func([]string) (time.Duration, error) { return time.Second, nil }}
	m := &Manager{dir: t.TempDir(), logger: slog.New(slog.DiscardHandler), detected: map[string]*Hardware{}, done: make(chan struct{}), run: r.run,
		can: capabilities{encoders: []string{"libx264", "libx265"}, filters: []string{"zscale", "tonemap"}}}
	m.timingGPU.Add(1)
	m.timeToneMappingLater()
	time.Sleep(20 * time.Millisecond)
	if got := m.ToneMappedHeight(); got != 720 || r.count() != 0 {
		t.Errorf("while the GPU's chains are timed: %dp, %d runs", got, r.count())
	}
	m.timingGPU.Done()
	m.measuring.Wait()
	if got := m.ToneMappedHeight(); got != 1080 || r.count() != 2 {
		t.Errorf("after the timing: %dp, %d runs", got, r.count())
	}
	without := &Manager{dir: t.TempDir(), logger: slog.New(slog.DiscardHandler), run: r.run, can: capabilities{encoders: []string{"libx264"}}}
	without.timeToneMappingLater()
	without.measuring.Wait()
	if got := without.ToneMappedHeight(); got != 720 || r.count() != 2 {
		t.Errorf("without zscale: %dp, %d runs", got, r.count())
	}
}

// An Intel GPU that tone maps keeps deinterlace_vaapi for HDR even when
// its SDR frames go through memory, and times no Vulkan chain.
func TestIntelToneMappingKeepsItsDeinterlacer(t *testing.T) {
	m, f := measuringManager(t, chainTimes{sdrMemory: time.Second / 2, sdrGPU: time.Second})
	intel := Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi"}, ToneMapping: true}
	got := m.measure(t.Context(), intel)
	if got.Resident || got.VulkanDecoding || !slices.Equal(got.Deinterlacers, []string{"deinterlace_vaapi"}) {
		t.Errorf("%+v", got)
	}
	if f.runs["hdr sample"] != nil || f.runs["check"] != nil {
		t.Errorf("Vulkan timed on an Intel GPU: %q", f.runs)
	}
}
