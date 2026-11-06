package admin

import (
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
)

// setOwnInterfaceSize changes the size the admin app shows in for the
// caller, one of accounts.InterfaceSizes (see accounts.User.InterfaceSize).
// The session's answer carries it, so it has no reading of its own.
func (h *handler) setOwnInterfaceSize(w http.ResponseWriter, r *http.Request) {
	var body struct {
		InterfaceSize *string `json:"interfaceSize"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.InterfaceSize == nil {
		writeError(w, http.StatusBadRequest, "invalid_interface_size")
		return
	}
	session := sessionFrom(r.Context())
	user, err := h.Accounts.UpdateUser(r.Context(), session.User.ID, accounts.UserChanges{InterfaceSize: body.InterfaceSize}, session.TokenHash)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"interfaceSize": user.InterfaceSize})
}
