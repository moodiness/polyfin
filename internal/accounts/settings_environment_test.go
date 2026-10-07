package accounts

import (
	"errors"
	"strings"
	"testing"
)

// The settings that were options of the environment start at their
// defaults: a 10 GB cache, each render node in turn, recording and backups
// off into the default folders.
func TestEnvironmentSettingsDefaults(t *testing.T) {
	got := newStore(t).Settings()
	if got.CacheSizeGB != 10 || got.VAAPIDevice != "" || got.Recording || got.RecordingsFolder != "" || got.Backups || got.BackupFolder != "" {
		t.Errorf("defaults: %+v", got)
	}
	if dir := got.RecordingsDir("/data"); dir != "" {
		t.Errorf("recordings folder while off: %q", dir)
	}
	if dir := got.BackupDir("/data"); dir != "" {
		t.Errorf("backup folder while off: %q", dir)
	}
	got.Recording, got.Backups = true, true
	if got.RecordingsDir("/data") != "/data/recordings" || got.BackupDir("/data") != "/data/backups" {
		t.Errorf("default folders: %q %q", got.RecordingsDir("/data"), got.BackupDir("/data"))
	}
	got.RecordingsFolder, got.BackupFolder = "/media/recordings", "/media/backups"
	if got.RecordingsDir("/data") != "/media/recordings" || got.BackupDir("/data") != "/media/backups" {
		t.Errorf("chosen folders: %q %q", got.RecordingsDir("/data"), got.BackupDir("/data"))
	}
}

func TestEnvironmentSettingsAreChecked(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	base := store.Settings()
	for _, tc := range []struct {
		name   string
		change func(*Settings)
		err    error
	}{
		{"a cache of 0 GB", func(s *Settings) { s.CacheSizeGB = 0 }, ErrInvalidCacheSize},
		{"a cache of 1 GB", func(s *Settings) { s.CacheSizeGB = 1 }, nil},
		{"a cache of 2000 GB", func(s *Settings) { s.CacheSizeGB = 2000 }, nil},
		{"a cache of 2001 GB", func(s *Settings) { s.CacheSizeGB = 2001 }, ErrInvalidCacheSize},
		{"each render node", func(s *Settings) { s.VAAPIDevice = "" }, nil},
		{"a render node", func(s *Settings) { s.VAAPIDevice = "/dev/dri/renderD128" }, nil},
		{"a card", func(s *Settings) { s.VAAPIDevice = "/dev/dri/card0" }, ErrInvalidVAAPIDevice},
		{"a render node's name", func(s *Settings) { s.VAAPIDevice = "renderD128" }, ErrInvalidVAAPIDevice},
		{"the default recordings folder", func(s *Settings) { s.RecordingsFolder = "" }, nil},
		{"a relative recordings folder", func(s *Settings) { s.RecordingsFolder = "relative/path" }, ErrInvalidRecordingsFolder},
		{"a recordings folder with a control character", func(s *Settings) { s.RecordingsFolder = "/a\nb" }, ErrInvalidRecordingsFolder},
		{"a recordings folder too long", func(s *Settings) { s.RecordingsFolder = "/" + strings.Repeat("a", MaxFolderBytes) }, ErrInvalidRecordingsFolder},
		{"the default backup folder", func(s *Settings) { s.BackupFolder = "" }, nil},
		{"a relative backup folder", func(s *Settings) { s.BackupFolder = "relative/path" }, ErrInvalidBackupFolder},
		{"a backup folder with a control character", func(s *Settings) { s.BackupFolder = "/a\x7fb" }, ErrInvalidBackupFolder},
		{"a backup folder too long", func(s *Settings) { s.BackupFolder = "/" + strings.Repeat("a", MaxFolderBytes) }, ErrInvalidBackupFolder},
	} {
		changed := base
		tc.change(&changed)
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, tc.err) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.err)
		}
	}
	// Folders are saved cleaned, and kept after a restart.
	changed := base
	changed.CacheSizeGB, changed.VAAPIDevice = 25, "/dev/dri/renderD129"
	changed.Recording, changed.RecordingsFolder = true, "/a/../b"
	changed.Backups, changed.BackupFolder = true, "/backups/"
	saved, err := store.UpdateSettings(ctx, changed)
	if err != nil || saved.RecordingsFolder != "/b" || saved.BackupFolder != "/backups" {
		t.Fatalf("saved: %+v %v", saved, err)
	}
	reopened, err := Open(ctx, store.db)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Settings()
	if got.CacheSizeGB != 25 || got.VAAPIDevice != "/dev/dri/renderD129" || !got.Recording || got.RecordingsFolder != "/b" ||
		!got.Backups || got.BackupFolder != "/backups" {
		t.Errorf("after reopening: %+v", got)
	}
}

// The database refuses what the settings refuse.
func TestEnvironmentSettingsDatabaseChecks(t *testing.T) {
	store := newStore(t)
	for _, change := range []string{"cache_size_gb = 0", "cache_size_gb = 2001", "vaapi_device = '/dev/dri/card0'",
		"recordings_folder = 'relative'", "recordings_folder = '/a' || chr(10)", "recordings_folder = '/' || repeat('a', 512)",
		"backup_folder = 'relative'", "backup_folder = '/a' || chr(9)", "backup_folder = '/' || repeat('a', 512)",
		"environment_pending = '{language}'"} {
		if _, err := store.db.Exec(t.Context(), "UPDATE settings SET "+change); err == nil {
			t.Errorf("the database took %s", change)
		}
	}
	for _, change := range []string{"cache_size_gb = 2000", "vaapi_device = '/dev/dri/renderD128'", "recordings_folder = '/media'",
		"environment_pending = '{cache_size_gb,vaapi_device,recording,backups,detailed_log}'"} {
		if _, err := store.db.Exec(t.Context(), "UPDATE settings SET "+change); err != nil {
			t.Errorf("the database refused %s: %v", change, err)
		}
	}
}

// The variables still set are copied once, at the first start: a folder
// turns its feature on with it.
func TestEnvironmentSettingsAreAdoptedOnce(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	env := Environment{CacheSizeGB: 20, DetailedLog: true, VAAPIDevice: "/dev/dri/renderD129",
		RecordingsFolder: "/media/recordings/", BackupFolder: "/media/backups"}
	if err := store.AdoptEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	check := func(when string) {
		t.Helper()
		reopened, err := Open(ctx, store.db)
		if err != nil {
			t.Fatal(err)
		}
		for _, got := range []Settings{store.Settings(), reopened.Settings()} {
			if got.CacheSizeGB != 20 || !got.DetailedLog || got.VAAPIDevice != "/dev/dri/renderD129" ||
				!got.Recording || got.RecordingsFolder != "/media/recordings" || !got.Backups || got.BackupFolder != "/media/backups" {
				t.Errorf("%s: %+v", when, got)
			}
		}
	}
	check("the first start")
	// A second start with other values changes nothing.
	if err := store.AdoptEnvironment(ctx, Environment{CacheSizeGB: 5, VAAPIDevice: "/dev/dri/renderD130", RecordingsFolder: "/other"}); err != nil {
		t.Fatal(err)
	}
	check("a second start")
}

// Without variables, the settings keep their defaults; the detailed log
// is copied only when POLYFIN_LOG_LEVEL asked for debug.
func TestEnvironmentSettingsKeepTheirValuesWithoutVariables(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	if err := store.AdoptEnvironment(ctx, Environment{}); err != nil {
		t.Fatal(err)
	}
	if got := store.Settings(); got.CacheSizeGB != DefaultCacheSizeGB || got.VAAPIDevice != "" || got.Recording || got.Backups ||
		got.DetailedLog || got.HardwareAcceleration != "auto" || len(got.SegmentSourcesOff) != 0 {
		t.Errorf("without variables: %+v", got)
	}
	// The detailed log turned on before stays on when the variable asked
	// for no debug log.
	other := newStore(t)
	if _, err := other.db.Exec(ctx, "UPDATE settings SET detailed_log = true"); err != nil {
		t.Fatal(err)
	}
	if err := other.AdoptEnvironment(ctx, Environment{DetailedLog: false}); err != nil {
		t.Fatal(err)
	}
	if !other.Settings().DetailedLog {
		t.Error("the detailed log was turned off")
	}
}
