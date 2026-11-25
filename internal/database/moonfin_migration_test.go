package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The migration serves Moonfin's companion to every existing user, none
// set apart, and keeps a user's synced settings while the user lasts.
func TestMoonfinMigrationServesEveryUser(t *testing.T) {
	t.Parallel()
	pool := testdb.Empty(t)
	ctx := t.Context()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "moonfin" })
	if at < 0 {
		t.Fatal("no moonfin migration")
	}
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := pool.QueryRow(ctx, "INSERT INTO users (name, password_hash) VALUES ('member', 'x') RETURNING id").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	var served bool
	var choice string
	var locked bool
	if err := pool.QueryRow(ctx, "SELECT s.moonfin, u.moonfin, u.moonfin_locked FROM settings s, users u WHERE u.id = $1", id).Scan(&served, &choice, &locked); err != nil {
		t.Fatal(err)
	}
	if !served || choice != "" || locked {
		t.Errorf("served %v, choice %q, locked %v", served, choice, locked)
	}
	if _, err := pool.Exec(ctx, "UPDATE users SET moonfin = 'sometimes' WHERE id = $1", id); err == nil {
		t.Error("a choice of another value was kept")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO moonfin_settings (user_id, envelope, updated_at) VALUES ($1, '{"global": {}}', now())`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO moonfin_defaults (profile, settings) VALUES ('watch', '{}')"); err == nil {
		t.Error("defaults for an unknown profile were kept")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM moonfin_settings").Scan(&left); err != nil || left != 0 {
		t.Errorf("settings left after their user: %d %v", left, err)
	}
}
