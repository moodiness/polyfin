package jellyfin

import (
	jsonv2 "encoding/json/v2"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/forwarded"
	"github.com/moodiness/polyfin/internal/throttle"
)

// maxBody bounds JSON request bodies.
const maxBody = 1 << 20

// decode reads a JSON request body as Jellyfin does (see bodyOptions).
func decode(r *http.Request, into any) error {
	return jsonv2.UnmarshalRead(http.MaxBytesReader(nil, r.Body, maxBody), into, bodyOptions)
}

// publicUsers lists the users shown on the sign-in screen of Jellyfin apps.
// Anyone may read it: it tells of each what otherUserDto does, their name
// and picture, no more. Like Jellyfin's, from outside the local network it
// leaves out those who may not sign in from there (UserController.cs,
// lines 626 to 632).
func (h *Handler) publicUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.Accounts.Users(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	visible := []UserDto{}
	local := forwarded.InLocalNetwork(r)
	for _, user := range users {
		if user.IsHidden || user.Refused(h.now()) || !user.AllowedFrom(local) {
			continue
		}
		visible = append(visible, otherUserDto(user, h.ServerID))
	}
	writeJSON(w, http.StatusOK, visible)
}

// reserve counts an attempt of the client toward failures before it is
// checked (see throttle.Failures.Reserve), or answers 429 when the client
// exhausted its failed attempts, and then returns nil.
func (h *Handler) reserve(w http.ResponseWriter, failures *throttle.Failures, key string) *throttle.Attempt {
	attempt, wait := failures.Reserve(key)
	if attempt == nil {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		processingError(w, http.StatusTooManyRequests)
	}
	return attempt
}

// signInFailures is the count the client's failed sign-ins and reset PINs
// of r go to. A page of another site can have the visitor's browser send
// them, as the API answers any origin for the apps served elsewhere: what
// the browser tells came from another site counts in a count of its own,
// so that such a page never refuses the visitor's own sign-ins, in apps
// and in the admin app. The browser sets the header; a page cannot. The
// block of an account after wrong passwords counts them all the same.
func (h *Handler) signInFailures(r *http.Request) *throttle.Failures {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return h.CrossSiteSignIns
	}
	return h.SignIns
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
	key := forwarded.Client(r)
	attempt := h.reserve(w, h.signInFailures(r), key)
	if attempt == nil {
		return
	}
	defer attempt.Cancel()
	user, err := h.Accounts.AuthenticateFrom(r.Context(), body.Username, body.Pw, accounts.DeviceInfo{DeviceID: c.DeviceID, RemoteAddress: key})
	var blocked *accounts.BlockError
	if errors.As(err, &blocked) && h.AccountBlocked != nil {
		h.AccountBlocked(blocked.User, accounts.DeviceInfo{DeviceID: c.DeviceID, DeviceName: c.Device, Client: c.Client,
			ClientVersion: c.Version, RemoteAddress: key})
	}
	switch {
	// A client blocked for wrong passwords answers as a wrong password
	// does, the right password included. Jellyfin, which disables such an
	// account instead, answers 403 to its right password.
	case errors.Is(err, accounts.ErrInvalidCredentials):
		attempt.Fail()
		h.Activity.SignInFailed(r.Context(), body.Username, forwarded.Client(r))
		processingError(w, http.StatusUnauthorized)
		return
	case errors.Is(err, accounts.ErrDisabled):
		h.Activity.SignInFailed(r.Context(), body.Username, forwarded.Client(r))
		processingError(w, http.StatusForbidden)
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	// An account with two-step sign-in, or an administrator while it is
	// required of them, signs in to apps through Quick Connect, approved
	// in the admin app: Jellyfin apps cannot ask for a code. Its password
	// alone is refused as a wrong one is, and counts as one toward the
	// client's attempts, so that the answer tells nothing of the password.
	if user.NeedsTwoStep(h.Accounts.Settings()) {
		attempt.Fail()
		h.Activity.SignInFailed(r.Context(), body.Username, forwarded.Client(r))
		processingError(w, http.StatusUnauthorized)
		return
	}
	attempt.Succeed()
	if h.signInRefusedRemote(w, r, user) {
		return
	}
	// Like Jellyfin, a user outside their allowed hours, administrators
	// included, cannot sign in. Quick Connect, which Jellyfin does not check
	// here, signs them in, and their requests are then refused.
	if !user.AllowedAt(h.now()) {
		processingError(w, http.StatusForbidden)
		return
	}
	h.signIn(w, r, user, c)
}

// signInRefusedRemote refuses user's sign-in from outside the local
// network, which they may not connect from, with an empty 403, and
// records it; it reports whether it did. Like Jellyfin, administrators
// are refused too, once their password is right (UserManager.cs, lines
// 633 to 641); unlike Jellyfin, which only logs it, the refusal is in the
// activity log. Polyfin also checks a sign-in through Quick Connect,
// which Jellyfin lets through to refuse its requests.
func (h *Handler) signInRefusedRemote(w http.ResponseWriter, r *http.Request, user accounts.User) bool {
	if !remoteRefused(r, user) {
		return false
	}
	h.Activity.SignInRefusedRemote(r.Context(), user, forwarded.Client(r))
	processingError(w, http.StatusForbidden)
	return true
}

// signIn issues a token for the calling device and answers with Jellyfin's
// AuthenticationResult.
func (h *Handler) signIn(w http.ResponseWriter, r *http.Request, user accounts.User, c credentials) {
	token, device, err := h.Accounts.SignInDevice(r.Context(), user.ID, accounts.DeviceInfo{
		DeviceID:      c.DeviceID,
		DeviceName:    c.Device,
		Client:        c.Client,
		ClientVersion: c.Version,
		RemoteAddress: forwarded.Client(r),
	})
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	// An account's first device, and those it used before, are not told.
	if device.Unfamiliar && h.NewDevice != nil {
		h.NewDevice(user, device)
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
	h.Activity.SignedIn(r.Context(), user, forwarded.Client(r))
	writeJSON(w, http.StatusOK, AuthenticationResult{
		User:        dto,
		SessionInfo: newSessionInfo(device, user, h.ServerID, h.controllable(device)),
		AccessToken: token,
		ServerId:    h.ServerID,
	})
}

// currentUser answers the caller. Like Jellyfin, an API key, which has no
// user, is a bad request.
func (h *Handler) currentUser(w http.ResponseWriter, r *http.Request) {
	c := callerFrom(r.Context())
	if c.APIKey != nil {
		badRequestProblem(w)
		return
	}
	h.writeUser(w, r, c.User)
}

// users lists the users, optionally filtered by the isHidden and
// isDisabled query parameters: every user, whole, for an administrator;
// for anyone else, what userDtoFor tells them of each.
func (h *Handler) users(w http.ResponseWriter, r *http.Request) {
	hidden, hiddenSet := boolQuery(r, "isHidden")
	disabled, disabledSet := boolQuery(r, "isDisabled")
	users, err := h.Accounts.Users(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	caller := callerFrom(r.Context()).User
	result := []UserDto{}
	for _, user := range users {
		if (hiddenSet && user.IsHidden != hidden) || (disabledSet && user.Refused(h.now()) != disabled) {
			continue
		}
		dto, shown, err := h.userDtoFor(r.Context(), caller, user)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		if shown {
			result = append(result, dto)
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// user answers one user as userDtoFor describes them to the caller. A user
// the caller is not shown is answered as one who does not exist.
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
	dto, shown, err := h.userDtoFor(r.Context(), callerFrom(r.Context()).User, user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if !shown {
		writeJSON(w, http.StatusNotFound, "User not found")
		return
	}
	writeJSON(w, http.StatusOK, dto)
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
