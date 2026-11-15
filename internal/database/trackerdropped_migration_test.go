package database

import (
	"errors"
	"io/fs"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The imports kept before the shows dropped were read them at their next
// import; the series dropped belong to a service that has them, and go
// with their user.
func TestTrackerDroppedMigration(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "tracker_dropped" })
	if at < 0 {
		t.Fatal("no tracker_dropped migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	var user string
	if err := pool.QueryRow(ctx, `WITH u AS (INSERT INTO users (name, password_hash) VALUES ('alice', 'x') RETURNING id)
		INSERT INTO tracking_imports (user_id, service, enabled, cursor) SELECT id, 'trakt', true, 'a|b' FROM u RETURNING user_id`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	var cursor string
	if err := pool.QueryRow(ctx, "SELECT dropped_cursor FROM tracking_imports WHERE user_id = $1", user).Scan(&cursor); err != nil || cursor != "" {
		t.Errorf("dropped cursor %q: %v", cursor, err)
	}

	insert := func(service, series string, seen any) error {
		_, err := pool.Exec(ctx, `INSERT INTO tracker_dropped (user_id, service, series_id, seen_at) VALUES ($1, $2, $3, $4)`,
			user, service, series, seen)
		return err
	}
	const series, other = "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"
	for _, service := range []string{"trakt", "simkl", "mdblist", "anilist", "myanimelist"} {
		if err := insert(service, series, "2026-10-05T12:00:00Z"); err != nil {
			t.Fatalf("%s: %v", service, err)
		}
	}
	var undated int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM tracker_dropped WHERE dropped_at IS NULL").Scan(&undated); err != nil || undated != 5 {
		t.Errorf("%d undated: %v", undated, err)
	}
	// PublicMetaDB tells no drop.
	var pgErr *pgconn.PgError
	if err := insert("publicmetadb", series, "2026-10-05T12:00:00Z"); !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Errorf("publicmetadb: %v", err)
	}
	// Each series dropped was seen dropped once.
	if err := insert("trakt", other, nil); !errors.As(err, &pgErr) || pgErr.Code != "23502" {
		t.Errorf("no seen_at: %v", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", user); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM tracker_dropped").Scan(&left); err != nil || left != 0 {
		t.Errorf("%d rows left after the user went: %v", left, err)
	}
}
