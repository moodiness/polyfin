package jellyfin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/torrent"
	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// packAddon serves a series of three episodes whose streams are all the
// season's pack, with no file index: the first's names its file, the
// others name none.
func packAddon(t *testing.T, pack *torrenttest.Torrent, tracker string) string {
	t.Helper()
	show := stremio.Meta{ID: "tt6000", Type: "series", Name: "Harbor Tales", Videos: []stremio.Video{
		{ID: "tt6000:1:1", Title: "First", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00.000Z"},
		{ID: "tt6000:1:2", Title: "Second", Season: 1, Episode: 2, Released: "2020-01-08T00:00:00.000Z"},
		{ID: "tt6000:1:3", Title: "Third", Season: 1, Episode: 3, Released: "2020-01-15T00:00:00.000Z"},
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.Path; {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "packs", Name: "Packs", Version: "1",
				Types: []string{"series"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "series", ID: "shows", Name: "Shows"}}})
		case strings.HasPrefix(path, "/catalog/series/shows"):
			preview := show
			preview.Videos = nil
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{preview}})
		case path == "/meta/series/tt6000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": show})
		case strings.HasPrefix(path, "/stream/series/tt6000:1:"):
			stream := stremio.Stream{Name: "Season pack", InfoHash: pack.HashHex(), Sources: []string{"tracker:" + tracker}}
			if path == "/stream/series/tt6000:1:1.json" {
				stream.BehaviorHints.Filename = "Harbor.Tales.S01E01.1080p.mkv"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{stream}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// A season's pack an addon lists without a file index plays the episode
// asked: the file the stream's filename names, else the one whose name
// tells the episode, rather than the pack's largest file, another
// episode.
func TestASeasonPackWithoutAnIndexPlaysTheEpisode(t *testing.T) {
	t.Parallel()
	episodes := [][]byte{torrenttest.Bytes(90 << 10), torrenttest.Bytes(110 << 10), torrenttest.Bytes(200 << 10)}
	pack := torrenttest.Build("Harbor.Tales.S01.1080p", 32<<10,
		torrenttest.File{Name: "Harbor.Tales.S01.1080p.nfo", Data: torrenttest.Bytes(1 << 10)},
		torrenttest.File{Name: "Harbor.Tales.S01E01.1080p.mkv", Data: episodes[0]},
		torrenttest.File{Name: "Harbor.Tales.S01E02.1080p.mkv", Data: episodes[1]},
		torrenttest.File{Name: "Harbor.Tales.S01E03.1080p.mkv", Data: episodes[2]})
	seeder := torrenttest.Seed(t, pack, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	proxy := torrenttest.StartProxy(t, true, "", "")
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(options *Options, _ *pgxpool.Pool) {
		engine, err := torrent.New(torrent.Config{Settings: func() torrent.Settings { return playback.TorrentSettings(options.Accounts.Settings()) },
			Dir: t.TempDir(), Budget: func() int64 { return 64 << 20 }, Logger: slog.New(slog.DiscardHandler)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(engine.Close)
		options.Playback.Torrents(engine)
	})
	user := s.user("allowed", func(c *accounts.UserChanges) { c.TorrentStreams = new(true) })
	addon, err := s.addons.Install(t.Context(), addons.Shared(), packAddon(t, pack, tracker.URL()), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"}}); err != nil {
		t.Fatal(err)
	}
	s.setting(t, func(settings *accounts.Settings) {
		settings.TorrentStreams = true
		settings.TorrentProxy = proxy.Address()
	})
	token := s.signIn("allowed", "tv")
	var views, page, listed QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	s.get(t, "/Shows/"+page.Items[0].Id+"/Episodes", token, &listed)
	ids := map[string]string{}
	for _, episode := range listed.Items {
		ids[episode.Name] = episode.Id
	}
	p := playbackSetup{testServer: s, token: token, user: user}
	// The first by the file its stream names, the second by its name's
	// S01E02.
	for i, name := range []string{"First", "Second"} {
		id, err := accounts.ParseID(ids[name])
		if err != nil {
			t.Fatalf("%s is not listed: %v", name, listed.Items)
		}
		versions, err := s.library.Versions(t.Context(), user, id)
		if err != nil || len(versions) != 1 {
			t.Fatalf("%s: versions %+v %v", name, versions, err)
		}
		p.analyzed(t, versions[0], "h264-aac-mp4")
		response, body := fetchURL(t, s.url+"/Videos/"+ids[name]+"/stream?static=true&mediaSourceId="+versions[0].ID.String()+
			"&ApiKey="+token, nil)
		if response.StatusCode != http.StatusOK || body != string(episodes[i]) {
			t.Errorf("%s: HTTP %d, %d bytes, not the episode's %d", name, response.StatusCode, len(body), len(episodes[i]))
		}
	}
}
