package server

import (
	"bytes"
	"crypto/rand"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

// countingFS counts the files opened, to tell how often a file is read;
// Stat does not open.
type countingFS struct {
	files fs.FS
	mu    sync.Mutex
	opens map[string]int
}

func counting(files fs.FS) *countingFS {
	return &countingFS{files: files, opens: map[string]int{}}
}

func (c *countingFS) Open(name string) (fs.File, error) {
	c.mu.Lock()
	c.opens[name]++
	c.mu.Unlock()
	return c.files.Open(name)
}

func (c *countingFS) Stat(name string) (fs.FileInfo, error) { return fs.Stat(c.files, name) }

func (c *countingFS) opened(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.opens[name]
}

// script returns JavaScript of about size bytes.
func script(size int) []byte {
	return bytes.Repeat([]byte("function f(){return document.querySelector('.card')}\n"), size/52)
}

func randomBytes(t *testing.T, size int) []byte {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	return data
}

// staticFilesAreCompressedOnce checks a file of a web application: every
// client that takes gzip gets it compressed, with its own ETag, from a
// single compression, even when the first requests come at once; others,
// and Range requests, get the file as it is.
func staticFilesAreCompressedOnce(t *testing.T, h http.Handler, files *countingFS, target, name string, body []byte, contentType, cache string) {
	t.Helper()
	responses := make([][]byte, 16)
	tags := make([]string, len(responses))
	var wait sync.WaitGroup
	for i := range responses {
		wait.Go(func() {
			response := fetch(h, http.MethodGet, target, "Accept-Encoding", "gzip, br")
			header := response.Header()
			if response.Code != http.StatusOK || header.Get("Content-Encoding") != "gzip" || !varies(header) ||
				header.Get("Content-Type") != contentType || header.Get("Cache-Control") != cache {
				t.Errorf("%s: %d, Content-Encoding %q, Vary %q, Content-Type %q, Cache-Control %q", target, response.Code,
					header.Get("Content-Encoding"), header.Values("Vary"), header.Get("Content-Type"), header.Get("Cache-Control"))
			}
			responses[i], tags[i] = response.Body.Bytes(), header.Get("ETag")
		})
	}
	wait.Wait()
	if t.Failed() {
		return
	}
	for i, response := range responses {
		if !bytes.Equal(gunzip(t, response), body) {
			t.Fatalf("%s: the compressed file differs", target)
		}
		if !strings.HasSuffix(tags[i], `-gzip"`) || tags[i] != tags[0] {
			t.Errorf("%s: ETag %q", target, tags[i])
		}
	}
	if n := files.opened(name); n != 1 {
		t.Errorf("%s: read %d times for %d clients, want once", target, n, len(responses))
	}
	if response := fetch(h, http.MethodGet, target, "Accept-Encoding", "gzip", "If-None-Match", tags[0]); response.Code != http.StatusNotModified {
		t.Errorf("%s: revalidating with the compressed ETag answered %d", target, response.Code)
	}
	if response := fetch(h, http.MethodGet, target, "Accept-Encoding", "gzip", "If-None-Match", `"other"`); response.Code != http.StatusOK ||
		!bytes.Equal(gunzip(t, response.Body.Bytes()), body) {
		t.Errorf("%s: another ETag answered %d", target, response.Code)
	}
	ranged := fetch(h, http.MethodGet, target, "Accept-Encoding", "gzip", "Range", "bytes=0-99")
	if ranged.Code != http.StatusPartialContent || ranged.Header().Get("Content-Encoding") != "" || !bytes.Equal(ranged.Body.Bytes(), body[:100]) {
		t.Errorf("%s: a range answered %d, Content-Encoding %q", target, ranged.Code, ranged.Header().Get("Content-Encoding"))
	}
	identity := fetch(h, http.MethodGet, target)
	if identity.Header().Get("Content-Encoding") != "" || !bytes.Equal(identity.Body.Bytes(), body) || !varies(identity.Header()) ||
		identity.Header().Get("ETag") == tags[0] {
		t.Errorf("%s: without gzip: Content-Encoding %q, Vary %q, ETag %q", target,
			identity.Header().Get("Content-Encoding"), identity.Header().Values("Vary"), identity.Header().Get("ETag"))
	}
}

func TestWebClientFilesAreCompressedOnce(t *testing.T) {
	bundle := script(40000)
	font := randomBytes(t, 4000)
	noise := randomBytes(t, 4000)
	mapFS := fstest.MapFS{
		"index.html":                         {Data: []byte(webIndex)},
		"main.jellyfin.bundle.js":            {Data: bundle, ModTime: time.Unix(1700000000, 0)},
		"8013.0123456789abcdef0123.chunk.js": {Data: bundle},
		"font.0123456789abcdef0123.woff2":    {Data: font},
		"noise.txt":                          {Data: noise},
	}
	files := counting(mapFS)
	h := New(Options{
		Database:      database{},
		Admin:         fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}},
		AdminAPI:      owner("admin api"),
		Jellyfin:      owner("jellyfin"),
		Web:           files,
		SetupRequired: setupState{}.SetupRequired,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	staticFilesAreCompressedOnce(t, h, files, "/web/8013.0123456789abcdef0123.chunk.js", "8013.0123456789abcdef0123.chunk.js",
		bundle, "text/javascript; charset=utf-8", immutableCache)
	staticFilesAreCompressedOnce(t, h, files, "/web/main.jellyfin.bundle.js?0123456789abcdef0123", "main.jellyfin.bundle.js",
		bundle, "text/javascript; charset=utf-8", immutableCache)
	// Revalidated by date too, whichever variant the client holds.
	modified := fetch(h, http.MethodGet, "/web/main.jellyfin.bundle.js", "Accept-Encoding", "gzip",
		"If-Modified-Since", time.Unix(1700000000, 0).UTC().Format(http.TimeFormat))
	if modified.Code != http.StatusNotModified {
		t.Errorf("If-Modified-Since: %d", modified.Code)
	}

	// A new jellyfin-web is compressed again.
	changed := script(30000)
	reads := files.opened("main.jellyfin.bundle.js")
	mapFS["main.jellyfin.bundle.js"] = &fstest.MapFile{Data: changed, ModTime: time.Unix(1800000000, 0)}
	for range 2 {
		response := fetch(h, http.MethodGet, "/web/main.jellyfin.bundle.js", "Accept-Encoding", "gzip")
		if !bytes.Equal(gunzip(t, response.Body.Bytes()), changed) {
			t.Error("a changed file: the old content is served")
		}
	}
	if n := files.opened("main.jellyfin.bundle.js") - reads; n != 1 {
		t.Errorf("a changed file: read %d times, want once", n)
	}

	// Fonts are compressed already, and text that does not shrink is sent
	// as it is, without compressing it again at each request.
	for target, body := range map[string][]byte{"/web/font.0123456789abcdef0123.woff2": font, "/web/noise.txt": noise} {
		for range 2 {
			response := fetch(h, http.MethodGet, target, "Accept-Encoding", "gzip")
			if response.Header().Get("Content-Encoding") != "" || !bytes.Equal(response.Body.Bytes(), body) {
				t.Errorf("%s: Content-Encoding %q", target, response.Header().Get("Content-Encoding"))
			}
		}
	}
}

func TestAdminFilesAreCompressedOnce(t *testing.T) {
	app := script(20000)
	icon := []byte(`<svg xmlns="http://www.w3.org/2000/svg">` + strings.Repeat(`<path d="M0 0h24v24H0z"/>`, 200) + `</svg>`)
	files := counting(fstest.MapFS{
		"index.html":         {Data: []byte("<!doctype html><title>Polyfin</title>")},
		"assets/app-1a2b.js": {Data: app},
		"polyfin.svg":        {Data: icon},
	})
	h := New(Options{
		Database: database{},
		Admin:    files,
		AdminAPI: owner("admin api"),
		Jellyfin: owner("jellyfin"),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	staticFilesAreCompressedOnce(t, h, files, "/admin/assets/app-1a2b.js", "assets/app-1a2b.js", app, "text/javascript; charset=utf-8", immutableCache)
	staticFilesAreCompressedOnce(t, h, files, "/admin/polyfin.svg", "polyfin.svg", icon, "image/svg+xml", "no-cache")
	if response := fetch(h, http.MethodGet, "/admin/polyfin.svg", "Accept-Encoding", "gzip"); response.Header().Get("Content-Security-Policy") == "" {
		t.Error("a compressed admin file has no Content-Security-Policy")
	}
}
