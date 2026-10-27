package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/silo/silotest"
	"github.com/moodiness/polyfin/internal/stremio"
)

// filtersAddon serves Films, movies its genre filter narrows to Action,
// Drama or Horror, Shows, a series of Action, and Picks, a catalog of
// collections whose own genre filter chooses collections: Action picks,
// the Action films, and Show picks, the shows.
func filtersAddon(t *testing.T) string {
	t.Helper()
	title := func(id, metaType, name, year, rating string, genres ...string) stremio.Meta {
		meta := stremio.Meta{ID: id, Type: metaType, Name: name, Genres: genres, ReleaseInfo: stremio.Text(year),
			Released: year + "-01-01T00:00:00.000Z", Runtime: "1h 30min"}
		if rating != "" {
			meta.Extras = &stremio.Extras{Certification: rating}
		}
		return meta
	}
	films := []stremio.Meta{title("tt7001", "movie", "Alpha", "1995", "PG", "Action"), title("tt7002", "movie", "Beta", "1998", "R", "Drama"),
		title("tt7003", "movie", "Gamma", "2005", "PG-13", "Action"), title("tt7004", "movie", "Delta", "1999", "R", "Horror"),
		title("tt7005", "movie", "Zeta", "2001", "", "Drama")}
	show := title("tt7100", "series", "Show", "1996", "TV-14", "Action")
	show.Videos = []stremio.Video{{ID: "tt7100:1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "1996-01-01T00:00:00.000Z"}}
	collection := func(id, name string, sources ...stremio.CollectionSource) stremio.Meta {
		return stremio.Meta{ID: id, Type: "collection", Name: name, Collection: &stremio.Collection{Sources: sources}}
	}
	picks := []stremio.Meta{collection("picks:action", "Action picks", stremio.CollectionSource{Type: "movie", CatalogID: "films", Genre: "Action"}),
		collection("picks:shows", "Show picks", stremio.CollectionSource{Type: "series", CatalogID: "shows"})}
	genre := func(options ...string) []stremio.Extra { return []stremio.Extra{{Name: "genre", Options: options}} }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		send := func(key string, value any) { _ = json.NewEncoder(w).Encode(map[string]any{key: value}) }
		// narrowed are the metas of a catalog its genre option keeps.
		narrowed := func(metas []stremio.Meta) []stremio.Meta {
			_, option, ok := strings.Cut(path, "/genre=")
			if !ok {
				return metas
			}
			option, _ = url.PathUnescape(option)
			return slices.DeleteFunc(slices.Clone(metas), func(m stremio.Meta) bool { return !slices.Contains(m.Genres, option) })
		}
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "filters", Name: "Filters", Version: "1",
				Types: []string{"movie", "series", "collection"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{
					{Type: "movie", ID: "films", Name: "Films", Extra: genre("Action", "Drama", "Horror")},
					{Type: "series", ID: "shows", Name: "Shows", Extra: genre("Action")},
					{Type: "collection", ID: "picks", Name: "Picks", Extra: genre("Franchises")},
				}})
		case strings.HasPrefix(path, "/catalog/movie/films"):
			send("metas", narrowed(films))
		case strings.HasPrefix(path, "/catalog/series/shows"):
			send("metas", narrowed([]stremio.Meta{show}))
		case strings.HasPrefix(path, "/catalog/collection/picks"):
			send("metas", picks)
		case strings.HasPrefix(path, "/meta/"):
			for _, meta := range append(append(slices.Clone(films), show), picks...) {
				if id, _ := url.PathUnescape(path[strings.LastIndex(path, "/")+1:]); id == meta.ID {
					send("meta", meta)
					return
				}
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// filtering is a server of the filters addon's three libraries, with a
// member signed in from Silo's apps and from a Jellyfin app.
type filtering struct {
	siloApps
	member                  accounts.User
	token, profile, account string
	// numbers are the libraries' numbers in Silo's apps, views their
	// identifiers in the Jellyfin API, by name.
	numbers, views map[string]string
}

func newFiltering(t *testing.T) filtering {
	t.Helper()
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), filtersAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "films"},
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"},
		{AddonID: addon.ID, CatalogType: "collection", CatalogID: "picks"},
	}); err != nil {
		t.Fatal(err)
	}
	allowSilo(t, s)
	f := filtering{siloApps: servingSilo(t, s), member: member, numbers: map[string]string{}, views: map[string]string{}}
	f.token, f.profile = f.signIn(t, "member")
	f.account = s.signIn("member", "tv")
	for _, library := range f.libraries(t, f.token, f.profile) {
		f.numbers[library.Name] = library.ID
	}
	var views QueryResult
	s.get(t, "/UserViews", f.account, &views)
	for _, view := range views.Items {
		f.views[view.Name] = view.Id
	}
	return f
}

type siloFilterOptions struct {
	Genres         []string `json:"genres"`
	ContentRatings []string `json:"content_ratings"`
}

// options reads the values of a filter sheet asked with q.
func (f filtering) options(t *testing.T, q url.Values) siloFilterOptions {
	t.Helper()
	var options siloFilterOptions
	f.get(t, "/api/v2/catalog/filters?"+q.Encode(), f.token, f.profile, &options)
	return options
}

// apple pages through the titles of a library the Apple apps ask for
// facets, following its cursors.
func (f filtering) apple(t *testing.T, library string, facets silotest.Facets, limit int) []string {
	t.Helper()
	q := silotest.AppleCatalog(library, facets, "title", limit)
	var titles []string
	for range 10 {
		var page siloPage
		f.get(t, "/api/v2/catalog?"+q.Encode(), f.token, f.profile, &page)
		titles = append(titles, cardTitles(page.Items)...)
		if !page.Page.HasMore {
			return titles
		}
		q.Set("cursor", page.Page.NextCursor)
	}
	t.Fatalf("%s pages on: %v", q.Encode(), titles)
	return nil
}

// android pages through the titles the Android apps ask with body,
// following its cursors.
func (f filtering) android(t *testing.T, body map[string]any) []string {
	t.Helper()
	var titles []string
	for range 10 {
		status, data := f.do(t, http.MethodPost, "/api/v2/catalog/query", f.token, f.profile, body)
		var page siloPage
		if status != http.StatusOK || json.Unmarshal(data, &page) != nil {
			t.Fatalf("%v: %d %s", body, status, data)
		}
		if err := silotest.CheckCatalogPage(data); err != nil {
			t.Errorf("%v: %v: %s", body, err, data)
		}
		titles = append(titles, cardTitles(page.Items)...)
		if !page.Page.HasMore {
			return titles
		}
		body["cursor"] = page.Page.NextCursor
	}
	t.Fatalf("%v pages on: %v", body, titles)
	return nil
}

// A filter sheet offers the genres a library's catalog offers, and the
// content ratings of its titles; a library of collections, the genres of
// its titles rather than those that choose its collections; the catalog,
// the genres of the libraries of titles, which it can be narrowed to; a
// list of the user, the genres of its titles. Home's sections offer none.
func TestSiloAppsFilterOptions(t *testing.T) {
	t.Parallel()
	f := newFiltering(t)
	if status, _ := f.call(http.MethodPost, "/UserFavoriteItems/"+itemIDOf(t, f.testServer, f.account, f.views["Films"], "Gamma"),
		app("tv", f.account), nil); status != http.StatusOK {
		t.Fatalf("favorite: %d", status)
	}
	for _, tc := range []struct {
		name            string
		q               url.Values
		genres, ratings []string
	}{
		{"Films, as Apple asks", silotest.AppleFilters(f.numbers["Films"], false), []string{"Action", "Drama", "Horror"}, []string{"PG", "PG-13", "R"}},
		{"Films, as Android asks", silotest.AndroidFilters(f.numbers["Films"], "", ""), []string{"Action", "Drama", "Horror"}, []string{"PG", "PG-13", "R"}},
		{"Shows", silotest.AppleFilters(f.numbers["Shows"], true), []string{"Action"}, []string{"TV-14"}},
		{"Picks, of collections", silotest.AppleFilters(f.numbers["Picks"], false), []string{"Action"}, []string{"PG", "PG-13", "TV-14"}},
		{"the catalog", silotest.AppleFilters("", false), []string{"Action", "Drama", "Horror"}, []string{}},
		{"the favorites", silotest.AndroidFilters("", "favorites", ""), []string{"Action"}, []string{"PG-13"}},
		{"a section", url.Values{"source": {"section"}, "section_id": {"continue_watching"}}, []string{}, []string{}},
	} {
		options := f.options(t, tc.q)
		if !slices.Equal(options.Genres, tc.genres) || !slices.Equal(options.ContentRatings, tc.ratings) {
			t.Errorf("%s: %+v, want %v and %v", tc.name, options, tc.genres, tc.ratings)
		}
	}
}

// The catalog keeps the titles that match the facets the apps send, page
// by page, two or more together, all of them or any, in a library of
// titles, in a library of collections and across every library; it
// refuses a facet it cannot tell of any title.
func TestSiloAppsFilterAPage(t *testing.T) {
	t.Parallel()
	f := newFiltering(t)
	films, picks := f.numbers["Films"], f.numbers["Picks"]
	for name, path := range map[string]string{"played": "/UserPlayedItems/", "favorite": "/UserFavoriteItems/"} {
		title := map[string]string{"played": "Beta", "favorite": "Gamma"}[name]
		if status, _ := f.call(http.MethodPost, path+itemIDOf(t, f.testServer, f.account, f.views["Films"], title), app("tv", f.account), nil); status != http.StatusOK {
			t.Fatalf("%s: %d", name, status)
		}
	}
	// Alpha is half watched, Zeta on the watchlist.
	if status, _ := f.call(http.MethodPost, "/UserItems/"+itemIDOf(t, f.testServer, f.account, f.views["Films"], "Alpha")+"/UserData",
		app("tv", f.account), map[string]any{"PlaybackPositionTicks": 27_000_000_000}); status != http.StatusOK {
		t.Fatalf("a resume point: %d", status)
	}
	if status, data := f.do(t, http.MethodPut, "/api/v2/watchlist/"+itemIDOf(t, f.testServer, f.account, f.views["Films"], "Zeta"),
		f.token, f.profile, nil); status/100 != 2 {
		t.Fatalf("on the watchlist: %d %s", status, data)
	}
	// A genre narrows a page: Action's two titles, one a page.
	q := silotest.AppleCatalog(films, silotest.Facets{Genres: []string{"Action"}}, "title", 1)
	var first siloPage
	f.get(t, "/api/v2/catalog?"+q.Encode(), f.token, f.profile, &first)
	if !slices.Equal(cardTitles(first.Items), []string{"Alpha"}) || !first.Page.HasMore {
		t.Errorf("Action's first page: %+v", first)
	}
	for _, tc := range []struct {
		name   string
		titles []string
		want   []string
	}{
		{"Action, a title a page", f.apple(t, films, silotest.Facets{Genres: []string{"Action"}}, 1), []string{"Alpha", "Gamma"}},
		{"Action of the 1990s", f.apple(t, films, silotest.Facets{Genres: []string{"Action"}, Decades: []int{1990}}, 50), []string{"Alpha"}},
		{"Action or Horror of the 1990s", f.apple(t, films, silotest.Facets{Genres: []string{"Action", "Horror"}, Decades: []int{1990}}, 1),
			[]string{"Alpha", "Delta"}},
		{"Horror or the 2000s", f.apple(t, films, silotest.Facets{Genres: []string{"Horror"}, Decades: []int{2000}, Any: true}, 50),
			[]string{"Delta", "Gamma", "Zeta"}},
		{"favorited, as Apple asks", f.apple(t, films, silotest.Facets{WatchStatus: "favorited"}, 50), []string{"Gamma"}},
		{"Drama of the 1990s, as Android asks", f.android(t, silotest.AndroidCatalog(films, silotest.Facets{Genres: []string{"Drama"},
			Decades: []int{1990}}, "added_at", "desc", 50)), []string{"Beta"}},
		{"watched", f.android(t, silotest.AndroidCatalog(films, silotest.Facets{WatchStatus: "watched"}, "", "", 50)), []string{"Beta"}},
		{"Drama unwatched", f.android(t, silotest.AndroidCatalog(films, silotest.Facets{Genres: []string{"Drama"}, WatchStatus: "unwatched"},
			"", "", 1)), []string{"Zeta"}},
		{"rated R", f.android(t, silotest.AndroidCatalog(films, silotest.Facets{ContentRatings: []string{"R"}}, "", "", 1)), []string{"Beta", "Delta"}},
		{"on the watchlist", f.apple(t, films, silotest.Facets{WatchStatus: "watchlist"}, 50), []string{"Zeta"}},
		{"in progress", f.android(t, silotest.AndroidCatalog(films, silotest.Facets{WatchStatus: "in_progress"}, "", "", 50)), []string{"Alpha"}},
		{"in progress or Horror", f.apple(t, films, silotest.Facets{Genres: []string{"Horror"}, WatchStatus: "in_progress", Any: true}, 50),
			[]string{"Alpha", "Delta"}},
		{"Picks' Action of the 1990s", f.apple(t, picks, silotest.Facets{Genres: []string{"Action"}, Decades: []int{1990}}, 50),
			[]string{"Alpha", "Show"}},
		{"Picks' movies", f.apple(t, picks, silotest.Facets{Type: "movie"}, 50), []string{"Alpha", "Gamma"}},
		{"Picks' favorites", f.android(t, silotest.AndroidCatalog(picks, silotest.Facets{WatchStatus: "favorited"}, "", "", 50)), []string{"Gamma"}},
		{"every library's Action", f.android(t, silotest.AndroidBrowse("Action", "", "", "", 50)), []string{"Alpha", "Gamma", "Show"}},
		{"every library's Action rated PG", f.android(t, silotest.AndroidBrowse("Action", "PG", "", "", 50)), []string{"Alpha"}},
	} {
		if !slices.Equal(sorted(tc.titles...), tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, tc.titles, tc.want)
		}
	}
	status, data := f.do(t, http.MethodGet, "/api/v2/catalog?"+silotest.AppleCatalog(films, silotest.Facets{HDR: true}, "title", 50).Encode(),
		f.token, f.profile, nil)
	if status != http.StatusUnprocessableEntity || !strings.Contains(string(data), `"code":"unsupported"`) {
		t.Errorf("HDR: %d %s", status, data)
	}
}

// Filters keep to the user's limits: a genre they may not see is not
// offered, and keeps nothing, a title they hid leaves the titles kept,
// and a library they hid offers nothing, in its sheet or in the
// catalog's.
func TestSiloAppsFiltersUnderTheUsersLimits(t *testing.T) {
	t.Parallel()
	f := newFiltering(t)
	films := f.numbers["Films"]
	f.limit(t, f.member, func(c *accounts.UserChanges) { c.BlockedGenres = &[]string{"horror"} })
	for name, q := range map[string]url.Values{"Films": silotest.AppleFilters(films, false), "the catalog": silotest.AppleFilters("", false)} {
		if options := f.options(t, q); !slices.Equal(options.Genres, []string{"Action", "Drama"}) {
			t.Errorf("%s with Horror blocked: %+v", name, options)
		}
	}
	if titles := f.apple(t, films, silotest.Facets{Genres: []string{"Horror"}}, 50); len(titles) != 0 {
		t.Errorf("Horror while blocked: %v", titles)
	}
	if titles := f.apple(t, films, silotest.Facets{Decades: []int{1990}}, 50); !slices.Equal(sorted(titles...), []string{"Alpha", "Beta"}) {
		t.Errorf("the 1990s with Horror blocked: %v", titles)
	}
	if status, _ := f.call(http.MethodPost, "/Polyfin/UserItems/"+itemIDOf(t, f.testServer, f.account, f.views["Films"], "Gamma")+"/Hidden",
		app("tv", f.account), nil); status != http.StatusNoContent && status != http.StatusOK {
		t.Fatalf("hiding: %d", status)
	}
	if titles := f.apple(t, films, silotest.Facets{Genres: []string{"Action"}}, 50); !slices.Equal(titles, []string{"Alpha"}) {
		t.Errorf("Action with Gamma hidden: %v", titles)
	}
	view, _ := accounts.ParseID(f.views["Films"])
	f.limit(t, f.member, func(c *accounts.UserChanges) {
		c.BlockedGenres = &[]string{}
		c.HiddenLibraries = &[]accounts.ID{view}
	})
	if status, data := f.do(t, http.MethodGet, "/api/v2/catalog/filters?"+silotest.AppleFilters(films, false).Encode(), f.token, f.profile, nil); status != http.StatusNotFound {
		t.Errorf("a hidden library's options: %d %s", status, data)
	}
	if status, data := f.do(t, http.MethodPost, "/api/v2/catalog/query", f.token, f.profile,
		silotest.AndroidCatalog(films, silotest.Facets{Genres: []string{"Drama"}}, "", "", 50)); status != http.StatusNotFound {
		t.Errorf("a hidden library filtered: %d %s", status, data)
	}
	if options := f.options(t, silotest.AppleFilters("", false)); !slices.Equal(options.Genres, []string{"Action"}) {
		t.Errorf("the catalog with Films hidden: %+v", options)
	}
}
