package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The admin app lists a channel left out of apps because its streams need
// a DRM Polyfin does not support, with that reason and the DRM each
// stream needs. A stream of the administrator's own brings it back.
func TestChannelsNeedingAnUnsupportedDRMShowWhy(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/get.php" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, "#EXTM3U\n#EXTINF:-1 tvg-chno=\"1\",Zeb One\n%[1]s/live/zeb.m3u8\n"+
			"#EXTINF:-1 tvg-chno=\"2\",Moss One\n#KODIPROP:inputstream.adaptive.drm_legacy=com.widevine.alpha|%[1]s/licence\n%[1]s/drm/moss.mpd\n",
			server.URL)
	}))
	t.Cleanup(server.Close)
	admin := api.signedIn("administrator", true)
	status, source, _ := admin.call(http.MethodPost, "/scopes/shared/iptv", map[string]any{"name": "Playlist", "kind": "m3u", "url": server.URL + "/get.php"})
	if status != http.StatusCreated {
		t.Fatalf("adding a playlist: %d %v", status, source)
	}
	id := source["id"].(string)
	base := "/scopes/shared/iptv/" + id
	shown := func() any {
		t.Helper()
		response, err := admin.client.Get(api.url + "/admin/api/scopes/shared/addons")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		var list []map[string]any
		_ = json.NewDecoder(response.Body).Decode(&list)
		for _, a := range list {
			if a["id"] == id {
				return a["source"].(map[string]any)["lineup"].(map[string]any)["shownChannels"]
			}
		}
		t.Fatalf("the playlist is not listed: %v", list)
		return nil
	}

	status, page, _ := admin.call(http.MethodGet, base+"/channels", nil)
	items, _ := page["items"].([]any)
	if status != http.StatusOK || len(items) != 2 {
		t.Fatalf("the channels: %d %v", status, page)
	}
	zeb, moss := items[0].(map[string]any), items[1].(map[string]any)
	stream := func(c map[string]any) map[string]any { return c["streams"].([]any)[0].(map[string]any) }
	if zeb["shown"] != true || zeb["needsDrm"] != false || stream(zeb)["drm"] != "" {
		t.Errorf("a channel apps play: %v", zeb)
	}
	if moss["name"] != "Moss One" || moss["enabled"] != true || moss["shown"] != false || moss["needsDrm"] != true || stream(moss)["drm"] != "Widevine" {
		t.Errorf("a channel needing a DRM: %v", moss)
	}
	if n := shown(); n != 1.0 {
		t.Errorf("channels shown: %v", n)
	}
	status, page, _ = admin.call(http.MethodGet, base+"/channels?shown=false", nil)
	if items, _ := page["items"].([]any); status != http.StatusOK || len(items) != 1 || items[0].(map[string]any)["name"] != "Moss One" {
		t.Errorf("the channels not shown: %d %v", status, page)
	}

	status, moss, _ = admin.call(http.MethodPost, base+"/channels/"+moss["id"].(string)+"/streams", map[string]any{"url": server.URL + "/live/moss.m3u8", "label": "Mine"})
	if status != http.StatusCreated || moss["shown"] != true || moss["needsDrm"] != false {
		t.Errorf("a channel with a stream of the administrator's: %d %v", status, moss)
	}
	if n := shown(); n != 2.0 {
		t.Errorf("channels shown with that stream: %v", n)
	}
}
