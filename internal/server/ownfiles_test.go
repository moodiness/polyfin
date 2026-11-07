package server

import (
	"bytes"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/andybalholm/brotli"

	"github.com/moodiness/polyfin/internal/jellyfin"
)

// adminFiles stands for the built admin app: its public files, as they
// are, and hashed assets.
func adminFiles(t *testing.T) fstest.MapFS {
	t.Helper()
	files := fstest.MapFS{
		"index.html":                  {Data: []byte(`<!doctype html><link rel="icon" href="/admin/polyfin.ico"><script src="/admin/assets/index-1a2b3c4d.js"></script>`)},
		"assets/index-1a2b3c4d.js":    {Data: script(40000)},
		"assets/index-1a2b3c4d.css":   {Data: bytes.Repeat([]byte(".card{color:var(--ink);padding:1rem}\n"), 1000)},
		"assets/figtree-1a2b3c.woff2": {Data: randomBytes(t, 4000)},
	}
	public := os.DirFS("../../web/public")
	err := fs.WalkDir(public, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := fs.ReadFile(public, name)
		files[name] = &fstest.MapFile{Data: data}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func ownFilesHandler(admin fs.FS) http.Handler {
	return New(Options{
		Database:    database{},
		Admin:       admin,
		AdminAPI:    owner("admin api"),
		Jellyfin:    owner("jellyfin"),
		Share:       owner("share"),
		ShareAssets: jellyfin.ShareAssets,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

// shareTargets are the addresses of every file of /share/assets/, by name
// and with the version the share page links it with.
func shareTargets(t *testing.T) []string {
	t.Helper()
	var targets []string
	err := fs.WalkDir(jellyfin.ShareAssets, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := fs.ReadFile(jellyfin.ShareAssets, name)
		targets = append(targets, shareAssetsPrefix+name, shareAssetsPrefix+name+"?v="+strings.Trim(etag(data), `"`))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return targets
}

// Every file under /admin/ and /share/assets/ is cached for good, or has
// an ETag that a second visit gets a 304 for, whatever encodings the
// browser takes: index.html is checked again at each load. The share
// page's files linked with their version are cached for good.
func TestOwnFilesAreCachedForGoodOrRevalidated(t *testing.T) {
	admin := adminFiles(t)
	h := ownFilesHandler(admin)
	targets := []string{"/admin/", "/admin/users/42"}
	for name := range admin {
		targets = append(targets, adminPrefix+name)
	}
	targets = append(targets, shareTargets(t)...)
	for _, target := range targets {
		for _, accept := range []string{"", "gzip, deflate", "gzip, deflate, br, zstd"} {
			response := fetch(h, http.MethodGet, target, "Accept-Encoding", accept)
			header := response.Header()
			cache := header.Get("Cache-Control")
			if response.Code != http.StatusOK {
				t.Errorf("%s (%q): %d", target, accept, response.Code)
				continue
			}
			if html := strings.HasPrefix(header.Get("Content-Type"), "text/html"); html && cache != "no-cache" {
				t.Errorf("%s (%q): the page has Cache-Control %q", target, accept, cache)
			}
			if strings.Contains(target, "?v=") && cache != immutableCache {
				t.Errorf("%s (%q): linked with its version, Cache-Control %q", target, accept, cache)
			}
			if cache == immutableCache {
				continue
			}
			tag := header.Get("ETag")
			if tag == "" {
				t.Errorf("%s (%q): neither cached for good (Cache-Control %q) nor tagged", target, accept, cache)
				continue
			}
			again := fetch(h, http.MethodGet, target, "Accept-Encoding", accept, "If-None-Match", tag)
			if again.Code != http.StatusNotModified || again.Body.Len() != 0 || again.Header().Get("ETag") != tag {
				t.Errorf("%s (%q): revalidating %s answered %d, %d bytes, ETag %q", target, accept, tag, again.Code,
					again.Body.Len(), again.Header().Get("ETag"))
			}
		}
	}
	// A version that is not the file's is checked again.
	stale := fetch(h, http.MethodGet, shareAssetsPrefix+"share.js?v=0123456789abcdef")
	if stale.Code != http.StatusOK || stale.Header().Get("Cache-Control") == immutableCache {
		t.Errorf("another version: %d, Cache-Control %q", stale.Code, stale.Header().Get("Cache-Control"))
	}
}

// unbrotli decodes a brotli body.
func unbrotli(t *testing.T, body []byte) []byte {
	t.Helper()
	data, err := io.ReadAll(brotli.NewReader(bytes.NewReader(body)))
	if err != nil {
		t.Fatalf("broken brotli: %v", err)
	}
	return data
}

// The admin app's and the share page's scripts and styles go in brotli to
// browsers that take it, once made, in gzip to the others that take gzip,
// and as they are to the rest, each with an ETag of its own. Until brotli
// is made, gzip goes; a browser that holds the gzip copy keeps it then.
func TestOwnScriptsAndStylesComeInBrotli(t *testing.T) {
	admin := adminFiles(t)
	h := ownFilesHandler(admin)
	files := map[string][]byte{
		"/admin/assets/index-1a2b3c4d.js":  admin["assets/index-1a2b3c4d.js"].Data,
		"/admin/assets/index-1a2b3c4d.css": admin["assets/index-1a2b3c4d.css"].Data,
	}
	for _, name := range []string{"share.js", "share.css", "hls.min.js"} {
		data, err := fs.ReadFile(jellyfin.ShareAssets, name)
		if err != nil {
			t.Fatal(err)
		}
		files[shareAssetsPrefix+name] = data
	}
	for target, body := range files {
		first := fetch(h, http.MethodGet, target, "Accept-Encoding", "gzip, deflate, br, zstd")
		if encoding := first.Header().Get("Content-Encoding"); encoding != "gzip" && encoding != "br" {
			t.Errorf("%s: the first answer is in %q", target, encoding)
		}
		gzipped := fetch(h, http.MethodGet, target, "Accept-Encoding", "gzip, deflate")
		if gzipped.Header().Get("Content-Encoding") != "gzip" || !bytes.Equal(gunzip(t, gzipped.Body.Bytes()), body) {
			t.Errorf("%s: gzip only: Content-Encoding %q", target, gzipped.Header().Get("Content-Encoding"))
		}
		held := fetch(h, http.MethodGet, target, "Accept-Encoding", "gzip, deflate, br, zstd", "If-None-Match", gzipped.Header().Get("ETag"))
		if held.Code != http.StatusNotModified {
			t.Errorf("%s: a browser that holds the gzip copy got %d", target, held.Code)
		}
		var response *httptest.ResponseRecorder
		for deadline := time.Now().Add(time.Minute); ; time.Sleep(10 * time.Millisecond) {
			response = fetch(h, http.MethodGet, target, "Accept-Encoding", "gzip, deflate, br, zstd")
			if response.Header().Get("Content-Encoding") == "br" || time.Now().After(deadline) {
				break
			}
		}
		header := response.Header()
		if header.Get("Content-Encoding") != "br" || !varies(header) || !strings.HasSuffix(header.Get("ETag"), `-br"`) {
			t.Errorf("%s: Content-Encoding %q, Vary %q, ETag %q", target, header.Get("Content-Encoding"), header.Values("Vary"), header.Get("ETag"))
			continue
		}
		if !bytes.Equal(unbrotli(t, response.Body.Bytes()), body) {
			t.Errorf("%s: the brotli copy differs", target)
		}
		if response.Body.Len() >= gzipped.Body.Len() {
			t.Errorf("%s: brotli %d bytes, gzip %d", target, response.Body.Len(), gzipped.Body.Len())
		}
		again := fetch(h, http.MethodGet, target, "Accept-Encoding", "br", "If-None-Match", header.Get("ETag"))
		if again.Code != http.StatusNotModified {
			t.Errorf("%s: revalidating the brotli copy answered %d", target, again.Code)
		}
		refused := fetch(h, http.MethodGet, target, "Accept-Encoding", "br;q=0, gzip")
		if refused.Header().Get("Content-Encoding") != "gzip" {
			t.Errorf("%s: br;q=0, gzip: Content-Encoding %q", target, refused.Header().Get("Content-Encoding"))
		}
		for _, accept := range []string{"", "identity", "br;q=0, gzip;q=0"} {
			plain := fetch(h, http.MethodGet, target, "Accept-Encoding", accept)
			if plain.Header().Get("Content-Encoding") != "" || !bytes.Equal(plain.Body.Bytes(), body) || plain.Header().Get("ETag") == "" {
				t.Errorf("%s (%q): Content-Encoding %q, ETag %q", target, accept, plain.Header().Get("Content-Encoding"), plain.Header().Get("ETag"))
			}
		}
	}
}
