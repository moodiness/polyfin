package admin

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/backup"
)

// fakeDump is what the fake pg_dump of fakeBackupTools writes.
const fakeDump = "PGDMP a fake dump"

// fakeBackupTools writes a pg_dump that writes fakeDump to its --file, or
// its output without one, and a pg_restore that describes any file as a
// backup whose schema is the first migration's.
func fakeBackupTools(t *testing.T) (pgDump, pgRestore string) {
	t.Helper()
	migration, err := os.ReadFile(filepath.Join("..", "database", "migrations", "0001_server_identity.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(migration)
	dir := t.TempDir()
	pgDump, pgRestore = filepath.Join(dir, "pg_dump"), filepath.Join(dir, "pg_restore")
	scripts := map[string]string{
		pgDump: `#!/bin/sh
file=/dev/stdout
for arg; do
	case $arg in --file=*) file=${arg#--file=} ;; esac
done
printf '` + fakeDump + `' > "$file"
`,
		pgRestore: `#!/bin/sh
case "$1" in
--list) printf '1; 0 0 TABLE DATA public schema_migrations polyfin\n2; 1 1 TABLE public settings polyfin\n' ;;
*) printf 'COPY public.schema_migrations (version, name, checksum, applied_at) FROM stdin;\n1\tserver_identity\t` +
			hex.EncodeToString(sum[:]) + `\t2026-01-01\n\\.\n' ;;
esac
`,
	}
	for path, script := range scripts {
		if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return pgDump, pgRestore
}

// fakeTools completes a backup configuration with fakeBackupTools.
func fakeTools(t *testing.T) func(*backup.Config, testDeps) {
	pgDump, pgRestore := fakeBackupTools(t)
	return func(c *backup.Config, _ testDeps) {
		c.PgDump, c.PgRestore, c.DatabaseURL = pgDump, pgRestore, "postgresql://polyfin@127.0.0.1:1/polyfin"
	}
}

const testPassphrase = "correct horse battery"

// fetch sends a request of b's session and returns the answer, its body
// read.
func (b browser) fetch(method, path, contentType string, body io.Reader) (*http.Response, []byte) {
	b.api.t.Helper()
	request, _ := http.NewRequestWithContext(b.api.t.Context(), method, b.api.url+"/admin/api"+path, body)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := b.client.Do(request)
	if err != nil {
		b.api.t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return response, data
}

// The backup passphrase is at least 12 characters, and never answered.
// Saved, it encrypts the backups, which are downloaded as they are, or
// decrypted with the passphrase typed; restoring one asks for it.
func TestEncryptedBackupsAreDownloadedAndRestored(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	api, _, service := backupAPI(t, dir, fakeTools(t))
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)

	settings := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en", "backupPassphrase": "eleven char"}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusBadRequest ||
		body["error"] != "invalid_backup_passphrase" {
		t.Errorf("a short passphrase: %d %v", status, body)
	}
	settings["backupPassphrase"] = testPassphrase
	status, saved, _ := administrator.call(http.MethodPut, "/settings", settings)
	if _, answered := saved["backupPassphrase"]; status != http.StatusOK || saved["backupPassphraseSet"] != true || answered {
		t.Fatalf("saved: %d %v %v", status, saved["backupPassphraseSet"], saved["backupPassphrase"])
	}
	if err := service.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	made, _ := service.Status(t.Context())
	if !strings.HasSuffix(made.File, ".dump.enc") {
		t.Fatalf("the backup made: %q", made.File)
	}
	sealed, err := os.ReadFile(filepath.Join(dir, made.File))
	if err != nil || bytes.Contains(sealed, []byte(fakeDump)) {
		t.Fatalf("the backup is in the clear: %v", err)
	}
	_, files, _ := administrator.call(http.MethodGet, "/backup/files", nil)
	if folder, _ := files["folder"].([]any); len(folder) != 1 || folder[0].(map[string]any)["encrypted"] != true {
		t.Errorf("files: %v", files)
	}

	// As it is.
	response, data := administrator.fetch(http.MethodGet, "/backup/files/"+made.File, "", nil)
	if response.StatusCode != http.StatusOK || !bytes.Equal(data, sealed) ||
		response.Header.Get("Content-Disposition") != `attachment; filename="`+made.File+`"` {
		t.Errorf("download: %d %q", response.StatusCode, response.Header)
	}
	if response, _ := member.fetch(http.MethodGet, "/backup/files/"+made.File, "", nil); response.StatusCode != http.StatusForbidden {
		t.Errorf("a member's download: %d", response.StatusCode)
	}
	if response, _ := administrator.fetch(http.MethodGet, "/backup/files/notes.txt", "", nil); response.StatusCode != http.StatusNotFound {
		t.Errorf("another file: %d", response.StatusCode)
	}

	// Decrypted.
	check := "/backup/files/" + made.File + "/check"
	if status, body, _ := administrator.call(http.MethodPost, check, map[string]string{"passphrase": "not the passphrase"}); status != http.StatusBadRequest ||
		body["error"] != "wrong_passphrase" {
		t.Errorf("a wrong passphrase: %d %v", status, body)
	}
	status, checked, _ := administrator.call(http.MethodPost, check, map[string]string{"passphrase": testPassphrase})
	ticket, _ := checked["ticket"].(string)
	if status != http.StatusOK || ticket == "" {
		t.Fatalf("the passphrase: %d %v", status, checked)
	}
	decrypted := "/backup/files/" + made.File + "/decrypted?ticket=" + url.QueryEscape(ticket)
	if response, _ := member.fetch(http.MethodGet, decrypted, "", nil); response.StatusCode != http.StatusForbidden {
		t.Errorf("a member's decrypted download: %d", response.StatusCode)
	}
	response, data = administrator.fetch(http.MethodGet, decrypted, "", nil)
	if response.StatusCode != http.StatusOK || string(data) != fakeDump || response.ContentLength != int64(len(fakeDump)) ||
		response.Header.Get("Content-Disposition") != `attachment; filename="`+strings.TrimSuffix(made.File, ".enc")+`"` {
		t.Errorf("decrypted: %d %q %q", response.StatusCode, data, response.Header)
	}
	// A ticket serves once.
	if response, _ := administrator.fetch(http.MethodGet, decrypted, "", nil); response.StatusCode != http.StatusNotFound {
		t.Errorf("a ticket used twice: %d", response.StatusCode)
	}
	plain := "polyfin-20200101-000000.dump"
	if err := os.WriteFile(filepath.Join(dir, plain), []byte(fakeDump), 0o600); err != nil {
		t.Fatal(err)
	}
	if status, body, _ := administrator.call(http.MethodPost, "/backup/files/"+plain+"/check", map[string]string{"passphrase": testPassphrase}); status != http.StatusBadRequest ||
		body["error"] != "not_encrypted" {
		t.Errorf("a backup not encrypted: %d %v", status, body)
	}

	// Restored once its passphrase opened it.
	status, plan, _ := administrator.call(http.MethodPost, "/backup/restore/prepare", map[string]any{"file": made.File})
	if status != http.StatusOK || plan["locked"] != true || plan["encrypted"] != true {
		t.Fatalf("prepared: %d %v", status, plan)
	}
	start := map[string]any{"id": plan["id"], "serverName": "Polyfin", "anyway": true}
	if status, body, _ := administrator.call(http.MethodPost, "/backup/restore", start); status != http.StatusConflict ||
		body["error"] != "passphrase_required" {
		t.Errorf("started locked: %d %v", status, body)
	}
	unlock := map[string]any{"id": plan["id"], "passphrase": "not the passphrase"}
	if status, body, _ := administrator.call(http.MethodPost, "/backup/restore/unlock", unlock); status != http.StatusBadRequest ||
		body["error"] != "wrong_passphrase" {
		t.Errorf("unlocked with a wrong passphrase: %d %v", status, body)
	}
	unlock["passphrase"] = testPassphrase
	if status, body, _ := administrator.call(http.MethodPost, "/backup/restore/unlock", unlock); status != http.StatusOK || body["locked"] != false ||
		body["id"] != plan["id"] {
		t.Errorf("unlocked: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPost, "/backup/restore", start); status != http.StatusAccepted {
		t.Errorf("started: %d %v", status, body)
	}
}

// encryptedBackup makes a backup encrypted with testPassphrase with the
// fake tools, and returns its name and content.
func encryptedBackup(t *testing.T) (string, []byte) {
	t.Helper()
	pgDump, _ := fakeBackupTools(t)
	dir := t.TempDir()
	service := backup.New(backup.Config{Folder: func() string { return dir }, PgDump: pgDump, DatabaseURL: "postgresql://polyfin@127.0.0.1:1/polyfin",
		Settings: func() accounts.Settings { return accounts.Settings{BackupsKept: 1, BackupPassphrase: testPassphrase} },
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err := service.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("the folder: %v %v", entries, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	return entries[0].Name(), data
}

// The setup page restores a backup instead of creating the first
// administrator, with the setup code alone, checked before the backup is
// even received; once an administrator exists, it no longer can.
func TestTheSetupPageRestoresABackup(t *testing.T) {
	t.Parallel()
	name, sealed := encryptedBackup(t)
	_, pgRestore := fakeBackupTools(t)
	setupFolder := filepath.Join(t.TempDir(), "backups")
	var service *backup.Service
	api := newTestAPI(t, 10, func(o *Options, d testDeps) {
		service = backup.New(backup.Config{Folder: func() string { return "" }, SetupFolder: setupFolder, PgRestore: pgRestore, DB: d.pool,
			Settings: o.Accounts.Settings, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		o.Backups = service
	})
	b := api.browser()
	upload := func(code string) (int, map[string]any) {
		t.Helper()
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, api.url+"/admin/api/setup/restore/upload?name="+name,
			bytes.NewReader(sealed))
		request.Header.Set("Content-Type", "application/octet-stream")
		request.Header.Set("X-Polyfin-Setup-Code", code)
		response, err := b.client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(response.Body).Decode(&body)
		return response.StatusCode, body
	}
	if status, body := upload("WXYZ-WXYZ"); status != http.StatusBadRequest || body["error"] != "invalid_setup_code" {
		t.Errorf("a wrong setup code: %d %v", status, body)
	}
	if _, err := os.Stat(setupFolder); !os.IsNotExist(err) {
		t.Errorf("a backup sent with a wrong setup code was written: %v", err)
	}
	status, plan := upload("abcd-efgh")
	if status != http.StatusOK || plan["locked"] != true || plan["uploaded"] != true {
		t.Fatalf("sent: %d %v", status, plan)
	}
	unlock := map[string]any{"setupCode": "WXYZ-WXYZ", "id": plan["id"], "passphrase": testPassphrase}
	if status, body, _ := b.call(http.MethodPost, "/setup/restore/unlock", unlock); status != http.StatusBadRequest ||
		body["error"] != "invalid_setup_code" {
		t.Errorf("unlocked with a wrong setup code: %d %v", status, body)
	}
	unlock["setupCode"], unlock["passphrase"] = setupCode, "not the passphrase"
	if status, body, _ := b.call(http.MethodPost, "/setup/restore/unlock", unlock); status != http.StatusBadRequest ||
		body["error"] != "wrong_passphrase" {
		t.Errorf("a wrong passphrase: %d %v", status, body)
	}
	unlock["passphrase"] = testPassphrase
	if status, body, _ := b.call(http.MethodPost, "/setup/restore/unlock", unlock); status != http.StatusOK || body["locked"] != false {
		t.Fatalf("unlocked: %d %v", status, body)
	}
	if status, body, _ := b.call(http.MethodPost, "/setup/restore", map[string]any{"setupCode": "WXYZ-WXYZ", "id": plan["id"]}); status != http.StatusBadRequest ||
		body["error"] != "invalid_setup_code" {
		t.Errorf("started with a wrong setup code: %d %v", status, body)
	}
	if status, body, _ := b.call(http.MethodPost, "/setup/restore", map[string]any{"setupCode": setupCode, "id": plan["id"]}); status != http.StatusAccepted {
		t.Fatalf("started: %d %v", status, body)
	}
	select {
	case requested := <-service.Requested():
		if requested.File != name || requested.By != "" {
			t.Errorf("requested: %+v", requested)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Polyfin is not asked to stop")
	}

	if _, err := api.store.CreateUser(t.Context(), accounts.NewUser{Name: "root", Password: "correct horse", IsAdministrator: true}); err != nil {
		t.Fatal(err)
	}
	if status, body := upload(setupCode); status != http.StatusConflict || body["error"] != "setup_complete" {
		t.Errorf("once an administrator exists: %d %v", status, body)
	}
	if status, body, _ := b.call(http.MethodPost, "/setup/restore", map[string]any{"setupCode": setupCode, "id": plan["id"]}); status != http.StatusConflict ||
		body["error"] != "setup_complete" {
		t.Errorf("started once an administrator exists: %d %v", status, body)
	}
}
