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
)

const (
	// maxPeers is how many peers a torrent connects to, maxDialing how many
	// it dials at once.
	maxPeers   = 30
	maxDialing = 8
	// pipeline is how many blocks a peer is asked for at once, and
	// requestAge how long a block asked of a peer waits before another
	// peer is asked for it too.
	pipeline   = 32
	requestAge = 15 * time.Second
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
	reads       map[*Reader]int64
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
}

// inflightPiece is a piece being downloaded: its bytes, the blocks
// received, and when each was asked for.
type inflightPiece struct {
	data      []byte
	got       []bool
	requested []time.Time
	left      int
}

func newTorrent(e *Engine, spec Spec, route Route, udp bool) *torrent {
	ctx, cancel := context.WithCancel(context.Background())
	t := &torrent{engine: e, hash: spec.Hash, route: route, udp: udp, ctx: ctx, cancel: cancel,
		trackers: slices.Clone(spec.Trackers), nodes: slices.Clone(spec.Nodes), dht: spec.DHT,
		inflight: map[int]*inflightPiece{}, peers: map[netip.AddrPort]*peer{}, known: map[netip.AddrPort]bool{},
		reads: map[*Reader]int64{}, changed: make(chan struct{}), announces: map[string]error{}}
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
// the torrent too.
func (t *torrent) watch() {
	defer t.recovered("route check", func(err error) {
		t.engine.failed(err)
		t.engine.remove(t, err)
	})
	ticker := time.NewTicker(t.engine.checkEvery)
	defer ticker.Stop()
	chokes := time.NewTicker(chokeEvery)
	defer chokes.Stop()
	for {
		select {
		case <-t.ctx.Done():
			return
		case <-chokes.C:
			t.mu.Lock()
			t.chooseUnchoked()
			t.mu.Unlock()
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
	conn, extensions, err := dialPeer(t.ctx, t.route, address, t.hash, t.engine.peerID, dht)
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
		if in := t.inflight[b.piece]; in != nil {
			in.requested[b.offset/blockSize] = time.Time{}
		}
	}
	clear(p.requests)
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
// the nodes they name and the engine's bootstrap nodes.
func (t *torrent) lookup() {
	defer t.recovered("DHT lookup", nil)
	for {
		t.mu.Lock()
		on, starts := t.usesDHT(), slices.Concat(t.nodes, t.engine.config.Bootstrap)
		t.mu.Unlock()
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
// dhtEvery, past the wait for its start.
func (t *torrent) lookupOnce(starts []string) bool {
	conn, err := t.route.ListenPacket(t.ctx)
	if err != nil {
		return false
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(t.ctx, 2*time.Minute)
	defer cancel()
	t.mu.Lock()
	t.looked = true
	t.mu.Unlock()
	found := false
	_ = lookupPeers(ctx, conn, t.hash, starts, func(peers []netip.AddrPort) {
		found = true
		t.addPeers(peers)
	})
	return found
}

// usesDHT reports whether the torrent is looked up on the DHT: its route
// carries UDP, and its sources ask for it or name no tracker, the DHT
// being then the only way to its peers. The lock is held.
func (t *torrent) usesDHT() bool {
	return t.udp && (t.dht || len(t.trackers) == 0)
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
	// The window ahead leaves room in the store for the pieces read now.
	window := int(max(1, min(max(4, readWindow/length), t.engine.store.budget()/length/2)))
	for d := range window {
		for _, offset := range t.reads {
			add(int(offset/length) + d)
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

// fill asks a peer that unchoked Polyfin for the blocks it lacks, up to
// pipeline. The lock is held.
func (t *torrent) fill(p *peer) {
	if t.info == nil || t.err != nil {
		return
	}
	now := time.Now()
	wants := false
	for _, index := range t.wanted() {
		if index >= len(p.has) || !p.has[index] {
			continue
		}
		wants = true
		if p.choked || len(p.requests) >= pipeline {
			break
		}
		in := t.inflight[index]
		if in == nil {
			size := t.info.pieceSize(index)
			blocks := int((size + blockSize - 1) / blockSize)
			in = &inflightPiece{data: make([]byte, size), got: make([]bool, blocks), requested: make([]time.Time, blocks), left: blocks}
			t.inflight[index] = in
		}
		for i := range in.got {
			if len(p.requests) >= pipeline {
				break
			}
			b := block{piece: index, offset: int64(i) * blockSize, length: min(blockSize, int64(len(in.data))-int64(i)*blockSize)}
			if _, mine := p.requests[b]; in.got[i] || mine || !in.requested[i].IsZero() && now.Sub(in.requested[i]) < requestAge {
				continue
			}
			in.requested[i] = now
			p.requests[b] = now
			p.send(msgRequest, requestPayload(b))
		}
	}
	if wants != p.interested {
		p.interested = wants
		if wants {
			p.send(msgInterested)
		} else {
			p.send(msgNotInterested)
		}
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
		t.refill(p)
		return
	}
	if err := t.engine.store.put(pieceKey{t.hash, index}, in.data); errors.Is(err, errClosed) {
		return
	} else if err != nil {
		t.engine.config.Logger.Warn("A torrent's piece could not be kept", "error", err)
		t.refill(p)
		return
	}
	t.completed(index)
}

// keepBlock copies a block into its piece, and returns the piece and its
// hash once it is whole; until then, it asks the peer for more.
func (t *torrent) keepBlock(p *peer, index int, offset int64, data []byte) (*inflightPiece, Hash, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	in := t.inflight[index]
	b := block{piece: index, offset: offset, length: int64(len(data))}
	delete(p.requests, b)
	if in == nil || offset%blockSize != 0 || offset+int64(len(data)) > int64(len(in.data)) || in.got[offset/blockSize] {
		t.fill(p)
		return nil, Hash{}, false
	}
	copy(in.data[offset:], data)
	in.got[offset/blockSize] = true
	in.left--
	t.downBytes += int64(len(data))
	if in.left > 0 {
		t.fill(p)
		return nil, Hash{}, false
	}
	delete(t.inflight, index)
	return in, t.info.Pieces[index], true
}

// refill asks a peer for the blocks Polyfin lacks.
func (t *torrent) refill(p *peer) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.fill(p)
}

// completed records a piece kept, tells the peers while Polyfin shares,
// and asks them for more.
func (t *torrent) completed(index int) {
	t.mu.Lock()
	defer t.mu.Unlock()
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
		t.fill(p)
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
// reader's context allows.
func (r *Reader) ReadAt(p []byte, off int64) (int, error) {
	t := r.t
	if off >= r.file.Length {
		return 0, io.EOF
	}
	start := r.file.Offset + off
	end := min(start+int64(len(p)), r.file.Offset+r.file.Length)
	n := 0
	for position := start; position < end; {
		t.mu.Lock()
		t.reads[r] = position
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

// open waits for the torrent's info, and returns a reader of its file
// index, -1 for the largest. Past the engine's StartTimeout without it,
// the torrent did not start: a *StartError tells what came of the search
// for its peers. The reader reads under ctx, the wait being bounded alone.
func (t *torrent) open(ctx context.Context, index int) (*Reader, error) {
	started := time.Now()
	deadline := time.NewTimer(t.engine.config.StartTimeout)
	defer deadline.Stop()
	for {
		t.mu.Lock()
		info, err, changed := t.info, t.err, t.changed
		if info != nil && err == nil {
			file, err := info.File(index)
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

// peerCount is how many peers the torrent is connected to.
func (t *torrent) peerCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.peers)
}
