package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// Existing administrators may manage subtitles once each user has the
// permission; other users may not.
func TestAppsMigrationLetsAdministratorsManageSubtitles(t *testing.T) {
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
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "apps" })
	if at < 0 {
		t.Fatal("no apps migration")
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
	var admin, member bool
	if err := pool.QueryRow(ctx, `SELECT (SELECT subtitle_management FROM users WHERE name = 'admin'),
		(SELECT subtitle_management FROM users WHERE name = 'member')`).Scan(&admin, &member); err != nil || !admin || member {
		t.Errorf("administrator %v, member %v, %v", admin, member, err)
	}
	// A picture comes whole, with its type and tag, within 5 MB.
	for statement, valid := range map[string]bool{
		`UPDATE users SET image = '\x01', image_type = 'image/png', image_tag = repeat('a', 32)`:                               true,
		`UPDATE users SET image = '\x01', image_type = 'image/gif', image_tag = repeat('a', 32)`:                               false,
		`UPDATE users SET image = '\x01', image_type = 'image/png', image_tag = ''`:                                            false,
		`UPDATE users SET image = NULL, image_type = 'image/png', image_tag = repeat('a', 32)`:                                 false,
		`UPDATE users SET image = decode(repeat('00', 5242881), 'hex'), image_type = 'image/png', image_tag = repeat('a', 32)`: false,
	} {
		if _, err := pool.Exec(ctx, statement); (err == nil) != valid {
			t.Errorf("%s: %v", statement, err)
		}
	}
}
