package admin

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// createInvite creates an invite as administrator and returns its id and
// token.
func createInvite(t *testing.T, administrator browser, body map[string]any) (string, string) {
	t.Helper()
	status, created, _ := administrator.call(http.MethodPost, "/invites", body)
	if status != http.StatusCreated {
		t.Fatalf("invite creation: %d %v", status, created)
	}
	return created["id"].(string), created["token"].(string)
}

// A guest joins through an invite's link with a name and a password only:
// whatever else they send, the account takes the model's settings, is no
// administrator, and is signed in to the admin app without a web client.
// The invites list then shows the link used up, and a revoked link refuses
// the next guest.
func TestGuestsJoinThroughInvitesWithoutChoosingTheirSettings(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	limited, err := api.store.CreateUser(t.Context(), accounts.NewUser{Name: "limited", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	if limited, err = api.store.UpdateUser(t.Context(), limited.ID, accounts.UserChanges{
		Parental: &accounts.ParentalControl{MaxRating: new(10)}, LiveTv: new(false), ContentDownloading: new(false), QualityGroup: new(720),
	}, nil); err != nil {
		t.Fatal(err)
	}
	id, token := createInvite(t, administrator, map[string]any{"modelUserId": limited.ID.String()})

	guest := api.browser()
	if status, body, _ := guest.call(http.MethodGet, "/invite/"+token, nil); status != http.StatusOK || body["serverName"] == "" {
		t.Fatalf("reading the invite: %d %v", status, body)
	}
	status, body, _ := guest.call(http.MethodPost, "/invite/"+token, map[string]any{
		"name": "guest", "password": "battery staple",
		"isAdministrator": true, "parentalControl": map[string]any{"maxRating": nil}, "liveTv": true, "downloads": true, "qualityGroup": 0,
	})
	if status != http.StatusCreated || body["webClient"] != false {
		t.Fatalf("joining: %d %v", status, body)
	}
	if status, session, _ := guest.call(http.MethodGet, "/session", nil); status != http.StatusOK || session["user"].(map[string]any)["name"] != "guest" {
		t.Errorf("the guest is not signed in to the admin app: %d %v", status, session)
	}
	if status, _, _ := guest.call(http.MethodGet, "/users", nil); status != http.StatusForbidden {
		t.Errorf("the guest reached the users: %d", status)
	}
	joined, err := api.store.Authenticate(t.Context(), "guest", "battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if joined.IsAdministrator || joined.Parental.MaxRating == nil || *joined.Parental.MaxRating != 10 || joined.LiveTv ||
		joined.ContentDownloading || joined.QualityGroup != 720 {
		t.Errorf("the guest chose their settings: %+v", joined)
	}

	listed := api.invites(t, administrator)
	if len(listed) != 1 || listed[0]["id"] != id || listed[0]["state"] != "used_up" || listed[0]["uses"] != 1.0 ||
		listed[0]["model"].(map[string]any)["name"] != "limited" || listed[0]["createdBy"].(map[string]any)["name"] != "administrator" {
		t.Errorf("invites list: %v", listed)
	}
	if status, body, _ := api.browser().call(http.MethodGet, "/invite/"+token, nil); status != http.StatusGone || body["error"] != "invite_used_up" {
		t.Errorf("a used-up link: %d %v", status, body)
	}

	other, otherToken := createInvite(t, administrator, map[string]any{"maxUses": 5, "expiresInDays": nil})
	if status, body, _ := administrator.call(http.MethodPost, "/invites/"+other+"/revoke", nil); status != http.StatusOK || body["state"] != "revoked" || body["expiresAt"] != nil {
		t.Fatalf("revoking: %d %v", status, body)
	}
	if status, body, _ := api.browser().call(http.MethodPost, "/invite/"+otherToken, map[string]any{"name": "late", "password": "battery staple"}); status != http.StatusGone || body["error"] != "invite_revoked" {
		t.Errorf("a revoked link: %d %v", status, body)
	}
	if _, err := api.store.Authenticate(t.Context(), "late", "battery staple"); err == nil {
		t.Error("a revoked link created an account")
	}
}

// invites lists the invites as administrator sees them.
func (api testAPI) invites(t *testing.T, administrator browser) []map[string]any {
	t.Helper()
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, api.url+"/admin/api/invites", nil)
	response, err := administrator.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var listed []map[string]any
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("invites list: %d %v", response.StatusCode, err)
	}
	return listed
}

// With the web client, the guest's page signs in through the Jellyfin API:
// joining opens no admin session.
func TestJoiningWithTheWebClientLeavesTheSignInToIt(t *testing.T) {
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) { o.WebClient = true })
	administrator := api.signedIn("administrator", true)
	_, token := createInvite(t, administrator, map[string]any{})
	guest := api.browser()
	if status, body, _ := guest.call(http.MethodPost, "/invite/"+token, map[string]any{"name": "guest", "password": "battery staple"}); status != http.StatusCreated || body["webClient"] != true {
		t.Fatalf("joining: %d %v", status, body)
	}
	if status, _, _ := guest.call(http.MethodGet, "/session", nil); status != http.StatusUnauthorized {
		t.Errorf("joining opened an admin session: %d", status)
	}
}

// Guessing tokens is throttled like guessing passwords.
func TestGuessedInviteTokensAreThrottled(t *testing.T) {
	api := newTestAPI(t, 2)
	administrator := api.signedIn("administrator", true)
	_, token := createInvite(t, administrator, map[string]any{})
	guest := api.browser()
	for _, guess := range []string{"nothing", token[:len(token)-2] + "AA"} {
		if status, body, _ := guest.call(http.MethodGet, "/invite/"+guess, nil); status != http.StatusNotFound || body["error"] != "invite_unknown" {
			t.Errorf("guess %d: %d %v", len(guess), status, body)
		}
	}
	status, body, response := guest.call(http.MethodPost, "/invite/"+token, map[string]any{"name": "guest", "password": "battery staple"})
	if status != http.StatusTooManyRequests || body["error"] != "too_many_attempts" || response.Header.Get("Retry-After") == "" {
		t.Errorf("joining after two guesses: %d %v", status, body)
	}
	users, err := api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(users, func(u accounts.User) bool { return u.Name == "guest" }) {
		t.Error("a throttled guest joined")
	}
}
