package admin

import (
	"net/http"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
)

// offeredWebThemes are the themes a user may be given as their own, besides
// the server's: LumaaGlaass and jellyfin-web's own look always, the Custom
// theme while it has a text.
func offeredWebThemes(settings accounts.Settings) []string {
	themes := []string{accounts.WebThemeLumaaGlaass, accounts.WebThemeNone}
	if settings.CustomThemeSet() {
		themes = append(themes, accounts.WebThemeCustom)
	}
	return themes
}

// webThemeOffered reports whether a user may be given theme as their own:
// empty, for the server's, or one of offeredWebThemes. It answers the
// request otherwise.
func webThemeOffered(w http.ResponseWriter, settings accounts.Settings, theme string) bool {
	if theme == "" || slices.Contains(offeredWebThemes(settings), theme) {
		return true
	}
	writeError(w, http.StatusBadRequest, "invalid_web_theme")
	return false
}

// webThemesJSON are the web player themes users choose among: the server's
// theme, which a user without their own gets, and those offered.
type webThemesJSON struct {
	ServerTheme string   `json:"serverTheme"`
	Themes      []string `json:"themes"`
}

func newWebThemesJSON(settings accounts.Settings) webThemesJSON {
	return webThemesJSON{ServerTheme: settings.WebTheme, Themes: offeredWebThemes(settings)}
}

// webThemes lists the themes a user may be given, for any signed-in user.
func (h *handler) webThemes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, newWebThemesJSON(h.Accounts.Settings()))
}

// ownWebTheme answers the caller's web player theme, whether they may
// change it, and the themes they choose among.
func (h *handler) ownWebTheme(w http.ResponseWriter, r *http.Request) {
	user, err := h.Accounts.User(r.Context(), sessionFrom(r.Context()).User.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeOwnWebTheme(w, h.Accounts.Settings(), user)
}

// setOwnWebTheme changes the caller's web player theme, unless the
// administrator keeps them from it.
func (h *handler) setOwnWebTheme(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WebTheme *string `json:"webTheme"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.WebTheme == nil {
		writeError(w, http.StatusBadRequest, "invalid_web_theme")
		return
	}
	settings := h.Accounts.Settings()
	if !webThemeOffered(w, settings, *body.WebTheme) {
		return
	}
	user, err := h.Accounts.SetOwnWebTheme(r.Context(), sessionFrom(r.Context()).User.ID, *body.WebTheme)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeOwnWebTheme(w, settings, user)
}

func writeOwnWebTheme(w http.ResponseWriter, settings accounts.Settings, user accounts.User) {
	writeJSON(w, http.StatusOK, struct {
		WebTheme string `json:"webTheme"`
		Locked   bool   `json:"locked"`
		webThemesJSON
	}{user.WebTheme, user.WebThemeLocked, newWebThemesJSON(settings)})
}
