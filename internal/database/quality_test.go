package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// Existing users keep every version, Original, once users have a quality
// group; only the groups offered can be stored.
func TestQualityGroupsMigrationKeepsUsersOnOriginal(t *testing.T) {
	pool := testdb.New(t)
	ctx := t.Context()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "quality_groups" })
	if at < 0 {
		t.Fatal("no quality_groups migration")
	}
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (name, password_hash, is_administrator, is_hidden)
		VALUES ('admin', 'x', true, true), ('member', 'x', false, false)`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	var kept int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM users WHERE quality_group = 0").Scan(&kept); err != nil || kept != 2 {
		t.Errorf("users on Original after the migration: %d %v", kept, err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO users (name, password_hash) VALUES ('new', 'x')"); err != nil {
		t.Fatal(err)
	}
	var group int
	if err := pool.QueryRow(ctx, "SELECT quality_group FROM users WHERE name = 'new'").Scan(&group); err != nil || group != 0 {
		t.Errorf("a new user's group: %d %v", group, err)
	}
	for _, height := range []int{480, 720, 1080, 1440, 2160, 0} {
		if _, err := pool.Exec(ctx, "UPDATE users SET quality_group = $1 WHERE name = 'member'", height); err != nil {
			t.Errorf("group %d refused: %v", height, err)
		}
	}
	for _, height := range []int{-1, 1, 576, 1000, 4320} {
		if _, err := pool.Exec(ctx, "UPDATE users SET quality_group = $1 WHERE name = 'member'", height); err == nil {
			t.Errorf("group %d was stored", height)
		}
	}
}
