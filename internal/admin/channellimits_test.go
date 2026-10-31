package admin

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// Users and groups choose the Live TV channels of the server's line-ups
// their members see: by group and by channel, from the choices the admin
// app lists, and, while hiding the groups added later, with the groups
// known when the rule was saved.
func TestLiveTvChannelRulesInUsersAndGroups(t *testing.T) {
	t.Parallel()
	var options *Options
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) { options = o })
	lib := options.ChannelChoices.(*library.Service)
	provider := newProvider(t)
	admin := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	status, source, _ := admin.call(http.MethodPost, "/scopes/shared/iptv", map[string]any{"name": "Playlist", "kind": "m3u",
		"url": provider + "/get.php?username=user&password=secret-pw"})
	if status != http.StatusCreated {
		t.Fatalf("adding a playlist: %d %v", status, source)
	}

	choices := func(query string) (groups []map[string]any, channels []map[string]any, total float64) {
		t.Helper()
		status, body, _ := admin.call(http.MethodGet, "/live-tv/channel-choices?"+query, nil)
		if status != http.StatusOK {
			t.Fatalf("channel choices %q: %d %v", query, status, body)
		}
		for _, g := range body["groups"].([]any) {
			groups = append(groups, g.(map[string]any))
		}
		for _, c := range body["channels"].([]any) {
			channels = append(channels, c.(map[string]any))
		}
		return groups, channels, body["total"].(float64)
	}
	groups, channels, total := choices("")
	if len(groups) != 2 || groups[0]["name"] != "News" || groups[1]["name"] != "Kids" || groups[0]["source"] != "Playlist" ||
		groups[0]["channels"] != 1.0 || total != 2 || len(channels) != 2 || channels[0]["name"] != "Zeb One" ||
		channels[0]["group"] != groups[0]["key"] || channels[1]["number"] != "2" {
		t.Fatalf("choices: %v %v %v", groups, channels, total)
	}
	news, kids := groups[0]["key"].(string), groups[1]["key"].(string)
	zeb, orbe := channels[0]["key"].(string), channels[1]["key"].(string)
	for query, want := range map[string][]string{
		"q=ORBE":                         {"Orbe Junior"},
		"group=" + url.QueryEscape(news): {"Zeb One"},
		"key=" + url.QueryEscape(orbe):   {"Orbe Junior"},
		"limit=1&offset=1":               {"Orbe Junior"},
	} {
		var names []string
		_, found, total := choices(query)
		for _, c := range found {
			names = append(names, c["name"].(string))
		}
		if !slices.Equal(names, want) || query == "limit=1&offset=1" && total != 2 {
			t.Errorf("choices %q: %v of %v", query, names, total)
		}
	}
	if status, _, _ := member.call(http.MethodGet, "/live-tv/channel-choices", nil); status != http.StatusForbidden {
		t.Errorf("choices asked by a member: %d", status)
	}

	status, kid, _ := admin.call(http.MethodPost, "/users", map[string]any{"name": "kid", "password": "correct horse"})
	if rule, _ := kid["liveTvChannels"].(map[string]any); status != http.StatusCreated || rule["mode"] != "all" {
		t.Fatalf("a new user: %d %v", status, kid)
	}
	id := kid["id"].(string)
	userID, _ := accounts.ParseID(id)
	seen := func() []string {
		t.Helper()
		user, err := api.store.User(t.Context(), userID)
		if err != nil {
			t.Fatal(err)
		}
		items, err := lib.Channels(t.Context(), user)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, item := range items {
			names = append(names, item.Name+" "+item.Number)
		}
		return names
	}

	// Every channel but the news, and none of the groups added later.
	rule := map[string]any{"mode": "except", "groups": []string{news}, "hideNewGroups": true}
	status, kid, _ = admin.call(http.MethodPatch, "/users/"+id, map[string]any{"liveTvChannels": rule})
	saved, _ := kid["liveTvChannels"].(map[string]any)
	if status != http.StatusOK || saved["mode"] != "except" || fmt.Sprint(saved["groups"]) != fmt.Sprint([]string{news}) ||
		fmt.Sprint(saved["knownGroups"]) != fmt.Sprint([]string{news, kids}) {
		t.Fatalf("the rule saved: %d %v", status, kid)
	}
	if got := seen(); !slices.Equal(got, []string{"Orbe Junior 2"}) {
		t.Errorf("channels seen without the news: %v", got)
	}
	// A category the administrator adds is a group added later: its
	// channels are hidden, until the rule is saved again.
	base := "/scopes/shared/iptv/" + source["id"].(string)
	status, favourites, _ := admin.call(http.MethodPost, base+"/categories", map[string]any{"name": "Favourites"})
	if status != http.StatusCreated {
		t.Fatalf("a category: %d %v", status, favourites)
	}
	_, page, _ := admin.call(http.MethodGet, base+"/channels?q=orbe", nil)
	channel := page["items"].([]any)[0].(map[string]any)["id"].(string)
	if status, body, _ := admin.call(http.MethodPatch, base+"/channels/"+channel, map[string]any{"category": favourites["id"]}); status != http.StatusOK {
		t.Fatalf("moving a channel: %d %v", status, body)
	}
	if got := seen(); len(got) != 0 {
		t.Errorf("channels of a group added later: %v", got)
	}
	if status, _, _ := admin.call(http.MethodPatch, "/users/"+id, map[string]any{"liveTvChannels": rule}); status != http.StatusOK {
		t.Fatalf("the rule saved again: %d", status)
	}
	if got := seen(); !slices.Equal(got, []string{"Orbe Junior 2"}) {
		t.Errorf("channels once the rule was saved again: %v", got)
	}

	for _, invalid := range []map[string]any{{"mode": "some"}, {"mode": "only", "channels": []string{""}}} {
		if status, body, _ := admin.call(http.MethodPatch, "/users/"+id, map[string]any{"liveTvChannels": invalid}); status != http.StatusBadRequest ||
			body["error"] != "invalid_live_tv_channels" {
			t.Errorf("rule %v: %d %v", invalid, status, body)
		}
	}

	// A group holds the rule of the members who follow it.
	status, group, _ := admin.call(http.MethodPost, "/groups", map[string]any{"name": "Kids"})
	if status != http.StatusCreated {
		t.Fatalf("a group: %d %v", status, group)
	}
	path := "/groups/" + group["id"].(string)
	if status := admin.raw(http.MethodPost, path+"/members", `{"userIds":["`+id+`"]}`, nil); status != http.StatusOK {
		t.Fatalf("joining the group: %d", status)
	}
	status, group, _ = admin.call(http.MethodPatch, path, map[string]any{"liveTvChannels": map[string]any{"mode": "only", "channels": []string{zeb}}})
	if saved, _ := group["liveTvChannels"].(map[string]any); status != http.StatusOK || saved["mode"] != "only" {
		t.Fatalf("the group's rule: %d %v", status, group)
	}
	if got := seen(); !slices.Equal(got, []string{"Zeb One 1"}) {
		t.Errorf("channels of the group's member: %v", got)
	}
}
