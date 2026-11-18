package database

import (
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/moodiness/polyfin/internal/testdb"
)

// Display preferences get their bounds on a server that already keeps
// some: the rows over a bound go, value and all, since a value cut short
// is no longer the app's, and every other row stays, with a time of
// writing. The table then refuses a value or a client's name over its
// bound.
func TestDisplayPreferenceBoundsRemoveOnlyTheRowsOverThem(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "display_preference_bounds" })
	if at < 0 {
		t.Fatal("no display_preference_bounds migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	// The database prints {"a": "…"}: nine bytes around the text.
	full, over := `{"a": "`+strings.Repeat("x", 131072-9)+`"}`, `{"a": "`+strings.Repeat("x", 131072-8)+`"}`
	longest := strings.Repeat("é", 32)
	if _, err := pool.Exec(ctx, `WITH u AS (
			INSERT INTO users (name, password_hash) VALUES ('alice', 'x'), ('bob', 'x') RETURNING id, name
		)
		INSERT INTO display_preferences (user_id, preference_id, client, value)
		SELECT u.id, p.id, p.client, p.value::jsonb
		FROM u JOIN (VALUES
			('alice', 'home', 'emby', '{"SortBy": "Name"}'),
			('alice', 'full', 'emby', $1),
			('alice', 'over', 'emby', $2),
			('alice', 'home', $3, '{"SortBy": "Random"}'),
			('alice', 'home', $3 || 'x', '{"SortBy": "Random"}'),
			('bob', 'home', 'emby', '{"SortBy": "DateCreated"}'),
			('bob', 'over', 'emby', $2)
		) p (name, id, client, value) ON p.name = u.name`, full, over, longest); err != nil {
		t.Fatal(err)
	}
	var sizes []int
	if err := pool.QueryRow(ctx, `SELECT array_agg(octet_length(value::text) ORDER BY preference_id)
		FROM display_preferences WHERE preference_id IN ('full', 'over') AND client = 'emby'`).Scan(&sizes); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sizes, []int{131072, 131073, 131073}) {
		t.Fatalf("the values' sizes before the migration: %v", sizes)
	}

	if err := migrate(ctx, pool, migrations[:at+1]); err != nil {
		t.Fatal(err)
	}
	var left []string
	if err := pool.QueryRow(ctx, `SELECT array_agg(u.name || '/' || p.preference_id || '/' || octet_length(p.client)
			ORDER BY u.name, p.preference_id, octet_length(p.client))
		FROM display_preferences p JOIN users u ON u.id = p.user_id
		WHERE p.updated_at <= now()`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if want := []string{"alice/full/4", "alice/home/4", "alice/home/64", "bob/home/4"}; !slices.Equal(left, want) {
		t.Errorf("rows left: %v, want %v", left, want)
	}
	var sortBy string
	if err := pool.QueryRow(ctx, `SELECT value->>'SortBy' FROM display_preferences p JOIN users u ON u.id = p.user_id
		WHERE u.name = 'bob'`).Scan(&sortBy); err != nil || sortBy != "DateCreated" {
		t.Errorf("bob's row left: %q %v", sortBy, err)
	}

	for _, refused := range []struct{ what, statement, value string }{
		{"a value over the bound", `UPDATE display_preferences SET value = $1::jsonb WHERE preference_id = 'home' AND client = 'emby'`, over},
		{"a value that is not an object", `UPDATE display_preferences SET value = $1::jsonb WHERE preference_id = 'full'`, `[]`},
		{"a client's name over the bound", `UPDATE display_preferences SET client = $1 WHERE preference_id = 'full'`, longest + "x"},
		{"a new row with a value over the bound", `INSERT INTO display_preferences (user_id, preference_id, client, value)
			SELECT id, 'new', 'emby', $1::jsonb FROM users`, over},
		{"a new row with a client's name over the bound", `INSERT INTO display_preferences (user_id, preference_id, client, value)
			SELECT id, 'new', $1, '{}' FROM users`, longest + "x"},
	} {
		_, err := pool.Exec(ctx, refused.statement, refused.value)
		// 23514 is a check violation.
		if violation := (*pgconn.PgError)(nil); !errors.As(err, &violation) || violation.Code != "23514" {
			t.Errorf("%s: %v, want a check violation", refused.what, err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE display_preferences SET value = $1::jsonb, client = $2 WHERE preference_id = 'full'`, full, longest); err != nil {
		t.Errorf("a value and a client's name of their bounds' sizes: %v", err)
	}
}
