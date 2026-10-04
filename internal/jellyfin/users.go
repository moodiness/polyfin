package jellyfin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/throttle"
)

// maxBody bounds JSON request bodies.
const maxBody = 1 << 20

func decode(r *http.Request, into any) error {
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxBody)).Decode(into)
}

// publicUsers lists the users shown on the sign-in screen of Jellyfin apps.
// Anyone may read it: users' saved configurations are left out, each
// showing a new user's.
func (h *Handler) publicUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.Accounts.Users(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	visible := []UserDto{}
	for _, user := range users {
		if !user.IsHidden && !user.IsDisabled {
			visible = append(visible, newUserDto(user, h.ServerID))
		}
	}
	writeJSON(w, http.StatusOK, visible)
}

func (h *Handler) authenticateByName(w http.ResponseWriter, r *http.Request) {
	c := readCredentials(r, h.Accounts.Settings().LegacyAuthorization)
	var body struct {
		Username string
		Pw       string
	}
	if !c.complete() || decode(r, &body) != nil {
		processingError(w, http.StatusBadRequest)
		return
	}
	key := throttle.ClientKey(r)
	if allowed, wait := h.SignIns.Allowed(key); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		processingError(w, http.StatusTooManyRequests)
		return
	}
	user, err := h.Accounts.Authenticate(r.Context(), body.Username, body.Pw)
	switch {
	// An account blocked for wrong passwords answers as a wrong password
	// does, the right password included. Jellyfin, which disables such an
	// account instead, answers 403 to its right password.
	case errors.Is(err, accounts.ErrInvalidCredentials):
		h.SignIns.Fail(key)
		processingError(w, http.StatusUnauthorized)
		return
	case errors.Is(err, accounts.ErrDisabled):
		processingError(w, http.StatusForbidden)
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	h.SignIns.Succeed(key)
	h.signIn(w, r, user, c)
}

// signIn issues a token for the calling device and answers with Jellyfin's
// AuthenticationResult.
func (h *Handler) signIn(w http.ResponseWriter, r *http.Request, user accounts.User, c credentials) {
	token, device, err := h.Accounts.SignInDevice(r.Context(), user.ID, accounts.DeviceInfo{
		DeviceID:      c.DeviceID,
		DeviceName:    c.Device,
		Client:        c.Client,
		ClientVersion: c.Version,
		RemoteAddress: remoteAddress(r),
	})
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	user, err = h.Accounts.User(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	dto, err := h.userDto(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, AuthenticationResult{
		User:        dto,
		SessionInfo: newSessionInfo(device, user, h.ServerID, h.controllable(device)),
		AccessToken: token,
		ServerId:    h.ServerID,
	})
}

func (h *Handler) currentUser(w http.ResponseWriter, r *http.Request) {
	h.writeUser(w, r, callerFrom(r.Context()).User)
}

// users lists every user, optionally filtered by the isHidden and
// isDisabled query parameters.
func (h *Handler) users(w http.ResponseWriter, r *http.Request) {
	hidden, hiddenSet := boolQuery(r, "isHidden")
	disabled, disabledSet := boolQuery(r, "isDisabled")
	users, err := h.Accounts.Users(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := []UserDto{}
	for _, user := range users {
		if (hiddenSet && user.IsHidden != hidden) || (disabledSet && user.IsDisabled != disabled) {
			continue
		}
		dto, err := h.userDto(r.Context(), user)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		result = append(result, dto)
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) user(w http.ResponseWriter, r *http.Request) {
	id, err := accounts.ParseID(r.PathValue("userId"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, "User not found")
		return
	}
	user, err := h.Accounts.User(r.Context(), id)
	if errors.Is(err, accounts.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, "User not found")
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeUser(w, r, user)
}

// writeUser answers with user's DTO.
func (h *Handler) writeUser(w http.ResponseWriter, r *http.Request, user accounts.User) {
	dto, err := h.userDto(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

func boolQuery(r *http.Request, name string) (value, set bool) {
	raw := query(r, name)
	if raw == "" {
		return false, false
	}
	value, err := strconv.ParseBool(strings.ToLower(raw))
	return value, err == nil
}
