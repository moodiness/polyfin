package mosaic

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/vector"
)

// Emblems are the images of Polyfin's own views that list no titles of
// their own to draw a mosaic from: wide, the same for every user and in
// every language, and without words, as apps write the view's name under
// it. Each draws the view's sign in the colors of Polyfin's logo, from
// violet to cyan, over the dark of the mosaics, its shards faint in a
// corner.
const (
	EmblemLiveTV      = "livetv"
	EmblemReplay      = "replay"
	EmblemCollections = "collections"
	EmblemPlaylists   = "playlists"
)

// The logo's colors, from its violet shard to its cyan one.
var (
	logoViolet = color.NRGBA{R: 0x6d, G: 0x4a, B: 0xff, A: 0xff}
	logoIndigo = color.NRGBA{R: 0x4c, G: 0x3f, B: 0xd9, A: 0xff}
	logoAccent = color.NRGBA{R: 0x5b, G: 0x63, B: 0xf6, A: 0xff}
	logoBlue   = color.NRGBA{R: 0x3e, G: 0x8e, B: 0xf7, A: 0xff}
	logoCyan   = color.NRGBA{R: 0x22, G: 0xd3, B: 0xee, A: 0xff}
)

// logoShards are the logo's five shards, on a square of 100, with their
// colors.
var logoShards = []struct {
	points []float32
	color  color.NRGBA
}{
	{[]float32{15.50, 86.20, 31.92, 63.21, 46.70, 86.20}, logoViolet},
	{[]float32{52.91, 86.20, 68.08, 55.86, 84.94, 86.20}, logoIndigo},
	{[]float32{34.91, 61.20, 64.70, 54.58, 49.80, 84.37}, logoAccent},
	{[]float32{36.10, 57.25, 50.25, 36.80, 64.41, 50.95}, logoBlue},
	{[]float32{52.66, 34.11, 73.22, 16.71, 66.89, 48.35}, logoCyan},
}

// DrawEmblem draws the wide image of emblem name, 1,920 by 1,080 pixels;
// an unknown name gives the background alone.
func DrawEmblem(name string) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, WideWidth, WideHeight))
	glow(canvas)
	// The shards, faint, in the bottom right corner.
	for _, shard := range logoShards {
		faint := shard.color
		faint.A = 0x38
		p := newPen(canvas)
		points := make([]float32, len(shard.points))
		for i := 0; i < len(points); i += 2 {
			points[i], points[i+1] = 1310+shard.points[i]*6, 473+shard.points[i+1]*6
		}
		p.polygon(points...)
		p.fill(image.NewUniform(faint))
	}
	ink := &sweep{from: logoViolet, to: logoCyan, x0: 760, y0: 760, x1: 1160, y1: 320}
	switch name {
	case EmblemLiveTV:
		liveTV(canvas, ink)
	case EmblemReplay:
		replay(canvas, ink)
	case EmblemCollections:
		collections(canvas, ink)
	case EmblemPlaylists:
		playlists(canvas, ink)
	}
	return canvas
}

// glow fills the image with the mosaics' background, lit from its middle
// in the logo's accent, darker toward the edges.
func glow(canvas *image.RGBA) {
	bounds := canvas.Bounds()
	cx, cy := float64(bounds.Dx())/2, float64(bounds.Dy())*0.46
	reach := float64(bounds.Dx()) * 0.55
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			d := math.Hypot(float64(x)-cx, (float64(y)-cy)*1.4) / reach
			light := 0.24 * math.Max(0, 1-d) * math.Max(0, 1-d)
			dark := 1 - 0.35*math.Min(1, d)
			i := canvas.PixOffset(x, y)
			canvas.Pix[i] = uint8(math.Round(float64(background.R)*dark + float64(logoAccent.R)*light))
			canvas.Pix[i+1] = uint8(math.Round(float64(background.G)*dark + float64(logoAccent.G)*light))
			canvas.Pix[i+2] = uint8(math.Round(float64(background.B)*dark + float64(logoAccent.B)*light))
			canvas.Pix[i+3] = 0xff
		}
	}
}

// sweep is the emblems' ink: from one color to another along a line, as
// the logo goes from violet to cyan.
type sweep struct {
	from, to       color.NRGBA
	x0, y0, x1, y1 float64
}

func (s *sweep) ColorModel() color.Model { return color.NRGBAModel }
func (s *sweep) Bounds() image.Rectangle {
	return image.Rect(-1e9, -1e9, 1e9, 1e9)
}
func (s *sweep) At(x, y int) color.Color {
	dx, dy := s.x1-s.x0, s.y1-s.y0
	t := ((float64(x)-s.x0)*dx + (float64(y)-s.y0)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	mix := func(a, b uint8) uint8 { return uint8(math.Round(float64(a) + (float64(b)-float64(a))*t)) }
	return color.NRGBA{R: mix(s.from.R, s.to.R), G: mix(s.from.G, s.to.G), B: mix(s.from.B, s.to.B), A: 0xff}
}

// pen traces shapes on a canvas, filled at once: shapes traced the other
// way round within one fill cut holes.
type pen struct {
	*vector.Rasterizer
	canvas *image.RGBA
}

func newPen(canvas *image.RGBA) pen {
	return pen{vector.NewRasterizer(canvas.Bounds().Dx(), canvas.Bounds().Dy()), canvas}
}

func (p pen) fill(ink image.Image) {
	p.Draw(p.canvas, p.canvas.Bounds(), ink, image.Point{})
}

// rounded traces a rectangle with rounded corners of radius r, clockwise
// on screen, or the other way round when back is set.
func (p pen) rounded(x0, y0, x1, y1, r float32, back bool) {
	// k places the control points of a quarter circle drawn as a cubic.
	const k = 0.5523
	if !back {
		p.MoveTo(x0+r, y0)
		p.LineTo(x1-r, y0)
		p.CubeTo(x1-r+r*k, y0, x1, y0+r-r*k, x1, y0+r)
		p.LineTo(x1, y1-r)
		p.CubeTo(x1, y1-r+r*k, x1-r+r*k, y1, x1-r, y1)
		p.LineTo(x0+r, y1)
		p.CubeTo(x0+r-r*k, y1, x0, y1-r+r*k, x0, y1-r)
		p.LineTo(x0, y0+r)
		p.CubeTo(x0, y0+r-r*k, x0+r-r*k, y0, x0+r, y0)
	} else {
		p.MoveTo(x0+r, y0)
		p.CubeTo(x0+r-r*k, y0, x0, y0+r-r*k, x0, y0+r)
		p.LineTo(x0, y1-r)
		p.CubeTo(x0, y1-r+r*k, x0+r-r*k, y1, x0+r, y1)
		p.LineTo(x1-r, y1)
		p.CubeTo(x1-r+r*k, y1, x1, y1-r+r*k, x1, y1-r)
		p.LineTo(x1, y0+r)
		p.CubeTo(x1, y0+r-r*k, x1-r+r*k, y0, x1-r, y0)
		p.LineTo(x0+r, y0)
	}
	p.ClosePath()
}

// frame traces the outline of a rounded rectangle, width w thick.
func (p pen) frame(x0, y0, x1, y1, r, w float32) {
	p.rounded(x0, y0, x1, y1, r, false)
	p.rounded(x0+w, y0+w, x1-w, y1-w, max(r-w, 1), true)
}

// polygon traces the polygon of points, x then y.
func (p pen) polygon(points ...float32) {
	p.MoveTo(points[0], points[1])
	for i := 2; i < len(points); i += 2 {
		p.LineTo(points[i], points[i+1])
	}
	p.ClosePath()
}

// play traces a play sign, a triangle pointing right with its middle at
// x, y and h high.
func (p pen) play(x, y, h float32) {
	w := h * 0.87
	p.polygon(x-w/3, y-h/2, x+w*2/3, y, x-w/3, y+h/2)
}

// stroke traces a straight line from x0, y0 to x1, y1, w thick, with round
// ends. Lines traced with one pen join round.
func (p pen) stroke(x0, y0, x1, y1, w float32) {
	const steps = 24
	r := float64(w) / 2
	// Round each end, from one side of the line to the other.
	along := math.Atan2(float64(y1-y0), float64(x1-x0))
	side := along + math.Pi/2
	at := func(x, y float32, a float64) (float32, float32) {
		return x + float32(r*math.Cos(a)), y + float32(r*math.Sin(a))
	}
	p.MoveTo(at(x0, y0, side))
	for i := 0; i <= steps; i++ {
		p.LineTo(at(x1, y1, side-math.Pi*float64(i)/steps))
	}
	for i := 0; i <= steps; i++ {
		p.LineTo(at(x0, y0, side-math.Pi-math.Pi*float64(i)/steps))
	}
	p.ClosePath()
}

// arc traces a band between radii r0 and r1 around x, y, from angle a0 to
// a1, in radians, clockwise on screen.
func (p pen) arc(x, y, r0, r1 float32, a0, a1 float64) {
	const steps = 96
	at := func(r float32, a float64) (float32, float32) {
		return x + r*float32(math.Cos(a)), y + r*float32(math.Sin(a))
	}
	p.MoveTo(at(r1, a0))
	for i := 1; i <= steps; i++ {
		p.LineTo(at(r1, a0+(a1-a0)*float64(i)/steps))
	}
	for i := steps; i >= 0; i-- {
		p.LineTo(at(r0, a0+(a1-a0)*float64(i)/steps))
	}
	p.ClosePath()
}

// The signs are drawn within a square of about 440 pixels in the middle of
// the image, their lines 34 pixels thick.
const (
	middleX = WideWidth / 2
	middleY = WideHeight / 2
	line    = 34
)

// liveTV draws a screen on its antennas, a play sign in it.
func liveTV(canvas *image.RGBA, ink image.Image) {
	p := newPen(canvas)
	p.frame(middleX-250, middleY-120, middleX+250, middleY+200, 44, line)
	p.fill(ink)
	p = newPen(canvas)
	p.stroke(middleX, middleY-145, middleX-90, middleY-230, line)
	p.stroke(middleX, middleY-145, middleX+90, middleY-230, line)
	p.fill(ink)
	p = newPen(canvas)
	p.play(middleX+8, middleY+40, 150)
	p.fill(ink)
}

// replay draws a circle turning back on itself, its arrow at its top
// pointing left, a play sign in it.
func replay(canvas *image.RGBA, ink image.Image) {
	const radius = 210
	y := float32(middleY + 15)
	p := newPen(canvas)
	p.arc(middleX, y, radius-line, radius, -math.Pi/2, 1.25*math.Pi)
	p.fill(ink)
	p = newPen(canvas)
	top := y - radius + line/2
	p.polygon(middleX+20, top-78, middleX+20, top+78, middleX-105, top)
	p.fill(ink)
	p = newPen(canvas)
	p.play(middleX+8, y, 150)
	p.fill(ink)
}

// collections draws a case, a play sign cut out of it, before the edges
// of two more behind it.
func collections(canvas *image.RGBA, ink image.Image) {
	for i, alpha := range []uint8{0x66, 0xb3} {
		// The farther first, the higher and further right.
		shift := float32(1-i) * 64
		p := newPen(canvas)
		p.stroke(middleX-185+shift, middleY-175-shift, middleX+215+shift, middleY-175-shift, line)
		p.stroke(middleX+215+shift, middleY-175-shift, middleX+215+shift, middleY+145-shift, line)
		p.fill(&fade{ink, alpha})
	}
	p := newPen(canvas)
	p.rounded(middleX-250, middleY-110, middleX+150, middleY+210, 36, false)
	// The play sign, traced the other way round, is a hole.
	h := float32(140)
	w := h * 0.87
	x, y := float32(middleX-50), float32(middleY+50)
	p.polygon(x-w/3, y-h/2, x-w/3, y+h/2, x+w*2/3, y)
	p.fill(ink)
}

// fade is an ink seen through alpha.
type fade struct {
	image.Image
	alpha uint8
}

func (f *fade) ColorModel() color.Model { return color.NRGBAModel }
func (f *fade) At(x, y int) color.Color {
	c := color.NRGBAModel.Convert(f.Image.At(x, y)).(color.NRGBA)
	c.A = uint8(uint16(c.A) * uint16(f.alpha) / 0xff)
	return c
}

// playlists draws the lines of a list, the last two short, a play sign
// beside them.
func playlists(canvas *image.RGBA, ink image.Image) {
	p := newPen(canvas)
	left := float32(middleX - 240)
	for i, y := range []float32{middleY - 150, middleY - 50, middleY + 50, middleY + 150} {
		end := float32(middleX + 240)
		if i >= 2 {
			end = middleX
		}
		p.stroke(left, y, end, y, line+6)
	}
	p.fill(ink)
	p = newPen(canvas)
	p.play(middleX+165, middleY+100, 220)
	p.fill(ink)
}
