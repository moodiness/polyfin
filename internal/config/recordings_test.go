package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Recording is on only with a folder Polyfin can write into; a folder it
// cannot use stops it from starting, rather than leaving recording off.
func TestRecordingsFolderMustBeWritable(t *testing.T) {
	base := map[string]string{"POLYFIN_DATABASE_URL": "postgresql://polyfin@db/polyfin"}
	if cfg, err := Load(env(base)); err != nil || cfg.RecordingsDir != "" {
		t.Fatalf("without a folder: %q %v", cfg.RecordingsDir, err)
	}
	dir := t.TempDir()
	with := func(value string) map[string]string {
		values := map[string]string{"POLYFIN_RECORDINGS_DIR": value}
		for k, v := range base {
			values[k] = v
		}
		return values
	}
	if cfg, err := Load(env(with(" " + dir + "/ "))); err != nil || cfg.RecordingsDir != dir {
		t.Fatalf("with a folder: %q %v", cfg.RecordingsDir, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("the check left files: %v", entries)
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	readOnly := filepath.Join(dir, "read-only")
	if err := os.Mkdir(readOnly, 0o500); err != nil {
		t.Fatal(err)
	}
	for _, refused := range []string{"recordings", filepath.Join(dir, "missing"), file, readOnly} {
		if refused == readOnly && os.Geteuid() == 0 {
			continue
		}
		if _, err := Load(env(with(refused))); err == nil || !strings.Contains(err.Error(), "POLYFIN_RECORDINGS_DIR") {
			t.Errorf("%s: %v", refused, err)
		}
	}
}
