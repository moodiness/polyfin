package stremio

import (
	"net/netip"
	"testing"
)

// Confined requests reach the internet only: neither a local network nor
// a range reserved for something else, which on some networks leads to a
// host of the network. An IPv6 address standing for an IPv4 address is
// judged as that address, so that a network reaching IPv4 through NAT64
// still reaches public hosts. The ranges kept for documentation, which
// lead nowhere, stay public.
func TestPublicAddressesAreTheInternets(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		address string
		public  bool
	}{
		// The internet.
		{"1.1.1.1", true},
		{"93.184.216.34", true},
		{"223.255.255.254", true},
		{"::ffff:93.184.216.34", true},
		{"2606:4700:4700::1111", true},
		{"2a00:1450:4007:80e::200e", true},
		// Documentation.
		{"192.0.2.1", true},
		{"198.51.100.1", true},
		{"203.0.113.1", true},
		{"2001:db8::1", true},
		// A public IPv4 address behind NAT64, and as a 6to4 address.
		{"64:ff9b::5db8:d822", true},
		{"2002:5db8:d822::1", true},

		// Loopback, unspecified.
		{"127.0.0.1", false},
		{"127.255.255.254", false},
		{"::1", false},
		{"0.0.0.0", false},
		{"::", false},
		// Private.
		{"10.0.0.1", false},
		{"172.16.0.1", false},
		{"172.31.255.255", false},
		{"192.168.1.1", false},
		// Link-local.
		{"169.254.169.254", false},
		{"169.254.0.1", false},
		{"fe80::1", false},
		{"fe80::1%eth0", false},
		// Carrier-grade NAT.
		{"100.64.0.1", false},
		{"100.127.255.255", false},
		// Unique local.
		{"fc00::1", false},
		{"fd00:ec2::254", false},
		// Multicast, broadcast.
		{"224.0.0.1", false},
		{"239.255.255.250", false},
		{"ff02::1", false},
		{"255.255.255.255", false},
		// IPv4-mapped.
		{"::ffff:127.0.0.1", false},
		{"::ffff:10.0.0.1", false},
		{"::ffff:169.254.169.254", false},

		// NAT64: a local address behind the gateway, and the prefix for
		// use within a network.
		{"64:ff9b::7f00:1", false},
		{"64:ff9b::c0a8:101", false},
		{"64:ff9b::a9fe:a9fe", false},
		{"64:ff9b::6440:1", false},
		{"64:ff9b:1::c0a8:101", false},
		{"64:ff9b:1::5db8:d822", false},
		// 6to4 of a local address.
		{"2002:c0a8:101::1", false},
		{"2002:7f00:1::1", false},
		{"2002:a9fe:a9fe::1", false},
		// Teredo.
		{"2001:0:4136:e378:8000:63bf:3fff:fdd2", false},
		// IPv4-compatible and IPv4-translated.
		{"::127.0.0.1", false},
		{"::192.168.1.1", false},
		{"::ffff:0:192.168.1.1", false},
		{"::ffff:0:93.184.216.34", false},
		// Site-local, with and without a zone.
		{"fec0::1", false},
		{"fec0::1%eth0", false},
		// Reserved IPv4 ranges.
		{"0.0.0.1", false},
		{"0.255.255.255", false},
		{"192.0.0.8", false},
		{"192.88.99.1", false},
		{"198.18.0.1", false},
		{"198.19.255.255", false},
		{"240.0.0.1", false},
		{"254.255.255.255", false},
		{"168.63.129.16", false},
		{"::ffff:198.18.0.1", false},
	} {
		if got := Public(netip.MustParseAddr(tc.address)); got != tc.public {
			t.Errorf("%s: public %t, want %t", tc.address, got, tc.public)
		}
	}
}
