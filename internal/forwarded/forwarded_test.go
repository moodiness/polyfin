package forwarded

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"reflect"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"
)

func networks(texts ...string) []netip.Prefix {
	result := make([]netip.Prefix, len(texts))
	for i, text := range texts {
		result[i] = netip.MustParsePrefix(text)
	}
	return result
}

func request(remote string, header ...string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remote
	for i := 0; i+1 < len(header); i += 2 {
		r.Header.Add(header[i], header[i+1])
	}
	return r
}

func TestResolve(t *testing.T) {
	t.Parallel()
	proxies := networks("10.0.0.0/24", "172.18.0.5/32", "fd00:1::/64")
	for _, tc := range []struct {
		name   string
		r      *http.Request
		known  []netip.Prefix
		client string
	}{
		{"no proxy known: the headers are ignored", request("10.0.0.2:4000", "X-Forwarded-For", "203.0.113.9"), nil, "10.0.0.2"},
		{"a spoof from an unknown address", request("198.51.100.4:4000", "X-Forwarded-For", "203.0.113.9", "X-Real-IP", "203.0.113.10"),
			proxies, "198.51.100.4"},
		{"a known proxy", request("10.0.0.2:4000", "X-Forwarded-For", "203.0.113.9"), proxies, "203.0.113.9"},
		{"what the client wrote before the proxy is ignored", request("10.0.0.2:4000", "X-Forwarded-For", "192.0.2.66, 203.0.113.9"),
			proxies, "203.0.113.9"},
		{"a chain through two known proxies", request("10.0.0.2:4000", "X-Forwarded-For", "192.0.2.66, 203.0.113.9, 172.18.0.5"),
			proxies, "203.0.113.9"},
		{"the chain across header lines", request("10.0.0.2:4000", "X-Forwarded-For", "192.0.2.66, 203.0.113.9", "X-Forwarded-For", "172.18.0.5"),
			proxies, "203.0.113.9"},
		{"every address a known proxy", request("10.0.0.2:4000", "X-Forwarded-For", "10.0.0.7, 172.18.0.5"), proxies, "10.0.0.7"},
		{"IPv6 proxy and client", request("[fd00:1::2]:4000", "X-Forwarded-For", "2001:db8::9"), proxies, "2001:db8::9"},
		{"an IPv6 client with a port, bracketed", request("10.0.0.2:4000", "X-Forwarded-For", "[2001:db8::9]:51000"), proxies, "2001:db8::9"},
		{"an IPv4 client with a port", request("10.0.0.2:4000", "X-Forwarded-For", "203.0.113.9:51000"), proxies, "203.0.113.9"},
		{"an IPv4-mapped connection", request("[::ffff:10.0.0.2]:4000", "X-Forwarded-For", "203.0.113.9"), proxies, "203.0.113.9"},
		{"X-Real-IP without X-Forwarded-For", request("10.0.0.2:4000", "X-Real-IP", "203.0.113.9"), proxies, "203.0.113.9"},
		{"X-Real-IP naming the client of X-Forwarded-For", request("10.0.0.2:4000", "X-Forwarded-For", "203.0.113.9", "X-Real-IP", "203.0.113.9"),
			proxies, "203.0.113.9"},
		{"X-Real-IP naming another client: the proxy itself", request("10.0.0.2:4000", "X-Forwarded-For", "203.0.113.9", "X-Real-IP", "203.0.113.10"),
			proxies, "10.0.0.2"},
		{"an empty X-Forwarded-For", request("10.0.0.2:4000", "X-Forwarded-For", " , ", "X-Real-IP", "203.0.113.10"), proxies, "203.0.113.10"},
		{"no header: the proxy itself", request("10.0.0.2:4000"), proxies, "10.0.0.2"},
		{"a malformed address: the proxy that added it", request("10.0.0.2:4000", "X-Forwarded-For", "203.0.113.9, unknown"), proxies,
			"10.0.0.2"},
		{"a malformed address behind a known proxy", request("10.0.0.2:4000", "X-Forwarded-For", "203.0.113.9, garbage, 172.18.0.5"), proxies,
			"172.18.0.5"},
		{"a host name", request("10.0.0.2:4000", "X-Forwarded-For", "client.example.org"), proxies, "10.0.0.2"},
		{"a malformed X-Real-IP", request("10.0.0.2:4000", "X-Real-IP", "300.1.2.3"), proxies, "10.0.0.2"},
	} {
		if got := Resolve(tc.r, tc.known); got.String() != tc.client {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.client)
		}
	}
	if got := Resolve(request("pipe"), proxies); got.IsValid() {
		t.Errorf("no address: %s", got)
	}
}

// The client and the local network, by the connection's address and the
// forwarded headers. Headers from an address that is not a known proxy
// name no client and take the request out of the local network. Those of
// a known proxy name the client unless they cannot be read or disagree:
// the request is then the proxy's own, and outside the local network.
func TestClientAndLocalNetworkBehindProxies(t *testing.T) {
	t.Parallel()
	proxies := networks("10.0.0.0/24", "172.18.0.5/32", "fd00:1::/64")
	for _, tc := range []struct {
		name   string
		r      *http.Request
		known  []netip.Prefix
		client string
		local  bool
	}{
		// A connection that is not from a known proxy.
		{"a local connection", request("192.168.1.5:4000"), proxies, "192.168.1.5", true},
		{"a local connection, no proxy known", request("172.18.0.1:4000"), nil, "172.18.0.1", true},
		{"a connection from the internet", request("203.0.113.9:4000"), proxies, "203.0.113.9", false},
		{"an unlisted proxy sending X-Forwarded-For", request("172.18.0.1:4000", "X-Forwarded-For", "192.168.1.50"), nil, "172.18.0.1", false},
		{"an unlisted proxy sending X-Real-IP", request("172.18.0.1:4000", "X-Real-IP", "192.168.1.50"), nil, "172.18.0.1", false},
		{"an unlisted proxy sending Forwarded", request("172.18.0.1:4000", "Forwarded", "for=192.168.1.50"), nil, "172.18.0.1", false},
		{"an unlisted proxy beside known ones", request("192.168.1.5:4000", "X-Forwarded-For", "192.168.1.50"), proxies, "192.168.1.5", false},
		{"an unlisted proxy on the server's machine", request("127.0.0.1:4000", "X-Real-IP", "127.0.0.1"), proxies, "127.0.0.1", false},
		{"an unlisted proxy on IPv6", request("[::1]:4000", "Forwarded", "for=\"[::1]\""), proxies, "::1", false},
		{"an unlisted proxy, the header empty", request("192.168.1.5:4000", "X-Forwarded-For", ""), proxies, "192.168.1.5", false},
		{"the internet sending headers", request("203.0.113.9:4000", "X-Forwarded-For", "192.168.1.50", "X-Real-IP", "127.0.0.1"), proxies,
			"203.0.113.9", false},

		// A known proxy that names its client.
		{"the proxy itself", request("10.0.0.2:4000"), proxies, "10.0.0.2", true},
		{"X-Forwarded-For, a local client", request("10.0.0.2:4000", "X-Forwarded-For", "192.168.1.50"), proxies, "192.168.1.50", true},
		{"X-Forwarded-For, a client on the internet", request("10.0.0.2:4000", "X-Forwarded-For", "203.0.113.9"), proxies, "203.0.113.9", false},
		{"X-Forwarded-For after what the client wrote", request("10.0.0.2:4000", "X-Forwarded-For", "192.168.1.66, 203.0.113.9"), proxies,
			"203.0.113.9", false},
		{"X-Forwarded-For and the same X-Real-IP", request("10.0.0.2:4000", "X-Forwarded-For", "192.168.1.50", "X-Real-IP", "192.168.1.50"),
			proxies, "192.168.1.50", true},
		{"X-Forwarded-For with a port and the same X-Real-IP",
			request("10.0.0.2:4000", "X-Forwarded-For", "192.168.1.50:51000", "X-Real-IP", "192.168.1.50"), proxies, "192.168.1.50", true},
		{"two known proxies", request("10.0.0.2:4000", "X-Forwarded-For", "192.168.1.50, 172.18.0.5"), proxies, "192.168.1.50", true},
		{"two known proxies, X-Real-IP naming the client",
			request("10.0.0.2:4000", "X-Forwarded-For", "192.168.1.50, 172.18.0.5", "X-Real-IP", "192.168.1.50"), proxies, "192.168.1.50", true},
		{"two known proxies, X-Real-IP naming the first",
			request("10.0.0.2:4000", "X-Forwarded-For", "192.168.1.50, 172.18.0.5", "X-Real-IP", "172.18.0.5"), proxies, "192.168.1.50", true},
		{"every address a known proxy", request("10.0.0.2:4000", "X-Forwarded-For", "10.0.0.7, 172.18.0.5", "X-Real-IP", "10.0.0.7"), proxies,
			"10.0.0.7", true},
		{"X-Real-IP alone, a local client", request("10.0.0.2:4000", "X-Real-IP", "192.168.1.50"), proxies, "192.168.1.50", true},
		{"X-Real-IP alone, a client on the internet", request("10.0.0.2:4000", "X-Real-IP", "203.0.113.9"), proxies, "203.0.113.9", false},
		{"an IPv4-mapped connection and client",
			request("[::ffff:10.0.0.2]:4000", "X-Forwarded-For", "::ffff:192.168.1.50", "X-Real-IP", "192.168.1.50"), proxies, "192.168.1.50", true},
		{"zoned addresses", request("[fd00:1::2%eth0]:4000", "X-Forwarded-For", "fe80::1%eth0", "X-Real-IP", "fe80::1"), proxies, "fe80::1", true},
		{"X-Forwarded-For beside Forwarded", request("10.0.0.2:4000", "X-Forwarded-For", "192.168.1.50", "Forwarded", "for=192.168.1.50"),
			proxies, "192.168.1.50", true},
		{"X-Forwarded-For beside an X-Real-IP that is no address",
			request("10.0.0.2:4000", "X-Forwarded-For", "192.168.1.50", "X-Real-IP", "unknown"), proxies, "192.168.1.50", true},

		// A known proxy whose headers leave the client in doubt.
		{"X-Real-IP beside a local X-Forwarded-For", request("10.0.0.2:4000", "X-Real-IP", "203.0.113.9", "X-Forwarded-For", "192.168.1.50"),
			proxies, "10.0.0.2", false},
		{"X-Real-IP beside another X-Forwarded-For", request("10.0.0.2:4000", "X-Real-IP", "203.0.113.9", "X-Forwarded-For", "198.51.100.77"),
			proxies, "10.0.0.2", false},
		{"two local clients named", request("10.0.0.2:4000", "X-Real-IP", "192.168.1.60", "X-Forwarded-For", "192.168.1.50"), proxies,
			"10.0.0.2", false},
		{"X-Real-IP beside an X-Forwarded-For of known proxies",
			request("10.0.0.2:4000", "X-Real-IP", "192.168.1.60", "X-Forwarded-For", "10.0.0.7, 172.18.0.5"), proxies, "10.0.0.2", false},
		{"an X-Forwarded-For that is no address", request("10.0.0.2:4000", "X-Forwarded-For", "unknown"), proxies, "10.0.0.2", false},
		{"an X-Forwarded-For that is no address beside X-Real-IP",
			request("10.0.0.2:4000", "X-Forwarded-For", "unknown", "X-Real-IP", "192.168.1.50"), proxies, "10.0.0.2", false},
		{"no address behind a known proxy", request("10.0.0.2:4000", "X-Forwarded-For", "192.168.1.50, garbage, 172.18.0.5"), proxies,
			"172.18.0.5", false},
		{"an X-Real-IP that is no address", request("10.0.0.2:4000", "X-Real-IP", "300.1.2.3"), proxies, "10.0.0.2", false},
		{"Forwarded alone", request("10.0.0.2:4000", "Forwarded", "for=192.168.1.50"), proxies, "10.0.0.2", false},
	} {
		var client string
		var local bool
		New(func() []netip.Prefix { return tc.known }, nil).Handler(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			client, local = Client(r), InLocalNetwork(r)
		})).ServeHTTP(httptest.NewRecorder(), tc.r)
		if client != tc.client || local != tc.local {
			t.Errorf("%s: client %s, local %v; want %s, %v", tc.name, client, local, tc.client, tc.local)
		}
	}
}

// Without forwarded headers, the connection's address alone tells the
// local network: loopback, private and link-local addresses, in no IPv6
// form of a public or translated address. With a header a client adds to
// look local, no address is on it.
func TestOnlyLocalConnectionsCountAsLocal(t *testing.T) {
	t.Parallel()
	var local bool
	handler := New(func() []netip.Prefix { return nil }, nil).Handler(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		local = InLocalNetwork(r)
	}))
	for remote, want := range map[string]bool{
		"127.0.0.1:1":              true,
		"[::1]:1":                  true,
		"192.168.1.5:1":            true,
		"10.1.2.3:1":               true,
		"172.31.255.1:1":           true,
		"169.254.10.10:1":          true,
		"[fe80::1%en0]:1":          true,
		"[fd12:3456::1]:1":         true,
		"[::ffff:192.168.1.5]:1":   true, // the same IPv4 host on a dual-stack listener
		"203.0.113.9:1":            false,
		"[::ffff:203.0.113.9]:1":   false,
		"172.32.0.1:1":             false,
		"100.64.0.1:1":             false, // carrier-grade NAT
		"0.0.0.0:1":                false,
		"[::]:1":                   false,
		"[2001:db8::1]:1":          false,
		"[64:ff9b::c0a8:105]:1":    false, // NAT64 of 192.168.1.5
		"[2002:c0a8:105::1]:1":     false, // 6to4 of 192.168.1.5
		"[fec0::1]:1":              false, // deprecated site-local
		"not an address":           false,
		"[2001:db8::1%eth0]:65535": false,
	} {
		handler.ServeHTTP(httptest.NewRecorder(), request(remote))
		if local != want {
			t.Errorf("%s: local %v, want %v", remote, local, want)
		}
		for _, header := range [][2]string{{"X-Forwarded-For", "192.168.1.50"}, {"X-Real-IP", "127.0.0.1"}, {"Forwarded", "for=127.0.0.1"}} {
			handler.ServeHTTP(httptest.NewRecorder(), request(remote, header[0], header[1]))
			if local {
				t.Errorf("%s with %s: local", remote, header[0])
			}
		}
	}
}

// The handler has Client read the address Resolve finds, under the
// setting of the moment.
func TestHandlerFollowsTheSetting(t *testing.T) {
	t.Parallel()
	var known []netip.Prefix
	proxies := New(func() []netip.Prefix { return known }, nil)
	var seen string
	handler := proxies.Handler(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = Client(r) }))
	serve := func(r *http.Request) string {
		handler.ServeHTTP(httptest.NewRecorder(), r)
		return seen
	}
	if got := serve(request("127.0.0.1:4000", "X-Forwarded-For", "203.0.113.9")); got != "127.0.0.1" {
		t.Errorf("no known proxy: %s", got)
	}
	known = networks("127.0.0.1/32")
	if got := serve(request("127.0.0.1:4000", "X-Forwarded-For", "203.0.113.9")); got != "203.0.113.9" {
		t.Errorf("a known proxy: %s", got)
	}
	if got := serve(request("127.0.0.1:4000")); got != "127.0.0.1" {
		t.Errorf("the proxy itself: %s", got)
	}
	// Without the handler, the connection's address.
	if got := Client(request("192.0.2.1:4000", "X-Forwarded-For", "203.0.113.9")); got != "192.0.2.1" {
		t.Errorf("without the handler: %s", got)
	}
}

// Addresses in the local network that send forwarded headers without
// being known proxies are named for an hour after their last such request,
// never addresses on the internet, nor known proxies, and at most
// maxUnknown at once.
func TestUnknownProxies(t *testing.T) {
	t.Parallel()
	var known []netip.Prefix
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	proxies := New(func() []netip.Prefix { return known }, nil)
	proxies.now = func() time.Time { return now }
	handler := proxies.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	serve := func(r *http.Request) { handler.ServeHTTP(httptest.NewRecorder(), r) }
	names := func() []string {
		var result []string
		for _, address := range proxies.Unknown() {
			result = append(result, address.String())
		}
		return result
	}

	serve(request("127.0.0.1:4000", "X-Forwarded-For", "203.0.113.9"))
	serve(request("[::ffff:172.18.0.2]:4000", "X-Real-IP", "203.0.113.9"))
	serve(request("203.0.113.50:4000", "X-Forwarded-For", "10.0.0.1"))
	serve(request("192.168.1.20:4000"))
	if got := names(); !slices.Equal(got, []string{"127.0.0.1", "172.18.0.2"}) {
		t.Fatalf("named: %v", got)
	}
	// Known now, it is no longer named, nor remembered again.
	known = networks("127.0.0.1/32")
	serve(request("127.0.0.1:4000", "X-Forwarded-For", "203.0.113.9"))
	if got := names(); !slices.Equal(got, []string{"172.18.0.2"}) {
		t.Errorf("once known: %v", got)
	}
	// An hour after its last forwarded request, it goes.
	now = now.Add(59 * time.Minute)
	serve(request("172.18.0.2:4000", "X-Forwarded-For", "203.0.113.9"))
	now = now.Add(59 * time.Minute)
	if got := names(); !slices.Equal(got, []string{"172.18.0.2"}) {
		t.Errorf("within the hour: %v", got)
	}
	now = now.Add(2 * time.Minute)
	if got := names(); len(got) != 0 {
		t.Errorf("after the hour: %v", got)
	}
	// Memory is bounded: the address seen longest ago is forgotten.
	for i := range maxUnknown + 4 {
		now = now.Add(time.Second)
		serve(request("10.1.0."+strconv.Itoa(i)+":4000", "X-Forwarded-For", "203.0.113.9"))
	}
	if len(proxies.unknown) != maxUnknown {
		t.Errorf("%d remembered", len(proxies.unknown))
	}
	got := names()
	if len(got) != maxUnknown || slices.Contains(got, "10.1.0.3") || !slices.Contains(got, "10.1.0.19") {
		t.Errorf("after many: %v", got)
	}
}

// fakeResolver answers the host names in addresses, and that no other
// exists; the lookup of a name in hold waits for its channel to close,
// once started tells of it.
type fakeResolver struct {
	mu        sync.Mutex
	addresses map[string][]string
	hold      map[string]chan struct{}
	started   chan string
}

func (f *fakeResolver) set(name string, addresses ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addresses[name] = addresses
}

func (f *fakeResolver) LookupNetIP(ctx context.Context, _, host string) ([]netip.Addr, error) {
	f.mu.Lock()
	texts, found := f.addresses[host]
	wait := f.hold[host]
	f.mu.Unlock()
	if wait != nil {
		f.started <- host
		select {
		case <-wait:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if !found {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	addresses := make([]netip.Addr, len(texts))
	for i, text := range texts {
		addresses[i] = netip.MustParseAddr(text)
	}
	return addresses, nil
}

// clientOf returns the client the handler of proxies sees for a request
// from remote that names 203.0.113.9 in X-Forwarded-For.
func clientOf(proxies *Proxies, remote string) string {
	var seen string
	proxies.Handler(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = Client(r) })).
		ServeHTTP(httptest.NewRecorder(), request(remote, "X-Forwarded-For", "203.0.113.9"))
	return seen
}

// A host name of Known proxies trusts every address it resolves to, the
// mapped ones as IPv4; once it moves, its old address stops counting at
// the next lookup, and its new one is no longer named as an unknown
// proxy. A name that resolves to nothing trusts nothing, and is told.
func TestProxyNames(t *testing.T) {
	t.Parallel()
	resolver := &fakeResolver{addresses: map[string][]string{"proxy": {"::ffff:172.18.0.16", "fd00::16"}}}
	proxies := New(func() []netip.Prefix { return nil }, resolver)
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	proxies.now = func() time.Time { return now }

	if got := clientOf(proxies, "172.18.0.16:4000"); got != "172.18.0.16" {
		t.Errorf("before the lookup: %s", got)
	}
	proxies.Lookup(t.Context(), []string{"proxy", "nothing.invalid"})
	for remote, want := range map[string]string{"172.18.0.16:4000": "203.0.113.9", "[fd00::16]:4000": "203.0.113.9",
		"172.18.0.3:4000": "172.18.0.3"} {
		if got := clientOf(proxies, remote); got != want {
			t.Errorf("from %s: %s, want %s", remote, got, want)
		}
	}
	want := []Name{{Name: "proxy", Addresses: []netip.Addr{netip.MustParseAddr("172.18.0.16"), netip.MustParseAddr("fd00::16")}, Resolved: now},
		{Name: "nothing.invalid", Addresses: []netip.Addr{}, Resolved: now}}
	if got := proxies.Names(); !reflect.DeepEqual(got, want) {
		t.Errorf("names: %+v", got)
	}

	resolver.set("proxy", "172.18.0.3")
	if got := clientOf(proxies, "172.18.0.16:4000"); got != "203.0.113.9" {
		t.Errorf("moved, before the next lookup: %s", got)
	}
	if got := proxies.Unknown(); !slices.Equal(got, []netip.Addr{netip.MustParseAddr("172.18.0.3")}) {
		t.Errorf("unknown before the move: %v", got)
	}
	proxies.Lookup(t.Context(), []string{"proxy", "nothing.invalid"})
	if got := clientOf(proxies, "172.18.0.16:4000"); got != "172.18.0.16" {
		t.Errorf("the old address after the move: %s", got)
	}
	if got := clientOf(proxies, "172.18.0.3:4000"); got != "203.0.113.9" {
		t.Errorf("the new address after the move: %s", got)
	}
	if got := proxies.Unknown(); !slices.Equal(got, []netip.Addr{netip.MustParseAddr("172.18.0.16")}) {
		t.Errorf("unknown after the move: %v", got)
	}

	proxies.Lookup(t.Context(), nil)
	if got := clientOf(proxies, "172.18.0.3:4000"); got != "172.18.0.3" {
		t.Errorf("once removed: %s", got)
	}
	if got := proxies.Names(); len(got) != 0 {
		t.Errorf("names once removed: %+v", got)
	}
}

// A name removed from the setting stops being trusted at once, while the
// names left are still being looked up.
func TestRemovedProxyNamesStopAtOnce(t *testing.T) {
	t.Parallel()
	hold := make(chan struct{})
	resolver := &fakeResolver{addresses: map[string][]string{"proxy": {"172.18.0.16"}, "slow": {"172.18.0.20"}},
		started: make(chan string, 1)}
	proxies := New(func() []netip.Prefix { return nil }, resolver)
	proxies.Lookup(t.Context(), []string{"proxy"})
	resolver.hold = map[string]chan struct{}{"slow": hold}
	done := make(chan struct{})
	go func() {
		proxies.Lookup(t.Context(), []string{"slow"})
		close(done)
	}()
	<-resolver.started
	if got := clientOf(proxies, "172.18.0.16:4000"); got != "172.18.0.16" {
		t.Errorf("removed, during the lookup: %s", got)
	}
	if got := clientOf(proxies, "172.18.0.20:4000"); got != "172.18.0.20" {
		t.Errorf("added, during the lookup: %s", got)
	}
	close(hold)
	<-done
	if got := clientOf(proxies, "172.18.0.20:4000"); got != "203.0.113.9" {
		t.Errorf("added, after the lookup: %s", got)
	}
}

// Watch looks the names up before it returns, then again as soon as the
// settings change.
func TestWatchFollowsTheSettings(t *testing.T) {
	t.Parallel()
	resolver := &fakeResolver{addresses: map[string][]string{"proxy": {"172.18.0.16"}, "other": {"172.18.0.30"}}}
	proxies := New(func() []netip.Prefix { return nil }, resolver)
	var mu sync.Mutex
	names, changed := []string{"proxy"}, make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	proxies.Watch(ctx, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return names
	}, func() <-chan struct{} {
		mu.Lock()
		defer mu.Unlock()
		return changed
	})
	if got := clientOf(proxies, "172.18.0.16:4000"); got != "203.0.113.9" {
		t.Fatalf("at the start: %s", got)
	}
	mu.Lock()
	names = []string{"other"}
	close(changed)
	changed = make(chan struct{})
	mu.Unlock()
	deadline := time.Now().Add(5 * time.Second)
	for clientOf(proxies, "172.18.0.30:4000") != "203.0.113.9" {
		if time.Now().After(deadline) {
			t.Fatal("the change was not looked up")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := clientOf(proxies, "172.18.0.16:4000"); got != "172.18.0.16" {
		t.Errorf("the name removed: %s", got)
	}
}

// A proxy that starts after Polyfin, its name resolving to nothing then,
// is trusted as soon as its forwarded requests have the names looked up
// again: the request that does so is judged as things were, the next one
// counts as the client it names.
func TestUnknownForwarderHasTheNamesLookedUp(t *testing.T) {
	t.Parallel()
	resolver := &fakeResolver{addresses: map[string][]string{}}
	proxies := New(func() []netip.Prefix { return nil }, resolver)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	proxies.Watch(ctx, func() []string { return []string{"proxy"} }, func() <-chan struct{} { return make(chan struct{}) })
	if names := proxies.Names(); len(names) != 1 || len(names[0].Addresses) != 0 {
		t.Fatalf("at the start: %+v", names)
	}
	resolver.set("proxy", "172.18.0.3")
	if got := clientOf(proxies, "172.18.0.3:4000"); got != "172.18.0.3" {
		t.Errorf("the request that has the names looked up: %s", got)
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(proxies.Names()[0].Addresses) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the names were not looked up")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := clientOf(proxies, "172.18.0.3:4000"); got != "203.0.113.9" {
		t.Errorf("the next request: %s", got)
	}
}

// Forwarded requests from addresses that are not known proxies have the
// names looked up at most once every 10 seconds, and only while some are
// listed.
func TestUnknownForwardersLookUpAtMostEvery10Seconds(t *testing.T) {
	t.Parallel()
	resolver := &fakeResolver{addresses: map[string][]string{}}
	proxies := New(func() []netip.Prefix { return nil }, resolver)
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	proxies.now = func() time.Time { return now }
	woken := func() bool {
		select {
		case <-proxies.wake:
			return true
		default:
			return false
		}
	}
	clientOf(proxies, "172.18.0.3:4000")
	if woken() {
		t.Error("woken with no name listed")
	}
	proxies.Lookup(t.Context(), []string{"proxy"})
	clientOf(proxies, "172.18.0.3:4000")
	if !woken() {
		t.Error("not woken by the first request")
	}
	now = now.Add(9 * time.Second)
	clientOf(proxies, "172.18.0.4:4000")
	if woken() {
		t.Error("woken again within 10 seconds")
	}
	now = now.Add(time.Second)
	clientOf(proxies, "172.18.0.4:4000")
	if !woken() {
		t.Error("not woken after 10 seconds")
	}
}
