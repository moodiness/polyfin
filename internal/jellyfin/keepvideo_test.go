package jellyfin

import (
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A track change in jellyfin-web's order: PlaybackInfo for the new track,
// DELETE of the old play session, then the new master playlist and its
// segments. With converted video kept, the old session's conversion runs
// on once it stopped, the new session plays it, joined segments decode,
// and the old session's second DELETE leaves the new one playing. Off, the
// DELETE stops the conversion, as before.
func TestTrackChangeInJellyfinWebsOrder(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: Polyfin converts video with the encoders FFmpeg has")
	}
	p := playing(t)
	p.remuxable(t)
	chrome := p.profile(t, "jellyfin-web-chrome")
	play := func() (string, string) {
		t.Helper()
		answer := p.ask(t, p.token, p.movie, chrome, map[string]any{"MediaSourceId": p.movie, "EnableDirectPlay": false, "AllowVideoStreamCopy": false})
		if len(answer.MediaSources) != 1 || !strings.Contains(answer.MediaSources[0].TranscodingUrl, "DeviceId=tv-id") {
			t.Fatalf("PlaybackInfo: %+v", answer)
		}
		return answer.PlaySessionId, p.url + answer.MediaSources[0].TranscodingUrl
	}
	// segment fetches the master and media playlists, then the
	// initialization segment and segment n, and checks they decode.
	segment := func(target string, n string) {
		t.Helper()
		if status, _, master := fetchText(t, target); status != http.StatusOK || !strings.Contains(master, "\nmain.m3u8?") {
			t.Fatalf("master playlist: %d %s", status, master)
		}
		base, query, _ := strings.Cut(target, "master.m3u8?")
		_, _, init := fetchText(t, base+"hls1/main/-1.mp4?"+query)
		status, _, data := fetchText(t, base+"hls1/main/"+n+".mp4?"+query)
		path := filepath.Join(t.TempDir(), "segment.mp4")
		if err := os.WriteFile(path, []byte(init+data), 0o600); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), "-v", "error", "-show_entries", "stream=codec_type",
			"-of", "csv=p=0", path).Output()
		if streams := strings.Fields(string(out)); status != http.StatusOK || err != nil || len(streams) != 2 {
			t.Errorf("segment %s: %d, streams %v %v", n, status, streams, err)
		}
	}
	stop := func(session string) {
		t.Helper()
		if status, _ := p.call(http.MethodDelete, "/Videos/ActiveEncodings?deviceId=tv-id&playSessionId="+url.QueryEscape(session),
			app("tv", p.token), nil); status != http.StatusNoContent {
			t.Fatalf("DELETE: %d", status)
		}
	}
	sessions := func() []string {
		var listed []string
		for _, running := range p.handler.Playback.Encodings() {
			listed = append(listed, running.Key.Session)
		}
		return listed
	}

	p.setting(t, func(settings *accounts.Settings) { settings.KeepConvertedVideo = true })
	old, target := play()
	segment(target, "0")
	next, switched := play()
	stop(old)
	if got := sessions(); len(got) != 1 || got[0] != old {
		t.Errorf("once the old session stopped: %v", got)
	}
	segment(switched, "1")
	stop(old)
	segment(switched, "2")
	if got := sessions(); len(got) != 1 || got[0] != next {
		t.Errorf("after the switch: %v, want %s", got, next)
	}
	stop(next)

	p.setting(t, func(settings *accounts.Settings) { settings.KeepConvertedVideo = false })
	off, target := play()
	segment(target, "0")
	play()
	stop(off)
	if got := sessions(); len(got) != 1 || got[0] != next {
		t.Errorf("off, once the old session stopped: %v, want only the kept %s", got, next)
	}
}
