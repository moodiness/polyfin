package admin

import (
	"net/http"
	"testing"
)

func TestAdministratorsLetUsersManageSubtitles(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	_, created, _ := administrator.call(http.MethodPost, "/users", map[string]any{"name": "member", "password": "correct horse"})
	// A new user may not, and has no picture.
	if created["subtitleManagement"] != false || created["imageTag"] != nil {
		t.Fatalf("new user: %v", created)
	}
	path := "/users/" + created["id"].(string)
	if status, body, _ := administrator.call(http.MethodPatch, path, map[string]any{"subtitleManagement": true}); status != http.StatusOK || body["subtitleManagement"] != true {
		t.Fatalf("granting: %d %v", status, body)
	}
	// Other changes keep it.
	if _, body, _ := administrator.call(http.MethodPatch, path, map[string]any{"liveTv": false}); body["subtitleManagement"] != true {
		t.Errorf("after another change: %v", body)
	}
}
