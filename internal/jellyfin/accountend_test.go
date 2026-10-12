package jellyfin

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// An account past its end date is disabled to apps: it cannot sign in, its
// app is refused, and its policy says so; the administrator's app sending
// that policy back does not disable it, and a later date lets the same
// app in again.
func TestAnEndedAccountIsDisabledToApps(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	guest := s.user("guest", nil)
	adminToken := s.signIn("admin", "tv")
	guestToken := s.signIn("guest", "phone")
	end := func(on time.Time) {
		t.Helper()
		if _, err := s.store.UpdateUser(t.Context(), guest.ID, accounts.UserChanges{EndsOn: &on}, nil); err != nil {
			t.Fatal(err)
		}
	}
	policy := func() map[string]any {
		t.Helper()
		var user struct{ Policy map[string]any }
		if status := s.get(t, "/Users/"+guest.ID.String(), adminToken, &user); status != http.StatusOK {
			t.Fatalf("user: %d", status)
		}
		return user.Policy
	}
	today := accounts.Day(time.Now())

	end(today.AddDate(0, 0, -1))
	if status, _ := s.call(http.MethodGet, "/Users/Me", app("phone", guestToken), nil); status != http.StatusUnauthorized {
		t.Errorf("the ended account's app: %d", status)
	}
	if status, _ := s.call(http.MethodPost, "/Users/AuthenticateByName", app("laptop", ""),
		map[string]string{"Username": "guest", "Pw": "correct horse"}); status != http.StatusForbidden {
		t.Errorf("the ended account's sign-in: %d", status)
	}
	read := policy()
	if read["IsDisabled"] != true {
		t.Errorf("the ended account's policy: IsDisabled %v", read["IsDisabled"])
	}
	encoded, _ := json.Marshal(read)
	if status, body := s.postRaw("/Users/"+guest.ID.String()+"/Policy", app("tv", adminToken), string(encoded)); status != http.StatusNoContent {
		t.Fatalf("policy sent back: %d %s", status, body)
	}
	if stored, err := s.store.User(t.Context(), guest.ID); err != nil || stored.IsDisabled {
		t.Errorf("the policy sent back disabled the account: %+v %v", stored.IsDisabled, err)
	}

	end(today.AddDate(0, 0, 2))
	if status, _ := s.call(http.MethodGet, "/Users/Me", app("phone", guestToken), nil); status != http.StatusOK {
		t.Errorf("the app after a later end date: %d", status)
	}
	if read := policy(); read["IsDisabled"] != false {
		t.Errorf("the policy after a later end date: IsDisabled %v", read["IsDisabled"])
	}
}
