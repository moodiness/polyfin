package admin

import (
	"fmt"
	"net/http"
	"testing"
)

// A Jellyfin token traded for a session between a client's wrong passwords
// for another account leaves them counted: the eleventh is refused, and
// then the client is, the token included.
func TestASessionFromAJellyfinTokenKeepsTheFailuresForOtherAccountsCounted(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	api.createUser("root", true)
	token := api.jellyfinToken(api.createUser("mallory", false))
	guess := func(password string) (int, any) {
		t.Helper()
		status, body, _ := api.browser().call(http.MethodPost, "/session", map[string]string{"name": "root", "password": password})
		return status, body["error"]
	}
	for i := range 10 {
		if status, code := guess(fmt.Sprintf("guess %d", i)); status != http.StatusUnauthorized || code != "invalid_credentials" {
			t.Fatalf("wrong password %d for root: %d %v, want 401 invalid_credentials", i+1, status, code)
		}
		if i == 9 {
			break
		}
		if status, body := api.browser().exchange(token); status != http.StatusOK {
			t.Fatalf("the client's own token after %d wrong passwords for root: %d %v, want 200", i+1, status, body)
		}
	}
	if status, code := guess("guess 10"); status != http.StatusTooManyRequests || code != "too_many_attempts" {
		t.Errorf("the eleventh wrong password for root, with own sessions between: %d %v, want 429 too_many_attempts", status, code)
	}
	if status, code := guess("correct horse"); status != http.StatusTooManyRequests {
		t.Errorf("the right password for root past the limit: %d %v, want 429", status, code)
	}
	if status, body := api.browser().exchange(token); status != http.StatusTooManyRequests {
		t.Errorf("the client's own token past the limit: %d %v, want 429", status, body)
	}
}
