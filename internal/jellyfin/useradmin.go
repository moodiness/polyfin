package jellyfin

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/throttle"
)

// Users managed from Jellyfin apps.
//
// Administrators create, rename and delete users from jellyfin-web's
// dashboard through the same rules as the admin interface: valid unique
// names, passwords of at least 8 characters, and always an enabled
// administrator left. A user who forgot their password asks for a PIN from
// the local network; the administrator reads it in the admin interface or
// the log, and the user's password becomes the PIN once they enter it.

// requestBody reads a required JSON body bound as parameter, adding to
// errs what ASP.NET reports when it is missing or does not convert. raw is
// nil then.
func requestBody(w http.ResponseWriter, r *http.Request, parameter string, errs bindErrors) (raw []byte, ok bool) {
	if !jsonContent(r.Header.Get("Content-Type")) {
		unsupportedMediaTypeProblem(w)
		return nil, false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		processingError(w, http.StatusRequestEntityTooLarge)
		return nil, false
	}
	switch {
	case len(bytes.TrimSpace(body)) == 0:
		errs.add("", "A non-empty request body is required.")
		errs.add(parameter, "The "+parameter+" field is required.")
	case !json.Valid(body) || bytes.TrimSpace(body)[0] != '{':
		errs.add("$", "The JSON value could not be converted.")
		errs.add(parameter, "The "+parameter+" field is required.")
	default:
		return body, true
	}
	return nil, true
}

// requiredString binds a required string property of a body: missing,
// it is reported as System.Text.Json reports a missing required property
// of typeName; empty, as ASP.NET's Required validation does.
func requiredString(fields map[string]json.RawMessage, name, typeName, parameter string, errs bindErrors) string {
	raw, ok := property(fields, name)
	if !ok {
		errs.add("$", "JSON deserialization for type '"+typeName+"' was missing required properties including: '"+name+"'.")
		errs.add(parameter, "The "+parameter+" field is required.")
		return ""
	}
	value, ok := jsonText(raw)
	if !ok {
		errs.add("$."+name, "The JSON value could not be converted to System.String. Path: $."+name+".")
		errs.add(parameter, "The "+parameter+" field is required.")
		return ""
	}
	if value == "" {
		errs.add(name, "The "+name+" field is required.")
	}
	return value
}

// property finds a body property by name without regard to case, the last
// of duplicates winning, as Jellyfin's JSON settings read them.
func property(fields map[string]json.RawMessage, name string) (json.RawMessage, bool) {
	if raw, ok := fields[name]; ok {
		return raw, true
	}
	for key, raw := range fields {
		if equalFoldASCII(key, name) {
			return raw, true
		}
	}
	return nil, false
}

func equalFoldASCII(a, b string) bool {
	return len(a) == len(b) && bytes.EqualFold([]byte(a), []byte(b))
}

// jsonText reads a string, or a number Jellyfin reads as one; null reads
// as the empty string.
func jsonText(raw json.RawMessage) (string, bool) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, true
	}
	if string(bytes.TrimSpace(raw)) == "null" {
		return "", true
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		return number.String(), true
	}
	return "", false
}

// createUser creates a user from jellyfin-web's dashboard, as Jellyfin's
// CreateUserByName: a user who is not an administrator, hidden from the
// sign-in screen. Unlike Jellyfin's, a Polyfin account always has a
// password: one left out or too short is refused, like a name that is not
// valid or taken.
func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	errs := bindErrors{}
	raw, ok := requestBody(w, r, "request", errs)
	if !ok {
		return
	}
	var name, password string
	if raw != nil {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		name = requiredString(fields, "Name", "Jellyfin.Api.Models.UserDtos.CreateUserByName", "request", errs)
		if value, ok := property(fields, "Password"); ok {
			password, _ = jsonText(value)
		}
	}
	if len(errs) > 0 {
		validationProblem(w, errs)
		return
	}
	user, err := h.Accounts.CreateUser(r.Context(), accounts.NewUser{Name: name, Password: password, IsHidden: true})
	switch {
	case errors.Is(err, accounts.ErrInvalidName), errors.Is(err, accounts.ErrNameTaken), errors.Is(err, accounts.ErrInvalidPassword):
		processingError(w, http.StatusBadRequest)
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	h.Activity.UserCreated(r.Context(), user)
	h.writeUser(w, r, user)
}

// deleteUser deletes a user with their devices, sessions and playlists.
// Like Jellyfin, the last administrator cannot be deleted; unlike
// Jellyfin, which signs them out first, the refusal leaves them signed in.
func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "userId")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	user, err := h.Accounts.User(r.Context(), id)
	if errors.Is(err, accounts.ErrNotFound) {
		notFoundProblem(w)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	err = h.Accounts.DeleteUser(r.Context(), id)
	switch {
	case errors.Is(err, accounts.ErrLastAdministrator):
		processingError(w, http.StatusBadRequest)
	case errors.Is(err, accounts.ErrNotFound):
		notFoundProblem(w)
	case err != nil:
		h.internalError(w, r, err)
	default:
		h.configurations.Delete(id)
		h.Activity.UserDeleted(r.Context(), user.Name)
		w.WriteHeader(http.StatusNoContent)
	}
}

// updateUser applies a UserDto posted by jellyfin-web, as Jellyfin does:
// the user is renamed when Name differs, and their configuration replaced
// by Configuration, a new user's when it is left out. Users update
// themselves; another user takes an administrator. The policy, which the
// DTO carries, is changed through its own endpoint and ignored here, but
// must name its providers, as Jellyfin validates them.
func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	errs := bindErrors{}
	raw, ok := requestBody(w, r, "updateUser", errs)
	if !ok {
		return
	}
	id, set := errs.userID(r)
	var fields map[string]json.RawMessage
	if raw != nil {
		_ = json.Unmarshal(raw, &fields)
		policy := map[string]json.RawMessage{}
		if value, ok := property(fields, "Policy"); ok {
			_ = json.Unmarshal(value, &policy)
		}
		for _, name := range []string{"AuthenticationProviderId", "PasswordResetProviderId"} {
			value, _ := property(policy, name)
			if text, ok := jsonText(value); !ok || text == "" {
				errs.add("Policy."+name, "The "+name+" field is required.")
			}
		}
	}
	configuration := defaultUserConfiguration()
	if value, ok := property(fields, "Configuration"); ok && string(bytes.TrimSpace(value)) != "null" {
		var problems map[string][]string
		if configuration, problems = parseUserConfiguration(value); len(problems) > 0 {
			errs.add("$.Configuration", "The JSON value could not be converted to MediaBrowser.Model.Configuration.UserConfiguration. Path: $.Configuration.")
			errs.add("updateUser", "The updateUser field is required.")
		}
	}
	if len(errs) > 0 {
		validationProblem(w, errs)
		return
	}
	c := callerFrom(r.Context())
	if !set {
		id = c.User.ID
	}
	user, err := h.Accounts.User(r.Context(), id)
	if errors.Is(err, accounts.ErrNotFound) {
		notFoundProblem(w)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if user.ID != c.User.ID && !c.User.IsAdministrator {
		writeJSON(w, http.StatusForbidden, "User update not allowed.")
		return
	}
	nameValue, _ := property(fields, "Name")
	name, _ := jsonText(nameValue)
	if name != user.Name {
		updated, err := h.Accounts.UpdateUserFromDevice(r.Context(), user.ID, accounts.UserChanges{Name: &name}, c.Device.ID)
		switch {
		case errors.Is(err, accounts.ErrInvalidName), errors.Is(err, accounts.ErrNameTaken):
			processingError(w, http.StatusBadRequest)
			return
		case err != nil:
			h.internalError(w, r, err)
			return
		}
		h.Activity.UserChanged(r.Context(), updated)
	}
	value, _ := json.Marshal(configuration)
	if err := h.Preferences.PutConfiguration(r.Context(), user.ID, value); err != nil {
		h.internalError(w, r, err)
		return
	}
	h.configurations.Put(user.ID, configuration)
	w.WriteHeader(http.StatusNoContent)
}

// inLocalNetwork reports whether r comes from the server's machine
// (local) or its local network (inNetwork, local included): loopback,
// private and link-local addresses, as Jellyfin counts them by default.
func inLocalNetwork(r *http.Request) (local, inNetwork bool) {
	address, err := netip.ParseAddr(remoteAddress(r))
	if err != nil {
		return false, false
	}
	address = address.Unmap()
	local = address.IsLoopback()
	return local, local || address.IsPrivate() || address.IsLinkLocalUnicast()
}

// ForgotPasswordResult is Jellyfin's answer to a forgotten password.
type ForgotPasswordResult struct {
	Action            string
	PinFile           string
	PinExpirationDate Time
}

// PinRedeemResult is Jellyfin's answer to a PIN entered.
type PinRedeemResult struct {
	Success    bool
	UsersReset []string
}

// pinPlace tells, in the server language, where the administrator finds
// the PIN: jellyfin-web shows it as the file Jellyfin writes the PIN to.
func (h *Handler) pinPlace() string {
	if h.Accounts.Settings().Language == "fr" {
		return "la page Utilisateurs de l’interface d’administration de Polyfin, et son journal"
	}
	return "the Users page of the Polyfin admin interface, and its log"
}

// forgotPassword gives the user named a PIN valid 30 minutes, which the
// administrator reads them, as Jellyfin writes it to a file on the
// server. Like Jellyfin, the answer is the same whether the user exists
// or not, and a request from outside the local network makes no PIN.
func (h *Handler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	errs := bindErrors{}
	raw, ok := requestBody(w, r, "forgotPasswordRequest", errs)
	if !ok {
		return
	}
	var name string
	if raw != nil {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		name = requiredString(fields, "EnteredUsername", "Jellyfin.Api.Models.UserDtos.ForgotPasswordDto", "forgotPasswordRequest", errs)
	}
	if len(errs) > 0 {
		validationProblem(w, errs)
		return
	}
	result := ForgotPasswordResult{Action: "PinCode", PinFile: h.pinPlace(), PinExpirationDate: Time(h.now().Add(accounts.PasswordResetLifetime))}
	if _, inNetwork := inLocalNetwork(r); !inNetwork {
		h.Logger.Warn("A password reset was asked from outside the local network: no PIN was made", "address", remoteAddress(r))
		writeJSON(w, http.StatusOK, result)
		return
	}
	user, pin, err := h.Accounts.RequestPasswordReset(r.Context(), name)
	switch {
	case errors.Is(err, accounts.ErrNotFound):
	case err != nil:
		h.internalError(w, r, err)
		return
	default:
		result.PinExpirationDate = Time(pin.ExpiresAt)
		// The administrator has no file to read: the PIN is in the log
		// and the admin interface's Users page.
		h.Logger.Info("A password reset PIN was made: the user's password becomes this PIN once they enter it",
			"user", user.Name, "pin", pin.PIN, "valid_until", pin.ExpiresAt.Format(time.RFC3339))
	}
	writeJSON(w, http.StatusOK, result)
}

// redeemPasswordPin resets the password of the users whose PIN is
// entered: it becomes the PIN, as entered. Like Jellyfin, a PIN that does
// not match answers 404; unlike Jellyfin, wrong PINs count as failed
// sign-ins, so that PINs are guessed no faster than passwords.
func (h *Handler) redeemPasswordPin(w http.ResponseWriter, r *http.Request) {
	errs := bindErrors{}
	raw, ok := requestBody(w, r, "forgotPasswordPinRequest", errs)
	if !ok {
		return
	}
	var pin string
	if raw != nil {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		pin = requiredString(fields, "Pin", "Jellyfin.Api.Models.UserDtos.ForgotPasswordPinDto", "forgotPasswordPinRequest", errs)
	}
	if len(errs) > 0 {
		validationProblem(w, errs)
		return
	}
	key := throttle.ClientKey(r)
	if allowed, wait := h.SignIns.Allowed(key); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		processingError(w, http.StatusTooManyRequests)
		return
	}
	users, err := h.Accounts.RedeemPasswordResetPIN(r.Context(), pin)
	if errors.Is(err, accounts.ErrNotFound) {
		h.SignIns.Fail(key)
		processingError(w, http.StatusNotFound)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.SignIns.Succeed(key)
	result := PinRedeemResult{Success: true, UsersReset: make([]string, 0, len(users))}
	for _, user := range users {
		result.UsersReset = append(result.UsersReset, user.Name)
		h.Activity.PasswordChanged(r.Context(), user)
	}
	writeJSON(w, http.StatusOK, result)
}
