package jellyfin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/torrent"
	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// hlsOnlyProfile plays nothing as it is: every version is repackaged or
// converted over HLS, which needs its keyframe index.
const hlsOnlyProfile = `{"MaxStreamingBitrate":120000000,"DirectPlayProfiles":[],
	"TranscodingProfiles":[{"Container":"ts","Type":"Video","VideoCodec":"h264","AudioCodec":"aac","Protocol":"hls","Context":"Streaming"}]}`

// slowTorrent is a 20-second H.264 movie in Matroska, 2.5 MB, as a
// single-file torrent of 256 KiB pieces, the size of an episode's sample.
func slowTorrent(t *testing.T) (string, *torrenttest.Torrent) {
	t.Helper()
	ffmpeg, file := vodFile(t, "a-movie.mkv", "-s", "640x360", "-b:v", "1000k")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return ffmpeg, torrenttest.Build("A Movie", 256<<10, torrenttest.File{Name: "a-movie.mkv", Data: data})
}

// playHLS asks PlaybackInfo for the version picked, with a profile that
// plays it over HLS, and returns the answer's ErrorCode, empty when a
// version plays.
func (p torrentPlay) playHLS(t *testing.T, picked string) string {
	t.Helper()
	query := url.Values{"UserId": {p.user.ID.String()}, "IsPlayback": {"true"}, "MediaSourceId": {picked}}
	status, body := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo?"+query.Encode(), app("web", p.token),
		map[string]any{"DeviceProfile": json.RawMessage(hlsOnlyProfile)})
	var answer playbackAnswer
	if status != http.StatusOK || json.Unmarshal(body, &answer) != nil || answer.ErrorCode == "" && len(answer.MediaSources) == 0 {
		t.Fatalf("PlaybackInfo: %d %s", status, body)
	}
	return answer.ErrorCode
}

// playsWaiting counts the goroutines of plays still waiting for their
// versions.
func playsWaiting() int {
	var dump strings.Builder
	_ = pprof.Lookup("goroutine").WriteTo(&dump, 2)
	n := 0
	for _, goroutine := range strings.Split(dump.String(), "\n\n") {
		if strings.Contains(goroutine, "jellyfin.(*Handler).choose") {
			n++
		}
	}
	return n
}

// playHLSLeaving asks PlaybackInfo as playHLS does, from an app that
// gives up waiting for the answer after wait.
func (p torrentPlay) playHLSLeaving(t *testing.T, picked string, wait time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), wait)
	defer cancel()
	query := url.Values{"UserId": {p.user.ID.String()}, "IsPlayback": {"true"}, "MediaSourceId": {picked}}
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.url+"/Items/"+p.movie+"/PlaybackInfo?"+query.Encode(),
		strings.NewReader(`{"DeviceProfile":`+hlsOnlyProfile+`}`))
	request.Header.Set("Authorization", app("web", p.token))
	request.Header.Set("Content-Type", "application/json")
	if response, err := http.DefaultClient.Do(request); err == nil {
		response.Body.Close()
		t.Fatalf("PlaybackInfo answered %d before the app gave up", response.StatusCode)
	}
}

// A torrent read over a slow swarm, which a play stopped waiting for, goes
// on: once the swarm sends faster, the next play starts, rather than the
// read waiting for good and every play after it giving up.
func TestATorrentReadAPlayGaveUpOnGoesOnForTheNext(t *testing.T) {
	t.Parallel()
	ffmpeg, tor := slowTorrent(t)
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	p := newTorrentPlay(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), 0, []*torrenttest.Torrent{tor}, []string{tracker.URL()})
	picked := p.details(t)
	// One block of 16 KiB every 700 ms: the file takes a minute. The app
	// gives up waiting while the play's reads are under way.
	seeder.Delay(700 * time.Millisecond)
	p.playHLSLeaving(t, picked, 20*time.Second)
	seeder.Delay(0)
	// The first play's reads end meanwhile; each play waits for them as
	// long as the start and a piece's wait.
	for try := range 2 {
		if code := p.playHLS(t, picked); code == "" {
			return
		} else if try == 1 {
			t.Fatalf("once the swarm is fast, PlaybackInfo still answers %q", code)
		}
	}
}

// Plays that gave up waiting for a torrent version leave no goroutine
// behind them, waiting for its analysis: the analysis goes on alone.
//
// It stays sequential: it counts the goroutines of every play in the test
// binary, which the tests running in parallel would add to.
func TestPlaysThatGaveUpLeaveNoGoroutineBehind(t *testing.T) {
	ffmpeg, tor := slowTorrent(t)
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	p := newTorrentPlay(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), 2*time.Second, []*torrenttest.Torrent{tor}, []string{tracker.URL()})
	picked := p.details(t)
	// The peer tells the torrent's files, then holds every piece: the
	// analysis waits for a piece's wait, twice, past what a play waits.
	seeder.Hold()
	defer seeder.Release()
	answered := make(chan string, 2)
	for range 2 {
		go func() { answered <- p.playHLS(t, picked) }()
	}
	for range 2 {
		if code := <-answered; code != "RateLimitExceeded" {
			t.Fatalf("PlaybackInfo answered %q", code)
		}
	}
	// The analysis still waits for the held pieces for a while.
	for deadline := time.Now().Add(10 * time.Second); playsWaiting() > 0; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutines of the plays that gave up still wait", playsWaiting())
		}
	}
}

// A torrent whose peer holds its pieces is a torrent that did not start in
// time: the app is told the title cannot be played now, within the start
// and a piece's wait, and the next play tries the torrent again at once,
// which plays once the peer sends.
func TestATorrentWhosePeerHoldsItsPiecesPlaysOnceItSends(t *testing.T) {
	t.Parallel()
	ffmpeg, tor := torrentMovie(t, "A Movie")
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	// The engine's own start timeout: its source stalls before a play
	// stops waiting for it.
	p := newTorrentPlay(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), 0, []*torrenttest.Torrent{tor}, []string{tracker.URL()})
	picked := p.details(t)
	seeder.Hold()
	defer seeder.Release()
	started := time.Now()
	if _, code := p.play(t, picked); code != "RateLimitExceeded" {
		t.Fatalf("PlaybackInfo answered %q after %v", code, time.Since(started))
	}
	if wait := torrent.StartTimeout + torrent.PieceTimeout + 30*time.Second; time.Since(started) > wait {
		t.Fatalf("PlaybackInfo answered after %v", time.Since(started))
	}
	seeder.Release()
	if info, code := p.play(t, picked); code != "" || len(info.MediaSources) == 0 {
		t.Fatalf("once the peer sends, PlaybackInfo answered %q", code)
	}
}

// A torrent version over a slow swarm whose pieces keep coming plays at its
// first play, once its analysis and the read of its index are in, even
// past its start and a piece's wait: the play waits on while the torrent
// receives, rather than telling the app to try later.
func TestATorrentWhosePiecesKeepComingPlaysAtItsFirstPlay(t *testing.T) {
	t.Parallel()
	ffmpeg, tor := slowTorrent(t)
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	// A start of 2 seconds: the play's first wait ends 32 seconds in.
	p := newTorrentPlay(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), 2*time.Second, []*torrenttest.Torrent{tor}, []string{tracker.URL()})
	picked := p.details(t)
	// One block of 16 KiB every 400 ms, never stalling: the head and the
	// index of the file take about a minute, longer than that first wait.
	seeder.Delay(400 * time.Millisecond)
	if code := p.playHLS(t, picked); code != "" {
		t.Fatalf("over a slow swarm whose pieces keep coming, PlaybackInfo answered %q", code)
	}
}
