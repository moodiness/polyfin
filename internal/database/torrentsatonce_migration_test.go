package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// A new server, one with no account yet, runs 10 torrents at once at
// most; a server set up before keeps the value it had, no limit or its
// own.
func TestTorrentsAtOnceDefaultIsOnlyANewServers(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "torrents_at_once_default" })
	if at < 0 {
		t.Fatal("no torrents_at_once_default migration")
	}
	for name, tc := range map[string]struct {
		account bool
		before  int
		want    int
	}{
		"a new server":                  {false, 0, 10},
		"a server set up, no limit":     {true, 0, 0},
		"a server set up, a limit":      {true, 4, 4},
		"a server with no account, set": {false, 4, 4},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pool := testdb.Empty(t)
			ctx := t.Context()
			if err := migrate(ctx, pool, migrations[:at]); err != nil {
				t.Fatal(err)
			}
			if tc.account {
				if _, err := pool.Exec(ctx, "INSERT INTO users (name, password_hash, is_administrator) VALUES ('admin', 'x', true)"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := pool.Exec(ctx, "UPDATE settings SET torrents_at_once = $1", tc.before); err != nil {
				t.Fatal(err)
			}
			if err := migrate(ctx, pool, migrations[:at+1]); err != nil {
				t.Fatal(err)
			}
			var got int
			if err := pool.QueryRow(ctx, "SELECT torrents_at_once FROM settings").Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("torrents at once: %d, want %d", got, tc.want)
			}
		})
	}
}
