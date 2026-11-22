package jellyfin

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// With the pause buffer on, an app whose player keeps to an HLS live
// window gets a channel it would play as it is over HLS, its video copied,
// so that the channel's buffer serves it. Other apps, and every app once
// the buffer is off, play it as it is.
func TestWindowAppsGetChannelsOverHLSWithTheBuffer(t *testing.T) {
	t.Parallel()
	s, token, user := tuned(t, newTVAddon(t, false, ""))
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	channel := channels.Items[0].Id
	liveAnalysis(t, s, user, channel)
	info := func(client, profile string) MediaSourceInfo {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", profile+".json"))
		if err != nil {
			t.Fatal(err)
		}
		// The app is the one the device signed in with.
		device := strings.ReplaceAll(strings.ToLower(client), " ", "-")
		as := func(token string) string {
			return strings.Replace(app(device, token), `Client="Test App"`, `Client="`+client+`"`, 1)
		}
		status, body := s.call(http.MethodPost, "/Users/AuthenticateByName", as(""), map[string]string{"Username": "member", "Pw": "correct horse"})
		var signedIn AuthenticationResult
		if status != http.StatusOK || json.Unmarshal(body, &signedIn) != nil {
			t.Fatalf("sign-in with %s: %d %s", client, status, body)
		}
		status, body = s.call(http.MethodPost, "/Items/"+channel+"/PlaybackInfo", as(signedIn.AccessToken),
			map[string]any{"DeviceProfile": json.RawMessage(data), "IsPlayback": true, "AutoOpenLiveStream": true})
		var answer playbackInfoResponse
		if status != http.StatusOK || json.Unmarshal(body, &answer) != nil || len(answer.MediaSources) != 1 {
			t.Fatalf("PlaybackInfo of a channel for %s: %d %s", client, status, body)
		}
		return answer.MediaSources[0]
	}
	repackaged := func(source MediaSourceInfo) bool {
		return !source.SupportsDirectPlay && source.TranscodingSubProtocol == "hls" && strings.Contains(source.TranscodingUrl, "master.m3u8") &&
			!strings.Contains(source.TranscodingUrl, "allowVideoStreamCopy=false")
	}

	s.setting(t, func(settings *accounts.Settings) { settings.LiveTvBufferMinutes = 30 })
	for _, app := range []struct{ client, profile string }{
		{"Jellyfin Web", "jellyfin-web-chrome"},
		{"Android TV", "androidtv"},
		{"Jellyfin Android", "jellyfin-android-integrated"},
	} {
		if source := info(app.client, app.profile); !repackaged(source) {
			t.Errorf("%s with the buffer on: %+v", app.client, source)
		}
	}
	if source := info("Test App", "jellyfin-web-chrome"); !source.SupportsDirectPlay {
		t.Errorf("another app with the buffer on: %+v", source)
	}

	s.setting(t, func(settings *accounts.Settings) { settings.LiveTvBufferMinutes = 0 })
	if source := info("Jellyfin Web", "jellyfin-web-chrome"); !source.SupportsDirectPlay {
		t.Errorf("jellyfin-web with the buffer off: %+v", source)
	}
}
