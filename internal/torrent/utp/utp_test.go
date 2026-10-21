package utp

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/netip"
	"os"
	"sync"
	"testing"
	"time"
)

// udpConn is a UDP socket of the host as a PacketConn.
type udpConn struct{ *net.UDPConn }

func (c udpConn) WriteToAddrPort(p []byte, to netip.AddrPort) error {
	_, err := c.UDPConn.WriteToUDPAddrPort(p, to)
	return err
}

func (c udpConn) ReadFrom(p []byte) (int, netip.AddrPort, error) {
	return c.UDPConn.ReadFromUDPAddrPort(p)
}

func listenUDP(t *testing.T) udpConn {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	return udpConn{conn}
}

// link is one end of an in-memory network between two PacketConns, which
// drops and reorders datagrams as drop tells.
type link struct {
	self, other netip.AddrPort
	in          chan []byte
	peer        *link
	// drop tells whether a datagram is lost; hold, that it waits for the
	// next one, which then goes first.
	mu         sync.Mutex
	drop, hold func() bool
	held       []byte
	closed     chan struct{}
	once       sync.Once
}

func newLinks(drop, hold func() bool) (*link, *link) {
	a := &link{self: netip.MustParseAddrPort("192.0.2.1:1000"), in: make(chan []byte, 4096), drop: drop, hold: hold, closed: make(chan struct{})}
	b := &link{self: netip.MustParseAddrPort("192.0.2.2:2000"), in: make(chan []byte, 4096), drop: drop, hold: hold, closed: make(chan struct{})}
	a.peer, b.peer, a.other, b.other = b, a, b.self, a.self
	return a, b
}

func (l *link) WriteToAddrPort(p []byte, to netip.AddrPort) error {
	if to != l.other {
		return errors.New("no route")
	}
	datagram := bytes.Clone(p)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.drop != nil && l.drop() {
		return nil
	}
	if l.hold != nil && l.held == nil && l.hold() {
		l.held = datagram
		return nil
	}
	for _, d := range [][]byte{datagram, l.held} {
		if d == nil {
			continue
		}
		select {
		case l.peer.in <- d:
		default:
		}
	}
	l.held = nil
	return nil
}

func (l *link) ReadFrom(p []byte) (int, netip.AddrPort, error) {
	select {
	case d := <-l.in:
		return copy(p, d), l.other, nil
	case <-l.closed:
		return 0, netip.AddrPort{}, net.ErrClosed
	}
}

func (l *link) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

// pair opens a connection between two sockets over a and b.
func pair(t *testing.T, a, b PacketConn, remote netip.AddrPort) (*Conn, *Conn) {
	t.Helper()
	dialer, listener := NewSocket(a), Listen(b)
	t.Cleanup(func() {
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
		t.Fatal(err)
	}
	select {
	case server := <-accepted:
		return client, server
	case <-ctx.Done():
		t.Fatal("no connection accepted")
	}
	return nil, nil
}

// exchange sends size random bytes each way at once, and checks both
// arrive whole.
func exchange(t *testing.T, a, b *Conn, size int) {
	t.Helper()
	send := func(c *Conn) ([]byte, chan error) {
		data := make([]byte, size)
		_, _ = crand.Read(data)
		done := make(chan error, 1)
		go func() {
			_, err := c.Write(data)
			done <- err
		}()
		return data, done
	}
	fromA, writeA := send(a)
	fromB, writeB := send(b)
	for _, side := range []struct {
		c    *Conn
		want []byte
	}{{b, fromA}, {a, fromB}} {
		_ = side.c.SetReadDeadline(time.Now().Add(time.Minute))
		got := make([]byte, size)
		if _, err := io.ReadFull(side.c, got); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, side.want) {
			t.Fatal("the bytes received are not those sent")
		}
	}
	for _, done := range []chan error{writeA, writeB} {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

func TestTransfersOverUDP(t *testing.T) {
	t.Parallel()
	a, b := listenUDP(t), listenUDP(t)
	client, server := pair(t, a, b, b.LocalAddr().(*net.UDPAddr).AddrPort())
	exchange(t, client, server, 4<<20)
	if got := server.RemoteAddr().String(); got != a.LocalAddr().String() {
		t.Fatalf("the server sees the client at %s", got)
	}
}

func TestTransfersDespiteLossAndReordering(t *testing.T) {
	t.Parallel()
	random := rand.New(rand.NewPCG(1, 2))
	var mu sync.Mutex
	chance := func(p float64) func() bool {
		return func() bool {
			mu.Lock()
			defer mu.Unlock()
			return random.Float64() < p
		}
	}
	a, b := newLinks(chance(0.03), chance(0.05))
	client, server := pair(t, a, b, b.self)
	exchange(t, client, server, 1<<20)
	// The packets lost were sent again, some found by selective
	// acknowledgments.
}

func TestCloseEndsTheStream(t *testing.T) {
	t.Parallel()
	a, b := newLinks(nil, nil)
	client, server := pair(t, a, b, b.self)
	if _, err := client.Write([]byte("last words")); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 10)
	if _, err := io.ReadFull(server, got); err != nil || string(got) != "last words" {
		t.Fatalf("read %q: %v", got, err)
	}
	client.Close()
	_ = server.SetReadDeadline(time.Now().Add(30 * time.Second))
	if n, err := server.Read(got); n != 0 || err != io.EOF {
		t.Fatalf("a read past the peer's FIN gave %d bytes and %v", n, err)
	}
	if _, err := client.Write([]byte("more")); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("a write after Close gave %v", err)
	}
}

func TestReadDeadline(t *testing.T) {
	t.Parallel()
	a, b := newLinks(nil, nil)
	client, _ := pair(t, a, b, b.self)
	_ = client.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	if _, err := client.Read(make([]byte, 1)); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("a read past its deadline gave %v", err)
	}
}

func TestDialWithoutAnswerEnds(t *testing.T) {
	t.Parallel()
	// Nothing answers: the SYNs are dropped.
	a, _ := newLinks(func() bool { return true }, nil)
	socket := NewSocket(a)
	defer socket.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := socket.Dial(ctx, a.other); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a dial nothing answers gave %v", err)
	}
	if socket.Conns() != 0 {
		t.Fatal("the connection that was not answered is kept")
	}
	// Without a deadline, the third SYN unanswered ends it.
	started := time.Now()
	if _, err := socket.Dial(context.Background(), a.other); !errors.Is(err, ErrTimeout) {
		t.Fatalf("a dial nothing answers gave %v", err)
	}
	if took := time.Since(started); took < 6*time.Second {
		t.Fatalf("the dial gave up after %v, before its third SYN timed out", took)
	}
}

func TestPeerGoneEndsTheConnection(t *testing.T) {
	t.Parallel()
	var gone sync.Mutex
	lost := false
	a, b := newLinks(func() bool {
		gone.Lock()
		defer gone.Unlock()
		return lost
	}, nil)
	client, _ := pair(t, a, b, b.self)
	gone.Lock()
	lost = true
	gone.Unlock()
	if _, err := client.Write(make([]byte, 64<<10)); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Minute))
	if _, err := client.Read(make([]byte, 1)); !errors.Is(err, ErrTimeout) {
		t.Fatalf("a read from a peer gone gave %v", err)
	}
}

func TestResetEndsTheConnection(t *testing.T) {
	t.Parallel()
	a, b := newLinks(nil, nil)
	client, server := pair(t, a, b, b.self)
	server.mu.Lock()
	server.transmit(stReset, server.seq, nil)
	server.mu.Unlock()
	_ = client.SetReadDeadline(time.Now().Add(30 * time.Second))
	if _, err := client.Read(make([]byte, 1)); !errors.Is(err, ErrReset) {
		t.Fatalf("a read after a RESET gave %v", err)
	}
}

// A socket that closes ends its connections, and tells their peers with a
// FIN: a peer keeps a connection it was not told of.
func TestSocketClosedEndsItsConnections(t *testing.T) {
	t.Parallel()
	a, b := newLinks(nil, nil)
	client, server := pair(t, a, b, b.self)
	client.s.Close()
	if _, err := client.Read(make([]byte, 1)); !errors.Is(err, ErrClosed) {
		t.Fatalf("a read after the socket closed gave %v", err)
	}
	if _, err := client.s.Dial(context.Background(), b.self); !errors.Is(err, ErrClosed) {
		t.Fatalf("a dial on a closed socket gave %v", err)
	}
	_ = server.SetReadDeadline(time.Now().Add(30 * time.Second))
	if _, err := server.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("the peer of a socket closed read %v, not its FIN", err)
	}
}
