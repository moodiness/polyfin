package jellyfin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/bandwidth"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/source"
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

// authorized is the header the app sends with the file's address from
// 2.6.3 on: its Authorization header.
func authorized(authorization string) http.Header {
	return http.Header{"Authorization": {authorization}}
}

// oldPlayerHeader is what the integrated player of versions 2.4.0 to
// 2.6.2 sends with the file's address: ExoPlayer's user agent, which the
// app names (ApplicationModule.kt, line 61, at v2.4.0; AppModule.kt,
// lines 82 to 105, at v2.6.2), and no token.
func oldPlayerHeader(version string) http.Header {
	return http.Header{"User-Agent": {androidAppBefore27 + "/" + version + " (Linux;Android 14) ExoPlayerLib/2.18.1"}}
}

// playsAsFile checks that the source the integrated player chooses
// (MediaSourceResolver.kt, lines 66 and 67: the one with the item's own
// id, else the first) plays as it is from a file, and that the URL the
// app builds for it serves want whole, then from a resume point, with
// the header the app sends only.
func playsAsFile(t *testing.T, base, item string, header http.Header, answer playbackInfoResponse, want []byte) {
	t.Helper()
	source := integratedSource(t, item, answer)
	if source.Protocol != "File" || !source.SupportsDirectPlay {
		t.Fatalf("the integrated player's source: Protocol %q, direct play %v, %+v", source.Protocol, source.SupportsDirectPlay, source)
	}
	target := integratedPlayerURL(base, item, answer.PlaySessionId, source)
	header = header.Clone()
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

// integratedSource is the source the integrated player plays of answer
// (MediaSourceResolver.kt, lines 66 and 67 at v2.7.3): the one with the
// item's own id, else the first.
func integratedSource(t *testing.T, item string, answer playbackInfoResponse) MediaSourceInfo {
	t.Helper()
	source := answer.MediaSources[0]
	for _, s := range answer.MediaSources {
		if id, _ := parseGUID(s.Id); id.String() == mustGUID(t, item) {
			source = s
		}
	}
	return source
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
	playsAsFile(t, p.url, p.movie, authorized(androidHeader(token)), answer, file)

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
// Jellyfin describes it, and plays from the URL the app builds: with the
// app's token from 2.6.3 on, with its play session alone before (#867).
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
	for _, play := range []struct {
		authorization, profile string
		header                 http.Header
	}{
		{androidHeader(token), "jellyfin-android-2.7.3-integrated", authorized(androidHeader(token))},
		{androidHeaderOf(androidAppBefore27, "2.6.2", token), "jellyfin-android-integrated", oldPlayerHeader("2.6.2")},
	} {
		answer := p.androidPlaybackInfo(t, play.authorization, p.movie, local.Id, play.profile, 7*time.Second)
		if len(answer.MediaSources) != 1 || answer.MediaSources[0].Id != local.Id {
			t.Fatalf("the local version: %+v", answer.MediaSources)
		}
		// The app plays the source it named, the only one.
		playsAsFile(t, p.url, p.movie, play.header, answer, file)
	}
}

// A torrent's file is described to the integrated player as a file, and
// plays from the URL the app builds, its bytes read from the swarm: with
// the app's token from 2.6.3 on, with its play session alone before.
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
	playsAsFile(t, p.url, p.movie, authorized(androidHeader(token)), answer, data)
	answer = p.androidPlaybackInfo(t, androidHeaderOf(androidAppBefore27, "2.4.0", token), p.movie, "", "jellyfin-android-integrated", 10*time.Second)
	playsAsFile(t, p.url, p.movie, oldPlayerHeader("2.4.0"), answer, data)
}

// Before 2.7.0, the app and its integrated player's profile were named
// "Jellyfin Android". From 2.4.0 it reads Protocol as 2.7.3 does, and
// gets its versions described as files: from 2.6.3 it sends its token
// in the header, before only its play session in the address (#867).
// 2.3.2 and 2.3.3 read Protocol too, but send no play session, and keep
// Http. An app updated to 2.7.3 since its sign-in, which keeps its token
// and the name it signed in with, is told by its header.
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
		playsAsFile(t, p.url, p.movie, authorized(header), answer, file)
	}
	for _, version := range []string{"2.4.0", "2.5.0-beta.1", "2.6.2"} {
		answer := p.androidPlaybackInfo(t, androidHeaderOf(androidAppBefore27, version, token), p.movie, "", "jellyfin-android-integrated", 7*time.Second)
		playsAsFile(t, p.url, p.movie, oldPlayerHeader(version), answer, file)
	}
	for _, version := range []string{"2.3.3", "2.3.2"} {
		header := androidHeaderOf(androidAppBefore27, version, token)
		for _, source := range p.androidPlaybackInfo(t, header, p.movie, "", "jellyfin-android-integrated", 0).MediaSources {
			if source.Protocol != "Http" || source.Path == "" {
				t.Errorf("%s: Protocol %q, Path %q", version, source.Protocol, source.Path)
			}
		}
	}
	// The same sign-in, the app updated to 2.7.3.
	answer := p.androidPlaybackInfo(t, androidHeader(token), p.movie, "", "jellyfin-android-2.7.3-integrated", 7*time.Second)
	playsAsFile(t, p.url, p.movie, authorized(androidHeader(token)), answer, file)
}

// Versions 2.4.0 to 2.6.2 of the app fetch a file with no token, its
// play session alone in the address (#867): an addon's link on a public
// address, which Polyfin sends other apps to, is relayed to them, as
// their player refuses a redirect from http to https, and its bytes
// count for the device that asked PlaybackInfo, as Now playing shows.
// 2.4.x names no version, later versions the title's own identifier.
func TestTheOldAndroidIntegratedPlayerRelaysAddonLinksForItsDevice(t *testing.T) {
	t.Parallel()
	const file = "\x1a\x45\xdf\xa3 media bytes"
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "", time.Time{}, strings.NewReader(file))
	}))
	t.Cleanup(host.Close)
	manifest := failoverAddon(t, "catalog", []stremio.Stream{{Name: "Linked", URL: "http://" + publicSource + "/files/movie.mp4",
		BehaviorHints: stremio.StreamBehavior{Filename: "Movie.2026.mp4"}}})
	sent, local := bandwidth.New(), strings.TrimPrefix(host.URL, "http://")
	s := newServerReaching(t, 10, "ffprobe-not-installed", func(o source.Opener) source.Opener { return reachedAt{Opener: o, local: local} },
		func(o *Options, _ *pgxpool.Pool) { o.Sent = sent })
	user := s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), manifest, false); err != nil {
		t.Fatal(err)
	}
	s.setting(t, func(settings *accounts.Settings) { settings.SyncAddonSubtitles = false })
	token := s.signInAs(t, "member", androidHeaderOf(androidAppBefore27, "2.4.0", ""))
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	movie := page.Items[0].Id
	id, _ := accounts.ParseID(movie)
	versions, err := s.library.Versions(t.Context(), user, id)
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	s.listed(t, user, movie)
	playbackSetup{testServer: s, user: user}.analyzed(t, versions[0], "h264-aac-mp4")

	for _, version := range []string{"2.4.0", "2.5.3", "2.6.2"} {
		header := androidHeaderOf(androidAppBefore27, version, token)
		named := ""
		if version != "2.4.0" {
			named = strings.ReplaceAll(mustGUID(t, movie), "-", "")
		}
		answer := s.androidPlaybackInfo(t, header, movie, named, "jellyfin-android-integrated", 0)
		played := integratedSource(t, movie, answer)
		if played.Protocol != "File" || !played.SupportsDirectPlay {
			t.Fatalf("%s: Protocol %q, direct play %v", version, played.Protocol, played.SupportsDirectPlay)
		}
		if status, body := s.call(http.MethodPost, "/Sessions/Playing", header, map[string]any{"ItemId": movie, "MediaSourceId": played.Id,
			"PlaySessionId": answer.PlaySessionId, "PositionTicks": 0, "PlayMethod": "DirectPlay"}); status != http.StatusNoContent {
			t.Fatalf("%s: start: %d %s", version, status, body)
		}
		sessions, err := s.handler.LiveSessions(t.Context())
		if err != nil || len(sessions) != 1 {
			t.Fatalf("%s: live sessions: %+v %v", version, sessions, err)
		}
		device := sessions[0].Device.ID.String()
		// A rate needs two samples: before the play reads, and after.
		sent.Release(sent.Hold(device))
		sent.Sample()
		before := sent.Total(device)

		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, integratedPlayerURL(s.url, movie, answer.PlaySessionId, played), nil)
		request.Header = oldPlayerHeader(version)
		response, err := noRedirects.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || string(got) != file {
			t.Fatalf("%s: %d to %q, %q; want the file relayed", version, response.StatusCode, response.Header.Get("Location"), got)
		}
		sent.Sample()
		if total := sent.Total(device) - before; total != int64(len(file)) {
			t.Errorf("%s: the device was sent %d bytes, want %d", version, total, len(file))
		}
		sessions, err = s.handler.LiveSessions(t.Context())
		if err != nil || len(sessions) != 1 || sessions[0].Sent <= 0 || sessions[0].Redirected {
			t.Errorf("%s: Now playing %+v %v, want the play sending bytes, not redirected", version, sessions, err)
		}
		if status, body := s.call(http.MethodPost, "/Sessions/Playing/Stopped", header, map[string]any{"ItemId": movie, "MediaSourceId": played.Id,
			"PlaySessionId": answer.PlaySessionId, "PositionTicks": 0}); status != http.StatusNoContent {
			t.Fatalf("%s: stop: %d %s", version, status, body)
		}
	}
}
