package database

import (
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// A database holding guides, channel choices and arrivals their keys do
// not match, as one whose index missed them, goes through the migration:
// those rows go, the others stay, and the keys are checked again and
// cascade.
func TestCatalogRenamesMigrationDeletesRowsItsKeysDoNotMatch(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "catalog_renames" })
	if at < 0 {
		t.Fatal("no catalog_renames migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	var addon string
	if err := pool.QueryRow(ctx, `INSERT INTO addons (manifest_url, manifest, position) VALUES ('http://lists/manifest.json',
		'{"id": "lists", "name": "Lists", "version": "1", "resources": ["catalog"], "catalogs": [
			{"type": "tv", "id": "channels", "name": "Channels"}, {"type": "À découvrir", "id": "list.1", "name": "List"}]}',
		1) RETURNING id`).Scan(&addon); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		// The rows the keys match.
		`INSERT INTO libraries (addon_id, catalog_type, catalog_id, position) VALUES ($1, 'tv', 'channels', 1)`,
		`INSERT INTO live_guides (id, addon_id, catalog_type, catalog_id, position, url)
			VALUES ('00000000-0000-0000-0000-000000000001', $1, 'tv', 'channels', 1, 'https://guide.example/a.xml')`,
		`INSERT INTO live_guide_maps (addon_id, catalog_type, catalog_id, channel_id, guide_id, xmltv_id, manual)
			VALUES ($1, 'tv', 'channels', '00000000-0000-0000-0000-00000000000a', '00000000-0000-0000-0000-000000000001', 'one', true)`,
		`INSERT INTO catalog_reach (addon_id, catalog_type, catalog_id, genre, reach, read_at) VALUES ($1, 'À découvrir', 'list.1', '', 3, now())`,
		`INSERT INTO catalog_arrivals (addon_id, catalog_type, catalog_id, genre, item_id, first_seen, last_seen, arrived)
			VALUES ($1, 'À découvrir', 'list.1', '', '00000000-0000-0000-0000-0000000000a1', now(), now(), true)`,
		// The keys checked no more, and rows they do not match.
		`ALTER TABLE live_guides DROP CONSTRAINT live_guides_addon_id_catalog_type_catalog_id_fkey`,
		`ALTER TABLE live_guide_maps DROP CONSTRAINT live_guide_maps_addon_id_catalog_type_catalog_id_fkey`,
		`ALTER TABLE catalog_arrivals DROP CONSTRAINT catalog_arrivals_addon_id_catalog_type_catalog_id_genre_fkey`,
		`INSERT INTO live_guides (id, addon_id, catalog_type, catalog_id, position, url)
			VALUES ('00000000-0000-0000-0000-000000000002', $1, 'tv', 'gone', 1, 'https://guide.example/b.xml')`,
		`INSERT INTO live_guide_maps (addon_id, catalog_type, catalog_id, channel_id, guide_id, xmltv_id, manual)
			VALUES ($1, 'tv', 'gone', '00000000-0000-0000-0000-00000000000b', NULL, NULL, true)`,
		`INSERT INTO catalog_arrivals (addon_id, catalog_type, catalog_id, genre, item_id, first_seen, last_seen, arrived)
			VALUES ($1, 'À découvrir', 'list.2', '', '00000000-0000-0000-0000-0000000000a2', now(), now(), true),
				($1, 'À découvrir', 'list.1', 'Drama', '00000000-0000-0000-0000-0000000000a3', now(), now(), true)`,
		// The keys back, not checked, as the migration finds them.
		`ALTER TABLE live_guides ADD CONSTRAINT live_guides_addon_id_catalog_type_catalog_id_fkey
			FOREIGN KEY (addon_id, catalog_type, catalog_id) REFERENCES libraries ON DELETE CASCADE NOT VALID`,
		`ALTER TABLE live_guide_maps ADD CONSTRAINT live_guide_maps_addon_id_catalog_type_catalog_id_fkey
			FOREIGN KEY (addon_id, catalog_type, catalog_id) REFERENCES libraries ON DELETE CASCADE NOT VALID`,
		`ALTER TABLE catalog_arrivals ADD CONSTRAINT catalog_arrivals_addon_id_catalog_type_catalog_id_genre_fkey
			FOREIGN KEY (addon_id, catalog_type, catalog_id, genre) REFERENCES catalog_reach ON DELETE CASCADE NOT VALID`,
	} {
		var args []any
		if strings.Contains(statement, "$1") {
			args = append(args, addon)
		}
		if _, err := pool.Exec(ctx, statement, args...); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}

	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	var guides, maps, arrivals []string
	if err := pool.QueryRow(ctx, `SELECT
			ARRAY(SELECT catalog_id FROM live_guides ORDER BY id),
			ARRAY(SELECT catalog_id FROM live_guide_maps ORDER BY channel_id),
			ARRAY(SELECT catalog_id || ':' || genre FROM catalog_arrivals ORDER BY item_id)`).Scan(&guides, &maps, &arrivals); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(guides, []string{"channels"}) || !slices.Equal(maps, []string{"channels"}) || !slices.Equal(arrivals, []string{"list.1:"}) {
		t.Errorf("kept: guides %q, channel choices %q, arrivals %q", guides, maps, arrivals)
	}
	var keys []string
	if err := pool.QueryRow(ctx, `SELECT ARRAY(SELECT conname || ' ' || convalidated || ' ' || confupdtype::text || confdeltype::text FROM pg_constraint
		WHERE conname IN ('live_guides_addon_id_catalog_type_catalog_id_fkey', 'live_guide_maps_addon_id_catalog_type_catalog_id_fkey',
			'catalog_arrivals_addon_id_catalog_type_catalog_id_genre_fkey') ORDER BY conname)`).Scan(&keys); err != nil {
		t.Fatal(err)
	}
	if want := []string{
		"catalog_arrivals_addon_id_catalog_type_catalog_id_genre_fkey true cc",
		"live_guide_maps_addon_id_catalog_type_catalog_id_fkey true cc",
		"live_guides_addon_id_catalog_type_catalog_id_fkey true cc",
	}; !slices.Equal(keys, want) {
		t.Errorf("keys %q, want %q", keys, want)
	}
}
