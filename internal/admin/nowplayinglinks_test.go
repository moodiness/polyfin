package admin

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
)

// playingSession is a playback of item by user on a device of their own,
// started at started.
func playingSession(user accounts.User, item accounts.ID, started time.Time) jellyfin.LiveSession {
	return jellyfin.LiveSession{
		User:    user,
		Device:  accounts.Device{ID: randomID(), DeviceInfo: accounts.DeviceInfo{DeviceName: "TV"}},
		Playing: playback.NowPlaying{PlayState: playback.PlayState{PlayMethod: "DirectPlay"}, Started: started},
		Item:    library.Item{ID: item, Kind: library.KindMovie, Name: "Amber Road"},
		Found:   true,
	}
}

// readSessions reads the playbacks Now playing lists to client.
func readSessions(t *testing.T, client browser) []liveSessionJSON {
	t.Helper()
	var all []liveSessionJSON
	if status := client.raw(http.MethodGet, "/sessions", "", &all); status != http.StatusOK {
		t.Fatalf("sessions: %d", status)
	}
	return all
}

// Now playing links a title the user reading reaches, in their own view
// rather than the player's: one out of it, such as a member's own source,
// stays plain text. Each playback is looked up once, not at every read,
// and a new playback of the same title again.
func TestNowPlayingLinksTheTitlesTheReaderReaches(t *testing.T) {
	t.Parallel()
	shown, hidden := randomID(), randomID()
	reached := &reachedItems{ids: []accounts.ID{shown}}
	sessions := &fakeSessions{}
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Sessions, o.ActivityTitles, o.WebClient = sessions, reached, true
	})
	admin := api.signedIn("root", true)
	robin := accounts.User{ID: randomID(), Name: "robin"}
	started := time.Date(2026, 11, 10, 20, 0, 0, 0, time.UTC)
	sessions.sessions = []jellyfin.LiveSession{playingSession(robin, shown, started), playingSession(robin, hidden, started)}
	for range 3 {
		all := readSessions(t, admin)
		if len(all) != 2 || all[0].Item == nil || all[1].Item == nil {
			t.Fatalf("sessions: %+v", all)
		}
		if !all[0].Item.Linked {
			t.Errorf("a title the administrator reaches is not linked: %+v", all[0].Item)
		}
		if all[1].Item.Linked {
			t.Errorf("a title out of the administrator's view is linked: %+v", all[1].Item)
		}
	}
	if !slices.Equal(reached.asked, []string{"root"}) {
		t.Errorf("titles found for %v, want the administrator once", reached.asked)
	}

	again := sessions.sessions[0]
	again.Playing.Started = started.Add(time.Hour)
	sessions.sessions = []jellyfin.LiveSession{again}
	if all := readSessions(t, admin); len(all) != 1 || all[0].Item == nil || !all[0].Item.Linked {
		t.Errorf("a new playback: %+v", all)
	}
	if !slices.Equal(reached.asked, []string{"root", "root"}) {
		t.Errorf("titles found for %v, want the administrator twice", reached.asked)
	}
}

// A member's own playback is looked up in their own view.
func TestNowPlayingLinksAMembersTitlesInTheirView(t *testing.T) {
	t.Parallel()
	shown := randomID()
	reached := &reachedItems{ids: []accounts.ID{shown}}
	sessions := &fakeSessions{}
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Sessions, o.ActivityTitles, o.WebClient = sessions, reached, true
	})
	member := api.signedIn("sam", false)
	users, err := api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(users, func(u accounts.User) bool { return u.Name == "sam" })
	if index < 0 {
		t.Fatal("sam not found")
	}
	sessions.sessions = []jellyfin.LiveSession{playingSession(users[index], shown, time.Now())}
	if all := readSessions(t, member); len(all) != 1 || all[0].Item == nil || !all[0].Item.Linked {
		t.Errorf("a member's playback: %+v", all)
	}
	if !slices.Equal(reached.asked, []string{"sam"}) {
		t.Errorf("titles found for %v, want the member", reached.asked)
	}
}

// Without the web player, no title of Now playing links, and none is
// looked up.
func TestNowPlayingLinksNoTitleWithoutTheWebPlayer(t *testing.T) {
	t.Parallel()
	shown := randomID()
	reached := &reachedItems{ids: []accounts.ID{shown}}
	sessions := &fakeSessions{}
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Sessions, o.ActivityTitles, o.WebClient = sessions, reached, false
	})
	admin := api.signedIn("root", true)
	sessions.sessions = []jellyfin.LiveSession{playingSession(accounts.User{ID: randomID(), Name: "robin"}, shown, time.Now())}
	if all := readSessions(t, admin); len(all) != 1 || all[0].Item == nil || all[0].Item.Linked {
		t.Errorf("without the web player: %+v", all)
	}
	if len(reached.asked) != 0 {
		t.Errorf("titles found for %v without the web player", reached.asked)
	}
}
