package jellyfin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
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

// torrentSourcesAddon serves one movie whose only stream is a torrent of
// the info hash, with the sources given.
func torrentSourcesAddon(t *testing.T, hash string, sources ...string) string {
	t.Helper()
	movie := stremio.Meta{ID: "tt7700", Type: "movie", Name: "Movie", Year: "2011"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "sources", Name: "Sources", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt7700.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt7700.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{{Name: "Torrent", InfoHash: hash, Sources: sources}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// A member allowed torrents plays a title whose torrent stream names a
// tracker on the server's loopback and one on a public address: the
// second is announced to through the route, and nothing is sent to the
// first.
func TestATorrentStreamsTrackerOnTheLoopbackIsNotAnnouncedTo(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var requests []string
	local := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		mu.Unlock()
	}))
	t.Cleanup(local.Close)
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1")
	addon := torrentSourcesAddon(t, strings.Repeat("ab", 20), "tracker:"+local.URL+"/router/reboot?now=1", "tracker:"+tracker.PublicURL())

	proxy := torrenttest.StartProxy(t, false, "", "")
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(options *Options, _ *pgxpool.Pool) {
		engine, err := torrent.New(torrent.Config{Settings: func() torrent.Settings { return playback.TorrentSettings(options.Accounts.Settings()) },
			Dir: t.TempDir(), Budget: func() int64 { return 64 << 20 }, StartTimeout: time.Second, PieceTimeout: time.Second,
			Logger: slog.New(slog.DiscardHandler)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(engine.Close)
		options.Playback.Torrents(engine)
	})
	member := s.user("member", func(c *accounts.UserChanges) { c.TorrentStreams = new(true) })
	if member.IsAdministrator {
		t.Fatal("the member is an administrator")
	}
	if _, err := s.addons.Install(t.Context(), addons.Shared(), addon, false); err != nil {
		t.Fatal(err)
	}
	s.setting(t, func(settings *accounts.Settings) {
		settings.TorrentStreams = true
		settings.TorrentProxy = proxy.Address()
	})

	token := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	if len(page.Items) != 1 {
		t.Fatalf("the member's library: %+v", page.Items)
	}
	// Play: PlaybackInfo, as the member's app sends it. No peer has the
	// torrent, which does not start.
	s.playbackInfo(t, "tv", token, page.Items[0].Id, nil)
	eventually(t, "the tracker on a public address to be announced to", func() bool { return len(tracker.Announces()) > 0 })
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 0 {
		t.Errorf("the tracker on the loopback received %q", requests)
	}
	for _, target := range proxy.Targets() {
		_, address, _ := strings.Cut(target, " ")
		if ap, err := netip.ParseAddrPort(address); err != nil || ap.Addr() != torrenttest.Public {
			t.Errorf("the route was asked for %q", target)
		}
	}
}
