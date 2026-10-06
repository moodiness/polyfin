package hls

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// pacedSource serves path at rate bytes a second, or its first stallAt
// bytes then nothing more until the test ends when stallAt is positive.
func pacedSource(t *testing.T, path string, rate, stallAt int) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ended := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Type", "video/x-matroska")
		start := 0
		if spec, ok := strings.CutPrefix(r.Header.Get("Range"), "bytes="); ok {
			from, _, _ := strings.Cut(spec, "-")
			start, _ = strconv.Atoi(from)
		}
		w.Header().Set("Content-Range", "bytes "+strconv.Itoa(start)+"-"+strconv.Itoa(len(data)-1)+"/"+strconv.Itoa(len(data)))
		w.Header().Set("Content-Length", strconv.Itoa(len(data)-start))
		w.WriteHeader(http.StatusPartialContent)
		for at := start; at < len(data); {
			if stallAt > 0 && at >= stallAt {
				select {
				case <-r.Context().Done():
				case <-ended:
				}
				return
			}
			n := min(4096, len(data)-at)
			if _, err := w.Write(data[at : at+n]); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			at += n
			if rate > 0 {
				time.Sleep(time.Duration(n) * time.Second / time.Duration(rate))
			}
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(ended) })
	return server
}

// twoSecondKeyframes writes a 30 s video with a keyframe every 2 s: its
// segments start at 0, 2, 6, 10, 14 s and so on.
func twoSecondKeyframes(t *testing.T, ffmpeg string) (string, Plan) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "steady.mkv")
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-t", "30", "-c:v", "libx264", "-preset", "ultrafast", "-g", "48", "-keyint_min", "48", path).CombinedOutput(); err != nil {
		t.Fatalf("make the source: %v: %s", err, out)
	}
	var keyframes []time.Duration
	for at := time.Duration(0); at < 30*time.Second; at += 2 * time.Second {
		keyframes = append(keyframes, at)
	}
	return path, NewPlan(keyframes, 30*time.Second)
}

// A segment whose FFmpeg makes no progress, its source answering nothing
// more, is answered ErrStalled once the stall timeout passed, rather than
// waited for until the player gives up; a slow source that keeps FFmpeg
// going is waited for past it.
func TestStalledSegmentsAreAnsweredWithinTheTimeout(t *testing.T) {
	ffmpeg, _ := tools(t)
	path, plan := twoSecondKeyframes(t, ffmpeg)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	// Stopped before the sources close, which wait for FFmpeg's requests.
	defer m.Close()
	m.stall = 1500 * time.Millisecond

	stalled := pacedSource(t, path, 0, 16<<10)
	open := func(context.Context) (Remux, func(), error) {
		return Remux{Input: stalled.URL + "/steady.mkv", Video: 0, Audio: -1, Format: FMP4, Plan: plan}, func() {}, nil
	}
	started := time.Now()
	_, err = m.Segment(t.Context(), Key{Session: "stalled", Audio: -1, Format: FMP4}, open, 0)
	if waited := time.Since(started); !errors.Is(err, ErrStalled) || waited < m.stall || waited > m.stall+3*time.Second {
		t.Errorf("a stalled source: %v after %v", err, waited)
	}

	// Paced at twice the video's own rate: segment 3, which ends at 14 s,
	// takes about 7 s to come.
	slow := pacedSource(t, path, int(info.Size()/15), 0)
	open = func(context.Context) (Remux, func(), error) {
		return Remux{Input: slow.URL + "/steady.mkv", Video: 0, Audio: -1, Format: FMP4, Plan: plan}, func() {}, nil
	}
	key := Key{Session: "slow", Audio: -1, Format: FMP4}
	if f, err := m.Segment(t.Context(), key, open, 0); err != nil {
		t.Fatal(err)
	} else {
		_ = f.Close()
	}
	started = time.Now()
	f, err := m.Segment(t.Context(), key, open, 3)
	if err != nil {
		t.Fatalf("a slow source that progresses: %v after %v", err, time.Since(started))
	}
	_ = f.Close()
	if waited := time.Since(started); waited < m.stall {
		t.Logf("segment 3 came in %v: the source was not slow enough to test the wait", waited)
	}
}

// An input that failed for good ends its encoding at once: the requests
// waiting get the failure, and the next one opens the encoding again.
func TestAFailedInputEndsItsEncoding(t *testing.T) {
	ffmpeg, _ := tools(t)
	path, plan := twoSecondKeyframes(t, ffmpeg)
	m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	stalled := pacedSource(t, path, 0, 16<<10)
	failure := errors.New("the host answers another file")
	var opened atomic.Int32
	failed := make(chan struct{})
	released := make(chan struct{}, 2)
	open := func(context.Context) (Remux, func(), error) {
		opened.Add(1)
		return Remux{Input: stalled.URL + "/steady.mkv", Video: 0, Audio: -1, Format: FMP4, Plan: plan,
			Failed: failed, Failure: func() error { return failure }}, func() { released <- struct{}{} }, nil
	}
	key := Key{Session: "failing", Audio: -1, Format: FMP4}
	time.AfterFunc(500*time.Millisecond, func() { close(failed) })
	started := time.Now()
	if _, err := m.Segment(t.Context(), key, open, 0); !errors.Is(err, failure) || time.Since(started) > 5*time.Second {
		t.Errorf("waiting while the input failed: %v after %v", err, time.Since(started))
	}
	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Error("the input of the failed encoding was not released")
	}
	m.mu.Lock()
	_, kept := m.encodings[key]
	m.mu.Unlock()
	if kept {
		t.Error("the failed encoding is kept")
	}
	// The next request opens it again, and learns of the failure from its
	// opener at once.
	if _, err := m.Segment(t.Context(), key, open, 0); !errors.Is(err, failure) || opened.Load() != 2 {
		t.Errorf("the next request: %v, %d opens", err, opened.Load())
	}
}

// FFmpeg's progress counts when what it wrote, its frames or its output
// time move, not when it reports the same again.
func TestProgressCountsWhatMoves(t *testing.T) {
	report := func(frame, size, at string) string {
		return "frame=" + frame + "\nfps=0.0\nbitrate=N/A\ntotal_size=" + size + "\nout_time_us=" + at + "\nspeed=N/A\nprogress=continue\n"
	}
	input := report("0", "0", "0") + report("0", "0", "0") + report("24", "0", "1000000") + report("24", "0", "1000000") +
		report("24", "4096", "1000000") + report("48", "4096", "2000000") + strings.ReplaceAll(report("48", "4096", "2000000"), "continue", "end")
	var calls int
	watchProgress(strings.NewReader(input), func() { calls++ })
	// The first report, then three that moved.
	if calls != 4 {
		t.Errorf("%d progresses, want 4", calls)
	}
}

// Jobs past the first segment probe 2 MB of the file, which the analysis
// left in the cache, rather than 5; burned subtitles and MPEG-TS files,
// cut on a grid, probe as usual.
func TestLaterJobsProbeLess(t *testing.T) {
	plan := NewPlan(seconds(0, 2, 6, 10), 14*time.Second)
	burned := 3
	probes := func(r Remux, n int) bool {
		args := r.args(n)
		return contains(args, "-probesize", "2M", "-analyzeduration", "1M") && slices.Index(args, "-probesize") < slices.Index(args, "-i")
	}
	copied := Remux{Input: "in", Audio: -1, Format: FMP4, Plan: plan}
	converted := Remux{Input: "in", Audio: -1, Format: TS, Plan: plan, Encode: &VideoEncoding{Encoder: "libx264", Width: 1280, Height: 720}}
	burning := Remux{Input: "in", Audio: -1, Format: TS, Plan: plan, Encode: &VideoEncoding{Encoder: "libx264", Width: 1280, Height: 720, Burn: &burned}}
	grid := Remux{Input: "in", Audio: -1, Format: TS, Plan: NewGridPlan(14 * time.Second), Encode: &VideoEncoding{Encoder: "libx264", Width: 1280, Height: 720}}
	for name, tc := range map[string]struct {
		r    Remux
		n    int
		want bool
	}{
		"a copy from the start":             {copied, 0, false},
		"a copy from a later segment":       {copied, 2, true},
		"a conversion from a later segment": {converted, 1, true},
		"burned subtitles":                  {burning, 2, false},
		"an MPEG-TS file":                   {grid, 2, false},
	} {
		if got := probes(tc.r, tc.n); got != tc.want {
			t.Errorf("%s: probing less %v, want %v: %q", name, got, tc.want, tc.r.args(tc.n))
		}
	}
}
