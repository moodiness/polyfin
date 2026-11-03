package boxset

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// fakeTMDB answers as TMDB's API does a movie's details and the find of
// an IMDb identifier: movies are by TMDB identifier, each in a collection
// or in none (nil), and finds by IMDb identifier; others are 404. It
// refuses the first limited requests for their rate, and records the
// time of each request. While held is set, each request is told on
// arrived, then waits for held to close.
type fakeTMDB struct {
	*httptest.Server
	mu       sync.Mutex
	movies   map[string]*tmdb.CollectionRef
	finds    map[string]int
	limited  int
	requests []time.Time
	paths    []string
	held     chan struct{}
	arrived  chan string
}

func newFakeTMDB(t *testing.T, movies map[string]*tmdb.CollectionRef, finds map[string]int) *fakeTMDB {
	t.Helper()
	f := &fakeTMDB{movies: movies, finds: finds}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests, f.paths = append(f.requests, time.Now()), append(f.paths, r.URL.Path)
		limited := f.limited > 0
		if limited {
			f.limited--
		}
		held, arrived := f.held, f.arrived
		f.mu.Unlock()
		if held != nil {
			arrived <- r.URL.Path
			<-held
		}
		if limited {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		if id, ok := strings.CutPrefix(r.URL.Path, "/find/"); ok {
			if movie, ok := f.finds[id]; ok && r.URL.Query().Get("external_source") == "imdb_id" {
				_ = json.NewEncoder(w).Encode(map[string]any{"movie_results": []any{map[string]any{"id": movie}}, "tv_results": []any{}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"movie_results": []any{}, "tv_results": []any{}})
			return
		}
		id, _ := strings.CutPrefix(r.URL.Path, "/movie/")
		collection, ok := f.movies[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "title": "movie " + id, "belongs_to_collection": collection})
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeTMDB) asked() ([]string, []time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.paths...), append([]time.Time(nil), f.requests...)
}

// newStore returns a store asking fake with key, kept by the clock now,
// starting a lookup at most every interval.
func newStore(t *testing.T, fake *fakeTMDB, key string, now *time.Time, interval time.Duration) *Store {
	t.Helper()
	client := tmdb.New(fake.URL, fake.URL+"/img/", "test", func() accounts.Settings { return accounts.Settings{TMDBKey: key} })
	s := New(migrated.New(t), client, Options{Interval: interval, Now: func() time.Time { return *now }})
	t.Cleanup(s.Close)
	return s
}

// fakeCatalog lists the identifiers of movies, telling each listing on
// listed.
type fakeCatalog struct {
	ids    []map[string]string
	listed chan struct{}
}

func (c fakeCatalog) MovieIDs(_ context.Context, most int) ([]map[string]string, error) {
	if most != PassMost {
		return nil, errors.New("not the bound of box sets")
	}
	select {
	case c.listed <- struct{}{}:
	default:
	}
	return c.ids, nil
}

func settle(t *testing.T, s *Store) {
	t.Helper()
	if err := s.Settled(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// The collection of a movie is not known when first asked for: it is
// looked up in the background, by the movie's TMDB identifier, else its
// IMDb one through TMDB's find, then known, with the collection's name and
// artwork, for every movie of it. A movie in no collection is known as
// such, and so is one TMDB does not know; each is asked again only once
// its answer is old: a month for one TMDB did not know, three for others.
func TestLearnsTheCollectionsOfMoviesInTheBackground(t *testing.T) {
	t.Parallel()
	saga := &tmdb.CollectionRef{ID: 10, Name: "Alpha Collection", PosterPath: "/alpha.jpg", BackdropPath: "/alpha-wide.jpg"}
	fake := newFakeTMDB(t, map[string]*tmdb.CollectionRef{"101": saga, "104": saga, "102": nil}, map[string]int{"tt0000104": 104})
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	s := newStore(t, fake, "k3y", &now, time.Millisecond)
	movies := [][]Key{
		Keys(map[string]string{"Tmdb": "101", "Imdb": "tt0000101"}),
		Keys(map[string]string{"Imdb": "tt0000104"}),
		Keys(map[string]string{"Tmdb": "102"}),
		Keys(map[string]string{"Tmdb": "999"}),
		nil,
	}
	got, err := s.Of(t.Context(), movies)
	if err != nil || len(got) != len(movies) || got[0] != nil || got[1] != nil {
		t.Fatalf("before the lookups: %v %v", got, err)
	}
	settle(t, s)
	got, err = s.Of(t.Context(), movies)
	if err != nil {
		t.Fatal(err)
	}
	want := Collection{ID: 10, Name: "Alpha Collection", PosterPath: "/alpha.jpg", BackdropPath: "/alpha-wide.jpg"}
	if got[0] == nil || *got[0] != want || got[1] == nil || *got[1] != want || got[2] != nil || got[3] != nil || got[4] != nil {
		t.Fatalf("after the lookups: %+v", got)
	}
	paths, _ := fake.asked()
	if want := "/movie/101 /find/tt0000104 /movie/104 /movie/102 /movie/999"; strings.Join(paths, " ") != want {
		t.Fatalf("TMDB was asked %v, want %s", paths, want)
	}
	asked := func(provider, id string) time.Time {
		t.Helper()
		var at time.Time
		if err := s.db.QueryRow(t.Context(), `SELECT asked_at FROM movie_collections WHERE provider = $1 AND id = $2`, provider, id).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at
	}
	first := now
	now = now.Add(31 * 24 * time.Hour)
	if _, err := s.Of(t.Context(), movies); err != nil {
		t.Fatal(err)
	}
	settle(t, s)
	if !asked("tmdb", "999").Equal(now) || !asked("tmdb", "101").Equal(first) || !asked("tmdb", "102").Equal(first) {
		t.Errorf("a month later: 999 asked at %v, 101 at %v, 102 at %v", asked("tmdb", "999"), asked("tmdb", "101"), asked("tmdb", "102"))
	}
	now = first.Add(91 * 24 * time.Hour)
	if _, err := s.Of(t.Context(), movies); err != nil {
		t.Fatal(err)
	}
	settle(t, s)
	if !asked("tmdb", "101").Equal(now) || !asked("imdb", "tt0000104").Equal(now) || !asked("tmdb", "102").Equal(now) {
		t.Errorf("three months later: 101 asked at %v, tt0000104 at %v, 102 at %v", asked("tmdb", "101"), asked("imdb", "tt0000104"), asked("tmdb", "102"))
	}
}

// The lookups respect TMDB's rate: they start at most one every interval,
// and a lookup TMDB refuses for its rate waits as long as Retry-After asks,
// then is made again.
func TestLookupsRespectTMDBsRate(t *testing.T) {
	t.Parallel()
	fake := newFakeTMDB(t, map[string]*tmdb.CollectionRef{"101": {ID: 10, Name: "Alpha Collection"}, "102": nil, "103": nil}, nil)
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	const interval = 300 * time.Millisecond
	s := newStore(t, fake, "k3y", &now, interval)
	fake.mu.Lock()
	fake.limited = 1
	fake.mu.Unlock()
	started := time.Now()
	if _, err := s.Of(t.Context(), [][]Key{{{"tmdb", "101"}}, {{"tmdb", "102"}}, {{"tmdb", "103"}}}); err != nil {
		t.Fatal(err)
	}
	settle(t, s)
	// The refused lookup waited a second, then the next two each waited
	// for their turn.
	if took, least := time.Since(started), time.Second+2*interval; took < least {
		t.Errorf("the lookups took %v, less than %v", took, least)
	}
	if paths, _ := fake.asked(); strings.Join(paths, " ") != "/movie/101 /movie/101 /movie/102 /movie/103" {
		t.Fatalf("TMDB was asked %v", paths)
	}
	if got, err := s.Of(t.Context(), [][]Key{{{"tmdb", "101"}}}); err != nil || got[0] == nil || got[0].ID != 10 {
		t.Errorf("after the refusal: %+v %v", got, err)
	}
}

// Without a TMDB key, no collection is known, and TMDB is never asked.
func TestKnowsNothingWithoutAKey(t *testing.T) {
	t.Parallel()
	fake := newFakeTMDB(t, map[string]*tmdb.CollectionRef{"101": {ID: 10, Name: "Alpha Collection"}}, nil)
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	s := newStore(t, fake, "", &now, time.Millisecond)
	got, err := s.Of(t.Context(), [][]Key{{{"tmdb", "101"}}})
	if s.Enabled() || err != nil || len(got) != 1 || got[0] != nil {
		t.Fatalf("without a key: %v %+v %v", s.Enabled(), got, err)
	}
	settle(t, s)
	if paths, _ := fake.asked(); len(paths) != 0 {
		t.Errorf("TMDB was asked %v", paths)
	}
}

// passStore returns a store asking fake with the key key holds, whose
// passes list catalog, checking every few milliseconds.
func passStore(t *testing.T, fake *fakeTMDB, key *atomic.Value, catalog Catalog) *Store {
	t.Helper()
	client := tmdb.New(fake.URL, fake.URL+"/img/", "test", func() accounts.Settings { return accounts.Settings{TMDBKey: key.Load().(string)} })
	s := New(migrated.New(t), client, Options{Catalog: catalog, Interval: time.Millisecond, PassCheck: 5 * time.Millisecond})
	t.Cleanup(s.Close)
	return s
}

// holding makes fake hold each request until release is called.
func holding(fake *fakeTMDB) (arrived <-chan string, release func()) {
	held, got := make(chan struct{}), make(chan string, 16)
	fake.mu.Lock()
	fake.held, fake.arrived = held, got
	fake.mu.Unlock()
	return got, func() {
		fake.mu.Lock()
		fake.held = nil
		fake.mu.Unlock()
		close(held)
	}
}

// A pass learns the movies of the libraries of movies with no request
// asking for them, as soon as a key is saved, at TMDB's pace: a lookup
// TMDB refuses for its rate waits as long as Retry-After asks.
func TestAPassLearnsTheLibrariesMoviesAhead(t *testing.T) {
	t.Parallel()
	saga := &tmdb.CollectionRef{ID: 10, Name: "Alpha Collection"}
	fake := newFakeTMDB(t, map[string]*tmdb.CollectionRef{"101": saga, "104": saga, "102": nil}, map[string]int{"tt0000104": 104})
	fake.limited = 1
	var key atomic.Value
	key.Store("k3y")
	catalog := fakeCatalog{ids: []map[string]string{{"Tmdb": "101"}, {"Imdb": "tt0000104"}, {"Tmdb": "102", "Imdb": "tt0000102"}, {}},
		listed: make(chan struct{}, 1)}
	started := time.Now()
	s := passStore(t, fake, &key, catalog)
	<-catalog.listed
	movies := [][]Key{{{"tmdb", "101"}}, {{"imdb", "tt0000104"}}, {{"tmdb", "102"}}}
	for {
		settle(t, s)
		known, err := s.kept(t.Context(), movies)
		if err != nil {
			t.Fatal(err)
		}
		if len(known) == len(movies) {
			if known[Key{"tmdb", "101"}].collection == nil || known[Key{"imdb", "tt0000104"}].collection == nil ||
				known[Key{"tmdb", "102"}].collection != nil {
				t.Errorf("learnt %+v", known)
			}
			break
		}
		select {
		case <-t.Context().Done():
			t.Fatal("the pass learnt nothing")
		case <-time.After(10 * time.Millisecond):
		}
	}
	paths, _ := fake.asked()
	if want := "/movie/101 /movie/101 /find/tt0000104 /movie/104 /movie/102"; strings.Join(paths, " ") != want {
		t.Errorf("TMDB was asked %v, want %s", paths, want)
	}
	if took := time.Since(started); took < time.Second {
		t.Errorf("the pass took %v, though TMDB asked to wait a second", took)
	}
}

// A request's lookup goes ahead of a pass's.
func TestARequestGoesAheadOfThePass(t *testing.T) {
	t.Parallel()
	fake := newFakeTMDB(t, map[string]*tmdb.CollectionRef{"101": nil, "102": nil, "103": nil, "200": nil}, nil)
	arrived, release := holding(fake)
	var key atomic.Value
	key.Store("k3y")
	catalog := fakeCatalog{ids: []map[string]string{{"Tmdb": "101"}, {"Tmdb": "102"}, {"Tmdb": "103"}}, listed: make(chan struct{}, 1)}
	s := passStore(t, fake, &key, catalog)
	if first := <-arrived; first != "/movie/101" {
		t.Fatalf("first asked %s", first)
	}
	if _, err := s.Of(t.Context(), [][]Key{{{"tmdb", "200"}}}); err != nil {
		t.Fatal(err)
	}
	release()
	settle(t, s)
	if paths, _ := fake.asked(); strings.Join(paths, " ") != "/movie/101 /movie/200 /movie/102 /movie/103" {
		t.Errorf("TMDB was asked %v", paths)
	}
}

// Once the key is taken off, the pass's lookups stop: those waiting are
// dropped, and TMDB is asked nothing more.
func TestThePassStopsWithoutTheKey(t *testing.T) {
	t.Parallel()
	fake := newFakeTMDB(t, map[string]*tmdb.CollectionRef{"101": nil, "102": nil, "103": nil}, nil)
	arrived, release := holding(fake)
	var key atomic.Value
	key.Store("k3y")
	catalog := fakeCatalog{ids: []map[string]string{{"Tmdb": "101"}, {"Tmdb": "102"}, {"Tmdb": "103"}}, listed: make(chan struct{}, 1)}
	s := passStore(t, fake, &key, catalog)
	<-arrived
	key.Store("")
	release()
	settle(t, s)
	if paths, _ := fake.asked(); strings.Join(paths, " ") != "/movie/101" {
		t.Errorf("TMDB was asked %v", paths)
	}
	if got, err := s.Of(t.Context(), [][]Key{{{"tmdb", "101"}}}); err != nil || got[0] != nil {
		t.Errorf("without the key: %+v %v", got, err)
	}
}
