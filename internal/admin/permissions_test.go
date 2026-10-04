package admin

import (
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

func TestAdministratorsSetConversionAndDownloadPermissions(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	_, created, _ := administrator.call(http.MethodPost, "/users", map[string]any{"name": "child", "password": "correct horse"})
	if created["transcoding"] != true || created["downloads"] != true {
		t.Fatalf("new user's permissions: %v", created)
	}
	path := "/users/" + created["id"].(string)
	id, _ := accounts.ParseID(created["id"].(string))
	stored := func() accounts.User {
		t.Helper()
		user, err := api.store.User(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		return user
	}

	status, updated, _ := administrator.call(http.MethodPatch, path, map[string]any{"transcoding": false})
	if status != http.StatusOK || updated["transcoding"] != false || updated["downloads"] != true {
		t.Fatalf("turning conversion off: %d %v", status, updated)
	}
	if user := stored(); user.VideoTranscoding || user.AudioTranscoding || !user.ContentDownloading {
		t.Errorf("stored after turning conversion off: %+v", user)
	}
	status, updated, _ = administrator.call(http.MethodPatch, path, map[string]any{"downloads": false, "isHidden": true})
	if status != http.StatusOK || updated["transcoding"] != false || updated["downloads"] != false {
		t.Fatalf("turning downloads off: %d %v", status, updated)
	}
	status, updated, _ = administrator.call(http.MethodPatch, path, map[string]any{"transcoding": true})
	if user := stored(); status != http.StatusOK || updated["transcoding"] != true || !user.VideoTranscoding || !user.AudioTranscoding || user.ContentDownloading {
		t.Errorf("turning conversion on: %d %v, stored %+v", status, updated, user)
	}
	// One kind taken away, as from a Jellyfin app's policy, is conversion
	// not allowed.
	video := false
	if _, err := api.store.UpdateUser(t.Context(), id, accounts.UserChanges{VideoTranscoding: &video}, nil); err != nil {
		t.Fatal(err)
	}
	if status, user, _ := administrator.call(http.MethodPatch, path, map[string]any{}); status != http.StatusOK || user["transcoding"] != false {
		t.Errorf("video conversion taken away: %d %v", status, user)
	}
	if status, _, _ := member.call(http.MethodPatch, path, map[string]any{"downloads": true}); status != http.StatusForbidden {
		t.Errorf("member changing permissions: %d", status)
	}
}

func TestSettingsSwitchConversionAndDownloads(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	_, settings, _ := administrator.call(http.MethodGet, "/settings", nil)
	if settings["transcoding"] != true || settings["downloads"] != true {
		t.Fatalf("default settings: %v", settings)
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en"}
	put := func(changes map[string]any) map[string]any {
		t.Helper()
		body := map[string]any{}
		for key, value := range base {
			body[key] = value
		}
		for key, value := range changes {
			body[key] = value
		}
		status, saved, _ := administrator.call(http.MethodPut, "/settings", body)
		if status != http.StatusOK {
			t.Fatalf("saving %v: %d %v", changes, status, saved)
		}
		return saved
	}
	if saved := put(map[string]any{"transcoding": false, "downloads": false}); saved["transcoding"] != false || saved["downloads"] != false {
		t.Errorf("turning both off: %v", saved)
	}
	if current := api.store.Settings(); current.Transcoding || current.Downloads {
		t.Errorf("stored: %+v", current)
	}
	// An admin app that does not know them keeps them.
	if saved := put(map[string]any{"serverName": "Maison"}); saved["transcoding"] != false || saved["downloads"] != false || saved["serverName"] != "Maison" {
		t.Errorf("saving without them: %v", saved)
	}
	if saved := put(map[string]any{"transcoding": true}); saved["transcoding"] != true || saved["downloads"] != false {
		t.Errorf("turning conversion on: %v", saved)
	}
	if current := api.store.Settings(); !current.Transcoding || current.Downloads {
		t.Errorf("stored: %+v", current)
	}
}
