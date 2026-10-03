package container

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"math/bits"
	"slices"
	"strings"
	"sync"
	"time"
)

// ebmlMagic starts every Matroska and WebM file: the EBML header's ID.
var ebmlMagic = []byte{0x1A, 0x45, 0xDF, 0xA3}

// The IDs of the elements read, from RFC 9559.
const (
	idEBML                = 0x1A45DFA3
	idDocType             = 0x4282
	idSegment             = 0x18538067
	idSeekHead            = 0x114D9B74
	idSeek                = 0x4DBB
	idSeekID              = 0x53AB
	idSeekPosition        = 0x53AC
	idInfo                = 0x1549A966
	idTimestampScale      = 0x2AD7B1
	idTracks              = 0x1654AE6B
	idTrackEntry          = 0xAE
	idTrackNumber         = 0xD7
	idTrackType           = 0x83
	idCodecID             = 0x86
	idCodecPrivate        = 0x63A2
	idContentEncodings    = 0x6D80
	idContentEncoding     = 0x6240
	idContentOrder        = 0x5031
	idContentScope        = 0x5032
	idContentType         = 0x5033
	idContentCompression  = 0x5034
	idContentCompAlgo     = 0x4254
	idContentCompSettings = 0x4255
	idCues                = 0x1C53BB6B
	idCuePoint            = 0xBB
	idCueTime             = 0xB3
	idCueTrackPositions   = 0xB7
	idCueTrack            = 0xF7
	idCueClusterPosition  = 0xF1
	idCueRelativePosition = 0xF0
	idCueDuration         = 0xB2
	idCluster             = 0x1F43B675
	idBlockGroup          = 0xA0
	idBlock               = 0xA1
	idBlockDuration       = 0x9B
	idSimpleBlock         = 0xA3
	idChapters            = 0x1043A770
	idTags                = 0x1254C367
	idAttachments         = 0x1941A469
	idAttachedFile        = 0x61A7
	idFileName            = 0x466E
	idFileMediaType       = 0x4660
	idFileData            = 0x465C
	idVoid                = 0xEC
	idCRC32               = 0xBF
)

const (
	// trackTypeVideo is the TrackType of video tracks, trackTypeSubtitle
	// that of subtitle tracks.
	trackTypeVideo    = 1
	trackTypeSubtitle = 0x11
	// defaultTimestampScale is the nanoseconds a tick lasts when Info
	// does not say.
	defaultTimestampScale = 1_000_000
	// maxSeekHeads bounds the SeekHeads followed from one another.
	maxSeekHeads = 8
	// maxInfo bounds the Info element read for its TimestampScale.
	maxInfo = 1 << 20
	// tailWindows is how many windows the end of a file read to find
	// Cues no SeekHead lists takes.
	tailWindows = 4
)

// Matroska is a Matroska or WebM file whose head was read: where its
// top-level elements are, its TimestampScale and its tracks. The Cues are
// read when first needed, and kept. Its methods may be called concurrently.
type Matroska struct {
	r            Reader
	size, window int64
	segment      element
	// firstCluster is where the first Cluster starts, -1 when the walk of
	// the head did not reach one.
	firstCluster int64
	// positions holds where the Info, Tracks, Cues and Attachments are,
	// when known.
	positions map[uint32]int64
	scale     uint64
	// trackList holds the tracks, the errors of their TrackEntries, and
	// the content encodings to undo on the frames of each Decodable track.
	trackList
	// head holds the spans the head was read from: a small file's, or one
	// whose Cues come first, also hold its Cues and Attachments.
	head [2]held

	mu   sync.Mutex
	cues *cueIndex
}

// OpenMatroska reads a Matroska or WebM file's head through r: the EBML
// header, SeekHeads, Info and Tracks. ErrNoIndex when r is not one. size is
// the file size.
func OpenMatroska(ctx context.Context, r Reader, size int64) (*Matroska, error) {
	f := &file{ctx: ctx, r: r, size: size, window: window}
	head, err := f.span(0, min(size, int64(len(ebmlMagic))))
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(head, ebmlMagic) {
		return nil, ErrNoIndex
	}
	return openMatroska(f)
}

// file returns a file reading m's Reader on behalf of ctx, starting with
// the spans of the head.
func (m *Matroska) file(ctx context.Context) *file {
	return &file{ctx: ctx, r: m.r, size: m.size, window: m.window, spans: m.head}
}

// Tracks returns m's tracks, in TrackEntry order.
func (m *Matroska) Tracks() []Track {
	return slices.Clone(m.tracks)
}

// readMatroska returns the times of the CuePoints of the first video track.
func readMatroska(f *file) ([]time.Duration, error) {
	m, err := openMatroska(f)
	if err != nil {
		return nil, err
	}
	track, err := m.videoTrack()
	if err != nil {
		return nil, err
	}
	cues, err := m.cuesData(f, track)
	if err != nil {
		return nil, err
	}
	return cueTimes(cues, track, m.scale)
}

// openMatroska reads the head of f, a file starting with an EBML header.
func openMatroska(f *file) (*Matroska, error) {
	ebml, err := f.element(0, f.size)
	if err != nil {
		return nil, err
	}
	header, err := f.payload(ebml, idEBML, 4<<10)
	if err != nil {
		return nil, err
	}
	docType := "matroska"
	err = children(header, func(id uint32, data []byte) error {
		if id == idDocType {
			docType = strings.TrimRight(string(data), "\x00")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if docType != "matroska" && docType != "webm" {
		return nil, ErrNoIndex
	}

	segment, err := findSegment(f, ebml.end)
	if err != nil {
		return nil, err
	}
	m := &Matroska{r: f.r, size: f.size, window: f.window, segment: segment, firstCluster: -1,
		positions: map[uint32]int64{}, scale: defaultTimestampScale}

	// The elements before the first Cluster are walked: they usually are
	// the SeekHead, Info and Tracks, within the first read, and
	// sometimes the Cues and Attachments.
	var seekHeads []int64
	for pos, walked := segment.data, 0; pos < segment.end && walked < maxTopLevel; walked++ {
		e, err := f.element(pos, segment.end)
		if err != nil {
			return nil, err
		}
		if e.id == idCluster {
			m.firstCluster = pos
			break
		}
		if e.unknown {
			return nil, ErrNoIndex
		}
		switch e.id {
		case idSeekHead:
			seekHeads = append(seekHeads, pos)
		case idInfo, idTracks, idCues, idAttachments:
			if _, ok := m.positions[e.id]; !ok {
				m.positions[e.id] = pos
			}
		}
		pos = e.end
	}

	// The SeekHeads tell where the rest is, the Cues usually after the
	// Clusters. One may point to another, at the end of the file.
	followed := map[int64]bool{}
	for i := 0; i < len(seekHeads) && len(followed) < maxSeekHeads; i++ {
		if followed[seekHeads[i]] {
			continue
		}
		followed[seekHeads[i]] = true
		data, err := f.master(seekHeads[i], idSeekHead, segment.end, maxIndex)
		if err != nil {
			return nil, err
		}
		entries, err := seekEntries(data)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.position > uint64(segment.end-segment.data) {
				return nil, fmt.Errorf("seek entry past the segment: %w", errInvalid)
			}
			at := segment.data + int64(entry.position)
			switch entry.id {
			case idSeekHead:
				seekHeads = append(seekHeads, at)
			case idInfo, idTracks, idCues, idAttachments:
				if _, ok := m.positions[entry.id]; !ok {
					m.positions[entry.id] = at
				}
			}
		}
	}

	if at, ok := m.positions[idInfo]; ok {
		data, err := f.master(at, idInfo, segment.end, maxInfo)
		if err != nil {
			return nil, err
		}
		err = children(data, func(id uint32, data []byte) error {
			if id == idTimestampScale {
				m.scale, err = unsigned(data)
			}
			return err
		})
		if err != nil {
			return nil, err
		}
		if m.scale == 0 {
			return nil, fmt.Errorf("zero TimestampScale: %w", errInvalid)
		}
	}

	at, ok := m.positions[idTracks]
	if !ok {
		return nil, ErrNoIndex
	}
	data, err := f.master(at, idTracks, segment.end, maxIndex)
	if err != nil {
		return nil, err
	}
	m.trackList, err = parseTracks(data)
	if err != nil {
		return nil, err
	}
	m.head = f.spans
	return m, nil
}

// videoTrack returns the TrackNumber of the first video track. A
// TrackEntry before it that does not read fails it.
func (m *Matroska) videoTrack() (uint64, error) {
	for i, track := range m.tracks {
		if m.errs[i] != nil {
			return 0, m.errs[i]
		}
		if track.Type != trackTypeVideo {
			continue
		}
		if track.Number == 0 {
			return 0, fmt.Errorf("video track without a number: %w", errInvalid)
		}
		return track.Number, nil
	}
	return 0, ErrNoIndex
}

// cuesData returns the data of m's Cues: those the head or a SeekHead
// lists, else those found after the last Cluster holding CuePoints for
// track.
func (m *Matroska) cuesData(f *file, track uint64) ([]byte, error) {
	if at, ok := m.positions[idCues]; ok {
		return f.master(at, idCues, m.segment.end, maxIndex)
	}
	return findCues(f, m.segment, m.firstCluster, track)
}

// findSegment returns the Segment following the EBML header. Its size may
// be unknown, as when it was written to a stream: it then ends with the
// file.
func findSegment(f *file, pos int64) (element, error) {
	for range 8 {
		e, err := f.element(pos, math.MaxInt64)
		if err != nil {
			return element{}, err
		}
		switch {
		case e.id == idSegment && e.unknown:
			e.end = f.size
			return e, nil
		case e.id == idSegment:
			return e, nil
		case e.id != idVoid || e.unknown:
			return element{}, ErrNoIndex
		}
		pos = e.end
	}
	return element{}, ErrNoIndex
}

// findCues looks for Cues no SeekHead lists, which a muxer writing to a
// stream it could not seek back into leaves right after the last Cluster,
// within one read of the end of the Segment. The Clusters are not walked
// to find it: that would read the whole file.
func findCues(f *file, segment element, firstCluster int64, track uint64) ([]byte, error) {
	if firstCluster < 0 {
		return nil, ErrNoIndex
	}
	end := min(segment.end, f.size)
	start := max(firstCluster, end-tailWindows*f.window)
	tail, err := f.span(start, end-start)
	if err != nil {
		return nil, err
	}
	// The ID may appear by chance in frame data, so a candidate is only
	// taken when it ends where the Segment or a top-level element begins,
	// and holds nothing but CuePoints.
	cuesID := []byte{0x1C, 0x53, 0xBB, 0x6B}
	for i := bytes.LastIndex(tail, cuesID); i >= 0; i = bytes.LastIndex(tail[:i], cuesID) {
		h, err := parseHeader(tail[i:])
		if err != nil || h.unknown || h.size > int64(len(tail)-i-h.length) {
			continue
		}
		next := i + h.length + int(h.size)
		if next < len(tail) {
			following, err := parseHeader(tail[next:])
			if err != nil || !followsCues[following.id] {
				continue
			}
		}
		data := tail[i+h.length : next]
		if times, err := cueTimes(data, track, 1); err == nil && len(times) > 0 {
			return data, nil
		}
	}
	return nil, ErrNoIndex
}

// followsCues holds the top-level elements that may follow Cues written
// after the last Cluster.
var followsCues = map[uint32]bool{
	idSeekHead: true, idInfo: true, idTracks: true, idChapters: true,
	idTags: true, idAttachments: true, idVoid: true, idCRC32: true,
}

// seekEntry is where a SeekHead says a top-level element is, relative to
// the Segment's data.
type seekEntry struct {
	id       uint32
	position uint64
}

func seekEntries(seekHead []byte) ([]seekEntry, error) {
	var entries []seekEntry
	err := children(seekHead, func(id uint32, seek []byte) error {
		if id != idSeek {
			return nil
		}
		var entry seekEntry
		var hasID, hasPosition bool
		err := children(seek, func(id uint32, data []byte) error {
			var err error
			switch id {
			case idSeekID:
				// The ID is stored with its length marker, as it is
				// written before an element.
				if len(data) == 0 || len(data) > 4 {
					return fmt.Errorf("SeekID of %d bytes: %w", len(data), errInvalid)
				}
				for _, b := range data {
					entry.id = entry.id<<8 | uint32(b)
				}
				hasID = true
			case idSeekPosition:
				entry.position, err = unsigned(data)
				hasPosition = true
			}
			return err
		})
		if err != nil {
			return err
		}
		if hasID && hasPosition {
			entries = append(entries, entry)
		}
		return nil
	})
	return entries, err
}

// cueTimes returns the times of the CuePoints that have a position for
// track, as Cues hold nothing else.
func cueTimes(cues []byte, track, scale uint64) ([]time.Duration, error) {
	var times []time.Duration
	err := children(cues, func(id uint32, point []byte) error {
		switch id {
		case idVoid, idCRC32:
			return nil
		case idCuePoint:
		default:
			return fmt.Errorf("element %X in Cues: %w", id, errInvalid)
		}
		var ticks uint64
		var hasTime, hasTrack bool
		err := children(point, func(id uint32, data []byte) error {
			var err error
			switch id {
			case idCueTime:
				ticks, err = unsigned(data)
				hasTime = true
			case idCueTrackPositions:
				err = children(data, func(id uint32, data []byte) error {
					if id != idCueTrack {
						return nil
					}
					number, err := unsigned(data)
					hasTrack = hasTrack || number == track
					return err
				})
			}
			return err
		})
		if err != nil {
			return err
		}
		if !hasTime {
			return fmt.Errorf("CuePoint without a time: %w", errInvalid)
		}
		if !hasTrack {
			return nil
		}
		high, ns := bits.Mul64(ticks, scale)
		if high != 0 || ns > math.MaxInt64 {
			return fmt.Errorf("CueTime %d out of range: %w", ticks, errInvalid)
		}
		if len(times) == maxKeyframes {
			return fmt.Errorf("more than %d CuePoints: %w", maxKeyframes, errInvalid)
		}
		times = append(times, time.Duration(ns))
		return nil
	})
	return times, err
}

// element is an EBML element read from the file: its ID, where it starts,
// where its data starts and where it ends.
type element struct {
	id               uint32
	start, data, end int64
	unknown          bool
}

// element reads the header of the element at off, within a parent ending
// at limit. An element of unknown size is taken to end with its parent.
func (f *file) element(off, limit int64) (element, error) {
	if off >= f.size {
		return element{}, fmt.Errorf("element at %d of %d: %w", off, f.size, io.ErrUnexpectedEOF)
	}
	b, err := f.span(off, min(maxHeader, f.size-off))
	if err != nil {
		return element{}, err
	}
	h, err := parseHeader(b)
	if err != nil {
		return element{}, fmt.Errorf("element at %d: %w", off, err)
	}
	e := element{id: h.id, start: off, data: off + int64(h.length), unknown: h.unknown}
	if h.unknown {
		e.end = limit
		return e, nil
	}
	if h.size > limit-e.data {
		return element{}, fmt.Errorf("element %X at %d overruns its parent: %w", h.id, off, errInvalid)
	}
	e.end = e.data + h.size
	return e, nil
}

// payload returns the data of e, an element expected to be id, when it is
// at most limit bytes.
func (f *file) payload(e element, id uint32, limit int64) ([]byte, error) {
	switch {
	case e.id != id:
		return nil, fmt.Errorf("element %X at %d instead of %X: %w", e.id, e.start, id, errInvalid)
	case e.unknown:
		return nil, ErrNoIndex
	case e.end-e.data > limit:
		return nil, fmt.Errorf("element %X of %d bytes: %w", id, e.end-e.data, errInvalid)
	}
	return f.span(e.data, e.end-e.data)
}

// master returns the data of the element id at off, within a parent
// ending at end, when it is at most limit bytes.
func (f *file) master(off int64, id uint32, end, limit int64) ([]byte, error) {
	e, err := f.element(off, end)
	if err != nil {
		return nil, err
	}
	return f.payload(e, id, limit)
}

// maxHeader is the longest an element header is: a 4-byte ID and an
// 8-byte size.
const maxHeader = 12

// header is an element header parsed from memory.
type header struct {
	id      uint32
	size    int64
	length  int
	unknown bool
}

func parseHeader(b []byte) (header, error) {
	id, idLength, err := vint(b, 4)
	if err != nil {
		return header{}, err
	}
	size, sizeLength, err := vint(b[idLength:], 8)
	if err != nil {
		return header{}, err
	}
	// The size is stored without its length marker, and all its bits set
	// mean unknown.
	marker := uint64(1) << (7 * sizeLength)
	size &^= marker
	h := header{id: uint32(id), size: int64(size), length: idLength + sizeLength}
	h.unknown = size == marker-1
	return h, nil
}

// vint reads the variable-size integer at the start of b, at most limit
// bytes long, with its length marker.
func vint(b []byte, limit int) (uint64, int, error) {
	if len(b) == 0 || b[0] == 0 {
		return 0, 0, fmt.Errorf("variable-size integer: %w", errInvalid)
	}
	n := bits.LeadingZeros8(b[0]) + 1
	if n > limit || n > len(b) {
		return 0, 0, fmt.Errorf("variable-size integer of %d bytes: %w", n, errInvalid)
	}
	var value uint64
	for _, c := range b[:n] {
		value = value<<8 | uint64(c)
	}
	return value, n, nil
}

// children calls fn with the ID and data of each element in data, which
// is the data of an element read whole.
func children(data []byte, fn func(id uint32, data []byte) error) error {
	for len(data) > 0 {
		h, err := parseHeader(data)
		if err != nil {
			return err
		}
		if h.unknown || h.size > int64(len(data)-h.length) {
			return fmt.Errorf("element %X overruns its parent: %w", h.id, errInvalid)
		}
		end := h.length + int(h.size)
		if err := fn(h.id, data[h.length:end]); err != nil {
			return err
		}
		data = data[end:]
	}
	return nil
}

// unsigned reads an unsigned integer element.
func unsigned(data []byte) (uint64, error) {
	if len(data) > 8 {
		return 0, fmt.Errorf("integer of %d bytes: %w", len(data), errInvalid)
	}
	var value uint64
	for _, c := range data {
		value = value<<8 | uint64(c)
	}
	return value, nil
}
