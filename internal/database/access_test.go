package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// Existing administrators keep controlling other users' apps once each
// user has their own permission; other users do not get it.
func TestUserAccessMigrationKeepsAdministratorsInControl(t *testing.T) {
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
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "user_access" })
	if at < 0 {
		t.Fatal("no user_access migration")
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
	rows, err := pool.Query(ctx, "SELECT name, remote_control, max_playbacks, max_bitrate, live_tv, sync_play FROM users ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type limits struct {
		name            string
		remote, liveTv  bool
		playbacks, rate int
		syncPlay        string
	}
	var got []limits
	for rows.Next() {
		var l limits
		if err := rows.Scan(&l.name, &l.remote, &l.playbacks, &l.rate, &l.liveTv, &l.syncPlay); err != nil {
			t.Fatal(err)
		}
		got = append(got, l)
	}
	want := []limits{
		{name: "admin", remote: true, liveTv: true, syncPlay: "CreateAndJoinGroups"},
		{name: "member", remote: false, liveTv: true, syncPlay: "CreateAndJoinGroups"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("users after the migration: %+v", got)
	}
	// The ranges hold in the database too.
	for _, change := range []string{"max_playbacks = 21", "max_playbacks = -1", "max_bitrate = -1", "sync_play = 'Sometimes'"} {
		if _, err := pool.Exec(ctx, "UPDATE users SET "+change); err == nil {
			t.Errorf("%s was stored", change)
		}
	}
}
