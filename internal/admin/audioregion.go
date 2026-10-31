package admin

import (
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/localization"
)

// ownAudioRegion answers the caller's preferred audio region, with the
// regions they may choose (see accounts.User.AudioRegion).
func (h *handler) ownAudioRegion(w http.ResponseWriter, r *http.Request) {
	user, err := h.Accounts.User(r.Context(), sessionFrom(r.Context()).User.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeAudioRegion(w, user)
}

// setOwnAudioRegion changes the caller's preferred audio region: one of
// localization.AudioVariants, or empty for none.
func (h *handler) setOwnAudioRegion(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Region *string `json:"region"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Region == nil {
		writeError(w, http.StatusBadRequest, "invalid_audio_region")
		return
	}
	session := sessionFrom(r.Context())
	user, err := h.Accounts.UpdateUser(r.Context(), session.User.ID, accounts.UserChanges{AudioRegion: body.Region}, session.TokenHash)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeAudioRegion(w, user)
}

func writeAudioRegion(w http.ResponseWriter, user accounts.User) {
	writeJSON(w, http.StatusOK, map[string]any{"region": user.AudioRegion, "regions": localization.AudioVariants})
}
