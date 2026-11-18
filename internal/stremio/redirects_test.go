package stremio

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// redirected keeps the headers of the requests the servers of a test
// received, by the name the test gives each place.
type redirected struct {
	mu      sync.Mutex
	headers map[string][]http.Header
}

func (r *redirected) add(place string, header http.Header) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.headers == nil {
		r.headers = map[string][]http.Header{}
	}
	r.headers[place] = append(r.headers[place], header.Clone())
}

func (r *redirected) at(place string) []http.Header {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.headers[place]
}

// redirectingHosts starts a provider and another host. The provider
// answers /hop/N with a redirect to /hop/N-1 on itself down to /hop/0,
// /same/... with a redirect to /edge on itself, and any other path with a
// redirect to /edge on the other host; /edge answers an addon's empty list
// of streams on both.
func redirectingHosts(t *testing.T) (provider *httptest.Server, got *redirected) {
	t.Helper()
	got = &redirected{}
	answer := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"streams":[]}`)
	}
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.add("other", r.Header)
		answer(w)
	}))
	t.Cleanup(other.Close)
	provider = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch hop, isHop := strings.CutPrefix(r.URL.Path, "/hop/"); {
		case r.URL.Path == "/edge":
			got.add("edge", r.Header)
			answer(w)
		case isHop:
			got.add("hop", r.Header)
			if n, _ := strconv.Atoi(hop); n > 0 {
				http.Redirect(w, r, "/hop/"+strconv.Itoa(n-1), http.StatusFound)
				return
			}
			answer(w)
		case strings.HasPrefix(r.URL.Path, "/same/"):
			got.add("provider", r.Header)
			http.Redirect(w, r, "/edge", http.StatusFound)
		default:
			got.add("provider", r.Header)
			http.Redirect(w, r, other.URL+"/edge", http.StatusFound)
		}
	}))
	t.Cleanup(provider.Close)
	return provider, got
}

// The addresses Polyfin requests hold what their host was given: an
// account's name and password in a stream's path and in a list's query, an
// addon's configuration in its path, a token in a manifest URL's query, a
// notification target's own address. The host an answer redirects to,
// another or the same, is not told the address the request comes from.
func TestARedirectedRequestDoesNotTellTheAddressItComesFrom(t *testing.T) {
	t.Parallel()
	provider, got := redirectingHosts(t)
	client := NewClient("test")
	open := func(method, address string) {
		t.Helper()
		response, err := client.Open(t.Context(), method, address, http.Header{}, false)
		if err != nil {
			t.Fatalf("%s: %v", address, err)
		}
		_ = response.Body.Close()
	}
	// A live stream as playback relays it, then a list as the guide's
	// refresh reads it.
	open(http.MethodGet, provider.URL+"/live/alice/pass-1/42.ts")
	open(http.MethodGet, provider.URL+"/player_api.php?username=alice&password=pass-2&action=get_live_streams")
	// An addon's resource, as every resource is read.
	manifest := provider.URL + "/service=key-3/manifest.json?token=tok-4"
	if _, err := client.Fetch(t.Context(), manifest, ResourceURL(manifest, "/stream/movie/tt1.json"), false); err != nil {
		t.Fatal(err)
	}
	// A request with a body, as a licence request is sent.
	response, err := client.Post(t.Context(), provider.URL+"/licence/key-5", http.Header{}, []byte("{}"), false)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	// The client other features send their users' requests with, as
	// notifications do to their targets.
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, provider.URL+"/hooks/secret-6", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	shared := HTTPClient(false)
	t.Cleanup(shared.CloseIdleConnections)
	if response, err = shared.Do(request); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	// The same host.
	open(http.MethodGet, provider.URL+"/same/alice/pass-7/42.ts")

	if n := len(got.at("other")); n != 5 {
		t.Fatalf("the other host was asked %d times, want 5", n)
	}
	if n := len(got.at("edge")); n != 1 {
		t.Fatalf("the provider was asked %d times after its redirect, want 1", n)
	}
	for _, place := range []string{"other", "edge"} {
		for i, header := range got.at(place) {
			if referer, set := header["Referer"]; set {
				t.Errorf("%s, request %d: sent Referer %q", place, i, referer)
			}
		}
	}
}

// A stream's own headers, a Referer among them, are sent as given to the
// address asked and to every address its answers redirect to: hosts that
// serve a stream only to their own pages' players check them at each.
func TestAStreamsOwnHeadersFollowItsRedirects(t *testing.T) {
	t.Parallel()
	provider, got := redirectingHosts(t)
	client := NewClient("test")
	header := http.Header{"Referer": {"https://pages.example/watch/1"}, "Origin": {"https://pages.example"}, "X-Player": {"web"}}
	for _, path := range []string{"/live/alice/pass-1/42.ts", "/same/alice/pass-1/42.ts", "/hop/2"} {
		response, err := client.Open(t.Context(), http.MethodGet, provider.URL+path, header, false)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		_ = response.Body.Close()
	}
	for place, want := range map[string]int{"provider": 2, "other": 1, "edge": 1, "hop": 3} {
		headers := got.at(place)
		if len(headers) != want {
			t.Errorf("%s was asked %d times, want %d", place, len(headers), want)
		}
		for i, received := range headers {
			for name, values := range header {
				if sent := received[name]; len(sent) != 1 || sent[0] != values[0] {
					t.Errorf("%s, request %d: sent %s %q, want %q", place, i, name, sent, values)
				}
			}
		}
	}
}

// Both clients, the one confined to public addresses too, send five
// requests at most for one, the first included, as they did.
func TestARequestStopsAtItsFifthRedirect(t *testing.T) {
	t.Parallel()
	provider, got := redirectingHosts(t)
	client := NewClient("test")
	response, err := client.Open(t.Context(), http.MethodGet, provider.URL+"/hop/4", http.Header{}, false)
	if err != nil {
		t.Fatalf("four redirects: %v", err)
	}
	_ = response.Body.Close()
	if n := len(got.at("hop")); n != 5 {
		t.Fatalf("%d requests for four redirects, want 5", n)
	}
	if _, err = client.Open(t.Context(), http.MethodGet, provider.URL+"/hop/5", http.Header{}, false); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("five redirects: %v, want ErrUnreachable", err)
	}
	if n := len(got.at("hop")); n != 10 {
		t.Errorf("%d requests in all, want 10: the fifth redirect is not followed", n)
	}

	// The confined client reaches no server of a test: its rule is asked
	// as Go's client asks it, with the Referer Go would have set.
	for name, client := range map[string]*http.Client{"trusted": client.trusted, "confined": client.confined} {
		first, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://provider.example/live/alice/pass-1/42.ts", nil)
		if err != nil {
			t.Fatal(err)
		}
		next, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://other.example/edge", nil)
		if err != nil {
			t.Fatal(err)
		}
		next.Header.Set("Referer", first.URL.String())
		via := []*http.Request{first}
		if err := client.CheckRedirect(next, via); err != nil {
			t.Errorf("%s: a first redirect: %v", name, err)
		}
		if referer, set := next.Header["Referer"]; set {
			t.Errorf("%s: the redirect is sent Referer %q", name, referer)
		}
		for len(via) < maxRedirects {
			via = append(via, first)
		}
		if err := client.CheckRedirect(next, via); err == nil {
			t.Errorf("%s: a redirect after %d requests is followed", name, len(via))
		}
	}
}
