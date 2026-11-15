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
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	_, created, _ := administrator.call(http.MethodPost, "/users", map[string]any{"name": "child", "password": "correct horse"})
	if created["videoTranscoding"] != true || created["audioTranscoding"] != true || created["downloads"] != true {
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

	// Each kind of conversion is saved on its own.
	status, updated, _ := administrator.call(http.MethodPatch, path, map[string]any{"videoTranscoding": false})
	if status != http.StatusOK || updated["videoTranscoding"] != false || updated["audioTranscoding"] != true || updated["downloads"] != true {
		t.Fatalf("turning video conversion off: %d %v", status, updated)
	}
	if user := stored(); user.VideoTranscoding || !user.AudioTranscoding || !user.ContentDownloading {
		t.Errorf("stored after turning video conversion off: %+v", user)
	}
	// Another change keeps the split.
	status, updated, _ = administrator.call(http.MethodPatch, path, map[string]any{"downloads": false, "isHidden": true})
	if status != http.StatusOK || updated["videoTranscoding"] != false || updated["audioTranscoding"] != true || updated["downloads"] != false {
		t.Fatalf("turning downloads off: %d %v", status, updated)
	}
	if user := stored(); user.VideoTranscoding || !user.AudioTranscoding {
		t.Errorf("stored after another change: %+v", user)
	}
	status, updated, _ = administrator.call(http.MethodPatch, path, map[string]any{"videoTranscoding": true, "audioTranscoding": false})
	if user := stored(); status != http.StatusOK || updated["videoTranscoding"] != true || updated["audioTranscoding"] != false ||
		!user.VideoTranscoding || user.AudioTranscoding || user.ContentDownloading {
		t.Errorf("turning video on and audio off: %d %v, stored %+v", status, updated, user)
	}
	// A split set elsewhere, as from a Jellyfin app's policy, shows as it is.
	if _, err := api.store.UpdateUser(t.Context(), id, accounts.UserChanges{VideoTranscoding: new(false), AudioTranscoding: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	if status, user, _ := administrator.call(http.MethodPatch, path, map[string]any{}); status != http.StatusOK ||
		user["videoTranscoding"] != false || user["audioTranscoding"] != true {
		t.Errorf("a split set elsewhere: %d %v", status, user)
	}
	if status, _, _ := member.call(http.MethodPatch, path, map[string]any{"downloads": true}); status != http.StatusForbidden {
		t.Errorf("member changing permissions: %d", status)
	}
}

// A group's video and audio conversion are saved apart, kept through
// another change, and its followers take both.
func TestGroupsSetVideoAndAudioConversionApart(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	admin := api.signedIn("admin", true)
	group := createdID(t, admin, "/groups", map[string]any{"name": "Kids"})
	path := "/groups/" + group
	status, kids, _ := admin.call(http.MethodPatch, path, map[string]any{"videoTranscoding": false})
	if status != http.StatusOK || kids["videoTranscoding"] != false || kids["audioTranscoding"] != true {
		t.Fatalf("turning the group's video conversion off: %d %v", status, kids)
	}
	status, kids, _ = admin.call(http.MethodPatch, path, map[string]any{"downloads": false})
	if status != http.StatusOK || kids["videoTranscoding"] != false || kids["audioTranscoding"] != true || kids["downloads"] != false {
		t.Fatalf("another change to the group: %d %v", status, kids)
	}
	ann := createdID(t, admin, "/users", map[string]any{"name": "ann", "password": "correct horse", "groupId": group})
	annID, _ := accounts.ParseID(ann)
	follower := func(video, audio bool) {
		t.Helper()
		user := api.user(t, admin, ann)
		stored, err := api.store.User(t.Context(), annID)
		if err != nil || user["videoTranscoding"] != video || user["audioTranscoding"] != audio ||
			stored.VideoTranscoding != video || stored.AudioTranscoding != audio {
			t.Errorf("the follower: %v, stored %+v %v; want video %v, audio %v", user, stored, err, video, audio)
		}
	}
	follower(false, true)
	if status, kids, _ = admin.call(http.MethodPatch, path, map[string]any{"videoTranscoding": true, "audioTranscoding": false}); status != http.StatusOK {
		t.Fatalf("swapping the group's conversion: %d %v", status, kids)
	}
	follower(true, false)
}

func TestSettingsSwitchConversion(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
