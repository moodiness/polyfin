package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// streamInfs are the variants a master playlist lists: their attributes,
// then their URI.
func streamInfs(playlist string) [][2]string {
	lines := strings.Split(strings.TrimSpace(playlist), "\n")
	var variants [][2]string
	for i, line := range lines {
		if attributes, ok := strings.CutPrefix(line, "#EXT-X-STREAM-INF:"); ok && i+1 < len(lines) {
			variants = append(variants, [2]string{attributes, lines[i+1]})
		}
	}
	return variants
}

// Several qualities when converting: off, a converted 1080p playback's
// master playlist lists one variant, as before. Always, it lists 1080p,
// 720p and 480p with their bitrates, the first first, each with its own
// media playlist and segments at its size. A copy and a 360p conversion
// list one. Outside the local network, judged through the known proxies,
// a local address gets one and an address on the internet three.
func TestSeveralQualitiesWhenConverting(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: Polyfin converts video with the encoders FFmpeg has")
	}
	p := playing(t)
	// The file is 64 × 64; its analysis says 1920 × 1080, which FFmpeg
	// scales as the analysis asks.
	analysis := p.remuxable(t)
	analysis.Streams[1].Width, analysis.Streams[1].Height = 1920, 1080
	data, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "UPDATE media_analyses SET analysis = $2 WHERE version_id = $1", p.versions[0].ID, data); err != nil {
		t.Fatal(err)
	}
	chrome := p.profile(t, "jellyfin-web-chrome")
	video := map[string]any{"MediaSourceId": p.movie, "EnableDirectPlay": false, "AllowVideoStreamCopy": false}
	copied := map[string]any{"MediaSourceId": p.movie, "EnableDirectPlay": false}
	target := func(more map[string]any) string {
		t.Helper()
		answer := p.ask(t, p.token, p.movie, chrome, more)
		if len(answer.MediaSources) != 1 || answer.MediaSources[0].TranscodingUrl == "" {
			t.Fatalf("PlaybackInfo: %+v", answer)
		}
		return answer.MediaSources[0].TranscodingUrl
	}
	master := func(more map[string]any) (string, string) {
		t.Helper()
		target := target(more)
		_, _, playlist := fetchText(t, p.url+target)
		return target, playlist
	}

	if _, playlist := master(video); len(streamInfs(playlist)) != 1 || strings.Contains(playlist, "INDEPENDENT") ||
		!strings.Contains(playlist, "BANDWIDTH=15128000,AVERAGE-BANDWIDTH=10128000,") || !strings.Contains(playlist, "RESOLUTION=1920x1080") {
		t.Errorf("off:\n%s", playlist)
	}

	p.setting(t, func(settings *accounts.Settings) { settings.AdaptiveQuality = accounts.AdaptiveQualityAlways })
	converted, playlist := master(video)
	variants := streamInfs(playlist)
	if len(variants) != 3 || !strings.Contains(playlist, "#EXT-X-INDEPENDENT-SEGMENTS\n") {
		t.Fatalf("always:\n%s", playlist)
	}
	// Each peaks at one and a half times its bitrate, with the 128 kb/s of
	// the Opus track copied.
	for i, want := range []struct{ bandwidth, resolution, uri string }{
		{"BANDWIDTH=15128000,AVERAGE-BANDWIDTH=10128000,", "RESOLUTION=1920x1080", "main.m3u8?"},
		{"BANDWIDTH=7628000,AVERAGE-BANDWIDTH=5128000,", "RESOLUTION=1280x720", "hls1/main1/main.m3u8?"},
		{"BANDWIDTH=3128000,AVERAGE-BANDWIDTH=2128000,", "RESOLUTION=852x480", "hls1/main2/main.m3u8?"},
	} {
		if v := variants[i]; !strings.HasPrefix(v[0], want.bandwidth) || !strings.Contains(v[0], want.resolution) ||
			!strings.Contains(v[0], `CODECS="avc1.`) || !strings.HasPrefix(v[1], want.uri+"DeviceId=") {
			t.Errorf("variant %d: %v", i, v)
		}
	}
	parsed, _ := url.Parse(converted)
	if heights, _ := p.handler.qualities.Get(parsed.Query().Get("PlaySessionId")); !slices.Equal(heights, []int{1080, 720, 480}) {
		t.Errorf("qualities for the dashboard: %v", heights)
	}
	// The lowest variant's media playlist lists its own segments, made at
	// its size; there is no fourth.
	base, query, _ := strings.Cut(p.url+converted, "master.m3u8?")
	status, _, media := fetchText(t, base+"hls1/main2/main.m3u8?"+query)
	if status != http.StatusOK || !strings.Contains(media, `#EXT-X-MAP:URI="-1.mp4?`) || !strings.Contains(media, "\n0.mp4?") {
		t.Errorf("the lowest variant's playlist: %d\n%s", status, media)
	}
	if status, _, _ := fetchText(t, base+"hls1/main3/main.m3u8?"+query); status != http.StatusNotFound {
		t.Errorf("a fourth variant: %d", status)
	}
	if status, _, _ := fetchText(t, base+"hls1/main3/0.mp4?"+query); status != http.StatusNotFound {
		t.Errorf("a fourth variant's segment: %d", status)
	}
	status, _, init := fetchText(t, base+"hls1/main2/-1.mp4?"+query)
	_, _, first := fetchText(t, base+"hls1/main2/0.mp4?"+query)
	path := filepath.Join(t.TempDir(), "0.mp4")
	if err := os.WriteFile(path, []byte(init+first), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := exec.Command(filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), "-v", "error", "-select_streams", "v",
		"-show_entries", "stream=width,height", "-of", "csv=p=0", path).Output()
	if got := strings.TrimSpace(string(out)); status != http.StatusOK || got != "852,480" {
		t.Errorf("the lowest variant's segment: %d, video %q", status, got)
	}

	// A copy keeps one variant.
	if _, playlist := master(copied); len(streamInfs(playlist)) != 1 {
		t.Errorf("a copy:\n%s", playlist)
	}

	// Outside the local network: the server's machine gets one variant; an
	// app on the internet, behind a known proxy, three.
	p.setting(t, func(settings *accounts.Settings) { settings.AdaptiveQuality = accounts.AdaptiveQualityRemote })
	if _, playlist := master(video); len(streamInfs(playlist)) != 1 {
		t.Errorf("outside the local network, from the server's machine:\n%s", playlist)
	}
	knowProxies(t, p.testServer, "127.0.0.0/8")
	call := behindProxy(t, p.testServer)
	remote := target(video)
	for client, want := range map[string]int{"203.0.113.7": 3, "192.168.1.20": 1} {
		if status, playlist := call(client, http.MethodGet, remote, "", nil); status != http.StatusOK || len(streamInfs(string(playlist))) != want {
			t.Errorf("outside the local network, from %s: %d\n%s", client, status, playlist)
		}
	}

	// A 360p conversion has nothing lower to offer.
	p.setting(t, func(settings *accounts.Settings) { settings.AdaptiveQuality = accounts.AdaptiveQualityAlways })
	p.limit(t, p.user, func(c *accounts.UserChanges) { c.MaxBitrate = new(600_000) })
	if _, playlist := master(video); len(streamInfs(playlist)) != 1 || !strings.Contains(playlist, "RESOLUTION=640x360") {
		t.Errorf("360p:\n%s", playlist)
	}
}
