package library

import (
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testdb"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// countedEnv is an environment whose pool records the statements it runs.
func countedEnv(t *testing.T) (env, *statements) {
	t.Helper()
	config, err := pgxpool.ParseConfig(testdb.CopyURL(t, migrated.New(t)))
	if err != nil {
		t.Fatal(err)
	}
	counted := &statements{}
	config.ConnConfig.Tracer = counted
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	users, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	member, _ := users.CreateUser(t.Context(), accounts.NewUser{Name: "member", Password: "correct horse"})
	client := stremio.NewClient("test")
	store := addons.New(pool, client)
	service := New(pool, store, client, slog.New(slog.NewTextHandler(io.Discard, nil)), users.Settings)
	service.followUpDelays = nil
	service.readAhead = false
	return env{t: t, service: service, addons: store, users: users, member: member}, counted
}

// listedOnce is the names of metas without those listed before, as a
// listing shows them.
func listedOnce(metas []stremio.Meta) []string {
	var result []string
	seen := map[string]bool{}
	for _, meta := range metas {
		if !seen[meta.ID] {
			seen[meta.ID] = true
			result = append(result, meta.Name)
		}
	}
	return result
}

// forgetPagesBefore forgets the pages of addon's catalog before skip, in
// memory and in the database, as pages no longer kept.
func (e env) forgetPagesBefore(skip int) {
	e.t.Helper()
	e.service.pages.DeleteFunc(func(key pageKey, _ fetched[[]stremio.Meta]) bool { return key.skip < skip })
	if _, err := e.service.db.Exec(e.t.Context(), "DELETE FROM catalog_pages WHERE skip < $1", skip); err != nil {
		e.t.Fatal(err)
	}
}

// A page deep in a catalog reads the pages it shows only, through where
// the pages read before start in the listing: the pages before it may be
// gone, and the titles they listed are still counted once, those a later
// page repeats left out. A page changed before it moves it.
func TestADeepPageReadsOnlyThePagesItShows(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	at := &clock{now: time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)}
	e.service.SetClock(at.Now)
	e.setting(func(s *accounts.Settings) { s.CatalogRefreshMinutes = 60 })
	addon := pagedAddon(2000, 100)
	movies := addon.catalogs["movie/top"]
	// The second page repeats ten titles of the first.
	copy(movies[150:160], movies[0:10])
	e.install(addons.Shared(), addon)
	want := listedOnce(movies)
	if len(want) != 1990 {
		t.Fatalf("%d titles listed once", len(want))
	}

	// Scrolling through the catalog.
	for start := 0; start < len(want); start += 100 {
		page := e.children(e.member, "Top", start, 100)
		if got := names(page.Items); !slices.Equal(got, want[start:min(start+100, len(want))]) {
			t.Fatalf("page at %d: %v", start, got)
		}
	}
	last := e.children(e.member, "Top", 1900, 100)
	if last.Total != 1990 || last.More || len(last.Items) != 90 {
		t.Fatalf("last page: %d titles of %d, more %t", len(last.Items), last.Total, last.More)
	}

	e.forgetPagesBefore(1900)
	asked := len(addon.catalogRequests())
	again := e.children(e.member, "Top", 1900, 100)
	if !slices.Equal(names(again.Items), names(last.Items)) || again.Total != last.Total || again.More != last.More {
		t.Errorf("last page again: %v, %d titles, more %t", names(again.Items), again.Total, again.More)
	}
	if requests := len(addon.catalogRequests()) - asked; requests != 0 {
		t.Errorf("the last page asked the addon for %d pages before it", requests)
	}

	// The first page changes: a title of the last page takes the place of
	// one of its own, which the last page then lists no more.
	addon.mu.Lock()
	movies[5] = movies[1950]
	addon.mu.Unlock()
	at.advance(61 * time.Minute)
	e.service.Children(withRefreshing(t.Context()), e.member, e.library(e.member, "Top").ID, 0, 100, "")
	want = listedOnce(movies)
	changed := e.children(e.member, "Top", 1900, 100)
	if got := names(changed.Items); !slices.Equal(got, want[1900:]) || changed.Total != len(want) {
		t.Errorf("last page once the first changed: %v, %d titles, want %v", got, changed.Total, want[1900:])
	}
}

// After a restart, a deep page reads the pages it needs from the database
// in one statement, and asks the addon nothing.
func TestKeptPagesAreReadInOneStatementAfterARestart(t *testing.T) {
	t.Parallel()
	e, counted := countedEnv(t)
	addon := pagedAddon(1000, 100)
	e.install(addons.Shared(), addon)
	before := e.children(e.member, "Top", 900, 100)
	if len(before.Items) != 100 || before.Items[0].Name != "movie 900" {
		t.Fatalf("last page: %v", names(before.Items))
	}

	e = e.restarted()
	e.library(e.member, "Top")
	asked := len(addon.catalogRequests())
	counted.take()
	after := e.children(e.member, "Top", 900, 100)
	kinds, _ := counted.take()
	if !slices.Equal(names(after.Items), names(before.Items)) || after.Total != before.Total {
		t.Errorf("after a restart: %v, %d titles", names(after.Items), after.Total)
	}
	if got := kinds["SELECT catalog_pages"]; got != 1 {
		t.Errorf("%d statements read the pages kept, want 1: %v", got, kinds)
	}
	if requests := len(addon.catalogRequests()) - asked; requests != 0 {
		t.Errorf("%d pages asked of the addon", requests)
	}
}

// A library a user opens is read to its end in the background: scrolling
// it then asks the addon nothing.
func TestAnOpenedLibraryIsReadThroughInTheBackground(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.service.readAhead = true
	addon := pagedAddon(1000, 100)
	e.install(addons.Shared(), addon)
	top := e.library(e.member, "Top").ID
	opened := WithOpening(t.Context())
	if page, err := e.service.Children(opened, e.member, top, 0, 100, ""); err != nil || len(page.Items) != 100 {
		t.Fatalf("first page: %d titles, %v", len(page.Items), err)
	}
	// The read asks for the pages it needs at once, a few past the end
	// among them: it is over once the catalog's index holds its end.
	src := source{addon: installed{addon: e.addon(t), shared: true}, catalog: addon.manifest.Catalogs[0]}
	eventually(t, "reading the library to its end", func() bool {
		index := e.service.indexOf(src, accounts.DefaultCatalogLimit)
		return index != nil && index.ended
	})
	asked := len(addon.catalogRequests())
	for start := 100; start < 1000; start += 100 {
		page, err := e.service.Children(opened, e.member, top, start, 100, "")
		if err != nil || len(page.Items) != 100 || page.Items[0].Name != addon.catalogs["movie/top"][start].Name {
			t.Fatalf("page at %d: %v, %v", start, names(page.Items), err)
		}
	}
	if requests := addon.catalogRequests()[asked:]; len(requests) != 0 {
		t.Errorf("scrolling asked the addon for %v", requests)
	}
}

// addon is the only addon e's server installed.
func (e env) addon(t *testing.T) addons.Addon {
	t.Helper()
	list, err := e.addons.Addons(t.Context(), addons.Shared())
	if err != nil || len(list) != 1 {
		t.Fatalf("addons: %v, %v", list, err)
	}
	return list[0]
}
