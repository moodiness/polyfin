package admin

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// An administrator gives a server addon a name of its own, which its
// answer, its libraries and the activity log show; a refresh of its
// manifest keeps it, and an empty name, or the manifest's, gives it back
// the manifest's name, which it then follows.
func TestAnAdministratorRenamesAServerAddon(t *testing.T) {
	t.Parallel()
	var manifestName atomic.Pointer[string]
	setName := func(name string) { manifestName.Store(&name) }
	setName("Demo Catalog")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id": "demo", "name": "` + *manifestName.Load() + `", "version": "1.0.0", "resources": ["catalog"],
			"catalogs": [{"type": "movie", "id": "top", "name": "Top"}]}`))
	}))
	t.Cleanup(server.Close)
	api := newTestAPI(t, 10)
	admin := api.signedIn("alex", true)
	status, addon, _ := admin.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": server.URL + "/manifest.json"})
	if status != http.StatusCreated || addon["name"] != "Demo Catalog" || addon["chosenName"] != nil || addon["manifestName"] != "Demo Catalog" {
		t.Fatalf("installing: %d %v", status, addon)
	}
	path := "/scopes/shared/addons/" + addon["id"].(string)
	rename := func(name string) (int, map[string]any) {
		t.Helper()
		status, body, _ := admin.call(http.MethodPatch, path, map[string]any{"name": name})
		return status, body
	}

	if status, body := rename("  My Demo  "); status != http.StatusOK || body["name"] != "My Demo" || body["chosenName"] != "My Demo" ||
		body["manifestName"] != "Demo Catalog" {
		t.Fatalf("renaming: %d %v", status, body)
	}
	var libraries []map[string]any
	admin.raw(http.MethodGet, "/scopes/shared/libraries", "", &libraries)
	if len(libraries) == 0 || libraries[0]["addonName"] != "My Demo" {
		t.Errorf("libraries: %v", libraries)
	}
	if names := activityNames(t, admin, "AddonRenamed"); !slices.Equal(names, []string{"alex renamed the addon Demo Catalog to My Demo"}) {
		t.Errorf("activity: %q", names)
	}
	for _, bad := range []string{strings.Repeat("a", addons.MaxNameLength+1), "Tab\there"} {
		if status, body := rename(bad); status != http.StatusBadRequest || body["error"] != "invalid_addon_name" {
			t.Errorf("name %q: %d %v", bad, status, body)
		}
	}

	setName("Demo Catalog 2")
	status, body, _ := admin.call(http.MethodPost, path+"/refresh", nil)
	if status != http.StatusOK || body["name"] != "My Demo" || body["manifestName"] != "Demo Catalog 2" {
		t.Errorf("refreshed: %d %v", status, body)
	}

	if status, body := rename(" "); status != http.StatusOK || body["name"] != "Demo Catalog 2" || body["chosenName"] != nil {
		t.Errorf("emptied: %d %v", status, body)
	}
	if status, body := rename("Demo Catalog 2"); status != http.StatusOK || body["chosenName"] != nil {
		t.Errorf("the manifest's name: %d %v", status, body)
	}
	setName("Demo Catalog 3")
	if status, body, _ := admin.call(http.MethodPost, path+"/refresh", nil); status != http.StatusOK || body["name"] != "Demo Catalog 3" {
		t.Errorf("following the manifest again: %d %v", status, body)
	}
	if names := activityNames(t, admin, "AddonRenamed"); len(names) != 2 || names[0] != "alex renamed the addon My Demo to Demo Catalog 2" {
		t.Errorf("activity after emptying: %q", names)
	}
}

// A user renames their own addons, even while they may not use them, and
// no one else's: not the server's, not another user's. An administrator
// sees a user's addon but does not rename it.
func TestAUserRenamesTheirOwnAddons(t *testing.T) {
	t.Parallel()
	var store *addons.Store
	api := newTestAPI(t, 10, func(_ *Options, deps testDeps) { store = deps.addons })
	admin := api.signedIn("alex", true)
	member := api.signedIn("member", false)
	users, err := api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var memberUser accounts.User
	for _, u := range users {
		if u.Name == "member" {
			memberUser = u
		}
	}
	// Installed as an administrator would reach it: the test's address is
	// on the local network.
	own, err := store.Install(t.Context(), addons.Personal(memberUser.ID), localAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	_, shared, _ := admin.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": localAddon(t)})
	_, adminOwn, _ := admin.call(http.MethodPost, "/scopes/me/addons", map[string]string{"manifestUrl": localAddon(t)})
	rename := map[string]any{"name": "Mine"}

	if _, err := api.store.UpdateUser(t.Context(), memberUser.ID, accounts.UserChanges{PersonalAddons: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	if status, body, _ := member.call(http.MethodPatch, "/scopes/me/addons/"+own.ID.String(), rename); status != http.StatusOK || body["name"] != "Mine" {
		t.Errorf("the member's own addon: %d %v", status, body)
	}
	if names := activityNames(t, admin, "AddonRenamed"); !slices.Equal(names, []string{"member renamed the addon Local to Mine"}) {
		t.Errorf("activity: %q", names)
	}
	if status, body, _ := member.call(http.MethodPatch, "/scopes/shared/addons/"+shared["id"].(string), rename); status != http.StatusForbidden {
		t.Errorf("a server addon: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPatch, "/scopes/me/addons/"+adminOwn["id"].(string), rename); status != http.StatusNotFound {
		t.Errorf("another user's addon: %d %v", status, body)
	}
	for _, scope := range []string{"me", "shared"} {
		if status, body, _ := admin.call(http.MethodPatch, "/scopes/"+scope+"/addons/"+own.ID.String(), map[string]any{"name": "Theirs"}); status != http.StatusNotFound {
			t.Errorf("an administrator renaming a user's addon in %s: %d %v", scope, status, body)
		}
	}
	if renamed, err := store.Find(t.Context(), own.ID); err != nil || renamed.Name() != "Mine" {
		t.Errorf("the member's addon: %v %v", renamed.Name(), err)
	}
}

// Polyfin's own sources keep the name their own settings give: an addon's
// name of its own is refused for them.
func TestAnIPTVSourceIsNotRenamedAsAnAddon(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	provider := newProvider(t)
	admin := api.signedIn("alex", true)
	status, source, _ := admin.call(http.MethodPost, "/scopes/shared/iptv", map[string]any{"name": "Playlist", "kind": "m3u",
		"url": provider + "/get.php?username=user&password=secret-pw"})
	if status != http.StatusCreated {
		t.Fatalf("adding a playlist: %d %v", status, source)
	}
	status, body, _ := admin.call(http.MethodPatch, "/scopes/shared/addons/"+source["id"].(string), map[string]any{"name": "Other"})
	if status != http.StatusBadRequest || body["error"] != "addon_not_renamable" {
		t.Errorf("renaming a playlist: %d %v", status, body)
	}
	var listed []map[string]any
	admin.raw(http.MethodGet, "/scopes/shared/addons", "", &listed)
	if len(listed) != 1 || listed[0]["name"] != "Playlist" || listed[0]["chosenName"] != nil {
		t.Errorf("the playlist: %v", listed)
	}
}
