package jellyfin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/torrent"
	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// torrentTitles is a server whose addon lists two movies, each with one
// torrent version seeded by its own peer, played through a local proxy
// by users allowed torrents. Torrents linger as long as by default.
type torrentTitles struct {
	testServer
	engine *torrent.Engine
	// pieces is where the engine keeps the pieces.
	pieces   string
	seeders  map[string]*torrenttest.Peer
	tors     map[string]*torrenttest.Torrent
	movies   map[string]string
	versions map[string]library.Version
}

// switchTitles are the two movies.
var switchTitles = []string{"Copper Valley", "Iron Coast"}

func newTorrentTitles(t *testing.T) torrentTitles {
	t.Helper()
	p := torrentTitles{seeders: map[string]*torrenttest.Peer{}, tors: map[string]*torrenttest.Torrent{}, movies: map[string]string{},
		versions: map[string]library.Version{}}
	proxy := torrenttest.StartProxy(t, true, "", "")
	trackers := map[string]string{}
	for i, name := range switchTitles {
		tor := torrenttest.Build(name, 32<<10, torrenttest.File{Name: "movie.mkv", Data: torrenttest.Bytes((300 + i) << 10)})
		p.tors[name] = tor
		p.seeders[name] = torrenttest.Seed(t, tor, "127.0.0.1")
		trackers[name] = torrenttest.HTTPTracker(t, "127.0.0.1", p.seeders[name].Address()).URL()
	}
	p.pieces = t.TempDir()
	p.testServer = newProbingServer(t, 10, "ffprobe-not-installed", func(options *Options, _ *pgxpool.Pool) {
		var err error
		p.engine, err = torrent.New(torrent.Config{Settings: func() torrent.Settings { return playback.TorrentSettings(options.Accounts.Settings()) },
			Dir: p.pieces, Budget: func() int64 { return 64 << 20 }, Logger: slog.New(slog.DiscardHandler)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.engine.Close)
		options.Playback.Torrents(p.engine)
	})
	metas := map[string]stremio.Meta{}
	for i, name := range switchTitles {
		metas["tt510"+string(rune('1'+i))] = stremio.Meta{ID: "tt510" + string(rune('1'+i)), Type: "movie", Name: name, Year: "2008"}
	}
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		id := strings.TrimSuffix(path[strings.LastIndex(path, "/")+1:], ".json")
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "torrents", Name: "Torrents", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{metas["tt5101"], metas["tt5102"]}})
		case strings.HasPrefix(path, "/meta/movie/") && metas[id].ID != "":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": metas[id]})
		case strings.HasPrefix(path, "/stream/movie/") && metas[id].ID != "":
			name := metas[id].Name
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{
				{Name: "Torrent", InfoHash: p.tors[name].HashHex(), Sources: []string{"tracker:" + trackers[name]}},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(addon.Close)
	allowed := func(c *accounts.UserChanges) { c.TorrentStreams = new(true) }
	member := p.user("member", allowed)
	p.user("other", allowed)
	if _, err := p.addons.Install(t.Context(), addons.Shared(), addon.URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	p.setting(t, func(settings *accounts.Settings) {
		settings.TorrentStreams = true
		settings.TorrentProxy = proxy.Address()
	})
	token := p.signIn("member", "web")
	var views, page QueryResult
	p.get(t, "/UserViews", token, &views)
	p.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	for _, item := range page.Items {
		p.movies[item.Name] = item.Id
	}
	setup := playbackSetup{testServer: p.testServer}
	for _, name := range switchTitles {
		id, _ := accounts.ParseID(p.movies[name])
		versions, err := p.library.Versions(t.Context(), member, id)
		if err != nil || len(versions) != 1 {
			t.Fatalf("versions of %s: %+v %v", name, versions, err)
		}
		p.versions[name] = versions[0]
		setup.analyzed(t, versions[0], "h264-aac-mp4")
	}
	return p
}

// play has a device of a user play a title as an app playing it as it
// is does: it reads the version's file whole, then reports the playback.
func (p torrentTitles) play(t *testing.T, device, token, name string) {
	t.Helper()
	version := p.versions[name]
	response, body := fetchURL(t, p.url+"/Videos/"+p.movies[name]+"/stream?static=true&mediaSourceId="+version.ID.String()+"&ApiKey="+token, nil)
	if want := p.tors[name].Data; response.StatusCode != http.StatusOK || body != string(want) {
		t.Fatalf("%s: HTTP %d, %d bytes, not the file's %d", name, response.StatusCode, len(body), len(want))
	}
	p.report(t, device, token, "/Sessions/Playing", name, nil)
}

// report sends a playback report of a title, with more fields if any.
func (p torrentTitles) report(t *testing.T, device, token, path, name string, more map[string]any) {
	t.Helper()
	body := map[string]any{"ItemId": p.movies[name], "MediaSourceId": p.versions[name].ID.String(), "PositionTicks": 0,
		"PlayMethod": "DirectPlay"}
	for key, value := range more {
		body[key] = value
	}
	if status, data := p.call(http.MethodPost, path, app(device, token), body); status != http.StatusNoContent {
		t.Fatalf("%s of %s: %d %s", path, name, status, data)
	}
}

// running reports whether the torrent of a title runs.
func (p torrentTitles) running(t *testing.T, name string) bool {
	t.Helper()
	hash, err := torrent.ParseHash(p.tors[name].HashHex())
	if err != nil {
		t.Fatal(err)
	}
	return p.engine.Running(hash)
}

// kept counts the pieces of a title's torrent on disk.
func (p torrentTitles) kept(t *testing.T, name string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(p.pieces, p.tors[name].HashHex()))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return len(entries)
}

// A device that starts another title ends the torrent of the one it left
// at once, rather than 30 seconds after its last read: the torrent stops,
// its peers are left, and its pieces are deleted.
func TestADeviceStartingAnotherTitleEndsItsPreviousTorrent(t *testing.T) {
	t.Parallel()
	p := newTorrentTitles(t)
	token := p.signIn("member", "web")
	p.play(t, "web", token, "Copper Valley")
	p.report(t, "web", token, "/Sessions/Playing/Stopped", "Copper Valley", nil)
	if !p.running(t, "Copper Valley") || p.kept(t, "Copper Valley") == 0 {
		t.Fatal("the torrent did not linger after its playback, with its pieces")
	}

	p.play(t, "web", token, "Iron Coast")
	if p.running(t, "Copper Valley") {
		t.Fatal("the previous title's torrent still runs once the device started another")
	}
	if n := p.kept(t, "Copper Valley"); n != 0 {
		t.Fatalf("the previous title's torrent keeps %d pieces", n)
	}
	eventually(t, "the previous title's peer to be left", func() bool { return p.seeders["Copper Valley"].Open() == 0 })
	if !p.running(t, "Iron Coast") {
		t.Fatal("the new title's torrent does not run")
	}
	// The same user moving on from another device ends it too, once the
	// first one stopped: while it plays, the torrent is still its own.
	p.report(t, "web", token, "/Sessions/Playing/Stopped", "Iron Coast", nil)
	phone := p.signIn("member", "phone")
	p.play(t, "phone", phone, "Copper Valley")
	if p.running(t, "Iron Coast") {
		t.Fatal("the torrent the user left on another device still runs")
	}
}

// A pause, a stop and a replay of the same title, as for a seek or another
// audio track, and a late stop of the playback before, keep its torrent:
// only another title ends it.
func TestAPauseOrAReplayOfTheSameTitleKeepsItsTorrent(t *testing.T) {
	t.Parallel()
	p := newTorrentTitles(t)
	token := p.signIn("member", "web")
	p.play(t, "web", token, "Copper Valley")
	p.report(t, "web", token, "/Sessions/Playing/Progress", "Copper Valley", map[string]any{"IsPaused": true, "PositionTicks": 600_000_000})
	if !p.running(t, "Copper Valley") {
		t.Fatal("a pause ended the torrent")
	}
	p.report(t, "web", token, "/Sessions/Playing/Stopped", "Copper Valley", nil)
	p.play(t, "web", token, "Copper Valley")
	// The app restarts it again, and the stop of the playback before
	// comes late.
	p.play(t, "web", token, "Copper Valley")
	p.report(t, "web", token, "/Sessions/Playing/Stopped", "Copper Valley", nil)
	if !p.running(t, "Copper Valley") || p.kept(t, "Copper Valley") == 0 {
		t.Fatal("a replay of the same title, or a late stop, ended the torrent")
	}

	p.play(t, "web", token, "Iron Coast")
	if p.running(t, "Copper Valley") {
		t.Fatal("another title did not end the torrent")
	}
}

// Another user's playback of a title keeps its torrent when a user who
// played it too moves on.
func TestAnotherUsersPlaybackKeepsTheTorrent(t *testing.T) {
	t.Parallel()
	p := newTorrentTitles(t)
	other := p.signIn("other", "tv")
	p.play(t, "tv", other, "Copper Valley")
	token := p.signIn("member", "web")
	p.play(t, "web", token, "Copper Valley")
	p.report(t, "web", token, "/Sessions/Playing/Stopped", "Copper Valley", nil)

	p.play(t, "web", token, "Iron Coast")
	if !p.running(t, "Copper Valley") || p.kept(t, "Copper Valley") == 0 {
		t.Fatal("the torrent another user plays was ended")
	}

	// Once that user moves on too, it ends.
	p.report(t, "tv", other, "/Sessions/Playing/Stopped", "Copper Valley", nil)
	p.play(t, "tv", other, "Iron Coast")
	if p.running(t, "Copper Valley") {
		t.Fatal("the torrent nobody plays any longer still runs")
	}
}
