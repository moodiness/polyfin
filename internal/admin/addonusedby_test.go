package admin

import (
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// usedByOf reads the Used by setting an addon's answer gives: its mode,
// then its groups and users.
func usedByOf(t *testing.T, addon map[string]any) (string, []string, []string) {
	t.Helper()
	usedBy, ok := addon["usedBy"].(map[string]any)
	if !ok {
		t.Fatalf("no usedBy in %v", addon)
	}
	list := func(name string) []string {
		result := []string{}
		for _, id := range usedBy[name].([]any) {
			result = append(result, id.(string))
		}
		return result
	}
	return usedBy["mode"].(string), list("groups"), list("users")
}

// A server addon is used by everyone until an administrator lists the
// groups and users who use it, on adding it or later; the change is in
// the activity log, a group or user deleted leaves the list, and a user's
// own addon takes no such setting.
func TestServerAddonsAreUsedByTheGroupsAndUsersListed(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	admin := api.signedIn("alex", true)
	member := api.signedIn("member", false)
	sam, err := api.store.CreateUser(t.Context(), accounts.NewUser{Name: "sam", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	family, err := api.store.CreateGroup(t.Context(), "Family", nil)
	if err != nil {
		t.Fatal(err)
	}

	status, addon, _ := admin.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": localAddon(t)})
	if mode, groups, users := usedByOf(t, addon); status != http.StatusCreated || mode != "everyone" || len(groups)+len(users) != 0 {
		t.Fatalf("installing: %d %v", status, addon)
	}
	path := "/scopes/shared/addons/" + addon["id"].(string)
	missing := accounts.ID{1}.String()
	for _, bad := range []map[string]any{
		{"mode": "some"},
		{"mode": "listed", "groups": []string{"not an id"}},
		{"mode": "listed", "groups": []string{missing}},
		{"mode": "listed", "users": []string{missing}},
	} {
		if status, body, _ := admin.call(http.MethodPatch, path, map[string]any{"usedBy": bad}); status != http.StatusBadRequest || body["error"] != "invalid_used_by" {
			t.Errorf("Used by %v: %d %v", bad, status, body)
		}
	}
	limit := map[string]any{"usedBy": map[string]any{"mode": "listed", "groups": []string{family.ID.String()}, "users": []string{sam.ID.String()}}}
	if status, body, _ := member.call(http.MethodPatch, path, limit); status != http.StatusForbidden {
		t.Errorf("a member limiting a server addon: %d %v", status, body)
	}
	status, body, _ := admin.call(http.MethodPatch, path, limit)
	if mode, groups, users := usedByOf(t, body); status != http.StatusOK || mode != "listed" ||
		!slices.Equal(groups, []string{family.ID.String()}) || !slices.Equal(users, []string{sam.ID.String()}) {
		t.Fatalf("limiting: %d %v", status, body)
	}
	var listed []map[string]any
	admin.raw(http.MethodGet, "/scopes/shared/addons", "", &listed)
	if len(listed) != 1 {
		t.Fatalf("addons: %v", listed)
	}
	if mode, _, users := usedByOf(t, listed[0]); mode != "listed" || len(users) != 1 {
		t.Errorf("the list: %v", listed[0])
	}
	// The same setting again writes nothing more.
	admin.call(http.MethodPatch, path, limit)
	if names := activityNames(t, admin, "AddonUsedByChanged"); !slices.Equal(names, []string{"alex limited the addon Local to Family, sam"}) {
		t.Errorf("activity after limiting: %q", names)
	}

	// Deleted, the group and the user leave the list, used by no one.
	if _, err := api.store.DeleteGroup(t.Context(), family.ID); err != nil {
		t.Fatal(err)
	}
	if err := api.store.DeleteUser(t.Context(), sam.ID); err != nil {
		t.Fatal(err)
	}
	admin.raw(http.MethodGet, "/scopes/shared/addons", "", &listed)
	if mode, groups, users := usedByOf(t, listed[0]); mode != "listed" || len(groups)+len(users) != 0 {
		t.Errorf("after the deletions: %v", listed[0])
	}

	status, body, _ = admin.call(http.MethodPatch, path, map[string]any{"usedBy": map[string]any{"mode": "everyone", "users": []string{missing}}})
	if mode, _, users := usedByOf(t, body); status != http.StatusOK || mode != "everyone" || len(users) != 0 {
		t.Errorf("everyone again: %d %v", status, body)
	}
	if names := activityNames(t, admin, "AddonUsedByChanged"); len(names) != 2 || names[0] != "alex made the addon Local used by everyone" {
		t.Errorf("activity after everyone again: %q", names)
	}

	// On adding: a source used by one user alone from the start.
	administrator, err := api.store.CreateUser(t.Context(), accounts.NewUser{Name: "kim", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	other := strings.Replace(localAddon(t), "secret-token", "other", 1)
	status, added, _ := admin.call(http.MethodPost, "/scopes/shared/addons", map[string]any{"manifestUrl": other,
		"usedBy": map[string]any{"mode": "listed", "users": []string{administrator.ID.String()}}})
	if mode, _, users := usedByOf(t, added); status != http.StatusCreated || mode != "listed" || !slices.Equal(users, []string{administrator.ID.String()}) {
		t.Errorf("adding a limited addon: %d %v", status, added)
	}
	if status, body, _ := admin.call(http.MethodPost, "/scopes/shared/addons", map[string]any{"manifestUrl": localAddon(t),
		"usedBy": map[string]any{"mode": "listed", "groups": []string{missing}}}); status != http.StatusBadRequest || body["error"] != "invalid_used_by" {
		t.Errorf("adding an addon for a missing group: %d %v", status, body)
	}
	if names := activityNames(t, admin, "AddonInstalled"); len(names) != 2 {
		t.Errorf("addons installed: %q", names)
	}

	// A user's own addon is theirs alone.
	status, own, _ := admin.call(http.MethodPost, "/scopes/me/addons", map[string]string{"manifestUrl": localAddon(t)})
	if status != http.StatusCreated {
		t.Fatalf("installing an own addon: %d %v", status, own)
	}
	if status, body, _ := admin.call(http.MethodPatch, "/scopes/me/addons/"+own["id"].(string), limit); status != http.StatusBadRequest || body["error"] != "invalid_used_by" {
		t.Errorf("limiting an own addon: %d %v", status, body)
	}
}

// A group's page gives the group a server source, or takes it, one at a
// time: the group joins or leaves the source's Used by, the rest of it
// unchanged, so that changes made at once all stay. A source used by
// everyone is left as it is, members are refused, and only a change is
// in the activity log.
func TestAGroupIsGivenAServerSourceAlone(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	admin := api.signedIn("alex", true)
	member := api.signedIn("member", false)
	sam, err := api.store.CreateUser(t.Context(), accounts.NewUser{Name: "sam", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	groups := make([]accounts.Group, 6)
	for i := range groups {
		if groups[i], err = api.store.CreateGroup(t.Context(), "Group "+string(rune('A'+i)), nil); err != nil {
			t.Fatal(err)
		}
	}
	family := groups[0].ID.String()

	status, addon, _ := admin.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": localAddon(t)})
	if status != http.StatusCreated {
		t.Fatalf("installing: %d %v", status, addon)
	}
	id := addon["id"].(string)
	source := func(group string) string { return "/groups/" + group + "/sources/" + id }
	usedBy := func() (string, []string, []string) {
		t.Helper()
		var listed []map[string]any
		admin.raw(http.MethodGet, "/scopes/shared/addons", "", &listed)
		if len(listed) != 1 {
			t.Fatalf("addons: %v", listed)
		}
		return usedByOf(t, listed[0])
	}

	// Used by everyone, it lists no group to change.
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		if status, body, _ := admin.call(method, source(family), nil); status != http.StatusConflict || body["error"] != "used_by_everyone" {
			t.Errorf("%s on a source used by everyone: %d %v", method, status, body)
		}
	}
	if mode, groups, users := usedBy(); mode != "everyone" || len(groups)+len(users) != 0 {
		t.Errorf("the source used by everyone after the refusals: %s %v %v", mode, groups, users)
	}

	admin.call(http.MethodPatch, "/scopes/shared/addons/"+id, map[string]any{"usedBy": map[string]any{"mode": "listed", "users": []string{sam.ID.String()}}})
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		if status, body, _ := member.call(method, source(family), nil); status != http.StatusForbidden {
			t.Errorf("a member's %s: %d %v", method, status, body)
		}
	}
	missing := accounts.ID{1}.String()
	for _, path := range []string{source(missing), "/groups/" + family + "/sources/" + missing, "/groups/" + family + "/sources/not-an-id"} {
		if status, body, _ := admin.call(http.MethodPut, path, nil); status != http.StatusNotFound || body["error"] != "not_found" {
			t.Errorf("PUT %s: %d %v", path, status, body)
		}
	}

	status, body, _ := admin.call(http.MethodPut, source(family), nil)
	if mode, listed, users := usedByOf(t, body); status != http.StatusOK || mode != "listed" ||
		!slices.Equal(listed, []string{family}) || !slices.Equal(users, []string{sam.ID.String()}) {
		t.Fatalf("giving the source: %d %v", status, body)
	}
	// Given again, it is as it was, and the log has it once.
	if status, body, _ := admin.call(http.MethodPut, source(family), nil); status != http.StatusOK {
		t.Errorf("giving it again: %d %v", status, body)
	}
	if names := activityNames(t, admin, "AddonUsedByChanged"); len(names) != 2 || names[0] != "alex limited the addon Local to Group A, sam" {
		t.Errorf("activity after giving the source: %q", names)
	}

	// The others given at once, and the first taken, all stay.
	var wg sync.WaitGroup
	statuses := make([]int, len(groups))
	for i, group := range groups {
		method := http.MethodPut
		if i == 0 {
			method = http.MethodDelete
		}
		wg.Go(func() { statuses[i], _, _ = admin.call(method, source(group.ID.String()), nil) })
	}
	wg.Wait()
	want := []string{}
	for i, group := range groups {
		if statuses[i] != http.StatusOK {
			t.Errorf("the change for %s: %d", group.Name, statuses[i])
		}
		if i > 0 {
			want = append(want, group.ID.String())
		}
	}
	slices.Sort(want)
	if mode, listed, users := usedBy(); mode != "listed" || !slices.Equal(listed, want) || !slices.Equal(users, []string{sam.ID.String()}) {
		t.Errorf("after the changes made at once: %s %v %v, want %v", mode, listed, users, want)
	}

	// Taken, it is out of the list; taken again, nothing changes.
	for range 2 {
		status, body, _ = admin.call(http.MethodDelete, source(groups[1].ID.String()), nil)
		if _, listed, _ := usedByOf(t, body); status != http.StatusOK || slices.Contains(listed, groups[1].ID.String()) || len(listed) != len(want)-1 {
			t.Errorf("taking the source: %d %v", status, body)
		}
	}
	if names := activityNames(t, admin, "AddonUsedByChanged"); len(names) != 3+len(groups) {
		t.Errorf("activity after the changes: %d %q", len(names), names)
	}

	// A user's own addon is not the server's.
	status, own, _ := admin.call(http.MethodPost, "/scopes/me/addons", map[string]string{"manifestUrl": localAddon(t)})
	if status != http.StatusCreated {
		t.Fatalf("installing an own addon: %d %v", status, own)
	}
	if status, body, _ := admin.call(http.MethodPut, "/groups/"+family+"/sources/"+own["id"].(string), nil); status != http.StatusNotFound {
		t.Errorf("giving an own addon: %d %v", status, body)
	}
}
