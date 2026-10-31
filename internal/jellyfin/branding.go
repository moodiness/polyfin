package jellyfin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/webtheme"
)

// maxBrandingBody fits the branding with its custom CSS and sign-in message
// at their largest, each character written as JSON's longest escape
// (\u003c).
const maxBrandingBody = 6*(accounts.MaxCustomCodeBytes+accounts.MaxLoginDisclaimerBytes) + maxBody

// BrandingOptions is Jellyfin's branding configuration, which jellyfin-web
// reads to show the disclaimer under its sign-in form and to apply the
// server's CSS, unless the user turned it off in their display settings.
// Jellyfin leaves out a text never set and keeps an empty one; Polyfin
// leaves out an empty one, which jellyfin-web treats alike. Polyfin has no
// splash screen, so SplashscreenEnabled is always false.
type BrandingOptions struct {
	LoginDisclaimer     string `json:",omitempty"`
	CustomCss           string `json:",omitempty"`
	SplashscreenEnabled bool
}

// brandingConfiguration answers /Branding/Configuration, to anyone: the
// sign-in message, and the CSS of the theme the web player shows the
// caller (see brandingTheme). To the web client's page, which names its
// theme in its cookie, the Custom theme's CSS is an import of the
// stylesheet the page loads itself, so that the page knows it for its own.
func (h *Handler) brandingConfiguration(w http.ResponseWriter, r *http.Request) {
	theme, page, err := h.brandingTheme(r)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	settings := h.Accounts.Settings()
	css := settings.ThemeCss(theme)
	if page && theme == accounts.WebThemeCustom && css != "" {
		css = webtheme.Import(webtheme.CustomCss + "?v=" + h.customCss.Hash(css))
	}
	writeJSON(w, http.StatusOK, BrandingOptions{LoginDisclaimer: settings.LoginDisclaimer, CustomCss: css})
}

// serverBranding answers /System/Configuration/branding, to signed-in
// users: the server's branding configuration, with the CSS of the server's
// theme (see accounts.Settings.ServedCss), which jellyfin-web's dashboard
// posts back (see updateBranding).
func (h *Handler) serverBranding(w http.ResponseWriter, _ *http.Request) {
	settings := h.Accounts.Settings()
	writeJSON(w, http.StatusOK, BrandingOptions{LoginDisclaimer: settings.LoginDisclaimer, CustomCss: settings.ServedCss()})
}

// brandingCSS answers the CSS of the theme the web player shows the
// caller (see brandingTheme), empty when there is none, as Jellyfin
// answers the server's.
func (h *Handler) brandingCSS(w http.ResponseWriter, r *http.Request) {
	theme, _, err := h.brandingTheme(r)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(h.Accounts.Settings().ThemeCss(theme)))
}

// brandingTheme is the theme whose CSS Jellyfin's branding gives r, which
// jellyfin-web applies after its own, where Jellyfin gives the server's to
// everyone: each user has a web player theme (see
// accounts.Settings.WebThemeOf), which the web client's page loads itself
// (see webtheme.Cookie). It is:
//   - for the browser whose page names the theme it shows in its cookie,
//     that theme, with page set (a name not in accounts.WebThemes is
//     jellyfin-web's own look);
//   - else for a user's token, their theme, and for an API key, the
//     server's;
//   - else none, jellyfin-web's own look.
func (h *Handler) brandingTheme(r *http.Request) (theme string, page bool, err error) {
	if cookie, err := r.Cookie(webtheme.Cookie(h.ServerID)); err == nil {
		return cookie.Value, true, nil
	}
	settings := h.Accounts.Settings()
	caller, ok, err := h.signedInCaller(r)
	switch {
	case err != nil || !ok:
		return accounts.WebThemeNone, false, err
	case caller.APIKey != nil:
		return settings.WebTheme, false, nil
	}
	return settings.WebThemeOf(caller.User), false, nil
}

// WebTheme is the web player theme of the user signed in, which Polyfin's
// script asks once a user signs in to the web client: Theme, the theme the
// web player shows them, lumaaglaass, none or custom, and UserTheme, their
// own, empty when they keep the server's.
type WebTheme struct {
	Theme     string
	UserTheme string
}

// webTheme answers GET /Polyfin/WebTheme. An API key, without a user,
// gets the server's theme.
func (h *Handler) webTheme(w http.ResponseWriter, r *http.Request) {
	user := callerFrom(r.Context()).User
	writeJSON(w, http.StatusOK, WebTheme{Theme: h.Accounts.Settings().WebThemeOf(user), UserTheme: user.WebTheme})
}

// updateBranding saves the branding jellyfin-web's dashboard posts to
// /System/Configuration/branding. As in Jellyfin, the body replaces the
// branding whole: a text left out is cleared. SplashscreenEnabled is read
// and ignored, as Polyfin has no splash screen. The CSS the web player's
// theme gets, posted back unchanged as jellyfin-web's branding page does,
// keeps the theme; another becomes the Custom CSS, and the theme Custom,
// with the script it had as the Custom script when it was another theme,
// so that the page keeps it: Jellyfin has no script. Texts above Polyfin's
// limits are refused.
func (h *Handler) updateBranding(w http.ResponseWriter, r *http.Request) {
	errs := bindErrors{}
	raw, ok := requestBodyUpTo(w, r, "configuration", errs, maxBrandingBody)
	if !ok {
		return
	}
	var body struct {
		LoginDisclaimer     *string
		CustomCss           *string
		SplashscreenEnabled *bool
	}
	if raw != nil && json.Unmarshal(raw, &body) != nil {
		errs.add("$", "The JSON value could not be converted.")
		errs.add("configuration", "The configuration field is required.")
	}
	if len(errs) > 0 {
		validationProblem(w, errs)
		return
	}
	settings := h.Accounts.Settings()
	settings.LoginDisclaimer = ""
	if body.LoginDisclaimer != nil {
		settings.LoginDisclaimer = *body.LoginDisclaimer
	}
	css := ""
	if body.CustomCss != nil {
		css = *body.CustomCss
	}
	if css != settings.ServedCss() {
		if settings.WebTheme != accounts.WebThemeCustom {
			settings.WebTheme, settings.CustomJs = accounts.WebThemeCustom, settings.ServedJs()
		}
		settings.CustomCss = css
	}
	if _, err := h.Accounts.UpdateSettings(r.Context(), settings); err != nil {
		switch {
		case errors.Is(err, accounts.ErrInvalidCustomCss):
			validationProblem(w, map[string][]string{"CustomCss": {fmt.Sprintf("The custom CSS must take at most %d MB.", accounts.MaxCustomCodeBytes>>20)}})
		case errors.Is(err, accounts.ErrInvalidLoginDisclaimer):
			validationProblem(w, map[string][]string{"LoginDisclaimer": {fmt.Sprintf("The login disclaimer must take at most %d KB.", accounts.MaxLoginDisclaimerBytes>>10)}})
		default:
			h.internalError(w, r, err)
		}
		return
	}
	h.Activity.SettingsSaved(r.Context(), actor(callerFrom(r.Context())))
	w.WriteHeader(http.StatusNoContent)
}
