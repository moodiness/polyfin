package hls

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newFoldersManager is a manager whose FFmpeg never runs.
func newFoldersManager(t *testing.T) *Manager {
	t.Helper()
	m, err := NewManager("ffmpeg-not-installed", t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m
}

// keepsSegment writes a segment into dir, as FFmpeg would, and returns a
// check that it is still there.
func keepsSegment(t *testing.T, dir string) func() {
	t.Helper()
	path := filepath.Join(dir, "0.ts")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("segment"), 0o600); err != nil {
		t.Fatal(err)
	}
	return func() {
		t.Helper()
		if _, err := os.Stat(path); err != nil {
			t.Errorf("the new one's segment: %v", err)
		}
	}
}

// An audio conversion stopping, its FFmpeg about to end, leaves the
// segments of a new one of the same key alone: the stop removes its own
// folder only. The test plays the stop's two halves around the new one's
// start, as a stop waiting for its FFmpeg lets it happen.
func TestAStoppedAudioConversionKeepsTheNextOnesSegments(t *testing.T) {
	t.Parallel()
	m := newFoldersManager(t)
	plan := NewGridPlan(30 * time.Second)
	open := func(context.Context) (Audio, Plan, func(), error) { return Audio{}, plan, func() {}, nil }
	key := AudioKey{Session: "session", Format: TS}
	// Asking past the plan's end opens the conversion without FFmpeg.
	conversion := func() *audioEncoding {
		t.Helper()
		if _, err := m.AudioSegment(t.Context(), key, open, plan.Len()); !errors.Is(err, ErrNotFound) {
			t.Fatalf("a segment past the end: %v", err)
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.audios[key]
	}
	old := conversion()
	// The stop takes it off the conversions first (see stopAudios).
	m.mu.Lock()
	delete(m.audios, key)
	m.mu.Unlock()
	next := conversion()
	kept := keepsSegment(t, next.dir)
	old.stop()
	kept()
	if _, err := os.Stat(old.dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the stopped one's folder: %v", err)
	}
}

// A live encoding stopping, its FFmpeg about to end, leaves the segments
// of a new one of the same key alone, as an audio conversion does.
func TestAStoppedLiveEncodingKeepsTheNextOnesSegments(t *testing.T) {
	t.Parallel()
	m := newFoldersManager(t)
	// An input that fails to open starts no FFmpeg; the encoding stays.
	failed := errors.New("no input")
	open := func(context.Context) (Remux, func(), error) { return Remux{}, nil, failed }
	key := Key{Session: "session", Audio: -1, Format: TS, User: "alice"}
	live := func() *live {
		t.Helper()
		if _, err := m.LivePlaylist(t.Context(), key, open, func(name string) string { return name }, 1); !errors.Is(err, failed) {
			t.Fatalf("a live encoding of no input: %v", err)
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.lives[key]
	}
	old := live()
	// The stop takes it off the live encodings first (see stopLives).
	m.mu.Lock()
	delete(m.lives, key)
	m.mu.Unlock()
	next := live()
	kept := keepsSegment(t, next.dir)
	old.stop()
	kept()
	if _, err := os.Stat(old.dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the stopped one's folder: %v", err)
	}
}
