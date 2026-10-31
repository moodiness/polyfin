package jellyfin

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// starting asks how the start of the play of the device token signed in
// goes, as the web client asks it.
func (s testServer) starting(t *testing.T, token string) startProgressJSON {
	t.Helper()
	status, body := s.call(http.MethodGet, "/Polyfin/Playback/Starting", app("web", token), nil)
	var progress startProgressJSON
	if status != http.StatusOK || json.Unmarshal(body, &progress) != nil {
		t.Fatalf("/Polyfin/Playback/Starting: %d %s", status, body)
	}
	return progress
}

// While a Play waits for torrents, its device is told how their start goes:
// the most advanced of them, here the one whose peer sent its details then
// holds its pieces, while the other found no peer and did not start. Once
// the play answers, the torrent it chose is still told, as the player reads
// its first segments. Another user, signed in on the same kind of device,
// is told nothing of it.
func TestAPlayTellsItsDeviceHowItsTorrentsStart(t *testing.T) {
	t.Parallel()
	ffmpeg, live := torrentMovie(t, "A Movie")
	seeder := torrenttest.Seed(t, live, "127.0.0.1")
	dead := torrenttest.Build("Another Movie", 64<<10, torrenttest.File{Name: "another.mkv", Data: torrenttest.Bytes(300 << 10)})
	trackers := []string{torrenttest.HTTPTracker(t, "127.0.0.1").URL(), torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address()).URL()}
	p := newTorrentPlay(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), 2*time.Second, []*torrenttest.Torrent{dead, live}, trackers)
	p.testServer.user("other", func(c *accounts.UserChanges) { c.TorrentStreams = new(true) })
	other := p.signIn("other", "web")
	if got := p.starting(t, p.token); got != (startProgressJSON{}) {
		t.Fatalf("before any play: %+v", got)
	}

	seeder.Hold()
	// Play, as jellyfin-web sends it, under the title's own identifier.
	query := "?UserId=" + p.user.ID.String() + "&StartTimeTicks=0&IsPlayback=true&AutoOpenLiveStream=true&MaxStreamingBitrate=120000000"
	profile := map[string]any{"DeviceProfile": deviceProfile(t, "jellyfin-web-chrome")}
	picked := p.details(t)
	answered := make(chan playbackInfoResponse, 1)
	go func() {
		var info playbackInfoResponse
		_, body := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo"+query+"&MediaSourceId="+picked, app("web", p.token), profile)
		_ = json.Unmarshal(body, &info)
		answered <- info
	}()
	var progress startProgressJSON
	eventually(t, "the start told while the play waits", func() bool {
		progress = p.starting(t, p.token)
		return progress.Stage == "pieces" && progress.PeersConnected == 1 && progress.Waiting > 0
	})
	if progress.PeersFound != 1 || progress.Failure != "" {
		t.Errorf("while the pieces are held: %+v", progress)
	}
	eventually(t, "the other torrent not to start", func() bool { return p.handler.Playback.Unstarted(p.versions[0].ID) })
	if got := p.starting(t, p.token); got.Stage != "pieces" {
		t.Errorf("once the other torrent did not start: %+v", got)
	}
	if got := p.starting(t, other); got != (startProgressJSON{}) {
		t.Errorf("another user is told %+v", got)
	}

	seeder.Release()
	p.converted(t, <-answered, p.versions[1].ID)
	if got := p.starting(t, p.token); got.Stage != "pieces" || got.Received == 0 {
		t.Errorf("once the play answered: %+v", got)
	}
}

// A play of a link tells nothing: no torrent starts.
func TestAPlayOfALinkTellsNoStart(t *testing.T) {
	t.Parallel()
	p := playing(t)
	if answer := p.ask(t, p.token, p.movie, p.profile(t, "jellyfin-web-chrome"), nil); len(answer.MediaSources) == 0 {
		t.Fatalf("the link did not play: %+v", answer)
	}
	status, body := p.call(http.MethodGet, "/Polyfin/Playback/Starting", app("tv", p.token), nil)
	var progress startProgressJSON
	if status != http.StatusOK || json.Unmarshal(body, &progress) != nil || progress != (startProgressJSON{}) {
		t.Errorf("after a link's play: %d %s", status, body)
	}
}

// When every torrent a play tries did not start, and none runs, the start
// is told as failed, with why the first did not.
func TestAStartWhoseTorrentsAllFailedIsToldAsFailed(t *testing.T) {
	t.Parallel()
	h := newTestServer(t, 10).handler
	s := &playStart{failed: map[accounts.ID]string{}}
	for i, why := range []string{"no_trackers", "silent"} {
		version := library.Version{ID: accounts.ID{byte(i + 1)}, Torrent: &library.Torrent{InfoHash: "0123456789abcdef0123456789abcdef0123456" + string(rune('0'+i))}}
		s.trying(version)
		s.failed[version.ID] = why
	}
	if got := h.progress(s); got != (startProgressJSON{Stage: "failed", Failure: "no_trackers"}) {
		t.Errorf("progress: %+v", got)
	}
}
