package admin

import (
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

func TestAdministratorsChooseWhoManagesCollections(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	_, created, _ := administrator.call(http.MethodPost, "/users", map[string]any{"name": "member", "password": "correct horse"})
	if created["collectionManagement"] != false {
		t.Fatalf("new user: %v", created["collectionManagement"])
	}
	// An administrator manages collections unless that is taken away.
	_, admin, _ := administrator.call(http.MethodPost, "/users", map[string]any{"name": "second", "password": "correct horse", "isAdministrator": true})
	if admin["collectionManagement"] != true {
		t.Errorf("new administrator: %v", admin["collectionManagement"])
	}

	path := "/users/" + created["id"].(string)
	status, updated, _ := administrator.call(http.MethodPatch, path, map[string]any{"collectionManagement": true})
	if status != http.StatusOK || updated["collectionManagement"] != true {
		t.Fatalf("granting it: %d %v", status, updated)
	}
	id, _ := accounts.ParseID(created["id"].(string))
	if stored, err := api.store.User(t.Context(), id); err != nil || !stored.CollectionManagement {
		t.Errorf("stored: %v %v", stored.CollectionManagement, err)
	}
	// A change that leaves it out keeps it.
	if status, updated, _ = administrator.call(http.MethodPatch, path, map[string]any{"isHidden": true}); status != http.StatusOK ||
		updated["collectionManagement"] != true {
		t.Errorf("a change without it: %d %v", status, updated)
	}
	if status, updated, _ = administrator.call(http.MethodPatch, "/users/"+admin["id"].(string), map[string]any{"collectionManagement": false}); status != http.StatusOK ||
		updated["collectionManagement"] != false || updated["isAdministrator"] != true {
		t.Errorf("taking it from an administrator: %d %v", status, updated)
	}
}
