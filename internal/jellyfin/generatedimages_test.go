package jellyfin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/stremio"
)

// mosaicAddon serves a catalog of movies, whose titles a test changes, with
// their posters, and a catalog of two collections that group it: one
// without a poster, named with accents, and one with its own.
type mosaicAddon struct {
	url    string
	mu     sync.Mutex
	movies []int
	// moviePages counts the reads of the movie catalog.
	moviePages atomic.Int32
}

func newMosaicAddon(t *testing.T) *mosaicAddon {
	t.Helper()
	a := &mosaicAddon{movies: []int{1, 2, 3, 4, 5, 6, 7, 8}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "mosaic", Name: "Mosaic", Version: "1", Types: []string{"movie", "collection"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}, {Type: "collection", ID: "sets", Name: "Sets"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			a.moviePages.Add(1)
			a.mu.Lock()
			metas := make([]stremio.Meta, 0, len(a.movies))
			for _, n := range a.movies {
				metas = append(metas, stremio.Meta{ID: movieID(n), Type: "movie", Name: fmt.Sprint("Movie ", n), Poster: a.url + fmt.Sprintf("/p/%d.png", n)})
			}
			a.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case strings.HasPrefix(path, "/catalog/collection/sets"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{
				{ID: "set:plain", Type: "collection", Name: "Été indien"},
				{ID: "set:posted", Type: "collection", Name: "Posted", Poster: a.url + "/p/99.png"}}})
		case strings.HasPrefix(path, "/meta/collection/"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/meta/collection/"), ".json")
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": stremio.Meta{ID: id, Type: "collection",
				Collection: &stremio.Collection{Sources: []stremio.CollectionSource{{Type: "movie", CatalogID: "top"}}}}})
		case strings.HasPrefix(path, "/p/"):
			var n int
			_, _ = fmt.Sscanf(path, "/p/%d.png", &n)
			poster := image.NewRGBA(image.Rect(0, 0, 60, 90))
			fill := color.RGBA{R: uint8(20 * (n % 6)), G: uint8(15 * (n % 8)), B: 100, A: 255}
			for i := 0; i < len(poster.Pix); i += 4 {
				poster.Pix[i], poster.Pix[i+1], poster.Pix[i+2], poster.Pix[i+3] = fill.R, fill.G, fill.B, fill.A
			}
			w.Header().Set("Content-Type", "image/png")
			_ = png.Encode(w, poster)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	a.url = server.URL
	return a
}

func movieID(n int) string { return fmt.Sprintf("tt%04d", n) }

func (a *mosaicAddon) setMovies(numbers ...int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.movies = numbers
}

// generating is a test server with the mosaic addon's catalogs as
// libraries, on a clock later moves past the time generated images are
// kept unchecked, and with a member and a kid who blocks a genre signed in.
type generating struct {
	testServer
	addon         *mosaicAddon
	addonID       accounts.ID
	elapsed       *atomic.Int64
	member, kid   string
	administrator string
	views         map[string]string
}

func newGenerating(t *testing.T) generating {
	t.Helper()
	g := generating{testServer: newTestServer(t, 10), addon: newMosaicAddon(t), elapsed: new(atomic.Int64)}
	g.library.SetClock(func() time.Time { return time.Now().Add(time.Duration(g.elapsed.Load())) })
	g.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	g.user("member", nil)
	g.user("kid", func(c *accounts.UserChanges) { c.BlockedGenres = &[]string{"Horror"} })
	installed, err := g.addons.Install(t.Context(), addons.Shared(), g.addon.url+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	g.addonID = installed.ID
	if _, err := g.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: installed.ID, CatalogType: "collection", CatalogID: "sets"},
		{AddonID: installed.ID, CatalogType: "movie", CatalogID: "top"}}); err != nil {
		t.Fatal(err)
	}
	g.administrator, g.member, g.kid = g.signIn("admin", "tv"), g.signIn("member", "tv"), g.signIn("kid", "tv")
	g.views = g.viewIDs(t, g.member)
	return g
}

// later moves the clock past the time generated images are kept
// unchecked.
func (g generating) later() { g.elapsed.Add(int64(2 * time.Minute)) }

// collections lists the collection library's collections as token's user,
// by name.
func (g generating) collections(t *testing.T, token string) map[string]BaseItemDto {
	t.Helper()
	var page QueryResult
	if status := g.get(t, "/Items?ParentId="+g.views["Sets"], token, &page); status != http.StatusOK {
		t.Fatalf("collections: %d", status)
	}
	byName := map[string]BaseItemDto{}
	for _, item := range page.Items {
		byName[item.Name] = item
	}
	return byName
}

// titlesOf lists a folder's titles as the member, which keeps their pages.
func (g generating) titlesOf(t *testing.T, id string) []string {
	t.Helper()
	return g.listingNames(t, g.member, "/Items?ParentId="+id+"&Limit=20")
}

// download downloads an image, decoded, with its ETag.
func (g generating) download(t *testing.T, path string) (int, image.Image, string) {
	t.Helper()
	status, header, body := g.send(http.MethodGet, path, "", "", "")
	if status != http.StatusOK {
		return status, nil, ""
	}
	picture, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return status, picture, header.Get("ETag")
}

// named reports whether an image has letters written on it: the test's
// posters are dark and shaded, only letters are near white.
func named(picture image.Image) bool {
	bounds := picture.Bounds()
	count := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 2 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 2 {
			r, g, b, _ := picture.At(x, y).RGBA()
			if r>>8 > 230 && g>>8 > 230 && b>>8 > 230 {
				count++
			}
		}
	}
	return count > 100
}

// Once the titles of an addon's collection without a poster are kept, it
// gets a generated Primary image, a poster, and a Thumb, a wide image,
// both drawn with its name; one with a poster keeps it. Listing the
// collections asks the addon for nothing. A user under parental control,
// or who hid one of the titles shown, gets no generated image.
func TestCollectionsWithoutAPosterGetGeneratedImages(t *testing.T) {
	t.Parallel()
	g := newGenerating(t)
	first := g.collections(t, g.member)
	plain, posted := first["Été indien"], first["Posted"]
	if plain.ImageTags["Primary"] != "" || plain.ImageTags["Thumb"] != "" {
		t.Errorf("before its titles are kept: %v", plain.ImageTags)
	}
	if reads := g.addon.moviePages.Load(); reads != 0 {
		t.Errorf("listing the collections read the titles' catalog %d times", reads)
	}
	if posted.ImageTags["Primary"] != library.ImageTag(g.addon.url+"/p/99.png") || posted.ImageTags["Thumb"] != "" {
		t.Errorf("a collection with a poster: %v", posted.ImageTags)
	}

	if got := g.titlesOf(t, plain.Id); len(got) != 8 {
		t.Fatalf("titles: %v", got)
	}
	g.later()
	plain = g.collections(t, g.member)["Été indien"]
	primary, thumb := plain.ImageTags["Primary"], plain.ImageTags["Thumb"]
	if primary == "" || thumb == "" || primary == thumb {
		t.Fatalf("generated images: %v", plain.ImageTags)
	}
	var item BaseItemDto
	if g.get(t, "/Items/"+plain.Id, g.member, &item); item.ImageTags["Primary"] != primary || item.ImageTags["Thumb"] != thumb {
		t.Errorf("the collection as an item: %v", item.ImageTags)
	}
	if posted = g.collections(t, g.member)["Posted"]; posted.ImageTags["Primary"] != library.ImageTag(g.addon.url+"/p/99.png") {
		t.Errorf("a collection with a poster, after: %v", posted.ImageTags)
	}

	status, poster, etag := g.download(t, "/Items/"+plain.Id+"/Images/Primary?tag="+primary)
	if status != http.StatusOK || poster.Bounds() != image.Rect(0, 0, 1000, 1500) || etag != `"`+primary+`"` || !named(poster) {
		t.Fatalf("the poster: %d %v %s named %t", status, poster.Bounds(), etag, named(poster))
	}
	// The posters fill the cells: the top-left one is the first title's.
	if r, gr, b, _ := poster.At(40, 40).RGBA(); b>>8 < 20 || r>>8 > b>>8 || gr>>8 > b>>8 {
		t.Errorf("top-left cell: %d %d %d", r>>8, gr>>8, b>>8)
	}
	if status, wide, _ := g.download(t, "/Items/"+plain.Id+"/Images/Thumb"); status != http.StatusOK || wide.Bounds() != image.Rect(0, 0, 1920, 1080) || !named(wide) {
		t.Errorf("the wide image: %d", status)
	}
	// Resized as artwork is.
	if status, small, _ := g.download(t, "/Items/"+plain.Id+"/Images/Primary?maxWidth=300"); status != http.StatusOK || small.Bounds().Dx() != 320 {
		t.Errorf("resized: %d %v", status, small)
	}

	// Parental control hides it, as an automatic library image.
	if kid := g.collections(t, g.kid)["Été indien"]; kid.ImageTags["Primary"] != "" || kid.ImageTags["Thumb"] != "" {
		t.Errorf("a user blocking genres: %v", kid.ImageTags)
	}
	// So does hiding one of the titles it shows.
	hidden := library.TitleItemID(library.KindMovie, movieID(2)).String()
	if status, _, _ := g.send(http.MethodPost, "/Polyfin/UserItems/"+hidden+"/Hidden", app("tv", g.member), "", ""); status != http.StatusNoContent {
		t.Fatalf("hiding a title: %d", status)
	}
	if mine := g.collections(t, g.member)["Été indien"]; mine.ImageTags["Primary"] != "" {
		t.Errorf("a user who hid a title shown: %v", mine.ImageTags)
	}
	if other := g.collections(t, g.administrator)["Été indien"]; other.ImageTags["Primary"] != primary {
		t.Errorf("another user: %v", other.ImageTags)
	}
}

// refreshTo changes the movies, then moves the clock past the catalog's
// refresh age until the collection lists them, and past the time
// generated images are kept unchecked.
func (g generating) refreshTo(t *testing.T, collection string, numbers ...int) {
	t.Helper()
	g.addon.setMovies(numbers...)
	want := make([]string, 0, len(numbers))
	for _, n := range numbers {
		want = append(want, fmt.Sprint("Movie ", n))
	}
	g.elapsed.Add(int64(61 * time.Minute))
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		got := g.titlesOf(t, collection)
		if slices.Equal(got, want) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("titles after the refresh: %v", got)
		}
	}
	g.later()
}

// A generated image keeps its titles, and its tag, while fewer than half
// of those it shows leave the collection's first titles; once half have
// left, it shows the first titles again, with a new tag. An image uploaded
// for the collection replaces the generated one, and deleting it brings
// that back.
func TestGeneratedImagesChangeWithHalfTheirTitles(t *testing.T) {
	t.Parallel()
	g := newGenerating(t)
	plain := g.collections(t, g.member)["Été indien"]
	g.titlesOf(t, plain.Id)
	g.later()
	tags := g.collections(t, g.member)["Été indien"].ImageTags
	if tags["Primary"] == "" {
		t.Fatalf("no generated image: %v", tags)
	}

	// One of the four shown as a poster, and of the six of the wide image.
	g.refreshTo(t, plain.Id, 1, 3, 4, 5, 6, 7, 8)
	if got := g.collections(t, g.member)["Été indien"].ImageTags; got["Primary"] != tags["Primary"] || got["Thumb"] != tags["Thumb"] {
		t.Errorf("one title gone: %v, was %v", got, tags)
	}
	// Two of the poster's four.
	g.refreshTo(t, plain.Id, 1, 4, 5, 6, 7, 8, 9)
	changed := g.collections(t, g.member)["Été indien"].ImageTags
	if changed["Primary"] == "" || changed["Primary"] == tags["Primary"] || changed["Thumb"] == tags["Thumb"] {
		t.Errorf("half the titles gone: %v, was %v", changed, tags)
	}

	id, _ := accounts.ParseID(plain.Id)
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 20, 30))); err != nil {
		t.Fatal(err)
	}
	if err := g.library.UploadImage(t.Context(), id, "Primary", picture.Bytes()); err != nil {
		t.Fatal(err)
	}
	uploaded, _ := g.library.UploadedArtwork(id, "Primary")
	if got := g.collections(t, g.member)["Été indien"].ImageTags; got["Primary"] != library.ImageTag(uploaded) || got["Thumb"] != changed["Thumb"] {
		t.Errorf("an uploaded poster: %v", got)
	}
	if status, _, body := g.send(http.MethodGet, "/Items/"+plain.Id+"/Images/Primary", "", "", ""); status != http.StatusOK || !bytes.HasPrefix(body, []byte("\x89PNG")) {
		t.Errorf("downloading the upload: %d", status)
	}
	if err := g.library.DeleteUploadedImage(t.Context(), id, "Primary"); err != nil {
		t.Fatal(err)
	}
	if got := g.collections(t, g.member)["Été indien"].ImageTags; got["Primary"] != changed["Primary"] {
		t.Errorf("once the upload is deleted: %v", got)
	}
}

// With Generate images for collections without one off, nothing is
// generated: not for an addon's collection, nor for a smart list shown as
// a collection, which gets one while it is on.
func TestGeneratedImagesCanBeTurnedOff(t *testing.T) {
	t.Parallel()
	g := newGenerating(t)
	picks := g.makeSmartList(t, nil, "Picks", smartlists.ShownAsCollection, []string{"movie/top"}, smartlists.SortCatalog)
	plain := g.collections(t, g.member)["Été indien"]
	g.titlesOf(t, plain.Id)
	g.titlesOf(t, picks)
	g.later()
	smart := func() map[string]string {
		t.Helper()
		var item BaseItemDto
		g.get(t, "/Items/"+picks, g.member, &item)
		return item.ImageTags
	}
	if tags := smart(); tags["Primary"] == "" || tags["Thumb"] == "" {
		t.Errorf("a smart collection: %v", tags)
	}
	if status, picture, _ := g.download(t, "/Items/"+picks+"/Images/Primary"); status != http.StatusOK || !named(picture) {
		t.Errorf("the smart collection's image: %d", status)
	}
	if tags := g.collections(t, g.member)["Été indien"].ImageTags; tags["Primary"] == "" {
		t.Fatalf("on: %v", tags)
	}

	g.setting(t, func(s *accounts.Settings) { s.GenerateImages = false })
	g.later()
	if tags := g.collections(t, g.member)["Été indien"].ImageTags; tags["Primary"] != "" || tags["Thumb"] != "" {
		t.Errorf("off: %v", tags)
	}
	if tags := smart(); tags["Primary"] != "" || tags["Thumb"] != "" {
		t.Errorf("a smart collection, off: %v", tags)
	}
	for _, id := range []string{plain.Id, picks} {
		if status, _, _ := g.send(http.MethodGet, "/Items/"+id+"/Images/Primary", "", "", ""); status != http.StatusNotFound {
			t.Errorf("downloading %s, off: %d", id, status)
		}
	}
}

// A library set to a generated image gets a wide Primary image from its
// first titles, or for a collection library from its collections' posters,
// once they are kept; a user under parental control is not shown it, and
// a custom image replaces it.
func TestLibrariesSetToGeneratedGetAWideImage(t *testing.T) {
	t.Parallel()
	g := newGenerating(t)
	for _, catalog := range []string{"top", "sets"} {
		kind := map[string]string{"top": "movie", "sets": "collection"}[catalog]
		if err := g.addons.SetLibraryImage(t.Context(), addons.Shared(), addons.LibraryKey{AddonID: g.addonID, CatalogType: kind, CatalogID: catalog},
			addons.LibraryImageGenerated); err != nil {
			t.Fatal(err)
		}
	}
	views := func(token string) map[string]string {
		t.Helper()
		var page QueryResult
		g.get(t, "/UserViews", token, &page)
		tags := map[string]string{}
		for _, view := range page.Items {
			tags[view.Name] = view.ImageTags["Primary"]
		}
		return tags
	}
	if tags := views(g.member); tags["Top"] != "" || tags["Sets"] != "" {
		t.Errorf("before the catalogs are kept: %v", tags)
	}
	g.titlesOf(t, g.views["Top"])
	plain := g.collections(t, g.member)["Été indien"]
	g.titlesOf(t, plain.Id)
	g.later()
	tags := views(g.member)
	if tags["Top"] == "" || tags["Sets"] == "" {
		t.Fatalf("generated library images: %v", tags)
	}
	if status, picture, _ := g.download(t, "/Items/"+g.views["Top"]+"/Images/Primary"); status != http.StatusOK ||
		picture.Bounds() != image.Rect(0, 0, 1920, 1080) || !named(picture) {
		t.Errorf("the library's image: %d", status)
	}
	if status, _, _ := g.download(t, "/Items/"+g.views["Sets"]+"/Images/Primary"); status != http.StatusOK {
		t.Errorf("the collection library's image: %d", status)
	}
	if kid := views(g.kid); kid["Top"] != "" {
		t.Errorf("a user blocking genres: %v", kid)
	}

	id, _ := accounts.ParseID(g.views["Top"])
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 32, 18))); err != nil {
		t.Fatal(err)
	}
	if err := g.library.UploadImage(t.Context(), id, "Primary", picture.Bytes()); err != nil {
		t.Fatal(err)
	}
	uploaded, _ := g.library.UploadedArtwork(id, "Primary")
	if got := views(g.member); got["Top"] != library.ImageTag(uploaded) {
		t.Errorf("an uploaded image: %v", got)
	}
	if err := g.library.DeleteUploadedImage(t.Context(), id, "Primary"); err != nil {
		t.Fatal(err)
	}
	if got := views(g.member); got["Top"] != tags["Top"] {
		t.Errorf("once the upload is deleted: %v, was %v", got, tags)
	}
}

// Only the images Polyfin generates are drawn as generated images. An
// addon's poster written in their URLs' scheme is no image: its posters
// are never downloaded, so a member's own addon, confined to public
// addresses, cannot have Polyfin reach a local one through it.
func TestAddonsCannotGiveGeneratedImages(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	var secretHits, directHits atomic.Int32
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/secret.png":
			secretHits.Add(1)
		case "/direct.png":
			directHits.Add(1)
		}
		w.Header().Set("Content-Type", "image/png")
		_ = png.Encode(w, image.NewRGBA(image.Rect(0, 0, 60, 90)))
	}))
	t.Cleanup(local.Close)
	generated := "polyfin-generated:poster?" + url.Values{"v": {"1"}, "name": {"x"}, "poster": {local.URL + "/secret.png"}}.Encode()
	metas := []stremio.Meta{
		{ID: "own:1", Type: "movie", Name: "Through the scheme", Poster: generated},
		{ID: "own:2", Type: "movie", Name: "Direct", Poster: local.URL + "/direct.png"},
	}
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "own", Name: "Mine", Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{"own:"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "mine", Name: "Mine"}}})
		case strings.HasPrefix(path, "/catalog/movie/mine"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case strings.HasPrefix(path, "/meta/movie/"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/meta/movie/"), ".json")
			for _, m := range metas {
				if m.ID == id {
					_ = json.NewEncoder(w).Encode(map[string]any{"meta": m})
				}
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(addon.Close)

	// No test server is on a public address: the member's catalog is read
	// while they are an administrator, whose own addons are not confined,
	// as it would be from an addon on one. They then become a member, and
	// their addon is confined.
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	owner := s.user("owner", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	installed, err := s.addons.Install(t.Context(), addons.Personal(owner.ID), addon.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Personal(owner.ID), []addons.LibraryChoice{
		{AddonID: installed.ID, CatalogType: "movie", CatalogID: "mine"}}); err != nil {
		t.Fatal(err)
	}
	list := func() []BaseItemDto {
		t.Helper()
		token := s.signIn("owner", "tv")
		var page QueryResult
		if status := s.get(t, "/Items?ParentId="+s.viewIDs(t, token)["Mine"], token, &page); status != http.StatusOK || len(page.Items) != 2 {
			t.Fatalf("the owner's library: %d %+v", status, page.Items)
		}
		return page.Items
	}
	list()
	if _, err := s.store.UpdateUser(t.Context(), owner.ID, accounts.UserChanges{IsAdministrator: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	statuses := map[string]int{}
	for _, item := range list() {
		if item.Name == "Through the scheme" && item.ImageTags["Primary"] != "" {
			t.Errorf("the poster in the scheme of generated images is listed: %v", item.ImageTags)
		}
		statuses[item.Name], _, _ = s.send(http.MethodGet, "/Items/"+item.Id+"/Images/Primary", "", "", "")
	}
	if statuses["Direct"] != http.StatusForbidden || directHits.Load() != 0 {
		t.Errorf("the confined addon's local poster: %d, reached %d times", statuses["Direct"], directHits.Load())
	}
	if statuses["Through the scheme"] == http.StatusOK || secretHits.Load() != 0 {
		t.Errorf("the poster in the scheme of generated images: %d, its poster reached %d times", statuses["Through the scheme"], secretHits.Load())
	}
}

// A movie or series library made from now on shows a generated image
// without anyone choosing it, as Jellyfin draws one for its libraries:
// apps see a wide Primary image on its tile. A user blocking genres still
// gets none, and a collection library keeps no image.
func TestNewLibrariesGetAGeneratedImage(t *testing.T) {
	t.Parallel()
	g := newGenerating(t)
	g.titlesOf(t, g.views["Top"])
	g.later()
	views := func(token string) map[string]BaseItemDto {
		t.Helper()
		var page QueryResult
		g.get(t, "/UserViews", token, &page)
		byName := map[string]BaseItemDto{}
		for _, view := range page.Items {
			byName[view.Name] = view
		}
		return byName
	}
	top := views(g.member)["Top"]
	if top.ImageTags["Primary"] == "" || top.PrimaryImageAspectRatio == nil || *top.PrimaryImageAspectRatio != 16.0/9.0 {
		t.Fatalf("the new movie library: tags %v, ratio %v", top.ImageTags, top.PrimaryImageAspectRatio)
	}
	if status, picture, _ := g.download(t, "/Items/"+g.views["Top"]+"/Images/Primary"); status != http.StatusOK || !named(picture) {
		t.Errorf("the new library's image: %d", status)
	}
	if sets := views(g.member)["Sets"]; sets.ImageTags["Primary"] != "" {
		t.Errorf("the collection library: %v", sets.ImageTags)
	}
	if kid := views(g.kid)["Top"]; kid.ImageTags["Primary"] != "" {
		t.Errorf("a user blocking genres: %v", kid.ImageTags)
	}
}
