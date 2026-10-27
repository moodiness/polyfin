package admin

import (
	"context"
	"errors"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/forwarded"
	"github.com/moodiness/polyfin/internal/notifications"
)

type sessionUser struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	IsAdministrator bool   `json:"isAdministrator"`
	// ImageTag identifies the user's profile picture, served at /UserImage,
	// null without one: the admin app shows it on the account button.
	ImageTag *string `json:"imageTag"`
}

func newSessionUser(user accounts.User) sessionUser {
	return sessionUser{ID: user.ID.String(), Name: user.Name, IsAdministrator: user.IsAdministrator, ImageTag: imageTag(user)}
}

type sessionKey struct{}

func sessionFrom(ctx context.Context) accounts.AdminSession {
	session, _ := ctx.Value(sessionKey{}).(accounts.AdminSession)
	return session
}

func setCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     cookiePath,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteStrictMode,
	})
}

func clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: cookiePath, MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

// signedIn serves next only for a valid session of an enabled user, within
// their allowed hours unless they are an administrator, as Jellyfin serves
// its own apps.
func (h *handler) signedIn(next http.HandlerFunc) http.Handler {
	return h.requireSession(next, true)
}

// signedInAnyHour is signedIn whatever the user's allowed hours, so that
// the app still shows who is signed in and signs them out.
func (h *handler) signedInAnyHour(next http.HandlerFunc) http.Handler {
	return h.requireSession(next, false)
}

func (h *handler) requireSession(next http.HandlerFunc, hours bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(cookieName)
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		session, err := h.Accounts.AdminSession(r.Context(), cookie.Value)
		if errors.Is(err, accounts.ErrNotFound) {
			clearCookie(w)
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		if session.Renewed {
			setCookie(w, r, cookie.Value, session.ExpiresAt)
		}
		if hours && !session.User.IsAdministrator && !session.User.AllowedAt(h.now()) {
			writeError(w, http.StatusForbidden, "outside_allowed_hours")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), sessionKey{}, session)))
	})
}

// administrator serves next only for a signed-in administrator.
func (h *handler) administrator(next http.HandlerFunc) http.Handler {
	return h.signedIn(func(w http.ResponseWriter, r *http.Request) {
		if !sessionFrom(r.Context()).User.IsAdministrator {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		next(w, r)
	})
}

func (h *handler) status(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readyWait)
	defer cancel()
	database, setupRequired := "unavailable", false
	if h.Database.Ping(ctx) == nil {
		required, err := h.Accounts.SetupRequired(ctx)
		if err == nil {
			database, setupRequired = "ready", required
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":          "Polyfin",
		"version":       h.Version,
		"serverId":      h.ServerID,
		"database":      database,
		"setupRequired": setupRequired,
		"webClient":     h.WebClient,
		"system":        system(h.InContainer),
	})
}

// system is where Polyfin runs, which tells the admin app how to name a
// local folder: "container" in a container, else its system, "linux",
// "macos" or "windows".
func system(inContainer bool) string {
	switch {
	case inContainer:
		return "container"
	case runtime.GOOS == "darwin":
		return "macos"
	case runtime.GOOS == "windows":
		return "windows"
	}
	return "linux"
}

func (h *handler) startSession(w http.ResponseWriter, r *http.Request, status int, user accounts.User) {
	token, expires, err := h.Accounts.CreateAdminSession(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	sendSession(w, r, status, user, token, expires)
}

func sendSession(w http.ResponseWriter, r *http.Request, status int, user accounts.User, token string, expires time.Time) {
	setCookie(w, r, token, expires)
	writeJSON(w, status, map[string]sessionUser{"user": newSessionUser(user)})
}

func (h *handler) setup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SetupCode string `json:"setupCode"`
		Name      string `json:"name"`
		Password  string `json:"password"`
		// Language is the admin interface's language; it becomes the server
		// language when the server speaks it.
		Language string `json:"language"`
	}
	if !decode(w, r, &body) {
		return
	}
	attempt := h.reserve(w, forwarded.Client(r))
	if attempt == nil {
		return
	}
	defer attempt.Cancel()
	if required, err := h.Accounts.SetupRequired(r.Context()); err != nil {
		h.internalError(w, r, err)
		return
	} else if !required {
		writeError(w, http.StatusConflict, "setup_complete")
		return
	}
	if !setupCodeMatches(h.SetupCode, body.SetupCode) {
		attempt.Fail()
		writeError(w, http.StatusBadRequest, "invalid_setup_code")
		return
	}
	user, err := h.Accounts.CreateFirstAdministrator(r.Context(), body.Name, body.Password, body.Language)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	attempt.Succeed()
	h.Logger.Info("First administrator created", "user", user.Name)
	h.startSession(w, r, http.StatusCreated, user)
}

func (h *handler) signIn(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	key := forwarded.Client(r)
	attempt := h.reserve(w, key)
	if attempt == nil {
		return
	}
	defer attempt.Cancel()
	user, err := h.Accounts.Authenticate(r.Context(), body.Name, body.Password)
	if errors.Is(err, accounts.ErrInvalidCredentials) {
		attempt.Fail()
	}
	var blocked *accounts.BlockError
	if errors.As(err, &blocked) && h.Notifications != nil {
		h.Notifications.AccountBlocked(blocked.User, accounts.DeviceInfo{Client: notifications.AdminApp, RemoteAddress: key})
	}
	if errors.Is(err, accounts.ErrInvalidCredentials) || errors.Is(err, accounts.ErrDisabled) {
		h.Activity.SignInFailed(r.Context(), body.Name, forwarded.Client(r))
	}
	if accountError(w, err) {
		return
	}
	if err == nil && !user.AllowedAt(h.now()) {
		// Like Jellyfin's sign-in, administrators included.
		writeError(w, http.StatusForbidden, "outside_allowed_hours")
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if user.NeedsTwoStep(h.Accounts.Settings()) {
		// The client's failed attempts are forgotten only once the code is
		// right: a right password between wrong codes would forget them.
		h.askSecondStep(w, r, user)
		return
	}
	attempt.Succeed()
	h.Activity.SignedIn(r.Context(), user, forwarded.Client(r))
	h.startSession(w, r, http.StatusOK, user)
}

// signInWithJellyfin opens an admin session for the user a Jellyfin app is
// signed in as, so that jellyfin-web's Dashboard, which leads administrators
// to the admin app, and the Polyfin entry that leads members there need no
// second sign-in. A member's session opens their own pages only, as with a
// password. The access token comes only in the Authorization header, as
// Jellyfin apps send it (MediaBrowser Token="…"), never in a cookie or the
// URL. Only a signed-in device's token counts: API keys have no user. The
// user must be allowed to sign in now, as with a password: not disabled
// (their devices are then signed out), not blocked after wrong passwords,
// within their allowed hours. An account that needs two-step sign-in, or
// to set it up, is refused, as its password is: its devices may have
// signed in before it was turned on, or through Quick Connect.
//
// A token that is no device's counts as a wrong password does toward the
// client's failed attempts; the token of someone who may not have a session
// now is no wrong guess and counts toward nothing. Neither is written to
// the activity log, as the admin app tries the token of whoever opens it.
//
// The session lasts as long as the device's sign-in: it ends when jellyfin-web
// signs out, when the device is signed out from Devices, and when the device
// signs in again, as the same user or another. A browser whose web client
// signs out, or changes user, so keeps no admin session of the user before,
// as with Jellyfin, whose dashboard shares jellyfin-web's sign-in.
func (h *handler) signInWithJellyfin(w http.ResponseWriter, r *http.Request) {
	attempt := h.reserve(w, forwarded.Client(r))
	if attempt == nil {
		return
	}
	defer attempt.Cancel()
	token := mediaBrowserToken(r.Header.Get("Authorization"))
	if token == "" {
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	device, user, err := h.Accounts.DeviceByToken(r.Context(), token, forwarded.Client(r))
	if errors.Is(err, accounts.ErrNotFound) {
		attempt.Fail()
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	now := h.now()
	switch {
	case h.Accounts.Settings().LoginAttempts > 0 && user.Blocked(now),
		user.NeedsTwoStep(h.Accounts.Settings()):
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	case !user.AllowedAt(now):
		writeError(w, http.StatusForbidden, "outside_allowed_hours")
		return
	}
	sessionToken, expires, err := h.Accounts.CreateDeviceAdminSession(r.Context(), device)
	if errors.Is(err, accounts.ErrNotFound) {
		// The device signed out since its token was read: no wrong guess.
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	attempt.Succeed()
	h.Activity.SignedIn(r.Context(), user, forwarded.Client(r))
	sendSession(w, r, http.StatusOK, user, sessionToken, expires)
}

// mediaBrowserToken reads the Token parameter of an Authorization header in
// Jellyfin's MediaBrowser scheme: comma-separated key=value pairs, keys in
// any case, values quoted or not.
func mediaBrowserToken(header string) string {
	scheme, params, _ := strings.Cut(strings.TrimSpace(header), " ")
	if !strings.EqualFold(scheme, "MediaBrowser") {
		return ""
	}
	for params != "" {
		params = strings.TrimLeft(params, " ,")
		key, rest, found := strings.Cut(params, "=")
		if !found {
			return ""
		}
		rest = strings.TrimLeft(rest, " ")
		var value string
		if quoted, ok := strings.CutPrefix(rest, `"`); ok {
			value, params, _ = strings.Cut(quoted, `"`)
		} else {
			value, params, _ = strings.Cut(rest, ",")
		}
		if strings.EqualFold(strings.TrimSpace(key), "Token") {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (h *handler) session(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]sessionUser{"user": newSessionUser(sessionFrom(r.Context()).User)})
}

func (h *handler) signOut(w http.ResponseWriter, r *http.Request) {
	if err := h.Accounts.DeleteAdminSession(r.Context(), sessionFrom(r.Context()).TokenHash); err != nil {
		h.internalError(w, r, err)
		return
	}
	clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}
