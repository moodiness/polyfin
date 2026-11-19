package jellyfinimport

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// A server, or plex.tv, that redirects to another host, here the same
// server named localhost rather than 127.0.0.1, is sent there no key nor
// token, as Jellyfin's in Authorization is not, while the request goes on;
// a sign-in answered with a 307 there fails rather than send the password.
func TestAServersKeyAndPasswordGoToNoOtherHost(t *testing.T) {
	t.Parallel()
	var (
		mu     sync.Mutex
		landed = map[string]http.Header{}
		bodies []string
	)
	var elsewhere string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Host, "127.0.0.1:") {
			// A 307 for the sign-in, which would send its body again.
			code := http.StatusFound
			if r.Method == http.MethodPost {
				code = http.StatusTemporaryRedirect
			}
			w.Header().Set("Location", elsewhere+r.URL.RequestURI())
			w.WriteHeader(code)
			return
		}
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		landed[r.URL.Path] = r.Header.Clone()
		bodies = append(bodies, string(body))
		mu.Unlock()
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(server.Close)
	elsewhere = strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	s := New(Options{Version: "1.2.3", Logger: slog.New(slog.DiscardHandler)})
	t.Cleanup(s.Close)

	for _, kind := range []Kind{Jellyfin, Emby, Plex} {
		c := &client{s: s, kind: kind, address: server.URL + "/" + string(kind), key: goodKey}
		if status, _, _, err := c.do(t.Context(), http.MethodGet, "/System/Info", nil, nil); err != nil || status != http.StatusOK {
			t.Fatalf("%s redirected: %d, %v", kind, status, err)
		}
	}
	if status, err := s.plexTVDo(t.Context(), http.MethodGet, server.URL+"/plex.tv/api/v2/user", nil, plexToken, nil); err != nil ||
		status != http.StatusOK {
		t.Fatalf("plex.tv redirected: %d, %v", status, err)
	}
	signIn := &client{s: s, kind: Emby, address: server.URL + "/sign-in"}
	if _, err := signIn.signIn(t.Context(), "alice", "alice's password"); err == nil {
		t.Error("a sign-in redirected to another host succeeded")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(landed) != 4 {
		t.Errorf("the other host was asked %d paths, want the 4 reads: %v", len(landed), landed)
	}
	for path, header := range landed {
		for _, name := range []string{"Authorization", "X-Emby-Token", "X-Plex-Token"} {
			if value := header.Get(name); strings.Contains(value, goodKey) || strings.Contains(value, plexToken) {
				t.Errorf("%s was sent %s: %q", path, name, value)
			}
		}
	}
	for _, body := range bodies {
		if strings.Contains(body, "alice's password") {
			t.Errorf("the other host was sent the password: %s", body)
		}
	}
}
