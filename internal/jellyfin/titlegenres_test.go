package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// genrelessAddon serves a movie catalog with no genre filter, as lists a
// user keeps are, whose movies have genres, and a series catalog whose
// genre filter offers Action only, whose series also have others. It
// refuses a genre its series catalog does not offer.
func genrelessAddon(t *testing.T) string {
	t.Helper()
	movies := []stremio.Meta{}
	for i, genres := range [][]string{{"Drama"}, {"Comedy", "drama"}, {"Sci-Fi"}, {"Comedy"}, {"Drama", "Comedy"}, nil} {
		movies = append(movies, stremio.Meta{ID: fmt.Sprintf("tt100%d", i), Type: "movie", Name: fmt.Sprintf("Movie %c", 'A'+i),
			Genres: genres, ReleaseInfo: "2020"})
	}
	series := []stremio.Meta{
		{ID: "tt2000", Type: "series", Name: "Show A", Genres: []string{"Action"}, ReleaseInfo: "2020-"},
		{ID: "tt2001", Type: "series", Name: "Show B", Genres: []string{"Action", "Sci-Fi"}, ReleaseInfo: "2020-"},
		{ID: "tt2002", Type: "series", Name: "Show C", Genres: []string{"Sci-Fi"}, ReleaseInfo: "2020-"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "genreless", Name: "Genreless", Version: "1", Types: []string{"movie", "series"},
				Resources: []stremio.Resource{{Name: "catalog"}},
				Catalogs: []stremio.Catalog{
					{Type: "movie", ID: "list", Name: "List"},
					{Type: "series", ID: "shows", Name: "Shows", Extra: []stremio.Extra{{Name: "genre", Options: []string{"Action"}}}},
				}})
		case strings.HasPrefix(path, "/catalog/movie/list"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": movies})
		case strings.HasPrefix(path, "/catalog/series/shows"):
			_, genre, _ := strings.Cut(strings.TrimSuffix(path, ".json"), "genre=")
			genre, _, _ = strings.Cut(genre, "&")
			genre, _ = url.PathUnescape(genre)
			switch genre {
			case "":
				_ = json.NewEncoder(w).Encode(map[string]any{"metas": series})
			case "Action":
				_ = json.NewEncoder(w).Encode(map[string]any{"metas": series[:2]})
			default:
				http.Error(w, "no such genre", http.StatusBadRequest)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// A library whose catalog offers no genre filter lists the genres of its
// titles, in its Genres tab and its filter menu, and is narrowed to them
// by its titles' own genres, by name or identifier, any of them, sorted
// as asked, as Jellyfin narrows a library. A genre its catalog offers is
// still asked of its addon; one it does not offer narrows its titles.
func TestALibraryWithoutGenreFilterIsNarrowedByItsTitlesGenres(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), genrelessAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "list"},
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"},
	}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	ids := map[string]string{}
	for _, view := range views.Items {
		ids[view.Name] = view.Id
	}
	list, shows := ids["List"], ids["Shows"]

	// The filter menu and the Genres tab offer the movies' genres, each
	// once, by name.
	var filters struct{ Genres []string }
	s.get(t, "/Items/Filters?parentId="+list+"&includeItemTypes=Movie", token, &filters)
	if want := []string{"Comedy", "Drama", "Sci-Fi"}; !slices.Equal(filters.Genres, want) {
		t.Errorf("filters: %v, want %v", filters.Genres, want)
	}
	var pairs struct{ Genres []NameGuidPair }
	s.get(t, "/Items/Filters2?parentId="+list+"&includeItemTypes=Movie", token, &pairs)
	if len(pairs.Genres) != 3 || pairs.Genres[1].Name != "Drama" || pairs.Genres[1].Id != nameID("genre", "Drama") {
		t.Errorf("filters2: %+v", pairs.Genres)
	}
	var genres QueryResult
	s.get(t, "/Genres?parentId="+list+"&includeItemTypes=Movie&recursive=true", token, &genres)
	drama := ""
	for _, genre := range genres.Items {
		if genre.Name == "Drama" {
			drama = genre.Id
		}
	}
	if got := itemNames(genres.Items); !slices.Equal(got, []string{"Comedy", "Drama", "Sci-Fi"}) || drama == "" {
		t.Errorf("genres tab: %v", got)
	}

	titles := "/Items?parentId=" + list + "&recursive=true&includeItemTypes=Movie&"
	for query, want := range map[string][]string{
		// The filter menu's genres, in the catalog's order, then sorted.
		"genres=Comedy": {"Movie B", "Movie D", "Movie E"},
		"genres=comedy&sortBy=SortName&sortOrder=Descending": {"Movie E", "Movie D", "Movie B"},
		"genres=Comedy%7CSci-Fi":                             {"Movie B", "Movie C", "Movie D", "Movie E"},
		// A row of the Genres tab, by identifier, in any order.
		"genreIds=" + drama + "&sortBy=Random&limit=25": {"Movie A", "Movie B", "Movie E"},
		"genres=Comedy&startIndex=1&limit=1":            {"Movie D"},
		"genres=Western":                                nil,
		// Studios and years are not genres: nothing narrows to them.
		"genres=Comedy&years=2020": nil,
	} {
		var page QueryResult
		if status := s.get(t, titles+query, token, &page); status != http.StatusOK {
			t.Errorf("%s: %d", query, status)
			continue
		}
		got := itemNames(page.Items)
		if strings.Contains(query, "Random") {
			slices.Sort(got)
		}
		total := len(want)
		if strings.Contains(query, "limit=1") {
			total = 3
		}
		if !slices.Equal(got, want) || page.TotalRecordCount != total {
			t.Errorf("%s: %v (total %d), want %v (total %d)", query, got, page.TotalRecordCount, want, total)
		}
	}

	// The series catalog offers Action: its filter menu offers it alone,
	// and its addon narrows it; Sci-Fi, which it does not offer, narrows
	// its series by their own genres.
	s.get(t, "/Items/Filters?parentId="+shows+"&includeItemTypes=Series", token, &filters)
	if !slices.Equal(filters.Genres, []string{"Action"}) {
		t.Errorf("series filters: %v", filters.Genres)
	}
	for query, want := range map[string][]string{
		"genres=Action":                         {"Show A", "Show B"},
		"genres=Sci-Fi":                         {"Show B", "Show C"},
		"genreIds=" + nameID("genre", "sci-fi"): {"Show B", "Show C"},
	} {
		var page QueryResult
		s.get(t, "/Items?parentId="+shows+"&recursive=true&includeItemTypes=Series&"+query, token, &page)
		if got := itemNames(page.Items); !slices.Equal(got, want) || page.TotalRecordCount != len(want) {
			t.Errorf("series %s: %v (total %d), want %v", query, got, page.TotalRecordCount, want)
		}
	}
}
