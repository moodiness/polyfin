package jellyfin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

// capTitles are three movies whose first version is a torrent, each
// seeded by its own peer, Copper Valley with a link after its torrent,
// played through a local proxy by two users allowed torrents; link is that
// link's version. Torrents linger as long as given, 30 seconds for 0.
type capTitles struct {
	torrentTitles
	link library.Version
}

var capNames = []string{"Copper Valley", "Iron Coast", "Signal Fire"}

func newCapTitles(t *testing.T, linger time.Duration) capTitles {
	t.Helper()
	p := capTitles{torrentTitles: torrentTitles{seeders: map[string]*torrenttest.Peer{}, tors: map[string]*torrenttest.Torrent{},
		movies: map[string]string{}, versions: map[string]library.Version{}}}
	proxy := torrenttest.StartProxy(t, true, "", "")
	trackers := map[string]string{}
	for i, name := range capNames {
		tor := torrenttest.Build(name, 32<<10, torrenttest.File{Name: "movie.mkv", Data: torrenttest.Bytes((300 + i) << 10)})
		p.tors[name] = tor
		p.seeders[name] = torrenttest.Seed(t, tor, "127.0.0.1")
		trackers[name] = torrenttest.HTTPTracker(t, "127.0.0.1", p.seeders[name].Address()).URL()
	}
	p.pieces = t.TempDir()
	p.testServer = newProbingServer(t, 10, "ffprobe-not-installed", func(options *Options, _ *pgxpool.Pool) {
		var err error
		p.engine, err = torrent.New(torrent.Config{Settings: func() torrent.Settings { return playback.TorrentSettings(options.Accounts.Settings()) },
			Dir: p.pieces, Budget: func() int64 { return 64 << 20 }, Linger: linger, Logger: slog.New(slog.DiscardHandler)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.engine.Close)
		options.Playback.Torrents(p.engine)
	})
	files := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "movie.mkv", time.Time{}, strings.NewReader(string(torrenttest.Bytes(200<<10))))
	}))
	t.Cleanup(files.Close)
	metas := map[string]stremio.Meta{}
	for i, name := range capNames {
		id := "tt520" + string(rune('1'+i))
		metas[id] = stremio.Meta{ID: id, Type: "movie", Name: name, Year: "2008"}
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
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{metas["tt5201"], metas["tt5202"], metas["tt5203"]}})
		case strings.HasPrefix(path, "/meta/movie/") && metas[id].ID != "":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": metas[id]})
		case strings.HasPrefix(path, "/stream/movie/") && metas[id].ID != "":
			name := metas[id].Name
			streams := []stremio.Stream{{Name: "Torrent", InfoHash: p.tors[name].HashHex(), Sources: []string{"tracker:" + trackers[name]}}}
			if name == "Copper Valley" {
				streams = append(streams, stremio.Stream{Name: "Link", URL: files.URL + "/movie.mkv"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
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
	for _, name := range capNames {
		id, _ := accounts.ParseID(p.movies[name])
		versions, err := p.library.Versions(t.Context(), member, id)
		if err != nil || len(versions) == 0 || versions[0].Torrent == nil {
			t.Fatalf("versions of %s: %+v %v", name, versions, err)
		}
		p.versions[name] = versions[0]
		for _, version := range versions {
			setup.analyzed(t, version, "h264-aac-mp4")
			if version.Torrent == nil {
				p.link = version
			}
		}
	}
	return p
}

// cap sets Torrents at once.
func (p capTitles) cap(t *testing.T, n int) {
	t.Helper()
	p.setting(t, func(settings *accounts.Settings) { settings.TorrentsAtOnce = n })
}

// ask asks PlaybackInfo for a title from a device, naming no version.
func (p capTitles) ask(t *testing.T, device, token, name string) playbackAnswer {
	t.Helper()
	return p.playbackInfo(t, device, token, p.movies[name], map[string]any{"DeviceProfile": deviceProfile(t, "jellyfin-web-chrome")})
}

// hold has a playback join the torrent of a title until the function it
// returns is called.
func (p capTitles) hold(t *testing.T, name string) func() {
	t.Helper()
	spec, err := torrent.NewSpec(p.tors[name].HashHex(), nil, p.versions[name].Torrent.Sources)
	if err != nil {
		t.Fatal(err)
	}
	return p.engine.Play(spec)
}

// At the cap, a title's torrent version does not start: the play takes its
// link version after it. With no limit, the torrent plays beside the other.
func TestATorrentAtTheCapGivesWayToALinkVersion(t *testing.T) {
	t.Parallel()
	p := newCapTitles(t, 0)
	p.cap(t, 1)
	other := p.signIn("other", "tv")
	p.play(t, "tv", other, "Iron Coast")
	token := p.signIn("member", "web")

	answer := p.ask(t, "web", token, "Copper Valley")
	if answer.ErrorCode != "" || len(answer.MediaSources) == 0 || firstID(answer) != p.link.ID.String() {
		t.Fatalf("at the cap, PlaybackInfo answered %q with %d sources, the first %v", answer.ErrorCode, len(answer.MediaSources), firstID(answer))
	}
	if p.running(t, "Copper Valley") || !p.running(t, "Iron Coast") {
		t.Fatal("a torrent started past the cap, or the one playing stopped")
	}

	// 0 is no limit.
	p.cap(t, 0)
	answer = p.ask(t, "web", token, "Copper Valley")
	if answer.ErrorCode != "" || len(answer.MediaSources) == 0 || firstID(answer) == p.link.ID.String() {
		t.Fatalf("with no limit, PlaybackInfo answered %q, the first source %v", answer.ErrorCode, firstID(answer))
	}
	if !p.running(t, "Copper Valley") || !p.running(t, "Iron Coast") {
		t.Fatal("with no limit, both torrents do not run")
	}
}

// At the cap, a title whose versions are all torrents cannot be played now,
// as when its torrent does not start; it plays once the torrent playing
// stops, its linger passed.
func TestATorrentAtTheCapPlaysOnceAPlaceFrees(t *testing.T) {
	t.Parallel()
	p := newCapTitles(t, time.Second)
	p.cap(t, 1)
	// Another user's playback holds the only place.
	release := p.hold(t, "Iron Coast")
	defer release()
	eventually(t, "the other torrent to run", func() bool { return p.running(t, "Iron Coast") })
	token := p.signIn("member", "web")
	if answer := p.ask(t, "web", token, "Signal Fire"); answer.ErrorCode != "RateLimitExceeded" || len(answer.MediaSources) != 0 {
		t.Fatalf("at the cap, PlaybackInfo answered %q with %d sources", answer.ErrorCode, len(answer.MediaSources))
	}
	if p.running(t, "Signal Fire") {
		t.Fatal("a torrent started past the cap")
	}

	release()
	eventually(t, "the other torrent to stop after its linger", func() bool { return !p.running(t, "Iron Coast") })
	if answer := p.ask(t, "web", token, "Signal Fire"); answer.ErrorCode != "" || len(answer.MediaSources) == 0 {
		t.Fatalf("once a place freed, PlaybackInfo answered %q", answer.ErrorCode)
	}
	if !p.running(t, "Signal Fire") {
		t.Fatal("the torrent does not run once a place freed")
	}
}

// At the cap, the torrent a device left for another title, lingering after
// its playback, gives its place at once to the device's next title, as
// that title's start would end it; another user's does not.
func TestATorrentTheDeviceLeftMakesRoomAtTheCap(t *testing.T) {
	t.Parallel()
	p := newCapTitles(t, 0)
	p.cap(t, 1)
	token := p.signIn("member", "web")
	p.play(t, "web", token, "Iron Coast")
	p.report(t, "web", token, "/Sessions/Playing/Stopped", "Iron Coast", nil)
	if !p.running(t, "Iron Coast") {
		t.Fatal("the torrent did not linger after its playback")
	}

	// Another user's title: the place stays the member's.
	other := p.signIn("other", "tv")
	if answer := p.ask(t, "tv", other, "Signal Fire"); answer.ErrorCode != "RateLimitExceeded" {
		t.Fatalf("another user's play took the place: %q", answer.ErrorCode)
	}
	if !p.running(t, "Iron Coast") {
		t.Fatal("another user's play ended the member's torrent")
	}

	if answer := p.ask(t, "web", token, "Signal Fire"); answer.ErrorCode != "" || len(answer.MediaSources) == 0 {
		t.Fatalf("the device's next title answered %q", answer.ErrorCode)
	}
	if p.running(t, "Iron Coast") || !p.running(t, "Signal Fire") {
		t.Fatal("the torrent the device left did not give its place")
	}
}

// One torrent played by two users' devices counts once against the cap.
func TestATorrentPlayedByTwoDevicesCountsOnce(t *testing.T) {
	t.Parallel()
	p := newCapTitles(t, 0)
	p.cap(t, 1)
	token := p.signIn("member", "web")
	p.play(t, "web", token, "Iron Coast")
	other := p.signIn("other", "tv")
	if answer := p.ask(t, "tv", other, "Iron Coast"); answer.ErrorCode != "" || len(answer.MediaSources) == 0 {
		t.Fatalf("the torrent playing was refused to another device: %q", answer.ErrorCode)
	}
	p.play(t, "tv", other, "Iron Coast")
	if status := p.engine.Status(t.Context()); status.Playing != 1 || status.AtOnce != 1 {
		t.Fatalf("Health tells %d torrents of %d", status.Playing, status.AtOnce)
	}
}

// firstID is the identifier of an answer's first source, empty for none.
func firstID(answer playbackAnswer) string {
	if len(answer.MediaSources) == 0 {
		return ""
	}
	return answer.MediaSources[0].Id
}
