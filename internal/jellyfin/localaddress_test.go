package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// /System/Info/Public gives as LocalAddress the address on the local
// network the settings give, as Jellyfin gives its PublishedServerUrl, and
// without one the address the request reached.
func TestPublicInfoLocalAddressFollowsTheSetting(t *testing.T) {
	t.Parallel()
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

// Over HTTPS, as Polyfin serves it itself, /System/Info/Public and
// /System/Info give as LocalAddress, without the setting, the address the
// request reached, https:// and the HTTPS port included, and stream
// addresses start with https://.
func TestAppsOverHTTPSSeeHTTPSAddresses(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	secure := httptest.NewTLSServer(s.handler)
	t.Cleanup(secure.Close)
	response, err := secure.Client().Get(secure.URL + "/System/Info/Public")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var info PublicSystemInfo
	if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(secure.URL, "https://") || info.LocalAddress != secure.URL {
		t.Errorf("LocalAddress %q, want %q", info.LocalAddress, secure.URL)
	}
	r := httptest.NewRequest(http.MethodGet, "https://media.example.org:8920/Items/1/PlaybackInfo", nil)
	if got := baseURL(r); got != "https://media.example.org:8920" {
		t.Errorf("stream addresses start with %q", got)
	}
}
