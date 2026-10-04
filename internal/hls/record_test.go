package hls

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"testing"
)

// Recordings take live slots: a user recording userLives channels plays
// none live and records no more, and the server runs serverLives in all.
func TestRecordingsCountTowardLiveLimits(t *testing.T) {
	m, err := NewManager("ffmpeg-not-installed", t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	unopened := func(context.Context) (Remux, func(), error) {
		t.Error("a refused encoding was opened")
		return Remux{}, func() {}, errors.New("refused")
	}
	recording := func(user string, n int) Key { return Key{Session: "record " + strconv.Itoa(n), User: user} }
	m.mu.Lock()
	for n := range userLives {
		m.recordings[recording("busy", n)] = func() {}
	}
	m.mu.Unlock()
	if _, err := m.LivePlaylist(t.Context(), Key{Session: "watch", User: "busy"}, unopened, nil); !errors.Is(err, ErrBusy) {
		t.Errorf("a channel watched by a user recording %d: %v", userLives, err)
	}
	if err := m.Record(t.Context(), recording("busy", userLives), unopened, t.TempDir()+"/x.ts"); !errors.Is(err, ErrBusy) {
		t.Errorf("one more recording of that user: %v", err)
	}
	if err := m.Record(t.Context(), recording("busy", 0), unopened, t.TempDir()+"/x.ts"); !errors.Is(err, ErrBusy) {
		t.Errorf("a recording running already: %v", err)
	}
	m.mu.Lock()
	for n := userLives; n < serverLives; n++ {
		m.recordings[recording("user"+strconv.Itoa(n), n)] = func() {}
	}
	m.mu.Unlock()
	if _, err := m.LivePlaylist(t.Context(), Key{Session: "watch", User: "other"}, unopened, nil); !errors.Is(err, ErrBusy) {
		t.Errorf("a channel watched while the server records %d: %v", serverLives, err)
	}
	if err := m.Record(t.Context(), recording("other", 0), unopened, t.TempDir()+"/x.ts"); !errors.Is(err, ErrBusy) {
		t.Errorf("one more recording on the server: %v", err)
	}
}
