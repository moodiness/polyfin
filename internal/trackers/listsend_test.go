package trackers

import (
	"net/http"
	"slices"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/library"
)

// MDBList's watchlist fills the watchlist, a page at a time, and is read
// again only once its last activities tell it changed; the import sends
// nothing.
func TestMDBListWatchlistIsReadOnceChanged(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	user := h.user(t, "hal")
	h.connected(t, user, MDBList, "mdblist-hal")
	if status := h.status(t, user, MDBList); !status.Watchlist || !status.WatchlistSent || status.ImportWatchlist {
		t.Errorf("MDBList connected: %+v", status)
	}
	var mu sync.Mutex
	changed := "2026-10-01T10:00:00Z"
	pages := map[string]string{
		"":   `{"movies":[{"id":501,"title":"First","imdb_id":"tt0000001","ids":{"imdb":"tt0000001","tmdb":501},"mediatype":"movie"}],"shows":[],"pagination":{"next_cursor":"c2"}}`,
		"c2": `{"movies":[{"id":9,"title":"Unknown","mediatype":"movie"}],"shows":[{"id":502,"title":"Show","imdb_id":"tt0100","tvdb_id":20,"mediatype":"show"}],"pagination":{}}`,
	}
	h.f.on("GET", "/mdblist/sync/last_activities", func(request) answer {
		mu.Lock()
		defer mu.Unlock()
		return answer{status: http.StatusOK, body: `{"watchlisted_at":"` + changed + `"}`}
	})
	h.f.on("GET", "/mdblist/watchlist/items", func(r request) answer {
		mu.Lock()
		defer mu.Unlock()
		if r.query.Get("sort") != "added" || r.query.Get("order") != "desc" {
			t.Errorf("read as %v", r.query)
		}
		return answer{status: http.StatusOK, body: pages[r.query.Get("cursor")]}
	})
	result := h.turnOnWatchlist(t, user, MDBList)
	movie, show := h.idOf(t, library.TitleRef{IMDb: "tt0000001"}), h.idOf(t, library.TitleRef{Series: true, IMDb: "tt0100"})
	if got := h.watchlistOf(t, user); !slices.Contains(got, movie) || !slices.Contains(got, show) || result.Watchlist != 2 {
		t.Fatalf("imported: %v (%+v)", got, result)
	}
	reads := len(h.f.sent("mdblist", "/watchlist/items"))
	if reads != 2 {
		t.Errorf("%d pages read, want 2", reads)
	}

	// Unchanged, it is not read again, and nothing leaves.
	mu.Lock()
	pages[""] = `{"movies":[],"shows":[]}`
	mu.Unlock()
	h.importNow(t, user, MDBList)
	if n := len(h.f.sent("mdblist", "/watchlist/items")); n != reads {
		t.Errorf("read again unchanged: %d pages", n-reads)
	}
	if got := h.watchlistOf(t, user); !slices.Contains(got, movie) {
		t.Errorf("unchanged, the movie left: %v", got)
	}

	// Changed, it is read whole: the movie it no longer holds leaves.
	mu.Lock()
	changed = "2026-10-02T10:00:00Z"
	pages[""] = `{"movies":[],"shows":[{"id":502,"imdb_id":"tt0100","mediatype":"show"}]}`
	mu.Unlock()
	h.importNow(t, user, MDBList)
	if got := h.watchlistOf(t, user); slices.Contains(got, movie) || !slices.Contains(got, show) {
		t.Errorf("changed without the movie: %v", got)
	}

	// Turned off and on, it is read whole again, unchanged or not.
	if _, err := h.SetWatchlistImport(t.Context(), user, MDBList, false); err != nil {
		t.Fatal(err)
	}
	before := len(h.f.sent("mdblist", "/watchlist/items"))
	h.turnOnWatchlist(t, user, MDBList)
	if n := len(h.f.sent("mdblist", "/watchlist/items")); n == before {
		t.Error("turned on again, not read")
	}
	h.idle(t)
	h.noWrites(t)
}

// posted lists the changes service received at path: its requests but
// reads.
func (f *fakes) posted(service, path string) []request {
	var found []request
	for _, r := range f.sent(service, path) {
		if r.method != http.MethodGet {
			found = append(found, r)
		}
	}
	return found
}

// waitPosted waits until service received n changes at path.
func (f *fakes) waitPosted(t *testing.T, n int, service, path string) {
	t.Helper()
	eventually(t, service+" receiving "+path, func() bool { return len(f.posted(service, path)) >= n })
}

// The user's own changes go to Trakt, Simkl and MDBList once they sync
// their watchlist there, all the titles of one change in one request.
// Simkl's statuses are asked first, so that a title it lists elsewhere is
// neither moved nor deleted. Trakt's full watchlist shows until a change
// goes through.
func TestTraktSimklAndMDBListGetTheUsersOwnChanges(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	user := h.user(t, "ida")
	services := []string{Trakt, Simkl, MDBList}
	for _, service := range services {
		h.connected(t, user, service, service+"-ida")
	}
	h.f.reply("GET", "/trakt/sync/watchlist", http.StatusOK, `[]`)
	movie := Title{Item: itemID(1), TMDB: 10}
	series := Title{Item: itemID(2), TMDB: 30, IMDb: "tt0100", Series: true}
	animeOnly := Title{Item: itemID(3), Series: true, Anime: &AnimeID{ID: 7}}

	// Not synced: nothing is sent.
	h.Watchlist(user, ownChange([]Title{movie}, nil))
	h.idle(t)
	for _, service := range services {
		if n := len(h.f.posted(service, "/sync/watchlist")) + len(h.f.posted(service, "/sync/watched")) + len(h.f.posted(service, "/watchlist/items/add")); n != 0 {
			t.Errorf("%s: sent %d while not synced", service, n)
		}
	}
	// Turned on: nothing of the watchlist is sent.
	for _, service := range services {
		h.turnOnWatchlist(t, user, service)
	}
	h.idle(t)
	h.noWrites(t)

	// Simkl lists the movie nowhere, or in Plan to Watch unwatched, and the
	// series as completed.
	var mu sync.Mutex
	movieList := "null"
	h.f.on("POST", "/simkl/sync/watched", func(r request) answer {
		mu.Lock()
		defer mu.Unlock()
		asked, _ := r.body.([]any)
		body := "["
		for i, item := range asked {
			if i > 0 {
				body += ","
			}
			ids, _ := item.(map[string]any)["ids"].(map[string]any)
			switch ids["tmdb"] {
			case float64(10):
				body += `{"result":` + map[bool]string{true: "false", false: "true"}[movieList == "null"] + `,"list":` + movieList + `,"last_watched_at":null}`
			case float64(30):
				body += `{"result":true,"list":"completed","last_watched_at":"2026-05-15T00:35:15Z"}`
			case float64(11):
				// The user has it, yet no list is told.
				body += `{"result":true,"last_watched_at":null}`
			case float64(13):
				body += `{"result":"unknown","list":null,"last_watched_at":null}`
			default:
				body += `{"result":"not_found"}`
			}
		}
		return answer{status: http.StatusOK, body: body + "]"}
	})

	h.Watchlist(user, ownChange([]Title{movie, series, animeOnly}, nil))
	h.f.waitPosted(t, 1, "trakt", "/sync/watchlist")
	h.f.waitPosted(t, 1, "mdblist", "/watchlist/items/add")
	h.f.waitPosted(t, 1, "simkl", "/sync/add-to-list")
	h.idle(t)
	both := `{"movies":[{"ids":{"tmdb":10}}],"shows":[{"ids":{"imdb":"tt0100","tmdb":30}}]}`
	adds := h.f.posted("trakt", "/sync/watchlist")
	if len(adds) != 1 || adds[0].method != http.MethodPost {
		t.Fatalf("Trakt: %+v", adds)
	}
	sameJSON(t, "Trakt add", adds[0].body, both)
	sameJSON(t, "MDBList add", h.f.sent("mdblist", "/watchlist/items/add")[0].body, both)
	sameJSON(t, "Simkl lookup", h.f.sent("simkl", "/sync/watched")[0].body,
		`[{"ids":{"tmdb":10},"type":"movie"},{"ids":{"imdb":"tt0100","tmdb":30},"type":"show"}]`)
	// The completed series is not moved to Plan to Watch.
	sameJSON(t, "Simkl add", h.f.sent("simkl", "/sync/add-to-list")[0].body, `{"movies":[{"ids":{"tmdb":10},"to":"plantowatch"}]}`)

	// Simkl adds only what it plainly says the user does not have: one it
	// does not know, not one it answers oddly for.
	odd, unknown, strange := Title{Item: itemID(4), TMDB: 11}, Title{Item: itemID(5), TMDB: 12}, Title{Item: itemID(6), TMDB: 13}
	h.Watchlist(user, ownChange([]Title{odd, unknown, strange}, nil))
	h.f.waitPosted(t, 2, "simkl", "/sync/add-to-list")
	h.idle(t)
	sameJSON(t, "Simkl add of odd answers", h.f.sent("simkl", "/sync/add-to-list")[1].body, `{"movies":[{"ids":{"tmdb":12},"to":"plantowatch"}]}`)

	// Removed: everywhere, Simkl only taking off the movie in Plan to
	// Watch, never the completed series.
	mu.Lock()
	movieList = `"plantowatch"`
	mu.Unlock()
	h.Watchlist(user, ownChange(nil, []Title{series, movie}))
	h.f.waitPosted(t, 1, "trakt", "/sync/watchlist/remove")
	h.f.waitPosted(t, 1, "mdblist", "/watchlist/items/remove")
	h.f.waitPosted(t, 1, "simkl", "/sync/history/remove")
	h.idle(t)
	sameJSON(t, "Trakt removal", h.f.sent("trakt", "/sync/watchlist/remove")[0].body, both)
	sameJSON(t, "MDBList removal", h.f.sent("mdblist", "/watchlist/items/remove")[0].body, both)
	sameJSON(t, "Simkl removal", h.f.sent("simkl", "/sync/history/remove")[0].body, `{"movies":[{"ids":{"tmdb":10}}]}`)

	// Nothing to change on Simkl: no write.
	h.Watchlist(user, ownChange(nil, []Title{series}))
	h.f.waitPosted(t, 4, "simkl", "/sync/watched")
	h.idle(t)
	if n := len(h.f.sent("simkl", "/sync/history/remove")); n != 1 {
		t.Errorf("Simkl removed the completed series: %d removals", n)
	}

	// Trakt's watchlist full: the change is dropped, and the connection
	// tells it until a change goes through.
	h.f.reply("POST", "/trakt/sync/watchlist", statusWatchlistFull, ``)
	h.Watchlist(user, ownChange([]Title{movie}, nil))
	h.f.waitPosted(t, 3, "trakt", "/sync/watchlist")
	h.idle(t)
	if status := h.status(t, user, Trakt); !status.WatchlistFull {
		t.Errorf("Trakt full: %+v", status)
	}
	if status := h.status(t, user, MDBList); status.WatchlistFull {
		t.Errorf("MDBList full: %+v", status)
	}
	if n := len(h.f.posted("trakt", "/sync/watchlist")); n != 3 {
		t.Errorf("a full watchlist was tried %d times", n-2)
	}
	h.f.reply("POST", "/trakt/sync/watchlist", http.StatusCreated, `{}`)
	h.Watchlist(user, ownChange([]Title{movie}, nil))
	h.f.waitPosted(t, 4, "trakt", "/sync/watchlist")
	h.idle(t)
	if status := h.status(t, user, Trakt); status.WatchlistFull {
		t.Errorf("Trakt still full once a change went through: %+v", status)
	}
	var queued int
	if err := h.db.QueryRow(t.Context(), "SELECT count(*) FROM tracking_events").Scan(&queued); err != nil || queued != 0 {
		t.Errorf("%d changes left queued: %v", queued, err)
	}

	// Turned off, the full watchlist is forgotten, and nothing is sent.
	h.f.reply("POST", "/trakt/sync/watchlist", statusWatchlistFull, ``)
	h.Watchlist(user, ownChange([]Title{movie}, nil))
	h.f.waitPosted(t, 5, "trakt", "/sync/watchlist")
	h.idle(t)
	for _, service := range services {
		if _, err := h.SetWatchlistImport(t.Context(), user, service, false); err != nil {
			t.Fatal(err)
		}
	}
	if status := h.status(t, user, Trakt); status.WatchlistFull {
		t.Errorf("turned off, still full: %+v", status)
	}
	h.Watchlist(user, ownChange([]Title{movie}, nil))
	h.idle(t)
	if n := len(h.f.posted("trakt", "/sync/watchlist")); n != 5 {
		t.Errorf("sent once turned off: %d", n-5)
	}
}
