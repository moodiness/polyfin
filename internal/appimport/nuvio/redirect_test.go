package nuvio

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// A backend that redirects to another host, here the same server named
// localhost rather than 127.0.0.1, is sent there neither its publishable
// key nor, on a 307, the email and password: the sign-in fails instead.
func TestTheBackendsKeyAndPasswordGoToNoOtherHost(t *testing.T) {
	t.Parallel()
	var (
		mu     sync.Mutex
		landed []*http.Request
		bodies []string
	)
	var elsewhere string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		first := strings.HasPrefix(r.Host, "127.0.0.1:")
		switch {
		case first && r.URL.Path == "/.well-known/nuvio":
			_, _ = io.WriteString(w, `{"publishable_key":"publishable-key"}`)
		case first && r.URL.Path == "/auth/v1/token":
			w.Header().Set("Location", elsewhere+r.URL.RequestURI())
			w.WriteHeader(http.StatusTemporaryRedirect)
		case first:
			w.Header().Set("Location", elsewhere+r.URL.RequestURI())
			w.WriteHeader(http.StatusFound)
		default:
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			landed, bodies = append(landed, r.Clone(r.Context())), append(bodies, string(body))
			mu.Unlock()
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(server.Close)
	elsewhere = strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	r := New(Options{Version: "1.2.3"})

	s := &session{r: r, backend: server.URL, client: r.client, key: "publishable-key"}
	if status, _, _, err := s.do(t.Context(), http.MethodGet, "/rest/v1/profiles", nil); err != nil || status != http.StatusOK {
		t.Fatalf("a read redirected: %d, %v", status, err)
	}
	if _, err := r.Read(t.Context(), server.URL, email, password, false, false); err == nil {
		t.Error("a sign-in redirected to another host succeeded")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(landed) != 1 || landed[0].URL.Path != "/rest/v1/profiles" {
		t.Fatalf("the other host was asked %d times, want once, for the read", len(landed))
	}
	if key := landed[0].Header.Values("apikey"); len(key) != 0 {
		t.Errorf("the other host was sent the key %q", key)
	}
	for _, body := range bodies {
		if strings.Contains(body, password) {
			t.Errorf("the other host was sent the password: %s", body)
		}
	}
}
