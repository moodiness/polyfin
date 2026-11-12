package library

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// studio is a network or company of TMDB's details on the fake TMDB.
func studio(id int, name, logo string) map[string]any {
	s := map[string]any{"id": id, "name": name}
	if logo != "" {
		s["logo_path"] = logo
	}
	return s
}

// studios gives the TMDB answer of path, a movie's or series' details,
// these networks and companies.
func (f *fakeCreditsTMDB) studios(path string, networks, companies []any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	answer := f.answers[path].(map[string]any)
	answer["networks"], answer["production_companies"], answer["tagline"] = networks, companies, ""
}

// discovered is a title of TMDB's discover on the fake TMDB.
func discovered(kind string, id int, name, date string, popularity float64) map[string]any {
	d := map[string]any{"id": id, "popularity": popularity, "vote_count": 10, "poster_path": fmt.Sprintf("/d%d.jpg", id)}
	if kind == "tv" {
		d["name"], d["first_air_date"] = name, date
	} else {
		d["title"], d["release_date"] = name, date
	}
	return d
}

// studioNames lists the names of the titles of a studio's page.
func studioNames(t *testing.T, s *Service, user accounts.User, name string, start, count int) []string {
	t.Helper()
	page, err := s.StudioTitles(t.Context(), user, []string{name}, func(option string) bool { return strings.EqualFold(option, name) },
		[]Kind{KindMovie, KindSeries}, start, count)
	if err != nil {
		t.Fatal(err)
	}
	return names(page.Items)
}

// A studio's page lists the titles its catalogs list for its name, then
// those whose kept details name it, then TMDB's most popular of it that
// the user's addons describe, each once, without titles for adults or not
// out yet, as the user may see them; its networks and companies of one
// name, whatever its case, are one studio, whose logo is its network's.
func TestAStudioPageListsCatalogsThenKeptThenDiscover(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.service.UseTitleCredits(e.queue)
	north := studio(71, "North Studio", "/north-co.png")
	e.tmdb.studios("/movie/1001", []any{}, []any{north})
	e.tmdb.studios("/movie/1012", []any{}, []any{north, studio(73, "Harbor Network", "")})
	e.tmdb.studios("/movie/1013", []any{}, []any{north})
	e.tmdb.studios("/movie/1002", []any{}, []any{studio(72, "north studio", "/north-low.png")})
	e.tmdb.studios("/tv/2031", []any{studio(81, "North Studio", "/north-net.png")}, []any{})
	for _, name := range []string{"Harbor Lights", "Kelp Bay", "Night Shift", "Quiet Pier", "Tidewater", "Salt Road"} {
		e.open(e.admin, name)
	}
	e.settle()
	picks := &fakeAddon{
		manifest: stremio.Manifest{ID: "picks", Name: "Picks", Version: "1", Types: []string{"movie"}, Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{{Type: "movie", ID: "picks", Name: "Picks", Extra: []stremio.Extra{{Name: "genre", Options: []string{"North Studio"}}}}}},
		catalogs: map[string][]stremio.Meta{"movie/picks": {
			{ID: "tt0882001", Type: "movie", Name: "Harbor Lights", Genres: stremio.Names{"North Studio"}},
			{ID: "pick:1", Type: "movie", Name: "Catalog Pick", Genres: stremio.Names{"North Studio"}},
		}},
	}
	e.install(addons.Shared(), picks)
	e.tmdb.mu.Lock()
	e.tmdb.answers["/discover/movie?page=1"] = map[string]any{"results": []any{
		discovered("movie", 1001, "Harbor Lights", "2011-01-01", 90),
		discovered("movie", 1301, "Discovered One", "2015-01-01", 50),
		map[string]any{"id": 1302, "title": "Grown Only", "release_date": "2015-01-01", "adult": true, "popularity": 80},
		discovered("movie", 1303, "Coming Soon", "2099-01-01", 70),
		discovered("movie", 1304, "No Date", "", 60),
	}}
	e.tmdb.answers["/discover/movie?page=2"] = map[string]any{"results": []any{discovered("movie", 1305, "Second Page", "2016-01-01", 10)}}
	e.tmdb.answers["/discover/tv?page=1"] = map[string]any{"results": []any{discovered("tv", 2301, "Discovered Show", "2017-01-01", 40)}}
	e.tmdb.answers["/discover/tv?page=2"] = map[string]any{"results": []any{}}
	e.tmdb.mu.Unlock()

	want := []string{"Harbor Lights", "Catalog Pick", "Kelp Bay", "Night Shift", "Quiet Pier", "Tidewater", "Discovered One", "Discovered Show",
		"Second Page"}
	if got := studioNames(t, e.service, e.admin, "North Studio", 0, 100); !slices.Equal(got, want) {
		t.Fatalf("North Studio's page:\n got %v\nwant %v", got, want)
	}
	if got := studioNames(t, e.service, e.admin, "north studio", 2, 3); !slices.Equal(got, want[2:5]) {
		t.Errorf("a window of it: %v", got)
	}
	page, err := e.service.StudioTitles(t.Context(), e.admin, []string{"North Studio"}, func(string) bool { return false }, []Kind{KindSeries}, 0, 100)
	if err != nil || !slices.Equal(names(page.Items), []string{"Tidewater", "Discovered Show"}) || page.Total != 2 || page.More {
		t.Errorf("its series: %v %d %v", names(page.Items), page.Total, err)
	}
	// Discover asked for every network and company of the name, two pages
	// of each kind, once.
	for _, prefix := range []string{"/3/discover/movie?", "/3/discover/tv?"} {
		if asked := e.tmdb.requests(prefix); len(asked) != 2 {
			t.Errorf("%s asked %v", prefix, asked)
		}
	}
	if asked := e.tmdb.requests("/3/discover/movie?"); !strings.HasSuffix(asked[0], "&with_companies=71%7C72") {
		t.Errorf("movies asked %v", asked)
	}
	if asked := e.tmdb.requests("/3/discover/tv?"); !strings.HasSuffix(asked[0], "&with_networks=81") {
		t.Errorf("series asked %v", asked)
	}
	if logo, err := e.service.StudioLogo(t.Context(), "NORTH STUDIO"); err != nil || logo != "/north-net.png" {
		t.Errorf("the logo: %q %v", logo, err)
	}

	// A restricted user does not get Night Shift (R); a user who hid two of
	// them does not get those.
	child := e.restrict("child", accounts.ParentalControl{MaxRating: new(13)})
	if got := studioNames(t, e.service, child, "North Studio", 0, 100); slices.Contains(got, "Night Shift") || !slices.Contains(got, "Kelp Bay") {
		t.Errorf("for a restricted user: %v", got)
	}
	for _, hidden := range []accounts.ID{e.titles["Kelp Bay"], itemID(titleKey(KindMovie, "tmdb:1301"))} {
		if err := e.service.userData.Hide(t.Context(), e.member.ID, hidden, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if got := studioNames(t, e.service, e.member, "North Studio", 0, 100); slices.Contains(got, "Kelp Bay") || slices.Contains(got, "Discovered One") ||
		!slices.Contains(got, "Second Page") {
		t.Errorf("for a user who hid two: %v", got)
	}

	// The studios of the libraries' titles, each once, with their counts.
	studios, err := e.service.Studios(t.Context(), e.admin, accounts.ID{})
	if err != nil {
		t.Fatal(err)
	}
	var listed []string
	for _, s := range studios {
		listed = append(listed, fmt.Sprintf("%s %d/%d %s", s.Name, s.Movies, s.Series, s.Logo))
	}
	if !slices.Equal(listed, []string{"Harbor Network 1/0 ", "North Studio 4/1 /north-net.png"}) {
		t.Errorf("the studios: %q", listed)
	}

	// Without a key, the page lists its catalogs' and kept titles only, and
	// TMDB is asked nothing.
	e.setting(func(s *accounts.Settings) { s.TMDBKey = "" })
	if got := studioNames(t, e.service, e.admin, "North Studio", 0, 100); !slices.Equal(got, want[:6]) {
		t.Errorf("without a key: %v", got)
	}
	if asked := e.tmdb.requests("/3/discover/"); len(asked) != 4 {
		t.Errorf("discover asked again: %v", asked)
	}
}

// A studio's page lists at most 40 movies and 40 series from TMDB's
// discover, through the user's addons: by IMDb identifier when they
// describe those only, looked up once, and none of a kind no addon
// describes.
func TestAStudiosDiscoverIsBoundedAndDescribed(t *testing.T) {
	t.Parallel()
	e := newKnownForEnv(t, []stremio.Resource{{Name: "meta", Types: []string{"movie"}, IDPrefixes: []string{"tt"}}}, nil)
	if err := e.service.keepStudios(t.Context(), tmdb.TitleDetails{Networks: []tmdb.Studio{{ID: 91, Name: "Harbor Network", LogoPath: "/harbor.png"}},
		Companies: []tmdb.Studio{{ID: 92, Name: "Harbor Network"}}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	e.tmdb.mu.Lock()
	for page := 1; page <= 3; page++ {
		var movies, series []any
		for i := range 25 {
			id := 3000 + 100*page + i
			movies = append(movies, discovered("movie", id, fmt.Sprintf("Movie %d", id), "2010-01-01", float64(1000-id)))
			series = append(series, discovered("tv", id, fmt.Sprintf("Series %d", id), "2010-01-01", float64(1000-id)))
			e.tmdb.answers[fmt.Sprintf("/movie/%d/external_ids", id)] = map[string]any{"imdb_id": fmt.Sprintf("tt%07d", id)}
		}
		e.tmdb.answers[fmt.Sprintf("/discover/movie?page=%d", page)] = map[string]any{"results": movies}
		e.tmdb.answers[fmt.Sprintf("/discover/tv?page=%d", page)] = map[string]any{"results": series}
	}
	e.tmdb.mu.Unlock()
	var got []string
	// The page waits a moment for the IMDb identifiers: those that come
	// later show the next time.
	for range 50 {
		if got = studioNames(t, e.service, e.admin, "Harbor Network", 0, 200); len(got) == 40 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(got) != 40 || got[0] != "Movie 3100" || got[39] != "Movie 3214" || slices.ContainsFunc(got, func(name string) bool { return strings.HasPrefix(name, "Series") }) {
		t.Errorf("Harbor Network's titles: %v", got)
	}
	if asked := e.tmdb.requests("/3/discover/movie?"); len(asked) != 2 || !strings.HasSuffix(asked[0], "&with_companies=92") {
		t.Errorf("movies asked %v", asked)
	}
	if asked := e.tmdb.requests("/3/discover/tv?"); len(asked) != 2 || !strings.HasSuffix(asked[0], "&with_networks=91") {
		t.Errorf("series asked %v", asked)
	}
	if asked := e.tmdb.requests("/3/movie/3100/external_ids"); len(asked) != 1 {
		t.Errorf("an IMDb identifier asked %d times", len(asked))
	}
	if asked := e.tmdb.requests("/3/tv/"); len(asked) != 0 {
		t.Errorf("series no addon describes were looked up: %v", asked)
	}
}

// Details kept before their studios' identifiers were are due once: a
// page reads them again, which keeps the studios and their logos.
func TestDetailsKeptWithoutStudioIdentifiersAreDueOnce(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.service.UseTitleCredits(e.queue)
	e.tmdb.studios("/movie/1002", []any{}, []any{studio(72, "North Studio", "/north.png")})
	e.open(e.admin, "Quiet Pier")
	e.settle()
	key := creditsKey{kind: "movie", provider: "imdb", id: "tt0882002"}
	if _, err := e.service.db.Exec(t.Context(), `UPDATE title_credits SET network_ids = NULL, company_ids = NULL;
		DELETE FROM tmdb_studios`); err != nil {
		t.Fatal(err)
	}
	e.service.forgetCredits(key)
	kept, err := e.service.readTitleCredits(t.Context(), []creditsKey{key})
	if k := kept[key]; err != nil || k.details || !k.due(time.Now(), "fr", e.titles["Quiet Pier"]) {
		t.Fatalf("a row kept before: %+v %v", k, err)
	}
	e.open(e.admin, "Quiet Pier")
	e.settle()
	kept, err = e.service.readTitleCredits(t.Context(), []creditsKey{key})
	if k := kept[key]; err != nil || !k.details || k.due(time.Now(), "fr", e.titles["Quiet Pier"]) || !slices.Equal(k.companyIDs, []int{72}) ||
		k.networkIDs == nil {
		t.Errorf("read again: %+v %v", k, err)
	}
	if logo, err := e.service.StudioLogo(t.Context(), "North Studio"); err != nil || logo != "/north.png" {
		t.Errorf("the logo: %q %v", logo, err)
	}
}
