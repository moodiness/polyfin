// Package server assembles Polyfin's HTTP routes.
package server

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/forwarded"
)

const (
	adminPrefix = "/admin/"
	// jellyfinBase is the path apps also reach the Jellyfin API, its
	// WebSocket and the web client under, as on a Jellyfin server behind a
	// reverse proxy that serves it at /jellyfin: given an address without a
	// path, some apps try it before the root.
	jellyfinBase = "/jellyfin"
	readyWait    = 2 * time.Second
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
	// Proxies tells every route the client of requests that come through
	// a known proxy (see forwarded.Client); nil reads the connection's
	// address alone.
	Proxies *forwarded.Proxies
	Logger  *slog.Logger
}

// New returns the handler serving every Polyfin route. Answers that gain
// from it go gzip-compressed to clients that accept it (see compress).
// Every route sees the client Proxies resolves. The Jellyfin API, its
// WebSocket and the web client answer under jellyfinBase as at the root;
// the admin app, /healthz and /readyz stay at the root only.
func New(options Options) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /healthz", rootOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, http.StatusOK, "ok")
	}), options.Jellyfin))
	mux.Handle("GET /readyz", rootOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !databaseReady(r.Context(), options.Database) {
			writeText(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		writeText(w, http.StatusOK, "ok")
	}), options.Jellyfin))
	// Once Polyfin is set up, its web client is the place to go, as on
	// Jellyfin; before, or without one, the admin app is.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		target := adminPrefix
		if options.Web != nil && setUp(r.Context(), options.SetupRequired) {
			target = basePath(r) + webPrefix
		}
		http.Redirect(w, r, target, http.StatusFound)
	})
	mux.Handle("/admin/api/", rootOnly(options.AdminAPI, options.Jellyfin))
	// No method in the pattern: it would conflict with "/admin/api/", which
	// must answer every method. adminApp restricts methods itself.
	mux.Handle("/admin/", rootOnly(adminApp(options.Admin, options.Logger), options.Jellyfin))
	// As the mux would, but at the root only.
	mux.Handle("/admin", rootOnly(http.RedirectHandler(adminPrefix, http.StatusMovedPermanently), options.Jellyfin))
	if options.Web != nil {
		mux.Handle(webPrefix, webClient(options.Web, func(ctx context.Context) bool {
			return setUp(ctx, options.SetupRequired)
		}, options.CustomJs, options.Jellyfin, options.Logger))
		// Permanently, as Jellyfin does; the mux's own redirect is a 307.
		mux.HandleFunc("/web", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, basePath(r)+webPrefix, http.StatusMovedPermanently)
		})
		// Jellyfin sends robots.txt to the web client's, which keeps search
		// engines away.
		mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, basePath(r)+webPrefix+"robots.txt", http.StatusFound)
		})
	}
	mux.Handle("/", options.Jellyfin)

	handler := securityHeaders(compress(mux))
	if options.Proxies != nil {
		handler = options.Proxies.Handler(handler)
	}
	return underJellyfinBase(handler)
}

type baseKey struct{}

// underJellyfinBase serves a request whose path starts with jellyfinBase,
// matched without regard to case, as the same request without it, before
// anything else sees the path: jellyfinBase alone stands for the root.
// basePath tells the routes the base the app wrote.
func underJellyfinBase(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest, ok := cutBase(r.URL.Path)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		base := r.URL.Path[:len(jellyfinBase)]
		stripped := r.WithContext(context.WithValue(r.Context(), baseKey{}, base))
		stripped.URL = new(url.URL)
		*stripped.URL = *r.URL
		stripped.URL.Path = rest
		stripped.URL.RawPath = ""
		if raw, ok := cutBase(r.URL.RawPath); ok {
			stripped.URL.RawPath = raw
		}
		next.ServeHTTP(w, stripped)
	})
}

// cutBase returns p without jellyfinBase, and whether p was under it.
func cutBase(p string) (string, bool) {
	if len(p) < len(jellyfinBase) || !strings.EqualFold(p[:len(jellyfinBase)], jellyfinBase) {
		return "", false
	}
	rest := p[len(jellyfinBase):]
	switch {
	case rest == "":
		return "/", true
	case rest[0] == '/':
		return rest, true
	}
	return "", false
}

// basePath returns the jellyfinBase a request came under, as the app wrote
// it, or "" for the root, for the addresses Polyfin sends back.
func basePath(r *http.Request) string {
	base, _ := r.Context().Value(baseKey{}).(string)
	return base
}

// rootOnly serves requests with h at the root only: under jellyfinBase,
// other answers them, as a Jellyfin server at /jellyfin would.
func rootOnly(h, other http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if basePath(r) != "" {
			other.ServeHTTP(w, r)
			return
		}
		h.ServeHTTP(w, r)
	})
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
// Text files go compressed to clients that take gzip, compressed once each.
func adminApp(files fs.FS, logger *slog.Logger) http.Handler {
	fileServer := http.StripPrefix(adminPrefix, http.FileServerFS(files))
	compressed := newGzipFiles(files)
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
			if !compressed.serve(w, r, name, info, "") {
				fileServer.ServeHTTP(w, r)
			}
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
		if basePath(r) == "" && (r.URL.Path == "/admin" || strings.HasPrefix(r.URL.Path, adminPrefix)) {
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
