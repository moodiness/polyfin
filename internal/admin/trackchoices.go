package admin

import (
	"context"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
)

// TrackChoices counts and forgets the audio and subtitle tracks a user
// chose on titles, which their next plays of a title show by default (see
// jellyfin.Handler.CountTrackChoices).
type TrackChoices interface {
	CountTrackChoices(ctx context.Context, user accounts.ID) (int, error)
	ForgetTrackChoices(ctx context.Context, user accounts.ID) error
}

// trackChoicesJSON tells how many titles the caller chose tracks on.
type trackChoicesJSON struct {
	Titles int `json:"titles"`
}

// ownTrackChoices answers how many titles the caller chose tracks on.
func (h *handler) ownTrackChoices(w http.ResponseWriter, r *http.Request) {
	answer := trackChoicesJSON{}
	if h.TrackChoices != nil {
		titles, err := h.TrackChoices.CountTrackChoices(r.Context(), sessionFrom(r.Context()).User.ID)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		answer.Titles = titles
	}
	writeJSON(w, http.StatusOK, answer)
}

// forgetOwnTrackChoices forgets the tracks the caller chose on every
// title: their next plays follow their language preferences again.
func (h *handler) forgetOwnTrackChoices(w http.ResponseWriter, r *http.Request) {
	if h.TrackChoices != nil {
		if err := h.TrackChoices.ForgetTrackChoices(r.Context(), sessionFrom(r.Context()).User.ID); err != nil {
			h.internalError(w, r, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
