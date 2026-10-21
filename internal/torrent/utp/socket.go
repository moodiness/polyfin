package utp

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"sync"
)

// PacketConn is the UDP socket a Socket runs over.
type PacketConn interface {
	// WriteToAddrPort sends p to an IP address and port.
	WriteToAddrPort(p []byte, to netip.AddrPort) error
	// ReadFrom reads a datagram and the address it came from.
	ReadFrom(p []byte) (int, netip.AddrPort, error)
	Close() error
}

// maxDatagram is the largest datagram read.
const maxDatagram = 64 << 10

// acceptBacklog is how many connections wait for Accept; past it, a peer's
// SYN is dropped, and it tries again.
const acceptBacklog = 16

// ErrClosed reports a connection whose socket closed.
var ErrClosed = errors.New("the uTP socket closed")

// Socket carries the uTP connections of one PacketConn, which it reads
// alone: packets are told apart by their sender and connection ID, and
// those of no connection are dropped.
type Socket struct {
	conn   PacketConn
	listen bool

	mu       sync.Mutex
	conns    map[connKey]*Conn
	accepted chan *Conn
	err      error
	done     chan struct{}
}

// connKey is how a packet finds its connection: who sent it, and the
// connection ID it carries, the receiving side's.
type connKey struct {
	remote netip.AddrPort
	id     uint16
}

// NewSocket runs uTP over conn, which it closes when it closes: Dial opens
// connections, and peers cannot open any.
func NewSocket(conn PacketConn) *Socket {
	return newSocket(conn, false)
}

// Listen runs uTP over conn, as NewSocket, and also accepts the
// connections peers open (see Accept).
func Listen(conn PacketConn) *Socket {
	return newSocket(conn, true)
}

func newSocket(conn PacketConn, listen bool) *Socket {
	s := &Socket{conn: conn, listen: listen, conns: map[connKey]*Conn{}, done: make(chan struct{})}
	if listen {
		s.accepted = make(chan *Conn, acceptBacklog)
	}
	go s.read()
	return s
}

// Done is closed once the socket closed, by Close or its PacketConn
// failing.
func (s *Socket) Done() <-chan struct{} { return s.done }

// Close closes the socket and its PacketConn, ending every connection with
// a FIN: a peer keeps a connection it was not told of, and may take
// Polyfin's next one from the same address for a duplicate.
func (s *Socket) Close() error {
	s.end(ErrClosed, true)
	return nil
}

func (s *Socket) end(err error, fin bool) {
	s.mu.Lock()
	if s.err != nil {
		s.mu.Unlock()
		return
	}
	s.err = err
	conns := make([]*Conn, 0, len(s.conns))
	for _, c := range s.conns {
		conns = append(conns, c)
	}
	close(s.done)
	s.mu.Unlock()
	if fin {
		for _, c := range conns {
			c.mu.Lock()
			c.sendFin()
			c.mu.Unlock()
		}
	}
	_ = s.conn.Close()
	for _, c := range conns {
		c.mu.Lock()
		c.fail(err)
		c.mu.Unlock()
	}
}

// read reads the datagrams, and gives each to its connection.
func (s *Socket) read() {
	buffer := make([]byte, maxDatagram)
	for {
		n, from, err := s.conn.ReadFrom(buffer)
		if err != nil {
			s.end(ErrClosed, false)
			return
		}
		h, payload, err := parse(buffer[:n])
		if err != nil {
			continue
		}
		key := connKey{from, h.connID}
		if h.typ == stSyn {
			// A SYN carries the ID the initiator receives with: the
			// connection's, here, is the next one.
			key.id++
		}
		s.mu.Lock()
		c := s.conns[key]
		s.mu.Unlock()
		if c == nil {
			if h.typ == stSyn && s.listen {
				s.accept(from, h)
			}
			continue
		}
		c.mu.Lock()
		c.handle(h, payload)
		c.mu.Unlock()
	}
}

// accept answers a peer's SYN with a new connection, unless Accept is
// behind. Only the reading goroutine fills the backlog, so the room checked
// is there once the connection answered.
func (s *Socket) accept(from netip.AddrPort, h header) {
	s.mu.Lock()
	if s.err != nil || len(s.accepted) == cap(s.accepted) {
		s.mu.Unlock()
		return
	}
	c := newConn(s, from, h.connID+1, h.connID)
	s.conns[connKey{from, c.recvID}] = c
	s.mu.Unlock()
	c.mu.Lock()
	c.accepted(h)
	c.mu.Unlock()
	s.accepted <- c
}

// Accept returns the next connection a peer opened, on a socket of Listen.
func (s *Socket) Accept() (*Conn, error) {
	if !s.listen {
		return nil, errors.New("the uTP socket does not listen")
	}
	select {
	case c := <-s.accepted:
		return c, nil
	case <-s.done:
		return nil, net.ErrClosed
	}
}

// Dial opens a connection to remote, waiting for its answer until ctx is
// done or the SYN was sent synTries times unanswered.
func (s *Socket) Dial(ctx context.Context, remote netip.AddrPort) (*Conn, error) {
	s.mu.Lock()
	if s.err != nil {
		err := s.err
		s.mu.Unlock()
		return nil, err
	}
	var id uint16
	for {
		var b [2]byte
		_, _ = rand.Read(b[:])
		id = binary.BigEndian.Uint16(b[:])
		if s.conns[connKey{remote, id}] == nil && s.conns[connKey{remote, id + 1}] == nil {
			break
		}
	}
	c := newConn(s, remote, id, id+1)
	s.conns[connKey{remote, id}] = c
	s.mu.Unlock()
	c.mu.Lock()
	c.connect()
	c.mu.Unlock()
	select {
	case <-c.established:
		return c, nil
	case <-c.ended:
		c.mu.Lock()
		err := c.err
		c.mu.Unlock()
		return nil, err
	case <-ctx.Done():
		c.mu.Lock()
		c.fail(ctx.Err())
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

// remove forgets a connection that ended.
func (s *Socket) remove(c *Conn) {
	s.mu.Lock()
	if s.conns[connKey{c.remote, c.recvID}] == c {
		delete(s.conns, connKey{c.remote, c.recvID})
	}
	s.mu.Unlock()
}

// Conns counts the socket's connections.
func (s *Socket) Conns() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.conns)
}
