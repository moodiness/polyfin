package ratings

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// MDBList's key goes in the query of each request: the host an answer
// redirects to is not told the address asked.
func TestARedirectOfMDBListIsNotToldTheKey(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var received []http.Header
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = append(received, r.Header.Clone())
		mu.Unlock()
		_, _ = io.WriteString(w, `{"api_requests": 1000, "api_requests_count": 276, "user_id": 3, "plan": "Free",
			"rate_limit": 1000, "rate_limit_remaining": 724, "rate_limit_reset": 1706918400, "auth_method": "apikey"}`)
	}))
	t.Cleanup(other.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" || r.URL.Query().Get("apikey") != testKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, other.URL+"/user", http.StatusFound)
	}))
	t.Cleanup(api.Close)
	s := New(migrated.New(t), api.URL, "test", slog.New(slog.NewTextHandler(io.Discard, nil)),
		func() accounts.Settings { return accounts.Settings{MDBListRatingsKey: testKey} })
	t.Cleanup(s.Close)
	if err := s.CheckKey(t.Context(), testKey); err != nil {
		t.Fatalf("a redirected answer: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("the other host was asked %d times, want 1", len(received))
	}
	if referer, set := received[0]["Referer"]; set {
		t.Errorf("the other host was sent Referer %q", referer)
	}
}
