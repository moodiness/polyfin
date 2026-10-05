package mediasegments

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/testdb"
)

// reply is a database's answer: a status, a JSON body and, if any, a
// Retry-After header.
type reply struct {
	status     int
	body       string
	retryAfter string
}

// The paths the fake databases answer on.
const (
	theIntroDBPath   = "/media"
	introDBPath      = "/segments"
	publicMetaDBPath = "/api/external/skips"
)

// databases stands in for TheIntroDB (/media), IntroDB (/segments) and
// PublicMetaDB (/api/external/skips): it answers each with the reply set
// for it, or 401 to a key it refuses, and records what each was asked and
// with which Authorization header.
type databases struct {
	mu       sync.Mutex
	replies  map[string]reply
	refusing map[string]string
	asked    map[string][]url.Values
	auth     map[string][]string
	url      string
}

func newDatabases(t *testing.T) *databases {
	t.Helper()
	d := &databases{replies: map[string]reply{}, refusing: map[string]string{}, asked: map[string][]url.Values{}, auth: map[string][]string{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		d.asked[r.URL.Path] = append(d.asked[r.URL.Path], r.URL.Query())
		d.auth[r.URL.Path] = append(d.auth[r.URL.Path], r.Header.Get("Authorization"))
		answer, ok := d.replies[r.URL.Path]
		if refused := d.refusing[r.URL.Path]; refused != "" && r.Header.Get("Authorization") == "Bearer "+refused {
			answer, ok = reply{status: http.StatusUnauthorized, body: `{"error":"Unauthorized"}`}, true
		}
		d.mu.Unlock()
		if !ok {
			answer = reply{status: http.StatusNotFound, body: `{"error":"media not found"}`}
		}
		w.Header().Set("Content-Type", "application/json")
		if answer.retryAfter != "" {
			w.Header().Set("Retry-After", answer.retryAfter)
		}
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
	d.replies[path] = reply{status: status, body: body}
}

// limit makes the database at path answer 429 with a Retry-After.
func (d *databases) limit(path, retryAfter string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.replies[path] = reply{status: http.StatusTooManyRequests, body: `{"error":"Too Many Requests"}`, retryAfter: retryAfter}
}

// refuse makes the database at path answer 401 to key.
func (d *databases) refuse(path, key string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.refusing[path] = key
}

// authorizations returns the Authorization headers the database at path
// was asked with, one a request.
func (d *databases) authorizations(path string) []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.auth[path])
}

// calls returns how many times each database was asked: TheIntroDB, then
// IntroDB.
func (d *databases) calls() (theIntroDB, introDB int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.asked[theIntroDBPath]), len(d.asked[introDBPath])
}

// publicMetaDB returns the Authorization headers PublicMetaDB was asked
// with, one a request.
func (d *databases) publicMetaDB() []string {
	return d.authorizations(publicMetaDBPath)
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
	s := New(pool, sources, "test", slog.New(slog.NewTextHandler(io.Discard, nil)), func() accounts.Settings { return accounts.Settings{} })
	c := &clock{now: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)}
	s.now = func() time.Time { return c.now }
	return s, c
}

// useKey saves key as the server's PublicMetaDB key, empty for none.
func useKey(s *Service, key string) {
	useSettings(s, accounts.Settings{PublicMetaDBKey: key})
}

// useSettings saves settings as the server's.
func useSettings(s *Service, settings accounts.Settings) {
	s.settings = func() accounts.Settings { return settings }
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

const goodKey = "pm-Good4mN2pQrS7tUvWx3yZaB4cD5eF6gH7iJ8kL9mN0oP1qR2sT3uV4wXyZ5aB"

// episodeRecords are PublicMetaDB's records for the episode, one a contributor
// and release. The physical release's gives every segment; of the streaming
// releases', s2 and s1 give three (s2 the more recent), s3 one, and s0 none.
var episodeRecords = []string{
	`{"id":"p1","tmdb_id":1396,"media_type":"tv","season":1,"episode":2,"source":"physical","created":"2026-01-01T00:00:00Z",
		"intro_start_ms":1000,"intro_end_ms":2000,"credits_start_ms":3000000,"credits_end_ms":3100000,
		"recap_start_ms":0,"recap_end_ms":500,"preview_start_ms":3500000,"preview_end_ms":3550000}`,
	`{"id":"s2","tmdb_id":1396,"media_type":"tv","season":1,"episode":2,"source":"streaming","created":"2025-06-01T00:00:00Z",
		"intro_start_ms":15000,"intro_end_ms":62000,"credits_start_ms":null,"credits_end_ms":null,
		"recap_start_ms":0,"recap_end_ms":15000,"preview_start_ms":3540000,"preview_end_ms":3570000}`,
	`{"id":"s3","tmdb_id":1396,"media_type":"tv","season":1,"episode":2,"source":"streaming","created":"2026-02-01T00:00:00Z",
		"intro_start_ms":20000,"intro_end_ms":50000,"credits_start_ms":null,"credits_end_ms":null,
		"recap_start_ms":null,"recap_end_ms":null,"preview_start_ms":null,"preview_end_ms":null}`,
	`{"id":"s1","tmdb_id":1396,"media_type":"tv","season":1,"episode":2,"source":"streaming","created":"2024-01-01T00:00:00Z",
		"intro_start_ms":10000,"intro_end_ms":40000,"credits_start_ms":null,"credits_end_ms":null,
		"recap_start_ms":0,"recap_end_ms":10000,"preview_start_ms":3500000,"preview_end_ms":3560000}`,
	`{"id":"s0","tmdb_id":1396,"media_type":"tv","season":1,"episode":2,"source":"streaming","created":"2026-09-01T00:00:00Z",
		"intro_start_ms":null,"intro_end_ms":null,"credits_start_ms":null,"credits_end_ms":null,
		"recap_start_ms":null,"recap_end_ms":null,"preview_start_ms":null,"preview_end_ms":null}`,
}

// skipPage is PublicMetaDB's answer listing records.
func skipPage(records ...string) string {
	return `{"items":[` + strings.Join(records, ",") + `],"total":` + strconv.Itoa(len(records)) + `,"page":1,"perPage":200,"totalPages":1}`
}

func TestPublicMetaDBJoinsTheOtherDatabasesByPreference(t *testing.T) {
	d := newDatabases(t)
	d.reply(theIntroDBPath, http.StatusOK, `{"tmdb_id":1396,"type":"tv","season":1,"episode":2,
		"intro":[{"start_ms":null,"end_ms":30500}],"recap":[{"start_ms":null,"end_ms":0}],
		"credits":[{"start_ms":3431000,"end_ms":null}],"preview":[{"start_ms":0,"end_ms":null}]}`)
	d.reply(publicMetaDBPath, http.StatusOK, skipPage(episodeRecords...))

	s, _ := service(t, d, TheIntroDB, IntroDB, PublicMetaDB)
	useKey(s, goodKey)
	// TheIntroDB's intro and credits are preferred; PublicMetaDB brings
	// the recap and the preview TheIntroDB says there are none of, from
	// the fullest record of a streaming release.
	want := []Segment{
		{Type: Recap, Start: 0, End: at(15)},
		{Type: Intro, Start: 0, End: at(30.5)},
		{Type: Outro, Start: at(3431), End: time.Hour},
		{Type: Preview, Start: at(3540), End: at(3570)},
	}
	if got := s.Segments(t.Context(), episode); !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if q := d.last(publicMetaDBPath); q.Get("tmdb_id") != "1396" || q.Get("media_type") != "tv" || q.Get("season") != "1" || q.Get("episode") != "2" {
		t.Errorf("PublicMetaDB was asked %v", q)
	}
	if auth := d.publicMetaDB(); !slices.Equal(auth, []string{"Bearer " + goodKey}) {
		t.Errorf("PublicMetaDB was asked with %q", auth)
	}

	// Preferring PublicMetaDB, its intro wins too; its record has no
	// credits, which TheIntroDB keeps giving. The records listed in
	// another order give the same segments.
	reversed := slices.Clone(episodeRecords)
	slices.Reverse(reversed)
	d.reply(publicMetaDBPath, http.StatusOK, skipPage(reversed...))
	s, _ = service(t, d, PublicMetaDB, TheIntroDB, IntroDB)
	useKey(s, goodKey)
	want = []Segment{
		{Type: Recap, Start: 0, End: at(15)},
		{Type: Intro, Start: at(15), End: at(62)},
		{Type: Outro, Start: at(3431), End: time.Hour},
		{Type: Preview, Start: at(3540), End: at(3570)},
	}
	if got := s.Segments(t.Context(), episode); !slices.Equal(got, want) {
		t.Errorf("preferring PublicMetaDB: got %+v, want %+v", got, want)
	}
}

func TestPublicMetaDBIsAskedOnlyWithAKey(t *testing.T) {
	d := newDatabases(t)
	d.reply(theIntroDBPath, http.StatusOK, `{"intro":[{"start_ms":null,"end_ms":30500}]}`)
	d.reply(publicMetaDBPath, http.StatusOK, skipPage(episodeRecords...))
	s, _ := service(t, d, TheIntroDB, PublicMetaDB)
	theIntroDBOnly := []Segment{{Type: Intro, Start: 0, End: at(30.5)}}
	if got := s.Segments(t.Context(), episode); !slices.Equal(got, theIntroDBOnly) {
		t.Errorf("without a key: got %+v", got)
	}
	if auth := d.publicMetaDB(); len(auth) != 0 {
		t.Fatalf("PublicMetaDB was asked without a key: %q", auth)
	}
	// The title's answers were kept before the key was saved: PublicMetaDB
	// was not asked, which is not that it found nothing. It is asked once
	// a key is saved, without TheIntroDB being asked again.
	useKey(s, goodKey)
	withPublicMetaDB := []Segment{
		{Type: Recap, Start: 0, End: at(15)},
		{Type: Intro, Start: 0, End: at(30.5)},
		{Type: Preview, Start: at(3540), End: at(3570)},
	}
	if got := s.Segments(t.Context(), episode); !slices.Equal(got, withPublicMetaDB) {
		t.Errorf("once a key is saved: got %+v, want %+v", got, withPublicMetaDB)
	}
	s.Segments(t.Context(), episode)
	if a, _ := d.calls(); a != 1 || !slices.Equal(d.publicMetaDB(), []string{"Bearer " + goodKey}) {
		t.Errorf("asked TheIntroDB %d times, PublicMetaDB with %q", a, d.publicMetaDB())
	}
	// Once the key is removed, PublicMetaDB's segments go and it is not
	// asked; saved again, they come back from what it answered.
	useKey(s, "")
	if got := s.Segments(t.Context(), episode); !slices.Equal(got, theIntroDBOnly) {
		t.Errorf("key removed: got %+v", got)
	}
	useKey(s, goodKey)
	if got := s.Segments(t.Context(), episode); !slices.Equal(got, withPublicMetaDB) || len(d.publicMetaDB()) != 1 {
		t.Errorf("key saved again: got %+v, PublicMetaDB asked %d times", got, len(d.publicMetaDB()))
	}
}

func TestPublicMetaDBKnowsTitlesByTMDBIdentifier(t *testing.T) {
	d := newDatabases(t)
	d.reply(publicMetaDBPath, http.StatusOK, skipPage(`{"id":"m1","tmdb_id":550,"media_type":"movie","source":"streaming",
		"created":"2025-01-01T00:00:00Z","intro_start_ms":null,"intro_end_ms":null,
		"credits_start_ms":7900000,"credits_end_ms":8300000,"recap_start_ms":null,"recap_end_ms":null,
		"preview_start_ms":null,"preview_end_ms":null}`))
	s, _ := service(t, d, IntroDB, PublicMetaDB)
	useKey(s, goodKey)
	movie := Title{Item: accounts.ID{2}, IMDb: "tt0137523", TMDB: "550"}
	if got, want := s.Segments(t.Context(), movie), []Segment{{Type: Outro, Start: at(7900), End: at(8300)}}; !slices.Equal(got, want) {
		t.Errorf("movie: got %+v, want %+v", got, want)
	}
	if q := d.last(publicMetaDBPath); q.Get("tmdb_id") != "550" || q.Get("media_type") != "movie" || q.Has("season") || q.Has("episode") {
		t.Errorf("PublicMetaDB was asked %v", q)
	}
	// Without a TMDB identifier, only IntroDB is asked.
	s.Segments(t.Context(), Title{Item: accounts.ID{3}, IMDb: "tt0903747", Season: 1, Episode: 1})
	if _, b := d.calls(); b != 2 || len(d.publicMetaDB()) != 1 {
		t.Errorf("asked IntroDB %d times and PublicMetaDB %d times", b, len(d.publicMetaDB()))
	}
}

func TestARefusedKeyIsNotAskedWithAgain(t *testing.T) {
	d := newDatabases(t)
	d.reply(publicMetaDBPath, http.StatusUnauthorized, `{"error":"Invalid or missing API key"}`)
	s, c := service(t, d, PublicMetaDB)
	var log strings.Builder
	s.logger = slog.New(slog.NewTextHandler(&log, nil))
	const refused = "pm-Refused00000000000000000000000000000000000000000000000000000"
	useKey(s, refused)
	titles := []Title{
		episode,
		{Item: accounts.ID{2}, TMDB: "550"},
		{Item: accounts.ID{3}, TMDB: "1396", Season: 1, Episode: 3},
	}
	for _, title := range titles {
		s.Segments(t.Context(), title)
	}
	c.advance(48 * time.Hour)
	s.Segments(t.Context(), episode)
	if asked := len(d.publicMetaDB()); asked != 1 {
		t.Errorf("a refused key was asked with %d times", asked)
	}
	if warnings := strings.Count(log.String(), "level=WARN"); warnings != 1 {
		t.Errorf("%d warnings for a refused key", warnings)
	}
	// Another key is asked with, once, and refused (403) in turn.
	d.reply(publicMetaDBPath, http.StatusForbidden, `{"error":"Forbidden"}`)
	useKey(s, goodKey+"2")
	for _, title := range titles {
		s.Segments(t.Context(), title)
	}
	if asked := len(d.publicMetaDB()); asked != 2 {
		t.Errorf("with a second key: asked %d times", asked)
	}
	// A key PublicMetaDB accepts asks about every title: those it refused
	// to answer were not asked.
	d.reply(publicMetaDBPath, http.StatusOK, skipPage(episodeRecords...))
	useKey(s, goodKey)
	if got := s.Segments(t.Context(), episode); len(got) != 3 {
		t.Errorf("with an accepted key: %+v", got)
	}
	if auth := d.publicMetaDB(); len(auth) != 3 || auth[2] != "Bearer "+goodKey {
		t.Errorf("asked with %q", auth)
	}
	// A key refused, then accepted by the check of the settings, as once
	// PublicMetaDB allows it again, is asked with.
	const restored = "pm-Restored0000000000000000000000000000000000000000000000000000"
	d.reply(publicMetaDBPath, http.StatusUnauthorized, `{"error":"Invalid or missing API key"}`)
	useKey(s, restored)
	s.Segments(t.Context(), titles[1])
	d.reply(publicMetaDBPath, http.StatusOK, skipPage())
	if err := s.CheckKey(t.Context(), PublicMetaDB, restored); err != nil {
		t.Fatal(err)
	}
	s.Segments(t.Context(), titles[1])
	if auth := d.publicMetaDB(); len(auth) != 6 || auth[5] != "Bearer "+restored {
		t.Errorf("after the check: %d requests", len(auth))
	}
	if strings.Contains(log.String(), refused) || strings.Contains(log.String(), goodKey) {
		t.Error("a key was logged")
	}
}

func TestARateLimitHoldsForEveryTitle(t *testing.T) {
	d := newDatabases(t)
	d.limit(publicMetaDBPath, "120")
	s, c := service(t, d, TheIntroDB, PublicMetaDB)
	useKey(s, goodKey)
	movie := Title{Item: accounts.ID{2}, TMDB: "550"}
	s.Segments(t.Context(), episode)
	s.Segments(t.Context(), movie)
	if asked := len(d.publicMetaDB()); asked != 1 {
		t.Errorf("during the rate limit: asked %d times", asked)
	}
	// The other database is not held back.
	if a, _ := d.calls(); a != 2 {
		t.Errorf("TheIntroDB asked %d times", a)
	}
	// After Retry-After, the titles not asked yet are; the one refused is
	// asked again an hour later, as after any failure.
	d.reply(publicMetaDBPath, http.StatusOK, skipPage(episodeRecords...))
	c.advance(3 * time.Minute)
	s.Segments(t.Context(), movie)
	s.Segments(t.Context(), episode)
	if asked := len(d.publicMetaDB()); asked != 2 {
		t.Errorf("after the rate limit: asked %d times", asked)
	}
	c.advance(time.Hour)
	if got := s.Segments(t.Context(), episode); len(got) != 3 || len(d.publicMetaDB()) != 3 {
		t.Errorf("an hour later: %+v, asked %d times", got, len(d.publicMetaDB()))
	}
	// Retry-After may be a date.
	d.limit(publicMetaDBPath, c.now.Add(10*time.Minute).Format(http.TimeFormat))
	s.Segments(t.Context(), Title{Item: accounts.ID{3}, TMDB: "1396", Season: 2, Episode: 1})
	s.Segments(t.Context(), Title{Item: accounts.ID{4}, TMDB: "1396", Season: 2, Episode: 2})
	c.advance(11 * time.Minute)
	s.Segments(t.Context(), Title{Item: accounts.ID{4}, TMDB: "1396", Season: 2, Episode: 2})
	if asked := len(d.publicMetaDB()); asked != 5 {
		t.Errorf("with a date: asked %d times", asked)
	}
}

func TestTheSavedOrderChangesWhichDatabaseWins(t *testing.T) {
	d := newDatabases(t)
	// Each database has an intro of its own; PublicMetaDB also a recap.
	d.reply(theIntroDBPath, http.StatusOK, `{"intro":[{"start_ms":null,"end_ms":30500}]}`)
	d.reply(introDBPath, http.StatusOK, `{"imdb_id":"tt0903747","intro":{"start_ms":40000,"end_ms":70000},"recap":null,"outro":null}`)
	d.reply(publicMetaDBPath, http.StatusOK, skipPage(episodeRecords...))
	s, _ := service(t, d, TheIntroDB, IntroDB, PublicMetaDB)
	intro := func(order ...string) Segment {
		t.Helper()
		useSettings(s, accounts.Settings{PublicMetaDBKey: goodKey, SegmentOrder: order})
		for _, segment := range s.Segments(t.Context(), episode) {
			if segment.Type == Intro {
				return segment
			}
		}
		t.Fatalf("no intro with the order %v", order)
		return Segment{}
	}
	theIntroDB := Segment{Type: Intro, Start: 0, End: at(30.5)}
	introDB := Segment{Type: Intro, Start: at(40), End: at(70)}
	publicMetaDB := Segment{Type: Intro, Start: at(15), End: at(62)}
	for _, tc := range []struct {
		order []string
		want  Segment
	}{
		{nil, theIntroDB},
		{[]string{PublicMetaDB, IntroDB, TheIntroDB}, publicMetaDB},
		{[]string{IntroDB, PublicMetaDB, TheIntroDB}, introDB},
		{[]string{}, theIntroDB},
	} {
		if got := intro(tc.order...); got != tc.want {
			t.Errorf("order %v: intro %+v, want %+v", tc.order, got, tc.want)
		}
	}
	// Reordering asks no database again: the answers kept are merged anew.
	if a, b := d.calls(); a != 1 || b != 1 || len(d.publicMetaDB()) != 1 {
		t.Errorf("asked %d, %d and %d times", a, b, len(d.publicMetaDB()))
	}

	// The order does not bring back a database POLYFIN_SEGMENTS leaves
	// out, nor PublicMetaDB without a key.
	s, _ = service(t, d, IntroDB, TheIntroDB)
	useSettings(s, accounts.Settings{PublicMetaDBKey: goodKey, SegmentOrder: []string{PublicMetaDB, TheIntroDB, IntroDB}})
	if got := s.Segments(t.Context(), episode); !slices.Equal(got, []Segment{theIntroDB}) || len(d.publicMetaDB()) != 1 {
		t.Errorf("without PublicMetaDB: %+v, asked %d times", got, len(d.publicMetaDB()))
	}
	s, _ = service(t, d, TheIntroDB, IntroDB, PublicMetaDB)
	useSettings(s, accounts.Settings{SegmentOrder: []string{PublicMetaDB, IntroDB, TheIntroDB}})
	if got := s.Segments(t.Context(), episode); !slices.Equal(got, []Segment{introDB}) || len(d.publicMetaDB()) != 1 {
		t.Errorf("without a key: %+v, asked %d times", got, len(d.publicMetaDB()))
	}
}

func TestOrderListsEveryDatabase(t *testing.T) {
	for _, tc := range []struct {
		on, saved, order, off []string
	}{
		{[]string{TheIntroDB, IntroDB, PublicMetaDB}, nil, []string{TheIntroDB, IntroDB, PublicMetaDB}, []string{}},
		{[]string{PublicMetaDB, TheIntroDB}, nil, []string{PublicMetaDB, TheIntroDB, IntroDB}, []string{IntroDB}},
		{[]string{PublicMetaDB, TheIntroDB}, []string{IntroDB, TheIntroDB, PublicMetaDB}, []string{IntroDB, TheIntroDB, PublicMetaDB}, []string{IntroDB}},
		{nil, nil, []string{TheIntroDB, IntroDB, PublicMetaDB}, []string{TheIntroDB, IntroDB, PublicMetaDB}},
	} {
		order, off := New(nil, Sources(tc.on), "test", nil, nil).Order(tc.saved)
		if !slices.Equal(order, tc.order) || !slices.Equal(off, tc.off) {
			t.Errorf("%v, saved %v: order %v, off %v", tc.on, tc.saved, order, off)
		}
	}
	if order, off := (*Service)(nil).Order(nil); len(order) != 3 || len(off) != 3 {
		t.Errorf("no service: %v, %v", order, off)
	}
}

func TestTheIntroDBKeyIsUsedUntilRefused(t *testing.T) {
	d := newDatabases(t)
	d.reply(theIntroDBPath, http.StatusOK, `{"intro":[{"start_ms":null,"end_ms":30500}]}`)
	s, _ := service(t, d, TheIntroDB)
	var log strings.Builder
	s.logger = slog.New(slog.NewTextHandler(&log, nil))
	want := []Segment{{Type: Intro, Start: 0, End: at(30.5)}}
	title := func(n byte) Title { return Title{Item: accounts.ID{n}, TMDB: "1396", Season: 1, Episode: int(n)} }

	s.Segments(t.Context(), title(1))
	const key = "tidb-Accepted"
	useSettings(s, accounts.Settings{TheIntroDBKey: key})
	s.Segments(t.Context(), title(2))
	if auth := d.authorizations(theIntroDBPath); !slices.Equal(auth, []string{"", "Bearer " + key}) {
		t.Fatalf("without, then with a key: %q", auth)
	}

	// A key refused later is logged once, and the question asked again
	// without it: the segments are still found, and later titles are
	// asked without it at once.
	const refused = "tidb-Revoked"
	d.refuse(theIntroDBPath, refused)
	useSettings(s, accounts.Settings{TheIntroDBKey: refused})
	for n := byte(3); n <= 5; n++ {
		if got := s.Segments(t.Context(), title(n)); !slices.Equal(got, want) {
			t.Errorf("episode %d with a refused key: %+v", n, got)
		}
	}
	if auth := d.authorizations(theIntroDBPath)[2:]; !slices.Equal(auth, []string{"Bearer " + refused, "", "", ""}) {
		t.Errorf("with a refused key: %q", auth)
	}
	if warnings := strings.Count(log.String(), "level=WARN"); warnings != 1 {
		t.Errorf("%d warnings", warnings)
	}
	// Another key is sent at once.
	useSettings(s, accounts.Settings{TheIntroDBKey: key})
	s.Segments(t.Context(), title(6))
	if auth := d.authorizations(theIntroDBPath); auth[len(auth)-1] != "Bearer "+key {
		t.Errorf("with another key: %q", auth[len(auth)-1])
	}
	if strings.Contains(log.String(), key) || strings.Contains(log.String(), refused) {
		t.Error("a key was logged")
	}
}

func TestTheIntroDBKeyIsCheckedBySubmittingNothing(t *testing.T) {
	// TheIntroDB's media answers ignore a key they do not know: only a
	// submission tells. It refuses the key before reading the body, and
	// an empty body once the key is accepted.
	var mu sync.Mutex
	var asked []string
	var status int
	theIntroDB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		asked = append(asked, r.Method+" "+r.URL.Path+" "+string(body)+" "+r.Header.Get("Authorization"))
		answer := status
		mu.Unlock()
		switch {
		case r.Header.Get("Authorization") != "Bearer good":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"Invalid or expired token"}`)
		default:
			w.WriteHeader(answer)
			_, _ = io.WriteString(w, `{"error":"invalid request body"}`)
		}
	}))
	t.Cleanup(theIntroDB.Close)
	s := New(nil, []Source{{Name: TheIntroDB, URL: theIntroDB.URL}}, "test", slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	for _, tc := range []struct {
		key      string
		answer   int
		accepted bool
		refused  bool
	}{
		{"good", http.StatusBadRequest, true, false},
		{"good", http.StatusUnprocessableEntity, true, false},
		{"made-up", http.StatusBadRequest, false, true},
		{"good", http.StatusTooManyRequests, false, false},
		{"good", http.StatusBadGateway, false, false},
		// An answer that cannot be told apart does not accept the key.
		{"good", http.StatusOK, false, false},
		{"good", http.StatusNotFound, false, false},
	} {
		mu.Lock()
		status = tc.answer
		mu.Unlock()
		err := s.CheckKey(t.Context(), TheIntroDB, tc.key)
		if (err == nil) != tc.accepted || errors.Is(err, ErrKeyRefused) != tc.refused {
			t.Errorf("%s, answering %d: %v", tc.key, tc.answer, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if asked[0] != "POST /submit {} Bearer good" || asked[2] != "POST /submit {} Bearer made-up" || len(asked) != 7 {
		t.Errorf("asked %q", asked)
	}
	// Unreachable, the key is not accepted either.
	theIntroDB.Close()
	if err := s.CheckKey(t.Context(), TheIntroDB, "good"); err == nil || errors.Is(err, ErrKeyRefused) {
		t.Errorf("unreachable: %v", err)
	}
}
