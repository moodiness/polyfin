package hls

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// at is how long the sample takes to convert at speed times real time.
func at(speed float64) time.Duration {
	return time.Duration(float64(sampleLength) / speed)
}

// The owner's NVIDIA GPU on a usual start, and on a start right after a
// nightly job: every chain slower, the SDR chain kept on the GPU reaching
// the time limit.
var (
	usual = chainTimes{sdrMemory: at(3.5), sdrGPU: at(0.5), hdrMemory: at(1.2), hdrGPU: at(1.3), processor: at(1.7)}
	busy  = chainTimes{sdrMemory: at(1.0), hdrMemory: at(0.3), hdrGPU: at(0.4), hdrLow: at(0.5), processor: at(0.8)}
)

// usualSpeeds are the usual timings, as kept.
var usualSpeeds = map[string]float64{chainSDRMemory: 3.5, chainSDRGPU: 0.5, chainHDRMemory: 1.2, chainHDRGPU: 1.3, chainProcessor: 1.7}

// testKey identifies the owner's GPU with driver, an NVIDIA Quadro P2000
// when there is one.
func testKey(driver string) func(*Hardware) timingKey {
	return func(hw *Hardware) timingKey {
		key := timingKey{Method: "none", FFmpeg: "n8.0 1a2b3c4d", Processors: 8}
		if hw != nil {
			key.Method, key.Encoders, key.GPU, key.Driver = hw.Method, strings.Join(hw.Encoders, ","), "Quadro P2000", driver
		}
		return key
	}
}

// nvidiaKey is what the timings of the owner's GPU hold for.
var nvidiaKey = testKey("580.95")(&Hardware{Method: "cuda", Encoders: []string{"h264_nvenc", "hevc_nvenc"}})

// timedManager is a Manager whose FFmpeg, the fake f, finds an NVIDIA GPU
// at once and times as times say, keeping the timings in the file at
// path, logging into log, and quiet once idle for 50 ms.
func timedManager(t *testing.T, times chainTimes) (*Manager, *fakeRunner, string, *syncBuffer) {
	t.Helper()
	m, f := measuringManager(t, times)
	log := &syncBuffer{}
	m.logger = slog.New(slog.NewTextHandler(log, nil))
	m.can.filters = append(m.can.filters, "zscale", "tonemap")
	m.done = make(chan struct{})
	m.retime.quiet, m.retime.poll = 50*time.Millisecond, 5*time.Millisecond
	timed := f.run
	m.run = func(ctx context.Context, args []string) (time.Duration, error) {
		// Detection encodes a quarter of a second with each encoder.
		if slices.Contains(args, "0.25") && !strings.Contains(strings.Join(args, " "), "hwdownload") {
			return time.Millisecond, nil
		}
		return timed(ctx, args)
	}
	path := filepath.Join(t.TempDir(), "gpu-timings.json")
	m.UseTimings(path)
	m.timings.identify = testKey("580.95")
	t.Cleanup(m.Close)
	return m, f, path, log
}

func (f *fakeRunner) set(times chainTimes) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.times = times
}

// keep writes the file at path with timings.
func keep(t *testing.T, path string, timings ...keptTiming) {
	t.Helper()
	data, err := json.Marshal(timingsFile{Version: timingsVersion, Timings: timings})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// kept is a timing for key that ended age ago, of speeds.
func kept(key timingKey, age time.Duration, speeds map[string]float64) keptTiming {
	return keptTiming{timingKey: key, At: time.Now().Add(-age), chainTimings: chainTimings{Speeds: speeds}}
}

// keptIn reads the timings of the file at path.
func keptIn(t *testing.T, path string) []keptTiming {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var file timingsFile
	if err := json.Unmarshal(data, &file); err != nil || file.Version != timingsVersion {
		t.Fatalf("the file: %v, version %d", err, file.Version)
	}
	return file.Timings
}

func near(a, b float64) bool {
	return math.Abs(a-b) < 0.01
}

// The best timings of the last 7 days apply as soon as the GPU is chosen,
// each chain's own; the timing at startup joins them.
func TestTheBestRecentTimingsApplyAtStart(t *testing.T) {
	t.Parallel()
	m, _, path, log := timedManager(t, usual)
	keep(t, path,
		kept(nvidiaKey, 48*time.Hour, map[string]float64{chainSDRMemory: 3.6, chainSDRGPU: 0.5, chainHDRMemory: 1.2, chainHDRGPU: 1.4, chainProcessor: 1.2}),
		kept(nvidiaKey, 24*time.Hour, map[string]float64{chainSDRMemory: 3.5, chainSDRGPU: 0.4, chainHDRMemory: 1.1, chainHDRGPU: 1.1, chainProcessor: 1.8}))
	release := make(chan struct{})
	timed := m.run
	m.run = func(ctx context.Context, args []string) (time.Duration, error) {
		if !slices.Contains(args, "0.25") {
			<-release
		}
		return timed(ctx, args)
	}
	m.SelectHardware("nvenc", "")
	hw := m.Hardware()
	if hw == nil || hw.Resident || !hw.VulkanDecoding || hw.HDRSpeed != 1.4 || m.ToneMappedHeight() != 1080 {
		t.Fatalf("before the timing: %+v at %dp", hw, m.ToneMappedHeight())
	}
	if path := m.HDRPath(hw); path.Reason != HDRKeepsUp || path.Processor || path.ProcessorSpeed != 1.8 {
		t.Errorf("HDR before the timing: %+v", path)
	}
	if !strings.Contains(log.String(), `msg="Applied the best recent timings" timings=2`) {
		t.Errorf("logged %s", log)
	}
	close(release)
	m.measuring.Wait()
	// The GPU's timing and the processor's are kept each on its own.
	if got := keptIn(t, path); len(got) != 4 || !near(got[2].Speeds[chainHDRGPU], 1.3) || !near(got[3].Speeds[chainProcessor], 1.7) {
		t.Errorf("kept %+v", got)
	}
	if hw := m.Hardware(); hw.HDRSpeed != 1.4 || !hw.VulkanDecoding || m.ToneMappedHeight() != 1080 {
		t.Errorf("after the timing: %+v at %dp", hw, m.ToneMappedHeight())
	}
}

// A timing at startup on a busy server changes nothing and is not kept:
// the best recent timings stay in force, and the GPU is timed again
// later, once Polyfin has been quiet a while.
func TestABusyTimingIsLeftOutAndTimedAgainWhenQuiet(t *testing.T) {
	t.Parallel()
	m, f, path, log := timedManager(t, busy)
	keep(t, path, kept(nvidiaKey, 24*time.Hour, usualSpeeds))
	m.retime.after = func() time.Duration { return 300 * time.Millisecond }
	// The test reads the file once written: on Windows, a read under way
	// makes the rename in its place fail, and the rename the read. It
	// looks at the GPU chosen once the timing made again is in force: the
	// GPU's timing goes in place before the processor's is kept.
	writes, applied := make(chan error, 8), make(chan struct{}, 1)
	m.timings.wrote = func(err error) { writes <- err }
	m.retime.applied = func() { applied <- struct{}{} }
	// A file converts meanwhile, which the timing at startup does not
	// wait for.
	m.mu.Lock()
	m.files = 1
	m.mu.Unlock()
	m.SelectHardware("nvenc", "")
	// The retry the busy timing schedules counts among the timings Close
	// waits for: the test waits for the processor's timing instead.
	eventually(t, "the timings at startup", m.processorTimed.Load)
	hw := m.Hardware()
	if hw.HDRSpeed != 1.3 || !hw.VulkanDecoding || m.HDRPath(hw).Reason != HDRKeepsUp || m.ToneMappedHeight() != 1080 {
		t.Errorf("after the busy timing: %+v at %dp", hw, m.ToneMappedHeight())
	}
	if got := keptIn(t, path); len(got) != 1 {
		t.Errorf("the busy timing was kept: %+v", got)
	}
	for _, want := range []string{`msg="Timed while the server was busy: kept the best recent timings" timed=gpu below="[hdr_gpu 0.4x (best 1.3x) hdr_memory 0.3x (best 1.2x) sdr_gpu failed (best 0.5x) sdr_memory 1.0x (best 3.5x)]"`,
		`timed=processor below="[processor 0.8x (best 1.7x)]"`, `msg="Timing again once the server is quiet"`} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("did not log %s: %s", want, log)
		}
	}
	// The timing made again waits for the file's conversion to end, then
	// for the quiet after it.
	f.set(chainTimes{sdrMemory: at(3.6), sdrGPU: at(0.5), hdrMemory: at(1.2), hdrGPU: at(1.6), processor: at(2.0)})
	time.Sleep(500 * time.Millisecond)
	if len(writes) != 0 {
		t.Fatalf("timed again while a file converts: %+v", keptIn(t, path))
	}
	quiet := time.Now()
	m.mu.Lock()
	m.files = 0
	m.mu.Unlock()
	select {
	case <-applied:
	case <-time.After(30 * time.Second):
		t.Fatal("the timing made again did not happen")
	}
	// It wrote the GPU's timing, then the processor's.
	for range 2 {
		if err := <-writes; err != nil {
			t.Fatalf("the timing made again was not kept: %v", err)
		}
	}
	if got := keptIn(t, path); len(got) != 3 {
		t.Fatalf("kept %+v", got)
	}
	// Polyfin looks at its work every poll: it may have seen the file's
	// conversion end up to a poll before the test let it.
	if got := keptIn(t, path)[1]; got.At.Before(quiet.Add(m.retime.quiet-2*m.retime.poll)) || !near(got.Speeds[chainHDRGPU], 1.6) {
		t.Errorf("timed again at %v, quiet from %v: %+v", got.At, quiet, got)
	}
	if hw := m.Hardware(); m.detected["nvenc"] != hw || !near(hw.HDRSpeed, 1.6) || !near(m.HDRPath(hw).ProcessorSpeed, 2.0) || len(writes) != 0 {
		t.Errorf("after the timing made again: %+v, %+v", hw, m.HDRPath(hw))
	}
}

// A timing made when quiet puts its path in place for the conversions
// starting afterwards; those under way keep the GPU as they found it.
func TestAQuietTimingUpdatesThePathConversionsUnderWayKeepTheirs(t *testing.T) {
	t.Parallel()
	slow := chainTimes{sdrMemory: at(3.5), sdrGPU: at(0.5), hdrMemory: at(0.7), hdrGPU: at(0.8), hdrLow: at(1.1), processor: at(0.9)}
	m, f, path, _ := timedManager(t, slow)
	m.SelectHardware("nvenc", "")
	m.measuring.Wait()
	// A conversion under way holds the GPU it started with.
	playing := m.Hardware()
	if path := m.HDRPath(playing); path.Reason != HDRLowerHeight || path.Height != 720 {
		t.Fatalf("at startup: %+v", path)
	}
	f.set(usual)
	if err := m.Retime(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	hw := m.Hardware()
	if hw == playing || m.detected["nvenc"] != hw || !near(hw.HDRSpeed, 1.3) || m.HDRPath(hw).Reason != HDRKeepsUp || m.ToneMappedHeight() != 1080 {
		t.Errorf("after the timing: %+v, %+v at %dp", hw, m.HDRPath(hw), m.ToneMappedHeight())
	}
	if !near(playing.HDRSpeed, 0.8) || !near(playing.LowHDRSpeed, 1.1) || !playing.VulkanDecoding {
		t.Errorf("the conversion under way's GPU changed: %+v", playing)
	}
	if got := keptIn(t, path); len(got) != 4 {
		t.Errorf("kept %+v", got)
	}
}

// Work starting during a timing made again stops it, and leaves nothing
// of it; the timing is made once Polyfin is quiet again.
func TestWorkStartingStopsATimingMadeAgain(t *testing.T) {
	t.Parallel()
	m, f, path, log := timedManager(t, usual)
	m.SelectHardware("nvenc", "")
	m.measuring.Wait()
	before := m.Hardware()
	f.set(chainTimes{sdrMemory: at(3.5), sdrGPU: at(0.5), hdrMemory: at(1.2), hdrGPU: at(1.6), processor: at(1.7)})
	started := make(chan struct{})
	var armed atomic.Bool
	armed.Store(true)
	timed := m.run
	m.run = func(ctx context.Context, args []string) (time.Duration, error) {
		if armed.CompareAndSwap(true, false) {
			close(started)
			<-ctx.Done()
			return 0, context.Cause(ctx)
		}
		return timed(ctx, args)
	}
	done := make(chan error, 1)
	go func() { done <- m.Retime(t.Context(), false) }()
	<-started
	m.mu.Lock()
	m.files = 1
	m.mu.Unlock()
	eventually(t, "the stop", func() bool {
		return strings.Contains(log.String(), `msg="Work started during the timing: it is tried again when the server is quiet"`)
	})
	if m.Hardware() != before || len(keptIn(t, path)) != 2 || strings.Count(log.String(), "Timed the GPU's conversion chains") != 2 {
		t.Errorf("the stopped timing went in place: %+v, %s", m.Hardware(), log)
	}
	m.mu.Lock()
	m.files = 0
	m.mu.Unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the timing was not made again")
	}
	if hw := m.Hardware(); !near(hw.HDRSpeed, 1.6) || len(keptIn(t, path)) != 4 {
		t.Errorf("after the timing made again: %+v", hw)
	}
}

// The timings kept count only for their GPU, driver and FFmpeg, within 7
// days; a missing, unreadable or foreign file counts as none, and the
// next timing writes it anew.
func TestTheTimingsFileHoldsForItsGPUWithinAWeek(t *testing.T) {
	t.Parallel()
	nvidia := Hardware{Method: "cuda", Encoders: []string{"h264_nvenc", "hevc_nvenc"}, ToneMapping: true}
	otherGPU := nvidiaKey
	otherGPU.GPU = "GeForce GTX 1060"
	for _, tc := range []struct {
		name    string
		file    func(t *testing.T, path string)
		applies bool
		after   int
	}{
		{"missing", func(*testing.T, string) {}, false, 1},
		{"corrupt", func(t *testing.T, path string) { _ = os.WriteFile(path, []byte("{nope"), 0o600) }, false, 1},
		{"of another version", func(t *testing.T, path string) {
			_ = os.WriteFile(path, []byte(`{"version":99,"timings":[]}`), 0o600)
		}, false, 1},
		{"another driver", func(t *testing.T, path string) {
			keep(t, path, kept(testKey("570.10")(&nvidia), time.Hour, usualSpeeds))
		}, false, 2},
		{"another GPU", func(t *testing.T, path string) { keep(t, path, kept(otherGPU, time.Hour, usualSpeeds)) }, false, 2},
		{"8 days old", func(t *testing.T, path string) { keep(t, path, kept(nvidiaKey, 8*24*time.Hour, usualSpeeds)) }, false, 1},
		{"6 days old", func(t *testing.T, path string) { keep(t, path, kept(nvidiaKey, 6*24*time.Hour, usualSpeeds)) }, true, 2},
	} {
		log := &syncBuffer{}
		m := &Manager{logger: slog.New(slog.NewTextHandler(log, nil))}
		path := filepath.Join(t.TempDir(), "gpu-timings.json")
		tc.file(t, path)
		m.UseTimings(path)
		m.timings.identify = testKey("580.95")
		if got := m.applyBest(&nvidia); (got.HDRSpeed == 1.3) != tc.applies || got.VulkanDecoding != tc.applies {
			t.Errorf("%s: applied %+v", tc.name, got)
		}
		if strings.Contains(log.String(), "The timings kept are ignored") != (tc.name == "corrupt" || tc.name == "of another version") {
			t.Errorf("%s: logged %s", tc.name, log)
		}
		m.timings.add(m.logger, nvidiaKey, chainTimings{Speeds: map[string]float64{chainHDRGPU: 1.2}}, time.Now())
		if got := keptIn(t, path); len(got) != tc.after || got[len(got)-1].timingKey != nvidiaKey {
			t.Errorf("%s: kept %+v", tc.name, got)
		}
		if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".*")); len(leftovers) != 0 {
			t.Errorf("%s: left %v", tc.name, leftovers)
		}
	}
}

// A timing that fails says why: FFmpeg's message, or that it reached the
// time limit.
func TestAFailedTimingSaysWhy(t *testing.T) {
	t.Parallel()
	limited, cancel := context.WithTimeout(t.Context(), time.Nanosecond)
	defer cancel()
	<-limited.Done()
	if err := runError(t.Context(), limited, errors.New("signal: killed"), nil); !errors.Is(err, errTimingLimit) || err.Error() != "FFmpeg reached the 20 s limit" {
		t.Errorf("at the limit: %v", err)
	}
	stopped, stop := context.WithCancel(t.Context())
	stop()
	if err := runError(stopped, stopped, errors.New("signal: killed"), nil); errors.Is(err, errTimingLimit) {
		t.Errorf("stopped: %v", err)
	}
	if err := runError(t.Context(), t.Context(), errors.New("exit status 1"), []byte(" No capable devices found\n")); err == nil ||
		err.Error() != "FFmpeg failed: exit status 1: No capable devices found" {
		t.Errorf("failed: %v", err)
	}

	m, _, _, log := timedManager(t, usual)
	timed := m.run
	m.run = func(ctx context.Context, args []string) (time.Duration, error) {
		vf := ""
		if i := slices.Index(args, "-vf"); i >= 0 {
			vf = args[i+1]
		}
		switch {
		case strings.Contains(vf, "scale_cuda"):
			return timingLimit, errTimingLimit
		case strings.Contains(vf, "zscale="):
			return time.Second, errors.New("FFmpeg failed: exit status 1: no path between colorspaces")
		}
		return timed(ctx, args)
	}
	m.SelectHardware("nvenc", "")
	m.measuring.Wait()
	for _, want := range []string{`kind=SDR chain=memory memory_speed=3.5x gpu_speed=failed gpu_error="FFmpeg reached the 20 s limit"`,
		`msg="Timed the processor's HDR tone mapping" encoder=h264_nvenc speed=failed height=720 error="FFmpeg failed: exit status 1: no path between colorspaces"`} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("did not log %s: %s", want, log)
		}
	}
	if strings.Contains(log.String(), "memory_error") {
		t.Errorf("a chain that converted has an error: %s", log)
	}
}
