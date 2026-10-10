package admin

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// jellyfinToken signs a Jellyfin app in as user and returns its access
// token, as jellyfin-web keeps it.
func (api testAPI) jellyfinToken(user accounts.User) string {
	api.t.Helper()
	token, _, err := api.store.SignInDevice(api.t.Context(), user.ID, accounts.DeviceInfo{
		DeviceID: "browser-" + user.Name, DeviceName: "Browser", Client: "Jellyfin Web", ClientVersion: "12.2.0",
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
		"Authorization", `MediaBrowser Client="Jellyfin Web", Device="Browser", DeviceId="browser", Version="12.2.0", Token="`+token+`"`)
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

// A member's token opens a member's session, logged as a sign-in: their own
// pages answer, the administrators' stay forbidden.
func TestJellyfinSignInOpensTheAdminAppForMembers(t *testing.T) {
	api := newTestAPI(t, 100)
	admin := api.signedIn("root", true)
	b := api.browser()
	status, body := b.exchange(api.jellyfinToken(api.createUser("alice", false)))
	if user, _ := body["user"].(map[string]any); status != http.StatusOK || user["name"] != "alice" || user["isAdministrator"] != false {
		t.Fatalf("member's token: %d %v", status, body)
	}
	if status, body, _ := b.call(http.MethodGet, "/session", nil); status != http.StatusOK || body["user"].(map[string]any)["name"] != "alice" {
		t.Errorf("session after the exchange: %d %v", status, body)
	}
	if status, body, _ := b.call(http.MethodGet, "/account/devices", nil); status != http.StatusOK {
		t.Errorf("own devices: %d %v", status, body)
	}
	for _, path := range []string{"/users", "/activity"} {
		if status, body, _ := b.call(http.MethodGet, path, nil); status != http.StatusForbidden || body["error"] != "forbidden" {
			t.Errorf("%s: %d %v", path, status, body)
		}
	}
	var page struct {
		Items []activityEntryJSON `json:"items"`
	}
	admin.raw(http.MethodGet, "/activity?limit=10", "", &page)
	if !slices.ContainsFunc(page.Items, func(entry activityEntryJSON) bool {
		return entry.Type == "AuthenticationSucceeded" && entry.Name == "alice signed in"
	}) {
		t.Errorf("activity: %+v", page.Items)
	}
}

// nightToken returns the token of a new user allowed on weekdays from 9 to
// 17 only.
func (api testAPI) nightToken(name string, administrator bool) string {
	api.t.Helper()
	user := api.createUser(name, administrator)
	token := api.jellyfinToken(user)
	weekdays := &[]accounts.AccessSchedule{{Day: "Weekday", StartHour: 9, EndHour: 17}}
	if _, err := api.store.UpdateUser(api.t.Context(), user.ID, accounts.UserChanges{AccessSchedules: weekdays}, nil); err != nil {
		api.t.Fatal(err)
	}
	return token
}

// disabledToken returns the token a new user had before being disabled.
func (api testAPI) disabledToken(name string, administrator bool) string {
	api.t.Helper()
	user := api.createUser(name, administrator)
	token := api.jellyfinToken(user)
	yes := true
	if _, err := api.store.UpdateUser(api.t.Context(), user.ID, accounts.UserChanges{IsDisabled: &yes}, nil); err != nil {
		api.t.Fatal(err)
	}
	return token
}

func TestJellyfinSignInRefusesWhoMayNotHaveASession(t *testing.T) {
	api := newTestAPI(t, 100)
	root := api.createUser("root", true)
	revoked := api.jellyfinToken(api.createUser("former", true))
	if err := api.store.SignOutDevice(t.Context(), revoked); err != nil {
		t.Fatal(err)
	}
	_, key, err := api.store.CreateAPIKey(t.Context(), "Requests")
	if err != nil {
		t.Fatal(err)
	}
	disabled, disabledMember := api.disabledToken("disabled", true), api.disabledToken("gone", false)
	night, nightMember := api.nightToken("night", true), api.nightToken("owl", false)
	saturday := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.Local)
	api.clock.Store(&saturday)
	for _, tc := range []struct {
		name, token string
		status      int
		code        string
	}{
		{"API key", key, http.StatusUnauthorized, "invalid_credentials"},
		{"signed-out device", revoked, http.StatusUnauthorized, "invalid_credentials"},
		{"disabled administrator", disabled, http.StatusUnauthorized, "invalid_credentials"},
		{"disabled member", disabledMember, http.StatusUnauthorized, "invalid_credentials"},
		{"administrator outside allowed hours", night, http.StatusForbidden, "outside_allowed_hours"},
		{"member outside allowed hours", nightMember, http.StatusForbidden, "outside_allowed_hours"},
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
// attempts; a valid token that may not open a session now counts toward
// nothing.
func TestJellyfinSignInCountsUnknownTokensAsFailures(t *testing.T) {
	api := newTestAPI(t, 3)
	root := api.createUser("root", true)
	night := api.nightToken("owl", false)
	saturday := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.Local)
	api.clock.Store(&saturday)
	for range 5 {
		api.browser().exchange(night)
	}
	if status, body := api.browser().exchange(api.jellyfinToken(root)); status != http.StatusOK {
		t.Fatalf("after tokens outside allowed hours: %d %v", status, body)
	}
	for range 3 {
		api.browser().exchange("0123456789abcdef0123456789abcdef")
	}
	if status, body := api.browser().exchange(api.jellyfinToken(root)); status != http.StatusTooManyRequests {
		t.Errorf("after unknown tokens: %d %v", status, body)
	}
}

// A user blocked after wrong passwords gets no session from a token either,
// until the block ends.
func TestJellyfinSignInRespectsTheLockout(t *testing.T) {
	api := newTestAPI(t, 100)
	admin := api.signedIn("admin", true)
	if status := admin.raw(http.MethodPut, "/settings", `{"serverName":"Polyfin","quickConnectEnabled":true,"legacyAuthorization":false,"language":"en","loginAttempts":3}`, nil); status != http.StatusOK {
		t.Fatalf("settings: %d", status)
	}
	for _, administrator := range []bool{true, false} {
		name := map[bool]string{true: "root", false: "alice"}[administrator]
		token := api.jellyfinToken(api.createUser(name, administrator))
		for range 3 {
			api.browser().call(http.MethodPost, "/session", map[string]string{"name": name, "password": "wrong password"})
		}
		b := api.browser()
		if status, body := b.exchange(token); status != http.StatusUnauthorized || body["error"] != "invalid_credentials" || b.hasSession() {
			t.Errorf("blocked %s: %d %v", name, status, body)
		}
	}
}
