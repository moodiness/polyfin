package stremio_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/stremio/reachtest"
)

// counted is a test server that counts its requests. It answers a
// manifest, or a redirect to the same path at to when it is set.
type counted struct {
	url  string
	hits atomic.Int32
	to   atomic.Value
}

func newCounted(t *testing.T) *counted {
	t.Helper()
	c := &counted{}
	c.to.Store("")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.hits.Add(1)
		if to := c.to.Load().(string); to != "" {
			http.Redirect(w, r, to+r.URL.RequestURI(), http.StatusTemporaryRedirect)
			return
		}
		_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "test", Name: "Test", Version: "1.0.0",
			Resources: []stremio.Resource{{Name: "catalog"}}})
	}))
	t.Cleanup(server.Close)
	c.url = server.URL
	return c
}

// A request to a source itself reaches local addresses only, unless it is
// confined to public ones: it is never made anywhere, and neither are its
// redirects.
func TestASourcesOwnRequestsStayOnTheNetworkOfTheSource(t *testing.T) {
	t.Parallel()
	client := stremio.NewClient("test")
	network := reachtest.New(client)
	local, internet := newCounted(t), newCounted(t)
	public := network.Public(t, internet.url)
	lan := network.Serve(t, "192.168.1.50", local.url)

	for _, c := range []struct {
		name     string
		address  string
		confined bool
		refused  error
	}{
		{"a local source, on the loopback", local.url, false, nil},
		{"a local source, on a private address", lan, false, nil},
		{"a public source, confined", public, true, nil},
		{"a public address, not confined", public, false, stremio.ErrPublicNetwork},
		{"the loopback, confined", local.url, true, stremio.ErrPrivateNetwork},
		{"a private address, confined", lan, true, stremio.ErrPrivateNetwork},
	} {
		reached := local.hits.Load() + internet.hits.Load()
		_, err := client.Manifest(t.Context(), c.address+"/manifest.json", c.confined)
		if !errors.Is(err, c.refused) || c.refused == nil && err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if n := local.hits.Load() + internet.hits.Load() - reached; c.refused == nil && n != 1 || c.refused != nil && n != 0 {
			t.Errorf("%s: %d requests", c.name, n)
		}
	}

	// A local source's redirect to the internet is not followed, nor a
	// public one's to the local network.
	local.to.Store(public)
	reached := internet.hits.Load()
	if _, err := client.Manifest(t.Context(), local.url+"/manifest.json", false); !errors.Is(err, stremio.ErrPublicNetwork) {
		t.Errorf("a local source redirecting to the internet: %v", err)
	}
	if n := internet.hits.Load() - reached; n != 0 {
		t.Errorf("the redirect of a local source reached the internet %d times", n)
	}
	local.to.Store("")
	internet.to.Store(lan)
	reached = local.hits.Load()
	if _, err := client.Manifest(t.Context(), public+"/manifest.json", true); !errors.Is(err, stremio.ErrPrivateNetwork) {
		t.Errorf("a public source redirecting to the local network: %v", err)
	}
	if n := local.hits.Load() - reached; n != 0 {
		t.Errorf("the redirect of a public source reached the local network %d times", n)
	}
}

// OpenSource asks a source that is no Stremio addon, as an IPTV list, the
// same way; Open, for what a source's answers name, reaches any address
// unless confined.
func TestOpenSourceStaysOnTheNetworkOfTheSourceAndOpenDoesNot(t *testing.T) {
	t.Parallel()
	client := stremio.NewClient("test")
	network := reachtest.New(client)
	local, internet := newCounted(t), newCounted(t)
	public := network.Public(t, internet.url)

	open := func(source bool, address string, confined bool) error {
		request := client.Open
		if source {
			request = client.OpenSource
		}
		response, err := request(t.Context(), http.MethodGet, address+"/list.m3u", nil, confined)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	for _, c := range []struct {
		name     string
		source   bool
		address  string
		confined bool
		refused  error
	}{
		{"a local source's list", true, local.url, false, nil},
		{"a public source's list, confined", true, public, true, nil},
		{"a public address as a local source's list", true, public, false, stremio.ErrPublicNetwork},
		{"a local address as a public source's list", true, local.url, true, stremio.ErrPrivateNetwork},
		{"what a local source names on the internet", false, public, false, nil},
		{"what a local source names on the local network", false, local.url, false, nil},
		{"what a public source names on the internet", false, public, true, nil},
		{"what a public source names on the local network", false, local.url, true, stremio.ErrPrivateNetwork},
	} {
		reached := local.hits.Load() + internet.hits.Load()
		if err := open(c.source, c.address, c.confined); !errors.Is(err, c.refused) || c.refused == nil && err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if n := local.hits.Load() + internet.hits.Load() - reached; c.refused == nil && n != 1 || c.refused != nil && n != 0 {
			t.Errorf("%s: %d requests", c.name, n)
		}
	}
}

// A name is where the client connects to, whatever else it resolves to:
// the client of the local network takes its local address, the confined
// one its public address, and neither the other's.
func TestANameIsReachedAtTheAddressesItsClientMayReach(t *testing.T) {
	t.Parallel()
	client := stremio.NewClient("test")
	network := reachtest.New(client)
	local, internet := newCounted(t), newCounted(t)
	network.Serve(t, "203.0.113.80:80", internet.url)
	network.Serve(t, "192.168.1.50:80", local.url)
	network.Name("both.test", []string{"203.0.113.80", "192.168.1.50"})

	if _, err := client.Manifest(t.Context(), "http://both.test/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	if local.hits.Load() != 1 || internet.hits.Load() != 0 {
		t.Errorf("not confined: %d local and %d public requests", local.hits.Load(), internet.hits.Load())
	}
	if _, err := client.Manifest(t.Context(), "http://both.test/manifest.json", true); err != nil {
		t.Fatal(err)
	}
	if local.hits.Load() != 1 || internet.hits.Load() != 1 {
		t.Errorf("confined: %d local and %d public requests", local.hits.Load(), internet.hits.Load())
	}
}

// The refusal of a read that tells where a source is, by the client of
// the network the source is not on, is no failure of the source: Health
// does not count it. Any other refusal is one.
func TestLocatingASourceCountsNoFailureForTheNetworkItIsNotOn(t *testing.T) {
	t.Parallel()
	client := stremio.NewClient("test")
	network := reachtest.New(client)
	internet := newCounted(t)
	manifest := network.Public(t, internet.url) + "/manifest.json"

	if _, err := client.Manifest(stremio.Locating(t.Context()), manifest, false); !errors.Is(err, stremio.ErrPublicNetwork) {
		t.Fatalf("read from the local network: %v", err)
	}
	if health, ok := client.Health(manifest); ok {
		t.Errorf("the refusal was counted: %+v", health)
	}
	if _, err := client.Manifest(stremio.Locating(t.Context()), manifest, true); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Manifest(t.Context(), manifest, false); !errors.Is(err, stremio.ErrPublicNetwork) || !errors.Is(err, stremio.ErrUnreachable) {
		t.Fatalf("a request of a source that is no longer local: %v", err)
	}
	if health, _ := client.Health(manifest); health.Requests != 2 || health.Failures != 1 || health.Failure != stremio.FailureUnreachable {
		t.Errorf("health: %+v", health)
	}
}
