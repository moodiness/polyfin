package admin

import (
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/mediasegments"
)

// The PublicMetaDB key: checked with PublicMetaDB before it is saved, never
// answered, kept when a PUT leaves it out, and removed by an empty one.
func TestSettingsPublicMetaDBKey(t *testing.T) {
	var status, asked atomic.Int32
	var authorization atomic.Value
	status.Store(http.StatusOK)
	publicMetaDB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		authorization.Store(r.Header.Get("Authorization"))
		w.WriteHeader(int(status.Load()))
		_, _ = io.WriteString(w, `{"items":[],"total":0,"page":1,"perPage":1,"totalPages":0}`)
	}))
	t.Cleanup(publicMetaDB.Close)
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		o.Segments = mediasegments.New(deps.pool, []mediasegments.Source{{Name: mediasegments.PublicMetaDB, URL: publicMetaDB.URL}},
			"test", o.Logger, o.Accounts.Settings)
	})
	administrator := api.signedIn("administrator", true)
	const key = "pm-Xk9mN2pQrS7tUvWx3yZaB4cD5eF6gH7iJ8kL9mN0oP1qR2sT3uV4wXyZ5aB"
	// holdsKey reports whether an answer holds the key, under any field.
	holdsKey := func(body map[string]any) bool {
		encoded, _ := json.Marshal(body)
		_, field := body["publicMetaDbKey"]
		return field || strings.Contains(string(encoded), key)
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["publicMetaDbKeySet"] != false || holdsKey(body) {
		t.Errorf("default: %v", body["publicMetaDbKeySet"])
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	set := maps.Clone(base)
	set["publicMetaDbKey"] = key

	// A key PublicMetaDB refuses, or cannot be asked about, is not saved.
	for answer, want := range map[int]struct {
		status int
		code   string
	}{
		http.StatusUnauthorized:        {http.StatusBadRequest, "invalid_publicmetadb_key"},
		http.StatusForbidden:           {http.StatusBadRequest, "invalid_publicmetadb_key"},
		http.StatusInternalServerError: {http.StatusBadGateway, "publicmetadb_unreachable"},
		http.StatusServiceUnavailable:  {http.StatusBadGateway, "publicmetadb_unreachable"},
	} {
		status.Store(int32(answer))
		if got, body, _ := administrator.call(http.MethodPut, "/settings", set); got != want.status || body["error"] != want.code {
			t.Errorf("PublicMetaDB answering %d: %d %v", answer, got, body)
		}
	}
	// A malformed key is refused without asking PublicMetaDB.
	status.Store(http.StatusOK)
	before := asked.Load()
	for _, malformed := range []string{"   ", "pm-" + strings.Repeat("a", 254), "pm-k\u00e9y", "pm-\x01"} {
		refused := maps.Clone(base)
		refused["publicMetaDbKey"] = malformed
		if got, body, _ := administrator.call(http.MethodPut, "/settings", refused); got != http.StatusBadRequest || body["error"] != "invalid_publicmetadb_key" {
			t.Errorf("%q: %d %v", malformed, got, body)
		}
	}
	if asked.Load() != before || api.store.Settings().PublicMetaDBKey != "" {
		t.Fatalf("refused keys: PublicMetaDB asked %d times, key saved: %t", asked.Load()-before, api.store.Settings().PublicMetaDBKey != "")
	}

	// An accepted key, pasted with spaces around it, is saved, and the
	// answer only tells that one is.
	set["publicMetaDbKey"] = " " + key + "\n"
	if got, body, _ := administrator.call(http.MethodPut, "/settings", set); got != http.StatusOK || body["publicMetaDbKeySet"] != true || holdsKey(body) {
		t.Fatalf("accepted key: %d %v", got, body["error"])
	}
	if authorization.Load() != "Bearer "+key || api.store.Settings().PublicMetaDBKey != key {
		t.Error("the key checked is not the key saved")
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["publicMetaDbKeySet"] != true || holdsKey(body) {
		t.Errorf("saved: %v", body["publicMetaDbKeySet"])
	}
	// Left out, the key is kept.
	if got, body, _ := administrator.call(http.MethodPut, "/settings", base); got != http.StatusOK || body["publicMetaDbKeySet"] != true {
		t.Errorf("left out: %d %v", got, body)
	}
	// Another key PublicMetaDB cannot be reached about leaves the saved one.
	publicMetaDB.Close()
	set["publicMetaDbKey"] = "pm-Other"
	if got, body, _ := administrator.call(http.MethodPut, "/settings", set); got != http.StatusBadGateway || body["error"] != "publicmetadb_unreachable" ||
		api.store.Settings().PublicMetaDBKey != key {
		t.Errorf("unreachable: %d %v", got, body)
	}
	// Empty removes it, without asking PublicMetaDB.
	removed := maps.Clone(base)
	removed["publicMetaDbKey"] = ""
	if got, body, _ := administrator.call(http.MethodPut, "/settings", removed); got != http.StatusOK || body["publicMetaDbKeySet"] != false ||
		api.store.Settings().PublicMetaDBKey != "" {
		t.Errorf("removed: %d %v", got, body)
	}
}
