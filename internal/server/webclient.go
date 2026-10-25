package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/moodiness/polyfin/internal/themecopy"
	"github.com/moodiness/polyfin/internal/webtheme"
)

const (
	webPrefix = "/web/"
	// webScript is the name Polyfin's own script is served under, in /web/,
	// beside the files of jellyfin-web.
	webScript = "polyfin.js"
	// customScript is the name the administrator's own script, set in the
	// admin app, is served under, in /web/.
	customScript = "custom.js"
)

var (
	//go:embed webclient.js
	webScriptBody []byte
	webScriptETag = etag(webScriptBody)
	// webScriptTag loads Polyfin's script from jellyfin-web's index.html.
	webScriptTag = []byte(`<script src="` + webScript + `"></script>`)
	// hashedName matches the content hash webpack puts in the names of
	// jellyfin-web's chunks, styles, fonts and images.
	hashedName = regexp.MustCompile(`\.[0-9a-f]{16,}\.`)
	// buildQuery matches the build hash index.html adds as the query of
	// the bundles whose names carry none.
	buildQuery = regexp.MustCompile(`^[0-9a-f]{16,}$`)
	// webTypes are the content types of jellyfin-web's files that Go does
	// not know on every system; Jellyfin also serves .data and .mem, which
	// subtitle rendering loads, as binary.
	webTypes = map[string]string{
		".woff2": "font/woff2",
		".woff":  "font/woff",
		".ttf":   "font/ttf",
		".eot":   "application/vnd.ms-fontobject",
		".ico":   "image/x-icon",
		".mp3":   "audio/mpeg",
		".txt":   "text/plain; charset=utf-8",
		".data":  "application/octet-stream",
		".mem":   "application/octet-stream",
	}
	// themeTypes are the content types of the bundled theme's files, which
	// a system's own types could name otherwise.
	themeTypes = map[string]string{
		".css": "text/css; charset=utf-8",
		".js":  "text/javascript; charset=utf-8",
	}
	// themeFiles are the bundled theme's files, compressed once each.
	themeFiles = newGzipFiles(webtheme.Files)
)

// WebClientFiles returns the files of jellyfin-web in dir, or nil when dir
// holds no index.html: then Polyfin has no web client.
func WebClientFiles(dir string) fs.FS {
	if dir == "" {
		return nil
	}
	files := os.DirFS(dir)
	if info, err := fs.Stat(files, "index.html"); err != nil || !info.Mode().IsRegular() {
		return nil
	}
	return files
}

// customScripts hashes the administrator's script, again only when it
// changed, for the address index.html loads it from.
type customScripts struct {
	get  func() string
	last atomic.Pointer[hashedScript]
}

// hashedScript is a script and the hash of its content, which index.html
// puts in its address so that browsers keep it until it changes.
type hashedScript struct {
	body, hash string
}

func (c *customScripts) current() hashedScript {
	if c.get == nil {
		return hashedScript{}
	}
	body := c.get()
	if last := c.last.Load(); last != nil && last.body == body {
		return *last
	}
	sum := sha256.Sum256([]byte(body))
	script := &hashedScript{body: body, hash: hex.EncodeToString(sum[:8])}
	c.last.Store(script)
	return *script
}

// webClient serves jellyfin-web at /web/ as Jellyfin does: its files, and
// the requests no file answers to the Jellyfin API, which has routes under
// /web/ too. index.html is never cached and gets Polyfin's script, and the
// administrator's when there is one; files whose name or query carries a
// build or content hash are cached for good. Other files are revalidated
// (Jellyfin sends no Cache-Control for them, which leaves browsers guessing
// a lifetime for config.json and the like). The default theme's files are
// Polyfin's own, under webtheme.Path (see serveTheme), and the copy of a
// theme's files kept in savedThemes under themecopy.Path (see
// serveSavedTheme). Text files go
// compressed to clients that take gzip, compressed once each (see
// gzipFiles).
func webClient(files fs.FS, setUp func(context.Context) bool, customJs func() string, savedThemes string, savedOriginal func(id, name string) (string, bool), api http.Handler, logger *slog.Logger) http.Handler {
	custom := &customScripts{get: customJs}
	compressed := newGzipFiles(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			api.ServeHTTP(w, r)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, webPrefix)
		switch name {
		case "", "index.html":
			// Before setup no account can sign in: the setup page is in the
			// admin app, as Jellyfin's web client opens its startup wizard.
			if !setUp(r.Context()) {
				http.Redirect(w, r, adminPrefix, http.StatusFound)
				return
			}
			serveWebIndex(w, r, files, custom.current(), logger)
			return
		case webScript:
			header := w.Header()
			header.Set("Content-Type", "text/javascript; charset=utf-8")
			header.Set("Cache-Control", "no-cache")
			header.Set("ETag", webScriptETag)
			http.ServeContent(w, r, webScript, time.Time{}, bytes.NewReader(webScriptBody))
			return
		case customScript:
			script := custom.current()
			if script.body == "" {
				api.ServeHTTP(w, r)
				return
			}
			header := w.Header()
			header.Set("Content-Type", "application/javascript; charset=utf-8")
			// The address index.html gives names this very content; any
			// other is revalidated, so an old address gets today's script.
			if r.URL.Query().Get("v") == script.hash {
				header.Set("Cache-Control", immutableCache)
			} else {
				header.Set("Cache-Control", "no-cache")
			}
			header.Set("ETag", `"`+script.hash+`"`)
			http.ServeContent(w, r, customScript, time.Time{}, strings.NewReader(script.body))
			return
		}
		if rest, ok := strings.CutPrefix(name, webtheme.Path); ok {
			serveTheme(w, r, rest, logger)
			return
		}
		if rest, ok := strings.CutPrefix(name, themecopy.Path); ok {
			serveSavedTheme(w, r, savedThemes, savedOriginal, rest)
			return
		}
		info, err := fs.Stat(files, name)
		if err != nil || !info.Mode().IsRegular() {
			api.ServeHTTP(w, r)
			return
		}
		header := w.Header()
		if hashedName.MatchString(path.Base(name)) || buildQuery.MatchString(r.URL.RawQuery) {
			header.Set("Cache-Control", immutableCache)
		} else {
			header.Set("Cache-Control", "no-cache")
		}
		contentType := webTypes[path.Ext(name)]
		if contentType != "" {
			header.Set("Content-Type", contentType)
		}
		if compressed.serve(w, r, name, info, contentType) {
			return
		}
		serveFile(w, r, files, name, info, api, logger)
	})
}

// serveFile sends the file name of files, whose information is info, as
// it is; when it cannot be opened, missing answers instead.
func serveFile(w http.ResponseWriter, r *http.Request, files fs.FS, name string, info fs.FileInfo, missing http.Handler, logger *slog.Logger) {
	file, err := files.Open(name)
	if err != nil {
		missing.ServeHTTP(w, r)
		return
	}
	defer file.Close()
	content, ok := file.(io.ReadSeeker)
	if !ok {
		data, err := io.ReadAll(file)
		if err != nil {
			logger.Error("A file of the web client cannot be read", "file", name, "error", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		content = bytes.NewReader(data)
	}
	http.ServeContent(w, r, name, info.ModTime(), content)
}

// savedThemePolicy keeps a file of a theme's copy from running anything
// when it is opened on its own, such as an SVG image with a script, which
// would run at Polyfin's address: a stylesheet, script, font or image a
// page loads ignores it.
const savedThemePolicy = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox"

// serveSavedTheme serves the file rest names of the copy of a theme's files
// kept in dir (see themecopy): a copy's id changes with each copy, so its
// files are cached for good, served with the type of their extension.
// Only the files of a copy's folder are served, never a path out of dir.
// A file of the copy kept missing from dir is redirected to the address
// original gives it, the one it came from; nil sends none.
func serveSavedTheme(w http.ResponseWriter, r *http.Request, dir string, original func(id, name string) (string, bool), rest string) {
	file, err := themecopy.OpenFile(dir, rest)
	if err != nil {
		// A file of the copy kept that is missing, as after a backup
		// restored on another system, is fetched by the browser from where
		// it came, which the theme named before the copy: the theme keeps
		// working online. The redirect is not kept, for the file may come
		// back with the copy made again.
		if id, name, ok := strings.Cut(rest, "/"); ok && original != nil {
			if address, ok := original(id, name); ok {
				w.Header().Set("Cache-Control", "no-store")
				http.Redirect(w, r, address, http.StatusFound)
				return
			}
		}
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	header := w.Header()
	header.Set("Cache-Control", immutableCache)
	header.Set("Content-Type", themecopy.ContentTypes[path.Ext(rest)])
	header.Set("Content-Security-Policy", savedThemePolicy)
	http.ServeContent(w, r, path.Base(rest), info.ModTime(), file)
}

// serveTheme serves the bundled theme's file at rest, a version then the
// file's name. At the version of the bundled files, it is cached for good;
// at any other it is redirected there, revalidated, so that it gets
// today's files: "latest", which the theme's script asks for when it
// cannot tell its own address, and the version an earlier Polyfin bundled,
// which an administrator's own script may still name.
func serveTheme(w http.ResponseWriter, r *http.Request, rest string, logger *slog.Logger) {
	version, name, _ := strings.Cut(rest, "/")
	info, err := fs.Stat(webtheme.Files, name)
	if version == "" || !fs.ValidPath(name) || err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	header := w.Header()
	if version != webtheme.Version {
		target := basePath(r) + webPrefix + webtheme.Address(name)
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		header.Set("Cache-Control", "no-cache")
		http.Redirect(w, r, target, http.StatusFound)
		return
	}
	header.Set("Cache-Control", immutableCache)
	contentType := themeTypes[path.Ext(name)]
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	if themeFiles.serve(w, r, name, info, contentType) {
		return
	}
	serveFile(w, r, webtheme.Files, name, info, http.NotFoundHandler(), logger)
}

// serveWebIndex serves jellyfin-web's index.html with Polyfin's script,
// and the administrator's custom script when there is one. The file is read
// at each request, as Jellyfin does, so that a new jellyfin-web needs no
// restart.
func serveWebIndex(w http.ResponseWriter, r *http.Request, files fs.FS, custom hashedScript, logger *slog.Logger) {
	page, err := fs.ReadFile(files, "index.html")
	if err != nil {
		logger.Error("The index.html of jellyfin-web cannot be read", "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	page = withWebScripts(page, custom)
	header := w.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Cache-Control", "no-cache")
	header.Set("ETag", etag(page))
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(page))
}

// withWebScripts adds Polyfin's script to the end of the page's head, where
// it runs before the page's own scripts, which are deferred; a page without
// a head gets it at its end. The custom script, when there is one, comes
// right after, deferred: it runs once the page is parsed, after
// jellyfin-web's own scripts.
func withWebScripts(page []byte, custom hashedScript) []byte {
	at := bytes.Index(bytes.ToLower(page), []byte("</head>"))
	if at < 0 {
		at = len(page)
	}
	var customTag []byte
	if custom.body != "" {
		customTag = []byte(`<script src="` + customScript + `?v=` + custom.hash + `" defer></script>`)
	}
	return slices.Concat(page[:at], webScriptTag, customTag, page[at:])
}

func etag(content []byte) string {
	sum := sha256.Sum256(content)
	return `"` + hex.EncodeToString(sum[:8]) + `"`
}
