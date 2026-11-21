package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// cachedAddon serves a movie with four versions, each named by the first
// word of its name, as an addon's streamData and labels tell them:
// Uncached, not cached on its debrid service, 50 seeders; Unknown, saying
// nothing of its cache, 5 seeders written in its labels; Cached, cached,
// 10 seeders; Seedless, cached, no seeders told.
func cachedAddon(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	movie := stremio.Meta{ID: "tt3758", Type: "movie", Name: "Movie", Runtime: "2h", Year: "2008"}
	cached := func(ready bool, seeders stremio.Number) stremio.StreamData {
		return stremio.StreamData{Service: stremio.StreamService{ID: "service", Cached: &ready}, Torrent: stremio.StreamTorrent{Seeders: seeders}}
	}
	streams := []stremio.Stream{
		{Name: "Uncached", Data: cached(false, 50), BehaviorHints: stremio.StreamBehavior{Filename: "Movie.2008.1080p.WEB-DL.mkv"}},
		{Name: "Unknown", Title: "Movie.2008.720p.WEB-DL.mkv\n👤 5", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.2008.720p.WEB-DL.mkv"}},
		{Name: "Cached", Data: cached(true, 10), BehaviorHints: stremio.StreamBehavior{Filename: "Movie.2008.2160p.WEB-DL.mkv"}},
		{Name: "Seedless", Data: cached(true, 0), BehaviorHints: stremio.StreamBehavior{Filename: "Movie.2008.480p.DVDRip.mkv"}},
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "cached", Name: "Store", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt3758.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt3758.json":
			for i := range streams {
				streams[i].URL = server.URL + "/files/" + streams[i].BehaviorHints.Filename
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
		case strings.HasPrefix(path, "/files/"):
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("\x1a\x45\xdf\xa3 media bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// cachedOn is the cached addon's movie, with its versions in the addon's
// order, as a member sees it.
func cachedOn(t *testing.T, s testServer) rulesSetup {
	t.Helper()
	user := s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), cachedAddon(t), false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	if len(page.Items) != 1 {
		t.Fatalf("titles: %+v", page.Items)
	}
	p := rulesSetup{playbackSetup: playbackSetup{testServer: s, token: token, user: user, movie: page.Items[0].Id}}
	movie, _ := accounts.ParseID(p.movie)
	versions, err := s.library.Versions(t.Context(), user, movie)
	if err != nil || len(versions) != 4 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	p.versions = versions
	return p
}

// With no criterion set, the versions an addon says are not cached on its
// debrid service come last, the others keeping the addons' order; with
// criteria, only the cached criterion reads it, cached first, then those
// the addon says nothing of. Most seeders first reads the streamData's
// seeders, else the labels', those with none last.
func TestVersionsOrderByCacheAndSeeders(t *testing.T) {
	t.Parallel()
	p := cachedOn(t, newTestServer(t, 10))
	for i, tc := range []struct {
		order []string
		want  []string
	}{
		{nil, []string{"Unknown", "Cached", "Seedless", "Uncached"}},
		{[]string{"cached"}, []string{"Cached", "Seedless", "Unknown", "Uncached"}},
		{[]string{"seeders"}, []string{"Uncached", "Cached", "Unknown", "Seedless"}},
		{[]string{"cached", "seeders"}, []string{"Cached", "Seedless", "Unknown", "Uncached"}},
		{[]string{"seeders", "cached"}, []string{"Uncached", "Cached", "Unknown", "Seedless"}},
		{[]string{"size-large"}, []string{"Uncached", "Unknown", "Cached", "Seedless"}},
	} {
		p.setting(t, func(s *accounts.Settings) { s.VersionOrder = tc.order })
		user, token := p.another(t, "cached"+strconv.Itoa(i), "")
		if got := firstWords(p.detailed(t, user, token)); !slices.Equal(got, tc.want) {
			t.Errorf("%v: %v, want %v", tc.order, got, tc.want)
		}
	}
}

// Short names say Cached for the versions an addon says are cached on its
// debrid service, in the server language.
func TestShortVersionNamesSayCached(t *testing.T) {
	t.Parallel()
	p := cachedOn(t, newTestServer(t, 10))
	p.setting(t, func(s *accounts.Settings) { s.VersionNames = "short" })
	var cached []bool
	for _, source := range p.detailed(t, p.user, p.token) {
		cached = append(cached, strings.HasSuffix(source.Name, " · Cached · Store"))
	}
	// Unknown, Cached, Seedless, then Uncached, not cached.
	if want := []bool{false, true, true, false}; !slices.Equal(cached, want) {
		t.Errorf("cached in short names: %v, want %v", cached, want)
	}
}
