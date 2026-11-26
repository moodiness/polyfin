package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The migration gives no existing user a PIN, keeps a hash and a count of
// wrong guesses that make sense, and a user's PIN while the user lasts.
func TestMoonfinPinsMigrationKeepsAPinWhileItsUserLasts(t *testing.T) {
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
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "moonfin_pins" })
	if at < 0 {
		t.Fatal("no moonfin_pins migration")
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
	var pins int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM moonfin_pins").Scan(&pins); err != nil || pins != 0 {
		t.Fatalf("PINs after the migration: %d %v", pins, err)
	}
	for _, values := range []string{"($1, '', 0, now())", "($1, 'hash', -1, now())"} {
		if _, err := pool.Exec(ctx, "INSERT INTO moonfin_pins (user_id, hash, wrong_guesses, updated_at) VALUES "+values, id); err == nil {
			t.Errorf("kept %s", values)
		}
	}
	if _, err := pool.Exec(ctx, "INSERT INTO moonfin_pins (user_id, hash, wrong_guesses, wait_until, updated_at) VALUES ($1, 'hash', 6, now(), now())", id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM moonfin_pins").Scan(&pins); err != nil || pins != 0 {
		t.Errorf("PINs left after their user: %d %v", pins, err)
	}
}
