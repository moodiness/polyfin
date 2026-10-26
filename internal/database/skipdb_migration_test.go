package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// SkipDB joins the segment databases last in an existing server's order,
// turned on, but on a server that turned every other database off; the
// order and the databases turned off then take it, and only it.
func TestSkipDBMigrationAddsTheFourthDatabase(t *testing.T) {
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "skipdb_segments" })
	if at < 0 {
		t.Fatal("no skipdb_segments migration")
	}
	for name, tc := range map[string]struct {
		order, off, wantOrder, wantOff string
	}{
		"defaults":      {"{theintrodb,introdb,publicmetadb}", "{}", "{theintrodb,introdb,publicmetadb,skipdb}", "{}"},
		"own order":     {"{publicmetadb,theintrodb,introdb}", "{introdb}", "{publicmetadb,theintrodb,introdb,skipdb}", "{introdb}"},
		"every one off": {"{introdb,theintrodb,publicmetadb}", "{theintrodb,introdb,publicmetadb}", "{introdb,theintrodb,publicmetadb,skipdb}", "{theintrodb,introdb,publicmetadb,skipdb}"},
	} {
		t.Run(name, func(t *testing.T) {
			pool := testdb.New(t)
			ctx := t.Context()
			if err := migrate(ctx, pool, migrations[:at]); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, "UPDATE settings SET segment_order = $1, segment_sources_off = $2", tc.order, tc.off); err != nil {
				t.Fatal(err)
			}
			if err := migrate(ctx, pool, migrations); err != nil {
				t.Fatal(err)
			}
			var order, off string
			if err := pool.QueryRow(ctx, "SELECT segment_order::text, segment_sources_off::text FROM settings").Scan(&order, &off); err != nil {
				t.Fatal(err)
			}
			if order != tc.wantOrder || off != tc.wantOff {
				t.Errorf("order %s, off %s; want %s, %s", order, off, tc.wantOrder, tc.wantOff)
			}
			for _, change := range []string{"segment_order = '{theintrodb,introdb,publicmetadb}'", "segment_order = '{theintrodb,introdb,publicmetadb,other}'",
				"segment_sources_off = '{skipdb,skipdb}'", "segment_sources_off = '{other}'"} {
				if _, err := pool.Exec(ctx, "UPDATE settings SET "+change); err == nil {
					t.Errorf("the database took %s", change)
				}
			}
			if _, err := pool.Exec(ctx, "UPDATE settings SET segment_order = '{skipdb,introdb,publicmetadb,theintrodb}', segment_sources_off = '{skipdb}'"); err != nil {
				t.Errorf("SkipDB first and off: %v", err)
			}
		})
	}
}
