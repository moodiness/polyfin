package torrenttest

import (
	"net"
	"net/netip"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/torrent/utp"
)

// SeedUTP starts a seeder of t reachable over uTP alone, on a UDP port of
// host. Closing it is silent, as when a peer drops off the network: its
// socket closes first, and its connections end without a FIN.
func SeedUTP(tb testing.TB, t *Torrent, host string) *Peer {
	tb.Helper()
	conn, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.MustParseAddr(host), 0)))
	if err != nil {
		tb.Fatal(err)
	}
	return listen(tb, t, newUTPListener(conn), true)
}

// SeedBoth starts a seeder of t reachable over TCP and uTP on the same
// port of host, as most clients are. A port the system gives for one
// protocol may be taken, or reserved, for the other: Windows reserves
// ranges of ports for each protocol apart. So each try asks the system
// for a port of one protocol, then takes it over the other, TCP first
// and UDP first in turn.
func SeedBoth(tb testing.TB, t *Torrent, host string) *Peer {
	tb.Helper()
	var last error
	for try := range 200 {
		tcp, udp, err := samePort(host, try%2 == 0)
		if err != nil {
			last = err
			continue
		}
		both := &bothListener{tcp: tcp, utp: newUTPListener(udp), accepted: make(chan net.Conn), done: make(chan struct{})}
		go both.accept(both.tcp)
		go both.accept(both.utp)
		return listen(tb, t, both, true)
	}
	tb.Fatalf("no port free over both TCP and UDP: %v", last)
	return nil
}

// samePort listens on one port of host over TCP and UDP, the port given
// by the system for TCP when tcpFirst, for UDP otherwise.
func samePort(host string, tcpFirst bool) (net.Listener, *net.UDPConn, error) {
	if tcpFirst {
		tcp, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
		if err != nil {
			return nil, nil, err
		}
		udp, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(netip.MustParseAddrPort(tcp.Addr().String())))
		if err != nil {
			tcp.Close()
			return nil, nil, err
		}
		return tcp, udp, nil
	}
	udp, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.MustParseAddr(host), 0)))
	if err != nil {
		return nil, nil, err
	}
	tcp, err := net.Listen("tcp", udp.LocalAddr().String())
	if err != nil {
		udp.Close()
		return nil, nil, err
	}
	return tcp, udp, nil
}

// Handshakes counts the handshakes the peer received for its torrent.
func (p *Peer) Handshakes() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.handshakes
}

// udpPacketConn is a UDP socket of the host, for a uTP socket.
type udpPacketConn struct{ *net.UDPConn }

func (c udpPacketConn) WriteToAddrPort(p []byte, to netip.AddrPort) error {
	_, err := c.WriteToUDPAddrPort(p, to)
	return err
}

func (c udpPacketConn) ReadFrom(p []byte) (int, netip.AddrPort, error) {
	n, from, err := c.ReadFromUDPAddrPort(p)
	return n, netip.AddrPortFrom(from.Addr().Unmap(), from.Port()), err
}

// utpListener accepts the uTP connections of a socket, at its UDP address.
// It closes its UDP socket first: its connections end without a FIN.
type utpListener struct {
	socket *utp.Socket
	udp    *net.UDPConn
}

func newUTPListener(udp *net.UDPConn) *utpListener {
	return &utpListener{socket: utp.Listen(udpPacketConn{udp}), udp: udp}
}

func (l *utpListener) Accept() (net.Conn, error) {
	c, err := l.socket.Accept()
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (l *utpListener) Close() error {
	l.udp.Close()
	return l.socket.Close()
}

func (l *utpListener) Addr() net.Addr { return l.udp.LocalAddr() }

// bothListener accepts the connections of a TCP listener and of a uTP one
// on the same port.
type bothListener struct {
	tcp      net.Listener
	utp      *utpListener
	accepted chan net.Conn
	done     chan struct{}
	once     sync.Once
}

func (l *bothListener) accept(from net.Listener) {
	for {
		c, err := from.Accept()
		if err != nil {
			return
		}
		select {
		case l.accepted <- c:
		case <-l.done:
			c.Close()
			return
		}
	}
}

func (l *bothListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.accepted:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *bothListener) Close() error {
	l.once.Do(func() {
		close(l.done)
		l.tcp.Close()
		l.utp.Close()
	})
	return nil
}

func (l *bothListener) Addr() net.Addr { return l.tcp.Addr() }
