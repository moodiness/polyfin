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

// endgameSwarm is a torrent of 8 pieces of 4 blocks whose info is known,
// with a slow peer that was asked for every block and a fast one that
// joins after it, both having every piece and unchoking Polyfin.
type endgameSwarm struct {
	tor        *torrenttest.Torrent
	t          *torrent
	slow, fast *peer
	reader     *Reader
}

func newEndgameSwarm(tb testing.TB, endgame bool) endgameSwarm {
	tb.Helper()
	tor := torrenttest.Build("Endgame", 64<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(512 << 10)})
	info, err := parseInfo(tor.Info)
	if err != nil {
		tb.Fatal(err)
	}
	e := fuzzEngine(tb, nil)
	e.noEndgame = !endgame
	tr := newTorrent(e, Spec{Hash: tor.Hash, File: -1}, nil, false)
	tb.Cleanup(tr.cancel)
	tr.info, tr.infoData, tr.have, tr.files = &info, tor.Info, make([]bool, len(info.Pieces)), info.Files
	join := func(port uint16) *peer {
		address := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), port)
		p := newPeer(tr, &streamConn{}, address)
		p.choked, p.has = false, make([]bool, len(info.Pieces))
		for i := range p.has {
			p.has[i] = true
		}
		tr.peers[address] = p
		return p
	}
	s := endgameSwarm{tor: tor, t: tr, reader: &Reader{t: tr, file: info.Files[0]}}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	// A read was at the start: the slow peer is asked for the pieces
	// after it, every block of the torrent.
	tr.reads[s.reader] = readSpan{}
	s.slow = join(6881)
	tr.fill(s.slow)
	s.fast = join(6882)
	return s
}

// wait has the reader wait for the bytes from at to end.
func (s endgameSwarm) wait(at, end int64) {
	s.t.mu.Lock()
	defer s.t.mu.Unlock()
	s.t.reads[s.reader] = readSpan{at: at, end: end}
}

// fill asks the fast peer for what it may be asked.
func (s endgameSwarm) fill() {
	s.t.mu.Lock()
	defer s.t.mu.Unlock()
	s.t.fill(s.fast)
}

// sent lists the blocks of the messages of id queued for a peer, and
// forgets them.
func sent(p *peer, id byte) []block {
	p.outMu.Lock()
	defer p.outMu.Unlock()
	var blocks []block
	kept := p.outbox[:0]
	for _, message := range p.outbox {
		if len(message) == 17 && message[4] == id {
			b, _ := readBlock(message[5:])
			blocks = append(blocks, b)
			continue
		}
		kept = append(kept, message)
	}
	p.outbox = kept
	return blocks
}

func piecesOf(blocks []block) map[int]int {
	pieces := map[int]int{}
	for _, b := range blocks {
		pieces[b.piece]++
	}
	return pieces
}

// The endgame asks a fast peer for the blocks of the read that a slow
// peer was asked for, cancels the slow peer's once received, and asks no
// duplicate of a piece no read waits for.
func TestEndgameAsksOnlyWhatTheReadWaitsFor(t *testing.T) {
	t.Parallel()
	s := newEndgameSwarm(t, true)
	if asked := sent(s.slow, msgRequest); len(asked) != pipeline {
		t.Fatalf("the slow peer was asked for %d blocks, want %d", len(asked), pipeline)
	}

	// No read waits: the fast peer is asked for nothing the slow one was.
	s.fill()
	if asked := sent(s.fast, msgRequest); len(asked) != 0 {
		t.Fatalf("with no read waiting, the fast peer was asked for %v", piecesOf(asked))
	}

	// A read waits for the first piece: its blocks only are asked again.
	s.wait(10, 1000)
	s.fill()
	asked := sent(s.fast, msgRequest)
	if pieces := piecesOf(asked); len(asked) != 4 || pieces[0] != 4 {
		t.Fatalf("the fast peer was asked for %v, want the 4 blocks of piece 0", pieces)
	}
	s.t.mu.Lock()
	duplicates := s.t.duplicates
	s.t.mu.Unlock()
	if duplicates != 4 {
		t.Fatalf("%d duplicates counted, want 4", duplicates)
	}

	// The fast peer sends them: the slow peer's are cancelled.
	for _, b := range asked {
		start := int64(b.piece)*int64(s.tor.PieceLength) + b.offset
		s.t.received(s.fast, b.piece, b.offset, s.tor.Data[start:start+b.length])
	}
	cancelled := sent(s.slow, msgCancel)
	if pieces := piecesOf(cancelled); len(cancelled) != 4 || pieces[0] != 4 {
		t.Fatalf("the slow peer was told to cancel %v, want the 4 blocks of piece 0", pieces)
	}
	s.t.mu.Lock()
	have, duplicates := s.t.have[0], s.t.duplicates
	_, left := s.slow.requests[asked[0]]
	s.t.mu.Unlock()
	if !have || duplicates != 0 || left {
		t.Fatalf("piece 0 kept %v, %d duplicates left, still asked of the slow peer %v", have, duplicates, left)
	}

	// The read is over: the pieces the slow peer holds, which no read
	// waits for, are not asked again.
	s.wait(1000, 1000)
	s.fill()
	if asked := sent(s.fast, msgRequest); len(asked) != 0 {
		t.Fatalf("the fast peer was asked for %v, which no read waits for", piecesOf(asked))
	}
}

// The endgame asks maxDuplicates blocks of a second peer at most, and
// none while blocks of the read are not asked yet, or when it is off.
func TestEndgameBounds(t *testing.T) {
	t.Parallel()
	t.Run("duplicates in flight", func(t *testing.T) {
		s := newEndgameSwarm(t, true)
		s.wait(0, 512<<10)
		s.fill()
		if asked := sent(s.fast, msgRequest); len(asked) != maxDuplicates {
			t.Fatalf("the fast peer was asked for %d blocks, want %d", len(asked), maxDuplicates)
		}
	})
	t.Run("blocks not asked yet", func(t *testing.T) {
		s := newEndgameSwarm(t, true)
		// The slow peer is choked: its blocks are asked of others.
		s.t.mu.Lock()
		s.slow.choked = true
		s.t.release(s.slow)
		s.t.mu.Unlock()
		s.wait(0, 64<<10)
		s.fill()
		asked := sent(s.fast, msgRequest)
		s.t.mu.Lock()
		duplicates := s.t.duplicates
		s.t.mu.Unlock()
		if len(asked) != pipeline || duplicates != 0 {
			t.Fatalf("the fast peer was asked for %d blocks, %d of them twice", len(asked), duplicates)
		}
	})
	t.Run("off", func(t *testing.T) {
		s := newEndgameSwarm(t, false)
		s.wait(0, 64<<10)
		s.fill()
		if asked := sent(s.fast, msgRequest); len(asked) != 0 {
			t.Fatalf("without the endgame, the fast peer was asked for %v", piecesOf(asked))
		}
	})
}

// On the local swarm, a read the slow peer was asked for ends with the
// blocks of a peer found next, the slow peer's cancelled, long before the
// slow peer, which sends nothing, is given up.
func TestEndgameFinishesARangeASlowPeerHeld(t *testing.T) {
	t.Parallel()
	tor := torrenttest.Build("Slow", 64<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(1 << 20)})
	slow := torrenttest.Seed(t, tor, "127.0.0.1")
	slow.Hold()
	fast := torrenttest.Seed(t, tor, "127.0.0.1")
	proxy := torrenttest.StartProxy(t, true, "", "")
	e := newEngine(t, proxied(proxy), 64<<20)
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + torrenttest.HTTPTracker(t, "127.0.0.1", slow.Address()).URL()})
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
	started := time.Now()
	buf := make([]byte, 128<<10)
	read := make(chan error, 1)
	go func() {
		_, err := r.ReadAt(buf, 0)
		read <- err
	}()
	waitFor(t, "the slow peer to be asked", func() bool { return slow.Requests() >= pipeline })

	// The fast peer is found next.
	more, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + torrenttest.HTTPTracker(t, "127.0.0.1", fast.Address()).URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(more)()
	if err := <-read; err != nil {
		t.Fatal(err)
	}
	if took := time.Since(started); took >= idleTimeout {
		t.Fatalf("the read took %v, as long as a peer that sends nothing waits to be given up", took)
	}
	if !bytes.Equal(buf, tor.Data[:len(buf)]) {
		t.Fatal("the range read is not the torrent's")
	}
	// The cancels reach the slow peer after the blocks they cancel.
	waitFor(t, "the slow peer's blocks to be cancelled", func() bool { return slow.Cancels() >= len(buf)/blockSize })
	if slow.Sent() != 0 || fast.Sent() < int64(len(buf)) {
		t.Fatalf("the slow peer sent %d bytes, the fast one %d", slow.Sent(), fast.Sent())
	}
}

// BenchmarkSlowPeer measures, on the local swarm, the time to the first
// byte and to the end of a range of 1 MiB that a slow peer, sending a
// block every 100 ms, was asked for before a fast peer was found, with
// and without the endgame:
//
//	go test -run '^$' -bench SlowPeer -benchtime 5x ./internal/torrent
func BenchmarkSlowPeer(b *testing.B) {
	for _, endgame := range []bool{true, false} {
		name := "endgame"
		if !endgame {
			name = "without"
		}
		b.Run(name, func(b *testing.B) {
			var first, end time.Duration
			for range b.N {
				f, r := measureSlowPeer(b, endgame)
				first += f
				end += r
			}
			b.ReportMetric(float64(first.Milliseconds())/float64(b.N), "ms-first-byte")
			b.ReportMetric(float64(end.Milliseconds())/float64(b.N), "ms-range-end")
			b.ReportMetric(0, "ns/op")
		})
	}
}

func measureSlowPeer(b *testing.B, endgame bool) (first, end time.Duration) {
	tor := torrenttest.Build("Slow", 256<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(4 << 20)})
	slow := torrenttest.Seed(b, tor, "127.0.0.1")
	slow.Delay(100 * time.Millisecond)
	fast := torrenttest.Seed(b, tor, "127.0.0.1")
	e := newEngine(b, proxied(torrenttest.StartProxy(b, true, "", "")), 64<<20)
	e.noEndgame = !endgame
	spec, _ := NewSpec(tor.HashHex(), nil, []string{"tracker:" + torrenttest.HTTPTracker(b, "127.0.0.1", slow.Address()).URL()})
	defer e.Play(spec)()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r, err := e.Open(ctx, spec)
	if err != nil {
		b.Fatal(err)
	}
	defer r.Close()
	waitFor(b, "the slow peer to be asked", func() bool { return slow.Requests() >= 2*256/16 })
	more, _ := NewSpec(tor.HashHex(), nil, []string{"tracker:" + torrenttest.HTTPTracker(b, "127.0.0.1", fast.Address()).URL()})
	defer e.Play(more)()
	started := time.Now()
	one := make([]byte, 1)
	if _, err := r.ReadAt(one, 0); err != nil {
		b.Fatal(err)
	}
	first = time.Since(started)
	buf := make([]byte, 1<<20)
	if _, err := io.ReadFull(io.NewSectionReader(r, 0, int64(len(buf))), buf); err != nil {
		b.Fatal(err)
	}
	end = time.Since(started)
	if !bytes.Equal(buf, tor.Data[:len(buf)]) {
		b.Fatal("the range read is not the torrent's")
	}
	return first, end
}
