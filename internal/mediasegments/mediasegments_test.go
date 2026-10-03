package mediasegments

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/testdb"
)

// reply is a database's answer: a status and a JSON body.
type reply struct {
	status int
	body   string
}

// databases stands in for TheIntroDB (/media) and IntroDB (/segments): it
// answers each with the reply set for it, and records what each was asked.
type databases struct {
	mu      sync.Mutex
	replies map[string]reply
	asked   map[string][]url.Values
	url     string
}

func newDatabases(t *testing.T) *databases {
	t.Helper()
	d := &databases{replies: map[string]reply{}, asked: map[string][]url.Values{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		d.asked[r.URL.Path] = append(d.asked[r.URL.Path], r.URL.Query())
		answer, ok := d.replies[r.URL.Path]
		d.mu.Unlock()
		if !ok {
			answer = reply{status: http.StatusNotFound, body: `{"error":"media not found"}`}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(answer.status)
		_, _ = io.WriteString(w, answer.body)
	}))
	t.Cleanup(server.Close)
	d.url = server.URL
	return d
}

func (d *databases) reply(path string, status int, body string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.replies[path] = reply{status, body}
}

// calls returns how many times each database was asked: TheIntroDB, then
// IntroDB.
func (d *databases) calls() (theIntroDB, introDB int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.asked["/media"]), len(d.asked["/segments"])
}

func (d *databases) last(path string) url.Values {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.asked[path]) == 0 {
		return nil
	}
	return d.asked[path][len(d.asked[path])-1]
}

// clock is a time tests move forward.
type clock struct{ now time.Time }

func (c *clock) advance(d time.Duration) { c.now = c.now.Add(d) }

// service returns a service asking the fake databases in the order named,
// on a fresh database, at a time the test controls.
func service(t *testing.T, d *databases, names ...string) (*Service, *clock) {
	t.Helper()
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	var sources []Source
	for _, name := range names {
		sources = append(sources, Source{Name: name, URL: d.url})
	}
	s := New(pool, sources, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	c := &clock{now: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)}
	s.now = func() time.Time { return c.now }
	return s, c
}

var episode = Title{Item: accounts.ID{1}, IMDb: "tt0903747", TMDB: "1396", Season: 1, Episode: 2, Runtime: time.Hour}

func at(seconds float64) time.Duration { return time.Duration(seconds * float64(time.Second)) }

func TestSegmentsMergeTheDatabasesByPreference(t *testing.T) {
	d := newDatabases(t)
	// TheIntroDB has an intro from the start, credits to the end, and
	// says there is no recap and no preview.
	d.reply("/media", http.StatusOK, `{"tmdb_id":1396,"type":"tv","season":1,"episode":2,
		"intro":[{"start_ms":null,"end_ms":30500}],
		"recap":[{"start_ms":null,"end_ms":0}],
		"credits":[{"start_ms":3431000,"end_ms":null}],
		"preview":[{"start_ms":0,"end_ms":null}]}`)
	// IntroDB has a recap, an intro and an outro of its own, and a scene
	// after the credits.
	d.reply("/segments", http.StatusOK, `{"imdb_id":"tt0903747","media_type":"tv","is_movie":false,"season":1,"episode":2,
		"intro":{"start_ms":40000,"end_ms":70000,"start_sec":40,"end_sec":70,"confidence":1,"submission_count":3},
		"recap":{"start_ms":0,"end_ms":25000,"start_sec":0,"end_sec":25,"confidence":1,"submission_count":1},
		"outro":{"start_ms":3400000,"end_ms":3500000,"start_sec":3400,"end_sec":3500,"confidence":1,"submission_count":1},
		"post_credits":{"start_ms":3550000,"end_ms":3590000,"start_sec":3550,"end_sec":3590,"confidence":1,"submission_count":1}}`)

	s, _ := service(t, d, TheIntroDB, IntroDB)
	got := s.Segments(t.Context(), episode)
	want := []Segment{
		{Type: Recap, Start: 0, End: at(25)},
		{Type: Intro, Start: 0, End: at(30.5)},
		{Type: Outro, Start: at(3431), End: time.Hour},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	// An episode is asked by its series' identifiers and its numbers;
	// TheIntroDB by TMDB identifier, which it prefers, and runtime.
	if q := d.last("/media"); q.Get("tmdb_id") != "1396" || q.Has("imdb_id") || q.Get("season") != "1" || q.Get("episode") != "2" ||
		q.Get("duration_ms") != "3600000" {
		t.Errorf("TheIntroDB was asked %v", q)
	}
	if q := d.last("/segments"); q.Get("imdb_id") != "tt0903747" || q.Get("season") != "1" || q.Get("episode") != "2" || q.Has("is_movie") {
		t.Errorf("IntroDB was asked %v", q)
	}

	// Preferring IntroDB, its intro and outro win, from the same answers.
	s, _ = service(t, d, IntroDB, TheIntroDB)
	want = []Segment{
		{Type: Recap, Start: 0, End: at(25)},
		{Type: Intro, Start: at(40), End: at(70)},
		{Type: Outro, Start: at(3400), End: at(3500)},
	}
	if got := s.Segments(t.Context(), episode); !slices.Equal(got, want) {
		t.Errorf("preferring IntroDB: got %+v, want %+v", got, want)
	}
}

func TestMoviesAreAskedAsMovies(t *testing.T) {
	d := newDatabases(t)
	d.reply("/media", http.StatusOK, `{"tmdb_id":1,"type":"movie","credits":[{"start_ms":6000000,"end_ms":null}],
		"preview":[{"start_ms":100000,"end_ms":160000}]}`)
	d.reply("/segments", http.StatusOK, `{"imdb_id":"tt0111161","media_type":"movie","is_movie":true,"season":0,"episode":0,
		"intro":null,"recap":null,"outro":{"start_ms":5900000,"end_ms":6100000},"post_credits":null}`)
	s, _ := service(t, d, TheIntroDB, IntroDB)
	// Without a TMDB identifier, TheIntroDB is asked by IMDb identifier.
	// The runtime is unknown: the credits lasting until the end cannot be
	// placed, and IntroDB's outro, which ends, takes their place.
	movie := Title{Item: accounts.ID{2}, IMDb: "tt0111161"}
	want := []Segment{
		{Type: Preview, Start: at(100), End: at(160)},
		{Type: Outro, Start: at(5900), End: at(6100)},
	}
	if got := s.Segments(t.Context(), movie); !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if q := d.last("/media"); q.Get("imdb_id") != "tt0111161" || q.Has("tmdb_id") || q.Has("season") || q.Has("duration_ms") {
		t.Errorf("TheIntroDB was asked %v", q)
	}
	if q := d.last("/segments"); q.Get("imdb_id") != "tt0111161" || q.Get("is_movie") != "true" || q.Has("season") {
		t.Errorf("IntroDB was asked %v", q)
	}
}

func TestAnswersAreKeptUntilTheyExpire(t *testing.T) {
	d := newDatabases(t)
	d.reply("/media", http.StatusOK, `{"tmdb_id":1396,"type":"tv","season":1,"episode":2,"intro":[{"start_ms":1000,"end_ms":31000}]}`)
	s, c := service(t, d, TheIntroDB, IntroDB)
	first := s.Segments(t.Context(), episode)
	if len(first) != 1 {
		t.Fatalf("segments %+v", first)
	}
	// IntroDB knew nothing (404); TheIntroDB found an intro.
	if a, b := d.calls(); a != 1 || b != 1 {
		t.Fatalf("asked %d and %d times", a, b)
	}
	if again := s.Segments(t.Context(), episode); !slices.Equal(again, first) {
		t.Errorf("kept segments %+v, first %+v", again, first)
	}
	if a, b := d.calls(); a != 1 || b != 1 {
		t.Errorf("a kept answer was asked again: %d and %d times", a, b)
	}
	// A day later, IntroDB is asked again, not TheIntroDB.
	c.advance(24*time.Hour + time.Minute)
	s.Segments(t.Context(), episode)
	if a, b := d.calls(); a != 1 || b != 2 {
		t.Errorf("after a day: asked %d and %d times", a, b)
	}
	// After 30 days, TheIntroDB is asked again.
	c.advance(29 * 24 * time.Hour)
	s.Segments(t.Context(), episode)
	if a, _ := d.calls(); a != 2 {
		t.Errorf("after 30 days: TheIntroDB asked %d times", a)
	}
}

func TestEmptyAnswersCountAsNothingFound(t *testing.T) {
	d := newDatabases(t)
	// IntroDB answers null segments for titles it has no data on.
	d.reply("/segments", http.StatusOK, `{"imdb_id":"tt0903747","media_type":"tv","is_movie":false,"season":1,"episode":2,
		"intro":null,"recap":null,"outro":null,"post_credits":null}`)
	s, c := service(t, d, TheIntroDB, IntroDB)
	if got := s.Segments(t.Context(), episode); len(got) != 0 {
		t.Errorf("segments %+v", got)
	}
	c.advance(23 * time.Hour)
	s.Segments(t.Context(), episode)
	if a, b := d.calls(); a != 1 || b != 1 {
		t.Errorf("nothing found was asked again within a day: %d and %d times", a, b)
	}
}

func TestAFailingDatabaseLeavesTheOtherAnswering(t *testing.T) {
	d := newDatabases(t)
	d.reply("/media", http.StatusTooManyRequests, `{"error":"Too Many Requests"}`)
	d.reply("/segments", http.StatusOK, `{"imdb_id":"tt0903747","intro":{"start_ms":40000,"end_ms":70000},"recap":null,"outro":null}`)
	s, c := service(t, d, TheIntroDB, IntroDB)
	want := []Segment{{Type: Intro, Start: at(40), End: at(70)}}
	if got := s.Segments(t.Context(), episode); !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	// The refusal is not repeated within the hour.
	c.advance(59 * time.Minute)
	s.Segments(t.Context(), episode)
	if a, b := d.calls(); a != 1 || b != 1 {
		t.Errorf("within the hour: asked %d and %d times", a, b)
	}
	// After it, only the database that failed is asked again; it answers
	// now, and its intro is preferred.
	d.reply("/media", http.StatusOK, `{"intro":[{"start_ms":null,"end_ms":20000}]}`)
	c.advance(2 * time.Minute)
	want = []Segment{{Type: Intro, Start: 0, End: at(20)}}
	if got := s.Segments(t.Context(), episode); !slices.Equal(got, want) {
		t.Errorf("after the hour: got %+v, want %+v", got, want)
	}
	if a, b := d.calls(); a != 2 || b != 1 {
		t.Errorf("after the hour: asked %d and %d times", a, b)
	}
	// When it fails again once its answer expired, what it found before
	// is still used.
	d.reply("/media", http.StatusInternalServerError, `{"error":"down"}`)
	c.advance(31 * 24 * time.Hour)
	if got := s.Segments(t.Context(), episode); !slices.Equal(got, want) {
		t.Errorf("after a failure: got %+v, want %+v", got, want)
	}
	// A body that is not the expected JSON is a failure too.
	d.reply("/media", http.StatusOK, `<html>maintenance</html>`)
	c.advance(2 * time.Hour)
	if got := s.Segments(t.Context(), episode); !slices.Equal(got, want) {
		t.Errorf("after a broken answer: got %+v, want %+v", got, want)
	}
}

func TestNothingIsAskedWithoutDatabasesOrIdentifiers(t *testing.T) {
	d := newDatabases(t)
	s, _ := service(t, d)
	if got := s.Segments(t.Context(), episode); got != nil {
		t.Errorf("disabled: %+v", got)
	}
	s, _ = service(t, d, TheIntroDB, IntroDB)
	for _, title := range []Title{
		{Item: accounts.ID{3}, IMDb: "tt12", TMDB: "abc"},
		{Item: accounts.ID{4}, IMDb: "tt0903747", Season: 0, Episode: 3},
	} {
		if got := s.Segments(t.Context(), title); got != nil {
			t.Errorf("%+v: %+v", title, got)
		}
	}
	// IntroDB knows titles by IMDb identifier only.
	s.Segments(t.Context(), Title{Item: accounts.ID{5}, TMDB: "550"})
	if a, b := d.calls(); a != 1 || b != 0 {
		t.Errorf("asked %d and %d times", a, b)
	}
}
