package hls

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// A recording's parts are joined into one Matroska file even when the
// temporary folder cannot be written, as in a read-only container, and
// nothing but the file is left in the recordings folder.
func TestRecordingPartsJoinWithoutATemporaryFolder(t *testing.T) {
	ffmpeg, _ := tools(t)
	folder := t.TempDir()
	var parts []string
	for n := range 2 {
		part := filepath.Join(folder, "part"+strconv.Itoa(n)+".ts")
		out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=160x90:rate=10:duration=1",
			"-c:v", "libx264", "-f", "mpegts", part).CombinedOutput()
		if err != nil {
			t.Fatalf("making a part: %v: %s", err, out)
		}
		parts = append(parts, part)
	}
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	path := filepath.Join(folder, "recording.mkv")
	if err := m.Finish(t.Context(), parts, path); err != nil {
		t.Fatalf("joining the parts: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.HasPrefix(data, []byte{0x1a, 0x45, 0xdf, 0xa3}) {
		t.Fatalf("the joined file is not Matroska: %v", err)
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if name := entry.Name(); name != "recording.mkv" && name != "part0.ts" && name != "part1.ts" {
			t.Errorf("left in the recordings folder: %s", name)
		}
	}
}

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

// recorded records the stream at input until stop returns, then ends the
// recording's context, and returns how long Record took to return after
// that, with the file it wrote. m.grace is given grace.
func recorded(t *testing.T, grace time.Duration, input string, stop func(path string)) (time.Duration, string) {
	t.Helper()
	ffmpeg, _ := tools(t)
	m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	m.grace = grace
	path := filepath.Join(t.TempDir(), "recording.ts")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- m.Record(ctx, Key{Session: "record 0", User: "user"}, func(context.Context) (Remux, func(), error) {
			return Remux{Input: input, Video: 0, Audio: 1}, func() {}, nil
		}, path)
	}()
	stop(path)
	cancel()
	stopped := time.Now()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("the recording ended with %v", err)
	}
	return time.Since(stopped), path
}

// probedDuration is the duration ffprobe reads in a recording, which it
// must read without an error.
func probedDuration(t *testing.T, path string) time.Duration {
	t.Helper()
	_, ffprobe := tools(t)
	var stderr bytes.Buffer
	cmd := exec.Command(ffprobe, "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil || stderr.Len() > 0 {
		t.Fatalf("ffprobe: %v: %s", err, stderr.Bytes())
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		t.Fatalf("ffprobe's duration: %q", out)
	}
	return time.Duration(seconds * float64(time.Second))
}

// A recording whose live playlist lists no new segment stops soon after
// it ends, rather than when FFmpeg is killed: a first interrupt leaves
// FFmpeg waiting for the next segment, a second ends that wait. The file
// holds every segment read.
func TestARecordingWhoseSourceStallsStopsAtOnce(t *testing.T) {
	ffmpeg, _ := tools(t)
	dir := t.TempDir()
	out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10:duration=3",
		"-f", "lavfi", "-i", "sine=duration=3", "-c:v", "libx264", "-g", "10", "-pix_fmt", "yuv420p", "-c:a", "aac",
		"-f", "hls", "-hls_time", "1", "-hls_list_size", "0", "-hls_flags", "omit_endlist", filepath.Join(dir, "live.m3u8")).CombinedOutput()
	if err != nil {
		t.Fatalf("making the segments: %v: %s", err, out)
	}
	var mu sync.Mutex
	segments, reloads := map[string]bool{}, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if name := strings.TrimPrefix(r.URL.Path, "/"); strings.HasSuffix(name, ".ts") {
			segments[name] = true
		} else if len(segments) == 3 {
			reloads++
		}
		mu.Unlock()
		http.ServeFile(w, r, filepath.Join(dir, r.URL.Path))
	}))
	t.Cleanup(server.Close)

	elapsed, path := recorded(t, recordGrace, server.URL+"/live.m3u8", func(string) {
		// FFmpeg read the 3 segments, then the playlist again: it waits
		// for the next segment.
		for deadline := time.Now().Add(20 * time.Second); ; time.Sleep(50 * time.Millisecond) {
			mu.Lock()
			stalled := reloads > 0
			mu.Unlock()
			if stalled {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("FFmpeg never read the whole playlist")
			}
		}
	})
	if elapsed > recordGrace+3*time.Second {
		t.Errorf("the recording stopped %s after its end, want about %s, well before FFmpeg is killed at %s", elapsed, recordGrace, recordKill)
	}
	if duration := probedDuration(t, path); duration < 2500*time.Millisecond {
		t.Errorf("the recording holds %s of the 3 seconds read", duration)
	}
}

// A recording whose source flows stops at the first interrupt, FFmpeg
// writing what it holds.
func TestARecordingWhoseSourceFlowsStopsAtTheFirstInterrupt(t *testing.T) {
	ffmpeg, _ := tools(t)
	source := filepath.Join(t.TempDir(), "source.ts")
	out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10:duration=30",
		"-f", "lavfi", "-i", "sine=duration=30", "-c:v", "libx264", "-g", "10", "-pix_fmt", "yuv420p", "-c:a", "aac", "-f", "mpegts", source).CombinedOutput()
	if err != nil {
		t.Fatalf("making the source: %v: %s", err, out)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	// An endless stream, as a channel's, sent about as it airs: its 30
	// seconds over 30 seconds, never ending while it is recorded.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		chunk := len(data) / 300 / 188 * 188
		for sent := 0; sent < len(data); sent += chunk {
			if _, err := w.Write(data[sent:min(sent+chunk, len(data))]); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}))
	t.Cleanup(server.Close)

	// FFmpeg is interrupted again only after 8 seconds: stopping before,
	// it stopped at the first.
	const grace = 8 * time.Second
	started := time.Now()
	elapsed, path := recorded(t, grace, server.URL+"/live.ts", func(path string) {
		for deadline := time.Now().Add(20 * time.Second); ; time.Sleep(50 * time.Millisecond) {
			if info, err := os.Stat(path); err == nil && info.Size() > 0 && time.Since(started) > 3*time.Second {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("FFmpeg never wrote the recording")
			}
		}
	})
	if elapsed > grace/2 {
		t.Errorf("the recording stopped %s after its end, want well before the second interrupt, %s after", elapsed, grace)
	}
	if duration := probedDuration(t, path); duration < time.Second {
		t.Errorf("the recording holds %s", duration)
	}
}
