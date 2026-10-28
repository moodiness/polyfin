package admin

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// An administrator adds an M3U list and its XMLTV guide that are files of
// the server, by their paths, shown whole; a path where there is no file
// is refused with why. A member is refused any file of the server, as a
// list or as a guide, whether it exists or not.
func TestIPTVSourcesFromFilesOfTheServer(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	member := api.signedIn("member", false)
	administrator := api.signedIn("administrator", true)
	dir := t.TempDir()
	list, guide := filepath.Join(dir, "custom.m3u"), filepath.Join(dir, "guide.xml")
	if err := os.WriteFile(list, []byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"zeb.zz\",Zeb One\nhttps://live.example/1.ts\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(guide, []byte(`<tv><channel id="zeb.zz"><display-name>Zeb One</display-name></channel></tv>`), 0o600); err != nil {
		t.Fatal(err)
	}

	status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/iptv", map[string]any{"name": "Files", "kind": "m3u", "url": list, "guideUrl": guide})
	if source, _ := body["source"].(map[string]any); status != http.StatusCreated || body["manifestUrl"] != list || source["channels"] != 1.0 {
		t.Fatalf("an administrator's list file: %d %v", status, body)
	}
	missing := filepath.Join(dir, "missing.m3u")
	if status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/iptv", map[string]any{"name": "Gone", "kind": "m3u", "url": missing}); status != http.StatusBadRequest ||
		body["error"] != "file_missing" {
		t.Errorf("a missing list file: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/iptv", map[string]any{"name": "Relative", "kind": "m3u", "url": "custom.m3u"}); status != http.StatusBadRequest ||
		body["error"] != "invalid_source_address" {
		t.Errorf("a relative path: %d %v", status, body)
	}

	for _, path := range []string{list, missing} {
		if status, body, _ := member.call(http.MethodPost, "/scopes/me/iptv", map[string]any{"name": "Mine", "kind": "m3u", "url": path}); status != http.StatusForbidden ||
			body["error"] != "local_file_refused" {
			t.Errorf("a member's list file %s: %d %v", path, status, body)
		}
		if status, body, _ := member.call(http.MethodPost, "/scopes/me/iptv/preview", map[string]any{"kind": "m3u", "url": path, "by": "group"}); status != http.StatusForbidden ||
			body["error"] != "local_file_refused" {
			t.Errorf("a member's preview of a file %s: %d %v", path, status, body)
		}
		if status, body, _ := member.call(http.MethodPost, "/scopes/me/iptv", map[string]any{"name": "Mine", "kind": "m3u", "url": "https://tv.example/list.m3u",
			"guideUrl": path}); status != http.StatusForbidden || body["error"] != "local_file_refused" {
			t.Errorf("a member's guide file %s: %d %v", path, status, body)
		}
		target := map[string]any{"addonId": "00000000-0000-0000-0000-000000000001", "catalogType": "tv", "catalogId": "channels"}
		target["url"] = path
		if status, body, _ := member.call(http.MethodPut, "/scopes/me/guides", target); status != http.StatusForbidden || body["error"] != "local_file_refused" {
			t.Errorf("a member's catalog guide file %s: %d %v", path, status, body)
		}
		delete(target, "url")
		target["urls"] = []any{"https://guide.example/a.xml", path}
		if status, body, _ := member.call(http.MethodPut, "/scopes/me/catalog-guides", target); status != http.StatusForbidden || body["error"] != "local_file_refused" {
			t.Errorf("a member's catalog guide list with a file %s: %d %v", path, status, body)
		}
	}
}
