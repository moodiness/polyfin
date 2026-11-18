package trackers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// MDBList's key goes in the query of each request, as Last.fm's does: the
// host an answer redirects to is not told the address asked.
func TestARedirectOfAServiceIsNotToldTheKey(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	user := h.user(t, "dave")
	var mu sync.Mutex
	var received []http.Header
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = append(received, r.Header.Clone())
		mu.Unlock()
		_, _ = io.WriteString(w, `{"user_id":3,"username":"dave-on-mdblist","rate_limit":1000}`)
	}))
	t.Cleanup(other.Close)
	h.f.on("GET", "/mdblist/user", func(r request) answer {
		if r.query.Get("apikey") != "mdblist-good" {
			return answer{status: http.StatusUnauthorized, body: `{"error":"Invalid API key"}`}
		}
		return answer{status: http.StatusFound, body: `{}`, header: http.Header{"Location": {other.URL + "/user"}}}
	})
	status, err := h.ConnectKey(t.Context(), user, MDBList, "mdblist-good")
	if err != nil || !status.Connected || status.Account != "dave-on-mdblist" {
		t.Fatalf("a redirected answer: %+v %v", status, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(received) == 0 {
		t.Fatal("the other host was not asked")
	}
	for _, header := range received {
		if referer, set := header["Referer"]; set {
			t.Errorf("the other host was sent Referer %q", referer)
		}
	}
}
