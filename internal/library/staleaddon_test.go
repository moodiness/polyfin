package library

import (
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Refresh everything makes an addon's kept pages and descriptions old, in
// memory and in the database, and forgets its page sizes and version
// lists: nothing is asked at once, then each is shown as it was kept and
// asked for again in the background, and the hourly read of the first
// pages asks again for those. Another addon's stay as they were.
func TestMarkingAnAddonStaleHasItsAnswersAskedAgainAsShown(t *testing.T) {
	t.Parallel()
	e := newLifetimeEnv(t)
	e.setting(func(s *accounts.Settings) { s.CatalogRefreshMinutes = 60 })
	other := &fakeAddon{
		manifest: stremio.Manifest{ID: "b", Name: "B", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "other", Name: "Other"}}},
		catalogs: map[string][]stremio.Meta{"movie/other": titles("movie", 1)},
	}
	e.install(addons.Shared(), other)
	a, b := e.addonID("a"), e.addonID("b")
	// No automatic image reads a page behind the test's back.
	for _, l := range []addons.LibraryKey{{AddonID: a, CatalogType: "movie", CatalogID: "top"}, {AddonID: b, CatalogType: "movie", CatalogID: "other"}} {
		if err := e.addons.SetLibraryImage(t.Context(), addons.Shared(), l, addons.LibraryImageNone); err != nil {
			t.Fatal(err)
		}
	}
	listOther := func() {
		t.Helper()
		if page, err := e.service.Children(t.Context(), e.member, e.library(e.member, "Other").ID, 0, 10, ""); err != nil || len(page.Items) != 1 {
			t.Fatalf("listing Other: %+v %v", page, err)
		}
	}
	movie := e.list().ID
	describe := func() string {
		t.Helper()
		item, err := e.service.Item(t.Context(), e.member, movie)
		if err != nil {
			t.Fatal(err)
		}
		return item.Overview
	}
	describe()
	e.versions(movie)
	listOther()
	sizes := map[accounts.ID]source{
		a: {addon: installed{addon: addons.Addon{ID: a}}, catalog: stremio.Catalog{Type: "movie", ID: "top"}},
		b: {addon: installed{addon: addons.Addon{ID: b}}, catalog: stremio.Catalog{Type: "movie", ID: "other"}},
	}
	for _, src := range sizes {
		e.service.pageSize(t.Context(), src)
		e.service.learnPageSize(t.Context(), src, 50)
	}
	pages, metas, streams := e.requests("catalog/"), e.requests("meta/"), e.requests("stream/")
	if pages != 1 || metas != 1 || streams != 1 || requestsTo(other, "catalog/") != 1 {
		t.Fatalf("before: %d pages, %d descriptions, %d version lists, %d of the other addon's pages", pages, metas, streams, requestsTo(other, "catalog/"))
	}
	e.wait(time.Minute)
	asked := e.requests("")

	stale, err := e.service.MarkAddonStale(t.Context(), a)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Pages != 1 || stale.Descriptions != 1 || stale.VersionLists != 1 {
		t.Errorf("made old: %+v", stale)
	}
	if got := e.requests(""); got != asked {
		t.Errorf("marking asked the addon %d times", got-asked)
	}
	// In the database: the addon's pages are past the catalog refresh age
	// and its descriptions past theirs, the other addon's page is not.
	now := e.service.now()
	age := func(table string, addon accounts.ID) time.Duration {
		t.Helper()
		var at time.Time
		if err := e.service.db.QueryRow(t.Context(), "SELECT max(fetched_at) FROM "+table+" WHERE addon_id = $1", addon).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return now.Sub(at)
	}
	if got := age("catalog_pages", a); got <= time.Hour || got > 2*time.Hour {
		t.Errorf("the addon's page kept is %v old", got)
	}
	if got := age("metas", a); got <= metaTTL || got > metaTTL+time.Hour {
		t.Errorf("the addon's description kept is %v old", got)
	}
	if got := age("catalog_pages", b); got > 2*time.Minute {
		t.Errorf("the other addon's page kept is %v old", got)
	}
	// Its page sizes are forgotten, the other addon's stay.
	if got := e.service.pageSize(t.Context(), sizes[a]); got != 0 {
		t.Errorf("the addon's page size: %d", got)
	}
	if got := e.service.pageSize(t.Context(), sizes[b]); got != 50 {
		t.Errorf("the other addon's page size: %d", got)
	}
	var kept []accounts.ID
	rows, err := e.service.db.Query(t.Context(), "SELECT addon_id FROM catalog_sizes")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id accounts.ID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		kept = append(kept, id)
	}
	if rows.Close(); len(kept) != 1 || kept[0] != b {
		t.Errorf("page sizes kept for %v, want the other addon's", kept)
	}

	// Shown, the page and the description are those kept, and asked for
	// again in the background; the version list is asked for again.
	e.list()
	describe()
	eventually(t, "asking for the page again", func() bool { return e.requests("catalog/") == pages+1 })
	eventually(t, "asking for the description again", func() bool { return e.requests("meta/") == metas+1 })
	e.versions(movie)
	if got := e.requests("stream/"); got != streams+1 {
		t.Errorf("version lists asked %d times, want %d", got, streams+1)
	}
	// Once asked again, they are fresh.
	e.list()
	describe()
	listOther()
	if got := e.requests("catalog/"); got != pages+1 {
		t.Errorf("pages asked %d times once fresh again", got)
	}
	if got := requestsTo(other, "catalog/"); got != 1 {
		t.Errorf("the other addon's page asked %d times", got)
	}
}

// The hourly read of the libraries' first pages asks again for the first
// pages of an addon made old, and not for another addon's.
func TestFirstPagesOfAnAddonMadeStaleAreReadWithinTheHour(t *testing.T) {
	t.Parallel()
	e := newLifetimeEnv(t)
	e.setting(func(s *accounts.Settings) { s.CatalogRefreshMinutes = 60 })
	other := &fakeAddon{
		manifest: stremio.Manifest{ID: "b", Name: "B", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "other", Name: "Other"}}},
		catalogs: map[string][]stremio.Meta{"movie/other": titles("movie", 1)},
	}
	e.install(addons.Shared(), other)
	for _, l := range []addons.LibraryKey{{AddonID: e.addonID("a"), CatalogType: "movie", CatalogID: "top"}, {AddonID: e.addonID("b"), CatalogType: "movie", CatalogID: "other"}} {
		if err := e.addons.SetLibraryImage(t.Context(), addons.Shared(), l, addons.LibraryImageNone); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.service.Warm(t.Context()); err != nil {
		t.Fatal(err)
	}
	if e.requests("catalog/") != 1 || requestsTo(other, "catalog/") != 1 {
		t.Fatalf("first read: %d and %d pages", e.requests("catalog/"), requestsTo(other, "catalog/"))
	}
	e.wait(time.Minute)
	if _, err := e.service.MarkAddonStale(t.Context(), e.addonID("a")); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Warm(t.Context()); err != nil {
		t.Fatal(err)
	}
	eventually(t, "reading the first page again", func() bool { return e.requests("catalog/") == 2 })
	if got := requestsTo(other, "catalog/"); got != 1 {
		t.Errorf("the other addon's first page asked %d times", got)
	}
}
