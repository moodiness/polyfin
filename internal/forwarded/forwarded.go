// Package forwarded tells the address of a request's client behind the
// reverse proxies an administrator lists under Settings › Security (Known
// proxies), by address, network or host name, and which proxies are
// missing from that list.
package forwarded

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// unknownMemory is how long after its last request with forwarded
	// headers an address that is not a known proxy stays named.
	unknownMemory = time.Hour
	// maxUnknown bounds the addresses remembered; past it, the one seen
	// longest ago is forgotten.
	maxUnknown = 16
	// lookupEvery is how often the host names of known proxies are
	// resolved again, besides at each change of the settings.
	lookupEvery = time.Minute
	// lookupTimeout bounds the resolution of one host name.
	lookupTimeout = 10 * time.Second
	// lookupSoonest is how often, at most, an address that sends forwarded
	// headers without being a known proxy has the names looked up again.
	lookupSoonest = 10 * time.Second
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
	trust := trust{known: known}
	peer := connection(r)
	if !trust.contains(peer) {
		return peer
	}
	return behind(r, peer, trust)
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

// Resolver looks up the addresses of a host name, as *net.Resolver does.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Name is a host name listed in Known proxies, with the addresses it
// resolved to at Resolved, sorted: none when it resolves to nothing.
type Name struct {
	Name      string
	Addresses []netip.Addr
	Resolved  time.Time
}

// Proxies resolves the client of every request through the known proxies
// of the current settings, and remembers the addresses in the local
// network that send forwarded headers without being a known proxy: a proxy
// missing from the setting, which System › Health names.
type Proxies struct {
	known    func() []netip.Prefix
	resolver Resolver
	now      func() time.Time

	// named is what the host names of the setting last resolved to,
	// never nil.
	named atomic.Pointer[resolution]
	// lookupMu has one Lookup at a time publish its names.
	lookupMu sync.Mutex

	mu sync.Mutex
	// unknown holds when each address was last seen sending forwarded
	// headers, at most maxUnknown of them.
	unknown map[netip.Addr]time.Time
	// woken is when such an address last had the names looked up again,
	// through wake, which Watch follows.
	woken time.Time
	wake  chan struct{}
}

// resolution is what the host names of Known proxies resolved to: each
// name in the order of the setting, and every address of all of them,
// sorted and once each.
type resolution struct {
	names     []Name
	addresses []netip.Addr
}

// New returns Proxies trusting the addresses and networks known returns,
// read at each request, and the addresses the host names given to Lookup
// resolve to, looked up with resolver; nil uses the system's resolver,
// which in a Docker network asks Docker's own DNS.
func New(known func() []netip.Prefix, resolver Resolver) *Proxies {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	p := &Proxies{known: known, resolver: resolver, now: time.Now, unknown: map[netip.Addr]time.Time{}, wake: make(chan struct{}, 1)}
	p.named.Store(&resolution{})
	return p
}

// Handler has next see the client Resolve finds through Client.
func (p *Proxies) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trust := p.trust()
		peer := connection(r)
		if !trust.contains(peer) {
			p.notice(r, peer)
			next.ServeHTTP(w, r)
			return
		}
		// The request is passed on as it is when it is from the proxy
		// itself, which Client reads from the connection.
		if client := behind(r, peer, trust); client != peer {
			r = r.WithContext(context.WithValue(r.Context(), contextKey{}, client))
		}
		next.ServeHTTP(w, r)
	})
}

// Unknown returns the addresses that sent forwarded headers in the last
// hour and are still not known proxies, in order.
func (p *Proxies) Unknown() []netip.Addr {
	trust := p.trust()
	cutoff := p.now().Add(-unknownMemory)
	p.mu.Lock()
	defer p.mu.Unlock()
	var addresses []netip.Addr
	for address, seen := range p.unknown {
		if seen.After(cutoff) && !trust.contains(address) {
			addresses = append(addresses, address)
		}
	}
	slices.SortFunc(addresses, netip.Addr.Compare)
	return addresses
}

// Names returns the host names of Known proxies with what they last
// resolved to, in the order of the setting.
func (p *Proxies) Names() []Name {
	return p.named.Load().names
}

// Lookup resolves names, the host names of Known proxies, and trusts the
// addresses they resolve to from then on, instead of those they resolved
// to before: a name that resolves to nothing, or fails to resolve, trusts
// nothing. The names no longer listed stop being trusted at once, before
// the others are looked up.
func (p *Proxies) Lookup(ctx context.Context, names []string) {
	p.lookupMu.Lock()
	defer p.lookupMu.Unlock()
	kept := &resolution{}
	for _, name := range p.named.Load().names {
		if slices.Contains(names, name.Name) {
			kept.names = append(kept.names, name)
		}
	}
	kept.index()
	p.named.Store(kept)

	resolved := &resolution{names: make([]Name, len(names))}
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Go(func() {
			resolved.names[i] = Name{Name: name, Addresses: p.lookup(ctx, name), Resolved: p.now()}
		})
	}
	wg.Wait()
	if ctx.Err() != nil {
		return
	}
	resolved.index()
	p.named.Store(resolved)
}

// Watch looks up the names names returns now, then again at each change
// of the settings changed tells, every minute, and when an address in the
// local network sends forwarded headers without being a known proxy, at
// most every lookupSoonest: a proxy that starts after Polyfin, or comes
// back with another address, is trusted within seconds. It runs until ctx
// ends, and returns after the first lookup.
func (p *Proxies) Watch(ctx context.Context, names func() []string, changed func() <-chan struct{}) {
	next := changed()
	p.Lookup(ctx, names())
	go func() {
		ticker := time.NewTicker(lookupEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-next:
				next = changed()
			case <-ticker.C:
			case <-p.wake:
			}
			p.Lookup(ctx, names())
		}
	}()
}

// lookup returns the addresses name resolves to, sorted and once each;
// none when it fails.
func (p *Proxies) lookup(ctx context.Context, name string) []netip.Addr {
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	found, err := p.resolver.LookupNetIP(ctx, "ip", name)
	if err != nil {
		return []netip.Addr{}
	}
	addresses := make([]netip.Addr, 0, len(found))
	for _, address := range found {
		addresses = append(addresses, address.Unmap().WithZone(""))
	}
	slices.SortFunc(addresses, netip.Addr.Compare)
	return slices.Compact(addresses)
}

// index gathers the addresses of every name.
func (r *resolution) index() {
	for _, name := range r.names {
		r.addresses = append(r.addresses, name.Addresses...)
	}
	slices.SortFunc(r.addresses, netip.Addr.Compare)
	r.addresses = slices.Compact(r.addresses)
}

// trust returns the known proxies of the moment.
func (p *Proxies) trust() trust {
	return trust{known: p.known(), named: p.named.Load().addresses}
}

// notice remembers peer when r carries forwarded headers, and has Watch
// look the names up again while some are listed (see lookupSoonest); r is
// judged as things are now. Only addresses in the local network count,
// where a reverse proxy usually is: a client on the internet can send the
// headers itself, and naming it would invite an administrator to trust
// it.
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
	if len(p.named.Load().names) > 0 && now.Sub(p.woken) >= lookupSoonest {
		p.woken = now
		select {
		case p.wake <- struct{}{}:
		default:
		}
	}
}

// behind finds the client of a request from the known proxy at peer (see
// Resolve).
func behind(r *http.Request, peer netip.Addr, trust trust) netip.Addr {
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
			if !trust.contains(address) {
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

// trust is the known proxies of a moment: the networks of the setting,
// and the addresses its host names resolved to, sorted.
type trust struct {
	known []netip.Prefix
	named []netip.Addr
}

// contains reports whether address is a known proxy.
func (t trust) contains(address netip.Addr) bool {
	if !address.IsValid() {
		return false
	}
	for _, network := range t.known {
		if network.Contains(address) {
			return true
		}
	}
	_, found := slices.BinarySearchFunc(t.named, address, netip.Addr.Compare)
	return found
}
