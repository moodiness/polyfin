package admin

import (
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A user's audio region is one of the regional variants Polyfin tells
// apart, or none: an administrator sets it on the user's page, the user
// under My account, and resetting their preferences clears it.
func TestUsersPreferAnAudioRegion(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	reader, err := api.store.CreateUser(t.Context(), accounts.NewUser{Name: "reader", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	users := "/users/" + reader.ID.String()
	for _, bad := range []string{"fr", "fre", "en-US", "fr-ca", "es-MX"} {
		if status, body, _ := administrator.call(http.MethodPatch, users, map[string]any{"audioRegion": bad}); status != http.StatusBadRequest ||
			body["error"] != "invalid_audio_region" {
			t.Errorf("region %q: %d %v", bad, status, body)
		}
	}
	if status, body, _ := administrator.call(http.MethodPatch, users, map[string]any{"audioRegion": "es-419"}); status != http.StatusOK ||
		body["audioRegion"] != "es-419" {
		t.Errorf("choosing Latin American Spanish for a user: %d %v", status, body)
	}

	if status, body, _ := member.call(http.MethodPut, "/account/audio-region", map[string]any{"region": "pt"}); status != http.StatusBadRequest ||
		body["error"] != "invalid_audio_region" {
		t.Errorf("an own region not offered: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPut, "/account/audio-region", map[string]any{"region": "fr-CA"}); status != http.StatusOK ||
		body["region"] != "fr-CA" {
		t.Errorf("choosing Quebec French: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodGet, "/account/audio-region", nil); status != http.StatusOK || body["region"] != "fr-CA" {
		t.Errorf("own region: %d %v", status, body)
	}
	if status, _, _ := member.call(http.MethodPost, "/account/preferences/reset", nil); status != http.StatusNoContent {
		t.Fatalf("resetting: %d", status)
	}
	if status, body, _ := member.call(http.MethodGet, "/account/audio-region", nil); status != http.StatusOK || body["region"] != "" {
		t.Errorf("own region after the reset: %d %v", status, body)
	}
}
