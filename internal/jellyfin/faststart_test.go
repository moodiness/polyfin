package jellyfin

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// fastClip makes 20 seconds of a moving picture, 1280 × 960 at 24 frames
// a second, with a keyframe every 10 seconds, about 2 Mb/s, and stereo AAC,
// in MP4 with its index first: a remote file whose first segment is long.
func fastClip(t *testing.T, ffmpeg string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clip.mp4")
	out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=1280x960:rate=24:duration=20",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=20", "-ac", "2",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-g", "240", "-keyint_min", "240", "-sc_threshold", "0",
		"-b:v", "2M", "-c:a", "aac", "-b:a", "96k", "-movflags", "+faststart", "-shortest", path).CombinedOutput()
	if err != nil {
		t.Fatalf("making a clip: %v: %s", err, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// fastServer serves the clip from a remote host, as an addon lists it,
// with Start remote plays faster on.
func fastServer(t *testing.T) (failoverSetup, string) {
	t.Helper()
	ffmpeg, ffprobe := failoverFFmpeg(t)
	clip := fastClip(t, ffmpeg)
	host, _ := steadyHost(t, clip)
	p := failoverServer(t, ffprobe, failoverAddon(t, "catalog", []stremio.Stream{{Name: "Long", URL: host.URL + "/clip.mp4",
		BehaviorHints: stremio.StreamBehavior{Filename: "Fast.2026.1080p.mp4", VideoSize: stremio.Number(len(clip))}}}))
	p.setting(t, func(settings *accounts.Settings) { settings.FastStart = true })
	return p, ffmpeg
}

// sizeOf is the size of the video of a segment, its initialization
// segment first when given, as ffprobe reads it, and checks that it
// decodes.
func sizeOf(t *testing.T, ffmpeg string, parts ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "segment")
	if err := os.WriteFile(path, []byte(strings.Join(parts, "")), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(ffmpeg, "-v", "error", "-i", path, "-f", "null", "-").CombinedOutput(); err != nil || len(strings.TrimSpace(string(out))) > 0 {
		t.Errorf("decoding: %v: %s", err, out)
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	out, err := exec.Command(ffprobe, "-v", "error", "-select_streams", "v", "-show_entries", "stream=width,height", "-of", "csv=p=0:s=x", path).Output()
	if err != nil {
		t.Fatalf("probing a segment: %v", err)
	}
	first, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return first
}

// A remote file repackaged into MPEG-TS segments, as Android TV asks for,
// starts with 2-second segments converted at 720p, up to its keyframe 10
// seconds in, from which its video is copied, after one discontinuity:
// the segments decode across it.
func TestARepackagedRemotePlayStartsWithFastSegments(t *testing.T) {
	t.Parallel()
	p, ffmpeg := fastServer(t)
	answer := p.ask(t, p.token, p.movie, recordedProfile(t, "androidtv"), map[string]any{"MediaSourceId": p.movie, "EnableDirectPlay": false})
	if len(answer.MediaSources) != 1 || !strings.HasSuffix(answer.MediaSources[0].TranscodingUrl, "&FastStartTicks=0") {
		t.Fatalf("PlaybackInfo: %+v", answer)
	}
	base, query, _ := strings.Cut(p.url+answer.MediaSources[0].TranscodingUrl, "master.m3u8?")
	_, _, playlist := fetchText(t, base+"main.m3u8?"+query)
	uris, breaks := segmentsOf(playlist)
	if len(uris) != 6 || !slices.Equal(breaks, []int{5}) || !strings.Contains(playlist, "#EXTINF:2.000000,") {
		t.Fatalf("playlist:\n%s", playlist)
	}
	segments := make([]string, len(uris))
	for n := range uris {
		segments[n] = fetchSegment(t, base+"hls1/main/"+strconv.Itoa(n)+".ts?"+query)
		want := "960x720"
		if n == 5 {
			want = "1280x960"
		}
		if got := sizeOf(t, ffmpeg, segments[n]); got != want {
			t.Errorf("segment %d: %s, want %s", n, got, want)
		}
	}
	if got := sizeOf(t, ffmpeg, segments[4], segments[5]); got != "960x720" {
		t.Errorf("across the switch: %s", got)
	}
}

// A converted play in fragmented MP4 segments that jellyfin-web resumes
// 12 seconds in gets the fast segments there, each kind of segment with
// its own initialization segment, the playlist marking where they start;
// another app, whose player may keep the first initialization segment,
// gets none in fragmented MP4.
func TestAResumedConvertedPlayStartsWithFastSegmentsInJellyfinWeb(t *testing.T) {
	t.Parallel()
	p, ffmpeg := fastServer(t)
	header := `MediaBrowser Client="Jellyfin Web", Device="Firefox", DeviceId="web-id", Version="10.11.0"`
	status, body := p.call(http.MethodPost, "/Users/AuthenticateByName", header, map[string]string{"Username": "member", "Pw": "correct horse"})
	var signedIn AuthenticationResult
	if status != http.StatusOK || json.Unmarshal(body, &signedIn) != nil {
		t.Fatalf("sign-in: %d %s", status, body)
	}
	header += `, Token="` + signedIn.AccessToken + `"`
	request := map[string]any{"MaxStreamingBitrate": 120_000_000, "DeviceProfile": recordedProfile(t, "jellyfin-web-chrome"),
		"MediaSourceId": p.movie, "EnableDirectPlay": false, "AllowVideoStreamCopy": false, "StartTimeTicks": 120_000_000}
	status, body = p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", header, request)
	var answer playbackAnswer
	if status != http.StatusOK || json.Unmarshal(body, &answer) != nil || len(answer.MediaSources) != 1 ||
		!strings.HasSuffix(answer.MediaSources[0].TranscodingUrl, "&FastStartTicks=120000000") {
		t.Fatalf("PlaybackInfo: %d %s", status, body)
	}
	base, query, _ := strings.Cut(p.url+answer.MediaSources[0].TranscodingUrl, "master.m3u8?")
	_, _, playlist := fetchText(t, base+"main.m3u8?"+query)
	uris, breaks := segmentsOf(playlist)
	if len(uris) != 6 || !slices.Equal(breaks, []int{1}) || strings.Count(playlist, "#EXT-X-MAP:") != 2 ||
		!strings.Contains(playlist, "#EXT-X-DISCONTINUITY\n#EXT-X-MAP:URI=\"hls1/main/-2.mp4?") {
		t.Fatalf("playlist:\n%s", playlist)
	}
	fastInit := fetchSegment(t, base+"hls1/main/-2.mp4?"+query)
	for n := 1; n < len(uris); n++ {
		if got := sizeOf(t, ffmpeg, fastInit, fetchSegment(t, base+"hls1/main/"+strconv.Itoa(n)+".mp4?"+query)); got != "960x720" {
			t.Errorf("segment %d: %s", n, got)
		}
	}
	init := fetchSegment(t, base+"hls1/main/-1.mp4?"+query)
	if got := sizeOf(t, ffmpeg, init, fetchSegment(t, base+"hls1/main/0.mp4?"+query)); got != "1280x960" {
		t.Errorf("segment 0: %s", got)
	}

	other := p.ask(t, p.token, p.movie, recordedProfile(t, "jellyfin-web-chrome"),
		map[string]any{"MediaSourceId": p.movie, "EnableDirectPlay": false, "AllowVideoStreamCopy": false})
	if len(other.MediaSources) != 1 || strings.Contains(other.MediaSources[0].TranscodingUrl, "FastStartTicks") {
		t.Errorf("another app in fragmented MP4: %+v", other.MediaSources)
	}
}
