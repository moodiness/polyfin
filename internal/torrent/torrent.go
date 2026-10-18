package torrent

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"io"
	"net/netip"
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
	// dhtEvery is how often the DHT is looked up again, maxNodesLearned
	// how many nodes peers tell are kept.
	dhtEvery        = 5 * time.Minute
	maxNodesLearned = 50
	// announcedPort is the port told to trackers: Polyfin connects out
	// only, as no peer can reach it through a proxy.
	announcedPort = 6881
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
		reads: map[*Reader]int64{}, changed: make(chan struct{})}
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

// watch stops the torrent as soon as its route cannot carry it, or is no
// longer the one set: nothing falls back to another route.
func (t *torrent) watch() {
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

func (t *torrent) dial(address netip.AddrPort) {
	t.mu.Lock()
	dht := t.dht && t.udp
	t.mu.Unlock()
	conn, extensions, err := dialPeer(t.ctx, t.route, address, t.hash, t.engine.peerID, dht)
	t.mu.Lock()
	t.dialing--
	t.signal()
	if err != nil || t.err != nil || t.peers[address] != nil {
		t.mu.Unlock()
		if conn != nil {
			conn.Close()
		}
		return
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
	t.mu.Unlock()
	go p.write()
	err = p.read()
	p.close()
	t.mu.Lock()
	delete(t.peers, address)
	t.release(p)
	t.chooseUnchoked()
	t.signal()
	t.mu.Unlock()
	_ = err
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
	event := "started"
	failures := 0
	for {
		answer, err := announceTo(t.ctx, t.route, tracker, t.announced(event), t.udp)
		if errors.Is(err, errTrackerUDP) {
			return
		}
		wait := time.Minute
		if err != nil {
			failures++
			wait = min(time.Duration(failures)*15*time.Second, 5*time.Minute)
			if t.ctx.Err() == nil {
				t.engine.config.Logger.Debug("A tracker could not be reached", "error", err)
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
	return announce{hash: t.hash, peerID: t.engine.peerID, port: announcedPort, event: event, left: left,
		uploaded: t.upBytes, downloaded: t.downBytes}
}

// lookup looks the torrent's peers up on the DHT while it runs, when its
// sources ask for it and the route carries UDP.
func (t *torrent) lookup() {
	for {
		t.mu.Lock()
		on, starts := t.dht && t.udp, slices.Clone(t.nodes)
		t.mu.Unlock()
		wait := 15 * time.Second
		if on && len(starts) > 0 {
			if conn, err := t.route.ListenPacket(t.ctx); err == nil {
				ctx, cancel := context.WithTimeout(t.ctx, 2*time.Minute)
				err = lookupPeers(ctx, conn, t.hash, starts, t.addPeers)
				cancel()
				conn.Close()
				wait = dhtEvery
			}
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

// handle reads a peer's message.
func (t *torrent) handle(p *peer, id byte, payload []byte) error {
	switch id {
	case msgChoke:
		t.mu.Lock()
		p.choked = true
		t.release(p)
		t.signal()
		t.mu.Unlock()
	case msgUnchoke:
		t.mu.Lock()
		p.choked = false
		t.fill(p)
		t.mu.Unlock()
	case msgInterested, msgNotInterested:
		t.mu.Lock()
		p.peerInterested = id == msgInterested
		t.chooseUnchoked()
		t.mu.Unlock()
	case msgHave:
		if len(payload) != 4 {
			return errors.New("a malformed have")
		}
		index := int(binary.BigEndian.Uint32(payload))
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.info != nil && index >= len(t.info.Pieces) || index > 1<<20 {
			return errors.New("a have past the torrent's pieces")
		}
		for len(p.has) <= index {
			p.has = append(p.has, false)
		}
		p.has[index] = true
		t.fill(p)
	case msgBitfield:
		t.mu.Lock()
		defer t.mu.Unlock()
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
// unchokes it.
func (t *torrent) serve(p *peer, b block) {
	t.mu.Lock()
	ok := p.unchoked && t.info != nil && b.piece < len(t.have) && t.have[b.piece] && b.length > 0 && b.length <= 2*blockSize &&
		b.offset+b.length <= t.info.pieceSize(b.piece)
	t.mu.Unlock()
	if !ok || !t.engine.up.allows() {
		return
	}
	p.outMu.Lock()
	queued := len(p.outbox)
	p.outMu.Unlock()
	if queued > pipeline {
		return
	}
	data := make([]byte, b.length)
	if found, err := t.engine.store.read(pieceKey{t.hash, b.piece}, data, b.offset); !found || err != nil {
		return
	}
	header := make([]byte, 8)
	binary.BigEndian.PutUint32(header, uint32(b.piece))
	binary.BigEndian.PutUint32(header[4:], uint32(b.offset))
	p.send(msgPiece, header, data)
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
	t.mu.Lock()
	in := t.inflight[index]
	b := block{piece: index, offset: offset, length: int64(len(data))}
	delete(p.requests, b)
	if in == nil || offset%blockSize != 0 || offset+int64(len(data)) > int64(len(in.data)) || in.got[offset/blockSize] {
		t.fill(p)
		t.mu.Unlock()
		return
	}
	copy(in.data[offset:], data)
	in.got[offset/blockSize] = true
	in.left--
	t.downBytes += int64(len(data))
	if in.left > 0 {
		t.fill(p)
		t.mu.Unlock()
		return
	}
	delete(t.inflight, index)
	expected := t.info.Pieces[index]
	t.mu.Unlock()
	if sha1.Sum(in.data) != expected {
		t.engine.config.Logger.Debug("A torrent's piece did not match its hash")
		t.mu.Lock()
		t.fill(p)
		t.mu.Unlock()
		return
	}
	if err := t.engine.store.put(pieceKey{t.hash, index}, in.data); errors.Is(err, errClosed) {
		return
	} else if err != nil {
		t.engine.config.Logger.Warn("A torrent's piece could not be kept", "error", err)
		t.mu.Lock()
		t.fill(p)
		t.mu.Unlock()
		return
	}
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
		start := piece * metadataBlock
		if !t.engine.up.allows() || t.infoData == nil || piece < 0 || start >= int64(len(t.infoData)) {
			p.send(msgExtended, []byte{byte(p.metadataID)}, encode(map[string]any{"msg_type": 2, "piece": piece}))
			return nil
		}
		end := min(start+metadataBlock, int64(len(t.infoData)))
		p.send(msgExtended, []byte{byte(p.metadataID)},
			encode(map[string]any{"msg_type": 1, "piece": piece, "total_size": len(t.infoData)}), t.infoData[start:end])
	case 1:
		t.receivedMetadata(int(piece), rest)
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
func (t *torrent) receivedMetadata(piece int, data []byte) {
	if t.info != nil || piece < 0 || piece >= len(t.metadataGot) {
		return
	}
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
// index, -1 for the largest.
func (t *torrent) open(ctx context.Context, index int) (*Reader, error) {
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
			return &Reader{t: t, file: file, ctx: ctx, size: file.Length}, nil
		}
		t.mu.Unlock()
		if err != nil {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
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
