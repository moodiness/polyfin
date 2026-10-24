package jellyfin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
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

// brandingConfiguration answers /Branding/Configuration, to anyone, and
// /System/Configuration/branding, to signed-in users, from the same
// settings: the CSS is the one the web player's theme gets (see
// accounts.Settings.ServedCss).
func (h *Handler) brandingConfiguration(w http.ResponseWriter, _ *http.Request) {
	settings := h.Accounts.Settings()
	writeJSON(w, http.StatusOK, BrandingOptions{LoginDisclaimer: settings.LoginDisclaimer, CustomCss: settings.ServedCss()})
}

// brandingCSS answers the server's CSS, empty when there is none, as
// Jellyfin does.
func (h *Handler) brandingCSS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(h.Accounts.Settings().ServedCss()))
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
