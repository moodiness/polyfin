package database

import (
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/testdb"
)

// madeSchema describes how the pool's database is made: its columns,
// constraints, indexes, triggers and functions, and the migrations it
// records.
func madeSchema(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, _ := pool.Query(t.Context(), `
		SELECT concat_ws(' ', 'column', table_name, ordinal_position, column_name, data_type, column_default, is_nullable, is_identity, is_generated)
		FROM information_schema.columns WHERE table_schema = 'public'
		UNION ALL
		SELECT concat_ws(' ', 'constraint', conrelid::regclass, conname, convalidated, pg_get_constraintdef(oid)) FROM pg_constraint
		WHERE connamespace = 'public'::regnamespace
		UNION ALL
		SELECT indexdef FROM pg_indexes WHERE schemaname = 'public'
		UNION ALL
		SELECT pg_get_triggerdef(t.oid) FROM pg_trigger t JOIN pg_class r ON r.oid = t.tgrelid
		WHERE r.relnamespace = 'public'::regnamespace AND NOT t.tgisinternal
		UNION ALL
		SELECT concat_ws(' ', 'function', p.oid::regprocedure, md5(p.prosrc)) FROM pg_proc p WHERE p.pronamespace = 'public'::regnamespace
			AND NOT EXISTS (SELECT FROM pg_depend d WHERE d.classid = 'pg_proc'::regclass AND d.objid = p.oid AND d.deptype = 'e')
		UNION ALL
		SELECT concat_ws(' ', 'sequence', sequencename, start_value, increment_by, last_value) FROM pg_sequences WHERE schemaname = 'public'
		UNION ALL
		SELECT concat_ws(' ', 'migration', version, name, checksum) FROM schema_migrations
		ORDER BY 1`)
	lines, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return lines
}

// shippedChecksums are the migrations this build ships, as a database
// that applied them all records them.
func shippedChecksums(t *testing.T) ([]migration, map[int]string) {
	t.Helper()
	migrations, err := shippedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	applied := map[int]string{}
	for _, m := range migrations {
		applied[m.version] = m.checksum
	}
	return migrations, applied
}

// The schema MigrateAs makes in one transaction, for a database that
// applied every migration, is the one Migrate makes one migration at a
// time: a backup restored into it finds the tables it was made of.
func TestMigrateAsMakesTheSchemaMigrateMakes(t *testing.T) {
	t.Parallel()
	// One transaction makes every table: it holds as many locks as a
	// restore does.
	testdb.LockRestores(t)
	ctx := t.Context()
	_, applied := shippedChecksums(t)
	fingerprint, err := Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	// The copy other packages' tests take of a database Migrate made.
	migrated, once := testdb.Copy(t, fingerprint[:16], Migrate), testdb.Empty(t)
	if err := pgx.BeginFunc(ctx, once, func(tx pgx.Tx) error { return MigrateAs(ctx, tx, applied) }); err != nil {
		t.Fatal(err)
	}
	want, got := madeSchema(t, migrated), madeSchema(t, once)
	if len(want) < 1000 {
		t.Fatalf("the schema Migrate makes is described in %d lines", len(want))
	}
	for _, line := range got {
		if !slices.Contains(want, line) {
			t.Errorf("made in one transaction only: %s", line)
		}
	}
	for _, line := range want {
		if !slices.Contains(got, line) {
			t.Errorf("made by Migrate only: %s", line)
		}
	}
	// Migrate finds nothing left to apply.
	if err := Migrate(ctx, once); err != nil {
		t.Errorf("Migrate on the schema made in one transaction: %v", err)
	}
}

// MigrateAs applies the migrations it is told a database applied, and no
// other: Migrate applies the rest afterwards. Migrations this build does
// not ship as told are refused, and a failure leaves nothing.
func TestMigrateAsAppliesTheMigrationsToldOnly(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	migrations, applied := shippedChecksums(t)
	first := map[int]string{}
	for _, m := range migrations[:3] {
		first[m.version] = m.checksum
	}
	pool := testdb.Empty(t)
	recorded := func() []string {
		t.Helper()
		rows, _ := pool.Query(ctx, "SELECT name FROM schema_migrations ORDER BY version")
		names, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		return names
	}
	made := func() bool {
		t.Helper()
		var made bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass('schema_migrations') IS NOT NULL").Scan(&made); err != nil {
			t.Fatal(err)
		}
		return made
	}

	failed := errors.New("the restore fails")
	for _, refused := range []struct {
		told map[int]string
		want error
	}{{map[int]string{len(migrations) + 1: "x"}, ErrNewerSchema}, {map[int]string{1: "edited"}, ErrChangedMigration}} {
		if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error { return MigrateAs(ctx, tx, refused.told) }); !errors.Is(err, refused.want) {
			t.Errorf("%v: %v, want %v", refused.told, err, refused.want)
		}
	}
	if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if err := MigrateAs(ctx, tx, first); err != nil {
			return err
		}
		return failed
	}); !errors.Is(err, failed) {
		t.Fatal(err)
	}
	if made() {
		t.Fatal("a transaction that failed left its schema")
	}

	if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error { return MigrateAs(ctx, tx, first) }); err != nil {
		t.Fatal(err)
	}
	want := []string{migrations[0].name, migrations[1].name, migrations[2].name}
	if got := recorded(); !slices.Equal(got, want) {
		t.Errorf("migrations applied: %q, want %q", got, want)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if got := recorded(); len(got) != len(applied) {
		t.Errorf("Migrate went on to %d migrations of %d", len(got), len(applied))
	}
}
