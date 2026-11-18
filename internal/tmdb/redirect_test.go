package tmdb

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// A key of the API's version 3 goes in the query of each request: the
// host an answer redirects to is not told the address asked.
func TestARedirectOfTMDBIsNotToldTheKey(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var received []http.Header
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = append(received, r.Header.Clone())
		mu.Unlock()
		_, _ = io.WriteString(w, `{"success": true}`)
	}))
	t.Cleanup(other.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != "k3y" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, other.URL+"/3/authentication", http.StatusFound)
	}))
	t.Cleanup(api.Close)
	c := New(api.URL+"/3", Images, "test", keyed(""))
	if err := c.CheckKey(t.Context(), "k3y"); err != nil {
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
