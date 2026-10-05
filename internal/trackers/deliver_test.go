package trackers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// connectAll connects user to every service, with tokens and keys named
// after the service.
func (h harness) connectAll(t *testing.T, user string) accounts.ID {
	t.Helper()
	userID := h.user(t, user)
	for _, service := range Services {
		h.connected(t, userID, service, service+"-"+user)
	}
	return userID
}

func itemID(n byte) accounts.ID { return accounts.ID{n} }

func jsonString(v any) (string, error) {
	encoded, err := json.Marshal(v)
	return string(encoded), err
}

func mustJSON(v any) string {
	encoded, _ := jsonString(v)
	return encoded
}

func TestPlaybackIsScrobbledWithItsProgress(t *testing.T) {
	h := newHarness(t)
	user := h.connectAll(t, "alice")
	// A movie: started, paused at 30%, played on, past the played mark
	// at 95%, stopped at 96%.
	h.Playback(user, play(Started, movie, 0, false, false))
	h.Playback(user, play(Progressed, movie, 30, true, false))
	h.Playback(user, play(Progressed, movie, 30, false, false))
	h.Playback(user, play(Progressed, movie, 95, false, true))
	h.Playback(user, play(Stopped, movie, 96, false, true))
	// An episode: started, stopped halfway.
	h.Playback(user, play(Started, episode, 0, false, false))
	h.Playback(user, play(Stopped, episode, 50, false, false))

	movieIDs := `{"imdb":"tt0000001","tmdb":10}`
	showIDs := `{"imdb":"tt0100","tmdb":30,"tvdb":20}`
	for _, service := range []string{Trakt, Simkl} {
		for _, action := range []string{"start", "pause", "stop"} {
			for _, r := range h.f.wait(t, map[string]int{"start": 3, "pause": 1, "stop": 2}[action], service, "/scrobble/"+action) {
				if r.header.Get("Authorization") != "Bearer "+service+"-alice" {
					t.Errorf("%s: authorized with %q", service, r.header.Get("Authorization"))
				}
			}
		}
		starts, pauses, stops := h.f.sent(service, "/scrobble/start"), h.f.sent(service, "/scrobble/pause"), h.f.sent(service, "/scrobble/stop")
		sameJSON(t, service+" movie start", starts[0].body, `{"movie":{"ids":`+movieIDs+`},"progress":0}`)
		sameJSON(t, service+" movie pause", pauses[0].body, `{"movie":{"ids":`+movieIDs+`},"progress":30}`)
		sameJSON(t, service+" movie resumed", starts[1].body, `{"movie":{"ids":`+movieIDs+`},"progress":30}`)
		sameJSON(t, service+" movie stop", stops[0].body, `{"movie":{"ids":`+movieIDs+`},"progress":96}`)
		sameJSON(t, service+" episode start", starts[2].body, `{"show":{"ids":`+showIDs+`},"episode":{"season":1,"number":2},"progress":0}`)
		sameJSON(t, service+" episode stop", stops[1].body, `{"show":{"ids":`+showIDs+`},"episode":{"season":1,"number":2},"progress":50}`)
	}
	trakt := h.f.sent("trakt", "/scrobble/start")[0]
	if trakt.header.Get("trakt-api-key") != "trakt-client" || trakt.header.Get("trakt-api-version") != "2" ||
		trakt.header.Get("Content-Type") != "application/json" || trakt.header.Get("User-Agent") != "Polyfin/1.2.3" {
		t.Errorf("Trakt headers: %v", trakt.header)
	}
	simkl := h.f.sent("simkl", "/scrobble/start")[0]
	if simkl.query.Get("client_id") != "simkl-client" || simkl.query.Get("app-name") != "polyfin" || simkl.query.Get("app-version") != "1.2.3" {
		t.Errorf("Simkl parameters: %v", simkl.query)
	}

	starts := h.f.wait(t, 3, "mdblist", "/scrobble/start")
	pauses := h.f.wait(t, 1, "mdblist", "/scrobble/pause")
	stops := h.f.wait(t, 2, "mdblist", "/scrobble/stop")
	sameJSON(t, "MDBList movie start", starts[0].body, `{"movie":{"ids":`+movieIDs+`},"progress":0}`)
	sameJSON(t, "MDBList movie pause", pauses[0].body, `{"movie":{"ids":`+movieIDs+`},"progress":30}`)
	sameJSON(t, "MDBList movie stop", stops[0].body, `{"movie":{"ids":`+movieIDs+`},"progress":96}`)
	sameJSON(t, "MDBList episode stop", stops[1].body, `{"show":{"ids":`+showIDs+`,"season":{"number":1,"episode":{"number":2}}},"progress":50}`)
	if stops[0].query.Get("apikey") != "mdblist-alice" || stops[0].header.Get("Authorization") != "" {
		t.Errorf("MDBList authorized with %v %v", stops[0].query, stops[0].header)
	}

	// PublicMetaDB keeps the resume point of the pause, the movie in its
	// history once Polyfin counted it played, and the episode's resume
	// point.
	resumes := h.f.wait(t, 2, "publicmetadb", "/api/external/resume")
	watched := h.f.wait(t, 1, "publicmetadb", "/api/external/watched")
	sameJSON(t, "movie resume point", resumes[0].body, `{"tmdb_id":10,"media_type":"movie","position_ms":1800000,"runtime_ms":6000000}`)
	sameJSON(t, "movie watched", watched[0].body, `{"tmdb_id":10,"media_type":"movie"}`)
	sameJSON(t, "episode resume point", resumes[1].body, `{"tmdb_id":30,"media_type":"tv","season":1,"episode":2,"position_ms":3000000,"runtime_ms":6000000}`)
	if watched[0].header.Get("Authorization") != "Bearer publicmetadb-alice" {
		t.Errorf("PublicMetaDB authorized with %v", watched[0].header)
	}
	h.idle(t)
	// One viewing makes one play on PublicMetaDB, and nothing else is sent.
	if len(h.f.sent("publicmetadb", "/api/external/watched")) != 1 || len(h.f.sent("trakt", "/sync/history")) != 0 ||
		len(h.f.sent("publicmetadb", "/api/external/resume")) != 2 {
		t.Errorf("sent too much: %d plays, %d resume points", len(h.f.sent("publicmetadb", "/api/external/watched")),
			len(h.f.sent("publicmetadb", "/api/external/resume")))
	}
	if status := h.status(t, user, Trakt); status.LastSentAt == nil || !status.LastSentAt.Equal(clockStart) {
		t.Errorf("Trakt: %+v", status)
	}
}

func TestPlayedMarksChangeTheHistory(t *testing.T) {
	h := newHarness(t)
	user := h.connectAll(t, "bob")
	// A season played, as of now; a movie played on a given day.
	h.Mark(user, Mark{Played: true, Scope: ScopeSeason, Titles: titles(pilot, episode)})
	day := time.Date(2026, 9, 1, 20, 30, 0, 0, time.UTC)
	h.Mark(user, Mark{Played: true, Date: &day, Titles: titles(movie)})
	// The series unplayed, then one season of it, then one episode.
	h.Mark(user, Mark{Played: false, Scope: ScopeSeries, Titles: titles(pilot, episode, finale)})
	h.Mark(user, Mark{Played: false, Scope: ScopeSeason, Titles: titles(pilot, episode)})
	h.Mark(user, Mark{Played: false, Titles: titles(finale)})

	show := `"ids":{"imdb":"tt0100","tmdb":30,"tvdb":20}`
	now := clockStart.Format(time.RFC3339)
	for _, service := range []struct{ name, add, remove string }{
		{"trakt", "/sync/history", "/sync/history/remove"},
		{"simkl", "/sync/history", "/sync/history/remove"},
		{"mdblist", "/sync/watched", "/sync/watched/remove"},
	} {
		added := h.f.wait(t, 2, service.name, service.add)
		removed := h.f.wait(t, 3, service.name, service.remove)
		sameJSON(t, service.name+" season played", added[0].body,
			`{"shows":[{`+show+`,"seasons":[{"number":1,"episodes":[{"number":1,"watched_at":"`+now+`"},{"number":2,"watched_at":"`+now+`"}]}]}]}`)
		sameJSON(t, service.name+" movie played", added[1].body, `{"movies":[{"ids":{"imdb":"tt0000001","tmdb":10},"watched_at":"2026-09-01T20:30:00Z"}]}`)
		sameJSON(t, service.name+" series unplayed", removed[0].body,
			`{"shows":[{`+show+`,"seasons":[{"number":1,"episodes":[{"number":1},{"number":2}]},{"number":2,"episodes":[{"number":1}]}]}]}`)
		sameJSON(t, service.name+" episode unplayed", removed[2].body, `{"shows":[{`+show+`,"seasons":[{"number":2,"episodes":[{"number":1}]}]}]}`)
	}

	// PublicMetaDB takes one play at a time, and removes what a mark
	// covers at once.
	h.f.wait(t, 6, "publicmetadb", "/api/external/watched")
	h.idle(t)
	watched := h.f.sent("publicmetadb", "/api/external/watched")
	var plays, removals []string
	for _, r := range watched {
		if r.method == http.MethodPost {
			encoded, _ := jsonString(r.body)
			plays = append(plays, encoded)
		} else {
			removals = append(removals, r.query.Encode())
		}
	}
	wantPlays := []string{
		`{"episode":1,"media_type":"tv","season":1,"tmdb_id":30,"watched_at":"` + now + `"}`,
		`{"episode":2,"media_type":"tv","season":1,"tmdb_id":30,"watched_at":"` + now + `"}`,
		`{"media_type":"movie","tmdb_id":10,"watched_at":"2026-09-01T20:30:00Z"}`,
	}
	wantRemovals := []string{"media_type=tv&tmdb_id=30", "media_type=tv&season=1&tmdb_id=30", "episode=1&media_type=tv&season=2&tmdb_id=30"}
	if strings.Join(plays, "\n") != strings.Join(wantPlays, "\n") || strings.Join(removals, " ") != strings.Join(wantRemovals, " ") {
		t.Errorf("PublicMetaDB:\n%s\n%s", strings.Join(plays, "\n"), strings.Join(removals, " "))
	}
}

func TestPublicMetaDBLooksUpTheTMDBIdentifier(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "carol")
	h.connected(t, user, PublicMetaDB, "pm-carol")
	h.f.on("GET", "/publicmetadb/api/external/mappings/lookup", func(r request) answer {
		if r.query.Get("id_type") == "imdb" && r.query.Get("id_value") == "tt0100" && r.query.Get("media_type") == "tv" {
			return answer{status: http.StatusOK, body: `{"results":[{"tmdb_id":30,"media_type":"tv"}],"total":1}`}
		}
		return answer{status: http.StatusOK, body: `{"results":[],"total":0}`}
	})
	imdbOnly := Title{Item: itemID(5), IMDb: "tt0100", Episode: true, Season: 1, Number: 2}
	unknown := Title{Item: itemID(6), IMDb: "tt9999999"}
	h.Mark(user, Mark{Played: true, Titles: titles(unknown, imdbOnly)})
	// Resume points name any identifier.
	h.Playback(user, Playback{Event: Stopped, Device: device, Item: imdbOnly.Item, Position: 10 * time.Minute, PositionKnown: true,
		Runtime: 40 * time.Minute, Title: titled(imdbOnly)})
	played := h.f.wait(t, 1, "publicmetadb", "/api/external/watched")
	sameJSON(t, "played", played[0].body, `{"tmdb_id":30,"media_type":"tv","season":1,"episode":2,"watched_at":"`+clockStart.Format(time.RFC3339)+`"}`)
	resume := h.f.wait(t, 1, "publicmetadb", "/api/external/resume")
	sameJSON(t, "resume point", resume[0].body, `{"id_type":"imdb","id_value":"tt0100","media_type":"tv","season":1,"episode":2,"position_ms":600000,"runtime_ms":2400000}`)
	h.idle(t)
	if len(h.f.sent("publicmetadb", "/api/external/watched")) != 1 {
		t.Error("a title PublicMetaDB does not know was sent")
	}
}

func TestOneViewingMakesOneHistoryEntry(t *testing.T) {
	h := newHarness(t)
	user := h.connectAll(t, "dave")
	// Trakt counts the stop at 85% watched; Simkl, as it may, does not.
	h.f.reply("POST", "/trakt/scrobble/stop", http.StatusCreated, `{"id":1,"action":"scrobble","progress":85}`)
	h.f.reply("POST", "/simkl/scrobble/stop", http.StatusCreated, `{"id":2,"action":"pause","progress":85}`)
	// MDBList already had it.
	h.f.reply("POST", "/mdblist/scrobble/stop", http.StatusConflict, `{"watched_at":"2026-10-05T11:59:00Z"}`)
	h.Playback(user, play(Started, movie, 0, false, false))
	h.Playback(user, play(Stopped, movie, 85, false, false))
	// The user then marks it played from an app, with an episode.
	h.Mark(user, Mark{Played: true, Titles: titles(movie, episode)})

	trakt := h.f.wait(t, 1, "trakt", "/sync/history")
	sameJSON(t, "Trakt", trakt[0].body, `{"shows":[{"ids":{"imdb":"tt0100","tmdb":30,"tvdb":20},
		"seasons":[{"number":1,"episodes":[{"number":2,"watched_at":"`+clockStart.Format(time.RFC3339)+`"}]}]}]}`)
	simkl := h.f.wait(t, 1, "simkl", "/sync/history")
	if body, _ := jsonString(simkl[0].body); !strings.Contains(body, `"movies"`) || !strings.Contains(body, `"shows"`) {
		t.Errorf("Simkl, which did not count the movie: %s", body)
	}
	mdblist := h.f.wait(t, 1, "mdblist", "/sync/watched")
	if body, _ := jsonString(mdblist[0].body); strings.Contains(body, `"movies"`) {
		t.Errorf("MDBList: %s", body)
	}

	// A played mark a day later is another viewing.
	h.idle(t)
	h.now = func() time.Time { return clockStart.Add(24 * time.Hour) }
	h.Mark(user, Mark{Played: true, Titles: titles(movie)})
	if again := h.f.wait(t, 2, "trakt", "/sync/history"); !strings.Contains(mustJSON(again[1].body), "tt0000001") {
		t.Errorf("the next day: %s", mustJSON(again[1].body))
	}
	h.idle(t)
}

func TestTraktTokensAreRefreshed(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "erin")
	// The token expires within the day.
	soon := clockStart.Add(time.Hour)
	if err := h.connect(t.Context(), user, Trakt, connection{token: "old-access", refresh: "old-refresh", expires: &soon, connectedAt: clockStart}); err != nil {
		t.Fatal(err)
	}
	var refreshes atomic.Int32
	h.f.on("POST", "/trakt/oauth/token", func(r request) answer {
		n := refreshes.Add(1)
		return answer{status: http.StatusOK, body: `{"access_token":"access-` + string(rune('0'+n)) + `","token_type":"bearer","expires_in":604800,
			"refresh_token":"refresh-` + string(rune('0'+n)) + `","scope":"public","created_at":` + strconv.FormatInt(clockStart.Unix(), 10) + `}`}
	})
	// Trakt refuses every token but the latest.
	h.f.on("POST", "/trakt/scrobble/stop", func(r request) answer {
		if r.header.Get("Authorization") != "Bearer access-"+string(rune('0'+refreshes.Load())) {
			return answer{status: http.StatusUnauthorized, body: `{}`}
		}
		return answer{status: http.StatusCreated, body: `{"action":"scrobble"}`}
	})
	h.Playback(user, play(Stopped, movie, 90, false, false))
	stops := h.f.wait(t, 1, "trakt", "/scrobble/stop")
	if stops[0].header.Get("Authorization") != "Bearer access-1" {
		t.Errorf("sent with %q", stops[0].header.Get("Authorization"))
	}
	refresh := h.f.sent("trakt", "/oauth/token")[0]
	sameJSON(t, "refresh", refresh.body, `{"refresh_token":"old-refresh","client_id":"trakt-client","client_secret":"trakt-secret",
		"redirect_uri":"urn:ietf:wg:oauth:2.0:oob","grant_type":"refresh_token"}`)

	// Revoked elsewhere: the next send is refused, refreshed once with the
	// new refresh token, and sent again.
	h.f.on("POST", "/trakt/scrobble/stop", func(r request) answer {
		if r.header.Get("Authorization") != "Bearer access-2" {
			return answer{status: http.StatusUnauthorized, body: `{}`}
		}
		return answer{status: http.StatusCreated, body: `{"action":"scrobble"}`}
	})
	h.Playback(user, play(Stopped, episode, 90, false, false))
	stops = h.f.wait(t, 3, "trakt", "/scrobble/stop")
	if stops[1].header.Get("Authorization") != "Bearer access-1" || stops[2].header.Get("Authorization") != "Bearer access-2" {
		t.Errorf("sent with %q then %q", stops[1].header.Get("Authorization"), stops[2].header.Get("Authorization"))
	}
	if second := h.f.sent("trakt", "/oauth/token")[1]; !strings.Contains(mustJSON(second.body), `"refresh_token":"refresh-1"`) {
		t.Errorf("second refresh: %s", mustJSON(second.body))
	}
	h.idle(t)
	c, _, _ := h.connection(t.Context(), user, Trakt)
	if c.token != "access-2" || c.refresh != "refresh-2" || c.problem != "" {
		t.Errorf("saved: %+v", c)
	}
	if strings.Contains(h.log.String(), "access-") || strings.Contains(h.log.String(), "refresh-") {
		t.Errorf("log: %s", h.log.String())
	}
}

func TestARefusedTokenAsksTheUserToReconnect(t *testing.T) {
	h := newHarness(t)
	user := h.connectAll(t, "frank")
	h.f.reply("POST", "/trakt/scrobble/stop", http.StatusUnauthorized, `{}`)
	h.f.reply("POST", "/trakt/oauth/token", http.StatusBadRequest, `{"error":"invalid_grant","error_description":"session not found"}`)
	h.f.reply("POST", "/mdblist/scrobble/stop", http.StatusUnauthorized, `{"error":"Invalid API key"}`)
	h.Playback(user, play(Stopped, movie, 90, false, false))
	h.f.wait(t, 1, "trakt", "/oauth/token")
	eventually(t, "Trakt asking to reconnect", func() bool { return h.status(t, user, Trakt).Problem == ProblemReconnect })
	eventually(t, "MDBList asking to reconnect", func() bool { return h.status(t, user, MDBList).Problem == ProblemReconnect })
	if status := h.status(t, user, Simkl); status.Problem != "" {
		t.Errorf("Simkl: %+v", status)
	}
	h.idle(t)

	// Nothing more goes to them until the user connects again.
	h.Playback(user, play(Stopped, episode, 90, false, false))
	h.Mark(user, Mark{Played: true, Titles: titles(movie)})
	h.f.wait(t, 2, "simkl", "/scrobble/stop")
	h.f.wait(t, 1, "simkl", "/sync/history")
	h.idle(t)
	if n := len(h.f.sent("trakt", "/scrobble/stop")); n != 1 || len(h.f.sent("mdblist", "/scrobble/stop")) != 1 ||
		len(h.f.sent("trakt", "/sync/history")) != 0 || len(h.f.sent("mdblist", "/sync/watched")) != 0 {
		t.Errorf("sent after the refusal: %d to Trakt", n)
	}
	h.f.reply("GET", "/mdblist/user", http.StatusOK, `{"username":"frank"}`)
	h.f.reply("POST", "/mdblist/scrobble/stop", http.StatusOK, `{"action":"scrobble"}`)
	if status, err := h.ConnectKey(t.Context(), user, MDBList, "mdblist-frank-2"); err != nil || status.Problem != "" {
		t.Fatalf("reconnected: %+v %v", status, err)
	}
	h.Playback(user, play(Stopped, finale, 90, false, false))
	if stops := h.f.wait(t, 2, "mdblist", "/scrobble/stop"); stops[1].query.Get("apikey") != "mdblist-frank-2" {
		t.Errorf("sent with %v", stops[1].query)
	}
}

func TestChangesAreRetriedAfterAnOutage(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "gina")
	h.connected(t, user, MDBList, "mdblist-gina")
	var calls atomic.Int32
	h.f.on("POST", "/mdblist/sync/watched", func(request) answer {
		if calls.Add(1) <= 4 {
			return answer{status: http.StatusServiceUnavailable, body: `{}`}
		}
		return answer{status: http.StatusOK, body: `{"updated":{"movies":1}}`}
	})
	// Starts are not retried.
	h.f.reply("POST", "/mdblist/scrobble/start", http.StatusServiceUnavailable, `{}`)
	h.Playback(user, play(Started, movie, 0, false, false))
	h.Mark(user, Mark{Played: true, Titles: titles(movie)})
	h.f.wait(t, 3, "mdblist", "/sync/watched")
	eventually(t, "MDBList shown unreachable", func() bool { return h.status(t, user, MDBList).Problem == ProblemUnreachable })
	h.f.wait(t, 5, "mdblist", "/sync/watched")
	eventually(t, "MDBList reachable again", func() bool { return h.status(t, user, MDBList).Problem == "" })
	h.idle(t)
	if status := h.status(t, user, MDBList); status.LastSentAt == nil {
		t.Errorf("after the outage: %+v", status)
	}
	if n := len(h.f.sent("mdblist", "/sync/watched")); n != 5 {
		t.Errorf("%d sends", n)
	}
	if n := len(h.f.sent("mdblist", "/scrobble/start")); n != 1 {
		t.Errorf("the start was sent %d times", n)
	}
	// The service's wait is respected.
	h.f.on("POST", "/mdblist/sync/watched/remove", func(request) answer {
		return answer{status: http.StatusTooManyRequests, body: `{}`, header: http.Header{"Retry-After": {"3600"}}}
	})
	h.Mark(user, Mark{Played: false, Titles: titles(movie)})
	h.f.wait(t, 1, "mdblist", "/sync/watched/remove")
	time.Sleep(100 * time.Millisecond)
	if n := len(h.f.sent("mdblist", "/sync/watched/remove")); n != 1 {
		t.Errorf("sent %d times within the hour asked", n)
	}
}

func TestQueuedChangesSurviveARestart(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "hank")
	h.connected(t, user, Simkl, "simkl-hank")
	h.f.reply("POST", "/simkl/sync/history", http.StatusBadGateway, `{}`)
	h.Mark(user, Mark{Played: true, Titles: titles(movie)})
	h.f.wait(t, 1, "simkl", "/sync/history")
	h.Close()

	h.f.reply("POST", "/simkl/sync/history", http.StatusCreated, `{"added":{"movies":1}}`)
	again := newService(h.db, h.store, h.f, io.Discard)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		again.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	sent := h.f.wait(t, 2, "simkl", "/sync/history")
	sameJSON(t, "after the restart", sent[1].body, `{"movies":[{"ids":{"imdb":"tt0000001","tmdb":10},"watched_at":"`+clockStart.Format(time.RFC3339)+`"}]}`)
	again.idle(t)
}

func TestNothingIsSentForUntrackedTitlesOrToOtherUsers(t *testing.T) {
	h := newHarness(t)
	alice := h.connectAll(t, "alice")
	bob := h.user(t, "bob")
	h.connected(t, bob, MDBList, "mdblist-bob")
	carol := h.user(t, "carol")

	// Live TV, music, titles without identifiers: no title.
	untracked := func(context.Context) (Title, bool) { return Title{}, false }
	h.Playback(alice, Playback{Event: Started, Device: device, Item: itemID(7), Title: untracked})
	h.Playback(alice, Playback{Event: Stopped, Device: device, Item: itemID(7), Played: true, Title: untracked})
	h.Mark(alice, Mark{Played: true, Titles: titles()})
	// A user without connections.
	h.Playback(carol, play(Stopped, movie, 95, false, true))
	h.Mark(carol, Mark{Played: true, Titles: titles(movie)})
	// Bob's own.
	h.Playback(bob, play(Stopped, episode, 95, false, true))
	h.Mark(bob, Mark{Played: true, Titles: titles(movie)})
	h.f.wait(t, 1, "mdblist", "/scrobble/stop")
	h.f.wait(t, 1, "mdblist", "/sync/watched")
	h.idle(t)

	h.f.mu.Lock()
	defer h.f.mu.Unlock()
	for _, r := range h.f.requests {
		if r.service != "mdblist" || r.query.Get("apikey") != "mdblist-bob" {
			t.Errorf("sent %s %s %s with %v", r.service, r.method, r.path, r.query)
		}
	}
	if len(h.f.requests) != 2 {
		t.Errorf("%d requests", len(h.f.requests))
	}
}
