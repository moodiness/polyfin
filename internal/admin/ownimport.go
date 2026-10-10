package admin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/moodiness/polyfin/internal/jellyfinimport"
)

// Under My account, a user imports their own watch history from another
// Jellyfin or Emby server, signing in there as themselves. The password is
// used for that import only: never stored, never answered back.

// ownImport answers whether the user may import their own watch history,
// and their own import running, else their last one, null when none.
// Another user's imports never show.
func (h *handler) ownImport(w http.ResponseWriter, r *http.Request) {
	enabled := h.JellyfinImport != nil && h.Accounts.Settings().ServerImports
	var current *jellyfinImportJSON
	if enabled {
		current = newJellyfinImportJSON(h.JellyfinImport.Own(sessionFrom(r.Context()).User.ID))
	}
	writeJSON(w, http.StatusOK, struct {
		Enabled bool                `json:"enabled"`
		Import  *jellyfinImportJSON `json:"import"`
	}{enabled, current})
}

// startOwnImport signs in to the server as the name and password given, and
// imports that user's watch data into the caller's account, in the
// background. One import runs at a time on the whole server: while any
// runs, it answers 409 server_import_running.
func (h *handler) startOwnImport(w http.ResponseWriter, r *http.Request) {
	if h.JellyfinImport == nil || !h.Accounts.Settings().ServerImports {
		writeError(w, http.StatusForbidden, "server_imports_disabled")
		return
	}
	var body struct {
		Kind     string `json:"kind"`
		Address  string `json:"address"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	kind, ok := importKind(body.Kind)
	name := strings.TrimSpace(body.Name)
	if !ok || kind == jellyfinimport.Plex || name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	connection := jellyfinimport.Connection{Kind: kind, Address: body.Address,
		Credentials: jellyfinimport.Credentials{Name: name, Password: body.Password}}
	status, err := h.JellyfinImport.ImportOwn(r.Context(), connection, sessionFrom(r.Context()).User)
	if errors.Is(err, jellyfinimport.ErrRunning) {
		// Another user's, or the administrator's: the caller waits for it.
		writeError(w, http.StatusConflict, "server_import_running")
		return
	}
	if jellyfinError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Import *jellyfinImportJSON `json:"import"`
	}{newJellyfinImportJSON(status)})
}
