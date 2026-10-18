package torrent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"syscall"
	"time"
)

// ErrNoRoute reports torrents played without a route: neither a network
// interface nor a SOCKS5 proxy is set.
var ErrNoRoute = errors.New("no route is set for torrents")

// ErrNoUDP reports a route that carries no UDP: a SOCKS5 proxy without
// UDP ASSOCIATE. UDP trackers and the DHT stay off over it.
var ErrNoUDP = errors.New("the route carries no UDP")

// ErrInterfaceUnsupported reports a network interface route on a system
// other than Linux, which cannot bind sockets to a device.
var ErrInterfaceUnsupported = errors.New("a network interface route needs Linux")

// The kinds of RouteError.
const (
	InterfaceMissing     = "interface_missing"
	InterfaceDown        = "interface_down"
	InterfaceNoAddress   = "interface_no_address"
	InterfaceUnsupported = "interface_unsupported"
	ProxyUnreachable     = "proxy_unreachable"
	ProxyRefused         = "proxy_refused"
)

// RouteError is why a route cannot carry the torrents: Kind tells what,
// Route names the interface or the proxy's address, and Err, when set,
// is the system's error.
type RouteError struct {
	Kind, Route string
	Err         error
}

func (e *RouteError) Error() string {
	var text string
	switch e.Kind {
	case InterfaceMissing:
		text = "the network interface " + e.Route + " is missing"
	case InterfaceDown:
		text = "the network interface " + e.Route + " is down"
	case InterfaceNoAddress:
		text = "the network interface " + e.Route + " has no address"
	case InterfaceUnsupported:
		text = "a network interface route needs Linux"
	case ProxyUnreachable:
		text = "the SOCKS5 proxy " + e.Route + " does not answer"
	case ProxyRefused:
		text = "the SOCKS5 proxy " + e.Route + " refused the user and password"
	}
	if e.Err != nil {
		text += ": " + e.Err.Error()
	}
	return text
}

func (e *RouteError) Unwrap() error {
	if e.Kind == InterfaceUnsupported {
		return ErrInterfaceUnsupported
	}
	return e.Err
}

// Route carries every socket of the torrents, and nothing is sent another
// way: a socket the route cannot open is not opened.
type Route interface {
	// Dial connects over TCP to address, a host name or an IP address
	// with a port; names are looked up over the route.
	Dial(ctx context.Context, address string) (net.Conn, error)
	// ListenPacket opens a UDP socket over the route, ErrNoUDP when it
	// carries none.
	ListenPacket(ctx context.Context) (PacketConn, error)
	// Check reports why the route cannot carry the torrents now, nil
	// when it can.
	Check(ctx context.Context) error
	// String describes the route for logs and Health.
	String() string
	Close() error
}

// PacketConn is a UDP socket of a route.
type PacketConn interface {
	// WriteTo sends p to address, a host name or an IP address with a
	// port.
	WriteTo(p []byte, address string) error
	// ReadFrom reads a datagram and the IP address and port it came from.
	ReadFrom(p []byte) (int, netip.AddrPort, error)
	SetReadDeadline(t time.Time) error
	Close() error
}

// InterfaceRoute binds every socket to the network interface name, such as
// a WireGuard interface: it is up with an address, or nothing is sent.
// Names are looked up by the name servers of /etc/resolv.conf over the
// interface; one on the loopback, which would look them up another way,
// is not asked.
func InterfaceRoute(name string) Route {
	r := &interfaceRoute{name: name}
	r.resolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		if ap, err := netip.ParseAddrPort(address); err == nil && ap.Addr().IsLoopback() {
			return nil, fmt.Errorf("the name server %s is on the loopback, off the route", ap.Addr())
		}
		if err := r.Check(ctx); err != nil {
			return nil, err
		}
		d := net.Dialer{Control: r.control}
		return d.DialContext(ctx, network, address)
	}}
	return r
}

type interfaceRoute struct {
	name     string
	resolver *net.Resolver
}

func (r *interfaceRoute) String() string { return "interface " + r.name }

func (r *interfaceRoute) Close() error { return nil }

// control binds a socket to the interface before it connects or sends.
func (r *interfaceRoute) control(_, _ string, raw syscall.RawConn) error {
	var bindErr error
	if err := raw.Control(func(fd uintptr) { bindErr = bindToDevice(fd, r.name) }); err != nil {
		return err
	}
	if bindErr != nil {
		return fmt.Errorf("bind a socket to the interface %s: %w", r.name, bindErr)
	}
	return nil
}

func (r *interfaceRoute) Check(context.Context) error {
	if !bindsDevices {
		return &RouteError{Kind: InterfaceUnsupported, Route: r.name}
	}
	iface, err := net.InterfaceByName(r.name)
	if err != nil {
		return &RouteError{Kind: InterfaceMissing, Route: r.name}
	}
	if iface.Flags&net.FlagUp == 0 {
		return &RouteError{Kind: InterfaceDown, Route: r.name}
	}
	addrs, err := iface.Addrs()
	if err == nil {
		for _, addr := range addrs {
			if prefix, ok := addr.(*net.IPNet); ok && !prefix.IP.IsLinkLocalUnicast() && !prefix.IP.IsUnspecified() {
				return nil
			}
		}
	}
	return &RouteError{Kind: InterfaceNoAddress, Route: r.name}
}

func (r *interfaceRoute) Dial(ctx context.Context, address string) (net.Conn, error) {
	if err := r.Check(ctx); err != nil {
		return nil, err
	}
	target, err := r.resolve(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Control: r.control}
	return d.DialContext(ctx, "tcp", target.String())
}

// resolve looks a host up over the interface, IP addresses as they are.
func (r *interfaceRoute) resolve(ctx context.Context, network, address string) (netip.AddrPort, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return netip.AddrPort{}, err
	}
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("invalid port in %q", address)
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return netip.AddrPortFrom(ip.Unmap(), uint16(n)), nil
	}
	ips, err := r.resolver.LookupNetIP(ctx, "ip4", host)
	if err != nil || len(ips) == 0 {
		return netip.AddrPort{}, fmt.Errorf("look %s up over the interface %s: %w", host, r.name, err)
	}
	return netip.AddrPortFrom(ips[0].Unmap(), uint16(n)), nil
}

func (r *interfaceRoute) ListenPacket(ctx context.Context) (PacketConn, error) {
	if err := r.Check(ctx); err != nil {
		return nil, err
	}
	lc := net.ListenConfig{Control: r.control}
	conn, err := lc.ListenPacket(ctx, "udp4", "0.0.0.0:0")
	if err != nil {
		return nil, err
	}
	return &interfacePacketConn{route: r, conn: conn.(*net.UDPConn)}, nil
}

type interfacePacketConn struct {
	route *interfaceRoute
	conn  *net.UDPConn
}

func (c *interfacePacketConn) WriteTo(p []byte, address string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	target, err := c.route.resolve(ctx, "udp", address)
	if err != nil {
		return err
	}
	_, err = c.conn.WriteToUDPAddrPort(p, target)
	return err
}

func (c *interfacePacketConn) ReadFrom(p []byte) (int, netip.AddrPort, error) {
	n, from, err := c.conn.ReadFromUDPAddrPort(p)
	return n, netip.AddrPortFrom(from.Addr().Unmap(), from.Port()), err
}

func (c *interfacePacketConn) SetReadDeadline(t time.Time) error { return c.conn.SetReadDeadline(t) }

func (c *interfacePacketConn) Close() error { return c.conn.Close() }
