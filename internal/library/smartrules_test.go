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

// smartToday is the day the tests of smart lists' new rules run on.
var smartToday = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

// episode is a video of a series aired on released, "YYYY-MM-DD".
func episode(series string, season, number int, released string) stremio.Video {
	return stremio.Video{ID: fmt.Sprintf("%s:%d:%d", series, season, number), Season: stremio.Number(season),
		Episode: stremio.Number(number), Released: released + "T00:00:00.000Z"}
}

// airingAddon serves Airing, a series catalog whose listings give no
// episode, and Releases, a movie catalog whose listings give a title's
// name and year alone; descriptions give the rest.
func airingAddon() *fakeAddon {
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "airing", Name: "Airing", Version: "1", Types: []string{"movie", "series"},
			IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs: []stremio.Catalog{
				{Type: "series", ID: "airing", Name: "Airing", Extra: []stremio.Extra{{Name: "skip"}}},
				{Type: "movie", ID: "releases", Name: "Releases", Extra: []stremio.Extra{{Name: "skip"}}},
			}},
		catalogs: map[string][]stremio.Meta{},
		metas:    map[string]stremio.Meta{},
		pageSize: 3,
	}
	add := func(full stremio.Meta) {
		row := stremio.Meta{ID: full.ID, Type: full.Type, Name: full.Name, ReleaseInfo: full.ReleaseInfo}
		catalog := "series/airing"
		if full.Type == "movie" {
			catalog = "movie/releases"
		}
		addon.catalogs[catalog] = append(addon.catalogs[catalog], row)
		addon.metas[full.Type+"/"+full.ID] = full
	}
	series := func(id, name string, videos ...stremio.Video) {
		add(stremio.Meta{ID: id, Type: "series", Name: name, ReleaseInfo: "2019-", Videos: videos})
	}
	// Old Show's latest regular episode aired in 2020; a special aired
	// this month.
	series("tt11", "Old Show",
		episode("tt11", 1, 1, "2019-03-01"), episode("tt11", 2, 1, "2020-03-01"), episode("tt11", 2, 2, "2020-05-01"),
		episode("tt11", 0, 1, "2026-10-01"))
	// Weekly Show's season 2 began in September; its next episode airs
	// next week.
	series("tt12", "Weekly Show",
		episode("tt12", 1, 1, "2025-02-01"), episode("tt12", 2, 1, "2026-09-01"), episode("tt12", 2, 2, "2026-10-27"),
		episode("tt12", 2, 3, "2026-11-05"))
	// Returning Show's season 3 began ten days ago.
	series("tt13", "Returning Show",
		episode("tt13", 1, 1, "2023-01-01"), episode("tt13", 3, 1, "2026-10-20"), episode("tt13", 3, 2, "2026-11-10"))
	// Mid Show's only season began in 2025 and still airs.
	series("tt14", "Mid Show", episode("tt14", 1, 1, "2025-01-01"), episode("tt14", 1, 30, "2026-10-25"))
	// Bare Show lists no episode.
	series("tt15", "Bare Show")

	movie := func(id, name, released, runtime, rating string, genres []string, cast ...string) {
		add(stremio.Meta{ID: id, Type: "movie", Name: name, ReleaseInfo: stremio.Text(released[:4]), Released: released + "T00:00:00.000Z",
			Runtime: stremio.Text(runtime), Genres: genres, Cast: cast, Director: []string{"Dee Director"},
			Extras: &stremio.Extras{Certification: rating}})
	}
	movie("tt21", "Fresh Thriller", "2026-10-20", "118 min", "PG-13", []string{"Thriller"}, "Ann Actor", "Bob Actor")
	movie("tt22", "Old Thriller", "2001-05-05", "100 min", "R", []string{"Thriller"}, "Ann Actor")
	movie("tt23", "Fresh Cartoon", "2026-10-25", "80 min", "G", []string{"Animation"}, "Ann Actor")
	movie("tt24", "Unrated Fresh", "2026-10-28", "", "", []string{"Thriller"}, "Ann Actor")
	return addon
}

// airingEnv installs airingAddon for the server, on smartToday, and
// returns the identifier of the addon.
func airingEnv(t *testing.T) (env, accounts.ID) {
	t.Helper()
	e := newEnv(t)
	e.service.SetClock(func() time.Time { return smartToday })
	e.install(addons.Shared(), airingAddon())
	list, err := e.addons.Addons(t.Context(), addons.Shared())
	if err != nil || len(list) != 1 {
		t.Fatalf("addons: %v %v", list, err)
	}
	return e, list[0].ID
}

// ordered lists a smart list for user until it lists want, in order, as
// the descriptions read in the background come.
func (e env) ordered(user accounts.User, list smartlists.List, want []string) {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		got := names(e.listing(user, list).Items)
		if slices.Equal(got, want) {
			return
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("listing: %v, want %v", got, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A list sorted by the latest episode orders series by their latest
// regular episode aired, specials and episodes to come left out; sorted by
// the latest season premiere, by when their latest season began. A series
// without an episode comes last.
func TestASmartListSortsSeriesByTheirLatestRegularEpisode(t *testing.T) {
	t.Parallel()
	e, addon := airingEnv(t)
	latest := e.makeList(nil, "Latest", addon, []string{"series/airing"}, group(smartlists.MatchAll), smartlists.SortLatestEpisode, 0)
	e.ordered(e.member, latest, []string{"Weekly Show", "Mid Show", "Returning Show", "Old Show", "Bare Show"})

	premieres := e.makeList(nil, "Premieres", addon, []string{"series/airing"}, group(smartlists.MatchAll), smartlists.SortSeasonPremiere, 0)
	e.ordered(e.member, premieres, []string{"Returning Show", "Weekly Show", "Mid Show", "Old Show", "Bare Show"})
}

// Runtime, content rating, person and release date rules judge what the
// descriptions give where the listings do not; exclusions leave out what
// they match; and what a title lacks matches neither a rule nor its
// negation.
func TestSmartListRulesJudgeWhatDescriptionsGive(t *testing.T) {
	t.Parallel()
	e, addon := airingEnv(t)
	movies := []string{"movie/releases"}
	recent := smartlists.Rule{Field: smartlists.FieldReleased, Min: new(-30.0), Max: new(0.0)}
	family := smartlists.Rule{Field: smartlists.FieldContentRating, Value: "PG-13"}
	ann := smartlists.Rule{Field: smartlists.FieldPerson, Value: "ann actor"}
	noCartoons := group(smartlists.MatchAny, smartlists.Rule{Field: smartlists.FieldGenre, Value: "Animation"})

	fresh := e.makeList(nil, "Fresh", addon, movies, group(smartlists.MatchAll, ann, recent, family), smartlists.SortName, 0)
	fresh.Exclude = noCartoons
	fresh, err := e.service.smart.Update(t.Context(), fresh)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.settled(e.member, fresh); !slices.Equal(got, []string{"Fresh Thriller"}) {
		t.Errorf("recent, at most PG-13, with Ann, no cartoon: %v", got)
	}

	for _, c := range []struct {
		name string
		rule smartlists.Rule
		want []string
	}{
		{"rated above PG-13", smartlists.Rule{Field: smartlists.FieldContentRating, Value: "PG-13", Not: true}, []string{"Old Thriller"}},
		{"at least 90 minutes", smartlists.Rule{Field: smartlists.FieldRuntime, Min: new(90.0)}, []string{"Fresh Thriller", "Old Thriller"}},
		{"not at least 90 minutes", smartlists.Rule{Field: smartlists.FieldRuntime, Min: new(90.0), Not: true}, []string{"Fresh Cartoon"}},
		{"not with Bob", smartlists.Rule{Field: smartlists.FieldPerson, Value: "Bob Actor", Not: true},
			[]string{"Fresh Cartoon", "Old Thriller", "Unrated Fresh"}},
		{"directed by Dee", smartlists.Rule{Field: smartlists.FieldPerson, Value: "Dee Director"},
			[]string{"Fresh Cartoon", "Fresh Thriller", "Old Thriller", "Unrated Fresh"}},
		{"not recent", smartlists.Rule{Field: smartlists.FieldReleased, Min: new(-30.0), Max: new(0.0), Not: true}, []string{"Old Thriller"}},
	} {
		list := e.makeList(nil, c.name, addon, movies, group(smartlists.MatchAll, c.rule), smartlists.SortName, 0)
		if got := e.settled(e.member, list); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}

	// Without MDBList, no title has votes: neither a votes rule nor its
	// negation matches, and none waits for them.
	for _, not := range []bool{false, true} {
		list := e.makeList(nil, "Votes", addon, movies, group(smartlists.MatchAll,
			smartlists.Rule{Field: smartlists.FieldVotes, Min: new(1.0), Not: not}), smartlists.SortName, 0)
		preview, err := e.service.PreviewSmartList(t.Context(), e.member, list)
		if err != nil || preview.Listed != 0 || preview.Unknown != 0 {
			t.Errorf("votes (not %v): %+v %v", not, preview, err)
		}
	}
}

// fakeStudiosTMDB answers TMDB's lookup by IMDb identifier and its movie
// details, with the production companies of the movies it knows, counting
// the requests.
type fakeStudiosTMDB struct {
	*httptest.Server
	mu    sync.Mutex
	asked int
}

// studiosOf is a movie TMDB knows: its TMDB identifier and studios.
type studiosOf struct {
	tmdb    int
	studios []string
}

func newFakeStudiosTMDB(t *testing.T, known map[string]studiosOf) *fakeStudiosTMDB {
	t.Helper()
	f := &fakeStudiosTMDB{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.asked++
		f.mu.Unlock()
		rest, _ := strings.CutPrefix(r.URL.Path, "/3/")
		if imdb, ok := strings.CutPrefix(rest, "find/"); ok {
			results := []map[string]any{}
			if movie, ok := known[imdb]; ok {
				results = append(results, map[string]any{"id": movie.tmdb})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"movie_results": results, "tv_results": []any{}})
			return
		}
		for _, movie := range known {
			if rest == fmt.Sprintf("movie/%d", movie.tmdb) {
				companies := []map[string]any{}
				for _, name := range movie.studios {
					companies = append(companies, map[string]any{"name": name})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"production_companies": companies})
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeStudiosTMDB) requests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.asked
}

// A studio rule judges the studios TMDB names, kept once asked; a title
// TMDB does not know matches neither the rule nor its negation.
func TestASmartListOfAStudioAsksTMDBOnce(t *testing.T) {
	t.Parallel()
	e, addon := airingEnv(t)
	f := newFakeStudiosTMDB(t, map[string]studiosOf{
		"tt21": {1, []string{"North Studio", "Other Company"}}, "tt22": {2, []string{"South Studio"}}, "tt23": {3, []string{"North Studio"}},
	})
	e.service.UseTMDB(tmdb.New(f.URL+"/3", f.URL+"/img/", "test", e.users.Settings))
	e.setting(func(s *accounts.Settings) { s.TMDBKey = "k3y" })
	movies := []string{"movie/releases"}
	north := e.makeList(nil, "North", addon, movies, group(smartlists.MatchAll,
		smartlists.Rule{Field: smartlists.FieldStudio, Value: "north studio"}), smartlists.SortName, 0)
	if got := e.settled(e.member, north); !slices.Equal(got, []string{"Fresh Cartoon", "Fresh Thriller"}) {
		t.Errorf("North Studio: %v", got)
	}
	notNorth := e.makeList(nil, "Not North", addon, movies, group(smartlists.MatchAll,
		smartlists.Rule{Field: smartlists.FieldStudio, Value: "North Studio", Not: true}), smartlists.SortName, 0)
	if got := e.settled(e.member, notNorth); !slices.Equal(got, []string{"Old Thriller"}) {
		t.Errorf("not North Studio: %v", got)
	}
	asked := f.requests()
	// Kept, the studios are not asked for again, even once TMDB answers
	// no more.
	f.Close()
	if got := e.settled(e.member, north); !slices.Equal(got, []string{"Fresh Cartoon", "Fresh Thriller"}) {
		t.Errorf("North Studio again: %v", got)
	}
	if asked == 0 || f.requests() != asked {
		t.Errorf("%d requests to TMDB, then %d", asked, f.requests())
	}
}
