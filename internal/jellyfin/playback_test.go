package jellyfin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/stremio"
)

const playbackFixtures = "testdata/jellyfin-12.1/playback"

// streamingAddon serves two movies with two streams each, the first with a
// SubRip subtitle, and the bytes of the streams.
func streamingAddon(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	movie := func(id, name string) stremio.Meta {
		return stremio.Meta{ID: id, Type: "movie", Name: name, Runtime: "2h", Description: "A story.", Year: "2008",
			Released: "2008-04-10T00:00:00.000Z", Poster: server.URL + "/poster.jpg", Genres: []string{"Drama"}, ImdbRating: "7.5",
			Extras: &stremio.Extras{Certification: "PG"}}
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "streams", Name: "Streams", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}, {Name: "subtitles"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie("tt1000", "Movie"), movie("tt2000", "Remote")}})
		case path == "/meta/movie/tt1000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie("tt1000", "Movie")})
		case path == "/meta/movie/tt2000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie("tt2000", "Remote")})
		case strings.HasPrefix(path, "/stream/movie/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{
				{Name: "Source 2160p", Description: "REMUX\nHEVC", URL: server.URL + "/files/remux.mkv",
					BehaviorHints: stremio.StreamBehavior{Filename: "Movie.2160p.mkv", VideoSize: 80_000_000_000}},
				{Name: "Source 1080p", Description: "WEB-DL", URL: server.URL + "/files/web.mp4",
					BehaviorHints: stremio.StreamBehavior{Filename: "Movie.1080p.mp4", VideoSize: 3_000_000_000}},
				{Name: "Statistics", ExternalURL: "https://addon.example/"},
			}})
		case path == "/subtitles/movie/tt1000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"subtitles": []stremio.Subtitle{{ID: "fr-1", URL: server.URL + "/files/movie.fr.srt", Lang: "fre"}}})
		case strings.HasPrefix(path, "/subtitles/movie/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"subtitles": []stremio.Subtitle{}})
		case path == "/files/movie.fr.srt":
			_, _ = io.WriteString(w, "1\r\n00:00:01,000 --> 00:00:04,000\r\nBonjour\r\n")
		case strings.HasPrefix(path, "/files/"):
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("\x1a\x45\xdf\xa3 media bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

type playbackSetup struct {
	testServer
	token string
	user  accounts.User
	// movie has a subtitle file; remote has none.
	movie, remote string
	versions      []library.Version
}

// playing installs the streaming addon, signs a member in, and stores the
// analysis of the movie's second version, as ffprobe would have.
func playing(t *testing.T) playbackSetup {
	t.Helper()
	s := newTestServer(t, 10)
	user := s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), streamingAddon(t), false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	var page QueryResult
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	ids := map[string]string{}
	for _, item := range page.Items {
		ids[item.Name] = item.Id
	}
	p := playbackSetup{testServer: s, token: token, user: user, movie: ids["Movie"], remote: ids["Remote"]}
	p.versions = p.versionsOf(t, p.movie)
	p.analyzed(t, p.versions[1], "h264-aac-mp4")
	return p
}

func (p playbackSetup) versionsOf(t *testing.T, item string) []library.Version {
	t.Helper()
	id, _ := accounts.ParseID(item)
	versions, err := p.library.Versions(t.Context(), p.user, id)
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions of %s: %+v %v", item, versions, err)
	}
	return versions
}

// analyzed stores the analysis of a recorded clip for a version. Polyfin's
// ffprobe reads every source over HTTP: analyses are of remote sources.
func (p playbackSetup) analyzed(t *testing.T, version library.Version, clip string) {
	t.Helper()
	probe, err := os.ReadFile(filepath.Join(playbackFixtures, "probes", clip+".json"))
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := media.Parse(probe)
	if err != nil {
		t.Fatal(err)
	}
	analysis.Remote = true
	data, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", version.ID, data); err != nil {
		t.Fatal(err)
	}
}

func (p playbackSetup) profile(t *testing.T, name string) json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestItemDetailsListVersions(t *testing.T) {
	p := playing(t)
	var movie BaseItemDto
	if status := p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie); status != http.StatusOK {
		t.Fatalf("detail: %d", status)
	}
	if movie.MediaSources == nil || len(*movie.MediaSources) != 2 {
		t.Fatalf("media sources: %+v", movie.MediaSources)
	}
	sources := *movie.MediaSources
	first, second := sources[0], sources[1]
	// The first version is named after its item; the others resolve as
	// items too.
	if first.Id != p.movie || second.Id != p.versions[1].ID.String() || first.ETag == second.ETag {
		t.Errorf("ids %s %s, etags %s %s", first.Id, second.Id, first.ETag, second.ETag)
	}
	if first.Name != "Source 2160p · REMUX · HEVC" || first.Protocol != "Http" || !first.IsRemote || first.RequiredHttpHeaders == nil {
		t.Errorf("first source: %+v", first)
	}
	if !strings.HasPrefix(first.Path, p.url+"/Videos/"+p.movie+"/stream.mkv?") || !strings.Contains(first.Path, grantParameter+"=") {
		t.Errorf("path: %s", first.Path)
	}
	// Subtitle files come first; the analyzed version describes its tracks.
	if len(first.MediaStreams) != 1 || first.MediaStreams[0].Type != "Subtitle" || first.MediaStreams[0].Language != "fra" {
		t.Errorf("unanalyzed version streams: %+v", first.MediaStreams)
	}
	if len(second.MediaStreams) < 3 || second.MediaStreams[1].Type != "Video" || second.Container != "mp4" ||
		second.DefaultSubtitleStreamIndex == nil || *second.DefaultSubtitleStreamIndex != -1 {
		t.Errorf("analyzed version: container %s, subtitle %v, streams %+v", second.Container, second.DefaultSubtitleStreamIndex, second.MediaStreams)
	}
	// A version opens as an item, its own source first.
	var version BaseItemDto
	p.get(t, "/Items/"+second.Id, p.token, &version)
	if version.Id != second.Id || version.Name != "Movie" || (*version.MediaSources)[0].Id != second.Id {
		t.Errorf("version as an item: %s %s", version.Id, version.Name)
	}
	var titleAncestors, versionAncestors []BaseItemDto
	p.get(t, "/Items/"+p.movie+"/Ancestors", p.token, &titleAncestors)
	if status := p.get(t, "/Items/"+second.Id+"/Ancestors", p.token, &versionAncestors); status != http.StatusOK ||
		!slices.Equal(itemNames(versionAncestors), itemNames(titleAncestors)) {
		t.Errorf("ancestors of a version: %d %v, of its title %v", status, itemNames(versionAncestors), itemNames(titleAncestors))
	}
}

func TestPlaybackInfoDecidesForTheDevice(t *testing.T) {
	p := playing(t)
	ask := func(profile string, mediaSourceID string) playbackInfoResponse {
		t.Helper()
		body := map[string]any{"UserId": p.user.ID.String(), "MaxStreamingBitrate": 120_000_000, "DeviceProfile": p.profile(t, profile),
			"AudioStreamIndex": "2", "SubtitleStreamIndex": "0"}
		if mediaSourceID != "" {
			body["MediaSourceId"] = mediaSourceID
		}
		status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), body)
		var response playbackInfoResponse
		if err := json.Unmarshal(data, &response); status != http.StatusOK || err != nil {
			t.Fatalf("%s: %d %s", profile, status, data)
		}
		return response
	}
	web := ask("jellyfin-web-chrome", p.versions[1].ID.String())
	if len(web.MediaSources) != 1 || web.PlaySessionId == "" {
		t.Fatalf("answer: %+v", web)
	}
	source := web.MediaSources[0]
	if !source.SupportsDirectPlay || !source.SupportsDirectStream || source.SupportsTranscoding || source.Container != "mp4" {
		t.Errorf("H.264/AAC in MP4 for jellyfin-web: %+v", source)
	}
	subtitle := source.MediaStreams[0]
	if subtitle.DeliveryMethod != "External" || !strings.HasSuffix(subtitle.DeliveryUrl, "/Subtitles/0/0/Stream.vtt?ApiKey="+p.token) {
		t.Errorf("subtitle delivery: %s %s", subtitle.DeliveryMethod, subtitle.DeliveryUrl)
	}
	if refused := ask("minimal-no-aac", p.versions[1].ID.String()).MediaSources[0]; refused.SupportsDirectPlay {
		t.Error("AAC audio played by a profile without AAC")
	}
	// Without a choice, the first version that can be analyzed is played:
	// the first cannot, as ffprobe is not installed in tests.
	if chosen := ask("jellyfin-web-chrome", "").MediaSources[0]; chosen.Id != p.versions[1].ID.String() {
		t.Errorf("chosen version: %s", chosen.Id)
	}
	status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token),
		map[string]any{"MediaSourceId": strings.Repeat("ab", 16)})
	if status != http.StatusOK || !strings.Contains(string(data), `"ErrorCode":"NoCompatibleStream"`) {
		t.Errorf("unknown version: %d %s", status, data)
	}
}

func TestStreamsNeedAGrant(t *testing.T) {
	p := playing(t)
	status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token),
		map[string]any{"MediaSourceId": p.versions[1].ID.String()})
	var info playbackInfoResponse
	if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil {
		t.Fatalf("playback info: %d %s", status, data)
	}
	stream := p.url + "/Videos/" + p.movie + "/stream?static=true&mediaSourceId=" + info.MediaSources[0].Id + "&tag=" + info.MediaSources[0].ETag
	fetch := func(target string) (*http.Response, string) {
		t.Helper()
		request, _ := http.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("Range", "bytes=0-3")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return response, string(body)
	}
	// The test addon is on the loopback interface: players outside could not
	// reach it, so Polyfin relays it.
	for name, target := range map[string]string{
		"play session": stream + "&playSessionId=" + info.PlaySessionId,
		"token":        stream + "&ApiKey=" + p.token,
		"media path":   info.MediaSources[0].Path,
	} {
		response, body := fetch(target)
		if response.StatusCode != http.StatusPartialContent || body != "\x1a\x45\xdf\xa3" || response.Header.Get("Content-Type") != "video/mp4" {
			t.Errorf("%s: %d %q %s", name, response.StatusCode, body, response.Header.Get("Content-Type"))
		}
	}
	for name, target := range map[string]string{
		"no credentials":      stream,
		"forged play session": stream + "&playSessionId=" + info.PlaySessionId[:len(info.PlaySessionId)-2] + "AA",
	} {
		if response, _ := fetch(target); response.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s: %d", name, response.StatusCode)
		}
	}
}

func TestSubtitlesNeedNoCredentials(t *testing.T) {
	p := playing(t)
	p.get(t, "/Items/"+p.movie, p.token, nil)
	for format, want := range map[string]string{
		"vtt": "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nBonjour\n",
		"srt": "1\n00:00:01,000 --> 00:00:04,000\nBonjour\n",
		"js":  `{"TrackEvents":[{"Id":"1","Text":"Bonjour","StartPositionTicks":10000000,"EndPositionTicks":40000000}]}`,
	} {
		response, err := http.Get(p.url + "/Videos/" + p.movie + "/" + p.movie + "/Subtitles/0/0/Stream." + format)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK || string(body) != want || response.Header.Get("Content-Length") == "" {
			t.Errorf("%s: %d %q", format, response.StatusCode, body)
		}
	}
	if response, _ := http.Get(p.url + "/Videos/" + p.movie + "/" + p.movie + "/Subtitles/5/Stream.vtt"); response.StatusCode != http.StatusInternalServerError {
		t.Errorf("a subtitle that does not exist: %d", response.StatusCode)
	}
}

func TestSessionsShowWhatDevicesPlay(t *testing.T) {
	p := playing(t)
	report := func(path string, body map[string]any) {
		t.Helper()
		if status, data := p.call(http.MethodPost, path, app("tv", p.token), body); status != http.StatusNoContent {
			t.Fatalf("%s: %d %s", path, status, data)
		}
	}
	report("/Sessions/Playing", map[string]any{"ItemId": p.movie, "MediaSourceId": p.movie, "PositionTicks": 0, "PlayMethod": "DirectPlay"})
	report("/Sessions/Playing/Progress", map[string]any{"ItemId": p.movie, "PositionTicks": 600_000_000, "IsPaused": true})
	var sessions []SessionInfo
	p.get(t, "/Sessions", p.token, &sessions)
	if len(sessions) != 1 || sessions[0].NowPlayingItem == nil || sessions[0].NowPlayingItem.Id != p.movie ||
		*sessions[0].PlayState.PositionTicks != 600_000_000 || !sessions[0].PlayState.IsPaused || sessions[0].PlayState.PlayMethod != "DirectPlay" {
		t.Fatalf("sessions: %+v", sessions)
	}
	report("/Sessions/Playing/Stopped", map[string]any{"ItemId": p.movie, "PositionTicks": 600_000_000})
	var stopped []SessionInfo
	p.get(t, "/Sessions", p.token, &stopped)
	if len(stopped) != 1 || stopped[0].NowPlayingItem != nil || stopped[0].PlayState.MediaSourceId != "" {
		t.Errorf("after stopping: %+v", stopped)
	}
	if status, _ := p.call(http.MethodPost, "/Sessions/Playing", app("tv", p.token), nil); status != http.StatusUnsupportedMediaType {
		t.Errorf("report without a body: %d", status)
	}
}

// TestPlaybackResponsesMatchJellyfin compares playback responses with those
// recorded from Jellyfin 12.1 by scripts/jellyfin-fixtures.sh, for versions
// analyzed as the clips Jellyfin played.
func TestPlaybackResponsesMatchJellyfin(t *testing.T) {
	p := playing(t)
	user := p.user.ID.String()
	p.analyzed(t, p.versions[0], "h264-ac3-srt-mkv")
	remote := p.versionsOf(t, p.remote)[0]
	p.analyzed(t, remote, "remote-h264-aac-mkv")
	signedIn := app("tv", p.token)
	playbackInfo := func(item string) []byte {
		_, body := p.call(http.MethodPost, "/Items/"+item+"/PlaybackInfo?userId="+user, signedIn,
			map[string]any{"UserId": user, "MaxStreamingBitrate": 120_000_000, "DeviceProfile": p.profile(t, "jellyfin-web-chrome")})
		return body
	}
	_, detail := p.call(http.MethodGet, "/Users/"+user+"/Items/"+p.movie, signedIn, nil)
	local, remoteInfo := playbackInfo(p.movie), playbackInfo(p.remote)
	p.call(http.MethodPost, "/Sessions/Playing", signedIn, map[string]any{"ItemId": p.remote, "MediaSourceId": p.remote, "PlayMethod": "DirectPlay", "PositionTicks": 0})
	p.call(http.MethodPost, "/Sessions/Playing/Progress", signedIn, map[string]any{"ItemId": p.remote, "MediaSourceId": p.remote, "PositionTicks": 20_000_000, "IsPaused": true})
	_, sessions := p.call(http.MethodGet, "/Sessions", signedIn, nil)
	for fixture, body := range map[string][]byte{
		"item-media-sources":   detail,
		"playback-info":        local,
		"playback-info-remote": remoteInfo,
		"sessions-now-playing": sessions,
	} {
		raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.1", fixture+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var want, got any
		if err := json.Unmarshal(raw, &want); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("%s: %v in %s", fixture, err, body)
		}
		for _, difference := range compareShapes(fixture, want, got, playbackShapes) {
			t.Error(difference)
		}
	}
}

// playbackShapes lists what Polyfin cannot match in the playback fixtures.
var playbackShapes = shapeRules{
	dynamic: []string{"ImageTags", "ImageBlurHashes", "ProviderIds"},
	absent: map[string][]string{
		// Polyfin has no files: neither titles nor subtitle files have a
		// path on the server.
		"Path": {"*"},
		// Jellyfin scores subtitle tracks while choosing the default one;
		// Polyfin chooses without exposing scores.
		"Score": {"*"},
		// Jellyfin counts no reference frames in remote sources, which
		// Polyfin's all are; the local clips had some.
		"RefFrames": {"item-media-sources", "playback-info"},
		// Polyfin does not transcode yet.
		"TranscodingUrl": {"playback-info"}, "TranscodingContainer": {"playback-info"},
		// Metadata addons do not provide these.
		"OriginalLanguage": {"*"}, "ProductionLocations": {"*"},
	},
	returned: map[string][]string{
		// The recorded titles lacked the ratings the test addon provides.
		"CommunityRating": {"item-media-sources", "sessions-now-playing"}, "OfficialRating": {"item-media-sources"},
	},
}
