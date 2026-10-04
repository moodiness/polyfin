package hls

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
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
