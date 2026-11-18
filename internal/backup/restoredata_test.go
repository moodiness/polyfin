package backup

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/process"
	"github.com/moodiness/polyfin/internal/testdb"
)

// strayStatement is what a backup must never get the database to run: it
// makes a table, and fills it with what a program prints, which the
// database server itself would run.
const strayStatement = "CREATE TABLE public.left_by_a_backup(line text);COPY public.left_by_a_backup FROM PROGRAM 'id';"

// withStatement returns dump, a backup in pg_dump's custom format, with
// the definition of one of its indexes replaced by statement, and the
// name of that index. The format's table of contents holds each object's
// SQL as a string, its length before it, which pg_restore writes out as
// it is: the definition replaced is one long enough to hold statement,
// padded with spaces, so that no length changes.
func withStatement(t *testing.T, dump []byte, statement string) ([]byte, string) {
	t.Helper()
	if len(dump) < 16 || string(dump[:5]) != "PGDMP" {
		t.Fatalf("not a custom-format dump: %q", dump[:min(len(dump), 16)])
	}
	intSize := int(dump[8])
	if intSize != 4 && intSize != 8 {
		t.Fatalf("integers of %d bytes", intSize)
	}
	indexName := regexp.MustCompile(`^CREATE (?:UNIQUE )?INDEX (\S+) ON `)
	changed := bytes.Clone(dump)
	for _, prefix := range []string{"CREATE INDEX ", "CREATE UNIQUE INDEX "} {
		for at := 0; ; {
			found := bytes.Index(dump[at:], []byte(prefix))
			if found < 0 {
				break
			}
			start := at + found
			at = start + 1
			if start < intSize+1 || dump[start-intSize-1] != 0 {
				continue
			}
			length := int(binary.LittleEndian.Uint32(dump[start-intSize : start-intSize+4]))
			if length < len(statement)+1 || start+length > len(dump) || !bytes.HasSuffix(dump[start:start+length], []byte(";\n")) {
				continue
			}
			name := indexName.FindSubmatch(dump[start : start+length])
			if name == nil {
				continue
			}
			copy(changed[start:], statement+strings.Repeat(" ", length-len(statement)-1)+"\n")
			return changed, string(name[1])
		}
	}
	t.Fatal("the dump holds no index definition long enough for the statement")
	return nil, ""
}

// ranNothing checks that the restored database holds nothing of
// strayStatement, and has index, whose definition the backup no longer
// held, as Polyfin's own schema defines it. The restore is one
// transaction: had the statement run, its table would be there; and
// without the table, the COPY that runs the program has nothing to fill,
// so it did not run either.
func ranNothing(t *testing.T, r *restoring, index string) {
	t.Helper()
	var made, own *string
	if err := r.pool.QueryRow(t.Context(), "SELECT to_regclass('public.left_by_a_backup')::text, to_regclass($1)::text",
		"public."+index).Scan(&made, &own); err != nil {
		t.Fatal(err)
	}
	if made != nil {
		t.Errorf("the restore ran a statement of the backup: the database has %s", *made)
	}
	if own == nil {
		t.Errorf("the restored database lacks Polyfin's index %s", index)
	}
	if last, err := r.service.LastRestore(t.Context()); err != nil || last == nil || last.Error != "" {
		t.Errorf("the restore is not recorded as done: %+v %v\n%s", last, err, r.logs)
	}
}

// addUser adds a user to the restoring service's database.
func (r *restoring) addUser(t *testing.T, name string) {
	t.Helper()
	store, err := accounts.Open(t.Context(), r.pool)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUser(t.Context(), accounts.NewUser{Name: name, Password: name + "'s password"}); err != nil {
		t.Fatal(err)
	}
}

// A restore takes the rows of a backup, and runs none of its statements.
// Here an administrator sends a backup in which the definition of an
// index was replaced with another statement: the rows are back, in
// Polyfin's own schema, index included, and the statement did not run.
func TestAStatementInABackupIsNotRun(t *testing.T) {
	t.Parallel()
	testdb.LockRestores(t)
	r := newRestoring(t)
	ctx := t.Context()
	r.addUser(t, "alice")
	at := time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local)
	dump, err := os.ReadFile(filepath.Join(r.dir, r.backUp(t, at)))
	if err != nil {
		t.Fatal(err)
	}
	changed, index := withStatement(t, dump, strayStatement)
	r.addUser(t, "bob")

	plan, err := r.service.PrepareUpload(ctx, "polyfin-20261001-040000.dump", bytes.NewReader(changed))
	if err != nil {
		t.Fatal(err)
	}
	r.service.now = func() time.Time { return at.Add(time.Hour) }
	r.restore(t, plan, false)
	ranNothing(t, r, index)
	if r.count(t, "SELECT count(*) FROM users WHERE name = 'alice'") != 1 || r.count(t, "SELECT count(*) FROM users WHERE name = 'bob'") != 0 {
		t.Error("the users are not those of the backup")
	}
}

// The same from the setup page, before any administrator exists: the
// backup sent there is restored without running its statement.
func TestAStatementInABackupSentFromTheSetupPageIsNotRun(t *testing.T) {
	t.Parallel()
	testdb.LockRestores(t)
	r := newRestoring(t)
	ctx := t.Context()
	r.addUser(t, "alice")
	dump, err := os.ReadFile(filepath.Join(r.dir, r.backUp(t, time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local))))
	if err != nil {
		t.Fatal(err)
	}
	changed, index := withStatement(t, dump, strayStatement)
	r.addUser(t, "bob")
	// A new server: backups are off, and the setup folder takes the file.
	r.service.cfg.Folder = func() string { return "" }
	r.service.cfg.SetupFolder = filepath.Join(t.TempDir(), "backups")
	plan, err := r.service.PrepareSetupUpload(ctx, "anything.dump", bytes.NewReader(changed))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.service.StartSetup(ctx, plan.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := r.service.Restore(ctx, <-r.service.Requested()); err != nil {
		t.Fatal(err)
	}
	ranNothing(t, r, index)
	if r.count(t, "SELECT count(*) FROM users WHERE name = 'alice'") != 1 || r.count(t, "SELECT count(*) FROM users WHERE name = 'bob'") != 0 {
		t.Error("the users are not those of the backup")
	}
}

// The same for a backup downloaded from a destination, where whoever
// holds the destination can change it.
func TestAStatementInABackupOfADestinationIsNotRun(t *testing.T) {
	t.Parallel()
	testdb.LockRestores(t)
	r := newRestoring(t)
	ctx := t.Context()
	h := &harness{service: r.service}
	dav := newTestWebDAV(t, "polyfin", "dav password")
	destination := h.create(t, Draft{Kind: KindWebDAV, Address: new(dav.url), Username: new("polyfin"), Secret: new("dav password")})
	r.addUser(t, "alice")
	at := time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local)
	name := r.backUp(t, at)
	sent := filepath.Join(dav.dir, name)
	dump, err := os.ReadFile(sent)
	if err != nil {
		t.Fatalf("the destination's copy: %v", err)
	}
	changed, index := withStatement(t, dump, strayStatement)
	if err := os.WriteFile(sent, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	r.addUser(t, "bob")

	plan, err := r.service.Prepare(ctx, &destination.ID, name)
	if err != nil {
		t.Fatal(err)
	}
	r.service.now = func() time.Time { return at.Add(time.Hour) }
	r.restore(t, plan, false)
	ranNothing(t, r, index)
	if r.count(t, "SELECT count(*) FROM users WHERE name = 'alice'") != 1 || r.count(t, "SELECT count(*) FROM users WHERE name = 'bob'") != 0 {
		t.Error("the users are not those of the backup")
	}
}

// A file named as an encrypted backup that does not start as one is
// refused as damaged, from the folder, from a destination and sent by an
// administrator: it is never restored without the passphrase its name
// promises. An encrypted backup is still kept for its passphrase, and a
// backup under a plain name is prepared as before.
func TestABackupNamedEncryptedThatIsNotIsRefused(t *testing.T) {
	t.Parallel()
	h := withDestinations(t)
	h.passphrase = testPassphrase
	h.service.cfg.PgRestore = fakePgRestore(t)
	ctx := t.Context()
	dav := newTestWebDAV(t, "polyfin", "dav password")
	destination := h.create(t, Draft{Kind: KindWebDAV, Address: new(dav.url), Username: new("polyfin"), Secret: new("dav password")})
	h.runAt(t, time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local))
	name := "polyfin-20261001-040000.dump.enc"
	sources := map[string]*accounts.ID{"the folder": nil, "the destination": &destination.ID}
	for from, id := range sources {
		plan, err := h.service.Prepare(ctx, id, name)
		if err != nil || !plan.Locked || !plan.Encrypted {
			t.Fatalf("%s: an encrypted backup is prepared as %+v %v", from, plan, err)
		}
		h.service.Discard(plan.ID)
	}

	// The same name, on a dump that is not encrypted.
	for _, dir := range []string{h.dir, dav.dir} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(fakeDump), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	listing, err := h.service.Backups(ctx)
	if err != nil || len(listing.Folder) != 1 || !listing.Folder[0].Encrypted || len(listing.Destinations) != 1 ||
		len(listing.Destinations[0].Files) != 1 || !listing.Destinations[0].Files[0].Encrypted {
		t.Fatalf("listing: %+v %v", listing, err)
	}
	for from, id := range sources {
		if plan, err := h.service.Prepare(ctx, id, name); !errors.Is(err, ErrDamagedBackup) {
			t.Errorf("%s: a plain dump named as encrypted is prepared as %+v %v", from, plan, err)
		}
	}
	if plan, err := h.service.PrepareUpload(ctx, name, strings.NewReader(fakeDump)); !errors.Is(err, ErrDamagedBackup) {
		t.Errorf("sent: a plain dump named as encrypted is prepared as %+v %v", plan, err)
	}
	if files := restoreFiles(t, h.dir); len(files) != 0 {
		t.Errorf("the files refused stay: %q", files)
	}
	plan, err := h.service.PrepareUpload(ctx, "mine.dump", strings.NewReader(fakeDump))
	if err != nil || plan.Locked || plan.Encrypted {
		t.Errorf("a plain dump under a plain name: %+v %v", plan, err)
	}
}

// databaseState describes what a database holds and how it is made, by
// what it describes: the rows of each table of public and the value of
// each sequence, then its columns, constraints, indexes, triggers, those
// of the foreign keys included, and functions.
func databaseState(t *testing.T, pool *pgxpool.Pool) map[string]string {
	t.Helper()
	ctx := t.Context()
	state := map[string]string{}
	one := func(key, query string) {
		t.Helper()
		var value string
		if err := pool.QueryRow(ctx, query).Scan(&value); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		state[key] = value
	}
	rows, _ := pool.Query(ctx, "SELECT relname::text, relkind::text FROM pg_class WHERE relnamespace = 'public'::regnamespace AND relkind IN ('r', 'S')")
	relations, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct{ Name, Kind string }])
	if err != nil {
		t.Fatal(err)
	}
	for _, relation := range relations {
		name := pgx.Identifier{"public", relation.Name}.Sanitize()
		if relation.Kind == "S" {
			one("sequence "+relation.Name, "SELECT last_value::text || ' ' || is_called::text FROM "+name)
		} else {
			one("rows of "+relation.Name, "SELECT count(*)::text || ' ' || coalesce(md5(string_agg(md5(t::text), '' ORDER BY md5(t::text))), '') FROM "+name+" t")
		}
	}
	for key, query := range map[string]string{
		"columns": `SELECT concat_ws(' ', table_name, ordinal_position, column_name, data_type, column_default, is_nullable, is_identity, is_generated)
			FROM information_schema.columns WHERE table_schema = 'public'`,
		"constraints": `SELECT concat_ws(' ', conrelid::regclass, conname, convalidated, pg_get_constraintdef(oid)) FROM pg_constraint
			WHERE connamespace = 'public'::regnamespace`,
		"indexes": `SELECT indexdef FROM pg_indexes WHERE schemaname = 'public'`,
		"triggers": `SELECT concat_ws(' ', t.tgrelid::regclass, CASE WHEN t.tgisinternal THEN c.conname ELSE t.tgname END, t.tgenabled, t.tgtype,
				t.tgfoid::regproc, CASE WHEN NOT t.tgisinternal THEN pg_get_triggerdef(t.oid) END)
			FROM pg_trigger t JOIN pg_class r ON r.oid = t.tgrelid LEFT JOIN pg_constraint c ON c.oid = t.tgconstraint
			WHERE r.relnamespace = 'public'::regnamespace`,
		"functions": `SELECT concat_ws(' ', p.oid::regprocedure, md5(p.prosrc)) FROM pg_proc p WHERE p.pronamespace = 'public'::regnamespace
			AND NOT EXISTS (SELECT FROM pg_depend d WHERE d.classid = 'pg_proc'::regclass AND d.objid = p.oid AND d.deptype = 'e')`,
	} {
		one(key, "SELECT coalesce(string_agg(line, E'\\n' ORDER BY line), '') FROM ("+query+") AS described(line)")
	}
	return state
}

// differences lists what differs between two states of a database.
func differences(was, is map[string]string) []string {
	var differing []string
	for _, key := range slices.Sorted(maps.Keys(was)) {
		if got, ok := is[key]; !ok {
			differing = append(differing, key+" is gone")
		} else if got != was[key] {
			differing = append(differing, fmt.Sprintf("%s:\n%s\nwas\n%s", key, got, was[key]))
		}
	}
	for _, key := range slices.Sorted(maps.Keys(is)) {
		if _, ok := was[key]; !ok {
			differing = append(differing, key+" is new")
		}
	}
	return differing
}

// A backup restores to the database it was made of: the rows of every
// table and the value of every sequence, in a schema that is Polyfin's
// own again, whatever was changed, dropped or added since: columns,
// constraints, indexes, triggers and functions, the foreign keys and the
// triggers set aside while the rows load included.
func TestARestoredDatabaseIsTheOneBackedUp(t *testing.T) {
	t.Parallel()
	testdb.LockRestores(t)
	r := newRestoring(t)
	ctx := t.Context()
	// Rows in tables that refer to each other, which a backup holds in the
	// order of their names: a user's devices and playlist come before the
	// user. Sequences whose next value follows the last, or is it.
	r.addUser(t, "alice")
	store, err := accounts.Open(ctx, r.pool)
	if err != nil {
		t.Fatal(err)
	}
	alice, err := store.Authenticate(ctx, "alice", "alice's password")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.SignInDevice(ctx, alice.ID, accounts.DeviceInfo{DeviceID: "tv", DeviceName: "TV", Client: "App", ClientVersion: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.pool.Exec(ctx, `SELECT setval('activity_log_id_seq', 4242, true), setval('playback_history_id_seq', 77, false)`); err != nil {
		t.Fatal(err)
	}
	if r.count(t, "SELECT count(*) FROM playlists") == 0 || r.count(t, "SELECT count(*) FROM devices") == 0 {
		t.Fatal("the user has no playlist or no device: the backup would hold no row referring to another")
	}
	backedUp := databaseState(t, r.pool)
	name, _, err := r.service.dump(ctx, r.dir, time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local), "")
	if err != nil {
		t.Fatal(err)
	}

	r.addUser(t, "bob")
	if _, err := r.pool.Exec(ctx, `UPDATE settings SET server_name = 'Changed since';
		SELECT setval('activity_log_id_seq', 1, false), setval('playback_history_id_seq', 9000, true);
		DROP TRIGGER users_watchlist ON users;
		ALTER TABLE devices DROP CONSTRAINT devices_user_id_fkey;
		ALTER TABLE users ADD COLUMN added_since text;
		DROP INDEX items_rated;
		CREATE TABLE added_since (line text);
		CREATE OR REPLACE FUNCTION watchlist_for_new_user() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END $$`); err != nil {
		t.Fatal(err)
	}
	if len(differences(backedUp, databaseState(t, r.pool))) < 8 {
		t.Fatal("the changes made since the backup do not show")
	}

	plan, err := r.service.Prepare(ctx, nil, name)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.service.restore(ctx, plan); err != nil {
		t.Fatalf("%v\n%s", err, r.logs)
	}
	for _, difference := range differences(backedUp, databaseState(t, r.pool)) {
		t.Errorf("the restored database differs from the one backed up: %s", difference)
	}
}

// scriptedPgRestore is a pg_restore that describes every file as a
// backup of pool's schema, and writes as its data the script the
// function it returns was last given.
func scriptedPgRestore(t *testing.T, pool *pgxpool.Pool) (string, func(script string)) {
	t.Helper()
	dir := t.TempDir()
	migrations, data := filepath.Join(dir, "migrations.sql"), filepath.Join(dir, "data.sql")
	if err := os.WriteFile(migrations, []byte(migrationsBlock(t, pool)), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
case " $* " in
*" --list "*) printf '1; 0 0 TABLE DATA public schema_migrations polyfin\n2; 1 1 TABLE public settings polyfin\n' ;;
*" --table=schema_migrations "*) cat '` + migrations + `' ;;
*) cat '` + data + `' ;;
esac
`
	path := filepath.Join(dir, "pg_restore")
	if err := process.WriteExecutable(path, strings.NewReader(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path, func(script string) {
		t.Helper()
		if err := os.WriteFile(data, []byte(script), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// migrationsBlock is the COPY block pg_restore writes for the migrations
// pool's database applied.
func migrationsBlock(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var block string
	if err := pool.QueryRow(t.Context(), `SELECT string_agg(concat_ws(E'\t', version, name, checksum, applied_at), E'\n' ORDER BY version)
		FROM schema_migrations`).Scan(&block); err != nil {
		t.Fatal(err)
	}
	return "COPY public.schema_migrations (version, name, checksum, applied_at) FROM stdin;\n" + block + "\n\\.\n"
}

// earlyDatabase is a database of its own holding Polyfin's schema as its
// first migrations make it, up to the addons', and the migrations it
// records: a few tables referring to each other, made and dropped with a
// fraction of the locks of the whole schema, so that a test restoring
// into it need not wait for its turn (see testdb.LockRestores).
func earlyDatabase(t *testing.T) (*pgxpool.Pool, map[int]string) {
	t.Helper()
	ctx := t.Context()
	files, err := filepath.Glob(filepath.Join("..", "database", "migrations", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(files)
	pool := testdb.Empty(t)
	if _, err := pool.Exec(ctx, `CREATE TABLE schema_migrations (version integer PRIMARY KEY, name text NOT NULL, checksum text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	applied := map[int]string{}
	for i, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		name := strings.TrimSuffix(filepath.Base(file)[len("0000_"):], ".sql")
		applied[i+1] = hex.EncodeToString(sum[:])
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, "INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)", i+1, name, applied[i+1]); err != nil {
			t.Fatal(err)
		}
		if name == "addons" {
			return pool, applied
		}
	}
	t.Fatal("no addons migration")
	return nil, nil
}

// A backup whose data asks for more than rows of Polyfin's tables is
// refused whole, as no backup of Polyfin's: the database is as it was,
// rows and schema.
func TestABackupAskingForMoreThanRowsIsRefusedWhole(t *testing.T) {
	t.Parallel()
	pool, _ := earlyDatabase(t)
	ctx := t.Context()
	pgRestore, write := scriptedPgRestore(t, pool)
	logs, dir := &bytes.Buffer{}, t.TempDir()
	service := New(Config{Folder: func() string { return dir }, DatabaseURL: testdb.CopyURL(t, pool), PgRestore: pgRestore, DB: pool,
		Secrets: newBox(t, testKey), Settings: func() accounts.Settings { return accounts.Settings{BackupsKept: 7} },
		Logger: slog.New(slog.NewTextHandler(logs, nil))})
	if _, err := pool.Exec(ctx, `UPDATE settings SET server_name = 'As it was';
		INSERT INTO users (name, password_hash) VALUES ('alice', 'x')`); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "polyfin-20261001-040000.dump")
	if err := os.WriteFile(file, []byte(fakeDump), 0o600); err != nil {
		t.Fatal(err)
	}
	migrations := migrationsBlock(t, pool)
	was := databaseState(t, pool)
	for name, script := range map[string]string{
		"a statement":                "SET statement_timeout = 0;\nCREATE TABLE public.left_by_a_backup (line text);\n" + migrations,
		"rows a program prints":      migrations + "COPY public.users (name) FROM PROGRAM 'id';\n",
		"rows of a table of its own": migrations + "COPY public.left_by_a_backup (line) FROM stdin;\nrow\n\\.\n",
		"a sequence set otherwise":   migrations + "SELECT pg_catalog.setval('public.users', 1, true), pg_catalog.pg_sleep(0);\n",
	} {
		write(script)
		logs.Reset()
		err := service.Restore(ctx, Plan{File: filepath.Base(file), path: file, By: "root"})
		if !errors.Is(err, ErrNotABackup) {
			t.Errorf("%s: the restore ends with %v", name, err)
		}
		if !strings.Contains(logs.String(), "could not be restored") {
			t.Errorf("%s: logs: %s", name, logs)
		}
		for _, difference := range differences(was, databaseState(t, pool)) {
			t.Errorf("%s: the database changed: %s", name, difference)
		}
	}
}
