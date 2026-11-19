package torrent

import (
	"bytes"
	"context"
	"crypto/sha1"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"
)

// dialedRoute is a route that reaches no peer, and tells which it was
// asked to dial.
type dialedRoute struct {
	mu    sync.Mutex
	dials []string
}

func (r *dialedRoute) Dial(_ context.Context, address string) (net.Conn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dials = append(r.dials, address)
	return nil, net.ErrClosed
}
func (r *dialedRoute) ListenPacket(context.Context) (PacketConn, error) { return nil, ErrNoUDP }
func (r *dialedRoute) Check(context.Context) error                      { return nil }
func (r *dialedRoute) String() string                                   { return "dialed" }
func (r *dialedRoute) Close() error                                     { return nil }

// dialed lists the addresses the route was asked to dial.
func (r *dialedRoute) dialed() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.dials...)
}

// peerAt connects a peer on an address of its own, which the torrent
// knows; it chokes Polyfin and tells no piece. The lock is held.
func peerAt(tr *torrent, ip string) *peer {
	address := netip.AddrPortFrom(netip.MustParseAddr(ip), 6881)
	p := newPeer(tr, &streamConn{}, address)
	tr.peers[address], tr.known[address] = p, candidate{}
	return p
}

// servingAt connects a peer on an address of its own that has every piece
// and unchoked Polyfin, as requestSwarm.join does on one address. The
// lock is held.
func servingAt(s requestSwarm, ip string) *peer {
	p := peerAt(s.t, ip)
	p.choked, p.has = false, make([]bool, len(s.t.info.Pieces))
	for i := range p.has {
		p.has[i] = true
	}
	return p
}

// connected reports whether a peer's connection is open.
func connected(p *peer) bool {
	select {
	case <-p.closed:
		return false
	default:
		return true
	}
}

// asksOf counts the blocks of a piece asked of a peer.
func asksOf(tr *torrent, p *peer, piece int) int {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	n := 0
	for b := range p.requests {
		if b.piece == piece {
			n++
		}
	}
	return n
}

// blocksOf are the blocks of a piece of the swarm's torrent, in order.
func blocksOf(s requestSwarm, piece int) []block {
	var blocks []block
	for offset := 0; offset < s.tor.PieceLength; offset += blockSize {
		blocks = append(blocks, block{piece: piece, offset: int64(offset), length: blockSize})
	}
	return blocks
}

// wrongBlock is a block of bytes that are no piece's.
func wrongBlock() []byte { return bytes.Repeat([]byte{0x5a}, blockSize) }

// has reports whether the swarm's torrent has a piece.
func (s requestSwarm) has(piece int) bool {
	s.t.mu.Lock()
	defer s.t.mu.Unlock()
	return s.t.have[piece]
}

// sendOther has a peer send one of the blocks asked of it, of another
// piece than the one given: its pipeline has room again, and it is asked
// for what it may be.
func (s requestSwarm) sendOther(tb testing.TB, p *peer, piece int) {
	tb.Helper()
	s.t.mu.Lock()
	var next *block
	for b := range p.requests {
		if b.piece != piece {
			next = &b
			break
		}
	}
	s.t.mu.Unlock()
	if next == nil {
		tb.Fatal("no block of another piece is asked of the peer")
	}
	s.deliver(p, *next)
}

// A block is kept only from a peer asked for it. One that a peer nobody
// asked sends first does not take the place of the block asked: the piece
// passes its hash with the blocks of the peer asked. A block sent twice,
// as after a request made again, costs its peer nothing.
func TestABlockIsKeptOnlyFromAPeerAskedForIt(t *testing.T) {
	t.Parallel()
	s := newRequestSwarm(t)
	s.t.mu.Lock()
	asked, other := servingAt(s, "127.0.0.2"), servingAt(s, "127.0.0.3")
	// The other peer chokes: it is asked nothing.
	other.choked = true
	s.t.fill(asked)
	s.t.mu.Unlock()
	if asksOf(s.t, asked, 0) == 0 || asksOf(s.t, other, 0) != 0 {
		t.Fatalf("%d blocks of the first piece asked of one peer, %d of the other", asksOf(s.t, asked, 0), asksOf(s.t, other, 0))
	}
	s.t.received(other, 0, 0, wrongBlock())
	for _, b := range blocksOf(s, 0) {
		s.deliver(asked, b)
	}
	if !s.has(0) {
		t.Fatal("the piece sent by the peer asked was not kept: a block nobody asked for took the place of one of its own")
	}
	// The peer asked sends a block of the next piece twice, and one of the
	// piece kept once more.
	blocks := blocksOf(s, 1)
	s.deliver(asked, blocks[0])
	s.deliver(asked, blocks[0])
	s.deliver(asked, blocksOf(s, 0)[0])
	for _, b := range blocks[1:] {
		s.deliver(asked, b)
	}
	if !s.has(1) {
		t.Fatal("the piece a block of which came twice was not kept")
	}
	if !connected(asked) || !connected(other) {
		t.Fatalf("a block sent twice, or one nobody asked for, ended a connection: %v, %v", connected(asked), connected(other))
	}
	if n := asksOf(s.t, asked, 2); n == 0 {
		t.Fatal("the peer that sent a block twice is asked no more")
	}
}

// A peer that alone sent a piece that fails its hash is disconnected, and
// its address is not dialed again, on any port: the piece is asked of
// another peer, and kept.
func TestAPeerWhoseWholePieceIsWrongIsNotDialedAgain(t *testing.T) {
	t.Parallel()
	s := newRequestSwarm(t)
	route := &dialedRoute{}
	s.t.route = route
	s.t.mu.Lock()
	wrong, right := servingAt(s, "127.0.0.2"), servingAt(s, "127.0.0.3")
	s.t.fill(wrong)
	s.t.mu.Unlock()
	for _, b := range blocksOf(s, 0) {
		s.t.received(wrong, b.piece, b.offset, wrongBlock())
	}
	if s.has(0) {
		t.Fatal("a piece of wrong bytes was kept")
	}
	if connected(wrong) {
		t.Fatal("the peer that alone sent a piece of wrong bytes is still connected")
	}
	if !connected(right) {
		t.Fatal("the other peer was disconnected")
	}
	if n := asksOf(s.t, right, 0); n != len(blocksOf(s, 0)) {
		t.Fatalf("%d blocks of the piece asked of the other peer, want them all", n)
	}
	if n := asksOf(s.t, wrong, 0) + asksOf(s.t, wrong, 1); n != 0 {
		t.Fatalf("%d blocks still asked of the peer disconnected", n)
	}
	for _, b := range blocksOf(s, 0) {
		s.deliver(right, b)
	}
	if !s.has(0) {
		t.Fatal("the piece the other peer sent was not kept")
	}
	// The same address on another port, then a peer never seen, are told:
	// once the second is dialed, the first was passed over.
	go s.t.connect()
	again, fresh := netip.MustParseAddrPort("127.0.0.2:7000"), netip.MustParseAddrPort("127.0.0.9:6881")
	s.t.addPeers([]netip.AddrPort{again, fresh})
	waitFor(t, "the peer never seen to be dialed", func() bool { return len(route.dialed()) > 0 })
	for _, address := range route.dialed() {
		if address != fresh.String() {
			t.Fatalf("%s was dialed: the address of a peer that sent wrong bytes, or one that is connected", address)
		}
	}
}

// spoiltShare is a swarm whose first piece just failed its hash with
// blocks of two peers: right sent its first blocks, as it sent the whole
// second piece before, and wrong its last block, of wrong bytes. A third
// peer serves other pieces.
func spoiltShare(t *testing.T) (s requestSwarm, right, wrong, third *peer) {
	t.Helper()
	s = newRequestSwarm(t)
	blocks := blocksOf(s, 0)
	last := blocks[len(blocks)-1]
	s.t.mu.Lock()
	right = servingAt(s, "127.0.0.2")
	s.t.fill(right)
	wrong, third = servingAt(s, "127.0.0.3"), servingAt(s, "127.0.0.4")
	// The last block of the first piece is another peer's to send, as
	// when a pipeline is full before a piece is asked whole.
	if !s.t.forget(right, last) {
		t.Fatal("the last block of the first piece was not asked")
	}
	s.t.ask(wrong, s.t.inflight[0], len(blocks)-1, last, time.Now())
	s.t.mu.Unlock()
	for _, b := range blocksOf(s, 1) {
		s.deliver(right, b)
	}
	for _, b := range blocks[:len(blocks)-1] {
		s.deliver(right, b)
	}
	s.t.received(wrong, last.piece, last.offset, wrongBlock())
	if s.has(0) || !s.has(1) {
		t.Fatalf("the first piece kept: %v, the second: %v", s.has(0), s.has(1))
	}
	return s, right, wrong, third
}

// When a piece fails its hash with blocks of several peers, none of them
// is disconnected: all but one may have sent right bytes. The piece is
// asked again of one peer alone, the one whose pieces passed before, and
// once it passes, the peer whose block differed is disconnected, and the
// others kept.
func TestAWrongBlockAmongSeveralPeersIsToldApartOnceThePiecePasses(t *testing.T) {
	t.Parallel()
	s, right, wrong, third := spoiltShare(t)
	// Their pipelines are full: each sends a block, and is asked for what
	// it may be.
	for _, p := range []*peer{wrong, third, right} {
		s.sendOther(t, p, 0)
	}
	if !connected(right) || !connected(wrong) || !connected(third) {
		t.Fatalf("a peer was disconnected before its bytes were told apart: %v, %v, %v", connected(right), connected(wrong), connected(third))
	}
	if n, m := asksOf(s.t, wrong, 0), asksOf(s.t, third, 0); n != 0 || m != 0 {
		t.Fatalf("%d and %d blocks of the piece asked of the other peers: it is asked again of one peer alone", n, m)
	}
	if asksOf(s.t, right, 0) == 0 {
		t.Fatal("the piece is not asked again of the peer whose pieces passed")
	}
	for _, b := range blocksOf(s, 0) {
		for sent := 0; ; sent++ {
			s.t.mu.Lock()
			_, asked := right.requests[b]
			s.t.mu.Unlock()
			if asked {
				break
			}
			if sent == pipeline {
				t.Fatalf("the block at %d is not asked of the peer the piece is asked of", b.offset)
			}
			s.sendOther(t, right, 0)
		}
		s.deliver(right, b)
	}
	if !s.has(0) {
		t.Fatal("the piece sent whole by one peer was not kept")
	}
	if connected(wrong) {
		t.Fatal("the peer whose block differed from the piece's is still connected")
	}
	if !connected(right) || !connected(third) {
		t.Fatalf("a peer that sent no wrong byte was disconnected: %v, %v", connected(right), connected(third))
	}
}

// The peer asked alone for a piece that failed its hash keeps it while it
// sends; once a block of the piece has waited as long as a stalled
// request does, the peer is given up, and the piece is another's to send.
func TestAPieceAskedOfOnePeerAloneGoesToAnotherWhenItWaits(t *testing.T) {
	t.Parallel()
	s, right, wrong, third := spoiltShare(t)
	// Its pipeline is full: it sends a block, and is asked for the piece.
	s.sendOther(t, right, 0)
	s.t.reask()
	if !connected(right) || asksOf(s.t, right, 0) == 0 {
		t.Fatalf("the peer asked a moment ago lost the piece: connected %v, %d blocks asked", connected(right), asksOf(s.t, right, 0))
	}
	s.t.mu.Lock()
	in := s.t.inflight[0]
	for i, at := range in.requested {
		if !at.IsZero() {
			in.requested[i] = at.Add(-stallWait)
		}
	}
	s.t.mu.Unlock()
	s.t.reask()
	s.t.mu.Lock()
	givenUp := right.givenUp
	s.t.mu.Unlock()
	if connected(right) || !givenUp {
		t.Fatalf("the peer that let the piece wait is connected: %v, given up: %v", connected(right), givenUp)
	}
	if !connected(wrong) || !connected(third) {
		t.Fatalf("another peer was disconnected: %v, %v", connected(wrong), connected(third))
	}
	// The others' pipelines are full: each sends a block, and the one the
	// piece is now asked of is asked for it.
	for _, p := range []*peer{wrong, third} {
		s.sendOther(t, p, 0)
	}
	if n, m := asksOf(s.t, wrong, 0), asksOf(s.t, third, 0); (n == 0) == (m == 0) {
		t.Fatalf("%d and %d blocks of the piece asked of the two peers left, want one of them asked alone", n, m)
	}
}

// extensionHandshake is the extension handshake of a peer that sends the
// info dictionary, of the size it announces.
func extensionHandshake(size int) []byte {
	return append([]byte{0}, encode(map[string]any{"m": map[string]any{"ut_metadata": 3}, "metadata_size": size})...)
}

// metadataPiece is a peer's answer to a request for a piece of the info
// dictionary.
func metadataPiece(piece, size int, data []byte) []byte {
	return append(append([]byte{extMetadata}, encode(map[string]any{"msg_type": 1, "piece": piece, "total_size": size})...), data...)
}

// infoless is a torrent whose info dictionary is not known, and the
// dictionary: the swarm's, or, when long, one of two pieces of metadata.
func infoless(t *testing.T, long bool) (*torrent, []byte) {
	t.Helper()
	s := newFuzzSwarm(t, true)
	hash, info := Hash(s.tor.Hash), s.tor.Info
	if long {
		info = encode(map[string]any{"name": "a.mkv", "piece length": 16 << 10, "length": 1000 * (16 << 10),
			"pieces": string(bytes.Repeat([]byte{7}, 1000*20))})
		hash = sha1.Sum(info)
		if len(info) <= metadataBlock || len(info) > 2*metadataBlock {
			t.Fatalf("an info dictionary of %d bytes, want two pieces", len(info))
		}
	}
	tr := newTorrent(fuzzEngine(t, nil), Spec{Hash: hash, File: -1}, nil, false)
	t.Cleanup(tr.cancel)
	return tr, info
}

// hasInfo reports whether the torrent knows its info dictionary.
func hasInfo(tr *torrent) bool {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.info != nil
}

// The size a peer announces for the info dictionary changes nothing of
// what another peer sends of it: the torrent has its info once the peer
// asked first answered, whatever was announced meanwhile.
func TestAPeersAnnouncedSizeLeavesWhatOthersSendOfTheInfo(t *testing.T) {
	t.Parallel()
	tr, info := infoless(t, false)
	tr.mu.Lock()
	first, other := peerAt(tr, "127.0.0.2"), peerAt(tr, "127.0.0.3")
	tr.mu.Unlock()
	if err := tr.handle(first, msgExtended, extensionHandshake(len(info))); err != nil {
		t.Fatal(err)
	}
	if first.queued() != 1 {
		t.Fatalf("%d requests sent to the first peer, want one", first.queued())
	}
	for _, size := range []int{len(info) + 1, len(info) - 1, maxInfoSize} {
		if err := tr.handle(other, msgExtended, extensionHandshake(size)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tr.handle(first, msgExtended, metadataPiece(0, len(info), info)); err != nil {
		t.Fatal(err)
	}
	if !hasInfo(tr) {
		t.Fatal("the torrent has no info: another peer's handshake dropped what the first one sent")
	}
	if !connected(first) || !connected(other) {
		t.Fatalf("a peer was disconnected: %v, %v", connected(first), connected(other))
	}
}

// An info dictionary that does not match the torrent's hash is the doing
// of the peer that sent it whole: that peer is disconnected, and the
// pieces another peer sends, before and after, make the torrent's info.
func TestAWrongInfoIsItsPeersAlone(t *testing.T) {
	t.Parallel()
	tr, info := infoless(t, true)
	tr.mu.Lock()
	right, wrong := peerAt(tr, "127.0.0.2"), peerAt(tr, "127.0.0.3")
	tr.mu.Unlock()
	for _, p := range []*peer{right, wrong} {
		if err := tr.handle(p, msgExtended, extensionHandshake(len(info))); err != nil {
			t.Fatal(err)
		}
		if p.queued() != 2 {
			t.Fatalf("%d requests sent to a peer, want two", p.queued())
		}
	}
	other := bytes.Repeat([]byte{0x5a}, len(info))
	for _, message := range []struct {
		from  *peer
		piece int
		data  []byte
	}{{right, 0, info}, {wrong, 0, other}, {wrong, 1, other}, {right, 1, info}} {
		start := message.piece * metadataBlock
		piece := metadataPiece(message.piece, len(info), message.data[start:min(start+metadataBlock, len(info))])
		if err := tr.handle(message.from, msgExtended, piece); err != nil {
			t.Fatal(err)
		}
	}
	if !hasInfo(tr) {
		t.Fatal("the torrent has no info: one peer's wrong pieces dropped the other's")
	}
	if connected(wrong) {
		t.Fatal("the peer that sent an info dictionary of wrong bytes is still connected")
	}
	if !connected(right) {
		t.Fatal("the peer that sent the torrent's info was disconnected")
	}
}

// The copies of the info dictionary being received have room for two of
// the largest: a third peer announcing as much waits, and is asked once a
// copy no piece of which came gives its place. A peer announcing less is
// asked at once, and the torrent has the info it sends.
func TestTheInfoBeingReceivedStaysWithinItsRoom(t *testing.T) {
	t.Parallel()
	tr, info := infoless(t, false)
	// A copy no piece of which came is stale at once.
	tr.engine.dropWait = time.Nanosecond
	tr.mu.Lock()
	var large []*peer
	for _, ip := range []string{"127.0.0.2", "127.0.0.3", "127.0.0.4"} {
		large = append(large, peerAt(tr, ip))
	}
	small := peerAt(tr, "127.0.0.5")
	tr.mu.Unlock()
	pieces := maxInfoSize / metadataBlock
	for _, p := range large {
		if err := tr.handle(p, msgExtended, extensionHandshake(maxInfoSize)); err != nil {
			t.Fatal(err)
		}
	}
	if a, b, c := large[0].queued(), large[1].queued(), large[2].queued(); a != pieces || b != pieces || c != 0 {
		t.Fatalf("%d, %d and %d pieces asked of the peers announcing the largest info, want %d of two and the third waiting", a, b, c, pieces)
	}
	tr.reask()
	if n := large[2].queued(); n != pieces {
		t.Fatalf("%d pieces asked of the peer that waited, once a copy no piece of which came is stale", n)
	}
	if err := tr.handle(small, msgExtended, extensionHandshake(len(info))); err != nil {
		t.Fatal(err)
	}
	if small.queued() != 1 {
		t.Fatalf("%d requests sent to the peer announcing a small info, want one at once", small.queued())
	}
	if err := tr.handle(small, msgExtended, metadataPiece(0, len(info), info)); err != nil {
		t.Fatal(err)
	}
	if !hasInfo(tr) {
		t.Fatal("the torrent has no info")
	}
}

// When every place is taken and peers wait for one, the peers that sent
// nothing kept for the engine's idleTimeout, and are asked nothing, give
// their places, one for each peer waiting. A peer connected a moment ago,
// one that is asked blocks, and one that sent a block a moment ago keep
// theirs, whether they choke or not.
func TestAPeerOfNoUseGivesItsPlaceToOneThatWaits(t *testing.T) {
	t.Parallel()
	s := newRequestSwarm(t)
	route := &dialedRoute{}
	s.t.route = route
	longAgo := time.Now().Add(-6 * time.Hour)
	var idle []*peer
	// A peer whose dial failed is not dialed again while the test runs.
	s.t.engine.redialWait = time.Hour
	s.t.mu.Lock()
	for i := range maxPeers - 3 {
		// Connected six hours ago, it has every piece, chokes Polyfin, and
		// sent nothing since but keep-alives.
		p := servingAt(s, netip.AddrFrom4([4]byte{127, 0, 1, byte(i + 1)}).String())
		p.choked, p.joined = true, longAgo
		idle = append(idle, p)
	}
	brief := servingAt(s, "127.0.2.1")
	brief.choked, brief.joined = true, time.Now()
	asked, useful := servingAt(s, "127.0.2.2"), servingAt(s, "127.0.2.3")
	asked.joined, useful.joined = longAgo, longAgo
	s.t.fill(asked)
	s.t.fill(useful)
	var sends block
	for b := range useful.requests {
		sends = b
		break
	}
	s.t.mu.Unlock()
	// The last one sends a block, then chokes: nothing is asked of it.
	s.deliver(useful, sends)
	if err := s.t.handle(useful, msgChoke, nil); err != nil {
		t.Fatal(err)
	}
	if asksOf(s.t, useful, sends.piece) != 0 || asksOf(s.t, asked, 0) == 0 {
		t.Fatal("the peers that keep their places are not as the test wants them")
	}

	go s.t.connect()
	waiting := make([]netip.AddrPort, 5)
	for i := range waiting {
		waiting[i] = netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 3, byte(i + 1)}), 6881)
	}
	s.t.addPeers(waiting)
	relieved := func() []*peer {
		var closed []*peer
		for _, p := range idle {
			if !connected(p) {
				closed = append(closed, p)
			}
		}
		return closed
	}
	waitFor(t, "a place for each peer that waits", func() bool { return len(relieved()) >= len(waiting) })
	for _, p := range relieved() {
		// What the peer's reader does as its connection ends.
		s.t.leave(p)
	}
	waitFor(t, "the peers that waited to be dialed", func() bool { return len(route.dialed()) >= len(waiting) })
	if n := len(relieved()); n != len(waiting) {
		t.Fatalf("%d peers gave their places for %d that waited", n, len(waiting))
	}
	if !connected(brief) || !connected(asked) || !connected(useful) {
		t.Fatalf("a peer of use, or connected a moment ago, lost its place: %v, %v, %v", connected(brief), connected(asked), connected(useful))
	}
}

// A torrent knows of 2,000 peers; when it knows as many, the peer told
// next takes the place of the one whose dials failed the most, never of
// one that was connected nor of one not dialed yet, and a dial that fails
// makes room for the next peer told. With no peer to forget, the table
// stays as it is.
func TestAFullTableOfPeersForgetsOneWhoseDialsFailed(t *testing.T) {
	t.Parallel()
	const full = 2000
	s := newFuzzSwarm(t, true)
	tr := newTorrent(fuzzEngine(t, nil), Spec{Hash: s.tor.Hash, File: -1}, nil, false)
	t.Cleanup(tr.cancel)
	at := func(a, i int) netip.AddrPort {
		return netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, byte(a), byte(i >> 8), byte(i)}), 6881)
	}
	failed, failedMore, left := at(2, 1), at(2, 2), at(2, 3)
	later := time.Now().Add(time.Hour)
	tr.mu.Lock()
	info := s.info
	tr.info, tr.infoData, tr.have = &info, s.tor.Info, make([]bool, len(info.Pieces))
	teller := peerAt(tr, "127.3.0.1")
	tr.known[failed] = candidate{fails: 2, retryAt: later, last: outcomeTimedOut}
	tr.known[failedMore] = candidate{fails: 4, retryAt: later, last: outcomeRefused}
	tr.known[left] = candidate{fails: 9, retryAt: later, last: outcomeLeft}
	tr.redials, tr.nextRedial = []netip.AddrPort{failed, failedMore, left}, later
	for i := 0; len(tr.known) < full; i++ {
		tr.known[at(1, i)] = candidate{}
		tr.queue = append(tr.queue, at(1, i))
	}
	tr.mu.Unlock()
	knows := func(address netip.AddrPort) bool {
		tr.mu.Lock()
		defer tr.mu.Unlock()
		return tr.knows(address)
	}

	// A peer exchange tells one peer, then a tracker three.
	exchanged, told := at(4, 1), []netip.AddrPort{at(4, 2), at(4, 3), at(4, 4)}
	if err := tr.handle(teller, msgExtended, append([]byte{extPEX}, pexPayload([]netip.AddrPort{exchanged}, 0)...)); err != nil {
		t.Fatal(err)
	}
	tr.addPeers(told)
	for i, address := range []netip.AddrPort{exchanged, told[0], told[1]} {
		if !knows(address) {
			t.Fatalf("peer %d told to a torrent that knows %d is not known: no peer whose dials failed made room for it", i, full)
		}
	}
	if knows(failed) || knows(failedMore) {
		t.Fatalf("the peers whose dials failed are still known: %v, %v", knows(failed), knows(failedMore))
	}
	if !knows(left) {
		t.Fatal("a peer that was connected was forgotten")
	}
	if knows(told[2]) {
		t.Fatal("a peer was told past the table's size, with no peer to forget")
	}
	// A dial of a peer not tried yet fails: it is the one to forget, and
	// the peer told next is known.
	tr.mu.Lock()
	tried := tr.queue[0]
	tr.queue = tr.queue[1:]
	tr.redial(tried, outcomeRefused, 1)
	size := len(tr.known)
	tr.mu.Unlock()
	if knows(tried) || size != full {
		t.Fatalf("the peer whose dial failed is known: %v; %d peers known", knows(tried), size)
	}
	tr.addPeers(told[2:])
	if !knows(told[2]) {
		t.Fatal("the peer told after a dial failed is not known")
	}
}
