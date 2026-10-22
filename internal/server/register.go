package server

import (
	"bytes"
	_ "embed"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// registerPage is the page at /register where visitors create their own
// member account, when Settings › Security › Registration is on.
//
//go:embed register.html
var registerPage []byte

// registerPolicy lets the page run its own inline script and style, load
// the Inter font, and nothing else from elsewhere.
const registerPolicy = "default-src 'self'; script-src 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
	"font-src https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

// registration serves /register: the page, its API at /register/api (the
// admin API's /admin/api/register, so that it also works under a base
// path, where the admin API does not), and Jellyfin's logo for a server
// without its own, from the web client's files.
func registration(api http.Handler, web fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimSuffix(r.URL.Path, "/") {
		case "/register":
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
				return
			}
			if strings.HasSuffix(r.URL.Path, "/") {
				http.Redirect(w, r, basePath(r)+"/register", http.StatusMovedPermanently)
				return
			}
			header := w.Header()
			header.Set("Content-Type", "text/html; charset=utf-8")
			header.Set("Cache-Control", "no-cache")
			header.Set("Content-Security-Policy", registerPolicy)
			header.Set("X-Frame-Options", "DENY")
			http.ServeContent(w, r, "register.html", time.Time{}, bytes.NewReader(registerPage))
		case "/register/api":
			forwarded := r.Clone(r.Context())
			forwarded.URL = new(url.URL)
			*forwarded.URL = *r.URL
			forwarded.URL.Path, forwarded.URL.RawPath = "/admin/api/register", ""
			api.ServeHTTP(w, forwarded)
		case "/register/jellyfin-logo.png":
			serveJellyfinLogo(w, r, web)
		default:
			http.NotFound(w, r)
		}
	})
}

// serveJellyfinLogo serves jellyfin-web's own wide logo, whose name carries
// a build hash.
func serveJellyfinLogo(w http.ResponseWriter, r *http.Request, web fs.FS) {
	if web == nil {
		http.NotFound(w, r)
		return
	}
	names, _ := fs.Glob(web, "banner-light*.png")
	if len(names) == 0 {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(web, names[0])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	header := w.Header()
	header.Set("Content-Type", "image/png")
	header.Set("Cache-Control", "public, max-age=86400")
	http.ServeContent(w, r, names[0], time.Time{}, bytes.NewReader(data))
}
