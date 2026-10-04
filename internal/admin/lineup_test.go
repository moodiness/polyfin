package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
)

// The line-up routes check what they are sent, answer the contract's
// codes and pages, and never show a custom stream's address in full.
func TestLineupRoutesValidate(t *testing.T) {
	api := newTestAPI(t, 10)
	provider := newProvider(t)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/iptv", map[string]any{"name": "Playlist", "kind": "m3u",
		"url": provider + "/get.php?username=user&password=secret-pw", "options": map[string]any{"numbering": "sequential"}})
	if status != http.StatusCreated || body["source"].(map[string]any)["options"].(map[string]any)["numbering"] != "sequential" {
		t.Fatalf("adding with options: %d %v", status, body)
	}
	base := "/scopes/shared/iptv/" + body["id"].(string)
	if status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/iptv", map[string]any{"name": "Other", "kind": "m3u",
		"url": provider + "/get.php?x=1", "options": map[string]any{"categories": "planet"}}); status != http.StatusBadRequest || body["error"] != "invalid_options" {
		t.Errorf("bad options: %d %v", status, body)
	}

	_, categories, _ := administrator.call(http.MethodGet, base+"/categories", nil)
	items := categories["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("categories: %v", categories)
	}
	news, kids := items[0].(map[string]any), items[1].(map[string]any)
	if news["key"] != "g:News" || news["position"] != 1.0 || news["channels"] != 1.0 || news["custom"] != false {
		t.Errorf("category JSON: %v", news)
	}
	status, page, _ := administrator.call(http.MethodGet, base+"/channels?limit=1&offset=1", nil)
	if status != http.StatusOK || page["total"] != 2.0 || page["offset"] != 1.0 || page["limit"] != 1.0 || len(page["items"].([]any)) != 1 {
		t.Fatalf("a page of channels: %d %v", status, page)
	}
	orbe := page["items"].([]any)[0].(map[string]any)
	if orbe["name"] != "Orbe Junior" || orbe["number"] != nil || orbe["category"].(map[string]any)["name"] != "Kids" || orbe["shown"] != true {
		t.Errorf("channel JSON: %v", orbe)
	}
	if _, page, _ := administrator.call(http.MethodGet, base+"/channels?limit=9000", nil); page["limit"] != 500.0 {
		t.Errorf("the largest page: %v", page["limit"])
	}
	_, page, _ = administrator.call(http.MethodGet, base+"/channels?q=zeb", nil)
	zeb := page["items"].([]any)[0].(map[string]any)
	channel := base + "/channels/" + zeb["id"].(string)

	for _, tc := range []struct {
		method, path string
		body         any
		status       int
		code         string
	}{
		{http.MethodGet, base + "/channels?limit=0", nil, http.StatusBadRequest, "invalid_request"},
		{http.MethodGet, base + "/channels?enabled=maybe", nil, http.StatusBadRequest, "invalid_request"},
		{http.MethodGet, base + "/preview?by=planet", nil, http.StatusBadRequest, "invalid_request"},
		{http.MethodPatch, base, map[string]any{"groups": []string{"News"}}, http.StatusBadRequest, "invalid_request"},
		{http.MethodPost, base + "/categories", map[string]any{"name": ""}, http.StatusBadRequest, "invalid_category_name"},
		{http.MethodDelete, base + "/categories/" + news["id"].(string), nil, http.StatusBadRequest, "category_not_custom"},
		{http.MethodPut, base + "/categories/order", map[string]any{"ids": []string{news["id"].(string)}}, http.StatusBadRequest, "invalid_order"},
		{http.MethodPatch, channel, map[string]any{"name": ""}, http.StatusBadRequest, "invalid_channel_name"},
		{http.MethodPatch, channel, map[string]any{"logo": "ftp://logos.example/a.png"}, http.StatusBadRequest, "invalid_logo"},
		{http.MethodPatch, channel, map[string]any{"description": strings.Repeat("a", 2001)}, http.StatusBadRequest, "invalid_description"},
		{http.MethodPatch, channel, map[string]any{"number": 0}, http.StatusBadRequest, "invalid_number"},
		{http.MethodPatch, channel, map[string]any{"number": 100000}, http.StatusBadRequest, "invalid_number"},
		{http.MethodPatch, channel, map[string]any{"category": "nope"}, http.StatusBadRequest, "invalid_category"},
		{http.MethodPatch, channel, map[string]any{"category": strings.Repeat("0", 32)}, http.StatusBadRequest, "invalid_category"},
		{http.MethodPost, channel + "/move", map[string]any{"before": orbe["id"]}, http.StatusBadRequest, "invalid_move"},
		{http.MethodPost, base + "/channels/bulk", map[string]any{"enabled": false}, http.StatusBadRequest, "invalid_bulk"},
		{http.MethodPost, base + "/channels/bulk", map[string]any{"enabled": false, "q": "z"}, http.StatusBadRequest, "invalid_bulk"},
		{http.MethodPut, channel + "/streams", map[string]any{"streams": []any{}}, http.StatusBadRequest, "invalid_streams"},
		{http.MethodPost, channel + "/streams", map[string]any{"url": "rtmp://cdn.example/a"}, http.StatusBadRequest, "invalid_stream_url"},
		{http.MethodDelete, channel + "/streams/" + zeb["streams"].([]any)[0].(map[string]any)["id"].(string), nil, http.StatusBadRequest, "stream_not_custom"},
		{http.MethodGet, "/scopes/shared/iptv/" + strings.Repeat("0", 32) + "/channels", nil, http.StatusNotFound, "not_found"},
		{http.MethodGet, base + "/channels/" + strings.Repeat("0", 32), nil, http.StatusNotFound, "not_found"},
	} {
		if status, body, _ := administrator.call(tc.method, tc.path, tc.body); status != tc.status || body["error"] != tc.code {
			t.Errorf("%s %s %v: %d %v, want %d %s", tc.method, tc.path, tc.body, status, body, tc.status, tc.code)
		}
	}
	if status, body, _ := member.call(http.MethodGet, base+"/channels", nil); status != http.StatusForbidden || body["error"] != "forbidden" {
		t.Errorf("a member reading the server's line-up: %d %v", status, body)
	}

	// Edits answer the channel; a custom stream's address is redacted.
	status, body, _ = administrator.call(http.MethodPost, channel+"/streams", map[string]any{"url": "https://cdn.example/live/token-123/zeb.ts", "label": "Backup"})
	encoded, _ := json.Marshal(body)
	if status != http.StatusCreated || strings.Contains(string(encoded), "token-123") || !strings.Contains(string(encoded), `"address":"https://cdn.example/…"`) {
		t.Errorf("a custom stream: %d %s", status, encoded)
	}
	if status, body, _ := administrator.call(http.MethodPatch, channel, map[string]any{"name": "Zeb", "number": 12, "category": kids["id"]}); status != http.StatusOK ||
		body["name"] != "Zeb" || body["renamed"] != true || body["number"] != 12.0 || body["moved"] != true {
		t.Errorf("a channel edit: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPatch, channel, map[string]any{"name": nil, "category": nil}); status != http.StatusOK ||
		body["name"] != "Zeb One" || body["moved"] != false || body["number"] != 12.0 {
		t.Errorf("back to the provider's: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPost, base+"/channels/bulk", map[string]any{"enabled": false, "q": "ORBE", "dryRun": true}); status != http.StatusOK ||
		body["matched"] != 1.0 || body["changed"] != 0.0 {
		t.Errorf("a dry run: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPost, base+"/categories", map[string]any{"name": "Favourites"}); status != http.StatusCreated ||
		body["custom"] != true || body["position"] != 3.0 || body["key"] != "u:"+body["id"].(string) {
		t.Errorf("a custom category: %d %v", status, body)
	}
}

// The guide routes of a catalog check addresses, ids, modes and mappings,
// and a member's own catalog and streams stay on public addresses.
func TestCatalogGuideRoutesValidate(t *testing.T) {
	var channels *iptv.Service
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) { channels = o.IPTV })
	provider := newProvider(t)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/iptv", map[string]any{"name": "Account", "kind": "xtream",
		"server": provider, "username": "user", "password": "secret-pw", "providerGuide": true})
	if status != http.StatusCreated {
		t.Fatalf("adding: %d %v", status, body)
	}
	catalog := map[string]any{"addonId": body["id"], "catalogType": "tv", "catalogId": "channels"}
	query := fmt.Sprintf("?addonId=%s&catalogType=tv&catalogId=channels", body["id"])
	status, guides, _ := administrator.call(http.MethodGet, "/scopes/shared/catalog-guides"+query, nil)
	list, _ := guides["guides"].([]any)
	if status != http.StatusOK || len(list) != 1 || guides["channels"] != 1.0 || guides["mapped"] != 1.0 || guides["manual"] != 0.0 {
		t.Fatalf("catalog guides: %d %v", status, guides)
	}
	guide := list[0].(map[string]any)
	if guide["url"] != provider+"/…" || guide["channels"] != 1.0 || guide["programmes"] != 1.0 || guide["position"] != 1.0 {
		t.Errorf("guide JSON: %v", guide)
	}
	_, mappings, _ := administrator.call(http.MethodGet, "/scopes/shared/catalog-guides/mappings"+query, nil)
	item := mappings["items"].([]any)[0].(map[string]any)
	mapping := item["mapping"].(map[string]any)
	if mappings["total"] != 1.0 || item["name"] != "Zeb One" || mapping["guideChannelId"] != "zeb.zz" || mapping["guideChannelName"] != "Elsewhere" ||
		mapping["manual"] != false {
		t.Errorf("mappings: %v", mappings)
	}
	_, found, _ := administrator.call(http.MethodGet, "/scopes/shared/catalog-guides/channels"+query+"&q=else", nil)
	if found["total"] != 1.0 || found["items"].([]any)[0].(map[string]any)["now"].(map[string]any)["title"] != "News" {
		t.Errorf("guide channels: %v", found)
	}
	with := func(extra map[string]any) map[string]any {
		result := map[string]any{}
		for k, v := range catalog {
			result[k] = v
		}
		for k, v := range extra {
			result[k] = v
		}
		return result
	}
	eleven := make([]string, 11)
	for i := range eleven {
		eleven[i] = fmt.Sprintf("https://guide.example/%d.xml", i)
	}
	for _, tc := range []struct {
		method, path string
		body         any
		code         string
	}{
		{http.MethodPut, "/scopes/shared/catalog-guides", with(map[string]any{"urls": []string{"ftp://guide.example/a.xml"}}), "invalid_guide_url"},
		{http.MethodPut, "/scopes/shared/catalog-guides", with(map[string]any{"urls": eleven}), "too_many_guides"},
		{http.MethodPut, "/scopes/shared/catalog-guides", with(map[string]any{"urls": []any{map[string]any{"id": strings.Repeat("0", 32)}}}), "invalid_guide"},
		{http.MethodPost, "/scopes/shared/catalog-guides/automap", with(map[string]any{"mode": "everything"}), "invalid_mode"},
		{http.MethodPut, "/scopes/shared/catalog-guides/mappings", with(map[string]any{"channelId": item["channelId"], "guideId": guide["id"],
			"guideChannelId": "missing.zz"}), "invalid_mapping"},
		{http.MethodPut, "/scopes/shared/catalog-guides/mappings", with(map[string]any{"channelId": item["channelId"], "guideId": guide["id"]}), "invalid_mapping"},
		{http.MethodPut, "/scopes/shared/catalog-guides/mappings", with(map[string]any{"channelId": strings.Repeat("0", 32)}), "not_found"},
		{http.MethodPost, "/scopes/shared/catalog-guides/automap", map[string]any{"addonId": strings.Repeat("0", 32), "catalogType": "tv",
			"catalogId": "channels", "mode": "remap"}, "invalid_library"},
	} {
		if _, body, _ := administrator.call(tc.method, tc.path, tc.body); body["error"] != tc.code {
			t.Errorf("%s %s %v: %v, want %s", tc.method, tc.path, tc.body, body, tc.code)
		}
	}
	// A manual "no guide", then back to automatic.
	if status, body, _ := administrator.call(http.MethodPut, "/scopes/shared/catalog-guides/mappings", with(map[string]any{"channelId": item["channelId"],
		"guideId": nil, "guideChannelId": nil})); status != http.StatusOK || body["mapping"].(map[string]any)["manual"] != true ||
		body["mapping"].(map[string]any)["guideId"] != nil {
		t.Errorf("no guide: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/catalog-guides/automap", with(map[string]any{"mode": "unmapped"})); status != http.StatusOK ||
		body["mapped"] != 0.0 || body["changed"] != 0.0 {
		t.Errorf("unmapped automatic mapping keeps the manual one: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodDelete, "/scopes/shared/catalog-guides/mappings"+query+"&channelId="+item["channelId"].(string), nil); status != http.StatusOK ||
		body["mapping"].(map[string]any)["guideChannelId"] != "zeb.zz" {
		t.Errorf("back to automatic: %d %v", status, body)
	}

	// A member's own source, as if public, stays confined: a custom stream
	// or a guide on the local network is refused.
	users, err := api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var owner accounts.User
	for _, u := range users {
		if u.Name == "member" {
			owner = u
		}
	}
	own, err := channels.Add(t.Context(), addons.Personal(owner.ID), iptv.NewSource{Name: "Mine",
		Account: iptv.Account{Kind: addons.KindM3U, URL: provider + "/get.php?mine=1"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	_, page, _ := member.call(http.MethodGet, "/scopes/me/iptv/"+own.ID.String()+"/channels", nil)
	first := page["items"].([]any)[0].(map[string]any)["id"].(string)
	if status, body, _ := member.call(http.MethodPost, "/scopes/me/iptv/"+own.ID.String()+"/channels/"+first+"/streams",
		map[string]any{"url": provider + "/live/9.ts"}); status != http.StatusForbidden || body["error"] != "private_network" {
		t.Errorf("a member's local custom stream: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPut, "/scopes/me/catalog-guides", map[string]any{"addonId": own.ID.String(), "catalogType": "tv",
		"catalogId": "channels", "urls": []string{provider + "/xmltv.php"}}); status != http.StatusForbidden || body["error"] != "private_network" {
		t.Errorf("a member's local guide: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodGet, "/scopes/me/catalog-guides"+query, nil); status != http.StatusBadRequest || body["error"] != "invalid_library" {
		t.Errorf("a member reading the server's catalog in their scope: %d %v", status, body)
	}
}
