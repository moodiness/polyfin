package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The migration leaves every source naming its titles as the provider
// does; the names learned go with their source.
func TestIPTVTitleNamesMigrationKeepsTheProvidersNames(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "iptv_title_names" })
	if at < 0 {
		t.Fatal("no iptv_title_names migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	var source string
	if err := pool.QueryRow(ctx, `WITH a AS (INSERT INTO addons (kind, manifest_url, manifest, position)
		VALUES ('xtream', 'https://provider.example/', '{}', 1) RETURNING id)
		INSERT INTO iptv_sources (addon_id, movies) SELECT id, true FROM a RETURNING addon_id`).Scan(&source); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	var named bool
	if err := pool.QueryRow(ctx, "SELECT metadata_names FROM iptv_sources WHERE addon_id = $1", source).Scan(&named); err != nil || named {
		t.Fatalf("an existing source names its titles after the metadata: %v %v", named, err)
	}
	exec := func(statement string, args ...any) error {
		_, err := pool.Exec(ctx, statement, args...)
		return err
	}
	for _, refused := range []string{
		"INSERT INTO iptv_title_names (addon_id, type, key, name, search, tmdb, imdb) VALUES ($1, 'channel', '1', 'One', 'one', '1', '')",
		"INSERT INTO iptv_title_names (addon_id, type, key, name, search, tmdb, imdb) VALUES ($1, 'movie', '', 'One', 'one', '1', '')",
		"INSERT INTO iptv_title_names (addon_id, type, key, name, search, tmdb, imdb) VALUES ($1, 'movie', '1', '', '', '1', '')",
	} {
		if err := exec(refused, source); err == nil {
			t.Errorf("kept %s", refused)
		}
	}
	if err := exec("INSERT INTO iptv_title_names (addon_id, type, key, name, search, tmdb, imdb) VALUES ($1, 'movie', '1', 'One', 'one', '1', '')",
		source); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := exec("DELETE FROM addons WHERE id = $1", source); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM iptv_title_names").Scan(&left); err != nil || left != 0 {
		t.Errorf("names left after their source: %d %v", left, err)
	}
}
