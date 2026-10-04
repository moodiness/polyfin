package admin

import (
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

func TestAdministratorsSetQualityGroups(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	_, created, _ := administrator.call(http.MethodPost, "/users", map[string]any{"name": "child", "password": "correct horse"})
	// New users keep every version: Original.
	if created["qualityGroup"] != 0.0 {
		t.Fatalf("new user's quality group: %v", created)
	}
	path := "/users/" + created["id"].(string)
	id, _ := accounts.ParseID(created["id"].(string))

	status, updated, _ := administrator.call(http.MethodPatch, path, map[string]any{"qualityGroup": 1080})
	if status != http.StatusOK || updated["qualityGroup"] != 1080.0 {
		t.Fatalf("setting the group: %d %v", status, updated)
	}
	if stored, err := api.store.User(t.Context(), id); err != nil || stored.QualityGroup != 1080 {
		t.Errorf("stored: %+v %v", stored, err)
	}
	// A change that leaves it out keeps it.
	if status, updated, _ = administrator.call(http.MethodPatch, path, map[string]any{"maxBitrate": 4_000_000}); status != http.StatusOK || updated["qualityGroup"] != 1080.0 {
		t.Errorf("a change without it: %d %v", status, updated)
	}
	for _, refused := range []any{-1, 1, 1000, 4320, "1080p"} {
		status, body, _ := administrator.call(http.MethodPatch, path, map[string]any{"qualityGroup": refused})
		if refused == "1080p" {
			// Not a number: the body itself is refused.
			if status != http.StatusBadRequest {
				t.Errorf("%v: %d %v", refused, status, body)
			}
			continue
		}
		if status != http.StatusBadRequest || body["error"] != "invalid_quality_group" {
			t.Errorf("%v: %d %v", refused, status, body)
		}
	}
	if user, _ := api.store.User(t.Context(), id); user.QualityGroup != 1080 {
		t.Errorf("stored after refused changes: %+v", user)
	}
	// Every group, and Original again.
	for _, group := range []int{2160, 1440, 720, 480, 0} {
		if status, updated, _ := administrator.call(http.MethodPatch, path, map[string]any{"qualityGroup": group}); status != http.StatusOK || updated["qualityGroup"] != float64(group) {
			t.Errorf("group %d: %d %v", group, status, updated)
		}
	}
	if status, _, _ := member.call(http.MethodPatch, path, map[string]any{"qualityGroup": 720}); status != http.StatusForbidden {
		t.Errorf("member changing the group: %d", status)
	}
}
