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
	// Addon logos are hosted by each addon, over https.
	adminPolicy = "default-src 'self'; img-src 'self' https:; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'self'"
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
	// Web holds the files of jellyfin-web, the web client served at /web/;
	// nil serves none.
	Web fs.FS
	// SetupRequired reports whether no administrator exists yet. Needed
	// with Web only.
	SetupRequired func(context.Context) (bool, error)
	// CustomJs returns the administrator's script for the web client,
	// empty for none; nil is none.
	CustomJs func() string
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
	// Once Polyfin is set up, its web client is the place to go, as on
	// Jellyfin; before, or without one, the admin app is.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		target := adminPrefix
		if options.Web != nil && setUp(r.Context(), options.SetupRequired) {
			target = webPrefix
		}
		http.Redirect(w, r, target, http.StatusFound)
	})
	mux.Handle("/admin/api/", options.AdminAPI)
	// No method in the pattern: it would conflict with "/admin/api/", which
	// must answer every method. adminApp restricts methods itself.
	mux.Handle("/admin/", adminApp(options.Admin, options.Logger))
	if options.Web != nil {
		mux.Handle(webPrefix, webClient(options.Web, func(ctx context.Context) bool {
			return setUp(ctx, options.SetupRequired)
		}, options.CustomJs, options.Jellyfin, options.Logger))
		// Permanently, as Jellyfin does; the mux's own redirect is a 307.
		mux.HandleFunc("/web", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, webPrefix, http.StatusMovedPermanently)
		})
		// Jellyfin sends robots.txt to the web client's, which keeps search
		// engines away.
		mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, webPrefix+"robots.txt", http.StatusFound)
		})
	}
	mux.Handle("/", options.Jellyfin)

	return securityHeaders(mux)
}

func databaseReady(ctx context.Context, db Pinger) bool {
	ctx, cancel := context.WithTimeout(ctx, readyWait)
	defer cancel()
	return db.Ping(ctx) == nil
}

// setUp reports whether an administrator exists; an unreachable database
// counts as not set up, so that the admin app tells why.
func setUp(ctx context.Context, setupRequired func(context.Context) (bool, error)) bool {
	ctx, cancel := context.WithTimeout(ctx, readyWait)
	defer cancel()
	required, err := setupRequired(ctx)
	return err == nil && !required
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

// securityHeaders sets the headers every answer gets, and the admin app's
// Content-Security-Policy. jellyfin-web at /web/ gets no such policy, as
// from Jellyfin, which sends none: the admin app's would break it, as it
// needs inline styles, blob: workers and media, WebAssembly, images and
// frames from elsewhere (trailers), and pages that frame it.
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
