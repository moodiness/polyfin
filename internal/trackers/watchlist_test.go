package trackers

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// watchlistOf lists user's watchlist as the Watchlist view does.
func (h harness) watchlistOf(t *testing.T, user accounts.ID) []accounts.ID {
	t.Helper()
	ids, err := watchlist.New(h.db).Titles(t.Context(), user)
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

// turnOnWatchlist turns the import of user's watchlist from service on,
// which imports it.
func (h harness) turnOnWatchlist(t *testing.T, user accounts.ID, service string) ImportResult {
	t.Helper()
	return h.imported(t, user, service, func() {
		if _, err := h.SetWatchlistImport(t.Context(), user, service, true); err != nil {
			t.Fatal(err)
		}
	})
}

func (h harness) importNow(t *testing.T, user accounts.ID, service string) ImportResult {
	t.Helper()
	return h.imported(t, user, service, func() {
		if _, err := h.ImportNow(t.Context(), user, service); err != nil {
			t.Fatal(err)
		}
	})
}

func TestTraktWatchlistFillsTheWatchlist(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "dave")
	h.connected(t, user, Trakt, "trakt-dave")
	h.connected(t, user, MDBList, "mdblist-dave")
	if status := h.status(t, user, Trakt); !status.Watchlist || status.ImportWatchlist {
		t.Errorf("Trakt connected: %+v", status)
	}
	if status := h.status(t, user, MDBList); status.Watchlist {
		t.Errorf("MDBList connected: %+v", status)
	}
	if _, err := h.SetWatchlistImport(t.Context(), user, MDBList, true); !errors.Is(err, ErrUnknownService) {
		t.Errorf("MDBList's watchlist: %v", err)
	}
	pages := [][]string{
		{`{"listed_at":"2026-09-01T10:00:00.000Z","type":"movie","movie":{"title":"First","ids":{"imdb":"tt0000001"}}}`,
			`{"listed_at":"2026-09-02T10:00:00.000Z","type":"show","show":{"title":"Show","ids":{"imdb":"tt0100","tvdb":20}}}`,
			// A season and an episode count for their show.
			`{"listed_at":"2026-09-03T10:00:00.000Z","type":"season","season":{"number":2},"show":{"ids":{"imdb":"tt0200"}}}`,
			`{"listed_at":"2026-09-04T10:00:00.000Z","type":"episode","episode":{"season":1,"number":3},"show":{"ids":{"imdb":"tt0300"}}}`,
			// Known by no identifier Polyfin finds titles by.
			`{"listed_at":"2026-09-05T10:00:00.000Z","type":"movie","movie":{"title":"Unknown","ids":{"trakt":7}}}`},
		{`{"listed_at":"2026-09-06T10:00:00.000Z","type":"movie","movie":{"ids":{"imdb":"tt0000002"}}}`},
	}
	h.f.on("GET", "/trakt/sync/watchlist", func(r request) answer {
		page, _ := strconv.Atoi(r.query.Get("page"))
		return answer{status: http.StatusOK, body: "[" + strings.Join(pages[page-1], ",") + "]",
			header: http.Header{"X-Pagination-Page-Count": {strconv.Itoa(len(pages))}}}
	})

	// Turned on alone, it reads the watchlist, not the history.
	result := h.turnOnWatchlist(t, user, Trakt)
	if result.Watchlist != 5 || result.Unmapped != 1 || result.Played != 0 || result.Problem != "" {
		t.Errorf("result: %+v", result)
	}
	if n := len(h.f.sent("trakt", "/sync/last_activities")) + len(h.f.sent("trakt", "/sync/history/movies")); n != 0 {
		t.Errorf("the history was read: %d requests", n)
	}
	if status := h.status(t, user, Trakt); status.ImportHistory || !status.ImportWatchlist || status.LastImport == nil ||
		status.LastImport.Watchlist != 5 {
		t.Errorf("status: %+v", status)
	}
	movie, second := library.TitleRef{IMDb: "tt0000001"}, library.TitleRef{IMDb: "tt0000002"}
	show := func(imdb string) library.TitleRef { return library.TitleRef{Series: true, IMDb: imdb} }
	ids := func(refs ...library.TitleRef) []accounts.ID {
		result := make([]accounts.ID, len(refs))
		for i, ref := range refs {
			result[i] = h.idOf(t, ref)
		}
		return result
	}
	if got := h.watchlistOf(t, user); !slices.Equal(got, ids(second, show("tt0300"), show("tt0200"), show("tt0100"), movie)) {
		t.Fatalf("watchlist: %v", got)
	}

	// Taken off Trakt, the first movie leaves at the next import.
	pages = [][]string{pages[0][1:4], pages[1]}
	if result := h.importNow(t, user, Trakt); result.Watchlist != 0 || result.Unmapped != 0 {
		t.Errorf("next import: %+v", result)
	}
	if got := h.watchlistOf(t, user); !slices.Equal(got, ids(second, show("tt0300"), show("tt0200"), show("tt0100"))) {
		t.Errorf("after the next import: %v", got)
	}
	// One that cannot read the list whole takes nothing off.
	h.f.reply("GET", "/trakt/sync/watchlist", http.StatusNotFound, `{}`)
	if result := h.importNow(t, user, Trakt); result.Problem != ProblemUnreachable {
		t.Errorf("failed import: %+v", result)
	}
	if got := h.watchlistOf(t, user); len(got) != 4 {
		t.Errorf("after a failed import: %v", got)
	}

	// Turned off, nothing is imported, and what the list alone held
	// leaves: a one-off app import holds the second movie too.
	if _, err := watchlist.New(h.db).Save(t.Context(), user, watchlist.Import, []watchlist.Title{{ID: h.idOf(t, second)}}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := h.SetWatchlistImport(t.Context(), user, Trakt, false); err != nil {
		t.Fatal(err)
	}
	if _, err := h.ImportNow(t.Context(), user, Trakt); !errors.Is(err, ErrImportOff) {
		t.Errorf("import now, both off: %v", err)
	}
	if got := h.watchlistOf(t, user); !slices.Equal(got, ids(second)) {
		t.Errorf("turned off: %v", got)
	}
	// Importing sends nothing back (disconnecting revokes the token).
	h.noWrites(t)
	// Turned on again, then disconnected: the same.
	h.f.on("GET", "/trakt/sync/watchlist", func(request) answer {
		return answer{status: http.StatusOK, body: "[" + strings.Join(pages[0], ",") + "]"}
	})
	h.turnOnWatchlist(t, user, Trakt)
	if got := h.watchlistOf(t, user); len(got) != 4 {
		t.Errorf("turned on again: %v", got)
	}
	if err := h.Disconnect(t.Context(), user, Trakt); err != nil {
		t.Fatal(err)
	}
	if got := h.watchlistOf(t, user); !slices.Equal(got, ids(second)) {
		t.Errorf("disconnected: %v", got)
	}
}

func TestSimklPlanToWatchFillsTheWatchlist(t *testing.T) {
	h := newHarness(t)
	h.anime = animeMapping(t)
	user := h.user(t, "erin")
	h.connected(t, user, Simkl, "simkl-erin")
	h.f.reply("GET", "/simkl/sync/all-items/movies/plantowatch", http.StatusOK,
		`{"movies":[{"added_to_watchlist_at":"2026-09-01T10:00:00Z","movie":{"title":"First","ids":{"simkl":1,"imdb":"tt0000001"}}}]}`)
	shows := `{"shows":[{"added_to_watchlist_at":"2026-09-02T10:00:00Z","show":{"title":"Show","ids":{"simkl":2,"imdb":"tt0100"}}}]}`
	h.f.on("GET", "/simkl/sync/all-items/shows/plantowatch", func(request) answer { return answer{status: http.StatusOK, body: shows} })
	// An anime series and an anime movie, found through the anime mapping,
	// and an anime it does not know.
	h.f.reply("GET", "/simkl/sync/all-items/anime/plantowatch", http.StatusOK, `{"anime":[
		{"added_to_watchlist_at":"2026-09-03T10:00:00Z","anime_type":"tv","show":{"title":"First","ids":{"anidb":1}}},
		{"added_to_watchlist_at":"2026-09-04T10:00:00Z","anime_type":"movie","show":{"title":"The movie","ids":{"anidb":4}}},
		{"added_to_watchlist_at":"2026-09-05T10:00:00Z","anime_type":"tv","show":{"title":"Unknown","ids":{"anidb":99}}}]}`)

	result := h.turnOnWatchlist(t, user, Simkl)
	if result.Watchlist != 4 || result.Unmapped != 1 || result.Problem != "" {
		t.Errorf("result: %+v", result)
	}
	if read := h.f.sent("simkl", "/sync/all-items/movies/plantowatch"); len(read) != 1 || read[0].query.Get("extended") != "full" {
		t.Errorf("read: %+v", read)
	}
	want := []accounts.ID{h.idOf(t, library.TitleRef{IMDb: "tt0000400"}), h.idOf(t, library.TitleRef{Series: true, IMDb: "tt0000100"}),
		h.idOf(t, library.TitleRef{Series: true, IMDb: "tt0100"}), h.idOf(t, library.TitleRef{IMDb: "tt0000001"})}
	if got := h.watchlistOf(t, user); !slices.Equal(got, want) {
		t.Fatalf("watchlist: %v, want %v", got, want)
	}

	// The show taken off the list leaves at the next import.
	shows = `{}`
	h.importNow(t, user, Simkl)
	if got := h.watchlistOf(t, user); !slices.Equal(got, slices.Delete(want, 2, 3)) {
		t.Errorf("after the next import: %v", got)
	}
	h.noWrites(t)
}
