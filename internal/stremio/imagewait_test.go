package stremio

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// An artwork server that answers 429, or 503, with a Retry-After asks to
// wait that long, as an addon does; another failure asks nothing.
func TestAnArtworkServerAsksToWait(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/busy.jpg":
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
		case "/down.jpg":
			w.Header().Set("Retry-After", "45")
			w.WriteHeader(http.StatusServiceUnavailable)
		case "/missing.jpg":
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	t.Cleanup(server.Close)
	client := NewClient("test")
	for path, want := range map[string]time.Duration{"/busy.jpg": 30 * time.Second, "/down.jpg": 45 * time.Second, "/missing.jpg": 0, "/failing.jpg": 0} {
		_, _, err := client.Image(t.Context(), server.URL+path, false)
		if wait, waits := RetryAfter(err); wait != want || waits != (want > 0) {
			t.Errorf("%s: wait %v %v (%v), want %v", path, wait, waits, err, want)
		}
	}
}
