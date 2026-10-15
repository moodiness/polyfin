package ocr

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
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

// displaySet is a PGS display set: its segments, kind first, and when it
// shows.
type displaySet struct {
	at       time.Duration
	segments [][]byte
}

// PGS reads the pictures of a SUP file: each segment behind a "PG" header
// with its presentation time in 90 kHz ticks. It hands each picture to
// emit, in order, as soon as its end is known (see decodePGS).
func PGS(sup []byte, emit func(Picture) error) error {
	var sets []displaySet
	open := false
	for len(sup) > 0 {
		if len(sup) < 13 || sup[0] != 'P' || sup[1] != 'G' {
			return fmt.Errorf("%w: no PG header", errInvalid)
		}
		at := ticks(binary.BigEndian.Uint32(sup[2:6]))
		n := 13 + int(binary.BigEndian.Uint16(sup[11:13]))
		if n > len(sup) {
			return fmt.Errorf("%w: segment past the end", errInvalid)
		}
		if !open {
			sets = append(sets, displaySet{at: at})
			open = true
		}
		set := &sets[len(sets)-1]
		set.segments = append(set.segments, segmentOf(sup[10], sup[13:n]))
		if sup[10] == segmentEnd {
			open = false
		}
		sup = sup[n:]
	}
	return decodePGS(sets, emit)
}

// PGSBlocks reads the pictures of a Matroska PGS track, as PGS does: each
// block a display set, its segments without a header.
func PGSBlocks(blocks []container.Block, emit func(Picture) error) error {
	sets := make([]displaySet, 0, len(blocks))
	for _, block := range blocks {
		set := displaySet{at: block.Start}
		data := block.Data
		for len(data) >= 3 {
			n := 3 + int(binary.BigEndian.Uint16(data[1:3]))
			if n > len(data) {
				break
			}
			set.segments = append(set.segments, segmentOf(data[0], data[3:n]))
			data = data[n:]
		}
		sets = append(sets, set)
	}
	return decodePGS(sets, emit)
}

func segmentOf(kind byte, payload []byte) []byte {
	return append([]byte{kind}, payload...)
}

// ticks converts 90 kHz ticks to a duration.
func ticks(t uint32) time.Duration {
	return time.Duration(t) * time.Second / 90000
}

// pgsObject is an object being defined or defined: its size and its
// run-length data.
type pgsObject struct {
	width, height int
	data          []byte
}

// placement is an object a composition shows: where, cropped or not.
type placement struct {
	object          uint16
	x, y            int
	forced, cropped bool
	cropX, cropY    int
	cropW, cropH    int
}

// decodePGS turns display sets into pictures: each display set that shows
// objects is a picture, until the next display set, which it hands to
// emit then. Only the picture showing is held: the caller decides how many
// it keeps. An error from emit stops the decoding.
func decodePGS(sets []displaySet, emit func(Picture) error) error {
	palettes := map[byte]*[256]pixel{}
	objects := map[uint16]*pgsObject{}
	var showing Picture
	shown := false
	for _, set := range sets {
		var composed bool
		var paletteID byte
		var width, height int
		var placed []placement
		for _, segment := range set.segments {
			kind, data := segment[0], segment[1:]
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
		b, forced := composePGS(placed, objects, palette, width, height)
		if prepared, ok := prepare(b); ok {
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
// declares.
func composePGS(placed []placement, objects map[uint16]*pgsObject, palette *[256]pixel, width, height int) (bitmap, bool) {
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
		part := image.Rect(0, 0, object.width, object.height)
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
		return bitmap{}, false
	}
	canvas := newBitmap(bounds.Dx(), bounds.Dy())
	for _, d := range parts {
		drawPGSObject(canvas, d.object, palette, d.part, d.at.Min.Sub(bounds.Min).Sub(d.part.Min))
	}
	return canvas, forced
}

// drawPGSObject decodes an object's run-length data, a byte other than 0
// a pixel of that color, 0 starting a run, 0 0 ending the line, and draws
// the opaque pixels of its part shown onto canvas, the object's origin at
// origin on canvas. The rest of the object is decoded no further than its
// data, and drawn nowhere.
func drawPGSObject(canvas bitmap, object *pgsObject, palette *[256]pixel, shown image.Rectangle, origin image.Point) {
	data := object.data
	x, y := 0, 0
	put := func(color byte, n int) {
		end := min(x+n, object.width)
		if value := palette[color]; value.alpha > 0 && y >= shown.Min.Y {
			for col := max(x, shown.Min.X); col < min(end, shown.Max.X); col++ {
				canvas.set(origin.X+col, origin.Y+y, value)
			}
		}
		x = end
	}
	for i := 0; i < len(data) && y < shown.Max.Y; {
		c := data[i]
		i++
		if c != 0 {
			put(c, 1)
			continue
		}
		if i >= len(data) {
			return
		}
		flags := data[i]
		i++
		switch {
		case flags == 0:
			x, y = 0, y+1
		case flags>>6 == 0:
			put(0, int(flags&0x3f))
		case flags>>6 == 1:
			if i >= len(data) {
				return
			}
			put(0, int(flags&0x3f)<<8|int(data[i]))
			i++
		case flags>>6 == 2:
			if i >= len(data) {
				return
			}
			put(data[i], int(flags&0x3f))
			i++
		default:
			if i+1 >= len(data) {
				return
			}
			put(data[i+1], int(flags&0x3f)<<8|int(data[i]))
			i += 2
		}
	}
}
