package admin

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/tmdb"
)

// The TMDB key: empty by default, checked with TMDB before it is saved,
// kept when a PUT leaves it out, removed by an empty one, read again on
// demand, and never in an answer or the log.
func TestSettingsTMDBKey(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var requests []string
	unreachable := false
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.Path+" "+r.URL.Query().Get("api_key"))
		down := unreachable
		mu.Unlock()
		switch {
		case down:
			w.WriteHeader(http.StatusServiceUnavailable)
		case r.URL.Query().Get("api_key") == "tmdb-Secret0123456789":
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	t.Cleanup(fake.Close)
	var log bytes.Buffer
	var logMu sync.Mutex
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Logger = slog.New(slog.NewTextHandler(lockedWriter{&logMu, &log}, nil))
		o.TMDB = tmdb.New(fake.URL+"/3", tmdb.Images, "test", o.Accounts.Settings)
	})
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	const key = "tmdb-Secret0123456789"
	holdsKey := func(body map[string]any) bool {
		encoded, _ := json.Marshal(body)
		_, field := body["tmdbKey"]
		return field || strings.Contains(string(encoded), key)
	}
	asked := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(requests)
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["tmdbKeySet"] != false || holdsKey(body) {
		t.Errorf("default: %v", body["tmdbKeySet"])
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	with := func(value string) map[string]any {
		body := map[string]any{"tmdbKey": value}
		for name, v := range base {
			body[name] = v
		}
		return body
	}
	for _, malformed := range []string{"with space", strings.Repeat("k", 513)} {
		if status, body, _ := administrator.call(http.MethodPut, "/settings", with(malformed)); status != http.StatusBadRequest || body["error"] != "invalid_tmdb_key" {
			t.Errorf("malformed %q: %d %v", malformed[:min(len(malformed), 12)], status, body)
		}
	}
	if asked() != 0 {
		t.Errorf("malformed keys were sent to TMDB: %d requests", asked())
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", with("tmdb-Wrong")); status != http.StatusBadRequest || body["error"] != "invalid_tmdb_key" {
		t.Errorf("refused key: %d %v", status, body)
	}
	mu.Lock()
	unreachable = true
	mu.Unlock()
	if status, body, _ := administrator.call(http.MethodPut, "/settings", with(key)); status != http.StatusBadGateway || body["error"] != "tmdb_unreachable" {
		t.Errorf("unreachable TMDB: %d %v", status, body)
	}
	mu.Lock()
	unreachable = false
	mu.Unlock()
	if api.store.Settings().TMDBKey != "" {
		t.Fatal("a key TMDB did not accept was saved")
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", with(" "+key+" ")); status != http.StatusOK || body["tmdbKeySet"] != true || holdsKey(body) {
		t.Fatalf("saving the key: %d %v", status, body)
	}
	if api.store.Settings().TMDBKey != key || requests[len(requests)-1] != "/3/authentication "+key {
		t.Errorf("saved %t, last request %q", api.store.Settings().TMDBKey == key, requests[len(requests)-1])
	}
	// Left out, it is kept; the same key again is not checked again.
	saved := asked()
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || body["tmdbKeySet"] != true {
		t.Errorf("left out: %d %v", status, body["tmdbKeySet"])
	}
	if status, _, _ := administrator.call(http.MethodPut, "/settings", with(key)); status != http.StatusOK || asked() != saved {
		t.Errorf("the same key: %d, %d more requests", status, asked()-saved)
	}
	if status, body, _ := administrator.call(http.MethodPost, "/settings/secrets/tmdbKey/reveal", nil); status != http.StatusOK || body["value"] != key {
		t.Errorf("reveal: %d", status)
	}
	if status, _, _ := member.call(http.MethodPost, "/settings/secrets/tmdbKey/reveal", nil); status != http.StatusForbidden {
		t.Errorf("member reveal: %d", status)
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", with("")); status != http.StatusOK || body["tmdbKeySet"] != false ||
		api.store.Settings().TMDBKey != "" {
		t.Errorf("removed: %d %v", status, body["tmdbKeySet"])
	}
	logMu.Lock()
	defer logMu.Unlock()
	if strings.Contains(log.String(), key) || strings.Contains(log.String(), "tmdb-Wrong") {
		t.Error("a key is in the log")
	}
}
