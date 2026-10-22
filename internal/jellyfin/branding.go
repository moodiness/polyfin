package jellyfin

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

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

// brandingConfiguration answers /System/Configuration/branding, to
// signed-in users: the settings as they are, which jellyfin-web's dashboard
// edits and posts back.
func (h *Handler) brandingConfiguration(w http.ResponseWriter, _ *http.Request) {
	settings := h.Accounts.Settings()
	writeJSON(w, http.StatusOK, BrandingOptions{LoginDisclaimer: settings.LoginDisclaimer, CustomCss: settings.CustomCss})
}

// appliedBranding answers /Branding/Configuration, to anyone: what
// jellyfin-web applies, the custom CSS followed by the rules showing the
// server's logo (see logoCSS).
func (h *Handler) appliedBranding(w http.ResponseWriter, _ *http.Request) {
	settings := h.Accounts.Settings()
	writeJSON(w, http.StatusOK, BrandingOptions{LoginDisclaimer: settings.LoginDisclaimer, CustomCss: appliedCSS(settings)})
}

// brandingCSS answers the server's CSS, empty when there is none, as
// Jellyfin does, with the rules showing the server's logo.
func (h *Handler) brandingCSS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(appliedCSS(h.Accounts.Settings())))
}

// appliedCSS is the custom CSS, then logoCSS when the server has a logo.
func appliedCSS(settings accounts.Settings) string {
	if settings.BrandLogo == "" {
		return settings.CustomCss
	}
	return settings.CustomCss + "\n" + logoCSS(logoTag(settings.BrandLogo))
}

// logoTag names a logo's content, for the address its rules give it, so
// that browsers keep it until it changes.
func logoTag(logo string) string {
	sum := sha256.Sum256([]byte(logo))
	return hex.EncodeToString(sum[:6])
}

// logoCSS shows the server's logo, at /Branding/Logo, wherever jellyfin-web
// 12.2 shows Jellyfin's: as the image of its header button and menu, as the
// background of its header title, splash screen, admin menu and TV layout.
// The address is relative to the page, /web/, so that it works under a
// reverse proxy's path too.
func logoCSS(tag string) string {
	url := "url('../Branding/Logo?tag=" + tag + "')"
	return `/* The server's logo (Polyfin › Settings › Web player › Logo). */
img[src*="icon-transparent"], img[src*="banner-light"], img[src*="banner-dark"], .adminDrawerLogo img {
  content: ` + url + ` !important; object-fit: contain;
}
/* A wide logo keeps its shape in the header, where Jellyfin's icon is square. */
.headerHomeButton img, .pageTitleWithLogo img, img[src*="icon-transparent"] {
  width: auto !important; max-width: 10em; height: 2.2em !important;
}
.pageTitleWithDefaultLogo, .layout-tv .pageTitleWithDefaultLogo, .splashLogo {
  background-image: ` + url + ` !important; background-size: contain !important;
  background-repeat: no-repeat !important; background-position: left center !important;
}
.splashLogo { background-position: center center !important; }
`
}

// brandingLogo answers the server's logo, to anyone (the sign-in screen
// shows it): 404 when it has none.
func (h *Handler) brandingLogo(w http.ResponseWriter, r *http.Request) {
	logo := h.Accounts.Settings().BrandLogo
	contentType, image, ok := accounts.BrandLogoImage(logo)
	if !ok {
		http.NotFound(w, r)
		return
	}
	header := w.Header()
	header.Set("Content-Type", contentType)
	header.Set("ETag", `"`+logoTag(logo)+`"`)
	if r.URL.Query().Get("tag") == logoTag(logo) {
		header.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		header.Set("Cache-Control", "no-cache")
	}
	// An SVG is shown as an image, never run as a page.
	header.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	http.ServeContent(w, r, "logo", time.Time{}, bytes.NewReader(image))
}

// updateBranding saves the branding jellyfin-web's dashboard posts to
// /System/Configuration/branding. As in Jellyfin, the body replaces the
// branding whole: a text left out is cleared. SplashscreenEnabled is read
// and ignored, as Polyfin has no splash screen; the custom script, which is
// not Jellyfin's, is kept. Texts above Polyfin's limits are refused.
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
