// Package testsubtitles writes picture subtitles for tests: a red
// rectangle, as a PGS stream (a .sup file) and as a VobSub pair.
package testsubtitles

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Rectangle is a rectangle in red at X, Y, Width × Height, on a canvas of
// CanvasWidth × CanvasHeight, shown during each span of Shown.
type Rectangle struct {
	CanvasWidth, CanvasHeight, X, Y, Width, Height int
	Shown                                          [][2]time.Duration
}

func u16(v int) []byte { return []byte{byte(v >> 8), byte(v)} }

// PGS writes the rectangle as a PGS stream, in palette color 1.
func (r Rectangle) PGS() []byte {
	var out []byte
	segment := func(at time.Duration, kind byte, payload []byte) {
		ticks := uint32(at / time.Second * 90000)
		header := []byte{'P', 'G', 0, 0, 0, 0, 0, 0, 0, 0, kind, 0, 0}
		binary.BigEndian.PutUint32(header[2:], ticks)
		binary.BigEndian.PutUint16(header[11:], uint16(len(payload)))
		out = append(append(out, header...), payload...)
	}
	window := slices.Concat([]byte{1, 0}, u16(r.X), u16(r.Y), u16(r.Width), u16(r.Height))
	// Each line: width pixels of color 1, then the end of the line.
	var lines []byte
	for range r.Height {
		lines = append(lines, 0, 0xC0|byte(r.Width>>8), byte(r.Width), 1, 0, 0)
	}
	object := slices.Concat([]byte{0, 0, 0, 0xC0}, []byte{byte((len(lines) + 4) >> 16), byte((len(lines) + 4) >> 8), byte(len(lines) + 4)},
		u16(r.Width), u16(r.Height), lines)
	for i, span := range r.Shown {
		composition := slices.Concat(u16(r.CanvasWidth), u16(r.CanvasHeight), []byte{0x10}, u16(2*i), []byte{0x80, 0, 0, 1}, []byte{0, 0, 0, 0},
			u16(r.X), u16(r.Y))
		segment(span[0], 0x16, composition)
		segment(span[0], 0x17, window)
		// Y, Cr, Cb and alpha of red.
		segment(span[0], 0x14, []byte{0, 0, 1, 81, 240, 90, 255})
		segment(span[0], 0x15, object)
		segment(span[0], 0x80, nil)
		// A composition without objects clears the screen.
		segment(span[1], 0x16, slices.Concat(u16(r.CanvasWidth), u16(r.CanvasHeight), []byte{0x10}, u16(2*i+1), []byte{0, 0, 0, 0}))
		segment(span[1], 0x17, window)
		segment(span[1], 0x80, nil)
	}
	return out
}

// VobSub writes the rectangle as a VobSub pair, its .idx file, of one
// English track, and its .sub file, of an MPEG-2 pack for each span, in
// palette color 1.
func (r Rectangle) VobSub() (idx string, sub []byte) {
	idx = "# VobSub index file, v7 (do not modify this line!)\nsize: " + fmt.Sprint(r.CanvasWidth) + "x" + fmt.Sprint(r.CanvasHeight) +
		"\npalette: 000000, ff0000" + strings.Repeat(", 000000", 14) + "\nid: en, index: 0\n"
	// Each line of each field: color 1 up to the end of the line.
	top := slices.Repeat([]byte{0, 1}, (r.Height+1)/2)
	bottom := slices.Repeat([]byte{0, 1}, r.Height/2)
	for _, span := range r.Shown {
		ms := span[0].Milliseconds()
		idx += fmt.Sprintf("timestamp: %02d:%02d:%02d:%03d, filepos: %09x\n", ms/3600000, ms/60000%60, ms/1000%60, ms%1000, len(sub))
		// The control sequences: colors, opacity, area and fields, shown at
		// once; then hidden after the span, in units of 1024/90000 s.
		first := 4 + len(top) + len(bottom)
		second := first + 24
		right, last := r.X+r.Width-1, r.Y+r.Height-1
		spu := slices.Concat(u16(0), u16(first), top, bottom, u16(0), u16(second), []byte{3, 0x00, 0x10, 4, 0x00, 0xF0},
			[]byte{5, byte(r.X >> 4), byte(r.X&15)<<4 | byte(right>>8), byte(right), byte(r.Y >> 4), byte(r.Y&15)<<4 | byte(last>>8), byte(last)},
			[]byte{6}, u16(4), u16(4+len(top)), []byte{1, 0xFF}, u16(int((span[1]-span[0])*90000/1024/time.Second)), u16(second), []byte{2, 0xFF})
		copy(spu, u16(len(spu)))
		pts := uint64(span[0] / time.Second * 90000)
		pes := slices.Concat([]byte{0x81, 0x80, 5, 0x21 | byte(pts>>29)&0x0E, byte(pts >> 22), byte(pts>>14) | 1, byte(pts >> 7), byte(pts<<1) | 1, 0x20}, spu)
		pack := slices.Concat([]byte{0, 0, 1, 0xBA, 0x44, 0, 4, 0, 4, 1, 1, 0x89, 0xC3, 0xF8, 0, 0, 1, 0xBD}, u16(len(pes)), pes)
		padding := 2048 - len(pack) - 6
		sub = append(sub, slices.Concat(pack, []byte{0, 0, 1, 0xBE}, u16(padding), bytes.Repeat([]byte{0xFF}, padding))...)
	}
	return idx, sub
}
