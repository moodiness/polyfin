package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// Configure on an addon's page sends whoever may replace its address to
// its configure page, whose address is in no other answer: the server's
// addons for administrators, a user's own while they may have some.
func TestConfigureOpensTheConfigurePageForWhoeverMayReplaceTheAddress(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hints := ""
		if strings.HasPrefix(r.URL.Path, "/secret/") {
			hints = `, "behaviorHints": {"configurable": true}`
		}
		_, _ = w.Write([]byte(`{"id": "top` + r.URL.Path + `", "name": "Top", "version": "1.0.0", "resources": ["stream"]` + hints + `}`))
	}))
	t.Cleanup(server.Close)
	var store *addons.Store
	api := newTestAPI(t, 10, func(_ *Options, deps testDeps) { store = deps.addons })
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	var memberUser accounts.User
	users, err := api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		if u.Name == "member" {
			memberUser = u
		}
	}
	install := func(path string) string {
		t.Helper()
		status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": server.URL + path})
		if status != http.StatusCreated {
			t.Fatalf("installing %s: %d %v", path, status, body)
		}
		return body["id"].(string)
	}
	configurable := install("/secret/manifest.json?token=k")
	plain := install("/plain/manifest.json")
	// The member's own addon, installed as an administrator would reach it.
	own, err := store.Install(t.Context(), addons.Personal(memberUser.ID), server.URL+"/secret/mine/manifest.json?token=m", false)
	if err != nil {
		t.Fatal(err)
	}
	mine := own.ID.String()

	// The flag is in the list, the address is not.
	flags := func(b browser, scope string) map[string]bool {
		t.Helper()
		response, data := b.fetch(http.MethodGet, "/scopes/"+scope+"/addons", "", nil)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("listing %s: %d", scope, response.StatusCode)
		}
		if strings.Contains(string(data), "token=") || strings.Contains(string(data), "/configure") {
			t.Errorf("the %s list holds an address: %s", scope, data)
		}
		var listed []struct {
			ID           string `json:"id"`
			Configurable bool   `json:"configurable"`
		}
		if err := json.Unmarshal(data, &listed); err != nil {
			t.Fatal(err)
		}
		result := map[string]bool{}
		for _, l := range listed {
			result[l.ID] = l.Configurable
		}
		return result
	}
	if got := flags(administrator, "shared"); !got[configurable] || got[plain] {
		t.Errorf("server addons' flags: %v", got)
	}
	if got := flags(member, "me"); !got[mine] {
		t.Errorf("the member's own addon's flag: %v", got)
	}

	configure := func(b browser, scope, id string) (int, string, string) {
		t.Helper()
		client := *b.client
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, api.url+"/admin/api/scopes/"+scope+"/addons/"+id+"/configure", nil)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		if response.StatusCode == http.StatusSeeOther && response.Header.Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s/%s: Referrer-Policy %q", scope, id, response.Header.Get("Referrer-Policy"))
		}
		return response.StatusCode, response.Header.Get("Location"), string(raw)
	}

	// An administrator, for a server addon.
	if status, location, _ := configure(administrator, "shared", configurable); status != http.StatusSeeOther ||
		location != server.URL+"/secret/configure?token=k" {
		t.Errorf("an administrator on a server addon: %d %q", status, location)
	}
	if status, _, body := configure(administrator, "shared", plain); status != http.StatusNotFound || !strings.Contains(body, "not_configurable") {
		t.Errorf("an addon without a configure page: %d %s", status, body)
	}
	// A user, for their own addon.
	if status, location, _ := configure(member, "me", mine); status != http.StatusSeeOther ||
		location != server.URL+"/secret/mine/configure?token=m" {
		t.Errorf("a user on their own addon: %d %q", status, location)
	}
	// A user, for a server addon: refused, under either scope.
	if status, location, _ := configure(member, "shared", configurable); status != http.StatusForbidden || location != "" {
		t.Errorf("a user on a server addon: %d %q", status, location)
	}
	if status, location, _ := configure(member, "me", configurable); status != http.StatusNotFound || location != "" {
		t.Errorf("a user on a server addon among their own: %d %q", status, location)
	}
	// Another user's own addon is not an administrator's.
	if status, location, _ := configure(administrator, "me", mine); status != http.StatusNotFound || location != "" {
		t.Errorf("an administrator on another user's addon: %d %q", status, location)
	}
	// A user who may not have their own addons may not configure them.
	if _, err := api.store.UpdateUser(t.Context(), memberUser.ID, accounts.UserChanges{PersonalAddons: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	if status, _, body := configure(member, "me", mine); status != http.StatusForbidden || !strings.Contains(body, "personal_addons_disabled") {
		t.Errorf("without their own addons: %d %s", status, body)
	}
}
