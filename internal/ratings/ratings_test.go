package ratings

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

const testKey = "test-key"

// request is a request the fake MDBList received.
type request struct {
	at       time.Time
	path     string
	key      string
	ids      []string
	idsTyped []any
}

// fakeMDBList answers MDBList's batch lookup as its documentation shows
// it, for the titles it knows, with its rate limit headers; refuse, when
// set, answers a request instead.
type fakeMDBList struct {
	mu       sync.Mutex
	requests []request
	known    map[string]string
	refuse   func(n int) (status int, header http.Header, body string, ok bool)
	server   *httptest.Server
}

func newFakeMDBList(t *testing.T) *fakeMDBList {
	f := &fakeMDBList{known: map[string]string{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

// media is a title as the batch lookup describes it, with the ratings of
// several sources, those Polyfin reads among them.
func media(imdb string, tmdb int, released string, ratings string) string {
	return fmt.Sprintf(`{"id": 1, "title": "Title", "year": %s, "released": %q, "type": "movie", "score": 80,
		"ids": {"imdb": %q, "trakt": 2, "tmdb": %d, "tvdb": null, "mal": null, "mdblist": "x"}, "ratings": [%s],
		"streams": [], "watch_providers": [], "certification": "PG"}`, released[:4], released, imdb, tmdb, ratings)
}

// allRatings are the ratings of a title MDBList knows well, its IMDb
// rating 8.1, its critics' score 97 and its audience score 92.
const allRatings = `{"source": "imdb", "value": 8.1, "score": 81, "votes": 673852, "url": 99},
	{"source": "metacritic", "value": 87, "score": 87, "votes": 21, "url": "/m"},
	{"source": "metacriticuser", "value": null, "score": null, "votes": null, "url": null},
	{"source": "trakt", "value": 78, "score": 78, "votes": 14033, "url": null},
	{"source": "tomatoes", "value": 97, "score": 97, "votes": 102, "url": "/m"},
	{"source": "popcorn", "value": 92, "score": 92, "votes": 250000, "url": "/m"},
	{"source": "tmdb", "value": 76, "score": 76, "votes": 10114, "url": null},
	{"source": "letterboxd", "value": 4, "score": 80, "votes": 876082, "url": "/f"}`

func (f *fakeMDBList) serve(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []any `json:"ids"`
	}
	data, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(data, &body)
	f.mu.Lock()
	n := len(f.requests)
	req := request{at: time.Now(), path: r.Method + " " + r.URL.Path, key: r.URL.Query().Get("apikey"), idsTyped: body.IDs}
	for _, id := range body.IDs {
		req.ids = append(req.ids, fmt.Sprint(id))
	}
	f.requests = append(f.requests, req)
	refuse := f.refuse
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if refuse != nil {
		if status, header, answer, ok := refuse(n); ok {
			for name, values := range header {
				w.Header()[name] = values
			}
			w.WriteHeader(status)
			_, _ = io.WriteString(w, answer)
			return
		}
	}
	w.Header().Set("X-RateLimit-Limit", "1000")
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(999-n))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(5*time.Hour).Unix(), 10))
	provider := strings.Split(strings.Trim(r.URL.Path, "/"), "/")[0]
	var items []string
	f.mu.Lock()
	for _, id := range req.ids {
		if item, ok := f.known[provider+":"+id]; ok {
			items = append(items, item)
		}
	}
	f.mu.Unlock()
	_, _ = io.WriteString(w, "["+strings.Join(items, ",")+"]")
}

func (f *fakeMDBList) know(provider, id, item string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.known[provider+":"+id] = item
}

func (f *fakeMDBList) asked() []request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.requests)
}

// clock is a clock tests move.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// newService returns a service asking f with key, on a database of its
// own, quick between requests, and what changes its key.
func newService(t *testing.T, f *fakeMDBList, key string) (*Service, func(string)) {
	t.Helper()
	var current atomic.Value
	current.Store(key)
	settings := func() accounts.Settings { return accounts.Settings{MDBListRatingsKey: current.Load().(string)} }
	s := New(migrated.New(t), f.server.URL, "test", slog.New(slog.NewTextHandler(io.Discard, nil)), settings)
	s.gap = 10 * time.Millisecond
	t.Cleanup(s.Close)
	return s, func(key string) { current.Store(key) }
}

// eventually waits for ok, failing after a few seconds.
func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("still not %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func movie(imdb string) Title { return Title{IMDb: imdb} }

func value(score *float64) string {
	if score == nil {
		return "none"
	}
	return strconv.FormatFloat(*score, 'f', -1, 64)
}

func (s Scores) String() string {
	return value(s.IMDb) + " " + value(s.Critics) + " " + value(s.Audience)
}

// Without the server's key, MDBList is never asked, and nothing is kept.
func TestNothingIsAskedWithoutAKey(t *testing.T) {
	t.Parallel()
	f := newFakeMDBList(t)
	f.know("imdb", "tt0000001", media("tt0000001", 1, "1975-06-20", allRatings))
	s, _ := newService(t, f, "")
	for range 3 {
		if kept := s.Kept(t.Context(), []Title{movie("tt0000001")}); kept != nil {
			t.Fatalf("kept without a key: %v", kept)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if asked := f.asked(); len(asked) > 0 {
		t.Fatalf("MDBList was asked without a key: %+v", asked)
	}
	if status := s.Status(); status.Enabled {
		t.Errorf("status without a key: %+v", status)
	}
}

// Titles are asked about in the background, in one request for each kind
// and identifier, with the key; Kept answers at once with what is kept:
// nothing the first time, the scores once MDBList answered, and never asks
// again for what it keeps. Titles without an identifier are not asked
// about, and titles MDBList does not know have no scores.
func TestTitlesAreAskedInBatchesAndKept(t *testing.T) {
	t.Parallel()
	f := newFakeMDBList(t)
	f.know("imdb", "tt0000001", media("tt0000001", 1, "1975-06-20", allRatings))
	f.know("imdb", "tt0000002", media("tt0000002", 2, "2020-01-01", `{"source": "imdb", "value": 6.5}, {"source": "tomatoes", "value": null}`))
	f.know("imdb", "tt0000003", media("tt0000003", 3, "2001-01-01", `{"source": "imdb", "value": 7}, {"source": "popcorn", "value": 55}`))
	f.know("tmdb", "44", media("", 44, "1999-01-01", `{"source": "tomatoes", "value": 40}`))
	s, _ := newService(t, f, testKey)
	titles := []Title{
		movie("tt0000001"), movie("tt0000002"), {}, {Series: true, IMDb: "tt0000003"}, {TMDB: "44"}, movie("tt9999999"),
	}
	if kept := s.Kept(t.Context(), titles); kept != nil {
		t.Fatalf("kept before MDBList answered: %v", kept)
	}
	eventually(t, "answered", func() bool { return len(f.asked()) == 3 && s.Status().Waiting == 0 })
	kept := s.Kept(t.Context(), titles)
	want := []string{"8.1 97 92", "6.5 none none", "none none none", "7 none 55", "none 40 none", "none none none"}
	if len(kept) != len(want) {
		t.Fatalf("kept %v", kept)
	}
	for i, w := range want {
		if got := kept[i].String(); got != w {
			t.Errorf("title %d: %s, want %s", i, got, w)
		}
	}
	// The IMDb rating's votes come with it, where MDBList gives them.
	if votes := kept[0].IMDbVotes; votes == nil || *votes != 673852 || kept[1].IMDbVotes != nil {
		t.Errorf("votes %v and %v, want 673852 and none", votes, kept[1].IMDbVotes)
	}
	asked := f.asked()
	byPath := map[string][]string{}
	for _, r := range asked {
		if r.key != testKey {
			t.Errorf("asked with key %q", r.key)
		}
		byPath[r.path] = append(byPath[r.path], r.ids...)
	}
	wantPaths := map[string][]string{
		"POST /imdb/movie": {"tt0000001", "tt0000002", "tt9999999"},
		"POST /imdb/show":  {"tt0000003"},
		"POST /tmdb/movie": {"44"},
	}
	for path, ids := range wantPaths {
		if !slices.Equal(byPath[path], ids) {
			t.Errorf("%s asked about %v, want %v", path, byPath[path], ids)
		}
	}
	for _, r := range asked {
		if r.path == "POST /tmdb/movie" {
			if _, number := r.idsTyped[0].(float64); !number {
				t.Errorf("TMDB identifiers sent as %T, want numbers", r.idsTyped[0])
			}
		}
	}
	// Kept, they are not asked about again, even by another service on the
	// same database, as after a restart.
	s.Kept(t.Context(), titles)
	again := New(s.db, f.server.URL, "test", s.logger, s.settings)
	t.Cleanup(again.Close)
	if got := again.Kept(t.Context(), titles); len(got) != len(want) || got[0].String() != want[0] ||
		got[0].IMDbVotes == nil || *got[0].IMDbVotes != 673852 {
		t.Errorf("after a restart: %v", got)
	}
	time.Sleep(100 * time.Millisecond)
	if n := len(f.asked()); n != 3 {
		t.Errorf("%d requests, want 3", n)
	}
	if status := s.Status(); !status.Known || status.Limit != 1000 || status.Remaining != 997 || status.Kept != 5 {
		t.Errorf("status %+v", status)
	}
}

// Ratings kept are asked for again after 7 days, those of a title released
// over a year before after 30 days, and a title MDBList did not know after
// 30 days.
func TestRatingsKeptAreAskedAgainOnceOld(t *testing.T) {
	t.Parallel()
	f := newFakeMDBList(t)
	c := &clock{now: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)}
	f.know("imdb", "tt0000001", media("tt0000001", 1, "2026-09-01", allRatings))
	f.know("imdb", "tt0000002", media("tt0000002", 2, "1975-06-20", allRatings))
	s, _ := newService(t, f, testKey)
	s.SetClock(c.Now)
	recent, old, unknown := movie("tt0000001"), movie("tt0000002"), movie("tt0000003")
	asks := func(title Title) int {
		n := len(f.asked())
		s.Kept(t.Context(), []Title{title})
		time.Sleep(50 * time.Millisecond)
		eventually(t, "answered", func() bool { return s.Status().Waiting == 0 })
		return len(f.asked()) - n
	}
	for _, title := range []Title{recent, old, unknown} {
		if n := asks(title); n != 1 {
			t.Fatalf("%v asked %d times at first", title, n)
		}
	}
	for _, step := range []struct {
		after                time.Duration
		recent, old, unknown int
	}{
		{6 * 24 * time.Hour, 0, 0, 0},
		{2 * 24 * time.Hour, 1, 0, 0}, // 8 days
		// The recent title, asked again at 8 days, is 21 days old.
		{21 * 24 * time.Hour, 1, 0, 0}, // 29 days
		{2 * 24 * time.Hour, 0, 1, 1},  // 31 days
	} {
		c.advance(step.after)
		for _, check := range []struct {
			title Title
			want  int
		}{{recent, step.recent}, {old, step.old}, {unknown, step.unknown}} {
			if n := asks(check.title); n != check.want {
				t.Errorf("at %s, %v asked %d times, want %d", c.Now().Format(time.DateOnly), check.title, n, check.want)
			}
		}
	}
}

// A 429 holds every request for as long as its Retry-After says; the
// titles of the request refused are asked about once it is over.
func TestATooManyRequestsHoldsRequestsForRetryAfter(t *testing.T) {
	t.Parallel()
	f := newFakeMDBList(t)
	f.know("imdb", "tt0000001", media("tt0000001", 1, "1975-06-20", allRatings))
	f.refuse = func(n int) (int, http.Header, string, bool) {
		if n > 0 {
			return 0, nil, "", false
		}
		return http.StatusTooManyRequests, http.Header{"Retry-After": {"1"}}, `{"error": "API rate limit exceeded!"}`, true
	}
	s, _ := newService(t, f, testKey)
	s.Kept(t.Context(), []Title{movie("tt0000001")})
	eventually(t, "asked", func() bool { return len(f.asked()) == 1 })
	eventually(t, "held", func() bool { return s.Status().Held == "limit" })
	if status := s.Status(); status.Waiting != 1 || status.HeldUntil.Before(time.Now().Add(500*time.Millisecond)) {
		t.Errorf("status while held: %+v", status)
	}
	eventually(t, "asked again", func() bool { return len(f.asked()) == 2 })
	asked := f.asked()
	if wait := asked[1].at.Sub(asked[0].at); wait < 900*time.Millisecond {
		t.Errorf("asked again %s after the 429, want a second", wait)
	}
	eventually(t, "kept", func() bool {
		kept := s.Kept(t.Context(), []Title{movie("tt0000001")})
		return kept != nil && kept[0].String() == "8.1 97 92"
	})
}

// Once the daily allowance is spent, nothing is asked until the reset
// MDBList tells, whatever the Retry-After; System › Health tells none left.
func TestTheDailyLimitHoldsRequestsUntilTheReset(t *testing.T) {
	t.Parallel()
	f := newFakeMDBList(t)
	reset := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	f.refuse = func(int) (int, http.Header, string, bool) {
		header := http.Header{"Retry-After": {"1"}, "X-Ratelimit-Limit": {"1000"}, "X-Ratelimit-Remaining": {"0"},
			"X-Ratelimit-Reset": {strconv.FormatInt(reset.Unix(), 10)}}
		return http.StatusTooManyRequests, header, `{"error": "Daily API limit exceeded!"}`, true
	}
	s, _ := newService(t, f, testKey)
	s.Kept(t.Context(), []Title{movie("tt0000001")})
	eventually(t, "held", func() bool { return s.Status().Held == "limit" })
	status := s.Status()
	if !status.HeldUntil.Equal(reset) || !status.Known || status.Remaining != 0 || status.Limit != 1000 {
		t.Errorf("status %+v, want held until %s with none left", status, reset)
	}
	s.Kept(t.Context(), []Title{movie("tt0000002")})
	time.Sleep(1500 * time.Millisecond)
	if n := len(f.asked()); n != 1 {
		t.Errorf("%d requests before the reset, want 1", n)
	}
}

// A key MDBList refuses holds the lookups, and a new key asks at once, also
// once the worker waits out the refused key's hold.
func TestARefusedKeyHoldsLookupsUntilAnotherKey(t *testing.T) {
	t.Parallel()
	f := newFakeMDBList(t)
	f.know("imdb", "tt0000001", media("tt0000001", 1, "1975-06-20", allRatings))
	f.refuse = func(n int) (int, http.Header, string, bool) {
		return http.StatusUnauthorized, nil, `{"error": "Invalid API key!"}`, n == 0
	}
	s, setKey := newService(t, f, "wrong-key")
	s.Kept(t.Context(), []Title{movie("tt0000001")})
	eventually(t, "held", func() bool { return s.Status().Held == "refused" })
	// The worker is past its gap after the refusal, waiting out the hour.
	time.Sleep(20 * s.gap)
	setKey(testKey)
	s.Kept(t.Context(), []Title{movie("tt0000001")})
	eventually(t, "kept", func() bool {
		kept := s.Kept(t.Context(), []Title{movie("tt0000001")})
		return kept != nil && kept[0].String() == "8.1 97 92"
	})
	if asked := f.asked(); len(asked) != 2 || asked[1].key != testKey {
		t.Errorf("requests %+v", asked)
	}
}

// Forget drops what is kept of a title, which is asked about again.
func TestForgottenRatingsAreAskedAgain(t *testing.T) {
	t.Parallel()
	f := newFakeMDBList(t)
	f.know("imdb", "tt0000001", media("tt0000001", 1, "1975-06-20", allRatings))
	s, _ := newService(t, f, testKey)
	title := movie("tt0000001")
	s.Kept(t.Context(), []Title{title})
	eventually(t, "kept", func() bool { return s.Kept(t.Context(), []Title{title}) != nil })
	if err := s.Forget(t.Context(), title); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := s.db.QueryRow(t.Context(), "SELECT count(*) FROM title_ratings").Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("%d rows kept after Forget (%v)", rows, err)
	}
	if kept := s.Kept(t.Context(), []Title{title}); kept != nil {
		t.Errorf("kept after Forget: %v", kept)
	}
	eventually(t, "asked again", func() bool { return len(f.asked()) == 2 })
}

// CheckKey asks MDBList's user route: a key it refuses is ErrKeyRefused,
// and one it accepts tells the allowance left.
func TestCheckKeyAsksMDBList(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" || r.URL.Query().Get("apikey") != testKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error": "Invalid API key!"}`)
			return
		}
		_, _ = io.WriteString(w, `{"api_requests": 1000, "api_requests_count": 276, "user_id": 3, "plan": "Free",
			"rate_limit": 1000, "rate_limit_remaining": 724, "rate_limit_reset": 1706918400, "auth_method": "apikey"}`)
	}))
	t.Cleanup(server.Close)
	s := New(migrated.New(t), server.URL, "test", slog.New(slog.NewTextHandler(io.Discard, nil)),
		func() accounts.Settings { return accounts.Settings{MDBListRatingsKey: testKey} })
	t.Cleanup(s.Close)
	if err := s.CheckKey(t.Context(), "wrong"); err != ErrKeyRefused {
		t.Errorf("a wrong key: %v", err)
	}
	if err := s.CheckKey(t.Context(), testKey); err != nil {
		t.Fatal(err)
	}
	s.SetClock(func() time.Time { return time.Unix(1706918400-3600, 0) })
	if status := s.Status(); !status.Known || status.Remaining != 724 || status.Limit != 1000 || status.ResetsAt.Unix() != 1706918400 {
		t.Errorf("status %+v", status)
	}
}
