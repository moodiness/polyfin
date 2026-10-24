package admin

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/forwarded"
	"github.com/moodiness/polyfin/internal/qrcode"
)

const (
	// stepCookieName holds the step between a right password and the code
	// of an account with two-step sign-in: it ties the step to the browser
	// that entered the password.
	stepCookieName = "polyfin_two_step"
	// stepLifetime is how long that step lasts.
	stepLifetime = 5 * time.Minute
	// maxPendingSteps bounds the steps kept, against many passwords
	// entered right without their code.
	maxPendingSteps = 10_000
)

// pendingStep is a browser that entered a user's right password, which
// needs their code, or, for an administrator required to use two-step
// sign-in without it, to set it up, before a session opens.
type pendingStep struct {
	user    accounts.ID
	setup   bool
	expires time.Time
}

// pendingSteps are the steps under way, by the hash of their token. They
// are kept in memory: a restart only has their users enter their password
// again.
type pendingSteps struct {
	mu    sync.Mutex
	steps map[string]pendingStep
}

func (p *pendingSteps) add(step pendingStep, now time.Time) string {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	token := base64.RawURLEncoding.EncodeToString(raw)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.steps == nil {
		p.steps = map[string]pendingStep{}
	}
	for key, other := range p.steps {
		if !other.expires.After(now) {
			delete(p.steps, key)
		}
	}
	if len(p.steps) >= maxPendingSteps {
		for key := range p.steps {
			delete(p.steps, key)
			break
		}
	}
	p.steps[string(accounts.HashAdminToken(token))] = step
	return token
}

func (p *pendingSteps) get(token string, now time.Time) (pendingStep, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	step, ok := p.steps[string(accounts.HashAdminToken(token))]
	return step, ok && step.expires.After(now)
}

func (p *pendingSteps) remove(token string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.steps, string(accounts.HashAdminToken(token)))
}

func setStepCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     stepCookieName,
		Value:    token,
		Path:     cookiePath,
		MaxAge:   int(stepLifetime.Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteStrictMode,
	})
}

func clearStepCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: stepCookieName, Path: cookiePath, MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

// twoStepKey counts the wrong codes entered for a user, whichever client
// entered them, beside those of each client.
func twoStepKey(user accounts.ID) string {
	return "two-step:" + user.String()
}

// twoStepSetupJSON is what an authenticator app reads: the secret, the
// otpauth:// address that holds it, and that address as a QR code, a row a
// line of 1 for dark modules and 0 for light ones.
type twoStepSetupJSON struct {
	Secret string   `json:"secret"`
	URI    string   `json:"uri"`
	QR     []string `json:"qr"`
}

func newTwoStepSetupJSON(setup accounts.TwoStepSetup) (twoStepSetupJSON, error) {
	code, err := qrcode.Encode([]byte(setup.URI))
	if err != nil {
		return twoStepSetupJSON{}, err
	}
	return twoStepSetupJSON{Secret: setup.Secret, URI: setup.URI, QR: code.Rows()}, nil
}

// askSecondStep answers a right password of a user who needs a second step:
// no session opens, the browser gets a step valid stepLifetime instead, and
// the answer says what it is for, "code", or "setup" with what the
// authenticator app reads.
func (h *handler) askSecondStep(w http.ResponseWriter, r *http.Request, user accounts.User) {
	step := pendingStep{user: user.ID, setup: !user.TwoStep, expires: h.now().Add(stepLifetime)}
	answer := map[string]any{"twoStep": "code"}
	if step.setup {
		setup, err := h.Accounts.StartTwoStep(r.Context(), user)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		shown, err := newTwoStepSetupJSON(setup)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		answer = map[string]any{"twoStep": "setup", "setup": shown}
	}
	setStepCookie(w, r, h.steps.add(step, h.now()))
	writeJSON(w, http.StatusOK, answer)
}

// signInSecondStep completes a sign-in with the code of the user's
// authenticator app or a recovery code, or, for an administrator setting
// two-step sign-in up, the first code of the app; the session then opens.
// Wrong codes count as wrong passwords do, toward the client's attempts,
// and toward the user's whatever the client. 401 two_step_expired: no step
// under way in this browser, or one that expired.
func (h *handler) signInSecondStep(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &body) {
		return
	}
	cookie, err := r.Cookie(stepCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "two_step_expired")
		return
	}
	step, ok := h.steps.get(cookie.Value, h.now())
	if !ok {
		clearStepCookie(w)
		writeError(w, http.StatusUnauthorized, "two_step_expired")
		return
	}
	// The code counts before it is checked, toward the client's attempts
	// and the user's, so that codes sent at once cannot all be checked.
	key := forwarded.Client(r)
	attempt := h.reserve(w, key, twoStepKey(step.user))
	if attempt == nil {
		return
	}
	defer attempt.Cancel()
	user, err := h.Accounts.User(r.Context(), step.user)
	if errors.Is(err, accounts.ErrNotFound) {
		h.steps.remove(cookie.Value)
		clearStepCookie(w)
		writeError(w, http.StatusUnauthorized, "two_step_expired")
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	now := h.now()
	switch {
	case user.Refused(now):
		h.steps.remove(cookie.Value)
		clearStepCookie(w)
		writeError(w, http.StatusForbidden, "account_disabled")
		return
	case !user.AllowedAt(now):
		writeError(w, http.StatusForbidden, "outside_allowed_hours")
		return
	}
	var codes []string
	recovery := false
	if step.setup {
		codes, err = h.Accounts.ConfirmTwoStep(r.Context(), user.ID, body.Code)
	} else {
		recovery, err = h.Accounts.VerifyTwoStep(r.Context(), user.ID, body.Code)
	}
	if errors.Is(err, accounts.ErrInvalidCode) {
		attempt.Fail()
		h.Activity.SignInFailed(r.Context(), user.Name, key)
	}
	if errors.Is(err, accounts.ErrTwoStepOn) || errors.Is(err, accounts.ErrTwoStepOff) || errors.Is(err, accounts.ErrTwoStepNotStarted) {
		// Changed meanwhile, by an administrator or another browser: the
		// password is asked again.
		h.steps.remove(cookie.Value)
		clearStepCookie(w)
		writeError(w, http.StatusUnauthorized, "two_step_expired")
		return
	}
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.steps.remove(cookie.Value)
	clearStepCookie(w)
	attempt.Succeed()
	if step.setup {
		user.TwoStep = true
		h.Activity.TwoStepTurnedOn(r.Context(), user)
	}
	if recovery {
		h.Activity.RecoveryCodeUsed(r.Context(), user, key)
	}
	h.Activity.SignedIn(r.Context(), user, key)
	token, expires, err := h.Accounts.CreateAdminSession(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	setCookie(w, r, token, expires)
	answer := map[string]any{"user": newSessionUser(user)}
	if codes != nil {
		answer["recoveryCodes"] = codes
	}
	writeJSON(w, http.StatusOK, answer)
}

// twoStepJSON is how the signed-in user's two-step sign-in stands:
// whether it is on, how many recovery codes are left, whether
// POLYFIN_SECRET_KEY can no longer read its secret, so that only recovery
// codes sign in, and whether the server requires it of them.
type twoStepJSON struct {
	On            bool `json:"on"`
	RecoveryCodes int  `json:"recoveryCodes"`
	Unreadable    bool `json:"unreadable"`
	Required      bool `json:"required"`
}

func (h *handler) ownTwoStep(w http.ResponseWriter, r *http.Request) {
	user := sessionFrom(r.Context()).User
	state, err := h.Accounts.TwoStep(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, twoStepJSON{On: state.On, RecoveryCodes: state.RecoveryCodes, Unreadable: state.Unreadable,
		Required: user.IsAdministrator && h.Accounts.Settings().TwoStepAdministrators})
}

// startOwnTwoStep gives the signed-in user a new secret to add to their
// authenticator app; two-step sign-in is on once confirmOwnTwoStep has a
// code of it. 409 two_step_on: it is on already.
func (h *handler) startOwnTwoStep(w http.ResponseWriter, r *http.Request) {
	setup, err := h.Accounts.StartTwoStep(r.Context(), sessionFrom(r.Context()).User)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	shown, err := newTwoStepSetupJSON(setup)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, shown)
}

// confirmOwnTwoStep turns two-step sign-in on with a code of the app given
// the secret, and answers the recovery codes, shown this once. 400
// invalid_code; 409 two_step_not_started without a setup started.
func (h *handler) confirmOwnTwoStep(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &body) {
		return
	}
	user := sessionFrom(r.Context()).User
	codes, err := h.Accounts.ConfirmTwoStep(r.Context(), user.ID, body.Code)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.Activity.TwoStepTurnedOn(r.Context(), user)
	writeJSON(w, http.StatusOK, map[string][]string{"recoveryCodes": codes})
}

// identity is what the signed-in user confirms a sensitive action with: a
// code of their authenticator app, or a recovery code, or their password.
type identity struct {
	Code     string `json:"code"`
	Password string `json:"password"`
}

// confirmIdentity checks the code or the password the signed-in user
// confirms a change of their two-step sign-in with (see checkIdentity).
func (h *handler) confirmIdentity(w http.ResponseWriter, r *http.Request, user accounts.User) bool {
	var body identity
	if !decode(w, r, &body) {
		return false
	}
	return h.checkIdentity(w, r, user, body)
}

// checkIdentity checks the code or the password the signed-in user gave,
// and answers when it is wrong: 400 invalid_code, 403 wrong_password.
// Wrong ones count as wrong passwords at sign-in do.
func (h *handler) checkIdentity(w http.ResponseWriter, r *http.Request, user accounts.User, given identity) bool {
	attempt := h.reserve(w, forwarded.Client(r), twoStepKey(user.ID))
	if attempt == nil {
		return false
	}
	// A right one takes back only its own attempt: it signs nobody in.
	defer attempt.Cancel()
	var err error
	if given.Code != "" {
		_, err = h.Accounts.VerifyTwoStep(r.Context(), user.ID, given.Code)
	} else {
		var right bool
		right, err = h.Accounts.CheckPassword(r.Context(), user.ID, given.Password)
		if err == nil && !right {
			err = accounts.ErrWrongPassword
		}
	}
	if errors.Is(err, accounts.ErrInvalidCode) || errors.Is(err, accounts.ErrWrongPassword) {
		attempt.Fail()
	}
	if accountError(w, err) {
		return false
	}
	if err != nil {
		h.internalError(w, r, err)
		return false
	}
	return true
}

// newOwnRecoveryCodes gives the signed-in user new recovery codes, which
// void the former ones, once confirmed with a code or their password.
func (h *handler) newOwnRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	user := sessionFrom(r.Context()).User
	if !h.confirmIdentity(w, r, user) {
		return
	}
	codes, err := h.Accounts.NewRecoveryCodes(r.Context(), user.ID)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"recoveryCodes": codes})
}

// turnOffOwnTwoStep turns the signed-in user's two-step sign-in off, once
// confirmed with a code or their password. 409 two_step_required: the
// server requires it of administrators.
func (h *handler) turnOffOwnTwoStep(w http.ResponseWriter, r *http.Request) {
	user := sessionFrom(r.Context()).User
	if user.IsAdministrator && h.Accounts.Settings().TwoStepAdministrators {
		writeError(w, http.StatusConflict, "two_step_required")
		return
	}
	if !h.confirmIdentity(w, r, user) {
		return
	}
	if _, err := h.Accounts.TurnOffTwoStep(r.Context(), user.ID); err != nil {
		h.internalError(w, r, err)
		return
	}
	h.Activity.TwoStepTurnedOff(r.Context(), user, user.Name)
	w.WriteHeader(http.StatusNoContent)
}

// turnOffUserTwoStep turns another user's two-step sign-in off, for one who
// lost their device, and records who did in the activity log. They sign in
// with their password alone, or set it up again if the server requires it
// of them. 409 two_step_off: it is off; two_step_own: the administrator's
// own, which they turn off from My account.
func (h *handler) turnOffUserTwoStep(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	administrator := sessionFrom(r.Context()).User
	if id == administrator.ID {
		writeError(w, http.StatusConflict, "two_step_own")
		return
	}
	user, err := h.Accounts.User(r.Context(), id)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if !user.TwoStep {
		writeError(w, http.StatusConflict, "two_step_off")
		return
	}
	if user, err = h.Accounts.TurnOffTwoStep(r.Context(), id); accountError(w, err) {
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.Activity.TwoStepTurnedOff(r.Context(), user, administrator.Name)
	h.writeUser(w, r, http.StatusOK, user)
}
