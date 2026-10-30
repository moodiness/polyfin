package mosaic

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// Badges are what a movie's or a series' poster shows over it: its
// ratings, top left, and whether a new episode or season came out, top
// right. They hold icons and numbers only, written in Go Bold, so that
// they read the same in every language.
type Badges struct {
	// Star is the star rating, from 0 to 10, 0 for none; IMDb tells it is
	// IMDb's, which the badge then names instead of showing a star.
	Star float64
	IMDb bool
	// Critics and Audience are the critics' and audience scores, from 0
	// to 100, -1 for none.
	Critics  int
	Audience int
	// Status tells a new episode or a new season.
	Status Status
}

// Status is the release a series' poster tells.
type Status int

const (
	// NoStatus tells nothing.
	NoStatus Status = iota
	// NewEpisode tells an episode came out lately.
	NewEpisode
	// NewSeason tells a season started lately.
	NewSeason
)

// Empty reports whether the badges show nothing.
func (b Badges) Empty() bool {
	return b.Star <= 0 && b.Critics < 0 && b.Audience < 0 && b.Status == NoStatus
}

// BadgeSize is a size preset of the badges: their height for a share of
// the poster's width, and the least height they keep on a small one, so
// that small cards still read them.
type BadgeSize int

const (
	BadgesSmall BadgeSize = iota
	BadgesMedium
	BadgesLarge
)

// share and least are a badge's height, as a share of the poster's width
// and in pixels at least.
func (s BadgeSize) share() (share, least float64) {
	switch s {
	case BadgesSmall:
		return 0.075, 13
	case BadgesLarge:
		return 0.125, 20
	}
	return 0.095, 16
}

// The colours of the badges: a dark glass behind white numbers, the
// star in gold, the critics' bubble in red, the audience in blue, and the
// status in green.
var (
	glass        = color.NRGBA{A: 0xc0}
	gold         = color.NRGBA{R: 0xf5, G: 0xc5, B: 0x18, A: 0xff}
	criticsRed   = color.NRGBA{R: 0xf2, G: 0x4b, B: 0x2e, A: 0xff}
	audienceBlue = color.NRGBA{R: 0x4f, G: 0xb3, B: 0xf6, A: 0xff}
	freshGreen   = color.NRGBA{R: 0x1f, G: 0x9d, B: 0x55, A: 0xf0}
	ink          = color.NRGBA{R: 0x11, G: 0x11, B: 0x11, A: 0xff}
)

// DrawBadges draws badges over a poster, sized for its width by size.
// Ratings stack down the left edge, as many as fit in the upper half; the
// status sits in the top right corner, or below the ratings when the
// poster is too narrow for both side by side.
func DrawBadges(canvas *image.RGBA, b Badges, size BadgeSize) {
	bounds := canvas.Bounds()
	if b.Empty() || bounds.Dx() < 32 || bounds.Dy() < 32 {
		return
	}
	share, least := size.share()
	h := math.Round(math.Max(least, float64(bounds.Dx())*share))
	margin := math.Round(h * 0.35)
	gap := math.Round(h * 0.25)
	w := newBadgeWriter(h)
	type pill struct {
		draw  func(x, y float64)
		width float64
	}
	var ratings []pill
	if b.Star > 0 {
		text := strconv.FormatFloat(math.Round(b.Star*10)/10, 'f', 1, 64)
		icon := h * 0.62
		if b.IMDb {
			icon = w.labelWidth("IMDb")
		}
		width := w.pillWidth(icon, text)
		ratings = append(ratings, pill{width: width, draw: func(x, y float64) {
			w.pill(canvas, x, y, width)
			if b.IMDb {
				w.label(canvas, x+w.pad, y, "IMDb")
			} else {
				star(canvas, x+w.pad+icon/2, y+h/2, icon/2, gold)
			}
			w.text(canvas, x+w.pad+icon+w.space, y, text)
		}})
	}
	for _, score := range []struct {
		value int
		icon  func(*image.RGBA, float64, float64, float64)
	}{{b.Critics, bubble}, {b.Audience, people}} {
		if score.value < 0 {
			continue
		}
		text := strconv.Itoa(min(score.value, 100)) + "%"
		icon := h * 0.62
		width := w.pillWidth(icon, text)
		drawIcon := score.icon
		ratings = append(ratings, pill{width: width, draw: func(x, y float64) {
			w.pill(canvas, x, y, width)
			drawIcon(canvas, x+w.pad, y+(h-icon)/2, icon)
			w.text(canvas, x+w.pad+icon+w.space, y, text)
		}})
	}
	left, top := float64(bounds.Min.X)+margin, float64(bounds.Min.Y)+margin
	room := float64(bounds.Dy()) / 2
	shown := 0
	widest := 0.0
	for _, p := range ratings {
		if float64(shown)*(h+gap)+h > room {
			break
		}
		p.draw(left, top+float64(shown)*(h+gap))
		widest = math.Max(widest, p.width)
		shown++
	}
	if b.Status == NoStatus {
		return
	}
	width := h * 1.5
	x, y := float64(bounds.Max.X)-margin-width, top
	if shown > 0 && left+widest+gap > x {
		// Too narrow for both side by side: the status goes under the
		// ratings.
		x, y = left, top+float64(shown)*(h+gap)
	}
	roundRect(canvas, x, y, width, h, h/2, freshGreen)
	icon := h * 0.56
	ix, iy := x+(width-icon)/2, y+(h-icon)/2
	if b.Status == NewSeason {
		layers(canvas, ix, iy, icon)
	} else {
		play(canvas, ix, iy, icon)
	}
}

// badgeWriter writes the numbers of the badges of height h in Go Bold.
type badgeWriter struct {
	h, pad, space float64
	face          font.Face
	labelFace     font.Face
}

func newBadgeWriter(h float64) *badgeWriter {
	face, _ := opentype.NewFace(builtin(), &opentype.FaceOptions{Size: h * 0.6, DPI: 72, Hinting: font.HintingNone})
	label, _ := opentype.NewFace(builtin(), &opentype.FaceOptions{Size: h * 0.42, DPI: 72, Hinting: font.HintingNone})
	return &badgeWriter{h: h, pad: math.Round(h * 0.3), space: math.Round(h * 0.2), face: face, labelFace: label}
}

// advance measures text in face, in pixels.
func advance(face font.Face, text string) float64 {
	return float64(font.MeasureString(face, text)) / 64
}

// pillWidth is the width of a badge holding an icon as wide as icon, then
// text.
func (w *badgeWriter) pillWidth(icon float64, text string) float64 {
	return math.Ceil(w.pad + icon + w.space + advance(w.face, text) + w.pad)
}

// pill draws the glass of a badge.
func (w *badgeWriter) pill(canvas *image.RGBA, x, y, width float64) {
	roundRect(canvas, x, y, width, w.h, w.h/2, glass)
}

// text writes text from x, centred in a badge whose top is y.
func (w *badgeWriter) text(canvas *image.RGBA, x, y float64, text string) {
	capHeight := w.h * 0.6 * 0.72
	writeIn(canvas, w.face, text, x, y+(w.h+capHeight)/2, image.White)
}

// labelWidth is the width of the gold label naming a source.
func (w *badgeWriter) labelWidth(text string) float64 {
	return math.Ceil(advance(w.labelFace, text) + w.h*0.3)
}

// label draws a gold label naming a source, from x, in a badge whose top
// is y.
func (w *badgeWriter) label(canvas *image.RGBA, x, y float64, text string) {
	width := w.labelWidth(text)
	height := w.h * 0.62
	roundRect(canvas, x, y+(w.h-height)/2, width, height, height*0.22, gold)
	capHeight := w.h * 0.42 * 0.72
	writeIn(canvas, w.labelFace, text, x+w.h*0.15, y+(w.h+capHeight)/2, image.NewUniform(ink))
}

// writeIn draws text in face from x on baseline.
func writeIn(canvas *image.RGBA, face font.Face, text string, x, baseline float64, src image.Image) {
	dot := fixed.Point26_6{X: fixed.Int26_6(x * 64), Y: fixed.Int26_6(baseline * 64)}
	for _, r := range text {
		dr, mask, maskp, step, ok := face.Glyph(dot, r)
		if ok {
			draw.DrawMask(canvas, dr, src, image.Point{}, mask, maskp, draw.Over)
		}
		dot.X += step
	}
}

// shape fills the path drawn by trace, which stays within x0, y0, x1,
// y1, in colour c. trace gets the offset to add to every point.
func shape(canvas *image.RGBA, x0, y0, x1, y1 float64, c color.Color, trace func(z *vector.Rasterizer, dx, dy float32)) {
	r := image.Rect(int(math.Floor(x0)), int(math.Floor(y0)), int(math.Ceil(x1)), int(math.Ceil(y1))).Intersect(canvas.Bounds())
	if r.Empty() {
		return
	}
	z := vector.NewRasterizer(r.Dx(), r.Dy())
	z.DrawOp = draw.Over
	trace(z, float32(-r.Min.X), float32(-r.Min.Y))
	z.Draw(canvas, r, image.NewUniform(c), image.Point{})
}

// kappa places the control points of a cubic curve drawing a quarter of
// a circle.
const kappa = 0.5523

// roundRect fills a rectangle whose corners are rounded by radius.
func roundRect(canvas *image.RGBA, x, y, width, height, radius float64, c color.Color) {
	radius = math.Min(radius, math.Min(width, height)/2)
	shape(canvas, x, y, x+width, y+height, c, func(z *vector.Rasterizer, dx, dy float32) {
		x0, y0 := float32(x)+dx, float32(y)+dy
		x1, y1 := x0+float32(width), y0+float32(height)
		r := float32(radius)
		k := r * kappa
		z.MoveTo(x0+r, y0)
		z.LineTo(x1-r, y0)
		z.CubeTo(x1-r+k, y0, x1, y0+r-k, x1, y0+r)
		z.LineTo(x1, y1-r)
		z.CubeTo(x1, y1-r+k, x1-r+k, y1, x1-r, y1)
		z.LineTo(x0+r, y1)
		z.CubeTo(x0+r-k, y1, x0, y1-r+k, x0, y1-r)
		z.LineTo(x0, y0+r)
		z.CubeTo(x0, y0+r-k, x0+r-k, y0, x0+r, y0)
		z.ClosePath()
	})
}

// circle fills a circle.
func circle(z *vector.Rasterizer, cx, cy, r float32) {
	k := r * kappa
	z.MoveTo(cx+r, cy)
	z.CubeTo(cx+r, cy+k, cx+k, cy+r, cx, cy+r)
	z.CubeTo(cx-k, cy+r, cx-r, cy+k, cx-r, cy)
	z.CubeTo(cx-r, cy-k, cx-k, cy-r, cx, cy-r)
	z.CubeTo(cx+k, cy-r, cx+r, cy-k, cx+r, cy)
	z.ClosePath()
}

// star fills a five-pointed star of radius r around cx, cy.
func star(canvas *image.RGBA, cx, cy, r float64, c color.Color) {
	shape(canvas, cx-r, cy-r, cx+r, cy+r, c, func(z *vector.Rasterizer, dx, dy float32) {
		for i := range 10 {
			radius := r
			if i%2 == 1 {
				radius = r * 0.45
			}
			angle := -math.Pi/2 + float64(i)*math.Pi/5
			x, y := float32(cx+radius*math.Cos(angle))+dx, float32(cy+radius*math.Sin(angle)+r*0.08)+dy
			if i == 0 {
				z.MoveTo(x, y)
			} else {
				z.LineTo(x, y)
			}
		}
		z.ClosePath()
	})
}

// bubble draws the critics' icon in a square of side s: a speech bubble
// with the lines of a review.
func bubble(canvas *image.RGBA, x, y, s float64) {
	roundRect(canvas, x, y+s*0.06, s, s*0.72, s*0.16, criticsRed)
	shape(canvas, x, y, x+s, y+s, criticsRed, func(z *vector.Rasterizer, dx, dy float32) {
		z.MoveTo(float32(x+s*0.18)+dx, float32(y+s*0.7)+dy)
		z.LineTo(float32(x+s*0.46)+dx, float32(y+s*0.7)+dy)
		z.LineTo(float32(x+s*0.16)+dx, float32(y+s*0.98)+dy)
		z.ClosePath()
	})
	for i, length := range []float64{0.64, 0.44} {
		roundRect(canvas, x+s*0.18, y+s*(0.24+0.2*float64(i)), s*length, s*0.1, s*0.05, color.White)
	}
}

// people draws the audience's icon in a square of side s: a group of
// three.
func people(canvas *image.RGBA, x, y, s float64) {
	figure := func(cx, top, scale float64, c color.Color) {
		shape(canvas, x, y, x+s, y+s, c, func(z *vector.Rasterizer, dx, dy float32) {
			head := float32(s * 0.14 * scale)
			circle(z, float32(cx)+dx, float32(top)+head+dy, head)
		})
		roundRect(canvas, cx-s*0.2*scale, top+s*0.32*scale, s*0.4*scale, s*0.42*scale, s*0.18*scale, c)
	}
	side := color.NRGBA{R: audienceBlue.R, G: audienceBlue.G, B: audienceBlue.B, A: 0xa8}
	figure(x+s*0.2, y+s*0.2, 0.8, side)
	figure(x+s*0.8, y+s*0.2, 0.8, side)
	figure(x+s*0.5, y+s*0.12, 1, audienceBlue)
}

// play draws the new episode's icon in a square of side s: a play mark.
func play(canvas *image.RGBA, x, y, s float64) {
	shape(canvas, x, y, x+s, y+s, color.White, func(z *vector.Rasterizer, dx, dy float32) {
		z.MoveTo(float32(x+s*0.2)+dx, float32(y)+dy)
		z.LineTo(float32(x+s*0.95)+dx, float32(y+s/2)+dy)
		z.LineTo(float32(x+s*0.2)+dx, float32(y+s)+dy)
		z.ClosePath()
	})
}

// layers draws the new season's icon in a square of side s: a stack of
// episodes.
func layers(canvas *image.RGBA, x, y, s float64) {
	back := color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x90}
	roundRect(canvas, x+s*0.3, y, s*0.7, s*0.6, s*0.08, back)
	roundRect(canvas, x+s*0.15, y+s*0.2, s*0.7, s*0.6, s*0.08, back)
	roundRect(canvas, x, y+s*0.4, s*0.7, s*0.6, s*0.08, color.White)
}
