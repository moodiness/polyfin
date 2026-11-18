package torrent

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
)

// errNotPublic reports a tracker or a DHT node that is not on the
// internet: nothing is sent to it.
var errNotPublic = errors.New("the address is not on the internet")

// public reports whether ip is on the internet (see Config.Public).
func (e *Engine) public(ip netip.Addr) bool {
	if e.config.Public != nil {
		return e.config.Public(ip)
	}
	return stremio.Public(ip)
}

// resolver is a route that looks names up itself, as the interface and
// WireGuard routes do: lookup returns the address the route would connect
// to over TCP, or send a datagram to, for address, a host and a port. A
// SOCKS5 proxy looks names up on its side, and tells no address.
type resolver interface {
	lookup(ctx context.Context, address string, tcp bool) (netip.AddrPort, error)
}

// publicRoute is a route to what Polyfin and the administrator did not
// choose: the trackers and DHT nodes a stream's sources name, and the
// nodes the DHT tells. It reaches public addresses only, over the route
// it wraps and no other: an address that is not public is refused, and a
// name is looked up once, checked, and reached at the address checked.
// Over a route that looks no name up itself, a name is left to the proxy.
type publicRoute struct {
	Route
	public func(netip.Addr) bool
}

// allows reports whether host may be reached: a public IP address, or a
// name. A host that ends in a number is an address in another notation,
// as "127.1" or "0x7f000001", which a proxy may read as one: it is no
// name, and is not allowed.
func (r publicRoute) allows(host string) bool {
	if ip, err := netip.ParseAddr(host); err == nil {
		return r.public(ip)
	}
	last := strings.TrimSuffix(host, ".")
	last = strings.ToLower(last[strings.LastIndexByte(last, '.')+1:])
	if last == "" {
		return false
	}
	if hex, ok := strings.CutPrefix(last, "0x"); ok {
		return strings.Trim(hex, "0123456789abcdef") != ""
	}
	return strings.Trim(last, "0123456789") != ""
}

// allowsTracker reports whether the host of a tracker's URL may be
// reached (see allows).
func (r publicRoute) allowsTracker(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && r.allows(u.Hostname())
}

// allowsNode reports whether the host of a DHT node, host:port, may be
// reached (see allows).
func (r publicRoute) allowsNode(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	return err == nil && r.allows(host)
}

// target is the address to reach for address, a host and a port: the
// same for a public IP address, the public address a name was looked up
// as when the route looks names up, and the name itself when it does
// not.
func (r publicRoute) target(ctx context.Context, address string, tcp bool) (string, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return "", err
	}
	if !r.allows(host) {
		return "", errNotPublic
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return address, nil
	}
	looked, ok := r.Route.(resolver)
	if !ok {
		return address, nil
	}
	found, err := looked.lookup(ctx, address, tcp)
	if err != nil {
		return "", err
	}
	if !r.public(found.Addr()) {
		return "", errNotPublic
	}
	return found.String(), nil
}

func (r publicRoute) Dial(ctx context.Context, address string) (net.Conn, error) {
	target, err := r.target(ctx, address, true)
	if err != nil {
		return nil, err
	}
	return r.Route.Dial(ctx, target)
}

func (r publicRoute) ListenPacket(ctx context.Context) (PacketConn, error) {
	conn, err := r.Route.ListenPacket(ctx)
	if err != nil {
		return nil, err
	}
	return publicPacketConn{PacketConn: conn, route: r}, nil
}

// publicPacketConn is a UDP socket of a publicRoute.
type publicPacketConn struct {
	PacketConn
	route publicRoute
}

func (c publicPacketConn) WriteTo(p []byte, address string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	target, err := c.route.target(ctx, address, false)
	if err != nil {
		return err
	}
	return c.PacketConn.WriteTo(p, target)
}

func (c publicPacketConn) WriteToAddrPort(p []byte, to netip.AddrPort) error {
	if !c.route.public(to.Addr()) {
		return errNotPublic
	}
	return c.PacketConn.WriteToAddrPort(p, to)
}
