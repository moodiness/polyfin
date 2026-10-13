package admin

import (
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The apps told collection libraries hold collections are none by default,
// saved by a PUT, kept by one leaving them out, and refused, with what was
// saved kept, when one is no app name.
func TestSettingsCollectionLibraryApps(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	apps := func(body map[string]any) []string {
		list, _ := body["collectionLibraryApps"].([]any)
		names := []string{}
		for _, name := range list {
			names = append(names, name.(string))
		}
		return names
	}
	_, body, _ := administrator.call(http.MethodGet, "/settings", nil)
	if list, ok := body["collectionLibraryApps"].([]any); !ok || len(list) != 0 {
		t.Errorf("default: %#v", body["collectionLibraryApps"])
	}
	bounds, _ := body["bounds"].(map[string]any)["collectionLibraryApps"].(map[string]any)
	if bounds["max"] != float64(accounts.MaxCollectionLibraryApps) || bounds["default"] == nil {
		t.Errorf("bounds: %v", bounds)
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	set := maps.Clone(base)
	set["collectionLibraryApps"] = []string{" Shelf TV ", "Example Player", "SHELF TV"}
	want := []string{"Shelf TV", "Example Player"}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", set); status != http.StatusOK || !slices.Equal(apps(body), want) {
		t.Fatalf("saving: %d %v", status, body)
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); !slices.Equal(apps(body), want) {
		t.Errorf("read: %v", body["collectionLibraryApps"])
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || !slices.Equal(apps(body), want) {
		t.Errorf("left out: %d %v", status, body["collectionLibraryApps"])
	}
	for _, refused := range [][]string{{""}, {"Shelf\nTV"}, {strings.Repeat("a", accounts.MaxCollectionLibraryAppLength+1)}} {
		body := maps.Clone(base)
		body["collectionLibraryApps"] = refused
		if status, answer, _ := administrator.call(http.MethodPut, "/settings", body); status != http.StatusBadRequest ||
			answer["error"] != "invalid_collection_library_apps" {
			t.Errorf("%q: %d %v", refused, status, answer)
		}
	}
	if got := api.store.Settings().CollectionLibraryApps; !slices.Equal(got, want) {
		t.Errorf("stored: %q", got)
	}
	set["collectionLibraryApps"] = []string{}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", set); status != http.StatusOK || len(apps(body)) != 0 {
		t.Errorf("emptied: %d %v", status, body["collectionLibraryApps"])
	}
}

// The apps to choose among are those every user is signed in with, each
// once whatever its letter case, by name; members may not list them.
func TestSettingsAppsListTheSignedInApps(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	listed := func() []string {
		t.Helper()
		status, body, _ := administrator.call(http.MethodGet, "/settings/apps", nil)
		list, ok := body["apps"].([]any)
		if status != http.StatusOK || !ok {
			t.Fatalf("apps: %d %v", status, body)
		}
		names := []string{}
		for _, name := range list {
			names = append(names, name.(string))
		}
		return names
	}
	if got := listed(); len(got) != 0 {
		t.Errorf("before any app signed in: %v", got)
	}
	all, err := api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	users := map[string]accounts.ID{}
	for _, user := range all {
		users[user.Name] = user.ID
	}
	for _, d := range []struct{ user, device, client string }{
		{"administrator", "tv", "Shelf TV"},
		{"administrator", "browser", "Jellyfin Web"},
		{"member", "tv", "shelf tv"},
		{"member", "phone", "example Player"},
		{"member", "tablet", ""},
	} {
		if _, _, err := api.store.SignInDevice(t.Context(), users[d.user], accounts.DeviceInfo{DeviceID: d.user + "-" + d.device,
			DeviceName: d.device, Client: d.client, ClientVersion: "1.0"}); err != nil {
			t.Fatal(err)
		}
	}
	got := listed()
	if len(got) != 3 || !strings.EqualFold(got[1], "Jellyfin Web") || got[0] != "example Player" || !strings.EqualFold(got[2], "Shelf TV") {
		t.Errorf("apps: %q", got)
	}
	if status, body, _ := member.call(http.MethodGet, "/settings/apps", nil); status != http.StatusForbidden || body["apps"] != nil {
		t.Errorf("a member: %d %v", status, body)
	}
}
