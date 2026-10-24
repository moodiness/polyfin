package torrent

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/netfail"
)

// How long a torrent may take to answer a playback. A link's host sends
// its headers within seconds; a torrent first needs peers, from trackers
// that may each take their 30 seconds (trackerTimeout) and from DHT
// lookups that hop from node to node, 3 seconds at most each, then the
// info dictionary from one of them (BEP 9), then its first pieces.
const (
	// StartTimeout bounds the wait for a torrent's start, from a read
	// asking for it: peers found and the info dictionary received. Two
	// rounds of a silent tracker fit, beside the DHT lookups.
	StartTimeout = 60 * time.Second
	// PieceTimeout is how long a read may wait for a piece: peers choose
	// whom they send to every 10 seconds, and try a new peer every 30.
	PieceTimeout = 30 * time.Second
)

// StartError reports a torrent that did not start in time: no peer sent
// its info dictionary within the engine's StartTimeout. It tells where
// peers were looked for, and what came of it.
type StartError struct {
	After time.Duration
	// Trackers counts the trackers announced to, Answered those that
	// answered, Refused those that refused the announce; the others did not
	// answer. UDPOff counts the UDP trackers left out, the route carrying
	// no UDP.
	Trackers, Answered, Refused, UDPOff int
	// DHT tells whether the DHT was looked up, and NoUDP that it could not
	// be, the route carrying no UDP.
	DHT, NoUDP bool
	// Found counts the peers found, and Connected those that answered;
	// Unconnected tells why the others are not.
	Found, Connected int
	Unconnected      PeerFailures
}

// PeerFailures counts a torrent's peers not connected by how their last
// dial or connection ended: TimedOut did not answer in time, over TCP or
// uTP; Refused refused the connection, as the peer or a SOCKS5 proxy
// tells; Unreachable have no route to them; Proxy were refused by the
// SOCKS5 proxy for another reason, or could not be asked through it;
// Closed closed the connection during the handshake; Left were connected,
// and their connection ended; Other failed otherwise; Untried were not
// dialed yet, or their first dial runs.
type PeerFailures struct {
	TimedOut, Refused, Unreachable, Proxy, Closed, Left, Other, Untried int
}

func (f PeerFailures) String() string {
	var parts []string
	for _, part := range []struct {
		n    int
		what string
	}{{f.TimedOut, "timed out"}, {f.Refused, "refused"}, {f.Unreachable, "unreachable"}, {f.Proxy, "refused by the proxy"},
		{f.Closed, "closed the connection"}, {f.Left, "left"}, {f.Other, "failed otherwise"}, {f.Untried, "not dialed yet"}} {
		if part.n > 0 {
			parts = append(parts, strconv.Itoa(part.n)+" "+part.what)
		}
	}
	return strings.Join(parts, ", ")
}

func (e *StartError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "the torrent did not start in %s: %d peers found, %d connected", e.After.Round(time.Second), e.Found, e.Connected)
	if unconnected := e.Unconnected.String(); unconnected != "" {
		fmt.Fprintf(&b, " (%s)", unconnected)
	}
	fmt.Fprintf(&b, "; %d trackers, %d answered, %d refused the announce", e.Trackers, e.Answered, e.Refused)
	if e.UDPOff > 0 {
		fmt.Fprintf(&b, ", %d over UDP left out", e.UDPOff)
	}
	switch {
	case e.DHT:
		b.WriteString("; the DHT was looked up")
	case e.NoUDP:
		b.WriteString("; no DHT: the route carries no UDP")
	default:
		b.WriteString("; no DHT: the addon asks for none")
	}
	return b.String()
}

// outcome is how a peer's last dial or connection ended (see
// PeerFailures).
type outcome uint8

const (
	outcomeNone outcome = iota
	outcomeTimedOut
	outcomeRefused
	outcomeUnreachable
	outcomeProxy
	outcomeClosed
	outcomeLeft
	outcomeOther
)

// outcomeOf tells how a dial that failed with err ended: over TCP and uTP
// both, err is TCP's (see dialPeer).
func outcomeOf(err error) outcome {
	var reply *socksReplyError
	var route *RouteError
	var timeout interface{ Timeout() bool }
	switch {
	case errors.As(err, &reply):
		switch reply.code {
		case socksRefused:
			return outcomeRefused
		case socksNetUnreachable, socksHostUnreachable:
			return outcomeUnreachable
		case socksTTLExpired:
			return outcomeTimedOut
		}
		return outcomeProxy
	case errors.As(err, &route):
		return outcomeProxy
	case netfail.Refused(err):
		return outcomeRefused
	case netfail.Unreachable(err):
		return outcomeUnreachable
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, os.ErrDeadlineExceeded), errors.As(err, &timeout) && timeout.Timeout(),
		netfail.TimedOut(err):
		return outcomeTimedOut
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF), netfail.Reset(err), netfail.BrokenPipe(err):
		return outcomeClosed
	}
	// A WireGuard tunnel's network stack tells its errors by their text
	// alone.
	switch text := err.Error(); {
	case strings.HasSuffix(text, "connection was refused"):
		return outcomeRefused
	case strings.HasSuffix(text, "no route to host"), strings.HasSuffix(text, "network is unreachable"):
		return outcomeUnreachable
	case strings.HasSuffix(text, "operation timed out"):
		return outcomeTimedOut
	case strings.HasSuffix(text, "connection reset by peer"):
		return outcomeClosed
	}
	return outcomeOther
}

// unconnected counts the peers found and not connected by how their last
// dial or connection ended. The lock is held.
func (t *torrent) unconnected() PeerFailures {
	var f PeerFailures
	for address, c := range t.known {
		if t.peers[address] != nil {
			continue
		}
		switch c.last {
		case outcomeTimedOut:
			f.TimedOut++
		case outcomeRefused:
			f.Refused++
		case outcomeUnreachable:
			f.Unreachable++
		case outcomeProxy:
			f.Proxy++
		case outcomeClosed:
			f.Closed++
		case outcomeLeft:
			f.Left++
		case outcomeOther:
			f.Other++
		default:
			f.Untried++
		}
	}
	return f
}

// BootstrapNodes are the DHT nodes the main BitTorrent clients start their
// lookups from, for a torrent whose sources name none: without them, a
// torrent whose trackers fail finds no peer on the DHT. They are used over
// a route that carries UDP alone, like the rest of the DHT.
var BootstrapNodes = []string{
	"router.bittorrent.com:6881",
	"router.utorrent.com:6881",
	"dht.transmissionbt.com:6881",
	"dht.libtorrent.org:25401",
}

// peerIDAlphabet is what the random part of the peer ID is drawn from, as
// Transmission does: trackers read the ID from the announce's query, and a
// byte that needs escaping is one more way to get it wrong.
const peerIDAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

// newPeerID is Polyfin's peer ID for a run: its client and version in the
// usual form, then random characters.
func newPeerID() [20]byte {
	var id [20]byte
	copy(id[:], "-PF0001-")
	_, _ = rand.Read(id[8:])
	for i := 8; i < len(id); i++ {
		id[i] = peerIDAlphabet[int(id[i])%len(peerIDAlphabet)]
	}
	return id
}

// startFailed records a torrent that did not start in time, for Health.
func (e *Engine) startFailed(err *StartError) {
	e.mu.Lock()
	e.unstarted, e.unstartedAt = err, time.Now()
	e.mu.Unlock()
	e.config.Logger.Info("A torrent did not start in time", "error", err)
}

// startedOne clears what Health tells of a torrent that did not start,
// once another started.
func (e *Engine) startedOne() {
	e.mu.Lock()
	e.unstarted = nil
	e.mu.Unlock()
}

// startError tells what came of the search for the torrent's peers. The
// lock is held.
func (t *torrent) startError(after time.Duration) *StartError {
	err := &StartError{After: after, Found: len(t.known), Connected: len(t.peers), Unconnected: t.unconnected(), NoUDP: !t.udp,
		DHT: t.looked}
	for _, tracker := range t.trackers {
		if strings.HasPrefix(tracker, "udp:") && !t.udp {
			err.UDPOff++
			continue
		}
		err.Trackers++
		switch outcome, asked := t.announces[tracker]; {
		case !asked:
		case outcome == nil:
			err.Answered++
		case errors.As(outcome, new(*refusedError)):
			err.Refused++
		}
	}
	return err
}
