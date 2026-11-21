package jellyfin

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// While a torrent's first analysis waits for its pieces, PlaybackInfo
// sends its status and spaces before its answer, as for a slow channel,
// so that Swiftfin, which gives up after 20 seconds without a byte, keeps
// waiting. The answer after the spaces is valid JSON and plays.
func TestASlowTorrentKeepsPlaybackInfoWaiting(t *testing.T) {
	t.Parallel()
	ffmpeg, tor := torrentMovie(t, "A Movie")
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	// The play waits for the start and a piece, 22 seconds in all, well
	// past the first space: a play that kept silent answers its refusal
	// first, whose first byte is not a space.
	p := newTorrentPlayWith(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), torrentWaits{start: 2 * time.Second, piece: 20 * time.Second},
		[]*torrenttest.Torrent{tor}, []string{tracker.URL()})
	p.setting(t, func(s *accounts.Settings) { s.AnalysisTimeout = accounts.MaxAnalysisTimeout })
	picked := p.details(t)
	// The peer tells the torrent's files, then holds every piece.
	seeder.Hold()
	var release sync.Once
	t.Cleanup(func() { release.Do(seeder.Release) })

	query := url.Values{"UserId": {p.user.ID.String()}, "StartTimeTicks": {"0"}, "IsPlayback": {"true"}, "AutoOpenLiveStream": {"true"},
		"MaxStreamingBitrate": {"120000000"}, "MediaSourceId": {picked}}
	payload, _ := json.Marshal(map[string]any{"DeviceProfile": deviceProfile(t, "swiftfin-native")})
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, p.url+"/Items/"+p.movie+"/PlaybackInfo?"+query.Encode(),
		strings.NewReader(string(payload)))
	request.Header.Set("Authorization", app("tv", p.token))
	request.Header.Set("Content-Type", "application/json")
	response, err := testClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	first, err := reader.ReadByte()
	if response.StatusCode != http.StatusOK || err != nil || first != ' ' {
		t.Fatalf("PlaybackInfo's first byte while the pieces are held: %d %q %v", response.StatusCode, first, err)
	}
	release.Do(seeder.Release)
	var answer playbackInfoResponse
	if err := json.NewDecoder(reader).Decode(&answer); err != nil {
		t.Fatalf("PlaybackInfo's answer after the spaces: %v", err)
	}
	if len(answer.MediaSources) == 0 || answer.PlaySessionId == "" {
		t.Errorf("PlaybackInfo's answer: %+v", answer)
	}
}

// An app on the Jellyfin SDK for Kotlin, whose request ends 30 seconds
// after it was sent, spaces or not, is answered RateLimitExceeded once 25
// seconds passed while a torrent version is still starting, rather than
// left to give up; the torrent goes on starting and its analysis on being
// read, and the app's next try plays it. The time limit is the test's to
// end (see Handler.appLimitEnd).
func TestAnAppWithATimeLimitIsToldToTryAgain(t *testing.T) {
	t.Parallel()
	ffmpeg, tor := torrentMovie(t, "A Movie")
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	p := newTorrentPlayWith(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), torrentWaits{start: 2 * time.Second, piece: 20 * time.Second},
		[]*torrenttest.Torrent{tor}, []string{tracker.URL()})
	p.setting(t, func(s *accounts.Settings) { s.AnalysisTimeout = accounts.MaxAnalysisTimeout })
	// The first play's time limit ends at once, the next one's never.
	var mu sync.Mutex
	var limits []time.Duration
	p.handler.appLimitEnd = func(left time.Duration) <-chan time.Time {
		mu.Lock()
		defer mu.Unlock()
		limits = append(limits, left)
		if len(limits) > 1 {
			return nil
		}
		ended := make(chan time.Time)
		close(ended)
		return ended
	}
	picked := p.details(t)
	seeder.Hold()
	var release sync.Once
	t.Cleanup(func() { release.Do(seeder.Release) })

	play := func() (playbackInfoResponse, string) {
		t.Helper()
		query := url.Values{"UserId": {p.user.ID.String()}, "StartTimeTicks": {"0"}, "IsPlayback": {"true"}, "AutoOpenLiveStream": {"true"},
			"MaxStreamingBitrate": {"120000000"}, "MediaSourceId": {picked}}
		payload, _ := json.Marshal(map[string]any{"DeviceProfile": deviceProfile(t, "androidtv")})
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, p.url+"/Items/"+p.movie+"/PlaybackInfo?"+query.Encode(),
			strings.NewReader(string(payload)))
		request.Header.Set("Authorization", app("tv", p.token))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("User-Agent", "Jellyfin for Android TV/0.19.4 via jellyfin-sdk-kotlin (OkHttp/4.12.0)")
		response, err := testClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		var answer playbackInfoResponse
		var failed noCompatibleStream
		if response.StatusCode != http.StatusOK || err != nil || json.Unmarshal(body, &answer) != nil || json.Unmarshal(body, &failed) != nil {
			t.Fatalf("PlaybackInfo: %d %q %v", response.StatusCode, body, err)
		}
		return answer, failed.ErrorCode
	}
	if _, code := play(); code != "RateLimitExceeded" {
		t.Fatalf("while the torrent starts: %q, want RateLimitExceeded", code)
	}
	mu.Lock()
	first := slices.Clone(limits)
	mu.Unlock()
	if len(first) != 1 || first[0] <= 0 || first[0] > appWaitLimit {
		t.Fatalf("the play waited for the torrent within %v, want once within %v", first, appWaitLimit)
	}
	release.Do(seeder.Release)
	if answer, code := play(); code != "" || len(answer.MediaSources) == 0 || answer.PlaySessionId == "" {
		t.Errorf("the next try: %q %+v", code, answer)
	}
}
