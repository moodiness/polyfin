package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"

	"golang.org/x/image/vector"
)

// The logo's five triangles (assets/polyfin.svg), in a 100 by 100 box,
// with their colors.
var logo = []struct {
	points [3][2]float32
	color  color.NRGBA
}{
	{[3][2]float32{{15.50, 86.20}, {31.92, 63.21}, {46.70, 86.20}}, color.NRGBA{0x6D, 0x4A, 0xFF, 0xFF}},
	{[3][2]float32{{52.91, 86.20}, {68.08, 55.86}, {84.94, 86.20}}, color.NRGBA{0x4C, 0x3F, 0xD9, 0xFF}},
	{[3][2]float32{{34.91, 61.20}, {64.70, 54.58}, {49.80, 84.37}}, color.NRGBA{0x5B, 0x63, 0xF6, 0xFF}},
	{[3][2]float32{{36.10, 57.25}, {50.25, 36.80}, {64.41, 50.95}}, color.NRGBA{0x3E, 0x8E, 0xF7, 0xFF}},
	{[3][2]float32{{52.66, 34.11}, {73.22, 16.71}, {66.89, 48.35}}, color.NRGBA{0x22, 0xD3, 0xEE, 0xFF}},
}

// icon draws the logo size pixels wide, as a PNG: in its colors, or in
// black for macOS's menu bar, which tints a template icon itself. A faded
// icon tells that Polyfin does not run.
func icon(size int, template, faded bool) []byte {
	// The triangles fill 15 to 85 of the box: they are scaled to fill the
	// icon, with a margin.
	scale := float32(size) / 76
	offset := float32(12)
	rgba := image.NewNRGBA(image.Rect(0, 0, size, size))
	for _, triangle := range logo {
		r := vector.NewRasterizer(size, size)
		for i, p := range triangle.points {
			x, y := (p[0]-offset)*scale, (p[1]-offset)*scale
			if i == 0 {
				r.MoveTo(x, y)
			} else {
				r.LineTo(x, y)
			}
		}
		r.ClosePath()
		c := triangle.color
		switch {
		case template:
			c = color.NRGBA{A: 0xFF}
		case faded:
			grey := uint8((uint16(c.R) + uint16(c.G) + uint16(c.B)) / 3)
			c = color.NRGBA{grey, grey, grey, 0xFF}
		}
		if faded {
			c.A = 0x70
		}
		r.Draw(rgba, rgba.Bounds(), image.NewUniform(c), image.Point{})
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, rgba)
	return buf.Bytes()
}

// ico wraps a PNG in an icon file, which Windows' tray loads.
func ico(pngData []byte, size int) []byte {
	var buf bytes.Buffer
	// The header: reserved, type 1 (icon), one image.
	_ = binary.Write(&buf, binary.LittleEndian, [3]uint16{0, 1, 1})
	dimension := uint8(size)
	if size >= 256 {
		dimension = 0
	}
	buf.Write([]byte{dimension, dimension, 0, 0})
	_ = binary.Write(&buf, binary.LittleEndian, [2]uint16{1, 32})
	_ = binary.Write(&buf, binary.LittleEndian, [2]uint32{uint32(len(pngData)), 22})
	buf.Write(pngData)
	return buf.Bytes()
}
