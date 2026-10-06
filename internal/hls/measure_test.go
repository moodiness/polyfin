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

// chainTimes are the injected durations of the timed conversions, by kind
// and chain; a missing one fails. checkFails fails the Vulkan decoding
// check, sampleFails the making of the samples.
type chainTimes struct {
	sdrMemory, sdrGPU, hdrMemory, hdrGPU time.Duration
	checkFails, sampleFails              bool
}

// fakeRunner stands in for FFmpeg: it answers each command line by what it
// does, and records them.
type fakeRunner struct {
	t     *testing.T
	times chainTimes
	mu    sync.Mutex
	runs  map[string][]string
}

func (f *fakeRunner) run(_ context.Context, args []string) (time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	failed := errors.New("failed")
	answer := func(kind string, d time.Duration) (time.Duration, error) {
		f.runs[kind] = args
		if d == 0 {
			return 0, failed
		}
		return d, nil
	}
	vf := ""
	if i := slices.Index(args, "-vf"); i >= 0 {
		vf = args[i+1]
	}
	switch {
	case strings.HasSuffix(args[len(args)-1], ".mkv"):
		kind := "sdr sample"
		if strings.Contains(vf, "smpte2084") {
			kind = "hdr sample"
		}
		f.runs[kind] = args
		if f.times.sampleFails {
			return 0, failed
		}
		return time.Second, nil
	case vf == "hwdownload,format=p010le":
		f.runs["check"] = args
		if f.times.checkFails {
			return 0, failed
		}
		return time.Second, nil
	case strings.Contains(vf, "libplacebo") && contains(args, "-hwaccel_output_format", "vulkan"):
		return answer("hdr gpu", f.times.hdrGPU)
	case strings.Contains(vf, "libplacebo"):
		return answer("hdr memory", f.times.hdrMemory)
	case strings.Contains(vf, "scale_cuda") || strings.Contains(vf, "scale_vaapi"):
		return answer("sdr gpu", f.times.sdrGPU)
	case strings.HasPrefix(vf, "scale=w=1920:h=1080"):
		return answer("sdr memory", f.times.sdrMemory)
	}
	f.t.Errorf("an unexpected command line: %q", args)
	return 0, failed
}

func measuringManager(t *testing.T, times chainTimes) (*Manager, *fakeRunner) {
	f := &fakeRunner{t: t, times: times, runs: map[string][]string{}}
	m := &Manager{dir: t.TempDir(), logger: slog.New(slog.DiscardHandler), detected: map[string]*Hardware{}, run: f.run,
		can: capabilities{encoders: []string{"libx264", "libx265", "h264_nvenc", "hevc_nvenc", "h264_vaapi"},
			filters: []string{"scale_cuda", "scale_vaapi", "hwupload", "yadif_cuda", "bwdif_cuda", "deinterlace_vaapi", "libplacebo"}}}
	return m, f
}

// Each kind of conversion keeps the chain that converted its sample
// faster: the memory path for SDR and Vulkan frames for HDR on the
// owner's GPU, and any other mix; a GPU chain that fails, or HDR frames
// Vulkan did not decode, keep the memory path.
func TestTheFasterChainIsKeptForEachKind(t *testing.T) {
	nvidia := Hardware{Method: "cuda", Encoders: []string{"h264_nvenc", "hevc_nvenc"}, ToneMapping: true}
	vaapi := Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi"}}
	for _, tc := range []struct {
		name             string
		hw               Hardware
		times            chainTimes
		resident, vulkan bool
		deinterlacers    []string
	}{
		{"SDR faster through memory, HDR on the GPU", nvidia,
			chainTimes{sdrMemory: 263 * time.Millisecond, sdrGPU: 625 * time.Millisecond, hdrMemory: 425 * time.Millisecond, hdrGPU: 253 * time.Millisecond},
			false, true, nil},
		{"both faster on the GPU", nvidia,
			chainTimes{sdrMemory: 600 * time.Millisecond, sdrGPU: 300 * time.Millisecond, hdrMemory: 600 * time.Millisecond, hdrGPU: 300 * time.Millisecond},
			true, true, []string{"yadif_cuda", "bwdif_cuda"}},
		{"both faster through memory", nvidia,
			chainTimes{sdrMemory: 300 * time.Millisecond, sdrGPU: 600 * time.Millisecond, hdrMemory: 300 * time.Millisecond, hdrGPU: 600 * time.Millisecond},
			false, false, nil},
		{"the GPU chains fail", nvidia, chainTimes{sdrMemory: time.Second, hdrMemory: time.Second}, false, false, nil},
		{"Vulkan does not decode", nvidia,
			chainTimes{sdrMemory: time.Second, sdrGPU: time.Second / 2, hdrMemory: time.Second, hdrGPU: time.Second / 2, checkFails: true},
			true, false, []string{"yadif_cuda", "bwdif_cuda"}},
		{"no sample", nvidia, chainTimes{sdrGPU: time.Millisecond, hdrGPU: time.Millisecond, sampleFails: true}, false, false, nil},
		{"VAAPI faster on the GPU", vaapi, chainTimes{sdrMemory: time.Second, sdrGPU: time.Second / 2}, true, false, []string{"deinterlace_vaapi"}},
		{"VAAPI faster through memory", vaapi, chainTimes{sdrMemory: time.Second / 2, sdrGPU: time.Second}, false, false, nil},
	} {
		m, f := measuringManager(t, tc.times)
		got := m.measure(t.Context(), tc.hw)
		if got.Resident != tc.resident || got.VulkanDecoding != tc.vulkan || !slices.Equal(got.Deinterlacers, tc.deinterlacers) {
			t.Errorf("%s: resident %v, Vulkan decoding %v, deinterlacers %v", tc.name, got.Resident, got.VulkanDecoding, got.Deinterlacers)
		}
		if tc.times.sampleFails {
			continue
		}
		// The samples are 2 s of 4K at 24 frames a second, written under
		// the segments' directory, SDR in H.264 on the GPU.
		sample := f.runs["sdr sample"]
		if !contains(sample, "testsrc2=size=3840x2160:rate=24", "-t", "2") || !strings.HasPrefix(sample[len(sample)-1], m.dir) ||
			!contains(sample, "-c:v", tc.hw.Encoders[0]) {
			t.Errorf("%s: the SDR sample %q", tc.name, sample)
		}
		// Each chain is the one conversions run.
		gpu, memory := tc.hw, tc.hw
		gpu.Resident = true
		resident := VideoEncoding{Encoder: tc.hw.Encoders[0], Width: 1920, Height: 1080, Hardware: &gpu}
		inMemory := VideoEncoding{Encoder: tc.hw.Encoders[0], Width: 1920, Height: 1080, Hardware: &memory}
		if !contains(f.runs["sdr gpu"], append(resident.inputs(), "-i")...) || !contains(f.runs["sdr gpu"], "-vf", resident.filters()) ||
			!contains(f.runs["sdr memory"], append(inMemory.inputs(), "-i")...) || !contains(f.runs["sdr memory"], "-vf", inMemory.filters()) {
			t.Errorf("%s: SDR chains %q and %q", tc.name, f.runs["sdr gpu"], f.runs["sdr memory"])
		}
		if !tc.hw.ToneMapping {
			if f.runs["hdr sample"] != nil {
				t.Errorf("%s: HDR timed on a GPU that does not tone map", tc.name)
			}
			continue
		}
		if !contains(f.runs["hdr sample"], "-c:v", "hevc_nvenc", "-profile:v", "main10") {
			t.Errorf("%s: the HDR sample %q", tc.name, f.runs["hdr sample"])
		}
		if tc.times.checkFails {
			if f.runs["hdr gpu"] != nil {
				t.Errorf("%s: HDR timed although Vulkan did not decode", tc.name)
			}
			continue
		}
		if !contains(f.runs["hdr gpu"], "-hwaccel", "vulkan") || contains(f.runs["hdr memory"], "-hwaccel", "vulkan") ||
			!contains(f.runs["hdr memory"], "-hwaccel", "cuda") {
			t.Errorf("%s: HDR chains %q and %q", tc.name, f.runs["hdr gpu"], f.runs["hdr memory"])
		}
	}
}

// A chain wins only when it converted, and faster than the other, or the
// other failed.
func TestTheGPUChainWinsOnlyWhenFaster(t *testing.T) {
	failed := errors.New("failed")
	for _, tc := range []struct {
		t    timing
		want bool
	}{
		{timing{memory: time.Second, gpu: time.Second / 2}, true},
		{timing{memory: time.Second / 2, gpu: time.Second}, false},
		{timing{memory: time.Second, gpu: time.Second}, false},
		{timing{memory: time.Second, gpuErr: failed}, false},
		{timing{memoryErr: failed, gpu: time.Second}, true},
		{timing{memoryErr: failed, gpuErr: failed}, false},
	} {
		if got := tc.t.gpuWins(); got != tc.want {
			t.Errorf("%+v: GPU wins %v", tc.t, got)
		}
	}
	if got := speed(250*time.Millisecond, nil); got != "8.0x" {
		t.Errorf("speed %s", got)
	}
}

// Conversions starting while the chains are timed go through memory; the
// chosen GPU then switches to what the timing found.
func TestConversionsGoThroughMemoryUntilTimed(t *testing.T) {
	m, f := measuringManager(t, chainTimes{sdrMemory: time.Second, sdrGPU: time.Second / 2, hdrMemory: time.Second, hdrGPU: time.Second / 2})
	release := make(chan struct{})
	timed := f.run
	m.run = func(ctx context.Context, args []string) (time.Duration, error) {
		<-release
		return timed(ctx, args)
	}
	m.done = make(chan struct{})
	hw := &Hardware{Method: "cuda", Encoders: []string{"h264_nvenc", "hevc_nvenc"}, ToneMapping: true}
	m.detected["auto"] = hw
	m.hardware.Store(hw)
	m.measureLater("auto", hw)
	if got := m.Hardware(); got.Resident || got.VulkanDecoding {
		t.Errorf("before the timing: %+v", got)
	}
	close(release)
	m.measuring.Wait()
	if got := m.Hardware(); !got.Resident || !got.VulkanDecoding || m.detected["auto"] != got {
		t.Errorf("after the timing: %+v", got)
	}
	// Closing stops a timing under way.
	m.hardware.Store(hw)
	m.run = func(ctx context.Context, _ []string) (time.Duration, error) {
		<-ctx.Done()
		return 0, ctx.Err()
	}
	m.measureLater("nvenc", hw)
	close(m.done)
	stopped := make(chan struct{})
	go func() { m.measuring.Wait(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("closing did not stop the timing")
	}
	if got := m.Hardware(); got.Resident {
		t.Errorf("a stopped timing chose %+v", got)
	}
}
