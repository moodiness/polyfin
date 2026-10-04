package jellyfin

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
)

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
// settings.
func (h *Handler) brandingConfiguration(w http.ResponseWriter, _ *http.Request) {
	settings := h.Accounts.Settings()
	writeJSON(w, http.StatusOK, BrandingOptions{LoginDisclaimer: settings.LoginDisclaimer, CustomCss: settings.CustomCss})
}

// brandingCSS answers the server's CSS, empty when there is none, as
// Jellyfin does.
func (h *Handler) brandingCSS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(h.Accounts.Settings().CustomCss))
}

// updateBranding saves the branding jellyfin-web's dashboard posts to
// /System/Configuration/branding. As in Jellyfin, the body replaces the
// branding whole: a text left out is cleared. SplashscreenEnabled is read
// and ignored, as Polyfin has no splash screen; the custom script, which is
// not Jellyfin's, is kept. Texts above Polyfin's limits are refused.
func (h *Handler) updateBranding(w http.ResponseWriter, r *http.Request) {
	errs := bindErrors{}
	raw, ok := requestBody(w, r, "configuration", errs)
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
	settings.LoginDisclaimer, settings.CustomCss = "", ""
	if body.LoginDisclaimer != nil {
		settings.LoginDisclaimer = *body.LoginDisclaimer
	}
	if body.CustomCss != nil {
		settings.CustomCss = *body.CustomCss
	}
	if _, err := h.Accounts.UpdateSettings(r.Context(), settings); err != nil {
		switch {
		case errors.Is(err, accounts.ErrInvalidCustomCss):
			validationProblem(w, map[string][]string{"CustomCss": {"The custom CSS must take at most 256 KB."}})
		case errors.Is(err, accounts.ErrInvalidLoginDisclaimer):
			validationProblem(w, map[string][]string{"LoginDisclaimer": {"The login disclaimer must take at most 8 KB."}})
		default:
			h.internalError(w, r, err)
		}
		return
	}
	h.Activity.SettingsSaved(r.Context(), actor(callerFrom(r.Context())))
	w.WriteHeader(http.StatusNoContent)
}
