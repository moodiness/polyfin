package backup

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dataScript is the script pg_restore writes for the data of pool's
// database: its settings, a COPY block for each table of public, in the
// order of their names, and the value of each sequence.
func dataScript(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := t.Context()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	var b strings.Builder
	b.WriteString("--\n-- PostgreSQL database dump\n--\n\n\\restrict abc\n\n-- Dumped from database version 18.6\n\n" +
		"SET statement_timeout = 0;\nSET lock_timeout = 0;\nSET idle_in_transaction_session_timeout = 0;\nSET transaction_timeout = 0;\n" +
		"SET client_encoding = 'UTF8';\nSET standard_conforming_strings = on;\nSELECT pg_catalog.set_config('search_path', '', false);\n" +
		"SET check_function_bodies = false;\nSET xmloption = content;\nSET client_min_messages = warning;\nSET row_security = off;\n\n")
	rows, _ := conn.Query(ctx, `SELECT quote_ident(c.relname), string_agg(quote_ident(a.attname), ', ' ORDER BY a.attnum)
		FROM pg_class c JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
		WHERE c.relnamespace = 'public'::regnamespace AND c.relkind = 'r' GROUP BY 1 ORDER BY 1`)
	tables, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct{ Name, Columns string }])
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		fmt.Fprintf(&b, "--\n-- Data for Name: %s; Type: TABLE DATA; Schema: public; Owner: -\n--\n\nCOPY public.%s (%s) FROM stdin;\n",
			table.Name, table.Name, table.Columns)
		if _, err := conn.Conn().PgConn().CopyTo(ctx, &b, "COPY public."+table.Name+" ("+table.Columns+") TO STDOUT"); err != nil {
			t.Fatal(err)
		}
		b.WriteString("\\.\n\n\n")
	}
	rows, _ = conn.Query(ctx, `SELECT format('SELECT pg_catalog.setval(''public.%s'', %s, %s);', quote_ident(sequencename),
			coalesce(last_value, start_value), CASE WHEN last_value IS NULL THEN 'false' ELSE 'true' END)
		FROM pg_sequences WHERE schemaname = 'public' ORDER BY sequencename`)
	sequences, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	for _, sequence := range sequences {
		b.WriteString(sequence + "\n")
	}
	b.WriteString("\n--\n-- PostgreSQL database dump complete\n--\n\n\\unrestrict abc\n\n")
	return b.String()
}

// What a restore loads is checked against Polyfin's schema, and loaded
// with statements of Polyfin's own: rows into the tables and columns the
// schema has, each table once, in any order of the tables, and values
// into its sequences. The database is then the one the script was
// written from, its foreign keys back and checked. A script naming
// anything else, or holding any other line, is refused, and none of its
// lines is run.
func TestARestoreLoadsOnlyWhatPolyfinsSchemaHas(t *testing.T) {
	t.Parallel()
	pool, applied := earlyDatabase(t)
	ctx := t.Context()
	// Rows that refer to each other, which a backup holds in the order of
	// their tables' names: an addon and a device come before their user.
	if _, err := pool.Exec(ctx, `WITH alice AS (INSERT INTO users (name, password_hash) VALUES ('alice', 'x') RETURNING id),
			device AS (INSERT INTO devices (user_id, token_hash, device_id, device_name, client, client_version, remote_address)
				SELECT id, '\x01', 'tv', 'TV', 'App', '1', '127.0.0.1' FROM alice)
		INSERT INTO addons (owner_id, manifest_url, manifest, position) SELECT id, 'https://addon.example/manifest.json', '{}', 1 FROM alice;
		UPDATE settings SET server_name = 'Backed up'`); err != nil {
		t.Fatal(err)
	}
	backedUp := databaseState(t, pool)
	script := dataScript(t, pool)
	if !strings.Contains(script, `"position"`) || strings.Index(script, "COPY public.devices ") > strings.Index(script, "COPY public.users ") {
		t.Fatalf("the script quotes no column, or holds the users before their devices:\n%s", script)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (name, password_hash) VALUES ('bob', 'x');
		UPDATE settings SET server_name = 'Changed since';
		ALTER TABLE devices DROP CONSTRAINT devices_user_id_fkey;
		CREATE TABLE added_since (line text)`); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	text := func(query string) string {
		t.Helper()
		var value string
		if err := tx.QueryRow(ctx, query).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	// A timeout of the server's, which a long load would meet.
	if _, err := tx.Exec(ctx, "SET LOCAL statement_timeout = '1h'"); err != nil {
		t.Fatal(err)
	}
	if err := dropAll(ctx, tx); err != nil {
		t.Fatal(err)
	}
	schema, err := buildSchema(ctx, tx, applied)
	if err != nil {
		t.Fatal(err)
	}
	load := func(script string) error {
		return newScript(strings.NewReader(script)).load(ctx, tx, schema)
	}
	if got := text("SELECT current_setting('statement_timeout')"); got != "0" {
		t.Errorf("the server's statement timeout while rows load: %s", got)
	}
	if got := text("SELECT concat_ws(' ', (SELECT count(*) FROM schema_migrations), (SELECT count(*) FROM settings))"); got != "0 0" {
		t.Errorf("the schema holds rows before the backup's are loaded: %s", got)
	}
	// The first migrations make no sequence: one made as they would.
	if _, err := tx.Exec(ctx, "CREATE SEQUENCE public.numbers"); err != nil {
		t.Fatal(err)
	}
	schema.sequences["numbers"] = true
	sequence := "SELECT concat_ws(' ', last_value, is_called::text) FROM numbers"

	if err := load(script + "SELECT pg_catalog.setval('public.numbers', 4242, true);\n"); err != nil {
		t.Fatal(err)
	}
	if got := text(sequence); got != "4242 true" {
		t.Errorf("the sequence: %s", got)
	}
	if err := load("SELECT pg_catalog.setval('public.\"numbers\"', 7, false);\n"); err != nil || text(sequence) != "7 false" {
		t.Errorf("the sequence set before its first value: %s %v", text(sequence), err)
	}

	for _, script := range []string{
		// Names the schema has not, or not as that.
		"COPY public.left_by_a_backup (line) FROM stdin;\nrow\n\\.\n",
		"COPY public.pg_authid (rolname) FROM stdin;\nrow\n\\.\n",
		"COPY public.users (id, no_such_column) FROM stdin;\n\\.\n",
		"COPY public.numbers (last_value) FROM stdin;\n1\n\\.\n",
		"COPY public.libraries (addon_id) FROM stdin;\n\\.\nCOPY public.libraries (addon_id) FROM stdin;\n\\.\n",
		"SELECT pg_catalog.setval('public.no_such_seq', 1, true);\n",
		"SELECT pg_catalog.setval('public.users', 1, true);\n",
		// COPY blocks pg_restore does not write.
		"COPY public.users (name) FROM PROGRAM 'id';\n",
		"COPY public.users FROM stdin;\n\\.\n",
		"COPY users (name) FROM stdin;\n\\.\n",
		"COPY pg_catalog.pg_authid (rolname) FROM stdin;\n\\.\n",
		"COPY public.users (name) FROM stdin WITH (FORMAT csv);\n\\.\n",
		"COPY public.users (name); DROP TABLE public.users; -- ) FROM stdin;\n",
		"COPY public.users (name) FROM stdin; DROP TABLE public.users;\n",
		"copy public.users (name) from stdin;\n\\.\n",
		// Sequence values not written as pg_restore writes them.
		"SELECT pg_catalog.setval('public.numbers', 1, true), pg_catalog.pg_sleep(0);\n",
		"SELECT pg_catalog.setval('public.numbers', (SELECT 1), true);\n",
		"SELECT pg_catalog.setval('public.numbers', 1);\n",
		"SELECT pg_catalog.setval('other.numbers', 1, true);\n",
		"SELECT pg_catalog.setval('public.numbers', 99999999999999999999, true);\n",
		"SELECT setval('public.numbers', 1, true);\n",
		// Any other line.
		"SELECT pg_catalog.set_config('search_path', 'public', false);\n",
		"SET search_path = public; DROP TABLE public.users;\n",
		"SET ROLE postgres;\n",
		"CREATE TABLE public.left_by_a_backup (line text);\n",
		"DROP TABLE public.users;\n",
		"ALTER TABLE public.users DISABLE TRIGGER ALL;\n",
		"DO $$ BEGIN RAISE EXCEPTION 'run'; END $$;\n",
		"SELECT pg_catalog.lo_create(1);\n",
		"\\connect other\n",
		"\\copy public.users from 'file'\n",
		" DROP TABLE public.users;\n",
	} {
		if err := load(script); !errors.Is(err, ErrNotABackup) {
			t.Errorf("%q is loaded: %v", script, err)
		}
	}
	if got := text(sequence); got != "7 false" {
		t.Errorf("the sequence after the scripts refused: %s", got)
	}
	if got := text("SELECT concat_ws(' ', to_regclass('public.users'), to_regclass('public.left_by_a_backup'), (SELECT count(*) FROM users))"); got != "users 1" {
		t.Errorf("the tables after the scripts refused: %s", got)
	}

	if _, err := tx.Exec(ctx, "DROP SEQUENCE public.numbers"); err != nil {
		t.Fatal(err)
	}
	if err := schema.finish(ctx, tx, applied); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for _, difference := range differences(backedUp, databaseState(t, pool)) {
		t.Errorf("the database loaded differs from the one the script was written from: %s", difference)
	}

	// Rows in another encoding, which the script names, are converted.
	// Rows that refer to none, or migrations other than those the schema
	// was made with, fail the restore once all are loaded.
	for script, want := range map[string]error{
		"SET client_encoding = 'LATIN1';\nCOPY public.users (name, password_hash) FROM stdin;\ncaf\xe9\tx\n\\.\n": ErrNotABackup,
		"COPY public.devices (user_id, token_hash, device_id, device_name, client, client_version, remote_address) FROM stdin;\n" +
			"00000000-0000-0000-0000-000000000001\t\\\\x01\ttv\tTV\tApp\t1\t127.0.0.1\n\\.\n": nil,
	} {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.WithoutCancel(ctx))
		if err := dropAll(ctx, tx); err != nil {
			t.Fatal(err)
		}
		schema, err := buildSchema(ctx, tx, applied)
		if err != nil {
			t.Fatal(err)
		}
		if err := newScript(strings.NewReader(script)).load(ctx, tx, schema); err != nil {
			t.Fatal(err)
		}
		err = schema.finish(ctx, tx, applied)
		if want != nil {
			var name string
			if !errors.Is(err, want) || tx.QueryRow(ctx, "SELECT name FROM users").Scan(&name) != nil || name != "café" {
				t.Errorf("rows in another encoding, without their migrations: %v, the user %q", err, name)
			}
		} else if err == nil || errors.Is(err, ErrNotABackup) || !strings.Contains(err.Error(), "devices_user_id_fkey") {
			t.Errorf("a device of no user: %v", err)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
	}
}
