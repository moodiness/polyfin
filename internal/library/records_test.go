package library

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Listings that save the same titles at once in other orders, as smart
// lists sorted differently do when an app opens its home, all succeed:
// none is cut short by a deadlock in the database.
func TestListingsSavingTheSameTitlesAtOnceSucceed(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	titles := make([]record, 200)
	for i := range titles {
		key := fmt.Sprintf("movie|tt%07d", i)
		titles[i] = record{ID: itemID(key), Key: key, Kind: KindMovie}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for list := range 2 {
		folder := accounts.ID{byte(list + 1)}
		listed := slices.Clone(titles)
		if list == 1 {
			slices.Reverse(listed)
		}
		for i := range listed {
			listed[i].Parent = &folder
		}
		wg.Go(func() {
			for range 10 {
				if err := e.service.save(t.Context(), listed); err != nil {
					errs <- err
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// Requests recording the same new titles and people at once, as apps
// opening a person's page do, all succeed: none fails on the key of an
// item another one inserts, and every credit is kept.
func TestRecordingTheSameNewItemsAtOnceSucceeds(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	const writers = 4
	for round := range 20 {
		titles := make([]record, 20)
		for i := range titles {
			key := fmt.Sprintf("movie|tt9%03d%03d", round, i)
			titles[i] = record{ID: itemID(key), Key: key, Kind: KindMovie}
		}
		person := fmt.Sprintf("Ida Round%d", round)
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make(chan error, 2*writers)
		for writer := range writers {
			wg.Go(func() {
				<-start
				errs <- e.service.save(t.Context(), titles)
				errs <- e.service.saveCredits(t.Context(), titles[writer].ID, []Person{{ID: PersonID(person), Name: person, Type: "Actor"}}, false)
			})
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("round %d: %v", round, err)
			}
		}
		r, err := e.service.load(t.Context(), PersonID(person))
		if err != nil || len(r.Credits) != writers {
			t.Fatalf("round %d, the person's credits: %v %v", round, r.Credits, err)
		}
	}
}

// A person's record takes its identifier from the name it is recorded
// under: a credit whose identifier is another name's adds to the record of
// its name, and a record whose key is another item's is left out without
// failing the others.
func TestRecordsKeepTheirKeysIdentifier(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	first, second := itemID(titleKey(KindMovie, "tt1")), itemID(titleKey(KindMovie, "tt2"))
	if err := e.service.saveCredits(t.Context(), first, []Person{{ID: PersonID("Ann Lee"), Name: "Ann Lee", Type: "Actor"}}, false); err != nil {
		t.Fatal(err)
	}
	if err := e.service.saveCredits(t.Context(), second, []Person{{ID: PersonID("Other Name"), Name: "ann lee", Type: "Director"}}, false); err != nil {
		t.Fatal(err)
	}
	ann, err := e.service.load(t.Context(), PersonID("Ann Lee"))
	if err != nil || !slices.Equal(ann.Credits[first.String()], []string{"Actor"}) || !slices.Equal(ann.Credits[second.String()], []string{"Director"}) {
		t.Errorf("Ann Lee's credits: %v %v", ann.Credits, err)
	}
	if _, err := e.service.load(t.Context(), PersonID("Other Name")); !errors.Is(err, ErrNotFound) {
		t.Errorf("the other name: %v", err)
	}

	// A record stored under another identifier than its key's, which no
	// version of Polyfin wrote.
	stray := accounts.ID{9}
	if _, err := e.service.db.Exec(t.Context(), `INSERT INTO items (id, key, kind, data) VALUES ($1, $2, 'movie', '{"kind": "movie"}')`,
		stray, titleKey(KindMovie, "tt3")); err != nil {
		t.Fatal(err)
	}
	records := []record{{ID: itemID(titleKey(KindMovie, "tt3")), Key: titleKey(KindMovie, "tt3"), Kind: KindMovie},
		{ID: itemID(titleKey(KindMovie, "tt4")), Key: titleKey(KindMovie, "tt4"), Kind: KindMovie}}
	if err := e.service.save(t.Context(), records); err != nil {
		t.Fatal(err)
	}
	if err := e.service.saveNew(t.Context(), records); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.load(t.Context(), records[1].ID); err != nil {
		t.Errorf("the other record: %v", err)
	}
	if _, err := e.service.load(t.Context(), stray); err != nil {
		t.Errorf("the stored record: %v", err)
	}
}

// A save merges the items it finds stored with what it records, whatever
// another writer does to them meanwhile: it never inserts one, which the
// key of another item would fail. Here the merge waits for an item another
// writer holds, which deletes the next one and gives its key to a record of
// another identifier: the save succeeds, merges the first, and leaves the
// second out, telling it.
func TestMergingItemsChangedMeanwhileSucceeds(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	log := &lockedBuffer{}
	e.service.logger = slog.New(slog.NewTextHandler(log, nil))
	records := []record{{Key: titleKey(KindMovie, "tt1"), Kind: KindMovie}, {Key: titleKey(KindMovie, "tt2"), Kind: KindMovie}}
	for i := range records {
		records[i].ID = itemID(records[i].Key)
	}
	slices.SortFunc(records, func(a, b record) int { return bytes.Compare(a.ID[:], b.ID[:]) })
	first, second := records[0], records[1]
	if err := e.service.save(ctx, records); err != nil {
		t.Fatal(err)
	}

	holder, err := e.service.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback(context.WithoutCancel(ctx)) }()
	var holderPID int
	if err := holder.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&holderPID); err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(ctx, "SELECT 1 FROM items WHERE id = $1 FOR UPDATE", first.ID); err != nil {
		t.Fatal(err)
	}
	folder := accounts.ID{7}
	listed := slices.Clone(records)
	for i := range listed {
		listed[i].Parent = &folder
	}
	saved := make(chan error, 1)
	go func() { saved <- e.service.save(ctx, listed) }()
	eventually(t, "the save to wait for the first item", func() bool {
		var waiting int
		_ = e.service.db.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid))", holderPID).Scan(&waiting)
		return waiting > 0
	})
	stray := accounts.ID{9}
	if _, err := holder.Exec(ctx, "DELETE FROM items WHERE id = $1", second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(ctx, `INSERT INTO items (id, key, kind, data) VALUES ($1, $2, 'movie', '{"kind": "movie"}')`, stray, second.Key); err != nil {
		t.Fatal(err)
	}
	if err := holder.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-saved; err != nil {
		t.Fatalf("the save: %v", err)
	}
	if r, err := e.service.load(ctx, first.ID); err != nil || r.Parent == nil || *r.Parent != folder {
		t.Errorf("the first item: %+v %v", r, err)
	}
	if _, err := e.service.load(ctx, second.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the second item: %v", err)
	}
	if r, err := e.service.load(ctx, stray); err != nil || r.Key != second.Key {
		t.Errorf("the record holding its key: %+v %v", r, err)
	}
	if !strings.Contains(log.String(), "Items were not recorded") || !strings.Contains(log.String(), second.Key) {
		t.Errorf("log:\n%s", log)
	}
}

// listingAddon serves the movie catalogs Top, of movies 0 to 4, and New,
// of movies 3 to 7, and the series catalog Shows, of one series of two
// seasons.
func listingAddon() *fakeAddon {
	movies := func(from, to int) []stremio.Meta {
		var result []stremio.Meta
		for i := from; i <= to; i++ {
			result = append(result, stremio.Meta{ID: fmt.Sprintf("tt%d", i), Type: "movie", Name: fmt.Sprintf("Movie %d", i)})
		}
		return result
	}
	show := stremio.Meta{ID: "tt100", Type: "series", Name: "Show"}
	for season := 1; season <= 2; season++ {
		for episode := 1; episode <= 3; episode++ {
			show.Videos = append(show.Videos, stremio.Video{ID: fmt.Sprintf("tt100:%d:%d", season, episode), Title: fmt.Sprintf("Episode %d", episode),
				Season: stremio.Number(season), Episode: stremio.Number(episode), Released: fmt.Sprintf("2020-0%d-0%dT00:00:00Z", season, episode)})
		}
	}
	preview := show
	preview.Videos = nil
	return &fakeAddon{
		manifest: stremio.Manifest{ID: "listings", Name: "Listings", Version: "1", Types: []string{"movie", "series"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs: []stremio.Catalog{
				{Type: "movie", ID: "top", Name: "Top"},
				{Type: "movie", ID: "new", Name: "New"},
				{Type: "series", ID: "shows", Name: "Shows"},
			}},
		catalogs: map[string][]stremio.Meta{"movie/top": movies(0, 4), "movie/new": movies(3, 7), "series/shows": {preview}},
		metas:    map[string]stremio.Meta{"series/tt100": show},
	}
}

// listed lists the whole of a library of the user, and returns the items
// listed.
func (e env) listed(user accounts.User, library string) []accounts.ID {
	e.t.Helper()
	var ids []accounts.ID
	for _, item := range e.children(user, library, 0, 100).Items {
		ids = append(ids, item.ID)
	}
	return ids
}

// itemVersions returns the xmin and xmax of the items ids: an update
// changes the first, a row lock the second.
func (e env) itemVersions(ids []accounts.ID) map[accounts.ID]string {
	e.t.Helper()
	rows, err := e.service.db.Query(e.t.Context(), "SELECT id, xmin::text || '/' || xmax::text FROM items WHERE id = ANY($1::uuid[])", ids)
	if err != nil {
		e.t.Fatal(err)
	}
	result := map[accounts.ID]string{}
	var id accounts.ID
	var versions string
	if _, err := pgx.ForEachRow(rows, []any{&id, &versions}, func() error {
		result[id] = versions
		return nil
	}); err != nil {
		e.t.Fatal(err)
	}
	if len(result) != len(ids) {
		e.t.Fatalf("%d items stored of %d", len(result), len(ids))
	}
	return result
}

// itemUpdates returns the count of rows of items updated so far, once
// every connection of the pool reported its statistics: each reports
// them as it goes idle, forced, before it answers.
func (e env) itemUpdates() int64 {
	e.t.Helper()
	ctx := e.t.Context()
	for _, conn := range e.service.db.AcquireAllIdle(ctx) {
		_, err := conn.Exec(ctx, "SELECT pg_stat_force_next_flush()")
		conn.Release()
		if err != nil {
			e.t.Fatal(err)
		}
	}
	var updates int64
	if err := e.service.db.QueryRow(ctx, "SELECT n_tup_upd FROM pg_stat_user_tables WHERE relid = 'items'::regclass").Scan(&updates); err != nil {
		e.t.Fatal(err)
	}
	return updates
}

// Listing the same pages again, the libraries, a library's titles, and a
// series' seasons and episodes, as Next Up does, writes nothing, and
// locks no row: what was saved is known to be stored as it is.
func TestListingAgainWritesNothing(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.install(addons.Shared(), listingAddon())
	show := itemID(titleKey(KindSeries, "tt100"))
	list := func() []accounts.ID {
		ids := append(e.listed(e.member, "Top"), e.listed(e.member, "Shows")...)
		seasons, err := e.service.Seasons(t.Context(), e.member, show)
		if err != nil {
			t.Fatal(err)
		}
		episodes, err := e.service.Episodes(t.Context(), e.member, show, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range slices.Concat(seasons, episodes) {
			ids = append(ids, item.ID)
		}
		return append(ids, e.library(e.member, "Top").ID)
	}
	// The series' description, read once it was listed, stored its rating:
	// the second listing finds the series changed since, and saves it once
	// more.
	ids := list()
	if len(ids) != 15 {
		t.Fatalf("%d items listed", len(ids))
	}
	list()
	versions, updates := e.itemVersions(ids), e.itemUpdates()
	list()
	if again := e.itemVersions(ids); !maps.Equal(again, versions) {
		t.Errorf("rows written or locked: %v, then %v", versions, again)
	}
	if n := e.itemUpdates() - updates; n != 0 {
		t.Errorf("%d rows updated", n)
	}
}

// A save leaves out only what is stored as it saved it: a row another
// writer changed or deleted behind its back, without telling, is saved
// again by the next listing.
func TestListingsSaveRowsChangedBehindThem(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.install(addons.Shared(), listingAddon())
	ids := e.listed(e.member, "Top")
	e.listed(e.member, "Top")
	changed, deleted := ids[0], ids[1]
	if _, err := e.service.db.Exec(t.Context(), `UPDATE items SET data = jsonb_set(data, '{meta,name}', '"Changed"') WHERE id = $1`, changed); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.db.Exec(t.Context(), "DELETE FROM items WHERE id = $1", deleted); err != nil {
		t.Fatal(err)
	}
	e.listed(e.member, "Top")
	if r, err := e.service.load(t.Context(), changed); err != nil || r.Meta.Name != "Movie 0" {
		t.Errorf("the row changed: %+v %v", r.Meta, err)
	}
	if r, err := e.service.load(t.Context(), deleted); err != nil || r.Meta.Name != "Movie 1" || r.Parent == nil {
		t.Errorf("the row deleted: %+v %v", r, err)
	}
}

// A movie two catalogs list keeps the catalog that listed it first, and
// that catalog's library as its folder, however often either is read
// again, which then writes nothing; once that catalog no longer lists it,
// the next catalog listing it takes it.
func TestATitleKeepsTheCatalogThatListedItFirst(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	addon := listingAddon()
	e.install(addons.Shared(), addon)
	top, fresh := e.library(e.member, "Top").ID, e.library(e.member, "New").ID
	both := itemID(titleKey(KindMovie, "tt3"))
	stored := func() (string, accounts.ID) {
		t.Helper()
		r, err := e.service.load(t.Context(), both)
		if err != nil {
			t.Fatal(err)
		}
		return r.CatalogID, deref(r.Parent)
	}
	e.listed(e.member, "Top")
	if catalog, parent := stored(); catalog != "top" || parent != top {
		t.Fatalf("listed by Top: %s %v", catalog, parent)
	}
	e.listed(e.member, "New")
	if catalog, parent := stored(); catalog != "top" || parent != top {
		t.Errorf("listed by New after Top: %s %v", catalog, parent)
	}
	if r, err := e.service.load(t.Context(), itemID(titleKey(KindMovie, "tt7"))); err != nil || r.CatalogID != "new" || deref(r.Parent) != fresh {
		t.Errorf("a movie only New lists: %+v %v", r, err)
	}
	ids := []accounts.ID{both}
	versions, updates := e.itemVersions(ids), e.itemUpdates()
	for range 2 {
		e.listed(e.member, "Top")
		e.listed(e.member, "New")
	}
	if again := e.itemVersions(ids); !maps.Equal(again, versions) {
		t.Errorf("the movie was written or locked: %v, then %v", versions, again)
	}
	if n := e.itemUpdates() - updates; n != 0 {
		t.Errorf("%d rows updated", n)
	}

	// Top drops the movie, which Polyfin forgets it held once it has not
	// seen it there for long.
	addon.mu.Lock()
	addon.catalogs["movie/top"] = slices.DeleteFunc(slices.Clone(addon.catalogs["movie/top"]), func(m stremio.Meta) bool { return m.ID == "tt3" })
	addon.mu.Unlock()
	later := time.Now().Add(arrivalForgotten + 24*time.Hour)
	e.service.SetClock(func() time.Time { return later })
	e.listed(e.member, "Top")
	e.listed(e.member, "New")
	if catalog, _ := stored(); catalog != "top" {
		t.Errorf("before Top's arrival is forgotten: %s", catalog)
	}
	if err := e.service.forgetArrivals(t.Context()); err != nil {
		t.Fatal(err)
	}
	e.listed(e.member, "New")
	if catalog, parent := stored(); catalog != "new" || parent != fresh {
		t.Errorf("once Top no longer lists it: %s %v", catalog, parent)
	}
}
