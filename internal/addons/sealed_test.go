package addons

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// sealingBox is a box with key, 32 bytes.
func sealingBox(t *testing.T, key string) *secrets.Box {
	t.Helper()
	box, err := secrets.New([]byte(key))
	if err != nil {
		t.Fatal(err)
	}
	return box
}

// storedAddress reads an addon's address and fingerprint as the database
// holds them.
func storedAddress(t *testing.T, pool *pgxpool.Pool, id accounts.ID) (address string, fingerprint *string) {
	t.Helper()
	if err := pool.QueryRow(t.Context(), "SELECT manifest_url, address_key FROM addons WHERE id = $1", id).Scan(&address, &fingerprint); err != nil {
		t.Fatal(err)
	}
	return address, fingerprint
}

// storedGuide reads the address of a catalog's first guide as the
// database holds it.
func storedGuide(t *testing.T, pool *pgxpool.Pool, id accounts.ID) string {
	t.Helper()
	var address string
	if err := pool.QueryRow(t.Context(), "SELECT url FROM live_guides WHERE addon_id = $1 ORDER BY position LIMIT 1", id).Scan(&address); err != nil {
		t.Fatal(err)
	}
	return address
}

// createSource adds an M3U source at address, with its one live TV
// catalog, as package iptv does.
func createSource(t *testing.T, store *Store, scope Scope, address string) (Addon, error) {
	t.Helper()
	return store.Create(t.Context(), scope, KindM3U, address, func(id accounts.ID) stremio.Manifest {
		return stremio.Manifest{ID: "iptv." + id.String(), Name: "TV", Version: "1.0.0", Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{{Type: "tv", ID: "channels", Name: "Channels"}}}
	}, func(pgx.Tx, Addon) error { return nil })
}

// With POLYFIN_SECRET_KEY, the addresses of addons, of IPTV sources and of
// guides are stored sealed, each under a nonce of its own, and read back
// as they were given, by a store started again with the key; an address
// installed twice in a scope is refused all the same. Without the key, or
// for a local folder or a tuner, they are stored as they are.
func TestAddressesAreStoredSealedWithAKeyAndAsTheyAreWithout(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	pool := migrated.New(t)
	users, err := accounts.Open(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	alice, err := users.CreateUser(ctx, accounts.NewUser{Name: "alice", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	key := "0123456789abcdef0123456789abcdef"
	client := stremio.NewClient("test")
	plain, sealed := New(pool, client), New(pool, client, Sealing(sealingBox(t, key)))
	served := newFakeAddon(t, catalogs("movie", 2))

	// Without a key, an address is stored as it is.
	unsealed, err := plain.Install(ctx, Shared(), served.url("plain-debrid-key"), false)
	if err != nil {
		t.Fatal(err)
	}
	if address, fingerprint := storedAddress(t, pool, unsealed.ID); address != served.url("plain-debrid-key") || fingerprint != nil {
		t.Errorf("without a key: %q, %v", address, fingerprint)
	}

	// With the key, sealed, and read back as it was given.
	secret := served.url("debrid-key-123")
	addon, err := sealed.Install(ctx, Shared(), secret, false)
	if err != nil {
		t.Fatal(err)
	}
	address, fingerprint := storedAddress(t, pool, addon.ID)
	if !secrets.Sealed(address) || strings.Contains(address, "debrid-key-123") || fingerprint == nil || strings.Contains(*fingerprint, "debrid") {
		t.Errorf("with the key: %q, %v", address, fingerprint)
	}
	if addon.ManifestURL != secret {
		t.Errorf("installed: %q", addon.ManifestURL)
	}
	// A refresh keeps the stored value.
	if _, _, err := sealed.Refresh(ctx, Shared(), addon.ID, false); err != nil {
		t.Fatal(err)
	}
	if again, _ := storedAddress(t, pool, addon.ID); again != address {
		t.Errorf("a refresh sealed the address again: %q", again)
	}

	// Installed again in the scope, it is refused; in another scope, it
	// is installed, sealed under another nonce.
	if _, err := sealed.Install(ctx, Shared(), secret, false); !errors.Is(err, ErrExists) {
		t.Errorf("installed twice: %v", err)
	}
	own, err := sealed.Install(ctx, Personal(alice.ID), secret, false)
	if err != nil {
		t.Fatal(err)
	}
	if other, _ := storedAddress(t, pool, own.ID); other == address || !secrets.Sealed(other) {
		t.Errorf("the same address in another scope: %q", other)
	}

	// An IPTV source and its guide, sealed; a local folder and a tuner,
	// whose addresses hold no credentials, as they are.
	source, err := createSource(t, sealed, Shared(), "http://tv.example/get.php?username=u&password=iptv-pass")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := createSource(t, sealed, Shared(), "http://tv.example/get.php?username=u&password=iptv-pass"); !errors.Is(err, ErrExists) {
		t.Errorf("a source added twice: %v", err)
	}
	key0 := LibraryKey{AddonID: source.ID, CatalogType: "tv", CatalogID: "channels"}
	if _, err := sealed.SetGuide(ctx, Shared(), key0, "http://tv.example/xmltv.php?password=guide-pass"); err != nil {
		t.Fatal(err)
	}
	if stored, _ := storedAddress(t, pool, source.ID); !secrets.Sealed(stored) || strings.Contains(stored, "iptv-pass") {
		t.Errorf("source: %q", stored)
	}
	if guide := storedGuide(t, pool, source.ID); !secrets.Sealed(guide) || strings.Contains(guide, "guide-pass") {
		t.Errorf("guide: %q", guide)
	}
	for kind, path := range map[string]string{KindLocal: "/media/movies", KindHDHomeRun: "http://192.168.1.20"} {
		created, err := sealed.Create(ctx, Shared(), kind, path, func(accounts.ID) stremio.Manifest { return stremio.Manifest{Name: kind} },
			func(pgx.Tx, Addon) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		if stored, fingerprint := storedAddress(t, pool, created.ID); stored != path || fingerprint != nil {
			t.Errorf("%s: %q, %v", kind, stored, fingerprint)
		}
	}

	// Started again with the key, the store reads every address back.
	reopened := New(pool, client, Sealing(sealingBox(t, key)))
	for id, want := range map[accounts.ID]string{unsealed.ID: served.url("plain-debrid-key"), addon.ID: secret, own.ID: secret,
		source.ID: "http://tv.example/get.php?username=u&password=iptv-pass"} {
		if found, err := reopened.Find(ctx, id); err != nil || found.ManifestURL != want {
			t.Errorf("reopened: %q, %v, want %q", found.ManifestURL, err, want)
		}
	}
	live, err := reopened.LiveCatalogOf(ctx, Shared(), key0)
	if err != nil || len(live.Guides) != 1 || live.Guides[0].URL != "http://tv.example/xmltv.php?password=guide-pass" ||
		live.Addon.ManifestURL != source.ManifestURL {
		t.Errorf("live catalog: %+v, %v", live, err)
	}

	// Another key reads the sealed addresses as not set.
	other := New(pool, client, Sealing(sealingBox(t, "fedcba9876543210fedcba9876543210")))
	if found, err := other.Find(ctx, addon.ID); err != nil || found.ManifestURL != "" {
		t.Errorf("another key: %q, %v", found.ManifestURL, err)
	}
	if live, err := other.LiveCatalogOf(ctx, Shared(), key0); err != nil || len(live.Guides) != 1 || live.Guides[0].URL != "" || live.Addon.ManifestURL != "" {
		t.Errorf("another key, live catalog: %+v, %v", live, err)
	}
}

// At start, the addresses stored as they are, by a server that ran
// without a key, are sealed once, with their fingerprints, which refuse
// them installed again. A key that cannot open them names them, by their
// addon's name and whose it is, and reads them as not set.
func TestStoredAddressesAreSealedAtStartAndNamedWhenUnreadable(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	pool := migrated.New(t)
	users, err := accounts.Open(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	alice, err := users.CreateUser(ctx, accounts.NewUser{Name: "alice", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	client := stremio.NewClient("test")
	plain := New(pool, client)
	served := newFakeAddon(t, catalogs("movie", 1))
	addon, err := plain.Install(ctx, Shared(), served.url("debrid-key-1"), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plain.SetName(ctx, Shared(), addon.ID, "Debrid"); err != nil {
		t.Fatal(err)
	}
	source, err := createSource(t, plain, Personal(alice.ID), "http://tv.example/get.php?username=u&password=iptv-pass")
	if err != nil {
		t.Fatal(err)
	}
	key0 := LibraryKey{AddonID: source.ID, CatalogType: "tv", CatalogID: "channels"}
	if _, err := plain.SetGuides(ctx, Personal(alice.ID), key0, []GuideAddress{{URL: "http://tv.example/a.xml?password=a"},
		{URL: "http://tv.example/b.xml?password=b"}}); err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	box := sealingBox(t, "0123456789abcdef0123456789abcdef")
	if report, err := box.Inspect(ctx, pool); err != nil || report.Plaintext != 4 {
		t.Errorf("before: %+v, %v", report, err)
	}
	if sealedCount, err := box.SealStored(ctx, pool); err != nil || sealedCount != 4 {
		t.Fatalf("sealed %d: %v", sealedCount, err)
	}
	first, fingerprint := storedAddress(t, pool, addon.ID)
	if !secrets.Sealed(first) || fingerprint == nil || *fingerprint != box.Fingerprint(served.url("debrid-key-1")) {
		t.Errorf("sealed: %q, %v", first, fingerprint)
	}
	if again, err := box.SealStored(ctx, pool); err != nil || again != 0 {
		t.Errorf("sealed again: %d, %v", again, err)
	}
	if report, err := box.Inspect(ctx, pool); err != nil || report.Plaintext != 0 || len(report.Unreadable) != 0 {
		t.Errorf("after: %+v, %v", report, err)
	}

	sealed := New(pool, client, Sealing(box))
	if found, err := sealed.Find(ctx, addon.ID); err != nil || found.ManifestURL != served.url("debrid-key-1") {
		t.Errorf("read back: %q, %v", found.ManifestURL, err)
	}
	if _, err := sealed.Install(ctx, Shared(), served.url("debrid-key-1"), false); !errors.Is(err, ErrExists) {
		t.Errorf("installed again once sealed: %v", err)
	}

	// Installed again while the key was not set, then sealed with it: both
	// stay, the second without a fingerprint.
	again, err := plain.Install(ctx, Shared(), served.url("debrid-key-1"), false)
	if err != nil {
		t.Fatal(err)
	}
	if sealedCount, err := box.SealStored(ctx, pool); err != nil || sealedCount != 1 {
		t.Fatalf("sealed %d: %v", sealedCount, err)
	}
	if stored, fingerprint := storedAddress(t, pool, again.ID); !secrets.Sealed(stored) || fingerprint != nil {
		t.Errorf("the address installed again: %q, %v", stored, fingerprint)
	}

	// Another key: each address is named once, and read as not set.
	other := sealingBox(t, "fedcba9876543210fedcba9876543210")
	if err := other.Prepare(ctx, pool, logger); err != nil {
		t.Fatal(err)
	}
	report, err := other.Inspect(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	want := []secrets.Unreadable{{Addon: "Debrid"}, {Addon: "Test"}, {Addon: "TV", User: "alice"}, {Addon: "TV", User: "alice", Guide: true}}
	if len(report.Unreadable) != len(want) {
		t.Fatalf("unreadable: %+v", report.Unreadable)
	}
	for i := range want {
		if report.Unreadable[i] != want[i] {
			t.Errorf("unreadable %d: %+v, want %+v", i, report.Unreadable[i], want[i])
		}
	}
	for _, named := range []string{"address of Debrid", "address of TV of alice", "guide address of TV of alice"} {
		if !strings.Contains(logs.String(), named) {
			t.Errorf("%q not named: %s", named, logs.String())
		}
	}
	for _, value := range []string{"debrid-key-1", "iptv-pass", "password=a"} {
		if strings.Contains(logs.String(), value) {
			t.Errorf("the log holds %q", value)
		}
	}
	if found, err := New(pool, client, Sealing(other)).Find(ctx, source.ID); err != nil || found.ManifestURL != "" {
		t.Errorf("another key: %q, %v", found.ManifestURL, err)
	}
}
