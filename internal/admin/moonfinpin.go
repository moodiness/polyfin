package admin

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/moonfin"
)

// The PIN that locks a user's profile on the user picker of Moonfin's
// apps (see moonfin.Store.CheckPIN): on a user's page, an administrator
// sets one, which is how a forgotten PIN is reset, or removes it, without
// the current one; on My account, the user sets, changes or removes their
// own with the current one, a wrong one counting as a wrong guess. No
// answer holds a PIN or its hash.

// moonfinPinJSON tells whether a user has a PIN, the wrong guesses given
// since the last right one, and, while the next guess waits, until when.
type moonfinPinJSON struct {
	Set          bool       `json:"set"`
	WrongGuesses int        `json:"wrongGuesses"`
	WaitUntil    *time.Time `json:"waitUntil"`
}

// writeMoonfinPin answers the state of the PIN of user.
func (h *handler) writeMoonfinPin(w http.ResponseWriter, r *http.Request, user accounts.ID) {
	state, err := h.Moonfin.PIN(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, moonfinPinJSON{Set: state.Set, WrongGuesses: state.WrongGuesses, WaitUntil: state.WaitUntil})
}

// moonfinPinError answers the refusals of the store's PINs, and reports
// whether err was one of them.
func moonfinPinError(w http.ResponseWriter, err error) bool {
	var wait *moonfin.PINWaitError
	switch {
	case errors.As(err, &wait):
		w.Header().Set("Retry-After", strconv.Itoa(wait.Seconds()))
		writeError(w, http.StatusTooManyRequests, "moonfin_pin_wait")
	case errors.Is(err, moonfin.ErrWrongPIN):
		writeError(w, http.StatusForbidden, "wrong_moonfin_pin")
	case errors.Is(err, moonfin.ErrInvalidPIN):
		writeError(w, http.StatusBadRequest, "invalid_moonfin_pin")
	default:
		return false
	}
	return true
}

// moonfinPinBody is the body of a PIN's change: the new PIN, and the
// current one, which the user gives for their own.
type moonfinPinBody struct {
	Pin        string `json:"pin"`
	CurrentPin string `json:"currentPin"`
}

// moonfinPinUser is the user the path names, for an administrator, while
// Polyfin serves Moonfin's companion. ok is false once it answered.
func (h *handler) moonfinPinUser(w http.ResponseWriter, r *http.Request) (accounts.ID, bool) {
	id, ok := pathID(w, r, "id")
	if !ok || !h.moonfinOffered(w) {
		return id, false
	}
	if _, err := h.Accounts.User(r.Context(), id); errors.Is(err, accounts.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found")
		return id, false
	} else if err != nil {
		h.internalError(w, r, err)
		return id, false
	}
	return id, true
}

// userMoonfinPin answers the state of the PIN of the user the path names.
func (h *handler) userMoonfinPin(w http.ResponseWriter, r *http.Request) {
	if id, ok := h.moonfinPinUser(w, r); ok {
		h.writeMoonfinPin(w, r, id)
	}
}

// setUserMoonfinPin gives the user the path names the PIN of the body, and
// starts their wrong guesses again.
func (h *handler) setUserMoonfinPin(w http.ResponseWriter, r *http.Request) {
	id, ok := h.moonfinPinUser(w, r)
	if !ok {
		return
	}
	var body moonfinPinBody
	if !decode(w, r, &body) {
		return
	}
	h.changedMoonfinPin(w, r, id, h.Moonfin.SetPIN(r.Context(), id, body.Pin))
}

// removeUserMoonfinPin takes the PIN of the user the path names away.
func (h *handler) removeUserMoonfinPin(w http.ResponseWriter, r *http.Request) {
	if id, ok := h.moonfinPinUser(w, r); ok {
		h.changedMoonfinPin(w, r, id, h.Moonfin.RemovePIN(r.Context(), id))
	}
}

// ownMoonfinPin answers the state of the caller's PIN.
func (h *handler) ownMoonfinPin(w http.ResponseWriter, r *http.Request) {
	if h.moonfinOffered(w) {
		h.writeMoonfinPin(w, r, sessionFrom(r.Context()).User.ID)
	}
}

// setOwnMoonfinPin gives the caller the PIN of the body, with the current
// one when they have one.
func (h *handler) setOwnMoonfinPin(w http.ResponseWriter, r *http.Request) {
	if !h.moonfinOffered(w) {
		return
	}
	var body moonfinPinBody
	if !decode(w, r, &body) {
		return
	}
	user := sessionFrom(r.Context()).User.ID
	h.changedMoonfinPin(w, r, user, h.Moonfin.ChangePIN(r.Context(), user, body.Pin, body.CurrentPin))
}

// removeOwnMoonfinPin takes the caller's PIN away, with the current one.
func (h *handler) removeOwnMoonfinPin(w http.ResponseWriter, r *http.Request) {
	if !h.moonfinOffered(w) {
		return
	}
	var body moonfinPinBody
	if !decode(w, r, &body) {
		return
	}
	user := sessionFrom(r.Context()).User.ID
	h.changedMoonfinPin(w, r, user, h.Moonfin.RemoveOwnPIN(r.Context(), user, body.CurrentPin))
}

// changedMoonfinPin answers the state of the PIN of user once a change
// ended with err.
func (h *handler) changedMoonfinPin(w http.ResponseWriter, r *http.Request, user accounts.ID, err error) {
	if moonfinPinError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeMoonfinPin(w, r, user)
}
