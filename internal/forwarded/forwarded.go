// Package forwarded tells the address of a request's client behind the
// reverse proxies an administrator lists under Settings › Security (Known
// proxies), and which proxies are missing from that list.
package forwarded

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	// unknownMemory is how long after its last request with forwarded
	// headers an address that is not a known proxy stays named.
	unknownMemory = time.Hour
	// maxUnknown bounds the addresses remembered; past it, the one seen
	// longest ago is forgotten.
	maxUnknown = 16
)

type contextKey struct{}

// Resolve returns the address of r's client when the proxies in known are
// trusted. A connection from a known proxy is from the rightmost address
// of X-Forwarded-For that is not a known proxy, read across every line of
// the header, or from its leftmost address when all are known proxies;
// without X-Forwarded-For, from X-Real-IP. An address it cannot read stops
// the walk: the request is from the proxy that added it. A connection from
// any other address is from that address, and its forwarded headers are
// ignored, since its client can write them. The address is invalid when
// r.RemoteAddr holds none.
func Resolve(r *http.Request, known []netip.Prefix) netip.Addr {
	peer := connection(r)
	if !trusted(known, peer) {
		return peer
	}
	return behind(r, peer, known)
}

// Client returns the address of r's client, as Proxies.Handler resolved
// it, else the connection's: the address sign-ins are throttled by, that
// devices show, and that tells the local network.
func Client(r *http.Request) string {
	if client, ok := r.Context().Value(contextKey{}).(netip.Addr); ok {
		return client.String()
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Proxies resolves the client of every request through the known proxies
// of the current settings, and remembers the addresses in the local
// network that send forwarded headers without being a known proxy: a proxy
// missing from the setting, which System › Health names.
type Proxies struct {
	known func() []netip.Prefix
	now   func() time.Time

	mu sync.Mutex
	// unknown holds when each address was last seen sending forwarded
	// headers, at most maxUnknown of them.
	unknown map[netip.Addr]time.Time
}

// New returns Proxies trusting the proxies known returns, read at each
// request.
func New(known func() []netip.Prefix) *Proxies {
	return &Proxies{known: known, now: time.Now, unknown: map[netip.Addr]time.Time{}}
}

// Handler has next see the client Resolve finds through Client.
func (p *Proxies) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		known := p.known()
		peer := connection(r)
		if !trusted(known, peer) {
			p.notice(r, peer)
			next.ServeHTTP(w, r)
			return
		}
		// The request is passed on as it is when it is from the proxy
		// itself, which Client reads from the connection.
		if client := behind(r, peer, known); client != peer {
			r = r.WithContext(context.WithValue(r.Context(), contextKey{}, client))
		}
		next.ServeHTTP(w, r)
	})
}

// Unknown returns the addresses that sent forwarded headers in the last
// hour and are still not known proxies, in order.
func (p *Proxies) Unknown() []netip.Addr {
	known := p.known()
	cutoff := p.now().Add(-unknownMemory)
	p.mu.Lock()
	defer p.mu.Unlock()
	var addresses []netip.Addr
	for address, seen := range p.unknown {
		if seen.After(cutoff) && !trusted(known, address) {
			addresses = append(addresses, address)
		}
	}
	slices.SortFunc(addresses, netip.Addr.Compare)
	return addresses
}

// notice remembers peer when r carries forwarded headers. Only addresses
// in the local network count, where a reverse proxy usually is: a client
// on the internet can send the headers itself, and naming it would invite
// an administrator to trust it.
func (p *Proxies) notice(r *http.Request, peer netip.Addr) {
	if !peer.IsValid() || !(peer.IsLoopback() || peer.IsPrivate() || peer.IsLinkLocalUnicast()) ||
		(r.Header.Get("X-Forwarded-For") == "" && r.Header.Get("X-Real-IP") == "") {
		return
	}
	now := p.now()
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, remembered := p.unknown[peer]; !remembered && len(p.unknown) >= maxUnknown {
		var oldest netip.Addr
		for address, seen := range p.unknown {
			if !oldest.IsValid() || seen.Before(p.unknown[oldest]) {
				oldest = address
			}
		}
		delete(p.unknown, oldest)
	}
	p.unknown[peer] = now
}

// behind finds the client of a request from the known proxy at peer (see
// Resolve).
func behind(r *http.Request, peer netip.Addr, known []netip.Prefix) netip.Addr {
	client, listed := peer, false
	lines := r.Header.Values("X-Forwarded-For")
	for i := len(lines) - 1; i >= 0; i-- {
		rest := lines[i]
		for rest != "" {
			entry := rest
			if comma := strings.LastIndexByte(rest, ','); comma >= 0 {
				entry, rest = rest[comma+1:], rest[:comma]
			} else {
				rest = ""
			}
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			listed = true
			address, ok := parse(entry)
			if !ok {
				return client
			}
			if !trusted(known, address) {
				return address
			}
			client = address
		}
	}
	if listed {
		return client
	}
	if address, ok := parse(strings.TrimSpace(r.Header.Get("X-Real-IP"))); ok {
		return address
	}
	return peer
}

// connection returns the address r's connection comes from, invalid when
// r.RemoteAddr holds none.
func connection(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return address.Unmap().WithZone("")
}

// parse reads an address of a forwarded header: an IPv4 or IPv6 address,
// bracketed or not, with or without a port, as proxies write them.
func parse(entry string) (netip.Addr, bool) {
	address, err := netip.ParseAddr(entry)
	if err != nil {
		withPort, portErr := netip.ParseAddrPort(entry)
		if portErr == nil {
			address, err = withPort.Addr(), nil
		} else if len(entry) > 2 && entry[0] == '[' && entry[len(entry)-1] == ']' {
			address, err = netip.ParseAddr(entry[1 : len(entry)-1])
		}
	}
	if err != nil {
		return netip.Addr{}, false
	}
	return address.Unmap().WithZone(""), true
}

// trusted reports whether address is in one of the known networks.
func trusted(known []netip.Prefix, address netip.Addr) bool {
	if !address.IsValid() {
		return false
	}
	for _, network := range known {
		if network.Contains(address) {
			return true
		}
	}
	return false
}
