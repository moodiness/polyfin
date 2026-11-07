package library

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testdb"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// writes is what the server counted of the rows of a table: inserted,
// updated and deleted, and those of its TOAST table, written or deleted.
type writes struct {
	inserted, updated, deleted int64
	toasted                    int64
}

// tableWrites returns the writes the server counted for each of tables.
// Every connection of pool reports its counts first: a backend otherwise
// reports them up to seconds later.
func tableWrites(t *testing.T, pool *pgxpool.Pool, tables ...string) map[string]writes {
	t.Helper()
	ctx := t.Context()
	for _, conn := range pool.AcquireAllIdle(ctx) {
		_, err := conn.Exec(ctx, "SELECT pg_stat_force_next_flush()")
		conn.Release()
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, _ := pool.Query(ctx, `SELECT c.relname, s.n_tup_ins, s.n_tup_upd, s.n_tup_del,
			coalesce(toast.n_tup_ins + toast.n_tup_upd + toast.n_tup_del, 0)
		FROM pg_class c JOIN pg_stat_all_tables s ON s.relid = c.oid
		LEFT JOIN pg_stat_all_tables toast ON toast.relid = c.reltoastrelid
		WHERE c.oid = ANY($1::regclass[])`, tables)
	result := map[string]writes{}
	var name string
	var w writes
	if _, err := pgx.ForEachRow(rows, []any{&name, &w.inserted, &w.updated, &w.deleted, &w.toasted}, func() error {
		result[name] = w
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(result) != len(tables) {
		t.Fatalf("counts of %v: %v", tables, result)
	}
	return result
}

// writesSince returns what was written to each table since before.
func writesSince(before, after map[string]writes) map[string]writes {
	result := map[string]writes{}
	for name, a := range after {
		b := before[name]
		result[name] = writes{a.inserted - b.inserted, a.updated - b.updated, a.deleted - b.deleted, a.toasted - b.toasted}
	}
	return result
}

// A catalog page the addon gives again unchanged writes nothing to the
// titles seen in its catalog, nor to how far the catalog was read: only
// the time it was fetched moves, while its stored titles, kept apart in
// the TOAST table for their size, stay as they are. A day later, its
// titles are noted as seen again, once, for the forgetting of the titles
// no read saw.
func TestAPageFetchedAgainWritesNothing(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	at := &clock{now: time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)}
	e.service.SetClock(at.Now)
	e.setting(func(s *accounts.Settings) { s.CatalogRefreshMinutes = 60 })
	addon := pagedAddon(40, 20)
	// Descriptions that do not compress, so that each page goes to the
	// TOAST table, as production's pages of about 27 kB do.
	for i := range addon.catalogs["movie/top"] {
		var description strings.Builder
		for k := range 8 {
			sum := sha256.Sum256(fmt.Appendf(nil, "%d %d", i, k))
			description.WriteString(hex.EncodeToString(sum[:]))
		}
		addon.catalogs["movie/top"][i].Description = description.String()
	}
	e.install(addons.Shared(), addon)
	top := e.library(e.member, "Top")
	refreshed := withRefreshing(t.Context())
	read := func() {
		t.Helper()
		if page, err := e.service.Children(refreshed, e.member, top.ID, 0, 40, ""); err != nil || len(page.Items) != 40 {
			t.Fatalf("listed %d titles, %v", len(page.Items), err)
		}
	}
	tables := []string{"catalog_pages", "catalog_reach", "catalog_arrivals"}
	read()
	var toasted bool
	if err := e.service.db.QueryRow(t.Context(), `SELECT bool_and(pg_column_toast_chunk_id(metas) IS NOT NULL)
		FROM catalog_pages`).Scan(&toasted); err != nil || !toasted {
		t.Fatalf("pages kept in the TOAST table: %v, %v", toasted, err)
	}

	at.advance(61 * time.Minute)
	before := tableWrites(t, e.service.db, tables...)
	asked := len(addon.catalogRequests())
	read()
	if again := len(addon.catalogRequests()) - asked; again != 2 {
		t.Fatalf("pages asked again: %d, want 2", again)
	}
	got := writesSince(before, tableWrites(t, e.service.db, tables...))
	if w := got["catalog_pages"]; w != (writes{updated: 2}) {
		t.Errorf("catalog_pages: %+v, want 2 rows updated and nothing toasted", w)
	}
	for _, table := range tables[1:] {
		if w := got[table]; w != (writes{}) {
			t.Errorf("%s: %+v, want nothing written", table, w)
		}
	}

	at.advance(seenAgain)
	read()
	var seen, reached int
	if err := e.service.db.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM catalog_arrivals WHERE last_seen = $1),
		(SELECT count(*) FROM catalog_reach WHERE read_at = $1 AND reach = 40)`, at.Now()).Scan(&seen, &reached); err != nil {
		t.Fatal(err)
	}
	if seen != 40 || reached != 1 {
		t.Errorf("a day later: %d titles seen again, %d catalogs read again; want 40 and 1", seen, reached)
	}
}

// countedAddon counts the movie catalog pages asked of an addon, by
// catalog, and those asked at once. Once hold is set, each page waits until
// hold of them are asked at once, or holdLimit passed, then all answer at
// once.
type countedAddon struct {
	inner http.Handler

	mu       sync.Mutex
	asked    map[string]int
	inFlight int
	peak     int
	hold     int
	reached  chan struct{}
}

func (a *countedAddon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	catalog, ok := strings.CutPrefix(r.URL.Path, "/catalog/movie/")
	if !ok {
		a.inner.ServeHTTP(w, r)
		return
	}
	catalog, _, _ = strings.Cut(catalog, "/")
	a.mu.Lock()
	a.asked[strings.TrimSuffix(catalog, ".json")]++
	a.inFlight++
	a.peak = max(a.peak, a.inFlight)
	reached := a.reached
	if reached != nil && a.inFlight >= a.hold {
		select {
		case <-reached:
		default:
			close(reached)
		}
	}
	a.mu.Unlock()
	if reached != nil {
		select {
		case <-reached:
		case <-time.After(holdLimit):
		case <-r.Context().Done():
		}
	}
	// Long enough for a page past the cap to be seen together with the
	// others.
	time.Sleep(10 * time.Millisecond)
	a.inner.ServeHTTP(w, r)
	a.mu.Lock()
	a.inFlight--
	a.mu.Unlock()
}

// take returns the pages asked by catalog since the last take, and the
// most asked at once.
func (a *countedAddon) take() (map[string]int, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	asked, peak := a.asked, a.peak
	a.asked, a.peak = map[string]int{}, 0
	return asked, peak
}

// Reading every collection, as the daily task does, asks each catalog of
// a collection its even share of the listing, reads two collections at
// once within the places of the addon's host, which it then fills, and
// writes little for each page.
func TestReadingTheCollectionsStaysWithinItsBudget(t *testing.T) {
	t.Parallel()
	const collections, perCatalog, pageSize = 6, 320, 20
	config, err := pgxpool.ParseConfig(testdb.CopyURL(t, migrated.New(t)))
	if err != nil {
		t.Fatal(err)
	}
	counted := &statements{}
	config.ConnConfig.Tracer = counted
	config.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	users, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	client := stremio.NewClient("test")
	store := addons.New(pool, client)
	service := New(pool, store, client, slog.New(slog.NewTextHandler(io.Discard, nil)), users.Settings)
	service.followUpDelays = nil
	service.readAhead = false
	e := env{t: t, service: service, addons: store, users: users}
	elapsed := new(atomic.Int64)
	service.SetClock(func() time.Time { return time.Now().Add(time.Duration(elapsed.Load())) })
	e.setting(func(s *accounts.Settings) { s.CatalogRefreshMinutes = 60 })

	// Production's shape: collections of two movie catalogs, read whole.
	fake := &fakeAddon{
		manifest: stremio.Manifest{ID: "sets", Name: "Sets", Version: "1", Types: []string{"movie", "collection"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs:  []stremio.Catalog{{Type: "collection", ID: "sets", Name: "Sets"}}},
		catalogs: map[string][]stremio.Meta{},
		metas:    map[string]stremio.Meta{},
		pageSize: pageSize,
	}
	sources := map[int][]string{}
	for c := range collections {
		var from []stremio.CollectionSource
		for _, side := range []string{"a", "b"} {
			id := fmt.Sprintf("c%d%s", c, side)
			fake.manifest.Catalogs = append(fake.manifest.Catalogs,
				stremio.Catalog{Type: "movie", ID: id, Name: id, Extra: []stremio.Extra{{Name: "skip"}}})
			movies := make([]stremio.Meta, perCatalog)
			for i := range movies {
				movies[i] = stremio.Meta{ID: fmt.Sprintf("tt%d%s%04d", c, side, i), Type: "movie", Name: fmt.Sprintf("%s %d", id, i)}
			}
			fake.catalogs["movie/"+id] = movies
			from = append(from, stremio.CollectionSource{Type: "movie", CatalogID: id})
			sources[c] = append(sources[c], id)
		}
		id := fmt.Sprintf("col:%d", c)
		meta := stremio.Meta{ID: id, Type: "collection", Name: fmt.Sprintf("Collection %d", c)}
		fake.catalogs["collection/sets"] = append(fake.catalogs["collection/sets"], meta)
		meta.Collection = &stremio.Collection{Sources: from}
		fake.metas["collection/"+id] = meta
	}
	addon := &countedAddon{inner: fake, asked: map[string]int{}}
	server := httptest.NewServer(addon)
	t.Cleanup(server.Close)
	if _, err := store.Install(t.Context(), addons.Shared(), server.URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	e.chooseLibraries(libraryOf("collection", "sets", "", 0))

	// The pages a whole listing needs, and one more for each catalog.
	budget := (WholeListing+pageSize-1)/pageSize + 2
	// check reads the collections; daily, as the task finds them each
	// day, it also counts the statements of each page.
	check := func(run string, daily bool) {
		t.Helper()
		if err := service.ReadCollections(t.Context()); err != nil {
			t.Fatal(err)
		}
		asked, peak := addon.take()
		counts, total := counted.take()
		pages := 0
		for c := range collections {
			got := asked[sources[c][0]] + asked[sources[c][1]]
			pages += got
			if got > budget {
				t.Errorf("%s: collection %d asked %d pages, want at most %d", run, c, got, budget)
			}
		}
		t.Logf("%s: %d pages, %d at once, %d statements: %v", run, pages, peak, total, counts)
		if peak > catalogHostFetches {
			t.Errorf("%s: %d pages asked at once, over the host's %d", run, peak, catalogHostFetches)
		}
		if daily && (pages == 0 || total > 8*pages) {
			t.Errorf("%s: %d statements for %d pages, want at most 8 a page", run, total, pages)
		}
		// A view reads the smart lists once as it is built: once for the
		// task, once for the library and once for each collection.
		if views := counts["SELECT smart_lists"]; views > collections+2 {
			t.Errorf("%s: %d views built for %d collections", run, views, collections)
		}
	}
	check("first read", false)
	// The next day's read finds every page past its refresh age: it asks
	// them all again, each catalog's 8 at once, and two collections fill
	// the host's places.
	elapsed.Add(int64(24 * time.Hour))
	addon.mu.Lock()
	addon.hold, addon.reached = catalogHostFetches, make(chan struct{})
	reached := addon.reached
	addon.mu.Unlock()
	check("next day", true)
	select {
	case <-reached:
	default:
		t.Errorf("never %d pages asked at once", catalogHostFetches)
	}
}

// A collection asks each of its catalogs an even share of what it lists;
// when titles both list leave it short, a second round asks for what is
// missing, and the listing is the same as if each had been read further.
func TestACollectionAsksItsCatalogsAnEvenShare(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	addon := pairAddon(60)
	movies := titles("movie", 70)
	// The second lists 50 of the first's titles, 10 places later.
	addon.catalogs["movie/first"], addon.catalogs["movie/second"] = movies[:60], movies[10:]
	addon.pageSize = 7
	e.install(addons.Shared(), addon)
	e.chooseLibraries(libraryOf("collection", "sets", "", 0))
	pair := e.children(e.member, "Sets", 0, 10).Items[0].ID
	asked := len(addon.catalogRequests())
	page, err := e.service.Children(t.Context(), e.member, pair, 0, 40, "")
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	seen := map[string]bool{}
	for rank := 0; len(want) < 40; rank++ {
		for _, list := range [][]stremio.Meta{movies[:60], movies[10:]} {
			if !seen[list[rank].ID] && len(want) < 40 {
				seen[list[rank].ID] = true
				want = append(want, list[rank].Name)
			}
		}
	}
	if got := names(page.Items); !slices.Equal(got, want) || !page.More {
		t.Errorf("listed %v, more %v; want %v", got, page.More, want)
	}
	// 21 titles each, 3 pages, leave 31: the second round asks 30 of
	// each, 2 more pages. A quarter more than an even share, 26 each,
	// left 36, and the second round asked twice that.
	if got := addon.catalogRequests()[asked:]; len(got) != 10 {
		t.Errorf("pages asked: %v, want 5 of each catalog", got)
	}
}
