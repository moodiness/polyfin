package library

import (
	"fmt"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/stremio"
)

// largeAddon serves All, a movie catalog, and Shows, a series catalog, of
// count titles each, in pages of 100.
func largeAddon(count int) *fakeAddon {
	return &fakeAddon{
		manifest: stremio.Manifest{ID: "large", Name: "Large", Version: "1", Types: []string{"movie", "series"},
			IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{
				{Type: "movie", ID: "all", Name: "All", Extra: []stremio.Extra{{Name: "skip"}}},
				{Type: "series", ID: "shows", Name: "Shows", Extra: []stremio.Extra{{Name: "skip"}}},
			}},
		catalogs: map[string][]stremio.Meta{"movie/all": titles("movie", count, "Drama"), "series/shows": titles("series", count, "Drama")},
		pageSize: 100,
	}
}

// The titles read for a list over large catalogs are kept once for every
// user who reads them the same way, counted in bytes, and never past the
// cache's bound: lists whose titles do not fit are read again from the
// pages kept, and answer as before.
func TestTheTitlesKeptForSmartListsStayWithinTheirBound(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	// Every catalog is read whole before it is kept, however slow the
	// test machine.
	e.service.smartWait = time.Minute
	e.install(addons.Shared(), largeAddon(2000))
	installed, err := e.addons.Addons(t.Context(), addons.Shared())
	if err != nil || len(installed) != 1 {
		t.Fatalf("addons: %v %v", installed, err)
	}
	addon := installed[0].ID
	users := []accounts.User{e.admin, e.member}
	for i := range 3 {
		user, err := e.users.CreateUser(t.Context(), accounts.NewUser{Name: fmt.Sprintf("viewer%d", i), Password: "correct horse"})
		if err != nil {
			t.Fatal(err)
		}
		users = append(users, user)
	}
	lists := []smartlists.List{
		e.makeList(nil, "Movies", addon, []string{"movie/all", "series/shows"},
			group(smartlists.MatchAll, smartlists.Rule{Field: smartlists.FieldType, Value: "movie"}), smartlists.SortCatalog, 0),
		e.makeList(nil, "Everything", addon, []string{"movie/all", "series/shows"}, group(smartlists.MatchAll), smartlists.SortName, 0),
		e.makeList(nil, "First ones", addon, []string{"movie/all", "series/shows"},
			group(smartlists.MatchAll, smartlists.Rule{Field: smartlists.FieldRank, Max: new(500.0)}), smartlists.SortCatalog, 0),
	}
	totals := []int{2000, 4000, 1000}
	listAll := func() {
		t.Helper()
		for _, user := range users {
			for i, list := range lists {
				if page := e.listing(user, list); page.Total != totals[i] || len(page.Items) != 100 {
					t.Fatalf("%s for %s: %d titles of %d", list.Name, user.Name, len(page.Items), page.Total)
				}
			}
		}
	}

	// One entry per list, whatever the number of users, its size counted.
	listAll()
	kept := e.service.smartCache
	if kept.Len() != len(lists) || kept.Bytes() == 0 || kept.Bytes() > smartBytes {
		t.Fatalf("kept: %d entries, %d bytes", kept.Len(), kept.Bytes())
	}
	// Each title counts at least what its description takes in the page
	// caches: every list keeps the 4,000 titles of its catalogs, its rules
	// being applied at each listing.
	if each := kept.Bytes() / (3 * 4000); each < smartTitleSize+2*cache.JSONSize(titles("movie", 1, "Drama")[0]) {
		t.Errorf("%d bytes counted for each title", each)
	}

	// A bound that holds the titles of two lists keeps two, the least
	// recently listed going; one smaller than a list's titles keeps none.
	// Every list answers as before.
	read, _, err := e.service.smartCandidates(t.Context(), e.mustView(e.member), lists[1])
	if err != nil {
		t.Fatal(err)
	}
	one := smartTitlesSize(read)
	for _, c := range []struct {
		limit, entries int
	}{{2*one + one/2, 2}, {one - 1, 0}} {
		e.service.smartCache = cache.New[smartKey, []smartTitle](200, smartLife).Sized(c.limit, smartTitlesSize)
		listAll()
		if bounded := e.service.smartCache; bounded.Bytes() > c.limit || bounded.Len() != c.entries {
			t.Errorf("bounded to %d bytes: %d entries, %d bytes", c.limit, bounded.Len(), bounded.Bytes())
		}
	}
}

// mustView is the view user browses.
func (e env) mustView(user accounts.User) view {
	e.t.Helper()
	v, err := e.service.view(e.t.Context(), user)
	if err != nil {
		e.t.Fatal(err)
	}
	return v
}
