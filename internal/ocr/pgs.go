package ocr

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"iter"
	"sort"
	"time"

	"github.com/moodiness/polyfin/internal/container"
)

// PGS subtitles are display sets of segments: a presentation composition
// placing objects (pictures) on screen, the palettes and objects it uses,
// windows, and an end. A display set that places no object clears the
// screen. Objects and palettes last for an epoch, which a composition
// starting one resets: a later display set may show an object defined
// before.

// The kinds of PGS segments.
const (
	segmentPalette     = 0x14
	segmentObject      = 0x15
	segmentComposition = 0x16
	segmentWindow      = 0x17
	segmentEnd         = 0x80
)

// errInvalid reports pictures that cannot be read.
var errInvalid = errors.New("invalid subtitle pictures")

// lastShown is how long the last picture of a track shows when nothing
// ends it.
const lastShown = 5 * time.Second

// displaySet is a PGS display set: when it shows, and its segments as its
// file or block holds them, not copied: each, behind skip bytes of a
// header, its kind, the size of its payload and its payload.
type displaySet struct {
	at       time.Duration
	skip     int
	segments []byte
}

// PGS reads the pictures of a SUP file: each segment behind a "PG" header
// with its presentation time in 90 kHz ticks. It hands each picture to
// emit, in order, as soon as its end is known (see decodePGS).
//
// The file's segments are checked first, so that a file cut short hands
// over nothing. Its display sets are then decoded one at a time, each
// where the file holds it: none is copied, nor held once decoded. The
// pixels they compose are bounded by the file's size (see pixelBudget).
func PGS(ctx context.Context, sup []byte, emit func(Picture) error) error {
	for rest := sup; len(rest) > 0; {
		if len(rest) < 13 || rest[0] != 'P' || rest[1] != 'G' {
			return fmt.Errorf("%w: no PG header", errInvalid)
		}
		n := 13 + int(binary.BigEndian.Uint16(rest[11:13]))
		if n > len(rest) {
			return fmt.Errorf("%w: segment past the end", errInvalid)
		}
		rest = rest[n:]
	}
	return decodePGS(ctx, supSets(sup), newPixelBudget(len(sup)), emit)
}

// supSets are the display sets of a SUP file whose segments were checked.
func supSets(sup []byte) iter.Seq[displaySet] {
	return func(yield func(displaySet) bool) {
		for len(sup) > 0 {
			// A display set runs to its end segment, or to the file's end.
			n, ended := 0, false
			for n < len(sup) && !ended {
				ended = sup[n+10] == segmentEnd
				n += 13 + int(binary.BigEndian.Uint16(sup[n+11:n+13]))
			}
			if !yield(displaySet{at: ticks(binary.BigEndian.Uint32(sup[2:6])), skip: 10, segments: sup[:n]}) {
				return
			}
			sup = sup[n:]
		}
	}
}

// PGSBlocks reads the pictures of a Matroska PGS track, as PGS does: each
// block a display set, its segments without a header.
func PGSBlocks(ctx context.Context, blocks []container.Block, emit func(Picture) error) error {
	return decodePGS(ctx, blockSets(blocks), newPixelBudget(blocksSize(blocks)), emit)
}

// blockSets are the display sets of a Matroska PGS track's blocks.
func blockSets(blocks []container.Block) iter.Seq[displaySet] {
	return func(yield func(displaySet) bool) {
		for _, block := range blocks {
			if !yield(displaySet{at: block.Start, segments: block.Data}) {
				return
			}
		}
	}
}

// ticks converts 90 kHz ticks to a duration.
func ticks(t uint32) time.Duration {
	return time.Duration(t) * time.Second / 90000
}

// pgsObject is an object being defined or defined: its size, and of its
// run-length data what can show, the runs of each of its lines up to its
// width. The rest of what it is given is dropped as it is read (see
// trim): an object holds no more than its size asks for, however long its
// data, and a part of it is drawn from the runs of that part alone,
// however often and in however many display sets it is placed.
type pgsObject struct {
	width, height int
	// data is the data kept, then what is not read yet: no more than the
	// start of a run, which the next segment ends.
	data []byte
	// kept is how much of data is kept, line the line it has reached and
	// x how many pixels of that line it holds.
	kept, line, x int
	// marks are places in data, pgsMarkGap bytes or more apart: the runs
	// of a part are found from the mark before them (see seek).
	marks []pgsMark
}

// pgsMark is a place in an object's data: the run starting at offset is
// on line, from pixel x.
type pgsMark struct {
	offset  int
	line, x uint16
}

const (
	// pgsMarkGap bounds what is read of an object's data to reach a part
	// of it, and with it the marks kept: one for 64 bytes of data or
	// more.
	pgsMarkGap = 64
	// pgsWork is how many runs and pixels a composition draws between
	// two asks of its context: a few milliseconds of them.
	pgsWork = 1 << 22
)

// nextRun reads the run at i of run-length data: a byte other than 0 is
// a pixel of that color, 0 starts a run, 0 0 ends the line. It returns
// the run's color, its length, below 0 for the end of a line, and where
// the next run starts: 0 when the data ends within this one.
func nextRun(data []byte, i int) (color byte, n, next int) {
	if i >= len(data) {
		return 0, 0, 0
	}
	if c := data[i]; c != 0 {
		return c, 1, i + 1
	}
	if i+1 >= len(data) {
		return 0, 0, 0
	}
	flags := data[i+1]
	n = int(flags & 0x3f)
	switch {
	case flags == 0:
		return 0, -1, i + 2
	case flags>>6 == 0:
		return 0, n, i + 2
	case i+2 >= len(data):
		return 0, 0, 0
	case flags>>6 == 1:
		return 0, n<<8 | int(data[i+2]), i + 3
	case flags>>6 == 2:
		return data[i+2], n, i + 3
	case i+3 >= len(data):
		return 0, 0, 0
	}
	return data[i+3], n<<8 | int(data[i+2]), i + 4
}

// trim reads the data the object was given since it last did, and keeps
// of it what can show: the runs that have pixels, on a line of the
// object, until that line is full, and the ends of those lines. Each byte
// is read once, whatever follows it.
func (o *pgsObject) trim() {
	i := o.kept
	for {
		_, n, next := nextRun(o.data, i)
		if next == 0 {
			break
		}
		if o.line < o.height && (n < 0 || n > 0 && o.x < o.width) {
			o.kept += copy(o.data[o.kept:], o.data[i:next])
			if n >= 0 {
				o.x = min(o.x+n, o.width)
			} else {
				o.line, o.x = o.line+1, 0
			}
			last := 0
			if len(o.marks) > 0 {
				last = o.marks[len(o.marks)-1].offset
			}
			if o.line < o.height && o.kept-last >= pgsMarkGap {
				o.marks = append(o.marks, pgsMark{offset: o.kept, line: uint16(o.line), x: uint16(o.x)})
			}
		}
		i = next
	}
	o.data = o.data[:o.kept+copy(o.data[o.kept:], o.data[i:])]
}

// rows is how many of its lines the object's data reaches: those it
// ended, and the one it has pixels of. The lines after are never drawn.
func (o *pgsObject) rows() int {
	if o.x > 0 {
		return min(o.line+1, o.height)
	}
	return min(o.line, o.height)
}

// seek is where in data to read line y, one the data has reached, for a
// part whose left edge is pixel x: at a run of that line, which starts
// at pixel at, x or before it. It is found from the last mark before,
// less than pgsMarkGap bytes and a run away.
func (o *pgsObject) seek(y, x int) (i, at int) {
	line := 0
	if k := sort.Search(len(o.marks), func(k int) bool {
		m := o.marks[k]
		return int(m.line) > y || int(m.line) == y && int(m.x) > x
	}); k > 0 {
		m := o.marks[k-1]
		i, line, at = m.offset, int(m.line), int(m.x)
	}
	for line < y {
		_, n, next := nextRun(o.data, i)
		if next == 0 {
			return len(o.data), 0
		}
		if i = next; n < 0 {
			line, at = line+1, 0
		}
	}
	return i, at
}

// placement is an object a composition shows: where, cropped or not.
type placement struct {
	object          uint16
	x, y            int
	forced, cropped bool
	cropX, cropY    int
	cropW, cropH    int
}

// decodePGS turns display sets into pictures as it is given them: each
// display set that shows objects is a picture, until the next display
// set, which it hands to emit then. Only the picture showing is held: the
// caller decides how many it keeps. An error from emit stops the
// decoding, and so does ctx when it ends: it is asked before each display
// set, and as one's composition is drawn. So does budget when the pixels
// composed and prepared run past it.
func decodePGS(ctx context.Context, sets iter.Seq[displaySet], budget *pixelBudget, emit func(Picture) error) error {
	palettes := map[byte]*[256]pixel{}
	objects := map[uint16]*pgsObject{}
	var showing Picture
	shown := false
	for set := range sets {
		if err := ctx.Err(); err != nil {
			return err
		}
		var composed bool
		var paletteID byte
		var width, height int
		var placed []placement
		// A segment that says it is longer than what is left ends its
		// display set.
		for rest := set.segments; len(rest) >= set.skip+3; {
			rest = rest[set.skip:]
			n := 3 + int(binary.BigEndian.Uint16(rest[1:3]))
			if n > len(rest) {
				break
			}
			kind, data := rest[0], rest[3:n]
			rest = rest[n:]
			switch kind {
			case segmentComposition:
				if len(data) < 11 {
					return fmt.Errorf("%w: short composition", errInvalid)
				}
				composed = true
				width, height = int(binary.BigEndian.Uint16(data[0:2])), int(binary.BigEndian.Uint16(data[2:4]))
				if data[7]&0x80 != 0 {
					// An epoch starts: what the last one defined is gone.
					clear(palettes)
					clear(objects)
				}
				paletteID = data[9]
				placed = placed[:0]
				n := int(data[10])
				rest := data[11:]
				for range n {
					if len(rest) < 8 {
						return fmt.Errorf("%w: short composition object", errInvalid)
					}
					p := placement{object: binary.BigEndian.Uint16(rest[0:2]), forced: rest[3]&0x40 != 0, cropped: rest[3]&0x80 != 0,
						x: int(binary.BigEndian.Uint16(rest[4:6])), y: int(binary.BigEndian.Uint16(rest[6:8]))}
					rest = rest[8:]
					if p.cropped {
						if len(rest) < 8 {
							return fmt.Errorf("%w: short cropping", errInvalid)
						}
						p.cropX, p.cropY = int(binary.BigEndian.Uint16(rest[0:2])), int(binary.BigEndian.Uint16(rest[2:4]))
						p.cropW, p.cropH = int(binary.BigEndian.Uint16(rest[4:6])), int(binary.BigEndian.Uint16(rest[6:8]))
						rest = rest[8:]
					}
					placed = append(placed, p)
				}
			case segmentPalette:
				if len(data) < 2 {
					return fmt.Errorf("%w: short palette", errInvalid)
				}
				palette := palettes[data[0]]
				if palette == nil {
					palette = &[256]pixel{}
					palettes[data[0]] = palette
				}
				for entries := data[2:]; len(entries) >= 5; entries = entries[5:] {
					// Y, Cr, Cb, alpha: only the luminance and the alpha
					// matter to the text.
					palette[entries[0]] = pixel{luma: entries[1], alpha: entries[4]}
				}
			case segmentObject:
				if len(data) < 4 {
					return fmt.Errorf("%w: short object", errInvalid)
				}
				id, flags := binary.BigEndian.Uint16(data[0:2]), data[3]
				if flags&0x80 != 0 {
					if len(data) < 11 {
						return fmt.Errorf("%w: short object", errInvalid)
					}
					objects[id] = &pgsObject{width: int(binary.BigEndian.Uint16(data[7:9])), height: int(binary.BigEndian.Uint16(data[9:11])),
						data: append([]byte(nil), data[11:]...)}
				} else if object := objects[id]; object != nil {
					object.data = append(object.data, data[4:]...)
				}
				if object := objects[id]; object != nil {
					object.trim()
				}
			}
		}
		if !composed {
			continue
		}
		if shown {
			showing.End = set.at
			if err := emit(showing); err != nil {
				return err
			}
		}
		shown = false
		palette := palettes[paletteID]
		if len(placed) == 0 || palette == nil {
			continue
		}
		b, forced, err := composePGS(ctx, placed, objects, palette, width, height, budget)
		if err != nil {
			return err
		}
		prepared, ok, err := prepare(b, budget)
		if err != nil {
			return err
		}
		if ok {
			showing = Picture{Start: set.at, Forced: forced, Image: prepared}
			shown = true
		}
	}
	if shown {
		showing.End = showing.Start + lastShown
		return emit(showing)
	}
	return nil
}

// composePGS draws the objects a composition places, each decoded through
// the palette, onto a bitmap covering them all within the screen, the
// composition's video of width by height, at most maxWidth by maxHeight:
// what lies outside is neither drawn nor allocated, whatever the stream
// declares, nor are an object's lines past those its data reaches. The
// bitmap's pixels are taken from budget first. ctx is asked as the
// objects are drawn, every pgsWork runs and pixels of them.
func composePGS(ctx context.Context, placed []placement, objects map[uint16]*pgsObject, palette *[256]pixel, width, height int,
	budget *pixelBudget) (bitmap, bool, error) {
	type drawn struct {
		object *pgsObject
		// part is the part of the object shown, by the object's pixels, and
		// at where it shows on screen.
		part, at image.Rectangle
	}
	var parts []drawn
	forced := false
	screen := image.Rect(0, 0, min(width, maxWidth), min(height, maxHeight))
	var bounds image.Rectangle
	for _, p := range placed {
		object := objects[p.object]
		if object == nil {
			continue
		}
		part := image.Rect(0, 0, object.width, object.rows())
		if p.cropped {
			part = part.Intersect(image.Rect(p.cropX, p.cropY, p.cropX+p.cropW, p.cropY+p.cropH))
		}
		at := part.Sub(part.Min).Add(image.Pt(p.x, p.y)).Intersect(screen)
		if at.Empty() {
			continue
		}
		part = at.Sub(image.Pt(p.x, p.y)).Add(part.Min)
		parts = append(parts, drawn{object, part, at})
		forced = forced || p.forced
		bounds = bounds.Union(at)
	}
	if len(parts) == 0 {
		return bitmap{}, false, nil
	}
	if err := budget.spend(bounds.Dx() * bounds.Dy()); err != nil {
		return bitmap{}, false, err
	}
	canvas := newBitmap(bounds.Dx(), bounds.Dy())
	work := 0
	for _, d := range parts {
		work += drawPGSObject(canvas, d.object, palette, d.part, d.at.Min.Sub(bounds.Min).Sub(d.part.Min))
		if work >= pgsWork {
			if err := ctx.Err(); err != nil {
				return bitmap{}, false, err
			}
			work = 0
		}
	}
	return canvas, forced, nil
}

// drawPGSObject draws the opaque pixels of the part shown of an object
// onto canvas, the object's origin at origin on canvas. It reads the
// runs of that part alone, on each of its lines from a mark before its
// left edge to its right edge, and the rest of the object not at all. It
// returns what the drawing cost: the lines and runs read and the pixels
// drawn.
func drawPGSObject(canvas bitmap, object *pgsObject, palette *[256]pixel, shown image.Rectangle, origin image.Point) int {
	work := 0
	for y := shown.Min.Y; y < shown.Max.Y && y <= object.line; y++ {
		work++
		for i, x := object.seek(y, shown.Min.X); x < shown.Max.X; {
			color, n, next := nextRun(object.data, i)
			if next == 0 || n < 0 {
				break
			}
			work++
			end := min(x+n, object.width)
			if value := palette[color]; value.alpha > 0 {
				from, to := max(x, shown.Min.X), min(end, shown.Max.X)
				for col := from; col < to; col++ {
					canvas.set(origin.X+col, origin.Y+y, value)
				}
				work += max(to-from, 0)
			}
			x, i = end, next
		}
	}
	return work
}
