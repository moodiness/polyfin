package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// convert has a device play a title converted, as jellyfin-web does when
// Play is pressed: PlaybackInfo with the browser's profile, the first
// segments read, the playback reported. It returns the play session.
func (p torrentTitles) convert(t *testing.T, device, token, name string) string {
	t.Helper()
	query := url.Values{"StartTimeTicks": {"0"}, "IsPlayback": {"true"}, "AutoOpenLiveStream": {"true"}, "MaxStreamingBitrate": {"120000000"},
		"MediaSourceId": {p.versions[name].ID.String()}}
	status, body := p.call(http.MethodPost, "/Items/"+p.movies[name]+"/PlaybackInfo?"+query.Encode(), app(device, token),
		map[string]any{"DeviceProfile": deviceProfile(t, "jellyfin-web-chrome")})
	var info playbackInfoResponse
	if status != http.StatusOK || json.Unmarshal(body, &info) != nil || len(info.MediaSources) != 1 || info.MediaSources[0].TranscodingUrl == "" {
		t.Fatalf("PlaybackInfo of %s: %d %s", name, status, body)
	}
	fetchSegments(t, p.testServer, info.MediaSources[0].TranscodingUrl, 0, 1)
	p.report(t, device, token, "/Sessions/Playing", name, map[string]any{"PlaySessionId": info.PlaySessionId, "PlayMethod": "Transcode"})
	return info.PlaySessionId
}

// converting reports whether a play session's remux or conversion runs.
func (p torrentTitles) converting(session string) bool {
	for _, running := range p.handler.Playback.Encodings() {
		if running.Key.Session == session {
			return true
		}
	}
	return false
}

// A device that moves on to another title without stopping the one it
// converted, as an app that sends no stop, ends that conversion, as the
// stop would have, and with it the torrent it read. A replay of the same
// title, and another device converting it, keep theirs.
func TestADeviceLeavingAConvertedTitleWithoutAStopEndsItsConversion(t *testing.T) {
	t.Parallel()
	ffmpeg, file := vodFile(t, "a-movie.mkv", "-c:v", "mpeg4")
	media, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	p := torrentTitlesOf(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), media)
	web, tv := p.signIn("member", "web"), p.signIn("member", "tv")
	left := p.convert(t, "web", web, "Copper Valley")
	other := p.convert(t, "tv", tv, "Copper Valley")

	// The same title again, as for another audio track: the app stops what
	// it leaves itself.
	p.report(t, "web", web, "/Sessions/Playing", "Copper Valley", map[string]any{"PlaySessionId": "replay", "PlayMethod": "Transcode"})
	if !p.converting(left) {
		t.Fatal("a replay of the same title stopped its conversion")
	}

	p.report(t, "web", web, "/Sessions/Playing", "Iron Coast", nil)
	if p.converting(left) {
		t.Fatal("the conversion of the title the device left without a stop runs on")
	}
	if !p.converting(other) || !p.running(t, "Copper Valley") {
		t.Fatal("another device's conversion of the title, or its torrent, stopped")
	}

	// Once the other device moves on too, nothing reads the torrent: it
	// stops.
	p.report(t, "tv", tv, "/Sessions/Playing", "Iron Coast", nil)
	if p.converting(other) {
		t.Fatal("the other device's conversion runs on once it left the title")
	}
	for deadline := time.Now().Add(time.Minute); p.running(t, "Copper Valley") || p.kept(t, "Copper Valley") != 0; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the torrent nobody reads any longer still runs")
		}
	}
}

// A report of a play session a device already left, arriving after it
// started another title, as a progress timer still running or a slow
// network sends it, stops nothing: the title the device plays is the one
// of its latest play session. Nor does the late progress of a title the
// device stopped before playing another.
func TestALateReportOfATitleTheDeviceLeftKeepsTheConversionItPlays(t *testing.T) {
	t.Parallel()
	ffmpeg, file := vodFile(t, "a-movie.mkv", "-c:v", "mpeg4")
	media, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	p := torrentTitlesOf(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), media)
	web := p.signIn("member", "web")
	first := p.convert(t, "web", web, "Copper Valley")
	second := p.convert(t, "web", web, "Iron Coast")
	if p.converting(first) {
		t.Fatal("the conversion of the title the device left runs on")
	}
	late := map[string]any{"PlaySessionId": first, "PlayMethod": "Transcode", "PositionTicks": 600_000_000}
	p.report(t, "web", web, "/Sessions/Playing/Progress", "Copper Valley", late)
	p.report(t, "web", web, "/Sessions/Playing", "Copper Valley", late)
	if !p.converting(second) {
		t.Fatal("a late report of the title the device left stopped the conversion it plays")
	}
	p.report(t, "web", web, "/Sessions/Playing/Progress", "Iron Coast", map[string]any{"PlaySessionId": second, "PlayMethod": "Transcode"})
	if !p.converting(second) {
		t.Fatal("a progress of the title the device plays stopped its conversion")
	}

	// A clean stop, another title, then a late progress of the one stopped.
	p.report(t, "web", web, "/Sessions/Playing/Stopped", "Iron Coast", map[string]any{"PlaySessionId": second})
	third := p.convert(t, "web", web, "Copper Valley")
	p.report(t, "web", web, "/Sessions/Playing/Progress", "Iron Coast", map[string]any{"PlaySessionId": second, "PlayMethod": "Transcode"})
	if !p.converting(third) {
		t.Fatal("a late progress of the title the device stopped stopped the conversion it plays")
	}

	// Moving on for real still ends it.
	p.convert(t, "web", web, "Iron Coast")
	if p.converting(third) {
		t.Fatal("the conversion of the title the device left runs on")
	}
}
