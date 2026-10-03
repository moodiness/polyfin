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

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// peopleAddon serves a movie catalog narrowed by genre, the metas of its
// titles, their artwork, and, when peopleSearch is set, a people-search
// catalog that finds Ann Lee's movies, one of them in no other catalog.
// searches counts the people searches.
func peopleAddon(t *testing.T, peopleSearch bool, searches *atomic.Int32) string {
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
		switch {
		case path == "/manifest.json":
			catalogs := []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top",
				Extra: []stremio.Extra{{Name: "genre", Options: []string{"Action", "Crime", "Drama"}}}}}
			if peopleSearch {
				catalogs = append(catalogs, stremio.Catalog{Type: "movie", ID: "people_search.people_search_movie", Name: "People Search",
					Extra: []stremio.Extra{{Name: "search", IsRequired: true}, {Name: "skip"}}})
			}
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "people", Name: "People", Version: "1", Types: []string{"movie"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}}, Catalogs: catalogs})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			items := movies()
			if genre := extra("genre"); genre != "" {
				items = slices.DeleteFunc(items, func(m stremio.Meta) bool { return !slices.Contains(m.Genres, genre) })
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": items})
		case strings.HasPrefix(path, "/catalog/movie/people_search.people_search_movie/"):
			searches.Add(1)
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
	searches    *atomic.Int32
}

func newPeopleSetup(t *testing.T, peopleSearch bool) peopleSetup {
	t.Helper()
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	searches := &atomic.Int32{}
	addon, err := s.addons.Install(t.Context(), addons.Shared(), peopleAddon(t, peopleSearch, searches), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "top"},
	}); err != nil {
		t.Fatal(err)
	}
	p := peopleSetup{testServer: s, token: s.signIn("member", "tv"), user: member.ID.String(), titles: map[string]string{}, searches: searches}
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
	searches := p.searches.Load()
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
	if again := p.searches.Load(); again != searches {
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
	// The people list knows everyone credited, by name.
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
	// closer; Romance shares none.
	if got := similar("/Items/" + p.titles["Heist"] + "/Similar?userId=" + p.user); !slices.Equal(got, []string{"Chase", "Brawl", "Caper"}) {
		t.Errorf("similar: %v", got)
	}
	if got := similar("/Movies/" + p.titles["Heist"] + "/Similar?limit=2"); !slices.Equal(got, []string{"Chase", "Brawl"}) {
		t.Errorf("limited: %v", got)
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
