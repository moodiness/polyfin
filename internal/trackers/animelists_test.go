package trackers

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/secrets"
)

// listed is an entry of a fake AniList or MyAnimeList list, with the
// entry's episode count, as MyAnimeList names its status.
type listed struct {
	status    string
	progress  int
	score     float64
	episodes  int
	movie     bool
	completed string
}

// fakeLists stands in for AniList's GraphQL API and MyAnimeList's REST API
// on h's fakes, each keeping a list of entries by the service's
// identifiers, and counting the writes they get.
type fakeLists struct {
	mu      sync.Mutex
	entries map[string]map[int]*listed
	// media are the episode counts of the entries not listed.
	media  map[int]int
	writes map[string]int
}

func (l *fakeLists) entry(service string, id int) *listed {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e := l.entries[service][id]; e != nil {
		copied := *e
		return &copied
	}
	return nil
}

func (l *fakeLists) writesTo(service string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.writes[service]
}

// set lists an entry of service.
func (l *fakeLists) set(service string, id int, e listed) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries[service][id] = &e
}

// newFakeLists serves both services' lists, empty, where every entry has
// 12 episodes but 34 and 24, the movie, and 33 and 23, the long series,
// which has 100.
func newFakeLists(t *testing.T, h harness) *fakeLists {
	t.Helper()
	l := &fakeLists{entries: map[string]map[int]*listed{AniList: {}, MyAnimeList: {}}, writes: map[string]int{}}
	episodes := func(id int) (int, bool) {
		switch id {
		case 34, 24:
			return 1, true
		case 33, 23:
			return 100, false
		}
		return 12, false
	}
	h.f.on("POST", "/anilist/", func(r request) answer {
		body, _ := r.body.(map[string]any)
		query, _ := body["query"].(string)
		variables, _ := body["variables"].(map[string]any)
		id := 0
		if n, ok := variables["id"].(float64); ok {
			id = int(n)
		}
		l.mu.Lock()
		defer l.mu.Unlock()
		switch {
		case strings.Contains(query, "SaveMediaListEntry"):
			l.writes[AniList]++
			e := l.entries[AniList][id]
			if e == nil {
				e = &listed{}
				l.entries[AniList][id] = e
			}
			if p, ok := variables["progress"].(float64); ok {
				e.progress = int(p)
			}
			if s, ok := variables["status"].(string); ok {
				e.status = anilistStatuses[s]
			}
			if s, ok := variables["score"].(float64); ok {
				e.score = s / 10
			}
			return answer{status: http.StatusOK, body: `{"data":{"SaveMediaListEntry":{"id":1}}}`}
		case strings.Contains(query, "MediaListCollection"):
			var entries []string
			for id, e := range l.entries[AniList] {
				count, movie := episodes(id)
				format := "TV"
				if movie {
					format = "MOVIE"
				}
				completed := `{"year":null,"month":null,"day":null}`
				if e.completed != "" {
					d, _ := time.Parse(time.DateOnly, e.completed)
					completed = fmt.Sprintf(`{"year":%d,"month":%d,"day":%d}`, d.Year(), d.Month(), d.Day())
				}
				entries = append(entries, fmt.Sprintf(`{"status":%q,"progress":%d,"score":%g,"completedAt":%s,"media":{"id":%d,"episodes":%d,"format":%q}}`,
					anilistStatus(e.status), e.progress, e.score*10, completed, id, count, format))
			}
			return answer{status: http.StatusOK, body: `{"data":{"MediaListCollection":{"hasNextChunk":false,"lists":[{"entries":[` +
				strings.Join(entries, ",") + `]}]}}}`}
		case strings.Contains(query, "Media("):
			count, movie := episodes(id)
			format := "TV"
			if movie {
				format = "MOVIE"
			}
			listEntry := "null"
			if e := l.entries[AniList][id]; e != nil {
				listEntry = fmt.Sprintf(`{"status":%q,"progress":%d}`, anilistStatus(e.status), e.progress)
			}
			return answer{status: http.StatusOK, body: fmt.Sprintf(`{"data":{"Media":{"id":%d,"episodes":%d,"format":%q,"mediaListEntry":%s}}}`,
				id, count, format, listEntry)}
		case strings.Contains(query, "Viewer"):
			return answer{status: http.StatusOK, body: `{"data":{"Viewer":{"id":7,"name":"ivy-on-anilist"}}}`}
		}
		return answer{status: http.StatusBadRequest, body: `{"errors":[{"message":"unknown query"}]}`}
	})
	for _, id := range []int{21, 22, 23, 24, 25} {
		h.f.on("GET", "/myanimelist/anime/"+strconv.Itoa(id), func(request) answer {
			l.mu.Lock()
			defer l.mu.Unlock()
			count, movie := episodes(id)
			mediaType := "tv"
			if movie {
				mediaType = "movie"
			}
			status := "null"
			if e := l.entries[MyAnimeList][id]; e != nil {
				status = fmt.Sprintf(`{"status":%q,"num_episodes_watched":%d,"score":%g,"is_rewatching":false}`, e.status, e.progress, e.score)
			}
			return answer{status: http.StatusOK, body: fmt.Sprintf(`{"id":%d,"num_episodes":%d,"media_type":%q,"my_list_status":%s}`,
				id, count, mediaType, status)}
		})
		h.f.on("PATCH", "/myanimelist/anime/"+strconv.Itoa(id)+"/my_list_status", func(r request) answer {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.writes[MyAnimeList]++
			e := l.entries[MyAnimeList][id]
			if e == nil {
				e = &listed{}
				l.entries[MyAnimeList][id] = e
			}
			if p := r.form.Get("num_watched_episodes"); p != "" {
				e.progress, _ = strconv.Atoi(p)
			}
			if s := r.form.Get("status"); s != "" {
				e.status = s
			}
			if s := r.form.Get("score"); s != "" {
				e.score, _ = strconv.ParseFloat(s, 64)
			}
			return answer{status: http.StatusOK, body: `{}`}
		})
	}
	h.f.on("GET", "/myanimelist/users/@me/animelist", func(r request) answer {
		l.mu.Lock()
		defer l.mu.Unlock()
		var data []string
		if r.query.Get("offset") == "0" {
			for id, e := range l.entries[MyAnimeList] {
				count, movie := episodes(id)
				mediaType := "tv"
				if movie {
					mediaType = "movie"
				}
				data = append(data, fmt.Sprintf(`{"node":{"id":%d,"num_episodes":%d,"media_type":%q},`+
					`"list_status":{"status":%q,"score":%g,"num_episodes_watched":%d,"is_rewatching":false,"finish_date":%q}}`,
					id, count, mediaType, e.status, e.score, e.progress, e.completed))
			}
		}
		return answer{status: http.StatusOK, body: `{"data":[` + strings.Join(data, ",") + `],"paging":{}}`}
	})
	h.f.reply("GET", "/myanimelist/users/@me", http.StatusOK, `{"id":8,"name":"ivy-on-mal"}`)
	h.f.reply("POST", "/myanimelist-auth/token", http.StatusOK, `{"access_token":"mal-refreshed","refresh_token":"mal-refresh-2","expires_in":2678400}`)
	return l
}

// markPlayed marks titles played, as an app does, and waits until what it
// sends is sent.
func (h harness) markPlayed(t *testing.T, user accounts.ID, played bool, list ...Title) {
	t.Helper()
	h.Mark(user, Mark{Played: played, Titles: titles(list...)})
	h.idle(t)
}

// animeUser is a user connected to AniList and MyAnimeList only.
func (h harness) animeUser(t *testing.T, name string) accounts.ID {
	t.Helper()
	user := h.user(t, name)
	h.connected(t, user, AniList, "anilist-"+name)
	h.connected(t, user, MyAnimeList, "mal-"+name)
	return user
}

func TestPlayedEpisodesRaiseTheirEntrysProgress(t *testing.T) {
	h := newHarness(t)
	h.anime = animeMapping(t)
	h.listLongSeries(t)
	lists := newFakeLists(t, h)
	user := h.animeUser(t, "ivy")
	check := func(what string, service string, id int, want listed) {
		t.Helper()
		got := lists.entry(service, id)
		if got == nil || got.status != want.status || got.progress != want.progress {
			t.Errorf("%s, %s entry %d: %+v, want %+v", what, service, id, got, want)
		}
	}

	// The third episode of the series' second season on TVDB, named by
	// IMDb and TVDB, is the third of AniDB's second entry: not listed, it
	// becomes Watching at 3.
	third := Title{Item: itemID(60), IMDb: "tt0000100", TVDB: 500, Episode: true, Season: 2, Number: 3}
	h.markPlayed(t, user, true, third)
	check("third episode", AniList, 32, listed{status: statusWatching, progress: 3})
	check("third episode", MyAnimeList, 22, listed{status: statusWatching, progress: 3})
	if lists.entry(AniList, 31) != nil || lists.entry(MyAnimeList, 21) != nil {
		t.Error("the first season's entry was changed")
	}
	if got := h.status(t, user, AniList); got.LastSentAt == nil || got.Problem != "" {
		t.Errorf("status: %+v", got)
	}

	// An earlier episode never lowers the progress, and writes nothing.
	writes := lists.writesTo(AniList) + lists.writesTo(MyAnimeList)
	h.markPlayed(t, user, true, Title{Item: itemID(61), IMDb: "tt0000100", TVDB: 500, Episode: true, Season: 2, Number: 1})
	check("earlier episode", AniList, 32, listed{status: statusWatching, progress: 3})
	check("earlier episode", MyAnimeList, 22, listed{status: statusWatching, progress: 3})
	// Nor does an unplayed mark, which sends nothing.
	h.markPlayed(t, user, false, third)
	if n := lists.writesTo(AniList) + lists.writesTo(MyAnimeList); n != writes {
		t.Errorf("%d writes for an earlier episode and an unplayed mark", n-writes)
	}

	// The last episode completes the entry; a Plan to Watch entry becomes
	// Watching.
	lists.set(AniList, 33, listed{status: statusPlanned})
	lists.set(MyAnimeList, 23, listed{status: statusPlanned})
	h.markPlayed(t, user, true, Title{Item: itemID(62), IMDb: "tt0000100", TVDB: 500, Episode: true, Season: 2, Number: 12},
		// The long series' first episode of its second season on TVDB,
		// the third in absolute numbering.
		Title{Item: itemID(63), IMDb: "tt0000300", TVDB: 600, Episode: true, Season: 2, Number: 1, Absolute: 3},
		// The movie, by its IMDb identifier.
		Title{Item: itemID(64), IMDb: "tt0000400"})
	check("last episode", AniList, 32, listed{status: statusCompleted, progress: 12})
	check("last episode", MyAnimeList, 22, listed{status: statusCompleted, progress: 12})
	check("absolute", AniList, 33, listed{status: statusWatching, progress: 3})
	check("absolute", MyAnimeList, 23, listed{status: statusWatching, progress: 3})
	check("movie", AniList, 34, listed{status: statusCompleted, progress: 1})
	check("movie", MyAnimeList, 24, listed{status: statusCompleted, progress: 1})

	// An episode an anime addon names goes straight to its entry, with the
	// addon's number, though the mapping places it nowhere on TVDB.
	direct, ok := h.Anime(t.Context(), itemID(65), "kitsu:15:4", false)
	if !ok {
		t.Fatal("kitsu:15:4 not identified")
	}
	h.markPlayed(t, user, true, direct)
	check("kitsu:15:4", AniList, 35, listed{status: statusWatching, progress: 4})
	check("kitsu:15:4", MyAnimeList, 25, listed{status: statusWatching, progress: 4})

	// A title the mapping does not place sends nothing.
	writes = lists.writesTo(AniList) + lists.writesTo(MyAnimeList)
	h.markPlayed(t, user, true, Title{Item: itemID(66), IMDb: "tt0999999", TVDB: 999, Episode: true, Season: 1, Number: 1},
		Title{Item: itemID(67), IMDb: "tt0000100", TVDB: 500, Episode: true, Season: 0, Number: 1})
	if n := lists.writesTo(AniList) + lists.writesTo(MyAnimeList); n != writes {
		t.Errorf("%d writes for titles the mapping does not place", n-writes)
	}
	var queued int
	if err := h.db.QueryRow(t.Context(), "SELECT count(*) FROM tracking_events").Scan(&queued); err != nil || queued != 0 {
		t.Errorf("%d changes queued: %v", queued, err)
	}
}

func TestAPlaybackCountsOncePlayed(t *testing.T) {
	h := newHarness(t)
	h.anime = animeMapping(t)
	lists := newFakeLists(t, h)
	user := h.animeUser(t, "jane")
	first := Title{Item: itemID(70), IMDb: "tt0000100", TVDB: 500, Episode: true, Season: 1, Number: 2}
	// Started and paused, nothing; past the played mark, the progress.
	h.Playback(user, play(Started, first, 0, false, false))
	h.Playback(user, play(Progressed, first, 30, true, false))
	h.idle(t)
	if n := lists.writesTo(AniList) + lists.writesTo(MyAnimeList); n != 0 {
		t.Errorf("%d writes before the played mark", n)
	}
	h.Playback(user, play(Progressed, first, 95, false, true))
	h.Playback(user, play(Stopped, first, 96, false, true))
	h.idle(t)
	if got := lists.entry(AniList, 31); got == nil || got.progress != 2 || got.status != statusWatching {
		t.Errorf("AniList: %+v", got)
	}
	if n := lists.writesTo(MyAnimeList); n != 1 {
		t.Errorf("MyAnimeList: %d writes, want 1", n)
	}
}

func TestListsAreImportedOnlyAdding(t *testing.T) {
	h := newHarness(t)
	h.anime = animeMapping(t)
	lists := newFakeLists(t, h)
	user := h.animeUser(t, "kim")
	// AniList: the second season watched to its second episode, the movie
	// completed on a date, the long series planned.
	lists.set(AniList, 32, listed{status: statusWatching, progress: 2})
	lists.set(AniList, 34, listed{status: statusCompleted, progress: 1, completed: "2026-09-20"})
	lists.set(AniList, 33, listed{status: statusPlanned})
	// MyAnimeList: the first season's first episode, on hold.
	lists.set(MyAnimeList, 21, listed{status: statusOnHold, progress: 1})

	if result := h.turnOn(t, user, AniList); result.Played != 3 || result.Problem != "" {
		t.Errorf("AniList: %+v", result)
	}
	got := h.data(t, user, library.TitleRef{Episode: true, IMDb: "tt0000100", Season: 2, Number: 1},
		library.TitleRef{Episode: true, IMDb: "tt0000100", Season: 2, Number: 2}, library.TitleRef{Episode: true, IMDb: "tt0000100", Season: 2, Number: 3},
		library.TitleRef{IMDb: "tt0000400"})
	if !got[0].Played || !got[1].Played || got[2].Played {
		t.Errorf("second season: %+v", got)
	}
	if !got[3].Played || got[3].LastPlayed == nil || !got[3].LastPlayed.Equal(time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("movie: %+v", got[3])
	}
	if list := h.watchlistOf(t, user); len(list) != 0 {
		t.Errorf("watchlist imported while off: %v", list)
	}
	// The Plan to Watch switch puts the planned series on the watchlist.
	if result := h.turnOnWatchlist(t, user, AniList); result.Watchlist != 1 {
		t.Errorf("watchlist: %+v", result)
	}
	if list := h.watchlistOf(t, user); len(list) != 1 || list[0] != h.idOf(t, library.TitleRef{Series: true, IMDb: "tt0000300"}) {
		t.Errorf("watchlist: %v", list)
	}

	if result := h.turnOn(t, user, MyAnimeList); result.Played != 1 {
		t.Errorf("MyAnimeList: %+v", result)
	}
	if got := h.data(t, user, library.TitleRef{Episode: true, IMDb: "tt0000100", Season: 1, Number: 1}); !got[0].Played {
		t.Errorf("first season: %+v", got)
	}
	// Imports only read.
	if n := lists.writesTo(AniList) + lists.writesTo(MyAnimeList); n != 0 {
		t.Errorf("%d writes by an import", n)
	}
	var queued int
	if err := h.db.QueryRow(t.Context(), "SELECT count(*) FROM tracking_events").Scan(&queued); err != nil || queued != 0 {
		t.Errorf("%d changes queued by an import: %v", queued, err)
	}
}

func TestRatingsAreSyncedOnlyWithTheSwitch(t *testing.T) {
	h := newHarness(t)
	h.anime = animeMapping(t)
	h.listLongSeries(t)
	lists := newFakeLists(t, h)
	user := h.animeUser(t, "lee")
	lists.set(AniList, 34, listed{status: statusCompleted, progress: 1, score: 7})
	lists.set(AniList, 33, listed{status: statusWatching, progress: 3})
	rate := func(title Title, score *float64) {
		t.Helper()
		h.Rate(user, Rating{Score: score, Title: titled(title)})
		h.idle(t)
	}
	movieTitle := Title{Item: h.idOf(t, library.TitleRef{IMDb: "tt0000400"}), IMDb: "tt0000400", TMDB: 4000}
	long := Title{Item: itemID(80), IMDb: "tt0000300", TVDB: 600, Series: true}
	split := Title{Item: itemID(81), IMDb: "tt0000100", TVDB: 500, Series: true}

	// Off, a rating is not sent and the import gives no score.
	rate(movieTitle, new(9.0))
	if n := lists.writesTo(AniList); n != 0 {
		t.Errorf("%d writes with the switch off", n)
	}
	h.turnOn(t, user, AniList)
	if got := h.data(t, user, library.TitleRef{IMDb: "tt0000400"}); got[0].Rating != nil {
		t.Errorf("rating imported with the switch off: %v", *got[0].Rating)
	}

	// On, the import gives the unrated movie its score...
	result := h.imported(t, user, AniList, func() {
		if _, err := h.SetRatingsSync(t.Context(), user, AniList, true); err != nil {
			t.Fatal(err)
		}
	})
	if result.Rated != 1 {
		t.Errorf("import: %+v", result)
	}
	if got := h.data(t, user, library.TitleRef{IMDb: "tt0000400"}); got[0].Rating == nil || *got[0].Rating != 7 {
		t.Errorf("imported rating: %+v", got[0])
	}
	// ...and ratings are sent: AniList's raw score is the rating × 10; a
	// series of one entry goes to it; clearing clears.
	rate(movieTitle, new(8.5))
	if got := lists.entry(AniList, 34); got.score != 8.5 {
		t.Errorf("movie score: %+v", got)
	}
	rate(long, new(6.0))
	if got := lists.entry(AniList, 33); got.score != 6 {
		t.Errorf("series score: %+v", got)
	}
	rate(movieTitle, nil)
	if got := lists.entry(AniList, 34); got.score != 0 {
		t.Errorf("cleared score: %+v", got)
	}
	// A series spread over several entries is skipped, and counted.
	writes := lists.writesTo(AniList)
	rate(split, new(5.0))
	if n := lists.writesTo(AniList); n != writes {
		t.Errorf("%d writes for a spread series", n-writes)
	}
	if got := h.status(t, user, AniList); !got.SyncRatings || got.LastImport == nil || got.LastImport.RatingsSkipped != 1 {
		t.Errorf("status: %+v", got)
	}
	// MyAnimeList, off, got nothing.
	if n := lists.writesTo(MyAnimeList); n != 0 {
		t.Errorf("%d writes to MyAnimeList, off", n)
	}
}

func TestAnExpiredAniListTokenStopsTheConnectionOnce(t *testing.T) {
	h := newHarness(t)
	h.anime = animeMapping(t)
	lists := newFakeLists(t, h)
	user := h.animeUser(t, "max")
	if got := h.status(t, user, AniList); got.ExpiresAt == nil || !got.ExpiresAt.Equal(clockStart.Add(365*24*time.Hour)) {
		t.Errorf("expiry: %+v", got.ExpiresAt)
	}
	// A year later, a played title stops the connection instead of being
	// sent, and the stop is told once.
	later := clockStart.Add(366 * 24 * time.Hour)
	h.now = func() time.Time { return later }
	episodeOne := Title{Item: itemID(90), IMDb: "tt0000100", TVDB: 500, Episode: true, Season: 1, Number: 1}
	h.markPlayed(t, user, true, episodeOne)
	if got := h.status(t, user, AniList); got.Problem != ProblemReconnect {
		t.Errorf("status: %+v", got)
	}
	if n := lists.writesTo(AniList); n != 0 {
		t.Errorf("%d writes with an expired token", n)
	}
	h.markPlayed(t, user, true, episodeOne)
	h.refreshDue()
	if n, _ := h.stops.of(user, AniList); n != 1 {
		t.Errorf("told %d times", n)
	}
	// MyAnimeList, refreshed, goes on.
	if got := lists.entry(MyAnimeList, 21); got == nil || got.progress != 1 {
		t.Errorf("MyAnimeList: %+v", got)
	}

	// An expired token is also found by the hourly check, without anything
	// played, and told once.
	other := h.animeUser(t, "ned")
	h.refreshDue()
	h.refreshDue()
	if n, _ := h.stops.of(other, AniList); n != 1 {
		t.Errorf("told %d times by the check", n)
	}
	// AniList refusing the token stops it too.
	h.now = func() time.Time { return clockStart }
	third := h.animeUser(t, "oz")
	h.f.on("POST", "/anilist/", func(request) answer {
		return answer{status: http.StatusBadRequest, body: `{"errors":[{"message":"Invalid token","status":400}],"data":null}`}
	})
	h.markPlayed(t, third, true, episodeOne)
	if n, _ := h.stops.of(third, AniList); n != 1 || h.status(t, third, AniList).Problem != ProblemReconnect {
		t.Errorf("refused token: told %d times", n)
	}
}

func TestAniListConnectsWithThePastedCode(t *testing.T) {
	h := newHarness(t)
	box, err := secrets.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	h.box = box
	newFakeLists(t, h)
	user := h.user(t, "pat")
	h.f.on("POST", "/anilist-auth/token", func(r request) answer {
		body, _ := r.body.(map[string]any)
		if body["code"] != "the-code" {
			return answer{status: http.StatusBadRequest, body: `{"error":"invalid_request"}`}
		}
		return answer{status: http.StatusOK, body: `{"token_type":"Bearer","expires_in":31536000,"access_token":"anilist-token","refresh_token":"unused"}`}
	})
	status, err := h.StartCode(t.Context(), user, AniList)
	if err != nil {
		t.Fatal(err)
	}
	page, err := url.Parse(status.Code.VerificationURL)
	if err != nil || page.Path != "/anilist-auth/authorize" || page.Query().Get("client_id") != "anilist-client" ||
		page.Query().Get("redirect_uri") != anilistPIN || page.Query().Get("response_type") != "code" {
		t.Errorf("page: %v %v", status.Code, err)
	}
	if _, err := h.Exchange(t.Context(), user, AniList, "another-code"); err != ErrInvalidCode {
		t.Errorf("wrong code: %v", err)
	}
	status, err = h.Exchange(t.Context(), user, AniList, " the-code\n")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Connected || status.Account != "ivy-on-anilist" || status.Code != nil || status.ExpiresAt == nil ||
		!status.ExpiresAt.Equal(clockStart.Add(365*24*time.Hour)) {
		t.Errorf("status: %+v", status)
	}
	sent := h.f.sent("anilist-auth", "/token")
	sameJSON(t, "token request", sent[len(sent)-1].body, `{"grant_type":"authorization_code","client_id":"anilist-client",
		"client_secret":"anilist-secret","redirect_uri":"https://anilist.co/api/v2/oauth/pin","code":"the-code"}`)
	// The token is stored encrypted, without the refresh token AniList
	// cannot use.
	var token, refresh string
	if err := h.db.QueryRow(t.Context(), "SELECT token, refresh_token FROM tracking_connections WHERE user_id = $1 AND service = 'anilist'",
		user).Scan(&token, &refresh); err != nil || token == "anilist-token" || strings.Contains(token, "anilist-token") || refresh != "" {
		t.Errorf("stored: %q %q %v", token, refresh, err)
	}
	// A wrong secret is the app's problem.
	h.f.reply("POST", "/anilist-auth/token", http.StatusUnauthorized, `{"error":"invalid_client"}`)
	if _, err := h.Exchange(t.Context(), user, AniList, "the-code"); err != ErrAppRefused {
		t.Errorf("refused app: %v", err)
	}
}

func TestMyAnimeListConnectsWithPKCEAndRefreshes(t *testing.T) {
	h := newHarness(t)
	lists := newFakeLists(t, h)
	user := h.user(t, "quinn")
	h.f.on("POST", "/myanimelist-auth/token", func(r request) answer {
		switch r.form.Get("grant_type") {
		case "refresh_token":
			if r.form.Get("refresh_token") != "mal-refresh" || r.form.Get("client_secret") != "mal-secret" {
				return answer{status: http.StatusBadRequest, body: `{"error":"invalid_grant"}`}
			}
			return answer{status: http.StatusOK, body: `{"access_token":"mal-token-2","refresh_token":"mal-refresh-2","expires_in":2678400}`}
		}
		if r.form.Get("code") != "mal-code" || r.form.Get("client_secret") != "mal-secret" || r.form.Get("redirect_uri") != "" {
			return answer{status: http.StatusBadRequest, body: `{"error":"invalid_grant"}`}
		}
		return answer{status: http.StatusOK, body: `{"token_type":"Bearer","access_token":"mal-token","refresh_token":"mal-refresh","expires_in":3600}`}
	})
	// Without Connect first, a code connects nothing.
	if _, err := h.Exchange(t.Context(), user, MyAnimeList, "mal-code"); err != ErrInvalidCode {
		t.Errorf("no connection waiting: %v", err)
	}
	status, err := h.StartCode(t.Context(), user, MyAnimeList)
	if err != nil {
		t.Fatal(err)
	}
	page, err := url.Parse(status.Code.VerificationURL)
	query := page.Query()
	if err != nil || page.Path != "/myanimelist-auth/authorize" || query.Get("client_id") != "mal-client" || query.Get("code_challenge_method") != "plain" ||
		len(query.Get("code_challenge")) < 43 || query.Get("state") == "" || query.Get("redirect_uri") != "" {
		t.Errorf("page: %v %v", status.Code, err)
	}
	// An address for another attempt is refused.
	if _, err := h.Exchange(t.Context(), user, MyAnimeList, "https://polyfin.example/admin/?code=mal-code&state=other"); err != ErrInvalidCode {
		t.Errorf("wrong state: %v", err)
	}
	// The address the browser landed on connects, with the verifier.
	status, err = h.Exchange(t.Context(), user, MyAnimeList, "https://polyfin.example/admin/tracking/myanimelist?code=mal-code&state="+query.Get("state"))
	if err != nil {
		t.Fatal(err)
	}
	if !status.Connected || status.Account != "ivy-on-mal" || status.ExpiresAt != nil {
		t.Errorf("status: %+v", status)
	}
	sent := h.f.sent("myanimelist-auth", "/token")
	if verifier := sent[len(sent)-1].form.Get("code_verifier"); verifier != query.Get("code_challenge") {
		t.Errorf("verifier %q, challenge %q", verifier, query.Get("code_challenge"))
	}
	// The token, expiring within a day, is refreshed before a send.
	h.anime = animeMapping(t)
	h.markPlayed(t, user, true, Title{Item: itemID(95), IMDb: "tt0000100", TVDB: 500, Episode: true, Season: 1, Number: 1})
	if got := lists.entry(MyAnimeList, 21); got == nil || got.progress != 1 {
		t.Errorf("entry: %+v", got)
	}
	patches := h.f.sent("myanimelist", "/anime/21/my_list_status")
	if len(patches) != 1 || patches[0].header.Get("Authorization") != "Bearer mal-token-2" {
		t.Errorf("patches: %+v", patches)
	}
	// Disconnecting forgets the tokens: MyAnimeList has no way to revoke
	// them.
	if err := h.Disconnect(t.Context(), user, MyAnimeList); err != nil {
		t.Fatal(err)
	}
	if h.status(t, user, MyAnimeList).Connected {
		t.Error("still connected")
	}
}

func TestPastedCodes(t *testing.T) {
	for pasted, want := range map[string][2]string{
		"abc123":                                {"abc123", ""},
		"  abc123 \n":                           {"abc123", ""},
		"https://x.example/cb?code=c1&state=s1": {"c1", "s1"},
		"https://x.example/cb?state=s1&code=c1#top": {"c1", "s1"},
		"code=c2&state=s2":                          {"c2", "s2"},
		"two words":                                 {"", ""},
	} {
		if code, state := pastedCode(pasted); code != want[0] || state != want[1] {
			t.Errorf("%q: %q %q, want %q", pasted, code, state, want)
		}
	}
}

func TestNextProgress(t *testing.T) {
	for _, c := range []struct {
		status          string
		progress, total int
		episode         int
		movie           bool
		next            int
		nextStatus      string
		changed         bool
	}{
		{"", 0, 12, 3, false, 3, statusWatching, true},
		{statusPlanned, 0, 0, 1, false, 1, statusWatching, true},
		{statusWatching, 5, 12, 3, false, 5, statusWatching, false},
		{statusWatching, 5, 12, 12, false, 12, statusCompleted, true},
		{statusDropped, 2, 12, 4, false, 4, statusDropped, true},
		{statusOnHold, 2, 12, 20, false, 12, statusCompleted, true},
		{"", 0, 1, 0, true, 1, statusCompleted, true},
		{statusRepeating, 3, 12, 12, false, 12, statusRepeating, true},
		{statusCompleted, 12, 12, 4, false, 12, statusCompleted, false},
	} {
		next, nextStatus, changed := nextProgress(c.status, c.progress, c.total, c.episode, c.movie)
		if next != c.next || nextStatus != c.nextStatus || changed != c.changed {
			t.Errorf("%+v: %d %s %v", c, next, nextStatus, changed)
		}
	}
}
