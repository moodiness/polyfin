package jellyfin

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// passwordStatuses returns the statuses Jellyfin 12.1 answered to a
// successful password change and to a wrong current password.
func passwordStatuses(t *testing.T) (changed, wrongCurrent int) {
	t.Helper()
	raw, err := os.ReadFile("testdata/jellyfin-12.1/next-statuses.json")
	if err != nil {
		t.Fatal(err)
	}
	var statuses map[string]int
	if err := json.Unmarshal(raw, &statuses); err != nil {
		t.Fatal(err)
	}
	return statuses["PasswordChange"], statuses["PasswordWrongCurrent"]
}

// signsIn reports the status of a sign-in with name and password.
func (s testServer) signsIn(name, password string) int {
	s.t.Helper()
	status, _ := s.call(http.MethodPost, "/Users/AuthenticateByName", app("laptop", ""),
		map[string]string{"Username": name, "Pw": password})
	return status
}

// stringAnswer decodes an answer Jellyfin gives as a JSON string.
func stringAnswer(body []byte) string {
	var text string
	if json.Unmarshal(body, &text) != nil {
		return "not a JSON string: " + string(body)
	}
	return text
}

func TestChangeOwnPassword(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("alice", nil)
	tv := s.signIn("alice", "tv")
	phone := s.signIn("alice", "phone")
	changed, wrongCurrent := passwordStatuses(t)

	status, body := s.call(http.MethodPost, "/Users/Password", app("tv", tv),
		map[string]string{"CurrentPw": "wrong horse", "NewPw": "battery staple"})
	if status != wrongCurrent || stringAnswer(body) != "Invalid user or password entered." {
		t.Errorf("wrong current password: got %d %s, want %d", status, body, wrongCurrent)
	}
	status, body = s.call(http.MethodPost, "/Users/Password", app("tv", tv),
		map[string]string{"CurrentPw": "correct horse", "NewPw": "short"})
	if status != http.StatusBadRequest {
		t.Errorf("too short a new password: got %d %s, want 400", status, body)
	}
	// Polyfin accounts cannot be left without a password.
	status, body = s.call(http.MethodPost, "/Users/Password", app("tv", tv),
		map[string]any{"CurrentPw": "correct horse", "ResetPassword": true})
	if status != http.StatusBadRequest {
		t.Errorf("password reset: got %d %s, want 400", status, body)
	}
	if status, _ := s.call(http.MethodGet, "/Users/Me", app("phone", phone), nil); status != http.StatusOK {
		t.Fatalf("a refused change signed the other device out: %d", status)
	}

	status, body = s.call(http.MethodPost, "/Users/Password", app("tv", tv),
		map[string]string{"CurrentPw": "correct horse", "NewPw": "battery staple"})
	if status != changed || len(body) != 0 {
		t.Fatalf("change: got %d %s, want %d", status, body, changed)
	}
	if status, _ := s.call(http.MethodGet, "/Users/Me", app("tv", tv), nil); status != http.StatusOK {
		t.Errorf("the device that changed the password was signed out: %d", status)
	}
	if status, _ := s.call(http.MethodGet, "/Users/Me", app("phone", phone), nil); status != http.StatusUnauthorized {
		t.Errorf("another device is still signed in: %d", status)
	}
	if status := s.signsIn("alice", "correct horse"); status != http.StatusUnauthorized {
		t.Errorf("the old password still signs in: %d", status)
	}
	if status := s.signsIn("alice", "battery staple"); status != http.StatusOK {
		t.Errorf("the new password does not sign in: %d", status)
	}
}

func TestChangeAnotherUsersPassword(t *testing.T) {
	s := newTestServer(t, 10)
	alice := s.user("alice", func(c *accounts.UserChanges) { yes := true; c.IsAdministrator = &yes })
	bob := s.user("bob", nil)
	adminToken := s.signIn("alice", "tv")
	bobToken := s.signIn("bob", "phone")

	// A member cannot change anyone else's password, even knowing it.
	status, body := s.call(http.MethodPost, "/Users/Password?userId="+alice.ID.String(), app("phone", bobToken),
		map[string]string{"CurrentPw": "correct horse", "NewPw": "battery staple"})
	if status != http.StatusForbidden || stringAnswer(body) != "User is not allowed to update the password." {
		t.Errorf("member changing an administrator's password: got %d %s, want 403", status, body)
	}
	if status := s.signsIn("alice", "correct horse"); status != http.StatusOK {
		t.Errorf("the administrator's password changed: %d", status)
	}

	status, body = s.call(http.MethodPost, "/Users/"+bob.ID.String()+"/Password", app("tv", adminToken),
		map[string]string{"NewPw": "battery staple"})
	if status != http.StatusNoContent {
		t.Fatalf("administrator changing a member's password: got %d %s, want 204", status, body)
	}
	if status, _ := s.call(http.MethodGet, "/Users/Me", app("phone", bobToken), nil); status != http.StatusUnauthorized {
		t.Errorf("the member's device is still signed in: %d", status)
	}
	if status, _ := s.call(http.MethodGet, "/Users/Me", app("tv", adminToken), nil); status != http.StatusOK {
		t.Errorf("the administrator was signed out: %d", status)
	}
	if status := s.signsIn("bob", "battery staple"); status != http.StatusOK {
		t.Errorf("the member's new password does not sign in: %d", status)
	}

	status, body = s.call(http.MethodPost, "/Users/Password?userId=0123456789abcdef0123456789abcdef", app("tv", adminToken),
		map[string]string{"NewPw": "battery staple"})
	if status != http.StatusNotFound {
		t.Errorf("unknown user: got %d %s, want 404", status, body)
	}
}

func TestPasswordGuessingIsThrottled(t *testing.T) {
	s := newTestServer(t, 2)
	s.user("alice", nil)
	token := s.signIn("alice", "tv")
	for range 2 {
		s.call(http.MethodPost, "/Users/Password", app("tv", token),
			map[string]string{"CurrentPw": "wrong horse", "NewPw": "battery staple"})
	}
	status, _ := s.call(http.MethodPost, "/Users/Password", app("tv", token),
		map[string]string{"CurrentPw": "correct horse", "NewPw": "battery staple"})
	if status != http.StatusTooManyRequests {
		t.Errorf("change after the failure limit: got %d, want 429", status)
	}
	// The guesses share the budget of sign-ins from the same address.
	if status := s.signsIn("alice", "correct horse"); status != http.StatusTooManyRequests {
		t.Errorf("sign-in after the failure limit: got %d, want 429", status)
	}
}
