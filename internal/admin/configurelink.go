package admin

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
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
// address, and answers a page of its own that names the addon and the
// host of its configure page (see addons.Addon.ConfigureURL), with a
// Continue button. Only Continue, a form posted back to Polyfin from that
// page, checks the link again and sends the browser to the configure
// page, once: the link is used up. A program that only reads the link,
// such as a messenger's preview, gets the page and never the address. The
// admin app renews the link before it expires. Neither the token nor the
// address is ever logged.

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

// configureGrant is what a configure token binds, and mark its signature,
// which tells a token apart once used (see configureUses).
type configureGrant struct {
	shared  bool
	addon   accounts.ID
	user    accounts.ID
	expires time.Time
	mark    string
}

// configureUses are the configure tokens used up by Continue, by their
// signature, until they expire; kept in memory, as a token lives
// configureLinkLife only.
type configureUses struct {
	mu   sync.Mutex
	used map[string]time.Time
}

// spent reports whether grant's token was used.
func (u *configureUses) spent(grant configureGrant) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	_, used := u.used[grant.mark]
	return used
}

// use marks grant's token used, and reports false when it already was:
// of two Continues at once, one goes on. The tokens past their expiry,
// which their expiry refuses anyway, are forgotten.
func (u *configureUses) use(grant configureGrant, now time.Time) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if _, used := u.used[grant.mark]; used {
		return false
	}
	if u.used == nil {
		u.used = map[string]time.Time{}
	}
	for mark, expires := range u.used {
		if !now.Before(expires) {
			delete(u.used, mark)
		}
	}
	u.used[grant.mark] = grant.expires
	return true
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
	grant := configureGrant{shared: payload[0] == 0, expires: time.Unix(int64(binary.BigEndian.Uint64(payload[33:])), 0), mark: mark}
	copy(grant.addon[:], payload[1:17])
	copy(grant.user[:], payload[17:33])
	if !h.now().Before(grant.expires) {
		return configureGrant{}, errConfigureExpired
	}
	return grant, nil
}

// configureTarget is what a configure link opens.
type configureTarget struct {
	grant   configureGrant
	name    string
	address *url.URL
}

// openConfigureLink answers GET /admin/configure/{token}, with no session:
// while the link opens (see configureTargetOf), a page of Polyfin's own
// naming the addon and the host of its configure page, whose Continue
// posts back to continueConfigureLink. It never sends the browser on: a
// program that follows the link gets the page alone. Otherwise a page in
// the browser's language tells the link expired (410) or is not valid
// (404), and no more. Neither the token nor the address is logged.
func (h *handler) openConfigureLink(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	target, err := h.configureTargetOf(r.Context(), token)
	if err == nil && h.configureUses.spent(target.grant) {
		err = errConfigureInvalid
	}
	if h.configureRefused(w, r, err) {
		return
	}
	writeConfigureContinue(w, r, target, configureLinkPath+token)
}

// continueConfigureLink answers Continue, POST /admin/configure/{token},
// posted from openConfigureLink's page: a 303 to the addon's configure
// page, without a referrer, after the checks of openConfigureLink again.
// The link is then used up. A post that is not the page's own (see
// configurePostedHere) is refused as a link that is not valid.
func (h *handler) continueConfigureLink(w http.ResponseWriter, r *http.Request) {
	if !configurePostedHere(r) {
		writeConfigurePage(w, r, http.StatusForbidden, configureInvalidText)
		return
	}
	target, err := h.configureTargetOf(r.Context(), r.PathValue("token"))
	if err == nil && !h.configureUses.use(target.grant, h.now()) {
		err = errConfigureInvalid
	}
	if h.configureRefused(w, r, err) {
		return
	}
	w.Header().Set("Location", target.address.String())
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(http.StatusSeeOther)
}

// configurePostedHere reports whether r was posted by a page of Polyfin's
// own origin: the browser says so (Sec-Fetch-Site), and its Origin is this
// server. The admin API's own check lets a request without an Origin
// through; Continue needs one.
func configurePostedHere(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		return false
	}
	origin, err := url.Parse(r.Header.Get("Origin"))
	return err == nil && origin.Host != "" && strings.EqualFold(origin.Host, r.Host)
}

// configureRefused answers the page of a link that does not open, and
// reports whether err is such a failure.
func (h *handler) configureRefused(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, errConfigureExpired):
		writeConfigurePage(w, r, http.StatusGone, configureExpiredText)
	case errors.Is(err, errConfigureInvalid):
		writeConfigurePage(w, r, http.StatusNotFound, configureInvalidText)
	default:
		h.Logger.Error("A configure link could not be checked", "error", err)
		writeConfigurePage(w, r, http.StatusInternalServerError, configureFailedText)
	}
	return true
}

// configureTargetOf is what token opens: errConfigureExpired or
// errConfigureInvalid unless the token is Polyfin's and not expired, its
// user still exists, is neither disabled nor ended, and may replace the
// address of the addon (see mayReplace), and the addon still has a
// configure page; another error when a check could not be made.
func (h *handler) configureTargetOf(ctx context.Context, token string) (configureTarget, error) {
	grant, err := h.readConfigureToken(token)
	if err != nil {
		return configureTarget{}, err
	}
	user, err := h.Accounts.User(ctx, grant.user)
	switch {
	case errors.Is(err, accounts.ErrNotFound):
		return configureTarget{}, errConfigureInvalid
	case err != nil:
		return configureTarget{}, err
	case user.Refused(h.now()):
		return configureTarget{}, errConfigureInvalid
	}
	scope := addons.Shared()
	if !grant.shared {
		scope = addons.Personal(user.ID)
	}
	if !h.mayReplace(user, scope) {
		return configureTarget{}, errConfigureInvalid
	}
	addon, err := h.Addons.InScope(ctx, scope, grant.addon)
	switch {
	case errors.Is(err, addons.ErrNotFound):
		return configureTarget{}, errConfigureInvalid
	case err != nil:
		return configureTarget{}, err
	}
	address, ok := addon.ConfigureURL()
	if !ok {
		return configureTarget{}, errConfigureInvalid
	}
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" {
		return configureTarget{}, errConfigureInvalid
	}
	return configureTarget{grant: grant, name: addon.Name(), address: parsed}, nil
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
	configureLeadText = i18n.Text{
		"Polyfin will open the configure page of %s, on %s.",
		"Polyfin va ouvrir la page de configuration de %s, sur %s.",
		"Polyfin öffnet die Konfigurationsseite von %s auf %s.",
		"Polyfin abrirá la página de configuración de %s, en %s.",
		"Polyfin aprirà la pagina di configurazione di %s, su %s.",
		"O Polyfin vai abrir a página de configuração de %s, em %s.",
	}
	configureNoteText = i18n.Text{
		"Continue only if you asked for it from Polyfin yourself: that page holds the addon’s settings and keys. The link works once.",
		"Ne continuez que si vous l’avez demandée vous-même depuis Polyfin\u202f: cette page contient les réglages et les clés de l’addon. Le lien ne sert qu’une fois.",
		"Fahre nur fort, wenn du sie selbst in Polyfin angefordert hast: Diese Seite enthält die Einstellungen und Schlüssel des Add-ons. Der Link funktioniert nur einmal.",
		"Continúe solo si la pidió usted mismo desde Polyfin: esa página contiene los ajustes y las claves de la extensión. El enlace solo sirve una vez.",
		"Continua solo se l’hai chiesta tu da Polyfin: quella pagina contiene le impostazioni e le chiavi dell’add-on. Il link funziona una sola volta.",
		"Continue apenas se você mesmo a pediu no Polyfin: essa página contém as configurações e as chaves do complemento. O link funciona uma única vez.",
	}
	configureContinueText = i18n.Text{
		"Continue",
		"Continuer",
		"Weiter",
		"Continuar",
		"Continua",
		"Continuar",
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

// configurePage is the page of a configure link: one message, and, with
// Action, a note and the Continue button posting there. It has no script
// and loads nothing from elsewhere.
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
main { max-width: 28rem; padding: 1.75rem 1.5rem; border-radius: 1rem; background: #23222c; overflow-wrap: anywhere; }
h1 { margin: 0 0 0.75rem; font-size: 1.25rem; line-height: 1.3; }
p { margin: 0; color: #c4c2d0; }
p + p { margin-top: 0.75rem; font-size: 0.9375rem; }
form { margin: 1.5rem 0 0; }
button { font: inherit; font-weight: 620; min-height: 2.75rem; padding: 0.5rem 1.25rem; border: 0; border-radius: 0.625rem;
  background: #6366f1; color: #fff; cursor: pointer; }
button:hover { background: #4f52e0; }
button:focus-visible { outline: 2px solid #c7c8ff; outline-offset: 2px; }
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
{{- if .Action}}
<p>{{.Note}}</p>
<form method="post" action="{{.Action}}"><button type="submit">{{.Continue}}</button></form>
{{- end}}
</main>
</body>
</html>
`))

// configurePageData is what configurePage shows.
type configurePageData struct {
	Language, Title, Message string
	// Action, Note and Continue are the Continue form's, for a link that
	// opens; empty otherwise.
	Action, Note, Continue string
}

// configureLanguage is the first of Polyfin's languages the browser of r
// prefers, else English.
func configureLanguage(r *http.Request) string {
	for part := range strings.SplitSeq(r.Header.Get("Accept-Language"), ",") {
		tag, _, _ := strings.Cut(strings.TrimSpace(part), ";")
		if found := i18n.Match(tag); found != "" {
			return found
		}
	}
	return "en"
}

// writeConfigurePage answers status with configurePage telling message,
// in the browser's language (see configureLanguage).
func writeConfigurePage(w http.ResponseWriter, r *http.Request, status int, message i18n.Text) {
	language := configureLanguage(r)
	writeConfigureHTML(w, status, "form-action 'none'", "no-referrer",
		configurePageData{Language: language, Title: configureTitleText.In(language), Message: message.In(language)})
}

// configureOrigin matches the origins configure pages may be at, as the
// page's policy names them.
var configureOrigin = regexp.MustCompile(`^https?://[A-Za-z0-9.:\[\]-]+$`)

// writeConfigureContinue answers the page of a link that opens: the addon
// and the host of its configure page, and Continue posting to action.
//   - Its policy lets the form post to Polyfin only, and the answer to
//     the post lead to the configure page's origin, which browsers check
//     the redirect of a form against.
//   - Its referrer policy is same-origin: the post then carries the
//     page's Origin, which configurePostedHere checks, where no-referrer
//     would have browsers send "null". No other origin learns of the page;
//     the redirect itself has no referrer.
func writeConfigureContinue(w http.ResponseWriter, r *http.Request, target configureTarget, action string) {
	language := configureLanguage(r)
	formAction := "form-action 'self'"
	if origin := target.address.Scheme + "://" + target.address.Host; configureOrigin.MatchString(origin) {
		formAction += " " + origin
	}
	writeConfigureHTML(w, http.StatusOK, formAction, "same-origin", configurePageData{
		Language: language,
		Title:    configureTitleText.In(language),
		Message:  fmt.Sprintf(configureLeadText.In(language), target.name, target.address.Host),
		Action:   action,
		Note:     configureNoteText.In(language),
		Continue: configureContinueText.In(language),
	})
}

// writeConfigureHTML answers status with configurePage showing data, never
// kept, with the policy formAction and the referrer policy referrer: the
// page's own style, and nothing else, may load, as the admin app's policy
// would refuse an inline style.
func writeConfigureHTML(w http.ResponseWriter, status int, formAction, referrer string, data configurePageData) {
	header := w.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Cache-Control", "no-store")
	header.Set("Referrer-Policy", referrer)
	header.Set("Content-Security-Policy",
		"default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; "+formAction+"; frame-ancestors 'none'")
	w.WriteHeader(status)
	_ = configurePage.Execute(w, data)
}
