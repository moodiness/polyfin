package jellyfin

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/moonfin"
)

// The sign-in PINs of Moonfin's companion (see moonfin.Store.CheckPIN):
// the PIN that locks a user's profile on the user picker of Moonfin's
// apps, kept for all their devices. An app asks whether the user has
// one, checks what is typed, and sets, changes or removes it with the
// current one; an administrator sets or removes a user's without it.
// Moonfin's apps tell the answers apart by their status and their error
// message, which are as Moonbase gives them. No answer holds a PIN or its
// hash.

// moonfinPinBodyBytes bounds the body of a PIN's request: two PINs.
const moonfinPinBodyBytes = 4 << 10

// The error messages Moonfin's apps read.
const (
	moonfinNoPin       = "No PIN is set"
	moonfinWrongPin    = "The current PIN is wrong"
	moonfinInvalidPin  = "A PIN is exactly four digits"
	moonfinTooManyPins = "Too many attempts"
)

// moonfinPinRoutes adds the routes of the sign-in PINs: route adds a
// user's, admin an administrator's.
func (h *Handler) moonfinPinRoutes(route, admin func(method, pattern string, handler http.HandlerFunc)) {
	route(http.MethodGet, "/Moonfin/Pin", h.moonfinPin)
	route(http.MethodPut, "/Moonfin/Pin", h.setMoonfinPin)
	route(http.MethodDelete, "/Moonfin/Pin", h.removeMoonfinPin)
	route(http.MethodPost, "/Moonfin/Pin/Verify", h.verifyMoonfinPin)
	admin(http.MethodPut, "/Moonfin/Users/{userId}/Pin", h.setUserMoonfinPin)
	admin(http.MethodDelete, "/Moonfin/Users/{userId}/Pin", h.removeUserMoonfinPin)
}

// moonfinPinBody reads the PIN and the current PIN of a request's body,
// whose names are matched in any case, as the companion's model binding
// matches them. An empty body names neither. ok is false once it
// answered the request.
func moonfinPinBody(w http.ResponseWriter, r *http.Request) (pin, current string, ok bool) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, moonfinPinBodyBytes))
	var body map[string]any
	if err != nil || len(bytes.TrimSpace(data)) > 0 && (json.Unmarshal(data, &body) != nil || body == nil) {
		moonfinError(w, http.StatusBadRequest, "The body must be a JSON object.")
		return "", "", false
	}
	for name, value := range body {
		text, _ := value.(string)
		switch strings.ToLower(name) {
		case "pin":
			pin = text
		case "currentpin":
			current = text
		}
	}
	return pin, current, true
}

// moonfinPinRefused answers the refusal of a PIN's request, and reports
// whether err was one.
func moonfinPinRefused(w http.ResponseWriter, err error) bool {
	var wait *moonfin.PINWaitError
	switch {
	case errors.As(err, &wait):
		w.Header().Set("Retry-After", strconv.Itoa(wait.Seconds()))
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": moonfinTooManyPins, "retryAfterSeconds": wait.Seconds()})
	case errors.Is(err, moonfin.ErrWrongPIN):
		moonfinError(w, http.StatusForbidden, moonfinWrongPin)
	case errors.Is(err, moonfin.ErrInvalidPIN):
		moonfinError(w, http.StatusBadRequest, moonfinInvalidPin)
	case errors.Is(err, moonfin.ErrNoPIN):
		moonfinError(w, http.StatusNotFound, moonfinNoPin)
	default:
		return false
	}
	return true
}

// writeMoonfinPinState answers whether the user has a PIN now, or the
// error of the change.
func (h *Handler) writeMoonfinPinState(w http.ResponseWriter, r *http.Request, enabled bool, err error) {
	if moonfinPinRefused(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": enabled})
}

// moonfinPin answers whether the caller has a PIN.
func (h *Handler) moonfinPin(w http.ResponseWriter, r *http.Request) {
	state, err := h.Moonfin.PIN(r.Context(), callerFrom(r.Context()).User.ID)
	h.writeMoonfinPinState(w, r, state.Set, err)
}

// setMoonfinPin gives the caller the PIN of the body, with the current
// one when they have one.
func (h *Handler) setMoonfinPin(w http.ResponseWriter, r *http.Request) {
	pin, current, ok := moonfinPinBody(w, r)
	if !ok {
		return
	}
	err := h.Moonfin.ChangePIN(r.Context(), callerFrom(r.Context()).User.ID, pin, current)
	h.writeMoonfinPinState(w, r, true, err)
}

// removeMoonfinPin takes the caller's PIN away, with the current one,
// which an app may also send as the PIN.
func (h *Handler) removeMoonfinPin(w http.ResponseWriter, r *http.Request) {
	pin, current, ok := moonfinPinBody(w, r)
	if !ok {
		return
	}
	if current == "" {
		current = pin
	}
	err := h.Moonfin.RemoveOwnPIN(r.Context(), callerFrom(r.Context()).User.ID, current)
	h.writeMoonfinPinState(w, r, false, err)
}

// verifyMoonfinPin checks the PIN of the body against the caller's.
func (h *Handler) verifyMoonfinPin(w http.ResponseWriter, r *http.Request) {
	pin, _, ok := moonfinPinBody(w, r)
	if !ok {
		return
	}
	err := h.Moonfin.CheckPIN(r.Context(), callerFrom(r.Context()).User.ID, pin)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"valid": true})
	case errors.Is(err, moonfin.ErrWrongPIN):
		writeJSON(w, http.StatusOK, map[string]any{"valid": false})
	case !moonfinPinRefused(w, err):
		h.internalError(w, r, err)
	}
}

// setUserMoonfinPin gives the user the path names the PIN of the body,
// without the current one, and starts their wrong guesses again, for an
// administrator.
func (h *Handler) setUserMoonfinPin(w http.ResponseWriter, r *http.Request) {
	user, ok := h.moonfinUser(w, r)
	if !ok {
		return
	}
	pin, _, ok := moonfinPinBody(w, r)
	if !ok {
		return
	}
	h.writeMoonfinPinState(w, r, true, h.Moonfin.SetPIN(r.Context(), user, pin))
}

// removeUserMoonfinPin takes the PIN of the user the path names away, for
// an administrator.
func (h *Handler) removeUserMoonfinPin(w http.ResponseWriter, r *http.Request) {
	if user, ok := h.moonfinUser(w, r); ok {
		h.writeMoonfinPinState(w, r, false, h.Moonfin.RemovePIN(r.Context(), user))
	}
}
