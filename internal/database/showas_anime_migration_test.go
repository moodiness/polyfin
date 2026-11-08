package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// A library of an anime catalog may be shown as shows or as movies once
// the choices widen; a library chosen before keeps its choice, and a
// value no library takes is still refused.
func TestShowAsAnimeMigrationWidensTheChoices(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "library_show_as_anime" })
	if at < 0 {
		t.Fatal("no library_show_as_anime migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `WITH a AS (INSERT INTO addons (manifest_url, manifest, position) VALUES ('http://anime/manifest.json',
		'{"id": "anime", "name": "Anime", "version": "1", "resources": ["catalog"], "catalogs": []}', 1) RETURNING id)
		INSERT INTO libraries (addon_id, catalog_type, catalog_id, position, show_as)
		SELECT id, 'movie', 'top', 1, 'listed' FROM a UNION ALL SELECT id, 'anime', 'mixed', 2, NULL FROM a`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE libraries SET show_as = 'shows' WHERE catalog_id = 'mixed'"); err == nil {
		t.Fatal("shows taken before the migration")
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	for _, showAs := range []string{"shows", "movies", "movies_and_shows"} {
		if _, err := pool.Exec(ctx, "UPDATE libraries SET show_as = $1 WHERE catalog_id = 'mixed'", showAs); err != nil {
			t.Errorf("%s: %v", showAs, err)
		}
	}
	if _, err := pool.Exec(ctx, "UPDATE libraries SET show_as = 'albums' WHERE catalog_id = 'mixed'"); err == nil {
		t.Error("albums taken")
	}
	var kept string
	if err := pool.QueryRow(ctx, "SELECT show_as FROM libraries WHERE catalog_id = 'top'").Scan(&kept); err != nil || kept != "listed" {
		t.Errorf("the movie library: %q %v", kept, err)
	}
}
