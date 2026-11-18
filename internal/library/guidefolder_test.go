package library

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/moodiness/polyfin/internal/workfolder"
)

// The folder guides are spooled in is emptied at start only when Polyfin
// made it: one that holds other files is left as it is and refused, and
// one with its marker loses what a run left there.
func TestGuidesAreSpooledOnlyInAFolderPolyfinMade(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "guides")
	kept := filepath.Join(dir, "city", "2019.pdf")
	if err := os.MkdirAll(filepath.Dir(kept), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kept, []byte("the administrator's own file"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := &Service{}
	if err := service.SpoolGuidesIn(dir); !errors.Is(err, workfolder.ErrOccupied) || service.guideDir != "" {
		t.Errorf("a folder with other files: %v, spooling in %q", err, service.guideDir)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("the administrator's file is gone: %v", err)
	}

	made := filepath.Join(t.TempDir(), "guides")
	if err := service.SpoolGuidesIn(made); err != nil || service.guideDir != made {
		t.Fatalf("a new folder: %v, spooling in %q", err, service.guideDir)
	}
	if err := os.WriteFile(filepath.Join(made, "left by a run"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (&Service{}).SpoolGuidesIn(made); err != nil {
		t.Fatal(err)
	}
	if left, err := os.ReadDir(made); err != nil || len(left) != 1 || left[0].Name() != workfolder.Marker {
		t.Errorf("after the next start it holds %v %v", left, err)
	}
}
