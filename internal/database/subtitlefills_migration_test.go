package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The migration keeps the subtitle indexes kept before, with no streams
// awaiting known (NULL), so that one leaving streams out is read again;
// those kept since tell the streams awaiting a fill, none included.
func TestSubtitleFillsMigrationKeepsTheIndexesKeptBefore(t *testing.T) {
	t.Parallel()
	pool := testdb.Empty(t)
	ctx := t.Context()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "subtitle_fills" })
	if at < 0 {
		t.Fatal("no subtitle_fills migration")
	}
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	const before, since, none = "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002", "00000000-0000-0000-0000-000000000003"
	if _, err := pool.Exec(ctx, "INSERT INTO media_subtitle_index (version_id, located) VALUES ($1, '{2}')", before); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	var located []int32
	var unknown bool
	if err := pool.QueryRow(ctx, "SELECT located, awaiting IS NULL FROM media_subtitle_index WHERE version_id = $1", before).
		Scan(&located, &unknown); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(located, []int32{2}) || !unknown {
		t.Errorf("an index kept before: located %v, awaiting unknown: %v", located, unknown)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO media_subtitle_index (version_id, located, awaiting) VALUES ($1, '{2}', '{3, 4}'), ($2, '{}', '{}')",
		since, none); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string][]int32{since: {3, 4}, none: {}} {
		var awaiting []int32
		if err := pool.QueryRow(ctx, "SELECT awaiting FROM media_subtitle_index WHERE version_id = $1", id).Scan(&awaiting); err != nil {
			t.Fatal(err)
		}
		if awaiting == nil || !slices.Equal(awaiting, want) {
			t.Errorf("awaiting %v, want %v", awaiting, want)
		}
	}
}
