package forwarded

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strconv"
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
		{"X-Forwarded-For before X-Real-IP", request("10.0.0.2:4000", "X-Forwarded-For", "203.0.113.9", "X-Real-IP", "203.0.113.10"),
			proxies, "203.0.113.9"},
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

// The handler has Client read the address Resolve finds, under the
// setting of the moment.
func TestHandlerFollowsTheSetting(t *testing.T) {
	var known []netip.Prefix
	proxies := New(func() []netip.Prefix { return known })
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
	var known []netip.Prefix
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	proxies := New(func() []netip.Prefix { return known })
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
