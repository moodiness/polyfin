package database

import (
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The users of the rows shown to chosen users before count as told and as
// having seen them; a row may then carry a note of 300 characters at most,
// tell its users only when shown to chosen ones, and leave out the titles
// played only for the kinds that offer it.
func TestHomeRowSuggestionsMigration(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "home_row_suggestions" })
	if at < 0 {
		t.Fatal("no home_row_suggestions migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `WITH u AS (INSERT INTO users (name, password_hash) VALUES ('alice', 'x') RETURNING id),
		r AS (INSERT INTO home_rows (position, name, kind, place, everyone) VALUES (1, 'For Alice', 'new', 'top', false) RETURNING id)
		INSERT INTO home_row_users (row_id, user_id) SELECT r.id, u.id FROM r, u`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO home_rows (position, name, kind, place) VALUES (2, 'Everyone', 'new', 'top')`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	var notices, settled int
	if err := pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE told_at IS NOT NULL AND seen_at IS NOT NULL AND apps_at IS NULL)
		FROM home_row_notices`).Scan(&notices, &settled); err != nil {
		t.Fatal(err)
	}
	if notices != 1 || settled != 1 {
		t.Errorf("notices: %d, told and seen: %d", notices, settled)
	}
	for statement, args := range map[string][]any{
		"INSERT INTO home_rows (position, name, kind, place, everyone, notify, note) VALUES (3, 'Told', 'new', 'top', false, true, $1)": {
			strings.Repeat("é", 300)},
		"INSERT INTO home_rows (position, name, kind, place, leave_played) VALUES (4, 'Unplayed', 'new', 'top', true)": nil,
	} {
		if _, err := pool.Exec(ctx, statement, args...); err != nil {
			t.Errorf("%s: %v", statement, err)
		}
	}
	for statement, args := range map[string][]any{
		"INSERT INTO home_rows (position, name, kind, place, note) VALUES (5, 'Long', 'new', 'top', $1)":              {strings.Repeat("é", 301)},
		"INSERT INTO home_rows (position, name, kind, place, notify) VALUES (5, 'Everyone told', 'new', 'top', true)": nil,
		"INSERT INTO home_rows (position, name, kind, place, leave_played) VALUES (5, 'Picks', 'picks', 'top', true)": nil,
	} {
		if _, err := pool.Exec(ctx, statement, args...); err == nil {
			t.Errorf("%s: accepted", statement)
		}
	}
	// A user's deletion takes their notices, and forgets them as a signer.
	if _, err := pool.Exec(ctx, "UPDATE home_rows SET note = 'Hi', note_by = (SELECT id FROM users) WHERE name = 'Told'"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM users"); err != nil {
		t.Fatal(err)
	}
	var left, signed int
	if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM home_row_notices), (SELECT count(*) FROM home_rows WHERE note_by IS NOT NULL)").Scan(
		&left, &signed); err != nil {
		t.Fatal(err)
	}
	if left != 0 || signed != 0 {
		t.Errorf("after the user's deletion: %d notices, %d rows signed", left, signed)
	}
}
