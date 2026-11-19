package stremio

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// An API that answers a sign-in with a 307 to another host, here the same
// server named localhost rather than 127.0.0.1, is not sent the email and
// password again there: the sign-in fails as unreachable instead.
func TestASignInIsNotSentAgainToAnotherHost(t *testing.T) {
	t.Parallel()
	var (
		mu     sync.Mutex
		bodies []string
	)
	var elsewhere string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Host, "127.0.0.1:") {
			w.Header().Set("Location", elsewhere+r.URL.RequestURI())
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		mu.Unlock()
		_, _ = io.WriteString(w, `{"result":{"authKey":"auth-key"}}`)
	}))
	t.Cleanup(server.Close)
	elsewhere = strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	r := New(Options{Version: "1.2.3", API: server.URL, Link: server.URL})

	if _, err := r.SignIn(t.Context(), "someone@example.com", "correct horse", false); !errors.Is(err, ErrUnreachable) {
		t.Errorf("a sign-in redirected to another host: %v, want ErrUnreachable", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 0 {
		t.Errorf("the other host was sent %q", bodies)
	}
}
