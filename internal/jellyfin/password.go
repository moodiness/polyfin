package jellyfin

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/throttle"
)

// The answers Jellyfin gives, as JSON strings, when a password change is
// refused.
const (
	wrongCurrentPassword = "Invalid user or password entered."
	passwordNotAllowed   = "User is not allowed to update the password."
)

// changePassword sets a user's password from a Jellyfin app. Users change
// their own by giving the current one; administrators set another user's
// without it. A wrong current password counts as a failed sign-in, so this
// endpoint guesses passwords no faster than signing in does.
func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	if !jsonContent(r.Header.Get("Content-Type")) {
		unsupportedMediaTypeProblem(w)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		processingError(w, http.StatusRequestEntityTooLarge)
		return
	}
	// Like Jellyfin, the request is bound whole before it is authorized:
	// identifier and body problems are reported together.
	errs := bindErrors{}
	id, set := errs.userID(r)
	var body struct {
		CurrentPw     string
		NewPw         string
		ResetPassword bool
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		errs.add("", "A non-empty request body is required.")
		errs.add("request", "The request field is required.")
	} else if json.Unmarshal(raw, &body) != nil {
		errs.add("$", "The JSON value could not be converted.")
		errs.add("request", "The request field is required.")
	}
	if len(errs) > 0 {
		validationProblem(w, errs)
		return
	}

	caller := callerFrom(r.Context())
	if !set {
		id = caller.User.ID
	}
	if id != caller.User.ID {
		if !caller.User.IsAdministrator {
			writeJSON(w, http.StatusForbidden, passwordNotAllowed)
			return
		}
		if _, err := h.Accounts.User(r.Context(), id); errors.Is(err, accounts.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, "User not found")
			return
		} else if err != nil {
			h.internalError(w, r, err)
			return
		}
	}
	// Jellyfin would leave the account without a password; Polyfin
	// accounts always have one, so the request is invalid here.
	if body.ResetPassword {
		validationProblem(w, map[string][]string{"ResetPassword": {"Polyfin accounts always have a password; set a new one instead."}})
		return
	}

	if id != caller.User.ID {
		// The administrator's own devices belong to another account: every
		// device of the user is signed out.
		_, err := h.Accounts.UpdateUser(r.Context(), id, accounts.UserChanges{Password: &body.NewPw}, nil)
		h.passwordChanged(w, r, err)
		return
	}
	// Unlike Jellyfin, an administrator changing their own password must
	// also give the current one, so that a token left on a shared device
	// cannot take the account over.
	key := throttle.ClientKey(r)
	if allowed, wait := h.SignIns.Allowed(key); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		processingError(w, http.StatusTooManyRequests)
		return
	}
	err = h.Accounts.ChangePasswordFromDevice(r.Context(), id, body.CurrentPw, body.NewPw, caller.Device.ID)
	if errors.Is(err, accounts.ErrWrongPassword) {
		h.SignIns.Fail(key)
		writeJSON(w, http.StatusForbidden, wrongCurrentPassword)
		return
	}
	// The current password was right, even when the new one is refused.
	h.SignIns.Succeed(key)
	h.passwordChanged(w, r, err)
}

// passwordChanged answers a password change that passed authorization.
func (h *Handler) passwordChanged(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, accounts.ErrInvalidPassword):
		validationProblem(w, map[string][]string{"NewPw": {"The new password must have at least 8 characters and at most 256 bytes."}})
	case errors.Is(err, accounts.ErrNotFound):
		writeJSON(w, http.StatusNotFound, "User not found")
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
