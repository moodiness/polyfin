// Package mosaic draws the images Polyfin makes for the collections and
// libraries that have none: the posters of their first titles side by
// side, under a gradient, with their name written across them.
package mosaic

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"unicode"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/text/unicode/norm"
)

// The sizes images are drawn at: a 2:3 poster, and a 16:9 wide image.
// Apps asking for smaller ones get them resized.
const (
	PosterWidth  = 1000
	PosterHeight = 1500
	WideWidth    = 1920
	WideHeight   = 1080
)

// Tiles is how many posters an image shows at most: four for a poster,
// six for a wide image.
func Tiles(wide bool) int {
	if wide {
		return 6
	}
	return 4
}

// gap is the space between posters, in the background's color.
const gap = 6

var background = color.RGBA{R: 0x14, G: 0x16, B: 0x1a, A: 0xff}

// Draw draws the image of name from posters, in order: a poster, or a wide
// image. Posters missing (nil) are left out; with none, the image is the
// gradient and the name alone.
func Draw(name string, posters []image.Image, wide bool, fonts *Fonts) *image.RGBA {
	width, height := PosterWidth, PosterHeight
	if wide {
		width, height = WideWidth, WideHeight
	}
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	var shown []image.Image
	for _, poster := range posters {
		if poster != nil && !poster.Bounds().Empty() {
			shown = append(shown, poster)
		}
	}
	shown = shown[:min(len(shown), Tiles(wide))]
	for i, cell := range cells(len(shown), wide, canvas.Bounds()) {
		cover(canvas, cell, shown[i])
	}
	shade(canvas)
	write(canvas, name, wide, fonts)
	return canvas
}

// cells divides bounds into the cells of n posters: for a poster, a 2 by 2
// grid of four, two side by side for two or three, or one; for a wide
// image, a 3 by 2 grid of six, three side by side for three to five, two,
// or one.
func cells(n int, wide bool, bounds image.Rectangle) []image.Rectangle {
	columns, rows := 1, 1
	switch {
	case n == 0:
		return nil
	case wide && n >= 6:
		columns, rows = 3, 2
	case wide && n >= 3:
		columns = 3
	case !wide && n >= 4:
		columns, rows = 2, 2
	case n >= 2:
		columns = 2
	}
	result := make([]image.Rectangle, 0, columns*rows)
	for row := range rows {
		for column := range columns {
			x0 := bounds.Min.X + column*bounds.Dx()/columns
			x1 := bounds.Min.X + (column+1)*bounds.Dx()/columns
			y0 := bounds.Min.Y + row*bounds.Dy()/rows
			y1 := bounds.Min.Y + (row+1)*bounds.Dy()/rows
			cell := image.Rect(x0, y0, x1, y1)
			// Half a gap on each inner side.
			if column > 0 {
				cell.Min.X += gap / 2
			}
			if column < columns-1 {
				cell.Max.X -= gap / 2
			}
			if row > 0 {
				cell.Min.Y += gap / 2
			}
			if row < rows-1 {
				cell.Max.Y -= gap / 2
			}
			result = append(result, cell)
		}
	}
	return result
}

// cover scales poster to fill cell, keeping its proportions: what goes
// past the cell is cut, evenly on both sides across, and from the bottom
// rather than the top, where titles show their faces and names.
func cover(canvas *image.RGBA, cell image.Rectangle, poster image.Image) {
	source := poster.Bounds()
	sw, sh := float64(source.Dx()), float64(source.Dy())
	cw, ch := float64(cell.Dx()), float64(cell.Dy())
	cut := source
	if sw/sh > cw/ch {
		// Wider than the cell: cut both sides.
		w := int(math.Round(sh * cw / ch))
		cut.Min.X += (source.Dx() - w) / 2
		cut.Max.X = cut.Min.X + w
	} else {
		// Taller: cut a third of the excess from the top.
		h := int(math.Round(sw * ch / cw))
		cut.Min.Y += (source.Dy() - h) / 3
		cut.Max.Y = cut.Min.Y + h
	}
	xdraw.CatmullRom.Scale(canvas, cell, poster, cut, draw.Src, nil)
}

// shade darkens the image under a gradient, darker at the bottom, so that
// the name reads on any poster.
func shade(canvas *image.RGBA) {
	bounds := canvas.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		t := float64(y-bounds.Min.Y) / float64(bounds.Dy())
		alpha := 0.32 + 0.56*math.Pow(t, 1.4)
		row := image.Rect(bounds.Min.X, y, bounds.Max.X, y+1)
		draw.Draw(canvas, row, image.NewUniform(color.NRGBA{A: uint8(alpha * 255)}), image.Point{}, draw.Over)
	}
}

// layout is how a name is written: its lines, at a size.
type layout struct {
	lines []string
	size  float64
}

// The sizes a name is written at, in pixels: from the first down to the
// last, the largest at which it fits on maxLines lines. A name too long
// for the smallest size is cut, with an ellipsis.
var (
	posterSizes = sizes(132, 60)
	wideSizes   = sizes(150, 68)
)

func sizes(largest, smallest float64) []float64 {
	var result []float64
	for size := largest; size > smallest; size *= 0.92 {
		result = append(result, math.Round(size))
	}
	return append(result, smallest)
}

// writer measures and draws text at one size, each character with the
// face of its font (see Fonts).
type writer struct {
	fonts *Fonts
	size  float64
	faces map[*sfnt.Font]font.Face
}

func (w *writer) face(r rune) font.Face {
	f := w.fonts.fontFor(r)
	if f == nil {
		return nil
	}
	if face, ok := w.faces[f]; ok {
		return face
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: w.size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		face = nil
	}
	w.faces[f] = face
	return face
}

// width measures text, in pixels; characters no font has take no room.
func (w *writer) width(text string) float64 {
	var total fixed.Int26_6
	for _, r := range text {
		if face := w.face(r); face != nil {
			advance, _ := face.GlyphAdvance(r)
			total += advance
		}
	}
	return float64(total) / 64
}

// wrap breaks text into lines no wider than width, at spaces; a word wider
// than width on its own is broken where it reaches it.
func (w *writer) wrap(text string, width float64) []string {
	var lines []string
	line := ""
	for _, word := range words(text) {
		candidate := word
		if line != "" {
			candidate = line + " " + word
		}
		if w.width(candidate) <= width {
			line = candidate
			continue
		}
		if line != "" {
			lines = append(lines, line)
		}
		line = word
		for w.width(line) > width {
			runes := []rune(line)
			cut := len(runes) - 1
			for cut > 1 && w.width(string(runes[:cut])) > width {
				cut--
			}
			lines = append(lines, string(runes[:cut]))
			line = string(runes[cut:])
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// words splits text at spaces, keeping a mark set apart by a space with
// its word, as French sets « », colons and question marks: an opening mark
// with the word after it, any other with the word before it.
func words(text string) []string {
	var result []string
	opening := ""
	for _, word := range strings.Fields(text) {
		marks := strings.IndexFunc(word, func(r rune) bool { return !unicode.IsPunct(r) }) < 0
		switch {
		case marks && strings.IndexFunc(word, func(r rune) bool { return !unicode.In(r, unicode.Ps, unicode.Pi) }) < 0:
			opening += word + " "
		case marks && len(result) > 0:
			result[len(result)-1] += " " + opening + word
			opening = ""
		default:
			result = append(result, opening+word)
			opening = ""
		}
	}
	if opening != "" {
		result = append(result, strings.TrimSpace(opening))
	}
	return result
}

// fit finds how name is written within width on maxLines lines.
func fit(name string, width float64, maxLines int, steps []float64, fonts *Fonts) layout {
	var w *writer
	var lines []string
	for _, size := range steps {
		w = &writer{fonts: fonts, size: size, faces: map[*sfnt.Font]font.Face{}}
		lines = w.wrap(name, width)
		if len(lines) <= maxLines {
			return layout{lines: lines, size: size}
		}
	}
	lines = lines[:maxLines]
	last := strings.TrimRightFunc(lines[maxLines-1], unicode.IsSpace)
	for last != "" && w.width(last+"…") > width {
		runes := []rune(last)
		last = strings.TrimRightFunc(string(runes[:len(runes)-1]), unicode.IsSpace)
	}
	lines[maxLines-1] = last + "…"
	return layout{lines: lines, size: w.size}
}

// write writes name across the middle of the image, centered, in white
// over a soft shadow.
func write(canvas *image.RGBA, name string, wide bool, fonts *Fonts) {
	name = strings.Join(strings.Fields(norm.NFC.String(name)), " ")
	if name == "" {
		return
	}
	bounds := canvas.Bounds()
	maxLines, steps := 3, posterSizes
	if wide {
		maxLines, steps = 2, wideSizes
	}
	l := fit(name, float64(bounds.Dx())*0.84, maxLines, steps, fonts)
	w := &writer{fonts: fonts, size: l.size, faces: map[*sfnt.Font]font.Face{}}
	ascent, descent := metrics(fonts.primary(), l.size)
	lineHeight := (ascent + descent) * 1.08
	top := float64(bounds.Min.Y) + (float64(bounds.Dy())-lineHeight*float64(len(l.lines)))/2
	shadow := image.NewUniform(color.NRGBA{A: 150})
	offset := math.Max(2, l.size*0.035)
	for i, line := range l.lines {
		x := float64(bounds.Min.X) + (float64(bounds.Dx())-w.width(line))/2
		baseline := top + float64(i)*lineHeight + (lineHeight-ascent-descent)/2 + ascent
		w.draw(canvas, line, x+offset, baseline+offset, shadow)
		w.draw(canvas, line, x, baseline, image.White)
	}
}

// metrics are the ascent and descent of a font at size, in pixels.
func metrics(f *sfnt.Font, size float64) (ascent, descent float64) {
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return size * 0.9, size * 0.25
	}
	m := face.Metrics()
	return float64(m.Ascent) / 64, float64(m.Descent) / 64
}

// draw draws text from x on baseline, in color.
func (w *writer) draw(canvas *image.RGBA, text string, x, baseline float64, ink image.Image) {
	dot := fixed.Point26_6{X: fixed.Int26_6(x * 64), Y: fixed.Int26_6(baseline * 64)}
	for _, r := range text {
		face := w.face(r)
		if face == nil {
			continue
		}
		dr, mask, maskp, advance, ok := face.Glyph(dot, r)
		if ok {
			draw.DrawMask(canvas, dr, ink, image.Point{}, mask, maskp, draw.Over)
		}
		dot.X += advance
	}
}
