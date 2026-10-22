package jellyfin

import (
	"encoding/json"
	"fmt"
	"image"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// posterHost is an artwork server that counts the requests for each path
// and, once failing, answers none.
type posterHost struct {
	*httptest.Server
	failing atomic.Bool
	mu      sync.Mutex
	asked   map[string]int
}

func newPosterHost(t *testing.T, picture []byte) *posterHost {
	h := &posterHost{asked: map[string]int{}}
	h.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		h.asked[r.URL.Path]++
		h.mu.Unlock()
		if h.failing.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(picture)
	}))
	t.Cleanup(h.Close)
	return h
}

// requests returns how many times each poster was asked for, by name.
func (h *posterHost) requests() map[string]int {
	h.mu.Lock()
	defer h.mu.Unlock()
	asked := map[string]int{}
	for path, n := range h.asked {
		asked[strings.TrimSuffix(strings.TrimPrefix(path, "/"), ".jpg")] = n
	}
	return asked
}

// postersAddon serves a movie catalog for each of catalogs, whose titles
// have the genres listed, in order, and their posters on art, named after
// the catalog and the title's index: top-0.jpg.
func postersAddon(t *testing.T, art, id string, catalogs map[string][]string) string {
	t.Helper()
	metas := map[string][]stremio.Meta{}
	var listed []stremio.Catalog
	for catalog, genres := range catalogs {
		listed = append(listed, stremio.Catalog{Type: "movie", ID: catalog, Name: catalog})
		for i, genre := range genres {
			metas[catalog] = append(metas[catalog], stremio.Meta{ID: fmt.Sprintf("%s:%s-%d", id, catalog, i), Type: "movie",
				Name: fmt.Sprintf("%s %d", catalog, i), Genres: []string{genre}, Poster: fmt.Sprintf("%s/%s-%d.jpg", art, catalog, i)})
		}
	}
	slices.SortFunc(listed, func(a, b stremio.Catalog) int { return strings.Compare(a.ID, b.ID) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		if path == "/manifest.json" {
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: id, Name: id, Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{id + ":"}, Resources: []stremio.Resource{{Name: "catalog"}}, Catalogs: listed})
			return
		}
		for catalog, list := range metas {
			if strings.HasPrefix(path, "/catalog/movie/"+catalog) {
				if strings.Contains(path, "skip=") {
					list = nil
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"metas": list})
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// cardSizes are the sizes jellyfin-web asks for a poster's card in a
// window 1,400 pixels wide (see warmedSizes), with the width a poster of
// 1,000 by 1,500 pixels is served at for each.
var cardSizes = map[string]int{
	"fillHeight=326&fillWidth=217&quality=96": 240,
	"fillHeight=652&fillWidth=434&quality=96": 480,
	"fillHeight=300&fillWidth=200&quality=96": 240,
	"fillHeight=600&fillWidth=400&quality=96": 400,
}

// After Polyfin read the libraries' first pages, an app is served their
// posters at jellyfin-web's card sizes from the cache: with the artwork
// server failing and no resize able to run. Only the titles some user
// sees are warmed: none of a library hidden from every user, nor a title
// of a genre every user who sees its library blocks, nor of a user's own
// library before they open it. What is warm is not asked for again.
func TestFirstPagePostersAreWarmedAtJellyfinWebCardSizes(t *testing.T) {
	t.Parallel()
	art := newPosterHost(t, jpegOf(t, 1000, 1500))
	s := newTestServer(t, 10)
	install := func(scope addons.Scope, manifestURL string, catalogs ...string) map[string]accounts.ID {
		t.Helper()
		addon, err := s.addons.Install(t.Context(), scope, manifestURL, false)
		if err != nil {
			t.Fatal(err)
		}
		var choices []addons.LibraryChoice
		for _, catalog := range catalogs {
			choices = append(choices, addons.LibraryChoice{AddonID: addon.ID, CatalogType: "movie", CatalogID: catalog})
		}
		libraries, err := s.addons.SetLibraries(t.Context(), scope, choices)
		if err != nil {
			t.Fatal(err)
		}
		ids := map[string]accounts.ID{}
		for _, l := range libraries {
			ids[l.Catalog.ID] = library.LibraryID(l)
		}
		return ids
	}
	shared := install(addons.Shared(), postersAddon(t, art.URL, "reels", map[string][]string{
		"top": {"Drama", "Drama", "Horror"}, "kept": {"Drama", "Drama"}}), "top", "kept")
	// The kid sees the top library without its horror; the member, an
	// administrator whose own addon reaches the local artwork server, sees
	// their own library only.
	s.user("kid", func(c *accounts.UserChanges) {
		c.HiddenLibraries = &[]accounts.ID{shared["kept"]}
		c.BlockedGenres = &[]string{"Horror"}
	})
	member := s.user("member", func(c *accounts.UserChanges) {
		c.IsAdministrator = new(true)
		c.HiddenLibraries = &[]accounts.ID{shared["top"], shared["kept"]}
	})
	own := install(addons.Personal(member.ID), postersAddon(t, art.URL, "home", map[string][]string{"mine": {"Drama", "Drama"}}), "mine")

	if err := s.library.Warm(t.Context()); err != nil {
		t.Fatal(err)
	}
	if asked, want := art.requests(), map[string]int{"top-0": 1, "top-1": 1}; !mapsEqual(asked, want) {
		t.Errorf("after the first pages were read, the artwork server was asked %v, want %v", asked, want)
	}
	servedFromCache(t, s, art, s.signIn("kid", "tv"), shared["top"], 2)

	// The member opens their own library: it is warmed with the next read,
	// and the posters warm already are not asked for again.
	token := s.signIn("member", "tv")
	if status := s.get(t, "/Items?ParentId="+own["mine"].String(), token, nil); status != http.StatusOK {
		t.Fatalf("opening the member's library: %d", status)
	}
	if err := s.library.Warm(t.Context()); err != nil {
		t.Fatal(err)
	}
	if asked, want := art.requests(), map[string]int{"top-0": 1, "top-1": 1, "mine-0": 1, "mine-1": 1}; !mapsEqual(asked, want) {
		t.Errorf("after the member opened their library, the artwork server was asked %v, want %v", asked, want)
	}
	servedFromCache(t, s, art, token, own["mine"], 2)
}

func mapsEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

// servedFromCache lists a library as the user of token and asks for each
// title's poster at every card size, while the artwork server fails and
// every place to resize is held: each must be served at its size.
func servedFromCache(t *testing.T, s testServer, art *posterHost, token string, libraryID accounts.ID, titles int) {
	t.Helper()
	var page QueryResult
	if status := s.get(t, "/Items?ParentId="+libraryID.String(), token, &page); status != http.StatusOK || len(page.Items) != titles {
		t.Fatalf("library: %d, %d titles, want %d", status, len(page.Items), titles)
	}
	art.failing.Store(true)
	resizes := s.handler.images.resizes
	for range cap(resizes) {
		resizes <- struct{}{}
	}
	defer func() {
		for range cap(resizes) {
			<-resizes
		}
		art.failing.Store(false)
	}()
	for _, item := range page.Items {
		for size, width := range cardSizes {
			request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet,
				s.url+"/Items/"+item.Id+"/Images/Primary?"+size+"&tag="+item.ImageTags["Primary"], nil)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			config, _, err := image.DecodeConfig(response.Body)
			_ = response.Body.Close()
			if response.StatusCode != http.StatusOK || err != nil || config.Width != width {
				t.Errorf("%s at %s: %d, %d pixels wide (%v), want %d", item.Name, size, response.StatusCode, config.Width, err, width)
			}
		}
	}
}

// warmedPosters are count posters on art, named after name.
func warmedPosters(art *artServer, name string, count int, iptv bool) []warmedPoster {
	posters := make([]warmedPoster, count)
	for i := range posters {
		posters[i] = warmedPoster{url: fmt.Sprintf("%s/%s-%d.jpg", art.URL, name, i), iptv: iptv}
	}
	return posters
}

// Warming takes a quarter of a host's places at most, one of a host to
// spare, so that apps' downloads go on meanwhile.
func TestPosterWarmingTakesAShareOfEachHost(t *testing.T) {
	t.Parallel()
	art := newArtServer(t, jpegOf(t, 40, 60))
	h := imageHandler(t.TempDir())
	share := imageHostFetches / imageWarmShare

	// Waves of share downloads all arrive, whatever the machine's speed,
	// and never more.
	art.holdWaves(share)
	h.warmImages(t.Context(), warmedPosters(art, "first", 3*share, false))
	if most := art.mostAtOnce(); most != share {
		t.Errorf("%d downloads warming at once from one host, want %d", most, share)
	}
	// While warming holds its places, an app's download takes another: a
	// wave of share and one more completes.
	art.holdWaves(share + 1)
	var wg sync.WaitGroup
	wg.Go(func() { h.warmImages(t.Context(), warmedPosters(art, "second", share, false)) })
	wg.Go(func() {
		if _, err := h.fetchArtwork(t.Context(), art.URL+"/app.jpg", false, false); err != nil {
			t.Error(err)
		}
	})
	wg.Wait()
	if most := art.mostAtOnce(); most != share+1 {
		t.Errorf("%d downloads at once while warming, want %d", most, share+1)
	}

	// One at a time from a host to spare.
	art.holdWaves(0)
	if _, err := h.fetchArtwork(t.Context(), art.URL+"/busy.jpg", false, false); err == nil {
		t.Error("a busy host answered")
	}
	art.holdWaves(1)
	h.warmImages(t.Context(), warmedPosters(art, "spared", 3, false))
	if most := art.mostAtOnce(); most != 1 {
		t.Errorf("%d downloads warming at once from a busy host", most)
	}
	logos := newArtServer(t, jpegOf(t, 40, 40))
	logos.holdWaves(1)
	h.warmImages(t.Context(), warmedPosters(logos, "logo", 3, true))
	if most := logos.mostAtOnce(); most != 1 {
		t.Errorf("%d downloads warming at once from an IPTV host", most)
	}
}

// Warming fills the image cache's disk up to what its trimming leaves, and
// stops there: it deletes no image for room, asks for no more, and what it
// kept goes first once apps' images fill the disk.
func TestPosterWarmingStopsAtTheCacheBound(t *testing.T) {
	t.Parallel()
	picture := jpegOf(t, 40, 60)
	art := newArtServer(t, picture)
	h := imageHandler(t.TempDir())
	size := int64(len("image/jpeg") + 1 + len(picture))
	// Room for an app's image, then three warmed, below what trimming leaves.
	h.images.diskBytes = (4*size + size/2) * 10 / 9
	if _, err := h.fetchArtwork(t.Context(), art.URL+"/app-0.jpg", false, false); err != nil {
		t.Fatal(err)
	}

	before := art.requests.Load()
	warmed := h.warmImages(t.Context(), warmedPosters(art, "warm", 20, false))
	asked := art.requests.Load() - before
	h.images.mu.Lock()
	diskSize, keptDisk := h.images.diskSize, h.images.keptDisk()
	h.images.mu.Unlock()
	if warmed != 3 || diskSize > keptDisk || asked >= 20 {
		t.Errorf("warming kept %d posters, %d bytes on disk of %d, after %d downloads of 20", warmed, diskSize, keptDisk, asked)
	}
	if !h.images.kept([]string{art.URL + "/app-0.jpg"}) {
		t.Error("warming deleted an app's image")
	}

	// Apps' images fill the disk: the warmed ones go first.
	for i := 1; i <= 3; i++ {
		if _, err := h.fetchArtwork(t.Context(), fmt.Sprintf("%s/app-%d.jpg", art.URL, i), false, false); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 4 {
		if !h.images.kept([]string{fmt.Sprintf("%s/app-%d.jpg", art.URL, i)}) {
			t.Errorf("app image %d was deleted before the warmed ones", i)
		}
	}
}
