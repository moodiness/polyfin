package jellyfin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
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
