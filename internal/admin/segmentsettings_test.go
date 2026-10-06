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

// The segment databases: every one of them in the order, each turned on or
// off. A save keeps what it leaves out, and an empty order, which older
// pages sent to follow POLYFIN_SEGMENTS, keeps the saved one too.
func TestSettingsSegmentSources(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	byDefault := []string{"theintrodb", "introdb", "publicmetadb"}
	sources := func(body map[string]any) ([]string, []string) {
		return names(body["segmentOrder"]), names(body["segmentSourcesOff"])
	}
	_, body, _ := administrator.call(http.MethodGet, "/settings", nil)
	if order, off := sources(body); !slices.Equal(order, byDefault) || len(off) != 0 {
		t.Errorf("default: %v, off %v", order, off)
	}
	if _, found := body["segmentOrderDefault"]; found {
		t.Error("the order POLYFIN_SEGMENTS gave is still answered")
	}

	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	saved, off := []string{"publicmetadb", "theintrodb", "introdb"}, []string{"introdb"}
	set := maps.Clone(base)
	set["segmentOrder"], set["segmentSourcesOff"] = saved, off
	if status, body, _ := administrator.call(http.MethodPut, "/settings", set); status != http.StatusOK {
		t.Fatalf("saving the sources: %d %v", status, body)
	} else if order, gotOff := sources(body); !slices.Equal(order, saved) || !slices.Equal(gotOff, off) {
		t.Errorf("saved: %v, off %v", order, gotOff)
	}
	if got := api.store.Settings(); !slices.Equal(got.SegmentOrder, saved) || !slices.Equal(got.SegmentSourcesOff, off) {
		t.Errorf("stored %v, off %v", got.SegmentOrder, got.SegmentSourcesOff)
	}
	// Left out, null, or an empty order, they are kept.
	keep := maps.Clone(base)
	keep["segmentOrder"], keep["segmentSourcesOff"] = nil, nil
	older := maps.Clone(base)
	older["segmentOrder"] = []string{}
	for _, body := range []map[string]any{base, keep, older} {
		status, answer, _ := administrator.call(http.MethodPut, "/settings", body)
		if order, gotOff := sources(answer); status != http.StatusOK || !slices.Equal(order, saved) || !slices.Equal(gotOff, off) {
			t.Errorf("left out: %d %v, off %v", status, order, gotOff)
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
	for _, refused := range [][]string{{"other"}, {"introdb", "introdb"}} {
		body := maps.Clone(base)
		body["segmentSourcesOff"] = refused
		if status, answer, _ := administrator.call(http.MethodPut, "/settings", body); status != http.StatusBadRequest || answer["error"] != "invalid_segment_sources_off" {
			t.Errorf("off %v: %d %v", refused, status, answer)
		}
	}
	if got := api.store.Settings(); !slices.Equal(got.SegmentOrder, saved) || !slices.Equal(got.SegmentSourcesOff, off) {
		t.Errorf("refused sources changed them: %v, off %v", got.SegmentOrder, got.SegmentSourcesOff)
	}
}

// The optional TheIntroDB key: checked with TheIntroDB before it is saved,
// kept when a PUT leaves it out, removed by an empty one; and neither key is
// ever in an answer or the log.
func TestSettingsTheIntroDBKey(t *testing.T) {
	var status atomic.Int32
	var mu sync.Mutex
	var requests []string
	// TheIntroDB answers a submission with an empty body 400 once it
	// accepts the key, or as status says.
	theIntroDB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
		mu.Unlock()
		w.WriteHeader(int(status.Load()))
		_, _ = io.WriteString(w, `{"error":"invalid request body"}`)
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
		// An answer that does not tell whether the key is accepted.
		http.StatusOK: {http.StatusBadGateway, "theintrodb_unreachable"},
	} {
		status.Store(int32(answer))
		if got, body, _ := administrator.call(http.MethodPut, "/settings", set); got != want.status || body["error"] != want.code {
			t.Errorf("TheIntroDB answering %d: %d %v", answer, got, body)
		}
	}
	mu.Lock()
	asked := len(requests)
	mu.Unlock()
	for _, malformed := range []string{"  ", strings.Repeat("k", 257), "tidb-k\u00e9y"} {
		body := maps.Clone(base)
		body["theIntroDbKey"] = malformed
		if got, answer, _ := administrator.call(http.MethodPut, "/settings", body); got != http.StatusBadRequest || answer["error"] != "invalid_theintrodb_key" {
			t.Errorf("%q: %d %v", malformed, got, answer)
		}
	}
	mu.Lock()
	if len(requests) != asked || api.store.Settings().TheIntroDBKey != "" {
		t.Fatalf("refused keys: %d more requests, key saved: %t", len(requests)-asked, api.store.Settings().TheIntroDBKey != "")
	}
	mu.Unlock()

	// A key TheIntroDB accepts, refusing the empty submission instead, is
	// saved with the PublicMetaDB one.
	status.Store(http.StatusBadRequest)
	set["publicMetaDbKey"] = publicMetaDBKey
	if got, body, _ := administrator.call(http.MethodPut, "/settings", set); got != http.StatusOK || body["theIntroDbKeySet"] != true ||
		body["publicMetaDbKeySet"] != true || holdsKey(body) {
		t.Fatalf("accepted: %d %v", got, body["error"])
	}
	mu.Lock()
	if requests[len(requests)-1] != "POST /submit Bearer "+key || api.store.Settings().TheIntroDBKey != key {
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
