package redirect

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// received keeps the headers of the requests a test's servers were sent,
// by the name the test gives each place.
type received struct {
	mu      sync.Mutex
	headers map[string][]http.Header
}

func (r *received) add(place string, header http.Header) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.headers == nil {
		r.headers = map[string][]http.Header{}
	}
	r.headers[place] = append(r.headers[place], header.Clone())
}

func (r *received) at(place string) []http.Header {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.headers[place]
}

// hosts starts two servers. The first answers /hop/N with a redirect to
// /hop/N-1 on itself down to /hop/0, /same/... with a redirect to /end on
// itself, and any other path with a redirect to /end on the second; /end
// answers 200 on both.
func hosts(t *testing.T) (first, other *httptest.Server, got *received) {
	t.Helper()
	got = &received{}
	other = httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got.add("other", r.Header)
	}))
	t.Cleanup(other.Close)
	first = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch hop, isHop := strings.CutPrefix(r.URL.Path, "/hop/"); {
		case r.URL.Path == "/end":
			got.add("end", r.Header)
		case isHop:
			got.add("hop", r.Header)
			if n, _ := strconv.Atoi(hop); n > 0 {
				http.Redirect(w, r, "/hop/"+strconv.Itoa(n-1), http.StatusFound)
			}
		case strings.HasPrefix(r.URL.Path, "/same/"):
			got.add("first", r.Header)
			http.Redirect(w, r, "/end", http.StatusFound)
		default:
			got.add("first", r.Header)
			http.Redirect(w, r, other.URL+"/end", http.StatusFound)
		}
	}))
	t.Cleanup(first.Close)
	return first, other, got
}

func get(t *testing.T, client *http.Client, address string, header http.Header) error {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, address, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, values := range header {
		request.Header[name] = values
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	return response.Body.Close()
}

// The address a request is redirected from holds what the first host was
// given: neither another host nor the same one is told it.
func TestARedirectIsNotToldTheAddressItComesFrom(t *testing.T) {
	t.Parallel()
	first, _, got := hosts(t)
	client := &http.Client{Transport: &http.Transport{}, CheckRedirect: Follow(Default)}
	t.Cleanup(client.CloseIdleConnections)
	for _, path := range []string{"/live/alice/pass-1/42.ts", "/player_api.php?username=alice&password=pass-2", "/same/key-3/manifest.json?token=tok-4"} {
		if err := get(t, client, first.URL+path, nil); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	if n := len(got.at("other")); n != 2 {
		t.Fatalf("the other host was asked %d times, want 2", n)
	}
	if n := len(got.at("end")); n != 1 {
		t.Fatalf("the first host was asked %d times after its redirect, want 1", n)
	}
	for _, place := range []string{"other", "end"} {
		for _, header := range got.at(place) {
			if referer, set := header["Referer"]; set {
				t.Errorf("%s was sent Referer %q", place, referer)
			}
		}
	}
}

// A Referer the caller set, which some hosts ask for, goes to the first
// address and to every next one as the caller set it, as the caller's
// other headers do.
func TestARefererTheCallerSetGoesToEveryAddress(t *testing.T) {
	t.Parallel()
	first, _, got := hosts(t)
	client := &http.Client{Transport: &http.Transport{}, CheckRedirect: Follow(Default)}
	t.Cleanup(client.CloseIdleConnections)
	header := http.Header{"Referer": {"https://player.example/watch"}, "X-Asked-By": {"a player"}}
	for _, path := range []string{"/live/alice/pass-1/42.ts", "/same/alice/pass-1/42.ts", "/hop/2"} {
		if err := get(t, client, first.URL+path, header); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	for place, want := range map[string]int{"first": 2, "other": 1, "end": 1, "hop": 3} {
		headers := got.at(place)
		if len(headers) != want {
			t.Errorf("%s was asked %d times, want %d", place, len(headers), want)
		}
		for _, header := range headers {
			if referer := header["Referer"]; len(referer) != 1 || referer[0] != "https://player.example/watch" {
				t.Errorf("%s was sent Referer %q, want the caller's", place, referer)
			}
			if header.Get("X-Asked-By") != "a player" {
				t.Errorf("%s was not sent the caller's other header: %v", place, header)
			}
		}
	}
}

// A client sends as many requests for one as its limit says, the first
// included, and no more.
func TestRedirectsStopAtTheLimit(t *testing.T) {
	t.Parallel()
	first, _, got := hosts(t)
	client := &http.Client{Transport: &http.Transport{}, CheckRedirect: Follow(3)}
	t.Cleanup(client.CloseIdleConnections)
	if err := get(t, client, first.URL+"/hop/2", nil); err != nil {
		t.Fatalf("2 redirects with a limit of 3 requests: %v", err)
	}
	if n := len(got.at("hop")); n != 3 {
		t.Fatalf("%d requests for 2 redirects, want 3", n)
	}
	err := get(t, client, first.URL+"/hop/3", nil)
	if !errors.Is(err, ErrTooMany) {
		t.Fatalf("3 redirects with a limit of 3 requests: %v, want ErrTooMany", err)
	}
	// The first request and the two redirects followed: the third
	// redirect is not.
	if n := len(got.at("hop")); n != 6 {
		t.Errorf("%d requests in all, want 6", n)
	}
	for _, header := range got.at("hop") {
		if referer, set := header["Referer"]; set {
			t.Errorf("a redirect was sent Referer %q", referer)
		}
	}
}
