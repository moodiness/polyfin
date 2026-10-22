package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

func TestRootFolderCountsAndSuggestionsAnswerAsJellyfin(t *testing.T) {
	t.Parallel()
	e := newEdited(t)
	var views QueryResult
	e.get(t, "/UserViews", e.member, &views)
	for _, path := range []string{"/Items/Root", "/Users/" + e.memberID + "/Items/Root", "/Items/" + rootFolderID.String()} {
		status, body := e.call(http.MethodGet, path, app("tv", e.member), nil)
		if status != http.StatusOK {
			t.Fatalf("%s: %d %s", path, status, body)
		}
		// Polyfin's root folder has no path on disk.
		matchesFixture(t, "root-folder", body, shapeRules{absent: map[string][]string{"Path": {"root-folder"}, "ChannelId": {"root-folder"},
			"ParentId": {"root-folder"}}, returned: map[string][]string{"CollectionType": {"root-folder"}}})
		var root BaseItemDto
		_ = json.Unmarshal(body, &root)
		if root.Type != "UserRootFolder" || root.Name != "Media Folders" || root.Id != rootFolderID.String() || *root.ChildCount != len(views.Items) {
			t.Errorf("%s: %s", path, body)
		}
	}
	var children QueryResult
	if status := e.get(t, "/Items?ParentId="+rootFolderID.String(), e.member, &children); status != http.StatusOK ||
		!slices.Equal(itemNames(children.Items), itemNames(views.Items)) {
		t.Errorf("root's children: %d %v, views %v", status, itemNames(children.Items), itemNames(views.Items))
	}

	status, body := e.call(http.MethodGet, "/Items/Counts", app("tv", e.member), nil)
	if status != http.StatusOK {
		t.Fatalf("counts: %d %s", status, body)
	}
	matchesFixture(t, "item-counts", body, shapeRules{})
	var counts ItemCounts
	_ = json.Unmarshal(body, &counts)
	if counts.MovieCount != 3 || counts.SeriesCount != 1 || counts.ItemCount != 4 {
		t.Errorf("counts: %s", body)
	}
	// The child does not count what they may not see.
	e.get(t, "/Items/Counts?userId="+e.childID, e.child, &counts)
	if counts.MovieCount != 2 || counts.SeriesCount != 0 {
		t.Errorf("child's counts: %+v", counts)
	}
	if status, _ := e.call(http.MethodPost, "/Users/"+e.memberID+"/FavoriteItems/"+e.ids["Allowed"], app("tv", e.member), nil); status != http.StatusOK {
		t.Fatalf("favorite: %d", status)
	}
	e.get(t, "/Items/Counts?isFavorite=true", e.member, &counts)
	if counts.MovieCount != 1 || counts.ItemCount != 1 {
		t.Errorf("favorite counts: %+v", counts)
	}
	e.get(t, "/Items/Counts?isFavorite=false", e.member, &counts)
	if counts.MovieCount != 2 || counts.SeriesCount != 1 {
		t.Errorf("counts of the others: %+v", counts)
	}

	for _, path := range []string{"/Items/Suggestions?type=Movie&limit=2", "/Users/" + e.memberID + "/Suggestions?type=Movie&limit=2"} {
		var suggestions QueryResult
		if status := e.get(t, path, e.member, &suggestions); status != http.StatusOK || len(suggestions.Items) != 2 ||
			slices.ContainsFunc(suggestions.Items, func(d BaseItemDto) bool { return d.Type != "Movie" }) {
			t.Errorf("%s: %d %v", path, status, suggestions.Items)
		}
	}
	var all QueryResult
	e.get(t, "/Items/Suggestions?mediaType=Video&enableTotalRecordCount=true", e.member, &all)
	if all.TotalRecordCount != 3 || len(all.Items) != 3 {
		t.Errorf("video suggestions: %d %v", all.TotalRecordCount, itemNames(all.Items))
	}
}

// emptySearchSuggestions is what jellyfin-web asks for the Suggestions of
// its search page before anything is typed.
const emptySearchSuggestions = "/Items?sortBy=IsFavoriteOrLiked&sortBy=Random&includeItemTypes=Movie,Series,MusicArtist&limit=20&recursive=true"

// The empty search page's Suggestions list titles from the user's
// libraries, their favorite and liked ones first: only what they may see,
// in the libraries they see, without the titles they hid.
func TestEmptySearchSuggestionsListTheTitlesTheUserSees(t *testing.T) {
	t.Parallel()
	e := newEdited(t)
	e.user("calm", func(c *accounts.UserChanges) { c.BlockedGenres = &[]string{"crime"} })
	suggested := func(token string) []string {
		t.Helper()
		var page QueryResult
		if status := e.get(t, emptySearchSuggestions, token, &page); status != http.StatusOK || page.TotalRecordCount != len(page.Items) {
			t.Fatalf("suggestions: %d %d %v", status, page.TotalRecordCount, itemNames(page.Items))
		}
		return itemNames(page.Items)
	}
	same := func(got []string, want ...string) bool {
		return len(got) == len(want) && !slices.ContainsFunc(want, func(name string) bool { return !slices.Contains(got, name) })
	}
	if got := suggested(e.member); !same(got, "Allowed", "Restricted", "Unrated", "Show") {
		t.Errorf("member's suggestions: %v", got)
	}
	if got := suggested(e.child); !same(got, "Allowed", "Unrated") {
		t.Errorf("child's suggestions: %v", got)
	}
	if got := suggested(e.signIn("calm", "phone")); !same(got, "Allowed", "Unrated") {
		t.Errorf("suggestions without the blocked genre: %v", got)
	}

	// A favorite and a liked title come first.
	for _, path := range []string{"/Users/" + e.memberID + "/FavoriteItems/" + e.ids["Unrated"], "/UserItems/" + e.ids["Show"] + "/Rating?likes=true"} {
		if status, body := e.call(http.MethodPost, path, app("tv", e.member), nil); status != http.StatusOK {
			t.Fatalf("%s: %d %s", path, status, body)
		}
	}
	for range 3 {
		if got := suggested(e.member); len(got) != 4 || !same(got[:2], "Unrated", "Show") {
			t.Errorf("favorite and liked first: %v", got)
		}
	}

	// A hidden title and a hidden library are left out.
	if status, body := e.call(http.MethodPost, "/Polyfin/UserItems/"+e.ids["Restricted"]+"/Hidden", app("tv", e.member), nil); status != http.StatusNoContent {
		t.Fatalf("hiding a title: %d %s", status, body)
	}
	member, _ := accounts.ParseID(e.memberID)
	shows, _ := accounts.ParseID(e.ids["Shows"])
	if _, err := e.store.UpdateUser(t.Context(), member, accounts.UserChanges{HiddenLibraries: &[]accounts.ID{shows}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := suggested(e.member); !slices.Equal(got, []string{"Unrated", "Allowed"}) {
		t.Errorf("suggestions without the hidden title and library: %v", got)
	}
}

// popularAddon serves two popular catalogs of 30 titles each, Movie 01 to
// Movie 30 and Series 01 to Series 30, most popular first. Movie 02 is
// rated R, Series 03 TV-MA, and Movie 04 is a crime title.
func popularAddon(t *testing.T) string {
	t.Helper()
	titles := func(kind, name string) []stremio.Meta {
		var result []stremio.Meta
		for i := 1; i <= 30; i++ {
			m := stremio.Meta{ID: fmt.Sprintf("tt%s%02d", kind, i), Type: kind, Name: fmt.Sprintf("%s %02d", name, i),
				Genres: []string{"Drama"}, Extras: &stremio.Extras{Certification: "PG"}}
			switch m.Name {
			case "Movie 02":
				m.Extras.Certification = "R"
			case "Series 03":
				m.Extras.Certification = "TV-MA"
			case "Movie 04":
				m.Genres = []string{"Crime"}
			}
			result = append(result, m)
		}
		return result
	}
	movies, series := titles("movie", "Movie"), titles("series", "Series")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "popular", Name: "Popular", Version: "1", Types: []string{"movie", "series"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "popular", Name: "Popular movies"}, {Type: "series", ID: "popular", Name: "Popular series"}}})
		case strings.Contains(path, "skip="):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{}})
		case path == "/catalog/movie/popular":
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": movies})
		case path == "/catalog/series/popular":
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": series})
		case strings.HasPrefix(path, "/meta/"):
			id := path[strings.LastIndex(path, "/")+1:]
			for _, meta := range append(slices.Clone(movies), series...) {
				if meta.ID == id {
					_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
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

// Before anything is typed, the search page suggests the user's favorite
// and liked titles, then titles from the top of each library's first page,
// in the catalog's order, the libraries mixed, and not the same each time:
// never a title the user may not see.
func TestEmptySearchSuggestsTheTopOfEachLibrary(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(13)} })
	s.user("calm", func(c *accounts.UserChanges) { c.BlockedGenres = &[]string{"crime"} })
	if _, err := s.addons.Install(t.Context(), addons.Shared(), popularAddon(t), false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	ids := s.viewIDs(t, token)
	for _, library := range []string{"Popular movies", "Popular series"} {
		var page QueryResult
		if status := s.get(t, "/Items?ParentId="+ids[library], token, &page); status != http.StatusOK || len(page.Items) != 30 {
			t.Fatalf("%s: %d %v", library, status, itemNames(page.Items))
		}
		for _, item := range page.Items {
			ids[item.Name] = item.Id
		}
	}
	rank := func(name string) int {
		n, _ := strconv.Atoi(name[len(name)-2:])
		return n
	}
	suggested := func(token string) []string {
		t.Helper()
		var page QueryResult
		if status := s.get(t, emptySearchSuggestions, token, &page); status != http.StatusOK || len(page.Items) != 20 {
			t.Fatalf("suggestions: %d %v", status, itemNames(page.Items))
		}
		return itemNames(page.Items)
	}

	for _, path := range []string{"/Users/" + member.ID.String() + "/FavoriteItems/" + ids["Movie 28"], "/UserItems/" + ids["Series 29"] + "/Rating?likes=true"} {
		if status, body := s.call(http.MethodPost, path, app("tv", token), nil); status != http.StatusOK {
			t.Fatalf("%s: %d %s", path, status, body)
		}
	}
	answers := map[string]bool{}
	for range 5 {
		got := suggested(token)
		answers[strings.Join(got, ",")] = true
		if !slices.Contains(got[:2], "Movie 28") || !slices.Contains(got[:2], "Series 29") {
			t.Errorf("the favorite and liked titles come first: %v", got)
		}
		// Nine titles of each library, among its first ten.
		movies := 0
		for _, name := range got[2:] {
			if strings.HasPrefix(name, "Movie") {
				movies++
			}
			if rank(name) > 10 {
				t.Errorf("%s is not among its library's top titles: %v", name, got)
			}
		}
		if movies != 9 {
			t.Errorf("the libraries are not mixed: %v", got)
		}
	}
	if len(answers) == 1 {
		t.Errorf("the suggestions never vary: %v", answers)
	}

	for name, user := range map[string]string{"child": "Movie 02,Series 03", "calm": "Movie 04"} {
		for range 3 {
			got := suggested(s.signIn(name, "phone"))
			for _, unseen := range strings.Split(user, ",") {
				if slices.Contains(got, unseen) {
					t.Errorf("%s is suggested %s: %v", name, unseen, got)
				}
			}
			for _, title := range got {
				if rank(title) > 12 {
					t.Errorf("%s: %s is not among its library's top titles: %v", name, title, got)
				}
			}
		}
	}

	// A hidden title and a hidden library are left out.
	if status, body := s.call(http.MethodPost, "/Polyfin/UserItems/"+ids["Movie 01"]+"/Hidden", app("tv", token), nil); status != http.StatusNoContent {
		t.Fatalf("hiding a title: %d %s", status, body)
	}
	series, _ := accounts.ParseID(ids["Popular series"])
	if _, err := s.store.UpdateUser(t.Context(), member.ID, accounts.UserChanges{HiddenLibraries: &[]accounts.ID{series}}, nil); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		got := suggested(token)
		if got[0] != "Movie 28" || slices.Contains(got, "Movie 01") || slices.ContainsFunc(got, func(name string) bool { return strings.HasPrefix(name, "Series") }) {
			t.Errorf("suggestions without the hidden title and library: %v", got)
		}
	}
}

// Other listings without a folder still answer none: only the empty search
// page's Suggestions are answered from the user's libraries.
func TestOtherListingsWithoutAFolderStayEmpty(t *testing.T) {
	t.Parallel()
	e := newEdited(t)
	for _, path := range []string{
		"/Items?sortBy=DateCreated&sortOrder=Descending&includeItemTypes=Movie,Series&limit=20&recursive=true",
		"/Items?sortBy=Random&includeItemTypes=Movie,Episode&limit=20&recursive=true",
		"/Items?sortBy=Random&limit=20&recursive=true",
		"/Items?sortBy=IsFavoriteOrLiked&sortBy=Random&includeItemTypes=Movie,Series&limit=20",
		"/Items?includeItemTypes=Movie,Series&recursive=true",
	} {
		var page QueryResult
		if status := e.get(t, path, e.member, &page); status != http.StatusOK || len(page.Items) != 0 || page.TotalRecordCount != 0 {
			t.Errorf("%s: %d %d %v", path, status, page.TotalRecordCount, itemNames(page.Items))
		}
	}
}
