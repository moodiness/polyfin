package hls

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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

// fakeLiveFFmpeg writes a script that runs as FFmpeg's HLS muxer would for
// a moment: it writes the segment numbered -start_number and a playlist
// naming it, ending it unless omit_endlist is asked, records its arguments
// in runs (those of live runs only are returned), then exits cleanly as when its input ends.
func fakeLiveFFmpeg(t *testing.T) (string, func() []string) {
	t.Helper()
	dir := t.TempDir()
	runs := filepath.Join(dir, "runs")
	script := `#!/bin/sh
echo "$*" >> ` + runs + `
case "$*" in *"-f hls"*) ;; *) exit 0 ;; esac
start=0; flags=""; playlist=""
while [ $# -gt 0 ]; do
  case "$1" in
    -start_number) start=$2; shift ;;
    -hls_flags) flags=$2; shift ;;
  esac
  playlist=$1
  shift
done
dir=$(dirname "$playlist")
printf 'segment' > "$dir/$start.ts"
{ printf '#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:%s\n' "$start"
  case "$flags" in *discont_start*) printf '#EXT-X-DISCONTINUITY\n' ;; esac
  printf '#EXTINF:3,\n%s.ts\n' "$start"
  case "$flags" in *omit_endlist*) ;; *) printf '#EXT-X-ENDLIST\n' ;; esac
} > "$playlist"
`
	path := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path, func() []string {
		data, _ := os.ReadFile(runs)
		var lives []string
		for line := range strings.Lines(string(data)) {
			if strings.Contains(line, "-f hls") {
				lives = append(lives, strings.TrimSpace(line))
			}
		}
		return lives
	}
}

// A live stream has no end: an input that ends is read again after a
// short backoff, numbering segments on with a discontinuity, and the
// playlist never says the stream ended.
func TestInterruptedLiveStreamsAreReadAgain(t *testing.T) {
	ffmpeg, runs := fakeLiveFFmpeg(t)
	m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	opens := 0
	open := func(context.Context) (Remux, func(), error) {
		opens++
		return Remux{Input: "http://127.0.0.1:1/live.ts", Video: 0, Audio: -1, Format: TS}, func() {}, nil
	}
	key := Key{Session: "s", Audio: -1, Format: TS, User: "alice"}
	uri := func(name string) string { return name }
	first, err := m.LivePlaylist(t.Context(), key, open, uri)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(first), "ENDLIST") {
		t.Errorf("the playlist ends: %s", first)
	}
	deadline := time.Now().Add(5 * time.Second)
	var playlist []byte
	for len(runs()) < 2 || !strings.Contains(string(playlist), "1.ts") {
		if time.Now().After(deadline) {
			t.Fatalf("the stream was not read again: %d runs, %s", len(runs()), playlist)
		}
		time.Sleep(100 * time.Millisecond)
		if playlist, err = m.LivePlaylist(t.Context(), key, open, uri); err != nil {
			t.Fatal(err)
		}
	}
	second := runs()[1]
	if !strings.Contains(second, "-start_number 1") || !strings.Contains(second, "discont_start") || !strings.Contains(second, "-hls_init_time 1") {
		t.Errorf("the second run: %s", second)
	}
	if strings.Contains(string(playlist), "ENDLIST") || !strings.Contains(string(playlist), "DISCONTINUITY") {
		t.Errorf("the playlist after a restart: %s", playlist)
	}
	// A clean end is no failure: the input is kept, not opened again.
	if opens != 1 {
		t.Errorf("the input was opened %d times", opens)
	}
}

// The backoff between quick runs grows from a second to four, and a
// stream read again too often gives up.
func TestLiveRestartsBackOffAndGiveUp(t *testing.T) {
	for quick, want := range []time.Duration{time.Second, time.Second, 2 * time.Second, 4 * time.Second, 4 * time.Second} {
		if got := liveBackoff(quick); got != want {
			t.Errorf("after %d quick runs: %s, want %s", quick, got, want)
		}
	}
	m, err := NewManager("ffmpeg-not-installed", t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	l := &live{m: m, key: Key{Session: "s"}, dir: t.TempDir(), opened: true, release: func() {}}
	now := time.Now()
	for range liveRestarts {
		l.restarts = append(l.restarts, now)
	}
	done := make(chan struct{})
	close(done)
	l.run = &liveRun{started: now.Add(-time.Minute), ended: now.Add(-30 * time.Second), done: done, err: ErrInterrupted}
	if _, err := l.ensure(t.Context(), nil); !errors.Is(err, ErrStopped) {
		t.Errorf("past the restarts allowed: %v", err)
	}
}
