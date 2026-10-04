package jellyfin

import (
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
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
		case path == "/files/remux.mkv":
			// A real Matroska file, whose index remuxing reads.
			http.ServeFile(w, r, filepath.Join("..", "container", "testdata", "forced.mkv"))
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
	return playingOn(t, newTestServer(t, 10))
}

// playingOn is playing on a given test server.
func playingOn(t *testing.T, s testServer) playbackSetup {
	t.Helper()
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

// Apps that ask for no version, such as Strand, list the versions
// PlaybackInfo gives for the user to pick: every one, as from Jellyfin.
func TestPlaybackInfoListsEveryVersionUnlessOneIsAsked(t *testing.T) {
	p := playing(t)
	p.analyzed(t, p.versions[0], "h264-ac3-srt-mkv")
	ask := func(mediaSourceID string) playbackInfoResponse {
		t.Helper()
		body := map[string]any{"UserId": p.user.ID.String(), "DeviceProfile": p.profile(t, "jellyfin-web-chrome")}
		if mediaSourceID != "" {
			body["MediaSourceId"] = mediaSourceID
		}
		status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), body)
		var response playbackInfoResponse
		if err := json.Unmarshal(data, &response); status != http.StatusOK || err != nil {
			t.Fatalf("%s: %d %s", mediaSourceID, status, data)
		}
		return response
	}
	all := ask("")
	if len(all.MediaSources) != 2 || all.MediaSources[0].Id != p.movie || all.MediaSources[1].Id != p.versions[1].ID.String() {
		t.Fatalf("versions: %+v", all.MediaSources)
	}
	// The first is decided for the app: AC3 is not direct played by
	// Chrome. The other is described as item details describe it.
	if decided := all.MediaSources[0]; decided.Container != "mkv" || decided.SupportsDirectPlay {
		t.Errorf("decided version: %s direct play %v", decided.Container, decided.SupportsDirectPlay)
	}
	var movie BaseItemDto
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
	described, listed := (*movie.MediaSources)[1], all.MediaSources[1]
	// Each answer signs its own media URLs.
	described.Path, listed.Path = "", ""
	if !reflect.DeepEqual(listed, described) {
		t.Errorf("other version:\n got %+v\nwant %+v", listed, described)
	}

	if one := ask(p.versions[1].ID.String()); len(one.MediaSources) != 1 || one.MediaSources[0].Id != p.versions[1].ID.String() {
		t.Errorf("a version asked: %+v", one.MediaSources)
	}
}

// remuxable stores the analysis of the file the addon serves for the
// first version, Opus then H.264 for 15 s, with more streams if given.
func (p playbackSetup) remuxable(t *testing.T, more ...media.Stream) media.Analysis {
	t.Helper()
	info, err := os.Stat(filepath.Join("..", "container", "testdata", "forced.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	analysis := media.Analysis{Format: "matroska,webm", Duration: 15008 * time.Millisecond, Size: info.Size(), Bitrate: 25_000, Remote: true,
		Streams: append([]media.Stream{
			{Index: 0, Type: "audio", Codec: "opus", Default: true, Channels: 1, SampleRate: 8000, ChannelLayout: "mono"},
			{Index: 1, Type: "video", Codec: "h264", Profile: "High", Level: 10, Width: 64, Height: 64, FrameRate: 24, AverageRate: 24, PixelFormat: "yuv420p", BitDepth: 8},
		}, more...)}
	data, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", p.versions[0].ID, data); err != nil {
		t.Fatal(err)
	}
	return analysis
}

func TestAppsThatCannotPlayAVersionGetARemux(t *testing.T) {
	p := playing(t)
	p.remuxable(t)
	// jellyfin-web asks again without direct play when the file failed to
	// play as it is.
	status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), map[string]any{
		"UserId": p.user.ID.String(), "MediaSourceId": p.movie, "MaxStreamingBitrate": 120_000_000, "EnableDirectPlay": false,
		"DeviceProfile": p.profile(t, "jellyfin-web-chrome")})
	var response playbackInfoResponse
	if err := json.Unmarshal(data, &response); status != http.StatusOK || err != nil || len(response.MediaSources) != 1 {
		t.Fatalf("%d %s", status, data)
	}
	source := response.MediaSources[0]
	if source.SupportsDirectPlay || !source.SupportsTranscoding || source.TranscodingSubProtocol != "hls" || source.TranscodingContainer != "mp4" {
		t.Fatalf("source: %+v", source)
	}
	target := source.TranscodingUrl
	// The subtitle file comes first: the audio is stream 1.
	for _, part := range []string{"/videos/" + hyphenated(mustID(t, p.movie)) + "/master.m3u8?DeviceId=", "&MediaSourceId=" + p.movie + "&VideoCodec=av1,hevc,h264,vp9&AudioCodec=aac,mp2,opus,flac&AudioStreamIndex=1&",
		"&SegmentContainer=mp4&MinSegments=2&PlaySessionId=" + url.QueryEscape(response.PlaySessionId) + "&ApiKey=" + p.token + "&", "&SubtitleMethod=Encode&TranscodeReasons=DirectPlayError"} {
		if !strings.Contains(target, part) {
			t.Errorf("TranscodingUrl lacks %q: %s", part, target)
		}
	}
	// Players fetch the playlists and segments with no credentials but the
	// URL's.
	get := func(target string) (int, http.Header, string) {
		t.Helper()
		response, err := http.Get(p.url + target)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return response.StatusCode, response.Header, string(body)
	}
	status, header, master := get(target)
	if status != http.StatusOK || header.Get("Content-Type") != "application/vnd.apple.mpegurl" ||
		!strings.Contains(master, `CODECS="avc1.64000A,Opus",RESOLUTION=64x64,FRAME-RATE=24`) || !strings.Contains(master, "\nmain.m3u8?DeviceId=") {
		t.Fatalf("master playlist: %d %s", status, master)
	}
	base := strings.Split(target, "master.m3u8")[0]
	_, _, media := get(base + strings.TrimSpace(strings.Split(master, "\n")[2]))
	for _, part := range []string{"#EXT-X-TARGETDURATION:7\n", "#EXT-X-MAP:URI=\"hls1/main/-1.mp4?DeviceId=",
		"#EXTINF:6.000000, nodesc\nhls1/main/0.mp4?", "#EXTINF:6.500000, nodesc\nhls1/main/1.mp4?", "#EXTINF:2.508000, nodesc\nhls1/main/2.mp4?", "#EXT-X-ENDLIST"} {
		if !strings.Contains(media, part) {
			t.Errorf("media playlist lacks %q:\n%s", part, media)
		}
	}
	query := strings.SplitN(target, "?", 2)[1]
	if status, _, _ := get(base + "hls1/main/0.mp4?" + strings.ReplaceAll(query, "PlaySessionId=", "PlaySessionId=forged")); status != http.StatusUnauthorized {
		t.Errorf("a forged play session got %d", status)
	}
	if os.Getenv("POLYFIN_TEST_FFMPEG") != "" {
		status, header, segment := get(base + "hls1/main/1.mp4?" + query)
		if status != http.StatusOK || header.Get("Content-Type") != "video/mp4" || !strings.Contains(segment[:64], "moof") {
			t.Errorf("segment: %d %s", status, header.Get("Content-Type"))
		}
	}
	if status, _ := p.call(http.MethodDelete, "/Videos/ActiveEncodings?deviceId=tv&playSessionId="+url.QueryEscape(response.PlaySessionId), app("tv", p.token), nil); status != http.StatusNoContent {
		t.Errorf("stopping the remux: %d", status)
	}
}

func TestAudioTheAppCannotTakeIsConverted(t *testing.T) {
	p := playing(t)
	p.remuxable(t)
	ask := func(profile string, more map[string]any) MediaSourceInfo {
		t.Helper()
		body := map[string]any{"UserId": p.user.ID.String(), "MediaSourceId": p.movie, "MaxStreamingBitrate": 120_000_000,
			"DeviceProfile": p.profile(t, profile)}
		maps.Copy(body, more)
		status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), body)
		var response playbackInfoResponse
		if err := json.Unmarshal(data, &response); status != http.StatusOK || err != nil || len(response.MediaSources) != 1 {
			t.Fatalf("%d %s", status, data)
		}
		return response.MediaSources[0]
	}
	// The app takes H.264, and AAC only: the Opus audio becomes AAC.
	source := ask("minimal", nil)
	target := source.TranscodingUrl
	if source.SupportsDirectPlay || !source.SupportsTranscoding || source.TranscodingContainer != "ts" ||
		!strings.HasSuffix(target, "&TranscodeReasons=ContainerNotSupported,AudioCodecNotSupported&allowAudioStreamCopy=false") ||
		strings.Contains(target, "AudioSampleRate") {
		t.Fatalf("source: %+v", source)
	}
	base := p.url + strings.Split(target, "master.m3u8")[0]
	query := strings.SplitN(target, "?", 2)[1]
	if _, _, master := fetchText(t, base+"master.m3u8?"+query); !strings.Contains(master, `CODECS="avc1.64000A,mp4a.40.2"`) {
		t.Errorf("master playlist:\n%s", master)
	}
	if ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG"); ffmpeg != "" {
		status, _, segment := fetchText(t, base+"hls1/main/0.ts?"+query)
		path := filepath.Join(t.TempDir(), "0.ts")
		if err := os.WriteFile(path, []byte(segment), 0o600); err != nil {
			t.Fatal(err)
		}
		out, _ := exec.Command(filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), "-v", "error", "-select_streams", "a",
			"-show_entries", "stream=codec_name,channels", "-of", "csv=p=0", path).Output()
		if got, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n"); status != http.StatusOK || got != "aac,1" {
			t.Errorf("segment: %d, audio %q", status, got)
		}
	}
	// jellyfin-web takes Opus, unless it asks for the audio to be converted.
	if copied := ask("jellyfin-web-chrome", map[string]any{"EnableDirectPlay": false}); strings.Contains(copied.TranscodingUrl, "allowAudioStreamCopy") {
		t.Errorf("Opus converted for jellyfin-web: %s", copied.TranscodingUrl)
	}
	if converted := ask("jellyfin-web-chrome", map[string]any{"EnableDirectPlay": false, "AllowAudioStreamCopy": false}); !strings.HasSuffix(converted.TranscodingUrl, "&allowAudioStreamCopy=false") {
		t.Errorf("copy refused: %s", converted.TranscodingUrl)
	}
}

func TestVideoTheAppCannotTakeIsConverted(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: Polyfin converts with the encoders FFmpeg has")
	}
	p := playing(t)
	p.remuxable(t)
	// jellyfin-web asks again without copying the video when a file failed
	// to play as it is.
	status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), map[string]any{
		"UserId": p.user.ID.String(), "MediaSourceId": p.movie, "MaxStreamingBitrate": 120_000_000,
		"EnableDirectPlay": false, "AllowVideoStreamCopy": false, "DeviceProfile": p.profile(t, "jellyfin-web-chrome")})
	var response playbackInfoResponse
	if err := json.Unmarshal(data, &response); status != http.StatusOK || err != nil || len(response.MediaSources) != 1 {
		t.Fatalf("%d %s", status, data)
	}
	target := response.MediaSources[0].TranscodingUrl
	if !strings.HasSuffix(target, "&TranscodeReasons=DirectPlayError&allowVideoStreamCopy=false") {
		t.Fatalf("TranscodingUrl: %s", target)
	}
	base := p.url + strings.Split(target, "master.m3u8")[0]
	query := strings.SplitN(target, "?", 2)[1]
	if _, _, master := fetchText(t, base+"master.m3u8?"+query); !strings.Contains(master, `CODECS="avc1.640029,Opus",RESOLUTION=64x64`) {
		t.Errorf("master playlist:\n%s", master)
	}
	status, _, segment := fetchText(t, base+"hls1/main/-1.mp4?"+query)
	_, _, first := fetchText(t, base+"hls1/main/0.mp4?"+query)
	path := filepath.Join(t.TempDir(), "0.mp4")
	if err := os.WriteFile(path, []byte(segment+first), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := exec.Command(filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), "-v", "error", "-select_streams", "v",
		"-show_entries", "stream=codec_name,width,height,pix_fmt", "-of", "csv=p=0", path).Output()
	if got := strings.TrimSpace(string(out)); status != http.StatusOK || got != "h264,64,64,yuv420p" {
		t.Errorf("segment: %d, video %q", status, got)
	}
}

func TestImageSubtitlesBrowsersCannotShowAreBurnedIn(t *testing.T) {
	p := playing(t)
	// An English PGS track, after the addon's file: stream 3 for Jellyfin.
	p.remuxable(t, media.Stream{Index: 2, Type: "subtitle", Codec: "hdmv_pgs_subtitle", Language: "eng", Width: 64, Height: 64})
	status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), map[string]any{
		"UserId": p.user.ID.String(), "MediaSourceId": p.movie, "MaxStreamingBitrate": 120_000_000, "SubtitleStreamIndex": 3,
		"DeviceProfile": p.profile(t, "jellyfin-web-chrome")})
	var response playbackInfoResponse
	if err := json.Unmarshal(data, &response); status != http.StatusOK || err != nil || len(response.MediaSources) != 1 {
		t.Fatalf("%d %s", status, data)
	}
	source := response.MediaSources[0]
	i := slices.IndexFunc(source.MediaStreams, func(s playback.MediaStream) bool { return s.Index == 3 })
	if i < 0 {
		t.Fatalf("streams: %+v", source.MediaStreams)
	}
	track := source.MediaStreams[i]
	if os.Getenv("POLYFIN_TEST_FFMPEG") == "" {
		// Without an encoder, the track is left out and the file plays as
		// it is.
		if !source.SupportsDirectPlay || track.DeliveryMethod != "Drop" {
			t.Errorf("without FFmpeg: direct play %v, track %s", source.SupportsDirectPlay, track.DeliveryMethod)
		}
		return
	}
	// Chrome shows no PGS: the video is converted, with the track drawn
	// onto it.
	if source.SupportsDirectPlay || track.DeliveryMethod != "Encode" || !strings.Contains(source.TranscodingUrl, "&AudioStreamIndex=1&SubtitleStreamIndex=3&") ||
		!strings.HasSuffix(source.TranscodingUrl, "&SubtitleMethod=Encode&TranscodeReasons=SubtitleCodecNotSupported&allowVideoStreamCopy=false") {
		t.Errorf("direct play %v, track %s, TranscodingUrl %s", source.SupportsDirectPlay, track.DeliveryMethod, source.TranscodingUrl)
	}
}

func mustID(t *testing.T, s string) accounts.ID {
	t.Helper()
	id, err := accounts.ParseID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// fetchText gets a URL with no credentials but those it carries, as
// players do.
func fetchText(t *testing.T, target string) (int, http.Header, string) {
	t.Helper()
	response, err := http.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return response.StatusCode, response.Header, string(body)
}

func TestRemuxesOfferSubtitlesAsRenditions(t *testing.T) {
	p := playing(t)
	p.remuxable(t)
	// A player that takes subtitles from the playlist only, as Swiftfin's
	// native player does, with jellyfin-web's codecs.
	var profile map[string]any
	if err := json.Unmarshal(p.profile(t, "jellyfin-web-chrome"), &profile); err != nil {
		t.Fatal(err)
	}
	profile["SubtitleProfiles"] = []map[string]string{{"Format": "vtt", "Method": "Hls"}}
	status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), map[string]any{
		"UserId": p.user.ID.String(), "MediaSourceId": p.movie, "MaxStreamingBitrate": 120_000_000, "EnableDirectPlay": false,
		"SubtitleStreamIndex": 0, "DeviceProfile": profile})
	var response playbackInfoResponse
	if err := json.Unmarshal(data, &response); status != http.StatusOK || err != nil || len(response.MediaSources) != 1 {
		t.Fatalf("%d %s", status, data)
	}
	source := response.MediaSources[0]
	if file := source.MediaStreams[0]; file.DeliveryMethod != "Hls" || file.DeliveryUrl != "" {
		t.Errorf("the addon's subtitle file: %s %s", file.DeliveryMethod, file.DeliveryUrl)
	}
	target := source.TranscodingUrl
	for _, part := range []string{"&AudioStreamIndex=1&SubtitleStreamIndex=0&VideoBitrate=", "&SubtitleMethod=Hls&TranscodeReasons=DirectPlayError"} {
		if !strings.Contains(target, part) {
			t.Errorf("TranscodingUrl lacks %q: %s", part, target)
		}
	}
	query := strings.SplitN(target, "?", 2)[1]
	base := p.url + strings.Split(target, "master.m3u8")[0]
	_, _, master := fetchText(t, base+"master.m3u8?"+query)
	if !strings.Contains(master, "#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID=\"subs\",NAME=\"French - SUBRIP - External\",LANGUAGE=\"fr\",DEFAULT=YES,AUTOSELECT=YES,FORCED=NO,URI=\"hls1/subtitles0/main.m3u8?DeviceId=") ||
		!strings.Contains(master, ",SUBTITLES=\"subs\"\nmain.m3u8?") {
		t.Errorf("master playlist:\n%s", master)
	}
	_, _, playlist := fetchText(t, base+"hls1/subtitles0/main.m3u8?"+query)
	for _, part := range []string{"#EXT-X-TARGETDURATION:7\n", "#EXTINF:6.000000,\n0.vtt?DeviceId=", "#EXTINF:2.508000,\n2.vtt?", "#EXT-X-ENDLIST"} {
		if !strings.Contains(playlist, part) {
			t.Errorf("subtitle playlist lacks %q:\n%s", part, playlist)
		}
	}
	// Cues keep the version's time; the map places them on the remux's.
	for n, want := range []string{
		"WEBVTT\nX-TIMESTAMP-MAP=MPEGTS:900000,LOCAL:00:00:00.000\n\n00:00:01.000 --> 00:00:04.000\nBonjour\n",
		"WEBVTT\nX-TIMESTAMP-MAP=MPEGTS:900000,LOCAL:00:00:00.000\n",
	} {
		status, header, segment := fetchText(t, base+"hls1/subtitles0/"+strconv.Itoa(n)+".vtt?"+query)
		if status != http.StatusOK || header.Get("Content-Type") != "text/vtt" || segment != want {
			t.Errorf("segment %d: %d %q", n, status, segment)
		}
	}
	if status, _, _ := fetchText(t, base+"hls1/subtitles5/main.m3u8?"+query); status != http.StatusNotFound {
		t.Errorf("a subtitle the version lacks: %d", status)
	}
}

func TestWholeExtractedTracksAreExternalSubtitles(t *testing.T) {
	p := playing(t)
	p.remuxable(t, media.Stream{Index: 2, Type: "subtitle", Codec: "subrip", Language: "eng"})
	// Earlier remuxes extracted the embedded track over the whole version.
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_subtitles (version_id, extracted) VALUES ($1, $2)", p.versions[0].ID,
		`{"covered":[[0,15008]],"tracks":{"2":[{"s":1000,"e":2500,"t":"Hello"}]}}`); err != nil {
		t.Fatal(err)
	}
	// jellyfin-web plays the file as it is, and takes subtitles as files.
	status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), map[string]any{
		"UserId": p.user.ID.String(), "MediaSourceId": p.movie, "MaxStreamingBitrate": 120_000_000, "SubtitleStreamIndex": 3,
		"DeviceProfile": p.profile(t, "jellyfin-web-chrome")})
	var response playbackInfoResponse
	if err := json.Unmarshal(data, &response); status != http.StatusOK || err != nil || len(response.MediaSources) != 1 {
		t.Fatalf("%d %s", status, data)
	}
	source := response.MediaSources[0]
	i := slices.IndexFunc(source.MediaStreams, func(s playback.MediaStream) bool { return s.Index == 3 })
	if !source.SupportsDirectPlay || i < 0 {
		t.Fatalf("source: %+v", source)
	}
	track := source.MediaStreams[i]
	if track.DeliveryMethod != "External" || !strings.HasSuffix(track.DeliveryUrl, "/Subtitles/3/0/Stream.vtt?ApiKey="+p.token) {
		t.Fatalf("embedded track: %s %s", track.DeliveryMethod, track.DeliveryUrl)
	}
	if status, _, body := fetchText(t, p.url+track.DeliveryUrl); status != http.StatusOK || body != "WEBVTT\n\n00:00:01.000 --> 00:00:02.500\nHello\n" {
		t.Errorf("track: %d %q", status, body)
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
	// The session was given for the second version; the app plays the
	// first, which PlaybackInfo listed too, under the same session.
	picked := p.url + "/Videos/" + p.movie + "/stream?static=true&mediaSourceId=" + p.movie + "&playSessionId=" + info.PlaySessionId
	if response, _ := fetch(picked); response.StatusCode != http.StatusPartialContent || response.Header.Get("Content-Type") != "video/x-matroska" {
		t.Errorf("a version picked under the session: %d %s", response.StatusCode, response.Header.Get("Content-Type"))
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
		// The test addon serves bytes that are not a Matroska file: without
		// a keyframe index, no remux is offered.
		"TranscodingUrl": {"playback-info"}, "TranscodingContainer": {"playback-info"},
		// Metadata addons do not provide these.
		"OriginalLanguage": {"*"}, "ProductionLocations": {"*"},
	},
	returned: map[string][]string{
		// The recorded titles lacked the ratings the test addon provides.
		"CommunityRating": {"item-media-sources", "sessions-now-playing"}, "OfficialRating": {"item-media-sources"},
	},
}
