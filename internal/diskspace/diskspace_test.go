//go:build linux || darwin || windows

package diskspace

import (
	"path/filepath"
	"strings"
	"testing"
)

// A folder's disk is measured on each system Polyfin runs on: room free and
// used, and the folder the disk is mounted on, which holds the folder.
func TestMeasureAFolder(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	free, used, mount, ok := Measure(dir)
	if !ok || free <= 0 || used <= 0 || mount == "" {
		t.Fatalf("Measure(%q) = %d, %d, %q, %v", dir, free, used, mount, ok)
	}
	// filepath.Rel compares names in any letter case on Windows.
	if rel, err := filepath.Rel(mount, dir); err != nil || strings.HasPrefix(rel, "..") {
		t.Errorf("mounted on %q, which does not hold %q", mount, dir)
	}
	// A folder that does not exist is not measured.
	if _, _, _, ok := Measure(filepath.Join(dir, "missing")); ok {
		t.Error("a missing folder was measured")
	}
}
