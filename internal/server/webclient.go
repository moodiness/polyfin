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
// a lifetime for config.json and the like).
func webClient(files fs.FS, setUp func(context.Context) bool, customJs func() string, api http.Handler, logger *slog.Logger) http.Handler {
	custom := &customScripts{get: customJs}
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
		info, err := fs.Stat(files, name)
		if err != nil || !info.Mode().IsRegular() {
			api.ServeHTTP(w, r)
			return
		}
		file, err := files.Open(name)
		if err != nil {
			api.ServeHTTP(w, r)
			return
		}
		defer file.Close()
		content, ok := file.(io.ReadSeeker)
		if !ok {
			data, err := io.ReadAll(file)
			if err != nil {
				logger.Error("A file of jellyfin-web cannot be read", "file", name, "error", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			content = bytes.NewReader(data)
		}
		header := w.Header()
		if hashedName.MatchString(path.Base(name)) || buildQuery.MatchString(r.URL.RawQuery) {
			header.Set("Cache-Control", immutableCache)
		} else {
			header.Set("Cache-Control", "no-cache")
		}
		if contentType, ok := webTypes[path.Ext(name)]; ok {
			header.Set("Content-Type", contentType)
		}
		http.ServeContent(w, r, name, info.ModTime(), content)
	})
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
