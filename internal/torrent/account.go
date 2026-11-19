package torrent

import (
	"crypto/sha1"
	"net/netip"
	"slices"
	"time"
)

// What a torrent holds against its peers, and how it makes room for
// others. A peer is banned only on proof: bytes it alone sent that do not
// match the torrent's hashes. Nothing else is counted against a peer: one
// that is slow, or holds a place for nothing, gives its place and is
// dialed again later.

// spoiltPiece is a piece that failed its hash with blocks of several
// peers: who sent each block of that attempt, and the block's SHA-1. Which
// of them sent wrong bytes is known once the piece passes, so it is asked
// again of one peer at a time, only (see blame).
type spoiltPiece struct {
	from []netip.Addr
	sums [][sha1.Size]byte
	only *peer
}

// banKey is what a ban holds of a peer's address: the address in IPv4,
// its /64 in IPv6, which one host has whole. The port is left out: Polyfin
// dials its peers, and a port more costs nothing to list.
func banKey(address netip.Addr) netip.Addr {
	if address.Is6() {
		if prefix, err := address.WithZone("").Prefix(64); err == nil {
			return prefix.Addr()
		}
	}
	return address
}

// isBanned reports whether a peer's address is not dialed (see ban). The
// lock is held.
func (t *torrent) isBanned(address netip.Addr) bool {
	_, banned := t.banned[banKey(address)]
	return banned
}

// ban closes the peers connected on an address that sent wrong bytes, and
// has it not dialed again while the torrent runs, nor first at its next
// run (see Engine.keepLocked). Past maxBanned addresses, the peer is
// closed all the same, and no longer remembered. The lock is held.
func (t *torrent) ban(address netip.Addr) {
	key := banKey(address)
	if t.banned == nil {
		t.banned = map[netip.Addr]struct{}{}
	}
	if len(t.banned) < maxBanned {
		t.banned[key] = struct{}{}
	}
	for _, p := range t.peers {
		if banKey(p.address.Addr()) == key {
			t.expel(p)
		}
	}
	t.reached = slices.DeleteFunc(t.reached, func(a netip.AddrPort) bool { return banKey(a.Addr()) == key })
}

// expel closes a peer's connection before its reader ends it (see leave):
// the blocks asked of it are the others' to ask at once, and it is asked
// nothing more, having no piece. The lock is held.
func (t *torrent) expel(p *peer) {
	p.close()
	p.has = nil
	p.dropMetadata()
	t.release(p)
}

// gone reports whether a peer's connection was closed.
func (p *peer) gone() bool {
	select {
	case <-p.closed:
		return true
	default:
		return false
	}
}

// blockData is block i of a piece's bytes.
func blockData(data []byte, i int) []byte {
	return data[i*blockSize : min((i+1)*blockSize, len(data))]
}

// blame tells who sent a whole piece that failed its hash. One peer alone:
// it sent wrong bytes, and is banned. Several, as when a piece of more
// blocks than a pipeline is asked of every peer: none is told apart yet,
// and none is disconnected, since all but one may be honest. The blocks'
// SHA-1 are kept with who sent each, maxSpoilt pieces at most, and the
// piece is asked again of one peer at a time (see entrust and spares):
// whole from that peer, it either fails again, which bans that peer, or
// passes, which tells the blocks that were wrong (see acquit).
func (t *torrent) blame(index int, in *inflightPiece) {
	t.mu.Lock()
	var senders []*peer
	for _, p := range in.from {
		if p != nil && !slices.Contains(senders, p) {
			senders = append(senders, p)
		}
	}
	if len(senders) < 2 {
		defer t.mu.Unlock()
		for _, p := range senders {
			t.engine.config.Logger.Debug("A torrent's peer is not dialed again: the piece it sent did not match its hash")
			t.ban(p.address.Addr())
		}
		if s := t.spoilt[index]; s != nil {
			t.entrust(index, s)
		}
		return
	}
	t.mu.Unlock()
	// The piece is whole: none of its bytes changes (see keepBlock).
	s := &spoiltPiece{from: make([]netip.Addr, len(in.from)), sums: make([][sha1.Size]byte, len(in.from))}
	for i, p := range in.from {
		if p != nil {
			s.from[i], s.sums[i] = p.address.Addr(), sha1.Sum(blockData(in.data, i))
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.spoilt == nil {
		t.spoilt = map[int]*spoiltPiece{}
	}
	if t.spoilt[index] == nil && len(t.spoilt) >= maxSpoilt {
		for other := range t.spoilt {
			delete(t.spoilt, other)
			break
		}
	}
	t.spoilt[index] = s
	t.entrust(index, s)
}

// acquit records a whole piece that passed its hash: its senders sent
// good blocks, and, when an earlier attempt at it failed with blocks of
// several peers, those whose block differs from the good one sent wrong
// bytes, and are banned.
func (t *torrent) acquit(index int, in *inflightPiece) {
	t.mu.Lock()
	for _, p := range in.from {
		if p != nil {
			p.good++
		}
	}
	s := t.spoilt[index]
	delete(t.spoilt, index)
	t.mu.Unlock()
	if s == nil {
		return
	}
	var wrong []netip.Addr
	for i, sum := range s.sums {
		if s.from[i].IsValid() && i*blockSize < len(in.data) && sha1.Sum(blockData(in.data, i)) != sum && !slices.Contains(wrong, s.from[i]) {
			wrong = append(wrong, s.from[i])
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, address := range wrong {
		t.engine.config.Logger.Debug("A torrent's peer is not dialed again: a block it sent did not match its piece")
		t.ban(address)
	}
}

// serves reports whether a peer can be asked for a whole piece: it is
// connected, does not choke Polyfin, has the piece, and dropped none of
// its blocks more than maxReasks times. The lock is held.
func (t *torrent) serves(p *peer, index int) bool {
	if t.peers[p.address] != p || p.gone() || p.choked || index >= len(p.has) || !p.has[index] {
		return false
	}
	if in := t.inflight[index]; in != nil {
		for i, got := range in.got {
			if !got && p.reasked[blockOf(in, index, i)] > maxReasks {
				return false
			}
		}
	}
	return true
}

// entrust chooses the peer a piece that failed its hash is asked again
// of: of those that serve it, the one whose blocks made the most pieces
// that passed; none when no peer serves it now (see oversee). The lock is
// held.
func (t *torrent) entrust(index int, s *spoiltPiece) {
	s.only = nil
	for _, p := range t.peers {
		if t.serves(p, index) && (s.only == nil || p.good > s.only.good) {
			s.only = p
		}
	}
}

// spares reports whether a peer is not asked for a block: one it dropped
// more than maxReasks times, or one of a piece asked again of another
// peer alone (see blame). The lock is held.
func (t *torrent) spares(p *peer, b block) bool {
	if p.reasked[b] > maxReasks {
		return true
	}
	s := t.spoilt[b.piece]
	return s != nil && s.only != p
}

// oversee checks the peers asked alone for a piece that failed its hash,
// each time reask runs. One that no longer serves it gives it to another;
// so does one that lets a block of it wait as long as a stalled peer's
// oldest request waits before it is asked again (stallWait), and that one
// is given up as a silent peer is: alone on a piece, it would keep the
// read that waits for it from any other peer. Its requests of the piece
// are cancelled; the blocks it sent stay. The lock is held.
func (t *torrent) oversee(now time.Time) {
	if len(t.spoilt) == 0 {
		return
	}
	wanted := t.wanted()
	for index, s := range t.spoilt {
		if p := s.only; p != nil {
			serves := t.serves(p, index)
			if serves && t.lingers(p, index, now) {
				t.engine.config.Logger.Debug("A torrent's peer was given up: it let a piece asked of it alone wait")
				t.expel(p)
				p.givenUp, serves = true, false
			}
			if !serves {
				for b := range p.requests {
					if b.piece == index {
						t.forget(p, b)
						p.send(msgCancel, requestPayload(b))
					}
				}
				s.only = nil
			}
		}
		if s.only == nil {
			if t.entrust(index, s); s.only == nil {
				continue
			}
			t.fill(s.only)
		}
		if slices.Contains(wanted, index) {
			t.press(s.only, index, now)
		}
	}
}

// press asks the peer a piece is asked of alone for one block of it past
// its pipeline, when it is asked none of it: the piece does not wait
// behind the blocks of other pieces asked of the peer before, and a peer
// that sends nothing lets it wait no longer than lingers allows. The lock
// is held.
func (t *torrent) press(p *peer, index int, now time.Time) {
	for b := range p.requests {
		if b.piece == index {
			return
		}
	}
	in := t.inflightOf(index)
	for i, got := range in.got {
		if b := blockOf(in, index, i); !got && in.asks[i] == 0 && p.reasked[b] <= maxReasks {
			t.ask(p, in, i, b, now)
			return
		}
	}
}

// lingers reports whether a block of a piece asked of a peer has waited
// since it was asked for as long as stallWait, which tests shorten with
// dropWait. The lock is held.
func (t *torrent) lingers(p *peer, index int, now time.Time) bool {
	in := t.inflight[index]
	if in == nil {
		return false
	}
	wait := stallWait / dropWait * t.engine.dropWait
	for b := range p.requests {
		if i := int(b.offset / blockSize); b.piece == index && i < len(in.requested) && now.Sub(in.requested[i]) >= wait {
			return true
		}
	}
	return false
}

// refill asks every peer for the blocks it may be asked, once a piece was
// dropped and its senders told apart.
func (t *torrent) refill() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, p := range t.peers {
		if !p.gone() {
			t.fill(p)
		}
	}
}

// relieve closes, while every place is taken and peers wait for one, the
// peers that hold a place for nothing: those asked no block that sent
// nothing kept, block or piece of the metadata, for the engine's
// idleTimeout, the longest first, one for each waiting peer that could be
// dialed now. A peer that chokes for a while, or has nothing wanted for a
// while, keeps its place: only one that did so for idleTimeout gives it,
// and only to a peer that waits. Keep-alives count for nothing here. Each
// leaves as any peer does, and is dialed again in its turn (see leave).
// relieve returns when the next peer may be relieved, zero when none
// holds a place another waits for. The lock is held.
func (t *torrent) relieve(now time.Time) time.Time {
	short := min(len(t.queue), maxDialing-t.dialing) - (maxPeers - len(t.peers) - t.dialing)
	if short <= 0 || t.err != nil {
		return time.Time{}
	}
	var idle []*peer
	var next time.Time
	for _, p := range t.peers {
		if p.gone() {
			// Its place is free once its reader ends.
			short--
			continue
		}
		if len(p.requests) > 0 {
			continue
		}
		if at := later(p.used, p.joined).Add(t.engine.idleTimeout); at.After(now) {
			if next.IsZero() || at.Before(next) {
				next = at
			}
			continue
		}
		idle = append(idle, p)
	}
	slices.SortFunc(idle, func(a, b *peer) int { return later(a.used, a.joined).Compare(later(b.used, b.joined)) })
	for _, p := range idle[:max(0, min(short, len(idle)))] {
		t.engine.config.Logger.Debug("A torrent's peer gave its place to one that waits: it sent nothing wanted",
			"for", now.Sub(later(p.used, p.joined)).Round(time.Second))
		p.close()
	}
	return next
}

// makeRoom forgets, once the torrent knows of maxKnown peers, the one
// whose dials failed the most in a row among those waiting to be dialed
// again, so that the next peer told has room: a peer that was connected,
// or whose dials the route did not carry, is never forgotten. The lock is
// held.
func (t *torrent) makeRoom() {
	if len(t.known) <= maxKnown {
		return
	}
	var worst netip.AddrPort
	fails := 0
	for _, address := range t.redials {
		if c := t.known[address]; c.last != outcomeLeft && c.last != outcomeNone && c.fails > fails {
			worst, fails = address, c.fails
		}
	}
	if fails == 0 {
		return
	}
	delete(t.known, worst)
	t.redials = slices.DeleteFunc(t.redials, func(address netip.AddrPort) bool { return address == worst })
}

// dropMetadata forgets the peer's copy of the info dictionary.
func (p *peer) dropMetadata() {
	p.metadata, p.metadataGot, p.metadataWant = nil, nil, 0
}

// metadataRoom reports whether a peer's copy of the info dictionary, of
// the size it announced, has room within maxMetadataHeld beside the
// copies being received, and makes it when copies announced larger than
// it, the largest first, are in the way: a peer that announces the
// largest size does not keep one that announces less from being asked.
// The lock is held.
func (t *torrent) metadataRoom(p *peer) bool {
	held := p.metadataSize
	var larger []*peer
	for _, other := range t.peers {
		held += other.metadataWant
		if other.metadataWant > p.metadataSize {
			larger = append(larger, other)
		}
	}
	if held <= maxMetadataHeld {
		return true
	}
	slices.SortFunc(larger, func(a, b *peer) int { return b.metadataWant - a.metadataWant })
	n := 0
	for ; n < len(larger) && held > maxMetadataHeld; n++ {
		held -= larger[n].metadataWant
	}
	if held > maxMetadataHeld {
		return false
	}
	for _, other := range larger[:n] {
		other.dropMetadata()
	}
	return true
}

// askWaiting asks the peers that wait for room for their copy of the info
// dictionary (see askMetadata), each time reask runs while the info is
// not known, and once a copy was dropped. When there is no room, the copy
// no piece of which came for stallWait, the longest, gives its place, and
// its peer is not asked again on this connection. The lock is held.
func (t *torrent) askWaiting(now time.Time) {
	wait := stallWait / dropWait * t.engine.dropWait
	for _, p := range t.peers {
		if p.metadataWant != 0 || p.metadataID == 0 || p.metadataSize == 0 || p.metadataOver || p.gone() {
			continue
		}
		if t.askMetadata(p); p.metadataWant != 0 {
			continue
		}
		var stale *peer
		for _, other := range t.peers {
			if other.metadataWant != 0 && now.Sub(other.metadataHeard) >= wait &&
				(stale == nil || other.metadataHeard.Before(stale.metadataHeard)) {
				stale = other
			}
		}
		if stale != nil {
			stale.dropMetadata()
			stale.metadataOver = true
			t.askMetadata(p)
		}
	}
}
