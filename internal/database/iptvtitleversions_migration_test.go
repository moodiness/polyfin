package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The migration leaves every source's titles apart, as they were; a
// group's members go with it, and its groups and the titles kept apart
// with their source.
func TestIPTVTitleVersionsMigrationGroupsNothingAtFirst(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "iptv_title_versions" })
	if at < 0 {
		t.Fatal("no iptv_title_versions migration")
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
	var grouped bool
	if err := pool.QueryRow(ctx, "SELECT title_versions FROM iptv_sources WHERE addon_id = $1", source).Scan(&grouped); err != nil || grouped {
		t.Fatalf("an existing source groups its titles: %v %v", grouped, err)
	}
	exec := func(statement string, args ...any) error {
		_, err := pool.Exec(ctx, statement, args...)
		return err
	}
	if err := exec("INSERT INTO iptv_title_groups (addon_id, type, tmdb, leader, face) VALUES ($1, 'movie', '438631', '51', '52')", source); err != nil {
		t.Fatal(err)
	}
	for _, refused := range []string{
		"INSERT INTO iptv_title_groups (addon_id, type, tmdb, leader, face) VALUES ($1, 'movie', '1', '51', '51')",
		"INSERT INTO iptv_title_groups (addon_id, type, tmdb, leader, face) VALUES ($1, 'channel', '2', '61', '61')",
		"INSERT INTO iptv_title_members (addon_id, type, key, tmdb, position, listed) VALUES ($1, 'movie', '53', '9', 0, true)",
		"INSERT INTO iptv_titles_apart (addon_id, type, key) VALUES ($1, 'movie', '')",
	} {
		if err := exec(refused, source); err == nil {
			t.Errorf("kept %s", refused)
		}
	}
	if err := exec(`INSERT INTO iptv_title_members (addon_id, type, key, tmdb, position, listed)
		VALUES ($1, 'movie', '51', '438631', 0, false), ($1, 'movie', '52', '438631', 1, true)`, source); err != nil {
		t.Fatal(err)
	}
	if err := exec("INSERT INTO iptv_titles_apart (addon_id, type, key) VALUES ($1, 'movie', '53')", source); err != nil {
		t.Fatal(err)
	}
	count := func(table string) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if err := exec("DELETE FROM iptv_title_groups WHERE addon_id = $1", source); err != nil || count("iptv_title_members") != 0 {
		t.Errorf("members left after their group: %d %v", count("iptv_title_members"), err)
	}
	if err := exec("DELETE FROM addons WHERE id = $1", source); err != nil || count("iptv_titles_apart") != 0 {
		t.Errorf("titles kept apart left after their source: %d %v", count("iptv_titles_apart"), err)
	}
}
