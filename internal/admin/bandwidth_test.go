package admin

import (
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/playback"
)

// Each play tells the bytes per second it sends and receives, and where
// its version is read from, null when that is not known; a play sent to
// read its source itself says so. The server's totals come apart.
func TestLiveSessionsTellTheirBandwidthAndSource(t *testing.T) {
	t.Parallel()
	sessions := &fakeSessions{sent: 2_500_000, received: 1_800_000}
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) { o.Sessions = sessions })
	admin := api.signedIn("root", true)
	torrent := jellyfin.LiveSession{
		User:    accounts.User{ID: randomID(), Name: "alice"},
		Device:  accounts.Device{ID: randomID(), DeviceInfo: accounts.DeviceInfo{DeviceName: "TV", Client: "Test App"}},
		Playing: playback.NowPlaying{PlayState: playback.PlayState{PlayMethod: "DirectPlay"}},
		Sent:    1_250_000, Received: 1_800_000,
		Origin: jellyfin.Origin{Kind: jellyfin.OriginTorrent, Name: "Movie Streams", Peers: 12},
	}
	redirected := jellyfin.LiveSession{
		User:       accounts.User{ID: randomID(), Name: "bob"},
		Device:     accounts.Device{ID: randomID(), DeviceInfo: accounts.DeviceInfo{DeviceName: "Phone", Client: "Other App"}},
		Playing:    playback.NowPlaying{PlayState: playback.PlayState{PlayMethod: "DirectPlay"}},
		Redirected: true,
	}
	sessions.sessions = []jellyfin.LiveSession{torrent, redirected}

	var listed []liveSessionJSON
	if status := admin.raw(http.MethodGet, "/sessions", "", &listed); status != http.StatusOK || len(listed) != 2 {
		t.Fatalf("sessions: %d %+v", status, listed)
	}
	first := listed[0]
	if first.Bandwidth != (bandwidthJSON{Sent: 1_250_000, Received: 1_800_000}) {
		t.Errorf("bandwidth: %+v", first.Bandwidth)
	}
	if first.Origin == nil || *first.Origin != (originJSON{Kind: "torrent", Name: "Movie Streams", Peers: 12}) {
		t.Errorf("origin: %+v", first.Origin)
	}
	var raw []map[string]any
	admin.raw(http.MethodGet, "/sessions", "", &raw)
	if second := raw[1]; second["origin"] != nil {
		t.Errorf("an unknown origin: %v", second["origin"])
	}
	if listed[1].Bandwidth != (bandwidthJSON{Redirected: true}) {
		t.Errorf("redirected: %+v", listed[1].Bandwidth)
	}

	var total totalBandwidthJSON
	if status := admin.raw(http.MethodGet, "/bandwidth", "", &total); status != http.StatusOK ||
		total != (totalBandwidthJSON{Sent: 2_500_000, Received: 1_800_000}) {
		t.Errorf("totals: %d %+v", status, total)
	}
}
