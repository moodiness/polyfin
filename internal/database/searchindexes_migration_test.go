package database

import (
	"context"
	"io/fs"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/testdb"
)

var (
	// trigramIndexes need pg_trgm; nameIndexes do not.
	trigramIndexes = []string{"items_episode_name_trgm", "items_person_name_trgm", "items_title_name_trgm"}
	nameIndexes    = []string{"items_person_name", "items_rated"}
)

// searchIndexes returns the migrations before search_indexes, and it.
func searchIndexes(t *testing.T) ([]migration, migration) {
	t.Helper()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "search_indexes" })
	if at < 0 {
		t.Fatal("no search_indexes migration")
	}
	return migrations[:at], migrations[at]
}

// itemsIndexes lists the indexes of the items table of the pool's schema.
func itemsIndexes(t *testing.T, ctx context.Context, db interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}) []string {
	t.Helper()
	rows, err := db.Query(ctx, "SELECT indexname::text FROM pg_catalog.pg_indexes WHERE schemaname = current_schema() AND tablename = 'items' ORDER BY 1")
	if err != nil {
		t.Fatal(err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func trigramsAvailable(t *testing.T, pool *pgxpool.Pool) bool {
	t.Helper()
	var available bool
	if err := pool.QueryRow(t.Context(), "SELECT EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'pg_trgm')").Scan(&available); err != nil {
		t.Fatal(err)
	}
	return available
}

// On a server with pg_trgm, the migration makes every index, and runs
// again cleanly once the extension and the indexes exist.
func TestSearchIndexesMigrationRunsAgain(t *testing.T) {
	pool := testdb.New(t)
	if !trigramsAvailable(t, pool) {
		t.Skip("the test server has no pg_trgm")
	}
	ctx := t.Context()
	before, search := searchIndexes(t)
	if err := migrate(ctx, pool, append(before, search)); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		for _, want := range slices.Concat(trigramIndexes, nameIndexes) {
			if !slices.Contains(itemsIndexes(t, ctx, pool), want) {
				t.Errorf("no index %s on items: %v", want, itemsIndexes(t, ctx, pool))
			}
		}
		if _, err := pool.Exec(ctx, search.sql); err != nil {
			t.Fatalf("the migration does not run again: %v", err)
		}
	}
}

// Where pg_trgm is missing, or its indexes cannot be made, the migration
// still succeeds, with the indexes that need no extension. The catalog
// views the migration reads are shadowed, in a schema searched before
// pg_catalog, to tell it so.
func TestSearchIndexesMigrationWithoutTrigrams(t *testing.T) {
	for name, shadow := range map[string][]string{
		"missing": {
			"CREATE VIEW pg_available_extensions AS SELECT * FROM pg_catalog.pg_available_extensions WHERE name <> 'pg_trgm'",
			"CREATE VIEW pg_extension AS SELECT * FROM pg_catalog.pg_extension WHERE extname <> 'pg_trgm'",
		},
		// The extension seems to be in a schema that has none of its
		// operator classes: making the indexes fails.
		"unusable": {
			`CREATE VIEW pg_extension AS SELECT e.oid, e.extname, e.extowner, current_schema()::regnamespace::oid AS extnamespace,
				e.extrelocatable, e.extversion, e.extconfig, e.extcondition
			FROM pg_catalog.pg_extension e WHERE e.extname = 'plpgsql'
			UNION ALL SELECT 0::oid, 'pg_trgm', 10::oid, current_schema()::regnamespace::oid, true, '1.6', NULL, NULL`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			pool := testdb.New(t)
			ctx := t.Context()
			before, search := searchIndexes(t)
			if err := migrate(ctx, pool, before); err != nil {
				t.Fatal(err)
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.WithoutCancel(ctx))
			if _, err := tx.Exec(ctx, "SELECT set_config('search_path', current_schema() || ', pg_catalog', true)"); err != nil {
				t.Fatal(err)
			}
			for _, statement := range shadow {
				if _, err := tx.Exec(ctx, statement); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := tx.Exec(ctx, search.sql); err != nil {
				t.Fatalf("the migration fails: %v", err)
			}
			got := itemsIndexes(t, ctx, tx)
			for _, want := range nameIndexes {
				if !slices.Contains(got, want) {
					t.Errorf("no index %s on items: %v", want, got)
				}
			}
			for _, unwanted := range trigramIndexes {
				if slices.Contains(got, unwanted) {
					t.Errorf("index %s made without pg_trgm: %v", unwanted, got)
				}
			}
		})
	}
}
