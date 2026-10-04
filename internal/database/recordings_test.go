package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// Existing administrators may record Live TV once each user has their own
// permission; other users may not. Recording settings start at
// Jellyfin's paddings, keeping recordings forever.
func TestRecordingsMigrationLetsAdministratorsRecord(t *testing.T) {
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
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "live_recordings" })
	if at < 0 {
		t.Fatal("no live_recordings migration")
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
	if err := pool.QueryRow(ctx, `SELECT bool_or(live_tv_management) FILTER (WHERE name = 'admin'),
		bool_or(live_tv_management) FILTER (WHERE name = 'member') FROM users`).Scan(&admin, &member); err != nil {
		t.Fatal(err)
	}
	if !admin || member {
		t.Errorf("may record after the migration: administrator %v, member %v", admin, member)
	}
	var pre, post, days int
	if err := pool.QueryRow(ctx, "SELECT recording_pre_padding, recording_post_padding, recording_retention_days FROM settings").
		Scan(&pre, &post, &days); err != nil || pre != 0 || post != 0 || days != 0 {
		t.Errorf("recording settings: %d %d %d %v", pre, post, days, err)
	}
	// The ranges hold in the database too.
	for _, change := range []string{"recording_pre_padding = -1", "recording_pre_padding = 3601", "recording_post_padding = 3601",
		"recording_retention_days = -1", "recording_retention_days = 3651"} {
		if _, err := pool.Exec(ctx, "UPDATE settings SET "+change); err == nil {
			t.Errorf("%s was stored", change)
		}
	}
}
