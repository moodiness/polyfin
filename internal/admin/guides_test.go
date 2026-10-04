package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// guideCall sends a guide request and returns the libraries answered, the
// raw body and the status.
func (b browser) guideCall(method, path string, body any) (int, []map[string]any, string) {
	b.api.t.Helper()
	encoded, _ := json.Marshal(body)
	request, _ := http.NewRequestWithContext(b.api.t.Context(), method, b.api.url+"/admin/api"+path, bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	response, err := b.client.Do(request)
	if err != nil {
		b.api.t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	var libraries []map[string]any
	_ = json.Unmarshal(raw, &libraries)
	return response.StatusCode, libraries, string(raw)
}

func tvGuide(libraries []map[string]any) map[string]any {
	for _, l := range libraries {
		if l["catalogType"] == "tv" {
			guide, _ := l["guide"].(map[string]any)
			return guide
		}
	}
	return nil
}

// The Libraries page sets a TV catalog's guide, which is fetched at once
// and shown redacted with how the fetch went, and refreshes it on demand.
func TestTVCatalogGuides(t *testing.T) {
	api := newTestAPI(t, 10)
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/catalog/tv/channels") {
			_, _ = io.WriteString(w, `{"metas": [{"id": "tv:one", "type": "tv", "name": "One HD"}, {"id": "tv:two", "type": "tv", "name": "Two"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"id": "iptv", "name": "IPTV", "version": "1.0.0", "resources": ["catalog"],
			"catalogs": [{"type": "tv", "id": "channels", "name": "Channels"}, {"type": "movie", "id": "top", "name": "Top"}]}`)
	}))
	t.Cleanup(addon.Close)
	now := time.Now().UTC()
	var downloads atomic.Int32
	guide := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		downloads.Add(1)
		_, _ = fmt.Fprintf(w, `<tv><channel id="1"><display-name>One</display-name></channel>
			<programme channel="1" start="%s" stop="%s"><title>News</title></programme></tv>`,
			now.Add(-time.Hour).Format("20060102150405 -0700"), now.Add(time.Hour).Format("20060102150405 -0700"))
	}))
	t.Cleanup(guide.Close)
	guideURL := guide.URL + "/get.php?username=me&password=secret-token"

	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": addon.URL + "/manifest.json"})
	if status != http.StatusCreated {
		t.Fatalf("installing: %d %v", status, body)
	}
	addonID := body["id"]
	target := map[string]any{"addonId": addonID, "catalogType": "tv", "catalogId": "channels", "url": guideURL}

	status, libraries, raw := administrator.guideCall(http.MethodPut, "/scopes/shared/guides", target)
	if status != http.StatusOK {
		t.Fatalf("saving a guide: %d %s", status, raw)
	}
	if strings.Contains(raw, "secret-token") || strings.Contains(raw, "get.php") {
		t.Errorf("the guide address is not redacted: %s", raw)
	}
	saved := tvGuide(libraries)
	if saved == nil || saved["url"] != guide.URL+"/…" || saved["fetchedAt"] == nil || saved["matched"] != 1.0 ||
		saved["channels"] != 2.0 || saved["error"] != "" || downloads.Load() != 1 {
		t.Errorf("saved guide: %v after %d downloads", saved, downloads.Load())
	}
	for _, l := range libraries {
		if l["catalogType"] == "movie" && l["guide"] != nil {
			t.Errorf("a movie library with a guide: %v", l)
		}
	}

	if status, libraries, raw = administrator.guideCall(http.MethodPost, "/scopes/shared/guides/refresh", target); status != http.StatusOK ||
		downloads.Load() != 2 || tvGuide(libraries)["matched"] != 1.0 {
		t.Errorf("refreshing: %d %s after %d downloads", status, raw, downloads.Load())
	}
	for name, bad := range map[string]map[string]any{
		"address": {"addonId": addonID, "catalogType": "tv", "catalogId": "channels", "url": "file:///etc/passwd"},
		"catalog": {"addonId": addonID, "catalogType": "movie", "catalogId": "top", "url": guideURL},
	} {
		want := map[string]string{"address": "invalid_guide_url", "catalog": "invalid_library"}[name]
		if status, _, raw := administrator.guideCall(http.MethodPut, "/scopes/shared/guides", bad); status != http.StatusBadRequest || !strings.Contains(raw, want) {
			t.Errorf("invalid %s: %d %s", name, status, raw)
		}
	}
	if status, _, raw := member.guideCall(http.MethodPut, "/scopes/shared/guides", target); status != http.StatusForbidden {
		t.Errorf("a member setting a server guide: %d %s", status, raw)
	}

	target["url"] = ""
	if status, libraries, raw = administrator.guideCall(http.MethodPut, "/scopes/shared/guides", target); status != http.StatusOK ||
		tvGuide(libraries)["url"] != "" || tvGuide(libraries)["fetchedAt"] != nil || downloads.Load() != 2 {
		t.Errorf("removing the guide: %d %s", status, raw)
	}
}
