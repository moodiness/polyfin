package server

import (
	"bufio"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/moodiness/polyfin/internal/metrics"
)

// Requests count by API and class of status, those under the /jellyfin
// base by the API that answers them; a connection taken over, as a
// WebSocket's, is still taken over through the count, and counts as
// switching protocols.
func TestRequestsCountByAPIAndStatus(t *testing.T) {
	jellyfin := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/socket" {
			http.NotFound(w, r)
			return
		}
		conn, buffered, err := http.NewResponseController(w).Hijack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotImplemented)
			return
		}
		defer conn.Close()
		_, _ = buffered.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		_ = buffered.Flush()
	})
	h := New(Options{
		Database:      database{},
		Admin:         fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}},
		AdminAPI:      http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }),
		Jellyfin:      jellyfin,
		Web:           webFiles,
		SetupRequired: setupState{}.SetupRequired,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	count := func(api, status string) float64 {
		return metrics.Value("polyfin_http_request_duration_seconds", map[string]string{"api": api, "status": status})
	}
	for _, c := range []struct {
		target, api, status string
	}{
		{"/Items/missing", metrics.APIJellyfin, "4xx"},
		{"/admin/api/users", metrics.APIAdmin, "4xx"},
		{"/admin/", metrics.APIAdmin, "2xx"},
		{"/healthz", metrics.APIOther, "2xx"},
		{"/jellyfin/web/main.jellyfin.bundle.js", metrics.APIWeb, "2xx"},
		{"/jellyfin/System/missing", metrics.APIJellyfin, "4xx"},
		// The admin app is at the root only: the Jellyfin API answers.
		{"/jellyfin/admin/api/users", metrics.APIJellyfin, "4xx"},
	} {
		before := count(c.api, c.status)
		get(h, c.target)
		if got := count(c.api, c.status); got != before+1 {
			t.Errorf("%s: %v %s requests, before %v", c.target, got, c.status, before)
		}
	}

	server := httptest.NewServer(h)
	defer server.Close()
	before := count(metrics.APIJellyfin, "1xx")
	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = io.WriteString(conn, "GET /socket HTTP/1.1\r\nHost: polyfin\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "HTTP/1.1 101") {
		t.Fatalf("the socket's answer: %q %v", line, err)
	}
	// The request counts once its handler returns.
	deadline := time.Now().Add(5 * time.Second)
	for count(metrics.APIJellyfin, "1xx") != before+1 {
		if time.Now().After(deadline) {
			t.Fatalf("the socket counted %v, before %v", count(metrics.APIJellyfin, "1xx"), before)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
