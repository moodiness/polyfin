package torrent

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"
)

// SOCKS5 (RFC 1928) and its user name and password authentication (RFC
// 1929).
const (
	socksVersion   = 5
	socksNoAuth    = 0
	socksPassword  = 2
	socksNoMethod  = 0xff
	socksConnect   = 1
	socksAssociate = 3
	socksIPv4      = 1
	socksDomain    = 3
	socksIPv6      = 4
	// socksUnsupported is the reply to a command the server does not
	// offer, as UDP ASSOCIATE.
	socksUnsupported = 7
)

// proxyTimeout bounds a proxy's handshake, and checkedFor how long a
// proxy that answered is not asked again.
const (
	proxyTimeout = 10 * time.Second
	checkedFor   = 5 * time.Second
)

// ProxyRoute sends every socket through the SOCKS5 proxy at address, with
// user and password when user is set: TCP through CONNECT, UDP through UDP
// ASSOCIATE when the proxy offers it. Names are looked up by the proxy.
// Nothing listens: no peer can connect in.
func ProxyRoute(address, user, password string) Route {
	return &proxyRoute{address: address, user: user, password: password}
}

type proxyRoute struct {
	address, user, password string

	mu        sync.Mutex
	checkedAt time.Time
	checkErr  error
}

func (r *proxyRoute) String() string { return "SOCKS5 proxy " + r.address }

func (r *proxyRoute) Close() error { return nil }

func (r *proxyRoute) Check(ctx context.Context) error {
	r.mu.Lock()
	if !r.checkedAt.IsZero() && time.Since(r.checkedAt) < checkedFor {
		err := r.checkErr
		r.mu.Unlock()
		return err
	}
	r.mu.Unlock()
	conn, err := r.handshake(ctx)
	if err == nil {
		conn.Close()
	}
	r.mu.Lock()
	r.checkedAt, r.checkErr = time.Now(), err
	r.mu.Unlock()
	return err
}

// handshake connects to the proxy and authenticates.
func (r *proxyRoute) handshake(ctx context.Context) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, proxyTimeout)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", r.address)
	if err != nil {
		return nil, r.unreachable(err)
	}
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	if err := r.authenticate(conn); err != nil {
		conn.Close()
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	return conn, nil
}

func (r *proxyRoute) authenticate(conn net.Conn) error {
	method := byte(socksNoAuth)
	if r.user != "" {
		method = socksPassword
	}
	if _, err := conn.Write([]byte{socksVersion, 1, method}); err != nil {
		return r.unreachable(err)
	}
	var reply [2]byte
	if _, err := io.ReadFull(conn, reply[:]); err != nil {
		return r.unreachable(err)
	}
	if reply[0] != socksVersion || reply[1] != method {
		return &RouteError{Kind: ProxyRefused, Route: r.address}
	}
	if method != socksPassword {
		return nil
	}
	if len(r.user) > 255 || len(r.password) > 255 {
		return errors.New("the SOCKS5 proxy's user or password is too long")
	}
	request := []byte{1, byte(len(r.user))}
	request = append(request, r.user...)
	request = append(request, byte(len(r.password)))
	request = append(request, r.password...)
	if _, err := conn.Write(request); err != nil {
		return r.unreachable(err)
	}
	if _, err := io.ReadFull(conn, reply[:]); err != nil {
		return r.unreachable(err)
	}
	if reply[1] != 0 {
		return &RouteError{Kind: ProxyRefused, Route: r.address}
	}
	return nil
}

// unreachable is the error of a proxy that does not answer.
func (r *proxyRoute) unreachable(err error) error {
	return &RouteError{Kind: ProxyUnreachable, Route: r.address, Err: err}
}

// socksAddress encodes host:port as a SOCKS5 address.
func socksAddress(address string) ([]byte, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid port in %q", address)
	}
	var b []byte
	if ip, err := netip.ParseAddr(host); err == nil {
		ip = ip.Unmap()
		if ip.Is4() {
			b = append([]byte{socksIPv4}, ip.AsSlice()...)
		} else {
			b = append([]byte{socksIPv6}, ip.AsSlice()...)
		}
	} else {
		if len(host) == 0 || len(host) > 255 {
			return nil, fmt.Errorf("invalid host in %q", address)
		}
		b = append([]byte{socksDomain, byte(len(host))}, host...)
	}
	return binary.BigEndian.AppendUint16(b, uint16(n)), nil
}

// readSocksAddress reads a SOCKS5 address from r.
func readSocksAddress(r io.Reader) (string, error) {
	var kind [1]byte
	if _, err := io.ReadFull(r, kind[:]); err != nil {
		return "", err
	}
	var host string
	switch kind[0] {
	case socksIPv4, socksIPv6:
		raw := make([]byte, 4)
		if kind[0] == socksIPv6 {
			raw = make([]byte, 16)
		}
		if _, err := io.ReadFull(r, raw); err != nil {
			return "", err
		}
		ip, _ := netip.AddrFromSlice(raw)
		host = ip.String()
	case socksDomain:
		var length [1]byte
		if _, err := io.ReadFull(r, length[:]); err != nil {
			return "", err
		}
		raw := make([]byte, length[0])
		if _, err := io.ReadFull(r, raw); err != nil {
			return "", err
		}
		host = string(raw)
	default:
		return "", errors.New("the SOCKS5 proxy answered an unknown address type")
	}
	var port [2]byte
	if _, err := io.ReadFull(r, port[:]); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(port[:])))), nil
}

// command sends a request and reads its reply, returning the address the
// proxy bound.
func command(conn net.Conn, cmd byte, address string) (string, error) {
	target, err := socksAddress(address)
	if err != nil {
		return "", err
	}
	if _, err := conn.Write(append([]byte{socksVersion, cmd, 0}, target...)); err != nil {
		return "", err
	}
	var reply [3]byte
	if _, err := io.ReadFull(conn, reply[:]); err != nil {
		return "", err
	}
	if reply[1] == socksUnsupported && cmd == socksAssociate {
		return "", ErrNoUDP
	}
	if reply[0] != socksVersion || reply[1] != 0 {
		return "", fmt.Errorf("the SOCKS5 proxy refused the request (reply %d)", reply[1])
	}
	return readSocksAddress(conn)
}

func (r *proxyRoute) Dial(ctx context.Context, address string) (net.Conn, error) {
	conn, err := r.handshake(ctx)
	if err != nil {
		r.failed(err)
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()
	_ = conn.SetDeadline(time.Now().Add(proxyTimeout))
	if _, err := command(conn, socksConnect, address); err != nil {
		conn.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if !stop() {
		conn.Close()
		return nil, ctx.Err()
	}
	_ = conn.SetDeadline(time.Time{})
	return conn, nil
}

// failed records that the proxy did not answer, for Check.
func (r *proxyRoute) failed(err error) {
	r.mu.Lock()
	r.checkedAt, r.checkErr = time.Now(), err
	r.mu.Unlock()
}

func (r *proxyRoute) ListenPacket(ctx context.Context) (PacketConn, error) {
	control, err := r.handshake(ctx)
	if err != nil {
		r.failed(err)
		return nil, err
	}
	_ = control.SetDeadline(time.Now().Add(proxyTimeout))
	bound, err := command(control, socksAssociate, "0.0.0.0:0")
	if err != nil {
		control.Close()
		return nil, err
	}
	_ = control.SetDeadline(time.Time{})
	relay, err := netip.ParseAddrPort(bound)
	if err != nil {
		control.Close()
		return nil, fmt.Errorf("the SOCKS5 proxy's UDP relay is not an address: %w", err)
	}
	if relay.Addr().IsUnspecified() {
		// The relay is on the proxy's own address.
		proxy, err := netip.ParseAddrPort(control.RemoteAddr().String())
		if err != nil {
			control.Close()
			return nil, err
		}
		relay = netip.AddrPortFrom(proxy.Addr(), relay.Port())
	}
	udp, err := net.ListenUDP("udp", nil)
	if err != nil {
		control.Close()
		return nil, err
	}
	_ = udp.SetReadBuffer(udpReadBuffer)
	c := &socksPacketConn{control: control, udp: udp, relay: netip.AddrPortFrom(relay.Addr().Unmap(), relay.Port()), in: make([]byte, 65536)}
	// The association lasts as long as its TCP connection.
	go func() {
		_, _ = io.Copy(io.Discard, control)
		c.Close()
	}()
	return c, nil
}

// socksPacketConn sends datagrams through a proxy's UDP relay. in is the
// buffer reads take a datagram in, out the one WriteToAddrPort builds one
// in, under outMu.
type socksPacketConn struct {
	control net.Conn
	udp     *net.UDPConn
	relay   netip.AddrPort
	once    sync.Once
	in      []byte
	outMu   sync.Mutex
	out     []byte
}

func (c *socksPacketConn) WriteTo(p []byte, address string) error {
	header, err := socksAddress(address)
	if err != nil {
		return err
	}
	datagram := append([]byte{0, 0, 0}, header...)
	datagram = append(datagram, p...)
	_, err = c.udp.WriteToUDPAddrPort(datagram, c.relay)
	return err
}

func (c *socksPacketConn) WriteToAddrPort(p []byte, to netip.AddrPort) error {
	c.outMu.Lock()
	defer c.outMu.Unlock()
	ip := to.Addr().Unmap()
	datagram := append(c.out[:0], 0, 0, 0)
	if ip.Is4() {
		raw := ip.As4()
		datagram = append(append(datagram, socksIPv4), raw[:]...)
	} else {
		raw := ip.As16()
		datagram = append(append(datagram, socksIPv6), raw[:]...)
	}
	datagram = binary.BigEndian.AppendUint16(datagram, to.Port())
	c.out = append(datagram, p...)
	_, err := c.udp.WriteToUDPAddrPort(c.out, c.relay)
	return err
}

func (c *socksPacketConn) ReadFrom(p []byte) (int, netip.AddrPort, error) {
	for {
		n, from, err := c.udp.ReadFromUDPAddrPort(c.in)
		if err != nil {
			return 0, netip.AddrPort{}, err
		}
		if netip.AddrPortFrom(from.Addr().Unmap(), from.Port()) != c.relay {
			continue
		}
		if n, source, ok := readSocksDatagram(c.in[:n], p); ok {
			return n, source, nil
		}
	}
}

// readSocksDatagram reads a datagram of the proxy's UDP relay: it copies
// its payload into p, and tells where it came from. A fragment, which
// Polyfin does not reassemble, or a malformed header is not read.
func readSocksDatagram(datagram, p []byte) (int, netip.AddrPort, bool) {
	if len(datagram) < 4 || datagram[2] != 0 {
		return 0, netip.AddrPort{}, false
	}
	reader := &sliceReader{data: datagram[3:]}
	source, err := readSocksAddress(reader)
	if err != nil {
		return 0, netip.AddrPort{}, false
	}
	ap, err := netip.ParseAddrPort(source)
	if err != nil {
		return 0, netip.AddrPort{}, false
	}
	return copy(p, reader.data), netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port()), true
}

func (c *socksPacketConn) SetReadDeadline(t time.Time) error { return c.udp.SetReadDeadline(t) }

func (c *socksPacketConn) Close() error {
	c.once.Do(func() {
		c.control.Close()
		c.udp.Close()
	})
	return nil
}

type sliceReader struct{ data []byte }

func (r *sliceReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}
