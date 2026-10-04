package database

import (
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// Existing administrators may manage collections once each user has the
// permission; other users may not.
func TestCollectionsMigrationLetsAdministratorsManageThem(t *testing.T) {
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
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "collections" })
	if at < 0 {
		t.Fatal("no collections migration")
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
	rows, err := pool.Query(ctx, "SELECT name FROM users WHERE collection_management ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	var managers []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		managers = append(managers, name)
	}
	if rows.Close(); !slices.Equal(managers, []string{"admin"}) {
		t.Errorf("users who may manage collections: %v", managers)
	}
	// A collection's name has a bounded length.
	if _, err := pool.Exec(ctx, "INSERT INTO collections (name) VALUES ($1)", strings.Repeat("a", 501)); err == nil {
		t.Error("a name of 501 characters was stored")
	}
}
