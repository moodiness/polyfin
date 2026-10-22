package torrent

import (
	"bytes"
	"context"
	"io"
	"net/netip"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// readWhole plays the torrent of a lone seeder and reads its file whole
// within a minute, however long its blocks take.
func readWhole(t *testing.T, e *Engine, tor *torrenttest.Torrent, seeder *torrenttest.Peer) {
	t.Helper()
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address()).URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r, err := e.Open(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	data, err := io.ReadAll(io.NewSectionReader(r, 0, r.Size()))
	if err != nil {
		t.Fatalf("read %d bytes of %d: %v; %d requests dropped", len(data), len(tor.Data), err, seeder.Dropped())
	}
	if !bytes.Equal(data, tor.Data) {
		t.Fatal("the file read is not the torrent's")
	}
}

// A peer that drops some REQUESTs without a word while it serves the
// others, the torrent's only one, is asked again for the blocks dropped:
// the file is read whole, the peer kept. When the peer drops only the last
// block asked, no block asked after it tells the drop: its silence does.
func TestADroppedRequestIsAskedAgain(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		size int
		drop func(int) bool
	}{
		"every third block": {1 << 20, func(i int) bool { return i%3 == 0 }},
		// A torrent of one piece, whose 4 blocks are asked in order.
		"the last block": {64 << 10, func(i int) bool { return i == 3 }},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tor := torrenttest.Build("Dropped", 64<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(c.size)})
			seeder := torrenttest.Seed(t, tor, "127.0.0.1")
			seeder.DropFirst(c.drop)
			e := newEngine(t, proxied(torrenttest.StartProxy(t, true, "", "")), 64<<20)
			e.dropWait = 300 * time.Millisecond
			readWhole(t, e, tor, seeder)
			if dropped := seeder.Dropped(); dropped == 0 || seeder.Cancels() < dropped || seeder.MostAsked() < 2 {
				t.Fatalf("%d requests dropped, %d cancelled, a block asked %d times at most", dropped, seeder.Cancels(), seeder.MostAsked())
			}
			if seeder.Open() != 1 {
				t.Fatal("the peer, which sends the blocks asked again, was given up")
			}
		})
	}
}

// A slow peer, the torrent's only one, whose last blocks asked wait longer
// than dropWait, is not taken for one that drops them: none of its
// requests is cancelled, none asked twice, and it is kept.
func TestASlowPeerIsNotTakenForOneThatDrops(t *testing.T) {
	t.Parallel()
	tor := torrenttest.Build("Slow", 64<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(512 << 10)})
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	// The 32 blocks are asked at once: the last waits 32 times 150 ms,
	// longer than dropWait.
	seeder.Delay(150 * time.Millisecond)
	e := newEngine(t, proxied(torrenttest.StartProxy(t, true, "", "")), 64<<20)
	e.dropWait = 3 * time.Second
	readWhole(t, e, tor, seeder)
	if seeder.Cancels() != 0 || seeder.MostAsked() != 1 || seeder.Open() != 1 {
		t.Fatalf("%d requests cancelled, a block asked %d times, %d connections open", seeder.Cancels(), seeder.MostAsked(), seeder.Open())
	}
}

// A peer whose connection stalls, sending nothing for a while with blocks
// asked of it, as a uTP connection does on a loaded link, is not taken for
// one that drops: none of its requests is cancelled, no block asked twice,
// and the file is read whole once it sends again.
func TestAStalledPeerIsNotAskedTwice(t *testing.T) {
	t.Parallel()
	tor := torrenttest.Build("Stalled", 64<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(2 << 20)})
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	e := newEngine(t, proxied(torrenttest.StartProxy(t, true, "", "")), 64<<20)
	e.dropWait = 300 * time.Millisecond
	go func() {
		// Past 256 KiB, the seeder holds what it is asked for 2 seconds,
		// longer than stallWait is here.
		for seeder.Sent() < 256<<10 {
			time.Sleep(time.Millisecond)
		}
		seeder.Hold()
		time.Sleep(2 * time.Second)
		seeder.Release()
	}()
	readWhole(t, e, tor, seeder)
	if seeder.Cancels() != 0 || seeder.MostAsked() != 1 || seeder.Open() != 1 {
		t.Fatalf("%d requests cancelled, a block asked %d times, %d connections open", seeder.Cancels(), seeder.MostAsked(), seeder.Open())
	}
}

// A peer that drops every REQUEST is given up once it sent no block for
// idleTimeout: when it sends nothing at all, by the read deadline; when it
// sends keep-alives, by reask. It is asked for no block twice, and waits
// the longest before it is dialed again.
func TestAPeerThatSendsNoBlockIsGivenUp(t *testing.T) {
	t.Parallel()
	for name, keepAlive := range map[string]time.Duration{"silent": 0, "keeping alive": 100 * time.Millisecond} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tor := torrenttest.Build("Silent", 64<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(1 << 20)})
			seeder := torrenttest.Seed(t, tor, "127.0.0.1")
			seeder.DropAll()
			seeder.KeepAlive(keepAlive)
			e := newEngine(t, proxied(torrenttest.StartProxy(t, true, "", "")), 64<<20)
			e.dropWait, e.idleTimeout = 400*time.Millisecond, time.Second
			spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address()).URL()})
			if err != nil {
				t.Fatal(err)
			}
			defer e.Play(spec)()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r, err := e.Open(ctx, spec)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			read := make(chan error, 1)
			go func() {
				_, err := r.ReadAt(make([]byte, 1), 0)
				read <- err
			}()
			waitFor(t, "the peer to be asked", func() bool { return seeder.Requests() > 0 })
			asked := time.Now()
			tr := torrentOf(e, tor.Hash)
			waitFor(t, "the peer to be given up", func() bool {
				peers, _ := tr.peerCount()
				return peers == 0 && seeder.Open() == 0
			})
			t.Logf("given up %v after it was asked", time.Since(asked).Round(10*time.Millisecond))
			// Its connection is closed: what it was asked is final.
			if most, requests := seeder.MostAsked(), seeder.Requests(); most != 1 || requests > pipeline {
				t.Fatalf("a block asked %d times, %d requests in all", most, requests)
			}
			tr.mu.Lock()
			c, known := tr.known[seeder.Address()]
			tr.mu.Unlock()
			if !known || c.retryAt.Before(time.Now().Add(maxRedialWait/2)) {
				t.Fatalf("the peer given up is dialed again at %v", c.retryAt)
			}
			cancel()
			if err := <-read; err == nil {
				t.Fatal("the read ended without the peer's blocks")
			}
		})
	}
}

// requestSwarm is a torrent of 2 MiB whose info is known, read from its
// start, and peers having every piece that unchoked Polyfin.
type requestSwarm struct {
	tor *torrenttest.Torrent
	t   *torrent
}

func newRequestSwarm(tb testing.TB) requestSwarm {
	tb.Helper()
	tor := torrenttest.Build("Requests", 64<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(2 << 20)})
	info, err := parseInfo(tor.Info)
	if err != nil {
		tb.Fatal(err)
	}
	tr := newTorrent(fuzzEngine(tb, nil), Spec{Hash: tor.Hash, File: -1}, nil, false)
	tb.Cleanup(tr.cancel)
	tr.info, tr.infoData, tr.have, tr.files = &info, tor.Info, make([]bool, len(info.Pieces)), info.Files
	tr.reads[&Reader{t: tr, file: info.Files[0]}] = readSpan{}
	return requestSwarm{tor: tor, t: tr}
}

// join connects a peer, without asking it anything. The lock is held.
func (s requestSwarm) join(port uint16) *peer {
	address := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), port)
	p := newPeer(s.t, &streamConn{}, address)
	p.choked, p.has = false, make([]bool, len(s.t.info.Pieces))
	for i := range p.has {
		p.has[i] = true
	}
	s.t.peers[address] = p
	return p
}

// deliver has a peer send a block.
func (s requestSwarm) deliver(p *peer, b block) {
	start := int64(b.piece)*int64(s.tor.PieceLength) + b.offset
	s.t.received(p, b.piece, b.offset, s.tor.Data[start:start+b.length])
}

// A request a peer skipped, sending blocks asked after it, is taken as
// dropped dropWait later, cancelled and asked again; a block the peer
// dropped more than maxReasks times is not asked of it again, and goes to
// another peer, the peer being kept: it serves the others.
func TestABlockAPeerKeepsDroppingGoesToAnother(t *testing.T) {
	t.Parallel()
	s := newRequestSwarm(t)
	s.t.mu.Lock()
	p := s.join(6881)
	s.t.fill(p)
	s.t.mu.Unlock()
	first := block{piece: 0, offset: 0, length: blockSize}
	for drop := 1; drop <= maxReasks+1; drop++ {
		// The block asked last arrives: when it was asked before the first
		// one, the peer is asked for another, which arrives next.
		s.t.mu.Lock()
		dropped := p.requests[first]
		s.t.mu.Unlock()
		for dropped.overtaken.IsZero() {
			s.t.mu.Lock()
			var next block
			for b, r := range p.requests {
				if b != first && ((next == block{}) || r.seq > p.requests[next].seq) {
					next = b
				}
			}
			s.t.mu.Unlock()
			if (next == block{}) {
				t.Fatalf("drop %d: nothing asked but the first block", drop)
			}
			s.deliver(p, next)
		}
		s.t.mu.Lock()
		sent(p, msgRequest)
		// Not yet dropWait later: the request is kept.
		s.t.mu.Unlock()
		s.t.reask()
		if cancelled := sent(p, msgCancel); len(cancelled) != 0 {
			t.Fatalf("drop %d: %v cancelled before dropWait", drop, cancelled)
		}
		s.t.mu.Lock()
		dropped.overtaken = time.Now().Add(-dropWait)
		s.t.mu.Unlock()
		s.t.reask()
		cancelled, asked := sent(p, msgCancel), sent(p, msgRequest)
		s.t.mu.Lock()
		_, again := p.requests[first]
		s.t.mu.Unlock()
		if len(cancelled) != 1 || cancelled[0] != first {
			t.Fatalf("drop %d: cancelled %v", drop, cancelled)
		}
		if want := drop <= maxReasks; again != want || (len(asked) > 0 && asked[0] == first) != want {
			t.Fatalf("drop %d: asked again %v, want %v", drop, again, want)
		}
	}
	select {
	case <-p.closed:
		t.Fatal("the peer, which serves the other blocks, was given up")
	default:
	}
	s.t.mu.Lock()
	defer s.t.mu.Unlock()
	other := s.join(6882)
	s.t.fill(other)
	if _, asked := other.requests[first]; !asked {
		t.Fatal("another peer was not asked for the block the first keeps dropping")
	}
}

// A peer that sent blocks, then not a byte, for four times as long as it
// takes to send one, dropWait at least, is asked for one more block, never
// asked yet: nothing is cancelled nor asked twice, as a stalled connection
// would have it, and it is not asked again until it sends. A peer whose
// bytes keep coming, a block on its way, is not silent.
func TestASilentPeerIsAskedForOneMoreBlock(t *testing.T) {
	t.Parallel()
	s := newRequestSwarm(t)
	s.t.mu.Lock()
	p := s.join(6881)
	s.t.fill(p)
	s.t.mu.Unlock()
	s.deliver(p, block{piece: 0, offset: 0, length: blockSize})
	// The block came as it was asked: a coarse clock, as Windows' may be,
	// measures no time for it. The peer sent one all the same.
	s.t.mu.Lock()
	p.pace = 0
	s.t.mu.Unlock()
	silence := func() {
		s.t.mu.Lock()
		p.heard = time.Now().Add(-dropWait)
		p.received.Store(p.heard.UnixNano())
		s.t.mu.Unlock()
	}
	s.t.mu.Lock()
	sent(p, msgRequest)
	pending := len(p.requests)
	// A byte arrived a moment ago: the peer is sending.
	p.heard = time.Now().Add(-dropWait)
	p.received.Store(time.Now().UnixNano())
	s.t.mu.Unlock()
	s.t.reask()
	if asked := sent(p, msgRequest); len(asked) != 0 {
		t.Fatalf("a peer sending was asked for %v", asked)
	}
	silence()
	s.t.reask()
	cancelled, asked := sent(p, msgCancel), sent(p, msgRequest)
	s.t.mu.Lock()
	requests := len(p.requests)
	s.t.mu.Unlock()
	if len(cancelled) != 0 || len(asked) != 1 || requests != pending+1 {
		t.Fatalf("%d cancelled, asked for %v, %d requests of %d", len(cancelled), asked, requests, pending)
	}
	silence()
	s.t.reask()
	if asked := sent(p, msgRequest); len(asked) != 0 {
		t.Fatalf("asked again for %v while the peer sent nothing", asked)
	}
}

// A silent peer that holds the last blocks wanted, none being left to ask
// of it, has its oldest request asked again once it stayed silent for
// stallWait, longer than a connection stalls, not before; that block's
// arrival, which may answer either request, overtakes no other.
func TestASilentPeerHoldingTheLastBlocksIsAskedAgain(t *testing.T) {
	t.Parallel()
	s := newRequestSwarm(t)
	s.t.mu.Lock()
	p := s.join(6881)
	s.t.mu.Unlock()
	// Every block but the last 3 arrived: those are the only requests.
	for {
		s.t.mu.Lock()
		s.t.fill(p)
		var next block
		for b, r := range p.requests {
			if len(p.requests) > 3 && ((next == block{}) || r.seq < p.requests[next].seq) {
				next = b
			}
		}
		s.t.mu.Unlock()
		if (next == block{}) {
			break
		}
		s.deliver(p, next)
	}
	s.t.mu.Lock()
	sent(p, msgRequest)
	var oldest block
	for b, r := range p.requests {
		if (oldest == block{}) || r.seq < p.requests[oldest].seq {
			oldest = b
		}
	}
	wait := func(d time.Duration) {
		p.heard = time.Now().Add(-d)
		p.received.Store(p.heard.UnixNano())
	}
	wait(stallWait - time.Second)
	s.t.mu.Unlock()
	s.t.reask()
	if cancelled := sent(p, msgCancel); len(cancelled) != 0 {
		t.Fatalf("%v cancelled before stallWait", cancelled)
	}
	s.t.mu.Lock()
	p.probed = false
	wait(stallWait)
	s.t.mu.Unlock()
	s.t.reask()
	cancelled, asked := sent(p, msgCancel), sent(p, msgRequest)
	if len(cancelled) != 1 || cancelled[0] != oldest || len(asked) != 1 || asked[0] != oldest {
		t.Fatalf("cancelled %v, asked %v, want the oldest request %v", cancelled, asked, oldest)
	}
	s.deliver(p, oldest)
	s.t.mu.Lock()
	defer s.t.mu.Unlock()
	for b, r := range p.requests {
		if !r.overtaken.IsZero() {
			t.Fatalf("%v overtaken by a block asked again", b)
		}
	}
}

// A request taken as dropped is cancelled and its block asked again,
// first of the peers that may take it, without breaking the endgame's
// rules: no block asked of more than two peers, duplicates only of what a
// read waits for, and within maxDuplicates.
func TestReaskKeepsTheEndgameRules(t *testing.T) {
	t.Parallel()
	s := newEndgameSwarm(t, true)
	sent(s.slow, msgRequest)
	s.wait(10, 1000)
	s.fill()
	if asked := sent(s.fast, msgRequest); len(asked) != 4 {
		t.Fatalf("the fast peer was asked for %v, want the 4 blocks of piece 0", piecesOf(asked))
	}
	// The slow peer skipped its requests dropWait ago.
	s.t.mu.Lock()
	for _, r := range s.slow.requests {
		r.overtaken = time.Now().Add(-dropWait)
	}
	s.t.mu.Unlock()
	s.t.reask()

	cancelled := sent(s.slow, msgCancel)
	fast, slow := sent(s.fast, msgRequest), sent(s.slow, msgRequest)
	s.t.mu.Lock()
	defer s.t.mu.Unlock()
	if len(cancelled) != pipeline {
		t.Fatalf("the slow peer was told %d cancels, want %d", len(cancelled), pipeline)
	}
	// The fast peer, the first that may take them, is asked for those not
	// asked of it; the slow one again, in the endgame, for those of the
	// read.
	if len(fast) != pipeline-4 || piecesOf(fast)[0] != 0 {
		t.Fatalf("the fast peer was asked for %v", piecesOf(fast))
	}
	if pieces := piecesOf(slow); len(slow) != 4 || pieces[0] != 4 {
		t.Fatalf("the slow peer was asked again for %v, want the 4 blocks of piece 0", pieces)
	}
	if s.t.duplicates != 4 {
		t.Fatalf("%d duplicates counted, want 4", s.t.duplicates)
	}
	for index, in := range s.t.inflight {
		for i, asks := range in.asks {
			if asks > 2 || asks > 1 && index != 0 {
				t.Fatalf("block %d of piece %d asked of %d peers", i, index, asks)
			}
		}
	}
	for _, b := range cancelled {
		if s.slow.reasked[b] != 1 {
			t.Fatalf("block %v counted %d times dropped", b, s.slow.reasked[b])
		}
	}
}
