package hls

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/process"
)

// streamingFFmpeg writes a script that runs as FFmpeg's HLS muxer does on
// a live stream sped up: it writes a segment of three seconds every
// interval, numbered on from -start_number and holding its own number,
// with a playlist listing every one of the run, so that none is missed
// however fast they come, and a discontinuity first for a run after
// another, until it is stopped.
func streamingFFmpeg(t *testing.T, interval string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake FFmpeg is a shell script")
	}
	script := `#!/bin/sh
case "$*" in *"-f hls"*) ;; *) exit 0 ;; esac
start=0; flags=""
for argument; do
  case "$previous" in
    -start_number) start=$argument ;;
    -hls_flags) flags=$argument ;;
  esac
  previous=$argument
done
playlist=$argument
dir=$(dirname "$playlist")
n=$start
while :; do
  printf 'segment %s' "$n" > "$dir/$n.tmp"; mv "$dir/$n.tmp" "$dir/$n.ts"
  from=$start
  {
    printf '#EXTM3U\n#EXT-X-VERSION:6\n#EXT-X-TARGETDURATION:3\n#EXT-X-MEDIA-SEQUENCE:%s\n' "$from"
    case "$flags" in *discont_start*) [ $from -eq $start ] && printf '#EXT-X-DISCONTINUITY\n' ;; esac
    i=$from
    while [ $i -le $n ]; do printf '#EXTINF:3.000000,\n%s.ts\n' "$i"; i=$((i+1)); done
  } > "$dir/tmp.m3u8"
  mv "$dir/tmp.m3u8" "$playlist"
  n=$((n+1))
  sleep ` + interval + `
done
`
	path := filepath.Join(t.TempDir(), "ffmpeg")
	if err := process.WriteExecutable(path, strings.NewReader(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// bufferedManager returns a manager running ffmpeg whose live encodings
// keep length, all within bytes, the disk telling free.
func bufferedManager(t *testing.T, ffmpeg string, length time.Duration, bytes int64) *Manager {
	t.Helper()
	m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	m.BufferLives(func() LiveBuffer { return LiveBuffer{Length: length, Bytes: bytes} })
	return m
}

// channelOpener opens a channel's stream, counting how often it is open.
func channelOpener(open *atomic.Int32) Opener {
	return func(context.Context) (Remux, func(), error) {
		open.Add(1)
		return Remux{Input: "http://127.0.0.1:1/live.ts", Video: 0, Audio: -1, Format: TS}, func() { open.Add(-1) }, nil
	}
}

// listed is the media sequence a playlist starts at, and the segments it
// lists, in order.
func listed(t *testing.T, playlist []byte) (int, []string) {
	t.Helper()
	sequence := -1
	var names []string
	for line := range strings.Lines(string(playlist)) {
		line = strings.TrimSpace(line)
		if value, ok := strings.CutPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"); ok {
			sequence, _ = strconv.Atoi(value)
		} else if line != "" && !strings.HasPrefix(line, "#") {
			names = append(names, line)
		}
	}
	if sequence < 0 || len(names) == 0 {
		t.Fatalf("not a live playlist: %s", playlist)
	}
	return sequence, names
}

// waitUntil polls until done holds, for up to 10 seconds.
func waitUntil(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// liveOf is the live encoding key watches.
func liveOf(m *Manager, key Key) *live {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lives[key.channel()]
}

// A channel's playlist goes back to where the play session joined, as far
// as the buffer keeps, which drops the oldest segments past its length and
// removes their files; a play session joining later shares the encoding
// and starts near the newest segment, from where it may go back as far
// as it watched.
func TestTheLiveBufferKeepsItsLength(t *testing.T) {
	t.Parallel()
	m := bufferedManager(t, streamingFFmpeg(t, "0.02"), time.Minute, 1<<40)
	var open atomic.Int32
	uri := func(name string) string { return name }
	alice := Key{Session: "alice", Audio: -1, Format: TS, User: "alice", Version: "channel"}
	first, err := m.LivePlaylist(t.Context(), alice, channelOpener(&open), uri, 1)
	if err != nil {
		t.Fatal(err)
	}
	joined, _ := listed(t, first)
	// Two minutes of stream, sped up.
	l := liveOf(m, alice)
	waitUntil(t, "two minutes of stream", func() bool {
		l.mu.Lock()
		defer l.mu.Unlock()
		return l.buffer.last >= 45
	})
	playlist, err := m.LivePlaylist(t.Context(), alice, channelOpener(&open), uri, 1)
	if err != nil {
		t.Fatal(err)
	}
	sequence, names := listed(t, playlist)
	if sequence <= joined || len(names) < 20 || len(names) > 22 {
		t.Fatalf("the playlist after two minutes starts at %d, joined at %d, and lists %d segments, want a minute's 20: %s",
			sequence, joined, len(names), playlist)
	}
	// Each segment is the one its place tells: the media sequence numbers
	// them as FFmpeg did.
	for i, name := range names {
		if name != strconv.Itoa(sequence+i)+".ts" {
			t.Fatalf("segment %d of the playlist is %s, want %d.ts", i, name, sequence+i)
		}
	}
	// A seek back of a minute plays the oldest segments listed.
	file, err := m.LiveSegment(alice, names[0])
	if err != nil {
		t.Fatalf("the segment a minute back: %v", err)
	}
	data, _ := os.ReadFile(file.Name())
	file.Close()
	if string(data) != "segment "+strconv.Itoa(sequence) {
		t.Errorf("the segment a minute back holds %q", data)
	}
	// What the buffer dropped is removed, a segment later.
	waitUntil(t, "the dropped segments removed", func() bool {
		_, err := os.Stat(filepath.Join(l.dir, strconv.Itoa(sequence-2)+".ts"))
		return errors.Is(err, os.ErrNotExist)
	})

	bob := Key{Session: "bob", Audio: -1, Format: TS, User: "bob", Version: "channel"}
	joining, err := m.LivePlaylist(t.Context(), bob, channelOpener(&open), uri, 1)
	if err != nil {
		t.Fatal(err)
	}
	if liveOf(m, bob) != l || open.Load() != 1 {
		t.Fatalf("a second user watching the channel runs an encoding of their own: %d inputs open", open.Load())
	}
	bobFrom, bobNames := listed(t, joining)
	if len(bobNames) < 2 || len(bobNames) > 4 || bobFrom < sequence+len(names)-4 {
		t.Errorf("a play session joining starts at %d with %d segments, want two near the newest: %s", bobFrom, len(bobNames), joining)
	}
	waitUntil(t, "Bob's window to grow", func() bool {
		playlist, err := m.LivePlaylist(t.Context(), bob, channelOpener(&open), uri, 1)
		if err != nil {
			t.Fatal(err)
		}
		from, names := listed(t, playlist)
		return from == bobFrom && len(names) >= 6
	})
}

// A channel's buffer outlives its last viewer for liveLinger: FFmpeg stops
// and the source is released at once, and a play session that comes back
// within it finds the segments it left, then the stream read again after
// a discontinuity. The buffer is removed once it lingered.
func TestTheLiveBufferLingersAfterTheLastViewer(t *testing.T) {
	t.Parallel()
	m := bufferedManager(t, streamingFFmpeg(t, "0.02"), time.Hour, 1<<40)
	var open atomic.Int32
	uri := func(name string) string { return name }
	key := Key{Session: "s", Audio: -1, Format: TS, User: "alice", Version: "channel"}
	first, err := m.LivePlaylist(t.Context(), key, channelOpener(&open), uri, 1)
	if err != nil {
		t.Fatal(err)
	}
	joined, _ := listed(t, first)
	l := liveOf(m, key)
	waitUntil(t, "a few segments", func() bool {
		l.mu.Lock()
		defer l.mu.Unlock()
		return l.buffer.last >= 15
	})

	// The player went quiet, its app reporting nothing.
	l.vmu.Lock()
	l.viewers["s"].used = time.Now().Add(-liveIdle - time.Second)
	l.vmu.Unlock()
	m.tendLives()
	if liveOf(m, key) != l || !l.isSuspended() || open.Load() != 0 {
		t.Fatalf("a channel no one watches: kept %t, suspended %t, %d inputs open", liveOf(m, key) == l, l.isSuspended(), open.Load())
	}
	// FFmpeg stopped may have left a segment it did not list: the folder
	// is compared with itself.
	l.mu.Lock()
	last, written := l.buffer.last, lastSegment(l.dir)
	l.mu.Unlock()
	time.Sleep(200 * time.Millisecond)
	l.mu.Lock()
	grew := l.buffer.last != last || lastSegment(l.dir) != written
	l.mu.Unlock()
	if grew {
		t.Fatal("FFmpeg goes on once suspended")
	}

	// It comes back: its playlist goes back to where it joined, and on
	// past a discontinuity once the stream is read again.
	var playlist []byte
	waitUntil(t, "the stream read again", func() bool {
		if playlist, err = m.LivePlaylist(t.Context(), key, channelOpener(&open), uri, 1); err != nil {
			t.Fatal(err)
		}
		_, names := listed(t, playlist)
		return names[len(names)-1] != strconv.Itoa(last)+".ts"
	})
	// FFmpeg stopped may have left a segment it did not list, which the
	// next run numbers past.
	if from, _ := listed(t, playlist); from != joined || !strings.Contains(string(playlist), strconv.Itoa(last)+".ts\n#EXT-X-DISCONTINUITY\n") {
		t.Fatalf("the playlist on coming back, joined at %d: %s", joined, playlist)
	}
	if open.Load() != 1 {
		t.Errorf("%d inputs open once resumed", open.Load())
	}

	// It stops: the buffer lingers, then goes.
	m.Stop("s")
	if !l.isSuspended() || open.Load() != 0 {
		t.Fatalf("a channel left: suspended %t, %d inputs open", l.isSuspended(), open.Load())
	}
	m.tendLives()
	if liveOf(m, key) != l {
		t.Fatal("the buffer went before lingering")
	}
	l.mu.Lock()
	l.suspendedAt = time.Now().Add(-liveLinger - time.Second)
	l.mu.Unlock()
	m.tendLives()
	if liveOf(m, key) != nil {
		t.Error("the buffer lingers past liveLinger")
	}
	if _, err := os.Stat(l.dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the buffer's folder: %v", err)
	}
}

// A paused app that reports its progress keeps the channel's buffer
// growing, though its player asks for nothing.
func TestAPausedViewerKeepsTheBufferGrowing(t *testing.T) {
	t.Parallel()
	m := bufferedManager(t, streamingFFmpeg(t, "0.02"), time.Minute, 1<<40)
	var open atomic.Int32
	key := Key{Session: "s", Audio: -1, Format: TS, User: "alice", Version: "channel"}
	if _, err := m.LivePlaylist(t.Context(), key, channelOpener(&open), func(name string) string { return name }, 1); err != nil {
		t.Fatal(err)
	}
	l := liveOf(m, key)
	l.vmu.Lock()
	l.viewers["s"].used = time.Now().Add(-liveIdle - time.Second)
	l.vmu.Unlock()
	m.KeepWatching("s")
	m.tendLives()
	if l.isSuspended() || open.Load() != 1 {
		t.Errorf("a channel paused: suspended %t, %d inputs open", l.isSuspended(), open.Load())
	}
}

// Without a buffer, a live encoding keeps its window, and stops with its
// last viewer, as before.
func TestALiveEncodingWithoutBufferStopsWithItsLastViewer(t *testing.T) {
	t.Parallel()
	m := bufferedManager(t, streamingFFmpeg(t, "0.02"), 0, 0)
	var open atomic.Int32
	uri := func(name string) string { return name }
	key := Key{Session: "s", Audio: -1, Format: TS, User: "alice", Version: "channel"}
	if _, err := m.LivePlaylist(t.Context(), key, channelOpener(&open), uri, 1); err != nil {
		t.Fatal(err)
	}
	l := liveOf(m, key)
	waitUntil(t, "a minute of stream", func() bool {
		l.mu.Lock()
		defer l.mu.Unlock()
		return l.buffer.last >= 25
	})
	playlist, err := m.LivePlaylist(t.Context(), key, channelOpener(&open), uri, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, names := listed(t, playlist); len(names) != liveWindow {
		t.Errorf("the playlist lists %d segments, want the live window's %d", len(names), liveWindow)
	}
	m.Stop("s")
	if liveOf(m, key) != nil || open.Load() != 0 {
		t.Errorf("a channel left keeps its encoding, %d inputs open", open.Load())
	}
	if _, err := os.Stat(l.dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("its folder: %v", err)
	}
}

// The buffers of all channels keep within their bound together, and
// within half of what the disk leaves them: the longest gives its oldest
// segments first, and none goes below its live window.
func TestLiveBuffersKeepWithinTheDisk(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		bytes int64
		// disk is the size of the disk, which the buffers fill.
		disk int64
	}{
		{"the bound", 300, 1 << 40},
		{"half the free space", 1 << 40, 800},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := bufferedManager(t, streamingFFmpeg(t, "0.02"), time.Hour, tc.bytes)
			m.diskFree = func(string) (int64, int64, string, bool) {
				free := tc.disk
				for _, l := range m.liveList() {
					l.mu.Lock()
					free -= l.buffer.bytes
					l.mu.Unlock()
				}
				return free, 0, "", true
			}
			var open atomic.Int32
			uri := func(name string) string { return name }
			var lives []*live
			for _, channel := range []string{"one", "two"} {
				key := Key{Session: channel, Audio: -1, Format: TS, User: "alice", Version: channel}
				if _, err := m.LivePlaylist(t.Context(), key, channelOpener(&open), uri, 1); err != nil {
					t.Fatal(err)
				}
				lives = append(lives, liveOf(m, key))
			}
			waitUntil(t, "two minutes of each channel", func() bool {
				for _, l := range lives {
					l.mu.Lock()
					last := l.buffer.last
					l.mu.Unlock()
					if last < 40 {
						return false
					}
				}
				return true
			})
			// Its viewers leave: FFmpeg stops, the buffers stay, and one
			// more pass takes the last segments in.
			m.Stop("one")
			m.Stop("two")
			m.capBuffers()
			var total int64
			var counts []int
			for _, l := range lives {
				l.mu.Lock()
				total += l.buffer.bytes
				counts = append(counts, len(l.buffer.segments))
				l.mu.Unlock()
			}
			if limit := min(tc.bytes, tc.disk/2); total > limit {
				t.Errorf("the buffers take %d bytes, past %d", total, limit)
			}
			for i, count := range counts {
				if count < liveWindow || count > 3*liveWindow {
					t.Errorf("channel %d keeps %d segments", i, count)
				}
			}
			if difference := counts[0] - counts[1]; difference < -3 || difference > 3 {
				t.Errorf("the channels keep %v segments: the longest did not give first", counts)
			}
		})
	}
}

// The playlist written from the buffer names each run's initialization
// segment, and counts the discontinuities that left it.
func TestTheLiveBufferPlaylistCountsDiscontinuities(t *testing.T) {
	t.Parallel()
	m := bufferedManager(t, "ffmpeg-not-installed", time.Minute, 0)
	l := &live{m: m, dir: t.TempDir(), viewers: map[string]*viewer{}, buffer: segmentBuffer{last: -1}}
	l.remux.Format = FMP4
	write := func(run, from, to int) {
		t.Helper()
		var playlist strings.Builder
		playlist.WriteString("#EXTM3U\n#EXT-X-MAP:URI=\"" + liveInitName(run, run > 0) + "\"\n")
		for n := from; n <= to; n++ {
			if err := os.WriteFile(filepath.Join(l.dir, strconv.Itoa(n)+".mp4"), []byte("segment"), 0o600); err != nil {
				t.Fatal(err)
			}
			playlist.WriteString("#EXTINF:2.500000,\n" + strconv.Itoa(n) + ".mp4\n")
		}
		if err := os.WriteFile(filepath.Join(l.dir, livePlaylist), []byte(playlist.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []string{"init.mp4", "init-30.mp4"} {
		if err := os.WriteFile(filepath.Join(l.dir, init), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buffer.init = liveInit
	write(0, 0, 29)
	l.takeIn()
	l.next = 30
	l.buffer.runFirst, l.buffer.discontinue, l.buffer.init = l.buffer.end(), true, "init-30.mp4"
	write(30, 30, 39)
	l.takeIn()
	// A minute of 2.5-second segments: 24, the first run's last 14, then
	// the second's, after a discontinuity and its initialization segment.
	if got := l.playlist(l.buffer.first, func(name string) string { return "/" + name }); string(got) != "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:3\n"+
		"#EXT-X-MEDIA-SEQUENCE:16\n#EXT-X-INDEPENDENT-SEGMENTS\n#EXT-X-MAP:URI=\"/init.mp4\"\n"+listing(16, 29)+
		"#EXT-X-DISCONTINUITY\n#EXT-X-MAP:URI=\"/init-30.mp4\"\n"+listing(30, 39) {
		t.Fatalf("the playlist:\n%s", got)
	}
	write(30, 40, 54)
	l.takeIn()
	if got := string(l.playlist(l.buffer.first, func(name string) string { return name })); !strings.HasPrefix(got,
		"#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:3\n#EXT-X-MEDIA-SEQUENCE:31\n#EXT-X-DISCONTINUITY-SEQUENCE:1\n"+
			"#EXT-X-INDEPENDENT-SEGMENTS\n#EXT-X-MAP:URI=\"init-30.mp4\"\n#EXTINF:2.500000,\n31.mp4\n") {
		t.Fatalf("the playlist once the discontinuity left it:\n%s", got)
	}
	// The first run's initialization segment goes with its last segment,
	// once the next segment comes.
	write(30, 55, 55)
	l.takeIn()
	for _, gone := range []string{"init.mp4", "29.mp4", "30.mp4"} {
		if _, err := os.Stat(filepath.Join(l.dir, gone)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s, dropped: %v", gone, err)
		}
	}
	if _, err := os.Stat(filepath.Join(l.dir, "init-30.mp4")); err != nil {
		t.Errorf("the second run's initialization segment: %v", err)
	}
}

// listing is the lines of a playlist listing segments from to to, of 2.5
// seconds each, named by number.
func listing(from, to int) string {
	var b strings.Builder
	for n := from; n <= to; n++ {
		b.WriteString("#EXTINF:2.500000,\n/" + strconv.Itoa(n) + ".mp4\n")
	}
	return b.String()
}
