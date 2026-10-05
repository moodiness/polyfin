package jellyfin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/trackers"
)

// trackerCall is a request a fake tracking service received.
type trackerCall struct {
	method, path string
	query        url.Values
	body         map[string]any
}

// trackerFakes stands in for MDBList (/mdblist) and PublicMetaDB
// (/publicmetadb), accepting every key and change, and knowing the test
// series by its TMDB identifier.
type trackerFakes struct {
	mu    sync.Mutex
	calls []trackerCall
	url   string
}

func newTrackerFakes(t *testing.T) *trackerFakes {
	t.Helper()
	f := &trackerFakes{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		call := trackerCall{method: r.Method, path: r.URL.Path, query: r.URL.Query()}
		_ = json.Unmarshal(raw, &call.body)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/mdblist/user":
			_, _ = io.WriteString(w, `{"username":"member"}`)
			return
		case "/publicmetadb/api/external/lists":
			_, _ = io.WriteString(w, `{"items":[]}`)
			return
		case "/publicmetadb/api/external/mappings/lookup":
			_, _ = io.WriteString(w, `{"results":[{"tmdb_id":30,"media_type":"tv"}],"total":1}`)
			return
		}
		f.mu.Lock()
		f.calls = append(f.calls, call)
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(server.Close)
	f.url = server.URL
	return f
}

// wait waits until n changes reached the fakes, and returns them.
func (f *trackerFakes) wait(t *testing.T, n int) []trackerCall {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		f.mu.Lock()
		calls := append([]trackerCall(nil), f.calls...)
		f.mu.Unlock()
		if len(calls) >= n {
			return calls
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d changes reached the services, want %d: %+v", len(calls), n, calls)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// watchedAt matches the dates of changes, which summaries leave out.
var watchedAt = regexp.MustCompile(`,?"watched_at":"[^"]*"`)

// summary is a call as method, path and body, without the key and the
// dates it holds.
func (c trackerCall) summary() string {
	text := c.method + " " + c.path
	query := url.Values{}
	for key, values := range c.query {
		if key != "apikey" {
			query[key] = values
		}
	}
	if len(query) > 0 {
		text += "?" + query.Encode()
	}
	if c.body != nil {
		body, _ := json.Marshal(c.body)
		text += " " + watchedAt.ReplaceAllString(string(body), "")
	}
	return text
}

func TestUsersTrackingServicesHearOfTheirMoviesAndEpisodes(t *testing.T) {
	fakes := newTrackerFakes(t)
	var tracker *trackers.Service
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(options *Options, pool *pgxpool.Pool) {
		tracker = trackers.New(trackers.Options{DB: pool, Settings: options.Accounts.Settings, Version: "test", Logger: options.Logger,
			URLs: map[string]string{trackers.MDBList: fakes.url + "/mdblist", trackers.PublicMetaDB: fakes.url + "/publicmetadb"}})
		options.Trackers = tracker
	})
	t.Cleanup(tracker.Close)
	tr := trackingOn(t, s)
	// Live TV and music, which no service tracks.
	if _, err := s.addons.Install(t.Context(), addons.Shared(), newTVAddon(t, false, "").url, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.Install(t.Context(), addons.Shared(), newFakeEclipse(t, "").url, false); err != nil {
		t.Fatal(err)
	}
	member := mustID(t, tr.user)
	for service, key := range map[string]string{trackers.MDBList: "mdblist-member", trackers.PublicMetaDB: "pm-member"} {
		if _, err := tracker.ConnectKey(t.Context(), member, service, key); err != nil {
			t.Fatal(err)
		}
	}
	// Another user's activity reaches no one.
	tr.testServer.user("other", nil)
	other := tr.signIn("other", "phone")
	s.call(http.MethodPost, "/Sessions/Playing", app("phone", other), map[string]any{"ItemId": tr.movie, "PositionTicks": 0})
	s.call(http.MethodPost, "/UserPlayedItems/"+tr.series, app("phone", other), nil)

	var channels, songs QueryResult
	s.get(t, "/LiveTv/Channels", tr.token, &channels)
	s.get(t, "/Items?IncludeItemTypes=Audio&Recursive=true", tr.token, &songs)
	if len(channels.Items) == 0 || len(songs.Items) == 0 {
		t.Fatalf("%d channels, %d songs", len(channels.Items), len(songs.Items))
	}
	for _, untracked := range []string{channels.Items[0].Id, songs.Items[0].Id} {
		tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": untracked, "PositionTicks": 0})
		tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": untracked})
		tr.mark(t, http.MethodPost, "/UserPlayedItems/"+untracked)
	}

	// The movie runs 1h30: started, stopped past the played mark.
	tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": tr.movie, "PositionTicks": 0})
	tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": tr.movie, "PositionTicks": 52_000_000_000})
	// The first season marked played; marking it again counts no new play.
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.seasons[0])
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.seasons[0])
	// Its first episode unmarked.
	tr.mark(t, http.MethodDelete, "/Users/"+tr.user+"/PlayedItems/"+tr.episodes[0])

	calls := fakes.wait(t, 8)
	var mdblist, publicMetaDB []string
	for _, call := range calls {
		if strings.HasPrefix(call.path, "/mdblist/") {
			if call.query.Get("apikey") != "mdblist-member" {
				t.Errorf("sent with %v", call.query)
			}
			mdblist = append(mdblist, call.summary())
		} else {
			publicMetaDB = append(publicMetaDB, call.summary())
		}
	}
	want := []string{
		`POST /mdblist/scrobble/start {"movie":{"ids":{"imdb":"tt0000","tmdb":10}},"progress":0}`,
		`POST /mdblist/scrobble/stop {"movie":{"ids":{"imdb":"tt0000","tmdb":10}},"progress":96.3}`,
		`POST /mdblist/sync/watched {"shows":[{"ids":{"imdb":"tt0100","tvdb":20},"seasons":[{"episodes":[{"number":1},{"number":2}],"number":1}]}]}`,
		`POST /mdblist/sync/watched/remove {"shows":[{"ids":{"imdb":"tt0100","tvdb":20},"seasons":[{"episodes":[{"number":1}],"number":1}]}]}`,
	}
	if strings.Join(mdblist, "\n") != strings.Join(want, "\n") {
		t.Errorf("MDBList:\n%s\nwant\n%s", strings.Join(mdblist, "\n"), strings.Join(want, "\n"))
	}
	want = []string{
		`POST /publicmetadb/api/external/watched {"media_type":"movie","tmdb_id":10}`,
		`POST /publicmetadb/api/external/watched {"episode":1,"media_type":"tv","season":1,"tmdb_id":30}`,
		`POST /publicmetadb/api/external/watched {"episode":2,"media_type":"tv","season":1,"tmdb_id":30}`,
		`DELETE /publicmetadb/api/external/watched?episode=1&media_type=tv&season=1&tmdb_id=30`,
	}
	if strings.Join(publicMetaDB, "\n") != strings.Join(want, "\n") {
		t.Errorf("PublicMetaDB:\n%s\nwant\n%s", strings.Join(publicMetaDB, "\n"), strings.Join(want, "\n"))
	}
}

func TestUserDataUploadsReachTrackingServicesAsPlayedMarks(t *testing.T) {
	fakes := newTrackerFakes(t)
	var tracker *trackers.Service
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(options *Options, pool *pgxpool.Pool) {
		tracker = trackers.New(trackers.Options{DB: pool, Settings: options.Accounts.Settings, Version: "test", Logger: options.Logger,
			URLs: map[string]string{trackers.MDBList: fakes.url + "/mdblist"}})
		options.Trackers = tracker
	})
	t.Cleanup(tracker.Close)
	tr := trackingOn(t, s)
	if _, err := tracker.ConnectKey(t.Context(), mustID(t, tr.user), trackers.MDBList, "mdblist-member"); err != nil {
		t.Fatal(err)
	}
	upload := func(path string, body map[string]any) {
		t.Helper()
		if status, data := tr.call(http.MethodPost, path, app("tv", tr.token), body); status != http.StatusOK {
			t.Fatalf("%s: %d %s", path, status, data)
		}
	}
	// Played offline on a given day, then uploaded again, then unplayed
	// through the older route; a favorite changes nothing played.
	upload("/UserItems/"+tr.episodes[0]+"/UserData", map[string]any{"Played": true, "LastPlayedDate": "2026-09-01T20:00:00Z"})
	upload("/UserItems/"+tr.episodes[0]+"/UserData", map[string]any{"Played": true})
	upload("/UserItems/"+tr.episodes[0]+"/UserData", map[string]any{"IsFavorite": true})
	upload("/Users/"+tr.user+"/Items/"+tr.episodes[0]+"/UserData", map[string]any{"Played": false})
	upload("/UserItems/"+tr.movie+"/UserData", map[string]any{"Played": true, "PlayCount": 1})

	calls := fakes.wait(t, 3)
	var got []string
	for _, call := range calls {
		body, _ := json.Marshal(call.body)
		got = append(got, call.method+" "+call.path+" "+string(body))
	}
	episode := `{"shows":[{"ids":{"imdb":"tt0100","tvdb":20},"seasons":[{"episodes":[{"number":1%s}],"number":1}]}]}`
	if len(got) != 3 || got[0] != "POST /mdblist/sync/watched "+fmt.Sprintf(episode, `,"watched_at":"2026-09-01T20:00:00Z"`) ||
		got[1] != "POST /mdblist/sync/watched/remove "+fmt.Sprintf(episode, "") ||
		!strings.HasPrefix(got[2], `POST /mdblist/sync/watched {"movies":[{"ids":{"imdb":"tt0000","tmdb":10}`) {
		t.Errorf("sent:\n%s", strings.Join(got, "\n"))
	}
}
