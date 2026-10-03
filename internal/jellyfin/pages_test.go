package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// pagesAddon serves movie and series catalogs that its genre extra narrows
// to genres, studios or years, and a movie catalog that cannot be
// narrowed.
func pagesAddon(t *testing.T) string {
	t.Helper()
	// Artwork is only described, never fetched, in these tests.
	title := func(id, metaType, name, year string, genres ...string) stremio.Meta {
		return stremio.Meta{ID: id, Type: metaType, Name: name, Genres: genres, Poster: "https://artwork.test/" + id + ".jpg",
			Background: "https://artwork.test/backdrop.jpg", ReleaseInfo: stremio.Text(year), Released: year + "-04-10T00:00:00.000Z",
			Runtime: "1h 30min", ImdbRating: "7.5", Extras: &stremio.Extras{Certification: "PG"}}
	}
	first := title("tt1001", "movie", "Movie 1", "2020", "Action")
	second := title("tt1002", "movie", "Movie 2", "2021", "Drama")
	third := title("tt1003", "movie", "Movie 3", "2021", "Action")
	unfiltered := title("tt1004", "movie", "Movie 4", "2020", "Action")
	show := title("tt2001", "series", "Show 1", "2020", "Action")
	// The titles each catalog lists, by the genre option it is narrowed to.
	listed := map[string]map[string][]stremio.Meta{
		"popular": {"": {first, second, third}, "Action": {first, third}, "Drama": {second}},
		"studios": {"": {first, second}, "North Studio": {first}, "South Studio": {second}},
		"yearly":  {"2020": {first}, "2021": {second, third}},
		"shows":   {"": {show}, "Action": {show}, "North Studio": {show}},
		"plain":   {"": {unfiltered}},
	}
	genre := func(options ...string) []stremio.Extra { return []stremio.Extra{{Name: "genre", Options: options}} }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "pages", Name: "Pages", Version: "1", Types: []string{"movie", "series"},
				Resources: []stremio.Resource{{Name: "catalog"}},
				Catalogs: []stremio.Catalog{
					{Type: "movie", ID: "popular", Name: "Popular", Extra: genre("Action", "Drama")},
					{Type: "movie", ID: "studios", Name: "Studios", Extra: genre("North Studio", "South Studio")},
					{Type: "movie", ID: "yearly", Name: "Yearly", Extra: []stremio.Extra{{Name: "genre", IsRequired: true, Options: []string{"2020", "2021"}}}},
					{Type: "series", ID: "shows", Name: "Shows", Extra: genre("Action", "North Studio")},
					{Type: "movie", ID: "plain", Name: "Plain"},
				}})
		case strings.HasPrefix(path, "/catalog/"):
			segments := strings.Split(strings.TrimPrefix(path, "/catalog/"), "/")
			option := ""
			if len(segments) == 3 {
				option, _ = url.PathUnescape(strings.TrimPrefix(segments[2], "genre="))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": listed[segments[1]][option]})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// pagesServer installs the pages addon with all its catalogs as libraries,
// and signs a member in.
func pagesServer(t *testing.T) (s testServer, token, user string, views map[string]string) {
	t.Helper()
	return pagesServerWith(t, pagesAddon(t), "")
}

// pagesServerWith installs an addon with its catalogs of type only (all
// when empty) as libraries, and signs a member in.
func pagesServerWith(t *testing.T, manifestURL, only string) (s testServer, token, user string, views map[string]string) {
	t.Helper()
	s = newTestServer(t, 10)
	member := s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), manifestURL, false)
	if err != nil {
		t.Fatal(err)
	}
	var choices []addons.LibraryChoice
	for _, catalog := range addon.Manifest.Catalogs {
		if only == "" || catalog.Type == only {
			choices = append(choices, addons.LibraryChoice{AddonID: addon.ID, CatalogType: catalog.Type, CatalogID: catalog.ID})
		}
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), choices); err != nil {
		t.Fatal(err)
	}
	token = s.signIn("member", "tv")
	var result QueryResult
	s.get(t, "/UserViews", token, &result)
	views = map[string]string{}
	for _, view := range result.Items {
		views[view.Name] = view.Id
	}
	return s, token, member.ID.String(), views
}

// namedPageOf opens the page at path, which must succeed.
func namedPageOf(t *testing.T, s testServer, token, path string) namedItemDto {
	t.Helper()
	status, body := s.call(http.MethodGet, path, app("tv", token), nil)
	var page namedItemDto
	if status != http.StatusOK {
		t.Fatalf("%s: %d %s", path, status, body)
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("%s: %v in %s", path, err, body)
	}
	return page
}

// listed returns the sorted names a listing returns, and its total.
func listed(t *testing.T, s testServer, token, path string) ([]string, int) {
	t.Helper()
	var page QueryResult
	if status := s.get(t, path, token, &page); status != http.StatusOK {
		t.Fatalf("%s: %d", path, status)
	}
	names := itemNames(page.Items)
	slices.Sort(names)
	return names, page.TotalRecordCount
}

const titlesOf = "/Items?recursive=true&includeItemTypes=Movie,Series&"

func TestGenreStudioAndYearPages(t *testing.T) {
	s, token, user, views := pagesServer(t)

	// The genre a title carries, and the one /Genres lists, open the same
	// page.
	var movies QueryResult
	s.get(t, "/Items?parentId="+views["Popular"], token, &movies)
	var movie BaseItemDto
	s.get(t, "/Items/"+movies.Items[0].Id, token, &movie)
	if movie.GenreItems == nil || len(*movie.GenreItems) != 1 || (*movie.GenreItems)[0].Name != "Action" {
		t.Fatalf("genres of %s: %+v", movie.Name, movie.GenreItems)
	}
	carried := (*movie.GenreItems)[0].Id
	var genres QueryResult
	s.get(t, "/Genres?parentId="+views["Popular"], token, &genres)
	if len(genres.Items) == 0 || genres.Items[0].Name != "Action" || genres.Items[0].Id != carried {
		t.Fatalf("genres of the library: %+v", genres.Items)
	}
	genre := namedPageOf(t, s, token, "/Genres/Action?userId="+user)
	if genre.Type != "Genre" || genre.Name != "Action" || genre.Id != carried || genre.UserData == nil ||
		genre.UserData.ItemId != carried || genre.MovieCount != 2 || genre.SeriesCount != 1 {
		t.Errorf("genre page: %+v", genre)
	}
	if other := namedPageOf(t, s, token, "/Genres/action"); other.Id != carried {
		t.Errorf("a genre named in another letter case: %s, want %s", other.Id, carried)
	}

	studio := namedPageOf(t, s, token, "/Studios/North%20Studio?userId="+user)
	if studio.Type != "Studio" || studio.Name != "North Studio" || studio.MovieCount != 1 || studio.SeriesCount != 1 {
		t.Errorf("studio page: %+v", studio)
	}
	if names, _ := listed(t, s, token, titlesOf+"studioIds="+studio.Id); !slices.Equal(names, []string{"Movie 1", "Show 1"}) {
		t.Errorf("titles by the studio page's identifier: %v", names)
	}

	year := namedPageOf(t, s, token, "/Years/2021?userId="+user)
	if year.Type != "Year" || year.Name != "2021" || year.SortName != "0000002021" || year.MovieCount != 2 || year.SeriesCount != 0 {
		t.Errorf("year page: %+v", year)
	}
	if again := namedPageOf(t, s, token, "/Years/2021"); again.Id != year.Id || again.Id == genre.Id || again.Id == studio.Id {
		t.Errorf("year identifiers: %s, %s; genre %s, studio %s", year.Id, again.Id, genre.Id, studio.Id)
	}
}

func TestListsByGenreStudioAndYear(t *testing.T) {
	s, token, _, views := pagesServer(t)
	action := namedPageOf(t, s, token, "/Genres/Action").Id
	all := "includeItemTypes=Movie,Series&"
	for _, test := range []struct {
		name, query string
		want        []string
	}{
		// Movie 4 is an Action movie too, but its library's catalog cannot
		// be narrowed to a genre.
		{"genre by identifier", all + "genreIds=" + action, []string{"Movie 1", "Movie 3", "Show 1"}},
		{"genre by hyphenated identifier", all + "genreIds=" + action[:8] + "-" + action[8:12] + "-" + action[12:16] + "-" +
			action[16:20] + "-" + action[20:], []string{"Movie 1", "Movie 3", "Show 1"}},
		{"genre by name", all + "genres=action", []string{"Movie 1", "Movie 3", "Show 1"}},
		{"another genre", all + "genres=Drama", []string{"Movie 2"}},
		{"studio by name", all + "studios=South%20Studio", []string{"Movie 2"}},
		{"year", all + "years=2021", []string{"Movie 2", "Movie 3"}},
		{"movies only", "includeItemTypes=Movie&genres=Action", []string{"Movie 1", "Movie 3"}},
	} {
		names, total := listed(t, s, token, "/Items?recursive=true&"+test.query)
		if !slices.Equal(names, test.want) || total != len(test.want) {
			t.Errorf("%s: %v (total %d), want %v", test.name, names, total, test.want)
		}
	}

	var page QueryResult
	s.get(t, titlesOf+"genreIds="+action+"&startIndex=1&limit=1", token, &page)
	if len(page.Items) != 1 || page.TotalRecordCount != 3 || page.StartIndex != 1 {
		t.Errorf("second page: %v, total %d", itemNames(page.Items), page.TotalRecordCount)
	}

	// A library's listing narrows its own catalog the same way.
	if names, _ := listed(t, s, token, "/Items?parentId="+views["Yearly"]+"&years=2020"); !slices.Equal(names, []string{"Movie 1"}) {
		t.Errorf("a library by year: %v", names)
	}
	if names, _ := listed(t, s, token, "/Items?parentId="+views["Popular"]+"&studios=North%20Studio"); len(names) != 0 {
		t.Errorf("a library whose catalog does not offer the studio: %v", names)
	}
}

// collectionsAddon serves, as AIOMetadata does, collections whose catalogs
// are only reachable through them: one fixed to the Drama option of a
// movie catalog, and one that groups a catalog without genres and a
// nested collection of a series catalog.
func collectionsAddon(t *testing.T) string {
	t.Helper()
	title := func(id, metaType, name string) stremio.Meta {
		return stremio.Meta{ID: id, Type: metaType, Name: name, ReleaseInfo: "2020"}
	}
	listed := map[string]map[string][]stremio.Meta{
		"hidden": {"Action": {title("tt3001", "movie", "Movie A1"), title("tt3002", "movie", "Movie A2")},
			"Drama": {title("tt3003", "movie", "Movie D")}},
		"hiddenshows": {"Action": {title("tt4001", "series", "Show A")}},
		"flat":        {"": {title("tt3004", "movie", "Movie F")}},
		"groups":      {"None": {{ID: "group:drama", Type: "collection", Name: "Drama"}, {ID: "group:outer", Type: "collection", Name: "Outer"}}},
	}
	collections := map[string]stremio.Collection{
		"group:drama": {Sources: []stremio.CollectionSource{{Type: "movie", CatalogID: "hidden", Genre: "Drama"}}},
		"group:outer": {Items: []stremio.Meta{{ID: "group:nested", Type: "collection", Name: "Nested"}},
			Sources: []stremio.CollectionSource{{Type: "movie", CatalogID: "flat"}}},
		"group:nested": {Sources: []stremio.CollectionSource{{Type: "series", CatalogID: "hiddenshows"}}},
	}
	genre := func(required bool, options ...string) []stremio.Extra {
		return []stremio.Extra{{Name: "genre", IsRequired: required, Options: options}}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "collections", Name: "Collections", Version: "1",
				Types: []string{"movie", "series", "collection"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{
					{Type: "collection", ID: "groups", Name: "Groups", Extra: genre(true, "None")},
					{Type: "movie", ID: "hidden", Name: "Hidden", Extra: genre(false, "Action", "Drama")},
					{Type: "series", ID: "hiddenshows", Name: "Hidden shows", Extra: genre(true, "Action")},
					{Type: "movie", ID: "flat", Name: "Flat"},
				}})
		case strings.HasPrefix(path, "/catalog/"):
			segments := strings.Split(strings.TrimPrefix(path, "/catalog/"), "/")
			option := ""
			if len(segments) == 3 {
				option, _ = url.PathUnescape(strings.TrimPrefix(segments[2], "genre="))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": listed[segments[1]][option]})
		case strings.HasPrefix(path, "/meta/collection/"):
			id, _ := url.PathUnescape(strings.TrimPrefix(path, "/meta/collection/"))
			collection, ok := collections[id]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": stremio.Meta{ID: id, Type: "collection", Name: id, Collection: &collection}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

func TestPagesListCatalogsBehindCollections(t *testing.T) {
	s, token, user, _ := pagesServerWith(t, collectionsAddon(t), "collection")
	// The collection fixed to Drama still offers its catalog's Action
	// titles; the nested collection's series count too.
	if page := namedPageOf(t, s, token, "/Genres/Action?userId="+user); page.MovieCount != 2 || page.SeriesCount != 1 {
		t.Errorf("genre page: %d movies, %d series", page.MovieCount, page.SeriesCount)
	}
	for query, want := range map[string][]string{
		"genres=Action": {"Movie A1", "Movie A2", "Show A"},
		"genres=Drama":  {"Movie D"},
		"genres=None":   nil,
	} {
		if names, total := listed(t, s, token, titlesOf+query); !slices.Equal(names, want) || total != len(want) {
			t.Errorf("%s: %v (total %d), want %v", query, names, total, want)
		}
	}
}

func TestUnknownGenresStudiosAndYears(t *testing.T) {
	s, token, user, _ := pagesServer(t)
	// Jellyfin answers the page of any name; it lists no title.
	for _, test := range []struct{ page, list string }{
		{"/Genres/Western", "genres=Western"},
		{"/Studios/Nobody", "studios=Nobody"},
		{"/Years/1999", "years=1999"},
	} {
		page := namedPageOf(t, s, token, test.page+"?userId="+user)
		if page.MovieCount != 0 || page.SeriesCount != 0 || page.UserData == nil {
			t.Errorf("%s: %+v", test.page, page)
		}
		lists := []string{test.list}
		switch page.Type {
		case "Genre":
			lists = append(lists, "genreIds="+page.Id)
		case "Studio":
			lists = append(lists, "studioIds="+page.Id)
		}
		for _, list := range lists {
			if names, total := listed(t, s, token, titlesOf+list); len(names) != 0 || total != 0 {
				t.Errorf("%s: %v (total %d)", list, names, total)
			}
		}
	}

	for _, path := range []string{"/Years/0", "/Years/-5", titlesOf + "years=nineteen"} {
		if status, body := s.call(http.MethodGet, path, app("tv", token), nil); status != http.StatusBadRequest ||
			string(body) != "Error processing request." {
			t.Errorf("%s: %d %s", path, status, body)
		}
	}
	for path, param := range map[string]string{
		"/Years/abc":          "year",
		"/Years/%20":          "year",
		"/Genres/%20":         "genreName",
		"/Studios/%20":        "name",
		"/Genres/A?userId=zz": "userId",
	} {
		status, body := s.call(http.MethodGet, path, app("tv", token), nil)
		var problem problemDetails
		_ = json.Unmarshal(body, &problem)
		if status != http.StatusBadRequest || len(problem.Errors[param]) != 1 {
			t.Errorf("%s: %d %s", path, status, body)
		}
	}

	// Only administrators open a page for another user; for a user that does
	// not exist, Jellyfin describes the item without user data.
	other := s.user("other", nil)
	if status, _ := s.call(http.MethodGet, "/Genres/Action?userId="+other.ID.String(), app("tv", token), nil); status != http.StatusForbidden {
		t.Errorf("member opening a page for another user: %d", status)
	}
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	admin := s.signIn("admin", "phone")
	status, body := s.call(http.MethodGet, "/Genres/Action?userId="+strings.Repeat("1", 32), app("phone", admin), nil)
	var fields map[string]any
	_ = json.Unmarshal(body, &fields)
	if _, hasUserData := fields["UserData"]; status != http.StatusOK || hasUserData || fields["Name"] != "Action" {
		t.Errorf("page for an unknown user: %d %s", status, body)
	}
}

// pageShapes lists what Polyfin cannot match in the fixtures of genre,
// studio and year pages.
var pageShapes = shapeRules{
	dynamic: []string{"ImageTags", "ImageBlurHashes", "ProviderIds"},
	absent: map[string][]string{
		// Polyfin has no files.
		"Path": {"*"}, "Container": {"*"},
		// The recorded genre had artwork; Polyfin's have none.
		"PrimaryImageAspectRatio": {"genre"},
		// Metadata addons do not provide it.
		"OriginalLanguage": {"*"},
	},
}

// TestPagesMatchJellyfin compares genre, studio and year pages and their
// listings with those recorded from Jellyfin 12.1 by
// scripts/jellyfin-fixtures.sh, with the same requests.
func TestPagesMatchJellyfin(t *testing.T) {
	s, token, user, _ := pagesServer(t)
	genre := namedPageOf(t, s, token, "/Genres/Action?userId="+user)
	studio := namedPageOf(t, s, token, "/Studios/North%20Studio?userId="+user)
	listing := "/Items?userId=" + user + "&recursive=true&includeItemTypes=Movie,Series&fields=PrimaryImageAspectRatio" +
		"&sortBy=SortName&sortOrder=Ascending"
	for fixture, path := range map[string]string{
		"genre":           "/Genres/Action?userId=" + user,
		"studio":          "/Studios/North%20Studio?userId=" + user,
		"year":            "/Years/2020?userId=" + user,
		"items-by-genre":  listing + "&genreIds=" + genre.Id,
		"items-by-studio": listing + "&studioIds=" + studio.Id,
		"items-by-year":   listing + "&years=2020",
	} {
		status, body := s.call(http.MethodGet, path, app("tv", token), nil)
		if status != http.StatusOK {
			t.Errorf("%s: %d %s", fixture, status, body)
			continue
		}
		raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.1", fixture+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var want, got any
		if err := json.Unmarshal(raw, &want); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("%s: %v in %s", fixture, err, body)
		}
		if items, _ := got.(map[string]any)["Items"].([]any); strings.HasPrefix(fixture, "items-") && len(items) == 0 {
			t.Errorf("%s: no titles to compare", fixture)
		}
		for _, difference := range compareShapes(fixture, want, got, pageShapes) {
			t.Error(difference)
		}
	}
}
