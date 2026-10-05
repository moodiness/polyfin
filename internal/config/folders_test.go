package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Recording and backups are on only with a folder Polyfin can write into;
// a folder it cannot use stops it from starting, rather than leaving them
// off.
func TestRecordingsAndBackupFoldersMustBeWritable(t *testing.T) {
	folders := map[string]func(Config) string{
		"POLYFIN_RECORDINGS_DIR": func(cfg Config) string { return cfg.RecordingsDir },
		"POLYFIN_BACKUP_DIR":     func(cfg Config) string { return cfg.BackupDir },
	}
	for variable, folder := range folders {
		t.Run(variable, func(t *testing.T) {
			base := map[string]string{"POLYFIN_DATABASE_URL": "postgresql://polyfin@db/polyfin"}
			if cfg, err := Load(env(base)); err != nil || folder(cfg) != "" {
				t.Fatalf("without a folder: %q %v", folder(cfg), err)
			}
			dir := t.TempDir()
			with := func(value string) map[string]string {
				values := map[string]string{variable: value}
				for k, v := range base {
					values[k] = v
				}
				return values
			}
			if cfg, err := Load(env(with(" " + dir + "/ "))); err != nil || folder(cfg) != dir {
				t.Fatalf("with a folder: %q %v", folder(cfg), err)
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
			for _, refused := range []string{"relative", filepath.Join(dir, "missing"), file, readOnly} {
				if refused == readOnly && os.Geteuid() == 0 {
					continue
				}
				if _, err := Load(env(with(refused))); err == nil || !strings.Contains(err.Error(), variable) {
					t.Errorf("%s: %v", refused, err)
				}
			}
		})
	}
}
