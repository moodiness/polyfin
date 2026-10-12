package accounts

import (
	"errors"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Known proxies are read from addresses and networks, IPv4 and IPv6, and
// host names, kept as they are compared, and refused when one is none of
// them or when there are too many.
func TestParseKnownProxies(t *testing.T) {
	proxies, names, err := ParseKnownProxies([]string{" 192.168.1.10 ", "Traefik", "172.18.0.7/16", "2001:db8::1", "fd00::/8",
		"::ffff:10.0.0.0/104", "192.168.1.10/32", "proxy.example.org", "my_proxy-1", "traefik"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"192.168.1.10", "172.18.0.0/16", "2001:db8::1", "fd00::/8", "10.0.0.0/8", "traefik", "proxy.example.org", "my_proxy-1"}
	if got := FormatKnownProxies(proxies, names); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if proxies, names, err := ParseKnownProxies(nil); err != nil || proxies == nil || len(proxies) != 0 || names == nil || len(names) != 0 {
		t.Errorf("none: %#v %#v %v", proxies, names, err)
	}
	for _, wrong := range []string{"", "192.168.1.300", "10.0.0.0/33", "fe80::1%eth0", "10.0.0.1-10.0.0.9", "::ffff:0:0/80",
		"-proxy", "proxy-", "proxy..example.org", "my proxy", "proxy:443", "http://proxy", strings.Repeat("a", 64)} {
		if _, _, err := ParseKnownProxies([]string{wrong}); !errors.Is(err, ErrInvalidKnownProxies) {
			t.Errorf("%q: %v", wrong, err)
		}
	}
	many := make([]string, MaxKnownProxies+1)
	for i := range many {
		many[i] = "10.0.0." + strconv.Itoa(i)
	}
	if _, _, err := ParseKnownProxies(many[:MaxKnownProxies]); err != nil {
		t.Errorf("%d entries: %v", MaxKnownProxies, err)
	}
	if _, _, err := ParseKnownProxies(append(many[:MaxKnownProxies], "traefik")); !errors.Is(err, ErrInvalidKnownProxies) {
		t.Errorf("%d entries and a name: %v", MaxKnownProxies, err)
	}
}

// Known proxies are saved, read again by a new start, and emptied; their
// addresses and names hold 64 entries at most together.
func TestKnownProxiesAreSaved(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	changed := store.Settings()
	changed.KnownProxies = []netip.Prefix{netip.MustParsePrefix("172.18.0.9/16"), netip.MustParsePrefix("2001:db8::7/128"),
		netip.MustParsePrefix("127.0.0.1/32")}
	changed.KnownProxyNames = []string{"Traefik", "proxy.example.org"}
	want := []netip.Prefix{netip.MustParsePrefix("172.18.0.0/16"), netip.MustParsePrefix("2001:db8::7/128"),
		netip.MustParsePrefix("127.0.0.1/32")}
	wantNames := []string{"traefik", "proxy.example.org"}
	saved, err := store.UpdateSettings(ctx, changed)
	if err != nil || !slices.Equal(saved.KnownProxies, want) || !slices.Equal(store.KnownProxies(), want) ||
		!slices.Equal(store.KnownProxyNames(), wantNames) {
		t.Fatalf("saved: %v %v %v", saved.KnownProxies, saved.KnownProxyNames, err)
	}
	reopened, err := Open(ctx, store.db)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Settings(); !slices.Equal(got.KnownProxies, want) || !slices.Equal(got.KnownProxyNames, wantNames) {
		t.Errorf("read again: %v %v", got.KnownProxies, got.KnownProxyNames)
	}
	changed.KnownProxies, changed.KnownProxyNames = nil, nil
	if saved, err := store.UpdateSettings(ctx, changed); err != nil || saved.KnownProxies == nil || len(saved.KnownProxies) != 0 ||
		saved.KnownProxyNames == nil || len(saved.KnownProxyNames) != 0 {
		t.Errorf("emptied: %#v %#v %v", saved.KnownProxies, saved.KnownProxyNames, err)
	}
	changed.KnownProxies = []netip.Prefix{{}}
	if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, ErrInvalidKnownProxies) {
		t.Errorf("an invalid network: %v", err)
	}
	changed.KnownProxies, changed.KnownProxyNames = nil, []string{"not a name"}
	if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, ErrInvalidKnownProxies) {
		t.Errorf("an invalid name: %v", err)
	}
	changed.KnownProxyNames = []string{"traefik"}
	for i := range MaxKnownProxies {
		changed.KnownProxies = append(changed.KnownProxies, netip.MustParsePrefix("10.0.0."+strconv.Itoa(i)+"/32"))
	}
	if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, ErrInvalidKnownProxies) {
		t.Errorf("%d networks and a name: %v", MaxKnownProxies, err)
	}
}
