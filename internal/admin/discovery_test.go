package admin

import (
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/discovery"
)

// Discovery is on and announces the interface's address by default; the
// address on the local network is saved without its trailing slash, kept
// when a PUT leaves it out, and refused when it is not an http or https
// URL; the switch turns off.
func TestSettingsLocalDiscovery(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["localDiscovery"] != true || body["localAddress"] != "" {
		t.Errorf("defaults: %v %v", body["localDiscovery"], body["localAddress"])
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	with := func(key string, value any) map[string]any {
		body := maps.Clone(base)
		body[key] = value
		return body
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", with("localAddress", " http://192.168.1.10:8096/ ")); status != http.StatusOK ||
		body["localAddress"] != "http://192.168.1.10:8096" {
		t.Fatalf("saving: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || body["localAddress"] != "http://192.168.1.10:8096" ||
		body["localDiscovery"] != true {
		t.Errorf("left out: %d %v %v", status, body["localAddress"], body["localDiscovery"])
	}
	for _, refused := range []string{"192.168.1.10:8096", "ftp://192.168.1.10", "http://user:pass@192.168.1.10:8096", "http://192.168.1.10:8096/?a=b",
		"http://" + strings.Repeat("a", 506)} {
		if status, body, _ := administrator.call(http.MethodPut, "/settings", with("localAddress", refused)); status != http.StatusBadRequest ||
			body["error"] != "invalid_local_address" {
			t.Errorf("%q: %d %v", refused, status, body)
		}
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", with("localDiscovery", false)); status != http.StatusOK || body["localDiscovery"] != false {
		t.Errorf("off: %d %v", status, body["localDiscovery"])
	}
	if settings := api.store.Settings(); settings.LocalDiscovery || settings.LocalAddress != "http://192.168.1.10:8096" {
		t.Errorf("stored: %v %q", settings.LocalDiscovery, settings.LocalAddress)
	}
}

// A discovery port another program uses is a warning of System › Health,
// linked to the switch and told to notifications; turning discovery off
// ends it.
func TestHealthTellsATakenDiscoveryPort(t *testing.T) {
	t.Parallel()
	taken, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	port := taken.LocalAddr().(*net.UDPAddr).Port
	var options *Options
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		store := o.Accounts
		service := discovery.Start(t.Context(), discovery.Options{Port: port, HTTP: &net.TCPAddr{Port: 8096}, ServerID: "0123",
			Settings: func() discovery.Settings {
				return discovery.Settings{Enabled: store.Settings().LocalDiscovery, Name: store.Settings().ServerName}
			},
			Changed: store.SettingsChanged, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		o.Health = HealthSources{Discovery: service.Status}
		options = o
	})
	administrator := api.signedIn("administrator", true)
	problems := func() []any {
		t.Helper()
		_, body, _ := administrator.call(http.MethodGet, "/health/problems?language=en", nil)
		list, _ := body["problems"].([]any)
		return list
	}
	if found := problems(); len(found) != 1 || !reflect.DeepEqual(found[0], map[string]any{"key": "discovery:port", "code": "discovery_port",
		"tone": "warning", "to": "/settings/general#local-discovery", "transient": false, "port": float64(port)}) {
		t.Fatalf("problems: %v", found)
	}
	told, err := HealthProblems(*options)(t.Context())
	if err != nil || len(told) != 1 || !strings.HasPrefix(told[0].Text, "Polyfin cannot listen on UDP port "+strconv.Itoa(port)+",") {
		t.Errorf("notifications: %+v %v", told, err)
	}
	settings := api.store.Settings()
	settings.LocalDiscovery = false
	if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	// Status reads the switch at once; the port is freed soon after.
	if found := problems(); len(found) != 0 {
		t.Errorf("off: %v", found)
	}
}

// An address of the container that discovery announced is a warning,
// linked to the address setting, told in French too.
func TestHealthTellsAContainersAddress(t *testing.T) {
	t.Parallel()
	var options *Options
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Health = HealthSources{Discovery: func() discovery.Status {
			return discovery.Status{Port: discovery.Port, ContainerAddress: "http://172.17.0.3:8096"}
		}}
		options = o
	})
	administrator := api.signedIn("administrator", true)
	_, body, _ := administrator.call(http.MethodGet, "/health/problems", nil)
	if found, _ := body["problems"].([]any); len(found) != 1 || !reflect.DeepEqual(found[0], map[string]any{"key": "discovery:address",
		"code": "discovery_address", "tone": "warning", "to": "/settings/general#local-address", "transient": false,
		"address": "http://172.17.0.3:8096"}) {
		t.Fatalf("problems: %v", found)
	}
	settings := api.store.Settings()
	settings.Language = "fr"
	if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	told, err := HealthProblems(*options)(t.Context())
	if err != nil || len(told) != 1 || !strings.Contains(told[0].Text, "reçoivent http://172.17.0.3:8096, une adresse du conteneur") {
		t.Errorf("notifications: %+v %v", told, err)
	}
}
