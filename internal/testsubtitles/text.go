package testsubtitles

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"slices"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Caption is text shown from Start to End, a line a string.
type Caption struct {
	Lines      []string
	Start, End time.Duration
}

// Captions are captions drawn as subtitle pictures are: white text with a
// black outline on a transparent background, centered near the bottom of
// a canvas of CanvasWidth × CanvasHeight, in Go Regular of Size pixels.
type Captions struct {
	CanvasWidth, CanvasHeight int
	Size                      float64
	Shown                     []Caption
}

// The values of a caption's pixels.
const (
	blank   = 0
	fill    = 1
	outline = 2
)

// caption is a caption drawn: its size, its pixels' values, and where it
// sits on the canvas.
type caption struct {
	x, y, width, height int
	pixels              []byte
}

// draw draws a caption's lines, centered, near the bottom of the canvas.
func (c Captions) draw(lines []string) caption {
	parsed, err := opentype.Parse(goregular.TTF)
	if err != nil {
		panic(err)
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: c.Size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	defer face.Close()
	metrics := face.Metrics()
	lineHeight := metrics.Height.Ceil()
	border := max(int(c.Size/16), 2)
	pad := border + 2
	width := 0
	for _, line := range lines {
		width = max(width, font.MeasureString(face, line).Ceil())
	}
	width += 2 * pad
	height := lineHeight*len(lines) + 2*pad
	mask := image.NewAlpha(image.Rect(0, 0, width, height))
	for i, line := range lines {
		d := font.Drawer{Dst: mask, Src: image.Opaque, Face: face}
		d.Dot = fixed.P((width-font.MeasureString(face, line).Ceil())/2, pad+i*lineHeight+metrics.Ascent.Ceil())
		d.DrawString(line)
	}
	pixels := make([]byte, width*height)
	for y := range height {
		for x := range width {
			if mask.AlphaAt(x, y).A >= 128 {
				pixels[y*width+x] = fill
			}
		}
	}
	for y := range height {
		for x := range width {
			if pixels[y*width+x] != blank {
				continue
			}
		near:
			for dy := -border; dy <= border; dy++ {
				for dx := -border; dx <= border; dx++ {
					if xx, yy := x+dx, y+dy; xx >= 0 && xx < width && yy >= 0 && yy < height && pixels[yy*width+xx] == fill {
						pixels[y*width+x] = outline
						break near
					}
				}
			}
		}
	}
	return caption{x: (c.CanvasWidth - width) / 2, y: c.CanvasHeight - height - c.CanvasHeight/20, width: width, height: height, pixels: pixels}
}

// PGS writes the captions as a PGS stream: each caption an epoch of its
// own, cleared at its end.
func (c Captions) PGS() []byte {
	var out []byte
	segment := func(at time.Duration, kind byte, payload []byte) {
		header := []byte{'P', 'G', 0, 0, 0, 0, 0, 0, 0, 0, kind, 0, 0}
		binary.BigEndian.PutUint32(header[2:], uint32(at*90000/time.Second))
		binary.BigEndian.PutUint16(header[11:], uint16(len(payload)))
		out = append(append(out, header...), payload...)
	}
	for i, shown := range c.Shown {
		drawn := c.draw(shown.Lines)
		window := slices.Concat([]byte{1, 0}, u16(drawn.x), u16(drawn.y), u16(drawn.width), u16(drawn.height))
		composition := slices.Concat(u16(c.CanvasWidth), u16(c.CanvasHeight), []byte{0x10}, u16(2*i), []byte{0x80, 0, 0, 1},
			[]byte{0, 0, 0, 0}, u16(drawn.x), u16(drawn.y))
		segment(shown.Start, 0x16, composition)
		segment(shown.Start, 0x17, window)
		// White and black, as Y, Cr, Cb and alpha.
		segment(shown.Start, 0x14, []byte{0, 0, fill, 235, 128, 128, 255, outline, 16, 128, 128, 255})
		data := pgsRLE(drawn)
		const most = 0xffff - 11
		first := true
		for len(data) > 0 || first {
			n := min(len(data), most)
			flags := byte(0)
			if first {
				flags |= 0x80
			}
			if n == len(data) {
				flags |= 0x40
			}
			payload := []byte{0, 0, 0, flags}
			if first {
				length := len(pgsRLE(drawn)) + 4
				payload = slices.Concat(payload, []byte{byte(length >> 16), byte(length >> 8), byte(length)}, u16(drawn.width), u16(drawn.height))
			}
			segment(shown.Start, 0x15, append(payload, data[:n]...))
			data, first = data[n:], false
		}
		segment(shown.Start, 0x80, nil)
		segment(shown.End, 0x16, slices.Concat(u16(c.CanvasWidth), u16(c.CanvasHeight), []byte{0x10}, u16(2*i+1), []byte{0, 0, 0, 0}))
		segment(shown.End, 0x17, window)
		segment(shown.End, 0x80, nil)
	}
	return out
}

// pgsRLE writes a caption's pixels in PGS run lengths.
func pgsRLE(drawn caption) []byte {
	var out []byte
	for y := range drawn.height {
		row := drawn.pixels[y*drawn.width : (y+1)*drawn.width]
		for x := 0; x < len(row); {
			value := row[x]
			n := 1
			for x+n < len(row) && row[x+n] == value && n < 0x3fff {
				n++
			}
			switch {
			case value == blank && n < 64:
				out = append(out, 0, byte(n))
			case value == blank:
				out = append(out, 0, 0x40|byte(n>>8), byte(n))
			case n < 3:
				out = append(out, bytes.Repeat([]byte{value}, n)...)
			case n < 64:
				out = append(out, 0, 0x80|byte(n), value)
			default:
				out = append(out, 0, 0xc0|byte(n>>8), byte(n), value)
			}
			x += n
		}
		out = append(out, 0, 0)
	}
	return out
}

// VobSub writes the captions as a VobSub pair of one track in language,
// an ISO 639-1 code: its .idx file and its .sub file, a subpicture unit
// in as many 2048-byte packs as it takes for each caption.
func (c Captions) VobSub(language string) (idx string, sub []byte) {
	idx = "# VobSub index file, v7 (do not modify this line!)\nsize: " + fmt.Sprint(c.CanvasWidth) + "x" + fmt.Sprint(c.CanvasHeight) +
		"\npalette: 000000, ffffff, 000000" + strings.Repeat(", 000000", 13) + "\nid: " + language + ", index: 0\n"
	for _, shown := range c.Shown {
		drawn := c.draw(shown.Lines)
		var top, bottom []byte
		for y := range drawn.height {
			line := spuLine(drawn.pixels[y*drawn.width : (y+1)*drawn.width])
			if y%2 == 0 {
				top = append(top, line...)
			} else {
				bottom = append(bottom, line...)
			}
		}
		ms := shown.Start.Milliseconds()
		idx += fmt.Sprintf("timestamp: %02d:%02d:%02d:%03d, filepos: %09x\n", ms/3600000, ms/60000%60, ms/1000%60, ms%1000, len(sub))
		first := 4 + len(top) + len(bottom)
		second := first + 24
		right, last := drawn.x+drawn.width-1, drawn.y+drawn.height-1
		// Value 1 is palette color 1, white; value 2 color 2, black; both
		// opaque, the rest clear.
		spu := slices.Concat(u16(0), u16(first), top, bottom, u16(0), u16(second), []byte{3, 0x02, 0x10, 4, 0x0F, 0xF0},
			[]byte{5, byte(drawn.x >> 4), byte(drawn.x&15)<<4 | byte(right>>8), byte(right), byte(drawn.y >> 4), byte(drawn.y&15)<<4 | byte(last>>8), byte(last)},
			[]byte{6}, u16(4), u16(4+len(top)), []byte{1, 0xFF}, u16(int((shown.End-shown.Start)*90000/1024/time.Second)), u16(second), []byte{2, 0xFF})
		copy(spu, u16(len(spu)))
		for chunk := 0; chunk*2000 < len(spu); chunk++ {
			data := spu[chunk*2000 : min((chunk+1)*2000, len(spu))]
			var pes []byte
			if chunk == 0 {
				pts := uint64(shown.Start * 90000 / time.Second)
				pes = slices.Concat([]byte{0x81, 0x80, 5, 0x21 | byte(pts>>29)&0x0E, byte(pts >> 22), byte(pts>>14) | 1, byte(pts >> 7), byte(pts<<1) | 1, 0x20}, data)
			} else {
				pes = slices.Concat([]byte{0x81, 0x00, 0, 0x20}, data)
			}
			pack := slices.Concat([]byte{0, 0, 1, 0xBA, 0x44, 0, 4, 0, 4, 1, 1, 0x89, 0xC3, 0xF8, 0, 0, 1, 0xBD}, u16(len(pes)), pes)
			padding := 2048 - len(pack) - 6
			sub = append(sub, slices.Concat(pack, []byte{0, 0, 1, 0xBE}, u16(padding), bytes.Repeat([]byte{0xFF}, padding))...)
		}
	}
	return idx, sub
}

// spuLine writes a line of two-bit values in a subpicture's run lengths,
// padded to a byte: a run of n values v is n<<2|v in one to four nibbles.
func spuLine(row []byte) []byte {
	var nibbles []byte
	for x := 0; x < len(row); {
		value := row[x]
		n := 1
		for x+n < len(row) && row[x+n] == value && n < 255 {
			n++
		}
		code := n<<2 | int(value)
		switch {
		case n < 4:
			nibbles = append(nibbles, byte(code))
		case n < 16:
			nibbles = append(nibbles, byte(code>>4), byte(code&15))
		case n < 64:
			nibbles = append(nibbles, byte(code>>8), byte(code>>4&15), byte(code&15))
		default:
			nibbles = append(nibbles, byte(code>>12), byte(code>>8&15), byte(code>>4&15), byte(code&15))
		}
		x += n
	}
	if len(nibbles)%2 == 1 {
		nibbles = append(nibbles, 0)
	}
	out := make([]byte, len(nibbles)/2)
	for i := range out {
		out[i] = nibbles[2*i]<<4 | nibbles[2*i+1]
	}
	return out
}
