package torrent

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"time"

	"github.com/moodiness/polyfin/internal/torrent/utp"
)

// Peers are reached over TCP and uTP (BEP 29), both over the route. uTP
// runs over one UDP socket of the route for each torrent, apart from the
// DHT's and the trackers': Polyfin listens for no peer, so one port shared
// by all would bring nothing, and the uTP socket lives as long as its
// torrent while a lookup's or an announce's ends with it.
const (
	// utpHeadStart is how long a peer's uTP has to answer before TCP is
	// tried too: a peer without uTP, or whose UDP is dropped, delays its
	// TCP connection by that much at most, and a peer whose uTP answered
	// within it is not reached twice. utpConnect bounds the wait for that
	// answer, two SYNs: a peer that refuses TCP and does not answer uTP
	// holds a dial that long.
	utpHeadStart = 500 * time.Millisecond
	utpConnect   = 3 * time.Second
	// tcpGrace is the least a TCP connection up waits for uTP before its
	// handshake, while the torrent has no peer yet: uTP's answer takes
	// about as long as TCP's.
	tcpGrace = 20 * time.Millisecond
	// utpRetry is how long, after the route opened no UDP socket for
	// uTP, it is not asked again.
	utpRetry = 30 * time.Second
)

// errNoUTP reports a peer tried over uTP while the torrent has no uTP
// socket.
var errNoUTP = errors.New("no uTP socket over the route")

// attempt is a connection to a peer one way, or why it failed; once shaken,
// whether the peer speaks the extension protocol.
type attempt struct {
	conn       net.Conn
	extensions bool
	utp        bool
	err        error
}

// dialPeer connects to a peer and shakes hands, within handshakeTimeout:
// over uTP first when the route carries UDP and the peer has an IPv4
// address, the route's UDP sockets being IPv4, and over TCP once uTP
// failed or did not answer within utpHeadStart. While the torrent has no
// peer yet, TCP connects at once too, so that its start waits for no head
// start, but shakes hands only if uTP is not up within as long again as
// TCP took to connect (tcpGrace at least). Only the connections race: the
// handshake goes over one connection at a time, the other waiting unused,
// and over that one only if the first fails. A peer never gets Polyfin's
// peer ID over both at once, which it takes for a duplicate and drops.
func (t *torrent) dialPeer(address netip.AddrPort, dht bool) (net.Conn, bool, error) {
	ctx, cancel := context.WithTimeout(t.ctx, handshakeTimeout)
	defer cancel()
	started := time.Now()
	connected := make(chan attempt, 2)
	pending := 0
	tcp := false
	startTCP := func() {
		tcp = true
		pending++
		go func() {
			defer t.recovered("TCP dial", func(err error) { connected <- attempt{err: err} })
			connected <- t.dialTCP(ctx, address)
		}()
	}
	// tcpMayShake tells TCP's handshake waits for uTP no more.
	tcpMayShake := true
	var headStart, grace <-chan time.Time
	if t.udp && !t.engine.utpOff && address.Addr().Unmap().Is4() {
		tcpMayShake = false
		pending++
		go func() {
			defer t.recovered("uTP dial", func(err error) { connected <- attempt{utp: true, err: err} })
			connected <- t.dialUTP(ctx, address)
		}()
		t.mu.Lock()
		first := len(t.peers) == 0
		t.mu.Unlock()
		if first {
			startTCP()
		} else {
			timer := time.NewTimer(utpHeadStart)
			defer timer.Stop()
			headStart = timer.C
		}
	} else {
		startTCP()
	}
	shaken := make(chan attempt, 1)
	shaking := false
	// up are the connections up and unused, uTP's first.
	var up []attempt
	var err error
	failed := func(r attempt) {
		if err == nil || !r.utp {
			err = r.err
		}
		if r.utp {
			tcpMayShake = true
			if !tcp {
				startTCP()
			}
		}
	}
	for {
		if !shaking && len(up) > 0 && (up[0].utp || tcpMayShake) {
			shaking = true
			go func(a attempt) {
				defer t.recovered("handshake", func(err error) {
					a.conn.Close()
					shaken <- attempt{utp: a.utp, err: err}
				})
				shaken <- t.shake(ctx, a, dht)
			}(up[0])
			up = up[1:]
		}
		if !shaking && pending == 0 && len(up) == 0 {
			return nil, false, err
		}
		select {
		case <-headStart:
			headStart, tcpMayShake = nil, true
			if !tcp {
				startTCP()
			}
		case <-grace:
			grace, tcpMayShake = nil, true
		case r := <-connected:
			pending--
			switch {
			case r.err != nil:
				failed(r)
			case r.utp:
				// uTP answered: TCP waits for its handshake.
				headStart = nil
				up = append([]attempt{r}, up...)
			default:
				up = append(up, r)
				if !tcpMayShake {
					timer := time.NewTimer(min(max(time.Since(started), tcpGrace), utpHeadStart))
					defer timer.Stop()
					grace = timer.C
				}
			}
		case r := <-shaken:
			shaking = false
			if r.err == nil {
				for _, a := range up {
					a.conn.Close()
				}
				for range pending {
					go func() {
						defer t.recovered("dial", nil)
						closeLater(connected)
					}()
				}
				return r.conn, r.extensions, nil
			}
			failed(r)
		}
	}
}

// closeLater closes the connection of an attempt that was not needed, as
// it ends: its context is done.
func closeLater(connected <-chan attempt) {
	if r := <-connected; r.conn != nil {
		r.conn.Close()
	}
}

func (t *torrent) dialTCP(ctx context.Context, address netip.AddrPort) attempt {
	conn, err := t.route.Dial(ctx, address.String())
	return attempt{conn: conn, err: err}
}

// dialUTP connects to a peer over uTP, waiting utpConnect at most.
func (t *torrent) dialUTP(ctx context.Context, address netip.AddrPort) attempt {
	socket := t.utpSocket()
	if socket == nil {
		return attempt{utp: true, err: errNoUTP}
	}
	ctx, cancel := context.WithTimeout(ctx, utpConnect)
	defer cancel()
	conn, err := socket.Dial(ctx, address)
	if err != nil {
		return attempt{utp: true, err: err}
	}
	return attempt{conn: conn, utp: true}
}

// shake shakes hands with a peer over a's connection, until ctx is done.
func (t *torrent) shake(ctx context.Context, a attempt, dht bool) attempt {
	conn := a.conn
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	extensions, err := handshake(conn, t.hash, t.engine.peerID, dht)
	if !stop() && err == nil {
		err = ctx.Err()
	}
	if err != nil {
		conn.Close()
		return attempt{utp: a.utp, err: err}
	}
	_ = conn.SetDeadline(time.Time{})
	return attempt{conn: conn, extensions: extensions, utp: a.utp}
}

// utpSocket returns the torrent's uTP socket, opening it over the route on
// first use or once the last one closed, as when a proxy dropped its UDP
// ASSOCIATE; nil once the torrent stopped, or within utpRetry of a route
// that opened none.
func (t *torrent) utpSocket() *utp.Socket {
	t.utpMu.Lock()
	defer t.utpMu.Unlock()
	if s := t.utpSock; s != nil {
		select {
		case <-s.Done():
		default:
			return s
		}
	}
	if t.utpStopped || time.Now().Before(t.utpRetryAt) {
		return nil
	}
	conn, err := t.route.ListenPacket(t.ctx)
	if err != nil {
		t.utpRetryAt = time.Now().Add(utpRetry)
		return nil
	}
	t.utpSock = utp.NewSocket(conn)
	return t.utpSock
}

// stopUTP closes the torrent's uTP socket, its connections with it, and
// opens no other.
func (t *torrent) stopUTP() {
	t.utpMu.Lock()
	defer t.utpMu.Unlock()
	t.utpStopped = true
	if t.utpSock != nil {
		t.utpSock.Close()
	}
}

// overUTP reports whether a peer was reached over uTP.
func overUTP(p *peer) bool {
	_, ok := p.conn.(*utp.Conn)
	return ok
}
