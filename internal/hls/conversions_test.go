package hls

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

// The playbacks whose video is converted are limited, files and live
// alike: one more is refused, while those running go on. A user's playback
// of a version counts once, whatever its play sessions, and copies never
// count.
func TestConversionsAreLimited(t *testing.T) {
	m, err := NewManager("ffmpeg-not-installed", t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	limit := 2
	m.LimitConversions(func() int { return limit })
	plan := NewPlan([]time.Duration{0, 6 * time.Second}, 12*time.Second)
	open := func(context.Context) (Remux, func(), error) {
		return Remux{Input: "http://127.0.0.1:1/movie.mkv", Audio: -1, Format: TS, Plan: plan}, func() {}, nil
	}
	// FFmpeg is not installed: what matters is whether the encoding may
	// start at all.
	busy := func(key Key, n int) bool {
		t.Helper()
		_, err := m.Segment(t.Context(), key, open, n)
		return errors.Is(err, ErrBusy)
	}
	live := func(key Key) bool {
		t.Helper()
		_, err := m.LivePlaylist(t.Context(), key, open, func(name string) string { return name })
		return errors.Is(err, ErrBusy)
	}
	converted := func(session, user, version string) Key {
		return Key{Session: session, Audio: -1, Format: TS, User: user, Version: version, Converts: true}
	}
	alice, bob, carol := converted("alice-1", "alice", "v1"), converted("bob-1", "bob", "v2"), converted("carol-1", "carol", "v3")
	channel := converted("carol-live", "carol", "channel")

	if busy(alice, 0) || busy(bob, 0) {
		t.Fatal("refused below the limit")
	}
	if !busy(carol, 0) || !live(channel) || m.MayConvert("carol", "v3") {
		t.Error("one conversion past the limit was not refused")
	}
	if copied := (Key{Session: "carol-2", Audio: -1, Format: TS, User: "carol", Version: "v3"}); busy(copied, 0) {
		t.Error("a remux copying the video was refused")
	}
	// The playbacks running go on: the next segment, a seek, or the same
	// title in a new play session, as apps start one to switch tracks.
	again := converted("alice-2", "alice", "v1")
	again.Audio = 1
	if busy(alice, 1) || busy(alice, 0) || busy(again, 0) || !m.MayConvert("alice", "v1") {
		t.Error("a playback converting its video was refused at the limit")
	}
	// A playback that stops makes room.
	m.Stop("bob-1")
	if busy(carol, 0) {
		t.Error("refused once a playback stopped")
	}
	// A higher limit applies at once, and live encodings count.
	limit = 3
	if live(channel) {
		t.Error("a live conversion refused below the limit")
	}
	dave := converted("dave-1", "dave", "v4")
	if !busy(dave, 0) {
		t.Error("a conversion past a limit reached with a live one was not refused")
	}
	limit = 0
	if busy(dave, 0) {
		t.Error("refused with no limit")
	}
}
