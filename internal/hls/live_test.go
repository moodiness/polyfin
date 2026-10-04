package hls

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"testing"
)

// Live encodings read their source until the app leaves: a user starting
// one more than userLives stops their oldest, and the server refuses one
// past serverLives.
func TestLiveEncodingsAreCapped(t *testing.T) {
	m, err := NewManager("ffmpeg-not-installed", t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	open := func(context.Context) (Remux, func(), error) {
		return Remux{Input: "http://127.0.0.1:1/live.m3u8", Audio: -1, Format: TS}, func() {}, nil
	}
	uri := func(name string) string { return name }
	start := func(user string, n int) (Key, error) {
		key := Key{Session: user + "-" + strconv.Itoa(n), Audio: -1, Format: TS, User: user}
		_, err := m.LivePlaylist(t.Context(), key, open, uri)
		return key, err
	}
	live := func(key Key) bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.lives[key] != nil
	}
	var keys []Key
	for n := range userLives + 1 {
		key, _ := start("alice", n)
		keys = append(keys, key)
	}
	if live(keys[0]) {
		t.Error("alice's oldest live encoding still runs past the limit")
	}
	for _, key := range keys[1:] {
		if !live(key) {
			t.Errorf("%s was stopped", key.Session)
		}
	}
	for n := range serverLives - userLives {
		if _, err := start("user"+strconv.Itoa(n), 0); errors.Is(err, ErrBusy) {
			t.Fatalf("refused below the server's limit, at %d", n)
		}
	}
	if _, err := start("bob", 0); !errors.Is(err, ErrBusy) {
		t.Errorf("one live encoding past the server's limit: %v, want ErrBusy", err)
	}
}
