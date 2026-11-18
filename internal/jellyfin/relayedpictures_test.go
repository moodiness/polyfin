package jellyfin

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/boxset"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// svgPicture is an SVG image: a document, which may hold a script, and
// which a browser that opens its address as a page runs.
const svgPicture = `<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><script>document.title = 'ran'</script></svg>`

// jpegStart is how a JPEG picture starts, and jpegBytes what the tests'
// artwork servers answer for a picture they do not decode: pictures are
// relayed by what their bytes are, so theirs start as one does.
const (
	jpegStart = "\xff\xd8\xff"
	jpegBytes = jpegStart + " jpeg bytes"
)

// webpPicture is a WebP picture of one pixel.
func webpPicture(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA==")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// gifPicture is a GIF picture.
func gifPicture(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := gif.Encode(&out, image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// servedPicture is what an artwork server answers for a picture: a type
// of its choosing, and bytes.
type servedPicture struct {
	contentType string
	body        []byte
}

// pictureHost is an addon whose movies, one for each picture, have the
// picture as their poster, on the addon's own server, and a TMDB image
// server holding the same pictures. asked counts the pictures asked for.
type pictureHost struct {
	url   string
	asked atomic.Int32
}

// poster is the address of the picture named name.
func (p *pictureHost) poster(name string) string { return p.url + "/posters/" + name }

func newPictureHost(t *testing.T, pictures map[string]servedPicture) *pictureHost {
	t.Helper()
	p := &pictureHost{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var metas []stremio.Meta
		for name := range pictures {
			metas = append(metas, stremio.Meta{ID: "pic:" + name, Type: "movie", Name: name, Poster: p.poster(name)})
		}
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "pictures", Name: "Pictures", Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{"pic:"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case strings.HasPrefix(path, "/meta/movie/pic%3A"):
			name := strings.TrimSuffix(strings.TrimPrefix(path, "/meta/movie/pic%3A"), ".json")
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": stremio.Meta{ID: "pic:" + name, Type: "movie", Name: name, Poster: p.poster(name)}})
		case strings.HasPrefix(path, "/posters/"), strings.HasPrefix(path, "/tmdb/original/"):
			picture, ok := pictures[path[strings.LastIndex(path, "/")+1:]]
			if !ok {
				http.NotFound(w, r)
				return
			}
			p.asked.Add(1)
			w.Header().Set("Content-Type", picture.contentType)
			_, _ = w.Write(picture.body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	p.url = server.URL
	return p
}

// movies installs the host's addon for the server and returns its movies'
// items by the name of their picture, as the user of token lists them.
func (p *pictureHost) movies(t *testing.T, s testServer, token string) map[string]string {
	t.Helper()
	if _, err := s.addons.Install(t.Context(), addons.Shared(), p.url+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	var page QueryResult
	if status := s.get(t, "/Items?ParentId="+s.viewIDs(t, token)["Top"], token, &page); status != http.StatusOK {
		t.Fatalf("the library: %d", status)
	}
	items := map[string]string{}
	for _, item := range page.Items {
		items[item.Name] = item.Id
	}
	return items
}

// boxSets gives the server the box sets of a TMDB whose image server is
// the host, and returns the options' change and what makes the collection
// id, whose poster is the picture named name.
func (p *pictureHost) boxSets(t *testing.T) (configure func(*Options, *pgxpool.Pool), add func(s testServer, id int64, name string)) {
	t.Helper()
	configure = func(o *Options, pool *pgxpool.Pool) {
		client := tmdb.New(p.url+"/api/", p.url+"/tmdb/", "test", func() accounts.Settings { return accounts.Settings{TMDBKey: "k3y"} })
		store := boxset.New(pool, client, boxset.Options{})
		t.Cleanup(store.Close)
		o.BoxSets = store
	}
	add = func(s testServer, id int64, name string) {
		t.Helper()
		if _, err := s.pool.Exec(t.Context(), `INSERT INTO tmdb_collections (id, name, poster_path, updated_at) VALUES ($1, $2, $3, now())`,
			id, name, "/"+name); err != nil {
			t.Fatal(err)
		}
	}
	return configure, add
}

// sandboxed reports whether an answer's policy sandboxes it.
func sandboxed(header http.Header) bool {
	for directive := range strings.SplitSeq(header.Get("Content-Security-Policy"), ";") {
		if strings.TrimSpace(directive) == "sandbox" {
			return true
		}
	}
	return false
}

// The image routes answer raster pictures only, told by their bytes: an
// SVG image an artwork server answers, a document that may hold a
// script, is not found, whatever type the server gave it, on the route of
// a title's artwork, without credentials and with them, on that of an
// image listed for a title, on a share link's and on a box set's.
func TestArtworkThatIsNoPictureIsNotRelayed(t *testing.T) {
	t.Parallel()
	svg := []byte(svgPicture)
	host := newPictureHost(t, map[string]servedPicture{
		"svg":      {"image/svg+xml", svg},
		"labelled": {"image/png", svg},
		"listed":   {"image/svg+xml", svg},
		"shared":   {"image/svg+xml", svg},
		"set":      {"image/svg+xml", svg},
		"page":     {"image/jpeg", []byte("<!doctype html><html><body><script>document.title = 'ran'</script></body></html>")},
	})
	configure, addBoxSet := host.boxSets(t)
	s := newProbingServer(t, 10, "ffprobe-not-installed", configure)
	admin := s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	token := s.signIn("admin", "tv")
	movies := host.movies(t, s, token)
	addBoxSet(s, 10, "set")
	refused := func(t *testing.T, target string, header http.Header) {
		t.Helper()
		response, body := fetchURL(t, target, header)
		if response.StatusCode != http.StatusNotFound || body != "" {
			t.Errorf("%d %q %q, want it not found", response.StatusCode, response.Header.Get("Content-Type"), body)
		}
		if kept := response.Header.Get("Cache-Control") + response.Header.Get("ETag"); kept != "" {
			t.Errorf("the answer is one to keep: %q", kept)
		}
	}
	for _, name := range []string{"svg", "labelled", "page"} {
		t.Run("a title's "+name+", without credentials", func(t *testing.T) {
			t.Parallel()
			refused(t, s.url+"/Items/"+movies[name]+"/Images/Primary", nil)
		})
		t.Run("a title's "+name+", with credentials", func(t *testing.T) {
			t.Parallel()
			refused(t, s.url+"/Items/"+movies[name]+"/Images/Primary?maxWidth=120", http.Header{"Authorization": {app("tv", token)}})
		})
	}
	t.Run("an image listed for a title", func(t *testing.T) {
		t.Parallel()
		var listed RemoteImageResult
		if status := s.get(t, "/Items/"+movies["listed"]+"/RemoteImages?type=Primary", token, &listed); status != http.StatusOK || len(listed.Images) != 1 {
			t.Fatalf("the images listed: %d %+v", status, listed)
		}
		refused(t, listed.Images[0].Url, nil)
		refused(t, listed.Images[0].Url, http.Header{"Authorization": {app("tv", token)}})
	})
	t.Run("a share link's", func(t *testing.T) {
		t.Parallel()
		s.setting(t, func(settings *accounts.Settings) { settings.ShareLinks = true })
		if _, err := s.store.UpdateUser(t.Context(), admin.ID, accounts.UserChanges{CanShare: new(true)}, nil); err != nil {
			t.Fatal(err)
		}
		item, _ := accounts.ParseID(movies["shared"])
		_, link, err := s.store.CreateShareLink(t.Context(), accounts.NewShareLink{User: admin.ID, Item: item, Kind: accounts.ShareMovie,
			Title: "shared", Expiry: accounts.DefaultShareLinkExpiry, MaxViewers: 1})
		if err != nil {
			t.Fatal(err)
		}
		site := httptest.NewServer(s.handler.Share())
		t.Cleanup(site.Close)
		refused(t, site.URL+"/share/"+link+"/image/primary", nil)
	})
	t.Run("a box set's", func(t *testing.T) {
		t.Parallel()
		refused(t, s.url+"/Polyfin/BoxSets/10/Images/Primary", nil)
		refused(t, s.url+"/Polyfin/BoxSets/10/Images/Primary", http.Header{"Authorization": {app("tv", token)}})
	})
}

// An SVG image kept on disk before only pictures were, with the type its
// server gave it, is not found either, and its server is not asked: what
// the disk holds is what would be answered.
func TestArtworkKeptThatIsNoPictureIsNotRelayed(t *testing.T) {
	t.Parallel()
	host := newPictureHost(t, map[string]servedPicture{"kept": {"image/png", pngOf(t, 20, 30)}, "set": {"image/png", pngOf(t, 20, 30)}})
	configure, addBoxSet := host.boxSets(t)
	cache := t.TempDir()
	dir := filepath.Join(cache, "images")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// As relayed artwork was kept: the type its server gave, then its
	// bytes.
	for _, target := range []string{host.poster("kept"), host.url + "/tmdb/original/set"} {
		if _, err := writeImage(dir, fileName(target), artwork{body: []byte(svgPicture), contentType: "image/svg+xml"}); err != nil {
			t.Fatal(err)
		}
	}
	s := newProbingServer(t, 10, "ffprobe-not-installed", configure, func(o *Options, _ *pgxpool.Pool) { o.CacheDir = cache })
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	token := s.signIn("admin", "tv")
	movies := host.movies(t, s, token)
	addBoxSet(s, 10, "set")
	var listed RemoteImageResult
	if status := s.get(t, "/Items/"+movies["kept"]+"/RemoteImages?type=Primary", token, &listed); status != http.StatusOK || len(listed.Images) != 1 {
		t.Fatalf("the images listed: %d %+v", status, listed)
	}
	for name, target := range map[string]string{
		"a title's artwork":           s.url + "/Items/" + movies["kept"] + "/Images/Primary",
		"an image listed for a title": listed.Images[0].Url,
		"a box set's artwork":         s.url + "/Polyfin/BoxSets/10/Images/Primary",
	} {
		for _, header := range []http.Header{nil, {"Authorization": {app("tv", token)}}} {
			response, body := fetchURL(t, target, header)
			if response.StatusCode != http.StatusNotFound || body != "" {
				t.Errorf("%s: %d %q %q, want it not found", name, response.StatusCode, response.Header.Get("Content-Type"), body)
			}
		}
	}
	if asked := host.asked.Load(); asked != 0 {
		t.Errorf("the artwork server was asked %d times: the image kept was not the one answered", asked)
	}
}

// Raster pictures are relayed with the type their bytes tell, whatever
// their server called them, resized or not, and sandboxed: opened as a
// page, a relayed answer runs and loads nothing.
func TestPicturesAreRelayedByWhatTheirBytesAre(t *testing.T) {
	t.Parallel()
	pictures := map[string]servedPicture{
		"jpeg":        {"image/jpeg", jpegOf(t, 600, 900)},
		"png":         {"image/png", pngOf(t, 600, 900)},
		"webp":        {"image/webp", webpPicture(t)},
		"gif":         {"image/gif", gifPicture(t)},
		"mislabelled": {"image/svg+xml", pngOf(t, 20, 30)},
		"set":         {"image/jpeg", jpegOf(t, 600, 900)},
	}
	types := map[string]string{"jpeg": "image/jpeg", "png": "image/png", "webp": "image/webp", "gif": "image/gif", "mislabelled": "image/png",
		"set": "image/jpeg"}
	host := newPictureHost(t, pictures)
	configure, addBoxSet := host.boxSets(t)
	s := newProbingServer(t, 10, "ffprobe-not-installed", configure)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	token := s.signIn("admin", "tv")
	movies := host.movies(t, s, token)
	addBoxSet(s, 10, "set")
	relayed := func(t *testing.T, name, target string, width int) {
		t.Helper()
		response, body := fetchURL(t, target, nil)
		if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != types[name] {
			t.Fatalf("%d %q, want %s", response.StatusCode, response.Header.Get("Content-Type"), types[name])
		}
		if !sandboxed(response.Header) {
			t.Errorf("the answer is not sandboxed: %q", response.Header.Get("Content-Security-Policy"))
		}
		if width == 0 {
			if body != string(pictures[name].body) {
				t.Errorf("the picture is not relayed as it is: %d bytes of %d", len(body), len(pictures[name].body))
			}
			return
		}
		if config, _, err := image.DecodeConfig(strings.NewReader(body)); err != nil || config.Width != width {
			t.Errorf("resized to %d (%v), want %d", config.Width, err, width)
		}
	}
	for name := range movies {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			relayed(t, name, s.url+"/Items/"+movies[name]+"/Images/Primary", 0)
		})
	}
	for _, name := range []string{"jpeg", "png"} {
		t.Run(name+", resized", func(t *testing.T) {
			t.Parallel()
			relayed(t, name, s.url+"/Items/"+movies[name]+"/Images/Primary?maxWidth=120", 120)
		})
	}
	t.Run("an image listed for a title", func(t *testing.T) {
		t.Parallel()
		var listed RemoteImageResult
		if status := s.get(t, "/Items/"+movies["png"]+"/RemoteImages?type=Primary", token, &listed); status != http.StatusOK || len(listed.Images) != 1 {
			t.Fatalf("the images listed: %d %+v", status, listed)
		}
		relayed(t, "png", listed.Images[0].Url, 0)
		relayed(t, "png", listed.Images[0].ThumbnailUrl, remoteThumbnailWidth)
	})
	t.Run("a box set's", func(t *testing.T) {
		t.Parallel()
		relayed(t, "set", s.url+"/Polyfin/BoxSets/10/Images/Primary", 0)
		relayed(t, "set", s.url+"/Polyfin/BoxSets/10/Images/Primary?maxWidth=120", 120)
	})
	// An app that kept a picture is told so, as before.
	response, _ := fetchURL(t, s.url+"/Items/"+movies["jpeg"]+"/Images/Primary", nil)
	if etag := response.Header.Get("ETag"); etag == "" {
		t.Error("a relayed picture has no ETag")
	} else if again, _ := fetchURL(t, s.url+"/Items/"+movies["jpeg"]+"/Images/Primary", http.Header{"If-None-Match": {etag}}); again.StatusCode != http.StatusNotModified {
		t.Errorf("a picture the app kept: %d", again.StatusCode)
	}
}
