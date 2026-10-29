package torrent

// Stage is how far a torrent is in its start.
type Stage string

const (
	// StagePeers: no peer is connected yet; trackers and the DHT are
	// asked for peers, and those found dialed.
	StagePeers Stage = "peers"
	// StageDetails: peers are connected, and the info dictionary, which
	// tells the torrent's files and pieces, is being received.
	StageDetails Stage = "details"
	// StagePieces: the torrent started, and its pieces come.
	StagePieces Stage = "pieces"
)

// Progress is how a running torrent's start goes: its stage, the peers
// found and those connected, the bytes of pieces received in this run,
// and those the reads under way still wait for. Nothing in it names a
// peer, a tracker or an address.
type Progress struct {
	Stage            Stage
	Found, Connected int
	Received         int64
	Waiting          int64
}

// Why is the code of why a torrent did not start: peers found that sent
// nothing ("silent"), none found over a route without UDP ("no_udp"),
// trackers that all refused or did not answer ("refused"), none found on
// the DHT, the addon having named no tracker and the settings adding none
// ("no_trackers"), or none found where it was looked for ("no_peers").
// Health and the progress of a start tell it.
func (e *StartError) Why() string {
	switch {
	case e.Found > 0:
		return "silent"
	case e.NoUDP:
		return "no_udp"
	case e.Trackers > 0 && e.Answered == 0:
		return "refused"
	case e.Trackers == 0 && e.NoneNamed:
		return "no_trackers"
	}
	return "no_peers"
}

// Progress tells how the start of the torrent of hash goes; false when it
// does not run.
func (e *Engine) Progress(hash Hash) (Progress, bool) {
	e.mu.Lock()
	t := e.torrents[hash]
	e.mu.Unlock()
	if t == nil {
		return Progress{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	p := Progress{Found: max(len(t.known), len(t.peers)), Connected: len(t.peers), Received: t.downBytes}
	switch {
	// A torrent whose last run kept its info dictionary and peers (see
	// seed) has started, but nothing comes before a peer connects.
	case len(t.peers) == 0:
		p.Stage = StagePeers
	case t.info == nil:
		p.Stage = StageDetails
	default:
		p.Stage = StagePieces
		p.Waiting = t.waiting()
	}
	return p, true
}

// waiting counts the bytes the reads under way wait for that have not
// arrived: those of the pieces they span that Polyfin lacks, less the
// blocks of them received. The lock is held, and the info is known.
func (t *torrent) waiting() int64 {
	length := t.info.PieceLength
	counted := map[int]bool{}
	var missing int64
	for _, span := range t.reads {
		if span.at >= span.end {
			continue
		}
		for index := int(span.at / length); index <= int((span.end-1)/length) && index < len(t.have); index++ {
			if counted[index] || t.have[index] {
				continue
			}
			counted[index] = true
			missing += t.info.pieceSize(index)
			if in := t.inflight[index]; in != nil {
				for i, got := range in.got {
					if got {
						missing -= blockOf(in, index, i).length
					}
				}
			}
		}
	}
	return missing
}
