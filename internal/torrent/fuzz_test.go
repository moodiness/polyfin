package torrent

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net"
	"net/netip"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// The fuzz tests feed Polyfin's parsers what other programs send: peers,
// trackers, DHT nodes, the SOCKS5 proxy and addons, and the WireGuard
// configurations providers hand out. None may panic, and what they keep
// stays within the limits set. The inputs that made one fail are in
// testdata/fuzz, run as regular tests.

// fuzzTorrent is a torrent of two files and three pieces, each of one
// block; large, of two pieces, of two blocks and one. Its bytes are always
// the same, so that the corpus keeps matching it.
func fuzzTorrent(large bool) *torrenttest.Torrent {
	size, pieceLength := 2<<10+300, 1<<10
	if large {
		size, pieceLength = 40<<10+17, 32<<10
	}
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i*7 + i>>8)
	}
	return torrenttest.Build("A Movie", pieceLength, torrenttest.File{Name: "a.mkv", Data: data[:size/4]},
		torrenttest.File{Name: "b.mkv", Data: data[size/4:]})
}

// wire is a peer wire message.
func wire(id byte, payload ...[]byte) []byte {
	length := 1
	for _, part := range payload {
		length += len(part)
	}
	message := binary.BigEndian.AppendUint32(nil, uint32(length))
	message = append(message, id)
	for _, part := range payload {
		message = append(message, part...)
	}
	return message
}

// extendedMessage is a message of the extension protocol, with the bytes
// after its dictionary.
func extendedMessage(id byte, d map[string]any, after []byte) []byte {
	return wire(msgExtended, []byte{id}, encode(d), after)
}

func u32(values ...uint32) []byte {
	var b []byte
	for _, v := range values {
		b = binary.BigEndian.AppendUint32(b, v)
	}
	return b
}

// handshakeAnswer is a peer's handshake for hash, with the extension
// protocol.
func handshakeAnswer(hash [20]byte) []byte {
	answer := append([]byte{byte(len(protocolName))}, protocolName...)
	answer = append(answer, 0, 0, 0, 0, 0, 0x10, 0, 0)
	answer = append(answer, hash[:]...)
	return append(answer, "-XX0001-abcdefghijkl"...)
}

// streamConn is a connection on which the other side sends data, then
// closes; what Polyfin sends is dropped.
type streamConn struct {
	data []byte
	// panics has a read past data panic, rather than end.
	panics bool
	mu     sync.Mutex
	closed bool
}

func (c *streamConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, net.ErrClosed
	}
	if len(c.data) == 0 {
		if c.panics {
			panic("a read past the connection's data")
		}
		return 0, io.EOF
	}
	n := copy(p, c.data)
	c.data = c.data[n:]
	return n, nil
}

func (c *streamConn) Write(p []byte) (int, error) { return len(p), nil }

func (c *streamConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

func (c *streamConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 50000}
}
func (c *streamConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 6881}
}

func (c *streamConn) SetDeadline(time.Time) error      { return nil }
func (c *streamConn) SetReadDeadline(time.Time) error  { return nil }
func (c *streamConn) SetWriteDeadline(time.Time) error { return nil }

// datagramConn is a UDP socket that receives datagrams, then times out.
// With echo, each takes the transaction ID of the last request, at bytes
// 12 to 16, into its bytes 4 to 8, as a UDP tracker answers.
type datagramConn struct {
	datagrams [][]byte
	from      netip.AddrPort
	echo      bool
	last      []byte
	writes    int
}

func (c *datagramConn) WriteTo(p []byte, _ string) error {
	c.last = append(c.last[:0], p...)
	c.writes++
	return nil
}

func (c *datagramConn) WriteToAddrPort(p []byte, to netip.AddrPort) error {
	return c.WriteTo(p, to.String())
}

func (c *datagramConn) ReadFrom(p []byte) (int, netip.AddrPort, error) {
	if len(c.datagrams) == 0 {
		return 0, netip.AddrPort{}, timeoutError{}
	}
	datagram := c.datagrams[0]
	c.datagrams = c.datagrams[1:]
	n := copy(p, datagram)
	if c.echo && n >= 8 && len(c.last) >= 16 {
		copy(p[4:8], c.last[12:16])
	}
	return n, c.from, nil
}

func (c *datagramConn) SetReadDeadline(time.Time) error { return nil }
func (c *datagramConn) Close() error                    { return nil }

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// packetRoute is a route whose UDP socket is conn, and that dials nothing.
type packetRoute struct{ conn PacketConn }

func (r packetRoute) Dial(context.Context, string) (net.Conn, error) { return nil, net.ErrClosed }
func (r packetRoute) ListenPacket(context.Context) (PacketConn, error) {
	return r.conn, nil
}
func (r packetRoute) Check(context.Context) error { return nil }
func (r packetRoute) String() string              { return "fuzz" }
func (r packetRoute) Close() error                { return nil }

// frames splits data into datagrams, each after its length on two bytes;
// what is left is the last one.
func frames(data []byte) [][]byte {
	var datagrams [][]byte
	for len(data) >= 2 {
		n := int(binary.BigEndian.Uint16(data))
		data = data[2:]
		if n > len(data) {
			break
		}
		datagrams = append(datagrams, data[:n])
		data = data[n:]
	}
	if len(data) > 0 {
		datagrams = append(datagrams, data)
	}
	return datagrams
}

func framed(datagrams ...[]byte) []byte {
	var b []byte
	for _, d := range datagrams {
		b = binary.BigEndian.AppendUint16(b, uint16(len(d)))
		b = append(b, d...)
	}
	return b
}

func FuzzDecode(f *testing.F) {
	for _, seed := range []string{"i42e", "i-3e", "i0e", "4:spam", "0:", "le", "de", "l4:spami42ee",
		"d3:cow3:moo4:spaml1:a1:bee", "d1:ad1:bi1eee", strings.Repeat("l", 33) + strings.Repeat("e", 33),
		string(fuzzTorrent(false).Info)} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		value, rest, err := decode(data)
		if err != nil {
			return
		}
		if len(rest) >= len(data) {
			t.Fatalf("decoding read nothing of %q", data)
		}
		// What was read encodes back to the same value.
		again, left, err := decode(encode(value))
		if err != nil || len(left) != 0 || !reflect.DeepEqual(again, value) {
			t.Fatalf("%q decodes to %#v, which encodes to %q", data, value, encode(value))
		}
	})
}

// checkInfoBounds checks what an info dictionary that was read tells:
// files end to end, within the pieces it has.
func checkInfoBounds(t *testing.T, info Info) {
	t.Helper()
	if info.PieceLength <= 0 || info.PieceLength > maxPieceLength || len(info.Pieces) == 0 {
		t.Fatalf("piece length %d, %d pieces", info.PieceLength, len(info.Pieces))
	}
	var offset int64
	for _, file := range info.Files {
		if file.Length < 0 || file.Offset != offset || file.Length > math.MaxInt64-offset {
			t.Fatalf("file %+v after %d bytes", file, offset)
		}
		offset += file.Length
	}
	if offset != info.Length {
		t.Fatalf("files of %d bytes, a torrent of %d", offset, info.Length)
	}
	last := len(info.Pieces) - 1
	if size := info.pieceSize(last); size <= 0 || size > info.PieceLength {
		t.Fatalf("a last piece of %d bytes, of %d at most", size, info.PieceLength)
	}
	if file, err := info.File(Spec{File: -1}); err == nil && file.Length > 0 {
		if index := (file.Offset + file.Length - 1) / info.PieceLength; index < 0 || index > int64(last) {
			t.Fatalf("the file's end is in piece %d of %d", index, len(info.Pieces))
		}
	}
}

func FuzzParseInfo(f *testing.F) {
	f.Add(fuzzTorrent(false).Info)
	f.Add(fuzzTorrent(true).Info)
	f.Add(torrenttest.Build("One", 16<<10, torrenttest.File{Name: "a.mkv", Data: make([]byte, 20<<10)}).Info)
	f.Add(encode(map[string]any{"name": "Padded", "piece length": 16, "pieces": strings.Repeat("x", 40),
		"files": []any{map[string]any{"length": 10, "path": []any{"a"}}, map[string]any{"length": 6, "path": []any{"pad"}, "attr": "p"},
			map[string]any{"length": 16, "path": []any{"b", "c"}}}}))
	f.Fuzz(func(t *testing.T, data []byte) {
		info, err := parseInfo(data)
		if err != nil {
			return
		}
		checkInfoBounds(t, info)
	})
}

// fuzzSwarm is a torrent a fuzzed peer shares: its info, and the bytes of
// its first two pieces.
type fuzzSwarm struct {
	tor  *torrenttest.Torrent
	info Info
}

func newFuzzSwarm(tb testing.TB, large bool) fuzzSwarm {
	tor := fuzzTorrent(large)
	info, err := parseInfo(tor.Info)
	if err != nil {
		tb.Fatal(err)
	}
	return fuzzSwarm{tor: tor, info: info}
}

func (s fuzzSwarm) piece(index int) []byte {
	return s.tor.Data[index*s.tor.PieceLength : min((index+1)*s.tor.PieceLength, len(s.tor.Data))]
}

// messages are what a peer that has the first two pieces sends: its
// bitfield, the pieces' blocks, and the other messages a peer sends.
func (s fuzzSwarm) messages() []byte {
	b := wire(msgBitfield, []byte{0xc0})
	b = append(b, wire(msgUnchoke)...)
	for index := range 2 {
		piece := s.piece(index)
		for offset := 0; offset < len(piece); offset += blockSize {
			b = append(b, wire(msgPiece, u32(uint32(index), uint32(offset)), piece[offset:min(offset+blockSize, len(piece))])...)
		}
	}
	second := uint32(len(s.piece(1)))
	return bytes.Join([][]byte{b,
		wire(msgHave, u32(1)),
		wire(msgInterested),
		wire(msgRequest, u32(1, 0, second)),
		wire(msgCancel, u32(1, 0, second)),
		wire(msgPort, []byte{0x1a, 0xe1}),
		{0, 0, 0, 0},
		extendedMessage(0, map[string]any{"m": map[string]any{"ut_metadata": 1, "ut_pex": 2}, "metadata_size": len(s.tor.Info), "v": "Peer"}, nil),
		wire(msgExtended, []byte{extPEX}, pexPayload(append(addresses(0, 2), netip.MustParseAddrPort("[2001:db8::1]:6881")), pexUTP, addresses(5, 1)...)),
		extendedMessage(extMetadata, map[string]any{"msg_type": 0, "piece": 0}, nil),
		extendedMessage(extMetadata, map[string]any{"msg_type": 0, "piece": 1}, nil),
		wire(msgNotInterested),
		wire(msgChoke),
	}, nil)
}

// metadata is what a peer that sends the torrent's info dictionary sends.
func (s fuzzSwarm) metadata() []byte {
	return bytes.Join([][]byte{
		extendedMessage(0, map[string]any{"m": map[string]any{"ut_metadata": 3}, "metadata_size": len(s.tor.Info)}, nil),
		extendedMessage(extMetadata, map[string]any{"msg_type": 1, "piece": 0, "total_size": len(s.tor.Info)}, s.tor.Info),
		extendedMessage(extMetadata, map[string]any{"msg_type": 2, "piece": 0}, nil),
	}, nil)
}

// fuzzEngine is an engine that shares, whose store holds the second piece
// of each swarm, which peers may ask for. It logs to logger, when set.
func fuzzEngine(tb testing.TB, logger *slog.Logger, swarms ...fuzzSwarm) *Engine {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	e, err := New(Config{Settings: func() Settings { return Settings{Enabled: true, UploadMbps: 1000} }, Dir: tb.TempDir(),
		Budget: func() int64 { return 64 << 20 }, Logger: logger})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(e.Close)
	for _, s := range swarms {
		if err := e.store.put(pieceKey{s.tor.Hash, 1}, s.piece(1)); err != nil {
			tb.Fatal(err)
		}
	}
	return e
}

// newFuzzPeer is a peer of the swarm that sends stream, connected to a
// torrent that has the second piece; the mode is FuzzPeerMessages's. The
// error is the handshake's.
func newFuzzPeer(e *Engine, s fuzzSwarm, mode byte, stream []byte) (*torrent, *peer, error) {
	tr := newTorrent(e, Spec{Hash: s.tor.Hash, File: -1}, nil, false)
	tr.files = s.info.Files
	if mode&1 != 0 {
		known := s.info
		tr.info, tr.infoData = &known, s.tor.Info
		tr.have = make([]bool, len(known.Pieces))
		tr.have[1] = true
	}
	conn := &streamConn{data: stream}
	if mode&2 != 0 {
		if _, err := handshake(conn, s.tor.Hash, e.peerID, false); err != nil {
			return tr, nil, err
		}
	}
	address := netip.MustParseAddrPort("127.0.0.1:6881")
	p := newPeer(tr, conn, address)
	p.unchoked = mode&4 != 0
	tr.peers[address] = p
	return tr, p, nil
}

// queuedBytes are the bytes waiting for a peer.
func queuedBytes(p *peer) int {
	p.outMu.Lock()
	defer p.outMu.Unlock()
	n := 0
	for _, message := range p.outbox {
		n += len(message)
	}
	return n
}

// FuzzPeerMessages reads what a peer sends. The mode's bits tell whether
// the torrent's info is known (1), whether the peer's handshake comes
// first (2), whether Polyfin unchoked the peer (4), and whether the
// torrent is the one of large pieces, of several blocks (8).
func FuzzPeerMessages(f *testing.F) {
	swarms := [2]fuzzSwarm{newFuzzSwarm(f, false), newFuzzSwarm(f, true)}
	e := fuzzEngine(f, nil, swarms[:]...)
	for i, s := range swarms {
		large := byte(i * 8)
		f.Add(large|1|4, s.messages())
		f.Add(large|1|2|4, append(handshakeAnswer(s.tor.Hash), s.messages()...))
		f.Add(large, append(s.metadata(), s.messages()...))
		f.Add(large|2, append(handshakeAnswer(s.tor.Hash), s.metadata()...))
	}
	f.Add(byte(1), wire(msgBitfield, bytes.Repeat([]byte{0xff}, 100)))

	f.Fuzz(func(t *testing.T, mode byte, stream []byte) {
		s := swarms[0]
		if mode&8 != 0 {
			s = swarms[1]
		}
		tr, p, err := newFuzzPeer(e, s, mode, stream)
		defer tr.cancel()
		if err != nil {
			return
		}
		if err := p.read(); err == nil {
			t.Fatal("the connection ended without an error")
		}
		if queued := queuedBytes(p); queued > maxOutbox {
			t.Fatalf("%d bytes queued for a peer, %d at most", queued, maxOutbox)
		}
		tr.mu.Lock()
		defer tr.mu.Unlock()
		if len(p.has) > maxPieces {
			t.Fatalf("the peer has %d pieces, a torrent %d at most", len(p.has), maxPieces)
		}
		if tr.info != nil && len(tr.have) != len(tr.info.Pieces) {
			t.Fatalf("Polyfin has %d pieces of %d", len(tr.have), len(tr.info.Pieces))
		}
	})
}

// The limits on what a peer sends end its connection with an error, or
// keep what Polyfin queues for it bounded.
func TestPeerLimits(t *testing.T) {
	t.Parallel()
	s := newFuzzSwarm(t, false)
	e := fuzzEngine(t, nil, s)
	read := func(t *testing.T, mode byte, stream []byte, setup func(*torrent)) (*torrent, *peer, error) {
		t.Helper()
		tr, p, err := newFuzzPeer(e, s, mode, stream)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(tr.cancel)
		if setup != nil {
			setup(tr)
		}
		return tr, p, p.read()
	}
	metadataRequests := func(n int) []byte {
		stream := extendedMessage(0, map[string]any{"m": map[string]any{"ut_metadata": 1}}, nil)
		for i := range n {
			stream = append(stream, extendedMessage(extMetadata, map[string]any{"msg_type": 0, "piece": i % 4}, nil)...)
		}
		return stream
	}

	t.Run("a bitfield past the pieces a torrent may have", func(t *testing.T) {
		_, p, err := read(t, 0, wire(msgBitfield, make([]byte, (maxPieces+7)/8+1)), nil)
		if err == nil || !strings.Contains(err.Error(), "bitfield") || p.has != nil {
			t.Fatalf("got %v, with %d pieces", err, len(p.has))
		}
	})
	t.Run("a bitfield past the torrent's pieces", func(t *testing.T) {
		if _, _, err := read(t, 1, wire(msgBitfield, []byte{0xff, 0xff}), nil); err == nil || !strings.Contains(err.Error(), "bitfield") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("a have past the pieces a torrent may have", func(t *testing.T) {
		_, p, err := read(t, 0, append(wire(msgHave, u32(maxPieces-1)), wire(msgHave, u32(maxPieces))...), nil)
		if err == nil || !strings.Contains(err.Error(), "have") || len(p.has) != maxPieces {
			t.Fatalf("got %v, with %d pieces", err, len(p.has))
		}
	})
	t.Run("metadata is answered while the peer reads", func(t *testing.T) {
		_, p, err := read(t, 1|4, metadataRequests(400), func(tr *torrent) { tr.infoData = make([]byte, 4*metadataBlock) })
		if !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		answered := 0
		for _, message := range p.outbox {
			if len(message) > metadataBlock {
				answered++
			}
		}
		if answered != pipeline+1 || len(p.outbox) != 400 {
			t.Fatalf("%d pieces of metadata sent, %d messages queued", answered, len(p.outbox))
		}
	})
	t.Run("a peer that does not read", func(t *testing.T) {
		requests := maxOutbox / len(extendedMessage(extMetadata, map[string]any{"msg_type": 0, "piece": 0}, nil))
		_, p, err := read(t, 1|4, metadataRequests(requests+10), nil)
		if !errors.Is(err, net.ErrClosed) || queuedBytes(p) > maxOutbox {
			t.Fatalf("got %v, with %d bytes queued", err, queuedBytes(p))
		}
		select {
		case <-p.closed:
		default:
			t.Fatal("the peer is still connected")
		}
	})
}

// lockedBuffer is a log written by several goroutines.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// fakeDialRoute is a route whose connections are conn.
type fakeDialRoute struct {
	packetRoute
	conn net.Conn
}

func (r fakeDialRoute) Dial(context.Context, string) (net.Conn, error) { return r.conn, nil }

// A panic while a peer's connection is read ends that connection only: it
// is logged once, the peer leaves, and the torrent goes on.
func TestAPanicEndsOnlyItsConnection(t *testing.T) {
	t.Parallel()
	s := newFuzzSwarm(t, false)
	var log lockedBuffer
	e := fuzzEngine(t, slog.New(slog.NewTextHandler(&log, nil)), s)
	conn := &streamConn{data: append(handshakeAnswer(s.tor.Hash), wire(msgUnchoke)...), panics: true}
	tr := newTorrent(e, Spec{Hash: s.tor.Hash, File: -1}, fakeDialRoute{conn: conn}, false)
	defer tr.cancel()
	address := netip.MustParseAddrPort("127.0.0.1:6881")
	tr.mu.Lock()
	tr.dialing++
	tr.mu.Unlock()
	tr.dial(address)

	tr.mu.Lock()
	peers, dialing := len(tr.peers), tr.dialing
	tr.mu.Unlock()
	if peers != 0 || dialing != 0 {
		t.Fatalf("%d peers, %d dialing after the panic", peers, dialing)
	}
	if got := strings.Count(log.String(), "ended on a panic"); got != 1 || !strings.Contains(log.String(), "a read past the connection") {
		t.Fatalf("logged %d times: %s", got, log.String())
	}
	// The torrent still connects to its other peers.
	tr.addPeers([]netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:6882")})
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if tr.err != nil || len(tr.queue) != 1 {
		t.Fatalf("the torrent stopped: %v, %d peers queued", tr.err, len(tr.queue))
	}
}

func FuzzHTTPTracker(f *testing.F) {
	f.Add([]byte("d8:intervali1800e5:peers12:\x7f\x00\x00\x01\x1a\xe1\x0a\x00\x00\x02\x1a\xe2e"))
	f.Add([]byte("d8:intervali60e5:peersld2:ip9:127.0.0.14:porti6881eed2:ip3:::14:porti1eeee"))
	f.Add([]byte("d6:peers618:\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01\x1a\xe1e"))
	f.Add([]byte("d14:failure reason6:refusee"))
	f.Fuzz(func(t *testing.T, body []byte) {
		answer, err := parseHTTPAnswer(body)
		if err != nil {
			return
		}
		if answer.interval <= 0 {
			t.Fatalf("an interval of %v", answer.interval)
		}
		for _, peer := range answer.peers {
			if !peer.IsValid() || peer.Port() == 0 {
				t.Fatalf("a peer at %v", peer)
			}
		}
	})
}

func FuzzUDPTracker(f *testing.F) {
	connect := append(u32(udpActionConnect, 0), 1, 2, 3, 4, 5, 6, 7, 8)
	reply := append(u32(udpActionAnnounce, 0, 60, 0, 1), 127, 0, 0, 1, 0x1a, 0xe1)
	f.Add(connect, reply, true)
	f.Add(connect, append(u32(udpActionError, 0), "refused"...), true)
	f.Add(connect, u32(udpActionAnnounce, 0, 0, 0, 0), false)
	f.Fuzz(func(t *testing.T, connect, reply []byte, echo bool) {
		conn := &datagramConn{datagrams: [][]byte{connect, reply}, echo: echo}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		answer, err := announceUDP(ctx, packetRoute{conn}, "tracker.invalid:6969", announce{port: 51413, left: 1})
		if err != nil {
			return
		}
		if answer.interval <= 0 {
			t.Fatalf("an interval of %v", answer.interval)
		}
		for _, peer := range answer.peers {
			if !peer.IsValid() || peer.Port() == 0 {
				t.Fatalf("a peer at %v", peer)
			}
		}
	})
}

func FuzzDHTLookup(f *testing.F) {
	hash := fuzzTorrent(false).Hash
	node := string(bytes.Repeat([]byte{1}, 20)) + "\x7f\x00\x00\x02\x1a\xe1"
	first := encode(map[string]any{"t": "\x00\x01", "y": "r", "r": map[string]any{"id": string(make([]byte, 20)), "token": "tk",
		"values": []any{"\x7f\x00\x00\x01\x1a\xe1", "\x7f\x00\x00\x03\x1a\xe2"}, "nodes": node}})
	second := encode(map[string]any{"t": "\x00\x02", "y": "r", "r": map[string]any{"id": node[:20], "nodes": node + node}})
	f.Add(framed(first, second))
	f.Add(framed(encode(map[string]any{"t": "\x00\x01", "y": "e", "e": []any{201, "error"}})))
	f.Fuzz(func(t *testing.T, stream []byte) {
		conn := &datagramConn{datagrams: frames(stream), from: netip.MustParseAddrPort("127.0.0.1:6881")}
		seen := map[netip.AddrPort]bool{}
		closest, err := lookupPeers(context.Background(), conn, hash, []string{"127.0.0.1:6881"}, func(peers []netip.AddrPort) {
			for _, peer := range peers {
				if !peer.IsValid() || peer.Port() == 0 || seen[peer] {
					t.Fatalf("a peer at %v, or found twice", peer)
				}
				seen[peer] = true
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		if conn.writes > dhtQueries {
			t.Fatalf("%d queries, %d at most", conn.writes, dhtQueries)
		}
		if len(closest) > dhtKept {
			t.Fatalf("%d nodes kept, %d at most", len(closest), dhtKept)
		}
	})
}

func FuzzSOCKS5Reply(f *testing.F) {
	accepted := []byte{socksVersion, socksPassword, 1, 0}
	f.Add(append(accepted, socksVersion, 0, 0, socksIPv4, 127, 0, 0, 1, 0x1a, 0xe1))
	f.Add(append(accepted, append([]byte{socksVersion, 0, 0, socksDomain, 5}, "proxy\x04\x38"...)...))
	f.Add(append(accepted, append([]byte{socksVersion, 0, 0, socksIPv6}, make([]byte, 18)...)...))
	f.Add(append(accepted, socksVersion, socksUnsupported, 0))
	f.Add([]byte{socksVersion, socksNoMethod})
	f.Fuzz(func(t *testing.T, replies []byte) {
		r := &proxyRoute{address: "127.0.0.1:1080", user: "someone", password: "secret"}
		conn := &streamConn{data: replies}
		if err := r.authenticate(conn); err != nil {
			return
		}
		bound, err := command(conn, socksAssociate, "0.0.0.0:0")
		if err != nil {
			return
		}
		// A name of 255 bytes at most, or an address, and a port, which
		// ListenPacket reads as an address or refuses.
		if len(bound) > 255+len("[]:65535") {
			t.Fatalf("the proxy bound %d bytes", len(bound))
		}
		_, _ = netip.ParseAddrPort(bound)
	})
}

func FuzzSOCKS5Datagram(f *testing.F) {
	f.Add(append([]byte{0, 0, 0, socksIPv4, 127, 0, 0, 1, 0x1a, 0xe1}, "payload"...))
	f.Add(append([]byte{0, 0, 0, socksIPv6}, make([]byte, 18)...))
	f.Add(append([]byte{0, 0, 0, socksDomain, 4}, "node\x1a\xe1"...))
	f.Add([]byte{0, 0, 1, socksIPv4})
	f.Fuzz(func(t *testing.T, datagram []byte) {
		p := make([]byte, 64)
		n, from, ok := readSocksDatagram(datagram, p)
		if !ok {
			return
		}
		if n > len(p) || n > len(datagram) || !from.IsValid() {
			t.Fatalf("%d bytes from %v", n, from)
		}
	})
}

// FuzzNewSpec reads an addon's torrent stream: every one the stream lists
// as a torrent's file plays, with what its sources may add bounded.
func FuzzNewSpec(f *testing.F) {
	hash := strings.Repeat("ab", 20)
	f.Add(`{"infoHash":"` + hash + `","fileIdx":2,"sources":["tracker:udp://tracker.example:80","tracker:http://tracker.example/announce",` +
		`"tracker:ftp://nothing","dht:` + hash + `","dht:node.example:6881","other"]}`)
	f.Add(`{"infoHash":" ` + strings.ToUpper(hash) + `","sources":["dht:[::1]:6881","tracker:udp://tracker.example"]}`)
	f.Add(`{"infoHash":"` + hash + `","fileIdx":-1}`)
	f.Fuzz(func(t *testing.T, raw string) {
		var stream stremio.Stream
		if json.Unmarshal([]byte(raw), &stream) != nil {
			return
		}
		spec, err := NewSpec(stream.InfoHash, stream.FileIdx, stream.Sources)
		if err != nil {
			if stream.Torrent() {
				t.Fatalf("a torrent stream that does not play: %v", err)
			}
			return
		}
		if spec.File < -1 || len(spec.Trackers) > MaxTrackers || len(spec.Nodes) > maxNodes {
			t.Fatalf("got %+v", spec)
		}
		for _, tracker := range spec.Trackers {
			if !ValidTracker(tracker) {
				t.Fatalf("an invalid tracker %q", tracker)
			}
		}
		for _, node := range spec.Nodes {
			if !validNode(node) {
				t.Fatalf("an invalid DHT node %q", node)
			}
		}
	})
}

// FuzzWireGuardConfig checks a configuration that was read writes back as
// one that reads the same, and that a refusal tells its line and key
// without repeating a value.
func FuzzWireGuardConfig(f *testing.F) {
	key := "yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk="
	valid := "[Interface]\nPrivateKey = " + key + "\nAddress = 10.2.0.2/32, fd00::2\nDNS = 10.2.0.1, 2001:db8::53\nMTU = 1280\n" +
		"ListenPort = 51820\nPostUp = true\n\n[Peer]\nPublicKey = " + key + "\nPresharedKey = " + key + "\n" +
		"Endpoint = vpn.example.com:51820\nAllowedIPs = 0.0.0.0/0, ::/0\nPersistentKeepalive = 25\n"
	for _, seed := range []string{valid, strings.ReplaceAll(valid, "\n", "\r\n"), strings.ToUpper(valid),
		strings.Replace(valid, "vpn.example.com", "[2001:db8::1]", 1), strings.Replace(valid, "0.0.0.0/0", "::ffff:0.0.0.0/96", 1),
		"[Interface]\n# only a comment\n[Peer]\n", "[Interface\n", "Key = Value\n", "[Peer]\nEndpoint = a..b:1\n"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		c, err := ParseWireGuardConfig(text)
		if err != nil {
			var configErr *WireGuardConfigError
			if !errors.As(err, &configErr) || configErr.Line < 0 || configErr.Line > strings.Count(text, "\n")+1 || len(err.Error()) > 200 {
				t.Fatalf("%q is refused with %#v", text, err)
			}
			if strings.Contains(err.Error(), key) {
				t.Fatalf("the refusal of %q tells a key: %v", text, err)
			}
			return
		}
		if !c.ipv4().IsValid() || c.Endpoint == "" || len(c.AllowedIPs) == 0 {
			t.Fatalf("%q reads as %+v", text, c)
		}
		written := c.Marshal()
		again, err := ParseWireGuardConfig(written)
		if err != nil {
			t.Fatalf("%q writes as %q, refused: %v", text, written, err)
		}
		if !reflect.DeepEqual(again, c) || again.Marshal() != written {
			t.Fatalf("%q writes as %q, which reads as %+v rather than %+v", text, written, again, c)
		}
	})
}
