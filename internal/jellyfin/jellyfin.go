// Package jellyfin serves the Jellyfin 12.1 HTTP API to Jellyfin apps.
package jellyfin

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/preferences"
	"github.com/moodiness/polyfin/internal/quickconnect"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/throttle"
)

const (
	// Version is the Jellyfin server version Polyfin reproduces.
	Version = "12.1.0"
	// productName identifies a Jellyfin-compatible server to apps; it is a
	// protocol value, not a claim to be Jellyfin.
	productName = "Jellyfin Server"
)

// Options are the dependencies of the Jellyfin API.
type Options struct {
	ServerID     string
	Accounts     *accounts.Store
	QuickConnect *quickconnect.Store
	// SignIns throttles failed password sign-ins per client address.
	SignIns *throttle.Failures
	// WebSocketPort is the port apps reach the server on.
	WebSocketPort int
	// Library browses the catalogs of users' addons.
	Library *library.Service
	// Stremio downloads artwork referenced by addons.
	Stremio *stremio.Client
	// Preferences stores the display preferences of Jellyfin apps.
	Preferences *preferences.Store
	Logger      *slog.Logger
}

// Handler serves the Jellyfin API.
type Handler struct {
	Options
	routes http.Handler
	images imageCache
}

// New returns the Jellyfin API handler.
func New(options Options) *Handler {
	h := &Handler{Options: options}
	rt := &router{}
	anonymous := func(method, pattern string, handler http.HandlerFunc) { rt.handle(method, pattern, handler) }
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}

	anonymous(http.MethodGet, "/System/Info/Public", h.publicSystemInfo)
	signedIn(http.MethodGet, "/System/Info", h.systemInfo)
	anonymous(http.MethodGet, "/System/Ping", h.ping)
	anonymous(http.MethodPost, "/System/Ping", h.ping)

	anonymous(http.MethodGet, "/Branding/Configuration", h.brandingConfiguration)
	anonymous(http.MethodGet, "/Branding/Css", h.brandingCSS)
	anonymous(http.MethodGet, "/Branding/Css.css", h.brandingCSS)

	anonymous(http.MethodGet, "/Users/Public", h.publicUsers)
	anonymous(http.MethodPost, "/Users/AuthenticateByName", h.authenticateByName)
	anonymous(http.MethodPost, "/Users/AuthenticateWithQuickConnect", h.authenticateWithQuickConnect)
	signedIn(http.MethodGet, "/Users/Me", h.currentUser)
	signedIn(http.MethodGet, "/Users", h.users)
	signedIn(http.MethodGet, "/Users/{userId}", h.user)

	anonymous(http.MethodGet, "/QuickConnect/Enabled", h.quickConnectEnabled)
	anonymous(http.MethodPost, "/QuickConnect/Initiate", h.quickConnectInitiate)
	anonymous(http.MethodGet, "/QuickConnect/Connect", h.quickConnectConnect)
	signedIn(http.MethodPost, "/QuickConnect/Authorize", h.quickConnectAuthorize)

	signedIn(http.MethodPost, "/Sessions/Logout", h.logout)
	signedIn(http.MethodPost, "/Sessions/Capabilities", h.capabilities)
	signedIn(http.MethodPost, "/Sessions/Capabilities/Full", h.fullCapabilities)

	h.browseRoutes(rt)
	h.auxiliaryRoutes(rt)

	h.routes = cors(rt)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.routes.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// processingError answers like Jellyfin's exception middleware.
func processingError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	_, _ = w.Write([]byte("Error processing request."))
}

func (h *Handler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.Logger.Error("Jellyfin API request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	processingError(w, http.StatusInternalServerError)
}

func (h *Handler) brandingConfiguration(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, BrandingOptions{})
}

func (h *Handler) brandingCSS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}
