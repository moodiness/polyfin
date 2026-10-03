package container

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/bits"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

// Fetcher reads exactly the n bytes at off with one request for that range
// alone; *source.Source is one. Subtitle blocks are scattered over a file,
// a few KiB every few MB: reads through a block cache would multiply them.
type Fetcher interface {
	Fetch(ctx context.Context, off int64, n int) ([]byte, error)
}

// ErrIncompleteIndex reports Cues that do not list every block of a
// subtitle track.
var ErrIncompleteIndex = errors.New("the Cues do not list every block of the track")

// errLaced reports a laced subtitle block: lacing packs several frames in a
// block, which muxers do for audio, never for subtitles.
var errLaced = errors.New("laced subtitle blocks are not supported")

// errMisplaced reports bytes that are not a block of the track read where
// one was expected: the position was worked out from a wrong Cluster header
// length, or the Cues are wrong.
var errMisplaced = errors.New("no block of the track where the Cues point")

// Block is a block of a subtitle track: a subtitle.
type Block struct {
	// Start is the block's CueTime; Duration its BlockDuration, else its
	// CueDuration.
	Start, Duration time.Duration
	// Data is the frame, decoded: decompressed or its header restored.
	Data []byte
}

const (
	// blockWindow is how much is read at a block's position: subtitle
	// blocks are smaller, but for long typeset ones, which are read again
	// whole.
	blockWindow = 4 << 10
	// Blocks closer than maxGap are read with one request, spanning at
	// most maxSpan: anime tracks pack thousands of blocks in a few
	// Clusters, film tracks have one every few MB.
	maxGap  = 64 << 10
	maxSpan = 4 << 20
	// maxFetches bounds the requests running at once: debrid hosts answer
	// 429 when hammered.
	maxFetches = 6
	// maxBlocks bounds the blocks of a track read, maxFetched the bytes
	// fetched for them, and maxBlock the size of one, encoded or decoded,
	// so that a hostile file cannot exhaust memory or the source.
	maxBlocks  = 200_000
	maxFetched = 256 << 20
	maxBlock   = 1 << 20
	// maxCueBlocks bounds the CueTrackPositions kept from the Cues.
	maxCueBlocks = 1 << 21
	// A Cluster is read whole to check that the Cues list every block of
	// the track in it: preferably one of at most smallCluster bytes, and
	// never one of more than maxCluster.
	smallCluster = 16 << 20
	maxCluster   = 64 << 20
)

// cueIndex is what a file's Cues tell about its subtitle blocks.
type cueIndex struct {
	// blocks holds, by subtitle track, the blocks the Cues locate, by
	// position in the file.
	blocks map[uint64][]cueBlock
	// unlocated marks the subtitle tracks for which some CueTrackPositions
	// give no relative position: their blocks are not all located.
	unlocated map[uint64]bool
	// clusters holds the positions of the Clusters any CueTrackPositions
	// gives, ascending: where one begins bounds the one before it.
	clusters []uint64
}

// cueBlock is a block the Cues locate, its times in ticks and its
// positions relative to the Segment's data and to its Cluster's data.
type cueBlock struct {
	time, duration    uint64
	hasDuration       bool
	cluster, relative uint64
}

// SubtitleLocations returns, for each subtitle track the Cues locate block
// by block (CueTrackPositions with CueRelativePosition), the number of
// blocks they list; none when the file has no Cues. It reads the Cues once,
// kept for SubtitleBlocks.
func (m *Matroska) SubtitleLocations(ctx context.Context) (map[uint64]int, error) {
	index, err := m.cueIndex(ctx)
	if err != nil {
		return nil, err
	}
	locations := map[uint64]int{}
	for track, blocks := range index.blocks {
		if !index.unlocated[track] {
			locations[track] = len(blocks)
		}
	}
	return locations, nil
}

// cueIndex reads m's Cues, once. Callers meanwhile wait for the one read.
func (m *Matroska) cueIndex(ctx context.Context) (*cueIndex, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cues != nil {
		return m.cues, nil
	}
	subtitles := map[uint64]bool{}
	var first uint64
	for _, track := range m.tracks {
		if track.Type == trackTypeSubtitle && track.Number != 0 {
			subtitles[track.Number] = true
			if first == 0 {
				first = track.Number
			}
		}
	}
	empty := &cueIndex{blocks: map[uint64][]cueBlock{}, unlocated: map[uint64]bool{}}
	if len(subtitles) == 0 {
		m.cues = empty
		return empty, nil
	}
	// Cues no SeekHead lists are recognized by their CuePoints for a track:
	// the video's, which every Cues have, else a subtitle track's.
	track, err := m.videoTrack()
	if err != nil {
		track = first
	}
	data, err := m.cuesData(m.file(ctx), track)
	if errors.Is(err, ErrNoIndex) {
		m.cues = empty
		return empty, nil
	}
	if err != nil {
		return nil, err
	}
	index, err := parseCues(data, subtitles)
	if err != nil {
		return nil, err
	}
	m.cues = index
	return index, nil
}

// parseCues reads the positions Cues give: of every Cluster, and of the
// blocks of the subtitle tracks.
func parseCues(cues []byte, subtitles map[uint64]bool) (*cueIndex, error) {
	index := &cueIndex{blocks: map[uint64][]cueBlock{}, unlocated: map[uint64]bool{}}
	kept := 0
	err := children(cues, func(id uint32, point []byte) error {
		switch id {
		case idVoid, idCRC32:
			return nil
		case idCuePoint:
		default:
			return fmt.Errorf("element %X in Cues: %w", id, errInvalid)
		}
		var ticks uint64
		var hasTime bool
		var positions [][]byte
		err := children(point, func(id uint32, data []byte) error {
			var err error
			switch id {
			case idCueTime:
				ticks, err = unsigned(data)
				hasTime = true
			case idCueTrackPositions:
				positions = append(positions, data)
			}
			return err
		})
		if err != nil {
			return err
		}
		if !hasTime {
			return fmt.Errorf("CuePoint without a time: %w", errInvalid)
		}
		for _, data := range positions {
			block := cueBlock{time: ticks}
			var track uint64
			var hasCluster, hasRelative bool
			err := children(data, func(id uint32, data []byte) error {
				var err error
				switch id {
				case idCueTrack:
					track, err = unsigned(data)
				case idCueClusterPosition:
					block.cluster, err = unsigned(data)
					hasCluster = true
				case idCueRelativePosition:
					block.relative, err = unsigned(data)
					hasRelative = true
				case idCueDuration:
					block.duration, err = unsigned(data)
					block.hasDuration = true
				}
				return err
			})
			if err != nil {
				return err
			}
			if kept++; kept > maxCueBlocks {
				return fmt.Errorf("more than %d CueTrackPositions: %w", maxCueBlocks, errInvalid)
			}
			if hasCluster {
				index.clusters = append(index.clusters, block.cluster)
			}
			switch {
			case !subtitles[track]:
			case !hasCluster || !hasRelative:
				index.unlocated[track] = true
			default:
				index.blocks[track] = append(index.blocks[track], block)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(index.clusters)
	index.clusters = slices.Compact(index.clusters)
	// A block listed twice, as by two CuePoints, is read once.
	for track, blocks := range index.blocks {
		slices.SortStableFunc(blocks, func(a, b cueBlock) int {
			return cmp.Or(cmp.Compare(a.cluster, b.cluster), cmp.Compare(a.relative, b.relative))
		})
		index.blocks[track] = slices.CompactFunc(blocks, func(a, b cueBlock) bool {
			return a.cluster == b.cluster && a.relative == b.relative
		})
	}
	return index, nil
}

// SubtitleBlocks reads every block of the subtitle track number through f,
// sorted by Start (ties: file order). It first checks one Cluster holding
// blocks of the track: when the Cues do not list every block of the track
// there, ErrIncompleteIndex. Blocks close together are read with one
// request; at most a few requests run at once.
func (m *Matroska) SubtitleBlocks(ctx context.Context, f Fetcher, number uint64) ([]Block, error) {
	i := slices.IndexFunc(m.tracks, func(track Track) bool { return track.Number == number })
	if i < 0 || m.tracks[i].Type != trackTypeSubtitle || number == 0 {
		return nil, fmt.Errorf("no subtitle track %d", number)
	}
	if !m.tracks[i].Decodable {
		return nil, fmt.Errorf("the blocks of track %d are encrypted or compressed in an unsupported way", number)
	}
	index, err := m.cueIndex(ctx)
	if err != nil {
		return nil, err
	}
	cues := index.blocks[number]
	if index.unlocated[number] || len(cues) == 0 {
		return nil, fmt.Errorf("track %d: %w", number, ErrIncompleteIndex)
	}
	if len(cues) > maxBlocks {
		return nil, fmt.Errorf("track %d of more than %d blocks: %w", number, maxBlocks, errInvalid)
	}
	segment := min(m.segment.end, m.size) - m.segment.data
	for _, cue := range cues {
		if cue.cluster >= uint64(segment) || cue.relative >= uint64(segment) {
			return nil, fmt.Errorf("Cues of track %d past the segment: %w", number, errInvalid)
		}
	}
	r := &blockReader{m: m, f: f, number: number, encodings: m.frames[number], headers: map[uint64]int64{}}
	header, err := r.check(ctx, cues, index.clusters)
	if err != nil {
		return nil, err
	}
	return r.read(ctx, cues, header)
}

// blockReader reads the blocks of a subtitle track.
type blockReader struct {
	m         *Matroska
	f         Fetcher
	number    uint64
	encodings []contentEncoding
	fetched   atomic.Int64

	mu sync.Mutex
	// headers holds the length of the headers of the Clusters read, by
	// position relative to the Segment's data.
	headers map[uint64]int64
}

// check reads a Cluster holding blocks of the track whole, and fails with
// ErrIncompleteIndex when it holds blocks the Cues do not list. It returns
// the length of the Cluster's header, which the others' likely share.
//
// The Cluster checked is a small one, by where the next Cluster the Cues
// know begins, among those with the most blocks listed: a muxer listing
// only some blocks shows it where several share a Cluster.
func (r *blockReader) check(ctx context.Context, cues []cueBlock, clusters []uint64) (int64, error) {
	segment := uint64(min(r.m.segment.end, r.m.size) - r.m.segment.data)
	listed := map[uint64]map[uint64]bool{}
	for _, cue := range cues {
		if listed[cue.cluster] == nil {
			listed[cue.cluster] = map[uint64]bool{}
		}
		listed[cue.cluster][cue.relative] = true
	}
	type candidate struct {
		cluster, bound uint64
		count          int
	}
	candidates := make([]candidate, 0, len(listed))
	for cluster, relatives := range listed {
		bound := segment - cluster
		if i, _ := slices.BinarySearch(clusters, cluster+1); i < len(clusters) {
			bound = clusters[i] - cluster
		}
		candidates = append(candidates, candidate{cluster, bound, len(relatives)})
	}
	small := func(c candidate) int {
		if c.bound <= smallCluster {
			return 0
		}
		return 1
	}
	best := slices.MinFunc(candidates, func(a, b candidate) int {
		return cmp.Or(cmp.Compare(small(a), small(b)), cmp.Compare(b.count, a.count),
			cmp.Compare(a.bound, b.bound), cmp.Compare(a.cluster, b.cluster))
	})

	at := r.m.segment.data + int64(best.cluster)
	h, err := r.clusterHeader(ctx, best.cluster)
	if err != nil {
		return 0, err
	}
	size := h.size
	if h.unknown {
		// The Cluster ends where the next begins, or the Segment.
		size = min(int64(best.bound)-int64(h.length), maxCluster)
	}
	switch {
	case size < 0, size > r.m.size-at-int64(h.length):
		return 0, fmt.Errorf("Cluster at %d past the end of the file: %w", at, errInvalid)
	case size > maxCluster:
		return 0, fmt.Errorf("Cluster of %d bytes to check: %w", size, errInvalid)
	}
	data, err := r.fetch(ctx, at+int64(h.length), size)
	if err != nil {
		return 0, err
	}
	found, err := trackBlocks(data, r.number, h.unknown)
	if err != nil {
		return 0, err
	}
	for _, relative := range found {
		if !listed[best.cluster][relative] {
			return 0, fmt.Errorf("track %d: %w", r.number, ErrIncompleteIndex)
		}
	}
	if len(found) != len(listed[best.cluster]) {
		return 0, fmt.Errorf("track %d at %d: %w: %w", r.number, at, errMisplaced, errInvalid)
	}
	return int64(h.length), nil
}

// trackBlocks returns the positions of the blocks of track in a Cluster's
// data. One of unknown size ends at the next top-level element, or where
// data was cut.
func trackBlocks(data []byte, track uint64, unknown bool) ([]uint64, error) {
	var found []uint64
	for pos := 0; pos < len(data); {
		h, err := parseHeader(data[pos:])
		ends := err == nil && !h.unknown && h.size <= int64(len(data)-pos-h.length)
		if unknown && (!ends || h.id > 0xFFFFFF) {
			// Top-level elements have 4-byte IDs, a Cluster's children
			// shorter ones.
			break
		}
		if !ends {
			return nil, fmt.Errorf("element in a Cluster overruns it: %w", errInvalid)
		}
		end := pos + h.length + int(h.size)
		payload := data[pos+h.length : end]
		var block []byte
		switch h.id {
		case idSimpleBlock:
			block = payload
		case idBlockGroup:
			err := children(payload, func(id uint32, data []byte) error {
				if id == idBlock {
					block = data
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
		}
		if block != nil {
			if number, _, err := blockTrack(block); err == nil && number == track {
				found = append(found, uint64(pos))
			}
		}
		pos = end
	}
	return found, nil
}

// clusterHeader reads the header of the Cluster at cluster, relative to
// the Segment's data, and records its length.
func (r *blockReader) clusterHeader(ctx context.Context, cluster uint64) (header, error) {
	at := r.m.segment.data + int64(cluster)
	data, err := r.fetch(ctx, at, min(maxHeader, r.m.size-at))
	if err != nil {
		return header{}, err
	}
	h, err := parseHeader(data)
	if err != nil || h.id != idCluster {
		return header{}, fmt.Errorf("the Cues point to no Cluster at %d: %w", at, errInvalid)
	}
	r.mu.Lock()
	r.headers[cluster] = int64(h.length)
	r.mu.Unlock()
	return h, nil
}

// headerLength returns the length of the header of the Cluster at
// cluster, when read.
func (r *blockReader) headerLength(cluster uint64) (int64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	length, ok := r.headers[cluster]
	return length, ok
}

// located is a block of the track to read, at its position in the file
// as worked out, and its rank in the file.
type located struct {
	cue  cueBlock
	at   int64
	rank int
}

// read reads the blocks the Cues list, assuming the Clusters not read yet
// have headers of length header, by groups of blocks close together.
func (r *blockReader) read(ctx context.Context, cues []cueBlock, header int64) ([]Block, error) {
	blocks := make([]located, len(cues))
	for i, cue := range cues {
		length, ok := r.headerLength(cue.cluster)
		if !ok {
			length = header
		}
		blocks[i] = located{cue: cue, at: r.m.segment.data + int64(cue.cluster) + length + int64(cue.relative)}
	}
	slices.SortStableFunc(blocks, func(a, b located) int { return cmp.Compare(a.at, b.at) })
	var groups [][]located
	for i := range blocks {
		blocks[i].rank = i
		if n := len(groups); n > 0 {
			group := groups[n-1]
			start, end := group[0].at, group[len(group)-1].at+blockWindow
			if blocks[i].at-end <= maxGap && blocks[i].at+blockWindow-start <= maxSpan {
				groups[n-1] = append(group, blocks[i])
				continue
			}
		}
		groups = append(groups, []located{blocks[i]})
	}

	result := make([]Block, len(blocks))
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxFetches)
	for _, group := range groups {
		g.Go(func() error {
			start := group[0].at
			end := min(group[len(group)-1].at+blockWindow, r.m.size)
			if start >= end {
				return fmt.Errorf("block of track %d past the end of the file: %w", r.number, errInvalid)
			}
			data, err := r.fetch(ctx, start, end-start)
			if err != nil {
				return err
			}
			for _, block := range group {
				if result[block.rank], err = r.block(ctx, block, data, start); err != nil {
					return err
				}
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	slices.SortStableFunc(result, func(a, b Block) int { return cmp.Compare(a.Start, b.Start) })
	return result, nil
}

// block reads a block from data, read at start, or from the file when
// data does not hold it. When the block is not where it was assumed to be,
// its Cluster's header is read for where it is.
func (r *blockReader) block(ctx context.Context, block located, data []byte, start int64) (Block, error) {
	result, err := r.parse(ctx, block.cue, block.at, data, start)
	if !errors.Is(err, errMisplaced) {
		return result, err
	}
	length, ok := r.headerLength(block.cue.cluster)
	if !ok {
		h, err := r.clusterHeader(ctx, block.cue.cluster)
		if err != nil {
			return Block{}, err
		}
		length = int64(h.length)
	}
	at := r.m.segment.data + int64(block.cue.cluster) + length + int64(block.cue.relative)
	if at == block.at {
		return Block{}, fmt.Errorf("track %d at %d: %w: %w", r.number, at, errMisplaced, errInvalid)
	}
	result, err = r.parse(ctx, block.cue, at, data, start)
	if errors.Is(err, errMisplaced) {
		return Block{}, fmt.Errorf("track %d at %d: %w: %w", r.number, at, err, errInvalid)
	}
	return result, err
}

// parse reads the block at the position at: a SimpleBlock, or a Block in
// a BlockGroup, of the track. Its bytes are taken from data, read at
// start, when it holds them. errMisplaced when no such block is there.
func (r *blockReader) parse(ctx context.Context, cue cueBlock, at int64, data []byte, start int64) (Block, error) {
	if at >= r.m.size {
		return Block{}, errMisplaced
	}
	if at < start || at+min(maxHeader, r.m.size-at) > start+int64(len(data)) {
		// A block whose Cluster header is longer than assumed may begin
		// past the window read.
		var err error
		if data, err = r.fetch(ctx, at, min(blockWindow, r.m.size-at)); err != nil {
			return Block{}, err
		}
		start = at
	}
	h, err := parseHeader(data[at-start:])
	switch {
	case err != nil, h.id != idSimpleBlock && h.id != idBlockGroup, h.unknown, h.size > maxBlock:
		return Block{}, errMisplaced
	case h.size > r.m.size-at-int64(h.length):
		return Block{}, errMisplaced
	}
	from, to := at+int64(h.length), at+int64(h.length)+h.size
	var payload []byte
	if to <= start+int64(len(data)) {
		payload = data[from-start : to-start]
	} else if payload, err = r.fetch(ctx, from, h.size); err != nil {
		return Block{}, err
	}

	block := payload
	var ticks uint64
	var hasDuration bool
	if h.id == idBlockGroup {
		block = nil
		err := children(payload, func(id uint32, data []byte) error {
			var err error
			switch id {
			case idBlock:
				block = data
			case idBlockDuration:
				ticks, err = unsigned(data)
				hasDuration = true
			}
			return err
		})
		if err != nil || block == nil {
			return Block{}, errMisplaced
		}
	}
	number, length, err := blockTrack(block)
	if err != nil || number != r.number || len(block) < length+3 {
		return Block{}, errMisplaced
	}
	// The timecode, relative to the Cluster's, is not needed: the Cues give
	// the block's time. The flags tell whether frames are laced.
	if block[length+2]&0x06 != 0 {
		return Block{}, fmt.Errorf("track %d: %w", r.number, errLaced)
	}
	frame := block[length+3:]
	result := Block{}
	if len(r.encodings) == 0 {
		// The frame is copied out of the read, which may be MBs.
		result.Data = slices.Clone(frame)
	} else if result.Data, err = decode(frame, r.encodings, maxBlock); err != nil {
		return Block{}, fmt.Errorf("track %d: %w", r.number, err)
	}
	if result.Start, err = scaled(cue.time, r.m.scale); err != nil {
		return Block{}, err
	}
	if !hasDuration {
		ticks, hasDuration = cue.duration, cue.hasDuration
	}
	if hasDuration {
		if result.Duration, err = scaled(ticks, r.m.scale); err != nil {
			return Block{}, err
		}
	}
	return result, nil
}

// fetch reads the n bytes at off through the Fetcher, within the bytes
// fetched for a track.
func (r *blockReader) fetch(ctx context.Context, off, n int64) ([]byte, error) {
	if r.fetched.Add(n) > maxFetched {
		return nil, fmt.Errorf("track %d needs more than %d bytes read: %w", r.number, maxFetched, errInvalid)
	}
	data, err := r.f.Fetch(ctx, off, int(n))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) < n {
		return nil, fmt.Errorf("fetching %d bytes at %d: %w", n, off, io.ErrUnexpectedEOF)
	}
	return data[:n], nil
}

// blockTrack reads the track number starting a Block, stored without its
// length marker, and the length it takes.
func blockTrack(block []byte) (uint64, int, error) {
	value, length, err := vint(block, 8)
	if err != nil {
		return 0, 0, err
	}
	return value &^ (1 << (7 * length)), length, nil
}

// scaled converts ticks of a TimestampScale to a duration, failing rather
// than overflowing.
func scaled(ticks, scale uint64) (time.Duration, error) {
	high, ns := bits.Mul64(ticks, scale)
	if high != 0 || ns > math.MaxInt64 {
		return 0, fmt.Errorf("time of %d ticks out of range: %w", ticks, errInvalid)
	}
	return time.Duration(ns), nil
}
