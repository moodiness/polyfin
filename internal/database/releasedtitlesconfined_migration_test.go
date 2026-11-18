package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The titles released before a release kept their confinement, known by
// their description without an addon, are confined: which addon gave their
// artwork addresses is no longer known. A title that still has its addon
// keeps its reach, and so does one no catalog listed, recorded with its
// name and identifiers alone, and what is no title.
func TestReleasedTitlesConfinedMigrationConfinesTheTitlesReleased(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "released_titles_confined" })
	if at < 0 {
		t.Fatal("no released_titles_confined migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	const addon = `[1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16]`
	items := []struct {
		key, kind, data string
		confined        bool
	}{
		{"movie|released", "movie", `{"kind": "movie", "meta": {"id": "released", "type": "movie", "name": "Released", "poster": "http://192.168.1.50/poster.jpg"}}`, true},
		{"series|released", "series", `{"kind": "series", "meta": {"id": "released", "type": "series", "name": "Released", "background": "http://192.168.1.50/b.jpg"},
			"rating": "PG", "ratedAt": "2026-10-01T00:00:00Z"}`, true},
		{"video|released", "video", `{"kind": "video", "meta": {"id": "released", "type": "other", "name": "Released", "logo": "http://192.168.1.50/logo.png"}}`, true},
		{"movie|described", "movie", `{"kind": "movie", "meta": {"id": "described", "type": "movie", "name": "Described", "description": "No image."}}`, true},
		{"movie|confined", "movie", `{"kind": "movie", "confined": true, "meta": {"id": "confined", "type": "movie", "name": "Confined", "poster": "http://192.168.1.50/c.jpg"}}`, true},
		{"movie|own", "movie", `{"kind": "movie", "addon": ` + addon + `, "catalogType": "movie", "catalogId": "mine", "confined": true,
			"meta": {"id": "own", "type": "movie", "name": "Own", "poster": "https://images.example/own.jpg"}}`, true},
		{"movie|listed", "movie", `{"kind": "movie", "addon": ` + addon + `, "catalogType": "movie", "catalogId": "top",
			"meta": {"id": "listed", "type": "movie", "name": "Listed", "poster": "http://192.168.1.60/listed.jpg"}}`, false},
		{"movie|tt0000001", "movie", `{"kind": "movie", "meta": {"id": "tt0000001", "type": "movie", "name": "Unlisted", "imdb_id": "tt0000001", "_tmdbId": "11"}}`, false},
		{"series|tt0000002", "series", `{"kind": "series", "meta": {"id": "tt0000002", "type": "series", "name": "", "_tvdbId": "22"},
			"rating": "", "ratedAt": "2026-10-01T00:00:00Z"}`, false},
		{"foldervideo|v1", "video", `{"kind": "video", "seriesId": "folder", "video": {"id": "v1", "thumbnail": "http://192.168.1.50/v1.jpg"}}`, false},
		{"episode|tt0000002:1:1", "episode", `{"kind": "episode", "seriesId": "tt0000002", "season": 1,
			"video": {"id": "tt0000002:1:1", "thumbnail": "http://192.168.1.50/e1.jpg"}}`, false},
		{"person|ana actor", "person", `{"kind": "person", "person": {"name": "Ana Actor", "image": "http://192.168.1.50/ana.jpg"}}`, false},
	}
	for _, item := range items {
		if _, err := pool.Exec(ctx, `INSERT INTO items (id, key, kind, data) VALUES (gen_random_uuid(), $1, $2, $3)`, item.key, item.kind, item.data); err != nil {
			t.Fatalf("%s: %v", item.key, err)
		}
	}
	// Only up to the migration tested: a later one may confine other
	// records.
	if err := migrate(ctx, pool, migrations[:at+1]); err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		var confined, kept bool
		if err := pool.QueryRow(ctx, `SELECT coalesce((data->'confined')::boolean, false), data - 'confined' = $2::jsonb - 'confined' FROM items WHERE key = $1`,
			item.key, item.data).Scan(&confined, &kept); err != nil {
			t.Fatalf("%s: %v", item.key, err)
		}
		if confined != item.confined {
			t.Errorf("%s: confined %t, want %t", item.key, confined, item.confined)
		}
		if !kept {
			t.Errorf("%s: the rest of its record changed", item.key)
		}
	}
}
