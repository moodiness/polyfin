package database

import (
	"io/fs"
	"slices"
	"strconv"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The ahead limit, counted in segments of about 6 s, becomes seconds: the
// former default the new one, 120 s, another value its length within the
// new bounds.
func TestAheadSegmentsBecomeSeconds(t *testing.T) {
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "ahead_seconds" })
	if at < 0 {
		t.Fatal("no ahead_seconds migration")
	}
	for segments, seconds := range map[int]int{10: 120, 1: 30, 4: 30, 20: 120, 30: 180, 60: 360} {
		t.Run(strconv.Itoa(segments), func(t *testing.T) {
			pool := testdb.New(t)
			ctx := t.Context()
			if err := migrate(ctx, pool, migrations[:at]); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, "UPDATE settings SET ahead_segments = $1", segments); err != nil {
				t.Fatal(err)
			}
			if err := migrate(ctx, pool, migrations); err != nil {
				t.Fatal(err)
			}
			var got int
			if err := pool.QueryRow(ctx, "SELECT ahead_seconds FROM settings").Scan(&got); err != nil || got != seconds {
				t.Errorf("%d segments became %d s, want %d: %v", segments, got, seconds, err)
			}
		})
	}
}
