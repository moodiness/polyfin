package jellyfin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/subtitles"
	"github.com/moodiness/polyfin/internal/testspeech"
)

// After a version's first play, an addon's subtitle 4 seconds late comes
// to apps in time, named as synced, while one already in time comes as it
// is.
func TestAddonSubtitlesComeSyncedAfterAFirstPlay(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	t.Parallel()
	const length = 12 * time.Minute
	spoken := testspeech.Bursts(1, length)
	version := testspeech.Version(t, ffmpeg, spoken, length, false)
	late := subtitles.SubRip(testspeech.Cues(spoken, func(d time.Duration) time.Duration { return d + 4*time.Second }))
	inTime := subtitles.SubRip(testspeech.Cues(spoken, func(d time.Duration) time.Duration { return d }))
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		meta := stremio.Meta{ID: "tt1000", Type: "movie", Name: "Movie", Runtime: "12 min", Year: "2008"}
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "speaking", Name: "Speaking", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}, {Name: "subtitles"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{meta}})
		case path == "/meta/movie/tt1000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
		case path == "/stream/movie/tt1000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{{Name: "Speaking 1080p", URL: server.URL + "/files/version.mkv",
				BehaviorHints: stremio.StreamBehavior{Filename: "Movie.mkv", VideoSize: stremio.Number(len(version))}}}})
		case path == "/subtitles/movie/tt1000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"subtitles": []stremio.Subtitle{
				{ID: "late", URL: server.URL + "/files/late.srt", Lang: "eng"},
				{ID: "in-time", URL: server.URL + "/files/in-time.srt", Lang: "fre"},
			}})
		case path == "/files/version.mkv":
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader(string(version)))
		case path == "/files/late.srt":
			_, _ = w.Write(late)
		case path == "/files/in-time.srt":
			_, _ = w.Write(inTime)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	s := newProbingServer(t, 10, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"))
	user := s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), server.URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	var page QueryResult
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	movie := page.Items[0].Id
	p := playbackSetup{testServer: s, token: token, user: user, movie: movie}
	// The app plays the version with the late subtitle chosen.
	info := func(mediaSource string) MediaSourceInfo {
		t.Helper()
		body := map[string]any{"UserId": user.ID.String(), "DeviceProfile": p.profile(t, "jellyfin-web-chrome"), "SubtitleStreamIndex": 0}
		if mediaSource != "" {
			body["MediaSourceId"] = mediaSource
		}
		status, data := p.call(http.MethodPost, "/Items/"+movie+"/PlaybackInfo", app("tv", token), body)
		var response playbackInfoResponse
		if err := json.Unmarshal(data, &response); status != http.StatusOK || err != nil || len(response.MediaSources) != 1 {
			t.Fatalf("PlaybackInfo: %d %s", status, data)
		}
		return response.MediaSources[0]
	}
	first := info("")
	if status, data := p.call(http.MethodPost, "/Sessions/Playing", app("tv", token),
		map[string]any{"ItemId": movie, "MediaSourceId": first.Id, "PositionTicks": 0, "PlayMethod": "DirectPlay"}); status != http.StatusNoContent {
		t.Fatalf("report: %d %s", status, data)
	}
	versionID, _ := accounts.ParseID(first.ETag)
	for deadline := time.Now().Add(time.Minute); ; time.Sleep(100 * time.Millisecond) {
		if _, found := s.handler.Playback.Speech(t.Context(), versionID); found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the version's speech was not found")
		}
	}
	source := info(first.Id)
	for i, want := range []struct {
		name   string
		synced bool
	}{{"English - SUBRIP - External (synced)", true}, {"French - SUBRIP - External", false}} {
		stream := source.MediaStreams[i]
		if stream.DisplayTitle != want.name || stream.DeliveryMethod != "External" {
			t.Errorf("subtitle %d: %q delivered %s", i, stream.DisplayTitle, stream.DeliveryMethod)
			continue
		}
		response, err := http.Get(s.url + stream.DeliveryUrl)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		cues, err := subtitles.Parse(body)
		if err != nil || len(cues) != len(spoken) {
			t.Fatalf("subtitle %d: %d cues, %v", i, len(cues), err)
		}
		for j, cue := range cues {
			if gap := cue.Start - spoken[j].Start; gap.Abs() > 200*time.Millisecond || !want.synced && gap != 0 {
				t.Errorf("subtitle %d: cue %d starts at %s, the speech at %s", i, j, cue.Start, spoken[j].Start)
				break
			}
		}
	}
}
