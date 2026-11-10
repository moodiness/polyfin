package trackers

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// PublicMetaDB's watchlist is found among the user's lists, read a page
// at a time, a series asked to be mapped as a series. A read that is not
// whole, a failed page or no watchlist, takes nothing off; the import
// sends nothing and creates no list.
func TestPublicMetaDBWatchlistFillsTheWatchlist(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	user := h.user(t, "fay")
	h.connected(t, user, PublicMetaDB, "pm-fay")
	if status := h.status(t, user, PublicMetaDB); !status.Watchlist || !status.WatchlistSent || status.ImportWatchlist {
		t.Errorf("PublicMetaDB connected: %+v", status)
	}
	if status := h.status(t, user, PublicMetaDB); status.Ratings {
		t.Errorf("ratings: %+v", status)
	}
	var mu sync.Mutex
	lists := []string{`{"items":[{"id":"c1","type":"custom"}],"totalPages":2}`, `{"items":[{"id":"w1","type":"watchlist"}],"totalPages":2}`}
	items := []string{
		`{"items":[{"id":"li_1","tmdb_id":501,"media_type":"movie","created_at":"2026-09-01T10:00:00Z"}],"totalPages":2}`,
		`{"items":[{"id":"li_2","tmdb_id":502,"media_type":"tv","created_at":"2026-09-02T10:00:00Z"},{"id":"li_3","tmdb_id":503,"media_type":"person"}],"totalPages":2}`,
	}
	page := func(pages *[]string) func(request) answer {
		return func(r request) answer {
			mu.Lock()
			defer mu.Unlock()
			n := 1
			if r.query.Get("page") == "2" {
				n = 2
			}
			if n > len(*pages) || (*pages)[n-1] == "" {
				return answer{status: http.StatusBadGateway, body: `{}`}
			}
			return answer{status: http.StatusOK, body: (*pages)[n-1]}
		}
	}
	h.f.on("GET", "/publicmetadb/api/external/lists", page(&lists))
	h.f.on("GET", "/publicmetadb/api/external/lists/w1/items", page(&items))
	// TMDB's 502 is the series tt0100, only when asked as a series.
	h.f.on("GET", "/publicmetadb/api/external/mappings", func(r request) answer {
		imdb := map[string]string{"movie:501": "tt0000001", "tv:502": "tt0100"}[r.query.Get("media_type")+":"+r.query.Get("tmdb_id")]
		if imdb == "" {
			return answer{status: http.StatusOK, body: `{"mappings":{"imdb":[]}}`}
		}
		return answer{status: http.StatusOK, body: `{"mappings":{"imdb":[{"id":"e1","value":"` + imdb + `"}]}}`}
	})
	result := h.turnOnWatchlist(t, user, PublicMetaDB)
	if result.Watchlist != 2 || result.Problem != "" {
		t.Errorf("result: %+v", result)
	}
	movie, show := h.idOf(t, library.TitleRef{IMDb: "tt0000001"}), h.idOf(t, library.TitleRef{Series: true, IMDb: "tt0100"})
	if got := h.watchlistOf(t, user); !slices.Equal(got, []accounts.ID{show, movie}) {
		t.Fatalf("imported: %v, want %v", got, []accounts.ID{show, movie})
	}
	whole := len(h.watchlistOf(t, user))

	// A failed page takes nothing off.
	mu.Lock()
	items = []string{`{"items":[{"id":"li_1","tmdb_id":501,"media_type":"movie"}],"totalPages":2}`, ""}
	mu.Unlock()
	if result := h.importNow(t, user, PublicMetaDB); result.Problem == "" {
		t.Errorf("a failed page: %+v", result)
	}
	if got := h.watchlistOf(t, user); len(got) != whole {
		t.Errorf("after a failed page: %v", got)
	}
	// No watchlist takes nothing off either.
	mu.Lock()
	lists = lists[:1]
	lists[0] = `{"items":[{"id":"c1","type":"custom"}],"totalPages":1}`
	mu.Unlock()
	if result := h.importNow(t, user, PublicMetaDB); result.Problem != "" {
		t.Errorf("no watchlist: %+v", result)
	}
	if got := h.watchlistOf(t, user); len(got) != whole {
		t.Errorf("after no watchlist: %v", got)
	}
	// Read whole without the movie, it leaves.
	mu.Lock()
	lists = []string{`{"items":[{"id":"w1","type":"watchlist"}],"totalPages":1}`}
	items = []string{`{"items":[{"id":"li_2","tmdb_id":502,"media_type":"tv"}],"totalPages":1}`}
	mu.Unlock()
	h.importNow(t, user, PublicMetaDB)
	if got := h.watchlistOf(t, user); slices.Contains(got, movie) || len(got) != whole-1 {
		t.Errorf("read whole without the movie: %v", got)
	}
	h.idle(t)
	h.noWrites(t)
}

// ownChange is a change of a watchlist, as the Jellyfin layer tells it.
func ownChange(added, removed []Title) WatchlistChange {
	return WatchlistChange{Titles: func(context.Context) ([]Title, []Title) { return added, removed }}
}

// The user's own changes go to their PublicMetaDB watchlist once they sync
// it: the list is made on the first add only, a removal finds the entry by
// paging, and an entry already gone counts as taken off.
func TestPublicMetaDBWatchlistGetsTheUsersOwnChanges(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	user := h.user(t, "gus")
	h.connected(t, user, PublicMetaDB, "pm-gus")
	movieTitle := Title{Item: itemID(1), TMDB: 10}
	series := Title{Item: itemID(2), TMDB: 30, IMDb: "tt0100", Series: true}
	animeOnly := Title{Item: itemID(3), Series: true, Anime: &AnimeID{ID: 7}}

	// Not synced: nothing is sent.
	h.Watchlist(user, ownChange([]Title{movieTitle}, nil))
	h.idle(t)
	if n := h.f.sentTo("publicmetadb"); n != 0 {
		t.Fatalf("sent while not synced: %d", n)
	}

	// Turned on: the import sends nothing and creates no list.
	h.turnOnWatchlist(t, user, PublicMetaDB)
	h.idle(t)
	h.noWrites(t)

	var mu sync.Mutex
	made := false
	h.f.on("GET", "/publicmetadb/api/external/lists", func(request) answer {
		mu.Lock()
		defer mu.Unlock()
		if !made {
			return answer{status: http.StatusOK, body: `{"items":[],"totalPages":1}`}
		}
		return answer{status: http.StatusOK, body: `{"items":[{"id":"w1","type":"watchlist"}],"totalPages":1}`}
	})
	h.f.on("POST", "/publicmetadb/api/external/lists", func(request) answer {
		mu.Lock()
		defer mu.Unlock()
		made = true
		return answer{status: http.StatusCreated, body: `{"id":"w1","name":"Watchlist","type":"watchlist"}`}
	})
	h.f.on("GET", "/publicmetadb/api/external/lists/w1/items", func(r request) answer {
		if r.query.Get("page") == "2" {
			return answer{status: http.StatusOK, body: `{"items":[{"id":"li_30","tmdb_id":30,"media_type":"tv"}],"totalPages":2}`}
		}
		return answer{status: http.StatusOK, body: `{"items":[{"id":"li_10","tmdb_id":10,"media_type":"movie"},{"id":"li_31","tmdb_id":30,"media_type":"movie"}],"totalPages":2}`}
	})
	h.f.reply("DELETE", "/publicmetadb/api/external/lists/w1/items/li_10", http.StatusNotFound, `{}`)

	h.Watchlist(user, ownChange([]Title{movieTitle, series, animeOnly}, nil))
	adds := h.f.wait(t, 2, "publicmetadb", "/api/external/lists/w1/items")
	h.idle(t)
	// The import looked for it once, the first add again, then made it.
	lookups := h.f.sent("publicmetadb", "/api/external/lists")
	if len(lookups) != 3 || lookups[2].method != http.MethodPost {
		t.Errorf("the list: %d requests", len(lookups))
	}
	var bodies []string
	for _, r := range h.f.sent("publicmetadb", "/api/external/lists/w1/items") {
		bodies = append(bodies, mustJSON(r.body))
	}
	if got := strings.Join(bodies, " "); got != `{"media_type":"movie","tmdb_id":10} {"media_type":"tv","tmdb_id":30}` {
		t.Errorf("adds: %s (%d)", got, len(adds))
	}

	// The series leaves by its entry, found on the second page; the movie
	// already gone counts as taken off.
	h.Watchlist(user, ownChange(nil, []Title{series, movieTitle}))
	h.f.wait(t, 1, "publicmetadb", "/api/external/lists/w1/items/li_30")
	h.idle(t)
	if deleted := h.f.sent("publicmetadb", "/api/external/lists/w1/items/li_30"); len(deleted) != 1 || deleted[0].method != http.MethodDelete {
		t.Errorf("the series: %+v", deleted)
	}
	if deleted := h.f.sent("publicmetadb", "/api/external/lists/w1/items/li_10"); len(deleted) != 1 {
		t.Errorf("the movie, gone already, was sent %d times", len(deleted))
	}
	if n := len(h.f.sent("publicmetadb", "/api/external/lists")); n != 3 {
		t.Errorf("the list was looked for again: %d", n)
	}
	var queued int
	if err := h.db.QueryRow(t.Context(), "SELECT count(*) FROM tracking_events").Scan(&queued); err != nil || queued != 0 {
		t.Errorf("%d changes left queued: %v", queued, err)
	}

	// Turned off, a change waiting is dropped and none is sent.
	if _, err := h.SetWatchlistImport(t.Context(), user, PublicMetaDB, false); err != nil {
		t.Fatal(err)
	}
	before := h.f.sentTo("publicmetadb")
	h.Watchlist(user, ownChange([]Title{movieTitle}, nil))
	h.idle(t)
	if n := h.f.sentTo("publicmetadb"); n != before {
		t.Errorf("sent once turned off: %d requests", n-before)
	}
}

// A refusal to wait is retried after the time PublicMetaDB asks.
func TestPublicMetaDBWatchlistWaitsWhenAsked(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	user := h.user(t, "hal")
	h.connected(t, user, PublicMetaDB, "pm-hal")
	if _, err := h.db.Exec(t.Context(), "INSERT INTO tracking_imports (user_id, service, watchlist) VALUES ($1, $2, true)", user, PublicMetaDB); err != nil {
		t.Fatal(err)
	}
	h.f.reply("GET", "/publicmetadb/api/external/lists", http.StatusOK, `{"items":[{"id":"w1","type":"watchlist"}],"totalPages":1}`)
	var mu sync.Mutex
	tries := 0
	h.f.on("POST", "/publicmetadb/api/external/lists/w1/items", func(request) answer {
		mu.Lock()
		defer mu.Unlock()
		tries++
		if tries == 1 {
			return answer{status: http.StatusTooManyRequests, body: `{}`, header: http.Header{"Retry-After": {"1"}}}
		}
		return answer{status: http.StatusCreated, body: `{"success":true,"item":{"id":"li_10"}}`}
	})
	h.Watchlist(user, ownChange([]Title{{Item: itemID(1), TMDB: 10}}, nil))
	sent := h.f.wait(t, 2, "publicmetadb", "/api/external/lists/w1/items")
	if gap := sent[1].at.Sub(sent[0].at); gap < 900*1e6 {
		t.Errorf("retried after %v", gap)
	}
	h.idle(t)
}
