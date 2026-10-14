package admin

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/webdav"

	"github.com/moodiness/polyfin/internal/backup"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/testdb"
)

// backupAPI is an admin API backing up into dir, sealing secrets with
// key; configure completes the backup service's configuration.
func backupAPI(t *testing.T, dir string, configure func(*backup.Config, testDeps)) (testAPI, testDeps, *backup.Service) {
	t.Helper()
	box, err := secrets.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	var deps testDeps
	var service *backup.Service
	api := newTestAPI(t, 10, func(o *Options, d testDeps) {
		deps = d
		config := backup.Config{Folder: func() string { return dir }, DB: d.pool, Secrets: box, Settings: o.Accounts.Settings,
			PgRestore: "pg-restore-not-installed", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
		if configure != nil {
			configure(&config, d)
		}
		service = backup.New(config)
		o.Backups = service
	})
	return api, deps, service
}

// Administrators add, change, test and delete destinations; their
// passwords and keys are never answered, and what cannot work is refused
// with a code for the field.
func TestBackupDestinationRoutes(t *testing.T) {
	t.Parallel()
	davDir := t.TempDir()
	handler := &webdav.Handler{FileSystem: webdav.Dir(davDir), LockSystem: webdav.NewMemLS()}
	dav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, password, ok := r.BasicAuth(); !ok || user != "polyfin" || password != "dav password" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer dav.Close()

	api, _, _ := backupAPI(t, t.TempDir(), nil)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)

	status, body, _ := administrator.call(http.MethodPost, "/backup/destinations", map[string]any{"kind": "webdav", "address": dav.URL + "/",
		"username": "polyfin", "secret": "dav password", "kept": 3})
	encoded, _ := json.Marshal(body)
	if status != http.StatusCreated || body["secretSet"] != true || body["kept"] != 3.0 || body["name"] != dav.URL+"/" ||
		strings.Contains(string(encoded), "dav password") {
		t.Fatalf("created: %d %s", status, encoded)
	}
	id := body["id"].(string)
	status, body, _ = administrator.call(http.MethodGet, "/backup/destinations", nil)
	if destinations, _ := body["destinations"].([]any); status != http.StatusOK || len(destinations) != 1 ||
		!strings.Contains(string(mustJSON(body["kinds"])), `["s3","sftp","webdav"]`) {
		t.Errorf("list: %d %v", status, body)
	}
	for _, refused := range []struct {
		body map[string]any
		code string
	}{
		{map[string]any{"kept": 91}, "invalid_backups_kept"},
		{map[string]any{"address": "nas.lan/dav"}, "invalid_destination_address"},
	} {
		if status, body, _ := administrator.call(http.MethodPatch, "/backup/destinations/"+id, refused.body); status != http.StatusBadRequest ||
			body["error"] != refused.code {
			t.Errorf("%v: %d %v", refused.body, status, body)
		}
	}
	for _, refused := range []struct {
		body map[string]any
		code string
	}{
		{map[string]any{"kind": "ftp", "address": "ftp.lan"}, "invalid_destination_kind"},
		{map[string]any{"kind": "s3", "address": "https://s3.example", "username": "a", "secret": "b"}, "invalid_bucket"},
		{map[string]any{"kind": "sftp", "address": "nas.lan", "username": "a", "privateKey": "nope"}, "invalid_private_key"},
		{map[string]any{"kind": "sftp", "address": "nas.lan", "username": "a"}, "invalid_destination_secret"},
	} {
		if status, body, _ := administrator.call(http.MethodPost, "/backup/destinations", refused.body); status != http.StatusBadRequest ||
			body["error"] != refused.code {
			t.Errorf("%v: %d %v", refused.body, status, body)
		}
	}

	if status, body, _ := administrator.call(http.MethodPost, "/backup/destinations/"+id+"/test", nil); status != http.StatusOK ||
		body["ok"] != true {
		t.Errorf("test: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPatch, "/backup/destinations/"+id, map[string]any{"secret": "wrong"}); status != http.StatusOK ||
		body["secretSet"] != true {
		t.Errorf("new password: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPost, "/backup/destinations/"+id+"/test", nil); status != http.StatusOK ||
		body["ok"] != false || !strings.Contains(body["error"].(string), "401") {
		t.Errorf("test with a wrong password: %d %v", status, body)
	}
	if entries, _ := os.ReadDir(davDir); len(entries) != 0 {
		t.Errorf("the tests left %v", entries)
	}

	for _, request := range []struct{ method, path string }{
		{http.MethodGet, "/backup/destinations"}, {http.MethodPost, "/backup/destinations"},
		{http.MethodPost, "/backup/destinations/" + id + "/test"}, {http.MethodDelete, "/backup/destinations/" + id},
		{http.MethodGet, "/backup/files"}, {http.MethodPost, "/backup/restore/prepare"}, {http.MethodPost, "/backup/restore"},
	} {
		if status, _, _ := member.call(request.method, request.path, map[string]any{}); status != http.StatusForbidden {
			t.Errorf("a member's %s %s: %d", request.method, request.path, status)
		}
	}

	if status, _, _ := administrator.call(http.MethodDelete, "/backup/destinations/"+id, nil); status != http.StatusNoContent {
		t.Errorf("delete: %d", status)
	}
	if status, _, _ := administrator.call(http.MethodDelete, "/backup/destinations/"+id, nil); status != http.StatusNotFound {
		t.Errorf("delete twice: %d", status)
	}
}

func mustJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}

// A destination whose last upload failed is an error on Health, one whose
// newest backup is older than two days a warning, both with its name, in
// the backups' status too; notifications tell them.
func TestBackupDestinationProblemsOnHealth(t *testing.T) {
	t.Parallel()
	api, deps, _ := backupAPI(t, t.TempDir(), nil)
	administrator := api.signedIn("administrator", true)
	status, body, _ := administrator.call(http.MethodPost, "/backup/destinations", map[string]any{"kind": "webdav",
		"address": "https://nas.lan/dav/", "username": "polyfin", "secret": "password"})
	if status != http.StatusCreated {
		t.Fatalf("created: %d %v", status, body)
	}
	find := func(code string) map[string]any {
		t.Helper()
		_, body, _ := administrator.call(http.MethodGet, "/health/problems?language=en", nil)
		problems, _ := body["problems"].([]any)
		for _, p := range problems {
			if problem := p.(map[string]any); problem["code"] == code {
				return problem
			}
		}
		return nil
	}
	if find("backup_destination_failed") != nil || find("backup_destination_stale") != nil {
		t.Error("a problem before the first upload")
	}

	if _, err := deps.pool.Exec(t.Context(), `UPDATE backup_destinations SET ran_at = now(), error = 'The WebDAV server answered 507',
		newest_at = now() - interval '1 day'`); err != nil {
		t.Fatal(err)
	}
	failed := find("backup_destination_failed")
	if failed == nil || failed["tone"] != "error" || failed["name"] != "https://nas.lan/dav/" {
		t.Errorf("failed: %v", failed)
	}
	_, backupBody, _ := administrator.call(http.MethodGet, "/backup", nil)
	destinations, _ := backupBody["destinations"].([]any)
	if len(destinations) != 1 || destinations[0].(map[string]any)["problem"] != "failed" ||
		destinations[0].(map[string]any)["error"] != "The WebDAV server answered 507" {
		t.Errorf("backup status: %v", backupBody["destinations"])
	}
	text := healthProblemJSON{Code: problemDestinationFailed, Name: "https://nas.lan/dav/"}
	if got := text.text("fr"); got != "https://nas.lan/dav/ : le dernier envoi des sauvegardes de la base de données a échoué." {
		t.Errorf("notification: %q", got)
	}

	if _, err := deps.pool.Exec(t.Context(), "UPDATE backup_destinations SET error = '', newest_at = now() - interval '3 days'"); err != nil {
		t.Fatal(err)
	}
	if stale := find("backup_destination_stale"); stale == nil || stale["tone"] != "warning" || find("backup_destination_failed") != nil {
		t.Errorf("stale: %v", stale)
	}
}

// A restore is prepared from a backup of the folder or a file sent, and
// started once the server's name is typed: without pg_restore, or for a
// backup the folder does not have, nothing is prepared.
func TestRestoreRoutes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "polyfin-20261001-040000.dump"), []byte("PGDMP"), 0o600); err != nil {
		t.Fatal(err)
	}
	api, _, _ := backupAPI(t, dir, nil)
	administrator := api.signedIn("administrator", true)
	if status, body, _ := administrator.call(http.MethodPost, "/backup/restore/prepare", map[string]any{"file": "polyfin-20200101-000000.dump"}); status != http.StatusNotFound ||
		body["error"] != "backup_not_found" {
		t.Errorf("a missing backup: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPost, "/backup/restore/prepare", map[string]any{"file": "polyfin-20261001-040000.dump"}); status != http.StatusConflict ||
		body["error"] != "pg_restore_missing" {
		t.Errorf("without pg_restore: %d %v", status, body)
	}
	if _, body, _ := administrator.call(http.MethodGet, "/backup", nil); body["restore"] != "pg_restore_missing" {
		t.Errorf("status without pg_restore: %v", body["restore"])
	}
	if status, body, _ := administrator.call(http.MethodPost, "/backup/restore", map[string]any{"id": "x", "serverName": "Polyfin"}); status != http.StatusConflict ||
		body["error"] != "restore_expired" {
		t.Errorf("nothing prepared: %d %v", status, body)
	}
	status, body, _ := administrator.call(http.MethodGet, "/backup/files", nil)
	if folder, _ := body["folder"].([]any); status != http.StatusOK || len(folder) != 1 {
		t.Errorf("files: %d %v", status, body)
	}
}

// With pg_dump and pg_restore: a backup holding a key the current one
// cannot read is prepared with its counts; it is restored only with the
// server's name typed, and with anyway; a file sent is prepared too.
func TestRestoreRoutesWithATool(t *testing.T) {
	t.Parallel()
	pgDump, pgRestore := os.Getenv("POLYFIN_TEST_PG_DUMP"), os.Getenv("POLYFIN_TEST_PG_RESTORE")
	if pgDump == "" || pgRestore == "" {
		t.Skip("POLYFIN_TEST_PG_DUMP and POLYFIN_TEST_PG_RESTORE name no pg_dump and pg_restore of PostgreSQL 18")
	}
	dir := t.TempDir()
	api, deps, service := backupAPI(t, dir, func(c *backup.Config, d testDeps) {
		c.PgDump, c.PgRestore, c.DatabaseURL = pgDump, pgRestore, testdb.CopyURL(t, d.pool)
	})
	administrator := api.signedIn("administrator", true)
	other, _ := secrets.New([]byte("fedcba9876543210fedcba9876543210"))
	if _, err := deps.pool.Exec(t.Context(), "UPDATE settings SET smtp_password = $1", other.Seal("smtp")); err != nil {
		t.Fatal(err)
	}
	if err := service.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	made, _ := service.Status(t.Context())
	status, plan, _ := administrator.call(http.MethodPost, "/backup/restore/prepare", map[string]any{"file": made.File})
	unreadable, _ := plan["unreadable"].(map[string]any)
	if status != http.StatusOK || unreadable["settings"] != 1.0 || plan["serverName"] != "Polyfin" || plan["file"] != made.File {
		t.Fatalf("prepared: %d %v", status, plan)
	}
	id := plan["id"]
	if status, body, _ := administrator.call(http.MethodPost, "/backup/restore", map[string]any{"id": id, "serverName": "Other", "anyway": true}); status != http.StatusBadRequest ||
		body["error"] != "wrong_server_name" {
		t.Errorf("another name: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPost, "/backup/restore", map[string]any{"id": id, "serverName": "Polyfin"}); status != http.StatusConflict ||
		body["error"] != "unreadable_secrets" {
		t.Errorf("not anyway: %d %v", status, body)
	}

	// A file sent replaces the backup prepared.
	data, err := os.ReadFile(filepath.Join(dir, made.File))
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, api.url+"/admin/api/backup/restore/upload?name=mine.dump",
		bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/octet-stream")
	response, err := administrator.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var uploaded map[string]any
	_ = json.NewDecoder(response.Body).Decode(&uploaded)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || uploaded["uploaded"] != true || uploaded["file"] != "mine.dump" || uploaded["madeAt"] != nil {
		t.Fatalf("uploaded: %d %v", response.StatusCode, uploaded)
	}
	if status, body, _ := administrator.call(http.MethodPost, "/backup/restore", map[string]any{"id": id, "serverName": "Polyfin", "anyway": true}); status != http.StatusConflict ||
		body["error"] != "restore_expired" {
		t.Errorf("the plan replaced: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPost, "/backup/restore", map[string]any{"id": uploaded["id"], "serverName": "Polyfin", "anyway": true}); status != http.StatusAccepted {
		t.Fatalf("restore: %d %v", status, body)
	}
	select {
	case requested := <-service.Requested():
		if requested.File != "mine.dump" || requested.By != "administrator" {
			t.Errorf("requested: %+v", requested)
		}
	case <-time.After(time.Second):
		t.Error("Polyfin is not asked to stop")
	}
}
