package admin

import (
	"encoding/json"
	"maps"
	"net/http"
	"strings"
	"testing"
)

// Turning metrics on makes their token, which no answer holds and an
// administrator reveals; turned off and on again, it stays; a new one ends
// the former; a member can neither reveal nor make one.
func TestSettingsMetricsToken(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator, member := api.signedIn("administrator", true), api.signedIn("member", false)
	holdsToken := func(body map[string]any) bool {
		encoded, _ := json.Marshal(body)
		token := api.store.Settings().MetricsToken
		return token != "" && strings.Contains(string(encoded), token)
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["metrics"] != false || body["metricsTokenSet"] != false {
		t.Fatalf("default: %v %v", body["metrics"], body["metricsTokenSet"])
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	on := maps.Clone(base)
	on["metrics"] = true
	if status, body, _ := administrator.call(http.MethodPut, "/settings", on); status != http.StatusOK || body["metrics"] != true ||
		body["metricsTokenSet"] != true || holdsToken(body) {
		t.Fatalf("turned on: %d %v %v", status, body["metrics"], body["metricsTokenSet"])
	}
	first := api.store.Settings().MetricsToken
	if len(first) != 32 {
		t.Fatalf("token: %q", first)
	}
	if status, body, _ := administrator.call(http.MethodPost, "/settings/secrets/metricsToken/reveal", nil); status != http.StatusOK || body["value"] != first {
		t.Errorf("revealed: %d %v", status, body)
	}
	if status, _, _ := member.call(http.MethodPost, "/settings/secrets/metricsToken/reveal", nil); status != http.StatusForbidden {
		t.Errorf("revealed to a member: %d", status)
	}

	// Left out, then off and on again: the same token.
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || body["metrics"] != true {
		t.Errorf("left out: %d %v", status, body["metrics"])
	}
	off := maps.Clone(base)
	off["metrics"] = false
	for _, change := range []map[string]any{off, on} {
		if status, _, _ := administrator.call(http.MethodPut, "/settings", change); status != http.StatusOK || api.store.Settings().MetricsToken != first {
			t.Errorf("metrics %v: %d, token changed", change["metrics"], status)
		}
	}

	if status, _, _ := member.call(http.MethodPost, "/settings/metrics-token", nil); status != http.StatusForbidden || api.store.Settings().MetricsToken != first {
		t.Errorf("a member made a token: %d", status)
	}
	status, body, _ := administrator.call(http.MethodPost, "/settings/metrics-token", nil)
	second := api.store.Settings().MetricsToken
	if status != http.StatusOK || body["metricsTokenSet"] != true || holdsToken(body) || second == first || len(second) != 32 {
		t.Errorf("made again: %d %v", status, body)
	}
	if _, body, _ := administrator.call(http.MethodPost, "/settings/secrets/metricsToken/reveal", nil); body["value"] != second {
		t.Errorf("revealed after: %v", body)
	}
}
