package hls

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// earlyEncoding returns the encoding of key, nil when there is none.
func (k *keptPlayback) earlyEncoding(key Key) *encoding {
	k.t.Helper()
	k.m.mu.Lock()
	defer k.m.mu.Unlock()
	return k.m.encodings[key]
}

// madeAlone waits until e made its first segment, and reports whether its
// FFmpeg then ended there: the next segment is not made, nor begun.
func madeAlone(t *testing.T, e *encoding) bool {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for {
		e.mu.Lock()
		made, next, early := e.ready[0], -1, e.early
		if e.job != nil {
			next = e.job.next
		}
		alone := !e.ready[1] && next == 1
		e.mu.Unlock()
		if made {
			// Long enough for FFmpeg to have gone on, were it not waiting.
			time.Sleep(300 * time.Millisecond)
			e.mu.Lock()
			alone = alone && !e.ready[1] && e.job.next == 1 && e.job.file == nil && e.job.done
			e.mu.Unlock()
			return alone && early
		}
		if time.Now().After(deadline) {
			t.Fatal("the first segment was never made")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func stopped(e *encoding) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stopped
}

// An encoding started ahead of its play makes its first segment and ends,
// counting toward no limit and listed nowhere, its input read within that
// segment; the play's first request takes it over, with the segments
// made, served without another FFmpeg, its input read on, and it counts
// from then on.
func TestAPlayTakesOverTheEncodingStartedEarly(t *testing.T) {
	t.Parallel()
	k := newKeptPlayback(t, FMP4)
	key := k.key("early-1", 1)
	var lift sync.Once
	lifted := make(chan struct{})
	open := func(ctx context.Context) (Remux, func(), error) {
		r, release, err := k.opener(key)(ctx)
		r.Lift = func() { lift.Do(func() { close(lifted) }) }
		return r, release, err
	}
	if err := k.m.Early(t.Context(), key, open); err != nil {
		t.Fatal(err)
	}
	e := k.earlyEncoding(key)
	if e == nil || !madeAlone(t, e) {
		t.Fatal("the encoding started early did not make its first segment alone")
	}
	if !k.m.MayConvert("bob", "v2") || len(k.m.Running()) != 0 {
		t.Errorf("before the play: another may convert %v, running %d, want true and 0", k.m.MayConvert("bob", "v2"), len(k.m.Running()))
	}
	select {
	case <-lifted:
		t.Error("the input was read on before the play")
	default:
	}

	if got := k.probe(k.segment(key, 0)).channels(); got != 2 {
		t.Errorf("the play's first segment has %d channels, want 2", got)
	}
	if k.earlyEncoding(key) != e {
		t.Fatal("the play did not take over the encoding started early")
	}
	select {
	case <-lifted:
	case <-time.After(10 * time.Second):
		t.Error("the play did not read the input on")
	}
	e.mu.Lock()
	early, runs := e.early, e.runs
	e.mu.Unlock()
	if early || runs != 1 {
		t.Errorf("after the play's request: early %v, FFmpeg runs %d, want false and 1", early, runs)
	}
	if k.m.MayConvert("bob", "v2") || len(k.m.Running()) != 1 {
		t.Errorf("after the play's request: another may convert %v, running %d, want false and 1", k.m.MayConvert("bob", "v2"), len(k.m.Running()))
	}
	// Taken over, nothing that ends an early start ends it.
	k.m.expireEarly(time.Now().Add(time.Hour))
	k.m.StopEarly(key.Session)
	if stopped(e) || k.probe(k.segment(key, 1)).channels() != 2 {
		t.Error("the play stopped once taken over")
	}
}

// A play that asks for another encoding, here another audio track, stops
// the one started early for its device; another device's goes on.
func TestAPlayWithOtherParametersStopsTheEarlyEncoding(t *testing.T) {
	t.Parallel()
	k := newKeptPlayback(t, TS)
	k.m.KeepVideo(func() bool { return false })
	k.m.LimitConversions(func() int { return 0 })
	key := k.key("early-1", 1)
	if err := k.m.Early(t.Context(), key, k.opener(key)); err != nil {
		t.Fatal(err)
	}
	e := k.earlyEncoding(key)
	other := k.key("early-1", 2)
	if got := k.probe(k.segment(other, 0)).channels(); got != 1 {
		t.Errorf("the other track's segment has %d channels, want 1", got)
	}
	if !stopped(e) || k.earlyEncoding(key) != nil {
		t.Error("the encoding started early went on beside the play")
	}
}

// One no play took over stops once earlyIdle passed, its segments removed;
// one per user and device runs, the newest; and StopEarly stops its
// session's.
func TestAnEarlyEncodingStopsWhenNoPlayComes(t *testing.T) {
	t.Parallel()
	k := newKeptPlayback(t, TS)
	k.m.LimitConversions(func() int { return 0 })
	removed := make(chan string, 4)
	k.m.removed = func(dir string, _ error) { removed <- dir }
	first := k.key("early-1", 1)
	if err := k.m.Early(t.Context(), first, k.opener(first)); err != nil {
		t.Fatal(err)
	}
	e := k.earlyEncoding(first)
	madeAlone(t, e)
	k.m.expireEarly(time.Now())
	if stopped(e) {
		t.Fatal("stopped before earlyIdle")
	}
	k.m.expireEarly(e.earlyAt.Add(earlyIdle))
	if !stopped(e) || k.earlyEncoding(first) != nil {
		t.Fatal("still running after earlyIdle")
	}
	select {
	case dir := <-removed:
		if dir != e.dir {
			t.Errorf("removed %s, want %s", dir, e.dir)
		}
	case <-time.After(30 * time.Second):
		t.Error("its segments were not removed")
	}

	// Another title opened on the same device: the newest alone runs.
	if err := k.m.Early(t.Context(), first, k.opener(first)); err != nil {
		t.Fatal(err)
	}
	e = k.earlyEncoding(first)
	next := k.key("early-2", 1)
	next.Version, next.Converts, next.Video = "v2", false, ""
	if err := k.m.Early(t.Context(), next, k.opener(next)); err != nil {
		t.Fatal(err)
	}
	if !stopped(e) || k.earlyEncoding(first) != nil || k.earlyEncoding(next) == nil {
		t.Error("two encodings started early run for one device")
	}
	k.m.StopEarly(next.Session)
	if k.earlyEncoding(next) != nil {
		t.Error("StopEarly left the encoding started early")
	}
}

// None starts while the server converts video: for another user's play,
// or ahead of another play.
func TestNoEarlyEncodingStartsWhileTheServerConverts(t *testing.T) {
	t.Parallel()
	k := newKeptPlayback(t, TS)
	k.m.KeepVideo(func() bool { return false })
	k.m.LimitConversions(func() int { return 0 })
	ahead := k.key("early-1", 1)
	ahead.User, ahead.Device = "carol", "phone"
	if err := k.m.Early(t.Context(), ahead, k.opener(ahead)); err != nil {
		t.Fatal(err)
	}
	key := k.key("early-2", 1)
	if err := k.m.Early(t.Context(), key, k.opener(key)); !errors.Is(err, ErrBusy) {
		t.Errorf("beside another conversion started early: %v, want ErrBusy", err)
	}
	k.m.StopEarly(ahead.Session)
	playing := k.key("play-1", 1)
	playing.User, playing.Device = "carol", "phone"
	k.segment(playing, 0)
	if err := k.m.Early(t.Context(), key, k.opener(key)); !errors.Is(err, ErrBusy) {
		t.Errorf("beside a play converting: %v, want ErrBusy", err)
	}
	if k.earlyEncoding(key) != nil {
		t.Error("an encoding started early while the server converts")
	}
}
