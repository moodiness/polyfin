package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// A folder's scan may fail as out of time once the migration ran: before
// it, the folder keeps no such failure. The failures known before stay.
func TestShareScanTimeMigrationKeepsAScanOutOfTime(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "share_scan_time" })
	if at < 0 {
		t.Fatal("no share_scan_time migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `WITH a AS (INSERT INTO addons (manifest_url, manifest, position, kind)
			VALUES ('smb://nas.example/media/movies', '{"id": "polyfin.local.movies", "version": "1", "name": "NAS"}', 1, 'local') RETURNING id)
		INSERT INTO local_folders (addon_id, kind, error) SELECT id, 'movies', 'too_many_files' FROM a`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE local_folders SET error = 'too_slow'"); err == nil {
		t.Fatal("too_slow is kept before the migration")
	}
	if err := migrate(ctx, pool, migrations[:at+1]); err != nil {
		t.Fatal(err)
	}
	var failure string
	if err := pool.QueryRow(ctx, "SELECT error FROM local_folders").Scan(&failure); err != nil || failure != "too_many_files" {
		t.Fatalf("the folder's failure is %q after the migration, %v; want too_many_files", failure, err)
	}
	for _, failure := range []string{"too_slow", "", "missing", "unreadable", "not_folder", "unreachable", "refused", "host_key_changed",
		"too_deep", "too_many_folders", "too_many_files", "listing_too_large"} {
		if _, err := pool.Exec(ctx, "UPDATE local_folders SET error = $1", failure); err != nil {
			t.Errorf("%q is not kept after the migration: %v", failure, err)
		}
	}
	if _, err := pool.Exec(ctx, "UPDATE local_folders SET error = 'too_large'"); err == nil {
		t.Error("a failure the scans do not know is kept")
	}
}
