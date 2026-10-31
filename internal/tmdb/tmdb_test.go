package tmdb

import (
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
)

// fakeTMDB answers as TMDB's API does for the paths it knows, 404
// otherwise, and counts the requests by path.
type fakeTMDB struct {
	*httptest.Server
	mu       sync.Mutex
	requests map[string]int
	keys     []string
}

func newFakeTMDB(t *testing.T, answers map[string]any) *fakeTMDB {
	t.Helper()
	f := &fakeTMDB{requests: map[string]int{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests[r.URL.Path]++
		f.keys = append(f.keys, r.URL.Query().Get("api_key")+"|"+r.Header.Get("Authorization"))
		f.mu.Unlock()
		if r.URL.Query().Get("api_key") != "k3y" && r.Header.Get("Authorization") != "Bearer ey.token.sig" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		answer, ok := answers[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeTMDB) asked(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[path]
}

func keyed(key string) func() accounts.Settings {
	return func() accounts.Settings { return accounts.Settings{TMDBKey: key} }
}

func TestAnswersAreKeptForADayAndAskedOnce(t *testing.T) {
	t.Parallel()
	fake := newFakeTMDB(t, map[string]any{
		"/3/movie/550/images": map[string]any{"posters": []map[string]any{{"file_path": "/p.jpg", "width": 500, "height": 750, "iso_639_1": "en", "vote_average": 5.2, "vote_count": 3}}},
	})
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var clock sync.Mutex
	c := newClient(fake.URL+"/3/", "https://images.example/t/p/", "test", keyed("k3y"), func() time.Time {
		clock.Lock()
		defer clock.Unlock()
		return now
	})
	advance := func(d time.Duration) {
		clock.Lock()
		now = now.Add(d)
		clock.Unlock()
	}
	for range 2 {
		images, err := c.TitleImages(t.Context(), Movie, "550")
		if err != nil || len(images.Posters) != 1 || images.Posters[0] != (Image{Path: "/p.jpg", Width: 500, Height: 750, Language: "en", Rating: 5.2, Votes: 3}) {
			t.Fatalf("images: %+v %v", images, err)
		}
	}
	if got := fake.asked("/3/movie/550/images"); got != 1 {
		t.Errorf("asked %d times, want once", got)
	}
	// What TMDB does not know is kept too.
	for range 2 {
		if _, err := c.FindByIMDb(t.Context(), Movie, "tt404"); !errors.Is(err, ErrNotFound) {
			t.Errorf("unknown title: %v", err)
		}
	}
	if got := fake.asked("/3/find/tt404"); got != 1 {
		t.Errorf("unknown title asked %d times, want once", got)
	}
	advance(23 * time.Hour)
	_, _ = c.TitleImages(t.Context(), Movie, "550")
	if got := fake.asked("/3/movie/550/images"); got != 1 {
		t.Errorf("within a day: asked %d times", got)
	}
	advance(2 * time.Hour)
	_, _ = c.TitleImages(t.Context(), Movie, "550")
	if got := fake.asked("/3/movie/550/images"); got != 2 {
		t.Errorf("after a day: asked %d times, want twice", got)
	}
	if c.ImageURL("/p.jpg", "w342") != "https://images.example/t/p/w342/p.jpg" {
		t.Errorf("image URL: %s", c.ImageURL("/p.jpg", "w342"))
	}
}

func TestKeysAndTokensAreSentAsTMDBTakesThem(t *testing.T) {
	t.Parallel()
	fake := newFakeTMDB(t, map[string]any{"/3/authentication": map[string]any{"success": true}})
	none := New(fake.URL+"/3", Images, "test", keyed(""))
	if none.Enabled() {
		t.Error("enabled without a key")
	}
	if _, err := none.TitleImages(t.Context(), Movie, "1"); !errors.Is(err, ErrNoKey) || len(fake.keys) != 0 {
		t.Errorf("without a key: %v, %d requests", err, len(fake.keys))
	}
	c := New(fake.URL+"/3", Images, "test", keyed(""))
	if err := c.CheckKey(t.Context(), "k3y"); err != nil {
		t.Errorf("API key: %v", err)
	}
	if err := c.CheckKey(t.Context(), "ey.token.sig"); err != nil {
		t.Errorf("read access token: %v", err)
	}
	if err := c.CheckKey(t.Context(), "wrong"); !errors.Is(err, ErrKeyRefused) {
		t.Errorf("refused key: %v", err)
	}
	if want := []string{"k3y|", "|Bearer ey.token.sig", "wrong|"}; strings.Join(fake.keys, ",") != strings.Join(want, ",") {
		t.Errorf("sent %q, want %q", fake.keys, want)
	}
	// An error never holds the key, which the URL carries.
	gone := New("http://127.0.0.1:1/3", Images, "test", keyed("s3cret"))
	if _, err := gone.TitleImages(t.Context(), Movie, "1"); err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Errorf("unreachable TMDB: %v", err)
	}
	refused := New(fake.URL+"/3", Images, "test", keyed("s3cret"))
	if _, err := refused.TitleImages(t.Context(), Movie, "1"); !errors.Is(err, ErrKeyRefused) {
		t.Errorf("refused key: %v", err)
	}
}

func TestPersonPhotosAreThoseOfTheMostPopularNamesake(t *testing.T) {
	t.Parallel()
	fake := newFakeTMDB(t, map[string]any{
		"/3/search/person": map[string]any{"results": []map[string]any{
			{"id": 1, "name": "Ann Leeds", "popularity": 50},
			{"id": 2, "name": "ann lee", "popularity": 3},
			{"id": 3, "name": "Ann Lee", "popularity": 9},
		}},
		"/3/person/3/images": map[string]any{"profiles": []map[string]any{{"file_path": "/ann.jpg", "width": 300, "height": 450}}},
	})
	c := New(fake.URL+"/3", Images, "test", keyed("k3y"))
	photos, err := c.PersonPhotos(t.Context(), " Ann Lee ")
	if err != nil || len(photos) != 1 || photos[0].Path != "/ann.jpg" {
		t.Errorf("photos: %+v %v", photos, err)
	}
	if _, err := c.PersonPhotos(t.Context(), "Nobody Known"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown person: %v", err)
	}
}

func TestAtMostFourRequestsGoToTMDBAtOnce(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	arrived := make(chan struct{}, 16)
	var inFlight, most atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		for {
			m := most.Load()
			if n <= m || most.CompareAndSwap(m, n) {
				break
			}
		}
		arrived <- struct{}{}
		<-release
		inFlight.Add(-1)
		_ = json.NewEncoder(w).Encode(map[string]any{"posters": []any{}})
	}))
	t.Cleanup(server.Close)
	c := New(server.URL+"/3", Images, "test", keyed("k3y"))
	const asked = 6
	var done sync.WaitGroup
	for i := range asked {
		done.Go(func() {
			if _, err := c.TitleImages(t.Context(), Movie, string(rune('1'+i))); err != nil {
				t.Errorf("request %d: %v", i, err)
			}
		})
	}
	for range maxRequests {
		<-arrived
	}
	// The four requests under way hold every place: the others wait for
	// one, and cannot have reached TMDB.
	if len(c.slots) != maxRequests {
		t.Errorf("%d places held, want %d", len(c.slots), maxRequests)
	}
	close(release)
	done.Wait()
	if got := most.Load(); got > maxRequests {
		t.Errorf("%d requests at once, want at most %d", got, maxRequests)
	}
	if len(arrived) != asked-maxRequests {
		t.Errorf("%d more requests, want %d", len(arrived), asked-maxRequests)
	}
}

// A movie's release dates are the earliest of each kind in any country:
// its first digital or physical release, its first in cinemas, limited or
// wide, and its first of any kind.
func TestReleaseDatesAreTheEarliestOfEachKind(t *testing.T) {
	t.Parallel()
	date := func(day string, kind int) map[string]any {
		return map[string]any{"release_date": day + "T00:00:00.000Z", "type": kind, "certification": ""}
	}
	fake := newFakeTMDB(t, map[string]any{
		"/3/movie/7/release_dates": map[string]any{"id": 7, "results": []map[string]any{
			{"iso_3166_1": "US", "release_dates": []any{date("2026-03-06", 3), date("2026-05-19", 4), date("2026-06-30", 5)}},
			{"iso_3166_1": "FR", "release_dates": []any{date("2026-02-14", 1), date("2026-03-04", 2), date("2026-05-12", 5)}},
		}},
		"/3/movie/8/release_dates": map[string]any{"id": 8, "results": []map[string]any{
			{"iso_3166_1": "DE", "release_dates": []any{date("2026-09-10", 3)}},
		}},
	})
	c := New(fake.URL+"/3", Images, "test", keyed("k3y"))
	day := func(text string) time.Time {
		parsed, _ := time.Parse(time.DateOnly, text)
		return parsed
	}
	got, err := c.ReleaseDates(t.Context(), "7")
	if want := (Releases{Home: day("2026-05-12"), Theatrical: day("2026-03-04"), First: day("2026-02-14")}); err != nil || got != want {
		t.Errorf("movie 7: %+v %v, want %+v", got, err, want)
	}
	got, err = c.ReleaseDates(t.Context(), "8")
	if want := (Releases{Theatrical: day("2026-09-10"), First: day("2026-09-10")}); err != nil || got != want {
		t.Errorf("movie 8: %+v %v, want %+v", got, err, want)
	}
	if _, err := c.ReleaseDates(t.Context(), "9"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown movie: %v", err)
	}
}
