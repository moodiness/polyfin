package jellyfin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// addonRequests counts the catalog requests a test addon receives, and
// among them the people searches.
type addonRequests struct {
	catalogs, searches atomic.Int32
	// all counts every request, whatever it asks for.
	all atomic.Int32
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
		requests.all.Add(1)
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestSimilarTitlesCanBeTurnedOff(t *testing.T) {
	t.Parallel()
	p := newPeopleSetup(t, false)
	if !p.store.Settings().SimilarTitles {
		t.Fatal("similar titles are off by default")
	}
	p.setting(t, func(s *accounts.Settings) { s.SimilarTitles = false })
	// Apps ask for similar titles on a title's page, once its details are
	// open: those describe the title, which the similar routes check
	// exists, as every item route does.
	if status := p.get(t, "/Users/"+p.user+"/Items/"+p.titles["Heist"], p.token, nil); status != http.StatusOK {
		t.Fatalf("details: %d", status)
	}
	before := p.requests.all.Load()
	for _, route := range []string{"/Items/", "/Movies/", "/Shows/", "/Trailers/"} {
		path := route + p.titles["Heist"] + "/Similar?userId=" + p.user
		var page QueryResult
		if status := p.get(t, path, p.token, &page); status != http.StatusOK || len(page.Items) != 0 || page.TotalRecordCount != 0 {
			t.Errorf("switched off, %s: %d %+v", path, status, page)
		}
	}
	if requests := p.requests.all.Load() - before; requests != 0 {
		t.Errorf("switched off, %d addon requests for similar titles", requests)
	}
	// Statuses stay those of Jellyfin.
	if status := p.get(t, "/Items/0123456789abcdef0123456789abcdef/Similar", p.token, nil); status != http.StatusNotFound {
		t.Errorf("unknown item: %d", status)
	}
	// Turned back on, similar titles come back at once.
	p.setting(t, func(s *accounts.Settings) { s.SimilarTitles = true })
	var page QueryResult
	p.get(t, "/Items/"+p.titles["Heist"]+"/Similar", p.token, &page)
	if got := itemNames(page.Items); !slices.Equal(got, []string{"Chase", "Brawl", "Caper"}) {
		t.Errorf("switched on again: %v", got)
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
// 12.2 by scripts/jellyfin-fixtures.sh, with the same requests.
func TestPeopleResponsesMatchJellyfin(t *testing.T) {
	t.Parallel()
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
		if items, isList := got.(map[string]any)["Items"].([]any); isList && len(items) == 0 {
			t.Errorf("%s: nothing to compare", fixture)
		}
		for _, difference := range compareShapes(fixture, want, got, peopleShapes) {
			t.Error(difference)
		}
	}
}

// lockedLog collects what a logger writes from several requests at once.
type lockedLog struct {
	mu   sync.Mutex
	text strings.Builder
}

func (l *lockedLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.text.Write(p)
}

func (l *lockedLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.text.String()
}

// creditsAddons serves, for rounds titles, an addon that lists and
// describes them, crediting in each the round's actor and two others, and
// whose people search finds twenty titles of the round's actor that no
// one listed yet; and a French addon that describes the same titles with
// another cast: the round's actor's name in capitals, and an actor of its
// own, first. The people search answers once it was asked together times
// for the same name, by as many installations of the addon.
func creditsAddons(t *testing.T, rounds, together int) (credits, french string) {
	t.Helper()
	listed := make([]stremio.Meta, rounds)
	for round := range listed {
		listed[round] = stremio.Meta{ID: fmt.Sprintf("tt5%06d", round), Type: "movie", Name: fmt.Sprintf("Movie %d", round),
			Genres: []string{"Drama"}, ReleaseInfo: "2020", Director: stremio.Names{"Cy Dale"}, Extras: &stremio.Extras{Cast: []stremio.CastMember{
				{Name: fmt.Sprintf("Ada Round%d", round), Character: "Lead"}, {Name: "Bo Kerr", Character: "Friend"}}}}
	}
	found := func(round int) []stremio.Meta {
		metas := make([]stremio.Meta, 20)
		for i := range metas {
			metas[i] = stremio.Meta{ID: fmt.Sprintf("tt6%03d%03d", round, i), Type: "movie", Name: fmt.Sprintf("Found %d-%d", round, i),
				ReleaseInfo: "2019", Extras: &stremio.Extras{Cast: []stremio.CastMember{{Name: fmt.Sprintf("Ada Round%d", round)}}}}
		}
		return metas
	}
	var mu sync.Mutex
	searches := map[string]int{}
	answered := map[string]chan struct{}{}
	serve := func(handler http.HandlerFunc) string {
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		return server.URL + "/manifest.json"
	}
	credits = serve(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "credits", Name: "Credits", Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}}, Catalogs: []stremio.Catalog{
					{Type: "movie", ID: "top", Name: "Top"},
					{Type: "movie", ID: "people_search.people_search_movie", Name: "People Search",
						Extra: []stremio.Extra{{Name: "search", IsRequired: true}, {Name: "skip"}}}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": listed})
		case strings.HasPrefix(path, "/catalog/movie/people_search.people_search_movie/search="):
			query := strings.TrimPrefix(path, "/catalog/movie/people_search.people_search_movie/")
			mu.Lock()
			searches[query]++
			if answered[query] == nil {
				answered[query] = make(chan struct{})
			}
			all := answered[query]
			if searches[query] == together {
				close(all)
			}
			mu.Unlock()
			select {
			case <-all:
			case <-r.Context().Done():
				return
			}
			var metas []stremio.Meta
			name, _ := url.PathUnescape(strings.TrimPrefix(query, "search="))
			if round, err := strconv.Atoi(strings.TrimPrefix(name, "Ada Round")); err == nil {
				metas = found(round)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case strings.HasPrefix(path, "/meta/movie/"):
			if i := slices.IndexFunc(listed, func(m stremio.Meta) bool { return m.ID == strings.TrimPrefix(path, "/meta/movie/") }); i >= 0 {
				_ = json.NewEncoder(w).Encode(map[string]any{"meta": listed[i]})
				return
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	french = serve(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "credits-fr", Name: "Crédits", Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "meta"}}})
		case strings.HasPrefix(path, "/meta/movie/"):
			if i := slices.IndexFunc(listed, func(m stremio.Meta) bool { return m.ID == strings.TrimPrefix(path, "/meta/movie/") }); i >= 0 {
				meta := listed[i]
				meta.Name, meta.Director = fmt.Sprintf("Film %d", i), stremio.Names{"Cyrille Dale"}
				meta.Extras = &stremio.Extras{Cast: []stremio.CastMember{{Name: "Zoé Traduite", Character: "Amie"},
					{Name: fmt.Sprintf("ADA ROUND%d", i), Character: "Rôle"}}}
				_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
				return
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	return credits, french
}

// Users whose metadata language describes titles with another cast than
// the server's open a title, then its actor's page, at once: each finds in
// a people search the same titles, which Polyfin records for the first
// time, together. Every page opens, and shows the server's people.
func TestPeoplePagesOpenedAtOnceAllOpen(t *testing.T) {
	t.Parallel()
	const rounds, together = 8, 6
	var logged lockedLog
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, _ *pgxpool.Pool) {
		o.Logger = slog.New(slog.NewTextHandler(&logged, nil))
	})
	creditsURL, frenchURL := creditsAddons(t, rounds, together)
	french, err := s.addons.Install(t.Context(), addons.Shared(), frenchURL, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLanguage(t.Context(), addons.Shared(), french.ID, "fr"); err != nil {
		t.Fatal(err)
	}
	// Each reader has the addon of their own, so that each people search
	// asks it: administrators, whose own addons may reach the test's local
	// addresses.
	type reader struct{ id, token, movies string }
	readers := make([]reader, together)
	for i := range readers {
		name := fmt.Sprintf("reader%d", i)
		user := s.user(name, func(c *accounts.UserChanges) { c.MetadataLanguage, c.IsAdministrator = new("fr"), new(true) })
		addon, err := s.addons.Install(t.Context(), addons.Personal(user.ID), creditsURL, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.addons.SetLibraries(t.Context(), addons.Personal(user.ID), []addons.LibraryChoice{
			{AddonID: addon.ID, CatalogType: "movie", CatalogID: "top"},
		}); err != nil {
			t.Fatal(err)
		}
		readers[i] = reader{id: user.ID.String(), token: s.signIn(name, "tv")}
		var views QueryResult
		s.get(t, "/UserViews", readers[i].token, &views)
		readers[i].movies = views.Items[0].Id
	}
	var listing QueryResult
	s.get(t, "/Items?ParentId="+readers[0].movies, readers[0].token, &listing)
	if len(listing.Items) != rounds {
		t.Fatalf("listing: %v", itemNames(listing.Items))
	}
	// atOnce asks each reader's path at once.
	atOnce := func(path func(i int, r reader) string) {
		var wg sync.WaitGroup
		for i, r := range readers {
			wg.Go(func() {
				request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, s.url+path(i, r), nil)
				request.Header.Set("Authorization", app("tv", r.token))
				response, err := http.DefaultClient.Do(request)
				if err != nil {
					t.Error(err)
					return
				}
				_ = response.Body.Close()
				if response.StatusCode != http.StatusOK {
					t.Errorf("%s: %d", path(i, r), response.StatusCode)
				}
			})
		}
		wg.Wait()
	}

	for round := range rounds {
		movie := listing.Items[round].Id
		atOnce(func(_ int, r reader) string { return "/Users/" + r.id + "/Items/" + movie })
		var title BaseItemDto
		s.get(t, "/Users/"+readers[0].id+"/Items/"+movie, readers[0].token, &title)
		if title.People == nil {
			t.Fatalf("round %d, the title: %+v", round, title)
		}
		ada := credit(*title.People, fmt.Sprintf("Ada Round%d", round))
		if title.Name != fmt.Sprintf("Film %d", round) || ada.Id == "" || len(*title.People) != 3 {
			t.Fatalf("round %d, the title in French with the server's people: %q %+v", round, title.Name, *title.People)
		}
		// Apps open a person by either path. The requests record the titles
		// their people searches found together: the table is held until as
		// many of them as the database pool lets wait for it.
		hold, err := pgx.ConnectConfig(t.Context(), s.pool.Config().ConnConfig.Copy())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := hold.Exec(t.Context(), "BEGIN; LOCK TABLE items IN SHARE MODE"); err != nil {
			t.Fatal(err)
		}
		opened := make(chan struct{})
		go func() {
			defer close(opened)
			atOnce(func(i int, r reader) string {
				if i%2 == 0 {
					return "/Items/" + ada.Id
				}
				return "/Users/" + r.id + "/Items/" + ada.Id
			})
		}()
		deadline := time.Now().Add(10 * time.Second)
		for count := 0; count < min(len(readers), int(s.pool.Config().MaxConns)); time.Sleep(10 * time.Millisecond) {
			if err := hold.QueryRow(t.Context(), "SELECT count(*) FROM pg_locks WHERE relation = 'items'::regclass AND NOT granted").Scan(&count); err != nil {
				t.Error(err)
				break
			}
			if time.Now().After(deadline) {
				t.Errorf("round %d: %d requests wait for the table", round, count)
				break
			}
		}
		_ = hold.Close(t.Context())
		<-opened
		for _, r := range readers {
			var person namedItemDto
			if status := s.get(t, "/Users/"+r.id+"/Items/"+ada.Id, r.token, &person); status != http.StatusOK ||
				person.Name != ada.Name || person.MovieCount != 21 {
				t.Errorf("round %d, the person: %d %+v", round, status, person)
			}
		}
	}
	if t.Failed() {
		t.Logf("server log:\n%s", logged.String())
	}

	// The French addon's cast is not the title's: no one knows its own
	// actor, and the round's actor, by any spelling, is the server's.
	if status := s.get(t, "/Persons/Zo%C3%A9%20Traduite", readers[0].token, nil); status != http.StatusNotFound {
		t.Errorf("the French addon's actor: %d", status)
	}
	var page QueryResult
	s.get(t, "/Users/"+readers[0].id+"/Items?personIds="+library.PersonID("ADA ROUND0").String()+"&recursive=true&includeItemTypes=Movie",
		readers[0].token, &page)
	if got := itemNames(page.Items); len(got) != 21 || !slices.Contains(got, "Film 0") {
		t.Errorf("the first round's actor's titles: %v", got)
	}
}
