package database

import (
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// A playlist's name is bounded by its table. A name longer before the
// bound is cut to it, counted in characters, so that the migration passes
// and the playlist stays; the others keep theirs, a watchlist none. A
// longer name is then refused, at creation and at a change.
func TestPlaylistNameBoundMigrationCutsLongerNames(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "playlist_name_bound" })
	if at < 0 {
		t.Fatal("no playlist_name_bound migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	const bound = 500
	// Two bytes a character: the bound counts characters.
	long, whole := strings.Repeat("é", 300_000), strings.Repeat("é", bound)
	// The user comes with their watchlist, which has no name.
	var owner string
	if err := pool.QueryRow(ctx, "INSERT INTO users (name, password_hash) VALUES ('alice', 'x') RETURNING id").Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO playlists (owner_id, name) VALUES ($1, $2), ($1, $3), ($1, 'Mix')", owner, long, whole); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations[:at+1]); err != nil {
		t.Fatalf("with a longer name stored: %v", err)
	}
	var all, cut, kept, unnamed int
	if err := pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE name = $2), count(*) FILTER (WHERE name = 'Mix'),
		count(*) FILTER (WHERE watchlist AND name IS NULL) FROM playlists WHERE owner_id = $1`, owner, whole).Scan(&all, &cut, &kept, &unnamed); err != nil {
		t.Fatal(err)
	}
	// The longer name is cut to the bound, which the name of that length
	// already was.
	if all != 4 || cut != 2 || kept != 1 || unnamed != 1 {
		t.Errorf("%d playlists: %d names of the bound's length, %d kept, %d watchlists without one, want 4: 2, 1 and 1", all, cut, kept, unnamed)
	}
	over := whole + "é"
	if _, err := pool.Exec(ctx, "INSERT INTO playlists (owner_id, name) VALUES ($1, $2)", owner, over); err == nil {
		t.Error("a playlist is created under a name over the bound")
	}
	if _, err := pool.Exec(ctx, "UPDATE playlists SET name = $2 WHERE owner_id = $1 AND name = 'Mix'", owner, over); err == nil {
		t.Error("a playlist takes a name over the bound")
	}
	if _, err := pool.Exec(ctx, "UPDATE playlists SET name = $2 WHERE owner_id = $1 AND name = 'Mix'", owner, whole); err != nil {
		t.Errorf("a name of the bound's length: %v", err)
	}
}
