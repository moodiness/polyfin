package hls

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/moodiness/polyfin/internal/workfolder"
)

// The folder of the segments is emptied at start only when Polyfin made
// it: one that holds other files is left as it is and no manager is made
// for it, and one with its marker loses what a run left there.
func TestAManagerEmptiesOnlyAFolderPolyfinMade(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.DiscardHandler)
	dir := filepath.Join(t.TempDir(), "segments")
	kept := filepath.Join(dir, "holidays", "2019.mkv")
	if err := os.MkdirAll(filepath.Dir(kept), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kept, []byte("the administrator's own file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if m, err := NewManager("ffmpeg-not-installed", dir, logger); !errors.Is(err, workfolder.ErrOccupied) || m != nil {
		t.Errorf("a folder with other files: %v", err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("the administrator's file is gone: %v", err)
	}

	made := filepath.Join(t.TempDir(), "segments")
	m, err := NewManager("ffmpeg-not-installed", made, logger)
	if err != nil {
		t.Fatal(err)
	}
	m.Close()
	// A run that was killed leaves its segments, and whatever else.
	for _, left := range []string{filepath.Join(made, "encoding-123456", "0.ts"), filepath.Join(made, "left by a run")} {
		if err := os.MkdirAll(filepath.Dir(left), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(left, []byte("segment"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m, err = NewManager("ffmpeg-not-installed", made, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if left, err := os.ReadDir(made); err != nil || len(left) != 1 || left[0].Name() != workfolder.Marker {
		t.Errorf("after the next start it holds %v %v", left, err)
	}
}
