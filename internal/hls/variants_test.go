package hls

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// variants plays the variants of one playback of a 30-second source,
// converted to three sizes. FFmpeg makes one segment ahead of the last
// asked for, then waits, still running, and the conversions at once are
// limited to 1.
type variants struct {
	t       *testing.T
	m       *Manager
	plan    Plan
	input   string
	ffprobe string
}

func newVariants(t *testing.T) *variants {
	ffmpeg, ffprobe := tools(t)
	input, keyframes := source(t, ffmpeg, ffprobe, "aac")
	m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	m.LimitConversions(func() int { return 1 })
	m.LimitAhead(func() time.Duration { return time.Millisecond })
	v := &variants{t: t, m: m, plan: NewPlan(keyframes, 30*time.Second), input: input, ffprobe: ffprobe}
	if v.plan.Len() < 4 {
		t.Fatalf("%d segments", v.plan.Len())
	}
	return v
}

func (v *variants) opener(variant int) Opener {
	sizes := [][2]int{{160, 90}, {128, 72}, {96, 54}}
	return func(context.Context) (Remux, func(), error) {
		return Remux{Input: v.input, Video: 0, Audio: 1, Format: TS, Plan: v.plan, Encode: &VideoEncoding{Encoder: "libx264", Level: "4.1",
			Width: sizes[variant][0], Height: sizes[variant][1], Bitrate: 300_000 >> variant, FrameRate: 24}}, func() {}, nil
	}
}

func (v *variants) key(variant int) Key {
	return Key{Session: "session", Audio: 1, Format: TS, User: "alice", Version: "v1", Converts: true, Variant: variant}
}

// segment asks for segment n of a variant, and returns where it is.
func (v *variants) segment(variant, n int) string {
	v.t.Helper()
	f, err := v.m.Segment(v.t.Context(), v.key(variant), v.opener(variant), n)
	if err != nil {
		v.t.Fatalf("variant %d, segment %d: %v", variant, n, err)
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || info.Size() == 0 {
		v.t.Fatalf("variant %d, segment %d: %v, %v", variant, n, info, err)
	}
	return f.Name()
}

func (v *variants) encoding(variant int) *encoding {
	v.t.Helper()
	v.m.mu.Lock()
	defer v.m.mu.Unlock()
	e := v.m.encodings[v.key(variant)]
	if e == nil {
		v.t.Fatalf("variant %d has no encoding", variant)
	}
	return e
}

// runs returns how many FFmpeg processes a variant started, and whether
// the last runs.
func (v *variants) runs(variant int) (int, bool) {
	e := v.encoding(variant)
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.runs, e.job != nil && !e.job.done
}

// current returns, by variant, whether it is the one asked for last.
func (v *variants) current() map[int]bool {
	got := map[int]bool{}
	for _, r := range v.m.Running() {
		got[r.Key.Variant] = r.Current
	}
	return got
}

// Each variant of a playback runs its own FFmpeg: a lower one is not
// encoded until a segment of it is asked for, and asking for it leaves the
// first's running. Segment n of every variant starts at the same time, on
// a keyframe; the segments made stay; and the variants count once toward
// the conversions at once.
func TestEachVariantRunsItsOwnFFmpeg(t *testing.T) {
	v := newVariants(t)
	// No variant is left long enough to stop.
	v.m.linger.Store(int64(time.Hour))

	first := v.segment(0, 0)
	if got := v.current(); len(got) != 1 || !got[0] {
		t.Fatalf("the first variant alone: %v", got)
	}
	n := v.plan.Len() - 2
	lower := v.segment(2, n)
	if got := v.current(); len(got) != 2 || got[0] || !got[2] {
		t.Errorf("after switching: %v", got)
	}
	// The first variant's FFmpeg waits for the player, one segment ahead.
	if runs, running := v.runs(0); runs != 1 || !running {
		t.Errorf("the first variant: %d FFmpeg started, running %v", runs, running)
	}
	if running, _ := v.m.Conversions(); running != 1 {
		t.Errorf("%d playbacks converting", running)
	}
	other := Key{Session: "other", Audio: 1, Format: TS, User: "bob", Version: "v2", Converts: true}
	if _, err := v.m.Segment(t.Context(), other, v.opener(0), 0); !errors.Is(err, ErrBusy) {
		t.Errorf("another playback past the limit: %v", err)
	}
	if _, err := os.Stat(first); err != nil {
		t.Errorf("the first variant's segment: %v", err)
	}
	// Back up to the first variant, at the same segment: the lower one's
	// FFmpeg was not started again.
	upper := v.segment(0, n)
	if runs, _ := v.runs(2); runs != 1 {
		t.Errorf("the lower variant started %d FFmpeg", runs)
	}
	if _, err := os.Stat(lower); err != nil {
		t.Errorf("the lower variant's segment: %v", err)
	}
	starts := func(path string) float64 {
		t.Helper()
		for _, p := range probePackets(t, v.ffprobe, path) {
			if p.Type != "video" {
				continue
			}
			if !strings.Contains(p.Flags, "K") {
				t.Errorf("%s starts on a frame that is not a keyframe", path)
			}
			at, _ := strconv.ParseFloat(p.PTS, 64)
			return at
		}
		t.Fatalf("%s holds no video", path)
		return 0
	}
	if a, b := starts(upper), starts(lower); math.Abs(a-b) > 0.001 || math.Abs(a-(v.plan.Start(n)+timestampOffset).Seconds()) > 0.05 {
		t.Errorf("segment %d starts at %.3f s in the first variant, %.3f s in the lower one, %.3f s in the plan", n, a, b,
			(v.plan.Start(n) + timestampOffset).Seconds())
	}
}

// An app measuring the qualities it switches between, as hls.js does,
// asks for the same segments of two variants in turn: each is answered,
// and neither variant's FFmpeg starts again. Once the app keeps to one,
// the other's FFmpeg stops after the linger, and its segments stay.
func TestSwitchingBackAndForthKeepsBothVariantsRunning(t *testing.T) {
	v := newVariants(t)
	v.m.linger.Store(int64(time.Hour))
	for n := range 2 {
		for range 3 {
			v.segment(0, n)
			v.segment(2, n)
		}
	}
	for _, variant := range []int{0, 2} {
		if runs, running := v.runs(variant); runs != 1 || !running {
			t.Errorf("variant %d: %d FFmpeg started, running %v", variant, runs, running)
		}
	}
	if running, _ := v.m.Conversions(); running != 1 {
		t.Errorf("%d playbacks converting", running)
	}

	// The app keeps to the first variant: the lower one stops once it
	// was not asked for in the linger.
	const linger = 2 * time.Second
	v.m.linger.Store(int64(linger))
	lower := v.encoding(2)
	v.segment(0, 1)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	lower.mu.Lock()
	for !lower.job.done {
		if err := lower.wait(ctx); err != nil {
			lower.mu.Unlock()
			t.Fatalf("the lower variant's FFmpeg never stopped: %v", err)
		}
	}
	if stopped := time.Since(lower.used); stopped < linger {
		t.Errorf("the lower variant stopped %v after it was asked for", stopped)
	}
	lower.mu.Unlock()
	if got := v.current(); !got[0] || got[2] {
		t.Errorf("the variants asked for last: %v", got)
	}
	// The first, asked for last, runs on.
	if runs, running := v.runs(0); runs != 1 || !running {
		t.Errorf("the first variant: %d FFmpeg started, running %v", runs, running)
	}
	// The lower variant's segments stay; one it did not make starts its
	// FFmpeg again.
	v.segment(2, 1)
	if runs, _ := v.runs(2); runs != 1 {
		t.Errorf("a segment made started FFmpeg: %d runs", runs)
	}
	v.segment(2, 3)
	if runs, _ := v.runs(2); runs != 2 {
		t.Errorf("a segment not made: %d runs", runs)
	}
}
