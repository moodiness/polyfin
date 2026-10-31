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
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/stremio"
)

// plainWordsAddon serves a catalog of movies whose genre filter offers
// Horror, Comedy and Drama, with their years and runtimes, one of series,
// and their descriptions.
func plainWordsAddon(t *testing.T) string {
	t.Helper()
	metas := []stremio.Meta{
		{ID: "tt1", Type: "movie", Name: "Night Shift", Genres: []string{"Horror"}, ReleaseInfo: "1985", Runtime: "95 min"},
		{ID: "tt2", Type: "movie", Name: "Long Night", Genres: []string{"Horror"}, ReleaseInfo: "1986", Runtime: "130 min"},
		{ID: "tt3", Type: "movie", Name: "Late Laughs", Genres: []string{"Comedy"}, ReleaseInfo: "1995", Runtime: "90 min"},
		{ID: "tt4", Type: "movie", Name: "Old Laughs", Genres: []string{"Comedy"}, ReleaseInfo: "1983", Runtime: "88 min"},
	}
	show := stremio.Meta{ID: "tt5", Type: "series", Name: "Eighties Show", Genres: []string{"Drama"}, ReleaseInfo: "1984", Runtime: "45 min"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "plain", Name: "Plain", Version: "1",
				Types: []string{"movie", "series"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{
					{Type: "movie", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "genre", Options: []string{"Horror", "Comedy", "Drama"}}}},
					{Type: "series", ID: "shows", Name: "Shows"},
				}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			// A genre, or a page past the first, which holds nothing.
			extras, _ := url.ParseQuery(strings.TrimPrefix(path, "/catalog/movie/top/"))
			listed := []stremio.Meta{}
			for _, m := range metas {
				if extras.Get("skip") == "" && (extras.Get("genre") == "" || slices.Contains(m.Genres, extras.Get("genre"))) {
					listed = append(listed, m)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": listed})
		case path == "/catalog/series/shows":
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{show}})
		case strings.HasPrefix(path, "/meta/"):
			id := path[strings.LastIndexByte(path, '/')+1:]
			for _, m := range append(slices.Clone(metas), show) {
				if m.ID == id {
					m.Extras = &stremio.Extras{Certification: "PG"}
					_ = json.NewEncoder(w).Encode(map[string]any{"meta": m})
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

// plainWords installs plainWordsAddon with its two catalogs as libraries.
func plainWords(t *testing.T) testServer {
	t.Helper()
	s := newTestServer(t, 10)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), plainWordsAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "top"},
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"},
	}); err != nil {
		t.Fatal(err)
	}
	return s
}

// searchCollections searches as jellyfin-web's search page asks for its
// Collections row.
func (s testServer) searchCollections(t *testing.T, token, term string) []BaseItemDto {
	t.Helper()
	var page QueryResult
	path := "/Items?searchTerm=" + url.QueryEscape(term) + "&IncludeItemTypes=BoxSet&Recursive=true&Limit=100&Fields=PrimaryImageAspectRatio"
	if status := s.get(t, path, token, &page); status != http.StatusOK {
		t.Fatalf("%s: %d", path, status)
	}
	return page.Items
}

// collectionOf returns the identifier of the one collection a search
// finds.
func (s testServer) collectionOf(t *testing.T, token, term string) string {
	t.Helper()
	found := s.searchCollections(t, token, term)
	if len(found) != 1 {
		t.Fatalf("%q: %v", term, itemNames(found))
	}
	return found[0].Id
}

// A French search in plain words finds the collection it stands for,
// named after it, which lists the movies of the 1980s under 2 hours of
// the user's libraries; it is not in the Collections view. A title's name,
// or a search for movies only, finds no collection.
func TestASearchInPlainWordsIsACollection(t *testing.T) {
	t.Parallel()
	s := plainWords(t)
	s.user("member", nil)
	token := s.signIn("member", "tv")
	const search = "films des années 80 de moins de 2 heures"

	found := s.searchCollections(t, token, " "+search+" ")
	if len(found) != 1 || found[0].Type != "BoxSet" || found[0].Name != search || !found[0].IsFolder {
		t.Fatalf("search: %+v", found)
	}
	id := found[0].Id
	if got := s.listingNames(t, token, "/Items?ParentId="+id); !slices.Equal(got, []string{"Night Shift", "Old Laughs"}) {
		t.Errorf("titles: %v", got)
	}
	var item BaseItemDto
	if status := s.get(t, "/Items/"+id, token, &item); status != http.StatusOK || item.Type != "BoxSet" || item.Name != search ||
		item.UserData.IsFavorite {
		t.Errorf("description: %d %+v", status, item)
	}
	if _, ok := s.viewIDs(t, token)["Collections"]; ok {
		t.Error("the search shows the Collections view")
	}
	// The same words find it again.
	if again := s.searchCollections(t, token, strings.ToUpper(search)); len(again) != 1 || again[0].Id != id {
		t.Errorf("searched again: %+v", again)
	}

	for _, term := range []string{"Night Shift", "Horror", "1985"} {
		if found := s.searchCollections(t, token, term); len(found) != 0 {
			t.Errorf("%q: %v", term, itemNames(found))
		}
	}
	var page QueryResult
	s.get(t, "/Items?searchTerm="+url.QueryEscape(search)+"&IncludeItemTypes=Movie&Recursive=true", token, &page)
	if slices.ContainsFunc(page.Items, func(item BaseItemDto) bool { return item.Type == "BoxSet" }) {
		t.Errorf("a search for movies: %v", itemNames(page.Items))
	}

	// Another user does not reach it.
	s.user("other", nil)
	if status := s.get(t, "/Items/"+id, s.signIn("other", "tv"), nil); status != http.StatusNotFound {
		t.Errorf("another user describes it: %d", status)
	}
}

// A search in plain words lists what the user may see: not the titles
// they hid, nor those of the genres they block.
func TestASearchInPlainWordsKeepsTheUsersLimits(t *testing.T) {
	t.Parallel()
	s := plainWords(t)
	s.user("member", nil)
	s.user("blocker", func(c *accounts.UserChanges) { c.BlockedGenres = &[]string{"Comedy"} })
	token, blocker := s.signIn("member", "tv"), s.signIn("blocker", "tv")
	const eighties = "80s movies under 2 hours"

	id := s.collectionOf(t, token, eighties)
	night := ""
	for _, title := range s.searchTitles(t, token, id) {
		if title.Name == "Night Shift" {
			night = title.Id
		}
	}
	if status, body := s.call(http.MethodPost, "/Polyfin/UserItems/"+night+"/Hidden", app("tv", token), nil); status != http.StatusOK &&
		status != http.StatusNoContent {
		t.Fatalf("hiding: %d %s", status, body)
	}
	if got := s.listingNames(t, token, "/Items?ParentId="+id); !slices.Equal(got, []string{"Old Laughs"}) {
		t.Errorf("once Night Shift is hidden: %v", got)
	}
	blocked := s.searchCollections(t, blocker, eighties)
	if len(blocked) != 1 {
		t.Fatalf("a user blocking comedies: %+v", blocked)
	}
	if got := s.listingNames(t, blocker, "/Items?ParentId="+blocked[0].Id); !slices.Equal(got, []string{"Night Shift"}) {
		t.Errorf("a user blocking comedies: %v", got)
	}
}

// searchTitles lists a collection's titles.
func (s testServer) searchTitles(t *testing.T, token, id string) []BaseItemDto {
	t.Helper()
	var page QueryResult
	if status := s.get(t, "/Items?ParentId="+id, token, &page); status != http.StatusOK {
		t.Fatalf("titles of %s: %d", id, status)
	}
	return page.Items
}

// Marking the collection of a search in plain words favorite saves it as
// the user's own smart list, shown as a collection; unmarking it deletes
// the list. A user whose apps would not show the list is refused.
func TestAddingASearchInPlainWordsToFavoritesSavesASmartList(t *testing.T) {
	t.Parallel()
	s := plainWords(t)
	member := s.user("member", nil)
	s.user("blocker", func(c *accounts.UserChanges) { c.BlockedGenres = &[]string{"Comedy"} })
	token := s.signIn("member", "tv")
	const search = "short horror movies"
	id := s.collectionOf(t, token, search)

	status, body := s.call(http.MethodPost, "/UserFavoriteItems/"+id, app("tv", token), nil)
	var data UserItemData
	if status != http.StatusOK || json.Unmarshal(body, &data) != nil || !data.IsFavorite {
		t.Fatalf("favorite: %d %s", status, body)
	}
	// Marked again, it is saved once.
	s.call(http.MethodPost, "/Users/"+member.ID.String()+"/FavoriteItems/"+id, app("tv", token), nil)
	lists, err := smartlists.New(s.pool).Lists(t.Context(), &member.ID)
	if err != nil || len(lists) != 1 || lists[0].Name != search || lists[0].ShownAs != smartlists.ShownAsCollection {
		t.Fatalf("own lists: %+v %v", lists, err)
	}
	var item BaseItemDto
	if s.get(t, "/Items/"+id, token, &item); !item.UserData.IsFavorite {
		t.Errorf("the search is not a favorite: %+v", item.UserData)
	}
	views := s.viewIDs(t, token)
	var page QueryResult
	s.get(t, "/Items?ParentId="+views["Collections"]+"&IncludeItemTypes=BoxSet&Recursive=true", token, &page)
	if got := itemNames(page.Items); !slices.Equal(got, []string{search}) {
		t.Fatalf("Collections view: %v", got)
	}
	if got := s.listingNames(t, token, "/Items?ParentId="+page.Items[0].Id); !slices.Equal(got, []string{}) {
		// No horror movie runs under 90 minutes.
		t.Errorf("the list saved: %v", got)
	}

	if status, body := s.call(http.MethodDelete, "/UserFavoriteItems/"+id, app("tv", token), nil); status != http.StatusOK {
		t.Fatalf("unfavorite: %d %s", status, body)
	}
	if lists, err := smartlists.New(s.pool).Lists(t.Context(), &member.ID); err != nil || len(lists) != 0 {
		t.Errorf("own lists once unmarked: %+v %v", lists, err)
	}

	blocker := s.signIn("blocker", "tv")
	blocked := s.collectionOf(t, blocker, search)
	if status, body := s.call(http.MethodPost, "/UserFavoriteItems/"+blocked, app("tv", blocker), nil); status != http.StatusForbidden || len(body) != 0 {
		t.Errorf("a user blocking genres: %d %s", status, body)
	}
}
