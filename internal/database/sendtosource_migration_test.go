package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// Send apps to the source, a switch, becomes a choice: on becomes
// everyone, off, the default, owners only, and a new server starts at
// owners only. The old column is gone, and a choice the server does not
// know is refused.
func TestSendToSourceMigrationKeepsTheSwitchAsAChoice(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "send_to_source" })
	if at < 0 {
		t.Fatal("no send_to_source migration")
	}
	for name, tc := range map[string]struct {
		before *bool
		want   string
	}{
		"a new server": {nil, "owners"},
		"off":          {new(false), "owners"},
		"on":           {new(true), "everyone"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pool := testdb.Empty(t)
			ctx := t.Context()
			if tc.before != nil {
				if err := migrate(ctx, pool, migrations[:at]); err != nil {
					t.Fatal(err)
				}
				if _, err := pool.Exec(ctx, "UPDATE settings SET send_apps_to_source = $1", *tc.before); err != nil {
					t.Fatal(err)
				}
			}
			if err := migrate(ctx, pool, migrations); err != nil {
				t.Fatal(err)
			}
			var got string
			if err := pool.QueryRow(ctx, "SELECT send_to_source FROM settings").Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("the setting became %q, want %q", got, tc.want)
			}
			var left bool
			if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM information_schema.columns
				WHERE table_schema = current_schema() AND table_name = 'settings' AND column_name = 'send_apps_to_source')`).Scan(&left); err != nil || left {
				t.Errorf("the old column is left: %v %v", left, err)
			}
			if _, err := pool.Exec(ctx, "UPDATE settings SET send_to_source = 'none'"); err != nil {
				t.Errorf("no one is refused: %v", err)
			}
			if _, err := pool.Exec(ctx, "UPDATE settings SET send_to_source = 'admins'"); err == nil {
				t.Error("an unknown choice is stored")
			}
		})
	}
}
