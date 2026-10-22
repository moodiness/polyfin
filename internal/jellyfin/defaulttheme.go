package jellyfin

import "github.com/moodiness/polyfin/internal/accounts"

// The default theme's settings of a user who never saved them.
//
// The default Custom JavaScript (accounts.DefaultCustomJs) loads the
// LumaaGlaass theme with its settings page and its home carousel off. The
// page ignores the loader's options: for a user who never saved it, it
// applies its own recommended values, the carousel on. So, on a server
// whose script is still the default one, Polyfin answers that user's theme
// preferences with the carousel off. Nothing is stored: the user's first
// save of the page replaces the answer, and preferences saved once, even
// emptied by the page's reset, are answered as they are. This can go once
// the page takes the loader's options as its defaults.
var defaultThemePreferencesID = displayPreferencesID("lumaaglaass")

const (
	// defaultThemeClient and defaultThemeEntry name where the page keeps
	// its settings: the CustomPrefs entry of its own id and client.
	defaultThemeClient = "lumaaglaass"
	defaultThemeEntry  = "settings"
	// defaultThemeSettings are the page's settings answered, as the JSON
	// it saves: its other fields keep its own defaults.
	defaultThemeSettings = `{"homeCarousel":false}`
)

// defaultThemeUnsaved is state, the preferences answered for id and client
// never saved, with the default theme's settings when they are its own and
// the server still loads it with the default script.
func (h *Handler) defaultThemeUnsaved(state displayState, id, client string) displayState {
	if id != defaultThemePreferencesID || client != defaultThemeClient || h.Accounts.Settings().CustomJs != accounts.DefaultCustomJs {
		return state
	}
	settings := defaultThemeSettings
	state.Custom = map[string]*string{defaultThemeEntry: &settings}
	return state
}
