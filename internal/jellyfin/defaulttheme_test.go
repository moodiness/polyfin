package jellyfin

import (
	"encoding/json"
	"net/http"
	"testing"
)

const themePreferences = "/DisplayPreferences/lumaaglaass?client=lumaaglaass"

// themeSettings is the default theme's settings entry in the preferences
// at path answered to the caller, and whether there is one.
func (s auxiliaryServer) themeSettings(authorization, path string) (map[string]any, bool) {
	s.t.Helper()
	status, body := s.call(http.MethodGet, path, authorization, "")
	if status != http.StatusOK {
		s.t.Fatalf("%s: %d %s", path, status, body)
	}
	var dto struct{ CustomPrefs map[string]*string }
	if err := json.Unmarshal(body, &dto); err != nil {
		s.t.Fatal(err)
	}
	entry, ok := dto.CustomPrefs["settings"]
	if !ok || entry == nil {
		return nil, false
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(*entry), &settings); err != nil {
		s.t.Fatalf("%s: settings %q: %v", path, *entry, err)
	}
	return settings, true
}

// With the default script, the theme's settings page and home start with
// the carousel off for a user who never saved them, whoever opens them; a
// user's own save, and the page's reset, are answered as saved.
func TestDefaultThemeStartsWithTheCarouselOff(t *testing.T) {
	t.Parallel()
	s := newAuxiliaryServer(t)
	_, alice := s.user("alice", false)
	bob, _ := s.user("bob", false)
	_, admin := s.user("admin", true)

	if settings, ok := s.themeSettings(alice, themePreferences); !ok || settings["homeCarousel"] != false {
		t.Fatalf("a new user's theme settings: %v, %v", settings, ok)
	}
	if settings, ok := s.themeSettings(admin, themePreferences+"&userId="+bob.ID.String()); !ok || settings["homeCarousel"] != false {
		t.Errorf("another user's theme settings, opened by an administrator: %v, %v", settings, ok)
	}
	// Other preferences are left as Jellyfin answers them.
	for _, path := range []string{"/DisplayPreferences/usersettings?client=emby", "/DisplayPreferences/lumaaglaass?client=emby",
		"/DisplayPreferences/settings?client=lumaaglaass"} {
		if settings, ok := s.themeSettings(alice, path); ok {
			t.Errorf("%s: %v", path, settings)
		}
	}

	request, _ := json.Marshal(map[string]any{"CustomPrefs": map[string]string{"settings": `{"homeCarousel":true,"collectionFilter":false}`}})
	if status, body := s.call(http.MethodPost, themePreferences, alice, string(request)); status != http.StatusNoContent {
		t.Fatalf("save: %d %s", status, body)
	}
	if settings, _ := s.themeSettings(alice, themePreferences); settings["homeCarousel"] != true || settings["collectionFilter"] != false || len(settings) != 2 {
		t.Errorf("after a save: %v", settings)
	}
	// The page's reset saves its entry away: the theme's own defaults apply.
	if status, body := s.call(http.MethodPost, themePreferences, alice, `{"CustomPrefs":{}}`); status != http.StatusNoContent {
		t.Fatalf("reset: %d %s", status, body)
	}
	if settings, ok := s.themeSettings(alice, themePreferences); ok {
		t.Errorf("after a reset: %v", settings)
	}
}

// A server whose script is its own, or none, gets the theme's own
// behaviour: nothing is answered for it.
func TestDefaultThemeSettingsOnlyWithTheDefaultScript(t *testing.T) {
	t.Parallel()
	for name, script := range map[string]string{"own": "console.log('mine')", "none": ""} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := newAuxiliaryServer(t)
			settings := s.store.Settings()
			settings.CustomJs = script
			if _, err := s.store.UpdateSettings(t.Context(), settings); err != nil {
				t.Fatal(err)
			}
			_, alice := s.user("alice", false)
			if settings, ok := s.themeSettings(alice, themePreferences); ok {
				t.Errorf("theme settings: %v", settings)
			}
		})
	}
}
