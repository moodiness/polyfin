package jellyfin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/backup"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/testdb"
)

// fakePgDump writes a file to its --file, as pg_dump writes a backup.
const fakePgDump = `#!/bin/sh
for arg; do
	case $arg in --file=*) file=${arg#--file=} ;; esac
done
printf 'PGDMP' > "$file"
`

// backupServer is a test server backing up into dir, with an
// administrator signed in on "dashboard" and a member, alice, on "tv";
// configure completes the backup service's configuration.
func backupServer(t *testing.T, dir string, configure func(*backup.Config, *pgxpool.Pool)) (s testServer, admin, member string,
	service *backup.Service) {
	t.Helper()
	s = newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, pool *pgxpool.Pool) {
		config := backup.Config{Folder: func() string { return dir }, DB: pool, PgRestore: "pg-restore-not-installed",
			Settings: o.Accounts.Settings, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
		if configure != nil {
			configure(&config, pool)
		}
		service = backup.New(config)
		o.Backups = service
	})
	s.user("root", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.user("alice", nil)
	return s, app("dashboard", s.signIn("root", "dashboard")), app("tv", s.signIn("alice", "tv")), service
}

// While backups are off there are none, and none can be made or restored.
// On, /Backup lists the backups of the folder, newest first, as Jellyfin
// 12.2's manifests of database backups; /Backup/Create makes one as Back
// up the database does; /Backup/Manifest describes one. Administrators
// only.
func TestBackupsAreThoseOfTheFolder(t *testing.T) {
	t.Parallel()
	off, admin, member := administrated(t)
	for _, c := range []struct {
		method, path string
		body         any
		status       int
		want         string
	}{
		{"GET", "/Backup", nil, 200, "[]"},
		{"POST", "/Backup/Create", map[string]any{}, 403, ""},
		{"GET", "/Backup/Manifest?path=x", nil, 404, `"Not Found"`},
		{"GET", "/Backup/Manifest", nil, 400, "path"},
		{"POST", "/Backup/Restore", map[string]string{"ArchiveFileName": "x.zip"}, 404, `"Not Found"`},
	} {
		status, body := off.call(c.method, c.path, admin, c.body)
		if status != c.status || !strings.Contains(string(body), c.want) {
			t.Errorf("off, %s %s: %d %s", c.method, c.path, status, body)
		}
		if c.path == "/Backup" {
			matchesFixture(t, "backups", body, shapeRules{})
		}
		if status, body := off.call(c.method, c.path, member, c.body); status != http.StatusForbidden || len(body) != 0 {
			t.Errorf("off, %s %s for a member: %d %q", c.method, c.path, status, body)
		}
	}

	dir := t.TempDir()
	for _, name := range []string{"polyfin-20261001-040000.dump", "polyfin-20261002-050000-before-restore.dump", "notes.txt",
		".polyfin-20261003-040000.dump.partial"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("PGDMP"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	script := filepath.Join(t.TempDir(), "pg_dump")
	if err := os.WriteFile(script, []byte(fakePgDump), 0o700); err != nil {
		t.Fatal(err)
	}
	s, admin, member, service := backupServer(t, dir, func(c *backup.Config, _ *pgxpool.Pool) { c.PgDump = script })
	status, body := s.call(http.MethodGet, "/Backup", admin, nil)
	var manifests []map[string]any
	if err := json.Unmarshal(body, &manifests); err != nil || status != http.StatusOK || len(manifests) != 2 {
		t.Fatalf("on: %d %s", status, body)
	}
	made := time.Date(2026, 10, 2, 5, 0, 0, 0, time.Local).UTC().Format("2006-01-02T15:04:05.0000000Z")
	first := manifests[0]
	if first["Path"] != "polyfin-20261002-050000-before-restore.dump" || first["DateCreated"] != made || first["ServerVersion"] != Version ||
		first["BackupEngineVersion"] == "" {
		t.Errorf("newest: %v", first)
	}
	if options, _ := first["Options"].(map[string]any); options["Database"] != true || options["Metadata"] != false ||
		options["Trickplay"] != false || options["Subtitles"] != false {
		t.Errorf("options: %v", first["Options"])
	}
	if status, body := s.call(http.MethodGet, "/Backup/Manifest?path=polyfin-20261001-040000.dump", admin, nil); status != http.StatusOK ||
		!strings.Contains(string(body), `"Path":"polyfin-20261001-040000.dump"`) {
		t.Errorf("manifest: %d %s", status, body)
	}
	for _, path := range []string{"notes.txt", ".polyfin-20261003-040000.dump.partial", "../polyfin-20261001-040000.dump"} {
		if status, _ := s.call(http.MethodGet, "/Backup/Manifest?path="+path, admin, nil); status != http.StatusNotFound {
			t.Errorf("manifest of %s: %d", path, status)
		}
	}
	if status, body := s.call(http.MethodGet, "/Backup", member, nil); status != http.StatusForbidden || len(body) != 0 {
		t.Errorf("a member: %d %s", status, body)
	}

	status, body = s.call(http.MethodPost, "/Backup/Create", admin, map[string]any{"Metadata": true, "Database": false})
	var created map[string]any
	if err := json.Unmarshal(body, &created); err != nil || status != http.StatusOK {
		t.Fatalf("create: %d %s", status, body)
	}
	backupStatus, _ := service.Status(t.Context())
	if created["Path"] != backupStatus.File || backupStatus.File == "" {
		t.Errorf("created %v, made %+v", created, backupStatus)
	}
	if _, err := os.Stat(filepath.Join(dir, backupStatus.File)); err != nil {
		t.Errorf("the backup made: %v", err)
	}
	if _, body := s.call(http.MethodGet, "/Backup", admin, nil); strings.Count(string(body), `"Path"`) != 3 {
		t.Errorf("after creating: %s", body)
	}

	// A restore of a backup the folder does not have, or without
	// pg_restore, is refused, and Polyfin keeps running.
	if status, _ := s.call(http.MethodPost, "/Backup/Restore", admin, map[string]string{"ArchiveFileName": "polyfin-20200101-000000.dump"}); status != http.StatusNotFound {
		t.Errorf("a missing backup: %d", status)
	}
	if status, _ := s.call(http.MethodPost, "/Backup/Restore", admin, map[string]string{"ArchiveFileName": "polyfin-20261001-040000.dump"}); status != http.StatusBadRequest {
		t.Errorf("without pg_restore: %d", status)
	}
	if status, _ := s.call(http.MethodPost, "/Backup/Restore", member, map[string]string{"ArchiveFileName": "polyfin-20261001-040000.dump"}); status != http.StatusForbidden {
		t.Errorf("a member's restore: %d", status)
	}
	select {
	case <-service.Requested():
		t.Error("a refused restore stops Polyfin")
	default:
	}
}

// /Backup/Restore restores a backup of the folder as the admin app does:
// Polyfin backs up the current database, then stops to restore it. A
// backup holding stored keys the current POLYFIN_SECRET_KEY cannot decrypt
// is refused with 400, as apps cannot show that warning.
func TestAppsRestoreABackupAsTheAdminAppDoes(t *testing.T) {
	t.Parallel()
	pgDump, pgRestore := os.Getenv("POLYFIN_TEST_PG_DUMP"), os.Getenv("POLYFIN_TEST_PG_RESTORE")
	if pgDump == "" || pgRestore == "" {
		t.Skip("POLYFIN_TEST_PG_DUMP and POLYFIN_TEST_PG_RESTORE name no pg_dump and pg_restore of PostgreSQL 18")
	}
	box, err := secrets.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	other, _ := secrets.New([]byte("fedcba9876543210fedcba9876543210"))
	dir := t.TempDir()
	var pool *pgxpool.Pool
	s, admin, _, service := backupServer(t, dir, func(c *backup.Config, p *pgxpool.Pool) {
		pool = p
		c.PgDump, c.PgRestore, c.Secrets = pgDump, pgRestore, box
		c.DatabaseURL = testdb.CopyURL(t, p)
	})
	if _, err := pool.Exec(t.Context(), "UPDATE settings SET smtp_password = $1", other.Seal("smtp")); err != nil {
		t.Fatal(err)
	}
	if err := service.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	sealed, _ := service.Status(t.Context())
	if status, body := s.call(http.MethodPost, "/Backup/Restore", admin, map[string]string{"ArchiveFileName": sealed.File}); status != http.StatusBadRequest {
		t.Errorf("another key: %d %s", status, body)
	}
	select {
	case <-service.Requested():
		t.Fatal("a backup with another key is restored")
	default:
	}

	if _, err := pool.Exec(t.Context(), "UPDATE settings SET smtp_password = ''"); err != nil {
		t.Fatal(err)
	}
	// Backups are named after the second they are made in: the next may
	// take the name of this one.
	if err := os.Remove(filepath.Join(dir, sealed.File)); err != nil {
		t.Fatal(err)
	}
	if err := service.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	readable, _ := service.Status(t.Context())
	if status, body := s.call(http.MethodPost, "/Backup/Restore", admin, map[string]string{"ArchiveFileName": readable.File}); status != http.StatusNoContent {
		t.Fatalf("restore: %d %s", status, body)
	}
	select {
	case plan := <-service.Requested():
		if plan.File != readable.File || plan.By != "root" || !strings.HasSuffix(plan.BeforeRestore, "-before-restore.dump") {
			t.Errorf("plan: %+v", plan)
		}
		if _, err := os.Stat(filepath.Join(dir, plan.BeforeRestore)); err != nil {
			t.Errorf("before-restore backup: %v", err)
		}
	default:
		t.Error("Polyfin is not asked to stop")
	}
}
