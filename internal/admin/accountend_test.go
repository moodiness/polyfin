package admin

import (
	"net/http"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// An administrator sets a member's end date on their page: from it, the
// member's admin session and sign-in are refused; a later date lets them
// sign in again. An administrator cannot be given one, and a date is a
// date.
func TestAnAdministratorEndsAnAccountFromItsPage(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	ids := map[string]string{}
	users, err := api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range users {
		ids[user.Name] = user.ID.String()
	}
	today := accounts.Day(time.Now())
	patch := func(name string, endsOn any) (int, map[string]any) {
		t.Helper()
		status, body, _ := administrator.call(http.MethodPatch, "/users/"+ids[name], map[string]any{"endsOn": endsOn})
		return status, body
	}

	yesterday := today.AddDate(0, 0, -1).Format(time.DateOnly)
	if status, body := patch("member", yesterday); status != http.StatusOK || body["endsOn"] != yesterday || body["ended"] != true {
		t.Fatalf("ending the member: %d %v", status, body)
	}
	if status, _, _ := member.call(http.MethodGet, "/session", nil); status != http.StatusUnauthorized {
		t.Errorf("the ended member's session: %d", status)
	}
	signIn := map[string]string{"name": "member", "password": "correct horse"}
	if status, body, _ := member.call(http.MethodPost, "/session", signIn); status != http.StatusForbidden || body["error"] != "account_disabled" {
		t.Errorf("the ended member's sign-in: %d %v", status, body)
	}
	later := today.AddDate(0, 0, 2).Format(time.DateOnly)
	if status, body := patch("member", later); status != http.StatusOK || body["endsOn"] != later || body["ended"] != false {
		t.Fatalf("a later end date: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPost, "/session", signIn); status != http.StatusOK {
		t.Errorf("the member's sign-in after a later date: %d %v", status, body)
	}
	if status, body := patch("administrator", later); status != http.StatusConflict || body["error"] != "administrator_cannot_end" {
		t.Errorf("an administrator's end date: %d %v", status, body)
	}
	for _, refused := range []any{"2026-02-30", "tomorrow", 20261012} {
		if status, body := patch("member", refused); status != http.StatusBadRequest || body["error"] != "invalid_ends_on" {
			t.Errorf("end date %v: %d %v", refused, status, body)
		}
	}
	if status, body := patch("member", nil); status != http.StatusOK || body["endsOn"] != nil {
		t.Errorf("Never: %d %v", status, body)
	}
}

// An invite link's account end is listed with it and given to each account
// it creates; 0 days is refused, as only null says never.
func TestAnInviteGivesItsAccountsTheirEnd(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	if status, body, _ := administrator.call(http.MethodPost, "/invites", map[string]any{"accountsEndInDays": 0}); status != http.StatusBadRequest ||
		body["error"] != "invalid_invite_account_end" {
		t.Errorf("an end of 0 days: %d %v", status, body)
	}
	_, token := createInvite(t, administrator, map[string]any{"accountsEndInDays": 1})
	if listed := api.invites(t, administrator); len(listed) != 1 || listed[0]["accountsEndInDays"] != 1.0 {
		t.Errorf("invites list: %v", listed)
	}
	if status, body, _ := api.browser().call(http.MethodPost, "/invite/"+token, map[string]any{"name": "guest", "password": "battery staple"}); status != http.StatusCreated {
		t.Fatalf("joining: %d %v", status, body)
	}
	guest, err := api.store.Authenticate(t.Context(), "guest", "battery staple")
	if want := accounts.Day(time.Now()).AddDate(0, 0, 1); err != nil || guest.EndsOn == nil || !guest.EndsOn.Equal(want) {
		t.Errorf("the guest ends on %v (%v), want %v", guest.EndsOn, err, want)
	}
}
