package library

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// fakeReleasesTMDB answers TMDB's lookup by IMDb identifier and the
// release dates of the movies it knows, counting the lookups of each IMDb
// identifier.
type fakeReleasesTMDB struct {
	*httptest.Server
	mu    sync.Mutex
	found map[string]int
}

// releasesOf is a movie TMDB knows: its TMDB identifier and its release
// dates, "YYYY-MM-DD" by TMDB's kind of release (3 in cinemas, 4 digital).
type releasesOf struct {
	tmdb  int
	dates map[int]string
}

func newFakeReleasesTMDB(t *testing.T, known map[string]releasesOf) *fakeReleasesTMDB {
	t.Helper()
	f := &fakeReleasesTMDB{found: map[string]int{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest, _ := strings.CutPrefix(r.URL.Path, "/3/")
		if imdb, ok := strings.CutPrefix(rest, "find/"); ok {
			f.mu.Lock()
			f.found[imdb]++
			f.mu.Unlock()
			results := []map[string]any{}
			if movie, ok := known[imdb]; ok {
				results = append(results, map[string]any{"id": movie.tmdb})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"movie_results": results, "tv_results": []any{}})
			return
		}
		for _, movie := range known {
			if rest == fmt.Sprintf("movie/%d/release_dates", movie.tmdb) {
				var dates []map[string]any
				for kind, day := range movie.dates {
					dates = append(dates, map[string]any{"release_date": day + "T00:00:00.000Z", "type": kind})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"iso_3166_1": "US", "release_dates": dates}}})
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeReleasesTMDB) asked(imdb string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.found[imdb]
}

// releasesAddon serves New, a catalog of movies whose listings give their
// name and premiere date, and their streams.
func releasesAddon() *fakeAddon {
	movie := func(id, name, released string) stremio.Meta {
		return stremio.Meta{ID: id, Type: "movie", Name: name, ReleaseInfo: stremio.Text(released[:4]), Released: released + "T00:00:00.000Z",
			Description: "What " + name + " is about."}
	}
	movies := []stremio.Meta{
		movie("tt901", "Digital Soon", "2026-10-01"),
		movie("tt902", "Digital Out", "2026-08-01"),
		movie("tt903", "Cinema Only", "2026-09-01"),
		movie("tt904", "Long Out", "2026-07-01"),
		movie("tt905", "Unknown To TMDB", "2026-10-10"),
		movie("nb906", "No Identifier", "2026-10-20"),
		movie("tt907", "Classic", "2001-05-05"),
	}
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "cinema", Name: "Cinema", Version: "1", Types: []string{"movie"}, IDPrefixes: []string{"tt", "nb"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "new", Name: "New"}}},
		catalogs: map[string][]stremio.Meta{"movie/new": movies},
		metas:    map[string]stremio.Meta{},
		streams:  map[string][]stremio.Stream{},
	}
	for _, m := range movies {
		addon.metas["movie/"+m.ID] = m
		addon.streams["movie/"+m.ID] = []stremio.Stream{{Name: "1080p", URL: "https://cdn.example/" + m.ID + ".mkv"}}
	}
	return addon
}

// releasesEnv installs releasesAddon for the server and a fake TMDB that
// knows when its movies are out, on a clock the test moves, and returns
// it with the clock's setter and the addon's identifier.
func releasesEnv(t *testing.T) (env, *fakeReleasesTMDB, func(day string), accounts.ID) {
	t.Helper()
	e := newEnv(t)
	// Lookups of release dates answer at once; a slow machine gets time.
	e.service.ratingWait = 30 * time.Second
	var mu sync.Mutex
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	e.service.SetClock(func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	})
	set := func(day string) {
		parsed, err := time.Parse(time.DateOnly, day)
		if err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		now = parsed.Add(12 * time.Hour)
		mu.Unlock()
	}
	f := newFakeReleasesTMDB(t, map[string]releasesOf{
		// Out at home on 20 November.
		"tt901": {901, map[int]string{3: "2026-10-01", 4: "2026-11-20"}},
		// Out at home on 15 October.
		"tt902": {902, map[int]string{3: "2026-08-01", 4: "2026-10-15"}},
		// In cinemas only, since 1 September: out 90 days later, on 30
		// November.
		"tt903": {903, map[int]string{3: "2026-09-01"}},
		// In cinemas only, since 1 July: out on 29 September.
		"tt904": {904, map[int]string{3: "2026-07-01"}},
	})
	e.service.UseTMDB(tmdb.New(f.URL+"/3", f.URL+"/img/", "test", e.users.Settings))
	e.install(addons.Shared(), releasesAddon())
	list, err := e.addons.Addons(t.Context(), addons.Shared())
	if err != nil || len(list) != 1 {
		t.Fatalf("addons: %v %v", list, err)
	}
	return e, f, set, list[0].ID
}

// A movie waits for its first digital release, else for 90 days after it
// came out in cinemas: it is left out of its library until then. A movie
// TMDB does not know, one without an identifier TMDB is asked by, and one
// whose listing tells it came out long ago, are listed; TMDB is not asked
// about the last.
func TestMoviesWaitForTheirDigitalRelease(t *testing.T) {
	t.Parallel()
	e, f, today, _ := releasesEnv(t)
	listed := func() []string {
		t.Helper()
		got := names(e.children(e.member, "New", 0, 100).Items)
		slices.Sort(got)
		return got
	}
	all := []string{"Cinema Only", "Classic", "Digital Out", "Digital Soon", "Long Out", "No Identifier", "Unknown To TMDB"}
	if got := listed(); !slices.Equal(got, all) {
		t.Errorf("without the wait: %v", got)
	}
	// Without the TMDB key, no movie waits.
	e.setting(func(s *accounts.Settings) { s.WaitDigitalRelease = true })
	if got := listed(); !slices.Equal(got, all) {
		t.Errorf("without the TMDB key: %v", got)
	}
	e.setting(func(s *accounts.Settings) { s.TMDBKey = "k3y" })
	if got, want := listed(), []string{"Classic", "Digital Out", "Long Out", "No Identifier", "Unknown To TMDB"}; !slices.Equal(got, want) {
		t.Errorf("on 30 October: %v, want %v", got, want)
	}
	if f.asked("tt907") != 0 {
		t.Errorf("TMDB was asked about a movie out long ago")
	}
	today("2026-11-19")
	if got, want := listed(), []string{"Classic", "Digital Out", "Long Out", "No Identifier", "Unknown To TMDB"}; !slices.Equal(got, want) {
		t.Errorf("on 19 November: %v, want %v", got, want)
	}
	today("2026-11-20")
	if got, want := listed(), []string{"Classic", "Digital Out", "Digital Soon", "Long Out", "No Identifier", "Unknown To TMDB"}; !slices.Equal(got, want) {
		t.Errorf("on 20 November: %v, want %v", got, want)
	}
	today("2026-11-30")
	if got := listed(); !slices.Equal(got, all) {
		t.Errorf("on 30 November: %v, want %v", got, all)
	}
}

// A list of upcoming titles lists the movies waiting for their digital
// release, with the day they are out as their premiere date and first in
// their overview; another list does not. Such a movie, opened, tells the
// same, has no version and does not play until it is out.
func TestUpcomingMoviesAreListedOnlyByListsOfThemAndDoNotPlay(t *testing.T) {
	t.Parallel()
	e, _, today, addon := releasesEnv(t)
	e.setting(func(s *accounts.Settings) { s.TMDBKey = "k3y"; s.WaitDigitalRelease = true })
	movies := []string{"movie/new"}
	upcoming := e.makeList(nil, "Coming", addon, movies, group(smartlists.MatchAll,
		smartlists.Rule{Field: smartlists.FieldHomeRelease, Value: smartlists.HomeUpcoming, Max: new(60.0)}), smartlists.SortName, 0)
	every := e.makeList(nil, "Every", addon, movies, group(smartlists.MatchAll), smartlists.SortName, 0)
	recent := e.makeList(nil, "Recent", addon, movies, group(smartlists.MatchAll,
		smartlists.Rule{Field: smartlists.FieldHomeRelease, Value: smartlists.HomeReleased, Max: new(30.0)}), smartlists.SortName, 0)
	if got := e.settled(e.member, upcoming); !slices.Equal(got, []string{"Cinema Only", "Digital Soon"}) {
		t.Errorf("coming: %v", got)
	}
	if got, want := e.settled(e.member, every), []string{"Classic", "Digital Out", "Long Out", "No Identifier", "Unknown To TMDB"}; !slices.Equal(got, want) {
		t.Errorf("every movie: %v, want %v", got, want)
	}
	if got := e.settled(e.member, recent); !slices.Equal(got, []string{"Digital Out"}) {
		t.Errorf("out in the last 30 days: %v", got)
	}
	var soon Item
	for _, item := range e.listing(e.member, upcoming).Items {
		if item.Name == "Digital Soon" {
			soon = item
		}
	}
	out := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	if soon.PremiereDate == nil || !soon.PremiereDate.Equal(out) || !strings.HasPrefix(soon.Overview, "Out at home on 2026-11-20.") {
		t.Errorf("listed: %v %q", soon.PremiereDate, soon.Overview)
	}
	opened, err := e.service.Item(t.Context(), e.member, soon.ID)
	if err != nil || opened.PremiereDate == nil || !opened.PremiereDate.Equal(out) ||
		opened.Overview != "Out at home on 2026-11-20.\n\nWhat Digital Soon is about." {
		t.Errorf("opened: %v %q %v", opened.PremiereDate, opened.Overview, err)
	}
	if day, held, err := e.service.Upcoming(t.Context(), e.member, soon.ID); err != nil || !held || !day.Equal(out) {
		t.Errorf("upcoming: %v %v %v", day, held, err)
	}
	if versions, err := e.service.Versions(t.Context(), e.member, soon.ID); err != nil || len(versions) != 0 {
		t.Errorf("versions before it is out: %+v %v", versions, err)
	}
	today("2026-11-20")
	if _, held, err := e.service.Upcoming(t.Context(), e.member, soon.ID); err != nil || held {
		t.Errorf("upcoming once out: %v %v", held, err)
	}
	if versions, err := e.service.Versions(t.Context(), e.member, soon.ID); err != nil || len(versions) != 1 {
		t.Errorf("versions once out: %+v %v", versions, err)
	}
	if opened, err := e.service.Item(t.Context(), e.member, soon.ID); err != nil || opened.Overview != "What Digital Soon is about." {
		t.Errorf("opened once out: %q %v", opened.Overview, err)
	}
}
