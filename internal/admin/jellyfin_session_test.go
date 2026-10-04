package admin

import (
	"net/http"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// jellyfinToken signs a Jellyfin app in as user and returns its access
// token, as jellyfin-web keeps it.
func (api testAPI) jellyfinToken(user accounts.User) string {
	api.t.Helper()
	token, _, err := api.store.SignInDevice(api.t.Context(), user.ID, accounts.DeviceInfo{
		DeviceID: "browser-" + user.Name, DeviceName: "Browser", Client: "Jellyfin Web", ClientVersion: "12.1.0",
	})
	if err != nil {
		api.t.Fatal(err)
	}
	return token
}

func (api testAPI) createUser(name string, administrator bool) accounts.User {
	api.t.Helper()
	user, err := api.store.CreateUser(api.t.Context(), accounts.NewUser{Name: name, Password: "correct horse", IsAdministrator: administrator})
	if err != nil {
		api.t.Fatal(err)
	}
	return user
}

// exchange asks for an admin session with a Jellyfin token in the
// Authorization header, as the admin app sends jellyfin-web's.
func (b browser) exchange(token string) (int, map[string]any) {
	b.api.t.Helper()
	status, body, _ := b.call(http.MethodPost, "/session/jellyfin", nil,
		"Authorization", `MediaBrowser Client="Jellyfin Web", Device="Browser", DeviceId="browser", Version="12.1.0", Token="`+token+`"`)
	return status, body
}

func (b browser) hasSession() bool {
	b.api.t.Helper()
	status, _, _ := b.call(http.MethodGet, "/session", nil)
	return status == http.StatusOK
}

func TestJellyfinSignInOpensTheAdminAppForAdministrators(t *testing.T) {
	api := newTestAPI(t, 100)
	root := api.createUser("root", true)
	b := api.browser()
	status, body := b.exchange(api.jellyfinToken(root))
	if user, _ := body["user"].(map[string]any); status != http.StatusOK || user["name"] != "root" || user["isAdministrator"] != true {
		t.Fatalf("administrator's token: %d %v", status, body)
	}
	if !b.hasSession() {
		t.Error("no admin session after the exchange")
	}
	var page struct {
		Items []activityEntryJSON `json:"items"`
	}
	b.raw(http.MethodGet, "/activity?limit=1", "", &page)
	if len(page.Items) != 1 || page.Items[0].Type != "AuthenticationSucceeded" || page.Items[0].Name != "root signed in" {
		t.Errorf("activity: %+v", page.Items)
	}
}

func TestJellyfinSignInRefusesWhoMayNotHaveASession(t *testing.T) {
	api := newTestAPI(t, 100)
	root := api.createUser("root", true)
	member := api.createUser("alice", false)
	revoked := api.jellyfinToken(api.createUser("former", true))
	if err := api.store.SignOutDevice(t.Context(), revoked); err != nil {
		t.Fatal(err)
	}
	disabledUser := api.createUser("disabled", true)
	disabled := api.jellyfinToken(disabledUser)
	yes := true
	if _, err := api.store.UpdateUser(t.Context(), disabledUser.ID, accounts.UserChanges{IsDisabled: &yes}, nil); err != nil {
		t.Fatal(err)
	}
	_, key, err := api.store.CreateAPIKey(t.Context(), "Requests")
	if err != nil {
		t.Fatal(err)
	}
	nightUser := api.createUser("night", true)
	night := api.jellyfinToken(nightUser)
	weekdays := &[]accounts.AccessSchedule{{Day: "Weekday", StartHour: 9, EndHour: 17}}
	if _, err := api.store.UpdateUser(t.Context(), nightUser.ID, accounts.UserChanges{AccessSchedules: weekdays}, nil); err != nil {
		t.Fatal(err)
	}
	saturday := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.Local)
	api.clock.Store(&saturday)
	for _, tc := range []struct {
		name, token string
		status      int
		code        string
	}{
		{"member", api.jellyfinToken(member), http.StatusUnauthorized, "invalid_credentials"},
		{"API key", key, http.StatusUnauthorized, "invalid_credentials"},
		{"signed-out device", revoked, http.StatusUnauthorized, "invalid_credentials"},
		{"disabled administrator", disabled, http.StatusUnauthorized, "invalid_credentials"},
		{"outside allowed hours", night, http.StatusForbidden, "outside_allowed_hours"},
		{"no token", "", http.StatusUnauthorized, "invalid_credentials"},
	} {
		b := api.browser()
		if status, body := b.exchange(tc.token); status != tc.status || body["error"] != tc.code {
			t.Errorf("%s: %d %v, want %d %s", tc.name, status, body, tc.status, tc.code)
		}
		if b.hasSession() {
			t.Errorf("%s: an admin session was opened", tc.name)
		}
	}
	// Only the Authorization header carries the token: never the URL, as
	// Jellyfin's ApiKey and api_key parameters, nor legacy headers.
	token := api.jellyfinToken(root)
	for _, request := range [][]string{
		{"/session/jellyfin?ApiKey=" + token},
		{"/session/jellyfin?api_key=" + token},
		{"/session/jellyfin?token=" + token},
		{"/session/jellyfin", "X-Emby-Token", token},
		{"/session/jellyfin", "X-Emby-Authorization", `MediaBrowser Token="` + token + `"`},
	} {
		b := api.browser()
		if status, body, _ := b.call(http.MethodPost, request[0], nil, request[1:]...); status != http.StatusUnauthorized || b.hasSession() {
			t.Errorf("%v: %d %v", request, status, body)
		}
	}
}

// Like a wrong password, an unknown token counts toward the client's failed
// attempts; a valid token that may not open a session counts toward
// nothing.
func TestJellyfinSignInCountsUnknownTokensAsFailures(t *testing.T) {
	api := newTestAPI(t, 3)
	root := api.createUser("root", true)
	member := api.jellyfinToken(api.createUser("alice", false))
	for range 5 {
		api.browser().exchange(member)
	}
	if status, body := api.browser().exchange(api.jellyfinToken(root)); status != http.StatusOK {
		t.Fatalf("after members' tokens: %d %v", status, body)
	}
	for range 3 {
		api.browser().exchange("0123456789abcdef0123456789abcdef")
	}
	if status, body := api.browser().exchange(api.jellyfinToken(root)); status != http.StatusTooManyRequests {
		t.Errorf("after unknown tokens: %d %v", status, body)
	}
}

// An administrator blocked after wrong passwords gets no session from a
// token either, until the block ends.
func TestJellyfinSignInRespectsTheLockout(t *testing.T) {
	api := newTestAPI(t, 100)
	admin := api.signedIn("admin", true)
	if status := admin.raw(http.MethodPut, "/settings", `{"serverName":"Polyfin","quickConnectEnabled":true,"legacyAuthorization":false,"language":"en","loginAttempts":3}`, nil); status != http.StatusOK {
		t.Fatalf("settings: %d", status)
	}
	root := api.createUser("root", true)
	token := api.jellyfinToken(root)
	for range 3 {
		api.browser().call(http.MethodPost, "/session", map[string]string{"name": "root", "password": "wrong password"})
	}
	b := api.browser()
	if status, body := b.exchange(token); status != http.StatusUnauthorized || body["error"] != "invalid_credentials" || b.hasSession() {
		t.Errorf("blocked administrator: %d %v", status, body)
	}
}
