package admin

import (
	"net/http"
	"slices"
	"testing"
)

// createdID creates through the admin API, and returns the identifier of
// what was created.
func createdID(t *testing.T, b browser, path string, body map[string]any) string {
	t.Helper()
	status, created, _ := b.call(http.MethodPost, path, body)
	if status != http.StatusCreated {
		t.Fatalf("%s: %d %v", path, status, created)
	}
	return created["id"].(string)
}

func TestGroupsHoldTheLimitsOfTheirMembers(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	admin := api.signedIn("admin", true)
	adminID := api.users(t, admin)[0]["id"].(string)
	ann := createdID(t, admin, "/users", map[string]any{"name": "ann", "password": "correct horse"})
	if status, body, _ := admin.call(http.MethodPatch, "/users/"+ann, map[string]any{"maxBitrate": 8_000_000}); status != http.StatusOK {
		t.Fatalf("ann's bitrate: %d %v", status, body)
	}

	status, kids, _ := admin.call(http.MethodPost, "/groups", map[string]any{"name": " Kids "})
	if status != http.StatusCreated || kids["name"] != "Kids" || kids["memberCount"] != 0.0 || kids["downloads"] != true ||
		kids["maxPlaybacks"] != 0.0 || kids["syncPlay"] != "CreateAndJoinGroups" {
		t.Fatalf("creating a group: %d %v", status, kids)
	}
	group := kids["id"].(string)
	for _, tc := range []struct {
		body   map[string]any
		status int
		code   string
	}{
		{map[string]any{"name": "KIDS"}, http.StatusConflict, "name_taken"},
		{map[string]any{"name": " "}, http.StatusBadRequest, "invalid_name"},
		{map[string]any{"name": "Copy", "copyFrom": "nope"}, http.StatusBadRequest, "invalid_copy_from"},
		{map[string]any{"name": "Copy", "copyFrom": "0123456789abcdef0123456789abcdef"}, http.StatusBadRequest, "invalid_copy_from"},
	} {
		if status, body, _ := admin.call(http.MethodPost, "/groups", tc.body); status != tc.status || body["error"] != tc.code {
			t.Errorf("%v: %d %v", tc.body, status, body)
		}
	}
	if status, copied, _ := admin.call(http.MethodPost, "/groups", map[string]any{"name": "Copy", "copyFrom": ann}); status != http.StatusCreated ||
		copied["maxBitrate"] != 8e6 {
		t.Errorf("copying ann: %d %v", status, copied)
	}

	status, kids, _ = admin.call(http.MethodPatch, "/groups/"+group, map[string]any{"downloads": false, "maxPlaybacks": 2,
		"accessSchedules": []map[string]any{{"day": "Everyday", "startHour": 7, "endHour": 21}}})
	if status != http.StatusOK || kids["downloads"] != false || kids["maxPlaybacks"] != 2.0 || len(kids["accessSchedules"].([]any)) != 1 {
		t.Fatalf("changing the group: %d %v", status, kids)
	}
	if status, body, _ := admin.call(http.MethodPatch, "/groups/"+group, map[string]any{"maxPlaybacks": 99}); status != http.StatusBadRequest || body["error"] != "invalid_max_playbacks" {
		t.Errorf("a limit out of range: %d %v", status, body)
	}

	// Creating a user in the group.
	status, cat, _ := admin.call(http.MethodPost, "/users", map[string]any{"name": "cat", "password": "correct horse", "groupId": group})
	if status != http.StatusCreated || cat["groupId"] != group || cat["groupName"] != "Kids" || cat["followGroup"] != true ||
		cat["downloads"] != false || cat["maxPlaybacks"] != 2.0 {
		t.Fatalf("creating cat in the group: %d %v", status, cat)
	}
	if status, body, _ := admin.call(http.MethodPost, "/users", map[string]any{"name": "boss", "password": "correct horse",
		"isAdministrator": true, "groupId": group}); status != http.StatusConflict || body["error"] != "admin_cannot_join_group" {
		t.Errorf("an administrator created in the group: %d %v", status, body)
	}

	// Adding members.
	var members []map[string]any
	if status := admin.raw(http.MethodPost, "/groups/"+group+"/members", `{"userIds":["`+adminID+`","`+ann+`"]}`, nil); status != http.StatusConflict {
		t.Errorf("adding an administrator: %d", status)
	}
	if status := admin.raw(http.MethodPost, "/groups/"+group+"/members", `{"userIds":["`+ann+`"]}`, &members); status != http.StatusOK ||
		len(members) != 2 || members[0]["name"] != "ann" || members[0]["followGroup"] != true || members[1]["name"] != "cat" {
		t.Fatalf("adding ann: %d %v", status, members)
	}
	listed := map[string]map[string]any{}
	for _, user := range api.users(t, admin) {
		listed[user["name"].(string)] = user
	}
	if listed["ann"]["groupName"] != "Kids" || listed["ann"]["maxBitrate"] != 0.0 || listed["admin"]["groupId"] != nil || listed["admin"]["followGroup"] != true {
		t.Errorf("users list: %v", listed)
	}

	// Ann follows the group: her own page cannot change its settings, until
	// she stops following it.
	if status, body, _ := admin.call(http.MethodPatch, "/users/"+ann, map[string]any{"downloads": true}); status != http.StatusConflict || body["error"] != "follows_group" {
		t.Errorf("changing a followed setting: %d %v", status, body)
	}
	status, body, _ := admin.call(http.MethodPatch, "/users/"+ann, map[string]any{"followGroup": false, "downloads": true})
	if status != http.StatusOK || body["followGroup"] != false || body["downloads"] != true || body["groupId"] != group || body["maxPlaybacks"] != 2.0 {
		t.Fatalf("detaching ann: %d %v", status, body)
	}
	if status, _, _ := admin.call(http.MethodPatch, "/groups/"+group, map[string]any{"maxPlaybacks": 3}); status != http.StatusOK {
		t.Fatal(status)
	}
	if user := api.user(t, admin, ann); user["maxPlaybacks"] != 2.0 {
		t.Errorf("detached ann followed a change: %v", user)
	}
	if user := api.user(t, admin, cat["id"].(string)); user["maxPlaybacks"] != 3.0 {
		t.Errorf("cat missed a change: %v", user)
	}
	status, body, _ = admin.call(http.MethodPatch, "/users/"+ann, map[string]any{"followGroup": true})
	if status != http.StatusOK || body["followGroup"] != true || body["downloads"] != false || body["maxPlaybacks"] != 3.0 {
		t.Errorf("following again: %d %v", status, body)
	}

	for _, tc := range []struct {
		user   string
		body   map[string]any
		status int
		code   string
	}{
		{adminID, map[string]any{"groupId": group}, http.StatusConflict, "admin_cannot_join_group"},
		{ann, map[string]any{"groupId": "nope"}, http.StatusBadRequest, "invalid_group"},
		{ann, map[string]any{"groupId": "0123456789abcdef0123456789abcdef"}, http.StatusBadRequest, "invalid_group"},
	} {
		if status, body, _ := admin.call(http.MethodPatch, "/users/"+tc.user, tc.body); status != tc.status || body["error"] != tc.code {
			t.Errorf("%v: %d %v", tc.body, status, body)
		}
	}
	// Made an administrator, cat leaves the group; ann leaves it by hand.
	if status, body, _ := admin.call(http.MethodPatch, "/users/"+cat["id"].(string), map[string]any{"isAdministrator": true}); status != http.StatusOK ||
		body["groupId"] != nil || body["maxPlaybacks"] != 3.0 {
		t.Errorf("cat made an administrator: %d %v", status, body)
	}
	if status, body, _ := admin.call(http.MethodPatch, "/users/"+ann, map[string]any{"groupId": ""}); status != http.StatusOK ||
		body["groupId"] != nil || body["groupName"] != nil || body["maxPlaybacks"] != 3.0 {
		t.Errorf("ann leaving: %d %v", status, body)
	}

	// An invite link naming the group puts its accounts in it.
	_, token := createInvite(t, admin, map[string]any{"groupId": group})
	if invites := api.invites(t, admin); invites[0]["group"].(map[string]any)["name"] != "Kids" {
		t.Errorf("invites list: %v", invites)
	}
	if status, body, _ := api.browser().call(http.MethodPost, "/invite/"+token, map[string]any{"name": "guest", "password": "battery staple"}); status != http.StatusCreated {
		t.Fatalf("joining: %d %v", status, body)
	}
	if status := admin.raw(http.MethodGet, "/groups/"+group+"/members", "", &members); status != http.StatusOK || len(members) != 1 || members[0]["name"] != "guest" {
		t.Errorf("members after the invite: %d %v", status, members)
	}
	if status, body, _ := admin.call(http.MethodPost, "/invites", map[string]any{"groupId": "0123456789abcdef0123456789abcdef"}); status != http.StatusBadRequest || body["error"] != "invalid_group" {
		t.Errorf("an invite naming no group: %d %v", status, body)
	}

	var groups []map[string]any
	if status := admin.raw(http.MethodGet, "/groups", "", &groups); status != http.StatusOK || len(groups) != 2 || groups[1]["name"] != "Kids" || groups[1]["memberCount"] != 1.0 {
		t.Errorf("groups: %d %v", status, groups)
	}
	if status, _, _ := admin.call(http.MethodDelete, "/groups/"+group, nil); status != http.StatusNoContent {
		t.Fatalf("deleting: %d", status)
	}
	if status, body, _ := admin.call(http.MethodGet, "/groups/"+group, nil); status != http.StatusNotFound || body["error"] != "not_found" {
		t.Errorf("deleted group: %d %v", status, body)
	}

	var page struct {
		Items []activityEntryJSON `json:"items"`
	}
	admin.raw(http.MethodGet, "/activity?limit=100&type=GroupCreated,GroupChanged,GroupDeleted,GroupMemberAdded,GroupMemberRemoved,GroupMemberDetached", "", &page)
	var names []string
	for _, entry := range slices.Backward(page.Items) {
		names = append(names, entry.Name)
	}
	want := []string{
		"Group Kids was created",
		"Group Copy was created",
		"Group Kids was changed: Access, Playback and access, Allowed hours",
		"cat joined the group Kids",
		"ann joined the group Kids",
		"ann no longer follows the group Kids",
		"Group Kids was changed: Playback and access",
		"cat left the group Kids",
		"ann left the group Kids",
		"guest joined the group Kids",
		"Group Kids was deleted",
	}
	if !slices.Equal(names, want) {
		t.Errorf("activity:\n%q\nwant\n%q", names, want)
	}
}

// users lists the users as the admin app does.
func (api testAPI) users(t *testing.T, administrator browser) []map[string]any {
	t.Helper()
	var users []map[string]any
	if status := administrator.raw(http.MethodGet, "/users", "", &users); status != http.StatusOK {
		t.Fatalf("users: %d", status)
	}
	return users
}

// user is one user of the users list.
func (api testAPI) user(t *testing.T, administrator browser, id string) map[string]any {
	t.Helper()
	for _, user := range api.users(t, administrator) {
		if user["id"] == id {
			return user
		}
	}
	t.Fatalf("no user %s", id)
	return nil
}
