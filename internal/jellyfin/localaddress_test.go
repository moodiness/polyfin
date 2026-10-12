package jellyfin

import (
	"encoding/json"
	"net/http"
	"testing"
)

// /System/Info/Public gives as LocalAddress the address on the local
// network the settings give, as Jellyfin gives its PublishedServerUrl, and
// without one the address the request reached.
func TestPublicInfoLocalAddressFollowsTheSetting(t *testing.T) {
	s := newTestServer(t, 10)
	localAddress := func() string {
		t.Helper()
		status, body := s.call(http.MethodGet, "/System/Info/Public", "", nil)
		var info PublicSystemInfo
		if err := json.Unmarshal(body, &info); status != http.StatusOK || err != nil {
			t.Fatalf("%d %v %s", status, err, body)
		}
		return info.LocalAddress
	}
	if got := localAddress(); got != s.url {
		t.Errorf("without the setting: %q, want %q", got, s.url)
	}
	settings := s.store.Settings()
	settings.LocalAddress = "http://192.168.1.10:8096/"
	if _, err := s.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	if got := localAddress(); got != "http://192.168.1.10:8096" {
		t.Errorf("with the setting: %q", got)
	}
}
