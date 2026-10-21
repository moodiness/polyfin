package utp

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	ipheader "gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
)

// pathConfig is a network path, the same each way: a bottleneck of rate
// bytes per second whose queue holds queue of packets at most, then a
// delay that varies by up to jitter either side, and a loss. The jitter
// keeps the packets in order, as a path's queues do.
type pathConfig struct {
	rate          int64
	queue         time.Duration
	delay, jitter time.Duration
	loss          float64
}

// direction carries the packets of one way of a path, in order.
type direction struct {
	config  pathConfig
	deliver func([]byte)

	mu        sync.Mutex
	random    *rand.Rand
	busyUntil time.Time
	lastAt    time.Time
	packets   chan timed
}

type timed struct {
	at     time.Time
	packet []byte
}

func newDirection(tb testing.TB, config pathConfig, seed uint64, deliver func([]byte)) *direction {
	d := &direction{config: config, deliver: deliver, random: rand.New(rand.NewPCG(seed, seed+1)), packets: make(chan timed, 1<<16)}
	done := make(chan struct{})
	tb.Cleanup(func() { close(done) })
	go func() {
		for {
			select {
			case p := <-d.packets:
				if wait := time.Until(p.at); wait > 0 {
					time.Sleep(wait)
				}
				d.deliver(p.packet)
			case <-done:
				return
			}
		}
	}()
	return d
}

// send takes a packet into the path, whose bytes it keeps.
func (d *direction) send(p []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.random.Float64() < d.config.loss {
		return
	}
	now := time.Now()
	at := now
	if d.config.rate > 0 {
		start := now
		if d.busyUntil.After(now) {
			start = d.busyUntil
		}
		if start.Sub(now) > d.config.queue {
			return
		}
		d.busyUntil = start.Add(time.Duration(int64(len(p)) * int64(time.Second) / d.config.rate))
		at = d.busyUntil
	}
	at = at.Add(d.config.delay + time.Duration((2*d.random.Float64()-1)*float64(d.config.jitter)))
	if at.Before(d.lastAt) {
		at = d.lastAt
	}
	d.lastAt = at
	select {
	case d.packets <- timed{at, append([]byte(nil), p...)}:
	default:
	}
}

// pathConn is one end of a path as a PacketConn.
type pathConn struct {
	self, other netip.AddrPort
	out         *direction
	in          chan []byte
	closed      chan struct{}
	once        sync.Once
}

// newPathConns returns the two ends of a path.
func newPathConns(tb testing.TB, config pathConfig) (*pathConn, *pathConn) {
	a := &pathConn{self: netip.MustParseAddrPort("192.0.2.1:1000"), in: make(chan []byte, 1<<16), closed: make(chan struct{})}
	b := &pathConn{self: netip.MustParseAddrPort("192.0.2.2:2000"), in: make(chan []byte, 1<<16), closed: make(chan struct{})}
	a.other, b.other = b.self, a.self
	a.out = newDirection(tb, config, 1, func(p []byte) { b.in <- p })
	b.out = newDirection(tb, config, 2, func(p []byte) { a.in <- p })
	return a, b
}

func (c *pathConn) WriteToAddrPort(p []byte, to netip.AddrPort) error {
	if to != c.other {
		return errors.New("no route")
	}
	c.out.send(p)
	return nil
}

func (c *pathConn) ReadFrom(p []byte) (int, netip.AddrPort, error) {
	select {
	case d := <-c.in:
		return copy(p, d), c.other, nil
	case <-c.closed:
		return 0, netip.AddrPort{}, net.ErrClosed
	}
}

func (c *pathConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

// stacks are two network stacks joined by a path, at 10.0.0.1 and
// 10.0.0.2, whose TCP is set as the WireGuard route's (SACK on).
func stacks(tb testing.TB, config pathConfig) (*stack.Stack, *stack.Stack) {
	ctx, cancel := context.WithCancel(context.Background())
	tb.Cleanup(cancel)
	var nets [2]*stack.Stack
	var links [2]*channel.Endpoint
	for i := range nets {
		s := stack.New(stack.Options{NetworkProtocols: []stack.NetworkProtocolFactory{ipv4.NewProtocol},
			TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol}})
		sack := tcpip.TCPSACKEnabled(true)
		s.SetTransportProtocolOption(tcp.ProtocolNumber, &sack)
		links[i] = channel.New(4096, 1500, "")
		if err := s.CreateNIC(1, links[i]); err != nil {
			tb.Fatal(err)
		}
		address := tcpip.AddrFrom4([4]byte{10, 0, 0, byte(i + 1)})
		if err := s.AddProtocolAddress(1, tcpip.ProtocolAddress{Protocol: ipv4.ProtocolNumber, AddressWithPrefix: address.WithPrefix()},
			stack.AddressProperties{}); err != nil {
			tb.Fatal(err)
		}
		s.AddRoute(tcpip.Route{Destination: ipheader.IPv4EmptySubnet, NIC: 1})
		nets[i] = s
		tb.Cleanup(s.Close)
	}
	for i := range links {
		to := links[1-i]
		d := newDirection(tb, config, uint64(i+1), func(p []byte) {
			pkb := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(p)})
			to.InjectInbound(ipheader.IPv4ProtocolNumber, pkb)
			pkb.DecRef()
		})
		from := links[i]
		go func() {
			for {
				pkt := from.ReadContext(ctx)
				if pkt == nil {
					return
				}
				view := pkt.ToView()
				d.send(view.AsSlice())
				view.Release()
				pkt.DecRef()
			}
		}()
	}
	return nets[0], nets[1]
}

func stackAddress(i byte, port uint16) tcpip.FullAddress {
	return tcpip.FullAddress{NIC: 1, Addr: tcpip.AddrFrom4([4]byte{10, 0, 0, i}), Port: port}
}

// stackUDP is a UDP socket of a stack as a PacketConn.
type stackUDP struct{ *gonet.UDPConn }

func (c stackUDP) WriteToAddrPort(p []byte, to netip.AddrPort) error {
	_, err := c.UDPConn.WriteTo(p, net.UDPAddrFromAddrPort(to))
	return err
}

func (c stackUDP) ReadFrom(p []byte) (int, netip.AddrPort, error) {
	n, from, err := c.UDPConn.ReadFrom(p)
	if err != nil {
		return 0, netip.AddrPort{}, err
	}
	return n, from.(*net.UDPAddr).AddrPort(), nil
}

// transfer sends size bytes from w to r, and returns how long they took.
func transfer(tb testing.TB, w io.Writer, r io.Reader, size int) time.Duration {
	tb.Helper()
	started := time.Now()
	done := make(chan error, 1)
	go func() {
		_, err := w.Write(make([]byte, size))
		done <- err
	}()
	if _, err := io.ReadFull(r, make([]byte, size)); err != nil {
		tb.Fatal(err)
	}
	if err := <-done; err != nil {
		tb.Fatal(err)
	}
	return time.Since(started)
}

// overUTP times size bytes over uTP on the stacks' path.
func overUTP(tb testing.TB, a, b *stack.Stack, size int) time.Duration {
	tb.Helper()
	ua, err := gonet.DialUDP(a, &tcpip.FullAddress{NIC: 1, Addr: stackAddress(1, 0).Addr, Port: 6881}, nil, ipv4.ProtocolNumber)
	if err != nil {
		tb.Fatal(err)
	}
	ub, err := gonet.DialUDP(b, &tcpip.FullAddress{NIC: 1, Addr: stackAddress(2, 0).Addr, Port: 6881}, nil, ipv4.ProtocolNumber)
	if err != nil {
		tb.Fatal(err)
	}
	client, server := pairOn(tb, stackUDP{ua}, stackUDP{ub}, netip.MustParseAddrPort("10.0.0.2:6881"))
	return transfer(tb, server, client, size)
}

// TestJitterDoesNotStallUTP sends 2 MiB over a path of 80 ms ± 40 ms each
// way, as to a far WireGuard server. A delay estimate that took the jitter
// for a queue would hold uTP near a packet per round trip, under 10 KB/s;
// it runs at about 1.5 MB/s, and the floor leaves room for a loaded
// machine under the race detector.
func TestJitterDoesNotStallUTP(t *testing.T) {
	t.Parallel()
	const size, floor = 2 << 20, 150 << 10
	a, b := newPathConns(t, pathConfig{rate: 50e6 / 8, queue: 250 * time.Millisecond, delay: 80 * time.Millisecond, jitter: 40 * time.Millisecond})
	client, server := pairOn(t, a, b, b.self)
	// Slower than the floor, the read ends at its deadline: "i/o timeout".
	_ = client.SetReadDeadline(time.Now().Add(size / floor * time.Second))
	took := transfer(t, server, client, size)
	t.Logf("2 MiB in %v: %.0f KB/s", took, float64(size)/took.Seconds()/1024)
}

// overTCP times size bytes over TCP on the stacks' path.
func overTCP(tb testing.TB, a, b *stack.Stack, size int) time.Duration {
	tb.Helper()
	listener, err := gonet.ListenTCP(b, stackAddress(2, 6881), ipv4.ProtocolNumber)
	if err != nil {
		tb.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := listener.Accept()
		if err == nil {
			accepted <- c
		}
	}()
	client, err := gonet.DialTCP(a, stackAddress(2, 6881), ipv4.ProtocolNumber)
	if err != nil {
		tb.Fatal(err)
	}
	defer client.Close()
	server := <-accepted
	defer server.Close()
	return transfer(tb, server, client, size)
}

// pairOn opens a uTP connection between two PacketConns.
func pairOn(tb testing.TB, a, b PacketConn, remote netip.AddrPort) (*Conn, *Conn) {
	tb.Helper()
	dialer, listener := NewSocket(a), Listen(b)
	tb.Cleanup(func() {
		dialer.Close()
		listener.Close()
	})
	accepted := make(chan *Conn, 1)
	go func() {
		c, err := listener.Accept()
		if err == nil {
			accepted <- c
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := dialer.Dial(ctx, remote)
	if err != nil {
		tb.Fatal(err)
	}
	select {
	case server := <-accepted:
		return client, server
	case <-ctx.Done():
		tb.Fatal("no connection accepted")
	}
	return nil, nil
}

// paths are those BenchmarkPath measures: a WireGuard tunnel's path to
// peers, near and far, with and without loss, behind a bottleneck of
// 50 Mbit/s with 250 ms of queue.
var paths = []struct {
	name   string
	config pathConfig
}{
	{"40ms±15ms", pathConfig{rate: 50e6 / 8, queue: 250 * time.Millisecond, delay: 40 * time.Millisecond, jitter: 15 * time.Millisecond}},
	{"40ms±15ms/loss1%", pathConfig{rate: 50e6 / 8, queue: 250 * time.Millisecond, delay: 40 * time.Millisecond, jitter: 15 * time.Millisecond, loss: 0.01}},
	{"80ms±40ms", pathConfig{rate: 50e6 / 8, queue: 250 * time.Millisecond, delay: 80 * time.Millisecond, jitter: 40 * time.Millisecond}},
	{"80ms±40ms/loss1%", pathConfig{rate: 50e6 / 8, queue: 250 * time.Millisecond, delay: 80 * time.Millisecond, jitter: 40 * time.Millisecond, loss: 0.01}},
}

// BenchmarkPath measures 4 MiB over uTP and over TCP on the same paths, two
// network stacks joined by a path of a delay, a jitter and a loss. The
// numbers of docs/torrents.md come from
// go test -run '^$' -bench Path -benchtime 3x ./internal/torrent/utp.
func BenchmarkPath(b *testing.B) {
	const size = 4 << 20
	for _, path := range paths {
		for _, transport := range []string{"uTP", "TCP"} {
			b.Run(path.name+"/"+transport, func(b *testing.B) {
				var took time.Duration
				for b.Loop() {
					sa, sb := stacks(b, path.config)
					if transport == "uTP" {
						took += overUTP(b, sa, sb, size)
					} else {
						took += overTCP(b, sa, sb, size)
					}
				}
				b.ReportMetric(float64(size)*float64(b.N)/took.Seconds()/1e6, "MB/s")
			})
		}
	}
}
