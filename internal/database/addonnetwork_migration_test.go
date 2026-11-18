package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The sources installed before they kept where they are tell nothing,
// which counts as on the internet, but the local folders, which are on
// the server; and the photos addons gave people before are downloaded
// from public addresses only, as no person tells which addon gave theirs.
func TestAddonNetworkMigrationLeavesSourcesUntoldAndConfinesPeoplesPhotos(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "addon_network" })
	if at < 0 {
		t.Fatal("no addon_network migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO addons (kind, manifest_url, manifest, position) VALUES
		('stremio', 'http://192.168.1.5:7000/manifest.json', '{}', 1),
		('stremio', 'https://addon.example/manifest.json', '{}', 2),
		('m3u', 'https://provider.example/list.m3u', '{}', 3),
		('local', '/media/movies', '{}', 4)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO items (id, key, kind, data) VALUES
		(gen_random_uuid(), 'person|with a photo', 'person', '{"person": {"name": "With a photo", "image": "http://192.168.1.50/photo.jpg"}}'),
		(gen_random_uuid(), 'person|confined', 'person', '{"person": {"name": "Confined", "image": "https://images.example/photo.jpg"}, "confined": true}'),
		(gen_random_uuid(), 'person|without', 'person', '{"person": {"name": "Without"}}'),
		(gen_random_uuid(), 'movie|tt1', 'movie', '{"meta": {"id": "tt1", "type": "movie", "poster": "http://192.168.1.50/poster.jpg"}}')`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}

	rows, err := pool.Query(ctx, "SELECT kind || ' ' || manifest_url, network FROM addons")
	if err != nil {
		t.Fatal(err)
	}
	networks := map[string]*string{}
	var name string
	var network *string
	for rows.Next() {
		if err := rows.Scan(&name, &network); err != nil {
			t.Fatal(err)
		}
		networks[name] = network
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(networks) != 4 || networks["stremio http://192.168.1.5:7000/manifest.json"] != nil || networks["stremio https://addon.example/manifest.json"] != nil ||
		networks["m3u https://provider.example/list.m3u"] != nil {
		t.Errorf("the sources tell where they are: %v", networks)
	}
	if folder := networks["local /media/movies"]; folder == nil || *folder != "local" {
		t.Errorf("the local folder is on %v", folder)
	}
	// Only local and public are networks.
	if _, err := pool.Exec(ctx, "UPDATE addons SET network = 'anywhere'"); err == nil {
		t.Error("a source may be told to be anywhere")
	}

	rows, err = pool.Query(ctx, "SELECT key, coalesce((data->>'confined')::boolean, false) FROM items")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"person|with a photo": true, "person|confined": true, "person|without": false, "movie|tt1": false}
	var confined bool
	for rows.Next() {
		if err := rows.Scan(&name, &confined); err != nil {
			t.Fatal(err)
		}
		if confined != want[name] {
			t.Errorf("%s: confined %v", name, confined)
		}
		delete(want, name)
	}
	if err := rows.Err(); err != nil || len(want) != 0 {
		t.Errorf("missing %v %v", want, err)
	}
}
