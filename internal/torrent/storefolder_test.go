package torrent

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/moodiness/polyfin/internal/workfolder"
)

// The folder of the torrents' pieces is emptied at start only when
// Polyfin made it: one that holds other files, as a download program's
// own folder of that name, is left as it is and refused, and one with its
// marker loses what a run left there.
func TestAStoreEmptiesOnlyAFolderPolyfinMade(t *testing.T) {
	t.Parallel()
	budget := func() int64 { return 1 << 20 }
	logger := slog.New(slog.DiscardHandler)
	dir := filepath.Join(t.TempDir(), "torrents")
	kept := filepath.Join(dir, "holidays", "2019.mkv")
	if err := os.MkdirAll(filepath.Dir(kept), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kept, []byte("the administrator's own file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if s, err := newStore(dir, budget, logger); !errors.Is(err, workfolder.ErrOccupied) || s != nil {
		t.Errorf("a folder with other files: %v", err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("the administrator's file is gone: %v", err)
	}

	made := filepath.Join(t.TempDir(), "torrents")
	if _, err := newStore(made, budget, logger); err != nil {
		t.Fatal(err)
	}
	// A run that was killed leaves its pieces, and whatever else.
	for _, left := range []string{filepath.Join(made, "0123456789abcdef0123456789abcdef01234567", "0"), filepath.Join(made, "left by a run")} {
		if err := os.MkdirAll(filepath.Dir(left), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(left, []byte("piece"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := newStore(made, budget, logger); err != nil {
		t.Fatal(err)
	}
	if left, err := os.ReadDir(made); err != nil || len(left) != 1 || left[0].Name() != workfolder.Marker {
		t.Errorf("after the next start it holds %v %v", left, err)
	}
}
