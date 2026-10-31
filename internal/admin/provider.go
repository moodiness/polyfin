package admin

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/forwarded"
	"github.com/moodiness/polyfin/internal/oidc"
)

// Sign-in with an OpenID Connect provider (Settings › Security › Identity
// provider). The admin app and Polyfin's script in jellyfin-web send the
// browser to providerStart, at the server's public address, which sends
// it on to the provider with a state, a nonce and a PKCE challenge; the
// provider sends it back to providerCallback, which checks the state
// against the browser's cookie, takes it once, exchanges the code and
// checks the ID token (see package oidc), then finds the account.
//
// For the admin app, the callback opens an admin session and goes to the
// admin app. For the web client, it gives the browser a ticket, in a
// cookie only providerWebSignIn reads, and goes to /web/, where Polyfin's
// script trades the ticket, with jellyfin-web's device, for a Jellyfin
// sign-in of that device, keeps it where jellyfin-web reads it, and opens
// the web client again, signed in.
//
// No token, code, state or secret is ever logged or answered, but the
// Jellyfin access token of the device signing in, which is answered to
// it as /Users/AuthenticateByName answers it.
const (
	// providerPath is where the routes of the provider's sign-in are.
	providerPath = "/admin/api/provider/"
	// providerCallbackPath is the redirect URI, after the public address.
	providerCallbackPath = providerPath + "callback"
	// providerCookieName holds the app the sign-in is for and its state,
	// tying the provider's answer to the browser that left for it. It is
	// Lax: the provider's answer is a navigation from its site.
	providerCookieName = "polyfin_provider"
	// providerTicketCookieName holds the ticket the web client trades
	// for its sign-in.
	providerTicketCookieName = "polyfin_provider_ticket"
	providerWebPath          = providerPath + "web"
	// providerFlowLifetime is how long the browser has at the provider;
	// providerTicketLifetime how long the web client has to trade its
	// ticket once back.
	providerFlowLifetime   = 10 * time.Minute
	providerTicketLifetime = 2 * time.Minute
	// maxProviderPending bounds the sign-ins and the tickets kept, against
	// many sign-ins started and never ended.
	maxProviderPending = 10_000
	// providerTimeout bounds each request to the provider.
	providerTimeout = 15 * time.Second
)

// The apps a provider's sign-in is for: the admin app, or jellyfin-web.
const (
	providerAdminApp = "admin"
	providerWebApp   = "web"
)

// providerFlow is a sign-in under way at the provider.
type providerFlow struct {
	flow oidc.Flow
	app  string
}

// once keeps values under random tokens, each taken once before it
// expires. They are kept in memory: a restart only has their users sign
// in again.
type once[T any] struct {
	mu      sync.Mutex
	entries map[string]onceEntry[T]
}

type onceEntry[T any] struct {
	value   T
	expires time.Time
}

// add keeps value under token, by its hash, until expires; expired values
// are dropped, and one at random when the store is full.
func (o *once[T]) add(token string, value T, expires, now time.Time) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.entries == nil {
		o.entries = map[string]onceEntry[T]{}
	}
	for key, entry := range o.entries {
		if !entry.expires.After(now) {
			delete(o.entries, key)
		}
	}
	if len(o.entries) >= maxProviderPending {
		for key := range o.entries {
			delete(o.entries, key)
			break
		}
	}
	o.entries[string(accounts.HashAdminToken(token))] = onceEntry[T]{value: value, expires: expires}
}

// take returns the value under token and forgets it, false when there is
// none or it expired.
func (o *once[T]) take(token string, now time.Time) (T, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	key := string(accounts.HashAdminToken(token))
	entry, ok := o.entries[key]
	delete(o.entries, key)
	if !ok || !entry.expires.After(now) {
		var zero T
		return zero, false
	}
	return entry.value, true
}

// providerJSON is the provider the sign-in pages offer: the name of its
// button, and the address that starts its sign-in, at the server's public
// address, to which ?app=admin or ?app=web is added.
type providerJSON struct {
	Name  string `json:"name"`
	Start string `json:"start"`
}

// offeredProvider is the provider the sign-in pages offer, nil while
// none is set or the server's public address, which its redirect URI is
// built from, is not.
func offeredProvider(settings accounts.Settings) *providerJSON {
	if !settings.ProviderSignIn() || settings.PublicAddress == "" {
		return nil
	}
	return &providerJSON{Name: settings.ProviderButton(), Start: settings.PublicAddress + providerPath + "start"}
}

// providerRedirectURI is where the provider sends browsers back: the
// callback at the server's public address, empty without one.
func providerRedirectURI(settings accounts.Settings) string {
	if settings.PublicAddress == "" {
		return ""
	}
	return settings.PublicAddress + providerCallbackPath
}

// providerConfig is the provider and Polyfin's client at it.
func providerConfig(settings accounts.Settings) oidc.Config {
	return oidc.Config{
		Issuer:       settings.ProviderIssuer,
		ClientID:     settings.ProviderClientID,
		ClientSecret: settings.ProviderClientSecret,
		RedirectURI:  providerRedirectURI(settings),
		Groups:       settings.ProviderAdminClaim == "groups",
	}
}

// providerAnswer keeps the answers of the provider's routes out of
// caches, and their address, which holds the provider's code and state,
// out of the next page's referrer.
func providerAnswer(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

// providerBack sends the browser back to app, with what went wrong, a
// code its page explains, or "signed-in" for the web client.
func providerBack(w http.ResponseWriter, r *http.Request, app, outcome string) {
	address := "/admin/?provider=" + url.QueryEscape(outcome)
	if app == providerWebApp {
		address = "/web/?polyfinProvider=" + url.QueryEscape(outcome)
	}
	http.Redirect(w, r, address, http.StatusSeeOther)
}

// secureRequest reports whether the browser reached the server over
// HTTPS, directly or through a proxy.
func secureRequest(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// providerStart sends the browser to the provider's sign-in page, for the
// admin app or, with ?app=web, for the web client. The flow's state is
// kept providerFlowLifetime, and given to the browser in a cookie the
// callback reads, so that a return to another browser, or a state made
// up, fails. The provider not set, or not reachable, sends the browser
// back with provider_off or provider_unreachable.
func (h *handler) providerStart(w http.ResponseWriter, r *http.Request) {
	providerAnswer(w)
	app := providerAdminApp
	if r.URL.Query().Get("app") == providerWebApp {
		app = providerWebApp
	}
	settings := h.Accounts.Settings()
	if offeredProvider(settings) == nil {
		providerBack(w, r, app, "provider_off")
		return
	}
	flow := oidc.NewFlow()
	address, err := h.provider.AuthorizationURL(r.Context(), providerConfig(settings), flow)
	if err != nil {
		h.Logger.Warn("The identity provider could not be reached", "issuer", settings.ProviderIssuer, "error", err)
		providerBack(w, r, app, "provider_unreachable")
		return
	}
	now := h.now()
	h.providerFlows.add(flow.State, providerFlow{flow: flow, app: app}, now.Add(providerFlowLifetime), now)
	http.SetCookie(w, &http.Cookie{
		Name:     providerCookieName,
		Value:    app + "." + flow.State,
		Path:     providerPath,
		MaxAge:   int(providerFlowLifetime.Seconds()),
		HttpOnly: true,
		Secure:   secureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, address, http.StatusSeeOther)
}

// providerCallback ends a sign-in at the provider. It counts as a sign-in
// with a password does toward the client's failed attempts: a state that
// is not the browser's, unknown, used or expired, and a code or an ID
// token refused count as wrong passwords, and are not written to the
// activity log, which is for people. Someone the provider signed in whom
// no account matches, and an account disabled or ended, are written to it
// as refused sign-ins; outside the account's allowed hours, the sign-in is
// refused as with a password.
//
// The account found, by its link to the person, else by the claim naming
// the user, else by the verified email (see accounts.Store.ProviderUser),
// signs in without its second step, which the provider asks for, as
// Jellyfin's SSO plugins do; wrong passwords, which block passwords, do
// not block it.
func (h *handler) providerCallback(w http.ResponseWriter, r *http.Request) {
	providerAnswer(w)
	app, state := providerAdminApp, ""
	if cookie, err := r.Cookie(providerCookieName); err == nil {
		if before, after, ok := strings.Cut(cookie.Value, "."); ok && (before == providerAdminApp || before == providerWebApp) {
			app, state = before, after
		}
	}
	http.SetCookie(w, &http.Cookie{Name: providerCookieName, Path: providerPath, MaxAge: -1, HttpOnly: true,
		Secure: secureRequest(r), SameSite: http.SameSiteLaxMode})
	client := forwarded.Client(r)
	attempt, _ := h.SignIns.Reserve(client)
	if attempt == nil {
		providerBack(w, r, app, "too_many_attempts")
		return
	}
	defer attempt.Cancel()
	query := r.URL.Query()
	now := h.now()
	given := query.Get("state")
	pending, ok := providerFlow{}, false
	if state != "" && subtle.ConstantTimeCompare([]byte(given), []byte(state)) == 1 {
		pending, ok = h.providerFlows.take(given, now)
	}
	if !ok || pending.app != app {
		attempt.Fail()
		providerBack(w, r, app, "provider_state")
		return
	}
	if query.Get("error") != "" {
		// The person cancelled, or the provider refused them: no guess.
		providerBack(w, r, app, "provider_refused")
		return
	}
	settings := h.Accounts.Settings()
	if offeredProvider(settings) == nil {
		providerBack(w, r, app, "provider_off")
		return
	}
	config := providerConfig(settings)
	claims, err := h.provider.SignIn(r.Context(), config, query.Get("code"), pending.flow)
	if err != nil {
		if !errors.Is(err, oidc.ErrDiscovery) {
			attempt.Fail()
		}
		h.Logger.Warn("A sign-in with the identity provider was refused", "error", err)
		providerBack(w, r, app, "provider_failed")
		return
	}
	identity := accounts.ProviderIdentity{
		Issuer:  config.Issuer,
		Subject: claims.Subject(),
		Name:    claims.String(settings.ProviderUserClaim),
		Email:   claims.Email(),
		Administrator: settings.ProviderAdminClaim != "" && settings.ProviderAdminValue != "" &&
			claims.Has(settings.ProviderAdminClaim, settings.ProviderAdminValue),
	}
	shown := identity.Name
	if shown == "" {
		shown = identity.Email
	}
	user, created, err := h.Accounts.ProviderUser(r.Context(), identity, settings.ProviderCreateAccounts)
	switch {
	case errors.Is(err, accounts.ErrNoProviderAccount), errors.Is(err, accounts.ErrProviderLinked),
		errors.Is(err, accounts.ErrInvalidName), errors.Is(err, accounts.ErrNameTaken):
		h.Activity.SignInFailed(r.Context(), shown, client)
		providerBack(w, r, app, "provider_no_account")
		return
	case err != nil:
		h.Logger.Error("A sign-in with the identity provider failed", "error", err)
		providerBack(w, r, app, "internal")
		return
	}
	if created {
		h.Logger.Info("Account created at a sign-in with the identity provider", "user", user.Name)
		h.Activity.UserCreated(r.Context(), user)
	}
	if user.Refused(now) {
		h.Activity.SignInFailed(r.Context(), user.Name, client)
		providerBack(w, r, app, "account_disabled")
		return
	}
	if h.remoteRefused(r, user) {
		providerBack(w, r, app, "outside_local_network")
		return
	}
	if !user.AllowedAt(now) {
		providerBack(w, r, app, "outside_allowed_hours")
		return
	}
	attempt.Succeed()
	if app == providerWebApp {
		ticket := randomToken()
		h.providerTickets.add(ticket, user.ID, now.Add(providerTicketLifetime), now)
		http.SetCookie(w, &http.Cookie{
			Name:     providerTicketCookieName,
			Value:    ticket,
			Path:     providerWebPath,
			MaxAge:   int(providerTicketLifetime.Seconds()),
			HttpOnly: true,
			Secure:   secureRequest(r),
			SameSite: http.SameSiteStrictMode,
		})
		providerBack(w, r, app, "signed-in")
		return
	}
	token, expires, err := h.Accounts.CreateAdminSession(r.Context(), user.ID)
	if err != nil {
		h.Logger.Error("A sign-in with the identity provider failed", "error", err)
		providerBack(w, r, app, "internal")
		return
	}
	h.Activity.SignedIn(r.Context(), user, client)
	setCookie(w, r, token, expires)
	http.Redirect(w, r, "/admin/", http.StatusSeeOther)
}

// randomToken is 32 random bytes in base64url.
func randomToken() string {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// providerWebSignIn trades the ticket a sign-in with the provider gave the
// browser for a Jellyfin sign-in of the web client's device, named in the
// Authorization header as jellyfin-web names it (MediaBrowser Client,
// Device, DeviceId and Version, as /Users/AuthenticateByName reads them),
// once: it answers the device's access token, its user's ID and the
// server's, and the server's name, which jellyfin-web keeps. A ticket
// missing, unknown, used or expired counts as a wrong password, 401
// invalid_credentials; an account disabled or ended since is 403
// account_disabled, from outside the local network without remote access
// 403 outside_local_network, outside its allowed hours 403
// outside_allowed_hours.
// The device is marked as signed in through the provider (see
// signInWithJellyfin).
func (h *handler) providerWebSignIn(w http.ResponseWriter, r *http.Request) {
	providerAnswer(w)
	client := forwarded.Client(r)
	attempt := h.reserve(w, client)
	if attempt == nil {
		return
	}
	defer attempt.Cancel()
	http.SetCookie(w, &http.Cookie{Name: providerTicketCookieName, Path: providerWebPath, MaxAge: -1, HttpOnly: true,
		Secure: secureRequest(r), SameSite: http.SameSiteStrictMode})
	header := r.Header.Get("Authorization")
	param := func(name string) string {
		value := mediaBrowserParam(header, name)
		if decoded, err := url.QueryUnescape(value); err == nil {
			return decoded
		}
		return value
	}
	info := accounts.DeviceInfo{DeviceID: param("DeviceId"), DeviceName: param("Device"), Client: param("Client"),
		ClientVersion: param("Version"), RemoteAddress: client, Provider: true}
	if info.DeviceID == "" || info.DeviceName == "" || info.Client == "" || info.ClientVersion == "" {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	now := h.now()
	cookie, err := r.Cookie(providerTicketCookieName)
	var id accounts.ID
	ok := false
	if err == nil && cookie.Value != "" {
		id, ok = h.providerTickets.take(cookie.Value, now)
	}
	if !ok {
		attempt.Fail()
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	user, err := h.Accounts.User(r.Context(), id)
	if errors.Is(err, accounts.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if user.Refused(now) {
		writeError(w, http.StatusForbidden, "account_disabled")
		return
	}
	if !user.AllowedFrom(forwarded.InLocalNetwork(r)) {
		writeError(w, http.StatusForbidden, "outside_local_network")
		return
	}
	if !user.AllowedAt(now) {
		writeError(w, http.StatusForbidden, "outside_allowed_hours")
		return
	}
	token, device, err := h.Accounts.SignInDevice(r.Context(), user.ID, info)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	attempt.Succeed()
	// An account's first device, and those it used before, are not told,
	// as with a password.
	if device.Unfamiliar && h.Notifications != nil {
		h.Notifications.NewDevice(user, device)
	}
	h.Activity.SignedIn(r.Context(), user, client)
	writeJSON(w, http.StatusOK, map[string]string{
		"accessToken": token,
		"userId":      user.ID.String(),
		"serverId":    h.ServerID,
		"serverName":  h.Accounts.Settings().ServerName,
	})
}
