package admin

import (
	"context"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/backup"
	"github.com/moodiness/polyfin/internal/tasks"
)

// The backups kept default to 7, from 1 to 90; a PUT leaving them out
// keeps them.
func TestSettingsBackups(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	_, body, _ := administrator.call(http.MethodGet, "/settings", nil)
	if body["backupsKept"] != 7.0 || body["backupFolder"] != "" {
		t.Errorf("defaults: %v %q", body["backupsKept"], body["backupFolder"])
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	for _, kept := range []int{accounts.MinBackupsKept, accounts.MaxBackupsKept} {
		settings := maps.Clone(base)
		settings["backupsKept"] = kept
		if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK ||
			body["backupsKept"] != float64(kept) {
			t.Fatalf("saving %d: %d %v", kept, status, body)
		}
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || body["backupsKept"] != 90.0 {
		t.Errorf("left out: %d %v", status, body)
	}
	for _, kept := range []int{0, accounts.MaxBackupsKept + 1} {
		settings := maps.Clone(base)
		settings["backupsKept"] = kept
		if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusBadRequest ||
			body["error"] != "invalid_backups_kept" {
			t.Errorf("%d kept: %d %v", kept, status, body)
		}
	}
	if got := api.store.Settings().BackupsKept; got != 90 {
		t.Errorf("saved: %d", got)
	}
}

// While the settings leave backups off: no folder, no status in the
// health. Turned on, the last run's result shows, and is a problem when it
// failed or the last backup is older than two days.
func TestBackupStatus(t *testing.T) {
	t.Parallel()
	off := newTestAPI(t, 10).signedIn("administrator", true)
	if status, body, _ := off.call(http.MethodGet, "/backup", nil); status != http.StatusOK || body["folder"] != "" || body["next"] != nil {
		t.Errorf("off: %d %v", status, body)
	}
	if _, body, _ := off.call(http.MethodGet, "/health", nil); body["backup"] != nil {
		t.Errorf("health without backups: %v", body["backup"])
	}

	dir := t.TempDir()
	var deps testDeps
	registry := tasks.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	registry.Register(tasks.Task{Key: backup.TaskKey, Triggers: []tasks.Trigger{tasks.Daily(backup.DefaultTime)},
		Text: map[string]tasks.Text{"en": {Name: "Back up"}}, Run: func(context.Context) error { return nil }})
	registry.Start(t.Context())
	api := newTestAPI(t, 10, func(o *Options, d testDeps) {
		deps = d
		o.Tasks = registry
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
	// A weekly backup is stale after two weeks only.
	if err := registry.SetTriggers(t.Context(), tasks.ID(backup.TaskKey), []tasks.Trigger{tasks.Weekly(time.Sunday, backup.DefaultTime)}); err != nil {
		t.Fatal(err)
	}
	for age, problem := range map[time.Duration]bool{49 * time.Hour: false, 13 * 24 * time.Hour: false, 15 * 24 * time.Hour: true} {
		record(now.Add(-age), "", now.Add(-age))
		if _, body, _ := administrator.call(http.MethodGet, "/backup", nil); body["problem"] != problem {
			t.Errorf("weekly, %v old: %v", age, body)
		}
	}
}
