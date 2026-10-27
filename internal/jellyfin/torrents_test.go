package jellyfin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/torrent"
	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// torrentAddon serves one movie with a link and two torrent streams of
// tor: its file 2, and, naming no file, its largest.
func torrentAddon(t *testing.T, tor *torrenttest.Torrent, tracker string) string {
	t.Helper()
	var server *httptest.Server
	movie := stremio.Meta{ID: "tt5000", Type: "movie", Name: "Movie", Year: "2008"}
	two := 2
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "torrents", Name: "Torrents", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt5000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt5000.json":
			sources := []string{"tracker:" + tracker}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{
				{Name: "Link", URL: server.URL + "/files/movie.mp4"},
				{Name: "Torrent file", InfoHash: tor.HashHex(), FileIdx: &two, Sources: sources},
				{Name: "Torrent largest", InfoHash: tor.HashHex(), Sources: sources},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// The progress of a title's versions, asked or pushed to its page, counts
// only the versions its user is offered: with torrents off, the link
// alone, so that the web player's play buttons follow what plays.
func TestVersionProgressLeavesOutTorrentsTheUserIsNotOffered(t *testing.T) {
	t.Parallel()
	tor := torrenttest.Build("A Movie", 32<<10, torrenttest.File{Name: "sample.mkv", Data: torrenttest.Bytes(40 << 10)})
	s := newTestServer(t, 10)
	user := s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), torrentAddon(t, tor, "127.0.0.1:1"), false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	movie := page.Items[0].Id
	// The details open the title's page, whose progress is then followed.
	var details BaseItemDto
	if status := s.get(t, "/Users/"+user.ID.String()+"/Items/"+movie, token, &details); status != http.StatusOK {
		t.Fatalf("details: %d", status)
	}
	var progress VersionProgress
	eventually(t, "the addon to answer", func() bool {
		if status := s.get(t, "/Polyfin/Items/"+movie+"/Versions", token, &progress); status != http.StatusOK {
			t.Fatalf("progress: %d", status)
		}
		return progress.Pending == 0 && progress.Known > 0
	})
	if progress != (VersionProgress{Pending: 0, Count: 1, Known: 1}) {
		t.Errorf("progress with torrents off: %+v, want the link alone", progress)
	}
}

// Torrent versions are listed only with the switch and the user's
// permission, play from their file through the route, and never download.
func TestTorrentVersions(t *testing.T) {
	t.Parallel()
	small, large, medium := torrenttest.Bytes(40<<10), torrenttest.Bytes(300<<10), torrenttest.Bytes(200<<10)
	tor := torrenttest.Build("A Movie", 32<<10, torrenttest.File{Name: "sample.mkv", Data: small},
		torrenttest.File{Name: "a-movie.mkv", Data: large}, torrenttest.File{Name: "extra.mkv", Data: medium})
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	proxy := torrenttest.StartProxy(t, true, "", "")
	var engine *torrent.Engine
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(options *Options, _ *pgxpool.Pool) {
		var err error
		engine, err = torrent.New(torrent.Config{Settings: func() torrent.Settings { return playback.TorrentSettings(options.Accounts.Settings()) },
			Dir: t.TempDir(), Budget: func() int64 { return 64 << 20 }, Logger: slog.New(slog.DiscardHandler)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(engine.Close)
		options.Playback.Torrents(engine)
	})
	member := s.user("member", func(c *accounts.UserChanges) { c.ContentDownloading = new(true) })
	allowed := s.user("allowed", func(c *accounts.UserChanges) {
		c.ContentDownloading = new(true)
		c.TorrentStreams = new(true)
	})
	if _, err := s.addons.Install(t.Context(), addons.Shared(), torrentAddon(t, tor, tracker.URL()), false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("allowed", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	movie, _ := accounts.ParseID(page.Items[0].Id)
	names := func(user accounts.User) []string {
		t.Helper()
		versions, err := s.library.Versions(t.Context(), user, movie)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, v := range versions {
			names = append(names, v.Name)
		}
		return names
	}

	// The switch off: no torrent version, whoever asks.
	if got := names(allowed); !slices.Equal(got, []string{"Link"}) {
		t.Fatalf("with the switch off: %q", got)
	}
	// The switch cannot be on without a route.
	settings := s.store.Settings()
	settings.TorrentStreams = true
	if _, err := s.store.UpdateSettings(t.Context(), settings); err == nil {
		t.Fatal("torrents were turned on without a route")
	}
	s.setting(t, func(settings *accounts.Settings) {
		settings.TorrentStreams = true
		settings.TorrentProxy = proxy.Address()
	})
	if got := names(member); !slices.Equal(got, []string{"Link"}) {
		t.Fatalf("without the permission: %q", got)
	}
	if got := names(allowed); !slices.Equal(got, []string{"Link", "Torrent file", "Torrent largest"}) {
		t.Fatalf("with the switch and the permission: %q", got)
	}
	versions, _ := s.library.Versions(t.Context(), allowed, movie)
	p := playbackSetup{testServer: s, token: token, user: allowed, movie: page.Items[0].Id, versions: versions}
	// A version listed to another user is no way around the permission.
	if _, err := s.library.Version(t.Context(), member, movie, versions[1].ID); err == nil {
		t.Fatal("a torrent version was found for a user without the permission")
	}

	for i, want := range map[int][]byte{1: medium, 2: large} {
		p.analyzed(t, versions[i], "h264-aac-mp4")
		response, body := fetchURL(t, s.url+"/Videos/"+p.movie+"/stream?static=true&mediaSourceId="+versions[i].ID.String()+"&ApiKey="+token, nil)
		if response.StatusCode != http.StatusOK || body != string(want) {
			t.Fatalf("%s: HTTP %d, %d bytes, not the file's %d", versions[i].Name, response.StatusCode, len(body), len(want))
		}
		// A download is refused, permission or not.
		if response, _ := fetchURL(t, s.url+"/Items/"+versions[i].ID.String()+"/Download?ApiKey="+token, nil); response.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: a download answered HTTP %d", versions[i].Name, response.StatusCode)
		}
	}
	// The seeder was reached through the proxy alone.
	relayed := 0
	for _, target := range proxy.Targets() {
		if target == "tcp "+seeder.Address().String() {
			relayed++
		}
	}
	if got := len(seeder.Remotes()); got == 0 || got != relayed {
		t.Fatalf("the seeder saw %d connections, the proxy relayed %d", got, relayed)
	}
	// The playback joined the torrent, which runs on until it ends.
	if hash, _ := torrent.ParseHash(tor.HashHex()); !engine.Running(hash) {
		t.Fatal("the playback's torrent does not run")
	}
}

// A torrent version is skipped while its route is down, with nothing sent,
// and plays again as soon as the route is back: the route's failure is
// not the version's.
func TestATorrentVersionPlaysAgainOnceItsRouteIsBack(t *testing.T) {
	t.Parallel()
	small, large, medium := torrenttest.Bytes(40<<10), torrenttest.Bytes(300<<10), torrenttest.Bytes(200<<10)
	tor := torrenttest.Build("A Movie", 32<<10, torrenttest.File{Name: "sample.mkv", Data: small},
		torrenttest.File{Name: "a-movie.mkv", Data: large}, torrenttest.File{Name: "extra.mkv", Data: medium})
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	proxy := torrenttest.StartProxy(t, true, "", "")
	var engine *torrent.Engine
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(options *Options, _ *pgxpool.Pool) {
		var err error
		engine, err = torrent.New(torrent.Config{Settings: func() torrent.Settings { return playback.TorrentSettings(options.Accounts.Settings()) },
			Dir: t.TempDir(), Budget: func() int64 { return 64 << 20 }, Logger: slog.New(slog.DiscardHandler)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(engine.Close)
		options.Playback.Torrents(engine)
	})
	allowed := s.user("allowed", func(c *accounts.UserChanges) { c.TorrentStreams = new(true) })
	if _, err := s.addons.Install(t.Context(), addons.Shared(), torrentAddon(t, tor, tracker.URL()), false); err != nil {
		t.Fatal(err)
	}
	s.setting(t, func(settings *accounts.Settings) {
		settings.TorrentStreams = true
		settings.TorrentProxy = proxy.Address()
	})
	token := s.signIn("allowed", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	movie, _ := accounts.ParseID(page.Items[0].Id)
	versions, err := s.library.Versions(t.Context(), allowed, movie)
	if err != nil || len(versions) != 3 {
		t.Fatalf("versions: %v %v", versions, err)
	}
	version := versions[2]
	p := playbackSetup{testServer: s, token: token, user: allowed, movie: page.Items[0].Id, versions: versions}
	p.analyzed(t, version, "h264-aac-mp4")
	offered := func() bool {
		t.Helper()
		answer := s.playbackInfo(t, "tv", token, p.movie, map[string]any{"MediaSourceId": version.ID.String()})
		return len(answer.MediaSources) == 1 && answer.MediaSources[0].Id == version.ID.String()
	}

	proxy.Stop()
	if offered() {
		t.Fatal("the torrent version was offered with its route down")
	}
	// The app is told the title cannot be played now, not that it suits
	// no app.
	if answer := s.playbackInfo(t, "tv", token, p.movie, map[string]any{"MediaSourceId": version.ID.String()}); answer.ErrorCode != "RateLimitExceeded" {
		t.Errorf("with the route down, PlaybackInfo answered %q", answer.ErrorCode)
	}
	response, _ := fetchURL(t, s.url+"/Videos/"+p.movie+"/stream?static=true&mediaSourceId="+version.ID.String()+"&ApiKey="+token, nil)
	if response.StatusCode == http.StatusOK {
		t.Fatal("the torrent version played with its route down")
	}
	if len(seeder.Remotes()) != 0 || len(tracker.Remotes()) != 0 {
		t.Fatal("the torrent was reached with its route down")
	}

	proxy.Resume(t)
	// The proxy is checked again within seconds.
	deadline := time.Now().Add(time.Minute)
	for engine.Ready(t.Context()) != nil {
		if time.Now().After(deadline) {
			t.Fatal("the route is not seen back")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if status := engine.Status(t.Context()); status.RouteError != nil {
		t.Fatalf("Health still tells %v", status.RouteError)
	}
	if !offered() {
		t.Fatal("the torrent version is not offered once its route is back")
	}
	response, body := fetchURL(t, s.url+"/Videos/"+p.movie+"/stream?static=true&mediaSourceId="+version.ID.String()+"&ApiKey="+token, nil)
	if response.StatusCode != http.StatusOK || body != string(large) {
		t.Fatalf("once the route is back: HTTP %d, %d bytes", response.StatusCode, len(body))
	}
}

// mediaTorrentAddon serves one movie whose versions are torrents alone,
// each the file of one of tors announced to its tracker.
func mediaTorrentAddon(t *testing.T, tors []*torrenttest.Torrent, trackers []string) string {
	t.Helper()
	movie := stremio.Meta{ID: "tt5001", Type: "movie", Name: "Movie", Year: "2009"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "torrents", Name: "Torrents", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt5001.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt5001.json":
			var streams []stremio.Stream
			for i, tor := range tors {
				streams = append(streams, stremio.Stream{Name: "Torrent " + strconv.Itoa(i+1), InfoHash: tor.HashHex(),
					Sources: []string{"tracker:" + trackers[i]}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// torrentMovie makes a torrent of name holding twenty seconds of MPEG-4
// Part 2 video and AAC in Matroska, a video a browser plays converted
// only, with the FFmpeg of POLYFIN_TEST_FFMPEG, skipping the test without
// one.
func torrentMovie(t *testing.T, name string) (string, *torrenttest.Torrent) {
	t.Helper()
	ffmpeg, file := vodFile(t, "a-movie.mkv", "-c:v", "mpeg4")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return ffmpeg, torrenttest.Build(name, 64<<10, torrenttest.File{Name: "a-movie.mkv", Data: data})
}

// torrentPlay is a server playing torrents through a local proxy, and a
// user allowed torrents, signed in as jellyfin-web.
type torrentPlay struct {
	testServer
	engine   *torrent.Engine
	proxy    *torrenttest.Proxy
	user     accounts.User
	token    string
	movie    string
	versions []library.Version
}

// newTorrentPlay starts a server whose analyses run ffprobe, and whose
// addon lists tors, each announced to its tracker. A torrent's start is
// waited for start, 0 for the engine's default. configure, if any,
// completes the handler's options.
func newTorrentPlay(t *testing.T, ffprobe string, start time.Duration, tors []*torrenttest.Torrent, trackers []string,
	configure ...func(*Options)) torrentPlay {
	t.Helper()
	p := torrentPlay{proxy: torrenttest.StartProxy(t, true, "", "")}
	p.testServer = newProbingServer(t, 10, ffprobe, func(options *Options, _ *pgxpool.Pool) {
		var err error
		p.engine, err = torrent.New(torrent.Config{Settings: func() torrent.Settings { return playback.TorrentSettings(options.Accounts.Settings()) },
			Dir: t.TempDir(), Budget: func() int64 { return 256 << 20 }, StartTimeout: start, Logger: slog.New(slog.DiscardHandler)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.engine.Close)
		options.Playback.Torrents(p.engine)
		for _, c := range configure {
			c(options)
		}
	})
	p.user = p.testServer.user("allowed", func(c *accounts.UserChanges) { c.TorrentStreams = new(true) })
	if _, err := p.addons.Install(t.Context(), addons.Shared(), mediaTorrentAddon(t, tors, trackers), false); err != nil {
		t.Fatal(err)
	}
	p.setting(t, func(settings *accounts.Settings) {
		settings.TorrentStreams = true
		settings.TorrentProxy = p.proxy.Address()
	})
	p.token = p.signIn("allowed", "web")
	var views, page QueryResult
	p.get(t, "/UserViews", p.token, &views)
	p.get(t, "/Items?ParentId="+views.Items[0].Id, p.token, &page)
	p.movie = page.Items[0].Id
	movie, _ := accounts.ParseID(p.movie)
	versions, err := p.library.Versions(t.Context(), p.user, movie)
	if err != nil || len(versions) != len(tors) {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	p.versions = versions
	return p
}

// details opens the title's page, as jellyfin-web does before Play, and
// returns the identifier of the version it picks, the first.
func (p torrentPlay) details(t *testing.T) string {
	t.Helper()
	var details BaseItemDto
	if status := p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &details); status != http.StatusOK ||
		details.MediaSources == nil || len(*details.MediaSources) == 0 {
		t.Fatalf("details: %d %+v", status, details.MediaSources)
	}
	return (*details.MediaSources)[0].Id
}

// play asks PlaybackInfo as jellyfin-web does when Play is pressed on the
// title's page: the version picked there, if any, the browser's device
// profile, IsPlayback and AutoOpenLiveStream in the query. It returns the
// answer and its ErrorCode, empty for one that plays.
func (p torrentPlay) play(t *testing.T, mediaSourceID string) (playbackInfoResponse, string) {
	t.Helper()
	query := url.Values{"UserId": {p.user.ID.String()}, "StartTimeTicks": {"0"}, "IsPlayback": {"true"}, "AutoOpenLiveStream": {"true"},
		"MaxStreamingBitrate": {"120000000"}}
	if mediaSourceID != "" {
		query.Set("MediaSourceId", mediaSourceID)
	}
	status, body := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo?"+query.Encode(), app("web", p.token),
		map[string]any{"DeviceProfile": deviceProfile(t, "jellyfin-web-chrome")})
	var info playbackInfoResponse
	var failed noCompatibleStream
	if status != http.StatusOK || json.Unmarshal(body, &info) != nil || json.Unmarshal(body, &failed) != nil {
		t.Fatalf("PlaybackInfo: %d %s", status, body)
	}
	return info, failed.ErrorCode
}

// converted checks that a PlaybackInfo answer offers the browser a
// version converted, the one of id, and that its segments are served.
func (p torrentPlay) converted(t *testing.T, info playbackInfoResponse, id accounts.ID) {
	t.Helper()
	for _, source := range info.MediaSources {
		if source.TranscodingUrl == "" {
			continue
		}
		if source.Id != id.String() && source.Id != p.movie {
			t.Fatalf("the browser was offered %s, not %s", source.Id, id)
		}
		fetchSegments(t, p.testServer, source.TranscodingUrl, 0, 1)
		return
	}
	t.Fatalf("the browser was offered no way to play the torrent: %+v", info.MediaSources)
}

// reached reports whether the proxy was asked to reach a peer.
func (p torrentPlay) reached(peer *torrenttest.Peer) bool {
	return slices.Contains(p.proxy.Targets(), "tcp "+peer.Address().String())
}

// A torrent version never analyzed plays from the first press of Play in
// jellyfin-web, with playback prepared ahead, as by default: opening the
// title's page leaves the torrent alone, Play joins its swarm, its
// analysis runs inside that playback, and PlaybackInfo answers a source
// the browser plays, here converted.
func TestATorrentVersionNeverAnalyzedPlaysFromItsFirstPlay(t *testing.T) {
	t.Parallel()
	ffmpeg, tor := torrentMovie(t, "A Movie")
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	p := newTorrentPlay(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), 0, []*torrenttest.Torrent{tor}, []string{tracker.URL()})
	p.setting(t, func(s *accounts.Settings) { s.PrepareAhead = true })
	t.Cleanup(func() { p.settle(t) })

	picked := p.details(t)
	p.settle(t)
	// The user reads the page for a while: long enough for a read of the
	// torrent in the background, had one started, to fail the version's
	// source past the cache's tries.
	src := p.handler.Playback.OpenSource(p.versions[0])
	defer src.Release()
	for deadline := time.Now().Add(8 * time.Second); src.Failure() == nil && time.Now().Before(deadline); {
		time.Sleep(50 * time.Millisecond)
	}
	if len(tracker.Remotes()) != 0 {
		t.Fatal("opening the title's page joined the torrent's swarm")
	}

	info, code := p.play(t, picked)
	if code != "" {
		t.Fatalf("PlaybackInfo answered %s", code)
	}
	p.converted(t, info, p.versions[0].ID)
	if hash, _ := torrent.ParseHash(tor.HashHex()); !p.engine.Running(hash) {
		t.Fatal("the playback's torrent does not run")
	}
	if !p.reached(seeder) {
		t.Fatal("the seeder was not reached through the proxy")
	}
}

// A torrent whose route is slow to carry it, as a remote proxy granting
// UDP late, is read once its playback joined the swarm, however long that
// takes: nothing reads it before and fails it meanwhile, such as the
// keyframe index read ahead.
func TestATorrentSlowToJoinIsReadOnceJoined(t *testing.T) {
	t.Parallel()
	ffmpeg, tor := torrentMovie(t, "A Movie")
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	p := newTorrentPlay(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), 0, []*torrenttest.Torrent{tor}, []string{tracker.URL()})
	picked := p.details(t)
	// Longer than the source cache tries a file that does not open.
	p.proxy.DelayUDP(6 * time.Second)
	info, code := p.play(t, picked)
	if code != "" {
		t.Fatalf("PlaybackInfo answered %s", code)
	}
	p.converted(t, info, p.versions[0].ID)
}

// Among a title's torrent versions, those after the first are tried as
// the play's too, joining their swarms: the first, whose swarm has no
// peer, does not start, and the second plays.
func TestATorrentVersionAfterTheFirstPlaysWhenTheFirstCannotStart(t *testing.T) {
	t.Parallel()
	ffmpeg, live := torrentMovie(t, "A Movie")
	dead := torrenttest.Build("Another Movie", 64<<10, torrenttest.File{Name: "another.mkv", Data: torrenttest.Bytes(300 << 10)})
	seeder := torrenttest.Seed(t, live, "127.0.0.1")
	empty := torrenttest.HTTPTracker(t, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	p := newTorrentPlay(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), 2*time.Second,
		[]*torrenttest.Torrent{dead, live}, []string{empty.URL(), tracker.URL()})
	p.details(t)
	info, code := p.play(t, "")
	if code != "" {
		t.Fatalf("PlaybackInfo answered %s", code)
	}
	p.converted(t, info, p.versions[1].ID)
	if len(empty.Announces()) == 0 {
		t.Error("the first version's swarm was not joined")
	}
}

// A torrent that cannot start, no peer sending it in time, answers what
// jellyfin-web tells as media that cannot be played at this time, rather
// than as media no app plays; it does again while the version is left out,
// and Health tells why.
func TestATorrentThatCannotStartCannotBePlayedNow(t *testing.T) {
	t.Parallel()
	dead := torrenttest.Build("A Movie", 64<<10, torrenttest.File{Name: "a-movie.mkv", Data: torrenttest.Bytes(300 << 10)})
	empty := torrenttest.HTTPTracker(t, "127.0.0.1")
	p := newTorrentPlay(t, "ffprobe-not-installed", 2*time.Second, []*torrenttest.Torrent{dead}, []string{empty.URL()})
	picked := p.details(t)
	for _, when := range []string{"once tried", "while left out"} {
		if info, code := p.play(t, picked); code != "RateLimitExceeded" || len(info.MediaSources) != 0 {
			t.Fatalf("%s: answered %q, %d sources", when, code, len(info.MediaSources))
		}
	}
	status := p.engine.Status(t.Context())
	if status.Unstarted == nil || status.Unstarted.Found != 0 || status.Unstarted.Answered != 1 {
		t.Fatalf("Health tells %+v", status.Unstarted)
	}
}

// A torrent version whose peer sends nothing past its start does not hold
// PlaybackInfo for its whole analysis: the play answers, once the start
// and a piece's wait passed, that the title cannot be played now, as for a
// torrent that did not start, whatever the analysis time allowed. The
// analysis goes on, and the next play plays the version once it is in.
func TestATorrentPlayAnswersOnceItsStartAndAPiecesWaitPassed(t *testing.T) {
	t.Parallel()
	ffmpeg, tor := torrentMovie(t, "A Movie")
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	const start = 2 * time.Second
	p := newTorrentPlay(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), start, []*torrenttest.Torrent{tor}, []string{tracker.URL()})
	p.setting(t, func(s *accounts.Settings) { s.AnalysisTimeout = accounts.MaxAnalysisTimeout })
	picked := p.details(t)
	// The peer tells the torrent's files, then holds every piece.
	seeder.Hold()
	defer seeder.Release()

	type answer struct {
		status int
		body   []byte
		took   time.Duration
	}
	answered := make(chan answer, 1)
	profile := deviceProfile(t, "jellyfin-web-chrome")
	go func() {
		started := time.Now()
		query := url.Values{"UserId": {p.user.ID.String()}, "StartTimeTicks": {"0"}, "IsPlayback": {"true"}, "AutoOpenLiveStream": {"true"},
			"MaxStreamingBitrate": {"120000000"}, "MediaSourceId": {picked}}
		status, body := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo?"+query.Encode(), app("web", p.token),
			map[string]any{"DeviceProfile": profile})
		answered <- answer{status, body, time.Since(started)}
	}()
	// The start, a piece's wait, and a margin for a busy machine; the
	// analysis alone was allowed 120 seconds.
	wait := start + torrent.PieceTimeout + 30*time.Second
	select {
	case a := <-answered:
		var failed noCompatibleStream
		if a.status != http.StatusOK || json.Unmarshal(a.body, &failed) != nil || failed.ErrorCode != "RateLimitExceeded" {
			t.Fatalf("PlaybackInfo answered %d %.200s after %v", a.status, a.body, a.took)
		}
	case <-time.After(wait):
		t.Fatalf("PlaybackInfo did not answer within %v", wait)
	}

	seeder.Release()
	eventually(t, "the analysis to go on", func() bool { _, ok := p.handler.Playback.Analyzed(t.Context(), p.versions[0].ID); return ok })
	if info, code := p.play(t, picked); code != "" || len(info.MediaSources) == 0 {
		t.Fatalf("the next play answered %q", code)
	}
}
