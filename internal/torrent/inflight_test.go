package torrent

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// stingyPeer is a peer of a torrent whose info dictionary it sends (BEP
// 9): it says it has every piece, unchokes, and answers each REQUEST with
// a block of its own bytes, but those at offset 0, which it never sends,
// so that no piece is ever whole. cycle chokes then unchokes whoever is
// connected, which has the engine forget what it asked.
type stingyPeer struct {
	listener net.Listener
	info     []byte
	hash     [20]byte
	pieces   int

	mu    sync.Mutex
	conns []*stingyConn
}

type stingyConn struct {
	conn net.Conn
	mu   sync.Mutex
}

func (c *stingyConn) send(id byte, parts ...[]byte) error {
	length := 1
	for _, part := range parts {
		length += len(part)
	}
	message := binary.BigEndian.AppendUint32(nil, uint32(length))
	message = append(message, id)
	for _, part := range parts {
		message = append(message, part...)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.conn.Write(message)
	return err
}

func startStingyPeer(tb testing.TB, info []byte, pieces int) *stingyPeer {
	tb.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatal(err)
	}
	p := &stingyPeer{listener: listener, info: info, hash: sha1.Sum(info), pieces: pieces}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			c := &stingyConn{conn: conn}
			p.mu.Lock()
			p.conns = append(p.conns, c)
			p.mu.Unlock()
			go p.serve(c)
		}
	}()
	tb.Cleanup(func() {
		listener.Close()
		p.mu.Lock()
		defer p.mu.Unlock()
		for _, c := range p.conns {
			c.conn.Close()
		}
	})
	return p
}

func (p *stingyPeer) address() netip.AddrPort {
	return netip.MustParseAddrPort(p.listener.Addr().String())
}

// cycle chokes and unchokes every connection.
func (p *stingyPeer) cycle() {
	p.mu.Lock()
	conns := slices.Clone(p.conns)
	p.mu.Unlock()
	for _, c := range conns {
		_ = c.send(msgChoke)
		_ = c.send(msgUnchoke)
	}
}

func (p *stingyPeer) serve(c *stingyConn) {
	defer c.conn.Close()
	var hello [68]byte
	if _, err := io.ReadFull(c.conn, hello[:]); err != nil || !bytes.Equal(hello[28:48], p.hash[:]) {
		return
	}
	reply := append([]byte{byte(len(protocolName))}, protocolName...)
	reply = append(reply, 0, 0, 0, 0, 0, 0x10, 0, 0)
	reply = append(reply, p.hash[:]...)
	reply = append(reply, "-XX0001-stingystingy"[:20]...)
	if _, err := c.conn.Write(reply); err != nil {
		return
	}
	_ = c.send(msgExtended, []byte{0}, encode(map[string]any{"m": map[string]any{"ut_metadata": 3}, "metadata_size": len(p.info)}))
	_ = c.send(msgBitfield, bytes.Repeat([]byte{0xff}, (p.pieces+7)/8))
	_ = c.send(msgUnchoke)
	filler := bytes.Repeat([]byte{0x5a}, blockSize)
	r := bufio.NewReader(c.conn)
	for {
		var header [4]byte
		if _, err := io.ReadFull(r, header[:]); err != nil {
			return
		}
		length := binary.BigEndian.Uint32(header[:])
		if length == 0 {
			continue
		}
		message := make([]byte, length)
		if _, err := io.ReadFull(r, message); err != nil {
			return
		}
		payload := message[1:]
		switch message[0] {
		case msgExtended:
			if len(payload) == 0 || payload[0] != 3 {
				continue
			}
			// A metadata request: msg_type 0 and its piece.
			piece := 0
			if i := bytes.Index(payload, []byte("5:piecei")); i >= 0 {
				end := bytes.IndexByte(payload[i+8:], 'e')
				piece, _ = strconv.Atoi(string(payload[i+8 : i+8+end]))
			}
			start := piece * metadataBlock
			if start >= len(p.info) {
				continue
			}
			end := min(start+metadataBlock, len(p.info))
			_ = c.send(msgExtended, []byte{extMetadata},
				encode(map[string]any{"msg_type": 1, "piece": piece, "total_size": len(p.info)}), p.info[start:end])
		case msgRequest:
			if len(payload) != 12 {
				continue
			}
			offset, n := binary.BigEndian.Uint32(payload[4:]), int(binary.BigEndian.Uint32(payload[8:]))
			if offset == 0 || n > len(filler) {
				// Never sent: the piece is never whole, never checked.
				continue
			}
			if c.send(msgPiece, payload[:8], filler[:n]) != nil {
				return
			}
		}
	}
}

// The pieces being downloaded are whole buffers in memory, which neither
// the Budget of the pieces on disk nor a read's window used to bound:
// each read at another offset, as a player's seek or a range request
// makes, left its pieces behind for as long as the torrent ran. Forty
// such reads of a torrent whose peer never completes a piece now hold no
// more than the engine's room, its Budget here, at any time, by the
// engine's own count; and once the reads are closed and the peer choked,
// only the pieces still wanted, those at the edges of the file, stay.
func TestSeeksLeaveNoMorePiecesInFlightThanTheRoom(t *testing.T) {
	t.Parallel()
	const (
		pieceLength = 256 << 10
		pieces      = 256
		budget      = 1 << 20
		seeks       = 40
	)
	// A torrent of one file of 64 MiB, whose pieces' hashes are anything:
	// no piece is ever whole, so none is ever checked.
	info := encode(map[string]any{"name": "movie.mkv", "piece length": pieceLength, "length": pieces * pieceLength,
		"pieces": strings.Repeat("0123456789abcdefghij", pieces)})
	stingy := startStingyPeer(t, info, pieces)
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", stingy.address())
	proxy := torrenttest.StartProxy(t, false, "", "")
	e := newEngine(t, proxied(proxy), budget)
	if room := e.inflightRoom(); room != budget {
		t.Fatalf("the engine's room is %d bytes, want its budget of %d", room, budget)
	}

	spec, err := NewSpec(Hash(stingy.hash).String(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	stop := e.Play(spec)
	defer stop()
	tr := torrentOf(e, spec.Hash)
	if tr == nil {
		t.Fatal("the torrent does not run")
	}
	// flying counts the pieces in flight and their bytes, which the
	// engine's own count must tell.
	flying := func(when string) (count int, held int64) {
		tr.mu.Lock()
		defer tr.mu.Unlock()
		for _, in := range tr.inflight {
			held += int64(len(in.data))
		}
		if own := e.inflightBytes.Load(); own != held {
			t.Fatalf("%s: the engine counts %d bytes in flight, the pieces hold %d", when, own, held)
		}
		return len(tr.inflight), held
	}
	// The read of a player that seeks to piece index, then gives up, as
	// when its range request is cancelled: the source cache opens a reader
	// for each range, and closes it.
	seek := func(index int) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		r, err := e.Open(ctx, spec)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer r.Close()
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = r.ReadAt(make([]byte, 64<<10), int64(index)*pieceLength)
		}()
		waitFor(t, "piece "+strconv.Itoa(index)+" to be asked for", func() bool {
			tr.mu.Lock()
			defer tr.mu.Unlock()
			return tr.inflight[index] != nil
		})
		cancel()
		<-done
	}
	// Seeks 5 pieces apart: past the window of each other.
	most := int64(0)
	for i := range seeks {
		index := 8 + 5*i
		seek(index)
		count, held := flying("after the seek to piece " + strconv.Itoa(index))
		if held > budget {
			t.Fatalf("after %d seeks, %d pieces in flight hold %d bytes, more than the room of %d", i+1, count, held, budget)
		}
		most = max(most, held)
	}
	tr.mu.Lock()
	reads := len(tr.reads)
	tr.mu.Unlock()
	if reads != 0 {
		t.Fatalf("%d reads still open", reads)
	}
	if most == 0 {
		t.Fatal("no piece was ever in flight")
	}
	// The peer chokes: it is asked for nothing, then again for what is
	// still wanted alone. What the last seek left goes.
	stingy.cycle()
	waitFor(t, "the pieces no read wants to be dropped", func() bool {
		tr.mu.Lock()
		defer tr.mu.Unlock()
		wanted := tr.wanted()
		for index := range tr.inflight {
			if !slices.Contains(wanted, index) {
				return false
			}
		}
		return true
	})
	count, held := flying("once the reads are closed")
	if count > 2*edgePieces || held > budget {
		t.Fatalf("no read open: %d pieces in flight holding %d bytes, want the file's %d edge pieces at most", count, held, 2*edgePieces)
	}
	t.Logf("budget %d bytes, %d seeks: %d bytes in flight at most, %d in %d pieces once the reads are closed", budget, seeks, most, held, count)
}

// roomPiece is the length of a roomSwarm's pieces: four blocks.
const roomPiece = 4 * blockSize

// roomSwarm is a torrent whose info is known, in an engine of a Budget
// the test chooses, whose reads and peers the test moves by hand: nothing
// of it runs by itself. data are the bytes of a block.
type roomSwarm struct {
	t    *torrent
	data func(b block) []byte
}

// roomEngine is an engine keeping budget bytes of pieces, which bounds
// the pieces in flight alike (see Engine.inflightRoom).
func roomEngine(tb testing.TB, budget int64) *Engine {
	tb.Helper()
	e, err := New(Config{Settings: func() Settings { return Settings{Enabled: true} }, Dir: tb.TempDir(),
		Budget: func() int64 { return budget }, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(e.Close)
	return e
}

// newRoomSwarm is a torrent of one file of pieces of roomPiece bytes.
func newRoomSwarm(tb testing.TB, e *Engine, name string, pieces int) roomSwarm {
	tb.Helper()
	return roomSwarmOf(tb, e, name, roomPiece, pieces)
}

// roomSwarmOf is a torrent of one file of pieces of length bytes. Its
// info is set by hand rather than read from an info dictionary, so that
// the pieces may be of any length, shorter than a block too.
func roomSwarmOf(tb testing.TB, e *Engine, name string, length, pieces int) roomSwarm {
	tb.Helper()
	data := torrenttest.Bytes(pieces * length)
	info := &Info{Name: name, PieceLength: int64(length), Pieces: make([]Hash, pieces),
		Files: []File{{Path: "a.mkv", Length: int64(len(data))}}, Length: int64(len(data))}
	for i := range info.Pieces {
		info.Pieces[i] = sha1.Sum(data[i*length : (i+1)*length])
	}
	tr := newTorrent(e, Spec{Hash: sha1.Sum([]byte(name)), File: -1}, nil, false)
	tb.Cleanup(tr.cancel)
	tr.info, tr.have = info, make([]bool, pieces)
	return roomSwarm{t: tr, data: func(b block) []byte {
		start := int64(b.piece)*int64(length) + b.offset
		return data[start : start+b.length]
	}}
}

// join connects a peer that unchoked Polyfin and has the pieces told,
// every piece when none is, without asking it anything.
func (s roomSwarm) join(port uint16, has ...int) *peer {
	s.t.mu.Lock()
	defer s.t.mu.Unlock()
	address := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), port)
	p := newPeer(s.t, &streamConn{}, address)
	p.choked, p.has = false, make([]bool, len(s.t.info.Pieces))
	for i := range p.has {
		p.has[i] = len(has) == 0 || slices.Contains(has, i)
	}
	s.t.peers[address] = p
	return p
}

// read is a reader of the torrent's file at the start of piece index:
// one that waits for its first byte when waits, and one that stands
// there otherwise, as a player paused does.
func (s roomSwarm) read(index int, waits bool) *Reader {
	file := s.t.info.Files[0]
	r := &Reader{t: s.t, file: file, ctx: context.Background(), size: file.Length}
	s.move(r, index, waits)
	return r
}

// move has a reader seek to the start of piece index.
func (s roomSwarm) move(r *Reader, index int, waits bool) {
	at := int64(index) * s.t.info.PieceLength
	span := readSpan{at: at, end: at}
	if waits {
		span.end++
	}
	s.t.mu.Lock()
	defer s.t.mu.Unlock()
	s.t.reads[r] = span
}

// roll asks the peers, in turn, for what they may be asked, then sorts
// out the pieces in flight, as a read that moves does (see
// Reader.ReadAtProgress).
func (s roomSwarm) roll(peers ...*peer) {
	s.t.mu.Lock()
	defer s.t.mu.Unlock()
	for _, p := range peers {
		s.t.fill(p)
	}
	s.t.shed()
}

// asked lists the blocks of piece index a peer is asked for.
func (s roomSwarm) asked(p *peer, index int) []block {
	s.t.mu.Lock()
	defer s.t.mu.Unlock()
	var blocks []block
	for b := range p.requests {
		if b.piece == index {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// serve has a peer send the blocks of piece index it is asked for, and
// those it is asked for meanwhile, until it is asked for none; it returns
// how many it sent.
func (s roomSwarm) serve(p *peer, index int) int {
	n := 0
	for {
		blocks := s.asked(p, index)
		if len(blocks) == 0 {
			return n
		}
		for _, b := range blocks {
			s.t.received(p, b.piece, b.offset, s.data(b))
			n++
		}
	}
}

// flying lists the pieces in flight, in order, whose bytes the torrent's
// own count must tell, each counting for a block at least.
func (s roomSwarm) flying(tb testing.TB) []int {
	tb.Helper()
	s.t.mu.Lock()
	defer s.t.mu.Unlock()
	var pieces []int
	held := int64(0)
	for index, in := range s.t.inflight {
		pieces = append(pieces, index)
		held += max(int64(len(in.data)), blockSize)
	}
	if held != s.t.inflightBytes {
		tb.Fatalf("the torrent counts %d bytes in flight, its %d pieces count for %d", s.t.inflightBytes, len(pieces), held)
	}
	slices.Sort(pieces)
	return pieces
}

// has reports whether a piece is kept, checked.
func (s roomSwarm) has(index int) bool {
	s.t.mu.Lock()
	defer s.t.mu.Unlock()
	return s.t.have[index]
}

// reads checks that a reader reads the first block of piece index.
func (s roomSwarm) reads(tb testing.TB, r *Reader, index int) {
	tb.Helper()
	b := block{piece: index, length: min(blockSize, s.t.info.PieceLength)}
	got := make([]byte, b.length)
	if _, err := r.ReadAt(got, int64(index)*s.t.info.PieceLength); err != nil {
		tb.Fatalf("the read of piece %d: %v", index, err)
	}
	if !bytes.Equal(got, s.data(b)) {
		tb.Fatalf("the read of piece %d did not return the torrent's bytes", index)
	}
}

// A piece no read wants any more is dropped once no peer is asked for a
// block of it: what the peers were asked for still comes, and a piece
// they complete meanwhile is kept. A read that comes back to a piece
// dropped has every block of it asked again, those received before
// included, and reads its bytes.
func TestAPieceNoReadWantsIsDroppedAndAskedAgain(t *testing.T) {
	t.Parallel()
	e := roomEngine(t, 64<<20)
	s := newRoomSwarm(t, e, "Dropped", 32)
	p := s.join(6881)
	// A read at piece 4: the peer is asked for the blocks of pieces 4 to
	// 11, its pipeline.
	r := s.read(4, true)
	s.roll(p)
	if got, want := s.flying(t), []int{4, 5, 6, 7, 8, 9, 10, 11}; !slices.Equal(got, want) {
		t.Fatalf("pieces %v in flight, want %v", got, want)
	}
	// Two blocks of piece 4 come, then the read moves to piece 20: the
	// pieces it leaves are asked of the peer still, and stay.
	for _, b := range s.asked(p, 4) {
		if b.offset < 2*blockSize {
			s.t.received(p, b.piece, b.offset, s.data(b))
		}
	}
	s.move(r, 20, true)
	s.roll(p)
	if got := s.flying(t); !slices.Contains(got, 4) || !slices.Contains(got, 11) {
		t.Fatalf("pieces %v in flight: those the peer is asked for are gone", got)
	}
	// What the peer was asked for still comes: piece 5, whole, is kept.
	if sent := s.serve(p, 5); sent != 4 || !s.has(5) {
		t.Fatalf("piece 5: %d blocks sent, kept %v", sent, s.has(5))
	}
	// The peer chokes: it is asked for nothing. The pieces no read wants
	// go, as watch sorts them out; the one the read waits for stays.
	if err := s.t.handle(p, msgChoke, nil); err != nil {
		t.Fatal(err)
	}
	s.roll()
	if got, want := s.flying(t), []int{20}; !slices.Equal(got, want) {
		t.Fatalf("pieces %v in flight once the peer choked, want %v: those no read wants stay", got, want)
	}
	if own := e.inflightBytes.Load(); own != roomPiece {
		t.Fatalf("the engine counts %d bytes in flight, want one piece's %d", own, roomPiece)
	}
	// The read comes back to piece 4: its four blocks are asked again.
	s.move(r, 4, true)
	if err := s.t.handle(p, msgUnchoke, nil); err != nil {
		t.Fatal(err)
	}
	if asked := s.asked(p, 4); len(asked) != 4 {
		t.Fatalf("%d blocks of piece 4 asked again, want its 4", len(asked))
	}
	if sent := s.serve(p, 4); sent != 4 {
		t.Fatalf("%d blocks of piece 4 sent, want 4", sent)
	}
	s.reads(t, r, 4)
}

// A read whose piece cannot start, the room being taken by pieces other
// reads wait for, is not held for good: its piece starts as soon as one
// of them is kept, or dropped as its read closes. And the room of pieces
// fetched ahead goes to a piece a read waits for at once, their requests
// cancelled.
func TestAReadHeldForRoomGoesOnOnceAPieceIsKeptOrDropped(t *testing.T) {
	t.Parallel()
	// held is a swarm in an engine with room for two pieces, both taken
	// by pieces at 4 and 10, which their reads wait for when wait, and a
	// third read waiting at 20.
	held := func(t *testing.T, wait bool) (roomSwarm, *peer, [3]*Reader) {
		t.Helper()
		e := roomEngine(t, 2*roomPiece)
		s := newRoomSwarm(t, e, "Held", 32)
		p := s.join(6881)
		first, second := s.read(4, wait), s.read(10, wait)
		s.roll(p)
		if got, want := s.flying(t), []int{4, 10}; !slices.Equal(got, want) {
			t.Fatalf("pieces %v in flight, want %v", got, want)
		}
		third := s.read(20, true)
		s.t.mu.Lock()
		s.t.fill(p)
		s.t.mu.Unlock()
		if got, want := s.flying(t), []int{4, 10}; !slices.Equal(got, want) {
			t.Fatalf("pieces %v in flight, want %v: piece 20 started past the room", got, want)
		}
		if own, room := e.inflightBytes.Load(), e.inflightRoom(); own != 2*roomPiece || own != room {
			t.Fatalf("the engine counts %d bytes in flight, want its room of %d, two pieces", own, room)
		}
		return s, p, [3]*Reader{first, second, third}
	}
	t.Run("a piece is kept", func(t *testing.T) {
		t.Parallel()
		s, p, readers := held(t, true)
		// Nothing gives way: the three pieces are wanted alike.
		s.roll(p)
		if got, want := s.flying(t), []int{4, 10}; !slices.Equal(got, want) {
			t.Fatalf("pieces %v in flight, want %v", got, want)
		}
		if sent := s.serve(p, 4); sent != 4 || !s.has(4) {
			t.Fatalf("piece 4: %d blocks sent, kept %v", sent, s.has(4))
		}
		if got, want := s.flying(t), []int{10, 20}; !slices.Equal(got, want) {
			t.Fatalf("pieces %v in flight once piece 4 is kept, want %v", got, want)
		}
		s.serve(p, 20)
		s.reads(t, readers[2], 20)
	})
	t.Run("a read closes", func(t *testing.T) {
		t.Parallel()
		s, p, readers := held(t, true)
		sent(p, msgCancel)
		readers[0].Close()
		if got, want := s.flying(t), []int{10, 20}; !slices.Equal(got, want) {
			t.Fatalf("pieces %v in flight once the read of piece 4 closed, want %v", got, want)
		}
		if cancelled := piecesOf(sent(p, msgCancel)); len(cancelled) != 1 || cancelled[4] != 4 {
			t.Fatalf("the peer was told to cancel %v, want the 4 blocks of piece 4", cancelled)
		}
		s.serve(p, 20)
		s.reads(t, readers[2], 20)
	})
	t.Run("pieces fetched ahead", func(t *testing.T) {
		t.Parallel()
		s, p, readers := held(t, false)
		sent(p, msgCancel)
		s.roll(p)
		got := s.flying(t)
		if len(got) != 2 || !slices.Contains(got, 20) {
			t.Fatalf("pieces %v in flight, want piece 20 and one of those fetched ahead", got)
		}
		if cancelled := piecesOf(sent(p, msgCancel)); len(cancelled) != 1 || cancelled[4]+cancelled[10] != 4 {
			t.Fatalf("the peer was told to cancel %v, want the 4 blocks of one piece fetched ahead", cancelled)
		}
		s.serve(p, 20)
		s.reads(t, readers[2], 20)
	})
}

// The endgame goes on while a piece a read waits for is held for room:
// the blocks of the pieces in flight that a slow peer holds are asked of
// a second peer, and the piece held starts once one of them is kept.
func TestTheEndgameGoesOnWhileAPieceWaitsForRoom(t *testing.T) {
	t.Parallel()
	e := roomEngine(t, 2*roomPiece)
	s := newRoomSwarm(t, e, "Endgame", 32)
	slow := s.join(6881)
	s.read(4, true)
	s.read(10, true)
	s.roll(slow)
	third := s.read(20, true)
	fast := s.join(6882)
	s.roll(fast)
	if got, want := s.flying(t), []int{4, 10}; !slices.Equal(got, want) {
		t.Fatalf("pieces %v in flight, want %v", got, want)
	}
	if first, second := len(s.asked(fast, 4)), len(s.asked(fast, 10)); first != 4 || second != 4 {
		t.Fatalf("the second peer is asked for %d blocks of piece 4 and %d of piece 10, want the 4 of each that the slow one holds", first, second)
	}
	if sent := s.serve(fast, 4); sent != 4 || !s.has(4) {
		t.Fatalf("piece 4: %d blocks sent, kept %v", sent, s.has(4))
	}
	if got, want := s.flying(t), []int{10, 20}; !slices.Equal(got, want) {
		t.Fatalf("pieces %v in flight once piece 4 is kept, want %v", got, want)
	}
	if sent := s.serve(slow, 20) + s.serve(fast, 20); sent != 4 {
		t.Fatalf("%d blocks of piece 20 sent, want 4", sent)
	}
	s.reads(t, third, 20)
}

// The engine's room is shared by its torrents, and none is kept from
// playing by what the others hold: a torrent that has no piece in flight
// starts one whatever they hold, and no second one past the room. A
// torrent that stops gives its room back.
func TestATorrentStartsAPieceWhateverTheOthersHold(t *testing.T) {
	t.Parallel()
	e := roomEngine(t, 2*roomPiece)
	room := e.inflightRoom()
	full := newRoomSwarm(t, e, "Full", 32)
	other := newRoomSwarm(t, e, "Other", 32)
	// The first torrent takes the engine's room: two pieces.
	fp := full.join(6881)
	full.read(4, true)
	full.read(10, true)
	full.roll(fp)
	if got, want := full.flying(t), []int{4, 10}; !slices.Equal(got, want) || e.inflightBytes.Load() != room {
		t.Fatalf("pieces %v in flight, %d bytes, want %v and the room of %d", got, e.inflightBytes.Load(), want, room)
	}
	// The second starts one piece of the two its reads wait for.
	op := other.join(6882)
	readers := map[int]*Reader{3: other.read(3, true), 8: other.read(8, true)}
	other.roll(op)
	started := other.flying(t)
	if len(started) != 1 || e.inflightBytes.Load() != room+roomPiece {
		t.Fatalf("pieces %v of the second torrent in flight, %d bytes in all, want one piece past the room of %d", started, e.inflightBytes.Load(), room)
	}
	// Once it is kept, the next one starts.
	next := 3 + 8 - started[0]
	other.serve(op, started[0])
	other.reads(t, readers[started[0]], started[0])
	if got, want := other.flying(t), []int{next}; !slices.Equal(got, want) {
		t.Fatalf("pieces %v of the second torrent in flight, want %v", got, want)
	}
	// The first torrent stops: its pieces in flight are forgotten.
	full.t.fail(errStopped)
	if got := full.flying(t); len(got) != 0 || e.inflightBytes.Load() != roomPiece {
		t.Fatalf("the torrent stopped: pieces %v in flight, %d bytes in all, want the other torrent's piece alone", got, e.inflightBytes.Load())
	}
	// The second now has the room: a second piece, and no third.
	other.read(12, true)
	other.read(16, true)
	other.roll(op)
	if got := other.flying(t); len(got) != 2 || e.inflightBytes.Load() != room {
		t.Fatalf("pieces %v of the second torrent in flight, %d bytes, want two pieces, the room of %d", got, e.inflightBytes.Load(), room)
	}
}

// Pieces shorter than a block, as an info dictionary may tell them, count
// for a block each among those in flight: no more of them start than the
// room holds blocks, however few bytes they hold.
func TestPiecesShorterThanABlockCountForABlockEach(t *testing.T) {
	t.Parallel()
	const short = 1 << 10
	e := roomEngine(t, 4*blockSize)
	s := roomSwarmOf(t, e, "Short", short, 64)
	p := s.join(6881)
	r := s.read(0, true)
	s.roll(p)
	if got, want := s.flying(t), []int{0, 1, 2, 3}; !slices.Equal(got, want) {
		t.Fatalf("pieces %v of %d bytes in flight, want %v: the room is %d bytes, four blocks", got, short, want, e.inflightRoom())
	}
	if own := e.inflightBytes.Load(); own != 4*blockSize {
		t.Fatalf("the engine counts %d bytes in flight, want four blocks' %d", own, 4*blockSize)
	}
	if sent := s.serve(p, 0); sent != 1 || !s.has(0) {
		t.Fatalf("piece 0: %d blocks sent, kept %v", sent, s.has(0))
	}
	if got, want := s.flying(t), []int{1, 2, 3, 4}; !slices.Equal(got, want) {
		t.Fatalf("pieces %v in flight once piece 0 is kept, want %v", got, want)
	}
	s.reads(t, r, 0)
}

// The room of a torrent's pieces in flight is four read windows, and for
// pieces longer than 8 MiB what one read asks for, its window and the
// edges of its file, within two of the longest pieces. The engine's room
// is four of the longest pieces, within its Budget. One read of a torrent
// of pieces of up to 16 MiB, as those of movies and episodes are, never
// meets the torrent's room.
func TestTheRoomOfPiecesInFlight(t *testing.T) {
	t.Parallel()
	const mib = 1 << 20
	for _, c := range []struct {
		budget, length         int64
		window                 int
		room, engine           int64
		oneReadFitsTheTorrents bool
	}{
		// The default Budget, a quarter of 10 GB, then the least, of 1 GB.
		{2_500_000_000, 256 << 10, 64, 64 * mib, 256 * mib, true},
		{2_500_000_000, 4 * mib, 4, 64 * mib, 256 * mib, true},
		{2_500_000_000, 8 * mib, 4, 64 * mib, 256 * mib, true},
		{2_500_000_000, 16 * mib, 4, 128 * mib, 256 * mib, true},
		{2_500_000_000, 32 * mib, 4, 128 * mib, 256 * mib, false},
		{2_500_000_000, maxPieceLength, 4, 128 * mib, 256 * mib, false},
		{250_000_000, 4 * mib, 4, 64 * mib, 250_000_000, true},
		{250_000_000, 16 * mib, 4, 128 * mib, 250_000_000, true},
		{250_000_000, maxPieceLength, 1, 128 * mib, 250_000_000, false},
		// Pieces of 1 KiB: a window of as many pieces as it holds blocks.
		{2_500_000_000, 1 << 10, 1024, 64 * mib, 256 * mib, true},
		// A Budget of less than a piece: one piece at a time.
		{mib, 4 * mib, 1, 64 * mib, mib, true},
	} {
		e := roomEngine(t, c.budget)
		tr := newTorrent(e, Spec{File: -1}, nil, false)
		t.Cleanup(tr.cancel)
		tr.info = &Info{PieceLength: c.length, Pieces: make([]Hash, 64), Length: 64 * c.length}
		tr.mu.Lock()
		window, room := tr.window(), tr.room()
		tr.mu.Unlock()
		if window != c.window || room != c.room || e.inflightRoom() != c.engine {
			t.Errorf("budget %d, pieces of %d: a window of %d pieces, room for %d bytes, %d in the engine; want %d, %d and %d",
				c.budget, c.length, window, room, e.inflightRoom(), c.window, c.room, c.engine)
		}
		if room < c.length || room < inflightRoom || room > 2*maxPieceLength {
			t.Errorf("pieces of %d: room for %d bytes, want a piece and %d at least, %d at most", c.length, room, inflightRoom, 2*maxPieceLength)
		}
		if fits := int64(window+2*edgePieces)*c.length <= room; fits != c.oneReadFitsTheTorrents {
			t.Errorf("pieces of %d: one read's %d pieces fit the room of %d: %v, want %v", c.length, window+2*edgePieces, room, fits, c.oneReadFitsTheTorrents)
		}
	}
}

// A torrent of the longest pieces, 64 MiB, plays through its window: two
// of them are in flight at once, the third waits for room, and each is
// read once kept.
//
//nolint:paralleltest // holds two pieces of 64 MiB in memory, which the tests beside it need not share
func TestATorrentOfTheLongestPiecesPlaysThroughItsWindow(t *testing.T) {
	const pieces = 4
	// Every block holds the same bytes, and so every piece has one hash:
	// the file's 256 MiB are never in memory.
	filler := torrenttest.Bytes(blockSize)
	sum := sha1.New()
	for range maxPieceLength / blockSize {
		sum.Write(filler)
	}
	hash := Hash(sum.Sum(nil))
	e := roomEngine(t, 1<<30)
	tr := newTorrent(e, Spec{Hash: hash, File: -1}, nil, false)
	t.Cleanup(tr.cancel)
	tr.info = &Info{Name: "long.mkv", PieceLength: maxPieceLength, Pieces: []Hash{hash, hash, hash, hash},
		Files: []File{{Path: "long.mkv", Length: pieces * maxPieceLength}}, Length: pieces * maxPieceLength}
	tr.have = make([]bool, pieces)
	s := roomSwarm{t: tr, data: func(b block) []byte { return filler[:b.length] }}
	const room = 2 * maxPieceLength
	tr.mu.Lock()
	window := tr.window()
	tr.mu.Unlock()
	if window != pieces {
		t.Fatalf("a window of %d pieces, want %d", window, pieces)
	}
	// Each peer has one piece, or two, so that each piece of the window
	// may be asked at once: the read's window is the four pieces.
	owners := []*peer{s.join(6881, 0, 3), s.join(6882, 1), s.join(6883, 2), nil}
	owners[3] = owners[0]
	r := s.read(0, true)
	s.roll(owners[0], owners[1], owners[2])
	if got, want := s.flying(t), []int{0, 1}; !slices.Equal(got, want) {
		t.Fatalf("pieces %v in flight, want %v: the room is two pieces", got, want)
	}
	if asked := s.asked(owners[2], 2); len(asked) != 0 {
		t.Fatalf("%d blocks of piece 2 asked past the room", len(asked))
	}
	tr.mu.Lock()
	own := tr.room()
	tr.mu.Unlock()
	if own != room {
		t.Fatalf("room for %d bytes, want two pieces' %d", own, room)
	}
	kept := 0
	for len(s.flying(t)) > 0 {
		flying := s.flying(t)
		tr.mu.Lock()
		held, own := tr.inflightBytes, e.inflightBytes.Load()
		tr.mu.Unlock()
		if len(flying) > 2 || held > room || own != held {
			t.Fatalf("pieces %v in flight, %d bytes, %d by the engine's count: more than the room of %d", flying, held, own, room)
		}
		index := flying[0]
		if sent := s.serve(owners[index], index); sent != maxPieceLength/blockSize || !s.has(index) {
			t.Fatalf("piece %d: %d blocks sent, kept %v", index, sent, s.has(index))
		}
		s.reads(t, r, index)
		kept++
	}
	if kept != pieces {
		t.Fatalf("%d pieces kept, want the window's %d", kept, pieces)
	}
}
