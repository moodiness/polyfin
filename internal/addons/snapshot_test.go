package addons

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testdb"
)

// queryCounter counts the statements a pool sends.
type queryCounter struct{ queries atomic.Int64 }

func (c *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.queries.Add(1)
	return ctx
}

func (c *queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// countedStore returns a store whose statements are counted, and a pool
// on the same schema that writes behind its back.
func countedStore(t *testing.T) (*Store, *queryCounter, *pgxpool.Pool) {
	t.Helper()
	other := testdb.New(t)
	if err := database.Migrate(t.Context(), other); err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := other.QueryRow(t.Context(), "SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(testdb.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	counter := &queryCounter{}
	config.ConnConfig.Tracer = counter
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return New(pool, stremio.NewClient("test")), counter, other
}

// watch starts following the store's changes until the test ends.
func watch(t *testing.T, store *Store) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		store.Watch(ctx, slog.New(slog.DiscardHandler))
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	eventually(t, "the store listens", func() bool { return store.watching.Load() })
}

// eventually waits up to 5 seconds for done.
func eventually(t *testing.T, what string, done func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !done(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%s: timed out", what)
		}
	}
}

func TestWatchedStoreAnswersFromMemory(t *testing.T) {
	store, counter, _ := countedStore(t)
	addon := newFakeAddon(t, catalogs("movie", 3))
	installed, err := store.Install(t.Context(), Shared(), addon.url("a"), false)
	if err != nil {
		t.Fatal(err)
	}
	watch(t, store)
	first, err := store.Libraries(t.Context(), Shared())
	if err != nil || len(first) != 3 {
		t.Fatalf("libraries: %v %v", first, err)
	}
	counter.queries.Store(0)
	for range 100 {
		list, err := store.Addons(t.Context(), Shared())
		if err != nil || len(list) != 1 || list[0].Manifest.Catalogs[0].ID != "movie-0" {
			t.Fatalf("addons: %v %v", list, err)
		}
		if libraries, err := store.Libraries(t.Context(), Shared()); err != nil || !slices.EqualFunc(libraries, first, func(a, b Library) bool {
			return a.Catalog.ID == b.Catalog.ID && a.Enabled == b.Enabled
		}) {
			t.Fatalf("libraries: %v %v", libraries, err)
		}
		if found, err := store.Find(t.Context(), installed.ID); err != nil || found.ID != installed.ID {
			t.Fatalf("find: %v %v", found, err)
		}
		if mine, err := store.Addons(t.Context(), Personal(accounts.ID{1})); err != nil || mine == nil || len(mine) != 0 {
			t.Fatalf("addons of a scope without any: %v %v", mine, err)
		}
	}
	if n := counter.queries.Load(); n != 0 {
		t.Errorf("400 reads sent %d statements", n)
	}
	// What a caller does to the list it got leaves the next caller's alone.
	list, _ := store.Addons(t.Context(), Shared())
	list[0] = Addon{}
	if again, _ := store.Addons(t.Context(), Shared()); again[0].ID != installed.ID {
		t.Error("a caller's change reached the snapshot")
	}
}

// Servers, or tests, sharing a database keep their tables in schemas of
// their own, and the database notifies every change on one channel: a
// change in another schema never has the store read its own again.
func TestWatchedStoreIgnoresOtherSchemas(t *testing.T) {
	store, counter, _ := countedStore(t)
	addon := newFakeAddon(t, catalogs("movie", 3))
	if _, err := store.Install(t.Context(), Shared(), addon.url("a"), false); err != nil {
		t.Fatal(err)
	}
	watch(t, store)
	if _, err := store.Libraries(t.Context(), Shared()); err != nil {
		t.Fatal(err)
	}
	neighbour, _ := newStore(t)
	counter.queries.Store(0)
	if _, err := neighbour.Install(t.Context(), Shared(), addon.url("b"), false); err != nil {
		t.Fatal(err)
	}
	// Its notification comes within milliseconds; the store is read for
	// far longer.
	for deadline := time.Now().Add(300 * time.Millisecond); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if libraries, err := store.Libraries(t.Context(), Shared()); err != nil || len(libraries) != 3 {
			t.Fatalf("libraries: %v %v", libraries, err)
		}
	}
	if n := counter.queries.Load(); n != 0 {
		t.Errorf("a change in another schema sent %d statements", n)
	}
}

func TestEveryWriteOfTheStoreIsReadAtOnce(t *testing.T) {
	store, users := newStore(t)
	// No notification can help: only the store's own writes tell it.
	store.watching.Store(true)
	ctx := t.Context()
	read := func() ([]Addon, []Library) {
		t.Helper()
		list, err := store.Addons(ctx, Shared())
		if err != nil {
			t.Fatal(err)
		}
		libraries, err := store.Libraries(ctx, Shared())
		if err != nil {
			t.Fatal(err)
		}
		return list, libraries
	}
	addon := newFakeAddon(t, catalogs("movie", 2))
	read()
	installed, err := store.Install(ctx, Shared(), addon.url("a"), false)
	if err != nil {
		t.Fatal(err)
	}
	if list, libraries := read(); len(list) != 1 || len(libraries) != 2 || !libraries[0].Enabled {
		t.Fatalf("after Install: %v %v", list, libraries)
	}
	if _, err := store.SetEnabled(ctx, Shared(), installed.ID, false); err != nil {
		t.Fatal(err)
	}
	if list, libraries := read(); list[0].Enabled || libraries[0].AddonActive {
		t.Errorf("after SetEnabled: %v %v", list, libraries)
	}
	name := "Renamed"
	if _, err := store.SetLibraries(ctx, Shared(), []LibraryChoice{{AddonID: installed.ID, CatalogType: "movie", CatalogID: "movie-1", Name: &name}}); err != nil {
		t.Fatal(err)
	}
	if _, libraries := read(); libraries[0].Catalog.ID != "movie-1" || *libraries[0].Name != name || libraries[1].Enabled {
		t.Errorf("after SetLibraries: %v", libraries)
	}
	key := LibraryKey{AddonID: installed.ID, CatalogType: "movie", CatalogID: "movie-1"}
	if err := store.SetLibraryImage(ctx, Shared(), key, LibraryImageAutomatic); err != nil {
		t.Fatal(err)
	}
	if _, libraries := read(); libraries[0].Image != LibraryImageAutomatic {
		t.Errorf("after SetLibraryImage: %v", libraries)
	}
	// A manifest refreshed shows its new catalogs and loses the old.
	addon.catalogs.Store([]stremio.Catalog{{Type: "movie", ID: "movie-1"}, {Type: "series", ID: "new"}})
	if _, err := store.Refresh(ctx, Shared(), installed.ID, false); err != nil {
		t.Fatal(err)
	}
	if list, libraries := read(); len(list[0].Manifest.Catalogs) != 2 || len(libraries) != 2 || libraries[1].Catalog.ID != "new" {
		t.Errorf("after Refresh: %v %v", list, libraries)
	}
	if _, err := store.Replace(ctx, Shared(), installed.ID, addon.url("b"), false); err != nil {
		t.Fatal(err)
	}
	if found, err := store.Find(ctx, installed.ID); err != nil || found.ManifestURL != addon.url("b") {
		t.Errorf("after Replace: %v %v", found, err)
	}
	second, err := store.Install(ctx, Shared(), addon.url("c"), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Reorder(ctx, Shared(), []accounts.ID{second.ID, installed.ID}); err != nil {
		t.Fatal(err)
	}
	if list, _ := read(); list[0].ID != second.ID {
		t.Errorf("after Reorder: %v", list)
	}
	// An IPTV source, its guides, and its new address.
	channels := func(name string) func(accounts.ID) stremio.Manifest {
		return func(accounts.ID) stremio.Manifest {
			return stremio.Manifest{ID: "iptv", Name: name, Catalogs: []stremio.Catalog{{Type: "tv", ID: "channels", Name: name}}}
		}
	}
	source, err := store.Create(ctx, Shared(), KindM3U, "https://list.example/a.m3u", channels("A"), func(pgx.Tx, Addon) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	guided := func() Library {
		t.Helper()
		_, libraries := read()
		index := slices.IndexFunc(libraries, func(l Library) bool { return l.AddonID == source.ID })
		if index < 0 {
			t.Fatalf("no library of the source: %v", libraries)
		}
		return libraries[index]
	}
	tv := LibraryKey{AddonID: source.ID, CatalogType: "tv", CatalogID: "channels"}
	if _, err := store.SetGuides(ctx, Shared(), tv, []GuideAddress{{URL: "https://guide.example/a.xml"}, {URL: "https://guide.example/b.xml"}}); err != nil {
		t.Fatal(err)
	}
	if l := guided(); len(l.Guides) != 2 || l.Guides[1].URL != "https://guide.example/b.xml" {
		t.Errorf("after SetGuides: %v", l.Guides)
	}
	if _, err := store.SetGuide(ctx, Shared(), tv, ""); err != nil {
		t.Fatal(err)
	}
	if l := guided(); len(l.Guides) != 1 || l.Guides[0].URL != "https://guide.example/b.xml" {
		t.Errorf("after SetGuide: %v", l.Guides)
	}
	if _, err := store.Update(ctx, Shared(), source.ID, KindM3U, "https://list.example/b.m3u", channels("B")(source.ID),
		func(pgx.Tx) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if l := guided(); l.AddonName != "B" {
		t.Errorf("after Update: %v", l)
	}
	if err := store.Remove(ctx, Shared(), installed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Find(ctx, installed.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("after Remove: %v", err)
	}
	// A user's own addons are a scope of their own.
	alice, err := users.CreateUser(ctx, accounts.NewUser{Name: "alice", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(ctx, Personal(alice.ID), addon.url("d"), false); err != nil {
		t.Fatal(err)
	}
	if mine, _ := store.Addons(ctx, Personal(alice.ID)); len(mine) != 1 {
		t.Errorf("alice's addons: %v", mine)
	}
	if shared, _ := store.Addons(ctx, Shared()); len(shared) != 2 {
		t.Errorf("the server's addons: %v", shared)
	}
}

func TestChangesMadeElsewhereReachTheSnapshot(t *testing.T) {
	store, _, other := countedStore(t)
	ctx := t.Context()
	source, err := store.Create(ctx, Shared(), KindM3U, "https://list.example/a.m3u", func(accounts.ID) stremio.Manifest {
		return stremio.Manifest{ID: "iptv", Name: "A", Catalogs: []stremio.Catalog{{Type: "tv", ID: "channels"}}}
	}, func(tx pgx.Tx, addon Addon) error {
		_, err := tx.Exec(ctx, `INSERT INTO live_guides (addon_id, catalog_type, catalog_id, position, url) VALUES ($1, 'tv', 'channels', 1, 'https://guide.example/a.xml')`,
			addon.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	watch(t, store)
	library := func() Library {
		libraries, err := store.Libraries(ctx, Shared())
		if err != nil || len(libraries) == 0 {
			t.Fatalf("libraries: %v %v", libraries, err)
		}
		return libraries[0]
	}
	if l := library(); len(l.Guides) != 1 || l.Guides[0].Error != "" {
		t.Fatalf("guides: %v", l.Guides)
	}
	// A guide download, as Live TV records it.
	if _, err := other.Exec(ctx, "UPDATE live_guides SET error = 'unreachable', channels = 7"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "a guide's status", func() bool { g := library().Guides; return len(g) == 1 && g[0].Error == "unreachable" })
	// Channels mapped to the guide, as the mapping counts them.
	if _, err := other.Exec(ctx, "UPDATE libraries SET guide_channels = 12, guide_matched = 5"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the mapped channels", func() bool { l := library(); return l.GuideChannels == 12 && l.GuideMapped == 5 })
	// A source's catalogs following its lists, in a transaction of its own.
	err = pgx.BeginFunc(ctx, other, func(tx pgx.Tx) error {
		return SyncCatalogs(ctx, tx, source.ID, stremio.Manifest{ID: "iptv", Name: "A", Catalogs: []stremio.Catalog{
			{Type: "tv", ID: "channels"}, {Type: "movie", ID: "films"}}})
	})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "a catalog synced", func() bool {
		libraries, _ := store.Libraries(ctx, Shared())
		return slices.ContainsFunc(libraries, func(l Library) bool { return l.Catalog.ID == "films" && l.Enabled })
	})
	if _, err := other.Exec(ctx, "DELETE FROM addons"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "an addon removed", func() bool { _, err := store.Find(ctx, source.ID); return errors.Is(err, ErrNotFound) })
}

func TestStoreFollowsChangesAgainAfterLosingTheDatabase(t *testing.T) {
	store, _, other := countedStore(t)
	ctx := t.Context()
	addon := newFakeAddon(t, catalogs("movie", 1))
	installed, err := store.Install(ctx, Shared(), addon.url("a"), false)
	if err != nil {
		t.Fatal(err)
	}
	watch(t, store)
	if _, err := store.Addons(ctx, Shared()); err != nil {
		t.Fatal(err)
	}
	// The listening connection goes away: until it is back, reads ask the
	// database, so a change made meanwhile is seen at once.
	if _, err := other.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity
		WHERE datname = current_database() AND query = 'LISTEN `+changesChannel+`' AND pid <> pg_backend_pid()`); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the store notices", func() bool { return !store.watching.Load() })
	if _, err := other.Exec(ctx, "UPDATE addons SET enabled = false"); err != nil {
		t.Fatal(err)
	}
	if list, err := store.Addons(ctx, Shared()); err != nil || list[0].Enabled {
		t.Errorf("addons while not listening: %v %v", list, err)
	}
	eventually(t, "the store listens again", func() bool { return store.watching.Load() })
	if _, err := other.Exec(ctx, "UPDATE addons SET enabled = true WHERE id = $1", installed.ID); err != nil {
		t.Fatal(err)
	}
	eventually(t, "a change after listening again", func() bool { list, _ := store.Addons(ctx, Shared()); return list[0].Enabled })
}

func TestUnwatchedStoreReadsTheDatabase(t *testing.T) {
	store, _, other := countedStore(t)
	ctx := t.Context()
	addon := newFakeAddon(t, catalogs("movie", 1))
	if _, err := store.Install(ctx, Shared(), addon.url("a"), false); err != nil {
		t.Fatal(err)
	}
	if list, _ := store.Addons(ctx, Shared()); !list[0].Enabled {
		t.Fatal("addon off")
	}
	if _, err := other.Exec(ctx, "UPDATE addons SET enabled = false"); err != nil {
		t.Fatal(err)
	}
	if list, _ := store.Addons(ctx, Shared()); list[0].Enabled {
		t.Error("a change made elsewhere was not read")
	}
}
