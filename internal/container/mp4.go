package container

import (
	"encoding/binary"
	"fmt"
	"io"
	"slices"
	"time"
)

// topLevelBoxes holds the box types an MP4 or QuickTime file starts with.
var topLevelBoxes = map[string]bool{
	"ftyp": true, "styp": true, "moov": true, "mdat": true,
	"free": true, "skip": true, "wide": true, "pnot": true,
}

const (
	// maxChildren bounds the boxes walked in one container, each of
	// which may cost a read when the container is large.
	maxChildren = 1024
	// maxHeaderBox bounds the small boxes read whole: mvhd, mdhd, hdlr.
	maxHeaderBox = 4 << 10
	// emptyEdit is the media time of an edit that shows nothing.
	emptyEdit = -1
)

// box is an ISO BMFF box: its type, where it starts, where its data starts
// and where it ends.
type box struct {
	typ              string
	start, data, end int64
}

// readMP4 returns the presentation times of the sync samples of the first
// video track, from the sample tables in moov.
func readMP4(f *file) ([]time.Duration, error) {
	t, movieTimescale, err := mp4Video(f)
	if err != nil {
		return nil, err
	}
	_, times, err := t.syncSamples(f, movieTimescale)
	return times, err
}

// mp4Video returns the first video track of an MP4 file, from moov, and
// the movie's timescale.
func mp4Video(f *file) (track, uint32, error) {
	// moov is at the start of files made for streaming, else after mdat,
	// which is skipped by its size.
	var moov box
	found := false
	for pos, walked := int64(0), 0; pos < f.size && !found; walked++ {
		if walked == maxTopLevel {
			return track{}, 0, ErrNoIndex
		}
		b, err := f.box(pos, f.size)
		if err != nil {
			return track{}, 0, err
		}
		switch b.typ {
		case "moov":
			moov, found = b, true
		case "moof":
			// Samples in fragments are described next to them, all over
			// the file.
			return track{}, 0, ErrNoIndex
		}
		pos = b.end
	}
	if !found {
		return track{}, 0, ErrNoIndex
	}

	var movieTimescale uint32
	var traks []box
	err := f.boxes(moov, func(b box) error {
		switch b.typ {
		case "mvex":
			return ErrNoIndex
		case "mvhd":
			data, err := f.boxData(b, maxHeaderBox)
			if err != nil {
				return err
			}
			movieTimescale, err = headerTimescale(data)
			return err
		case "trak":
			traks = append(traks, b)
		}
		return nil
	})
	if err != nil {
		return track{}, 0, err
	}
	for _, trak := range traks {
		t, err := readTrak(f, trak)
		if err != nil {
			return track{}, 0, err
		}
		if t.handler == "vide" {
			return t, movieTimescale, nil
		}
	}
	return track{}, 0, ErrNoIndex
}

// track is what a trak box tells of its track, its sample tables left in
// the file until the track is known to be the one wanted.
type track struct {
	handler   string
	timescale uint32
	elst      []byte
	stbl      box
	hasStbl   bool
}

func readTrak(f *file, trak box) (track, error) {
	var t track
	err := f.boxes(trak, func(b box) error {
		switch b.typ {
		case "edts":
			return f.boxes(b, func(b box) error {
				if b.typ != "elst" {
					return nil
				}
				var err error
				t.elst, err = f.boxData(b, maxIndex)
				return err
			})
		case "mdia":
			return f.boxes(b, func(b box) error {
				switch b.typ {
				case "mdhd":
					data, err := f.boxData(b, maxHeaderBox)
					if err != nil {
						return err
					}
					t.timescale, err = headerTimescale(data)
					return err
				case "hdlr":
					data, err := f.boxData(b, maxHeaderBox)
					if err != nil {
						return err
					}
					if len(data) < 12 {
						return fmt.Errorf("hdlr of %d bytes: %w", len(data), errInvalid)
					}
					t.handler = string(data[8:12])
				case "minf":
					return f.boxes(b, func(b box) error {
						if b.typ == "stbl" {
							t.stbl, t.hasStbl = b, true
						}
						return nil
					})
				}
				return nil
			})
		}
		return nil
	})
	return t, err
}

// syncSamples returns the sync samples of the track, numbered from 0, and
// their presentation times, shifted by its edit list: those before the
// first the edit list keeps are left out.
func (t track) syncSamples(f *file, movieTimescale uint32) ([]uint64, []time.Duration, error) {
	if !t.hasStbl {
		return nil, nil, ErrNoIndex
	}
	var stts, ctts, stss []byte
	hasStss := false
	err := f.boxes(t.stbl, func(b box) error {
		var err error
		switch b.typ {
		case "stts":
			stts, err = f.boxData(b, maxIndex)
		case "ctts":
			ctts, err = f.boxData(b, maxIndex)
		case "stss":
			stss, err = f.boxData(b, maxIndex)
			hasStss = true
		}
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	if stts == nil {
		return nil, nil, ErrNoIndex
	}
	timeToSample, err := table(stts, "stts", 8)
	if err != nil {
		return nil, nil, err
	}
	var offsets []byte
	if ctts != nil {
		if offsets, err = table(ctts, "ctts", 8); err != nil {
			return nil, nil, err
		}
	}

	var total uint64
	for entry := range slices.Chunk(timeToSample, 8) {
		total += uint64(binary.BigEndian.Uint32(entry))
	}
	// Sync samples are numbered from 1; without stss every sample is one.
	var syncs []uint64
	if hasStss {
		numbers, err := table(stss, "stss", 4)
		if err != nil {
			return nil, nil, err
		}
		syncs = make([]uint64, 0, len(numbers)/4)
		for entry := range slices.Chunk(numbers, 4) {
			if number := uint64(binary.BigEndian.Uint32(entry)); number >= 1 && number <= total {
				syncs = append(syncs, number-1)
			}
		}
		slices.Sort(syncs)
		syncs = slices.Compact(syncs)
	} else {
		if total > maxKeyframes {
			return nil, nil, fmt.Errorf("%d samples, all sync: %w", total, errInvalid)
		}
		syncs = make([]uint64, total)
		for i := range syncs {
			syncs[i] = uint64(i)
		}
	}

	decode := runs{entries: timeToSample}
	composition := runs{entries: offsets}
	presented := make([]int64, 0, len(syncs))
	for _, sample := range syncs {
		dts, err := decode.decodeTime(sample)
		if err != nil {
			return nil, nil, err
		}
		presented = append(presented, dts+composition.offset(sample))
	}

	edit, err := readEdits(t.elst)
	if err != nil {
		return nil, nil, err
	}
	var shift int64
	if edit.shows {
		// FFmpeg presents the first sample the edit shows, the earliest at
		// or after its media time, at the time of the edit, and keeps the
		// samples from the last sync sample not after that media time,
		// which a decoder needs, giving those before negative times.
		if shift, err = firstPresented(timeToSample, offsets, edit.media); err != nil {
			return nil, nil, err
		}
		first := 0
		for i, pts := range presented {
			if pts <= edit.media {
				first = i
			}
		}
		presented, syncs = presented[first:], syncs[first:]
	}
	var delay time.Duration
	if edit.empty > 0 {
		if delay, err = duration(edit.empty, movieTimescale); err != nil {
			return nil, nil, err
		}
	}

	times := make([]time.Duration, 0, len(presented))
	for _, pts := range presented {
		d, err := duration(pts-shift, t.timescale)
		if err != nil {
			return nil, nil, err
		}
		times = append(times, d+delay)
	}
	return syncs, times, nil
}

// edit is the start of an edit list, as FFmpeg applies it: media is the
// media time the first edit showing something starts at, shows whether
// there is one, and empty the length of the empty edits before it, in the
// movie's timescale, which delay the track.
type edit struct {
	media, empty int64
	shows        bool
}

func readEdits(elst []byte) (edit, error) {
	var e edit
	if elst == nil {
		return e, nil
	}
	if len(elst) < 4 {
		return e, fmt.Errorf("elst of %d bytes: %w", len(elst), errInvalid)
	}
	size := 12
	if elst[0] == 1 {
		size = 20
	}
	entries, err := table(elst, "elst", size)
	if err != nil {
		return e, err
	}
	for entry := range slices.Chunk(entries, size) {
		var segment uint64
		var media int64
		if size == 20 {
			segment, media = binary.BigEndian.Uint64(entry), int64(binary.BigEndian.Uint64(entry[8:]))
		} else {
			segment, media = uint64(binary.BigEndian.Uint32(entry)), int64(int32(binary.BigEndian.Uint32(entry[4:])))
		}
		if media != emptyEdit {
			if media < 0 || media > maxTicks {
				return e, fmt.Errorf("edit at media time %d: %w", media, errInvalid)
			}
			e.media, e.shows = media, true
			return e, nil
		}
		if segment > uint64(maxTicks-e.empty) {
			return e, fmt.Errorf("empty edits too long: %w", errInvalid)
		}
		e.empty += int64(segment)
	}
	return e, nil
}

// maxTicks bounds the decode times and edit durations accepted, far
// beyond any real file, so that adding them cannot overflow.
const maxTicks = 1 << 62

// firstPresented returns the earliest presentation time at or after start,
// or start when no sample is presented that late. The tables are walked
// by spans of samples sharing their duration and composition offset, so
// that the work depends on their size, not on the counts they claim.
func firstPresented(stts, ctts []byte, start int64) (int64, error) {
	best, found := int64(0), false
	var dts int64
	var offset int64
	var offsetLeft uint64
	for entry := range slices.Chunk(stts, 8) {
		left := uint64(binary.BigEndian.Uint32(entry))
		delta := int64(binary.BigEndian.Uint32(entry[4:]))
		for left > 0 {
			for offsetLeft == 0 && len(ctts) >= 8 {
				offsetLeft = uint64(binary.BigEndian.Uint32(ctts))
				offset = int64(int32(binary.BigEndian.Uint32(ctts[4:])))
				ctts = ctts[8:]
			}
			n := left
			if offsetLeft > 0 {
				n = min(n, offsetLeft)
				offsetLeft -= n
			} else {
				// Samples past the composition offsets have none.
				offset = 0
			}
			// The samples of the span are presented at pts + k*delta, for
			// k below n: the first at or after start is computed.
			pts := dts + offset
			var k int64
			if need := start - pts; need > 0 {
				if delta == 0 {
					k = int64(n)
				} else {
					k = (need + delta - 1) / delta
				}
			}
			if k < int64(n) {
				if candidate := pts + k*delta; !found || candidate < best {
					best, found = candidate, true
				}
			}
			// n and delta are 32-bit, so their product fits.
			if dts += int64(n) * delta; dts > maxTicks {
				return 0, fmt.Errorf("decode time %d: %w", dts, errInvalid)
			}
			left -= n
		}
	}
	if !found {
		return start, nil
	}
	return best, nil
}

// runs walks a run-length table of (count, value) entries, as stts and
// ctts are, toward ascending sample numbers.
type runs struct {
	entries []byte
	// first is the number of the first sample of the current entry, and
	// time the decode time it starts at, for stts.
	first uint64
	time  int64
}

// decodeTime returns the decode time of sample, which follows those
// asked before and is in the table.
func (r *runs) decodeTime(sample uint64) (int64, error) {
	for len(r.entries) >= 8 {
		count := uint64(binary.BigEndian.Uint32(r.entries))
		delta := int64(binary.BigEndian.Uint32(r.entries[4:]))
		if sample < r.first+count {
			// The sample's distance to the run's start and delta are
			// 32-bit, so their product fits.
			if t := r.time + int64(sample-r.first)*delta; t <= maxTicks {
				return t, nil
			}
			break
		}
		if r.time += int64(count) * delta; r.time > maxTicks {
			break
		}
		r.first += count
		r.entries = r.entries[8:]
	}
	return 0, fmt.Errorf("decode time of sample %d: %w", sample, errInvalid)
}

// offset returns the composition offset of sample, which follows those
// asked before; samples past the table have none. Offsets are signed in
// both versions of ctts: the first version says they are not, but FFmpeg
// reads them as signed, and files carry negative ones in it.
func (r *runs) offset(sample uint64) int64 {
	for len(r.entries) >= 8 {
		count := uint64(binary.BigEndian.Uint32(r.entries))
		if sample < r.first+count {
			return int64(int32(binary.BigEndian.Uint32(r.entries[4:])))
		}
		r.first += count
		r.entries = r.entries[8:]
	}
	return 0
}

// table returns the entries of a full box holding a 32-bit entry count
// then entries of size bytes, checking they are all there.
func table(data []byte, name string, size int) ([]byte, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("%s of %d bytes: %w", name, len(data), errInvalid)
	}
	count := uint64(binary.BigEndian.Uint32(data[4:]))
	if count > uint64(len(data)-8)/uint64(size) {
		return nil, fmt.Errorf("%s of %d entries in %d bytes: %w", name, count, len(data), errInvalid)
	}
	return data[8 : 8+int(count)*size], nil
}

// headerTimescale reads the timescale of an mvhd or mdhd box, which share
// their layout.
func headerTimescale(data []byte) (uint32, error) {
	at := 12
	if len(data) > 0 && data[0] == 1 {
		at = 20
	}
	if len(data) < at+4 {
		return 0, fmt.Errorf("media header of %d bytes: %w", len(data), errInvalid)
	}
	return binary.BigEndian.Uint32(data[at:]), nil
}

// box reads the header of the box at off, within a parent ending at limit.
func (f *file) box(off, limit int64) (box, error) {
	if limit-off < 8 {
		return box{}, fmt.Errorf("box at %d: %w", off, io.ErrUnexpectedEOF)
	}
	b, err := f.span(off, min(16, limit-off))
	if err != nil {
		return box{}, err
	}
	size, length := uint64(binary.BigEndian.Uint32(b)), int64(8)
	switch size {
	case 0:
		// The box runs to the end of its parent, or of the file.
		size = uint64(limit - off)
	case 1:
		if len(b) < 16 {
			return box{}, fmt.Errorf("box at %d: %w", off, io.ErrUnexpectedEOF)
		}
		size, length = binary.BigEndian.Uint64(b[8:]), 16
	}
	if size < uint64(length) || size > uint64(limit-off) {
		return box{}, fmt.Errorf("box %q of %d bytes at %d overruns its parent: %w", b[4:8], size, off, errInvalid)
	}
	return box{typ: string(b[4:8]), start: off, data: off + length, end: off + int64(size)}, nil
}

// boxes calls fn with each box in parent.
func (f *file) boxes(parent box, fn func(b box) error) error {
	// Fewer than 8 bytes left are padding some writers leave.
	for pos, walked := parent.data, 0; parent.end-pos >= 8; walked++ {
		if walked == maxChildren {
			return fmt.Errorf("more than %d boxes in %q: %w", maxChildren, parent.typ, errInvalid)
		}
		b, err := f.box(pos, parent.end)
		if err != nil {
			return err
		}
		if err := fn(b); err != nil {
			return err
		}
		pos = b.end
	}
	return nil
}

// boxData returns the data of b when it is at most limit bytes.
func (f *file) boxData(b box, limit int64) ([]byte, error) {
	if b.end-b.data > limit {
		return nil, fmt.Errorf("box %q of %d bytes: %w", b.typ, b.end-b.data, errInvalid)
	}
	return f.span(b.data, b.end-b.data)
}
