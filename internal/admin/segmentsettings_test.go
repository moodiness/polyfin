package admin

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/mediasegments"
)

// names reads a JSON list of names.
func names(value any) []string {
	list, _ := value.([]any)
	out := []string{}
	for _, item := range list {
		name, _ := item.(string)
		out = append(out, name)
	}
	return out
}

// The order of the segment databases: the one in effect always lists all
// three, those POLYFIN_SEGMENTS turns off after the others; a saved one
// replaces it until an empty one goes back to POLYFIN_SEGMENTS'.
func TestSettingsSegmentOrder(t *testing.T) {
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		// POLYFIN_SEGMENTS=introdb,theintrodb
		o.Segments = mediasegments.New(deps.pool, mediasegments.Sources([]string{"introdb", "theintrodb"}), "test", o.Logger, o.Accounts.Settings)
	})
	administrator := api.signedIn("administrator", true)
	byDefault := []string{"introdb", "theintrodb", "publicmetadb"}
	order := func(body map[string]any) ([]string, []string, []string) {
		return names(body["segmentOrder"]), names(body["segmentOrderDefault"]), names(body["segmentSourcesOff"])
	}
	_, body, _ := administrator.call(http.MethodGet, "/settings", nil)
	if inEffect, def, off := order(body); !slices.Equal(inEffect, byDefault) || !slices.Equal(def, byDefault) || !slices.Equal(off, []string{"publicmetadb"}) {
		t.Errorf("default: %v, %v, %v", inEffect, def, off)
	}

	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	saved := []string{"publicmetadb", "theintrodb", "introdb"}
	set := maps.Clone(base)
	set["segmentOrder"] = saved
	if status, body, _ := administrator.call(http.MethodPut, "/settings", set); status != http.StatusOK {
		t.Fatalf("saving an order: %d %v", status, body)
	} else if inEffect, def, _ := order(body); !slices.Equal(inEffect, saved) || !slices.Equal(def, byDefault) {
		t.Errorf("saved: %v, default %v", inEffect, def)
	}
	if got := api.store.Settings().SegmentOrder; !slices.Equal(got, saved) {
		t.Errorf("stored %v", got)
	}
	// Left out, or null, the order is kept.
	keep := maps.Clone(base)
	keep["segmentOrder"] = nil
	for _, body := range []map[string]any{base, keep} {
		if status, answer, _ := administrator.call(http.MethodPut, "/settings", body); status != http.StatusOK || !slices.Equal(names(answer["segmentOrder"]), saved) {
			t.Errorf("left out: %d %v", status, answer["segmentOrder"])
		}
	}
	for _, refused := range [][]string{
		{"theintrodb"},
		{"theintrodb", "introdb"},
		{"theintrodb", "introdb", "introdb"},
		{"theintrodb", "introdb", "publicmetadb", "introdb"},
		{"theintrodb", "introdb", "other"},
		{"TheIntroDB", "introdb", "publicmetadb"},
	} {
		body := maps.Clone(base)
		body["segmentOrder"] = refused
		if status, answer, _ := administrator.call(http.MethodPut, "/settings", body); status != http.StatusBadRequest || answer["error"] != "invalid_segment_order" {
			t.Errorf("%v: %d %v", refused, status, answer)
		}
	}
	if got := api.store.Settings().SegmentOrder; !slices.Equal(got, saved) {
		t.Errorf("refused orders changed it: %v", got)
	}
	// Empty goes back to POLYFIN_SEGMENTS' order.
	reset := maps.Clone(base)
	reset["segmentOrder"] = []string{}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", reset); status != http.StatusOK || !slices.Equal(names(body["segmentOrder"]), byDefault) {
		t.Errorf("reset: %d %v", status, body["segmentOrder"])
	}
	if got := api.store.Settings().SegmentOrder; len(got) != 0 {
		t.Errorf("stored after a reset: %v", got)
	}
}

// The optional TheIntroDB key: checked with TheIntroDB before it is saved,
// kept when a PUT leaves it out, removed by an empty one; and neither key is
// ever in an answer or the log.
func TestSettingsTheIntroDBKey(t *testing.T) {
	var status atomic.Int32
	status.Store(http.StatusOK)
	var mu sync.Mutex
	var authorizations []string
	theIntroDB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authorizations = append(authorizations, r.Header.Get("Authorization"))
		mu.Unlock()
		w.WriteHeader(int(status.Load()))
		_, _ = io.WriteString(w, `{"error":"no data found for this tmdb_id"}`)
	}))
	t.Cleanup(theIntroDB.Close)
	publicMetaDB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"items":[]}`)
	}))
	t.Cleanup(publicMetaDB.Close)
	var log bytes.Buffer
	var logMu sync.Mutex
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		o.Logger = slog.New(slog.NewTextHandler(lockedWriter{&logMu, &log}, nil))
		o.Segments = mediasegments.New(deps.pool, []mediasegments.Source{
			{Name: mediasegments.TheIntroDB, URL: theIntroDB.URL}, {Name: mediasegments.PublicMetaDB, URL: publicMetaDB.URL},
		}, "test", o.Logger, o.Accounts.Settings)
	})
	administrator := api.signedIn("administrator", true)
	const key, publicMetaDBKey = "tidb-Secret0123456789", "pm-Secret0123456789"
	holdsKey := func(body map[string]any) bool {
		encoded, _ := json.Marshal(body)
		_, theIntroDBField := body["theIntroDbKey"]
		_, publicMetaDBField := body["publicMetaDbKey"]
		return theIntroDBField || publicMetaDBField || strings.Contains(string(encoded), key) || strings.Contains(string(encoded), publicMetaDBKey)
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["theIntroDbKeySet"] != false || holdsKey(body) {
		t.Errorf("default: %v", body["theIntroDbKeySet"])
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	set := maps.Clone(base)
	set["theIntroDbKey"] = key

	for answer, want := range map[int]struct {
		status int
		code   string
	}{
		http.StatusUnauthorized:        {http.StatusBadRequest, "invalid_theintrodb_key"},
		http.StatusForbidden:           {http.StatusBadRequest, "invalid_theintrodb_key"},
		http.StatusTooManyRequests:     {http.StatusBadGateway, "theintrodb_unreachable"},
		http.StatusInternalServerError: {http.StatusBadGateway, "theintrodb_unreachable"},
	} {
		status.Store(int32(answer))
		if got, body, _ := administrator.call(http.MethodPut, "/settings", set); got != want.status || body["error"] != want.code {
			t.Errorf("TheIntroDB answering %d: %d %v", answer, got, body)
		}
	}
	mu.Lock()
	asked := len(authorizations)
	mu.Unlock()
	for _, malformed := range []string{"  ", strings.Repeat("k", 257), "tidb-k\u00e9y"} {
		body := maps.Clone(base)
		body["theIntroDbKey"] = malformed
		if got, answer, _ := administrator.call(http.MethodPut, "/settings", body); got != http.StatusBadRequest || answer["error"] != "invalid_theintrodb_key" {
			t.Errorf("%q: %d %v", malformed, got, answer)
		}
	}
	mu.Lock()
	if len(authorizations) != asked || api.store.Settings().TheIntroDBKey != "" {
		t.Fatalf("refused keys: %d more requests, key saved: %t", len(authorizations)-asked, api.store.Settings().TheIntroDBKey != "")
	}
	mu.Unlock()

	// TheIntroDB knowing nothing of the movie checked still accepts the
	// key, saved with the PublicMetaDB one.
	status.Store(http.StatusNotFound)
	set["publicMetaDbKey"] = publicMetaDBKey
	if got, body, _ := administrator.call(http.MethodPut, "/settings", set); got != http.StatusOK || body["theIntroDbKeySet"] != true ||
		body["publicMetaDbKeySet"] != true || holdsKey(body) {
		t.Fatalf("accepted: %d %v", got, body["error"])
	}
	mu.Lock()
	if authorizations[len(authorizations)-1] != "Bearer "+key || api.store.Settings().TheIntroDBKey != key {
		t.Error("the key checked is not the key saved")
	}
	mu.Unlock()
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["theIntroDbKeySet"] != true || holdsKey(body) {
		t.Errorf("saved: %v", body["theIntroDbKeySet"])
	}
	if got, body, _ := administrator.call(http.MethodPut, "/settings", base); got != http.StatusOK || body["theIntroDbKeySet"] != true {
		t.Errorf("left out: %d %v", got, body)
	}
	// A key refused while the other is accepted saves neither.
	status.Store(http.StatusUnauthorized)
	both := maps.Clone(base)
	both["theIntroDbKey"], both["publicMetaDbKey"] = "tidb-Other", "pm-Other"
	if got, body, _ := administrator.call(http.MethodPut, "/settings", both); got != http.StatusBadRequest || body["error"] != "invalid_theintrodb_key" ||
		api.store.Settings().PublicMetaDBKey != publicMetaDBKey || api.store.Settings().TheIntroDBKey != key {
		t.Errorf("one refused: %d %v", got, body)
	}
	removed := maps.Clone(base)
	removed["theIntroDbKey"] = ""
	if got, body, _ := administrator.call(http.MethodPut, "/settings", removed); got != http.StatusOK || body["theIntroDbKeySet"] != false ||
		api.store.Settings().TheIntroDBKey != "" {
		t.Errorf("removed: %d %v", got, body)
	}
	logMu.Lock()
	defer logMu.Unlock()
	if strings.Contains(log.String(), "Secret0123456789") || strings.Contains(log.String(), "tidb-Other") || strings.Contains(log.String(), "pm-Other") {
		t.Errorf("a key was logged: %s", log.String())
	}
}

// lockedWriter writes to w under mu, for a log read while requests run.
type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
