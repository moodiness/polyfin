package backup

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/testdb"
)

// The addresses .strm files name are in a backup as the database holds
// them, sealed: the backup never shows them, and a restore with the key
// brings them back readable. One sealed with another key counts among the
// addresses that key cannot open.
func TestStrmAddressesAreBackedUpSealed(t *testing.T) {
	t.Parallel()
	testdb.LockRestores(t)
	r := newRestoring(t)
	ctx := t.Context()
	const address = "https://cdn.example/a.mp4?token=strm-backup-key"
	key := r.service.cfg.Secrets
	other := newBox(t, []byte("fedcba9876543210fedcba9876543210"))
	if _, err := r.pool.Exec(ctx, `WITH folder AS (INSERT INTO addons (owner_id, kind, manifest_url, manifest, position)
			VALUES (NULL, 'local', '/media/movies', '{"name": "Shelf"}', 1) RETURNING id),
		kind AS (INSERT INTO local_folders (addon_id, kind) SELECT id, 'movies' FROM folder RETURNING addon_id)
		INSERT INTO local_files (addon_id, path, size, modified, unit, stream_url)
		SELECT addon_id, f.path, 40, now(), f.path, f.address FROM kind, (VALUES ('A (2001).strm', $1), ('B (2002).strm', $2)) f (path, address)`,
		key.Seal(address), other.Seal("https://cdn.example/b.mp4")); err != nil {
		t.Fatal(err)
	}
	name := r.backUp(t, time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local))
	rows, err := r.service.pgRestore(ctx, "--data-only", "--file=-", "--table=local_files", filepath.Join(r.dir, name))
	if err != nil || !bytes.Contains(rows, []byte("A (2001).strm")) {
		t.Fatalf("the backup's files: %s %v", rows, err)
	}
	if bytes.Contains(rows, []byte("strm-backup-key")) || bytes.Contains(rows, []byte("cdn.example")) {
		t.Errorf("the backup holds an address in the clear: %s", rows)
	}
	if _, err := r.pool.Exec(ctx, "DELETE FROM local_files"); err != nil {
		t.Fatal(err)
	}
	plan, err := r.service.Prepare(ctx, nil, name)
	if err != nil {
		t.Fatal(err)
	}
	if plan.UnreadableCount() != 1 || plan.Unreadable[secrets.KindAddress] != 1 {
		t.Errorf("unreadable: %v", plan.Unreadable)
	}
	r.restore(t, plan, true)
	var stored string
	if err := r.pool.QueryRow(ctx, "SELECT stream_url FROM local_files WHERE path = 'A (2001).strm'").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if opened, err := key.Open(stored); !secrets.Sealed(stored) || err != nil || opened != address {
		t.Errorf("restored: %q, opened %q %v", stored, opened, err)
	}
}
