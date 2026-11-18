package jellyfin

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// noLink is a token of a share link's shape that is no link's, the nth.
func noLink(n int) string {
	raw := make([]byte, 48)
	raw[0], raw[47] = byte(n+1), 1
	return base64.RawURLEncoding.EncodeToString(raw)
}

// Share links that do not exist are counted apart from the sign-ins:
// as many as the limit refuse the next one, and never a sign-in. Text
// that is no token at all is refused without counting.
func TestUnknownShareLinksNeverRefuseASignIn(t *testing.T) {
	t.Parallel()
	const limit = 3
	s := newTestServer(t, limit)
	s.user("alice", nil)
	mux := http.NewServeMux()
	mux.Handle("/share/", s.handler.Share())
	mux.Handle("/", s.handler)
	site := httptest.NewServer(mux)
	t.Cleanup(site.Close)
	s.url = site.URL
	page := func(token string) int {
		t.Helper()
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, site.URL+"/share/"+token, nil)
		response, err := testClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		return response.StatusCode
	}
	signIn := func() int {
		t.Helper()
		status, _ := s.call(http.MethodPost, "/Users/AuthenticateByName", app("tv", ""), map[string]string{"Username": "alice", "Pw": "correct horse"})
		return status
	}
	for n := range 2 * limit {
		if status := page("x?" + strconv.Itoa(n)); status != http.StatusNotFound {
			t.Fatalf("text that is no token, %d: %d", n+1, status)
		}
	}
	if status := signIn(); status != http.StatusOK {
		t.Errorf("the right password after text that is no token: %d", status)
	}
	for n := range limit {
		if status := page(noLink(n)); status != http.StatusNotFound {
			t.Fatalf("unknown link %d: %d, text that is no token counted", n+1, status)
		}
	}
	if status := signIn(); status != http.StatusOK {
		t.Errorf("the right password after %d unknown links: %d", limit, status)
	}
	if status := page(noLink(limit)); status != http.StatusTooManyRequests {
		t.Errorf("unknown link %d: %d, want 429", limit+1, status)
	}
	if status := signIn(); status != http.StatusOK {
		t.Errorf("the right password once share links are refused: %d", status)
	}
}

// Wrong passwords, as many as refuse the next sign-in, refuse no share
// link.
func TestWrongPasswordsNeverRefuseAShareLink(t *testing.T) {
	t.Parallel()
	s := sharing(t)
	token, _ := s.link(t, 1)
	wrong := map[string]string{"Username": "nobody", "Pw": "wrong horse"}
	refused := false
	for range 20 {
		if status, _ := s.call(http.MethodPost, "/Users/AuthenticateByName", app("tv", ""), wrong); status == http.StatusTooManyRequests {
			refused = true
			break
		}
	}
	if !refused {
		t.Fatal("wrong passwords were never refused")
	}
	if status, _ := s.guest(t, http.MethodGet, "/share/"+token, nil); status != http.StatusOK {
		t.Errorf("the share link after too many wrong passwords: %d", status)
	}
	if status, session := s.start(t, token); status != http.StatusOK {
		t.Errorf("playing it: %d %s", status, session)
	}
}

// Share links that do not exist, asked for by a page of another site as
// its images, count nothing: a link still opens after as many as the
// limit. Opened from another site, as a link in a message is, they count,
// and the one after the limit is refused.
func TestUnknownShareLinksOfOtherSitesPagesCountNothing(t *testing.T) {
	t.Parallel()
	const limit = 10 // as the test server's
	s := sharing(t)
	token, _ := s.link(t, 1)
	sent := func(token, destination string) int {
		t.Helper()
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, s.site+"/share/"+token, nil)
		request.Header.Set("Sec-Fetch-Site", "cross-site")
		request.Header.Set("Sec-Fetch-Dest", destination)
		response, err := noRedirects.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		return response.StatusCode
	}
	for n := range limit {
		if status := sent(noLink(n), "image"); status != http.StatusNotFound {
			t.Fatalf("unknown link %d as an image: %d", n+1, status)
		}
	}
	if status, _ := s.guest(t, http.MethodGet, "/share/"+token, nil); status != http.StatusOK {
		t.Errorf("the share link after %d unknown ones as images: %d", limit, status)
	}
	for n := range limit {
		if status := sent(noLink(n), "document"); status != http.StatusNotFound {
			t.Fatalf("unknown link %d opened from another site: %d", n+1, status)
		}
	}
	if status := sent(noLink(limit), "document"); status != http.StatusTooManyRequests {
		t.Errorf("unknown link %d opened from another site: %d, want 429", limit+1, status)
	}
}
