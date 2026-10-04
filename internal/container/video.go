package container

import (
	"bytes"
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"
)

// Video is the first video track of a Matroska or MP4 file as its index
// describes it: its codec, and where each of its keyframes is, so that
// a keyframe is read and decoded alone, without the media around it.
type Video struct {
	// CodecID names the codec as Matroska does, such as V_MPEG4/ISO/AVC.
	CodecID string
	// CodecPrivate is what a decoder needs before the first frame, as
	// Matroska's CodecPrivate holds it: an MP4 track's avcC, hvcC or av1C.
	CodecPrivate []byte
	// Settings is the data of a Matroska Video element describing the
	// frames: their size, and the shape they are shown in.
	Settings []byte
	// Keyframes are the times of the keyframes, ascending, without
	// duplicates.
	Keyframes []time.Duration

	size int64
	// frames holds where each keyframe is, in the order of Keyframes.
	frames []frameAt
	// track is a Matroska track's number, encodings the content encodings
	// to undo on its frames.
	track     uint64
	encodings []contentEncoding
	matroska  bool
}

// frameAt is where a keyframe is. An MP4 sample's place is known exactly,
// n bytes at off. A Matroska block is in the Cluster at off, relative bytes
// into the Cluster's data when hasRelative; its size is not known before
// its header is read.
type frameAt struct {
	off, n      int64
	relative    int64
	hasRelative bool
}

// ErrUnsupportedCodec reports a video track whose codec Polyfin does not
// hand a decoder frame by frame. It is an ErrUnreadable.
var ErrUnsupportedCodec = fmt.Errorf("unsupported video codec: %w", ErrUnreadable)

const (
	// maxFrame bounds the size of one frame read: a keyframe of 4K video
	// at a high bitrate takes a few MB.
	maxFrame = 32 << 20
	// The first read of a Matroska keyframe, whose size its index does
	// not give, is a guess: half as large again as the keyframes read so
	// far, on average, within these bounds. A larger one takes a second
	// request for the rest.
	minFrameGuess   = 64 << 10
	maxFrameGuess   = 4 << 20
	firstFrameGuess = 256 << 10
	// A source serving several ranges with one request is asked at most
	// batchFrames frames, about batchBytes, at once.
	batchFrames = 16
	batchBytes  = 16 << 20
)

// OpenVideo reads the index of the first video track of a Matroska, WebM
// or MP4 file through r: its codec and the keyframes its index lists, the
// Cues of a Matroska file or the sample tables of an MP4 file. ErrNoIndex
// when the file has none; ErrUnsupportedCodec when its codec cannot be
// decoded frame by frame. size is the file size.
func OpenVideo(ctx context.Context, r Reader, size int64) (*Video, error) {
	f := &file{ctx: ctx, r: r, size: size, window: window}
	head, err := f.span(0, min(size, 8))
	if err != nil {
		return nil, err
	}
	var v *Video
	switch {
	case bytes.HasPrefix(head, ebmlMagic):
		v, err = matroskaVideo(f)
	case len(head) == 8 && topLevelBoxes[string(head[4:8])]:
		v, err = mp4VideoFrames(f)
	default:
		return nil, ErrNoIndex
	}
	if err != nil {
		return nil, err
	}
	if len(v.Keyframes) == 0 {
		return nil, ErrNoIndex
	}
	v.size = size
	return v, nil
}

// matroskaVideo reads the first video track of a Matroska file and the
// CuePoints of its keyframes.
func matroskaVideo(f *file) (*Video, error) {
	m, err := openMatroska(f)
	if err != nil {
		return nil, err
	}
	number, err := m.videoTrack()
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(m.tracks, func(t Track) bool { return t.Number == number })
	track := m.tracks[i]
	if !track.Decodable {
		return nil, fmt.Errorf("the frames of track %d are encrypted or compressed in an unsupported way: %w", number, ErrUnsupportedCodec)
	}
	cues, err := m.cuesData(f, number)
	if err != nil {
		return nil, err
	}
	points, err := videoCues(cues, number, m.scale)
	if err != nil {
		return nil, err
	}
	v := &Video{CodecID: track.CodecID, CodecPrivate: track.CodecPrivate, Settings: track.Video,
		track: number, encodings: m.frames[number], matroska: true}
	for _, point := range points {
		off := m.segment.data + int64(point.cluster)
		if point.cluster >= uint64(f.size) || off >= f.size {
			return nil, m.outside(point.cluster, "Cluster of a keyframe")
		}
		v.Keyframes = append(v.Keyframes, point.time)
		v.frames = append(v.frames, frameAt{off: off, relative: int64(point.relative), hasRelative: point.hasRelative})
	}
	return v, nil
}

// videoCue is a CuePoint of the video track: its time, and its positions
// relative to the Segment's data and to its Cluster's data.
type videoCue struct {
	time              time.Duration
	cluster, relative uint64
	hasRelative       bool
}

// videoCues returns the CuePoints giving a Cluster for track, by time,
// the first of each time.
func videoCues(cues []byte, track, scale uint64) ([]videoCue, error) {
	var points []videoCue
	err := children(cues, func(id uint32, point []byte) error {
		switch id {
		case idVoid, idCRC32:
			return nil
		case idCuePoint:
		default:
			return fmt.Errorf("element %X in Cues: %w", id, errInvalid)
		}
		var ticks uint64
		var hasTime, found bool
		var cue videoCue
		err := children(point, func(id uint32, data []byte) error {
			var err error
			switch id {
			case idCueTime:
				ticks, err = unsigned(data)
				hasTime = true
			case idCueTrackPositions:
				if found {
					return nil
				}
				var number uint64
				var position videoCue
				var hasCluster bool
				err = children(data, func(id uint32, data []byte) error {
					var err error
					switch id {
					case idCueTrack:
						number, err = unsigned(data)
					case idCueClusterPosition:
						position.cluster, err = unsigned(data)
						hasCluster = true
					case idCueRelativePosition:
						position.relative, err = unsigned(data)
						position.hasRelative = true
					}
					return err
				})
				if err == nil && number == track && hasCluster {
					cue, found = position, true
				}
			}
			return err
		})
		if err != nil {
			return err
		}
		if !hasTime {
			return fmt.Errorf("CuePoint without a time: %w", errInvalid)
		}
		if !found {
			return nil
		}
		if len(points) == maxKeyframes {
			return fmt.Errorf("more than %d CuePoints: %w", maxKeyframes, errInvalid)
		}
		if cue.time, err = scaled(ticks, scale); err != nil {
			return err
		}
		points = append(points, cue)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(points, func(a, b videoCue) int { return cmp.Compare(a.time, b.time) })
	return slices.CompactFunc(points, func(a, b videoCue) bool { return a.time == b.time }), nil
}

// mp4VideoFrames reads the first video track of an MP4 file: its sample
// description, and the times and places of its sync samples.
func mp4VideoFrames(f *file) (*Video, error) {
	t, movieTimescale, err := mp4Video(f)
	if err != nil {
		return nil, err
	}
	samples, times, err := t.syncSamples(f, movieTimescale)
	if err != nil {
		return nil, err
	}
	v := &Video{}
	var stsd, stsz, stsc, stco []byte
	wide := false
	err = f.boxes(t.stbl, func(b box) error {
		var err error
		switch b.typ {
		case "stsd":
			stsd, err = f.boxData(b, maxIndex)
		case "stsz":
			stsz, err = f.boxData(b, maxIndex)
		case "stsc":
			stsc, err = f.boxData(b, maxIndex)
		case "stco":
			stco, err = f.boxData(b, maxIndex)
		case "co64":
			stco, err = f.boxData(b, maxIndex)
			wide = true
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if stsd == nil || stsz == nil || stsc == nil || stco == nil {
		return nil, ErrNoIndex
	}
	if err := v.describe(stsd); err != nil {
		return nil, err
	}
	places, err := sampleRanges(samples, stsz, stsc, stco, wide)
	if err != nil {
		return nil, err
	}
	type keyframe struct {
		time  time.Duration
		place Range
	}
	keyframes := make([]keyframe, len(times))
	for i := range times {
		if places[i].Off < 0 || places[i].Off >= f.size || int64(places[i].N) > f.size-places[i].Off {
			return nil, fmt.Errorf("sample %d past the end of the file: %w", samples[i], errInvalid)
		}
		keyframes[i] = keyframe{times[i], places[i]}
	}
	slices.SortStableFunc(keyframes, func(a, b keyframe) int { return cmp.Compare(a.time, b.time) })
	keyframes = slices.CompactFunc(keyframes, func(a, b keyframe) bool { return a.time == b.time })
	for _, k := range keyframes {
		v.Keyframes = append(v.Keyframes, k.time)
		v.frames = append(v.frames, frameAt{off: k.place.Off, n: int64(k.place.N)})
	}
	return v, nil
}

// mp4Codecs maps the sample entries whose frames Polyfin decodes alone to
// the Matroska codecs they are, and the box holding what Matroska keeps as
// CodecPrivate.
var mp4Codecs = map[string]struct{ codec, config string }{
	"avc1": {"V_MPEG4/ISO/AVC", "avcC"},
	"avc3": {"V_MPEG4/ISO/AVC", "avcC"},
	"hvc1": {"V_MPEGH/ISO/HEVC", "hvcC"},
	"hev1": {"V_MPEGH/ISO/HEVC", "hvcC"},
	"dvh1": {"V_MPEGH/ISO/HEVC", "hvcC"},
	"dvhe": {"V_MPEGH/ISO/HEVC", "hvcC"},
	"av01": {"V_AV1", "av1C"},
	"vp09": {"V_VP9", ""},
}

// visualEntry is the size of a VisualSampleEntry's fields, after its box
// header and before its child boxes.
const visualEntry = 78

// describe reads the first sample entry of an stsd box: the codec, its
// configuration, and the frames' size and pixel shape as a Video element.
func (v *Video) describe(stsd []byte) error {
	if len(stsd) < 8+8 {
		return fmt.Errorf("stsd of %d bytes: %w", len(stsd), errInvalid)
	}
	entry := stsd[8:]
	size := binary.BigEndian.Uint32(entry)
	if size < 8+visualEntry || uint64(size) > uint64(len(entry)) {
		return fmt.Errorf("sample entry of %d bytes: %w", size, errInvalid)
	}
	entry = entry[:size]
	typ := string(entry[4:8])
	codec, ok := mp4Codecs[typ]
	if !ok {
		return fmt.Errorf("sample entry %q: %w", typ, ErrUnsupportedCodec)
	}
	v.CodecID = codec.codec
	width := binary.BigEndian.Uint16(entry[8+24:])
	height := binary.BigEndian.Uint16(entry[8+26:])
	var hSpacing, vSpacing uint32
	for rest := entry[8+visualEntry:]; len(rest) >= 8; {
		n := binary.BigEndian.Uint32(rest)
		if n < 8 || uint64(n) > uint64(len(rest)) {
			break
		}
		child, data := string(rest[4:8]), rest[8:n]
		switch {
		case codec.config != "" && child == codec.config:
			v.CodecPrivate = slices.Clone(data)
		case child == "pasp" && len(data) >= 8:
			hSpacing, vSpacing = binary.BigEndian.Uint32(data), binary.BigEndian.Uint32(data[4:])
		}
		rest = rest[n:]
	}
	if codec.config != "" && v.CodecPrivate == nil {
		return fmt.Errorf("sample entry %q without %s: %w", typ, codec.config, ErrUnsupportedCodec)
	}
	var settings []byte
	settings = appendUint(settings, idPixelWidth, uint64(width))
	settings = appendUint(settings, idPixelHeight, uint64(height))
	if hSpacing > 0 && vSpacing > 0 && hSpacing != vSpacing {
		settings = appendUint(settings, idDisplayWidth, uint64(width)*uint64(hSpacing)/uint64(vSpacing))
		settings = appendUint(settings, idDisplayHeight, uint64(height))
	}
	v.Settings = settings
	return nil
}

// The Matroska Video element's children an MP4 track's frames are
// described with.
const (
	idPixelWidth    = 0xB0
	idPixelHeight   = 0xBA
	idDisplayWidth  = 0x54B0
	idDisplayHeight = 0x54BA
)

// appendUint appends an unsigned integer element.
func appendUint(b []byte, id uint32, value uint64) []byte {
	b = appendID(b, id)
	n := 1
	for value>>(8*n) != 0 && n < 8 {
		n++
	}
	b = append(b, 0x80|byte(n))
	for i := n - 1; i >= 0; i-- {
		b = append(b, byte(value>>(8*i)))
	}
	return b
}

// appendID appends an element ID, whose length marker is part of it.
func appendID(b []byte, id uint32) []byte {
	switch {
	case id > 0xFFFFFF:
		return append(b, byte(id>>24), byte(id>>16), byte(id>>8), byte(id))
	case id > 0xFFFF:
		return append(b, byte(id>>16), byte(id>>8), byte(id))
	case id > 0xFF:
		return append(b, byte(id>>8), byte(id))
	}
	return append(b, byte(id))
}

// sampleRanges returns where samples, ascending, are in the file, from
// the sample sizes (stsz), the samples of each chunk (stsc) and where
// each chunk starts (stco, or co64 when wide).
func sampleRanges(samples []uint64, stsz, stsc, stco []byte, wide bool) ([]Range, error) {
	if len(stsz) < 12 {
		return nil, fmt.Errorf("stsz of %d bytes: %w", len(stsz), errInvalid)
	}
	uniform := binary.BigEndian.Uint32(stsz[4:])
	total := uint64(binary.BigEndian.Uint32(stsz[8:]))
	var sizes []byte
	if uniform == 0 {
		if total > uint64(len(stsz)-12)/4 {
			return nil, fmt.Errorf("stsz of %d entries in %d bytes: %w", total, len(stsz), errInvalid)
		}
		sizes = stsz[12 : 12+total*4]
	}
	sizeOf := func(sample uint64) int64 {
		if sizes == nil {
			return int64(uniform)
		}
		return int64(binary.BigEndian.Uint32(sizes[sample*4:]))
	}
	chunks, err := table(stsc, "stsc", 12)
	if err != nil {
		return nil, err
	}
	width := 4
	if wide {
		width = 8
	}
	offsets, err := table(stco, "stco", width)
	if err != nil {
		return nil, err
	}
	chunkCount := uint64(len(offsets) / width)
	chunkOffset := func(chunk uint64) int64 {
		if wide {
			return int64(binary.BigEndian.Uint64(offsets[chunk*8:]))
		}
		return int64(binary.BigEndian.Uint32(offsets[chunk*4:]))
	}

	result := make([]Range, len(samples))
	next := 0
	sample := uint64(0)
	for entry := 0; entry*12 < len(chunks) && next < len(samples); entry++ {
		first := uint64(binary.BigEndian.Uint32(chunks[entry*12:]))
		perChunk := uint64(binary.BigEndian.Uint32(chunks[entry*12+4:]))
		last := chunkCount + 1
		if (entry+1)*12 < len(chunks) {
			last = uint64(binary.BigEndian.Uint32(chunks[(entry+1)*12:]))
		}
		if first == 0 || last < first || last > chunkCount+1 || perChunk == 0 {
			return nil, fmt.Errorf("stsc entry %d: %w", entry, errInvalid)
		}
		for chunk := first; chunk < last && next < len(samples); chunk++ {
			end := sample + perChunk
			if end > total {
				return nil, fmt.Errorf("chunk %d past the last sample: %w", chunk, errInvalid)
			}
			if samples[next] >= end {
				sample = end
				continue
			}
			off := chunkOffset(chunk - 1)
			for s := sample; s < end && next < len(samples); s++ {
				n := sizeOf(s)
				if samples[next] == s {
					if n > maxFrame {
						return nil, fmt.Errorf("sample %d of %d bytes: %w", s, n, errInvalid)
					}
					result[next] = Range{Off: off, N: int(n)}
					next++
				}
				off += n
			}
			sample = end
		}
	}
	if next < len(samples) {
		return nil, fmt.Errorf("sample %d in no chunk: %w", samples[next], errInvalid)
	}
	return result, nil
}

// ReadFrames reads the keyframes numbered by indexes, ascending, in the
// order of Keyframes, through f, and passes each frame to fn in turn, as a
// decoder takes it: a Matroska frame with its content encodings undone.
// An MP4 keyframe takes one request; a Matroska one, whose size its index
// does not give, one or rarely two. When f is also a RangeFetcher, several
// keyframes are read with each request.
func (v *Video) ReadFrames(ctx context.Context, f Fetcher, indexes []int, fn func(index int, frame []byte) error) error {
	r := &frameReader{v: v, f: f, guess: firstFrameGuess}
	_, r.ranges = f.(RangeFetcher)
	for len(indexes) > 0 {
		batch := []Range{}
		var bytes int64
		for _, index := range indexes {
			if index < 0 || index >= len(v.frames) {
				return fmt.Errorf("no keyframe %d", index)
			}
			place := r.first(v.frames[index])
			if len(batch) > 0 && (!r.ranges || len(batch) == batchFrames || bytes+int64(place.N) > batchBytes) {
				break
			}
			batch = append(batch, place)
			bytes += int64(place.N)
		}
		data, err := r.fetchBatch(ctx, batch)
		if err != nil {
			return err
		}
		for i, place := range batch {
			index := indexes[i]
			frame, err := r.frame(ctx, v.frames[index], data[i], place.Off)
			if err != nil {
				return fmt.Errorf("keyframe at %v: %w", v.Keyframes[index], err)
			}
			if err := fn(index, frame); err != nil {
				return err
			}
		}
		indexes = indexes[len(batch):]
	}
	return nil
}

// frameReader reads keyframes, learning as it goes how long Cluster
// headers are and how large keyframes are.
type frameReader struct {
	v *Video
	f Fetcher
	// ranges is set while f serves several ranges with one request.
	ranges bool
	// header is the length of the Cluster headers, once one was read.
	header int64
	// guess is how much is read first of a Matroska keyframe; read and
	// count sum up the keyframes read.
	guess, read, count int64
}

// first is the range read first for a keyframe: an MP4 sample exactly;
// for a Matroska block, a guess of its size, from the start of its
// Cluster until the length of a Cluster header is known, or when its
// index does not say where in the Cluster the block is.
func (r *frameReader) first(place frameAt) Range {
	if !r.v.matroska {
		return Range{Off: place.off, N: int(place.n)}
	}
	off := place.off
	n := r.guess
	switch {
	case !place.hasRelative:
		n += maxHeader
	case r.header == 0 && place.relative <= minFrameGuess:
		n += maxHeader + place.relative
	case r.header == 0:
		off += maxHeader + place.relative
	default:
		off += r.header + place.relative
	}
	return Range{Off: off, N: int(min(n, r.v.size-off))}
}

// fetchBatch reads ranges with one request when the source serves
// several at once, else with one each.
func (r *frameReader) fetchBatch(ctx context.Context, ranges []Range) ([][]byte, error) {
	if rf, ok := r.f.(RangeFetcher); ok && r.ranges && len(ranges) > 1 {
		data, err := rf.FetchRanges(ctx, ranges)
		switch {
		case errors.Is(err, ErrMultiRangeUnsupported):
			r.ranges = false
		case err != nil:
			return nil, err
		case len(data) != len(ranges):
			return nil, fmt.Errorf("fetching %d ranges: %d answered: %w", len(ranges), len(data), io.ErrUnexpectedEOF)
		default:
			return data, nil
		}
	}
	data := make([][]byte, len(ranges))
	for i, place := range ranges {
		var err error
		if data[i], err = r.fetch(ctx, place.Off, int64(place.N)); err != nil {
			return nil, err
		}
	}
	return data, nil
}

// fetch reads exactly the n bytes at off.
func (r *frameReader) fetch(ctx context.Context, off, n int64) ([]byte, error) {
	data, err := r.f.Fetch(ctx, off, int(n))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) < n {
		return nil, fmt.Errorf("fetching %d bytes at %d: %w", n, off, io.ErrUnexpectedEOF)
	}
	return data[:n], nil
}

// frame returns the keyframe at place from data, read at start, reading
// what data lacks.
func (r *frameReader) frame(ctx context.Context, place frameAt, data []byte, start int64) ([]byte, error) {
	if !r.v.matroska {
		if int64(len(data)) < place.n {
			return nil, fmt.Errorf("fetching %d bytes at %d: %w", place.n, place.off, io.ErrUnexpectedEOF)
		}
		return data[:place.n], nil
	}
	var at int64
	switch {
	case start == place.off:
		// The read starts with the Cluster's header.
		h, err := parseHeader(data)
		if err != nil || h.id != idCluster {
			return nil, fmt.Errorf("the Cues point to no Cluster at %d: %w", place.off, errInvalid)
		}
		r.header = int64(h.length)
		if !place.hasRelative {
			return r.firstBlock(ctx, place.off+int64(h.length), data, start)
		}
		at = place.off + int64(h.length) + place.relative
	case r.header == 0:
		at = place.off + maxHeader + place.relative
	default:
		at = place.off + r.header + place.relative
	}
	frame, err := r.block(ctx, at, data, start)
	if !errors.Is(err, errMisplaced) || start == place.off {
		return frame, err
	}
	// The Cluster's header is not as long as assumed.
	head, err := r.fetch(ctx, place.off, min(maxHeader, r.v.size-place.off))
	if err != nil {
		return nil, err
	}
	h, err := parseHeader(head)
	if err != nil || h.id != idCluster {
		return nil, fmt.Errorf("the Cues point to no Cluster at %d: %w", place.off, errInvalid)
	}
	if real := place.off + int64(h.length) + place.relative; real != at {
		frame, err = r.block(ctx, real, nil, 0)
	}
	if errors.Is(err, errMisplaced) {
		return nil, fmt.Errorf("no keyframe where the Cues point in the Cluster at %d: %w", place.off, errInvalid)
	}
	return frame, err
}

// firstBlock returns the frame of the first block of the track in the
// Cluster whose data starts at from, which must be a keyframe. Only the
// elements whose headers data, read at start, holds are walked.
func (r *frameReader) firstBlock(ctx context.Context, from int64, data []byte, start int64) ([]byte, error) {
	for at := from; at+min(maxHeader, r.v.size-at) <= start+int64(len(data)); {
		frame, err := r.block(ctx, at, data, start)
		if !errors.Is(err, errMisplaced) {
			return frame, err
		}
		h, err := parseHeader(data[at-start:])
		if err != nil || h.unknown || h.id > 0xFFFFFF {
			break
		}
		at += int64(h.length) + h.size
	}
	return nil, fmt.Errorf("no keyframe at the start of the Cluster at %d: %w", from, errInvalid)
}

// elementAt reads the element header at at, from *data read at *start
// when it holds it, else from a new read, which replaces them.
func (r *frameReader) elementAt(ctx context.Context, at int64, data *[]byte, start *int64) (header, error) {
	if at >= r.v.size {
		return header{}, fmt.Errorf("element past the end of the file: %w", io.ErrUnexpectedEOF)
	}
	if at < *start || at+min(maxHeader, r.v.size-at) > *start+int64(len(*data)) {
		read, err := r.fetch(ctx, at, min(r.guess, r.v.size-at))
		if err != nil {
			return header{}, err
		}
		*data, *start = read, at
	}
	h, err := parseHeader((*data)[at-*start:])
	if err != nil {
		return header{}, fmt.Errorf("element at %d: %w", at, errInvalid)
	}
	return h, nil
}

// block returns the frame of the SimpleBlock, or the Block of the
// BlockGroup, at at: a keyframe of the track. Its bytes are taken from
// data, read at start, as far as it holds them. errMisplaced when no
// keyframe of the track is there.
func (r *frameReader) block(ctx context.Context, at int64, data []byte, start int64) ([]byte, error) {
	h, err := r.elementAt(ctx, at, &data, &start)
	switch {
	case err != nil && !errors.Is(err, errInvalid):
		return nil, err
	case err != nil, h.id != idSimpleBlock && h.id != idBlockGroup, h.unknown, h.size > maxFrame:
		return nil, errMisplaced
	case h.size > r.v.size-at-int64(h.length):
		return nil, fmt.Errorf("block at %d past the end of the file: %w", at, io.ErrUnexpectedEOF)
	}
	from, to := at+int64(h.length), at+int64(h.length)+h.size
	var payload []byte
	if end := start + int64(len(data)); to <= end {
		payload = data[from-start : to-start]
	} else {
		rest, err := r.fetch(ctx, end, to-end)
		if err != nil {
			return nil, err
		}
		payload = append(slices.Clip(data[from-start:]), rest...)
	}
	block := payload
	if h.id == idBlockGroup {
		block = nil
		referenced := false
		err := children(payload, func(id uint32, data []byte) error {
			switch id {
			case idBlock:
				block = data
			case idReferenceBlock:
				referenced = true
			}
			return nil
		})
		if err != nil || block == nil || referenced {
			return nil, errMisplaced
		}
	}
	number, length, err := blockTrack(block)
	if err != nil || number != r.v.track || len(block) < length+3 {
		return nil, errMisplaced
	}
	flags := block[length+2]
	if h.id == idSimpleBlock && flags&0x80 == 0 {
		return nil, errMisplaced
	}
	if flags&0x06 != 0 {
		return nil, fmt.Errorf("laced video block at %d: %w", at, ErrUnreadable)
	}
	r.read += to - at
	r.count++
	r.guess = min(max((r.read/r.count)*3/2, minFrameGuess), maxFrameGuess)
	frame := block[length+3:]
	if len(r.v.encodings) > 0 {
		return decode(frame, r.v.encodings, maxFrame)
	}
	// The frame is copied out of the read, which may hold several.
	return slices.Clone(frame), nil
}
