package admin

import (
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
)

// ownSound answers the caller's sound options (see accounts.User
// BoostVolume and ConvertSound), and whether their group holds them.
func (h *handler) ownSound(w http.ResponseWriter, r *http.Request) {
	user, err := h.Accounts.User(r.Context(), sessionFrom(r.Context()).User.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeSound(w, user)
}

// setOwnSound changes the caller's sound options, those the body names.
// A member who follows their group keeps its values: a change is refused
// with follows_group.
func (h *handler) setOwnSound(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BoostVolume  *bool `json:"boostVolume"`
		ConvertSound *bool `json:"convertSound"`
	}
	if !decode(w, r, &body) {
		return
	}
	session := sessionFrom(r.Context())
	user, err := h.Accounts.UpdateUser(r.Context(), session.User.ID,
		accounts.UserChanges{BoostVolume: body.BoostVolume, ConvertSound: body.ConvertSound}, session.TokenHash)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeSound(w, user)
}

func writeSound(w http.ResponseWriter, user accounts.User) {
	writeJSON(w, http.StatusOK, map[string]any{
		"boostVolume":  user.BoostVolume,
		"convertSound": user.ConvertSound,
		"followsGroup": user.GroupID != nil && user.FollowGroup,
	})
}
