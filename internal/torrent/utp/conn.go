package utp

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"
)

const (
	// maxPayload is a packet's payload at most: with its header, UDP's
	// and IPv6's, it fits the 1280 bytes every IPv6 link carries, and so
	// a tunnel's packets.
	maxPayload = 1200
	// recvWindow bounds the bytes received and not read, those received
	// out of order with them; maxReorder how far past the next one a
	// packet is kept.
	recvWindow = 1 << 20
	maxReorder = 1024
	// sendBuffer is what Write queues before it waits; maxInflight bounds
	// the packets sent and not acknowledged.
	sendBuffer  = 256 << 10
	maxInflight = 1024
	// The congestion window: minCwnd and maxCwnd bound it, it starts at
	// initialCwnd as TCP's does (RFC 6928), and LEDBAT aims at
	// targetDelay of delay added, growing it by maxGain bytes per round
	// trip at most. The current delay is taken over buckets of half a
	// round trip, minBucket at least.
	minCwnd     = maxPayload
	initialCwnd = 10 * maxPayload
	maxCwnd     = 4 << 20
	targetDelay = 100 * time.Millisecond
	maxGain     = 3000
	minBucket   = 20 * time.Millisecond
	// The retransmission timeout: initialRTO before a round trip is
	// measured, minRTO and maxRTO its bounds (BEP 29). A connection ends
	// at its maxTimeouts-th timeout in a row, about 15 seconds without an
	// answer, or once synTries SYNs went unanswered, 7 seconds.
	initialRTO  = time.Second
	minRTO      = 500 * time.Millisecond
	maxRTO      = 30 * time.Second
	maxTimeouts = 5
	synTries    = 3
	// keepAlive is how long a connection may send nothing, as a NAT
	// forgets an idle flow; delayedAck how long a lone packet received in
	// order waits for its acknowledgment.
	keepAlive  = 29 * time.Second
	delayedAck = 10 * time.Millisecond
)

// ErrReset reports a connection the peer reset; ErrTimeout one whose peer
// stopped answering.
var (
	ErrReset   = errors.New("the peer reset the uTP connection")
	ErrTimeout = errors.New("the uTP peer stopped answering")
)

// The states of a connection.
const (
	stateSynSent = iota
	stateConnected
	stateEnded
)

// epoch is the start of the clock packets carry, in microseconds.
var epoch = time.Now()

func micros() uint32 { return uint32(time.Since(epoch).Microseconds()) }

// packet is a packet sent and not acknowledged yet.
type packet struct {
	typ     byte
	seq     uint16
	payload []byte
	buf     [maxPayload]byte
	sentAt  time.Time
	// sent counts its transmissions; counted tells it counts in the
	// bytes in flight; needResend that it is lost, to send again;
	// sacked that a selective acknowledgment told it arrived; fastResent
	// that it was told lost once already.
	sent       int
	counted    bool
	needResend bool
	sacked     bool
	fastResent bool
}

var packets = sync.Pool{New: func() any { return new(packet) }}

// Conn is a uTP connection: a net.Conn whose bytes go in packets over its
// socket.
type Conn struct {
	s              *Socket
	remote         netip.AddrPort
	recvID, sendID uint16
	// established is closed once the connection is, ended once it ended.
	established chan struct{}
	ended       chan struct{}

	mu      sync.Mutex
	state   int
	err     error
	closed  bool
	finSent bool
	changed chan struct{}
	waiting int
	timer   *time.Timer
	timerAt time.Time
	out     []byte
	sackBuf [maxSACK]byte

	// Sending: seq is the next sequence number, inflight the packets
	// not acknowledged in order, flightBytes the bytes among them that
	// count, sendData[sendOff:] what Write queued.
	seq         uint16
	inflight    []*packet
	flightBytes int
	sendData    []byte
	sendOff     int
	peerWnd     int
	cwnd        int
	ssthresh    int
	slowStart   bool
	cwndLimited bool
	// cutSeq is where the window was last halved: a loss of a packet sent
	// before is the same congestion.
	cutSeq   uint16
	cut      bool
	dupAcks  int
	rtt      time.Duration
	rttVar   time.Duration
	rto      time.Duration
	rtoAt    time.Time
	timeouts int
	lastSent time.Time
	// The delay LEDBAT measures: the lowest sample of this minute and of
	// the last one, those of the current bucket and of the last one, and
	// our delay, the current one above the base one.
	histAt     time.Time
	histCur    uint32
	histPrev   uint32
	recentAt   time.Time
	recentCur  uint32
	recentPrev uint32
	ourDelay   time.Duration
	replyDiff  uint32

	// Receiving: ack is the last sequence number received in order,
	// recv[recvOff:] the bytes not read, reorder the packets received
	// past ack+1.
	ack          uint16
	recv         []byte
	recvOff      int
	reorder      map[uint16][]byte
	reorderBytes int
	needAck      bool
	unacked      int
	ackAt        time.Time
	finGot       bool
	finSeq       uint16
	eof          bool

	readDeadline, writeDeadline time.Time
}

func newConn(s *Socket, remote netip.AddrPort, recvID, sendID uint16) *Conn {
	return &Conn{s: s, remote: remote, recvID: recvID, sendID: sendID, established: make(chan struct{}), ended: make(chan struct{}),
		changed: make(chan struct{}), reorder: map[uint16][]byte{}, cwnd: initialCwnd, ssthresh: maxCwnd, slowStart: true,
		rto: initialRTO, peerWnd: maxPayload}
}

// connect sends the SYN of a connection being dialed. The lock is held.
func (c *Conn) connect() {
	c.state = stateSynSent
	c.seq = 1
	p := packets.Get().(*packet)
	*p = packet{typ: stSyn, seq: c.seq}
	c.seq++
	c.inflight = append(c.inflight, p)
	c.sendPacket(p, time.Now())
	c.arm()
}

// accepted answers a peer's SYN. The lock is held.
func (c *Conn) accepted(h header) {
	var b [2]byte
	_, _ = rand.Read(b[:])
	c.state = stateConnected
	c.seq = binary.BigEndian.Uint16(b[:])
	c.ack = h.seq
	c.replyDiff = micros() - h.timestamp
	c.peerWnd = int(min(h.wnd, maxCwnd))
	close(c.established)
	c.sendState()
	c.arm()
}

// handle reads a packet of the connection. The lock is held.
func (c *Conn) handle(h header, payload []byte) {
	if c.err != nil {
		return
	}
	if h.typ == stReset {
		c.fail(ErrReset)
		return
	}
	now := time.Now()
	if c.state == stateSynSent {
		if h.typ != stState || h.ack != c.seq-1 {
			return
		}
		c.state = stateConnected
		c.ack = h.seq - 1
		close(c.established)
	}
	c.replyDiff = micros() - h.timestamp
	if h.typ == stSyn {
		// The peer did not receive the answer to its SYN.
		c.sendState()
		return
	}
	c.peerWnd = int(min(h.wnd, maxCwnd))
	c.acked(h, len(payload), now)
	if h.typ == stData || h.typ == stFin {
		c.receive(h, payload, now)
	}
	if c.err != nil {
		return
	}
	c.flush()
	if c.needAck {
		c.sendState()
	}
	c.arm()
	c.signal()
}

// acked reads what a packet acknowledges: every packet up to h.ack, and
// those its selective acknowledgment tells. An acknowledgment of a packet
// never sent, or older than the oldest in flight, is not read.
func (c *Conn) acked(h header, size int, now time.Time) {
	if len(c.inflight) == 0 {
		c.dupAcks = 0
		return
	}
	first := c.inflight[0].seq
	if seqLess(h.ack, first-1) || seqLess(c.seq-1, h.ack) {
		return
	}
	bytes := 0
	n := 0
	var sample time.Duration
	for n < len(c.inflight) && !seqLess(h.ack, c.inflight[n].seq) {
		p := c.inflight[n]
		if p.counted {
			c.flightBytes -= len(p.payload)
		}
		if !p.sacked {
			bytes += len(p.payload)
		}
		if p.sent == 1 && sample == 0 {
			sample = now.Sub(p.sentAt)
		}
		n++
	}
	if n > 0 {
		for _, p := range c.inflight[:n] {
			packets.Put(p)
		}
		c.inflight = append(c.inflight[:0], c.inflight[n:]...)
		c.dupAcks = 0
		c.timeouts = 0
		c.rtoAt = time.Time{}
		if sample > 0 {
			c.measured(sample)
		}
	} else if h.typ == stState && size == 0 && h.ack == first-1 {
		c.dupAcks++
		if c.dupAcks == 3 && !c.inflight[0].fastResent {
			c.lose(c.inflight[0])
		}
	}
	if h.sack != nil {
		base := h.ack + 2
		for _, p := range c.inflight {
			d := int(p.seq - base)
			if p.sacked || d >= len(h.sack)*8 || h.sack[d/8]&(1<<(d%8)) == 0 {
				continue
			}
			p.sacked, p.needResend = true, false
			if p.counted {
				p.counted = false
				c.flightBytes -= len(p.payload)
			}
			bytes += len(p.payload)
		}
		// A packet three or more packets past which arrived is lost.
		count := 0
		for i := len(c.inflight) - 1; i >= 0; i-- {
			p := c.inflight[i]
			if p.sacked {
				count++
			} else if count >= 3 && !p.fastResent {
				c.lose(p)
			}
		}
	}
	if bytes > 0 {
		c.congestion(bytes, h.timestampDiff, now)
	}
}

// measured takes a round trip sample, for the timeout (BEP 29).
func (c *Conn) measured(sample time.Duration) {
	if c.rtt == 0 {
		c.rtt, c.rttVar = sample, sample/2
	} else {
		delta := c.rtt - sample
		if delta < 0 {
			delta = -delta
		}
		c.rttVar += (delta - c.rttVar) / 4
		c.rtt += (sample - c.rtt) / 8
	}
	c.rto = min(max(c.rtt+4*c.rttVar, minRTO), maxRTO)
}

// lose sends a lost packet again, and halves the window once for the
// packets sent before.
func (c *Conn) lose(p *packet) {
	p.fastResent, p.needResend = true, true
	if p.counted {
		p.counted = false
		c.flightBytes -= len(p.payload)
	}
	if c.cut && seqLess(p.seq, c.cutSeq) {
		return
	}
	c.cwnd = max(c.cwnd/2, minCwnd)
	c.ssthresh, c.slowStart = c.cwnd, false
	c.cut, c.cutSeq = true, c.seq
}

// congestion grows or shrinks the window as LEDBAT does (BEP 29, RFC
// 6817), from the bytes acknowledged and the delay the peer measured: by
// maxGain bytes per round trip at most, as the delay is below targetDelay,
// less above, and by half at most. The delay is the lowest sample of about
// the last round trip, above the lowest of the last two minutes, which also
// follows the drift of the two clocks: a jitter alone is not taken for a
// queue. Until a loss or the target, the window doubles each round trip.
func (c *Conn) congestion(bytes int, diff uint32, now time.Time) {
	if diff != 0 {
		switch {
		case c.histAt.IsZero():
			c.histAt, c.histCur, c.histPrev = now, diff, diff
		case now.Sub(c.histAt) >= time.Minute:
			c.histAt, c.histPrev, c.histCur = now, c.histCur, diff
		case int32(diff-c.histCur) < 0:
			c.histCur = diff
		}
		base := c.histCur
		if int32(c.histPrev-base) < 0 {
			base = c.histPrev
		}
		// The current delay is the lowest of two buckets of half a
		// round trip each, the one filling and the last one.
		bucket := max(c.rtt/2, minBucket)
		switch {
		case c.recentAt.IsZero():
			c.recentAt, c.recentCur, c.recentPrev = now, diff, diff
		case now.Sub(c.recentAt) >= bucket:
			c.recentAt, c.recentPrev, c.recentCur = now, c.recentCur, diff
		case int32(diff-c.recentCur) < 0:
			c.recentCur = diff
		}
		current := c.recentCur
		if int32(c.recentPrev-current) < 0 {
			current = c.recentPrev
		}
		c.ourDelay = max(time.Duration(int32(current-base))*time.Microsecond, 0)
	}
	if !c.cwndLimited {
		return
	}
	if c.slowStart {
		if c.ourDelay < targetDelay && c.cwnd < c.ssthresh {
			c.cwnd = min(c.cwnd+bytes, maxCwnd)
			return
		}
		c.slowStart = false
	}
	gain := float64(maxGain) * float64(targetDelay-c.ourDelay) / float64(targetDelay) * float64(bytes) / float64(c.cwnd)
	c.cwnd = min(max(c.cwnd+max(int(gain), -bytes/2), minCwnd), maxCwnd)
}

// receive keeps a packet's payload, in order or past the next one. A packet
// past the next one is kept within the receive window; the next one is kept
// as long as what was not read leaves room for it, the packets past it
// aside: they wait for it, and a window they filled would otherwise hold
// it out for good. What a connection keeps stays within twice the window.
// Every second packet received in order is acknowledged at once, as TCP
// does, the first within delayedAck; any other at once. The lock is held.
func (c *Conn) receive(h header, payload []byte, now time.Time) {
	c.needAck = true
	d := int(int16(h.seq - (c.ack + 1)))
	if d < 0 || d >= maxReorder || c.finGot && seqLess(c.finSeq, h.seq) {
		return
	}
	if h.typ == stFin {
		if c.finGot && h.seq != c.finSeq {
			return
		}
		c.finGot, c.finSeq, payload = true, h.seq, nil
	}
	if d > 0 {
		if _, ok := c.reorder[h.seq]; !ok && c.buffered()+len(payload) <= recvWindow {
			c.reorder[h.seq] = append([]byte{}, payload...)
			c.reorderBytes += len(payload)
		}
		return
	}
	if len(c.recv)-c.recvOff+len(payload) > recvWindow {
		return
	}
	c.deliver(payload)
	c.ack = h.seq
	if h.typ == stData && len(c.reorder) == 0 {
		if c.unacked++; c.unacked < 2 {
			c.needAck = false
			if c.ackAt.IsZero() {
				c.ackAt = now.Add(delayedAck)
			}
		}
	}
	for {
		next, ok := c.reorder[c.ack+1]
		if !ok {
			break
		}
		delete(c.reorder, c.ack+1)
		c.reorderBytes -= len(next)
		c.deliver(next)
		c.ack++
	}
	if c.finGot && c.ack == c.finSeq {
		c.eof = true
	}
}

func (c *Conn) deliver(p []byte) {
	if c.recvOff > 0 && len(c.recv)+len(p) > cap(c.recv) {
		n := copy(c.recv, c.recv[c.recvOff:])
		c.recv, c.recvOff = c.recv[:n], 0
	}
	c.recv = append(c.recv, p...)
}

// buffered is what was received and not read.
func (c *Conn) buffered() int { return len(c.recv) - c.recvOff + c.reorderBytes }

// window is the receive window to tell the peer.
func (c *Conn) window() int { return max(recvWindow-c.buffered(), 0) }

func (c *Conn) queued() int { return len(c.sendData) - c.sendOff }

// flush sends the lost packets again, then what Write queued, within the
// congestion window and the peer's. The lock is held.
func (c *Conn) flush() {
	if c.state != stateConnected || c.err != nil {
		return
	}
	now := time.Now()
	for _, p := range c.inflight {
		if !p.needResend {
			continue
		}
		if !c.fits(len(p.payload)) {
			c.cwndLimited = true
			return
		}
		c.sendPacket(p, now)
	}
	for c.queued() > 0 && len(c.inflight) < maxInflight {
		size := min(c.queued(), maxPayload)
		if !c.fits(size) {
			c.cwndLimited = true
			return
		}
		p := packets.Get().(*packet)
		*p = packet{typ: stData, seq: c.seq}
		p.payload = append(p.buf[:0], c.sendData[c.sendOff:c.sendOff+size]...)
		c.sendOff += size
		if c.sendOff == len(c.sendData) {
			c.sendData, c.sendOff = c.sendData[:0], 0
		}
		c.seq++
		c.inflight = append(c.inflight, p)
		c.sendPacket(p, now)
	}
	if c.queued() == 0 {
		c.cwndLimited = false
	}
}

// fits tells whether size more bytes may be in flight: one packet always
// may, within the peer's window.
func (c *Conn) fits(size int) bool {
	if c.flightBytes+size > c.peerWnd {
		return false
	}
	return c.flightBytes == 0 || c.flightBytes+size <= c.cwnd
}

func (c *Conn) sendPacket(p *packet, now time.Time) {
	p.sent++
	p.sentAt, p.needResend = now, false
	if !p.counted {
		p.counted = true
		c.flightBytes += len(p.payload)
	}
	c.transmit(p.typ, p.seq, p.payload)
}

// sendState acknowledges what was received, with a selective
// acknowledgment of the packets kept past the next one.
func (c *Conn) sendState() {
	c.transmit(stState, c.seq, nil)
}

// transmit sends a packet, with the acknowledgment of what was received.
func (c *Conn) transmit(typ byte, seq uint16, payload []byte) {
	id := c.sendID
	if typ == stSyn {
		id = c.recvID
	}
	h := header{typ: typ, connID: id, timestamp: micros(), timestampDiff: c.replyDiff, wnd: uint32(c.window()), seq: seq, ack: c.ack}
	if typ == stState {
		h.sack = c.sack()
	}
	if h.sack != nil || len(c.reorder) == 0 {
		c.needAck = false
		c.unacked, c.ackAt = 0, time.Time{}
	}
	c.out = append(appendHeader(c.out[:0], h), payload...)
	// A packet the socket cannot send counts as lost: its timeout tells.
	_ = c.s.conn.WriteToAddrPort(c.out, c.remote)
	c.lastSent = time.Now()
}

// sack is the selective acknowledgment of the packets kept past ack+1,
// nil for none.
func (c *Conn) sack() []byte {
	if len(c.reorder) == 0 {
		return nil
	}
	clear(c.sackBuf[:])
	base, top := c.ack+2, -1
	for seq := range c.reorder {
		if d := int(seq - base); d < maxSACK*8 {
			c.sackBuf[d/8] |= 1 << (d % 8)
			top = max(top, d)
		}
	}
	if top < 0 {
		return nil
	}
	return c.sackBuf[:(top/32+1)*4]
}

// arm sets the timer to the next timeout or keep-alive. The lock is held.
func (c *Conn) arm() {
	if c.err != nil {
		return
	}
	now := time.Now()
	switch {
	case len(c.inflight) == 0 && c.queued() == 0:
		c.rtoAt = time.Time{}
	case c.rtoAt.IsZero():
		c.rtoAt = now.Add(c.rto)
	}
	next := c.rtoAt
	if c.state == stateConnected && (next.IsZero() || c.lastSent.Add(keepAlive).Before(next)) {
		next = c.lastSent.Add(keepAlive)
	}
	if !c.ackAt.IsZero() && c.ackAt.Before(next) {
		next = c.ackAt
	}
	if next.IsZero() || !c.timerAt.IsZero() && !c.timerAt.After(next) {
		return
	}
	c.timerAt = next
	d := max(next.Sub(now), time.Millisecond)
	if c.timer == nil {
		c.timer = time.AfterFunc(d, c.onTimer)
	} else {
		c.timer.Reset(d)
	}
}

func (c *Conn) onTimer() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timerAt = time.Time{}
	if c.err != nil {
		return
	}
	now := time.Now()
	if !c.rtoAt.IsZero() && !now.Before(c.rtoAt) {
		c.rtoAt = time.Time{}
		if len(c.inflight) > 0 {
			c.timeout(now)
		} else if c.queued() > 0 {
			// The peer's window is closed: one packet probes it.
			c.peerWnd = max(c.peerWnd, maxPayload)
			c.flush()
		}
		if c.err != nil {
			return
		}
	}
	if c.state == stateConnected && (now.Sub(c.lastSent) >= keepAlive || !c.ackAt.IsZero() && !now.Before(c.ackAt)) {
		c.sendState()
	}
	c.arm()
}

// timeout sends the packets in flight again from a window of one packet,
// waiting twice as long for the next timeout; the connection ends after
// too many. The lock is held.
func (c *Conn) timeout(now time.Time) {
	c.timeouts++
	if c.state == stateSynSent && c.timeouts >= synTries || c.timeouts >= maxTimeouts {
		c.fail(ErrTimeout)
		return
	}
	c.rto = min(c.rto*2, maxRTO)
	c.ssthresh = max(c.cwnd/2, 2*minCwnd)
	c.cwnd, c.slowStart = minCwnd, true
	for _, p := range c.inflight {
		if p.sacked {
			continue
		}
		p.needResend = true
		if p.counted {
			p.counted = false
			c.flightBytes -= len(p.payload)
		}
	}
	if c.state == stateSynSent {
		c.sendPacket(c.inflight[0], now)
		return
	}
	c.flush()
}

// fail ends the connection with err. The lock is held.
func (c *Conn) fail(err error) {
	if c.err != nil {
		return
	}
	c.err, c.state = err, stateEnded
	if c.timer != nil {
		c.timer.Stop()
	}
	for _, p := range c.inflight {
		packets.Put(p)
	}
	c.inflight, c.flightBytes = nil, 0
	close(c.ended)
	c.signal()
	c.s.remove(c)
}

// signal wakes the reads and writes waiting. The lock is held.
func (c *Conn) signal() {
	if c.waiting > 0 {
		close(c.changed)
		c.changed = make(chan struct{})
	}
}

// wait waits for the connection to change, or deadline. The lock is held,
// and released meanwhile.
func (c *Conn) wait(deadline time.Time) error {
	var expired <-chan time.Time
	if !deadline.IsZero() {
		d := time.Until(deadline)
		if d <= 0 {
			return os.ErrDeadlineExceeded
		}
		timer := time.NewTimer(d)
		defer timer.Stop()
		expired = timer.C
	}
	changed := c.changed
	c.waiting++
	c.mu.Unlock()
	select {
	case <-changed:
	case <-expired:
	}
	c.mu.Lock()
	c.waiting--
	return nil
}

// Read reads what the peer sent, io.EOF once it closed the connection and
// all was read.
func (c *Conn) Read(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for {
		if c.recvOff < len(c.recv) {
			before := c.window()
			n := copy(b, c.recv[c.recvOff:])
			c.recvOff += n
			if c.recvOff == len(c.recv) {
				c.recv, c.recvOff = c.recv[:0], 0
			}
			// A window that was nearly closed tells the peer it opened.
			if before < recvWindow/4 && c.window() >= recvWindow/4 && c.err == nil && c.state == stateConnected {
				c.sendState()
			}
			return n, nil
		}
		switch {
		case c.eof:
			return 0, io.EOF
		case c.closed:
			return 0, net.ErrClosed
		case c.err != nil:
			return 0, c.err
		}
		if err := c.wait(c.readDeadline); err != nil {
			return 0, err
		}
	}
}

// Write queues b for the peer, waiting while sendBuffer bytes wait.
func (c *Conn) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	written := 0
	for {
		switch {
		case c.closed:
			return written, net.ErrClosed
		case c.err != nil:
			return written, c.err
		}
		if room := sendBuffer - c.queued(); room > 0 && written < len(b) {
			n := min(room, len(b)-written)
			c.sendData = append(c.sendData, b[written:written+n]...)
			written += n
			c.flush()
			c.arm()
		}
		if written == len(b) {
			return written, nil
		}
		if err := c.wait(c.writeDeadline); err != nil {
			return written, err
		}
	}
}

// Close ends the connection, telling the peer with a FIN.
func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	c.sendFin()
	c.fail(net.ErrClosed)
	return nil
}

// sendFin tells the peer the connection ends, once. The lock is held.
func (c *Conn) sendFin() {
	if c.err == nil && c.state == stateConnected && !c.finSent {
		c.finSent = true
		c.transmit(stFin, c.seq, nil)
		c.seq++
	}
}

// RemoteAddr is the peer's UDP address.
func (c *Conn) RemoteAddr() net.Addr { return net.UDPAddrFromAddrPort(c.remote) }

// LocalAddr is unknown: a route's socket does not tell it.
func (c *Conn) LocalAddr() net.Addr { return &net.UDPAddr{} }

func (c *Conn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readDeadline, c.writeDeadline = t, t
	c.signal()
	return nil
}

func (c *Conn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readDeadline = t
	c.signal()
	return nil
}

func (c *Conn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writeDeadline = t
	c.signal()
	return nil
}

var _ net.Conn = (*Conn)(nil)
