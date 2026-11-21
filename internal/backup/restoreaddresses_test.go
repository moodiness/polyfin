package backup

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testdb"
)

// A backup holds the addresses of addons, IPTV sources and guides sealed,
// as the database stores them. Those another key sealed are counted before
// the restore; once restored, those of the key read back as they were
// given, and are refused added again, while the others read as not set.
func TestARestoredBackupKeepsTheAddressesSealed(t *testing.T) {
	t.Parallel()
	testdb.LockRestores(t)
	r := newRestoring(t)
	ctx := t.Context()
	key, other := r.service.cfg.Secrets, newBox(t, []byte("fedcba9876543210fedcba9876543210"))
	client := stremio.NewClient("test")
	build := func(accounts.ID) stremio.Manifest {
		return stremio.Manifest{Name: "TV", Catalogs: []stremio.Catalog{{Type: "tv", ID: "channels", Name: "Channels"}}}
	}
	none := func(pgx.Tx, addons.Addon) error { return nil }
	const kept, lostAddress = "http://tv.example/get.php?username=u&password=kept-pass", "https://addon.example/lost-key/manifest.json"
	source, err := addons.New(r.pool, client, addons.Sealing(key)).Create(ctx, addons.Shared(), addons.KindM3U, kept, build, none)
	if err != nil {
		t.Fatal(err)
	}
	var lost accounts.ID
	if err := r.pool.QueryRow(ctx, `INSERT INTO addons (owner_id, kind, manifest_url, address_key, manifest, position)
		VALUES (NULL, 'stremio', $1, $2, '{"name": "Lost"}', 9) RETURNING id`, other.Seal(lostAddress), other.Fingerprint(lostAddress)).Scan(&lost); err != nil {
		t.Fatal(err)
	}
	if _, err := r.pool.Exec(ctx, "INSERT INTO live_guides (addon_id, catalog_type, catalog_id, position, url) VALUES ($1, 'tv', 'channels', 1, $2)",
		source.ID, other.Seal("http://tv.example/xmltv.php?password=lost-guide")); err != nil {
		t.Fatal(err)
	}
	name := r.backUp(t, time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local))
	if _, err := addons.New(r.pool, client, addons.Sealing(key)).Remove(ctx, addons.Shared(), source.ID); err != nil {
		t.Fatal(err)
	}

	plan, err := r.service.Prepare(ctx, nil, name)
	if err != nil {
		t.Fatal(err)
	}
	if plan.UnreadableCount() != 2 || plan.Unreadable[secrets.KindAddress] != 2 {
		t.Errorf("unreadable: %v", plan.Unreadable)
	}
	r.restore(t, plan, true)

	restored := addons.New(r.pool, client, addons.Sealing(key))
	if found, err := restored.Find(ctx, source.ID); err != nil || found.ManifestURL != kept {
		t.Errorf("restored source: %q, %v", found.ManifestURL, err)
	}
	var stored string
	if err := r.pool.QueryRow(ctx, "SELECT manifest_url FROM addons WHERE id = $1", source.ID).Scan(&stored); err != nil || !secrets.Sealed(stored) {
		t.Errorf("restored as stored: %q, %v", stored, err)
	}
	if _, err := restored.Create(ctx, addons.Shared(), addons.KindM3U, kept, build, none); !errors.Is(err, addons.ErrExists) {
		t.Errorf("added again after the restore: %v", err)
	}
	if found, err := restored.Find(ctx, lost); err != nil || found.ManifestURL != "" {
		t.Errorf("another key's address: %q, %v", found.ManifestURL, err)
	}
	if live, err := restored.LiveCatalogOf(ctx, addons.Shared(), addons.LibraryKey{AddonID: source.ID, CatalogType: "tv", CatalogID: "channels"}); err != nil ||
		len(live.Guides) != 1 || live.Guides[0].URL != "" {
		t.Errorf("another key's guide: %+v, %v", live, err)
	}
}
