package admin

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// newProvider serves an M3U playlist, and an Xtream Codes account (user /
// secret-pw) with its XMLTV guide, all naming one channel.
func newProvider(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch r.URL.Path {
		case "/get.php":
			_, _ = fmt.Fprintf(w, "#EXTM3U\n#EXTINF:-1 tvg-id=\"zeb.zz\" group-title=\"News\",Zeb One\n%s/live/1.ts\n"+
				"#EXTINF:-1 group-title=\"Kids\",Orbe Junior\n%s/live/2.ts\n", server.URL, server.URL)
		case "/player_api.php":
			if query.Get("username") != "user" || query.Get("password") != "secret-pw" {
				_, _ = io.WriteString(w, `{"user_info": {"auth": 0}}`)
				return
			}
			switch query.Get("action") {
			case "":
				_, _ = io.WriteString(w, `{"user_info": {"auth": 1, "allowed_output_formats": ["m3u8"]}}`)
			case "get_live_categories":
				_, _ = io.WriteString(w, `[{"category_id": "1", "category_name": "News"}]`)
			case "get_live_streams":
				_, _ = io.WriteString(w, `[{"num": 1, "name": "Zeb One", "stream_id": 7, "epg_channel_id": "zeb.zz", "category_id": "1"}]`)
			}
		case "/xmltv.php":
			now := time.Now().UTC()
			_, _ = fmt.Fprintf(w, `<tv><channel id="zeb.zz"><display-name>Elsewhere</display-name></channel>
				<programme channel="zeb.zz" start="%s" stop="%s"><title>News</title></programme></tv>`,
				now.Add(-time.Hour).Format("20060102150405 -0700"), now.Add(time.Hour).Format("20060102150405 -0700"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func (b browser) libraries(scope string) []map[string]any {
	b.api.t.Helper()
	response, err := b.client.Get(b.api.url + "/admin/api/scopes/" + scope + "/libraries")
	if err != nil {
		b.api.t.Fatal(err)
	}
	defer response.Body.Close()
	var libraries []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&libraries)
	return libraries
}

// IPTV sources are added beside addons and listed among them, their
// credentials never shown; their groups are chosen and their list
// refreshed from the same rows.
func TestIPTVSources(t *testing.T) {
	api := newTestAPI(t, 10)
	provider := newProvider(t)
	administrator := api.signedIn("administrator", true)

	status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/iptv", map[string]any{"name": "Playlist", "kind": "m3u",
		"url": provider + "/get.php?username=user&password=secret-pw"})
	if status != http.StatusCreated {
		t.Fatalf("adding a playlist: %d %v", status, body)
	}
	encoded, _ := json.Marshal(body)
	if strings.Contains(string(encoded), "secret-pw") || strings.Contains(string(encoded), "username") {
		t.Errorf("credentials shown: %s", encoded)
	}
	source, _ := body["source"].(map[string]any)
	if body["kind"] != "m3u" || body["manifestUrl"] != provider+"/…" || source == nil || source["channels"] != 2.0 ||
		source["includedGroups"] != nil || source["error"] != "" || source["nextAt"] == nil {
		t.Errorf("source: %v", body)
	}
	id := body["id"].(string)

	if status, body, _ := administrator.call(http.MethodPatch, "/scopes/shared/iptv/"+id, map[string]any{"groups": []string{"Kids"}}); status != http.StatusOK ||
		fmt.Sprint(body["source"].(map[string]any)["includedGroups"]) != "[Kids]" {
		t.Errorf("choosing groups: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPatch, "/scopes/shared/iptv/"+id, map[string]any{"groups": nil}); status != http.StatusOK ||
		body["source"].(map[string]any)["includedGroups"] != nil {
		t.Errorf("every group again: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/addons/"+id+"/refresh", nil); status != http.StatusOK ||
		body["source"].(map[string]any)["channels"] != 2.0 {
		t.Errorf("refreshing the list: %d %v", status, body)
	}
	// It is not a Stremio addon: it has no manifest to replace.
	if status, body, _ := administrator.call(http.MethodPatch, "/scopes/shared/addons/"+id, map[string]any{"manifestUrl": provider + "/manifest.json"}); status != http.StatusBadRequest {
		t.Errorf("replacing its manifest: %d %v", status, body)
	}
	response, err := administrator.client.Get(api.url + "/admin/api/scopes/shared/addons")
	if err != nil {
		t.Fatal(err)
	}
	listed, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(listed), `"kind":"m3u"`) || !strings.Contains(string(listed), `"channels":2`) || strings.Contains(string(listed), "secret-pw") {
		t.Errorf("listed among the addons: %s", listed)
	}

	// An Xtream account, with the provider's own guide, fetched at once.
	status, body, _ = administrator.call(http.MethodPost, "/scopes/shared/iptv", map[string]any{"name": "Account", "kind": "xtream",
		"server": provider, "username": "user", "password": "secret-pw", "providerGuide": true})
	if status != http.StatusCreated || body["kind"] != "xtream" {
		t.Fatalf("adding an account: %d %v", status, body)
	}
	var guide map[string]any
	for _, l := range administrator.libraries("shared") {
		if l["addonId"] == body["id"] {
			guide, _ = l["guide"].(map[string]any)
		}
	}
	if guide == nil || guide["url"] != provider+"/…" || guide["matched"] != 1.0 || guide["nextAt"] == nil {
		t.Errorf("the provider's guide: %v", guide)
	}
	for _, tc := range []struct {
		body map[string]any
		code string
	}{
		{map[string]any{"name": "Wrong", "kind": "xtream", "server": provider, "username": "user", "password": "nope"}, "iptv_login_refused"},
		{map[string]any{"name": "Page", "kind": "m3u", "url": provider + "/xmltv.php"}, "invalid_channel_list"},
		{map[string]any{"name": "Bad", "kind": "m3u", "url": "file:///etc/passwd"}, "invalid_source_address"},
		{map[string]any{"name": "Kind", "kind": "other", "url": provider + "/get.php"}, "invalid_source_address"},
		{map[string]any{"name": "", "kind": "m3u", "url": provider + "/get.php?x"}, "invalid_source_name"},
		{map[string]any{"name": "Again", "kind": "m3u", "url": provider + "/get.php?username=user&password=secret-pw"}, "addon_exists"},
	} {
		if status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/iptv", tc.body); body["error"] != tc.code {
			t.Errorf("%v: %d %v", tc.body, status, body)
		}
	}
}

// A user's own sources follow the rules of their own addons: only an
// administrator's reach the local network, and none are added while the
// server turns users' own addons off.
func TestPersonalIPTVSources(t *testing.T) {
	api := newTestAPI(t, 10)
	provider := newProvider(t)
	member := api.signedIn("member", false)
	administrator := api.signedIn("administrator", true)
	playlist := map[string]any{"name": "Mine", "kind": "m3u", "url": provider + "/get.php"}
	if status, body, _ := member.call(http.MethodPost, "/scopes/me/iptv", playlist); status != http.StatusForbidden || body["error"] != "private_network" {
		t.Errorf("a member's local source: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPost, "/scopes/shared/iptv", playlist); status != http.StatusForbidden || body["error"] != "forbidden" {
		t.Errorf("a member adding a server source: %d %v", status, body)
	}
	status, body, _ := administrator.call(http.MethodPost, "/scopes/me/iptv", playlist)
	if status != http.StatusCreated {
		t.Fatalf("an administrator's own local source: %d %v", status, body)
	}
	id := body["id"].(string)
	settings := maps.Clone(map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en", "personalAddons": false})
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK {
		t.Fatalf("turning users' own addons off: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPost, "/scopes/me/iptv", map[string]any{"name": "Other", "kind": "m3u", "url": "https://tv.example/list.m3u"}); status != http.StatusForbidden || body["error"] != "personal_addons_disabled" {
		t.Errorf("adding while own addons are off: %d %v", status, body)
	}
	if status, _, _ := member.call(http.MethodPatch, "/scopes/me/iptv/"+id, map[string]any{"groups": nil}); status != http.StatusForbidden && status != http.StatusNotFound {
		t.Errorf("changing another user's source: %d", status)
	}
}

// The refresh interval of lists and guides is a setting, 12 hours by
// default, from 1 to 168.
func TestSettingsLiveTvRefreshHours(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["liveTvRefreshHours"] != 12.0 {
		t.Errorf("default: %v", body["liveTvRefreshHours"])
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	settings := maps.Clone(base)
	settings["liveTvRefreshHours"] = 6
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK || body["liveTvRefreshHours"] != 6.0 {
		t.Fatalf("saving: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || body["liveTvRefreshHours"] != 6.0 {
		t.Errorf("left out: %d %v", status, body)
	}
	for _, hours := range []int{0, accounts.MaxLiveTvRefreshHours + 1} {
		refused := maps.Clone(base)
		refused["liveTvRefreshHours"] = hours
		if status, body, _ := administrator.call(http.MethodPut, "/settings", refused); status != http.StatusBadRequest || body["error"] != "invalid_live_tv_refresh_hours" {
			t.Errorf("%d hours: %d %v", hours, status, body)
		}
	}
}
