package jellyfin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A .strm file of a movies folder is a movie with one media source, the
// address it names, played as an addon's link is: PlaybackInfo describes
// the video the address serves, analyzed through Polyfin, and the stream
// route relays its bytes to a member, whom Send apps to the source keeps
// from the server's sources; an app that may not play it as it is gets it
// over HLS. No answer names the address, whose key the source alone sees,
// and the scan never asks it.
func TestStrmFilesPlayThroughTheRelay(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := discFFmpeg(t, "-encoders", "libx264")
	made := containerFile(t, ffmpeg, "Movie (2008).mp4", "-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "-c:a", "aac",
		"-movflags", "+faststart")
	data, err := os.ReadFile(made)
	if err != nil {
		t.Fatal(err)
	}
	var keyed, unkeyed atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/videos/movie.mp4" || r.URL.Query().Get("token") != "secret-key" {
			unkeyed.Add(1)
			http.Error(w, "no key", http.StatusForbidden)
			return
		}
		keyed.Add(1)
		http.ServeContent(w, r, "movie.mp4", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(origin.Close)
	strm := filepath.Join(t.TempDir(), "Movie (2008).strm")
	if err := os.WriteFile(strm, []byte("#KODIPROP:inputstream=inputstream.adaptive\n"+origin.URL+"/videos/movie.mp4?token=secret-key\n"),
		0o644); err != nil {
		t.Fatal(err)
	}
	p, item, local := localContainerFile(t, ffprobe, strm)
	if keyed.Load() != 0 {
		t.Errorf("the scan asked the source %d times", keyed.Load())
	}

	status, answer := p.call(http.MethodPost, "/Items/"+item+"/PlaybackInfo", app("tv", p.token), map[string]any{
		"UserId": p.user.ID.String(), "MediaSourceId": local.Id, "DeviceProfile": p.profile(t, "jellyfin-web-12.4-chrome")})
	var info playbackInfoResponse
	if err := json.Unmarshal(answer, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, answer)
	}
	source := info.MediaSources[0]
	if source.Id != local.Id || source.Container != "mp4" || source.RunTimeTicks == nil ||
		(time.Duration(*source.RunTimeTicks)*100-containerSeconds*time.Second).Abs() > 200*time.Millisecond ||
		!slices.ContainsFunc(tracks(source), func(track string) bool { return strings.HasPrefix(track, "Video h264 ") }) ||
		!source.SupportsDirectPlay && !source.SupportsDirectStream {
		t.Fatalf("the address's source: %s, runtime %v, tracks %q, direct play %v", source.Container, source.RunTimeTicks, tracks(source),
			source.SupportsDirectPlay)
	}
	if keyed.Load() == 0 || unkeyed.Load() != 0 {
		t.Errorf("the analysis asked the source %d times with its key, %d without", keyed.Load(), unkeyed.Load())
	}
	if strings.Contains(string(answer), "secret-key") || strings.Contains(string(answer), origin.URL) {
		t.Errorf("PlaybackInfo names the address: %s", answer)
	}

	response, body := fetchURL(t, p.url+"/Videos/"+local.Id+"/stream?static=true&ApiKey="+p.token, nil)
	if response.StatusCode != http.StatusOK || response.Request.URL.Host == strings.TrimPrefix(origin.URL, "http://") || body != string(data) {
		t.Errorf("the stream: %d from %s, %d bytes of %d", response.StatusCode, response.Request.URL.Host, len(body), len(data))
	}

	// An app that may not play it as it is gets it over HLS, read through
	// Polyfin too: the segments hold the address's video.
	status, answer = p.call(http.MethodPost, "/Items/"+item+"/PlaybackInfo", app("tv", p.token), map[string]any{
		"UserId": p.user.ID.String(), "MediaSourceId": local.Id, "DeviceProfile": p.profile(t, "jellyfin-web-12.4-chrome"),
		"EnableDirectPlay": false, "EnableDirectStream": false})
	info = playbackInfoResponse{}
	if err := json.Unmarshal(answer, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 ||
		info.MediaSources[0].TranscodingUrl == "" {
		t.Fatalf("PlaybackInfo over HLS: %d %s", status, answer)
	}
	uris := segmentURIs(t, p, info.MediaSources[0].TranscodingUrl)
	var init []byte
	if strings.Contains(uris[0], "/-1.") {
		init = getSegment(t, p, uris[0])
	}
	if times, _ := frameTimes(t, ffprobe, decoded(t, ffmpeg, init, getSegment(t, p, uris[len(uris)-1]))); len(times) == 0 {
		t.Errorf("the last segment holds no frame")
	}
	if unkeyed.Load() != 0 {
		t.Errorf("the source was asked %d times without its key", unkeyed.Load())
	}
}
