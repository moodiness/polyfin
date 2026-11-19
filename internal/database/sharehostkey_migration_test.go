package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// An SFTP folder without a saved host key may have moved to another
// server: after the migration, the next key it is offered is shown to be
// accepted. A folder with a saved key, and other kinds of folders, keep
// theirs as they were.
func TestShareHostKeyForgottenMigrationShowsTheNextKey(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "share_host_key_forgotten" })
	if at < 0 {
		t.Fatal("no share_host_key_forgotten migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	folders := []struct {
		address, hostKey string
		forgotten        bool
	}{
		{"sftp://nas.example/movies", "", true},
		{"SFTP://other.example:2222/shows", "", true},
		{"sftp://saved.example/movies", "ssh-ed25519 AAAA", false},
		{"smb://nas.example/movies", "", false},
		{"/media/movies", "", false},
	}
	for i, f := range folders {
		if _, err := pool.Exec(ctx, `WITH a AS (INSERT INTO addons (manifest_url, manifest, position, kind)
				VALUES ($1, '{"id": "polyfin.local", "version": "1", "name": "NAS"}', $2, 'local') RETURNING id)
			INSERT INTO local_folders (addon_id, kind, share_host_key) SELECT id, 'movies', $3 FROM a`, f.address, i+1, f.hostKey); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrate(ctx, pool, migrations[:at+1]); err != nil {
		t.Fatal(err)
	}
	for _, f := range folders {
		var forgotten bool
		if err := pool.QueryRow(ctx, `SELECT share_host_key_forgotten FROM local_folders f JOIN addons a ON a.id = f.addon_id
			WHERE a.manifest_url = $1`, f.address).Scan(&forgotten); err != nil || forgotten != f.forgotten {
			t.Errorf("%s: forgotten %v, want %v (%v)", f.address, forgotten, f.forgotten, err)
		}
	}
}
