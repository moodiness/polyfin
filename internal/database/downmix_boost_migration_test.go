package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The volume of a stereo mix moves to Jellyfin's 2 on servers still at the
// old default, 1, and a new server starts at 2; a server that chose
// another volume keeps it.
func TestDownmixBoostMigrationMovesOnlyTheOldDefault(t *testing.T) {
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "downmix_boost_default" })
	if at < 0 {
		t.Fatal("no downmix_boost_default migration")
	}
	for name, tc := range map[string]struct {
		before *float64
		want   float64
	}{
		"a new server":       {nil, 2},
		"the old default":    {new(1.0), 2},
		"a volume of 1.5":    {new(1.5), 1.5},
		"the lowest volume":  {new(0.5), 0.5},
		"the highest volume": {new(3.0), 3},
	} {
		t.Run(name, func(t *testing.T) {
			pool := testdb.New(t)
			ctx := t.Context()
			if tc.before != nil {
				if err := migrate(ctx, pool, migrations[:at]); err != nil {
					t.Fatal(err)
				}
				if _, err := pool.Exec(ctx, "UPDATE settings SET downmix_boost = $1", *tc.before); err != nil {
					t.Fatal(err)
				}
			}
			if err := migrate(ctx, pool, migrations); err != nil {
				t.Fatal(err)
			}
			var got float64
			if err := pool.QueryRow(ctx, "SELECT downmix_boost FROM settings").Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("the volume became %v, want %v", got, tc.want)
			}
		})
	}
}
