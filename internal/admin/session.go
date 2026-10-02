package admin

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/throttle"
)

type sessionUser struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	IsAdministrator bool   `json:"isAdministrator"`
}

func newSessionUser(user accounts.User) sessionUser {
	return sessionUser{ID: user.ID.String(), Name: user.Name, IsAdministrator: user.IsAdministrator}
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

// signedIn serves next only for a valid session of an enabled user.
func (h *handler) signedIn(next http.HandlerFunc) http.Handler {
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
	})
}

func (h *handler) startSession(w http.ResponseWriter, r *http.Request, status int, user accounts.User) {
	token, expires, err := h.Accounts.CreateAdminSession(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	setCookie(w, r, token, expires)
	writeJSON(w, status, map[string]sessionUser{"user": newSessionUser(user)})
}

func (h *handler) setup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SetupCode string `json:"setupCode"`
		Name      string `json:"name"`
		Password  string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	key := throttle.ClientKey(r)
	if h.throttled(w, key) {
		return
	}
	if required, err := h.Accounts.SetupRequired(r.Context()); err != nil {
		h.internalError(w, r, err)
		return
	} else if !required {
		writeError(w, http.StatusConflict, "setup_complete")
		return
	}
	if !setupCodeMatches(h.SetupCode, body.SetupCode) {
		h.SignIns.Fail(key)
		writeError(w, http.StatusBadRequest, "invalid_setup_code")
		return
	}
	user, err := h.Accounts.CreateFirstAdministrator(r.Context(), body.Name, body.Password)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.SignIns.Succeed(key)
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
	key := throttle.ClientKey(r)
	if h.throttled(w, key) {
		return
	}
	user, err := h.Accounts.Authenticate(r.Context(), body.Name, body.Password)
	if errors.Is(err, accounts.ErrInvalidCredentials) {
		h.SignIns.Fail(key)
	}
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.SignIns.Succeed(key)
	h.startSession(w, r, http.StatusOK, user)
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
