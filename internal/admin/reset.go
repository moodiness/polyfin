package admin

import (
	"context"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
)

// PreferencesReset forgets what a user's Jellyfin apps saved in their
// account, the display preferences and the configuration, with what the
// Jellyfin API remembers of them (see jellyfin.Handler.ResetPreferences).
type PreferencesReset interface {
	ResetPreferences(ctx context.Context, user accounts.ID) error
}

// userDefaults answers the settings Reset to defaults would give a user,
// as limitsJSON, for the admin app to list what changes before asking.
func (h *handler) userDefaults(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	limits, err := h.Accounts.LimitsAfterReset(r.Context(), id)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newLimitsJSON(limits))
}

// resetUser puts the settings a group holds of a user back to a new
// user's, and answers the user as they are now.
func (h *handler) resetUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	user, err := h.Accounts.ResetLimits(r.Context(), id)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.Activity.LimitsReset(r.Context(), user, sessionFrom(r.Context()).User.Name)
	h.writeUser(w, r, http.StatusOK, user)
}

// groupDefaults answers the settings Reset to defaults would give a group.
func (h *handler) groupDefaults(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	limits, err := h.Accounts.GroupLimitsAfterReset(r.Context(), id)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newLimitsJSON(limits))
}

// resetGroup puts the settings of a group back to a new user's, which
// reach the members who follow it at once, and answers the group.
func (h *handler) resetGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	group, err := h.Accounts.ResetGroup(r.Context(), id)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.Activity.GroupReset(r.Context(), group, sessionFrom(r.Context()).User.Name)
	writeJSON(w, http.StatusOK, newGroupJSON(group))
}

// resetOwnPreferences puts back the server's defaults for what the caller
// chose themselves: the preferences and the configuration Jellyfin apps
// saved in their account, the home rows they turned off, and their
// metadata language. What they watched, their favorites and resume points,
// their connections and imports stay. Each part can be reset again alone,
// so a failure part way leaves nothing that a second try does not finish.
func (h *handler) resetOwnPreferences(w http.ResponseWriter, r *http.Request) {
	session := sessionFrom(r.Context())
	if h.Preferences != nil {
		if err := h.Preferences.ResetPreferences(r.Context(), session.User.ID); err != nil {
			h.internalError(w, r, err)
			return
		}
	}
	if h.HomeRows != nil {
		if err := h.HomeRows.TurnAllOn(r.Context(), session.User.ID); err != nil {
			h.internalError(w, r, err)
			return
		}
	}
	_, err := h.Accounts.UpdateUser(r.Context(), session.User.ID, accounts.UserChanges{MetadataLanguage: new("")}, session.TokenHash)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
