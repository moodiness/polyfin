package admin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/moodiness/polyfin/internal/jellyfinimport"
)

// Under My account, a user imports their own watch history from another
// Jellyfin or Emby server, signing in there as themselves, or from their
// Letterboxd export file (see letterboxd.go). The password is used for that
// import only: never stored, never answered back.

// ownImport answers whether the user may import their own watch history,
// whether they may import a Letterboxd export and the server's metadata
// addons search movies, without which no movie of it is found, and their
// latest import, from a server or Letterboxd, running or ended, null when
// none. Another user's imports never show.
func (h *handler) ownImport(w http.ResponseWriter, r *http.Request) {
	user := sessionFrom(r.Context()).User
	enabled := (h.JellyfinImport != nil || h.Letterboxd != nil) && h.Accounts.Settings().ServerImports
	var current any
	searchable := false
	if enabled {
		var fromServer *jellyfinimport.Status
		if h.JellyfinImport != nil {
			fromServer = h.JellyfinImport.Own(user.ID)
		}
		if fromServer != nil {
			current = newJellyfinImportJSON(fromServer)
		}
		if h.Letterboxd != nil {
			searchable = h.letterboxdSearchable(r)
			if own := h.Letterboxd.Current(user.ID); own != nil && (fromServer == nil || own.StartedAt.After(fromServer.StartedAt)) {
				current = newLetterboxdImportJSON(own)
			}
		}
	}
	// Collections tells whether the user's own import may bring the
	// server's collections: only a user who may manage collections does.
	writeJSON(w, http.StatusOK, struct {
		Enabled     bool `json:"enabled"`
		Letterboxd  bool `json:"letterboxd"`
		Searchable  bool `json:"searchable"`
		Collections bool `json:"collections"`
		Import      any  `json:"import"`
	}{enabled, enabled && h.Letterboxd != nil, searchable, user.CollectionManagement, current})
}

// startOwnImport imports into the caller's account, in the background,
// the watch data of a Letterboxd export, which a multipart form sends
// (see startLetterboxd), or that of the user signing in to a Jellyfin or
// Emby server with the name and password given. One import from a server
// runs at a time on the whole server, and one import into an account:
// while either runs, it answers 409 server_import_running.
func (h *handler) startOwnImport(w http.ResponseWriter, r *http.Request) {
	if !h.Accounts.Settings().ServerImports {
		writeError(w, http.StatusForbidden, "server_imports_disabled")
		return
	}
	user := sessionFrom(r.Context()).User
	if multipartForm(r) {
		if h.Letterboxd == nil {
			writeError(w, http.StatusForbidden, "server_imports_disabled")
			return
		}
		h.startLetterboxd(w, r, user, user)
		return
	}
	if h.JellyfinImport == nil {
		writeError(w, http.StatusForbidden, "server_imports_disabled")
		return
	}
	var body struct {
		Kind     string `json:"kind"`
		Address  string `json:"address"`
		Name     string `json:"name"`
		Password string `json:"password"`
		// Server and Watchlist are Plex's: the server chosen among those
		// the caller's sign-in to Plex reaches, and whether its Watchlist
		// comes too, by default.
		Server    string `json:"server"`
		Watchlist *bool  `json:"watchlist"`
		extrasJSON
	}
	if !decode(w, r, &body) {
		return
	}
	kind, ok := importKind(body.Kind)
	name := strings.TrimSpace(body.Name)
	if !ok || kind != jellyfinimport.Plex && name == "" || kind == jellyfinimport.Plex && body.Server == "" {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if h.Letterboxd != nil && h.Letterboxd.Running(user.ID) {
		writeError(w, http.StatusConflict, "server_import_running")
		return
	}
	var status *jellyfinimport.Status
	var err error
	extras := body.extras()
	if kind == jellyfinimport.Plex {
		extras.Watchlist = body.Watchlist == nil || *body.Watchlist
		status, err = h.JellyfinImport.ImportOwnPlex(r.Context(), user, body.Server, extras)
		if plexSignInError(w, err) {
			return
		}
	} else {
		connection := jellyfinimport.Connection{Kind: kind, Address: body.Address,
			Credentials: jellyfinimport.Credentials{Name: name, Password: body.Password}, Extras: extras}
		status, err = h.JellyfinImport.ImportOwn(r.Context(), connection, user)
	}
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
