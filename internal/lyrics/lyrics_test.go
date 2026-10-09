package lyrics

import (
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/testdb"
)

// syncedLRC is a song's synced lyrics as LRCLIB gives them: a description
// tag, lines with two- and three-digit fractions, an empty line marking a
// pause, and a chorus sung twice from one line.
const syncedLRC = "[ar:Tone Quartet]\n[00:01.50] First line\n[00:03.05]Second line\n[00:04.123] \n[00:06.00][01:10.5] Chorus\n"

// fakeLRCLIB answers as LRCLIB's /api/get, by track name: synced and
// plain lyrics, plain lyrics only, an instrumental, an unknown track, a
// track it fails on while failing is set (or limits with a Retry-After
// while limiting is set), and one it answers once release is closed. It
// counts the requests for each track and keeps the last query and
// User-Agent.
type fakeLRCLIB struct {
	server    *httptest.Server
	failing   atomic.Bool
	limiting  atomic.Bool
	release   chan struct{}
	mu        sync.Mutex
	asked     map[string]int
	lastQuery url.Values
	userAgent string
}

func newFakeLRCLIB(t *testing.T) *fakeLRCLIB {
	t.Helper()
	f := &fakeLRCLIB{asked: map[string]int{}, release: make(chan struct{})}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/get" {
			http.NotFound(w, r)
			return
		}
		name := r.URL.Query().Get("track_name")
		f.mu.Lock()
		f.asked[name]++
		f.lastQuery, f.userAgent = r.URL.Query(), r.Header.Get("User-Agent")
		f.mu.Unlock()
		answer := func(v map[string]any) {
			v["trackName"], v["artistName"] = name, r.URL.Query().Get("artist_name")
			_ = json.NewEncoder(w).Encode(v)
		}
		switch name {
		case "Synced Song":
			answer(map[string]any{"instrumental": false, "plainLyrics": "First line\nSecond line", "syncedLyrics": syncedLRC})
		case "Plain Song":
			answer(map[string]any{"instrumental": false, "plainLyrics": "Line one\r\nLine two\n\nLine three\n", "syncedLyrics": nil})
		case "Interlude":
			answer(map[string]any{"instrumental": true, "plainLyrics": nil, "syncedLyrics": nil})
		case "Flaky Song":
			switch {
			case f.failing.Load():
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
			case f.limiting.Load():
				w.Header().Set("Retry-After", "120")
				http.Error(w, "slow down", http.StatusTooManyRequests)
			default:
				answer(map[string]any{"plainLyrics": "Back again", "syncedLyrics": nil})
			}
		case "Slow Song":
			select {
			case <-f.release:
			case <-r.Context().Done():
				return
			}
			answer(map[string]any{"plainLyrics": "Late words", "syncedLyrics": nil})
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":404,"name":"TrackNotFound","message":"Failed to find specified track"}`))
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeLRCLIB) count(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.asked[name]
}

// last is the query and User-Agent of the last request.
func (f *fakeLRCLIB) last() (url.Values, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastQuery, f.userAgent
}

// clock is a settable time.
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

// newService returns a service asking f, with lyrics turned on while on
// holds true, on a clock tests move.
func newService(t *testing.T, f *fakeLRCLIB) (*Service, *atomic.Bool, *clock) {
	t.Helper()
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	on := &atomic.Bool{}
	on.Store(true)
	settings := func() accounts.Settings {
		s := accounts.DefaultSettings()
		s.Lyrics = on.Load()
		return s
	}
	s := New(pool, f.server.URL+"/", "1.2.3", slog.New(slog.DiscardHandler), settings)
	c := &clock{now: time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)}
	s.now = c.Now
	return s, on, c
}

func song(title string) Track {
	var id accounts.ID
	_, _ = rand.Read(id[:])
	return Track{ID: id, Artist: "Tone Quartet", Title: title, Album: "Sine Studies", Duration: 183600 * time.Millisecond}
}

func TestSyncedLyricsAreAskedOnceAndReadLineByLine(t *testing.T) {
	f := newFakeLRCLIB(t)
	s, _, _ := newService(t, f)
	track := song("Synced Song")

	for range 3 {
		found, ok := s.Lyrics(t.Context(), track, 5*time.Second)
		want := []Line{{"First line", 1500 * time.Millisecond}, {"Second line", 3050 * time.Millisecond}, {"", 4123 * time.Millisecond},
			{"Chorus", 6 * time.Second}, {"Chorus", 70500 * time.Millisecond}}
		if !ok || !found.Synced || !slices.Equal(found.Lines, want) {
			t.Fatalf("lyrics: %v %+v, want synced %+v", ok, found, want)
		}
	}
	if n := f.count("Synced Song"); n != 1 {
		t.Errorf("LRCLIB was asked %d times about one song", n)
	}
	// The track is asked about by its artist, title, album and length in
	// whole seconds, by Polyfin with its version and repository.
	want := url.Values{"artist_name": {"Tone Quartet"}, "track_name": {"Synced Song"}, "album_name": {"Sine Studies"}, "duration": {"184"}}
	query, agent := f.last()
	if query.Encode() != want.Encode() {
		t.Errorf("query %v, want %v", query, want)
	}
	if agent != "Polyfin/1.2.3 (https://github.com/moodiness/polyfin)" {
		t.Errorf("User-Agent %q", agent)
	}
	other := song("Unknown Song")
	if known := s.Known(t.Context(), []accounts.ID{track.ID, other.ID}); !known[track.ID] || known[other.ID] {
		t.Errorf("known: %v", known)
	}
}

func TestPlainLyricsAreUnsynced(t *testing.T) {
	f := newFakeLRCLIB(t)
	s, _, _ := newService(t, f)
	track := song("Plain Song")
	// Without an album nor a length, the track is asked about without them.
	track.Album, track.Duration = "", 0

	found, ok := s.Lyrics(t.Context(), track, 5*time.Second)
	want := []Line{{Text: "Line one"}, {Text: "Line two"}, {Text: ""}, {Text: "Line three"}}
	if !ok || found.Synced || !slices.Equal(found.Lines, want) {
		t.Fatalf("lyrics: %v %+v, want plain %+v", ok, found, want)
	}
	if query, _ := f.last(); query.Has("album_name") || query.Has("duration") {
		t.Errorf("query %v", query)
	}
	if !s.Known(t.Context(), []accounts.ID{track.ID})[track.ID] {
		t.Error("plain lyrics are not known")
	}
}

func TestTracksWithoutLyricsAreRemembered(t *testing.T) {
	f := newFakeLRCLIB(t)
	s, _, c := newService(t, f)
	unknown, instrumental, nameless := song("Unknown Song"), song("Interlude"), song("Nameless")
	nameless.Artist = " "

	for range 2 {
		for _, track := range []Track{unknown, instrumental, nameless} {
			if found, ok := s.Lyrics(t.Context(), track, 5*time.Second); ok {
				t.Errorf("%s has lyrics: %+v", track.Title, found)
			}
		}
	}
	if f.count("Unknown Song") != 1 || f.count("Interlude") != 1 || f.count("Nameless") != 0 {
		t.Errorf("asked %v", f.asked)
	}
	if known := s.Known(t.Context(), []accounts.ID{unknown.ID, instrumental.ID}); len(known) != 0 {
		t.Errorf("known: %v", known)
	}
	// LRCLIB grows: a track it did not know is asked about again a month
	// later; an instrumental stays one.
	c.advance(missingFor - time.Minute)
	s.Lyrics(t.Context(), unknown, 5*time.Second)
	c.advance(time.Minute)
	s.Lyrics(t.Context(), unknown, 5*time.Second)
	s.Lyrics(t.Context(), instrumental, 5*time.Second)
	if f.count("Unknown Song") != 2 || f.count("Interlude") != 1 {
		t.Errorf("a month later, asked %v", f.asked)
	}
}

func TestFailuresAreRetriedLater(t *testing.T) {
	f := newFakeLRCLIB(t)
	s, _, c := newService(t, f)
	track, other := song("Flaky Song"), song("Synced Song")

	f.failing.Store(true)
	if _, ok := s.Lyrics(t.Context(), track, 5*time.Second); ok {
		t.Fatal("lyrics from a failed request")
	}
	// LRCLIB is left alone a minute after a failure, for every track.
	c.advance(failedFor - time.Second)
	if _, ok := s.Lyrics(t.Context(), other, 5*time.Second); ok || f.count("Synced Song") != 0 {
		t.Errorf("LRCLIB was asked right after a failure: %v", f.asked)
	}
	// The failure was not an answer: the track is asked about again.
	f.failing.Store(false)
	c.advance(time.Second)
	if found, ok := s.Lyrics(t.Context(), track, 5*time.Second); !ok || found.Lines[0].Text != "Back again" || f.count("Flaky Song") != 2 {
		t.Errorf("after the pause: %v %+v, asked %v", ok, found, f.asked)
	}

	// A 429 leaves LRCLIB alone as long as its Retry-After says.
	limited := song("Flaky Song")
	f.limiting.Store(true)
	s.Lyrics(t.Context(), limited, 5*time.Second)
	f.limiting.Store(false)
	c.advance(119 * time.Second)
	s.Lyrics(t.Context(), limited, 5*time.Second)
	if n := f.count("Flaky Song"); n != 3 {
		t.Errorf("asked %d times before the Retry-After ended", n)
	}
	c.advance(time.Second)
	if _, ok := s.Lyrics(t.Context(), limited, 5*time.Second); !ok || f.count("Flaky Song") != 4 {
		t.Errorf("after the Retry-After: %v, asked %v", ok, f.asked)
	}
}

func TestSlowAnswersDoNotHoldRequests(t *testing.T) {
	f := newFakeLRCLIB(t)
	s, _, _ := newService(t, f)
	track := song("Slow Song")

	started := time.Now()
	if _, ok := s.Lyrics(t.Context(), track, 50*time.Millisecond); ok {
		t.Fatal("lyrics before LRCLIB answered")
	}
	if waited := time.Since(started); waited > 2*time.Second {
		t.Errorf("waited %v for LRCLIB", waited)
	}
	// A request while the first is under way joins it.
	s.Lyrics(t.Context(), track, 0)
	close(f.release)
	// The answer that came late is kept for the next request.
	deadline := time.Now().Add(5 * time.Second)
	for !s.Known(t.Context(), []accounts.ID{track.ID})[track.ID] {
		if time.Now().After(deadline) {
			t.Fatal("the late answer was not kept")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if found, ok := s.Lyrics(t.Context(), track, 0); !ok || found.Lines[0].Text != "Late words" || f.count("Slow Song") != 1 {
		t.Errorf("lyrics %v %+v, asked %v", ok, found, f.asked)
	}
}

func TestLyricsTurnedOffAskNothing(t *testing.T) {
	f := newFakeLRCLIB(t)
	s, on, _ := newService(t, f)
	found := song("Synced Song")
	if _, ok := s.Lyrics(t.Context(), found, 5*time.Second); !ok {
		t.Fatal("no lyrics")
	}

	on.Store(false)
	never := song("Plain Song")
	s.LookUp(never)
	if _, ok := s.Lyrics(t.Context(), never, 5*time.Second); ok {
		t.Error("lyrics while turned off")
	}
	// Lyrics found before are not served either.
	if _, ok := s.Lyrics(t.Context(), found, 5*time.Second); ok {
		t.Error("kept lyrics served while turned off")
	}
	if known := s.Known(t.Context(), []accounts.ID{found.ID}); len(known) != 0 {
		t.Errorf("known while turned off: %v", known)
	}
	time.Sleep(50 * time.Millisecond)
	if n := f.count("Plain Song"); n != 0 {
		t.Errorf("LRCLIB was asked %d times while turned off", n)
	}
	var none *Service
	if _, ok := none.Lyrics(t.Context(), found, time.Second); ok || none.Known(t.Context(), []accounts.ID{found.ID}) != nil {
		t.Error("a nil service found lyrics")
	}
}

func TestLookUpKeepsLyricsForLater(t *testing.T) {
	f := newFakeLRCLIB(t)
	s, _, _ := newService(t, f)
	track := song("Synced Song")
	s.LookUp(track)
	deadline := time.Now().Add(5 * time.Second)
	for !s.Known(t.Context(), []accounts.ID{track.ID})[track.ID] {
		if time.Now().After(deadline) {
			t.Fatal("a track looked up has no lyrics")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, ok := s.Lyrics(t.Context(), track, 0); !ok || f.count("Synced Song") != 1 {
		t.Errorf("lyrics %v, asked %v", ok, f.asked)
	}
}

func TestParseLRC(t *testing.T) {
	for _, tc := range []struct {
		name, lrc string
		want      []Line
	}{
		{"fractions", "[00:01]a\n[00:01.5]b\n[00:01.25]c\n[00:01:75]d\n[01:02.345]e",
			[]Line{{"a", time.Second}, {"c", 1250 * time.Millisecond}, {"b", 1500 * time.Millisecond}, {"d", 1750 * time.Millisecond},
				{"e", 62345 * time.Millisecond}}},
		{"out of order and repeated", "[00:20.00]later\r\n[00:05.00][00:30.00]twice\r\n",
			[]Line{{"twice", 5 * time.Second}, {"later", 20 * time.Second}, {"twice", 30 * time.Second}}},
		{"tags and untimed lines left out", "[ti:Song]\n[length: 03:00]\nno time\n[00:02.00]sung", []Line{{"sung", 2 * time.Second}}},
		{"word timing left out", "[00:03.00] <00:03.00>Every <00:03.50>word", []Line{{"Every word", 3 * time.Second}}},
		{"no lines", "[ar:Someone]\nplain words", nil},
	} {
		if got := parseLRC(tc.lrc); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
	// Synced lyrics without a timed line fall back to the plain ones.
	if a := (record{SyncedLyrics: "[ar:Someone]", PlainLyrics: "words"}).answer(time.Now()); a.kind != kindPlain {
		t.Errorf("answer kind %s", a.kind)
	}
}
