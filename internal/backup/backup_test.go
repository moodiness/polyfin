package backup

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/testdb"
)

// password is the database password of the tests, which must appear
// nowhere but in pg_dump's PGPASSWORD.
const password = "s3cret pa'ss@word"

// fakePgDump is a pg_dump that records its arguments, one per line, and
// its environment in the folder FAKE_PG_DUMP_RECORD names, writes part of
// a dump to its --file, then fails when FAKE_PG_DUMP_FAIL is set, and
// otherwise ends the dump.
const fakePgDump = `#!/bin/sh
printf '%s\n' "$@" > "$FAKE_PG_DUMP_RECORD/args"
env > "$FAKE_PG_DUMP_RECORD/env"
for arg; do
	case $arg in --file=*) file=${arg#--file=} ;; esac
done
printf 'PGDMP partial' > "$file"
if [ -n "$FAKE_PG_DUMP_FAIL" ]; then
	echo "pg_dump: error: connection to server failed" >&2
	exit 1
fi
printf ' complete' >> "$file"
`

type harness struct {
	service *Service
	// dir is the folder the service is given; empty turns backups off.
	dir    string
	record string
	logs   *bytes.Buffer
	kept   int
}

func newHarness(t *testing.T, db DB) *harness {
	t.Helper()
	h := &harness{dir: t.TempDir(), record: t.TempDir(), logs: &bytes.Buffer{}, kept: 3}
	script := filepath.Join(t.TempDir(), "pg_dump")
	if err := os.WriteFile(script, []byte(fakePgDump), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_PG_DUMP_RECORD", h.record)
	h.service = New(Config{
		Folder:      func() string { return h.dir },
		DatabaseURL: "postgresql://polyfin:" + strings.ReplaceAll(strings.ReplaceAll(password, " ", "%20"), "@", "%40") + "@db.example:5432/polyfin?sslmode=disable&pool_max_conns=4",
		PgDump:      script,
		DB:          db,
		Settings:    func() accounts.Settings { return accounts.Settings{BackupHour: 4, BackupsKept: h.kept} },
		Logger:      slog.New(slog.NewTextHandler(h.logs, nil)),
	})
	return h
}

func (h *harness) files(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(h.dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// pg_dump gets the password in PGPASSWORD alone: never on its command
// line, in another variable, nor in the logs; Polyfin's own variables,
// which hold the database URL, are not passed on.
func TestThePasswordStaysOutOfTheCommandLineAndTheLogs(t *testing.T) {
	h := newHarness(t, nil)
	t.Setenv("POLYFIN_DATABASE_URL", "postgresql://polyfin:"+password+"@db.example/polyfin")
	at := time.Date(2026, 10, 5, 4, 0, 1, 0, time.Local)
	h.service.now = func() time.Time { return at }
	if err := h.service.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(filepath.Join(h.record, "args"))
	env, _ := os.ReadFile(filepath.Join(h.record, "env"))
	want := []string{"--format=custom", "--no-password", "--file=" + filepath.Join(h.dir, ".polyfin-20261005-040001.dump.partial"),
		"--dbname=postgresql://polyfin@db.example:5432/polyfin?sslmode=disable"}
	if got := strings.Split(strings.TrimSpace(string(args)), "\n"); !slices.Equal(got, want) {
		t.Errorf("arguments:\n%q\nwant\n%q", got, want)
	}
	var passwords []string
	for line := range strings.Lines(string(env)) {
		line = strings.TrimSuffix(line, "\n")
		if strings.Contains(line, password) || strings.HasPrefix(line, "POLYFIN_") {
			passwords = append(passwords, line)
		}
	}
	if !slices.Equal(passwords, []string{"PGPASSWORD=" + password}) {
		t.Errorf("environment lines with the password or Polyfin's variables: %q", passwords)
	}
	if strings.Contains(h.logs.String(), password) || strings.Contains(h.logs.String(), "s3cret") {
		t.Errorf("the logs hold the password: %s", h.logs)
	}
	if !slices.Equal(h.files(t), []string{"polyfin-20261005-040001.dump"}) {
		t.Errorf("files: %v", h.files(t))
	}
	if data, _ := os.ReadFile(filepath.Join(h.dir, "polyfin-20261005-040001.dump")); string(data) != "PGDMP partial complete" {
		t.Errorf("backup: %q", data)
	}
	if info, err := os.Stat(filepath.Join(h.dir, "polyfin-20261005-040001.dump")); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("backup mode: %v %v", info, err)
	}
}

// A dump that fails leaves no file named like a backup, nor its partial
// one; its error, with pg_dump's, is what the run returns and records, and
// the last backup made stays recorded.
func TestAFailedDumpLeavesNoBackup(t *testing.T) {
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, pool)
	made := time.Date(2026, 10, 4, 4, 0, 0, 0, time.Local)
	h.service.now = func() time.Time { return made }
	if err := h.service.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	status, err := h.service.Status(t.Context())
	if err != nil || status.RanAt == nil || !status.RanAt.Equal(made) || status.Error != "" || status.MadeAt == nil ||
		status.File != "polyfin-20261004-040000.dump" || status.Size != int64(len("PGDMP partial complete")) {
		t.Fatalf("after a success: %+v %v", status, err)
	}
	if status.Problem(made.Add(StaleAfter)) || !status.Problem(made.Add(StaleAfter+time.Minute)) {
		t.Error("a backup is stale only after StaleAfter")
	}

	t.Setenv("FAKE_PG_DUMP_FAIL", "1")
	failed := made.Add(24 * time.Hour)
	h.service.now = func() time.Time { return failed }
	err = h.service.Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "connection to server failed") || strings.Contains(err.Error(), password) {
		t.Fatalf("failure: %v", err)
	}
	if files := h.files(t); !slices.Equal(files, []string{"polyfin-20261004-040000.dump"}) {
		t.Errorf("files after a failure: %v", files)
	}
	status, err = h.service.Status(t.Context())
	if err != nil || !status.RanAt.Equal(failed) || !strings.Contains(status.Error, "connection to server failed") ||
		!status.MadeAt.Equal(made) || status.File != "polyfin-20261004-040000.dump" || !status.Problem(failed) {
		t.Errorf("after a failure: %+v %v", status, err)
	}
	if strings.Contains(h.logs.String(), password) {
		t.Errorf("the logs hold the password: %s", h.logs)
	}

	// A partial backup a crash left is deleted by the next run.
	t.Setenv("FAKE_PG_DUMP_FAIL", "")
	crashed := filepath.Join(h.dir, ".polyfin-20261005-040000.dump.partial")
	if err := os.WriteFile(crashed, []byte("PGDMP"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.service.now = func() time.Time { return failed.Add(time.Hour) }
	if err := h.service.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if files := h.files(t); !slices.Equal(files, []string{"polyfin-20261004-040000.dump", "polyfin-20261005-050000.dump"}) {
		t.Errorf("files after a crash: %v", files)
	}
}

// Without a status recorded, there is nothing to look at; nor without a
// folder.
func TestStatusBeforeTheFirstRun(t *testing.T) {
	var none Status
	if none.Problem(time.Now()) {
		t.Error("no run is a problem")
	}
	off := New(Config{})
	if off.Available() || off.Dir() != "" {
		t.Error("backups are on without a folder")
	}
	if err := off.Run(t.Context()); !errors.Is(err, ErrOff) {
		t.Errorf("a backup without a folder: %v", err)
	}
	var nilService *Service
	if nilService.Available() {
		t.Error("a nil service is available")
	}
	if status, err := nilService.Status(context.Background()); err != nil || status.RanAt != nil {
		t.Errorf("nil service: %+v %v", status, err)
	}
}

// Retention keeps exactly the newest BackupsKept of Polyfin's backups,
// and touches no other file: neither those named almost like one, nor a
// folder or a link named like one.
func TestRetentionKeepsTheNewestBackupsOnly(t *testing.T) {
	h := newHarness(t, nil)
	h.kept = 3
	others := []string{"notes.txt", "polyfin-latest.dump", "polyfin-20200101-000000.dump.bak", "polyfin-2020010-000000.dump",
		"Polyfin-20200101-000000.dump", "other-20200101-000000.dump", ".polyfin-20200101-000000.dump"}
	for _, name := range others {
		if err := os.WriteFile(filepath.Join(h.dir, name), []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(h.dir, "polyfin-20000101-000000.dump"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(h.dir, "notes.txt"), filepath.Join(h.dir, "polyfin-20000102-000000.dump")); err != nil {
		t.Fatal(err)
	}
	old := []string{"polyfin-20250101-040000.dump", "polyfin-20250102-040000.dump", "polyfin-20251231-235959.dump"}
	for _, name := range old {
		if err := os.WriteFile(filepath.Join(h.dir, name), []byte("PGDMP"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	at := time.Date(2026, 1, 1, 4, 0, 0, 0, time.Local)
	for day := range 2 {
		h.service.now = func() time.Time { return at.AddDate(0, 0, day) }
		if err := h.service.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	want := append(slices.Clone(others), "polyfin-20000101-000000.dump", "polyfin-20000102-000000.dump",
		"polyfin-20251231-235959.dump", "polyfin-20260101-040000.dump", "polyfin-20260102-040000.dump")
	slices.Sort(want)
	if got := h.files(t); !slices.Equal(got, want) {
		t.Errorf("files:\n%q\nwant\n%q", got, want)
	}
	if !strings.Contains(h.logs.String(), "polyfin-20250101-040000.dump") {
		t.Errorf("the deletions are not logged: %s", h.logs)
	}

	// Fewer backups than kept are all kept; keeping one keeps the newest.
	if deleted, err := prune(h.dir, 90); err != nil || len(deleted) != 0 {
		t.Errorf("keeping 90: %v %v", deleted, err)
	}
	if deleted, err := prune(h.dir, 1); err != nil || !slices.Equal(deleted, []string{"polyfin-20251231-235959.dump", "polyfin-20260101-040000.dump"}) {
		t.Errorf("keeping 1: %v %v", deleted, err)
	}
}

// Two runs in the same second do not overwrite the first backup.
func TestABackupIsNeverOverwritten(t *testing.T) {
	h := newHarness(t, nil)
	at := time.Date(2026, 10, 5, 4, 0, 0, 0, time.Local)
	h.service.now = func() time.Time { return at }
	if err := h.service.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.dir, "polyfin-20261005-040000.dump"), []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := h.service.Run(t.Context()); err == nil {
		t.Error("a second backup of the same second was made")
	}
	if data, _ := os.ReadFile(filepath.Join(h.dir, "polyfin-20261005-040000.dump")); string(data) != "first" {
		t.Errorf("the first backup was overwritten: %q", data)
	}
}

// The connection string pg_dump gets has no password, in either of the
// forms pgx reads, and none of the parameters libpq would refuse.
func TestSplitPassword(t *testing.T) {
	for _, test := range []struct{ connection, conninfo, password string }{
		{"postgresql://polyfin:p%40ss@postgres:5432/polyfin", "postgresql://polyfin@postgres:5432/polyfin", "p@ss"},
		{"postgres://polyfin@postgres/polyfin?sslmode=require", "postgres://polyfin@postgres/polyfin?sslmode=require", ""},
		{"postgresql://polyfin:user@postgres/polyfin?password=query&pool_max_conns=10&search_path=x&application_name=polyfin",
			"postgresql://polyfin@postgres/polyfin?application_name=polyfin", "query"},
		{"postgresql://:only@postgres/polyfin", "postgresql://postgres/polyfin", "only"},
		{"host=postgres port=5432 user=polyfin password=secret dbname=polyfin sslmode=disable pool_max_conns=4",
			"host='postgres' port='5432' user='polyfin' dbname='polyfin' sslmode='disable'", "secret"},
		{"host = postgres password = 'two words' dbname=poly\\ fin",
			"host='postgres' dbname='poly fin'", "two words"},
		{"password='a \\'b\\\\c' host=db", "host='db'", `a 'b\c`},
	} {
		conninfo, got, err := splitPassword(test.connection)
		if err != nil || conninfo != test.conninfo || got != test.password {
			t.Errorf("%s: %q %q %v, want %q %q", test.connection, conninfo, got, err, test.conninfo, test.password)
		}
	}
	for _, refused := range []string{"host", "host='unterminated", "postgresql://polyfin:pw@[::1/x"} {
		if _, _, err := splitPassword(refused); err == nil || strings.Contains(err.Error(), "pw") {
			t.Errorf("%s: %v", refused, err)
		}
	}
}

// Backups follow the folder the settings give: off, they are not
// available, the daily task has no hour, and a run by hand fails; another
// folder is written to at once.
func TestBackupsFollowTheSettingsFolder(t *testing.T) {
	h := newHarness(t, nil)
	on := h.dir
	h.dir = ""
	if h.service.Available() || h.service.Dir() != "" || h.service.Hour() != -1 {
		t.Errorf("off: available %v, folder %q, hour %d", h.service.Available(), h.service.Dir(), h.service.Hour())
	}
	if err := h.service.Run(t.Context()); !errors.Is(err, ErrOff) {
		t.Errorf("run by hand while off: %v", err)
	}
	if entries, _ := os.ReadDir(on); len(entries) != 0 {
		t.Errorf("files written while off: %v", entries)
	}
	other := t.TempDir()
	h.dir = other
	if !h.service.Available() || h.service.Dir() != other || h.service.Hour() != 4 {
		t.Errorf("on: available %v, folder %q, hour %d", h.service.Available(), h.service.Dir(), h.service.Hour())
	}
	if err := h.service.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if files := h.files(t); len(files) != 1 || !backupName.MatchString(files[0]) {
		t.Errorf("files in the other folder: %v", files)
	}
}
