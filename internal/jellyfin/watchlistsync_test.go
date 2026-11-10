package jellyfin

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/trackers"
)

// The titles a user puts on their watchlist or takes off it themselves,
// from the watchlist routes or the playlist's, reach the PublicMetaDB
// watchlist they sync: an episode as its series, once; the series leaving
// with its last episode. Moving an entry or deleting the playlist sends
// nothing.
func TestTheUsersOwnWatchlistChangesReachPublicMetaDB(t *testing.T) {
	t.Parallel()
	fakes := newTrackerFakes(t)
	var tracker *trackers.Service
	var db *pgxpool.Pool
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(options *Options, pool *pgxpool.Pool) {
		tracker = trackers.New(trackers.Options{DB: pool, Settings: options.Accounts.Settings, Version: "test", Logger: options.Logger,
			URLs: map[string]string{trackers.PublicMetaDB: fakes.url + "/publicmetadb"}})
		options.Trackers = tracker
		db = pool
	})
	t.Cleanup(tracker.Close)
	tr := trackingOn(t, s)
	member := mustID(t, tr.user)
	if _, err := tracker.ConnectKey(t.Context(), member, trackers.PublicMetaDB, "pm-member"); err != nil {
		t.Fatal(err)
	}
	// Synced without an import, which this test does not look at.
	if _, err := db.Exec(t.Context(), `INSERT INTO tracking_imports (user_id, service, watchlist) VALUES ($1, $2, true)
		ON CONFLICT (user_id, service) DO UPDATE SET watchlist = true`, member, trackers.PublicMetaDB); err != nil {
		t.Fatal(err)
	}
	call := func(method, path string, body any) {
		t.Helper()
		if status, data := tr.call(method, path, app("tv", tr.token), body); status >= 300 {
			t.Fatalf("%s %s: %d %s", method, path, status, data)
		}
	}
	var held struct{ PlaylistId string }
	s.get(t, "/Polyfin/UserItems/Watchlist", tr.token, &held)
	playlist := held.PlaylistId
	entries := func() map[string]string {
		t.Helper()
		var page QueryResult
		s.get(t, "/Playlists/"+playlist+"/Items", tr.token, &page)
		ids := map[string]string{}
		for _, item := range page.Items {
			ids[item.Id] = item.PlaylistItemId
		}
		return ids
	}

	call(http.MethodPost, "/Polyfin/UserItems/"+tr.movie+"/Watchlist", nil)
	call(http.MethodPost, "/Polyfin/UserItems/"+tr.episodes[0]+"/Watchlist", nil)
	// Another episode of the series, from the playlist's route: no news.
	call(http.MethodPost, "/Playlists/"+playlist+"/Items?ids="+tr.episodes[1], nil)
	fakes.wait(t, 2)
	// Moved, then one episode of two taken off: nothing.
	call(http.MethodPost, "/Playlists/"+playlist+"/Items/"+entries()[tr.movie]+"/Move/0", nil)
	call(http.MethodDelete, "/Playlists/"+playlist+"/Items?entryIds="+entries()[tr.episodes[0]], nil)
	// The last episode leaves, then the playlist made the movie alone and
	// the series again.
	call(http.MethodDelete, "/Polyfin/UserItems/"+tr.series+"/Watchlist", nil)
	call(http.MethodPost, "/Playlists/"+playlist, map[string]any{"Ids": []string{tr.episodes[0]}})
	fakes.wait(t, 5)
	// Deleting the playlist sends nothing.
	call(http.MethodDelete, "/Items/"+playlist, nil)
	time.Sleep(200 * time.Millisecond)

	calls := fakes.wait(t, 5)
	var got []string
	for _, c := range calls {
		got = append(got, c.summary())
	}
	want := []string{
		`POST /publicmetadb/api/external/lists/w1/items {"media_type":"movie","tmdb_id":10}`,
		`POST /publicmetadb/api/external/lists/w1/items {"media_type":"tv","tmdb_id":30}`,
		`DELETE /publicmetadb/api/external/lists/w1/items/li_30`,
		`POST /publicmetadb/api/external/lists/w1/items {"media_type":"tv","tmdb_id":30}`,
		`DELETE /publicmetadb/api/external/lists/w1/items/li_10`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("PublicMetaDB:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
