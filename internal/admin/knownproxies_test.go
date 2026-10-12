package admin

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/forwarded"
)

// Known proxies are empty by default, saved as they are compared, kept
// when a PUT leaves them out, and refused when an entry is not an address
// nor a network, or when there are too many.
func TestSettingsKnownProxies(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); !reflect.DeepEqual(body["knownProxies"], []any{}) {
		t.Errorf("default: %#v", body["knownProxies"])
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	with := func(entries ...string) map[string]any {
		body := maps.Clone(base)
		body["knownProxies"] = append([]string{}, entries...)
		return body
	}
	want := []any{"127.0.0.1", "172.18.0.0/16", "2001:db8::/32"}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", with("127.0.0.1", " 172.18.0.9/16", "2001:db8::/32")); status != http.StatusOK ||
		!reflect.DeepEqual(body["knownProxies"], want) {
		t.Fatalf("saving: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || !reflect.DeepEqual(body["knownProxies"], want) {
		t.Errorf("left out: %d %v", status, body["knownProxies"])
	}
	many := make([]string, 65)
	for i := range many {
		many[i] = "10.0.0." + strconv.Itoa(i)
	}
	for name, body := range map[string]map[string]any{"a host name": with("proxy.example.org"), "65 entries": with(many...)} {
		if status, answer, _ := administrator.call(http.MethodPut, "/settings", body); status != http.StatusBadRequest || answer["error"] != "invalid_known_proxies" {
			t.Errorf("%s: %d %v", name, status, answer)
		}
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", with()); status != http.StatusOK || !reflect.DeepEqual(body["knownProxies"], []any{}) {
		t.Errorf("emptied: %d %v", status, body["knownProxies"])
	}
	if got := api.store.KnownProxies(); len(got) != 0 {
		t.Errorf("stored: %v", got)
	}
}

// System › Health names an address in the local network that sends
// forwarded headers without being a known proxy, links to the setting,
// tells notifications in the server language, and stops once the address
// is known.
func TestHealthNamesUnknownProxies(t *testing.T) {
	t.Parallel()
	var options *Options
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Health = HealthSources{Proxies: forwarded.New(o.Accounts.KnownProxies)}
		options = o
	})
	administrator := api.signedIn("administrator", true)
	request := httptest.NewRequest(http.MethodGet, "/web/", nil)
	request.RemoteAddr = "127.0.0.1:52000"
	request.Header.Set("X-Forwarded-For", "203.0.113.9")
	options.Health.Proxies.Handler(http.NotFoundHandler()).ServeHTTP(httptest.NewRecorder(), request)

	problems := func() []any {
		t.Helper()
		_, body, _ := administrator.call(http.MethodGet, "/health/problems?language=en", nil)
		list, _ := body["problems"].([]any)
		return list
	}
	found := problems()
	if len(found) != 1 || !reflect.DeepEqual(found[0], map[string]any{"key": "unknown_proxy:127.0.0.1", "code": "unknown_proxy", "tone": "warning",
		"to": "/settings/security#known-proxies", "transient": false, "address": "127.0.0.1"}) {
		t.Fatalf("problems: %v", found)
	}
	told, err := HealthProblems(*options)(t.Context())
	if err != nil || len(told) != 1 || !strings.HasPrefix(told[0].Text, "127.0.0.1 forwards requests") || told[0].Page != "/settings/security#known-proxies" {
		t.Errorf("notifications: %+v %v", told, err)
	}
	body := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en", "knownProxies": []string{"127.0.0.1"}}
	if status, answer, _ := administrator.call(http.MethodPut, "/settings", body); status != http.StatusOK {
		t.Fatalf("saving: %d %v", status, answer)
	}
	if found := problems(); len(found) != 0 {
		t.Errorf("once known: %v", found)
	}
}
