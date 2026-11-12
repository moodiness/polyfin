package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// A local folder's file whose container has no index now gets its images:
// the results telling that such a file cannot have them go, so that it is
// made again; the other results stay, a link's without an index too.
func TestLocalResultsWithoutAnIndexAreDropped(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "thumbnail_results_no_index" })
	if at < 0 {
		t.Fatal("no thumbnail_results_no_index migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO thumbnail_results (version_id, item_id, kind, state, reason, thumbnails) VALUES
		('00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000009', 'local', 'cannot', 'no_index', 0),
		('00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000009', 'local', 'cannot', 'no_video', 0),
		('00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000009', 'local', 'made', '', 12),
		('00000000-0000-0000-0000-000000000004', '00000000-0000-0000-0000-000000000009', 'link', 'cannot', 'no_index', 0)`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	var left []int
	rows, err := pool.Query(ctx, "SELECT get_byte(uuid_send(version_id), 15) FROM thumbnail_results ORDER BY version_id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		left = append(left, n)
	}
	if err := rows.Err(); err != nil || !slices.Equal(left, []int{2, 3, 4}) {
		t.Errorf("results left %v, %v: 2, 3 and 4 expected", left, err)
	}
}
