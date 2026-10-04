package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// raw sends a request and decodes any JSON answer into into.
func (b browser) raw(method, path, body string, into any) int {
	b.api.t.Helper()
	request, _ := http.NewRequestWithContext(b.api.t.Context(), method, b.api.url+"/admin/api"+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := b.client.Do(request)
	if err != nil {
		b.api.t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if into != nil {
		_ = json.Unmarshal(data, into)
	}
	return response.StatusCode
}

func TestAPIKeysAreShownOnceAndRevoked(t *testing.T) {
	api := newTestAPI(t, 10)
	admin := api.signedIn("root", true)
	member := api.signedIn("alice", false)
	if status := member.raw(http.MethodGet, "/api-keys", "", nil); status != http.StatusForbidden {
		t.Errorf("member: %d", status)
	}
	if status, body, _ := admin.call(http.MethodPost, "/api-keys", map[string]string{"app": " "}); status != http.StatusBadRequest || body["error"] != "invalid_app" {
		t.Errorf("empty app: %d %v", status, body)
	}
	var created apiKeyJSON
	if status := admin.raw(http.MethodPost, "/api-keys", `{"app":"Requests"}`, &created); status != http.StatusCreated || len(created.Key) != 32 {
		t.Fatalf("create: %d %+v", status, created)
	}
	var keys []map[string]any
	admin.raw(http.MethodGet, "/api-keys", "", &keys)
	if len(keys) != 1 || keys[0]["app"] != "Requests" || keys[0]["lastUsedAt"] != nil {
		t.Errorf("keys: %v", keys)
	}
	if _, shown := keys[0]["key"]; shown {
		t.Error("the key is listed")
	}
	if _, err := api.store.APIKeyByToken(t.Context(), created.Key); err != nil {
		t.Fatal(err)
	}
	if status := admin.raw(http.MethodDelete, "/api-keys/"+created.ID, "", nil); status != http.StatusNoContent {
		t.Errorf("revoke: %d", status)
	}
	if _, err := api.store.APIKeyByToken(t.Context(), created.Key); err == nil {
		t.Error("the revoked key still works")
	}
	if status := admin.raw(http.MethodDelete, "/api-keys/"+created.ID, "", nil); status != http.StatusNotFound {
		t.Errorf("revoked twice: %d", status)
	}
}

func TestActivityAndResetPinsAreShownToAdministrators(t *testing.T) {
	api := newTestAPI(t, 10)
	admin := api.signedIn("root", true)
	api.browser().call(http.MethodPost, "/session", map[string]string{"name": "root", "password": "wrong password"})
	admin.call(http.MethodPost, "/users", map[string]any{"name": "bob", "password": "correct horse"})
	if _, _, err := api.store.RequestPasswordReset(t.Context(), "BOB"); err != nil {
		t.Fatal(err)
	}
	var page struct {
		Items []activityEntryJSON `json:"items"`
		Total int                 `json:"total"`
	}
	if status := admin.raw(http.MethodGet, "/activity?limit=2", "", &page); status != http.StatusOK {
		t.Fatalf("activity: %d", status)
	}
	if page.Total != 3 || len(page.Items) != 2 || page.Items[0].Type != "UserCreated" || page.Items[1].Type != "AuthenticationFailed" {
		t.Errorf("activity: %+v", page)
	}
	if status := admin.raw(http.MethodGet, "/activity?limit=500", "", nil); status != http.StatusBadRequest {
		t.Errorf("limit too high: %d", status)
	}
	var users []userJSON
	admin.raw(http.MethodGet, "/users", "", &users)
	for _, user := range users {
		if (user.Name == "bob") != (user.PasswordResetPin != nil) {
			t.Errorf("%s: PIN %+v", user.Name, user.PasswordResetPin)
		}
	}
}
