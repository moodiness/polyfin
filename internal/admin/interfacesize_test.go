package admin

import (
	"net/http"
	"testing"
)

// A user's interface size is Compact until they choose Large under My
// account; the session's answer carries it, for every browser they open
// the admin app in, and a size not offered is refused.
func TestUsersChooseTheirInterfaceSize(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	member := api.signedIn("member", false)
	other := api.signedIn("other", false)

	sessionSize := func(client browser) any {
		status, body, _ := client.call(http.MethodGet, "/session", nil)
		user, _ := body["user"].(map[string]any)
		if status != http.StatusOK || user == nil {
			t.Fatalf("session: %d %v", status, body)
		}
		return user["interfaceSize"]
	}
	if size := sessionSize(member); size != "compact" {
		t.Errorf("a new user's: %v", size)
	}
	for _, bad := range []any{"", "Large", "huge", nil} {
		if status, body, _ := member.call(http.MethodPut, "/account/interface-size", map[string]any{"interfaceSize": bad}); status != http.StatusBadRequest ||
			body["error"] != "invalid_interface_size" {
			t.Errorf("size %v: %d %v", bad, status, body)
		}
	}
	if status, body, _ := member.call(http.MethodPut, "/account/interface-size", map[string]any{"interfaceSize": "large"}); status != http.StatusOK ||
		body["interfaceSize"] != "large" {
		t.Errorf("choosing Large: %d %v", status, body)
	}
	if size := sessionSize(member); size != "large" {
		t.Errorf("the session after choosing Large: %v", size)
	}
	if size := sessionSize(other); size != "compact" {
		t.Errorf("another user's: %v", size)
	}
	if status, body, _ := member.call(http.MethodPut, "/account/interface-size", map[string]any{"interfaceSize": "compact"}); status != http.StatusOK ||
		body["interfaceSize"] != "compact" {
		t.Errorf("back to Compact: %d %v", status, body)
	}
	if size := sessionSize(member); size != "compact" {
		t.Errorf("the session after Compact: %v", size)
	}
}
