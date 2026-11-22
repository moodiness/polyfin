package hls

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A synthetic channel, FFmpeg's test pattern streamed eight times as fast
// as it plays, goes through a live encoding keeping a minute: after two
// minutes of it, the playlist lists the last minute, a seek a minute back
// plays the picture of a minute before the newest segment, and what is
// older is gone from the disk.
func TestALiveChannelGoesBackAMinute(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := tools(t)
	channel := filepath.Join(t.TempDir(), "channel.ts")
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=25",
		"-t", "150", "-c:v", "libx264", "-preset", "ultrafast", "-g", "75", "-keyint_min", "75", "-sc_threshold", "0",
		"-f", "mpegts", channel).CombinedOutput(); err != nil {
		t.Fatalf("the channel: %v: %s", err, out)
	}
	m := bufferedManager(t, ffmpeg, time.Minute, 1<<40)
	open := func(context.Context) (Remux, func(), error) {
		return Remux{Input: channel, InputOptions: []string{"-readrate", "8"}, Video: 0, Audio: -1, Format: TS}, func() {}, nil
	}
	uri := func(name string) string { return name }
	key := Key{Session: "s", Audio: -1, Format: TS, User: "alice", Version: "channel"}
	if _, err := m.LivePlaylist(t.Context(), key, open, uri, 1); err != nil {
		t.Fatal(err)
	}
	l := liveOf(m, key)
	deadline := time.Now().Add(time.Minute)
	for {
		l.mu.Lock()
		last := l.buffer.last
		l.mu.Unlock()
		// Segments of three seconds, cut on the keyframes.
		if last >= 41 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("two minutes of the channel took over a minute: %d segments", last+1)
		}
		time.Sleep(100 * time.Millisecond)
	}
	playlist, err := m.LivePlaylist(t.Context(), key, open, uri, 1)
	if err != nil {
		t.Fatal(err)
	}
	sequence, names := listed(t, playlist)
	var durations []float64
	var window float64
	for line := range strings.Lines(string(playlist)) {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "#EXTINF:"); ok {
			seconds, _ := strconv.ParseFloat(strings.TrimSuffix(value, ","), 64)
			durations = append(durations, seconds)
			window += seconds
		}
	}
	if sequence == 0 || window < 60 || window > 63.5 {
		t.Fatalf("the playlist after two minutes starts at %d and lasts %.1f seconds, want a minute: %s", sequence, window, playlist)
	}

	// The player plays the newest segment, and goes back a minute.
	at := window - durations[len(durations)-1] - 60
	back, start := 0, 0.0
	for back < len(durations)-1 && start+durations[back] <= at {
		start += durations[back]
		back++
	}
	firstPicture := func(name string) float64 {
		t.Helper()
		file, err := m.LiveSegment(key, name)
		if err != nil {
			t.Fatalf("segment %s: %v", name, err)
		}
		defer file.Close()
		out, err := exec.Command(ffprobe, "-v", "error", "-select_streams", "v:0", "-show_entries", "packet=pts_time", "-of", "csv=p=0",
			"-read_intervals", "%+#1", file.Name()).Output()
		if err != nil {
			t.Fatalf("ffprobe of segment %s: %v", name, err)
		}
		pts, err := strconv.ParseFloat(strings.Trim(strings.Split(string(out), "\n")[0], ", \r"), 64)
		if err != nil {
			t.Fatalf("the first picture of segment %s: %q", name, out)
		}
		return pts
	}
	newest := len(names) - 1
	between := 0.0
	for _, d := range durations[back:newest] {
		between += d
	}
	if got := firstPicture(names[newest]) - firstPicture(names[back]); math.Abs(got-between) > 0.1 || between < 57 || between > 63 {
		t.Errorf("the segment a minute back is %.2f seconds of picture before the newest, its playlist telling %.2f", got, between)
	}

	// What the buffer dropped is gone from the disk.
	if _, err := os.Stat(filepath.Join(l.dir, "0.ts")); !os.IsNotExist(err) {
		t.Errorf("the first segment, dropped: %v", err)
	}
	// Every file left is accounted for: a segment of the buffer, one
	// dropped and removed once the next comes, one FFmpeg made since the
	// buffer last took its segments in, FFmpeg's playlist, or a file
	// FFmpeg is writing. The buffer takes in what FFmpeg listed first,
	// under its lock, so that nothing comes in meanwhile.
	l.mu.Lock()
	l.takeIn()
	kept := map[string]bool{livePlaylist: true}
	for _, s := range l.buffer.segments {
		kept[s.name] = true
	}
	for _, name := range l.buffer.doomed {
		kept[name] = true
	}
	last := l.buffer.last
	entries, _ := os.ReadDir(l.dir)
	l.mu.Unlock()
	for _, entry := range entries {
		name := entry.Name()
		if n, ok := segmentNumber(name); kept[name] || ok && n > last || strings.HasSuffix(name, ".tmp") {
			continue
		}
		t.Errorf("the buffer's folder holds %s, which the buffer does not account for", name)
	}
}
