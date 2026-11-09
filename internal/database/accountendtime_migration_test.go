package database

import (
	"io/fs"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The accounts given an end date before keep it, as it was, without an
// exact end; an account has a date or an exact end, never both, and an
// administrator neither.
func TestAccountEndTimeMigrationKeepsTheEndDates(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "account_end_time" })
	if at < 0 {
		t.Fatal("no account_end_time migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (name, password_hash, ends_on, end_recorded) VALUES
		('ended', '', '2026-10-20', true), ('ending', '', '2026-12-01', false), ('never', '', NULL, false)`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, "SELECT name, ends_on, ends_at IS NULL, end_recorded FROM users ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		var on *time.Time
		var noExactEnd, recorded bool
		if err := rows.Scan(&name, &on, &noExactEnd, &recorded); err != nil {
			t.Fatal(err)
		}
		date := "never"
		if on != nil {
			date = on.Format(time.DateOnly)
		}
		if !noExactEnd {
			t.Errorf("%s has an exact end", name)
		}
		got = append(got, name+" "+date+" "+map[bool]string{true: "recorded", false: "not recorded"}[recorded])
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"ended 2026-10-20 recorded", "ending 2026-12-01 not recorded", "never never not recorded"}; !slices.Equal(got, want) {
		t.Errorf("users %q, want %q", got, want)
	}
	for _, tc := range []struct {
		name, sql string
		ok        bool
	}{
		{"an exact end", `INSERT INTO users (name, password_hash, ends_at) VALUES ('guest', '', '2026-11-09 18:40+01')`, true},
		{"both ends", `INSERT INTO users (name, password_hash, ends_on, ends_at) VALUES ('both', '', '2026-11-09', '2026-11-09 18:40+01')`, false},
		{"an administrator's exact end", `INSERT INTO users (name, password_hash, is_administrator, ends_at) VALUES ('admin', '', true, '2026-11-09 18:40+01')`, false},
		{"an administrator's date", `INSERT INTO users (name, password_hash, is_administrator, ends_on) VALUES ('admin', '', true, '2026-11-09')`, false},
	} {
		_, err := pool.Exec(ctx, tc.sql)
		if (err == nil) != tc.ok {
			t.Errorf("%s: error %v, want accepted %v", tc.name, err, tc.ok)
		}
	}
}
