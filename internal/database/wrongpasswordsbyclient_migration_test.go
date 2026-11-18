package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The counts and blocks in force when the migration runs were of whole
// accounts, for every client: they end, and no client starts with a count.
func TestWrongPasswordsByClientMigrationEndsTheBlocksInForce(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "wrong_passwords_by_client" })
	if at < 0 {
		t.Fatal("no wrong_passwords_by_client migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (name, password_hash, invalid_login_attempts, blocked_until) VALUES
		('blocked', 'x', 3, now() + interval '10 minutes'),
		('counting', 'x', 2, NULL),
		('untouched', 'x', 0, NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	var shown, clients int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM users WHERE invalid_login_attempts <> 0 OR blocked_until IS NOT NULL),
		(SELECT count(*) FROM wrong_passwords)`).Scan(&shown, &clients); err != nil {
		t.Fatal(err)
	}
	if shown != 0 || clients != 0 {
		t.Errorf("after the migration: %d accounts show a count or a block, %d clients kept", shown, clients)
	}
}
