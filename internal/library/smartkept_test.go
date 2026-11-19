package library

import (
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/stremio"
)

// keptEntry returns the titles kept for list as user reads its catalogs.
func (e env) keptEntry(user accounts.User, list smartlists.List) smartEntry {
	e.t.Helper()
	v := e.mustView(user)
	entry, ok := e.service.smartCache.Get(smartKey{list.ID, list.UpdatedAt, smartReader(v, e.service.smartSources(v, list))})
	if !ok {
		e.t.Fatalf("nothing kept for %s", list.Name)
	}
	return entry
}

// sameRead reports whether a and b are the same read of a catalog, not
// two reads of the same titles.
func sameRead(a, b []stremio.Meta) bool {
	return len(a) > 0 && len(b) > 0 && &a[0] == &b[0]
}

// The titles read for a smart list serve its listings as long as its
// catalogs' pages stay as they were: a page fetched again unchanged leaves
// them, and one the addon changed has its catalog read again alone, while
// the collection images, which take the pages kept only, take the titles
// kept until a listing reads them again.
func TestTheTitlesKeptForASmartListLastUntilItsPagesChange(t *testing.T) {
	t.Parallel()
	e, addon, id := smartEnv(t)
	list := e.makeList(nil, "Everything", id, []string{"movie/popular", "series/shows"}, group(smartlists.MatchAll), smartlists.SortCatalog, 0)
	first := names(e.listing(e.member, list).Items)
	read := e.keptEntry(e.member, list)
	if len(first) != 7 || slices.Contains(read.missing, true) {
		t.Fatalf("first listing: %v, missing %v", first, read.missing)
	}
	sources := e.service.smartSources(e.mustView(e.member), list)

	// The series page, fetched again as it was, changes nothing.
	if _, err := e.service.fetchPage(t.Context(), sources[1], sources[1].key(0)); err != nil {
		t.Fatal(err)
	}
	requests := len(addon.catalogRequests())
	if got := names(e.listing(e.member, list).Items); !slices.Equal(got, first) {
		t.Errorf("after an unchanged page: %v", got)
	}
	if kept := e.keptEntry(e.member, list); !sameRead(kept.lists[0], read.lists[0]) || !sameRead(kept.lists[1], read.lists[1]) {
		t.Error("a catalog was read again although its pages did not change")
	}

	// A movie the addon adds shows once its page is fetched again: the
	// movies are read again, the series are not.
	addon.mu.Lock()
	addon.catalogs["movie/popular"] = append([]stremio.Meta{{ID: "tt10", Type: "movie", Name: "Nouveau", ReleaseInfo: "2024",
		ImdbRating: "7.5", Genres: []string{"Drama"}}}, addon.catalogs["movie/popular"]...)
	addon.mu.Unlock()
	if got := names(e.listing(e.member, list).Items); !slices.Equal(got, first) {
		t.Errorf("before the page is fetched again: %v", got)
	}
	if _, err := e.service.fetchPage(t.Context(), sources[0], sources[0].key(0)); err != nil {
		t.Fatal(err)
	}
	images, _, err := e.service.smartCandidates(withKeptOnly(t.Context()), e.mustView(e.member), list)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(images, func(c smartTitle) bool { return c.item.Name == "Nouveau" }) || len(images) != len(first) {
		t.Errorf("the images' read did not take the titles kept: %d titles", len(images))
	}
	if got := names(e.listing(e.member, list).Items); !slices.Contains(got, "Nouveau") {
		t.Errorf("once the page changed: %v", got)
	}
	kept := e.keptEntry(e.member, list)
	if sameRead(kept.lists[0], read.lists[0]) || !sameRead(kept.lists[1], read.lists[1]) {
		t.Error("the catalog whose page changed was not the only one read again")
	}
	if asked := len(addon.catalogRequests()) - requests; asked != 1 {
		t.Errorf("%d catalog pages asked of the addon, want the one fetched again", asked)
	}
}

// Editing a list's rules, adding a catalog to it or removing one has its
// next listing read it again.
func TestASmartListEditedIsReadAgain(t *testing.T) {
	t.Parallel()
	e, _, id := smartEnv(t)
	list := e.makeList(nil, "Edited", id, []string{"movie/popular"}, group(smartlists.MatchAll), smartlists.SortName, 0)
	if got := names(e.listing(e.member, list).Items); len(got) != 5 {
		t.Fatalf("first listing: %v", got)
	}
	update := func(change func(*smartlists.List)) {
		t.Helper()
		change(&list)
		updated, err := e.service.smart.Update(t.Context(), list)
		if err != nil {
			t.Fatal(err)
		}
		list = updated
	}
	update(func(l *smartlists.List) {
		l.Rules = group(smartlists.MatchAll, smartlists.Rule{Field: smartlists.FieldRank, Max: new(2.0)})
	})
	if got := names(e.listing(e.member, list).Items); !slices.Equal(got, []string{"La Haine", "Le Dîner"}) {
		t.Errorf("rules edited: %v", got)
	}
	update(func(l *smartlists.List) {
		l.Rules = group(smartlists.MatchAll)
		l.Catalogs = append(l.Catalogs, smartlists.Catalog{AddonID: id, Type: "series", ID: "shows"})
	})
	if got := names(e.listing(e.member, list).Items); len(got) != 7 || !slices.Contains(got, "Friends") {
		t.Errorf("a catalog added: %v", got)
	}
	update(func(l *smartlists.List) { l.Catalogs = l.Catalogs[1:] })
	if got := names(e.listing(e.member, list).Items); !slices.Equal(got, []string{"Friends", "Kaamelott"}) {
		t.Errorf("a catalog removed: %v", got)
	}
}

// A list read to the catalog limit is read again to a new one.
func TestASmartListIsReadAgainToANewCatalogLimit(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.service.smartWait = time.Minute
	e.setting(func(s *accounts.Settings) { s.CatalogLimit = accounts.MinCatalogLimit })
	e.install(addons.Shared(), pagedAddon(150, 50))
	installed, err := e.addons.Addons(t.Context(), addons.Shared())
	if err != nil || len(installed) != 1 {
		t.Fatalf("addons: %v %v", installed, err)
	}
	list := e.makeList(nil, "Top", installed[0].ID, []string{"movie/top"}, group(smartlists.MatchAll), smartlists.SortCatalog, 0)
	if total := e.listing(e.member, list).Total; total != accounts.MinCatalogLimit {
		t.Errorf("to the catalog limit: %d titles", total)
	}
	e.setting(func(s *accounts.Settings) { s.CatalogLimit = 200 })
	if total := e.listing(e.member, list).Total; total != 150 {
		t.Errorf("to a higher catalog limit: %d titles", total)
	}
}

// lateAddon serves Late, a series catalog of one series.
func lateAddon() *fakeAddon {
	return &fakeAddon{
		manifest: stremio.Manifest{ID: "late", Name: "Late", Version: "1", Types: []string{"series"}, IDPrefixes: []string{"tt"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs:  []stremio.Catalog{{Type: "series", ID: "late", Name: "Late", Extra: []stremio.Extra{{Name: "skip"}}}}},
		catalogs: map[string][]stremio.Meta{"series/late": {{ID: "tt90", Type: "series", Name: "Late Show", ReleaseInfo: "2020",
			ImdbRating: "7.1", Genres: []string{"Drama"}}}},
		metas: map[string]stremio.Meta{},
	}
}

// While the addon of a list's catalog asks to wait, the list lists the
// other catalogs' titles, not complete: its next listings read again only
// the catalog it lacks, without asking that addon, until it answers.
func TestASmartListReadsAgainOnlyTheCatalogItLacks(t *testing.T) {
	t.Parallel()
	e, _, cinema := smartEnv(t)
	late := lateAddon()
	e.install(addons.Shared(), late)
	var lateID accounts.ID
	installed, err := e.addons.Addons(t.Context(), addons.Shared())
	if err != nil {
		t.Fatal(err)
	}
	host := ""
	for _, a := range installed {
		if a.Manifest.ID == "late" {
			lateID, host = a.ID, stremio.HostOf(a.ManifestURL)
		}
	}
	list := e.makeList(nil, "Both", cinema, []string{"movie/popular"}, group(smartlists.MatchAll), smartlists.SortName, 0)
	list.Catalogs = append(list.Catalogs, smartlists.Catalog{AddonID: lateID, Type: "series", ID: "late"})
	list, err = e.service.smart.Update(t.Context(), list)
	if err != nil {
		t.Fatal(err)
	}
	waiting := func(wait bool) {
		e.service.catalogWaits.mu.Lock()
		defer e.service.catalogWaits.mu.Unlock()
		if e.service.catalogWaits.hosts == nil {
			e.service.catalogWaits.hosts = map[string]catalogWait{}
		}
		if wait {
			e.service.catalogWaits.hosts[host] = catalogWait{until: time.Now().Add(time.Hour), last: time.Hour}
		} else {
			delete(e.service.catalogWaits.hosts, host)
		}
	}

	waiting(true)
	if page := e.listing(e.member, list); page.Total != 5 || slices.Contains(names(page.Items), "Late Show") {
		t.Fatalf("while Late asks to wait: %v", names(page.Items))
	}
	read := e.keptEntry(e.member, list)
	if !slices.Equal(read.missing, []bool{false, true}) {
		t.Fatalf("missing: %v", read.missing)
	}
	if page := e.listing(e.member, list); page.Total != 5 {
		t.Errorf("listed again while Late asks to wait: %v", names(page.Items))
	}
	if kept := e.keptEntry(e.member, list); !sameRead(kept.lists[0], read.lists[0]) || !slices.Equal(kept.missing, []bool{false, true}) {
		t.Errorf("listed again: the movies read again, or Late's catalog not lacking: %v", kept.missing)
	}
	if asked := len(late.catalogRequests()); asked != 0 {
		t.Errorf("%d pages asked of Late while it asks to wait", asked)
	}

	waiting(false)
	if page := e.listing(e.member, list); page.Total != 6 || !slices.Contains(names(page.Items), "Late Show") {
		t.Errorf("once Late answers: %v", names(page.Items))
	}
	if kept := e.keptEntry(e.member, list); !sameRead(kept.lists[0], read.lists[0]) || slices.Contains(kept.missing, true) {
		t.Errorf("once Late answers: the movies read again, or a catalog still lacking: %v", kept.missing)
	}
}
