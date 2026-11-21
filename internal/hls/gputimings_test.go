package hls

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The GPU's conversion chains are not timed at startup while a timing
// of the last 7 days in which a chain converted is kept for the same GPU,
// driver and FFmpeg build: the best applies. A failed timing, an older
// one, or one of another GPU, driver or FFmpeg build, has them timed at
// once.
func TestTheGPUTimingIsKeptAWeekPerGPUDriverAndFFmpeg(t *testing.T) {
	t.Parallel()
	failed := map[string]float64{chainSDRMemory: 0, chainSDRGPU: 0, chainHDRMemory: 0, chainHDRGPU: 0}
	otherGPU, otherBuild := nvidiaKey, nvidiaKey
	otherGPU.GPU = "GeForce GTX 1060"
	otherBuild.FFmpeg = "n8.1 5e6f7a8b"
	otherDriver := testKey("570.10")(&Hardware{Method: "cuda", Encoders: []string{"h264_nvenc", "hevc_nvenc"}})
	for _, tc := range []struct {
		name  string
		kept  keptTiming
		timed bool
	}{
		{"kept 2 days ago", kept(nvidiaKey, 48*time.Hour, usualSpeeds), false},
		{"failed 2 days ago", kept(nvidiaKey, 48*time.Hour, failed), true},
		{"kept 8 days ago", kept(nvidiaKey, 8*24*time.Hour, usualSpeeds), true},
		{"of another GPU", kept(otherGPU, 48*time.Hour, usualSpeeds), true},
		{"of another driver", kept(otherDriver, 48*time.Hour, usualSpeeds), true},
		{"of another FFmpeg build", kept(otherBuild, 48*time.Hour, usualSpeeds), true},
		{"of the processor only", kept(nvidiaKey, 48*time.Hour, map[string]float64{chainProcessor: 1.7}), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _, path, log := timedManager(t, usual)
			keep(t, path, tc.kept)
			var runs atomic.Int32
			timed := m.run
			m.run = func(ctx context.Context, args []string) (time.Duration, error) {
				if !slices.Contains(args, "0.25") {
					runs.Add(1)
				}
				return timed(ctx, args)
			}
			if _, ok := m.DetectHardware("nvenc", ""); !ok {
				t.Fatal("no GPU")
			}
			m.measuring.Wait()
			got := keptIn(t, path)
			skipped := strings.Contains(log.String(),
				`msg="The GPU's conversion chains are not timed at startup: their best timings of the last 7 days apply" method=cuda sdr_chain=memory`)
			if !tc.timed {
				if runs.Load() != 0 || len(got) != 1 || !skipped {
					t.Errorf("%d runs, kept %+v, logged %s", runs.Load(), got, log)
				}
				if hw := m.Hardware(); hw.HDRSpeed != 1.3 || !hw.VulkanDecoding {
					t.Errorf("the best kept does not apply: %+v", hw)
				}
				return
			}
			if runs.Load() == 0 || skipped {
				t.Fatalf("%d runs, logged %s", runs.Load(), log)
			}
			if last := got[len(got)-1]; last.timingKey != nvidiaKey || !near(last.Speeds[chainHDRGPU], 1.3) {
				t.Errorf("kept %+v", got)
			}
		})
	}
}

// A timing at startup in which none of the GPU's chains converted is not
// kept: the next start times them again instead of trusting it.
func TestAFailedGPUTimingIsNotKept(t *testing.T) {
	t.Parallel()
	m, f, path, log := timedManager(t, chainTimes{})
	if _, ok := m.DetectHardware("nvenc", ""); !ok {
		t.Fatal("no GPU")
	}
	m.measuring.Wait()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("a failed timing was kept: %+v", keptIn(t, path))
	}
	if m.keptGPUTiming(m.Hardware()) {
		t.Error("a failed timing counts as kept")
	}
	// Once the GPU converts, the next start times it and keeps it.
	f.set(usual)
	m.measureLater("nvenc", m.Hardware())
	m.measuring.Wait()
	if got := keptIn(t, path); len(got) != 1 || !near(got[0].Speeds[chainHDRGPU], 1.3) {
		t.Errorf("kept %+v, logged %s", got, log)
	}
}
