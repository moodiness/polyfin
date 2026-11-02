package jellyfin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// androidHeader is the Authorization header the Jellyfin app for Android
// 2.7.3 sends, its API calls and its integrated player's requests alike
// (AppModule.kt, lines 125 to 141).
func androidHeader(token string) string {
	return androidHeaderOf(androidApp, "2.7.3", token)
}

// androidHeaderOf is the Authorization header the app sends as client,
// in version: "Jellyfin Android" before 2.7.0.
func androidHeaderOf(client, version, token string) string {
	header := `MediaBrowser Client="` + client + `", Device="Pixel", DeviceId="android-838", Version="` + version + `"`
	if token != "" {
		header += `, Token="` + token + `"`
	}
	return header
}

// signInAndroid signs name in from the Jellyfin app for Android 2.7.3.
func (s testServer) signInAndroid(t *testing.T, name string) string {
	t.Helper()
	return s.signInAs(t, name, androidHeader(""))
}

// signInAs signs name in with authorization, an app's header.
func (s testServer) signInAs(t *testing.T, name, authorization string) string {
	t.Helper()
	status, body := s.call(http.MethodPost, "/Users/AuthenticateByName", authorization, map[string]string{"Username": name, "Pw": "correct horse"})
	var result AuthenticationResult
	if status != http.StatusOK || json.Unmarshal(body, &result) != nil {
		t.Fatalf("sign-in of %s: %d %s", name, status, body)
	}
	return result.AccessToken
}

// androidPlaybackInfo asks PlaybackInfo for item as the app does
// (MediaSourceResolver.kt, lines 41 to 62): authorization the header
// sent, the DeviceProfile of profile, the start, AutoOpenLiveStream, and
// the media source when one is named. It returns the answer.
func (s testServer) androidPlaybackInfo(t *testing.T, authorization, item, mediaSourceID, profile string, start time.Duration) playbackInfoResponse {
	t.Helper()
	body := map[string]any{"DeviceProfile": deviceProfile(t, profile), "MaxStreamingBitrate": 120_000_000,
		"StartTimeTicks": int64(start / 100), "AutoOpenLiveStream": true}
	if mediaSourceID != "" {
		body["MediaSourceId"] = mediaSourceID
	}
	status, data := s.call(http.MethodPost, "/Items/"+item+"/PlaybackInfo", authorization, body)
	var response playbackInfoResponse
	if err := json.Unmarshal(data, &response); status != http.StatusOK || err != nil || len(response.MediaSources) == 0 {
		t.Fatalf("PlaybackInfo with %s: %d %s", profile, status, data)
	}
	return response
}

// integratedPlayerURL is the address the app's integrated player plays a
// source described as a file from (QueueManager.kt, lines 350 to 359):
// VideosApi.getVideoStreamUrl, with no token in it.
func integratedPlayerURL(base, item, session string, source MediaSourceInfo) string {
	query := url.Values{"static": {"true"}, "playSessionId": {session}, "mediaSourceId": {source.Id}, "deviceId": {"android-838"}}
	return base + "/Videos/" + item + "/stream?" + query.Encode()
}

// playsAsFile checks that the source the integrated player chooses
// (MediaSourceResolver.kt, lines 66 and 67: the one with the item's own
// id, else the first) plays as it is from a file, and that the URL the
// app builds for it serves want whole, then from a resume point, with
// the app's Authorization header only.
func playsAsFile(t *testing.T, base, item, authorization string, answer playbackInfoResponse, want []byte) {
	t.Helper()
	source := answer.MediaSources[0]
	for _, s := range answer.MediaSources {
		if id, _ := parseGUID(s.Id); id.String() == mustGUID(t, item) {
			source = s
		}
	}
	if source.Protocol != "File" || !source.SupportsDirectPlay {
		t.Fatalf("the integrated player's source: Protocol %q, direct play %v, %+v", source.Protocol, source.SupportsDirectPlay, source)
	}
	target := integratedPlayerURL(base, item, answer.PlaySessionId, source)
	header := http.Header{"Authorization": {authorization}}
	response, body := fetchURL(t, target, header)
	if response.StatusCode != http.StatusOK || body != string(want) {
		t.Fatalf("the app's URL: %d, %d bytes of %d", response.StatusCode, len(body), len(want))
	}
	resume := len(want) / 2
	header.Set("Range", "bytes="+strconv.Itoa(resume)+"-")
	response, body = fetchURL(t, target, header)
	if response.StatusCode != http.StatusPartialContent || body != string(want[resume:]) {
		t.Fatalf("the app's URL from a resume point: %d, %d bytes of %d", response.StatusCode, len(body), len(want)-resume)
	}
}

// mustGUID is id as Polyfin writes identifiers.
func mustGUID(t *testing.T, id string) string {
	t.Helper()
	parsed, ok := parseGUID(id)
	if !ok {
		t.Fatalf("not an identifier: %q", id)
	}
	return parsed.String()
}

// The Jellyfin app for Android's integrated player loads a source
// described as Http as an HLS playlist, and one described as a file from
// /Videos/{id}/stream?static=true (#838): an addon's link is described to
// it as a file, as Jellyfin describes its own, and plays from that URL,
// whole and from a resume point. jellyfin-web, in a browser or in the
// app's web view under the app's client name, keeps Http and Path, as do
// the app's older profile and a version that is an HLS playlist.
func TestTheAndroidIntegratedPlayerPlaysAddonLinksAsFiles(t *testing.T) {
	t.Parallel()
	p := playing(t)
	p.remuxable(t)
	file, err := os.ReadFile(filepath.Join("..", "container", "testdata", "forced.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	token := p.signInAndroid(t, "member")
	// The second version is an HLS playlist: the app reads it as one.
	if _, err := p.pool.Exec(t.Context(), `UPDATE media_analyses SET analysis = jsonb_set(analysis, '{format}', '"hls,applehttp"') WHERE version_id = $1`,
		p.versions[1].ID); err != nil {
		t.Fatal(err)
	}
	answer := p.androidPlaybackInfo(t, androidHeader(token), p.movie, "", "jellyfin-android-2.7.3-integrated", 7*time.Second)
	if len(answer.MediaSources) != 2 || answer.MediaSources[1].Protocol != "Http" {
		t.Errorf("an HLS version to the integrated player: %+v", answer.MediaSources)
	}
	playsAsFile(t, p.url, p.movie, androidHeader(token), answer, file)

	for _, ask := range []struct{ name, authorization, profile string }{
		{"the app's web view", androidHeader(token), "jellyfin-web-chrome"},
		{"the 2.6.4 app's web view", androidHeaderOf(androidAppBefore27, "2.6.4", token), "jellyfin-web-chrome"},
		{"jellyfin-web 12.4 in Chrome", app("web", p.token), "jellyfin-web-12.4-chrome"},
		{"the app's older profile from 2.7.3", androidHeader(token), "jellyfin-android-integrated"},
		{"the integrated player's profile from another app", app("tv", p.token), "jellyfin-android-2.7.3-integrated"},
		{"Android TV", app("tv", p.token), "androidtv"},
		{"Swiftfin", app("tv", p.token), "swiftfin-native"},
	} {
		for _, source := range p.androidPlaybackInfo(t, ask.authorization, p.movie, p.movie, ask.profile, 0).MediaSources {
			if source.Protocol != "Http" || source.Path == "" {
				t.Errorf("%s: Protocol %q, Path %q", ask.name, source.Protocol, source.Path)
			}
		}
	}
}

// A local file is described to the integrated player as a file, as
// Jellyfin describes it, and plays from the URL the app builds.
func TestTheAndroidIntegratedPlayerPlaysLocalFilesAsFiles(t *testing.T) {
	t.Parallel()
	var folders *localfiles.Service
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.NewTextHandler(io.Discard, nil)),
			o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders))
	})
	p := playingOn(t, s)
	file, err := os.ReadFile(filepath.Join("..", "container", "testdata", "forced.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Movie (2008) 1080p.mkv"), file, 0o644); err != nil {
		t.Fatal(err)
	}
	addon, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shelf", Path: dir, Kind: localfiles.KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if err := folders.Link(t.Context(), addon.ID, "Movie (2008) 1080p.mkv", "tt1000"); err != nil {
		t.Fatal(err)
	}
	var movie BaseItemDto
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
	if movie.MediaSources == nil || len(*movie.MediaSources) != 3 {
		t.Fatalf("media sources: %+v", movie.MediaSources)
	}
	local := (*movie.MediaSources)[2]
	id, err := accounts.ParseID(local.Id)
	if err != nil {
		t.Fatal(err)
	}
	// ffprobe is not installed: the file is described as it would read.
	analysis := p.remuxable(t)
	analysis.Remote = false
	data, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", id, data); err != nil {
		t.Fatal(err)
	}
	token := p.signInAndroid(t, "member")
	answer := p.androidPlaybackInfo(t, androidHeader(token), p.movie, local.Id, "jellyfin-android-2.7.3-integrated", 7*time.Second)
	if len(answer.MediaSources) != 1 || answer.MediaSources[0].Id != local.Id {
		t.Fatalf("the local version: %+v", answer.MediaSources)
	}
	// The app plays the source it named, the only one.
	playsAsFile(t, p.url, p.movie, androidHeader(token), answer, file)
}

// A torrent's file is described to the integrated player as a file, and
// plays from the URL the app builds, its bytes read from the swarm.
func TestTheAndroidIntegratedPlayerPlaysTorrentsAsFiles(t *testing.T) {
	t.Parallel()
	ffmpeg, file := vodFile(t, "a-movie.mkv")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	tor := torrenttest.Build("A Movie", 64<<10, torrenttest.File{Name: "a-movie.mkv", Data: data})
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	p := newTorrentPlay(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), 0, []*torrenttest.Torrent{tor}, []string{tracker.URL()})
	token := p.signInAndroid(t, "allowed")
	answer := p.androidPlaybackInfo(t, androidHeader(token), p.movie, "", "jellyfin-android-2.7.3-integrated", 10*time.Second)
	playsAsFile(t, p.url, p.movie, androidHeader(token), answer, data)
}

// Before 2.7.0, the app and its integrated player's profile were named
// "Jellyfin Android". From 2.6.3 it plays File and Http as 2.7.3 does,
// and sends its token in the header, and gets its versions described as
// files; 2.6.2 and older send no token with the file's address and keep
// Http. An app updated to 2.7.3 since its sign-in, which keeps its
// token and the name it signed in with, is told by its header.
func TestTheAndroidIntegratedPlayerBefore27(t *testing.T) {
	t.Parallel()
	p := playing(t)
	p.remuxable(t)
	file, err := os.ReadFile(filepath.Join("..", "container", "testdata", "forced.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	token := p.signInAs(t, "member", androidHeaderOf(androidAppBefore27, "2.6.2", ""))
	for _, version := range []string{"2.6.3", "2.6.3-beta.1", "2.6.4"} {
		header := androidHeaderOf(androidAppBefore27, version, token)
		answer := p.androidPlaybackInfo(t, header, p.movie, "", "jellyfin-android-integrated", 7*time.Second)
		playsAsFile(t, p.url, p.movie, header, answer, file)
	}
	for _, version := range []string{"2.6.2", "2.4.0", "2.3.3"} {
		header := androidHeaderOf(androidAppBefore27, version, token)
		for _, source := range p.androidPlaybackInfo(t, header, p.movie, "", "jellyfin-android-integrated", 0).MediaSources {
			if source.Protocol != "Http" || source.Path == "" {
				t.Errorf("%s: Protocol %q, Path %q", version, source.Protocol, source.Path)
			}
		}
	}
	// The same sign-in, the app updated to 2.7.3.
	answer := p.androidPlaybackInfo(t, androidHeader(token), p.movie, "", "jellyfin-android-2.7.3-integrated", 7*time.Second)
	playsAsFile(t, p.url, p.movie, androidHeader(token), answer, file)
}
