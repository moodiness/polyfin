package admin

import (
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/activity"
	"github.com/moodiness/polyfin/internal/plays"
)

// The statistics mark the app and device of a guest's playback through
// a share link, so the admin names them in its language, apart from an
// app that happens to bear the same name.
func TestStatisticsMarkTheGuestsAppAndDevice(t *testing.T) {
	t.Parallel()
	var history *plays.History
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		history = plays.NewHistory(deps.pool, o.Accounts.Settings, slog.New(slog.NewTextHandler(io.Discard, nil)))
		o.Plays = history
	})
	administrator := api.signedIn("admin", true)
	users, err := api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(-2 * time.Hour)
	for i, shared := range []bool{true, false} {
		history.Record(t.Context(), plays.Change{Kind: plays.Stopped, Playback: plays.Playback{User: users[0].ID, UserName: "admin",
			Title: plays.Title{Item: accounts.ID{byte(i + 1)}, Kind: plays.Movie, Name: "Movie"}, App: "Shared link", DeviceName: "Web browser",
			Started: start, LastReport: start.Add(time.Hour), Ended: start.Add(time.Hour), Played: time.Duration(i+1) * time.Hour, Shared: shared}})
	}
	var body struct {
		Apps    []statGroupJSON  `json:"apps"`
		Devices []statDeviceJSON `json:"devices"`
	}
	administrator.raw(http.MethodGet, "/statistics?period=7d", "", &body)
	wantApps := []statGroupJSON{{Name: "Shared link", Plays: 1, Played: 7200}, {Name: "Shared link", Shared: true, Plays: 1, Played: 3600}}
	wantDevices := []statDeviceJSON{{Name: "Web browser", App: "Shared link", Plays: 1, Played: 7200},
		{Name: "Web browser", App: "Shared link", Shared: true, Plays: 1, Played: 3600}}
	if len(body.Apps) != 2 || body.Apps[0] != wantApps[0] || body.Apps[1] != wantApps[1] {
		t.Errorf("apps: %+v, want %+v", body.Apps, wantApps)
	}
	if len(body.Devices) != 2 || body.Devices[0] != wantDevices[0] || body.Devices[1] != wantDevices[1] {
		t.Errorf("devices: %+v, want %+v", body.Devices, wantDevices)
	}
}

// In French, a guest's playback is the guest's, through the sharer's link.
func TestGuestPlaybackActivityInFrench(t *testing.T) {
	t.Parallel()
	var log *activity.Store
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) { log = o.Activity })
	administrator := api.signedIn("alex", true)
	settings := api.store.Settings()
	settings.Language = "fr"
	if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	users, err := api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	log.SharedPlayed(t.Context(), accounts.ShareLink{User: accounts.ShareLinkUser{ID: users[0].ID, Name: users[0].Name}, Item: accounts.ID{1}, Title: "Clair 9 (1999)"}, "192.0.2.1")
	var page struct {
		Items []activityEntryJSON `json:"items"`
	}
	administrator.raw(http.MethodGet, "/activity?type="+activity.SharedPlayback, "", &page)
	want := "Un invité a lu Clair 9 (1999) grâce au lien partagé par alex"
	if len(page.Items) != 1 || page.Items[0].Name != want {
		t.Errorf("entries: %+v, want %q", page.Items, want)
	}
}
