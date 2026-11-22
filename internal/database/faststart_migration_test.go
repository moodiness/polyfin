package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// Start remote plays faster is on for a new server and for one still at
// the old default, off; a server that turned it on keeps it, and off
// chosen after the change stays off when Polyfin migrates again, as at
// each start.
func TestFastStartMigrationTurnsTheOldDefaultOn(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "fast_start_default" })
	if at < 0 {
		t.Fatal("no fast_start_default migration")
	}
	for name, tc := range map[string]struct {
		before *bool
		want   bool
	}{
		"a new server":           {nil, true},
		"the old default, off":   {new(false), true},
		"turned on by its admin": {new(true), true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pool := testdb.Empty(t)
			ctx := t.Context()
			if tc.before != nil {
				if err := migrate(ctx, pool, migrations[:at]); err != nil {
					t.Fatal(err)
				}
				if _, err := pool.Exec(ctx, "UPDATE settings SET fast_start = $1", *tc.before); err != nil {
					t.Fatal(err)
				}
			}
			if err := migrate(ctx, pool, migrations); err != nil {
				t.Fatal(err)
			}
			var got bool
			if err := pool.QueryRow(ctx, "SELECT fast_start FROM settings").Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("the setting became %v, want %v", got, tc.want)
			}
			// An administrator's choice of off, stored after the change,
			// stays as the migrations run again.
			if _, err := pool.Exec(ctx, "UPDATE settings SET fast_start = false"); err != nil {
				t.Fatal(err)
			}
			if err := migrate(ctx, pool, migrations); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, "SELECT fast_start FROM settings").Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got {
				t.Error("off, chosen after the change, was turned on again")
			}
		})
	}
}
