package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The IMDb identifiers of TMDB titles kept before stay, as found; those
// kept without one go, to be asked again; a title not found cannot have an
// IMDb identifier. A check of a moviedb_id that is right has nothing to
// replace it, while a wrong one is replaced or dropped.
func TestMoviedbIDChecksMigrationKeepsOnlyKnownIMDbIDs(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "moviedb_id_checks" })
	if at < 0 {
		t.Fatal("no moviedb_id_checks migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO title_imdb_ids (kind, tmdb_id, imdb_id, asked_at) VALUES
		('tv', 19614, 'tt0099864', now()), ('tv', 100088, 'tt3581920', now()),
		('tv', 113981, NULL, now()), ('tv', 502119, NULL, now())`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, "SELECT tmdb_id, imdb_id, found FROM title_imdb_ids ORDER BY tmdb_id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var id int
		var imdb string
		var found bool
		if err := rows.Scan(&id, &imdb, &found); err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Errorf("%d not found", id)
		}
		got = append(got, imdb)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"tt0099864", "tt3581920"}; !slices.Equal(got, want) {
		t.Errorf("kept %v, want %v", got, want)
	}
	for _, tc := range []struct {
		name, sql string
		ok        bool
	}{
		{"not found with an IMDb id", `INSERT INTO title_imdb_ids (kind, tmdb_id, imdb_id, found, asked_at) VALUES ('tv', 502119, 'tt3581920', false, now())`, false},
		{"not found without an IMDb id", `INSERT INTO title_imdb_ids (kind, tmdb_id, imdb_id, found, asked_at) VALUES ('tv', 502119, NULL, false, now())`, true},
		{"right and replaced", `INSERT INTO moviedb_id_checks VALUES ('tv', 19614, 'tt0099864', 0, false, 1, now())`, false},
		{"right", `INSERT INTO moviedb_id_checks VALUES ('tv', 19614, 'tt0099864', 0, false, NULL, now())`, true},
		{"wrong and replaced", `INSERT INTO moviedb_id_checks VALUES ('tv', 113981, 'tt0099864', 0, true, 19614, now())`, true},
		{"wrong and dropped", `INSERT INTO moviedb_id_checks VALUES ('tv', 502119, 'tt3581920', 0, true, NULL, now())`, true},
	} {
		_, err := pool.Exec(ctx, tc.sql)
		if (err == nil) != tc.ok {
			t.Errorf("%s: error %v, want accepted %v", tc.name, err, tc.ok)
		}
	}
}
