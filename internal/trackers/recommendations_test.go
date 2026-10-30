package trackers

import (
	"errors"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// fixture reads a file of testdata: an answer of a service, in its shape.
func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// listTitles records what catalogs listed, by key and description.
func (h harness) listTitles(t *testing.T, records map[string]string) {
	t.Helper()
	for key, data := range records {
		kind, _, _ := strings.Cut(key, "|")
		if _, err := h.db.Exec(t.Context(), "INSERT INTO items (id, key, kind, data) VALUES ($1, $2, $3, $4)", itemOf(key), key, kind, data); err != nil {
			t.Fatal(err)
		}
	}
}

// recommended reads user's recommendations as a home row does, after the
// read it started ended.
func (h harness) recommended(t *testing.T, user accounts.ID) []accounts.ID {
	t.Helper()
	if _, err := h.Recommended(t.Context(), user); err != nil {
		t.Fatal(err)
	}
	h.idleRecommendations(t, user)
	ids, err := h.Recommended(t.Context(), user)
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

// idleRecommendations waits until no read of user's recommendations runs.
func (h harness) idleRecommendations(t *testing.T, user accounts.ID) {
	t.Helper()
	eventually(t, "the read of the recommendations ending", func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return !h.recommending[laneKey{user, Trakt}]
	})
}

// answerRecommendations answers Trakt's recommendations with the recorded
// movies and shows.
func (h harness) answerRecommendations(t *testing.T) {
	t.Helper()
	h.f.reply("GET", "/trakt/recommendations/movies", http.StatusOK, fixture(t, "trakt-recommendations-movies.json"))
	h.f.reply("GET", "/trakt/recommendations/shows", http.StatusOK, fixture(t, "trakt-recommendations-shows.json"))
}

// Trakt's recommendations are read with the user's connection, without
// what their Trakt collection and watchlist hold, and their titles are
// found by IMDb, TMDB or TVDB identifier, movies and shows in turn: those
// found by none are left out.
func TestTraktRecommendationsAreFoundByTheirIdentifiers(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "rita")
	h.connected(t, user, Trakt, "trakt-rita")
	h.listTitles(t, map[string]string{
		"movie|other:9":     `{"kind":"movie","meta":{"id":"other:9","type":"movie","name":"Listed Under Its Own Name","imdb_id":"tt0000009"}}`,
		"movie|tmdb:550":    `{"kind":"movie","meta":{"id":"tmdb:550","type":"movie","name":"Known By TMDB Only"}}`,
		"series|tvdb:81189": `{"kind":"series","meta":{"id":"tvdb:81189","type":"series","name":"Known By TVDB"}}`,
	})
	h.answerRecommendations(t)

	// The first read starts as a row first asks: nothing until it ends.
	if ids, err := h.Recommended(t.Context(), user); err != nil || len(ids) != 0 {
		t.Fatalf("first asked: %v %v", ids, err)
	}
	got := h.recommended(t, user)
	want := []accounts.ID{itemOf("movie|other:9"), itemOf("series|tvdb:81189"), itemOf("movie|tmdb:550"), itemOf("movie|tt0000077")}
	if !slices.Equal(got, want) {
		t.Errorf("recommended:\n got %v\nwant %v", got, want)
	}
	for _, path := range []string{"/recommendations/movies", "/recommendations/shows"} {
		sent := h.f.sent(Trakt, path)
		if len(sent) != 1 {
			t.Fatalf("%s: %d requests", path, len(sent))
		}
		r := sent[0]
		if r.query.Get("limit") != strconv.Itoa(recommendationsLimit) || r.query.Get("ignore_collected") != "true" ||
			r.query.Get("ignore_watchlisted") != "true" || r.header.Get("Authorization") != "Bearer trakt-rita" ||
			r.header.Get("trakt-api-key") != "trakt-client" {
			t.Errorf("%s: %v %v", path, r.query, r.header)
		}
	}
	h.noWrites(t)
}

// Recommendations are kept, and read again once due: every 12 hours, an
// hour after a read that failed, which keeps what was read before. The
// hourly check reads again those a row asked for within a week.
func TestTraktRecommendationsAreReadAgainWhenDue(t *testing.T) {
	h := newHarness(t)
	var clock atomic.Int64
	clock.Store(clockStart.UnixNano())
	h.now = func() time.Time { return time.Unix(0, clock.Load()).UTC() }
	advance := func(d time.Duration) { clock.Add(int64(d)) }
	user := h.user(t, "sam")
	h.connected(t, user, Trakt, "trakt-sam")
	var failing atomic.Bool
	var movies atomic.Value
	movies.Store(`[{"title":"First","ids":{"trakt":1,"imdb":"tt0000001"}}]`)
	h.f.on("GET", "/trakt/recommendations/movies", func(request) answer {
		if failing.Load() {
			return answer{status: http.StatusBadGateway, body: "{}"}
		}
		return answer{status: http.StatusOK, body: movies.Load().(string)}
	})
	h.f.reply("GET", "/trakt/recommendations/shows", http.StatusOK, `[]`)
	reads := func() int { return len(h.f.sent(Trakt, "/recommendations/movies")) }

	first := h.recommended(t, user)
	if len(first) != 1 || reads() != 1 {
		t.Fatalf("first read: %v, %d reads", first, reads())
	}
	// Not due: kept, not read again.
	advance(11 * time.Hour)
	if got := h.recommended(t, user); !slices.Equal(got, first) || reads() != 1 {
		t.Errorf("before due: %v, %d reads", got, reads())
	}
	h.recommendationsDue()
	if got := h.recommended(t, user); !slices.Equal(got, first) || reads() != 1 {
		t.Errorf("checked before due: %v, %d reads", got, reads())
	}
	// Due, and failing: what was read stays.
	advance(time.Hour)
	failing.Store(true)
	h.recommendationsDue()
	eventually(t, "the failed read", func() bool { return reads() > 1 })
	if got := h.recommended(t, user); !slices.Equal(got, first) {
		t.Errorf("after a failed read: %v", got)
	}
	after := reads()
	advance(30 * time.Minute)
	if h.recommended(t, user); reads() != after {
		t.Errorf("read again within the hour after a failure: %d reads", reads())
	}
	// Tried again an hour after the failure.
	failing.Store(false)
	movies.Store(`[{"title":"Second","ids":{"trakt":2,"imdb":"tt0000002"}}]`)
	advance(30 * time.Minute)
	h.recommendationsDue()
	eventually(t, "the read after the failure", func() bool {
		ids, err := h.Recommended(t.Context(), user)
		return err == nil && len(ids) == 1 && ids[0] != first[0]
	})
	// Not asked for within a week: the hourly check leaves them.
	advance(8 * 24 * time.Hour)
	before := reads()
	h.recommendationsDue()
	h.idleRecommendations(t, user)
	if reads() != before {
		t.Errorf("recommendations no row asked for were read again: %d reads", reads())
	}
}

// Without a connection to Trakt that works there are no recommendations:
// nothing is asked of Trakt, and disconnecting forgets those read.
func TestTraktRecommendationsNeedAConnection(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "tom")
	h.answerRecommendations(t)
	if ok, err := h.Recommends(t.Context(), user); ok || err != nil {
		t.Errorf("not connected: %v %v", ok, err)
	}
	if _, err := h.Recommended(t.Context(), user); !errors.Is(err, ErrNotConnected) {
		t.Errorf("not connected: %v", err)
	}
	h.connected(t, user, Simkl, "simkl-tom")
	if _, err := h.Recommended(t.Context(), user); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Simkl connected: %v", err)
	}
	h.connected(t, user, Trakt, "trakt-tom")
	if ok, err := h.Recommends(t.Context(), user); !ok || err != nil {
		t.Errorf("connected: %v %v", ok, err)
	}
	if got := h.recommended(t, user); len(got) == 0 {
		t.Fatal("no recommendations")
	}
	if err := h.Disconnect(t.Context(), user, Trakt); err != nil {
		t.Fatal(err)
	}
	var kept int
	if err := h.db.QueryRow(t.Context(), "SELECT count(*) FROM tracking_recommendations").Scan(&kept); err != nil || kept != 0 {
		t.Errorf("%d kept after disconnecting: %v", kept, err)
	}
	if _, err := h.Recommended(t.Context(), user); !errors.Is(err, ErrNotConnected) {
		t.Errorf("disconnected: %v", err)
	}
	// A token Trakt refused is no connection that works.
	h.connected(t, user, Trakt, "trakt-tom")
	if _, err := h.db.Exec(t.Context(), "UPDATE tracking_connections SET problem = 'reconnect' WHERE user_id = $1", user); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Recommended(t.Context(), user); !errors.Is(err, ErrNotConnected) {
		t.Errorf("refused: %v", err)
	}
	if n := len(h.f.sent(Trakt, "/recommendations/movies")); n != 1 {
		t.Errorf("%d reads", n)
	}
}
