package addons

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// served is an addon whose manifest a test changes: its catalogs, a
// failure, or another kind of addon. It counts the requests it answers;
// base is its address, url its manifest's.
type served struct {
	base, url string
	catalogs  atomic.Pointer[[]stremio.Catalog]
	failing   atomic.Bool
	eclipse   atomic.Bool
	hits      atomic.Int32
}

func serve(t *testing.T, catalogs ...stremio.Catalog) *served {
	t.Helper()
	s := &served{}
	s.set(catalogs...)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s.hits.Add(1)
		switch {
		case s.failing.Load():
			http.Error(w, "down for a moment", http.StatusInternalServerError)
		case s.eclipse.Load():
			_, _ = w.Write([]byte(`{"id":"music","name":"Music","version":"1.0.0","resources":["search","stream"],"types":["track"]}`))
		default:
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "test", Name: "Test", Version: "1.0.0",
				Resources: []stremio.Resource{{Name: "catalog"}}, Catalogs: *s.catalogs.Load()})
		}
	}))
	s.base, s.url = server.URL, server.URL+"/manifest.json"
	return s
}

func (s *served) set(catalogs ...stremio.Catalog) { s.catalogs.Store(&catalogs) }

func movies(ids ...string) []stremio.Catalog {
	list := make([]stremio.Catalog, len(ids))
	for i, id := range ids {
		list[i] = stremio.Catalog{Type: "movie", ID: id, Name: "Movies " + id}
	}
	return list
}

// refreshAll runs RefreshSources for every user of users, with the
// server's settings but for personal, and fails the test on an error.
func refreshAll(t *testing.T, store *Store, users *accounts.Store, personal bool) SourcesRefreshed {
	t.Helper()
	all, err := users.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	settings := users.Settings()
	settings.PersonalAddons = personal
	done, err := store.RefreshSources(t.Context(), all, settings, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return done
}

// catalogIDs lists the catalogs of the manifest stored for the addon id.
func catalogIDs(t *testing.T, store *Store, id accounts.ID) []string {
	t.Helper()
	addon, err := store.Find(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, c := range addon.Manifest.Catalogs {
		ids = append(ids, c.ID)
	}
	return ids
}

// A run stores what an addon's manifest says now: a catalog added waits,
// off, under Libraries, and the library of a catalog renamed follows it,
// keeping its name.
func TestRefreshSourcesBringsAddedAndRenamedCatalogs(t *testing.T) {
	t.Parallel()
	store, users := newStore(t)
	ctx := t.Context()
	addon := serve(t, stremio.Catalog{Type: "movie", ID: "top-v1", Name: "Top"})
	installed, err := store.Install(ctx, Shared(), addon.url, false)
	if err != nil {
		t.Fatal(err)
	}
	name := "Best"
	if _, err := store.SetLibraries(ctx, Shared(), []LibraryChoice{{AddonID: installed.ID, CatalogType: "movie", CatalogID: "top-v1", Name: &name}}); err != nil {
		t.Fatal(err)
	}
	addon.set(stremio.Catalog{Type: "movie", ID: "top-v2", Name: "Top"}, stremio.Catalog{Type: "series", ID: "new", Name: "New"})

	if done := refreshAll(t, store, users, true); done != (SourcesRefreshed{Refreshed: 1}) {
		t.Errorf("run: %+v", done)
	}
	if ids := catalogIDs(t, store, installed.ID); !slices.Equal(ids, []string{"top-v2", "new"}) {
		t.Errorf("catalogs after the run: %v", ids)
	}
	libraries, err := store.Libraries(ctx, Shared())
	if err != nil {
		t.Fatal(err)
	}
	var on, off []string
	for _, library := range libraries {
		if library.Enabled {
			on = append(on, library.Catalog.ID+" "+*library.Name)
		} else {
			off = append(off, library.Catalog.ID)
		}
	}
	if !slices.Equal(on, []string{"top-v2 Best"}) || !slices.Equal(off, []string{"new"}) {
		t.Errorf("libraries on %v, off %v", on, off)
	}
}

// A library whose catalog a run finds missing stays, with the manifest
// as it was, until a run RemovalConfirmation later finds it missing still.
// A catalog that came back in between starts again. Refresh on the
// source's page removes such a library at once.
func TestRefreshSourcesRemovesALibraryOnTheConfirmingRun(t *testing.T) {
	t.Parallel()
	store, users := newStore(t)
	ctx := t.Context()
	addon := serve(t, movies("a", "b")...)
	installed, err := store.Install(ctx, Shared(), addon.url, false)
	if err != nil {
		t.Fatal(err)
	}
	if ids := enabled(t, store, Shared()); !slices.Equal(ids, []string{"a", "b"}) {
		t.Fatalf("libraries at install: %v", ids)
	}
	// Some time passed since the catalogs were first found missing.
	elapse := func() {
		t.Helper()
		if _, err := store.db.Exec(ctx, "UPDATE missing_catalogs SET since = since - make_interval(secs => $1)", RemovalConfirmation.Seconds()); err != nil {
			t.Fatal(err)
		}
	}
	missing := func() int {
		t.Helper()
		var n int
		if err := store.db.QueryRow(ctx, "SELECT count(*) FROM missing_catalogs").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	addon.set(movies("a", "c")...)
	for run := range 2 {
		if done := refreshAll(t, store, users, true); done != (SourcesRefreshed{Held: 1}) {
			t.Errorf("run %d: %+v", run, done)
		}
		if ids := enabled(t, store, Shared()); !slices.Equal(ids, []string{"a", "b"}) {
			t.Errorf("libraries after run %d: %v", run, ids)
		}
		// The whole manifest is held, the catalog added with it.
		if ids := catalogIDs(t, store, installed.ID); !slices.Equal(ids, []string{"a", "b"}) {
			t.Errorf("catalogs after run %d: %v", run, ids)
		}
	}
	// It came back: a later miss counts from then.
	addon.set(movies("a", "b", "c")...)
	if done := refreshAll(t, store, users, true); done != (SourcesRefreshed{Refreshed: 1}) || missing() != 0 {
		t.Errorf("run with the catalog back: %+v, %d missing", done, missing())
	}
	addon.set(movies("a", "c")...)
	if done := refreshAll(t, store, users, true); done != (SourcesRefreshed{Held: 1}) {
		t.Errorf("run with the catalog missing again: %+v", done)
	}
	elapse()
	if done := refreshAll(t, store, users, true); done != (SourcesRefreshed{Refreshed: 1}) {
		t.Errorf("confirming run: %+v", done)
	}
	if ids := enabled(t, store, Shared()); !slices.Equal(ids, []string{"a"}) {
		t.Errorf("libraries after the confirming run: %v", ids)
	}
	if ids := catalogIDs(t, store, installed.ID); !slices.Equal(ids, []string{"a", "c"}) || missing() != 0 {
		t.Errorf("catalogs after the confirming run: %v, %d missing", ids, missing())
	}

	// Refresh, by hand, removes at once, a run having just held it.
	addon.set(movies("c")...)
	if done := refreshAll(t, store, users, true); done != (SourcesRefreshed{Held: 1}) {
		t.Errorf("run before Refresh: %+v", done)
	}
	_, changes, err := store.Refresh(ctx, Shared(), installed.ID, false)
	if err != nil || len(changes.Removed) != 1 || changes.Removed[0].CatalogID != "a" || len(changes.Held) != 0 {
		t.Errorf("Refresh: %+v %v", changes, err)
	}
	if ids := enabled(t, store, Shared()); len(ids) != 0 || missing() != 0 {
		t.Errorf("libraries after Refresh: %v, %d missing", ids, missing())
	}
}

// An addon that fails, or answers as another kind of addon, is left as it
// is, and the run goes on with the next.
func TestRefreshSourcesLeavesAFailingAddon(t *testing.T) {
	t.Parallel()
	store, users := newStore(t)
	ctx := t.Context()
	failing, other, next := serve(t, movies("a")...), serve(t, movies("b")...), serve(t, movies("c")...)
	var ids []accounts.ID
	for _, addon := range []*served{failing, other, next} {
		installed, err := store.Install(ctx, Shared(), addon.url, false)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, installed.ID)
	}
	before, err := store.Addons(ctx, Shared())
	if err != nil {
		t.Fatal(err)
	}
	failing.set()
	failing.failing.Store(true)
	other.set()
	other.eclipse.Store(true)
	next.set(movies("c", "d")...)
	if _, _, err := store.Refresh(ctx, Shared(), ids[1], false); !errors.Is(err, ErrKindChanged) {
		t.Fatalf("Refresh of an addon answering as another kind: %v", err)
	}

	if done := refreshAll(t, store, users, true); done != (SourcesRefreshed{Refreshed: 1, Failed: 2}) {
		t.Errorf("run: %+v", done)
	}
	for i, id := range ids[:2] {
		after, err := store.Find(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if !after.RefreshedAt.Equal(before[i].RefreshedAt) || !slices.Equal(catalogIDs(t, store, id), []string{before[i].Manifest.Catalogs[0].ID}) {
			t.Errorf("addon %d changed: %+v", i, after)
		}
	}
	if got := catalogIDs(t, store, ids[2]); !slices.Equal(got, []string{"c", "d"}) {
		t.Errorf("the next addon's catalogs: %v", got)
	}
	if got := enabled(t, store, Shared()); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("libraries: %v", got)
	}
}

// Users' own addons are refreshed as their own Refresh would be: skipped
// while the server turns them off, or the user's own permission does; a
// member's read from public addresses only, an administrator's wherever
// it is.
func TestRefreshSourcesRefreshesOwnAddonsAsTheirOwners(t *testing.T) {
	t.Parallel()
	store, users, network, _ := reaching(t)
	ctx := t.Context()
	root, err := users.CreateUser(ctx, accounts.NewUser{Name: "root", Password: "correct horse", IsAdministrator: true})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := users.CreateUser(ctx, accounts.NewUser{Name: "bob", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	roots, bobsLocal, bobsPublic := serve(t, movies("a")...), serve(t, movies("b")...), serve(t, movies("c")...)
	rootsAddon, err := store.Install(ctx, Personal(root.ID), roots.url, false)
	if err != nil {
		t.Fatal(err)
	}
	// One of Bob's addons is on the loopback, which his own Refresh may
	// not reach: installed as an administrator would.
	local, err := store.Install(ctx, Personal(bob.ID), bobsLocal.url, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Refresh(ctx, Personal(bob.ID), local.ID, true); !errors.Is(err, stremio.ErrPrivateNetwork) {
		t.Fatalf("Bob's own Refresh: %v", err)
	}
	public, err := store.Install(ctx, Personal(bob.ID), network.Public(t, bobsPublic.base)+"/manifest.json", true)
	if err != nil {
		t.Fatal(err)
	}
	all := []*served{roots, bobsLocal, bobsPublic}
	requests := func() int32 {
		var n int32
		for _, s := range all {
			n += s.hits.Load()
		}
		return n
	}
	for _, s := range all {
		s.hits.Store(0)
		s.set(append(*s.catalogs.Load(), stremio.Catalog{Type: "movie", ID: "new", Name: "New"})...)
	}

	if done := refreshAll(t, store, users, false); done != (SourcesRefreshed{Skipped: 3}) || requests() != 0 {
		t.Errorf("run with users' own addons off: %+v, %d requests", done, requests())
	}
	if _, err := users.UpdateUser(ctx, root.ID, accounts.UserChanges{PersonalAddons: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	if done := refreshAll(t, store, users, true); done != (SourcesRefreshed{Refreshed: 1, Failed: 1, Skipped: 1}) || roots.hits.Load() != 0 {
		t.Errorf("run with root's own addons off: %+v, %d requests", done, roots.hits.Load())
	}
	if _, err := users.UpdateUser(ctx, root.ID, accounts.UserChanges{PersonalAddons: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	if done := refreshAll(t, store, users, true); done != (SourcesRefreshed{Refreshed: 2, Failed: 1}) {
		t.Errorf("run: %+v", done)
	}
	for _, c := range []struct {
		name string
		id   accounts.ID
		want []string
	}{
		{"root's, on the loopback", rootsAddon.ID, []string{"a", "new"}},
		{"Bob's, on a public address", public.ID, []string{"c", "new"}},
		// Never asked: confined, as Bob's own Refresh is.
		{"Bob's, on the loopback", local.ID, []string{"b"}},
	} {
		if got := catalogIDs(t, store, c.id); !slices.Equal(got, c.want) {
			t.Errorf("%s: catalogs %v, want %v", c.name, got, c.want)
		}
	}
	if bobsLocal.hits.Load() != 0 {
		t.Errorf("Bob's addon on the loopback was asked %d times", bobsLocal.hits.Load())
	}
}
