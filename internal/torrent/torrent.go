package torrent

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/torrent/utp"
)

const (
	// maxPeers is how many peers a torrent connects to, maxDialing how many
	// it dials at once.
	maxPeers   = 30
	maxDialing = 8
	// pipeline is how many blocks a peer is asked for at once. dropWait is
	// how long a request a peer skipped, sending blocks asked after it,
	// waits before it is taken as dropped and its block asked again, of
	// another peer or the same; past maxReasks drops of a block, the peer
	// is not asked for it again (see reask).
	pipeline  = 32
	dropWait  = 5 * time.Second
	maxReasks = 2
	// stallWait is how long a peer that sent blocks may then send not a
	// byte before the oldest request it holds is asked again, when no new
	// request can tell whether it dropped it: twice as long as a uTP
	// connection lives on unanswered, longer than any stall of the link.
	// Tests that shorten dropWait shorten it alike.
	stallWait = 30 * time.Second
	// maxDuplicates bounds the blocks of the endgame asked of a second
	// peer and not received yet.
	maxDuplicates = 16
	// readWindow is how far past a read the pieces are fetched, and
	// edgePieces how many pieces of a file's start and end are, for its
	// index.
	readWindow = 16 << 20
	edgePieces = 2
	// unchokeSlots is how many interested peers are shared with at once,
	// while sharing.
	unchokeSlots = 4
	// checkEvery is how often the route is checked while a torrent runs,
	// chokeEvery how often the peers shared with are chosen again.
	checkEvery = 5 * time.Second
	chokeEvery = 10 * time.Second
	// dhtEvery is how often the DHT is looked up again, dhtAgain how soon
	// after a lookup that found no peer or could not be made, and
	// maxNodesLearned how many nodes peers tell are kept.
	dhtEvery        = 5 * time.Minute
	dhtAgain        = 15 * time.Second
	maxNodesLearned = 50
)

// errStopped reports a read of a torrent that stopped: its playback ended.
var errStopped = errors.New("the torrent stopped: its playback ended")

// torrent is a torrent being played: its swarm, its pieces, its readers.
type torrent struct {
	engine *Engine
	hash   Hash
	route  Route
	// udp tells whether the route carries UDP: UDP trackers and the DHT.
	udp    bool
	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	trackers []string
	nodes    []string
	dht      bool
	info     *Info
	infoData []byte
	// metadata is the info dictionary being received from peers, by
	// pieces of metadataBlock.
	metadata    []byte
	metadataGot []bool
	have        []bool
	inflight    map[int]*inflightPiece
	peers       map[netip.AddrPort]*peer
	known       map[netip.AddrPort]bool
	queue       []netip.AddrPort
	dialing     int
	reads       map[*Reader]readSpan
	files       []File
	changed     chan struct{}
	err         error
	upBytes     int64
	downBytes   int64
	refs        int
	linger      *time.Timer
	// announces is what each tracker last answered, nil for peers, and
	// looked whether the DHT was looked up, for a StartError.
	announces map[string]error
	looked    bool

	// duplicates counts the blocks asked of more than one peer, once for
	// each peer past the first.
	duplicates int
	// flags are the flags peer exchange told of peers (pexUTP among
	// them), and pexWaiting the peers it told before the torrent's info
	// tells whether it is private, dialed once it is not.
	flags      map[netip.AddrPort]byte
	pexWaiting []netip.AddrPort
	// stopLookup stops the DHT lookup running, nil when none runs.
	stopLookup context.CancelFunc
	// utpMu guards the uTP socket (see utpSocket): it is opened while
	// the torrent's lock is free.
	utpMu      sync.Mutex
	utpSock    *utp.Socket
	utpStopped bool
	utpRetryAt time.Time
}

// inflightPiece is a piece being downloaded: its bytes, the blocks
// received, when each was last asked for, and of how many peers.
type inflightPiece struct {
	data      []byte
	got       []bool
	requested []time.Time
	asks      []uint8
	left      int
}

// readSpan is what a read waits for: the bytes from at to end, among the
// torrent's. Once read, end is at.
type readSpan struct {
	at, end int64
}

func newTorrent(e *Engine, spec Spec, route Route, udp bool) *torrent {
	ctx, cancel := context.WithCancel(context.Background())
	t := &torrent{engine: e, hash: spec.Hash, route: route, udp: udp, ctx: ctx, cancel: cancel,
		trackers: slices.Clone(spec.Trackers), nodes: slices.Clone(spec.Nodes), dht: spec.DHT,
		inflight: map[int]*inflightPiece{}, peers: map[netip.AddrPort]*peer{}, known: map[netip.AddrPort]bool{},
		flags: map[netip.AddrPort]byte{}, reads: map[*Reader]readSpan{}, changed: make(chan struct{}), announces: map[string]error{}}
	return t
}

// start runs the torrent's swarm until it stops.
func (t *torrent) start() {
	go t.watch()
	go t.connect()
	t.mu.Lock()
	trackers := slices.Clone(t.trackers)
	t.mu.Unlock()
	for _, tracker := range trackers {
		go t.announce(tracker)
	}
	go t.lookup()
}

// merge adds the trackers and nodes of another listing of the torrent.
func (t *torrent) merge(spec Spec) {
	t.mu.Lock()
	var added []string
	for _, tracker := range spec.Trackers {
		if !slices.Contains(t.trackers, tracker) {
			t.trackers = append(t.trackers, tracker)
			added = append(added, tracker)
		}
	}
	for _, node := range spec.Nodes {
		if !slices.Contains(t.nodes, node) {
			t.nodes = append(t.nodes, node)
		}
	}
	t.dht = t.dht || spec.DHT
	t.mu.Unlock()
	for _, tracker := range added {
		go t.announce(tracker)
	}
}

// signal wakes whoever waits for the torrent to change. The lock is held.
func (t *torrent) signal() {
	close(t.changed)
	t.changed = make(chan struct{})
}

// fail stops the torrent at once, telling its readers why.
func (t *torrent) fail(err error) {
	t.mu.Lock()
	if t.err != nil {
		t.mu.Unlock()
		return
	}
	t.err = err
	peers := make([]*peer, 0, len(t.peers))
	for _, p := range t.peers {
		peers = append(peers, p)
	}
	t.signal()
	t.mu.Unlock()
	t.cancel()
	t.stopUTP()
	for _, p := range peers {
		p.close()
	}
}

// recovered is deferred at the top of each goroutine that handles one
// peer connection, tracker announce or DHT lookup, and the route's checks:
// the parsers check what other programs send, and recovered is the last
// line of defence beside them. A panic ends only that goroutine, logged
// once with its stack, then, when set, tells then why; the server and the
// torrent's other exchanges go on.
func (t *torrent) recovered(what string, then func(error)) {
	v := recover()
	if v == nil {
		return
	}
	t.engine.config.Logger.Error("A torrent's "+what+" ended on a panic", "panic", fmt.Sprint(v), "stack", string(debug.Stack()))
	if then != nil {
		then(fmt.Errorf("the torrent's %s ended on a panic: %v", what, v))
	}
}

// watch stops the torrent as soon as its route cannot carry it, or is no
// longer the one set: nothing falls back to another route. A panic stops
// the torrent too. Meanwhile, it chooses the peers shared with, and asks
// again for the blocks peers dropped (see reask).
func (t *torrent) watch() {
	defer t.recovered("route check", func(err error) {
		t.engine.failed(err)
		t.engine.remove(t, err)
	})
	ticker := time.NewTicker(t.engine.checkEvery)
	defer ticker.Stop()
	chokes := time.NewTicker(chokeEvery)
	defer chokes.Stop()
	stale := time.NewTicker(max(t.engine.dropWait/4, 10*time.Millisecond))
	defer stale.Stop()
	for {
		select {
		case <-t.ctx.Done():
			return
		case <-chokes.C:
			t.mu.Lock()
			t.chooseUnchoked()
			t.mu.Unlock()
		case <-stale.C:
			t.reask()
		case <-ticker.C:
			route, err := t.engine.currentRoute()
			if err == nil && route != t.route {
				err = errors.New("the route for torrents changed")
			}
			if err == nil {
				err = t.route.Check(t.ctx)
			}
			if err != nil && t.ctx.Err() == nil {
				t.engine.failed(err)
				t.engine.remove(t, err)
				return
			}
		}
	}
}

// addPeers queues peers to connect to.
func (t *torrent) addPeers(peers []netip.AddrPort) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, address := range peers {
		if !address.IsValid() || address.Port() == 0 || t.known[address] || len(t.known) > 2000 {
			continue
		}
		t.known[address] = true
		t.queue = append(t.queue, address)
	}
	t.signal()
}

// connect keeps the torrent connected to up to maxPeers peers.
func (t *torrent) connect() {
	for {
		t.mu.Lock()
		for len(t.queue) > 0 && t.dialing < maxDialing && len(t.peers)+t.dialing < maxPeers && t.err == nil {
			address := t.queue[0]
			t.queue = t.queue[1:]
			t.dialing++
			go t.dial(address)
		}
		changed := t.changed
		t.mu.Unlock()
		select {
		case <-t.ctx.Done():
			return
		case <-changed:
		}
	}
}

// dial connects to a peer, and reads it until the connection ends.
func (t *torrent) dial(address netip.AddrPort) {
	defer t.recovered("peer connection", nil)
	p := t.join(address)
	if p == nil {
		return
	}
	defer t.leave(p)
	go p.write()
	_ = p.read()
}

// join dials a peer counted in dialing, and adds it to the torrent's
// peers; nil when it does not answer, or is no longer needed. The dial is
// uncounted as it ends, by a panic too.
func (t *torrent) join(address netip.AddrPort) *peer {
	t.mu.Lock()
	dht := t.usesDHT()
	t.mu.Unlock()
	dialed := false
	defer func() {
		if !dialed {
			t.mu.Lock()
			t.dialing--
			t.signal()
			t.mu.Unlock()
		}
	}()
	conn, extensions, err := t.dialPeer(address, dht)
	dialed = true
	t.mu.Lock()
	defer t.mu.Unlock()
	t.dialing--
	t.signal()
	if err != nil || t.err != nil || t.peers[address] != nil {
		if conn != nil {
			conn.Close()
		}
		return nil
	}
	p := newPeer(t, conn, address)
	t.peers[address] = p
	over := "TCP"
	if overUTP(p) {
		over = "uTP"
	}
	t.engine.config.Logger.Debug("A torrent reached a peer", "over", over)
	if extensions {
		size := 0
		if t.engine.up.allows() {
			size = len(t.infoData)
		}
		p.sendExtendedHandshake(size)
	}
	t.sendBitfield(p)
	return p
}

// leave closes a peer's connection, and gives the blocks asked of it to
// the other peers.
func (t *torrent) leave(p *peer) {
	p.close()
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.peers, p.address)
	t.release(p)
	t.chooseUnchoked()
	t.signal()
}

// sendBitfield tells a peer the pieces Polyfin has, while it shares them.
// The lock is held.
func (t *torrent) sendBitfield(p *peer) {
	if t.info == nil || !t.engine.up.allows() || !slices.Contains(t.have, true) {
		return
	}
	bits := make([]byte, (len(t.have)+7)/8)
	for i, has := range t.have {
		if has {
			bits[i/8] |= 0x80 >> (i % 8)
		}
	}
	p.send(msgBitfield, bits)
}

// release forgets the blocks asked of a peer, which others may be asked
// for. The lock is held.
func (t *torrent) release(p *peer) {
	for b := range p.requests {
		t.forget(p, b)
	}
}

// ask asks a peer for block i of an inflight piece. The lock is held.
func (t *torrent) ask(p *peer, in *inflightPiece, i int, b block, now time.Time) {
	if in.asks[i] > 0 {
		t.duplicates++
	}
	in.asks[i]++
	in.requested[i] = now
	if len(p.requests) == 0 {
		p.heard = now
	}
	p.seq++
	p.requests[b] = &request{seq: p.seq}
	p.send(msgRequest, requestPayload(b))
}

// arrived notes a block a peer sent: the requests asked of it before the
// block and still waiting are overtaken, and the peer is heard. The lock
// is held.
func (t *torrent) arrived(p *peer, b block, now time.Time) {
	r := p.requests[b]
	if r == nil {
		return
	}
	for _, other := range p.requests {
		if !r.again && other.seq < r.seq && other.overtaken.IsZero() {
			other.overtaken = now
		}
	}
	if wait := now.Sub(p.heard); !p.paced {
		p.pace, p.paced = wait, true
	} else {
		p.pace = (3*p.pace + wait) / 4
	}
	p.heard, p.probed = now, false
}

// forget forgets a block asked of a peer, received or not; false when it
// was not asked. The lock is held.
func (t *torrent) forget(p *peer, b block) bool {
	if _, asked := p.requests[b]; !asked {
		return false
	}
	delete(p.requests, b)
	if in := t.inflight[b.piece]; in != nil {
		i := b.offset / blockSize
		if in.asks[i] > 1 {
			t.duplicates--
		}
		in.asks[i]--
		if in.asks[i] == 0 {
			in.requested[i] = time.Time{}
		}
	}
	return true
}

// cancelOthers cancels a block asked of peers other than the one that
// sent it. The lock is held.
func (t *torrent) cancelOthers(in *inflightPiece, b block) {
	if in.asks[b.offset/blockSize] == 0 {
		return
	}
	for _, other := range t.peers {
		if t.forget(other, b) {
			other.send(msgCancel, requestPayload(b))
		}
	}
}

// reask asks again for the blocks peers dropped, of the other peers
// first, and of the same peer when none of them takes it: a peer may drop
// a REQUEST without a word, and a torrent it serves alone would wait for
// the block for good. Polyfin does not offer the fast extension (BEP 6),
// so no peer tells a drop with REJECT_REQUEST.
//
// A peer serves requests in order: a request is dropped once blocks asked
// after it arrived and it did not, dropWait later. A slow peer is not
// taken for one that drops: its blocks come in order, however long the
// last ones wait. A peer that dropped the last requests asked of it sends
// nothing after them; nor does one whose connection stalls, as uTP's do
// on a loaded link, while the blocks asked are on their way. So silence
// asks nothing again at first: once the peer sent not a byte for four
// times as long as it takes to send a block, dropWait at least, it is
// asked for one more block, never asked yet, once until it sends again:
// its arrival tells the requests before it were dropped, and a peer that
// only stalls sends it in turn. Only when no such block is left, and the
// peer stayed silent for stallWait, longer than a connection stalls, is
// the oldest request asked again. The CANCEL goes first, so that a peer
// that only queued a request does not send its block twice. A block
// a peer dropped more than maxReasks times is not asked of it again: it
// may not have it, and the others may. A peer is given up, as one that
// sends nothing is after idleTimeout, only when it sent no block asked
// for idleTimeout, keep-alives aside; it stays known, so it is not dialed
// again. A request cancelled is no longer counted as asked, so the
// endgame's duplicates, of the blocks reads wait for, stay within
// maxDuplicates.
func (t *torrent) reask() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.info == nil || t.err != nil {
		return
	}
	now := time.Now()
	var asked []*peer
	again := map[*peer]block{}
	cancel := func(p *peer, b block) {
		t.forget(p, b)
		p.send(msgCancel, requestPayload(b))
	}
	for _, p := range t.peers {
		if len(p.requests) == 0 {
			continue
		}
		silent := now.Sub(p.heard)
		if silent >= t.engine.idleTimeout {
			t.engine.config.Logger.Debug("A torrent's peer was given up: it sent none of the blocks asked", "for", silent.Round(time.Second))
			p.close()
			t.release(p)
			asked = append(asked, p)
			continue
		}
		dropped := false
		for b, r := range p.requests {
			if !r.overtaken.IsZero() && now.Sub(r.overtaken) >= t.engine.dropWait {
				p.reasked[b]++
				cancel(p, b)
				dropped = true
			}
		}
		last := p.heard
		if received := time.Unix(0, p.received.Load()); received.After(last) {
			last = received
		}
		quiet := now.Sub(last)
		switch {
		case dropped || !p.paced || p.probed || quiet < max(t.engine.dropWait, 4*p.pace):
		case t.probe(p, now):
			p.probed = true
		case quiet >= stallWait/dropWait*t.engine.dropWait:
			var oldest block
			for b, r := range p.requests {
				if (oldest == block{}) || r.seq < p.requests[oldest].seq {
					oldest = b
				}
			}
			cancel(p, oldest)
			again[p] = oldest
			p.probed, dropped = true, true
		}
		if dropped {
			asked = append(asked, p)
		}
	}
	if len(asked) == 0 {
		return
	}
	defer func() {
		for p, b := range again {
			if r := p.requests[b]; r != nil {
				r.again = true
			}
		}
	}()
	for _, p := range t.peers {
		if !slices.Contains(asked, p) {
			t.fill(p)
		}
	}
	for _, p := range asked {
		select {
		case <-p.closed:
		default:
			t.fill(p)
		}
	}
}

// probe asks a silent peer for one more block, past its pipeline: the
// first it has of those wanted, asked of no peer yet; false when none is
// left. The lock is held.
func (t *torrent) probe(p *peer, now time.Time) bool {
	for _, index := range t.wanted() {
		if index >= len(p.has) || !p.has[index] {
			continue
		}
		in := t.inflightOf(index)
		for i := range in.got {
			b := blockOf(in, index, i)
			if !in.got[i] && in.asks[i] == 0 && p.reasked[b] <= maxReasks {
				t.ask(p, in, i, b, now)
				return true
			}
		}
	}
	return false
}

// announce asks a tracker for peers until the torrent stops, then tells it
// the torrent stopped.
func (t *torrent) announce(tracker string) {
	defer t.recovered("tracker announce", nil)
	event := "started"
	failures := 0
	for {
		answer, err := announceTo(t.ctx, t.route, tracker, t.announced(event), t.udp)
		if errors.Is(err, errTrackerUDP) {
			return
		}
		if t.ctx.Err() == nil {
			t.mu.Lock()
			t.announces[tracker] = err
			t.mu.Unlock()
		}
		wait := time.Minute
		if err != nil {
			failures++
			wait = min(time.Duration(failures)*15*time.Second, 5*time.Minute)
			if t.ctx.Err() == nil {
				t.engine.config.Logger.Debug("A tracker could not be reached", "tracker", trackerHost(tracker), "error", err)
			}
		} else {
			failures = 0
			event = ""
			t.addPeers(answer.peers)
			wait = min(max(answer.interval, time.Minute), 30*time.Minute)
		}
		timer := time.NewTimer(wait)
		select {
		case <-t.ctx.Done():
			timer.Stop()
			if event == "" && !t.failed() {
				// The tracker forgets Polyfin, over the route still up.
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_, _ = announceTo(ctx, t.route, tracker, t.announced("stopped"), t.udp)
				cancel()
			}
			return
		case <-timer.C:
		}
	}
}

// announced is what the torrent tells trackers now.
func (t *torrent) announced(event string) announce {
	t.mu.Lock()
	defer t.mu.Unlock()
	left := int64(1)
	if t.info != nil {
		left = t.info.Length
	}
	return announce{hash: t.hash, peerID: t.engine.peerID, port: announcePort(t.engine.peerID), event: event, left: left,
		uploaded: t.upBytes, downloaded: t.downBytes}
}

// lookup looks the torrent's peers up on the DHT while it runs, when the
// route carries UDP and its sources ask for it or name no tracker, from
// the nodes they name and the engine's bootstrap nodes. It ends once the
// torrent's info tells it is private (BEP 27).
func (t *torrent) lookup() {
	defer t.recovered("DHT lookup", nil)
	for {
		t.mu.Lock()
		private := t.private()
		on, starts := t.usesDHT(), slices.Concat(t.nodes, t.engine.config.Bootstrap)
		t.mu.Unlock()
		if private {
			return
		}
		wait := dhtAgain
		if on && len(starts) > 0 && t.lookupOnce(starts) {
			wait = dhtEvery
		}
		timer := time.NewTimer(wait)
		select {
		case <-t.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// lookupOnce looks the torrent's peers up on the DHT from starts, over a
// socket of the route; false when the route opens none, or the lookup
// found no peer: a node's answer lost or later than dhtWait, as a
// datagram may be, would otherwise keep the torrent off the DHT for
// dhtEvery, past the wait for its start. The lookup stops as soon as the
// torrent's info tells it is private.
func (t *torrent) lookupOnce(starts []string) bool {
	conn, err := t.route.ListenPacket(t.ctx)
	if err != nil {
		return false
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(t.ctx, 2*time.Minute)
	defer cancel()
	t.mu.Lock()
	if !t.usesDHT() {
		t.mu.Unlock()
		return true
	}
	t.looked = true
	t.stopLookup = cancel
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		t.stopLookup = nil
		t.mu.Unlock()
	}()
	found := false
	_ = lookupPeers(ctx, conn, t.hash, starts, func(peers []netip.AddrPort) {
		found = true
		t.addPeers(peers)
	})
	return found
}

// usesDHT reports whether the torrent is looked up on the DHT: its route
// carries UDP, and its sources ask for it or name no tracker, the DHT
// being then the only way to its peers, and it is not private. The lock
// is held.
func (t *torrent) usesDHT() bool {
	return t.udp && (t.dht || len(t.trackers) == 0) && !t.private()
}

// private reports whether the torrent's info is known and tells it is
// private (BEP 27): no DHT, no peer exchange. The lock is held.
func (t *torrent) private() bool {
	return t.info != nil && t.info.Private
}

// handle reads a peer's message. Its locks are released by defer, so that
// a panic, which recovered turns into the connection's end, leaves the
// torrent's lock free for its other peers.
func (t *torrent) handle(p *peer, id byte, payload []byte) error {
	switch id {
	case msgChoke:
		t.mu.Lock()
		defer t.mu.Unlock()
		p.choked = true
		t.release(p)
		t.signal()
	case msgUnchoke:
		t.mu.Lock()
		defer t.mu.Unlock()
		p.choked = false
		t.fill(p)
	case msgInterested, msgNotInterested:
		t.mu.Lock()
		defer t.mu.Unlock()
		p.peerInterested = id == msgInterested
		t.chooseUnchoked()
	case msgHave:
		if len(payload) != 4 {
			return errors.New("a malformed have")
		}
		index := binary.BigEndian.Uint32(payload)
		t.mu.Lock()
		defer t.mu.Unlock()
		// A torrent has maxPieces pieces at most, which bounds what a
		// peer is known to have before the torrent's info is.
		if index >= maxPieces || t.info != nil && int(index) >= len(t.info.Pieces) {
			return errors.New("a have past the torrent's pieces")
		}
		for len(p.has) <= int(index) {
			p.has = append(p.has, false)
		}
		p.has[index] = true
		t.fill(p)
	case msgBitfield:
		t.mu.Lock()
		defer t.mu.Unlock()
		// A bit a piece: the torrent's pieces once its info is known,
		// maxPieces at most before.
		pieces := maxPieces
		if t.info != nil {
			pieces = len(t.info.Pieces)
		}
		if len(payload) > (pieces+7)/8 {
			return errors.New("a bitfield past the torrent's pieces")
		}
		p.has = make([]bool, len(payload)*8)
		for i := range p.has {
			p.has[i] = payload[i/8]&(0x80>>(i%8)) != 0
		}
		t.fill(p)
	case msgRequest:
		b, ok := readBlock(payload)
		if !ok {
			return errors.New("a malformed request")
		}
		t.serve(p, b)
	case msgPiece:
		if len(payload) < 8 {
			return errors.New("a malformed piece")
		}
		t.received(p, int(binary.BigEndian.Uint32(payload)), int64(binary.BigEndian.Uint32(payload[4:])), payload[8:])
	case msgPort:
		if len(payload) == 2 {
			t.learnNode(p, binary.BigEndian.Uint16(payload))
		}
	case msgExtended:
		if len(payload) == 0 {
			return errors.New("a malformed extended message")
		}
		return t.extended(p, payload[0], payload[1:])
	}
	return nil
}

// learnNode keeps the DHT node a peer runs, to look the torrent up from.
func (t *torrent) learnNode(p *peer, port uint16) {
	if port == 0 {
		return
	}
	address := netip.AddrPortFrom(p.address.Addr(), port).String()
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.nodes) < maxNodesLearned && !slices.Contains(t.nodes, address) {
		t.nodes = append(t.nodes, address)
	}
}

// serve answers a peer's request for a block, while Polyfin shares and
// unchokes it, and fewer than pipeline messages wait for the peer.
func (t *torrent) serve(p *peer, b block) {
	if !t.shares(p, b) || !t.engine.up.allows() || p.queued() > pipeline {
		return
	}
	// The block is 2*blockSize at most.
	data := make([]byte, b.length)
	if found, err := t.engine.store.read(pieceKey{t.hash, b.piece}, data, b.offset); !found || err != nil {
		return
	}
	header := make([]byte, 8)
	binary.BigEndian.PutUint32(header, uint32(b.piece))
	binary.BigEndian.PutUint32(header[4:], uint32(b.offset))
	p.send(msgPiece, header, data)
}

// shares reports whether a peer may have a block: Polyfin unchokes it,
// and has the piece, of which the block is 2*blockSize at most.
func (t *torrent) shares(p *peer, b block) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return p.unchoked && t.info != nil && b.piece >= 0 && b.piece < len(t.have) && t.have[b.piece] && b.length > 0 &&
		b.length <= 2*blockSize && b.offset+b.length <= t.info.pieceSize(b.piece)
}

// uploaded counts bytes shared.
func (t *torrent) uploaded(n int64) {
	t.mu.Lock()
	t.upBytes += n
	t.mu.Unlock()
	t.engine.countUpload(n)
}

// chooseUnchoked unchokes the interested peers, up to unchokeSlots, while
// Polyfin shares, and chokes every one otherwise. The lock is held.
func (t *torrent) chooseUnchoked() {
	sharing := t.engine.up.allows() && t.info != nil
	slots := 0
	for _, p := range t.peers {
		switch {
		case p.unchoked && (!sharing || !p.peerInterested):
			p.unchoked = false
			p.send(msgChoke)
		case p.unchoked:
			slots++
		}
	}
	if !sharing {
		return
	}
	for _, p := range t.peers {
		if slots >= unchokeSlots {
			return
		}
		if p.peerInterested && !p.unchoked {
			p.unchoked = true
			p.send(msgUnchoke)
			slots++
		}
	}
}

// wanted lists the pieces to fetch, most urgent first: those at and after
// each read, nearest first, then the start and end of each file read.
// The lock is held.
func (t *torrent) wanted() []int {
	if t.info == nil {
		return nil
	}
	length := t.info.PieceLength
	count := len(t.info.Pieces)
	seen := map[int]bool{}
	var order []int
	add := func(i int) {
		if i >= 0 && i < count && !t.have[i] && !seen[i] {
			seen[i] = true
			order = append(order, i)
		}
	}
	window := t.window()
	for d := range window {
		for _, span := range t.reads {
			add(int(span.at/length) + d)
		}
	}
	for _, f := range t.files {
		first := int(f.Offset / length)
		last := int((f.Offset + max(f.Length, 1) - 1) / length)
		for d := range edgePieces {
			add(first + d)
			add(last - d)
		}
	}
	return order
}

// window is how many pieces past a read are fetched: readWindow, within
// room in the store for the pieces read now. The lock is held.
func (t *torrent) window() int {
	length := t.info.PieceLength
	return int(max(1, min(max(4, readWindow/length), t.engine.store.budget()/length/2)))
}

// needed lists the pieces the reads wait for, those of their spans, up
// to a window each. The lock is held.
func (t *torrent) needed() []int {
	length := t.info.PieceLength
	var pieces []int
	for _, span := range t.reads {
		if span.end <= span.at {
			continue
		}
		first := int(span.at / length)
		last := min(int((span.end-1)/length), first+t.window()-1, len(t.have)-1)
		for i := first; i <= last; i++ {
			if !t.have[i] && !slices.Contains(pieces, i) {
				pieces = append(pieces, i)
			}
		}
	}
	return pieces
}

// blockOf is block i of an inflight piece.
func blockOf(in *inflightPiece, index, i int) block {
	offset := int64(i) * blockSize
	return block{piece: index, offset: offset, length: min(blockSize, int64(len(in.data))-offset)}
}

// inflightOf is the piece index being downloaded, started when it is
// not. The lock is held.
func (t *torrent) inflightOf(index int) *inflightPiece {
	in := t.inflight[index]
	if in == nil {
		size := t.info.pieceSize(index)
		blocks := int((size + blockSize - 1) / blockSize)
		in = &inflightPiece{data: make([]byte, size), got: make([]bool, blocks), requested: make([]time.Time, blocks),
			asks: make([]uint8, blocks), left: blocks}
		t.inflight[index] = in
	}
	return in
}

// fill asks a peer that unchoked Polyfin for the blocks it lacks, up to
// pipeline: those of the endgame first, then the pieces wanted in order.
// The lock is held.
func (t *torrent) fill(p *peer) {
	if t.info == nil || t.err != nil {
		return
	}
	now := time.Now()
	// The peer is told Polyfin is interested before it is asked anything,
	// endgame duplicates included: a peer that unchoked first may drop a
	// REQUEST from a peer that did not say it is interested. The endgame's
	// pieces are among those wanted.
	wanted := t.wanted()
	wants := slices.ContainsFunc(wanted, func(index int) bool { return index < len(p.has) && p.has[index] })
	if wants != p.interested {
		p.interested = wants
		if wants {
			p.send(msgInterested)
		} else {
			p.send(msgNotInterested)
		}
	}
	if !wants {
		return
	}
	t.endgame(p, now)
	for _, index := range wanted {
		if index >= len(p.has) || !p.has[index] {
			continue
		}
		if p.choked || len(p.requests) >= pipeline {
			break
		}
		in := t.inflightOf(index)
		for i := range in.got {
			if len(p.requests) >= pipeline {
				break
			}
			// A block asked is left to its peer until reask cancels it:
			// only the endgame asks a block of two peers at once. A block
			// the peer dropped past maxReasks times is left to the others.
			if in.got[i] || in.asks[i] > 0 {
				continue
			}
			b := blockOf(in, index, i)
			if p.reasked[b] > maxReasks {
				continue
			}
			t.ask(p, in, i, b, now)
		}
	}
}

// endgame asks a peer, once every block the reads wait for is asked, for
// those other peers were asked for, the longest waiting first: a slow
// peer's blocks come from the first to send them, the others cancelled.
// A block is asked of two peers at most, maxDuplicates blocks at once,
// and the pieces no read waits for are left to the peers asked. The lock
// is held.
func (t *torrent) endgame(p *peer, now time.Time) {
	if t.engine.noEndgame || p.choked || len(p.requests) >= pipeline || t.duplicates >= maxDuplicates {
		return
	}
	type held struct {
		in    *inflightPiece
		i     int
		b     block
		since time.Time
	}
	var blocks []held
	for _, index := range t.needed() {
		in := t.inflight[index]
		if in == nil {
			// A piece not asked yet: not the endgame.
			return
		}
		for i, got := range in.got {
			switch {
			case got:
				continue
			case in.asks[i] == 0:
				return
			case in.asks[i] > 1 || index >= len(p.has) || !p.has[index]:
				continue
			}
			b := blockOf(in, index, i)
			if _, mine := p.requests[b]; !mine && p.reasked[b] <= maxReasks {
				blocks = append(blocks, held{in: in, i: i, b: b, since: in.requested[i]})
			}
		}
	}
	slices.SortFunc(blocks, func(a, b held) int { return a.since.Compare(b.since) })
	for _, h := range blocks {
		if len(p.requests) >= pipeline || t.duplicates >= maxDuplicates {
			return
		}
		t.ask(p, h.in, h.i, h.b, now)
	}
}

// received keeps a block a peer sent, and the piece once it is whole and
// matches its hash.
func (t *torrent) received(p *peer, index int, offset int64, data []byte) {
	in, expected, whole := t.keepBlock(p, index, offset, data)
	if !whole {
		return
	}
	if sha1.Sum(in.data) != expected {
		t.engine.config.Logger.Debug("A torrent's piece did not match its hash")
		t.discard(p, index)
		return
	}
	if err := t.engine.store.put(pieceKey{t.hash, index}, in.data); errors.Is(err, errClosed) {
		return
	} else if err != nil {
		t.engine.config.Logger.Warn("A torrent's piece could not be kept", "error", err)
		t.discard(p, index)
		return
	}
	t.completed(index)
}

// keepBlock copies a block into its piece, cancels it where others were
// asked for it too, and returns the piece and its hash once it is whole;
// until then, it asks the peer for more.
func (t *torrent) keepBlock(p *peer, index int, offset int64, data []byte) (*inflightPiece, Hash, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	in := t.inflight[index]
	b := block{piece: index, offset: offset, length: int64(len(data))}
	t.arrived(p, b, time.Now())
	t.forget(p, b)
	if in == nil || offset%blockSize != 0 || offset >= int64(len(in.data)) || in.got[offset/blockSize] ||
		b != blockOf(in, index, int(offset/blockSize)) {
		t.fill(p)
		return nil, Hash{}, false
	}
	copy(in.data[offset:], data)
	in.got[offset/blockSize] = true
	in.left--
	t.downBytes += int64(len(data))
	t.cancelOthers(in, b)
	for _, other := range t.peers {
		delete(other.reasked, b)
	}
	if in.left > 0 {
		t.fill(p)
		return nil, Hash{}, false
	}
	// The piece stays in flight, whole, until it is kept or found wrong:
	// meanwhile, none of its blocks is asked again.
	return in, t.info.Pieces[index], true
}

// discard drops a whole piece found wrong, or that could not be kept, and
// asks the peer for the blocks Polyfin lacks.
func (t *torrent) discard(p *peer, index int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.inflight, index)
	t.fill(p)
}

// completed records a piece kept, tells the peers while Polyfin shares,
// and asks them for more.
func (t *torrent) completed(index int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.inflight, index)
	if t.err != nil {
		return
	}
	t.have[index] = true
	t.signal()
	sharing := t.engine.up.allows()
	have := make([]byte, 4)
	binary.BigEndian.PutUint32(have, uint32(index))
	for _, other := range t.peers {
		if sharing {
			other.send(msgHave, have)
		}
		t.fill(other)
	}
}

// evicted forgets a piece the store removed.
func (t *torrent) evicted(index int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if index < len(t.have) {
		t.have[index] = false
	}
}

// extended reads a message of the extension protocol.
func (t *torrent) extended(p *peer, id byte, payload []byte) error {
	if id == extPEX {
		t.mu.Lock()
		defer t.mu.Unlock()
		return t.receivedPEX(p, payload)
	}
	d, rest, err := decodeDict(payload)
	if err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if id == 0 {
		if n, ok := num(dict(d, "m"), "ut_metadata"); ok && n > 0 && n < 256 {
			p.metadataID = int(n)
		}
		if n, ok := num(d, "metadata_size"); ok && n > 0 && n <= maxInfoSize {
			p.metadataSize = int(n)
		}
		t.askMetadata(p)
		return nil
	}
	if id != extMetadata {
		return nil
	}
	kind, _ := num(d, "msg_type")
	piece, _ := num(d, "piece")
	switch kind {
	case 0:
		if p.metadataID == 0 {
			return nil
		}
		// The piece is checked against the info dictionary's pieces of
		// metadataBlock before it is multiplied, which a large one would
		// overflow. Like blocks, metadata is answered while fewer than
		// pipeline messages wait for the peer.
		pieces := int64(len(t.infoData)+metadataBlock-1) / metadataBlock
		if !t.engine.up.allows() || piece < 0 || piece >= pieces || p.queued() > pipeline {
			p.send(msgExtended, []byte{byte(p.metadataID)}, encode(map[string]any{"msg_type": 2, "piece": piece}))
			return nil
		}
		start := piece * metadataBlock
		end := min(start+metadataBlock, int64(len(t.infoData)))
		p.send(msgExtended, []byte{byte(p.metadataID)},
			encode(map[string]any{"msg_type": 1, "piece": piece, "total_size": len(t.infoData)}), t.infoData[start:end])
	case 1:
		t.receivedMetadata(piece, rest)
	}
	return nil
}

// askMetadata asks a peer for the pieces of the info dictionary Polyfin
// lacks. The lock is held.
func (t *torrent) askMetadata(p *peer) {
	if t.info != nil || p.metadataID == 0 || p.metadataSize == 0 {
		return
	}
	if t.metadata == nil || len(t.metadata) != p.metadataSize {
		t.metadata = make([]byte, p.metadataSize)
		t.metadataGot = make([]bool, (p.metadataSize+metadataBlock-1)/metadataBlock)
	}
	for i, got := range t.metadataGot {
		if !got {
			p.send(msgExtended, []byte{byte(p.metadataID)}, encode(map[string]any{"msg_type": 0, "piece": i}))
		}
	}
}

// receivedMetadata keeps a piece of the info dictionary, and reads it once
// whole. The lock is held.
func (t *torrent) receivedMetadata(index int64, data []byte) {
	if t.info != nil || index < 0 || index >= int64(len(t.metadataGot)) {
		return
	}
	piece := int(index)
	start := piece * metadataBlock
	if len(data) != min(metadataBlock, len(t.metadata)-start) {
		return
	}
	copy(t.metadata[start:], data)
	t.metadataGot[piece] = true
	if slices.Contains(t.metadataGot, false) {
		return
	}
	info, err := checkInfo(t.hash, t.metadata)
	if err != nil {
		// A peer sent wrong pieces: start over.
		clear(t.metadataGot)
		return
	}
	t.info, t.infoData = &info, t.metadata
	t.metadata, t.metadataGot = nil, nil
	t.have = make([]bool, len(info.Pieces))
	for _, p := range t.peers {
		if len(p.has) > len(info.Pieces) {
			p.has = p.has[:len(info.Pieces)]
		}
		if info.Private && p.extended {
			// Peer exchange, offered while the info was not known, is
			// turned off.
			p.send(msgExtended, []byte{0}, encode(map[string]any{"m": map[string]any{"ut_pex": 0}}))
		}
		t.fill(p)
	}
	t.queuePEX()
	if info.Private && t.stopLookup != nil {
		// A private torrent is not looked up on the DHT.
		t.stopLookup()
	}
	t.signal()
}

// Reader reads a file of a torrent being played, as a source of the
// source cache: each read asks for its pieces first.
type Reader struct {
	t    *torrent
	file File
	ctx  context.Context
	size int64
}

// Size is the file's size.
func (r *Reader) Size() int64 { return r.size }

// ReadAt reads the file at off, waiting for its pieces as long as the
// reader's context allows. What it waits for is the endgame's.
func (r *Reader) ReadAt(p []byte, off int64) (int, error) {
	t := r.t
	if off >= r.file.Length {
		return 0, io.EOF
	}
	start := r.file.Offset + off
	end := min(start+int64(len(p)), r.file.Offset+r.file.Length)
	defer func() {
		t.mu.Lock()
		if span, reading := t.reads[r]; reading {
			t.reads[r] = readSpan{at: span.at, end: span.at}
		}
		t.mu.Unlock()
	}()
	n := 0
	for position := start; position < end; {
		t.mu.Lock()
		t.reads[r] = readSpan{at: position, end: end}
		length := t.info.PieceLength
		index := int(position / length)
		within := position - int64(index)*length
		chunk := min(end-position, length-within)
		has := t.have[index]
		err, changed := t.err, t.changed
		if !has {
			for _, peer := range t.peers {
				t.fill(peer)
			}
		}
		t.mu.Unlock()
		if err != nil {
			return n, err
		}
		if has {
			found, err := t.engine.store.read(pieceKey{t.hash, index}, p[n:n+int(chunk)], within)
			if err != nil {
				return n, err
			}
			if found {
				n += int(chunk)
				position += chunk
			} else {
				// The store removed the piece meanwhile: fetch it again.
				t.evicted(index)
			}
			continue
		}
		select {
		case <-r.ctx.Done():
			return n, r.ctx.Err()
		case <-changed:
		}
	}
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// Close ends the reader's reads.
func (r *Reader) Close() error {
	r.t.mu.Lock()
	delete(r.t.reads, r)
	r.t.mu.Unlock()
	return nil
}

// open waits for the torrent's info, and returns a reader of the file
// spec plays (see Info.File). Past the engine's StartTimeout without it,
// the torrent did not start: a *StartError tells what came of the search
// for its peers. The reader reads under ctx, the wait being bounded alone.
func (t *torrent) open(ctx context.Context, spec Spec) (*Reader, error) {
	started := time.Now()
	deadline := time.NewTimer(t.engine.config.StartTimeout)
	defer deadline.Stop()
	for {
		t.mu.Lock()
		info, err, changed := t.info, t.err, t.changed
		if info != nil && err == nil {
			file, err := info.File(spec)
			if err != nil {
				t.mu.Unlock()
				return nil, err
			}
			if !slices.Contains(t.files, file) {
				t.files = append(t.files, file)
				for _, p := range t.peers {
					t.fill(p)
				}
			}
			t.mu.Unlock()
			t.engine.startedOne()
			return &Reader{t: t, file: file, ctx: ctx, size: file.Length}, nil
		}
		t.mu.Unlock()
		if err != nil {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			t.mu.Lock()
			err := t.startError(time.Since(started))
			t.mu.Unlock()
			t.engine.startFailed(err)
			return nil, err
		case <-changed:
		}
	}
}

// failed reports whether the torrent stopped because its route failed.
func (t *torrent) failed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.err != nil && t.err != errStopped
}

// peerCount is how many peers the torrent is connected to, and how many of
// them over uTP.
func (t *torrent) peerCount() (all, viaUTP int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.peers), t.utpPeers()
}

// utpPeers counts the peers reached over uTP. The lock is held.
func (t *torrent) utpPeers() int {
	n := 0
	for _, p := range t.peers {
		if overUTP(p) {
			n++
		}
	}
	return n
}
