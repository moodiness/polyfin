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
	"time"
)

const (
	webPrefix = "/web/"
	// webScript is the name Polyfin's own script is served under, in /web/,
	// beside the files of jellyfin-web.
	webScript = "polyfin.js"
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

// webClient serves jellyfin-web at /web/ as Jellyfin does: its files, and
// the requests no file answers to the Jellyfin API, which has routes under
// /web/ too. index.html is never cached and gets Polyfin's script; files
// whose name or query carries a build hash are cached for good. Other files
// are revalidated (Jellyfin sends no Cache-Control for them, which leaves
// browsers guessing a lifetime for config.json and the like).
func webClient(files fs.FS, setUp func(context.Context) bool, api http.Handler, logger *slog.Logger) http.Handler {
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
			serveWebIndex(w, r, files, logger)
			return
		case webScript:
			header := w.Header()
			header.Set("Content-Type", "text/javascript; charset=utf-8")
			header.Set("Cache-Control", "no-cache")
			header.Set("ETag", webScriptETag)
			http.ServeContent(w, r, webScript, time.Time{}, bytes.NewReader(webScriptBody))
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

// serveWebIndex serves jellyfin-web's index.html with Polyfin's script.
// The file is read at each request, as Jellyfin does, so that a new
// jellyfin-web needs no restart.
func serveWebIndex(w http.ResponseWriter, r *http.Request, files fs.FS, logger *slog.Logger) {
	page, err := fs.ReadFile(files, "index.html")
	if err != nil {
		logger.Error("The index.html of jellyfin-web cannot be read", "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	page = withWebScript(page)
	header := w.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Cache-Control", "no-cache")
	header.Set("ETag", etag(page))
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(page))
}

// withWebScript adds Polyfin's script to the end of the page's head, where
// it runs before the page's own scripts, which are deferred; a page without
// a head gets it at its end.
func withWebScript(page []byte) []byte {
	at := bytes.Index(bytes.ToLower(page), []byte("</head>"))
	if at < 0 {
		at = len(page)
	}
	return slices.Concat(page[:at], webScriptTag, page[at:])
}

func etag(content []byte) string {
	sum := sha256.Sum256(content)
	return `"` + hex.EncodeToString(sum[:8]) + `"`
}
