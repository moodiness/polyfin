package jellyfin

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/quickconnect"
	"github.com/moodiness/polyfin/internal/totp"
)

// An account with two-step sign-in signs in to Jellyfin apps through Quick
// Connect approved in the admin app; its password alone is refused as a
// wrong one is. So is an administrator's while it is required of them.
func TestTwoStepAccountsSignInThroughQuickConnect(t *testing.T) {
	t.Parallel()
	var approvals *quickconnect.Store
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, _ *pgxpool.Pool) { approvals = o.QuickConnect })
	alice := s.user("alice", nil)
	s.user("bob", nil)
	setup, err := s.store.StartTwoStep(t.Context(), alice)
	if err != nil {
		t.Fatal(err)
	}
	code, _ := totp.Code(setup.Secret, time.Now())
	if _, err := s.store.ConfirmTwoStep(t.Context(), alice.ID, code); err != nil {
		t.Fatal(err)
	}

	tv := app("tv", "")
	for name, password := range map[string]string{"right password": "correct horse", "wrong password": "wrong horse"} {
		status, body := s.call(http.MethodPost, "/Users/AuthenticateByName", tv, map[string]string{"Username": "alice", "Pw": password})
		if status != http.StatusUnauthorized {
			t.Errorf("%s: %d %s", name, status, body)
		}
	}
	if status, body := s.call(http.MethodPost, "/Users/AuthenticateByName", tv, map[string]string{"Username": "bob", "Pw": "correct horse"}); status != http.StatusOK {
		t.Errorf("another user: %d %s", status, body)
	}

	status, body := s.call(http.MethodPost, "/QuickConnect/Initiate", tv, nil)
	var initiated QuickConnectResult
	if status != http.StatusOK || json.Unmarshal(body, &initiated) != nil {
		t.Fatalf("initiate: %d %s", status, body)
	}
	// As the admin app approves it, for the user signed in there.
	if err := approvals.Authorize(initiated.Code, alice.ID); err != nil {
		t.Fatal(err)
	}
	status, body = s.call(http.MethodPost, "/Users/AuthenticateWithQuickConnect", tv, map[string]string{"Secret": initiated.Secret})
	var result AuthenticationResult
	if status != http.StatusOK || json.Unmarshal(body, &result) != nil || result.User.Id != alice.ID.String() {
		t.Fatalf("Quick Connect: %d %s", status, body)
	}
	if status, _ := s.call(http.MethodGet, "/Users/Me", app("tv", result.AccessToken), nil); status != http.StatusOK {
		t.Errorf("the token of Quick Connect: %d", status)
	}

	// Required of administrators: one without it is refused too, and a
	// member is not.
	s.user("root", func(c *accounts.UserChanges) { yes := true; c.IsAdministrator = &yes })
	laptop := s.signIn("root", "laptop")
	s.setting(t, func(settings *accounts.Settings) { settings.TwoStepAdministrators = true })
	if status, _ := s.call(http.MethodPost, "/Users/AuthenticateByName", tv, map[string]string{"Username": "root", "Pw": "correct horse"}); status != http.StatusUnauthorized {
		t.Errorf("an administrator without it: %d", status)
	}
	// The devices signed in before stay signed in.
	if status, _ := s.call(http.MethodGet, "/Users/Me", app("laptop", laptop), nil); status != http.StatusOK {
		t.Errorf("a device signed in before: %d", status)
	}
	if status, _ := s.call(http.MethodPost, "/Users/AuthenticateByName", tv, map[string]string{"Username": "bob", "Pw": "correct horse"}); status != http.StatusOK {
		t.Errorf("a member: %d", status)
	}
	// API keys made by administrators are unchanged.
	_, key, err := s.store.CreateAPIKey(t.Context(), "Scripts")
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := s.call(http.MethodGet, "/Users?ApiKey="+key, "", nil); status != http.StatusOK {
		t.Errorf("an API key: %d", status)
	}
}
