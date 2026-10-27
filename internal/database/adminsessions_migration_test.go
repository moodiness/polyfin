package database

import (
	"io/fs"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/testdb"
)

// Admin sessions opened before they named their device end on upgrade:
// nothing tells those taken from a web client, which could outlive its
// sign-out, from those opened with a password.
func TestAdminSessionDeviceMigrationEndsEarlierSessions(t *testing.T) {
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "admin_session_device" })
	if at < 0 {
		t.Fatal("no admin_session_device migration")
	}
	pool := testdb.New(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `WITH u AS (INSERT INTO users (name, password_hash, is_administrator) VALUES ('root', 'x', true) RETURNING id)
		INSERT INTO admin_sessions (token_hash, user_id, expires_at) SELECT 'before', id, $1 FROM u`, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM admin_sessions").Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d admin sessions from before the upgrade remain", left)
	}
}
