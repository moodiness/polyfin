// Package reachtest stands, in tests, for the networks a stremio.Client
// reaches. Test servers listen on the loopback: a network takes one for a
// host at another address, public or local, and answers the names a test
// makes up in place of the system's resolver. Each of the client's rules
// still applies to the address it believes it connects to.
package reachtest

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/stremio"
)

// Network is the hosts and names a client reaches in a test.
type Network struct {
	mu sync.Mutex
	// servers maps an address and port a client connects to, such as
	// 203.0.113.7:80, to the loopback address of the test server that
	// stands for it.
	servers map[string]string
	// names are the answers of each name, lookup after lookup, the last
	// for every later one; asked counts the lookups of each.
	names map[string][][]netip.Addr
	asked map[string]int
	// hosts counts the public hosts given out (see Public).
	hosts int
}

// New returns a network client reaches through, without any host or name.
// It is called before the client is used.
func New(client *stremio.Client) *Network {
	n := &Network{servers: map[string]string{}, names: map[string][][]netip.Addr{}, asked: map[string]int{}}
	client.WrapDials(n.dial)
	return n
}

// Serve has the test server at server, its address on the loopback as
// httptest gives it, stand for the host at address, an IP address with
// the port it is reached on, or alone for the server's own port, and
// returns the address it is then reached at, such as
// http://192.168.1.50:49152.
func (n *Network) Serve(t *testing.T, address, server string) string {
	t.Helper()
	parsed, err := url.Parse(server)
	if err != nil || parsed.Port() == "" {
		t.Fatalf("no test server at %q: %v", server, err)
	}
	at := address
	if ip, err := netip.ParseAddr(address); err == nil {
		at = net.JoinHostPort(ip.String(), parsed.Port())
	} else if _, err := netip.ParseAddrPort(address); err != nil {
		t.Fatalf("%q is no address: %v", address, err)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.servers[at] = parsed.Host
	return "http://" + at
}

// Public has the test server at server stand for a host on the internet,
// at an address of its own in a documentation range, which counts as
// public, and returns the address it is then reached at.
func (n *Network) Public(t *testing.T, server string) string {
	t.Helper()
	n.mu.Lock()
	n.hosts++
	host := n.hosts
	n.mu.Unlock()
	if host > 254 {
		t.Fatal("no documentation address left")
	}
	return n.Serve(t, fmt.Sprintf("203.0.113.%d", host), server)
}

// Name sets what the resolver answers for host: the addresses of the
// first lookup, then those of the next, and so on; the last ones answer
// every later lookup. The address of a server at host keeps the port of
// the address Serve or Public returned for it.
func (n *Network) Name(host string, answers ...[]string) {
	parsed := make([][]netip.Addr, len(answers))
	for i, answer := range answers {
		for _, ip := range answer {
			parsed[i] = append(parsed[i], netip.MustParseAddr(ip))
		}
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.names[host], n.asked[host] = parsed, 0
}

// Lookups counts the lookups of host since Name set its answers.
func (n *Network) Lookups(host string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.asked[host]
}

// resolve returns the addresses of host: its own when it is an address,
// else those the test gave its name; false for a name it gave none.
func (n *Network) resolve(host string) ([]netip.Addr, bool) {
	if ip, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{ip}, true
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	answers, ok := n.names[host]
	if !ok || len(answers) == 0 {
		return nil, ok
	}
	lookup := min(n.asked[host], len(answers)-1)
	n.asked[host]++
	return answers[lookup], true
}

// dial is how a client whose rule is allowed connects on the network:
// it tries the addresses of the host in turn, as a dialer does, refuses
// those its rule refuses, reaches the test server that stands for an
// address, and leaves any other to the client's own dial.
func (n *Network) dial(allowed func(netip.Addr) error, own stremio.Dial) stremio.Dial {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, known := n.resolve(host)
		if !known {
			return own(ctx, network, address)
		}
		var failure error = &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		for _, ip := range ips {
			if err := allowed(ip); err != nil {
				failure = err
				continue
			}
			at := net.JoinHostPort(ip.String(), port)
			n.mu.Lock()
			server, stands := n.servers[at]
			n.mu.Unlock()
			var conn net.Conn
			if stands {
				conn, err = new(net.Dialer).DialContext(ctx, network, server)
			} else {
				conn, err = own(ctx, network, at)
			}
			if err == nil {
				return conn, nil
			}
			failure = err
		}
		return nil, failure
	}
}
