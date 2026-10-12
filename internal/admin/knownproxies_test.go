package admin

import (
	"context"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/forwarded"
)

// Known proxies are empty by default, saved as they are compared, the host
// names after the addresses, kept when a PUT leaves them out, and refused
// when an entry is not an address, a network nor a host name, or when
// there are too many.
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
	want := []any{"127.0.0.1", "172.18.0.0/16", "2001:db8::/32", "traefik"}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", with("127.0.0.1", "Traefik", " 172.18.0.9/16", "2001:db8::/32")); status != http.StatusOK ||
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
	for name, body := range map[string]map[string]any{"not a host name": with("traefik:8080"), "65 entries": with(many...)} {
		if status, answer, _ := administrator.call(http.MethodPut, "/settings", body); status != http.StatusBadRequest || answer["error"] != "invalid_known_proxies" {
			t.Errorf("%s: %d %v", name, status, answer)
		}
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", with()); status != http.StatusOK || !reflect.DeepEqual(body["knownProxies"], []any{}) {
		t.Errorf("emptied: %d %v", status, body["knownProxies"])
	}
	if got := api.store.KnownProxies(); len(got) != 0 || len(api.store.KnownProxyNames()) != 0 {
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
		o.Health = HealthSources{Proxies: forwarded.New(o.Accounts.KnownProxies, nil)}
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

// tableResolver answers the host names it lists, and that no other exists.
type tableResolver map[string][]netip.Addr

func (r tableResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if addresses, ok := r[host]; ok {
		return addresses, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

// The host names saved in Known proxies are looked up: System › Health
// shows what each resolves to, warns of one that resolves to nothing,
// in notifications too, and stops once it is removed.
func TestHealthShowsProxyNames(t *testing.T) {
	t.Parallel()
	var options *Options
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		proxies := forwarded.New(o.Accounts.KnownProxies, tableResolver{"traefik": {netip.MustParseAddr("172.18.0.3")}})
		proxies.Watch(t.Context(), o.Accounts.KnownProxyNames, o.Accounts.SettingsChanged)
		o.Health = HealthSources{Proxies: proxies}
		options = o
	})
	administrator := api.signedIn("administrator", true)
	save := func(entries ...string) {
		t.Helper()
		body := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en", "knownProxies": entries}
		if status, answer, _ := administrator.call(http.MethodPut, "/settings", body); status != http.StatusOK {
			t.Fatalf("saving: %d %v", status, answer)
		}
	}
	// names waits for Health to show the names given, and returns what
	// each resolves to.
	names := func(want ...string) map[string]any {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			_, body, _ := administrator.call(http.MethodGet, "/health", nil)
			list, _ := body["proxyNames"].([]any)
			found := map[string]any{}
			var shown []string
			for _, entry := range list {
				name := entry.(map[string]any)
				shown = append(shown, name["name"].(string))
				found[name["name"].(string)] = name["addresses"]
			}
			if slices.Equal(shown, want) {
				return found
			}
			if time.Now().After(deadline) {
				t.Fatalf("health shows %v, want %v", shown, want)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	problems := func() []any {
		t.Helper()
		_, body, _ := administrator.call(http.MethodGet, "/health/problems?language=en", nil)
		list, _ := body["problems"].([]any)
		return list
	}

	save("10.0.0.1", "traefik", "nothing.invalid")
	if got := names("traefik", "nothing.invalid"); !reflect.DeepEqual(got, map[string]any{"traefik": []any{"172.18.0.3"},
		"nothing.invalid": []any{}}) {
		t.Errorf("resolved: %v", got)
	}
	if found := problems(); len(found) != 1 || !reflect.DeepEqual(found[0], map[string]any{"key": "proxy_name:nothing.invalid",
		"code": "proxy_name_unresolved", "tone": "warning", "to": "/settings/security#known-proxies", "transient": false,
		"name": "nothing.invalid"}) {
		t.Fatalf("problems: %v", found)
	}
	told, err := HealthProblems(*options)(t.Context())
	if err != nil || len(told) != 1 || !strings.HasPrefix(told[0].Text, "The known proxy nothing.invalid resolves to no address") {
		t.Errorf("notifications: %+v %v", told, err)
	}
	save("traefik")
	names("traefik")
	if found := problems(); len(found) != 0 {
		t.Errorf("once removed: %v", found)
	}
}
