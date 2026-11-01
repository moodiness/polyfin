package admin

import (
	"net/http"
	"testing"
)

// A user turns Boost the volume and Convert the sound on and off under My
// account; an administrator sets them on the user's page and the group's,
// and a member who follows their group keeps the group's.
func TestUsersChooseTheirSound(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	admin := api.signedIn("administrator", true)
	member := api.signedIn("member", false)

	if status, body, _ := member.call(http.MethodGet, "/account/sound", nil); status != http.StatusOK ||
		body["boostVolume"] != false || body["convertSound"] != false || body["followsGroup"] != false {
		t.Fatalf("a new user's: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPut, "/account/sound", map[string]any{"boostVolume": true}); status != http.StatusOK ||
		body["boostVolume"] != true || body["convertSound"] != false {
		t.Errorf("boosting: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPut, "/account/sound", map[string]any{"convertSound": true}); status != http.StatusOK ||
		body["boostVolume"] != true || body["convertSound"] != true {
		t.Errorf("converting: %d %v", status, body)
	}
	var id string
	for _, user := range api.users(t, admin) {
		if user["name"] == "member" {
			id = user["id"].(string)
			if user["boostVolume"] != true || user["convertSound"] != true {
				t.Errorf("the user's page: %v", user)
			}
		}
	}
	if status, body, _ := admin.call(http.MethodPatch, "/users/"+id, map[string]any{"boostVolume": false}); status != http.StatusOK ||
		body["boostVolume"] != false || body["convertSound"] != true {
		t.Errorf("the administrator's change: %d %v", status, body)
	}

	status, group, _ := admin.call(http.MethodPost, "/groups", map[string]any{"name": "Loud"})
	if status != http.StatusCreated || group["boostVolume"] != false || group["convertSound"] != false {
		t.Fatalf("a new group: %d %v", status, group)
	}
	if status, group, _ = admin.call(http.MethodPatch, "/groups/"+group["id"].(string), map[string]any{"boostVolume": true}); status != http.StatusOK ||
		group["boostVolume"] != true || group["convertSound"] != false {
		t.Fatalf("the group's: %d %v", status, group)
	}
	if status, body, _ := admin.call(http.MethodPatch, "/users/"+id, map[string]any{"groupId": group["id"]}); status != http.StatusOK ||
		body["boostVolume"] != true || body["convertSound"] != false || body["followGroup"] != true {
		t.Fatalf("joining the group: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodGet, "/account/sound", nil); status != http.StatusOK ||
		body["boostVolume"] != true || body["convertSound"] != false || body["followsGroup"] != true {
		t.Errorf("a following member's: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPut, "/account/sound", map[string]any{"convertSound": true}); status != http.StatusConflict ||
		body["error"] != "follows_group" {
		t.Errorf("a following member's change: %d %v", status, body)
	}
}
