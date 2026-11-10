package database

import (
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// A member who no longer followed their group keeps every section as
// their own; one who followed it, and a user in no group, keep none. Only
// the sections of a group's page are accepted.
func TestGroupOwnSectionsMigrationKeepsWhatMembersFollowed(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "group_own_sections" })
	if at < 0 {
		t.Fatal("no group_own_sections migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `WITH g AS (INSERT INTO user_groups (name) VALUES ('Kids') RETURNING id)
		INSERT INTO users (name, password_hash, group_id, follow_group) VALUES
			('follows', '', (SELECT id FROM g), true), ('own', '', (SELECT id FROM g), false), ('alone', '', NULL, true)`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, "SELECT name, own_sections FROM users ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		var own []string
		if err := rows.Scan(&name, &own); err != nil {
			t.Fatal(err)
		}
		got = append(got, name+" "+strings.Join(own, ","))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"alone ", "follows ", "own access,playback,parental,libraries,genres,hours,channels"}; !slices.Equal(got, want) {
		t.Errorf("users %q, want %q", got, want)
	}
	if _, err := pool.Exec(ctx, "UPDATE users SET own_sections = '{name}' WHERE name = 'own'"); err == nil {
		t.Error("an unknown section was accepted")
	}
}
