package mosaic

import (
	"image"
	"image/color"
	"math"
	"sync"

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
	copy(canvas.Pix, backdrop().Pix)
	sign := emblemInk{from: logoViolet, to: logoCyan, x0: 760, y0: 760, x1: 1160, y1: 320, alpha: 0xff}
	switch name {
	case EmblemLiveTV:
		liveTV(canvas, sign)
	case EmblemReplay:
		replay(canvas, sign)
	case EmblemCollections:
		collections(canvas, sign)
	case EmblemPlaylists:
		playlists(canvas, sign)
	}
	return canvas
}

// backdrop is what every emblem is drawn over, drawn once: the mosaics'
// background, lit from its middle in the logo's accent, darker toward the
// edges, and the logo's shards, faint, in the bottom right corner.
var backdrop = sync.OnceValue(func() *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, WideWidth, WideHeight))
	cx, cy := float64(WideWidth)/2, float64(WideHeight)*0.46
	reach := float64(WideWidth) * 0.55
	pix := canvas.Pix
	i := 0
	for y := range WideHeight {
		dy := (float64(y) - cy) * 1.4
		for x := range WideWidth {
			d := math.Hypot(float64(x)-cx, dy) / reach
			light := 0.24 * math.Max(0, 1-d) * math.Max(0, 1-d)
			dark := 1 - 0.35*math.Min(1, d)
			pix[i] = uint8(float64(background.R)*dark + float64(logoAccent.R)*light + 0.5)
			pix[i+1] = uint8(float64(background.G)*dark + float64(logoAccent.G)*light + 0.5)
			pix[i+2] = uint8(float64(background.B)*dark + float64(logoAccent.B)*light + 0.5)
			pix[i+3] = 0xff
			i += 4
		}
	}
	for _, shard := range logoShards {
		points := make([]float32, len(shard.points))
		for i := 0; i < len(points); i += 2 {
			points[i], points[i+1] = 1310+shard.points[i]*6, 473+shard.points[i+1]*6
		}
		var p pen
		p.polygon(points...)
		p.fill(canvas, emblemInk{from: shard.color, to: shard.color, x1: 1, alpha: 0x38})
	}
	return canvas
})

// emblemInk is what the emblems are drawn in: from one color to another along a
// line, as the logo goes from violet to cyan, seen through alpha.
type emblemInk struct {
	from, to       color.NRGBA
	x0, y0, x1, y1 float64
	alpha          uint8
}

// faded is the ink seen through alpha as well.
func (k emblemInk) faded(alpha uint8) emblemInk {
	k.alpha = uint8(uint16(k.alpha) * uint16(alpha) / 0xff)
	return k
}

// pen traces shapes, then fills them at once: shapes traced the other way
// round within one fill cut holes. It keeps what it traced and its bounds,
// so that a fill rasterizes only the shapes' box.
type pen struct {
	path                   []step
	minX, minY, maxX, maxY float32
}

// step is one move of a pen: to a point (move), a line to it (line), or
// a cubic curve through two control points to it (cube), or the close
// of a shape.
type step struct {
	kind   byte
	points [6]float32
}

func (p *pen) add(kind byte, points ...float32) {
	if len(p.path) == 0 && len(points) > 0 {
		p.minX, p.minY, p.maxX, p.maxY = points[0], points[1], points[0], points[1]
	}
	for i := 0; i+1 < len(points); i += 2 {
		p.minX, p.maxX = min(p.minX, points[i]), max(p.maxX, points[i])
		p.minY, p.maxY = min(p.minY, points[i+1]), max(p.maxY, points[i+1])
	}
	s := step{kind: kind}
	copy(s.points[:], points)
	p.path = append(p.path, s)
}

func (p *pen) MoveTo(x, y float32)                 { p.add('M', x, y) }
func (p *pen) LineTo(x, y float32)                 { p.add('L', x, y) }
func (p *pen) CubeTo(ax, ay, bx, by, x, y float32) { p.add('C', ax, ay, bx, by, x, y) }
func (p *pen) ClosePath()                          { p.add('Z') }

// fill paints what the pen traced onto canvas in k, then forgets it.
func (p *pen) fill(canvas *image.RGBA, k emblemInk) {
	defer func() { p.path = p.path[:0] }()
	box := image.Rect(int(math.Floor(float64(p.minX)))-1, int(math.Floor(float64(p.minY)))-1,
		int(math.Ceil(float64(p.maxX)))+1, int(math.Ceil(float64(p.maxY)))+1).Intersect(canvas.Bounds())
	if len(p.path) == 0 || box.Empty() {
		return
	}
	ox, oy := float32(box.Min.X), float32(box.Min.Y)
	z := vector.NewRasterizer(box.Dx(), box.Dy())
	for _, s := range p.path {
		q := s.points
		switch s.kind {
		case 'M':
			z.MoveTo(q[0]-ox, q[1]-oy)
		case 'L':
			z.LineTo(q[0]-ox, q[1]-oy)
		case 'C':
			z.CubeTo(q[0]-ox, q[1]-oy, q[2]-ox, q[3]-oy, q[4]-ox, q[5]-oy)
		case 'Z':
			z.ClosePath()
		}
	}
	mask := image.NewAlpha(image.Rect(0, 0, box.Dx(), box.Dy()))
	z.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
	dx, dy := k.x1-k.x0, k.y1-k.y0
	length := dx*dx + dy*dy
	for y := range box.Dy() {
		row := mask.Pix[y*mask.Stride : y*mask.Stride+box.Dx()]
		i := canvas.PixOffset(box.Min.X, box.Min.Y+y)
		for x, cover := range row {
			if cover != 0 {
				t := ((float64(box.Min.X+x)-k.x0)*dx + (float64(box.Min.Y+y)-k.y0)*dy) / length
				t = math.Max(0, math.Min(1, t))
				a := float64(cover) * float64(k.alpha) / (0xff * 0xff)
				for c, ends := range [3][2]uint8{{k.from.R, k.to.R}, {k.from.G, k.to.G}, {k.from.B, k.to.B}} {
					paint := float64(ends[0]) + (float64(ends[1])-float64(ends[0]))*t
					canvas.Pix[i+c] = uint8(float64(canvas.Pix[i+c])*(1-a) + paint*a + 0.5)
				}
			}
			i += 4
		}
	}
}

// rounded traces a rectangle with rounded corners of radius r, clockwise
// on screen, or the other way round when back is set.
func (p *pen) rounded(x0, y0, x1, y1, r float32, back bool) {
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
func (p *pen) frame(x0, y0, x1, y1, r, w float32) {
	p.rounded(x0, y0, x1, y1, r, false)
	p.rounded(x0+w, y0+w, x1-w, y1-w, max(r-w, 1), true)
}

// polygon traces the polygon of points, x then y.
func (p *pen) polygon(points ...float32) {
	p.MoveTo(points[0], points[1])
	for i := 2; i < len(points); i += 2 {
		p.LineTo(points[i], points[i+1])
	}
	p.ClosePath()
}

// play traces a play sign, a triangle pointing right with its middle at
// x, y and h high.
func (p *pen) play(x, y, h float32) {
	w := h * 0.87
	p.polygon(x-w/3, y-h/2, x+w*2/3, y, x-w/3, y+h/2)
}

// stroke traces a straight line from x0, y0 to x1, y1, w thick, with round
// ends. Lines traced with one pen join round.
func (p *pen) stroke(x0, y0, x1, y1, w float32) {
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
func (p *pen) arc(x, y, r0, r1 float32, a0, a1 float64) {
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
func liveTV(canvas *image.RGBA, k emblemInk) {
	var p pen
	p.frame(middleX-250, middleY-120, middleX+250, middleY+200, 44, line)
	p.fill(canvas, k)
	p.stroke(middleX, middleY-145, middleX-90, middleY-230, line)
	p.stroke(middleX, middleY-145, middleX+90, middleY-230, line)
	p.fill(canvas, k)
	p.play(middleX+8, middleY+40, 150)
	p.fill(canvas, k)
}

// replay draws a circle turning back on itself, its arrow at its top
// pointing left, a play sign in it.
func replay(canvas *image.RGBA, k emblemInk) {
	const radius = 210
	y := float32(middleY + 15)
	var p pen
	p.arc(middleX, y, radius-line, radius, -math.Pi/2, 1.25*math.Pi)
	p.fill(canvas, k)
	top := y - radius + line/2
	p.polygon(middleX+20, top-78, middleX+20, top+78, middleX-105, top)
	p.fill(canvas, k)
	p.play(middleX+8, y, 150)
	p.fill(canvas, k)
}

// collections draws a case, a play sign cut out of it, before the edges
// of two more behind it.
func collections(canvas *image.RGBA, k emblemInk) {
	for i, alpha := range []uint8{0x66, 0xb3} {
		// The farther first, the higher and further right.
		shift := float32(1-i) * 64
		var p pen
		p.stroke(middleX-185+shift, middleY-175-shift, middleX+215+shift, middleY-175-shift, line)
		p.stroke(middleX+215+shift, middleY-175-shift, middleX+215+shift, middleY+145-shift, line)
		p.fill(canvas, k.faded(alpha))
	}
	var p pen
	p.rounded(middleX-250, middleY-110, middleX+150, middleY+210, 36, false)
	// The play sign, traced the other way round, is a hole.
	h := float32(140)
	w := h * 0.87
	x, y := float32(middleX-50), float32(middleY+50)
	p.polygon(x-w/3, y-h/2, x-w/3, y+h/2, x+w*2/3, y)
	p.fill(canvas, k)
}

// playlists draws the lines of a list, the last two short, a play sign
// beside them.
func playlists(canvas *image.RGBA, k emblemInk) {
	var p pen
	left := float32(middleX - 240)
	for i, y := range []float32{middleY - 150, middleY - 50, middleY + 50, middleY + 150} {
		end := float32(middleX + 240)
		if i >= 2 {
			end = middleX
		}
		p.stroke(left, y, end, y, line+6)
	}
	p.fill(canvas, k)
	p.play(middleX+165, middleY+100, 220)
	p.fill(canvas, k)
}
