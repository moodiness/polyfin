package trackers

import (
	"maps"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// droppedOf lists the series kept dropped for user on service: the date
// the service gave, and when an import first found them, at clockStart.
func (h harness) droppedOf(t *testing.T, user accounts.ID, service string) map[accounts.ID]*time.Time {
	t.Helper()
	rows, err := h.db.Query(t.Context(), "SELECT series_id, dropped_at, seen_at FROM tracker_dropped WHERE user_id = $1 AND service = $2",
		user, service)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	kept := map[accounts.ID]*time.Time{}
	for rows.Next() {
		var series accounts.ID
		var at *time.Time
		var seen time.Time
		if err := rows.Scan(&series, &at, &seen); err != nil {
			t.Fatal(err)
		}
		if !seen.Equal(clockStart) {
			t.Errorf("%v seen at %v", series, seen)
		}
		kept[series] = at
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return kept
}

// cursorOf is what column of tracking_imports holds for user's imports
// from service: "cursor" for the history, "dropped_cursor" for the shows
// dropped.
func (h harness) cursorOf(t *testing.T, user accounts.ID, service, column string) string {
	t.Helper()
	var cursor string
	if err := h.db.QueryRow(t.Context(), "SELECT "+column+" FROM tracking_imports WHERE user_id = $1 AND service = $2",
		user, service).Scan(&cursor); err != nil {
		t.Fatal(err)
	}
	return cursor
}

// sameDropped fails unless got holds exactly the series of want, with
// their dates.
func sameDropped(t *testing.T, what string, got, want map[accounts.ID]*time.Time) {
	t.Helper()
	if !maps.EqualFunc(got, want, func(a, b *time.Time) bool { return a == nil && b == nil || a != nil && b != nil && a.Equal(*b) }) {
		t.Errorf("%s: dropped %v, want %v", what, got, want)
	}
}

// series is the series item Polyfin names by imdb.
func (h harness) series(t *testing.T, imdb string) accounts.ID {
	t.Helper()
	return h.idOf(t, library.TitleRef{Series: true, IMDb: imdb})
}

// emptyHistory has the fake of service answer an empty history and no
// resume points, where it would answer an object.
func (h harness) emptyHistory(service string) {
	h.f.reply("GET", "/"+service+"/sync/playback", http.StatusOK, `[]`)
	if service == Trakt {
		h.f.reply("GET", "/trakt/sync/history/movies", http.StatusOK, `[]`)
		h.f.reply("GET", "/trakt/sync/history/episodes", http.StatusOK, `[]`)
	}
}

func TestTraktDroppedShowsAreKeptWhileListed(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	user := h.user(t, "gina")
	h.connected(t, user, Trakt, "trakt-gina")
	h.emptyHistory(Trakt)
	activities := func(droppedAt, watchedAt string) {
		h.f.reply("GET", "/trakt/sync/last_activities", http.StatusOK,
			`{"episodes":{"watched_at":"`+watchedAt+`"},"shows":{"dropped_at":"`+droppedAt+`"}}`)
	}
	listed := func(pages ...[]string) {
		h.f.on("GET", "/trakt/users/hidden/dropped", func(r request) answer {
			page, _ := strconv.Atoi(r.query.Get("page"))
			return answer{status: http.StatusOK, body: "[" + strings.Join(pages[page-1], ",") + "]",
				header: http.Header{"X-Pagination-Page-Count": {strconv.Itoa(len(pages))}}}
		})
	}
	first := `{"hidden_at":"2026-09-20T10:00:00.000Z","type":"show","show":{"title":"First","ids":{"trakt":1,"imdb":"tt0100","tmdb":30}}}`
	second := `{"hidden_at":"2026-09-25T10:00:00.000Z","type":"show","show":{"title":"Second","ids":{"trakt":2,"imdb":"tt0200"}}}`
	activities("2026-10-01T10:00:00.000Z", "")
	listed([]string{first}, []string{second,
		// Known only by TMDB, which Polyfin never listed: left out, and
		// not counted.
		`{"hidden_at":"2026-09-26T10:00:00.000Z","type":"show","show":{"ids":{"trakt":3,"tmdb":99}}}`})

	if result := h.turnOn(t, user, Trakt); result.Unmapped != 0 || result.Problem != "" {
		t.Errorf("result: %+v", result)
	}
	pages := h.f.sent("trakt", "/users/hidden/dropped")
	if len(pages) != 2 || pages[0].query.Get("type") != "show" || pages[1].query.Get("page") != "2" {
		t.Errorf("pages: %+v", pages)
	}
	both := map[accounts.ID]*time.Time{h.series(t, "tt0100"): at("2026-09-20T10:00:00Z"), h.series(t, "tt0200"): at("2026-09-25T10:00:00Z")}
	sameDropped(t, "first import", h.droppedOf(t, user, Trakt), both)

	// Nothing dropped or taken back since: not read again.
	h.importNow(t, user, Trakt)
	if n := len(h.f.sent("trakt", "/users/hidden/dropped")); n != 2 {
		t.Errorf("read again unchanged: %d requests", n)
	}

	// The first taken back leaves.
	activities("2026-10-03T10:00:00.000Z", "")
	listed([]string{second})
	h.importNow(t, user, Trakt)
	secondOnly := map[accounts.ID]*time.Time{h.series(t, "tt0200"): at("2026-09-25T10:00:00Z")}
	sameDropped(t, "taken back", h.droppedOf(t, user, Trakt), secondOnly)
	cursor := h.cursorOf(t, user, Trakt, "dropped_cursor")

	// A read that fails keeps those kept, to be read again at the next
	// import, and the history is imported all the same.
	activities("2026-10-04T10:00:00.000Z", "2026-10-04T09:00:00.000Z")
	h.f.reply("GET", "/trakt/sync/history/episodes", http.StatusOK,
		`[{"watched_at":"2026-10-04T09:00:00.000Z","type":"episode","episode":{"season":1,"number":2},"show":{"ids":{"imdb":"tt0100"}}}]`)
	h.f.reply("GET", "/trakt/users/hidden/dropped", http.StatusNotFound, `{}`)
	if result := h.importNow(t, user, Trakt); result.Played != 1 || result.Problem != "" {
		t.Errorf("failed read: %+v", result)
	}
	if got := h.cursorOf(t, user, Trakt, "cursor"); got != "|2026-10-04T09:00:00.000Z" {
		t.Errorf("history cursor after a failed read: %q", got)
	}
	sameDropped(t, "failed read", h.droppedOf(t, user, Trakt), secondOnly)
	if got := h.cursorOf(t, user, Trakt, "dropped_cursor"); got != cursor {
		t.Errorf("cursor after a failed read: %q, want %q", got, cursor)
	}
	listed([]string{first, second})
	h.importNow(t, user, Trakt)
	sameDropped(t, "read again", h.droppedOf(t, user, Trakt), both)

	// A service asking to wait too long stops the import, as anywhere in
	// its read.
	activities("2026-10-05T10:00:00.000Z", "2026-10-04T09:00:00.000Z")
	h.f.on("GET", "/trakt/users/hidden/dropped", func(request) answer {
		return answer{status: http.StatusTooManyRequests, body: `{}`, header: http.Header{"Retry-After": {"60"}}}
	})
	if result := h.importNow(t, user, Trakt); result.Problem != ProblemRateLimited {
		t.Errorf("rate limited: %+v", result)
	}
	sameDropped(t, "rate limited", h.droppedOf(t, user, Trakt), both)

	// Turned off, the history no longer drops anything; turned on, the
	// dropped shows are read anew.
	if _, err := h.SetImport(t.Context(), user, Trakt, false); err != nil {
		t.Fatal(err)
	}
	if got := h.droppedOf(t, user, Trakt); len(got) != 0 || h.cursorOf(t, user, Trakt, "dropped_cursor") != "" {
		t.Errorf("turned off: %v", got)
	}
	listed([]string{first, second})
	h.turnOn(t, user, Trakt)
	sameDropped(t, "turned on again", h.droppedOf(t, user, Trakt), both)

	h.noWrites(t)

	// Disconnected, nothing is kept.
	if err := h.Disconnect(t.Context(), user, Trakt); err != nil {
		t.Fatal(err)
	}
	if got := h.droppedOf(t, user, Trakt); len(got) != 0 {
		t.Errorf("disconnected: %v", got)
	}
}

// A user who never dropped anything on Trakt has nothing read.
func TestTraktDroppedShowsAreNotReadUntilOneIs(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	user := h.user(t, "hal")
	h.connected(t, user, Trakt, "trakt-hal")
	h.emptyHistory(Trakt)
	h.f.reply("GET", "/trakt/sync/last_activities", http.StatusOK, `{"shows":{"dropped_at":null}}`)
	h.turnOn(t, user, Trakt)
	if n := len(h.f.sent("trakt", "/users/hidden/dropped")); n != 0 {
		t.Errorf("%d reads", n)
	}
}

func TestSimklDroppedShowsAndAnimeAreKept(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.anime = animeMapping(t)
	h.listAnime(t)
	user := h.user(t, "hana")
	h.connected(t, user, Simkl, "simkl-hana")
	h.emptyHistory(Simkl)
	activities := func(shows string) {
		h.f.reply("GET", "/simkl/sync/activities", http.StatusOK,
			`{"all":"`+shows+`","tv_shows":{"all":"`+shows+`"},"anime":{"all":"2026-09-01T00:00:00Z"}}`)
	}
	// The history's reads of the same lists find nothing.
	dropped := func(list, body string) {
		h.f.on("GET", "/simkl/sync/all-items/"+list, func(r request) answer {
			if r.query.Get("extended") != "ids_only" {
				return answer{status: http.StatusOK, body: `{}`}
			}
			return answer{status: http.StatusOK, body: body}
		})
	}
	activities("2026-10-01T00:00:00Z")
	dropped("shows/dropped", `{"shows":[{"show":{"title":"Show","ids":{"simkl":2,"imdb":"tt0100"}}}]}`)
	// On hold is not dropped.
	dropped("shows/hold", `{"shows":[{"show":{"title":"Held","ids":{"simkl":3,"imdb":"tt0200"}}}]}`)
	dropped("anime/hold", `{"anime":[{"show":{"title":"Long","ids":{"simkl":901,"mal":"23"}},"anime_type":"tv"}]}`)
	// The second entry of the split series drops the series, and the one
	// an anime catalog lists; the movie and the anime no mapping knows are
	// left out.
	dropped("anime/dropped", `{"anime":[
		{"show":{"title":"Second","ids":{"simkl":900,"anidb":"2","mal":"22"}},"anime_type":"tv"},
		{"show":{"title":"The movie","ids":{"simkl":903,"kitsu":"14"}},"anime_type":"movie"},
		{"show":{"title":"Unknown","ids":{"simkl":902,"mal":"99"}},"anime_type":"tv"}]}`)

	if result := h.turnOn(t, user, Simkl); result.Unmapped != 0 || result.Problem != "" {
		t.Errorf("result: %+v", result)
	}
	reads := func(list string) int {
		n := 0
		for _, r := range h.f.sent("simkl", "/sync/all-items/"+list) {
			if r.query.Get("extended") == "ids_only" {
				n++
				if r.query.Get("date_from") != "" {
					t.Errorf("%s read from %s", list, r.query.Get("date_from"))
				}
			}
		}
		return n
	}
	if reads("shows/dropped") != 1 || reads("anime/dropped") != 1 || reads("shows/hold") != 0 || reads("anime/hold") != 0 {
		t.Errorf("reads: shows %d, anime %d", reads("shows/dropped"), reads("anime/dropped"))
	}
	anime := map[accounts.ID]*time.Time{h.series(t, "tt0000100"): nil, itemOf("series|kitsu:12"): nil}
	all := maps.Clone(anime)
	all[h.series(t, "tt0100")] = nil
	sameDropped(t, "first import", h.droppedOf(t, user, Simkl), all)

	// Unchanged: not read again.
	h.importNow(t, user, Simkl)
	if reads("shows/dropped") != 1 || reads("anime/dropped") != 1 {
		t.Errorf("read again unchanged: shows %d, anime %d", reads("shows/dropped"), reads("anime/dropped"))
	}

	// The show taken back leaves; the anime stays.
	activities("2026-10-03T00:00:00Z")
	dropped("shows/dropped", `{}`)
	h.importNow(t, user, Simkl)
	sameDropped(t, "taken back", h.droppedOf(t, user, Simkl), anime)
	h.noWrites(t)
}

func TestMDBListDroppedShowsAreKept(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	user := h.user(t, "iris")
	h.connected(t, user, MDBList, "mdblist-iris")
	h.emptyHistory(MDBList)
	activities := func(droppedAt, episodesAt string) {
		h.f.reply("GET", "/mdblist/sync/last_activities", http.StatusOK,
			`{"watched_at":"2026-10-01T00:00:00Z","episode_watched_at":"`+episodesAt+`","dropped_at":"`+droppedAt+`"}`)
	}
	listed := func(pages ...string) {
		h.f.on("GET", "/mdblist/sync/dropped", func(r request) answer {
			offset, _ := strconv.Atoi(r.query.Get("offset"))
			page := offset / 1000
			return answer{status: http.StatusOK, body: `{"shows":[` + pages[page] + `],"pagination":{"has_more":` +
				strconv.FormatBool(page+1 < len(pages)) + `}}`}
		})
	}
	first := `{"dropped_at":"2026-09-20T10:00:00Z","show":{"title":"First","ids":{"imdb":"tt0100","tmdb":30}}}`
	third := `{"dropped_at":"2026-09-21T10:00:00Z","show":{"title":"Third","ids":{"imdb":"tt0300"}}}`
	activities("2026-10-02T00:00:00Z", "2026-10-01T00:00:00Z")
	// A show taken back may stay listed, without a date.
	listed(first, `{"dropped_at":null,"show":{"title":"Second","ids":{"imdb":"tt0200"}}},`+third)

	h.turnOn(t, user, MDBList)
	if pages := h.f.sent("mdblist", "/sync/dropped"); len(pages) != 2 || pages[1].query.Get("offset") != "1000" || pages[0].query.Get("since") != "" {
		t.Errorf("pages: %+v", pages)
	}
	sameDropped(t, "first import", h.droppedOf(t, user, MDBList),
		map[accounts.ID]*time.Time{h.series(t, "tt0100"): at("2026-09-20T10:00:00Z"), h.series(t, "tt0300"): at("2026-09-21T10:00:00Z")})

	// Unchanged: not read again.
	h.importNow(t, user, MDBList)
	if n := len(h.f.sent("mdblist", "/sync/dropped")); n != 2 {
		t.Errorf("read again unchanged: %d requests", n)
	}

	activities("2026-10-04T00:00:00Z", "2026-10-01T00:00:00Z")
	listed(third)
	h.importNow(t, user, MDBList)
	thirdOnly := map[accounts.ID]*time.Time{h.series(t, "tt0300"): at("2026-09-21T10:00:00Z")}
	sameDropped(t, "taken back", h.droppedOf(t, user, MDBList), thirdOnly)
	cursor := h.cursorOf(t, user, MDBList, "dropped_cursor")

	// MDBList's dropped shows are in beta: a read of them that fails keeps
	// those kept, to be read again at the next import, and the history is
	// imported all the same.
	activities("2026-10-05T00:00:00Z", "2026-10-05T00:00:00Z")
	h.f.reply("GET", "/mdblist/sync/watched", http.StatusOK, `{"episodes":[{"last_watched_at":"2026-10-04T21:00:00Z",
		"episode":{"season":1,"number":2,"show":{"ids":{"imdb":"tt0100"}}}}],"pagination":{"has_more":false}}`)
	h.f.reply("GET", "/mdblist/sync/dropped", http.StatusInternalServerError, `{}`)
	if result := h.importNow(t, user, MDBList); result.Played != 1 || result.Problem != "" {
		t.Errorf("failed read: %+v", result)
	}
	if got := h.cursorOf(t, user, MDBList, "cursor"); got != "2026-10-01T00:00:00Z|2026-10-05T00:00:00Z" {
		t.Errorf("history cursor after a failed read: %q", got)
	}
	sameDropped(t, "failed read", h.droppedOf(t, user, MDBList), thirdOnly)
	if got := h.cursorOf(t, user, MDBList, "dropped_cursor"); got != cursor {
		t.Errorf("cursor after a failed read: %q, want %q", got, cursor)
	}
	listed(first)
	h.importNow(t, user, MDBList)
	sameDropped(t, "read again", h.droppedOf(t, user, MDBList), map[accounts.ID]*time.Time{h.series(t, "tt0100"): at("2026-09-20T10:00:00Z")})
	h.noWrites(t)
}

// MDBList takes a title off the history by clearing its date: it is not
// watched.
func TestMDBListTitlesWithoutADateAreNotWatched(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	user := h.user(t, "jack")
	h.connected(t, user, MDBList, "mdblist-jack")
	h.emptyHistory(MDBList)
	h.f.reply("GET", "/mdblist/sync/last_activities", http.StatusOK, `{"watched_at":"2026-10-01T00:00:00Z","episode_watched_at":"2026-10-01T00:00:00Z"}`)
	h.f.reply("GET", "/mdblist/sync/watched", http.StatusOK, `{
		"movies":[{"last_watched_at":null,"movie":{"ids":{"imdb":"tt0000001"}}},
			{"last_watched_at":"2026-09-01T20:00:00Z","movie":{"ids":{"imdb":"tt0000002"}}}],
		"episodes":[{"last_watched_at":null,"episode":{"season":1,"number":1,"show":{"ids":{"imdb":"tt0100"}}}},
			{"last_watched_at":"","episode":{"season":1,"number":2,"show":{"ids":{"imdb":"tt0100"}}}}],
		"pagination":{"has_more":false}}`)

	if result := h.turnOn(t, user, MDBList); result.Played != 1 || result.Unmapped != 0 || result.Problem != "" {
		t.Errorf("result: %+v", result)
	}
	got := h.data(t, user, library.TitleRef{IMDb: "tt0000001"}, library.TitleRef{IMDb: "tt0000002"},
		library.TitleRef{Episode: true, IMDb: "tt0100", Season: 1, Number: 1}, library.TitleRef{Episode: true, IMDb: "tt0100", Season: 1, Number: 2})
	if got[0].Played || !got[1].Played || got[2].Played || got[3].Played {
		t.Errorf("played: %+v", got)
	}
}

func TestAniListDroppedEntriesDropTheirSeries(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.anime = animeMapping(t)
	lists := newFakeLists(t, h)
	user := h.user(t, "kate")
	h.connected(t, user, AniList, "anilist-kate")
	// The split series' second entry dropped; the long series on hold; the
	// movie dropped.
	lists.set(AniList, 32, listed{status: statusDropped, progress: 2})
	lists.set(AniList, 33, listed{status: statusOnHold, progress: 1})
	lists.set(AniList, 34, listed{status: statusDropped})

	if result := h.turnOn(t, user, AniList); result.Problem != "" {
		t.Errorf("result: %+v", result)
	}
	sameDropped(t, "first import", h.droppedOf(t, user, AniList), map[accounts.ID]*time.Time{h.series(t, "tt0000100"): nil})

	// Watched again, it leaves at the next import.
	lists.set(AniList, 32, listed{status: statusWatching, progress: 3})
	h.importNow(t, user, AniList)
	if got := h.droppedOf(t, user, AniList); len(got) != 0 {
		t.Errorf("watched again: %v", got)
	}
}
