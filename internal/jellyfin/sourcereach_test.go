package jellyfin

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/stremio/reachtest"
)

// reachedFiles is a server of the files addons name: a poster, a logo, a
// video and a subtitle file, counting its requests by path.
type reachedFiles struct {
	mu   sync.Mutex
	hits map[string]int
	url  string
	png  []byte
}

func newReachedFiles(t *testing.T) *reachedFiles {
	t.Helper()
	f := &reachedFiles{hits: map[string]int{}, png: pngOf(t, 60, 90)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.hits[r.URL.Path]++
		f.mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, ".png"):
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(f.png)
		case strings.HasSuffix(r.URL.Path, ".srt"):
			_, _ = io.WriteString(w, "1\r\n00:00:01,000 --> 00:00:04,000\r\nBonjour\r\n")
		default:
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader(mediaBytes))
		}
	}))
	t.Cleanup(server.Close)
	f.url = server.URL
	return f
}

// reached counts the requests the server received in all.
func (f *reachedFiles) reached() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for _, hits := range f.hits {
		total += hits
	}
	return total
}

// namingAddon serves a Stremio addon id with one catalog of one movie,
// whose poster, stream and subtitle file are at files.
func namingAddon(t *testing.T, id, files string) string {
	t.Helper()
	movie := stremio.Meta{ID: "tt" + id, Type: "movie", Name: id, Runtime: "2h", Year: "2010", Released: "2010-01-01T00:00:00.000Z",
		Poster: files + "/" + id + "/poster.png"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: id, Name: id, Version: "1", Types: []string{"movie"}, IDPrefixes: []string{"tt" + id},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}, {Name: "subtitles"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "all", Name: id + " movies"}}})
		case strings.HasPrefix(path, "/catalog/movie/all"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case strings.HasPrefix(path, "/meta/movie/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case strings.HasPrefix(path, "/stream/movie/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{{Name: "Named", URL: files + "/" + id + "/movie.mp4",
				BehaviorHints: stremio.StreamBehavior{Filename: id + ".1080p.mp4", VideoSize: stremio.Number64(len(mediaBytes))}}}})
		case strings.HasPrefix(path, "/subtitles/movie/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"subtitles": []stremio.Subtitle{{ID: "fr-1", URL: files + "/" + id + "/movie.fr.srt", Lang: "fre"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// reachSetup is a server with addons that name files of the local network
// or of the internet, each on a public address or on the local network.
type reachSetup struct {
	testServer
	network  *reachtest.Network
	lan, web *reachedFiles
	admin    accounts.User
	token    string
	sources  []reachSource
}

// reachSource is an addon of reachSetup: where it is, whose it is, where
// the files it names are, and whether those are reached.
type reachSource struct {
	name    string
	scope   addons.Scope
	public  bool
	files   *reachedFiles
	reached bool
	movie   string
}

func newReachSetup(t *testing.T) *reachSetup {
	t.Helper()
	r := &reachSetup{lan: newReachedFiles(t), web: newReachedFiles(t)}
	r.testServer = newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, _ *pgxpool.Pool) { r.network = reachtest.New(o.Stremio) })
	r.setting(t, func(settings *accounts.Settings) { settings.SyncAddonSubtitles = false })
	r.admin = r.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	r.token = r.signIn("admin", "tv")
	own := addons.Personal(r.admin.ID)
	r.sources = []reachSource{
		{name: "publiclan", scope: addons.Shared(), public: true, files: r.lan},
		{name: "publicweb", scope: addons.Shared(), public: true, files: r.web, reached: true},
		{name: "locallan", scope: addons.Shared(), files: r.lan, reached: true},
		{name: "ownpubliclan", scope: own, public: true, files: r.lan},
		{name: "ownlocallan", scope: own, files: r.lan, reached: true},
	}
	web := r.network.Public(t, r.web.url)
	choices := map[*accounts.ID][]addons.LibraryChoice{}
	for _, source := range r.sources {
		files := r.lan.url
		if source.files == r.web {
			files = web
		}
		address := namingAddon(t, source.name, files)
		if source.public {
			address = r.network.Public(t, address)
		}
		installed, err := r.addons.Install(t.Context(), source.scope, address+"/manifest.json", false)
		if err != nil {
			t.Fatal(err)
		}
		if installed.Network == addons.NetworkPublic != source.public {
			t.Fatalf("%s installed on %q", source.name, installed.Network)
		}
		choices[source.scope.Owner] = append(choices[source.scope.Owner], addons.LibraryChoice{AddonID: installed.ID, CatalogType: "movie", CatalogID: "all"})
	}
	for owner, list := range choices {
		if _, err := r.addons.SetLibraries(t.Context(), addons.Scope{Owner: owner}, list); err != nil {
			t.Fatal(err)
		}
	}
	// The administrator's app lists each library once.
	views := r.viewIDs(t, r.token)
	for i, source := range r.sources {
		var page QueryResult
		if status := r.get(t, "/Items?ParentId="+views[source.name+" movies"], r.token, &page); status != http.StatusOK || len(page.Items) != 1 {
			t.Fatalf("the library of %s: %d %+v", source.name, status, page.Items)
		}
		r.sources[i].movie = page.Items[0].Id
	}
	return r
}

// check asks the Jellyfin API for the poster, the video and the subtitle
// file each addon names: those of an addon that may reach them are
// answered, the others are refused without any request for them.
func (r *reachSetup) check(t *testing.T) {
	t.Helper()
	for _, source := range r.sources {
		id, _ := accounts.ParseID(source.movie)
		versions, err := r.library.Versions(t.Context(), r.admin, id)
		if err != nil || len(versions) != 1 {
			t.Fatalf("the versions of %s: %+v %v", source.name, versions, err)
		}
		r.listed(t, r.admin, source.movie)
		// The poster, without credentials, as apps load it: by the tag the
		// title gave.
		var title BaseItemDto
		if status := r.get(t, "/Items/"+source.movie, r.token, &title); status != http.StatusOK {
			t.Fatalf("the title of %s: %d", source.name, status)
		}
		before := source.files.reached()
		image, body := r.fetch(t, "/Items/"+source.movie+"/Images/Primary?tag="+title.ImageTags["Primary"])
		// The video, relayed for an app that names its token in a header.
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet,
			r.url+"/Videos/"+source.movie+"/stream?static=true&mediaSourceId="+versions[0].ID.String(), nil)
		request.Header.Set("Authorization", app("tv", r.token))
		video, err := noRedirects.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		played, _ := io.ReadAll(video.Body)
		video.Body.Close()
		// The subtitle file the addon lists for the title.
		subtitle, text := r.fetch(t, "/Videos/"+source.movie+"/"+source.movie+"/Subtitles/0/0/Stream.srt?ApiKey="+r.token)
		requests := source.files.reached() - before
		switch {
		case source.reached:
			if image.StatusCode != http.StatusOK || !bytes.Equal(body, source.files.png) {
				t.Errorf("the poster %s names: %d, %d bytes", source.name, image.StatusCode, len(body))
			}
			if video.StatusCode != http.StatusOK || string(played) != mediaBytes {
				t.Errorf("the video %s names: %d %q", source.name, video.StatusCode, played)
			}
			if subtitle.StatusCode != http.StatusOK || !strings.Contains(string(text), "Bonjour") {
				t.Errorf("the subtitle file %s names: %d %q", source.name, subtitle.StatusCode, text)
			}
		default:
			if image.StatusCode != http.StatusForbidden {
				t.Errorf("the poster %s names on the local network: %d", source.name, image.StatusCode)
			}
			if video.StatusCode == http.StatusOK || video.StatusCode == http.StatusFound || video.StatusCode == http.StatusPartialContent {
				t.Errorf("the video %s names on the local network: %d %q", source.name, video.StatusCode, played)
			}
			if subtitle.StatusCode == http.StatusOK {
				t.Errorf("the subtitle file %s names on the local network: %d %q", source.name, subtitle.StatusCode, text)
			}
			if requests != 0 {
				t.Errorf("%d requests reached the local network for what %s names", requests, source.name)
			}
		}
	}
}

// What an addon of the server, or an administrator's own, names is
// reached as where the addon is: on a public address, the poster, the
// video and the subtitle file it names on the local network are refused
// without a request reaching the local network, while those it names on
// public hosts are served; on the local network, what it names there is
// still served.
func TestWhatAnAddonNamesIsReachedAsWhereTheAddonIs(t *testing.T) {
	t.Parallel()
	newReachSetup(t).check(t)
}

// The titles and the versions kept of a server addon on a public address
// before Polyfin told where sources are, free to reach the local network,
// are confined once the sources are located at start, without anyone's
// action; those of an addon on the local network stay free.
func TestWhatWasKeptOfAPublicAddonIsConfinedOnceSourcesAreLocated(t *testing.T) {
	t.Parallel()
	r := newReachSetup(t)
	r.addons.SetReconfine(r.library.Reconfine)
	// As after the upgrade: the titles listed were kept without any
	// confinement, and nothing tells where the addons are.
	if _, err := r.pool.Exec(t.Context(), "UPDATE items SET data = data - 'confined'"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.pool.Exec(t.Context(), "UPDATE addons SET network = NULL"); err != nil {
		t.Fatal(err)
	}
	r.addons.LocateSources(t.Context(), slog.New(slog.DiscardHandler))
	r.check(t)
}

// The logo a channel of an IPTV source shows follows where the source is
// too: one the list of a source on a public address names on the local
// network is refused without a request for it, while the logo and the
// stream an administrator gave the channel themselves are theirs to
// reach.
func TestTheLogoAnIPTVSourceNamesIsReachedAsWhereTheSourceIs(t *testing.T) {
	t.Parallel()
	var network *reachtest.Network
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, _ *pgxpool.Pool) { network = reachtest.New(o.Stremio) })
	lan := newReachedFiles(t)
	list := func(logo string) string {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "#EXTM3U\n#EXTINF:-1 tvg-logo=\""+logo+"\" group-title=\"News\",Zeb One\nhttps://live.example/1.ts\n")
		}))
		t.Cleanup(server.Close)
		return server.URL
	}
	channel := func(name, address string) accounts.ID {
		t.Helper()
		addon, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: name, Account: iptv.Account{Kind: addons.KindM3U, URL: address + "/list.m3u"}}, false)
		if err != nil {
			t.Fatal(err)
		}
		_, channels, err := s.iptv.ListChannels(t.Context(), addons.Shared(), addon.ID, iptv.ChannelFilter{Limit: 10})
		if err != nil || len(channels) != 1 {
			t.Fatalf("the channels of %s: %v %v", name, channels, err)
		}
		return channels[0].ID
	}
	public := channel("Public", network.Public(t, list(lan.url+"/public/logo.png")))
	local := channel("Local", list(lan.url+"/local/logo.png"))

	if response, _ := s.fetch(t, "/Items/"+public.String()+"/Images/Primary"); response.StatusCode != http.StatusForbidden || lan.reached() != 0 {
		t.Errorf("the logo a public source names on the local network: %d, reached %d times", response.StatusCode, lan.reached())
	}
	if response, body := s.fetch(t, "/Items/"+local.String()+"/Images/Primary"); response.StatusCode != http.StatusOK || !bytes.Equal(body, lan.png) {
		t.Errorf("the logo a local source names on the local network: %d, %d bytes", response.StatusCode, len(body))
	}
	// The administrator's own logo for the public source's channel.
	chosen := lan.url + "/chosen/logo.png"
	addon, err := s.iptv.ChannelSource(t.Context(), public)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.iptv.UpdateChannel(t.Context(), addons.Shared(), addon, public, iptv.ChannelChanges{Logo: &iptv.Nullable[string]{Value: &chosen}}); err != nil {
		t.Fatal(err)
	}
	if response, body := s.fetch(t, "/Items/"+public.String()+"/Images/Primary"); response.StatusCode != http.StatusOK || !bytes.Equal(body, lan.png) {
		t.Errorf("the logo chosen for a public source's channel: %d, %d bytes", response.StatusCode, len(body))
	}
	// The stream the administrator added to that channel may be on the
	// local network; the one the list names stays on public addresses.
	custom := lan.url + "/custom/stream.ts"
	if _, err := s.iptv.AddStream(t.Context(), addons.Shared(), addon, public, custom, "Mine", false); err != nil {
		t.Fatal(err)
	}
	admin := s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	versions, err := s.library.Versions(t.Context(), admin, public)
	if err != nil || len(versions) != 2 {
		t.Fatalf("the channel's versions: %+v %v", versions, err)
	}
	for _, version := range versions {
		if version.Confined != (version.URL != custom) {
			t.Errorf("the stream at %s is confined %v", version.URL, version.Confined)
		}
	}
}
