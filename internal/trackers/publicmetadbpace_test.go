package trackers

import (
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/mediasegments"
	"github.com/moodiness/polyfin/internal/pace"
)

// PublicMetaDB's limit counts every request of the server's address: each
// request takes its turn, a delivery's TMDB lookup as its play, the
// segment lookups keep the same pace, and a rate limit one of them is told
// holds the others.
func TestSegmentLookupsKeepThePublicMetaDBPace(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	const gap = 100 * time.Millisecond
	shared := pace.New(gap)
	h.publicMetaDBPace = shared
	segments := mediasegments.New(h.db, []mediasegments.Source{{Name: mediasegments.PublicMetaDB, URL: h.f.url + "/publicmetadb"}}, "test",
		slog.New(slog.NewTextHandler(io.Discard, nil)), func() accounts.Settings { return accounts.Settings{PublicMetaDBKey: "pm-server"} })
	segments.UsePublicMetaDBPace(shared)

	// Three users' plays are sent while four movies' segments are looked
	// up, all at once. The last play is of a movie known by IMDb only,
	// whose TMDB identifier is looked up first.
	h.f.reply("GET", "/publicmetadb/api/external/mappings/lookup", http.StatusOK, `{"results":[{"tmdb_id":30,"media_type":"movie"}],"total":1}`)
	imdbOnly := Title{Item: itemID(5), IMDb: "tt0100"}
	for i, played := range []Title{movie, movie, imdbOnly} {
		user := h.user(t, "user"+strconv.Itoa(i))
		h.connected(t, user, PublicMetaDB, "pm-user"+strconv.Itoa(i))
		h.Mark(user, Mark{Played: true, Titles: titles(played)})
	}
	var lookups sync.WaitGroup
	for i := range 4 {
		lookups.Go(func() {
			segments.Segments(t.Context(), mediasegments.Title{Item: itemID(byte(100 + i)), TMDB: strconv.Itoa(550 + i)})
		})
	}
	lookups.Wait()
	h.f.wait(t, 3, "publicmetadb", "/api/external/watched")
	h.idle(t)
	if skips, mappings := len(h.f.sent("publicmetadb", "/api/external/skips")), len(h.f.sent("publicmetadb", "/api/external/mappings/lookup")); skips != 4 || mappings != 1 {
		t.Fatalf("PublicMetaDB was asked for %d segments and %d TMDB identifiers, want 4 and 1", skips, mappings)
	}
	spaced(t, h.f, gap)

	// A rate limit told to a segment lookup holds the next play sent.
	h.f.on("GET", "/publicmetadb/api/external/skips", func(request) answer {
		return answer{status: http.StatusTooManyRequests, body: `{"error":"Too Many Requests"}`, header: http.Header{"Retry-After": {"1"}}}
	})
	segments.Segments(t.Context(), mediasegments.Title{Item: itemID(110), TMDB: "560"})
	limited := h.f.sent("publicmetadb", "/api/external/skips")[4].at
	user := h.user(t, "late")
	h.connected(t, user, PublicMetaDB, "pm-late")
	h.Mark(user, Mark{Played: true, Titles: titles(movie)})
	played := h.f.wait(t, 4, "publicmetadb", "/api/external/watched")[3].at
	if waited := played.Sub(limited); waited < 900*time.Millisecond {
		t.Errorf("a play was sent %v after PublicMetaDB asked to wait a second", waited)
	}
}

// spaced fails t unless the requests to PublicMetaDB came at least half of
// gap apart: each is sent in its turn, gap after the one before, and
// arrives a little later.
func spaced(t *testing.T, f *fakes, gap time.Duration) {
	t.Helper()
	f.mu.Lock()
	var times []time.Time
	for _, r := range f.requests {
		if r.service == "publicmetadb" {
			times = append(times, r.at)
		}
	}
	f.mu.Unlock()
	slices.SortFunc(times, time.Time.Compare)
	for i := 1; i < len(times); i++ {
		if apart := times[i].Sub(times[i-1]); apart < gap/2 {
			t.Errorf("requests %d and %d to PublicMetaDB came %v apart, want about %v", i-1, i, apart, gap)
		}
	}
}
