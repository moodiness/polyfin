package admin

import (
	"io"
	"log/slog"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/backup"
)

// The backup hour defaults to 4, from 0 to 23, and the backups kept to 7,
// from 1 to 90; a PUT leaving them out keeps them.
func TestSettingsBackups(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	_, body, _ := administrator.call(http.MethodGet, "/settings", nil)
	if body["backupHour"] != 4.0 || body["backupsKept"] != 7.0 || body["backupFolder"] != "" {
		t.Errorf("defaults: %v %v %q", body["backupHour"], body["backupsKept"], body["backupFolder"])
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	for _, values := range [][2]int{{0, accounts.MinBackupsKept}, {23, accounts.MaxBackupsKept}} {
		settings := maps.Clone(base)
		settings["backupHour"], settings["backupsKept"] = values[0], values[1]
		if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK ||
			body["backupHour"] != float64(values[0]) || body["backupsKept"] != float64(values[1]) {
			t.Fatalf("saving %v: %d %v", values, status, body)
		}
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK ||
		body["backupHour"] != 23.0 || body["backupsKept"] != 90.0 {
		t.Errorf("left out: %d %v", status, body)
	}
	for field, refused := range map[string][]int{"backupHour": {-1, 24}, "backupsKept": {0, accounts.MaxBackupsKept + 1}} {
		code := map[string]string{"backupHour": "invalid_backup_hour", "backupsKept": "invalid_backups_kept"}[field]
		for _, value := range refused {
			settings := maps.Clone(base)
			settings[field] = value
			if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusBadRequest || body["error"] != code {
				t.Errorf("%s %d: %d %v", field, value, status, body)
			}
		}
	}
	if store := api.store.Settings(); store.BackupHour != 23 || store.BackupsKept != 90 {
		t.Errorf("saved: %d %d", store.BackupHour, store.BackupsKept)
	}
}

// The collection read hour defaults to -1, never, and otherwise goes from
// 0 to 23; a PUT leaving it out keeps it.
func TestSettingsCollectionReadHour(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["collectionReadHour"] != -1.0 {
		t.Errorf("default: %v", body["collectionReadHour"])
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	for _, hour := range []int{0, 23} {
		settings := maps.Clone(base)
		settings["collectionReadHour"] = hour
		if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK ||
			body["collectionReadHour"] != float64(hour) {
			t.Fatalf("saving %d: %d %v", hour, status, body)
		}
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || body["collectionReadHour"] != 23.0 {
		t.Errorf("left out: %d %v", status, body)
	}
	for _, hour := range []int{-2, 24} {
		settings := maps.Clone(base)
		settings["collectionReadHour"] = hour
		if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusBadRequest ||
			body["error"] != "invalid_collection_read_hour" {
			t.Errorf("hour %d: %d %v", hour, status, body)
		}
	}
	if got := api.store.Settings().CollectionReadHour; got != 23 {
		t.Errorf("saved: %d", got)
	}
}

// While the settings leave backups off: no folder, no status in the
// health. Turned on, the last run's result shows, and is a problem when it
// failed or the last backup is older than two days.
func TestBackupStatus(t *testing.T) {
	off := newTestAPI(t, 10).signedIn("administrator", true)
	if status, body, _ := off.call(http.MethodGet, "/backup", nil); status != http.StatusOK || body["folder"] != "" || body["next"] != nil {
		t.Errorf("off: %d %v", status, body)
	}
	if _, body, _ := off.call(http.MethodGet, "/health", nil); body["backup"] != nil {
		t.Errorf("health without backups: %v", body["backup"])
	}

	dir := t.TempDir()
	var deps testDeps
	api := newTestAPI(t, 10, func(o *Options, d testDeps) {
		deps = d
		settings := o.Accounts.Settings
		dataDir := o.DataDir
		o.Backups = backup.New(backup.Config{Folder: func() string { return settings().BackupDir(dataDir) }, DB: d.pool, Settings: settings,
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	})
	administrator := api.signedIn("administrator", true)
	if status, body, _ := administrator.call(http.MethodPut, "/settings", map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true,
		"language": "en", "backups": true, "backupFolder": dir}); status != http.StatusOK || body["backupFolder"] != dir || body["backups"] != true {
		t.Fatalf("turning backups on: %d %v", status, body)
	}
	status, body, _ := administrator.call(http.MethodGet, "/backup", nil)
	if status != http.StatusOK || body["folder"] != dir || body["next"] == nil || body["ranAt"] != nil || body["problem"] != false {
		t.Errorf("before the first backup: %d %v", status, body)
	}
	_, health, _ := administrator.call(http.MethodGet, "/health", nil)
	if backupHealth, _ := health["backup"].(map[string]any); backupHealth == nil || backupHealth["folder"] != dir {
		t.Errorf("health: %v", health["backup"])
	}
	disks, _ := health["disks"].([]any)
	found := false
	for _, disk := range disks {
		found = found || disk.(map[string]any)["folder"] == "backups"
	}
	if !found {
		t.Errorf("no backups disk: %v", disks)
	}

	record := func(ranAt time.Time, failure string, madeAt time.Time) {
		t.Helper()
		if _, err := deps.pool.Exec(t.Context(), `INSERT INTO backup_status (ran_at, error, made_at, file, size)
			VALUES ($1, $2, $3, 'polyfin-20261005-040000.dump', 1234)
			ON CONFLICT (singleton) DO UPDATE SET ran_at = excluded.ran_at, error = excluded.error, made_at = excluded.made_at`,
			ranAt, failure, madeAt); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	for _, test := range []struct {
		name          string
		ranAt, madeAt time.Time
		failure       string
		problem       bool
	}{
		{name: "recent", ranAt: now.Add(-time.Hour), madeAt: now.Add(-time.Hour), problem: false},
		{name: "failed", ranAt: now.Add(-time.Hour), madeAt: now.Add(-25 * time.Hour), failure: "pg_dump failed", problem: true},
		{name: "stale", ranAt: now.Add(-49 * time.Hour), madeAt: now.Add(-49 * time.Hour), problem: true},
	} {
		record(test.ranAt, test.failure, test.madeAt)
		_, body, _ := administrator.call(http.MethodGet, "/backup", nil)
		if body["problem"] != test.problem || body["error"] != test.failure || body["file"] != "polyfin-20261005-040000.dump" || body["size"] != 1234.0 {
			t.Errorf("%s: %v", test.name, body)
		}
		_, health, _ := administrator.call(http.MethodGet, "/health", nil)
		if backupHealth, _ := health["backup"].(map[string]any); backupHealth == nil || backupHealth["problem"] != test.problem {
			t.Errorf("%s health: %v", test.name, health["backup"])
		}
	}
}
