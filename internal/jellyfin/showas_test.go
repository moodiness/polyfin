package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// newPicksAddon serves an addon of lists of movies and shows: a catalog of
// a type Polyfin has no use of its own for, Picks, of two movies and a
// series, each typed as what it is, and a movie catalog, Mixed Movies,
// that also lists that series. It describes each title under its own type
// only, the series with two episodes.
func newPicksAddon(t *testing.T) string {
	t.Helper()
	harbor := stremio.Meta{ID: "tt1000001", Type: "movie", Name: "Harbor Lights"}
	gull := stremio.Meta{ID: "tt1000002", Type: "movie", Name: "Gull Point"}
	ridge := stremio.Meta{ID: "tt2000001", Type: "series", Name: "Ridge Show"}
	catalogs := map[string][]stremio.Meta{"lists/picks": {harbor, gull, ridge}, "movie/mixed": {harbor, ridge}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "org.example.lists", Name: "Lists", Version: "1",
				Types: []string{"movie", "series", "lists"},
				Resources: []stremio.Resource{{Name: "catalog"},
					{Name: "meta", Types: []string{"movie", "series"}, IDPrefixes: []string{"tt"}}},
				Catalogs: []stremio.Catalog{
					{Type: "lists", ID: "picks", Name: "Picks", Extra: []stremio.Extra{{Name: "skip"}}},
					{Type: "movie", ID: "mixed", Name: "Mixed Movies", Extra: []stremio.Extra{{Name: "skip"}}}}})
		case strings.HasPrefix(path, "/catalog/"):
			var metas []stremio.Meta
			if !strings.Contains(path, "skip=") {
				for key, listed := range catalogs {
					if strings.HasPrefix(path, "/catalog/"+key) {
						metas = listed
					}
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case path == "/meta/movie/tt1000001.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": harbor})
		case path == "/meta/movie/tt1000002.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": gull})
		case path == "/meta/series/tt2000001.json":
			meta := ridge
			meta.Videos = []stremio.Video{{ID: "tt2000001:1:1", Title: "One", Season: 1, Episode: 1},
				{ID: "tt2000001:1:2", Title: "Two", Season: 1, Episode: 2}}
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// listsLibraries is a server with the lists addon's two catalogs as
// libraries, and a member browsing them.
type listsLibraries struct {
	tracking
	// picks and mixed are its catalogs as library choices.
	picks, mixed addons.LibraryChoice
}

func newListsLibraries(t *testing.T) listsLibraries {
	t.Helper()
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), newPicksAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	return listsLibraries{tracking: tracking{testServer: s, token: s.signIn("member", "tv"), user: member.ID.String()},
		picks: addons.LibraryChoice{AddonID: addon.ID, CatalogType: "lists", CatalogID: "picks"},
		mixed: addons.LibraryChoice{AddonID: addon.ID, CatalogType: "movie", CatalogID: "mixed"}}
}

// set makes the libraries of choices, as the administration's page saves
// them, users' data following the libraries that switch.
func (p listsLibraries) set(t *testing.T, choices ...addons.LibraryChoice) {
	t.Helper()
	before, err := p.addons.Libraries(t.Context(), addons.Shared())
	if err != nil {
		t.Fatal(err)
	}
	after, err := p.addons.SetLibraries(t.Context(), addons.Shared(), choices)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.library.FollowTitles(t.Context(), before, after); err != nil {
		t.Fatal(err)
	}
}

// views returns the member's libraries by name.
func (p listsLibraries) views(t *testing.T) map[string]BaseItemDto {
	t.Helper()
	var views QueryResult
	p.get(t, "/UserViews", p.token, &views)
	byName := map[string]BaseItemDto{}
	for _, view := range views.Items {
		byName[view.Name] = view
	}
	return byName
}

// foldersTab lists a mixed library's Folders tab as jellyfin-web 12.4
// asks for it (hooks/useFetchItems.ts, LibraryTab.Folders, with the item
// types of constants/views/mixed.ts and its default page of 100): its
// titles, not recursively, by name, in the catalog's order, which Polyfin
// keeps, checking the total.
func (p listsLibraries) foldersTab(t *testing.T, view string, want int) []string {
	t.Helper()
	var page QueryResult
	path := "/Items?ParentId=" + view + "&IncludeItemTypes=Folder,Movie,Series&SortBy=SortName&SortOrder=Ascending&StartIndex=0&Limit=100"
	if status := p.get(t, path, p.token, &page); status != http.StatusOK {
		t.Fatalf("%s: %d", path, status)
	}
	if page.TotalRecordCount != want {
		t.Errorf("%s: a total of %d, want %d", path, page.TotalRecordCount, want)
	}
	names := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		names = append(names, item.Name)
	}
	return names
}

// A library of a catalog of a type Polyfin has no use of its own for,
// whose titles are movies and series by their own types, is shown as
// movies and shows unless one chose otherwise, once Polyfin read its first
// page: jellyfin-web shows its tabs as a mixed library's, which list every
// title, with exact totals. As home videos it showed none of them.
func TestListsOfMoviesAndShowsShowAsMoviesAndShows(t *testing.T) {
	t.Parallel()
	p := newListsLibraries(t)
	p.set(t, p.picks)
	if err := p.library.Warm(t.Context()); err != nil {
		t.Fatal(err)
	}
	picks, ok := p.views(t)["Picks"]
	if !ok {
		t.Fatal("no Picks library")
	}
	if picks.CollectionType != "" {
		t.Errorf("Picks is a %q library", picks.CollectionType)
	}
	if got, want := p.foldersTab(t, picks.Id, 3), []string{"Harbor Lights", "Gull Point", "Ridge Show"}; !slices.Equal(got, want) {
		t.Errorf("the Folders tab: %v, want %v", got, want)
	}
	checkNarrowedListings(t, p.testServer, p.token, picks.Id, "Movie,Series", map[string][]string{
		"Movie":        {"Harbor Lights", "Gull Point"},
		"Series":       {"Ridge Show"},
		"Movie,Series": {"Harbor Lights", "Gull Point", "Ridge Show"},
	})
}

// A movie catalog that also lists a series, shown as movies and shows,
// has no content type, as a mixed library, and lists each title as its
// own type tells: its tabs show the series too, and each type's total
// counts that type alone. Listed as movies, its tabs never showed the
// series.
func TestMovieCatalogShownAsMoviesAndShows(t *testing.T) {
	t.Parallel()
	p := newListsLibraries(t)
	mixed := p.mixed
	mixed.ShowAs = addons.ShowAsMoviesAndShows
	p.set(t, mixed)
	view, ok := p.views(t)["Mixed Movies"]
	if !ok || view.CollectionType != "" {
		t.Fatalf("Mixed Movies: %+v", view)
	}
	if got, want := p.foldersTab(t, view.Id, 2), []string{"Harbor Lights", "Ridge Show"}; !slices.Equal(got, want) {
		t.Errorf("the Folders tab: %v, want %v", got, want)
	}
	checkNarrowedListings(t, p.testServer, p.token, view.Id, "", map[string][]string{
		"Movie":        {"Harbor Lights"},
		"Series":       {"Ridge Show"},
		"Movie,Series": {"Harbor Lights", "Ridge Show"},
	})
	var page QueryResult
	p.get(t, "/Items?ParentId="+view.Id+"&Recursive=true&IncludeItemTypes=Series", p.token, &page)
	if len(page.Items) != 1 {
		t.Fatalf("the series: %+v", page.Items)
	}
	if places, _ := (vaultLibrary{tracking: p.tracking}).episodes(t, page.Items[0].Id, ""); !slices.Equal(places, []string{"1.1 One", "1.2 Two"}) {
		t.Errorf("the series' episodes: %v", places)
	}
}

// Switching a movie catalog between listed and movies and shows changes
// none of its items: played marks and favorites stay where they are.
func TestShowingAMovieCatalogMixedKeepsWatchState(t *testing.T) {
	t.Parallel()
	p := newListsLibraries(t)
	listed, mixed := p.mixed, p.mixed
	listed.ShowAs, mixed.ShowAs = addons.ShowAsListed, addons.ShowAsMoviesAndShows
	p.set(t, listed)
	view := p.views(t)["Mixed Movies"].Id
	titles := func() map[string]BaseItemDto {
		t.Helper()
		var page QueryResult
		p.get(t, "/Items?ParentId="+view+"&Recursive=true&IncludeItemTypes=Movie,Series", p.token, &page)
		byName := map[string]BaseItemDto{}
		for _, item := range page.Items {
			byName[item.Name] = item
		}
		return byName
	}
	before := titles()
	movie, series := before["Harbor Lights"], before["Ridge Show"]
	if movie.Type != "Movie" || series.Type != "Series" {
		t.Fatalf("the titles: %+v", before)
	}
	_, episodes := (vaultLibrary{tracking: p.tracking}).episodes(t, series.Id, "")
	if len(episodes) != 2 {
		t.Fatalf("the episodes: %v", episodes)
	}
	p.mark(t, http.MethodPost, "/UserPlayedItems/"+movie.Id)
	p.mark(t, http.MethodPost, "/UserFavoriteItems/"+series.Id)
	p.mark(t, http.MethodPost, "/UserPlayedItems/"+episodes[0])

	for _, choice := range []addons.LibraryChoice{mixed, listed} {
		p.set(t, choice)
		after := titles()
		if got := after["Harbor Lights"]; got.Id != movie.Id || !got.UserData.Played {
			t.Errorf("%s: the movie: %+v", choice.ShowAs, got)
		}
		if got := after["Ridge Show"]; got.Id != series.Id || !got.UserData.IsFavorite {
			t.Errorf("%s: the series: %+v", choice.ShowAs, got)
		}
		if data := p.userData(t, episodes[0]); !data.Played {
			t.Errorf("%s: the first episode: %+v", choice.ShowAs, data)
		}
		if got := p.list(t, "/Shows/NextUp?UserId="+p.user); !slices.Equal(got, episodes[1:]) {
			t.Errorf("%s: Next Up: %v", choice.ShowAs, got)
		}
	}
}
