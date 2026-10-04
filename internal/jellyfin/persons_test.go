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
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// addonRequests counts the catalog requests a test addon receives, and
// among them the people searches.
type addonRequests struct {
	catalogs, searches atomic.Int32
}

// peopleAddon serves a movie catalog narrowed by genre and one of crime
// movies, the metas of its titles, their artwork, and, when peopleSearch is
// set, a people-search catalog that finds Ann Lee's movies, one of them in
// no other catalog.
func peopleAddon(t *testing.T, peopleSearch bool, requests *addonRequests) string {
	t.Helper()
	var server *httptest.Server
	movies := func() []stremio.Meta {
		movie := func(id, name, year string, genres []string, cast ...string) stremio.Meta {
			meta := stremio.Meta{ID: id, Type: "movie", Name: name, Genres: genres, ReleaseInfo: stremio.Text(year),
				Poster: server.URL + "/poster.jpg", Extras: &stremio.Extras{}}
			for _, name := range cast {
				meta.Extras.Cast = append(meta.Extras.Cast, stremio.CastMember{Name: name, Photo: server.URL + "/" + strings.ReplaceAll(name, " ", "") + ".jpg"})
			}
			return meta
		}
		heist := movie("tt0000001", "Heist", "2020", []string{"Action", "Crime"}, "Ann Lee")
		heist.Director = stremio.Names{"Dee Rector"}
		romance := movie("tt0000003", "Romance", "2020", []string{"Drama"}, "Ann Lee")
		romance.Extras.Cast[0].Photo = ""
		return []stremio.Meta{
			heist,
			movie("tt0000002", "Chase", "2019", []string{"Action", "Crime"}, "Bob Ray"),
			romance,
			movie("tt0000004", "Brawl", "2021", []string{"Action"}),
			movie("tt0000005", "Caper", "2010", []string{"Crime"}),
		}
	}
	debut := stremio.Meta{ID: "tt0000009", Type: "movie", Name: "Debut", ReleaseInfo: "2005", Poster: "https://example.com/debut.jpg"}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		extra := func(name string) string {
			_, value, _ := strings.Cut(strings.TrimSuffix(path, ".json"), name+"=")
			value, _, _ = strings.Cut(value, "&")
			value, _ = url.PathUnescape(value)
			return value
		}
		if strings.HasPrefix(path, "/catalog/") {
			requests.catalogs.Add(1)
		}
		switch {
		case path == "/manifest.json":
			catalogs := []stremio.Catalog{
				{Type: "movie", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "genre", Options: []string{"Action", "Crime", "Drama"}}}},
				{Type: "movie", ID: "crime", Name: "Crime", Extra: []stremio.Extra{{Name: "genre", Options: []string{"Crime"}}}},
			}
			if peopleSearch {
				catalogs = append(catalogs, stremio.Catalog{Type: "movie", ID: "people_search.people_search_movie", Name: "People Search",
					Extra: []stremio.Extra{{Name: "search", IsRequired: true}, {Name: "skip"}}})
			}
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "people", Name: "People", Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}}, Catalogs: catalogs})
		case strings.HasPrefix(path, "/catalog/movie/crime"):
			items := slices.DeleteFunc(movies(), func(m stremio.Meta) bool { return !slices.Contains(m.Genres, "Crime") })
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": items})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			items := movies()
			if genre := extra("genre"); genre != "" {
				items = slices.DeleteFunc(items, func(m stremio.Meta) bool { return !slices.Contains(m.Genres, genre) })
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": items})
		case strings.HasPrefix(path, "/catalog/movie/people_search.people_search_movie/"):
			requests.searches.Add(1)
			var found []stremio.Meta
			if extra("search") == "Ann Lee" && extra("skip") == "" {
				found = []stremio.Meta{debut, movies()[2]}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": found})
		case strings.HasPrefix(path, "/meta/movie/"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/meta/movie/"), ".json")
			for _, meta := range append(movies(), debut) {
				if meta.ID == id {
					_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
					return
				}
			}
			http.NotFound(w, r)
		case strings.HasSuffix(path, ".jpg"):
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// peopleSetup installs the people addon with its movie catalog as a library,
// signs a member in and lists the library's titles by name.
type peopleSetup struct {
	testServer
	token, user string
	titles      map[string]string
	requests    *addonRequests
}

func newPeopleSetup(t *testing.T, peopleSearch bool) peopleSetup {
	t.Helper()
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	requests := &addonRequests{}
	addon, err := s.addons.Install(t.Context(), addons.Shared(), peopleAddon(t, peopleSearch, requests), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "top"},
	}); err != nil {
		t.Fatal(err)
	}
	p := peopleSetup{testServer: s, token: s.signIn("member", "tv"), user: member.ID.String(), titles: map[string]string{}, requests: requests}
	var views, page QueryResult
	s.get(t, "/UserViews", p.token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, p.token, &page)
	for _, item := range page.Items {
		p.titles[item.Name] = item.Id
	}
	return p
}

// open describes a title as apps do on its page, and returns its credits.
func (p peopleSetup) open(t *testing.T, title string) []BaseItemPerson {
	t.Helper()
	var item BaseItemDto
	if status := p.get(t, "/Users/"+p.user+"/Items/"+p.titles[title], p.token, &item); status != http.StatusOK || item.People == nil {
		t.Fatalf("%s: %d %+v", title, status, item)
	}
	return *item.People
}

func (p peopleSetup) names(t *testing.T, path string) []string {
	t.Helper()
	var page QueryResult
	if status := p.get(t, path, p.token, &page); status != http.StatusOK {
		t.Fatalf("%s: %d", path, status)
	}
	return itemNames(page.Items)
}

func credit(people []BaseItemPerson, name string) BaseItemPerson {
	for _, person := range people {
		if person.Name == name {
			return person
		}
	}
	return BaseItemPerson{}
}

func TestATitlesActorOpensAsAPersonWithTheirPhoto(t *testing.T) {
	p := newPeopleSetup(t, true)
	ann := credit(p.open(t, "Heist"), "Ann Lee")
	if ann.Id == "" || ann.Type != "Actor" || ann.PrimaryImageTag == "" {
		t.Fatalf("credit: %+v", ann)
	}
	var person namedItemDto
	if status := p.get(t, "/Users/"+p.user+"/Items/"+ann.Id, p.token, &person); status != http.StatusOK ||
		person.Type != "Person" || person.Name != "Ann Lee" || person.ImageTags["Primary"] != ann.PrimaryImageTag {
		t.Fatalf("person: %d %+v", status, person)
	}
	// jellyfin-web only shows the sections of a person's page that count
	// titles.
	if person.MovieCount != 3 || person.SeriesCount != 0 {
		t.Errorf("counts: %d movies, %d series", person.MovieCount, person.SeriesCount)
	}
	var byName namedItemDto
	if status := p.get(t, "/Persons/Ann%20Lee", p.token, &byName); status != http.StatusOK || byName.Id != ann.Id {
		t.Errorf("by name: %d %+v", status, byName)
	}
	if status := p.get(t, "/Persons/Nobody", p.token, nil); status != http.StatusNotFound {
		t.Errorf("unknown name: %d", status)
	}
	response, err := http.Get(p.url + "/Items/" + ann.Id + "/Images/Primary?tag=" + ann.PrimaryImageTag)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "image/jpeg" {
		t.Errorf("photo: %d %s", response.StatusCode, response.Header.Get("Content-Type"))
	}
}

func TestAPersonsTitlesComeFromPeopleSearchCatalogs(t *testing.T) {
	p := newPeopleSetup(t, true)
	ann := credit(p.open(t, "Heist"), "Ann Lee")
	listing := "/Users/" + p.user + "/Items?personIds=" + ann.Id + "&recursive=true&includeItemTypes=Movie,Series&sortBy=PremiereDate,SortName&sortOrder=Descending,Ascending"
	// The search finds Debut and Romance; Heist is the title she was seen
	// credited in.
	if got := p.names(t, listing); !slices.Equal(got, []string{"Heist", "Romance", "Debut"}) {
		t.Errorf("titles: %v", got)
	}
	// Further pages, and the person's own page, read the search from cache.
	searches := p.requests.searches.Load()
	p.get(t, "/Items/"+ann.Id, p.token, nil)
	if got := p.names(t, listing+"&excludeItemIds="+p.titles["Heist"]+"&limit=1"); !slices.Equal(got, []string{"Romance"}) {
		t.Errorf("first title but Heist: %v", got)
	}
	if got := p.names(t, "/Items?personIds="+ann.Id+"&includeItemTypes=Series"); len(got) != 0 {
		t.Errorf("series: %v", got)
	}
	// The titles listed open, though the library does not list Debut.
	var page QueryResult
	p.get(t, listing, p.token, &page)
	var debut BaseItemDto
	if status := p.get(t, "/Items/"+page.Items[2].Id, p.token, &debut); status != http.StatusOK || debut.Name != "Debut" {
		t.Errorf("found title: %d %+v", status, debut)
	}
	if again := p.requests.searches.Load(); again != searches {
		t.Errorf("%d people searches again", again-searches)
	}
}

func TestAPersonsTitlesAreTheKnownOnesWithoutPeopleSearch(t *testing.T) {
	p := newPeopleSetup(t, false)
	ann := credit(p.open(t, "Heist"), "Ann Lee")
	p.open(t, "Chase")
	listing := "/Items?personIds=" + ann.Id + "&includeItemTypes=Movie"
	if got := p.names(t, listing); !slices.Equal(got, []string{"Heist"}) {
		t.Errorf("after one title: %v", got)
	}
	// A title crediting her without a photo adds to her titles and keeps
	// her photo.
	p.open(t, "Romance")
	if got := p.names(t, listing); !slices.Equal(got, []string{"Heist", "Romance"}) {
		t.Errorf("after two titles: %v", got)
	}
	var person namedItemDto
	if p.get(t, "/Items/"+ann.Id, p.token, &person); person.ImageTags["Primary"] != ann.PrimaryImageTag || person.MovieCount != 2 {
		t.Errorf("person: %+v", person)
	}
	// The people list knows everyone credited in the titles the user
	// reaches, by name.
	if got := p.names(t, "/Persons?searchTerm=lee"); !slices.Equal(got, []string{"Ann Lee"}) {
		t.Errorf("people named lee: %v", got)
	}
	if got := p.names(t, "/Persons"); !slices.Equal(got, []string{"Ann Lee", "Bob Ray", "Dee Rector"}) {
		t.Errorf("people: %v", got)
	}
	if got := p.names(t, "/Persons?personTypes=Director"); !slices.Equal(got, []string{"Dee Rector"}) {
		t.Errorf("directors: %v", got)
	}
	if got := p.names(t, "/Persons?appearsInItemId="+p.titles["Chase"]); !slices.Equal(got, []string{"Bob Ray"}) {
		t.Errorf("people of Chase: %v", got)
	}
	var page struct{ TotalRecordCount, StartIndex int }
	if p.get(t, "/Persons?startIndex=1&limit=1", p.token, &page); page.TotalRecordCount != 3 || page.StartIndex != 1 {
		t.Errorf("page: %+v", page)
	}
}

// privateAddon serves a catalog with one movie, Secret, crediting Ann Lee
// and Zed Hidden, under identifiers no other test addon describes.
func privateAddon(t *testing.T) string {
	t.Helper()
	secret := stremio.Meta{ID: "private:1", Type: "movie", Name: "Secret", Genres: []string{"Action"}, Poster: "https://example.com/secret.jpg",
		Extras: &stremio.Extras{Cast: []stremio.CastMember{{Name: "Ann Lee"}, {Name: "Zed Hidden"}}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "private", Name: "Private", Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{"private:"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "mine", Name: "Mine"}}})
		case strings.HasPrefix(path, "/catalog/movie/mine"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{secret}})
		case strings.HasPrefix(path, "/meta/movie/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": secret})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// Like Jellyfin, which knows people through the items a user can access,
// a user only sees the people credited, and the titles known to credit
// them, in what their own addons reach.
func TestPeopleStayWithinTheTitlesAUserReaches(t *testing.T) {
	p := newPeopleSetup(t, false)
	owner := p.testServer.user("owner", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	token := p.signIn("owner", "phone")
	addon, err := p.addons.Install(t.Context(), addons.Personal(owner.ID), privateAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.addons.SetLibraries(t.Context(), addons.Personal(owner.ID), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "mine"},
	}); err != nil {
		t.Fatal(err)
	}
	var views, page QueryResult
	p.get(t, "/UserViews", token, &views)
	for _, view := range views.Items {
		if view.Name == "Mine" {
			p.get(t, "/Items?ParentId="+view.Id, token, &page)
		}
	}
	if len(page.Items) != 1 {
		t.Fatalf("private library: %+v", page)
	}
	var secret BaseItemDto
	p.get(t, "/Items/"+page.Items[0].Id, token, &secret)
	zed := credit(*secret.People, "Zed Hidden")
	ann := credit(p.open(t, "Heist"), "Ann Lee")

	if got := p.names(t, "/Items?personIds="+ann.Id); !slices.Equal(got, []string{"Heist"}) {
		t.Errorf("Ann Lee's titles for the member: %v", got)
	}
	var person namedItemDto
	if p.get(t, "/Items/"+ann.Id, p.token, &person); person.MovieCount != 1 {
		t.Errorf("Ann Lee's movies for the member: %d", person.MovieCount)
	}
	if got := p.names(t, "/Persons"); slices.Contains(got, "Zed Hidden") {
		t.Errorf("people for the member: %v", got)
	}
	for _, path := range []string{"/Items/" + zed.Id, "/Persons/Zed%20Hidden", "/Items?personIds=" + zed.Id} {
		var page QueryResult
		if status := p.get(t, path, p.token, &page); status != http.StatusNotFound && len(page.Items) != 0 {
			t.Errorf("%s for the member: %d %+v", path, status, page)
		}
	}
	// The owner sees both, and the shared title the member opened.
	ownerNames := func(path string) []string {
		var page QueryResult
		p.get(t, path, token, &page)
		return itemNames(page.Items)
	}
	if got := ownerNames("/Items?personIds=" + ann.Id); !slices.Equal(got, []string{"Heist", "Secret"}) {
		t.Errorf("Ann Lee's titles for the owner: %v", got)
	}
	if got := ownerNames("/Persons?searchTerm=zed"); !slices.Equal(got, []string{"Zed Hidden"}) {
		t.Errorf("people for the owner: %v", got)
	}
}

func TestSimilarTitlesRankBySharedGenres(t *testing.T) {
	p := newPeopleSetup(t, false)
	similar := func(path string) []string {
		t.Helper()
		var page QueryResult
		if status := p.get(t, path, p.token, &page); status != http.StatusOK || page.TotalRecordCount != len(page.Items) {
			t.Fatalf("%s: %d %+v", path, status, page)
		}
		return itemNames(page.Items)
	}
	// Chase shares both genres, Brawl and Caper one, Brawl's year is
	// closer; Romance shares none. The library's catalog is read narrowed
	// to Action, so Caper comes from the crime catalog, which no library
	// lists.
	before := p.requests.catalogs.Load()
	if got := similar("/Items/" + p.titles["Heist"] + "/Similar?userId=" + p.user); !slices.Equal(got, []string{"Chase", "Brawl", "Caper"}) {
		t.Errorf("similar: %v", got)
	}
	// Apps ask for similar titles on every title page: one first page of
	// each catalog offering a genre of the title, then none until the
	// pages expire.
	if requests := p.requests.catalogs.Load() - before; requests != 2 {
		t.Errorf("%d catalog requests for similar titles", requests)
	}
	before = p.requests.catalogs.Load()
	if got := similar("/Movies/" + p.titles["Heist"] + "/Similar?limit=2"); !slices.Equal(got, []string{"Chase", "Brawl"}) {
		t.Errorf("limited: %v", got)
	}
	if requests := p.requests.catalogs.Load() - before; requests != 0 {
		t.Errorf("%d catalog requests for similar titles again", requests)
	}
	// Like Jellyfin, titles the user played are left out.
	if status, _ := p.call(http.MethodPost, "/UserPlayedItems/"+p.titles["Chase"], app("tv", p.token), nil); status != http.StatusOK {
		t.Fatalf("played: %d", status)
	}
	if got := similar("/Items/" + p.titles["Heist"] + "/Similar?limit=2"); !slices.Equal(got, []string{"Brawl", "Caper"}) {
		t.Errorf("after playing Chase: %v", got)
	}
	if status := p.get(t, "/Items/0123456789abcdef0123456789abcdef/Similar", p.token, nil); status != http.StatusNotFound {
		t.Errorf("unknown item: %d", status)
	}
}

// peopleShapes lists what Polyfin cannot match in the people and similar
// fixtures.
var peopleShapes = shapeRules{
	dynamic: []string{"ImageTags", "ImageBlurHashes", "ProviderIds"},
	absent: map[string][]string{
		// Metadata addons give no biography, birth date or birthplace of
		// people, and Polyfin has no files.
		"Overview": {"person", "person-by-name"}, "PremiereDate": {"person", "person-by-name", "persons"},
		"ProductionLocations": {"person", "person-by-name"}, "Path": {"person", "person-by-name"},
		"Container": {"*"}, "OriginalLanguage": {"*"}, "OfficialRating": {"person-titles", "similar"},
		"RunTimeTicks": {"person-titles", "similar"}, "CommunityRating": {"person-titles", "similar"},
		// A title a people search found was listed in no folder.
		"ParentId": {"person-titles"},
	},
}

// TestPeopleResponsesMatchJellyfin compares person pages, people lists,
// a person's titles and similar titles with those recorded from Jellyfin
// 12.1 by scripts/jellyfin-fixtures.sh, with the same requests.
func TestPeopleResponsesMatchJellyfin(t *testing.T) {
	p := newPeopleSetup(t, true)
	ann := credit(p.open(t, "Heist"), "Ann Lee")
	for fixture, path := range map[string]string{
		"person":         "/Users/" + p.user + "/Items/" + ann.Id,
		"person-by-name": "/Persons/Ann%20Lee?userId=" + p.user,
		"persons":        "/Persons?userId=" + p.user + "&searchTerm=Ann%20Lee&limit=24",
		"person-titles": "/Items?userId=" + p.user + "&personIds=" + ann.Id + "&recursive=true&includeItemTypes=Movie,Series" +
			"&fields=ParentId,PrimaryImageAspectRatio&sortBy=PremiereDate,ProductionYear,SortName&sortOrder=Descending,Descending,Ascending" +
			"&startIndex=0&limit=20",
		"similar": "/Items/" + p.titles["Heist"] + "/Similar?userId=" + p.user + "&limit=12&fields=PrimaryImageAspectRatio,CanDelete",
	} {
		status, body := p.call(http.MethodGet, path, app("tv", p.token), nil)
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
		if items, isList := got.(map[string]any)["Items"].([]any); isList && len(items) == 0 {
			t.Errorf("%s: nothing to compare", fixture)
		}
		for _, difference := range compareShapes(fixture, want, got, peopleShapes) {
			t.Error(difference)
		}
	}
}
