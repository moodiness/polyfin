package admin

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/i18n"
)

// Configure links.
//
// Configure on an addon's page opens configureLinkPath followed by a
// token, a link signed for one user and one addon (see configureLink).
// Opening it needs no session, so that it works from any browser or app,
// such as a phone's browser while the admin app runs in an app's web
// view. Polyfin then checks again that the user may replace the addon's
// address, and sends the browser to the addon's configure page (see
// addons.Addon.ConfigureURL). The page renews the link before it expires.
// Neither the token nor the address is ever logged.

const (
	// configureLinkPath is where configure links are opened.
	configureLinkPath = "/admin/configure/"
	// configureLinkLife is how long a configure link opens its page.
	configureLinkLife = 10 * time.Minute
	// configurePurpose is what configure tokens are signed for (see
	// playback.Signer.Mark).
	configurePurpose = "admin.configure"
	// configurePayload is a token's scope (0 for the server's, 1 for the
	// user's own), addon, user and expiry in Unix seconds.
	configurePayload = 1 + 16 + 16 + 8
)

var (
	errConfigureExpired = errors.New("configure link expired")
	errConfigureInvalid = errors.New("configure link invalid")
)

// configureGrant is what a configure token binds.
type configureGrant struct {
	shared  bool
	addon   accounts.ID
	user    accounts.ID
	expires time.Time
}

// mayReplace reports whether user may replace the address of an addon of
// scope now: an administrator a server addon's, the owner their own while
// they may have some and no parental control or blocked genres keep them
// to the server's (see personalAddonsRefused).
func (h *handler) mayReplace(user accounts.User, scope addons.Scope) bool {
	if scope.Owner == nil {
		return user.IsAdministrator
	}
	return *scope.Owner == user.ID && h.Accounts.Settings().PersonalAddonsAllowed(user) && !user.Restricted()
}

// configureLink is the link Configure opens for the caller of r, for addon
// of scope: empty when the addon has no configure page, the caller may
// not replace its address, or no signer is set.
func (h *handler) configureLink(r *http.Request, scope addons.Scope, addon addons.Addon) string {
	user := sessionFrom(r.Context()).User
	if h.Signer == nil || user.ID == (accounts.ID{}) || !addon.Configurable() || !h.mayReplace(user, scope) {
		return ""
	}
	grant := configureGrant{shared: scope.Owner == nil, addon: addon.ID, user: user.ID, expires: h.now().Add(configureLinkLife)}
	return configureLinkPath + h.signConfigure(grant)
}

// signConfigure is the token of grant: its scope, addon, user and expiry,
// then their signature, and nothing else.
func (h *handler) signConfigure(grant configureGrant) string {
	var payload [configurePayload]byte
	if !grant.shared {
		payload[0] = 1
	}
	copy(payload[1:17], grant.addon[:])
	copy(payload[17:33], grant.user[:])
	binary.BigEndian.PutUint64(payload[33:], uint64(grant.expires.Unix()))
	encoded := base64.RawURLEncoding.EncodeToString(payload[:])
	return encoded + "." + h.Signer.Mark(configurePurpose, encoded)
}

// readConfigureToken checks a configure token's signature, then its
// expiry: errConfigureInvalid for a token Polyfin did not sign as it is,
// errConfigureExpired for one past its expiry.
func (h *handler) readConfigureToken(token string) (configureGrant, error) {
	encoded, mark, found := strings.Cut(token, ".")
	if h.Signer == nil || !found || !h.Signer.VerifyMark(configurePurpose, encoded, mark) {
		return configureGrant{}, errConfigureInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(payload) != configurePayload || payload[0] > 1 {
		return configureGrant{}, errConfigureInvalid
	}
	grant := configureGrant{shared: payload[0] == 0, expires: time.Unix(int64(binary.BigEndian.Uint64(payload[33:])), 0)}
	copy(grant.addon[:], payload[1:17])
	copy(grant.user[:], payload[17:33])
	if !h.now().Before(grant.expires) {
		return configureGrant{}, errConfigureExpired
	}
	return grant, nil
}

// openConfigureLink answers GET /admin/configure/{token}, with no session:
// 303 to the addon's configure page while the token is Polyfin's and not
// expired, its user still exists, is enabled and may replace the address
// of the addon (see mayReplace), and the addon still has a configure
// page. Otherwise a page in the browser's language tells the link expired
// (410) or is not valid (404), and no more. Neither the token nor the
// address is logged.
func (h *handler) openConfigureLink(w http.ResponseWriter, r *http.Request) {
	address, err := h.configureAddress(r.Context(), r.PathValue("token"))
	switch {
	case errors.Is(err, errConfigureExpired):
		writeConfigurePage(w, r, http.StatusGone, configureExpiredText)
	case errors.Is(err, errConfigureInvalid):
		writeConfigurePage(w, r, http.StatusNotFound, configureInvalidText)
	case err != nil:
		h.Logger.Error("A configure link could not be checked", "error", err)
		writeConfigurePage(w, r, http.StatusInternalServerError, configureFailedText)
	default:
		w.Header().Set("Location", address)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.WriteHeader(http.StatusSeeOther)
	}
}

// configureAddress is the configure page token opens, after the checks of
// openConfigureLink: errConfigureExpired or errConfigureInvalid when one
// fails, another error when one could not be made.
func (h *handler) configureAddress(ctx context.Context, token string) (string, error) {
	grant, err := h.readConfigureToken(token)
	if err != nil {
		return "", err
	}
	user, err := h.Accounts.User(ctx, grant.user)
	switch {
	case errors.Is(err, accounts.ErrNotFound):
		return "", errConfigureInvalid
	case err != nil:
		return "", err
	case user.IsDisabled:
		return "", errConfigureInvalid
	}
	scope := addons.Shared()
	if !grant.shared {
		scope = addons.Personal(user.ID)
	}
	if !h.mayReplace(user, scope) {
		return "", errConfigureInvalid
	}
	addon, err := h.Addons.InScope(ctx, scope, grant.addon)
	switch {
	case errors.Is(err, addons.ErrNotFound):
		return "", errConfigureInvalid
	case err != nil:
		return "", err
	}
	address, ok := addon.ConfigureURL()
	if !ok {
		return "", errConfigureInvalid
	}
	return address, nil
}

var (
	configureTitleText = i18n.Text{
		"Configure an addon",
		"Configurer un addon",
		"Add-on konfigurieren",
		"Configurar una extensión",
		"Configura un add-on",
		"Configurar um complemento",
	}
	configureExpiredText = i18n.Text{
		"This link has expired. Go back to Polyfin and click Configure again.",
		"Ce lien a expiré. Revenez dans Polyfin et cliquez à nouveau sur Configurer.",
		"Dieser Link ist abgelaufen. Kehre zu Polyfin zurück und klicke erneut auf Konfigurieren.",
		"Este enlace ha caducado. Vuelva a Polyfin y haga clic de nuevo en Configurar.",
		"Questo link è scaduto. Torna su Polyfin e fai di nuovo clic su Configura.",
		"Este link expirou. Volte ao Polyfin e clique em Configurar novamente.",
	}
	configureInvalidText = i18n.Text{
		"This link isn’t valid. Go back to Polyfin and click Configure again.",
		"Ce lien n’est pas valide. Revenez dans Polyfin et cliquez à nouveau sur Configurer.",
		"Dieser Link ist ungültig. Kehre zu Polyfin zurück und klicke erneut auf Konfigurieren.",
		"Este enlace no es válido. Vuelva a Polyfin y haga clic de nuevo en Configurar.",
		"Questo link non è valido. Torna su Polyfin e fai di nuovo clic su Configura.",
		"Este link não é válido. Volte ao Polyfin e clique em Configurar novamente.",
	}
	configureFailedText = i18n.Text{
		"Polyfin could not open this link. Try again in a moment.",
		"Polyfin n’a pas pu ouvrir ce lien. Réessayez dans un instant.",
		"Polyfin konnte diesen Link nicht öffnen. Versuche es gleich noch einmal.",
		"Polyfin no ha podido abrir este enlace. Inténtelo de nuevo en un momento.",
		"Polyfin non è riuscito ad aprire questo link. Riprova tra un momento.",
		"O Polyfin não conseguiu abrir este link. Tente novamente em instantes.",
	}
)

// configurePage is the page of a configure link that does not open: one
// message, with no script and nothing loaded from elsewhere.
var configurePage = template.Must(template.New("configure").Parse(`<!doctype html>
<html lang="{{.Language}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>{{.Title}} · Polyfin</title>
<style>
:root { color-scheme: dark light; }
body { margin: 0; min-height: 100vh; display: grid; place-items: center; padding: 1.5rem; box-sizing: border-box;
  font: 1.0625rem/1.5 system-ui, -apple-system, "Segoe UI", Roboto, sans-serif; background: #15141c; color: #ecebf3; }
main { max-width: 28rem; padding: 1.75rem 1.5rem; border-radius: 1rem; background: #23222c; }
h1 { margin: 0 0 0.75rem; font-size: 1.25rem; line-height: 1.3; }
p { margin: 0; color: #c4c2d0; }
@media (prefers-color-scheme: light) {
  body { background: #f3f2f7; color: #1b1a22; }
  main { background: #fff; }
  p { color: #4a4857; }
}
</style>
</head>
<body>
<main>
<h1>{{.Title}}</h1>
<p>{{.Message}}</p>
</main>
</body>
</html>
`))

// writeConfigurePage answers status with configurePage telling message,
// in the first of Polyfin's languages the browser prefers, else English.
func writeConfigurePage(w http.ResponseWriter, r *http.Request, status int, message i18n.Text) {
	language := "en"
	for part := range strings.SplitSeq(r.Header.Get("Accept-Language"), ",") {
		tag, _, _ := strings.Cut(strings.TrimSpace(part), ";")
		if found := i18n.Match(tag); found != "" {
			language = found
			break
		}
	}
	header := w.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Cache-Control", "no-store")
	// The page's own style, and nothing else, may load: the admin app's
	// policy would refuse an inline style.
	header.Set("Content-Security-Policy",
		"default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	w.WriteHeader(status)
	_ = configurePage.Execute(w, struct{ Language, Title, Message string }{language, configureTitleText.In(language), message.In(language)})
}
