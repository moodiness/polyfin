package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// catalogAddon serves a manifest, a searchable movie catalog with genres, a
// series catalog, a collection catalog, metas and the artwork they
// reference.
func catalogAddon(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	movies := func() []stremio.Meta {
		var result []stremio.Meta
		for i, genre := range []string{"Action", "Drama", "Action"} {
			result = append(result, stremio.Meta{ID: fmt.Sprintf("tt000%d", i), Type: "movie", Name: fmt.Sprintf("Movie %d", i),
				Genres: []string{genre}, Poster: server.URL + "/poster.jpg", Background: server.URL + "/backdrop.jpg",
				ReleaseInfo: "2020", Released: "2020-04-10T00:00:00.000Z", Runtime: "1h 30min", ImdbRating: "7.5",
				TmdbID: "10", Extras: &stremio.Extras{Certification: "PG",
					Cast: []stremio.CastMember{{Name: "Actor", Character: "Hero", Photo: server.URL + "/poster.jpg"}}},
				Trailers: []stremio.Trailer{{Source: "abc", Type: "Trailer"}}})
		}
		return result
	}
	show := func() stremio.Meta {
		return stremio.Meta{ID: "tt0100", Type: "series", Name: "Show", Description: "A show",
			Poster: server.URL + "/poster.jpg", Background: server.URL + "/backdrop.jpg", Logo: server.URL + "/logo.png",
			ReleaseInfo: "2010-2011", Released: "2010-06-16T00:00:00.000Z", Status: "Ended", Genres: []string{"Drama"}, ImdbRating: "8",
			Runtime: "45min", ImdbID: "tt0100", TvdbID: "20",
			Extras: &stremio.Extras{Certification: "TV-14", Cast: []stremio.CastMember{{Name: "Actor", Character: "Hero"}}},
			Videos: []stremio.Video{
				{ID: "tt0100:1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2010-06-16T00:00:00.000Z",
					Thumbnail: server.URL + "/still.jpg", Overview: "It begins", Runtime: "45min"},
				{ID: "tt0100:1:2", Title: "Second", Season: 1, Episode: 2, Released: "2010-06-23T00:00:00.000Z", Runtime: "44min"},
				{ID: "tt0100:2:1", Title: "Return", Season: 2, Episode: 1, Released: "2011-12-13T00:00:00.000Z", Runtime: "46min"},
			}}
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		extra := func(name string) string {
			_, value, _ := strings.Cut(strings.TrimSuffix(path, ".json"), name+"=")
			value, _, _ = strings.Cut(value, "&")
			value, _ = url.PathUnescape(value)
			return value
		}
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "test", Name: "Test", Version: "1", Types: []string{"movie", "series", "collection"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{
					{Type: "movie", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "genre", Options: []string{"Action", "Drama"}}, {Name: "search"}}},
					{Type: "series", ID: "shows", Name: "Shows"},
					{Type: "collection", ID: "groups", Name: "Groups"},
				}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			items := movies()
			if genre := extra("genre"); genre != "" {
				items = slices.DeleteFunc(items, func(m stremio.Meta) bool { return m.Genres[0] != genre })
			}
			if search := extra("search"); search != "" {
				items = slices.DeleteFunc(items, func(m stremio.Meta) bool { return !strings.Contains(m.Name, search) })
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": items})
		case strings.HasPrefix(path, "/catalog/series/shows"):
			preview := show()
			preview.Videos = nil
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{preview}})
		case strings.HasPrefix(path, "/catalog/collection/groups"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{{ID: "group:1", Type: "collection", Name: "Group"}}})
		case strings.HasPrefix(path, "/meta/collection/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": stremio.Meta{ID: "group:1", Type: "collection", Name: "Group",
				Collection: &stremio.Collection{Sources: []stremio.CollectionSource{{Type: "movie", CatalogID: "top"}}}}})
		case path == "/meta/series/tt0100.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": show()})
		case strings.HasPrefix(path, "/meta/movie/"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/meta/movie/"), ".json")
			for _, meta := range movies() {
				if meta.ID == id {
					meta.Description = "Complete description"
					_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
					return
				}
			}
			http.NotFound(w, r)
		case path == "/poster.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// browsing installs the catalog addon with its movie and collection catalogs
// as libraries, and signs a member in.
func browsing(t *testing.T) (testServer, string, map[string]string) {
	t.Helper()
	return browsingOn(t, newTestServer(t, 10))
}

// browsingOn is browsing on a given test server.
func browsingOn(t *testing.T, s testServer) (testServer, string, map[string]string) {
	t.Helper()
	s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), catalogAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "top"},
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"},
		{AddonID: addon.ID, CatalogType: "collection", CatalogID: "groups"},
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
	return s, token, ids
}

// episodeStreams serves two streams of every episode. Their files are
// served too: a source that fails would stop the analyses that tests count.
func episodeStreams(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "episodes", Name: "Episodes", Version: "1", Types: []string{"series"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "stream"}}})
		case strings.HasPrefix(path, "/stream/series/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{
				{Name: "1080p", URL: server.URL + "/files/1080p.mkv"}, {Name: "720p", URL: server.URL + "/files/720p.mkv"}}})
		case strings.HasPrefix(path, "/files/"):
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("\x1a\x45\xdf\xa3 media bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// jellyfin-web plays an episode with its series' episodes from it on,
// asked from startItemId: the episode, or the version the user picked.
func TestEpisodeListsStartAtTheItemAsked(t *testing.T) {
	tr := newTracking(t)
	if _, err := tr.addons.Install(t.Context(), addons.Shared(), episodeStreams(t), false); err != nil {
		t.Fatal(err)
	}
	list := func(start string) QueryResult {
		t.Helper()
		var page QueryResult
		if status := tr.get(t, "/Shows/"+tr.series+"/Episodes?limit=100&startItemId="+start, tr.token, &page); status != http.StatusOK {
			t.Fatalf("from %s: %d", start, status)
		}
		return page
	}
	ids := func(page QueryResult) []string {
		result := []string{}
		for _, item := range page.Items {
			result = append(result, item.Id)
		}
		return result
	}
	if page := list(tr.episodes[1]); !slices.Equal(ids(page), tr.episodes[1:]) || page.TotalRecordCount != 2 {
		t.Errorf("from the second episode: %v, %d in all", ids(page), page.TotalRecordCount)
	}
	member, _ := tr.store.Authenticate(t.Context(), "member", "correct horse")
	second, _ := accounts.ParseID(tr.episodes[1])
	versions, err := tr.library.Versions(t.Context(), member, second)
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions: %v %v", versions, err)
	}
	if page := list(versions[1].ID.String()); !slices.Equal(ids(page), tr.episodes[1:]) {
		t.Errorf("from the second episode's other version: %v", ids(page))
	}
	if page := list(strings.Repeat("ab", 16)); len(page.Items) != 0 {
		t.Errorf("from an item the list does not have: %v", ids(page))
	}
}

// The web player's menus leave out the libraries marked so, one of the
// server's and one of the user's own, which /UserViews still lists for
// their home rows. /Polyfin/UserViews lists the caller's views as
// /UserViews does, in the same order, Polyfin's own views never left out,
// and none of another user's own libraries, though marked too.
func TestPolyfinUserViewsTellTheLibrariesLeftOutOfMenus(t *testing.T) {
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	other := s.user("other", nil)
	install := func(scope addons.Scope, manifestURL string, choices ...addons.LibraryChoice) {
		t.Helper()
		addon, err := s.addons.Install(t.Context(), scope, manifestURL, false)
		if err != nil {
			t.Fatal(err)
		}
		for i := range choices {
			choices[i].AddonID = addon.ID
		}
		if _, err := s.addons.SetLibraries(t.Context(), scope, choices); err != nil {
			t.Fatal(err)
		}
	}
	install(addons.Shared(), catalogAddon(t), addons.LibraryChoice{CatalogType: "movie", CatalogID: "top", HideInMenus: true},
		addons.LibraryChoice{CatalogType: "series", CatalogID: "shows"}, addons.LibraryChoice{CatalogType: "collection", CatalogID: "groups"})
	install(addons.Personal(member.ID), privateAddon(t), addons.LibraryChoice{CatalogType: "movie", CatalogID: "mine", HideInMenus: true})
	install(addons.Personal(other.ID), privateAddon(t), addons.LibraryChoice{CatalogType: "movie", CatalogID: "mine", HideInMenus: true})
	viewID := func(scope addons.Scope, catalogID string) string {
		t.Helper()
		libraries, err := s.addons.Libraries(t.Context(), scope)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range libraries {
			if l.Enabled && l.Catalog.ID == catalogID {
				return library.LibraryID(l).String()
			}
		}
		t.Fatalf("no library %s", catalogID)
		return ""
	}
	hidden := []string{viewID(addons.Shared(), "top"), viewID(addons.Personal(member.ID), "mine")}
	theirs := viewID(addons.Personal(other.ID), "mine")

	token := s.signIn("member", "tv")
	// A playlist adds Polyfin's Playlists view.
	s.createPlaylist(t, token, map[string]any{"Name": "Evening"})
	var views QueryResult
	if status := s.get(t, "/UserViews", token, &views); status != http.StatusOK {
		t.Fatalf("views: %d", status)
	}
	if !slices.Contains(itemNames(views.Items), "Playlists") {
		t.Fatalf("no Playlists view: %v", itemNames(views.Items))
	}
	var menus struct{ Items []MenuView }
	if status := s.get(t, "/Polyfin/UserViews", token, &menus); status != http.StatusOK {
		t.Fatalf("menu views: %d", status)
	}
	listed := func(id string) bool {
		return slices.ContainsFunc(views.Items, func(view BaseItemDto) bool { return view.Id == id })
	}
	if !listed(hidden[0]) || !listed(hidden[1]) {
		t.Errorf("/UserViews left out a library hidden from the menus: %v", itemIDs(views.Items))
	}
	if len(menus.Items) != len(views.Items) {
		t.Fatalf("menu views: %+v, views: %v", menus.Items, itemIDs(views.Items))
	}
	for i, view := range menus.Items {
		if view.Id != views.Items[i].Id {
			t.Errorf("menu view %d: %s, want %s (%s)", i, view.Id, views.Items[i].Id, views.Items[i].Name)
		}
		if want := slices.Contains(hidden, view.Id); view.HideInMenus != want {
			t.Errorf("%s (%s) hidden in menus: %v, want %v", views.Items[i].Name, view.Id, view.HideInMenus, want)
		}
		if view.Id == theirs {
			t.Errorf("another user's library listed: %+v", view)
		}
	}
	if status := s.get(t, "/Polyfin/UserViews", "", nil); status != http.StatusUnauthorized {
		t.Errorf("without credentials: %d", status)
	}
}

func (s testServer) get(t *testing.T, path, token string, into any) int {
	t.Helper()
	status, body := s.call(http.MethodGet, path, app("tv", token), nil)
	if into != nil && status == http.StatusOK {
		if err := json.Unmarshal(body, into); err != nil {
			t.Fatalf("%s: %v in %s", path, err, body)
		}
	}
	return status
}

func itemNames(items []BaseItemDto) []string {
	result := make([]string, len(items))
	for i, item := range items {
		result[i] = item.Name
	}
	return result
}

func TestBrowsingLibraries(t *testing.T) {
	s, token, views := browsing(t)
	if views["Top"] == "" || views["Groups"] == "" {
		t.Fatalf("views: %v", views)
	}

	var page QueryResult
	s.get(t, "/Items?ParentId="+views["Top"]+"&StartIndex=0&Limit=2", token, &page)
	if got := itemNames(page.Items); !slices.Equal(got, []string{"Movie 0", "Movie 1"}) || page.TotalRecordCount != 3 {
		t.Fatalf("first page: %v total %d", got, page.TotalRecordCount)
	}
	if page.Items[0].Type != "Movie" || page.Items[0].ProductionYear == nil || *page.Items[0].ProductionYear != 2020 ||
		page.Items[0].RunTimeTicks == nil || *page.Items[0].RunTimeTicks != 90*60*10_000_000 {
		t.Errorf("movie fields: %+v", page.Items[0])
	}
	s.get(t, "/Items?parentId="+views["Top"]+"&includeItemTypes=Series", token, &page)
	if len(page.Items) != 0 {
		t.Errorf("series filter on a movie library: %v", itemNames(page.Items))
	}
	s.get(t, "/Items?filters=IsFavorite&parentId="+views["Top"], token, &page)
	if len(page.Items) != 0 {
		t.Errorf("favorites before any favorite exists: %v", itemNames(page.Items))
	}

	var latest []BaseItemDto
	s.get(t, "/Items/Latest?parentId="+views["Top"]+"&limit=2", token, &latest)
	if len(latest) != 2 {
		t.Errorf("latest: %v", itemNames(latest))
	}

	var groups QueryResult
	member, _ := s.store.Authenticate(t.Context(), "member", "correct horse")
	s.get(t, "/Users/"+member.ID.String()+"/Items?ParentId="+views["Groups"], token, &groups)
	if len(groups.Items) != 1 || groups.Items[0].Type != "BoxSet" {
		t.Fatalf("collection library: %+v", groups.Items)
	}
	var content QueryResult
	s.get(t, "/Items?ParentId="+groups.Items[0].Id, token, &content)
	if len(content.Items) != 3 {
		t.Fatalf("collection content: %v", itemNames(content.Items))
	}

	var movie BaseItemDto
	if status := s.get(t, "/Users/"+member.ID.String()+"/Items/"+content.Items[1].Id, token, &movie); status != http.StatusOK {
		t.Fatalf("movie detail: %d", status)
	}
	if movie.Overview == nil || *movie.Overview != "Complete description" || movie.ParentId != groups.Items[0].Id {
		t.Errorf("detail is not completed from the meta resource: %+v", movie)
	}
	var byIDs QueryResult
	s.get(t, "/Items?ids="+content.Items[1].Id+",not-an-id", token, &byIDs)
	if len(byIDs.Items) != 1 || byIDs.Items[0].Id != content.Items[1].Id {
		t.Errorf("items by identifier: %v", itemNames(byIDs.Items))
	}
	var ancestors []BaseItemDto
	s.get(t, "/Items/"+movie.Id+"/Ancestors", token, &ancestors)
	if got := itemNames(ancestors); !slices.Equal(got, []string{"Group", "Groups"}) {
		t.Errorf("ancestors: %v", got)
	}
	if status := s.get(t, "/Items/"+strings.Repeat("0", 32), token, nil); status != http.StatusNotFound {
		t.Errorf("unknown item: %d", status)
	}
}

func TestBrowsingAnotherUsersLibrariesNeedsAnAdministrator(t *testing.T) {
	s, token, _ := browsing(t)
	other := s.user("other", nil)
	if status := s.get(t, "/Users/"+other.ID.String()+"/Views", token, nil); status != http.StatusForbidden {
		t.Errorf("member reading another user's views: %d", status)
	}
}

func TestGenres(t *testing.T) {
	s, token, views := browsing(t)
	var filters struct{ Genres []string }
	s.get(t, "/Items/Filters?parentId="+views["Top"], token, &filters)
	if !slices.Equal(filters.Genres, []string{"Action", "Drama"}) {
		t.Fatalf("filters: %v", filters.Genres)
	}
	var genres QueryResult
	s.get(t, "/Genres?parentId="+views["Top"], token, &genres)
	var action string
	for _, genre := range genres.Items {
		if genre.Name == "Action" {
			action = genre.Id
		}
	}
	var page QueryResult
	s.get(t, "/Items?parentId="+views["Top"]+"&genreIds="+action, token, &page)
	if got := itemNames(page.Items); !slices.Equal(got, []string{"Movie 0", "Movie 2"}) {
		t.Errorf("Action movies: %v", got)
	}
	s.get(t, "/Items?parentId="+views["Top"]+"&genres=Western", token, &page)
	if len(page.Items) != 0 {
		t.Errorf("a genre the catalog does not offer: %v", itemNames(page.Items))
	}
}

func TestImagesAreRelayedWithoutCredentials(t *testing.T) {
	s, token, views := browsing(t)
	var page QueryResult
	s.get(t, "/Items?ParentId="+views["Top"], token, &page)
	movie := page.Items[0]
	if movie.ImageTags["Primary"] == "" {
		t.Fatalf("no Primary image tag: %+v", movie.ImageTags)
	}
	response, err := http.Get(s.url + "/Items/" + movie.Id + "/Images/Primary?tag=" + movie.ImageTags["Primary"])
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 64)
	n, _ := response.Body.Read(body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "image/jpeg" || string(body[:n]) != "jpeg bytes" {
		t.Fatalf("image: %d %s %q", response.StatusCode, response.Header.Get("Content-Type"), body[:n])
	}
	request, _ := http.NewRequest(http.MethodGet, s.url+"/Items/"+movie.Id+"/Images/Primary", nil)
	request.Header.Set("If-None-Match", response.Header.Get("ETag"))
	if cached, err := http.DefaultClient.Do(request); err != nil || cached.StatusCode != http.StatusNotModified {
		t.Errorf("revalidation: %v %v", cached.StatusCode, err)
	}
	for _, path := range []string{"/Images/Logo", "/Images/Primary/1"} {
		if response, _ := http.Get(s.url + "/Items/" + movie.Id + path); response.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d", path, response.StatusCode)
		}
	}
	// The addon lists a backdrop its artwork server does not have: not
	// found, as an image the movie does not have.
	if len(movie.BackdropImageTags) == 0 {
		t.Fatalf("no backdrop: %+v", movie.BackdropImageTags)
	}
	if response, _ := http.Get(s.url + "/Items/" + movie.Id + "/Images/Backdrop/0?tag=" + movie.BackdropImageTags[0]); response.StatusCode != http.StatusNotFound {
		t.Errorf("a backdrop the artwork server does not have: %d", response.StatusCode)
	}
}

func TestCreditedPeopleHavePhotosAndPages(t *testing.T) {
	s, token, views := browsing(t)
	var page QueryResult
	s.get(t, "/Items?ParentId="+views["Top"], token, &page)
	var movie BaseItemDto
	s.get(t, "/Items/"+page.Items[0].Id, token, &movie)
	if movie.People == nil || len(*movie.People) == 0 || (*movie.People)[0].PrimaryImageTag == "" {
		t.Fatalf("people: %+v", movie.People)
	}
	actor := (*movie.People)[0]
	response, err := http.Get(s.url + "/Items/" + actor.Id + "/Images/Primary?tag=" + actor.PrimaryImageTag)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Errorf("photo: %d", response.StatusCode)
	}
	var person BaseItemDto
	if status := s.get(t, "/Items/"+actor.Id, token, &person); status != http.StatusOK || person.Type != "Person" || person.Name != "Actor" ||
		person.ImageTags["Primary"] != actor.PrimaryImageTag {
		t.Errorf("person page: %d %+v", status, person)
	}
}

// browseShapes lists what Polyfin cannot match in the browsing fixtures.
var browseShapes = shapeRules{
	dynamic: []string{"ImageTags", "ImageBlurHashes", "ProviderIds"},
	absent: map[string][]string{
		// Polyfin has no files; the test addon has no streams, so the
		// format and size of the titles are unknown.
		"Path": {"*"}, "Container": {"*"}, "Width": {"*"}, "Height": {"*"},
		// Without versions, details describe a placeholder, whose size,
		// bitrate and tracks are unknown.
		"Size": {"movie", "episode"}, "Bitrate": {"movie", "episode"}, "DefaultAudioStreamIndex": {"movie", "episode"},
		// Metadata addons do not provide these.
		"OriginalLanguage": {"*"}, "ProductionLocations": {"*"}, "SeriesStudio": {"*"},
		"CommunityRating": {"episode", "episodes"},
		// Jellyfin derives these from a collection's titles, which Polyfin
		// does not crawl to describe the collection.
		"OfficialRating": {"boxsets", "boxset"}, "PremiereDate": {"boxsets", "boxset"}, "ProductionYear": {"boxsets", "boxset"},
		// The size of a remote catalog, or of a series not opened yet, is
		// unknown.
		"ChildCount": {"views", "boxset"}, "RecursiveItemCount": {"boxset"},
		"DateLastMediaAdded": {"boxset"}, "UnplayedItemCount": {"library-series", "boxsets", "boxset"},
		// Jellyfin's root folder holds the libraries; Polyfin has none.
		"ParentId": {"views"},
		// Jellyfin drew artwork for the recorded libraries from their
		// titles; Polyfin's libraries have none.
		"PrimaryImageItemId": {"virtual-folders"},
	},
	returned: map[string][]string{
		// The recorded items lacked the rating and cast photo the fake addon
		// provides.
		"OfficialRating": {"latest"}, "PrimaryImageTag": {"movie"},
	},
}

// TestBrowseResponsesMatchJellyfin compares browsing responses with those
// recorded from Jellyfin 12.2 by scripts/jellyfin-fixtures.sh, with the
// same requests.
func TestBrowseResponsesMatchJellyfin(t *testing.T) {
	s, token, views := browsing(t)
	member, _ := s.store.Authenticate(t.Context(), "member", "correct horse")
	user := member.ID.String()
	first := func(path string, keep func(BaseItemDto) bool) string {
		var page QueryResult
		s.get(t, path, token, &page)
		for _, item := range page.Items {
			if keep == nil || keep(item) {
				return item.Id
			}
		}
		t.Fatalf("nothing in %s", path)
		return ""
	}
	movie := first("/Items?ParentId="+views["Top"], nil)
	series := first("/Items?ParentId="+views["Shows"], nil)
	season := first("/Shows/"+series+"/Seasons", func(item BaseItemDto) bool { return *item.IndexNumber == 1 })
	episode := first("/Shows/"+series+"/Episodes?seasonId="+season, nil)
	boxset := first("/Items?ParentId="+views["Groups"], nil)

	library := "/Items?userId=" + user + "&startIndex=0&limit=100&recursive=true&sortOrder=Ascending&fields=MediaSourceCount" +
		"&fields=PrimaryImageAspectRatio&sortBy=SortName&imageTypeLimit=1&enableImageTypes=Primary&enableImageTypes=Backdrop"
	counts := "Fields=ItemCounts,PrimaryImageAspectRatio,CanDelete,MediaSourceCount"
	for fixture, path := range map[string]string{
		"views":          "/UserViews?userId=" + user,
		"library-movies": library + "&parentId=" + views["Top"] + "&includeItemTypes=Movie",
		"library-series": library + "&parentId=" + views["Shows"] + "&includeItemTypes=Series",
		"latest": "/Items/Latest?userId=" + user + "&parentId=" + views["Top"] + "&fields=PrimaryImageAspectRatio&fields=Path" +
			"&imageTypeLimit=1&enableImageTypes=Primary&enableImageTypes=Backdrop&enableImageTypes=Thumb&limit=16",
		"movie":           "/Users/" + user + "/Items/" + movie,
		"series":          "/Users/" + user + "/Items/" + series,
		"season":          "/Users/" + user + "/Items/" + season,
		"episode":         "/Users/" + user + "/Items/" + episode,
		"seasons":         "/Shows/" + series + "/Seasons?userId=" + user + "&" + counts,
		"episodes":        "/Shows/" + series + "/Episodes?seasonId=" + season + "&userId=" + user + "&" + counts + ",Overview",
		"boxsets":         "/Users/" + user + "/Items?ParentId=" + views["Groups"] + "&" + counts,
		"boxset":          "/Users/" + user + "/Items/" + boxset,
		"boxset-children": "/Users/" + user + "/Items?ParentId=" + boxset + "&" + counts,
		"ancestors":       "/Items/" + episode + "/Ancestors",
		"filters":         "/Items/Filters?userId=" + user + "&parentId=" + views["Top"] + "&includeItemTypes=Movie",
		"filters2":        "/Items/Filters2?userId=" + user + "&parentId=" + views["Top"] + "&includeItemTypes=Movie",
		"search": "/Items?userId=" + user + "&limit=100&recursive=true&searchTerm=Movie&fields=PrimaryImageAspectRatio&fields=CanDelete" +
			"&fields=MediaSourceCount&includeItemTypes=Movie&includeItemTypes=Series&imageTypeLimit=1&enableTotalRecordCount=false",
		"display-preferences": "/DisplayPreferences/usersettings?userId=" + user + "&client=emby",
		"resume": "/UserItems/Resume?userId=" + user + "&limit=12&fields=PrimaryImageAspectRatio&mediaTypes=Video&imageTypeLimit=1" +
			"&enableImageTypes=Primary&enableImageTypes=Backdrop&enableImageTypes=Thumb&enableTotalRecordCount=false",
		"system-endpoint": "/System/Endpoint",
	} {
		status, body := s.call(http.MethodGet, path, app("tv", token), nil)
		if status != http.StatusOK {
			t.Errorf("%s: %d %s", fixture, status, body)
			continue
		}
		raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.2", fixture+".json"))
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
		for _, difference := range compareShapes(fixture, want, got, browseShapes) {
			t.Error(difference)
		}
	}
}

func TestVirtualFoldersDescribeTheLibraries(t *testing.T) {
	s, memberToken, _ := browsing(t)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	token := s.signIn("admin", "phone")

	status, body := s.call(http.MethodGet, "/Library/VirtualFolders", app("phone", token), nil)
	if status != http.StatusOK {
		t.Fatalf("administrator: %d %s", status, body)
	}
	var folders []VirtualFolderInfo
	if err := json.Unmarshal(body, &folders); err != nil {
		t.Fatal(err)
	}
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	if len(folders) != len(views.Items) || len(folders) == 0 {
		t.Fatalf("got %d folders for %d views", len(folders), len(views.Items))
	}
	for i, view := range views.Items {
		folder := folders[i]
		if folder.Name != view.Name || folder.ItemId != view.Id || folder.CollectionType != view.CollectionType {
			t.Errorf("folder %d: %+v, view %s %s %s", i, folder, view.Name, view.Id, view.CollectionType)
		}
		if folder.Locations == nil || len(folder.Locations) != 0 || folder.RefreshStatus != "Idle" {
			t.Errorf("folder %s: locations %v, status %s", folder.Name, folder.Locations, folder.RefreshStatus)
		}
	}

	raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.2", "virtual-folders.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want, got any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	for _, difference := range compareShapes("virtual-folders", want, got, browseShapes) {
		t.Error(difference)
	}

	// Like Jellyfin, a member is refused with an empty 403.
	if status, body := s.call(http.MethodGet, "/Library/VirtualFolders", app("tv", memberToken), nil); status != http.StatusForbidden || len(body) != 0 {
		t.Errorf("member: %d %q", status, body)
	}
}
