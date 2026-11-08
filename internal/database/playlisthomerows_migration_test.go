package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The rows made before keep their kinds; a row may then list the
// Watchlist, the favorite playlists, never a library, or a playlist,
// which it must name and whose deletion deletes it.
func TestPlaylistHomeRowsMigration(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "playlist_home_rows" })
	if at < 0 {
		t.Fatal("no playlist_home_rows migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO home_rows (position, name, kind, place) VALUES (1, 'New', 'new', 'top'), (2, 'Picks', 'picks', 'top')`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	var playlist string
	if err := pool.QueryRow(ctx, `WITH u AS (INSERT INTO users (name, password_hash) VALUES ('alice', 'x') RETURNING id)
		INSERT INTO playlists (owner_id, name) SELECT id, 'Mix' FROM u RETURNING id`).Scan(&playlist); err != nil {
		t.Fatal(err)
	}
	for statement, args := range map[string][]any{
		"INSERT INTO home_rows (position, name, kind, place) VALUES (3, 'Watchlist', 'watchlist', 'top')":                          nil,
		"INSERT INTO home_rows (position, name, kind, place, count) VALUES (4, 'Mine', 'playlists', 'bottom', 5)":                  nil,
		"INSERT INTO home_rows (position, name, kind, place, playlist_id, library) VALUES (5, 'Mix', 'playlist', 'top', $1, true)": {playlist},
	} {
		if _, err := pool.Exec(ctx, statement, args...); err != nil {
			t.Errorf("%s: %v", statement, err)
		}
	}
	for statement, args := range map[string][]any{
		"INSERT INTO home_rows (position, name, kind, place) VALUES (6, 'Mix', 'playlist', 'top')":                                  nil,
		"INSERT INTO home_rows (position, name, kind, place, playlist_id) VALUES (6, 'New', 'new', 'top', $1)":                      {playlist},
		"INSERT INTO home_rows (position, name, kind, place, library) VALUES (6, 'Mine', 'playlists', 'top', true)":                 nil,
		"INSERT INTO home_rows (position, name, kind, place) VALUES (6, 'Other', 'albums', 'top')":                                  nil,
		"INSERT INTO home_rows (position, name, kind, place, playlist_id) VALUES (6, 'Gone', 'playlist', 'top', gen_random_uuid())": nil,
	} {
		if _, err := pool.Exec(ctx, statement, args...); err == nil {
			t.Errorf("%s: accepted", statement)
		}
	}
	// A row's folder has personal images of its own, of a playlist or not,
	// which go with the row or the playlist; one of nothing is refused.
	images := `INSERT INTO personal_images (user_id, view_id, home_row_id, playlist_id, name, titles, posters)
		SELECT u.id, gen_random_uuid(), r.id, $2::uuid, 'Row', '{}', '{}' FROM users u, home_rows r WHERE u.name = 'alice' AND r.name = $1`
	for _, args := range [][]any{{"New", nil}, {"Mix", playlist}, {"Picks", nil}} {
		if _, err := pool.Exec(ctx, images, args...); err != nil {
			t.Errorf("an image of %v: %v", args, err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO personal_images (user_id, view_id, name, titles, posters)
		SELECT id, gen_random_uuid(), 'None', '{}', '{}' FROM users WHERE name = 'alice'`); err == nil {
		t.Error("an image of nothing: accepted")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM home_rows WHERE name = 'Picks'"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM playlists WHERE id = $1", playlist); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, "SELECT name FROM home_rows ORDER BY position")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(names, []string{"New", "Watchlist", "Mine"}) {
		t.Errorf("rows: %v", names)
	}
	var left []string
	if err := pool.QueryRow(ctx, "SELECT array_agg(r.name) FROM personal_images i JOIN home_rows r ON r.id = i.home_row_id").Scan(&left); err != nil ||
		!slices.Equal(left, []string{"New"}) {
		t.Errorf("the images left: %v %v", left, err)
	}
}
