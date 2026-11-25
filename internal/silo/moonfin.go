package silo

import (
	"errors"
	"io"
	"net/http"
	"strings"
)

// Moonfin's companion, for Moonfin's apps signed in through Silo's API.
//
// A Moonfin app signed in to a Silo server finds the companion among the
// plugins the server lists for the signed-in profile (GET
// /api/v2/settings/plugins), by its plugin identifier, then calls its
// routes under the plugin's content path,
// /api/v2/plugin-content/plugins/{installation}/Moonfin/... Polyfin lists
// it while the companion is served to the user, and answers those routes
// as the Jellyfin API's handler answers /Moonfin/... for the same session
// (see the jellyfin package's moonfin.go). Silo's own apps never read the
// list.

const (
	// moonfinPlugin is the plugin identifier Moonfin's apps look for, and
	// moonfinInstallation the installation Polyfin lists it as.
	moonfinPlugin       = "moonfin.moonbase"
	moonfinInstallation = "moonfin"
	// moonfinBodyBytes bounds the body a call carries, as the Jellyfin
	// API bounds it: synced settings, or an app's log.
	moonfinBodyBytes = 1 << 20
)

func (s *Server) moonfinRoutes() {
	s.mux.HandleFunc("GET /api/v2/settings/plugins", s.signedIn(s.plugins))
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
		s.mux.HandleFunc(method+" /api/v2/plugin-content/plugins/{installation_id}/{path...}", s.signedIn(s.pluginContent))
	}
}

// pluginJSON is a plugin installed for the profile: its installation, its
// identifier and version, with no settings, routes or files of its own.
type pluginJSON struct {
	ID               string `json:"id"`
	PluginID         string `json:"plugin_id"`
	Version          string `json:"version"`
	UserConfigSchema []any  `json:"user_config_schema"`
	Routes           []any  `json:"routes"`
	Assets           []any  `json:"assets"`
}

// plugins lists Moonfin's companion while it is served to the user, and
// nothing otherwise.
func (s *Server) plugins(w http.ResponseWriter, _ *http.Request, v viewer) {
	items := []pluginJSON{}
	if s.options.Accounts.Settings().MoonfinFor(v.user) {
		items = append(items, pluginJSON{ID: moonfinInstallation, PluginID: moonfinPlugin, Version: s.options.Version,
			UserConfigSchema: []any{}, Routes: []any{}, Assets: []any{}})
	}
	writeJSON(w, http.StatusOK, struct {
		Items []pluginJSON `json:"items"`
	}{items})
}

// pluginContent answers a call of the companion's routes, as the Jellyfin
// API's handler answers it for the app's session, with its query and
// body. Another installation, or a path outside the companion's routes,
// is not found.
func (s *Server) pluginContent(w http.ResponseWriter, r *http.Request, v viewer) {
	path := r.PathValue("path")
	if r.PathValue("installation_id") != moonfinInstallation || !strings.HasPrefix(strings.ToLower(path), "moonfin/") {
		problem(w, http.StatusNotFound, "not_found", "This plugin route does not exist on this server.")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, moonfinBodyBytes))
	if maxBytes := new(http.MaxBytesError); errors.As(err, &maxBytes) {
		problem(w, http.StatusRequestEntityTooLarge, "payload_too_large", "The request body is too large.")
		return
	}
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid_request", "The request body could not be read.")
		return
	}
	if len(body) == 0 {
		body = nil
	}
	target := "/" + path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	request := jellyfinRequest(r, r.Method, target, authorization(v.device.DeviceInfo, v.token), body, "Accept-Language")
	if contentType := r.Header.Get("Content-Type"); body != nil && contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	s.options.Jellyfin.ServeHTTP(w, request)
}
