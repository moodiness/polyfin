package metrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// /metrics answers 404 while off, 401 without the token or with another,
// and the metrics with it.
func TestHandlerAnswersOnlyWithTheToken(t *testing.T) {
	const token = "0123456789abcdef0123456789abcdef"
	for _, c := range []struct {
		name          string
		on            bool
		stored, given string
		status        int
	}{
		{"off", false, token, "Bearer " + token, http.StatusNotFound},
		{"no token", true, token, "", http.StatusUnauthorized},
		{"another token", true, token, "Bearer 0123456789abcdef0123456789abcdee", http.StatusUnauthorized},
		{"the token as a password", true, token, "Basic " + token, http.StatusUnauthorized},
		{"a token the key cannot read", true, "", "Bearer ", http.StatusUnauthorized},
		{"the token", true, token, "Bearer " + token, http.StatusOK},
		{"the token, the scheme in lower case", true, token, "bearer " + token, http.StatusOK},
	} {
		handler := Handler(func() (bool, string) { return c.on, c.stored })
		request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		if c.given != "" {
			request.Header.Set("Authorization", c.given)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != c.status {
			t.Errorf("%s: %d, want %d", c.name, response.Code, c.status)
			continue
		}
		body := response.Body.String()
		if c.status == http.StatusOK && !strings.Contains(body, "go_goroutines ") {
			t.Errorf("%s: the answer holds no metric: %s", c.name, body)
		}
		if c.status != http.StatusOK && strings.Contains(body, "go_goroutines") {
			t.Errorf("%s: metrics answered", c.name)
		}
	}
}

// The requests of addons are counted under the name each is listed with,
// those of addons sharing a name together, never by address; those of
// addons removed since are no longer told.
func TestAddonRequestsAreNamedAsListed(t *testing.T) {
	const listed, other, gone = "https://listed.example/secret-config", "https://mirror.example/other-config", "https://gone.example/config"
	AddonRequest(listed, "stream", 2*time.Second, false)
	AddonRequest(listed, "stream", 3*time.Second, false)
	AddonRequest(listed, "catalog", 200*time.Millisecond, true)
	AddonRequest(other, "stream", time.Second, false)
	AddonRequest(gone, "meta", time.Second, true)
	AddonNames(func(context.Context) (map[string]string, error) {
		return map[string]string{listed: "Shared addon", other: "Shared addon"}, nil
	})
	t.Cleanup(func() { AddonNames(nil) })
	for _, c := range []struct {
		resource, outcome string
		want              float64
	}{
		{"stream", Failure, 3},
		{"catalog", Success, 1},
		{"stream", Success, 0},
	} {
		if got := Value("polyfin_addon_request_duration_seconds", map[string]string{"addon": "Shared addon", "resource": c.resource,
			"outcome": c.outcome}); got != c.want {
			t.Errorf("%s %s: %v, want %v", c.resource, c.outcome, got, c.want)
		}
	}
	handler := Handler(func() (bool, string) { return true, "token" })
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	body, _ := io.ReadAll(response.Body)
	for _, hidden := range []string{"listed.example", "secret-config", "mirror.example", "gone.example", `resource="meta"`} {
		if strings.Contains(string(body), hidden) {
			t.Errorf("the answer holds %q", hidden)
		}
	}
	if !strings.Contains(string(body), `polyfin_addon_request_duration_seconds_bucket{addon="Shared addon",outcome="failure",resource="stream",le="2.5"} 2`) {
		t.Errorf("the durations of the failures: %s", body)
	}
}
