package redirect

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// landing is a request that reached a host without being redirected.
type landing struct {
	host   string
	header http.Header
	body   string
}

// landings keeps the requests a named server answered with 200.
type landings struct {
	mu  sync.Mutex
	got []landing
}

func (l *landings) add(got landing) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.got = append(l.got, got)
}

func (l *landings) take() []landing {
	l.mu.Lock()
	defer l.mu.Unlock()
	got := l.got
	l.got = nil
	return got
}

// named starts one server that answers for every host name, through a
// client following redirects by Follow whose transport reaches it whatever
// the name and port asked. A request with to and code in its query is
// redirected there with that status; any other lands.
func named(t *testing.T) (*http.Client, *landings) {
	t.Helper()
	got := &landings{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if to := r.URL.Query().Get("to"); to != "" {
			code, _ := strconv.Atoi(r.URL.Query().Get("code"))
			w.Header().Set("Location", to)
			w.WriteHeader(code)
			return
		}
		body, _ := io.ReadAll(r.Body)
		got.add(landing{host: r.Host, header: r.Header.Clone(), body: string(body)})
	}))
	t.Cleanup(server.Close)
	address := server.Listener.Addr().String()
	transport := &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}}
	client := &http.Client{Transport: transport, CheckRedirect: Follow(Default)}
	t.Cleanup(client.CloseIdleConnections)
	return client, got
}

// via is address, redirected with code to each of next in turn.
func via(address string, code int, next ...string) string {
	for i := len(next) - 1; i >= 0; i-- {
		next[i] += "?" + url.Values{"to": {address}, "code": {strconv.Itoa(code)}}.Encode()
		address = next[i]
	}
	return address
}

func send(ctx context.Context, t *testing.T, client *http.Client, method, address string, header http.Header, body string) error {
	t.Helper()
	var content io.Reader
	if body != "" {
		content = strings.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, address, content)
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

// The headers a request names secret go to its first host, a subdomain of
// it and another port of it, as Go's client sends Authorization, and to
// no other host, nor back to the first once a redirect left it. Its other
// headers go everywhere, and a request that names none keeps them all.
func TestSecretHeadersGoToNoOtherHost(t *testing.T) {
	t.Parallel()
	client, got := named(t)
	secret := SecretHeaders(SecretHeaders(t.Context(), "X-Plex-Token"), "apikey")
	header := http.Header{"X-Plex-Token": {"plex-token"}, "Apikey": {"backend-key"}, "X-Asked-By": {"a test"}}
	for _, c := range []struct {
		name    string
		ctx     context.Context
		address string
		host    string
		kept    bool
	}{
		{"the first host", secret, "http://first.test/end", "first.test", true},
		{"another port", secret, via("http://first.test:8443/end", http.StatusFound, "http://first.test/go"), "first.test:8443", true},
		{"a subdomain", secret, via("http://api.first.test/end", http.StatusTemporaryRedirect, "http://first.test/go"),
			"api.first.test", true},
		{"another host", secret, via("http://other.test/end", http.StatusFound, "http://first.test/go"), "other.test", false},
		{"a parent domain", secret, via("http://first.test/end", http.StatusFound, "http://api.first.test/go"), "first.test", false},
		{"back to the first", secret, via("http://first.test/end", http.StatusFound, "http://first.test/go", "http://other.test/go"),
			"first.test", false},
		{"none named", t.Context(), via("http://other.test/end", http.StatusFound, "http://first.test/go"), "other.test", true},
	} {
		if err := send(c.ctx, t, client, http.MethodGet, c.address, header, ""); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		landed := got.take()
		if len(landed) != 1 || landed[0].host != c.host {
			t.Fatalf("%s: landed %+v, want once at %s", c.name, landed, c.host)
		}
		sent := landed[0].header
		if sent.Get("X-Asked-By") != "a test" {
			t.Errorf("%s: the other header was not sent: %v", c.name, sent)
		}
		for _, name := range []string{"X-Plex-Token", "Apikey"} {
			if _, set := sent[name]; set != c.kept {
				t.Errorf("%s: %s sent %v, want %v", c.name, name, set, c.kept)
			}
		}
	}
}

// A 307 or 308 that would send a body marked secret again to another host
// is refused, before that host is asked; to the first host or a subdomain
// it is followed, and a 303, which sends no body, is followed anywhere.
// Without the mark, the body goes on as Go's client sends it.
func TestASecretBodyIsNotSentAgainToAnotherHost(t *testing.T) {
	t.Parallel()
	client, got := named(t)
	secret := SecretBody(t.Context())
	const form = "username=alice&password=pass-1"
	for _, c := range []struct {
		name    string
		ctx     context.Context
		address string
		refused bool
		body    string
	}{
		{"307 to another host", secret, via("http://other.test/end", http.StatusTemporaryRedirect, "http://first.test/go"), true, ""},
		{"308 to another host", secret, via("http://other.test/end", http.StatusPermanentRedirect, "http://first.test/go"), true, ""},
		{"307 to the first host", secret, via("http://first.test/end", http.StatusTemporaryRedirect, "http://first.test/go"), false, form},
		{"308 to a subdomain", secret, via("http://auth.first.test/end", http.StatusPermanentRedirect, "http://first.test/go"), false, form},
		{"303 to another host", secret, via("http://other.test/end", http.StatusSeeOther, "http://first.test/go"), false, ""},
		{"not marked", t.Context(), via("http://other.test/end", http.StatusTemporaryRedirect, "http://first.test/go"), false, form},
	} {
		err := send(c.ctx, t, client, http.MethodPost, c.address, http.Header{"Content-Type": {"application/x-www-form-urlencoded"}}, form)
		landed := got.take()
		if c.refused {
			if !errors.Is(err, ErrSecretBody) || len(landed) != 0 {
				t.Errorf("%s: %v, landed %+v, want ErrSecretBody before any other host", c.name, err, landed)
			}
			continue
		}
		if err != nil || len(landed) != 1 || landed[0].body != c.body {
			t.Errorf("%s: %v, landed %+v, want once with body %q", c.name, err, landed, c.body)
		}
	}
}
