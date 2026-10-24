package database

import (
	"encoding/json"
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The movies and shows folders of an upgraded server describe and stream
// their plain videos' identifiers, as new folders do; a home videos
// folder's identifiers stay its own.
func TestPlainVideosMigrationServesTheirIdentifiers(t *testing.T) {
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "local_plain_videos" })
	if at < 0 {
		t.Fatal("no local_plain_videos migration")
	}
	pool := testdb.New(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	manifest := func(kind, prefix string, prefixes ...string) string {
		data, err := json.Marshal(map[string]any{"id": "polyfin.local." + kind, "version": "1", "name": kind, "types": []string{kind},
			"resources": []any{map[string]any{"name": "catalog"},
				map[string]any{"name": "meta", "types": []string{kind}, "idPrefixes": append([]string{prefix}, prefixes...)},
				map[string]any{"name": "stream", "types": []string{kind}, "idPrefixes": append([]string{prefix}, prefixes...)}}})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	folders := map[string]string{
		"movies":     manifest("movie", "tt", "tmdb:"),
		"shows":      manifest("series", "tt", "tmdb:"),
		"homevideos": manifest("polyfin.homevideo", "polyfin-video:"),
	}
	position := 0
	for kind, m := range folders {
		position++
		if _, err := pool.Exec(ctx, `WITH a AS (INSERT INTO addons (manifest_url, manifest, position, kind) VALUES ($1, $2, $3, 'local') RETURNING id)
			INSERT INTO local_folders (addon_id, kind) SELECT id, $4 FROM a`, "/media/"+kind, m, position, kind); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"movies": {"tt", "tmdb:", "polyfin-plain:"}, "shows": {"tt", "tmdb:", "polyfin-plain:"},
		"homevideos": {"polyfin-video:"}}
	for kind, prefixes := range want {
		var resources []struct {
			Name       string   `json:"name"`
			IDPrefixes []string `json:"idPrefixes"`
		}
		var data []byte
		if err := pool.QueryRow(ctx, `SELECT a.manifest->'resources' FROM addons a JOIN local_folders f ON f.addon_id = a.id
			WHERE f.kind = $1`, kind).Scan(&data); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &resources); err != nil {
			t.Fatal(err)
		}
		if len(resources) != 3 || resources[0].Name != "catalog" || resources[0].IDPrefixes != nil ||
			!slices.Equal(resources[1].IDPrefixes, prefixes) || !slices.Equal(resources[2].IDPrefixes, prefixes) {
			t.Errorf("the %s folder's resources: %+v", kind, resources)
		}
	}
}
