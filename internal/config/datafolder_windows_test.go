package config

import (
	"os"
	"path/filepath"
	"testing"
)

// On Windows a folder has neither an owner nor a mode Polyfin can read:
// the data folder is made when missing, and one that is there is taken
// whoever Polyfin runs as, with no fault told; what is no folder still
// stops the start.
func TestOnWindowsTheDataFolderIsNotJudgedByItsOwnerOrMode(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "config", "polyfin-server")
	for _, strict := range []bool{false, true} {
		for _, self := range []int{os.Geteuid(), 0, 1000} {
			if fault, err := prepareDataFolder(dir, strict, self, false); err != nil || fault != nil {
				t.Errorf("as user %d: %v, %v (strict %v)", self, fault, err, strict)
			}
		}
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("the data folder was not made: %v", err)
	}
	if fault, err := PrepareDataFolder(dir, true); err != nil || fault != nil {
		t.Errorf("in a container: %v, %v", fault, err)
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareDataFolder(file, false, os.Geteuid(), false); err == nil {
		t.Error("a file is taken for the data folder")
	}
}
