package torrent

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// announce is what a torrent tells a tracker.
type announce struct {
	hash       Hash
	peerID     [20]byte
	port       uint16
	event      string // "started", "stopped" or empty
	left       int64
	uploaded   int64
	downloaded int64
}

// trackerAnswer is a tracker's peers and when to ask it again.
type trackerAnswer struct {
	peers    []netip.AddrPort
	interval time.Duration
}

// trackerTimeout bounds one announce, retries of a UDP tracker included.
const trackerTimeout = 30 * time.Second

// errTrackerUDP reports a UDP tracker over a route without UDP.
var errTrackerUDP = errors.New("UDP trackers are off: the route carries no UDP")

// announceTo asks the tracker at raw for peers over route. A UDP tracker
// needs udp, which is false over a proxy without UDP ASSOCIATE.
func announceTo(ctx context.Context, route Route, raw string, a announce, udp bool) (trackerAnswer, error) {
	ctx, cancel := context.WithTimeout(ctx, trackerTimeout)
	defer cancel()
	u, err := url.Parse(raw)
	if err != nil {
		return trackerAnswer{}, err
	}
	switch u.Scheme {
	case "http", "https":
		return announceHTTP(ctx, route, u, a)
	case "udp":
		if !udp {
			return trackerAnswer{}, errTrackerUDP
		}
		return announceUDP(ctx, route, u.Host, a)
	}
	return trackerAnswer{}, fmt.Errorf("unsupported tracker scheme %q", u.Scheme)
}

// routeClient is an HTTP client every connection of which goes through
// route; it never uses the environment's proxy.
func routeClient(route Route) *http.Client {
	return &http.Client{Transport: &http.Transport{
		Proxy:               nil,
		DialContext:         func(ctx context.Context, _, address string) (net.Conn, error) { return route.Dial(ctx, address) },
		TLSHandshakeTimeout: 15 * time.Second,
		DisableKeepAlives:   true,
	}}
}

func announceHTTP(ctx context.Context, route Route, u *url.URL, a announce) (trackerAnswer, error) {
	query := u.RawQuery
	if query != "" {
		query += "&"
	}
	query += "info_hash=" + escapeBytes(a.hash[:]) + "&peer_id=" + escapeBytes(a.peerID[:]) +
		"&port=" + strconv.Itoa(int(a.port)) + "&uploaded=" + strconv.FormatInt(a.uploaded, 10) +
		"&downloaded=" + strconv.FormatInt(a.downloaded, 10) + "&left=" + strconv.FormatInt(a.left, 10) +
		"&compact=1&numwant=50"
	if a.event != "" {
		query += "&event=" + a.event
	}
	target := *u
	target.RawQuery = query
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return trackerAnswer{}, err
	}
	client := routeClient(route)
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return trackerAnswer{}, fmt.Errorf("the tracker did not answer: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return trackerAnswer{}, fmt.Errorf("the tracker answered HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxTrackerAnswer))
	if err != nil {
		return trackerAnswer{}, err
	}
	return parseHTTPAnswer(body)
}

// maxTrackerAnswer bounds what an HTTP tracker's answer is read of: 50
// peers take 300 bytes compact and a few kilobytes as dictionaries, and a
// longer answer is cut, then refused as malformed bencode.
const maxTrackerAnswer = 1 << 20

// maxTrackerInterval bounds the interval a tracker asks for, in seconds,
// before it is a time.Duration, which a larger one would overflow; the
// announces wait 30 minutes at most anyway.
const maxTrackerInterval = 24 * 60 * 60

// parseHTTPAnswer reads an HTTP tracker's bencoded answer.
func parseHTTPAnswer(body []byte) (trackerAnswer, error) {
	d, _, err := decodeDict(body)
	if err != nil {
		return trackerAnswer{}, err
	}
	if reason := str(d, "failure reason"); reason != "" {
		return trackerAnswer{}, &refusedError{reason: printable([]byte(reason))}
	}
	answer := trackerAnswer{interval: 30 * time.Minute}
	if interval, ok := num(d, "interval"); ok && interval > 0 {
		answer.interval = time.Duration(min(interval, maxTrackerInterval)) * time.Second
	}
	switch peers := d["peers"].(type) {
	case string:
		answer.peers = compactPeers([]byte(peers), 4)
	case []any:
		for _, entry := range peers {
			p, _ := entry.(map[string]any)
			ip, err := netip.ParseAddr(str(p, "ip"))
			port, _ := num(p, "port")
			if err == nil && port > 0 && port < 65536 {
				answer.peers = append(answer.peers, netip.AddrPortFrom(ip.Unmap(), uint16(port)))
			}
		}
	}
	answer.peers = append(answer.peers, compactPeers([]byte(str(d, "peers6")), 16)...)
	return answer, nil
}

// compactPeers reads peers as trackers and DHT nodes list them: an
// address of size bytes, then a port.
func compactPeers(data []byte, size int) []netip.AddrPort {
	var peers []netip.AddrPort
	for len(data) >= size+2 {
		ip, _ := netip.AddrFromSlice(data[:size])
		port := binary.BigEndian.Uint16(data[size:])
		if port != 0 {
			peers = append(peers, netip.AddrPortFrom(ip.Unmap(), port))
		}
		data = data[size+2:]
	}
	return peers
}

// The UDP tracker protocol (BEP 15).
const (
	udpProtocolID     = 0x41727101980
	udpActionConnect  = 0
	udpActionAnnounce = 1
	udpActionError    = 3
)

func announceUDP(ctx context.Context, route Route, hostport string, a announce) (trackerAnswer, error) {
	conn, err := route.ListenPacket(ctx)
	if err != nil {
		return trackerAnswer{}, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.SetReadDeadline(time.Now()) })
	defer stop()
	connect := make([]byte, 16)
	binary.BigEndian.PutUint64(connect, udpProtocolID)
	binary.BigEndian.PutUint32(connect[8:], udpActionConnect)
	reply, err := udpRoundTrip(ctx, conn, hostport, connect, 16)
	if err != nil {
		return trackerAnswer{}, err
	}
	request := make([]byte, 98)
	copy(request, reply[8:16])
	binary.BigEndian.PutUint32(request[8:], udpActionAnnounce)
	copy(request[16:], a.hash[:])
	copy(request[36:], a.peerID[:])
	binary.BigEndian.PutUint64(request[56:], uint64(a.downloaded))
	binary.BigEndian.PutUint64(request[64:], uint64(max(a.left, 0)))
	binary.BigEndian.PutUint64(request[72:], uint64(a.uploaded))
	event := map[string]uint32{"": 0, "completed": 1, "started": 2, "stopped": 3}[a.event]
	binary.BigEndian.PutUint32(request[80:], event)
	_, _ = rand.Read(request[88:92])
	binary.BigEndian.PutUint32(request[92:], 50)
	binary.BigEndian.PutUint16(request[96:], a.port)
	reply, err = udpRoundTrip(ctx, conn, hostport, request, 20)
	if err != nil {
		return trackerAnswer{}, err
	}
	answer := trackerAnswer{interval: time.Duration(binary.BigEndian.Uint32(reply[8:])) * time.Second}
	if answer.interval <= 0 {
		answer.interval = 30 * time.Minute
	}
	answer.peers = compactPeers(reply[20:], 4)
	return answer, nil
}

// udpRoundTrip sends request with a fresh transaction ID, at bytes 12 to
// 16, and waits for its reply of at least size bytes, trying again three
// times.
func udpRoundTrip(ctx context.Context, conn PacketConn, hostport string, request []byte, size int) ([]byte, error) {
	_, _ = rand.Read(request[12:16])
	action := binary.BigEndian.Uint32(request[8:])
	buffer := make([]byte, 2048)
	for try := range 3 {
		if err := conn.WriteTo(request, hostport); err != nil {
			return nil, err
		}
		deadline := time.Now().Add(time.Duration(5<<try) * time.Second)
		if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		_ = conn.SetReadDeadline(deadline)
		for {
			n, _, err := conn.ReadFrom(buffer)
			if err != nil {
				if ctx.Err() != nil {
					return nil, fmt.Errorf("the tracker did not answer: %w", ctx.Err())
				}
				var timeout net.Error
				if errors.As(err, &timeout) && timeout.Timeout() {
					break
				}
				return nil, err
			}
			if n < 8 || [4]byte(buffer[4:8]) != [4]byte(request[12:16]) {
				continue
			}
			switch got := binary.BigEndian.Uint32(buffer); {
			case got == udpActionError:
				return nil, &refusedError{reason: printable(buffer[8:n])}
			case got != action || n < size:
				continue
			}
			return append([]byte(nil), buffer[:n]...), nil
		}
	}
	return nil, errors.New("the tracker did not answer")
}

// refusedError reports a tracker that refused an announce, with the reason
// it gave (BEP 3's failure reason, BEP 15's error message), as it gave it.
type refusedError struct {
	reason string
}

func (e *refusedError) Error() string {
	if e.reason == "" {
		return "the tracker refused the announce"
	}
	return "the tracker refused the announce: " + e.reason
}

// maxReason bounds what a tracker's reason for a refusal is kept of.
const maxReason = 200

// printable is a tracker's message as it can be logged: its printable
// ASCII, maxReason bytes at most.
func printable(message []byte) string {
	var b strings.Builder
	for _, c := range message {
		if b.Len() >= maxReason {
			break
		}
		if c >= ' ' && c <= '~' {
			b.WriteByte(c)
		}
	}
	return strings.TrimSpace(b.String())
}

// escapeBytes percent-encodes the bytes of an info hash or a peer ID for
// an announce's query, every byte but the unreserved characters of RFC
// 3986, as BEP 3's clients do. url.QueryEscape writes a space as "+",
// which trackers that read the query as raw bytes take for itself: the
// hash or the ID is then the wrong one, and the announce is refused.
func escapeBytes(raw []byte) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for _, c := range raw {
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '-', c == '.', c == '_', c == '~':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}

// announcePort is the port told to trackers. Polyfin connects out only,
// as no peer can reach it through a proxy, but trackers want a port all
// the same: 0 is refused as invalid, and many refuse the 6881 to 6889 of
// the first clients, which networks used to throttle. A port of the
// dynamic range (RFC 6335), drawn from the peer ID's random part, is what
// clients with a random port send, and stays the same for the run.
func announcePort(peerID [20]byte) uint16 {
	return 49152 + binary.BigEndian.Uint16(peerID[18:])%16384
}

// trackerHost is a tracker's host, as logs name it: its URL's path and
// query may hold a passkey.
func trackerHost(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Host
	}
	return ""
}
