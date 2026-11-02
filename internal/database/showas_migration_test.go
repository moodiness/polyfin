package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// A library an administrator set to show its titles as movies and shows
// keeps showing them so once the switch becomes a choice; the others have
// no choice yet, so that Polyfin may decide one for a catalog of other
// videos from its first page.
func TestShowAsMigrationKeepsTheChoice(t *testing.T) {
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "library_show_as" })
	if at < 0 {
		t.Fatal("no library_show_as migration")
	}
	pool := testdb.New(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `WITH a AS (INSERT INTO addons (manifest_url, manifest, position) VALUES ('http://vault/manifest.json',
		'{"id": "vault", "name": "Vault", "version": "1", "resources": ["catalog"], "catalogs": []}', 1) RETURNING id)
		INSERT INTO libraries (addon_id, catalog_type, catalog_id, position, show_as_titles)
		SELECT id, 'library', 'store', 1, true FROM a UNION ALL SELECT id, 'other', 'clips', 2, false FROM a
		UNION ALL SELECT id, 'movie', 'top', 3, false FROM a`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	got := map[string]*string{}
	rows, err := pool.Query(ctx, "SELECT catalog_id, show_as FROM libraries")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var showAs *string
		if err := rows.Scan(&id, &showAs); err != nil {
			t.Fatal(err)
		}
		got[id] = showAs
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got["store"] == nil || *got["store"] != "movies_and_shows" || got["clips"] != nil || got["top"] != nil {
		t.Errorf("the libraries' choices: %v", got)
	}
}
