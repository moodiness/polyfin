package database

import (
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The collation version changed only when both versions are known and
// differ: a database without a recorded version, or a provider that gives
// none, as the C locale's, is no change.
func TestCollationChanged(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name             string
		recorded, actual string
		want             bool
	}{
		{"equal", "2.41", "2.41", false},
		{"different", "2.36", "2.41", true},
		{"none recorded", "", "2.41", false},
		{"none provided", "2.36", "", false},
		{"neither", "", "", false},
	} {
		got := Collation{Database: "polyfin", Provider: "libc", Recorded: c.recorded, Actual: c.actual}.Changed()
		if got != c.want {
			t.Errorf("%s: Changed() = %v, want %v", c.name, got, c.want)
		}
	}
}

// The test database's collation version is the one its system provides,
// as on any server whose PostgreSQL did not change it: no change.
func TestReadCollationOfAnUnchangedDatabase(t *testing.T) {
	t.Parallel()
	pool := testdb.Empty(t)
	ctx := t.Context()
	c, err := ReadCollation(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	var name string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if c.Database != name {
		t.Errorf("Database = %q, want %q", c.Database, name)
	}
	if !slices.Contains([]string{"libc", "icu", "builtin"}, c.Provider) {
		t.Errorf("Provider = %q", c.Provider)
	}
	if c.Recorded != c.Actual {
		t.Errorf("Recorded = %q, Actual = %q: the test database's versions should match", c.Recorded, c.Actual)
	}
	if c.Changed() {
		t.Errorf("Changed() = true for %+v", c)
	}
}
