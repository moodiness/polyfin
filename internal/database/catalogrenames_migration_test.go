package database

import (
	"encoding/json"
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The types libraries kept, by catalog identifier, become keys by the
// catalog's current type and identifier, but for a catalog back to the
// type it had; and a catalog's guides, channel choices and arrivals
// follow its library and reach when its identifier changes.
func TestCatalogRenamesMigrationKeepsKeysAndCascades(t *testing.T) {
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
	if err := pool.QueryRow(ctx, `INSERT INTO addons (manifest_url, manifest, position, library_key_types) VALUES ('http://folder/manifest.json',
		'{"id": "folder", "name": "Folder", "version": "1", "resources": ["catalog"], "catalogs": [
			{"type": "series", "id": "files", "name": "Files"}, {"type": "movie", "id": "files", "name": "Files"},
			{"type": "movie", "id": "back", "name": "Back"}, {"type": "tv", "id": "channels", "name": "Channels"}]}',
		1, '{"files": "movie", "back": "movie", "gone": "series"}') RETURNING id`).Scan(&addon); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO libraries (addon_id, catalog_type, catalog_id, position) VALUES ($1, 'tv', 'channels', 1)`,
		`INSERT INTO live_guides (id, addon_id, catalog_type, catalog_id, position, url)
			VALUES ('00000000-0000-0000-0000-000000000001', $1, 'tv', 'channels', 1, 'https://guide.example/a.xml')`,
		`INSERT INTO live_guide_maps (addon_id, catalog_type, catalog_id, channel_id, guide_id, xmltv_id, manual)
			VALUES ($1, 'tv', 'channels', gen_random_uuid(), '00000000-0000-0000-0000-000000000001', 'one', true)`,
		`INSERT INTO catalog_reach (addon_id, catalog_type, catalog_id, genre, reach, read_at) VALUES ($1, 'movie', 'back', '', 3, now())`,
		`INSERT INTO catalog_arrivals (addon_id, catalog_type, catalog_id, genre, item_id, first_seen, last_seen, arrived)
			VALUES ($1, 'movie', 'back', '', gen_random_uuid(), now(), now(), true)`,
	} {
		if _, err := pool.Exec(ctx, statement, addon); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	if _, err := pool.Exec(ctx, "UPDATE libraries SET catalog_id = 'channels2'"); err == nil {
		t.Fatal("a library with guides renamed before the migration")
	}

	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	// "files" is a series first in the manifest, which kept the movie
	// type; "back" is a movie again; "gone" is no catalog of the addon.
	var raw []byte
	if err := pool.QueryRow(ctx, "SELECT library_keys FROM addons WHERE id = $1", addon).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var keys []map[string]map[string]string
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0]["catalog"]["type"] != "series" || keys[0]["catalog"]["id"] != "files" ||
		keys[0]["key"]["type"] != "movie" || keys[0]["key"]["id"] != "files" {
		t.Errorf("the keys: %s", raw)
	}
	var column bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'addons' AND column_name = 'library_key_types')`).Scan(&column); err != nil || column {
		t.Errorf("library_key_types kept: %v %v", column, err)
	}
	if _, err := pool.Exec(ctx, "UPDATE libraries SET catalog_id = 'channels2'"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE catalog_reach SET catalog_id = 'back2'"); err != nil {
		t.Fatal(err)
	}
	var guides, maps, arrivals int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM live_guides WHERE catalog_id = 'channels2'),
		(SELECT count(*) FROM live_guide_maps WHERE catalog_id = 'channels2'), (SELECT count(*) FROM catalog_arrivals WHERE catalog_id = 'back2')`).
		Scan(&guides, &maps, &arrivals); err != nil || guides != 1 || maps != 1 || arrivals != 1 {
		t.Errorf("followed: %d guides, %d channel choices, %d arrivals: %v", guides, maps, arrivals, err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM libraries"); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM live_guides), (SELECT count(*) FROM live_guide_maps)").Scan(&guides, &maps); err != nil ||
		guides != 0 || maps != 0 {
		t.Errorf("deleted with the library: %d guides left, %d channel choices left: %v", guides, maps, err)
	}
}
