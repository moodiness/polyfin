package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/jellyfinimport"
)

// Under My account, a user signs in to Plex, on Plex's own sign-in page,
// to import their own watch data from a Plex Media Server their account
// reaches (see jellyfinimport.Service.PlexSignIn). The import service
// holds the sign-in for the caller only, and the tokens Plex gives are
// never answered.

// plexSignInAllowed tells whether the caller may sign in to Plex to import
// their own watch data, answering 403 server_imports_disabled otherwise.
func (h *handler) plexSignInAllowed(w http.ResponseWriter) bool {
	if h.JellyfinImport == nil || !h.Accounts.Settings().ServerImports {
		writeError(w, http.StatusForbidden, "server_imports_disabled")
		return false
	}
	return true
}

// startPlexSignIn starts the caller's sign-in to Plex, dropping the one
// they held before, and answers the address of Plex's sign-in page to
// open, and when it expires.
func (h *handler) startPlexSignIn(w http.ResponseWriter, r *http.Request) {
	if !h.plexSignInAllowed(w) {
		return
	}
	link, err := h.JellyfinImport.PlexSignIn(r.Context(), sessionFrom(r.Context()).User.ID)
	if plexSignInError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Link      string    `json:"link"`
		ExpiresAt time.Time `json:"expiresAt"`
	}{link.URL, link.Expires.UTC().Truncate(time.Second)})
}

// pollPlexSignIn answers 202 and pending until Plex signs the caller in,
// then the account, its Plex Home's profiles and the servers it reaches.
func (h *handler) pollPlexSignIn(w http.ResponseWriter, r *http.Request) {
	if !h.plexSignInAllowed(w) {
		return
	}
	account, err := h.JellyfinImport.PlexAccount(r.Context(), sessionFrom(r.Context()).User.ID)
	h.answerPlexAccount(w, r, account, err)
}

// choosePlexProfile switches the caller's sign-in to a profile of their
// Plex Home, with its PIN when it has one, and answers the account with
// the servers that profile reaches.
func (h *handler) choosePlexProfile(w http.ResponseWriter, r *http.Request) {
	if !h.plexSignInAllowed(w) {
		return
	}
	var body struct {
		Profile string `json:"profile"`
		PIN     string `json:"pin"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Profile == "" || len(body.PIN) > 16 {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	account, err := h.JellyfinImport.PlexProfile(r.Context(), sessionFrom(r.Context()).User.ID, body.Profile, body.PIN)
	h.answerPlexAccount(w, r, account, err)
}

// dropPlexSignIn drops the caller's sign-in to Plex, signing it out.
func (h *handler) dropPlexSignIn(w http.ResponseWriter, r *http.Request) {
	if !h.plexSignInAllowed(w) {
		return
	}
	h.JellyfinImport.DropPlexSignIn(sessionFrom(r.Context()).User.ID)
	w.WriteHeader(http.StatusNoContent)
}

// answerPlexAccount answers the account a sign-in to Plex read, or what
// failed.
func (h *handler) answerPlexAccount(w http.ResponseWriter, r *http.Request, account jellyfinimport.PlexAccount, err error) {
	if errors.Is(err, jellyfinimport.ErrPlexSignInPending) {
		writeJSON(w, http.StatusAccepted, struct {
			Pending bool `json:"pending"`
		}{true})
		return
	}
	if plexSignInError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newPlexAccountJSON(account))
}

// plexAccountJSON is a Plex account signed in: its name, the profiles of
// its Plex Home, empty outside one, the profile read, and the servers it
// reaches, by machine identifier.
type plexAccountJSON struct {
	Name     string            `json:"name"`
	Profiles []plexProfileJSON `json:"profiles"`
	Profile  string            `json:"profile"`
	Servers  []plexServerJSON  `json:"servers"`
}

type plexProfileJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Managed   bool   `json:"managed"`
	Protected bool   `json:"protected"`
}

type plexServerJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Owned       bool   `json:"owned"`
	Preselected bool   `json:"preselected"`
}

func newPlexAccountJSON(account jellyfinimport.PlexAccount) plexAccountJSON {
	result := plexAccountJSON{Name: account.Name, Profile: account.Profile,
		Profiles: make([]plexProfileJSON, 0, len(account.Profiles)), Servers: make([]plexServerJSON, 0, len(account.Servers))}
	for _, p := range account.Profiles {
		result.Profiles = append(result.Profiles, plexProfileJSON{ID: p.ID, Name: p.Name, Managed: p.Managed, Protected: p.Protected})
	}
	for _, s := range account.Servers {
		result.Servers = append(result.Servers, plexServerJSON{ID: s.ID, Name: s.Name, Owned: s.Owned, Preselected: s.Preselected})
	}
	return result
}

// plexSignInError answers the ways signing in to Plex, and importing from
// the server chosen, fail, and reports whether err was one.
func plexSignInError(w http.ResponseWriter, err error) bool {
	for _, known := range []struct {
		err    error
		status int
		code   string
	}{
		{jellyfinimport.ErrPlexSignInEnded, http.StatusConflict, "plex_sign_in_ended"},
		{jellyfinimport.ErrPlexSignInPending, http.StatusConflict, "plex_sign_in_pending"},
		{jellyfinimport.ErrNotSignedIn, http.StatusConflict, "plex_sign_in_pending"},
		{jellyfinimport.ErrPlexUnknownProfile, http.StatusBadRequest, "plex_unknown_profile"},
		{jellyfinimport.ErrPlexProfilePIN, http.StatusBadRequest, "plex_profile_pin_refused"},
		{jellyfinimport.ErrForbidden, http.StatusBadRequest, "plex_profile_forbidden"},
		{jellyfinimport.ErrPlexUnknownServer, http.StatusBadRequest, "plex_unknown_server"},
		{jellyfinimport.ErrPlexServerUnreachable, http.StatusBadGateway, "plex_server_unreachable"},
		{jellyfinimport.ErrPlexTV, http.StatusBadGateway, "plex_tv_failed"},
		{jellyfinimport.ErrUnreachable, http.StatusBadGateway, "plex_tv_unreachable"},
	} {
		if errors.Is(err, known.err) {
			writeError(w, known.status, known.code)
			return true
		}
	}
	return false
}
