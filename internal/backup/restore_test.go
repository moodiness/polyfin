package backup

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/testdb"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// restoring is a service backing up and restoring a migrated database of
// its own with PostgreSQL 18's pg_dump and pg_restore, which
// POLYFIN_TEST_PG_DUMP and POLYFIN_TEST_PG_RESTORE name: the tests skip
// without them.
type restoring struct {
	service *Service
	pool    *pgxpool.Pool
	dir     string
	logs    *bytes.Buffer
}

func newRestoring(t *testing.T) *restoring {
	t.Helper()
	pgDump, pgRestore := os.Getenv("POLYFIN_TEST_PG_DUMP"), os.Getenv("POLYFIN_TEST_PG_RESTORE")
	if pgDump == "" || pgRestore == "" {
		t.Skip("POLYFIN_TEST_PG_DUMP and POLYFIN_TEST_PG_RESTORE name no pg_dump and pg_restore of PostgreSQL 18")
	}
	pool := migrated.New(t)
	r := &restoring{pool: pool, dir: t.TempDir(), logs: &bytes.Buffer{}}
	// The restore empties the database it is given: the test's own copy.
	r.service = New(Config{Folder: func() string { return r.dir }, DatabaseURL: testdb.CopyURL(t, pool), PgDump: pgDump,
		PgRestore: pgRestore, DB: pool, Secrets: newBox(t, testKey),
		Settings: func() accounts.Settings { return accounts.Settings{BackupHour: 4, BackupsKept: 7} },
		Logger:   slog.New(slog.NewTextHandler(r.logs, nil))})
	return r
}

// backUp makes a backup at at, and returns its name.
func (r *restoring) backUp(t *testing.T, at time.Time) string {
	t.Helper()
	r.service.now = func() time.Time { return at }
	if err := r.service.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	status, err := r.service.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return status.File
}

// restore starts the restore of plan, as root, and runs it, as Polyfin
// does once stopped.
func (r *restoring) restore(t *testing.T, plan Plan, anyway bool) {
	t.Helper()
	if err := r.service.Start(t.Context(), plan.ID, anyway, "root"); err != nil {
		t.Fatal(err)
	}
	select {
	case requested := <-r.service.Requested():
		if err := r.service.Restore(t.Context(), requested); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("Polyfin is not asked to stop")
	}
}

func (r *restoring) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := r.pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A restore of a backup of the folder brings the database back as it was
// when the backup was made: the users and devices added since are gone, so
// apps signed in since must sign in again, and the server's secret, which
// signs stream links, is the backup's. The database is backed up first,
// as a before-restore backup that retention keeps; the restore is
// recorded in the restored database, with the administrator who asked.
func TestARestoreBringsTheDatabaseBackAsItWas(t *testing.T) {
	r := newRestoring(t)
	ctx := t.Context()
	store, err := accounts.Open(ctx, r.pool)
	if err != nil {
		t.Fatal(err)
	}
	alice, err := store.CreateUser(ctx, accounts.NewUser{Name: "alice", Password: "alice's password"})
	if err != nil {
		t.Fatal(err)
	}
	before, _, err := store.SignInDevice(ctx, alice.ID, accounts.DeviceInfo{DeviceID: "tv", DeviceName: "TV", Client: "App", ClientVersion: "1"})
	if err != nil {
		t.Fatal(err)
	}
	secret, err := database.Secret(ctx, r.pool)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local)
	name := r.backUp(t, at)

	if _, err := store.CreateUser(ctx, accounts.NewUser{Name: "bob", Password: "bob's password"}); err != nil {
		t.Fatal(err)
	}
	after, _, err := store.SignInDevice(ctx, alice.ID, accounts.DeviceInfo{DeviceID: "phone", DeviceName: "Phone", Client: "App", ClientVersion: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.pool.Exec(ctx, "UPDATE server_identity SET secret = decode(md5(random()::text), 'hex')"); err != nil {
		t.Fatal(err)
	}

	listing, err := r.service.Backups(ctx)
	if err != nil || len(listing.Folder) != 1 || listing.Folder[0].Name != name || !listing.Folder[0].MadeAt.Equal(at) {
		t.Fatalf("listing: %+v %v", listing, err)
	}
	plan, err := r.service.Prepare(ctx, nil, name)
	if err != nil || plan.UnreadableCount() != 0 || plan.MadeAt == nil || !plan.MadeAt.Equal(at) || plan.Size != listing.Folder[0].Size {
		t.Fatalf("prepared: %+v %v", plan, err)
	}
	r.service.now = func() time.Time { return at.Add(time.Hour) }
	r.restore(t, plan, false)

	reopened, err := accounts.Open(ctx, r.pool)
	if err != nil {
		t.Fatal(err)
	}
	if r.count(t, "SELECT count(*) FROM users WHERE name = 'bob'") != 0 {
		t.Error("a user added after the backup is still there")
	}
	if _, _, err := reopened.DeviceByToken(ctx, after, "127.0.0.1"); err == nil {
		t.Error("a device signed in after the backup is still signed in")
	}
	if _, user, err := reopened.DeviceByToken(ctx, before, "127.0.0.1"); err != nil || user.Name != "alice" {
		t.Errorf("a device signed in before the backup: %+v %v", user, err)
	}
	if restored, _ := database.Secret(ctx, r.pool); !bytes.Equal(restored, secret) {
		t.Error("the server's secret is not the backup's")
	}
	last, err := r.service.LastRestore(ctx)
	if err != nil || last == nil || last.File != name || last.Administrator != "root" || last.Error != "" {
		t.Errorf("last restore: %+v %v", last, err)
	}
	if r.count(t, "SELECT count(*) FROM activity_log WHERE type = 'BackupRestored' AND name = $1",
		"root restored the database backup "+name) != 1 {
		t.Error("the restore is not in the activity log")
	}

	// The before-restore backup is kept, listed, and never deleted to keep
	// the newest.
	beforeRestore := "polyfin-20261001-050000-before-restore.dump"
	info, err := os.Stat(filepath.Join(r.dir, beforeRestore))
	if err != nil {
		t.Fatalf("no before-restore backup: %v", err)
	}
	// The panel tells of it as the last backup made, not of the restored
	// database's own record, which predates the backup restored.
	status, err := r.service.Status(ctx)
	if err != nil || status.File != beforeRestore || status.Error != "" || status.Size != info.Size() || status.MadeAt == nil ||
		!status.MadeAt.Equal(at.Add(time.Hour)) || status.Problem(at.Add(2*time.Hour)) {
		t.Errorf("status after the restore: %+v %v", status, err)
	}
	if deleted, err := prune(r.dir, 1); err != nil || len(deleted) != 0 {
		t.Errorf("retention deleted %v %v", deleted, err)
	}
	listing, _ = r.service.Backups(ctx)
	if len(listing.Folder) != 2 || listing.Folder[0].Name != beforeRestore || !listing.Folder[0].BeforeRestore {
		t.Errorf("listing after: %+v", listing.Folder)
	}
}

// A backup of an older schema, here sent to a destination and restored
// from there, is migrated once restored: what the newer schema added, and
// what was stored in it since, is gone, then made again empty. An uploaded
// backup restores the same way.
func TestARestoreOfAnOlderBackupIsMigrated(t *testing.T) {
	r := newRestoring(t)
	ctx := t.Context()
	// The schema before the destinations: the newest migration undone.
	if _, err := r.pool.Exec(ctx, `DROP TABLE backup_destinations, backup_restore;
		DELETE FROM schema_migrations WHERE name = 'backup_destinations'`); err != nil {
		t.Fatal(err)
	}
	old := r.backUp(t, time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local))
	if err := database.Migrate(ctx, r.pool); err != nil {
		t.Fatal(err)
	}
	dav := newTestWebDAV(t, "polyfin", "dav password")
	d, err := r.service.CreateDestination(ctx, Draft{Kind: KindWebDAV, Address: new(dav.url), Username: new("polyfin"),
		Secret: new("dav password")})
	if err != nil {
		t.Fatal(err)
	}
	local, _ := localBackups(r.dir)
	if _, err := r.service.sendTo(ctx, stored{Destination: d, secret: r.service.cfg.Secrets.Seal("dav password")}, r.dir, local); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(r.dir, old)); err != nil {
		t.Fatal(err)
	}
	listing, err := r.service.Backups(ctx)
	if err != nil || len(listing.Folder) != 0 || len(listing.Destinations) != 1 || len(listing.Destinations[0].Files) != 1 ||
		listing.Destinations[0].Files[0].Name != old {
		t.Fatalf("listing: %+v %v", listing, err)
	}
	plan, err := r.service.Prepare(ctx, &d.ID, old)
	if err != nil || plan.Destination == nil || plan.Destination.ID != d.ID {
		t.Fatalf("prepared from the destination: %+v %v", plan, err)
	}
	r.service.now = func() time.Time { return time.Date(2026, 10, 2, 4, 0, 0, 0, time.Local) }
	r.restore(t, plan, false)
	if n := r.count(t, "SELECT count(*) FROM backup_destinations"); n != 0 {
		t.Errorf("destinations after the restore: %d", n)
	}
	if n := r.count(t, "SELECT count(*) FROM schema_migrations WHERE name = 'backup_destinations'"); n != 1 {
		t.Error("the restored database was not migrated")
	}
	for _, entry := range fileNames(t, r.dir) {
		if restoreTemporary.MatchString(entry) {
			t.Errorf("the download is left: %s", entry)
		}
	}

	// The same backup, sent by an administrator.
	f, err := os.Open(filepath.Join(dav.dir, old))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	uploaded, err := r.service.PrepareUpload(ctx, `C:\Backups\mine.dump`, f)
	if err != nil || !uploaded.Uploaded || uploaded.File != "mine.dump" || uploaded.MadeAt != nil {
		t.Fatalf("uploaded: %+v %v", uploaded, err)
	}
	r.service.now = func() time.Time { return time.Date(2026, 10, 3, 4, 0, 0, 0, time.Local) }
	r.restore(t, uploaded, false)
	if last, _ := r.service.LastRestore(ctx); last == nil || last.File != "mine.dump" || last.Error != "" {
		t.Errorf("last restore: %+v", last)
	}
}

// A backup made by a newer Polyfin, or whose migrations differ, or a file
// that is no backup of Polyfin, is refused before anything changes.
func TestBackupsThisPolyfinCannotRestoreAreRefused(t *testing.T) {
	r := newRestoring(t)
	ctx := t.Context()
	if _, err := r.pool.Exec(ctx, "INSERT INTO schema_migrations (version, name, checksum) VALUES (9999, 'from_the_future', 'x')"); err != nil {
		t.Fatal(err)
	}
	newer := r.backUp(t, time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local))
	if _, err := r.pool.Exec(ctx, `DELETE FROM schema_migrations WHERE version = 9999;
		UPDATE schema_migrations SET checksum = 'edited' WHERE version = 1`); err != nil {
		t.Fatal(err)
	}
	changed := r.backUp(t, time.Date(2026, 10, 2, 4, 0, 0, 0, time.Local))
	if _, err := r.service.Prepare(ctx, nil, newer); !errors.Is(err, ErrNewerBackup) {
		t.Errorf("a newer backup: %v", err)
	}
	if _, err := r.service.Prepare(ctx, nil, changed); !errors.Is(err, ErrChangedBackup) {
		t.Errorf("a backup with other migrations: %v", err)
	}
	if _, err := r.service.PrepareUpload(ctx, "notes.dump", strings.NewReader("not a backup")); !errors.Is(err, ErrNotABackup) {
		t.Errorf("a file that is no backup: %v", err)
	}
	for _, name := range []string{"../polyfin-20261001-040000.dump", "notes.txt", "polyfin-20300101-000000.dump"} {
		if _, err := r.service.Prepare(ctx, nil, name); !errors.Is(err, ErrBackupNotFound) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := r.service.Start(ctx, "nothing", true, "root"); !errors.Is(err, ErrNoPlan) {
		t.Errorf("starting nothing prepared: %v", err)
	}
	for _, entry := range fileNames(t, r.dir) {
		if strings.Contains(entry, "before-restore") || restoreTemporary.MatchString(entry) {
			t.Errorf("left: %s", entry)
		}
	}
}

// A backup holding stored keys the current POLYFIN_SECRET_KEY cannot
// decrypt is flagged with their counts by kind, before anything changes;
// it is restored only when asked anyway.
func TestABackupMadeWithAnotherKeyIsFlagged(t *testing.T) {
	r := newRestoring(t)
	ctx := t.Context()
	other := newBox(t, []byte("fedcba9876543210fedcba9876543210"))
	if _, err := r.pool.Exec(ctx, "UPDATE settings SET smtp_password = $1, metrics_token = $2, theintrodb_key = 'plain'",
		other.Seal("smtp"), other.Seal("metrics")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.pool.Exec(ctx, `INSERT INTO backup_destinations (kind, address, username, secret) VALUES ('webdav', 'https://nas.lan/', 'a', $1),
		('webdav', 'https://other.lan/', 'a', $2)`, other.Seal("password"), r.service.cfg.Secrets.Seal("readable")); err != nil {
		t.Fatal(err)
	}
	name := r.backUp(t, time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local))
	if _, err := r.pool.Exec(ctx, "UPDATE settings SET server_name = 'Changed since'"); err != nil {
		t.Fatal(err)
	}
	plan, err := r.service.Prepare(ctx, nil, name)
	if err != nil {
		t.Fatal(err)
	}
	if plan.UnreadableCount() != 3 || plan.Unreadable[secrets.KindSetting] != 2 || plan.Unreadable[secrets.KindDestination] != 1 {
		t.Errorf("unreadable: %v", plan.Unreadable)
	}
	if err := r.service.Start(ctx, plan.ID, false, "root"); !errors.Is(err, ErrUnreadableSecrets) {
		t.Fatalf("not confirmed: %v", err)
	}
	for _, entry := range fileNames(t, r.dir) {
		if strings.Contains(entry, "before-restore") {
			t.Errorf("a refused restore made %s", entry)
		}
	}
	select {
	case <-r.service.Requested():
		t.Error("a refused restore stops Polyfin")
	default:
	}
	if r.count(t, "SELECT count(*) FROM settings WHERE server_name = 'Changed since'") != 1 {
		t.Error("a refused restore changed the database")
	}
	r.restore(t, plan, true)
	if r.count(t, "SELECT count(*) FROM settings WHERE server_name = 'Changed since'") != 0 {
		t.Error("restored anyway: the change since is still there")
	}
}

// A restore that fails part way leaves the database as it was, and
// records why.
func TestAFailedRestoreLeavesTheDatabaseAsItWas(t *testing.T) {
	r := newRestoring(t)
	ctx := t.Context()
	name := r.backUp(t, time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local))
	if _, err := r.pool.Exec(ctx, "UPDATE settings SET server_name = 'Changed since'"); err != nil {
		t.Fatal(err)
	}
	plan, err := r.service.Prepare(ctx, nil, name)
	if err != nil {
		t.Fatal(err)
	}
	// The data of the backup ends early: pg_restore fails in its middle.
	data, err := os.ReadFile(plan.path)
	if err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(r.dir, "polyfin-20261001-050000.dump")
	if err := os.WriteFile(broken, data[:len(data)*3/4], 0o600); err != nil {
		t.Fatal(err)
	}
	plan.path, plan.By = broken, "root"
	if err := r.service.Restore(ctx, plan); err == nil {
		t.Fatal("a broken backup was restored")
	}
	if r.count(t, "SELECT count(*) FROM settings WHERE server_name = 'Changed since'") != 1 {
		t.Error("the failed restore changed the database")
	}
	last, err := r.service.LastRestore(ctx)
	if err != nil || last == nil || !strings.Contains(last.Error, "pg_restore") || last.File != name {
		t.Errorf("last restore: %+v %v", last, err)
	}
	if !regexp.MustCompile(`could not be restored`).MatchString(r.logs.String()) {
		t.Errorf("logs: %s", r.logs)
	}
}

// The script pg_restore writes is read as COPY's text format writes
// values.
func TestCopyValues(t *testing.T) {
	for field, want := range map[string]string{
		`plain`: "plain", `a\tb\nc\\d`: "a\tb\nc\\d", `\101\x42\r`: "AB\r", `\x`: "x", `end\`: `end\`,
	} {
		if got := unescapeCopy(field); got != want {
			t.Errorf("%q: %q, want %q", field, got, want)
		}
	}
	script := "SET x = 1;\n\\restrict abc\nCOPY public.\"order\" (id, \"user\", note) FROM stdin;\n1\tA\t\\N\n2\tB\tx\\ty\n\\.\n\\unrestrict abc\n"
	var rows []map[string]string
	err := newScript(strings.NewReader(script)).eachRow(func(table string, row map[string]string) error {
		if table != "order" {
			t.Errorf("table %q", table)
		}
		rows = append(rows, row)
		return nil
	})
	if err != nil || len(rows) != 2 || rows[0]["user"] != "A" || len(rows[0]) != 2 || rows[1]["note"] != "x\ty" {
		t.Errorf("rows: %v %v", rows, err)
	}
	if err := newScript(strings.NewReader("\\connect other\n")).eachRow(nil); !errors.Is(err, errScript) {
		t.Errorf("a psql command: %v", err)
	}
}
