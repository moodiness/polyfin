package torrent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

// ErrNoDNS reports a host name to look up over a WireGuard tunnel whose
// configuration names no DNS server: it is not looked up another way.
var ErrNoDNS = errors.New("the WireGuard configuration names no DNS server to look tracker names up")

// The times of a WireGuard tunnel: a handshake within tunnelFresh tells it
// is up; past that, a check makes a new one and waits tunnelWait for it.
// tunnelFresh is above wireguard-go's 5 seconds between two handshakes
// (RekeyTimeout), so that a check never waits for one it was not given.
const (
	tunnelFresh = 30 * time.Second
	tunnelWait  = 8 * time.Second
)

// WireGuardRoute runs a WireGuard tunnel inside Polyfin, from config: the
// userspace WireGuard of wireguard-go, with gVisor's network stack as its
// interface. Every socket of the route is a socket of that stack, whose
// packets go to the configuration's server alone, encrypted, over one UDP
// socket of the host: nothing is set up on the host, no capability and no
// TUN device are needed, and nothing reaches the network another way.
// Host names are looked up over the tunnel by the configuration's DNS
// servers, ErrNoDNS without one; the server's own name, when Endpoint is
// one, is looked up the host's usual way, as any WireGuard client does.
// Nothing listens: no peer can connect in. The tunnel starts at the first
// check; logger takes wireguard-go's errors, at debug level.
func WireGuardRoute(config *WireGuardConfig, logger *slog.Logger) Route {
	return &wireGuardRoute{config: config, logger: logger, fresh: tunnelFresh, wait: tunnelWait}
}

type wireGuardRoute struct {
	config      *WireGuardConfig
	logger      *slog.Logger
	fresh, wait time.Duration

	// checking makes one check at a time, and starts the tunnel once.
	checking sync.Mutex
	// deviceErr is wireguard-go's last error, told with a failed
	// handshake.
	deviceErr atomic.Pointer[string]

	mu        sync.Mutex
	closed    bool
	dev       *device.Device
	run       *tunnelRun
	peer      *device.Peer
	checkedAt time.Time
	checkErr  error
}

// errRouteClosed reports a socket asked of a route that was replaced.
var errRouteClosed = errors.New("the route for torrents changed")

func (r *wireGuardRoute) String() string { return "WireGuard tunnel to " + r.config.Endpoint }

func (r *wireGuardRoute) Close() error {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	r.stop()
	return nil
}

// stop stops the tunnel, its sockets first. wireguard-go may log as its
// device closes: r.mu is not held.
func (r *wireGuardRoute) stop() {
	r.mu.Lock()
	dev, run := r.dev, r.run
	r.dev, r.run, r.peer = nil, nil, nil
	r.mu.Unlock()
	if run != nil {
		run.stop()
	}
	if dev != nil {
		dev.Close()
	}
}

// tunnelRun is a run of the tunnel's network stack, and what Polyfin sends
// through it. wireguard-go's stack panics on a packet sent while its
// device closes (a send on a closed channel), so each of Polyfin's sends
// (a datagram, a TCP write or dial, a name looked up) holds sends for
// reading, and stop takes it for writing before the device closes: it
// waits for the sends in flight, which closing the sockets and ending ctx
// cut short, and refuses those that come next.
type tunnelRun struct {
	tnet   *netstack.Net
	ctx    context.Context
	cancel context.CancelFunc

	sends   sync.RWMutex
	stopped bool

	mu sync.Mutex
	// sockets are the sockets open on the run, by what Polyfin was given,
	// nil once the run stops.
	sockets map[io.Closer]io.Closer
}

func newTunnelRun(tnet *netstack.Net) *tunnelRun {
	ctx, cancel := context.WithCancel(context.Background())
	return &tunnelRun{tnet: tnet, ctx: ctx, cancel: cancel, sockets: map[io.Closer]io.Closer{}}
}

// send sends through the tunnel by f, unless the run stopped.
func (run *tunnelRun) send(f func() error) error {
	run.sends.RLock()
	defer run.sends.RUnlock()
	if run.stopped {
		return errRouteClosed
	}
	return f()
}

// bound is ctx, ended too when the run stops.
func (run *tunnelRun) bound(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(run.ctx, cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}

// opened records a socket given as given, closed when the run stops;
// false when it stopped, socket being then closed.
func (run *tunnelRun) opened(given, socket io.Closer) bool {
	run.mu.Lock()
	defer run.mu.Unlock()
	if run.sockets == nil {
		socket.Close()
		return false
	}
	run.sockets[given] = socket
	return true
}

// closed forgets a socket closed.
func (run *tunnelRun) closed(given io.Closer) {
	run.mu.Lock()
	defer run.mu.Unlock()
	delete(run.sockets, given)
}

// stop ends the run's dials and lookups, closes its sockets, which ends
// their writes, and waits for the sends in flight.
func (run *tunnelRun) stop() {
	run.cancel()
	run.mu.Lock()
	sockets := run.sockets
	run.sockets = nil
	run.mu.Unlock()
	for _, socket := range sockets {
		socket.Close()
	}
	run.sends.Lock()
	run.stopped = true
	run.sends.Unlock()
}

// TunnelStatus is what Health tells of a WireGuard tunnel: its server,
// its addresses in Polyfin, and when its last handshake was, zero for
// none yet.
type TunnelStatus struct {
	Endpoint      string
	Addresses     []netip.Prefix
	LastHandshake time.Time
}

func (r *wireGuardRoute) tunnel() TunnelStatus {
	return TunnelStatus{Endpoint: r.config.Endpoint, Addresses: r.config.Addresses, LastHandshake: r.lastHandshake()}
}

// Check reports the tunnel down when its server cannot be looked up or
// reached, or does not answer a handshake, as with a wrong key: a
// handshake within r.fresh tells it is up, else a new one is made. A
// failure is kept for checkedFor, as a proxy's, unless ctx ended it
// before r.wait.
func (r *wireGuardRoute) Check(ctx context.Context) error {
	r.checking.Lock()
	defer r.checking.Unlock()
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return errRouteClosed
	}
	if r.checkErr != nil && time.Since(r.checkedAt) < checkedFor {
		err := r.checkErr
		r.mu.Unlock()
		return err
	}
	r.mu.Unlock()
	err := r.check(ctx)
	if err != nil && ctx.Err() != nil {
		return err
	}
	r.mu.Lock()
	r.checkedAt, r.checkErr = time.Now(), err
	r.mu.Unlock()
	return err
}

func (r *wireGuardRoute) check(ctx context.Context) error {
	peer, err := r.start(ctx)
	if err != nil {
		return err
	}
	last := r.lastHandshake()
	if !last.IsZero() && time.Since(last) < r.fresh {
		return nil
	}
	r.deviceErr.Store(nil)
	if err := peer.SendHandshakeInitiation(false); err != nil {
		r.deviceError("%v", err)
	}
	if r.awaitHandshake(ctx, last) {
		return nil
	}
	failure := &RouteError{Kind: WireGuardHandshake, Route: r.config.Endpoint}
	if message := r.deviceErr.Load(); message != nil {
		failure.Err = errors.New(*message)
	}
	// A server named by its host name may have moved: the next check
	// looks it up again.
	if _, err := netip.ParseAddrPort(r.config.Endpoint); err != nil && ctx.Err() == nil {
		r.stop()
	}
	return failure
}

// awaitHandshake reports whether a handshake completes after last, within
// r.wait and before ctx is done.
func (r *wireGuardRoute) awaitHandshake(ctx context.Context, last time.Time) bool {
	wait := time.NewTimer(r.wait)
	defer wait.Stop()
	poll := time.NewTicker(50 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-poll.C:
			if r.lastHandshake().After(last) {
				return true
			}
		case <-wait.C:
			return false
		case <-ctx.Done():
			return false
		}
	}
}

// start starts the tunnel unless it runs, and returns its peer, the
// server. It is called with r.checking held.
func (r *wireGuardRoute) start(ctx context.Context) (*device.Peer, error) {
	r.mu.Lock()
	peer := r.peer
	r.mu.Unlock()
	if peer != nil {
		return peer, nil
	}
	server, err := r.lookUpServer(ctx)
	if err != nil {
		return nil, err
	}
	addresses := make([]netip.Addr, len(r.config.Addresses))
	for i, prefix := range r.config.Addresses {
		addresses[i] = prefix.Addr()
	}
	tun, tnet, err := netstack.CreateNetTUN(addresses, r.config.DNS, r.config.mtu())
	if err != nil {
		return nil, &RouteError{Kind: WireGuardDown, Route: r.config.Endpoint, Err: err}
	}
	dev := device.NewDevice(tun, conn.NewDefaultBind(), &device.Logger{Verbosef: device.DiscardLogf, Errorf: r.deviceError})
	if err := dev.IpcSet(r.config.uapi(server)); err != nil {
		dev.Close()
		return nil, &RouteError{Kind: WireGuardDown, Route: r.config.Endpoint, Err: err}
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, &RouteError{Kind: WireGuardDown, Route: r.config.Endpoint, Err: err}
	}
	peer = dev.LookupPeer(device.NoisePublicKey(r.config.publicKey))
	r.mu.Lock()
	if r.closed || peer == nil {
		r.mu.Unlock()
		dev.Close()
		return nil, errRouteClosed
	}
	r.dev, r.run, r.peer = dev, newTunnelRun(tnet), peer
	r.mu.Unlock()
	return peer, nil
}

// lookUpServer returns the server's address, looking its name up the
// host's usual way when Endpoint names it: the tunnel is not up yet. An
// IPv4 address is preferred.
func (r *wireGuardRoute) lookUpServer(ctx context.Context) (netip.AddrPort, error) {
	if server, err := netip.ParseAddrPort(r.config.Endpoint); err == nil {
		return server, nil
	}
	host, port, _ := net.SplitHostPort(r.config.Endpoint)
	n, _ := strconv.ParseUint(port, 10, 16)
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		if err == nil {
			err = errors.New("no address")
		}
		return netip.AddrPort{}, &RouteError{Kind: WireGuardEndpoint, Route: r.config.Endpoint, Err: err}
	}
	best := ips[0].Unmap()
	for _, ip := range ips {
		if ip.Unmap().Is4() {
			best = ip.Unmap()
			break
		}
	}
	return netip.AddrPortFrom(best, uint16(n)), nil
}

// deviceError keeps wireguard-go's last error for Health, and logs it at
// debug level: while the server is down it says the same every few
// seconds. Its messages name peers by an abbreviated public key, never a
// private one.
func (r *wireGuardRoute) deviceError(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	r.deviceErr.Store(&message)
	r.logger.Debug("WireGuard: "+message, "endpoint", r.config.Endpoint)
}

// lastHandshake is when the tunnel's last handshake was, zero for none.
func (r *wireGuardRoute) lastHandshake() time.Time {
	r.mu.Lock()
	dev := r.dev
	r.mu.Unlock()
	if dev == nil {
		return time.Time{}
	}
	state, err := dev.IpcGet()
	if err != nil {
		return time.Time{}
	}
	var sec, nsec int64
	scanner := bufio.NewScanner(strings.NewReader(state))
	for scanner.Scan() {
		key, value, _ := strings.Cut(scanner.Text(), "=")
		switch key {
		case "last_handshake_time_sec":
			sec, _ = strconv.ParseInt(value, 10, 64)
		case "last_handshake_time_nsec":
			nsec, _ = strconv.ParseInt(value, 10, 64)
		}
	}
	if sec == 0 && nsec == 0 {
		return time.Time{}
	}
	return time.Unix(sec, nsec)
}

// stack returns the tunnel's network stack, once a check found the tunnel
// up: while it is down, no socket opens.
func (r *wireGuardRoute) stack() (*tunnelRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case r.closed:
		return nil, errRouteClosed
	case r.checkErr != nil:
		return nil, r.checkErr
	case r.run == nil:
		return nil, &RouteError{Kind: WireGuardHandshake, Route: r.config.Endpoint}
	}
	return r.run, nil
}

func (r *wireGuardRoute) Dial(ctx context.Context, address string) (net.Conn, error) {
	if err := r.Check(ctx); err != nil {
		return nil, err
	}
	run, err := r.stack()
	if err != nil {
		return nil, err
	}
	ctx, cancel := run.bound(ctx)
	defer cancel()
	target, err := r.resolve(ctx, run, address, true)
	if err != nil {
		return nil, err
	}
	var conn net.Conn
	if err := run.send(func() error {
		tcp, err := run.tnet.DialContextTCPAddrPort(ctx, target)
		if err != nil {
			// A nil *gonet.TCPConn would be a net.Conn that is not nil.
			return err
		}
		conn = tcp
		return nil
	}); err != nil {
		return nil, err
	}
	tunneled := &tunnelConn{Conn: conn, run: run}
	if !run.opened(tunneled, conn) {
		return nil, errRouteClosed
	}
	return tunneled, nil
}

// tunnelConn is a TCP connection of the tunnel's network stack.
type tunnelConn struct {
	net.Conn
	run *tunnelRun
}

func (c *tunnelConn) Write(p []byte) (n int, err error) {
	err = c.run.send(func() error {
		n, err = c.Conn.Write(p)
		return err
	})
	return n, err
}

func (c *tunnelConn) Close() error {
	c.run.closed(c)
	return c.Conn.Close()
}

// resolve looks a host up over the tunnel, by the configuration's DNS
// servers, IP addresses as they are. IPv6 is taken when tcp and the tunnel
// has an IPv6 address; UDP sockets are IPv4.
func (r *wireGuardRoute) resolve(ctx context.Context, run *tunnelRun, address string, tcp bool) (netip.AddrPort, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return netip.AddrPort{}, err
	}
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("invalid port in %q", address)
	}
	ipv6 := tcp && r.config.hasIPv6()
	if ip, err := netip.ParseAddr(host); err == nil {
		ip = ip.Unmap()
		if ip.Is6() && !ipv6 {
			return netip.AddrPort{}, fmt.Errorf("%s is an IPv6 address, which the tunnel does not carry here", ip)
		}
		return netip.AddrPortFrom(ip, uint16(n)), nil
	}
	if len(r.config.DNS) == 0 {
		return netip.AddrPort{}, ErrNoDNS
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var found []string
	if err := run.send(func() (err error) {
		found, err = run.tnet.LookupContextHost(ctx, host)
		return err
	}); err != nil {
		return netip.AddrPort{}, fmt.Errorf("look %s up over the WireGuard tunnel: %w", host, err)
	}
	var fallback netip.Addr
	for _, item := range found {
		ip, err := netip.ParseAddr(item)
		if err != nil {
			continue
		}
		if ip = ip.Unmap(); ip.Is4() {
			return netip.AddrPortFrom(ip, uint16(n)), nil
		}
		if ipv6 && !fallback.IsValid() {
			fallback = ip
		}
	}
	if fallback.IsValid() {
		return netip.AddrPortFrom(fallback, uint16(n)), nil
	}
	return netip.AddrPort{}, fmt.Errorf("look %s up over the WireGuard tunnel: no address", host)
}

func (r *wireGuardRoute) ListenPacket(ctx context.Context) (PacketConn, error) {
	if err := r.Check(ctx); err != nil {
		return nil, err
	}
	run, err := r.stack()
	if err != nil {
		return nil, err
	}
	udp, err := run.tnet.ListenUDPAddrPort(netip.AddrPortFrom(r.config.ipv4(), 0))
	if err != nil {
		return nil, err
	}
	c := &tunnelPacketConn{route: r, run: run, conn: udp}
	if !run.opened(c, udp) {
		return nil, errRouteClosed
	}
	return c, nil
}

// tunnelPacketConn is a UDP socket of the tunnel's network stack.
type tunnelPacketConn struct {
	route *wireGuardRoute
	run   *tunnelRun
	conn  net.PacketConn
}

func (c *tunnelPacketConn) WriteTo(p []byte, address string) error {
	ctx, cancel := c.run.bound(context.Background())
	defer cancel()
	target, err := c.route.resolve(ctx, c.run, address, false)
	if err != nil {
		return err
	}
	return c.send(p, target)
}

func (c *tunnelPacketConn) WriteToAddrPort(p []byte, to netip.AddrPort) error {
	ip := to.Addr().Unmap()
	if !ip.Is4() {
		return fmt.Errorf("%s is an IPv6 address, which the tunnel does not carry here", ip)
	}
	return c.send(p, netip.AddrPortFrom(ip, to.Port()))
}

func (c *tunnelPacketConn) send(p []byte, to netip.AddrPort) error {
	return c.run.send(func() error {
		_, err := c.conn.WriteTo(p, net.UDPAddrFromAddrPort(to))
		return err
	})
}

func (c *tunnelPacketConn) ReadFrom(p []byte) (int, netip.AddrPort, error) {
	n, from, err := c.conn.ReadFrom(p)
	if err != nil {
		return n, netip.AddrPort{}, err
	}
	udp, ok := from.(*net.UDPAddr)
	if !ok {
		return 0, netip.AddrPort{}, fmt.Errorf("a datagram from %v", from)
	}
	ap := udp.AddrPort()
	return n, netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port()), nil
}

func (c *tunnelPacketConn) SetReadDeadline(t time.Time) error { return c.conn.SetReadDeadline(t) }

func (c *tunnelPacketConn) Close() error {
	c.run.closed(c)
	return c.conn.Close()
}
