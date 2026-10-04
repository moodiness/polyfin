package database

import (
	"io/fs"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The thumbnails and chapter images made before 0033 may be out of order:
// the migration drops them all, tiles included, so that they are made
// again, and leaves the settings alone.
func TestThumbnailsAreMadeAgain(t *testing.T) {
	pool := testdb.New(t)
	ctx := t.Context()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	i := len(migrations) - 1
	for migrations[i].name != "thumbnails_again" {
		i--
	}
	if err := migrate(ctx, pool, migrations[:i]); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"UPDATE settings SET trickplay = true, chapter_images = true",
		"INSERT INTO thumbnail_versions (version_id, item_id, bytes) VALUES ('00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000002', 12)",
		`INSERT INTO trickplay_sets (version_id, width, height, tile_width, tile_height, thumbnail_count, interval_ms, bandwidth)
			VALUES ('00000000-0000-0000-0000-000000000001', 320, 180, 10, 10, 4, 10000, 100)`,
		"INSERT INTO trickplay_tiles (version_id, width, tile, data) VALUES ('00000000-0000-0000-0000-000000000001', 320, 0, 'tile')",
		"INSERT INTO chapter_images (version_id, chapter, tag, data) VALUES ('00000000-0000-0000-0000-000000000001', 0, 'tag', 'image')",
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"thumbnail_versions", "trickplay_sets", "trickplay_tiles", "chapter_images"} {
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Errorf("%s: %d rows left, %v", table, count, err)
		}
	}
	var trickplay, chapters bool
	if err := pool.QueryRow(ctx, "SELECT trickplay, chapter_images FROM settings").Scan(&trickplay, &chapters); err != nil || !trickplay || !chapters {
		t.Errorf("settings: %v %v %v", trickplay, chapters, err)
	}
}
