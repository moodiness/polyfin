package admin

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
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

func TestSettingsSwitchConversion(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	_, settings, _ := administrator.call(http.MethodGet, "/settings", nil)
	if settings["transcoding"] != true {
		t.Fatalf("default settings: %v", settings)
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en"}
	put := func(changes map[string]any) map[string]any {
		t.Helper()
		body := maps.Clone(base)
		maps.Copy(body, changes)
		status, saved, _ := administrator.call(http.MethodPut, "/settings", body)
		if status != http.StatusOK {
			t.Fatalf("saving %v: %d %v", changes, status, saved)
		}
		return saved
	}
	if saved := put(map[string]any{"transcoding": false}); saved["transcoding"] != false {
		t.Errorf("turning conversion off: %v", saved)
	}
	if current := api.store.Settings(); current.Transcoding {
		t.Errorf("stored: %+v", current)
	}
	// An admin app that does not know it keeps it.
	if saved := put(map[string]any{"serverName": "Maison"}); saved["transcoding"] != false || saved["serverName"] != "Maison" {
		t.Errorf("saving without it: %v", saved)
	}
	if saved := put(map[string]any{"transcoding": true}); saved["transcoding"] != true {
		t.Errorf("turning conversion on: %v", saved)
	}
}

func TestAdministratorsTurnDownloadsOffForEveryone(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	_, child, _ := administrator.call(http.MethodPost, "/users", map[string]any{"name": "child", "password": "correct horse"})
	if status, _, _ := administrator.call(http.MethodPatch, "/users/"+child["id"].(string), map[string]any{"downloads": false}); status != http.StatusOK {
		t.Fatalf("child's downloads: %d", status)
	}
	if status, _, _ := member.call(http.MethodPost, "/users/downloads/off", nil); status != http.StatusForbidden {
		t.Errorf("a member turning downloads off: %d", status)
	}
	// turnOff answers with the names of the users who had the permission.
	turnOff := func() []string {
		t.Helper()
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, api.url+"/admin/api/users/downloads/off", nil)
		answer, err := administrator.client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer answer.Body.Close()
		var users []map[string]any
		if err := json.NewDecoder(answer.Body).Decode(&users); err != nil || answer.StatusCode != http.StatusOK {
			t.Fatalf("turning downloads off: %d %v", answer.StatusCode, err)
		}
		names := []string{}
		for _, user := range users {
			if user["downloads"] != false {
				t.Errorf("%v answered with downloads %v", user["name"], user["downloads"])
			}
			names = append(names, user["name"].(string))
		}
		slices.Sort(names)
		return names
	}
	if names := turnOff(); !slices.Equal(names, []string{"administrator", "member"}) {
		t.Errorf("turned off for %v", names)
	}
	users, err := api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range users {
		if user.ContentDownloading {
			t.Errorf("%s may still download", user.Name)
		}
	}
	if names := turnOff(); len(names) != 0 {
		t.Errorf("turned off again for %v", names)
	}
}
