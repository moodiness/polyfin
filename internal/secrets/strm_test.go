package secrets_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// The addresses .strm files name, stored as plaintext by a server that ran
// without a key, are sealed at the start with the key and counted among
// the secrets sealed, once; another key names a folder whose addresses it
// cannot open once, by the folder's name, as a backup counts each of them.
func TestStrmAddressesAreSealedAtStartAndNamedWhenUnreadable(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	pool := migrated.New(t)
	if _, err := pool.Exec(ctx, `WITH folder AS (INSERT INTO addons (owner_id, kind, manifest_url, manifest, position)
			VALUES (NULL, 'local', '/media/movies', '{"name": "Shelf"}', 1) RETURNING id),
		kind AS (INSERT INTO local_folders (addon_id, kind) SELECT id, 'movies' FROM folder RETURNING addon_id)
		INSERT INTO local_files (addon_id, path, size, modified, unit, stream_url)
		SELECT addon_id, f.path, 40, now(), f.path, f.address FROM kind,
			(VALUES ('A (2001).strm', 'https://cdn.example/a.mp4?token=strm-plain-a'),
				('B (2002).strm', 'https://cdn.example/b.mp4?token=strm-plain-b'), ('C (2003).mkv', NULL)) f (path, address)`); err != nil {
		t.Fatal(err)
	}
	var none *secrets.Box
	if report, err := none.Inspect(ctx, pool); err != nil || report.Plaintext != 2 || len(report.Unreadable) != 0 {
		t.Errorf("without a key: %+v %v", report, err)
	}
	key := newBox(t, "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	if sealed, err := key.SealStored(ctx, pool); err != nil || sealed != 2 {
		t.Fatalf("sealed %d: %v", sealed, err)
	}
	if sealed, err := key.SealStored(ctx, pool); err != nil || sealed != 0 {
		t.Errorf("sealed again %d: %v", sealed, err)
	}
	rows, err := pool.Query(ctx, "SELECT coalesce(stream_url, '') FROM local_files ORDER BY path")
	if err != nil {
		t.Fatal(err)
	}
	var stored []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		stored = append(stored, value)
	}
	if err := rows.Err(); err != nil || len(stored) != 3 || stored[2] != "" {
		t.Fatalf("stored: %q %v", stored, err)
	}
	for i, want := range []string{"https://cdn.example/a.mp4?token=strm-plain-a", "https://cdn.example/b.mp4?token=strm-plain-b"} {
		if opened, err := key.Open(stored[i]); !secrets.Sealed(stored[i]) || strings.Contains(stored[i], "strm-plain") || err != nil ||
			opened != want {
			t.Errorf("address %d: %q, opened %q %v", i, stored[i], opened, err)
		}
	}
	if report, err := key.Inspect(ctx, pool); err != nil || report.Plaintext != 0 || len(report.Unreadable) != 0 {
		t.Errorf("with the key: %+v %v", report, err)
	}
	other := newBox(t, strings.Repeat("ab", 32))
	if report, err := other.Inspect(ctx, pool); err != nil || !slices.Equal(report.Unreadable, []secrets.Unreadable{{Addon: "Shelf"}}) {
		t.Errorf("with another key: %+v %v", report, err)
	}
	if kind, count := other.UnreadableIn("local_files", map[string]string{"stream_url": stored[0]}); kind != secrets.KindAddress || count != 1 {
		t.Errorf("a backup's row: %s %d", kind, count)
	}
	if kind, count := key.UnreadableIn("local_files", map[string]string{"stream_url": stored[0]}); kind != secrets.KindAddress || count != 0 {
		t.Errorf("a backup's row with the key: %s %d", kind, count)
	}
	if !slices.Contains(secrets.SealedTables(), "local_files") {
		t.Errorf("sealed tables: %v", secrets.SealedTables())
	}
}
