package trackers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
)

// imported runs start, which starts an import of user's history from
// service, and waits until it ended.
func (h harness) imported(t *testing.T, user accounts.ID, service string, start func()) ImportResult {
	t.Helper()
	start()
	key := laneKey{user, service}
	eventually(t, "the import ending", func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.importing[key] == nil
	})
	status := h.status(t, user, service)
	if status.LastImport == nil {
		t.Fatalf("no import recorded: %+v", status)
	}
	return *status.LastImport
}

// turnOn turns the import of user's history from service on, which
// imports it.
func (h harness) turnOn(t *testing.T, user accounts.ID, service string) ImportResult {
	t.Helper()
	return h.imported(t, user, service, func() {
		if _, err := h.SetImport(t.Context(), user, service, true); err != nil {
			t.Fatal(err)
		}
	})
}

// idOf is the item ref designates, as Polyfin names it.
func (h harness) idOf(t *testing.T, ref library.TitleRef) accounts.ID {
	t.Helper()
	found, err := h.titles.Resolve(t.Context(), []library.TitleRef{ref})
	if err != nil || len(found[0]) != 1 {
		t.Fatalf("%+v: %v %v", ref, found, err)
	}
	return found[0][0].ID
}

func (h harness) data(t *testing.T, user accounts.ID, refs ...library.TitleRef) []userdata.Data {
	t.Helper()
	ids := make([]accounts.ID, len(refs))
	for i, ref := range refs {
		ids[i] = h.idOf(t, ref)
	}
	got, err := h.userData.Get(t.Context(), user, ids)
	if err != nil {
		t.Fatal(err)
	}
	result := make([]userdata.Data, len(ids))
	for i, id := range ids {
		result[i] = got[id]
	}
	return result
}

func at(text string) *time.Time {
	t, err := time.Parse(time.RFC3339, text)
	if err != nil {
		panic(err)
	}
	return &t
}

// noWrites fails when any service received anything but a read.
func (h harness) noWrites(t *testing.T) {
	t.Helper()
	h.f.mu.Lock()
	defer h.f.mu.Unlock()
	for _, r := range h.f.requests {
		if r.method != http.MethodGet {
			t.Errorf("an import sent %s %s %s", r.service, r.method, r.path)
		}
	}
	var queued int
	if err := h.db.QueryRow(t.Context(), "SELECT count(*) FROM tracking_events").Scan(&queued); err != nil || queued != 0 {
		t.Errorf("%d changes queued: %v", queued, err)
	}
}

func TestImportIsOffByDefault(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "alice")
	h.connected(t, user, MDBList, "mdblist-alice")
	if status := h.status(t, user, MDBList); status.ImportHistory || status.Importing || status.LastImport != nil {
		t.Errorf("connected: %+v", status)
	}
	h.importDue()
	if _, err := h.ImportNow(t.Context(), user, MDBList); !errors.Is(err, ErrImportOff) {
		t.Errorf("import now: %v", err)
	}
	if _, err := h.SetImport(t.Context(), user, Trakt, true); !errors.Is(err, ErrNotConnected) {
		t.Errorf("not connected: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if n := len(h.f.sent("mdblist", "/sync/watched")) + len(h.f.sent("mdblist", "/sync/last_activities")); n != 0 {
		t.Errorf("%d requests", n)
	}
}

func TestTraktHistoryIsReadPageByPage(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "bob")
	h.connected(t, user, Trakt, "trakt-bob")
	h.f.reply("GET", "/trakt/sync/last_activities", http.StatusOK,
		`{"movies":{"watched_at":"2026-10-01T10:00:00.000Z"},"episodes":{"watched_at":"2026-10-02T10:00:00.000Z"}}`)
	movies := [][]string{
		{`{"watched_at":"2026-09-01T20:00:00.000Z","type":"movie","movie":{"ids":{"trakt":1,"imdb":"tt0000001","tmdb":10}}}`,
			// An older play of the same movie.
			`{"watched_at":"2025-01-01T20:00:00.000Z","type":"movie","movie":{"ids":{"trakt":1,"imdb":"tt0000001","tmdb":10}}}`},
		// Known only by TMDB, which Polyfin never listed.
		{`{"watched_at":"2026-09-02T20:00:00.000Z","type":"movie","movie":{"ids":{"trakt":2,"imdb":null,"tmdb":99}}}`},
	}
	h.f.on("GET", "/trakt/sync/history/movies", func(r request) answer {
		page, _ := strconv.Atoi(r.query.Get("page"))
		return answer{status: http.StatusOK, body: "[" + strings.Join(movies[page-1], ",") + "]",
			header: http.Header{"X-Pagination-Page-Count": {"2"}}}
	})
	h.f.reply("GET", "/trakt/sync/history/episodes", http.StatusOK,
		`[{"watched_at":"2026-09-03T21:00:00.000Z","type":"episode","episode":{"season":1,"number":2,"ids":{"trakt":9}},
			"show":{"ids":{"trakt":5,"imdb":"tt0100","tmdb":30,"tvdb":20}}}]`)
	h.f.reply("GET", "/trakt/sync/playback", http.StatusOK,
		`[{"progress":25,"paused_at":"2026-10-03T08:00:00.000Z","type":"movie","movie":{"runtime":120,"ids":{"imdb":"tt0000002"}}},
			{"progress":50,"paused_at":"2026-10-03T09:00:00.000Z","type":"episode","episode":{"season":1,"number":3,"runtime":40},
			"show":{"ids":{"imdb":"tt0100"}}}]`)

	result := h.turnOn(t, user, Trakt)
	if result.Played != 2 || result.Resumed != 2 || result.Unmapped != 1 || result.Problem != "" || !result.At.Equal(clockStart) {
		t.Errorf("result: %+v", result)
	}
	pages := h.f.sent("trakt", "/sync/history/movies")
	if len(pages) != 2 || pages[0].query.Get("page") != "1" || pages[1].query.Get("page") != "2" || pages[1].query.Get("limit") != "250" {
		t.Errorf("pages: %+v", pages)
	}
	if playback := h.f.sent("trakt", "/sync/playback"); len(playback) != 1 || playback[0].query.Get("extended") != "full" ||
		playback[0].header.Get("Authorization") != "Bearer trakt-bob" {
		t.Errorf("playback: %+v", playback)
	}
	movie, episode := library.TitleRef{IMDb: "tt0000001"}, library.TitleRef{Episode: true, IMDb: "tt0100", Season: 1, Number: 2}
	resumedMovie, resumedEpisode := library.TitleRef{IMDb: "tt0000002"}, library.TitleRef{Episode: true, IMDb: "tt0100", Season: 1, Number: 3}
	got := h.data(t, user, movie, episode, resumedMovie, resumedEpisode)
	if !got[0].Played || got[0].PlayCount != 1 || !got[0].LastPlayed.Equal(*at("2026-09-01T20:00:00Z")) {
		t.Errorf("movie: %+v", got[0])
	}
	if !got[1].Played || !got[1].LastPlayed.Equal(*at("2026-09-03T21:00:00Z")) {
		t.Errorf("episode: %+v", got[1])
	}
	if got[2].Played || got[2].Position != 30*time.Minute || got[2].Runtime != 2*time.Hour || !got[2].LastPlayed.Equal(*at("2026-10-03T08:00:00Z")) {
		t.Errorf("resumed movie: %+v", got[2])
	}
	if got[3].Position != 20*time.Minute {
		t.Errorf("resumed episode: %+v", got[3])
	}
	// The episode counts in its series and season, as Next Up reads them.
	entries, err := h.userData.Episodes(t.Context(), user)
	if err != nil || len(entries) != 2 || entries[0].Series == (accounts.ID{}) || entries[0].Season == (accounts.ID{}) {
		t.Errorf("episodes: %+v %v", entries, err)
	}

	// Nothing watched since: the next import reads only the resume points.
	h.imported(t, user, Trakt, func() {
		if _, err := h.ImportNow(t.Context(), user, Trakt); err != nil {
			t.Fatal(err)
		}
	})
	if n := len(h.f.sent("trakt", "/sync/history/movies")); n != 2 || len(h.f.sent("trakt", "/sync/playback")) != 2 {
		t.Errorf("read again: %d pages", n)
	}
	// Importing sends nothing back.
	h.noWrites(t)
}

func TestImportsMergeWithoutLosingAnything(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "carol")
	h.connected(t, user, MDBList, "mdblist-carol")
	refs := map[string]library.TitleRef{}
	for _, name := range []string{"keepCount", "laterHere", "unwatchedThere", "newerHere", "newerThere", "pastPlayed", "beforeResume", "playedHere"} {
		refs[name] = library.TitleRef{IMDb: fmt.Sprintf("tt%07d", len(refs)+1)}
	}
	seed := func(name string, change func(*userdata.Data)) {
		t.Helper()
		item := userdata.Item{ID: h.idOf(t, refs[name])}
		if _, err := h.userData.Change(t.Context(), user, []userdata.Item{item}, change); err != nil {
			t.Fatal(err)
		}
	}
	// Played three times here, last after the service's date.
	seed("keepCount", func(d *userdata.Data) { d.Played, d.PlayCount, d.LastPlayed = true, 3, at("2026-09-01T00:00:00Z") })
	seed("laterHere", func(d *userdata.Data) { d.Played, d.PlayCount, d.LastPlayed = true, 1, at("2026-10-01T00:00:00Z") })
	// Played here, and not in the service's history: it stays played.
	seed("unwatchedThere", func(d *userdata.Data) { d.Played, d.PlayCount, d.LastPlayed = true, 1, at("2026-01-01T00:00:00Z") })
	seed("newerHere", func(d *userdata.Data) {
		d.Position, d.Runtime, d.LastPlayed = 10*time.Minute, 100*time.Minute, at("2026-10-04T00:00:00Z")
	})
	seed("newerThere", func(d *userdata.Data) {
		d.Position, d.Runtime, d.LastPlayed = 10*time.Minute, 100*time.Minute, at("2026-10-01T00:00:00Z")
	})
	seed("playedHere", func(d *userdata.Data) { d.Played, d.PlayCount, d.LastPlayed = true, 1, at("2026-01-01T00:00:00Z") })

	movie := func(name, date string) string {
		return `{"last_watched_at":"` + date + `","movie":{"ids":{"imdb":"` + refs[name].IMDb + `"}}}`
	}
	h.f.reply("GET", "/mdblist/sync/last_activities", http.StatusOK, `{"watched_at":"2026-10-02T00:00:00Z","episode_watched_at":"2026-10-02T00:00:00Z"}`)
	h.f.reply("GET", "/mdblist/sync/watched", http.StatusOK, `{"movies":[`+movie("keepCount", "2026-08-01T00:00:00Z")+","+
		movie("laterHere", "2026-09-15T00:00:00Z")+`],"episodes":[],"pagination":{"has_more":false}}`)
	point := func(name string, progress float64, date string) string {
		return `{"progress":` + strconv.FormatFloat(progress, 'f', -1, 64) + `,"paused_at":"` + date + `","type":"movie","movie":{"ids":{"imdbid":"` +
			refs[name].IMDb + `"}}}`
	}
	h.f.reply("GET", "/mdblist/sync/playback", http.StatusOK, "["+strings.Join([]string{
		point("newerHere", 50, "2026-10-02T00:00:00Z"),
		point("newerThere", 50, "2026-10-02T00:00:00Z"),
		// Past the played threshold (90%), before the resume one (5%).
		point("pastPlayed", 95, "2026-10-02T00:00:00Z"),
		point("beforeResume", 2, "2026-10-02T00:00:00Z"),
		point("playedHere", 50, "2026-10-02T00:00:00Z"),
	}, ",")+"]")

	result := h.turnOn(t, user, MDBList)
	if result.Played != 1 || result.Resumed != 1 || result.Unmapped != 0 {
		t.Errorf("result: %+v", result)
	}
	got := h.data(t, user, refs["keepCount"], refs["laterHere"], refs["unwatchedThere"], refs["newerHere"], refs["newerThere"],
		refs["pastPlayed"], refs["beforeResume"], refs["playedHere"])
	if d := got[0]; !d.Played || d.PlayCount != 3 || !d.LastPlayed.Equal(*at("2026-09-01T00:00:00Z")) {
		t.Errorf("played here three times: %+v", d)
	}
	if d := got[1]; !d.Played || !d.LastPlayed.Equal(*at("2026-10-01T00:00:00Z")) {
		t.Errorf("played here later: %+v", d)
	}
	if d := got[2]; !d.Played {
		t.Errorf("unplayed by an import: %+v", d)
	}
	if d := got[3]; d.Position != 10*time.Minute || !d.LastPlayed.Equal(*at("2026-10-04T00:00:00Z")) {
		t.Errorf("newer resume point here: %+v", d)
	}
	if d := got[4]; d.Position != 50*time.Minute || !d.LastPlayed.Equal(*at("2026-10-02T00:00:00Z")) {
		t.Errorf("newer resume point there: %+v", d)
	}
	if d := got[5]; !d.Played || d.PlayCount != 1 || d.Position != 0 {
		t.Errorf("past the played threshold: %+v", d)
	}
	if d := got[6]; d.Played || d.Position != 0 || d.LastPlayed != nil {
		t.Errorf("before the resume threshold: %+v", d)
	}
	if d := got[7]; !d.Played || d.Position != 0 {
		t.Errorf("played here, paused there: %+v", d)
	}
	h.noWrites(t)
}

func TestImportsOfSeveralServicesAddUp(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "dave")
	h.connected(t, user, MDBList, "mdblist-dave")
	h.connected(t, user, PublicMetaDB, "pm-dave")
	pilot := library.TitleRef{Episode: true, IMDb: "tt0100", Season: 1, Number: 1}
	second := library.TitleRef{Episode: true, IMDb: "tt0100", Season: 1, Number: 2}
	third := library.TitleRef{Episode: true, IMDb: "tt0100", Season: 1, Number: 3}
	h.f.reply("GET", "/mdblist/sync/last_activities", http.StatusOK, `{"watched_at":"2026-10-02T00:00:00Z"}`)
	h.f.reply("GET", "/mdblist/sync/watched", http.StatusOK, `{"movies":[],"episodes":[{"last_watched_at":"2026-09-01T00:00:00Z",
		"episode":{"season":1,"number":1,"show":{"ids":{"imdb":"tt0100"}}}}],"pagination":{"has_more":false}}`)
	h.f.reply("GET", "/mdblist/sync/playback", http.StatusOK, `[{"progress":40,"paused_at":"2026-10-01T00:00:00Z","type":"episode",
		"episode":{"season":1,"number":3,"runtime":50},"show":{"ids":{"imdbid":"tt0100"}}}]`)
	// PublicMetaDB names titles by TMDB only, and maps them to IMDb; it
	// maps one movie two ways, which is not reliable.
	h.f.on("GET", "/publicmetadb/api/external/watched", func(r request) answer {
		if r.query.Get("page") == "1" {
			return answer{status: http.StatusOK, body: `{"items":[{"tmdb_id":30,"media_type":"tv","season":1,"episode":2,"watched_at":"2026-09-02T00:00:00Z"}],
				"total":2,"page":1,"perPage":500,"totalPages":2}`}
		}
		return answer{status: http.StatusOK, body: `{"items":[{"tmdb_id":77,"media_type":"movie","watched_at":null}],"total":2,"page":2,"perPage":500,"totalPages":2}`}
	})
	h.f.reply("GET", "/publicmetadb/api/external/resume", http.StatusOK, `{"items":[{"tmdb_id":30,"media_type":"tv","season":1,"episode":3,
		"position_ms":1800000,"runtime_ms":3000000,"progress":60,"updated":"2026-10-03T00:00:00Z"}],"totalPages":1}`)
	h.f.on("GET", "/publicmetadb/api/external/mappings", func(r request) answer {
		if r.query.Get("tmdb_id") == "30" && r.query.Get("media_type") == "tv" && r.query.Get("id_type") == "imdb" {
			return answer{status: http.StatusOK, body: `{"tmdb_id":30,"media_type":"tv","mappings":{"imdb":[{"value":"tt0100"}]},"total":1}`}
		}
		return answer{status: http.StatusOK, body: `{"mappings":{"imdb":[{"value":"tt0000077"},{"value":"tt0000078"}]}}`}
	})

	if result := h.turnOn(t, user, MDBList); result.Played != 1 || result.Resumed != 1 {
		t.Errorf("MDBList: %+v", result)
	}
	if result := h.turnOn(t, user, PublicMetaDB); result.Played != 1 || result.Resumed != 1 || result.Unmapped != 1 {
		t.Errorf("PublicMetaDB: %+v", result)
	}
	got := h.data(t, user, pilot, second, third)
	if !got[0].Played || !got[1].Played {
		t.Errorf("the union of the histories: %+v %+v", got[0], got[1])
	}
	// The latest resume point wins: PublicMetaDB's.
	if got[2].Position != 30*time.Minute || got[2].Runtime != 50*time.Minute {
		t.Errorf("resume point: %+v", got[2])
	}
	if pages := h.f.sent("publicmetadb", "/api/external/watched"); len(pages) != 2 || pages[1].query.Get("perPage") != "500" ||
		pages[1].header.Get("Authorization") != "Bearer pm-dave" {
		t.Errorf("pages: %+v", pages)
	}
	// The mapping is asked once a title.
	h.imported(t, user, PublicMetaDB, func() {
		if _, err := h.ImportNow(t.Context(), user, PublicMetaDB); err != nil {
			t.Fatal(err)
		}
	})
	if n := len(h.f.sent("publicmetadb", "/api/external/mappings")); n != 2 {
		t.Errorf("%d lookups", n)
	}
	h.noWrites(t)
}

func TestImportsRunEverySixHoursAndOnDemand(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "erin")
	h.connected(t, user, MDBList, "mdblist-erin")
	var activities atomic.Int32
	h.f.on("GET", "/mdblist/sync/last_activities", func(request) answer {
		activities.Add(1)
		return answer{status: http.StatusOK, body: `{}`}
	})
	h.f.reply("GET", "/mdblist/sync/watched", http.StatusOK, `{"pagination":{"has_more":false}}`)
	h.f.reply("GET", "/mdblist/sync/playback", http.StatusOK, `[]`)
	h.turnOn(t, user, MDBList)

	due := func(after time.Duration) {
		h.now = func() time.Time { return clockStart.Add(after) }
		h.importDue()
		eventually(t, "the imports due ending", func() bool {
			h.mu.Lock()
			defer h.mu.Unlock()
			return len(h.importing) == 0
		})
	}
	due(5 * time.Hour)
	if n := activities.Load(); n != 1 {
		t.Errorf("imported again within 6 hours: %d", n)
	}
	due(6 * time.Hour)
	if n := activities.Load(); n != 2 {
		t.Errorf("not imported after 6 hours: %d", n)
	}
	// Import now runs at once, and the 6 hours start again from it.
	h.imported(t, user, MDBList, func() {
		if _, err := h.ImportNow(t.Context(), user, MDBList); err != nil {
			t.Fatal(err)
		}
	})
	due(11 * time.Hour)
	if n := activities.Load(); n != 3 {
		t.Errorf("after import now: %d", n)
	}
	// Turned off, nothing is due.
	if status, err := h.SetImport(t.Context(), user, MDBList, false); err != nil || status.ImportHistory {
		t.Fatalf("off: %+v %v", status, err)
	}
	due(48 * time.Hour)
	if n := activities.Load(); n != 3 {
		t.Errorf("imported while off: %d", n)
	}
	// Disconnecting forgets the import: the next connection starts off.
	if err := h.Disconnect(t.Context(), user, MDBList); err != nil {
		t.Fatal(err)
	}
	h.connected(t, user, MDBList, "mdblist-erin")
	if status := h.status(t, user, MDBList); status.ImportHistory || status.LastImport != nil {
		t.Errorf("after connecting again: %+v", status)
	}
}

func TestImportsWaitAsAskedAndAtTheirPace(t *testing.T) {
	h := newHarness(t)
	h.timing.importGaps = map[string]time.Duration{MDBList: 100 * time.Millisecond}
	user := h.user(t, "frank")
	h.connected(t, user, MDBList, "mdblist-frank")
	h.f.reply("GET", "/mdblist/sync/last_activities", http.StatusOK, `{"watched_at":"2026-10-02T00:00:00Z"}`)
	var calls atomic.Int32
	h.f.on("GET", "/mdblist/sync/watched", func(r request) answer {
		if calls.Add(1) == 1 {
			return answer{status: http.StatusTooManyRequests, body: `{}`, header: http.Header{"Retry-After": {"1"}}}
		}
		more := r.query.Get("offset") == "0"
		return answer{status: http.StatusOK, body: `{"movies":[{"last_watched_at":"2026-09-01T00:00:00Z","movie":{"ids":{"imdb":"tt000000` +
			strconv.Itoa(int(calls.Load())) + `"}}}],"pagination":{"has_more":` + strconv.FormatBool(more) + `}}`}
	})
	h.f.reply("GET", "/mdblist/sync/playback", http.StatusOK, `[]`)
	result := h.turnOn(t, user, MDBList)
	if result.Played != 2 || result.Problem != "" {
		t.Errorf("result: %+v", result)
	}
	pages := h.f.sent("mdblist", "/sync/watched")
	if len(pages) != 3 || pages[1].at.Sub(pages[0].at) < time.Second || pages[2].query.Get("offset") != "1000" {
		t.Errorf("pages: %d", len(pages))
	}
	h.f.mu.Lock()
	for i := 1; i < len(h.f.requests); i++ {
		if gap := h.f.requests[i].at.Sub(h.f.requests[i-1].at); gap < 90*time.Millisecond {
			t.Errorf("requests %d and %d only %v apart", i-1, i, gap)
		}
	}
	h.f.mu.Unlock()

	// Asked to wait longer than an import waits, it gives up, keeping what
	// it read.
	h.f.reply("GET", "/mdblist/sync/last_activities", http.StatusOK, `{"watched_at":"2026-10-03T00:00:00Z"}`)
	h.f.on("GET", "/mdblist/sync/watched", func(request) answer {
		return answer{status: http.StatusTooManyRequests, body: `{"error":"Daily API limit exceeded!"}`, header: http.Header{"Retry-After": {"43200"}}}
	})
	result = h.imported(t, user, MDBList, func() {
		if _, err := h.ImportNow(t.Context(), user, MDBList); err != nil {
			t.Fatal(err)
		}
	})
	if result.Problem != ProblemRateLimited {
		t.Errorf("result: %+v", result)
	}
	if status := h.status(t, user, MDBList); status.Problem != "" {
		t.Errorf("the connection: %+v", status)
	}
}

func TestImportsKeepToTheirUser(t *testing.T) {
	h := newHarness(t)
	alice, bob := h.user(t, "alice"), h.user(t, "bob")
	h.connected(t, alice, MDBList, "mdblist-alice")
	h.connected(t, bob, MDBList, "mdblist-bob")
	h.f.reply("GET", "/mdblist/sync/last_activities", http.StatusOK, `{}`)
	h.f.on("GET", "/mdblist/sync/watched", func(r request) answer {
		imdb := map[string]string{"mdblist-alice": "tt0000001", "mdblist-bob": "tt0000002"}[r.query.Get("apikey")]
		return answer{status: http.StatusOK, body: `{"movies":[{"last_watched_at":"2026-09-01T00:00:00Z","movie":{"ids":{"imdb":"` + imdb + `"}}}]}`}
	})
	h.f.reply("GET", "/mdblist/sync/playback", http.StatusOK, `[]`)
	h.turnOn(t, alice, MDBList)
	first, second := library.TitleRef{IMDb: "tt0000001"}, library.TitleRef{IMDb: "tt0000002"}
	if got := h.data(t, alice, first, second); !got[0].Played || got[1].Played {
		t.Errorf("alice: %+v", got)
	}
	if got := h.data(t, bob, first, second); got[0].Played || got[1].Played {
		t.Errorf("bob, who imports nothing: %+v", got)
	}
	if status := h.status(t, bob, MDBList); status.ImportHistory || status.LastImport != nil {
		t.Errorf("bob: %+v", status)
	}
}

func TestSimklHistoryIsReadSinceItsLastActivity(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "gina")
	h.connected(t, user, Simkl, "simkl-gina")
	activity := "2026-10-01T00:00:00Z"
	h.f.on("GET", "/simkl/sync/activities", func(request) answer {
		return answer{status: http.StatusOK, body: `{"all":"` + activity + `"}`}
	})
	h.f.reply("GET", "/simkl/sync/all-items/movies/completed", http.StatusOK,
		`{"movies":[{"last_watched_at":"2026-09-01T00:00:00Z","movie":{"ids":{"simkl":1,"imdb":"tt0000001","tmdb":"10"}}}]}`)
	h.f.reply("GET", "/simkl/sync/all-items/shows/watching", http.StatusOK, `{"shows":[{"show":{"ids":{"imdb":"tt0100","tvdb":"20"}},
		"seasons":[{"number":1,"episodes":[{"number":1,"watched_at":"2026-09-02T00:00:00Z"}]}]}]}`)
	h.f.reply("GET", "/simkl/sync/all-items/shows/completed", http.StatusOK, `{}`)
	h.f.reply("GET", "/simkl/sync/all-items/shows/hold", http.StatusOK, `null`)
	h.f.reply("GET", "/simkl/sync/playback", http.StatusOK, `[]`)
	if result := h.turnOn(t, user, Simkl); result.Played != 2 || result.Problem != "" {
		t.Errorf("result: %+v", result)
	}
	watching := h.f.sent("simkl", "/sync/all-items/shows/watching")
	if len(watching) != 1 || watching[0].query.Get("date_from") != "" || watching[0].query.Get("extended") != "full" ||
		watching[0].query.Get("episode_watched_at") != "yes" || watching[0].query.Get("client_id") != "simkl-client" {
		t.Errorf("first read: %+v", watching)
	}
	again := func() {
		h.imported(t, user, Simkl, func() {
			if _, err := h.ImportNow(t.Context(), user, Simkl); err != nil {
				t.Fatal(err)
			}
		})
	}
	// Nothing changed: no list is read.
	again()
	if n := len(h.f.sent("simkl", "/sync/all-items/shows/watching")); n != 1 {
		t.Errorf("read again unchanged: %d", n)
	}
	// Changed: read since the last activity seen.
	activity = "2026-10-04T00:00:00Z"
	again()
	if watching := h.f.sent("simkl", "/sync/all-items/shows/watching"); len(watching) != 2 || watching[1].query.Get("date_from") != "2026-10-01T00:00:00Z" {
		t.Errorf("read since: %+v", watching)
	}
	h.noWrites(t)
}

func TestAnImportWithARefusedTokenAsksToReconnect(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "hank")
	h.connected(t, user, MDBList, "mdblist-hank")
	h.f.reply("GET", "/mdblist/sync/last_activities", http.StatusUnauthorized, `{"error":"Invalid API key"}`)
	if result := h.turnOn(t, user, MDBList); result.Problem != ProblemReconnect {
		t.Errorf("result: %+v", result)
	}
	if status := h.status(t, user, MDBList); status.Problem != ProblemReconnect {
		t.Errorf("connection: %+v", status)
	}
	// No import is due while the connection waits.
	h.now = func() time.Time { return clockStart.Add(7 * time.Hour) }
	h.importDue()
	if n := len(h.f.sent("mdblist", "/sync/last_activities")); n != 1 {
		t.Errorf("%d reads", n)
	}
}
