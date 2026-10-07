package config

import (
	"testing"
)

// POLYFIN_RECORDINGS_DIR and POLYFIN_BACKUP_DIR give their folder, cleaned,
// to the settings; the folder is not looked at on disk, and a path that is
// not absolute is left out with a warning.
func TestRecordingsAndBackupFoldersAreCopied(t *testing.T) {
	for variable, folder := range map[string]func(Config) string{
		"POLYFIN_RECORDINGS_DIR": func(cfg Config) string { return cfg.Environment.RecordingsFolder },
		"POLYFIN_BACKUP_DIR":     func(cfg Config) string { return cfg.Environment.BackupFolder },
	} {
		t.Run(variable, func(t *testing.T) {
			if cfg, err := Load(with(nil)); err != nil || folder(cfg) != "" {
				t.Fatalf("without a folder: %q %v", folder(cfg), err)
			}
			if cfg, err := Load(with(map[string]string{variable: " /missing/folder/ "})); err != nil || folder(cfg) != "/missing/folder" || len(cfg.Warnings) != 0 {
				t.Errorf("with a folder: %q %v %v", folder(cfg), err, cfg.Warnings)
			}
			if cfg, err := Load(with(map[string]string{variable: "relative"})); err != nil || folder(cfg) != "" || len(cfg.Warnings) != 1 {
				t.Errorf("relative: %q %v %v", folder(cfg), err, cfg.Warnings)
			}
		})
	}
}
