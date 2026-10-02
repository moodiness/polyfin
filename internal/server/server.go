// Package server assembles Polyfin's HTTP routes.
package server

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"
)

const (
	adminPrefix = "/admin/"
	readyWait   = 2 * time.Second
	// Hashed build files never change content under the same name.
	immutableCache = "public, max-age=31536000, immutable"
	adminPolicy    = "default-src 'self'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'self'"
)

// Pinger reports whether the database answers.
type Pinger interface {
	Ping(context.Context) error
}

// Options are the dependencies of the HTTP handler.
type Options struct {
	Database Pinger
	// Admin is the built admin application: index.html and its assets.
	Admin fs.FS
	// AdminAPI serves /admin/api/.
	AdminAPI http.Handler
	// Jellyfin serves every path not owned by Polyfin itself.
	Jellyfin http.Handler
	Logger   *slog.Logger
}

// New returns the handler serving every Polyfin route.
func New(options Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !databaseReady(r.Context(), options.Database) {
			writeText(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		writeText(w, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, adminPrefix, http.StatusFound)
	})
	mux.Handle("/admin/api/", options.AdminAPI)
	// No method in the pattern: it would conflict with "/admin/api/", which
	// must answer every method. adminApp restricts methods itself.
	mux.Handle("/admin/", adminApp(options.Admin, options.Logger))
	mux.Handle("/", options.Jellyfin)

	return securityHeaders(mux)
}

func databaseReady(ctx context.Context, db Pinger) bool {
	ctx, cancel := context.WithTimeout(ctx, readyWait)
	defer cancel()
	return db.Ping(ctx) == nil
}

// adminApp serves the single-page application. Paths without a file
// extension are client-side routes and receive index.html; a missing file
// with an extension is a real 404, so a stale asset never loads the page.
func adminApp(files fs.FS, logger *slog.Logger) http.Handler {
	fileServer := http.StripPrefix(adminPrefix, http.FileServerFS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, adminPrefix)
		if name == "" || name == "index.html" {
			serveIndex(w, files, logger)
			return
		}
		if info, err := fs.Stat(files, name); err == nil && !info.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", immutableCache)
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
		serveIndex(w, files, logger)
	})
}

func serveIndex(w http.ResponseWriter, files fs.FS, logger *slog.Logger) {
	page, err := fs.ReadFile(files, "index.html")
	if err != nil {
		logger.Error("The admin application is missing; build it with `make web`", "error", err)
		http.Error(w, "The admin application is not built.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(page)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "no-referrer")
		if r.URL.Path == "/admin" || strings.HasPrefix(r.URL.Path, adminPrefix) {
			header.Set("Content-Security-Policy", adminPolicy)
		}
		next.ServeHTTP(w, r)
	})
}

func writeText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
