package ocr

import (
	"errors"
	"image"
	"time"

	"github.com/moodiness/polyfin/internal/container"
)

// Picture is a subtitle picture ready to be read: cropped to its text,
// dark text on a white background, upscaled when its lines are small.
type Picture struct {
	// Start and End are when it shows, on its track's timeline.
	Start, End time.Duration
	// Forced is set for a picture the track forces on screen.
	Forced bool
	Image  *image.Gray
}

// pixel is a subtitle pixel: its luminance and its opacity, both from 0
// to 255.
type pixel struct {
	luma, alpha uint8
}

// bitmap is a picture as its track draws it.
type bitmap struct {
	width, height int
	pixels        []pixel
}

func newBitmap(width, height int) bitmap {
	return bitmap{width: width, height: height, pixels: make([]pixel, width*height)}
}

func (b bitmap) at(x, y int) pixel { return b.pixels[y*b.width+x] }

func (b bitmap) set(x, y int, p pixel) { b.pixels[y*b.width+x] = p }

const (
	// visible is the opacity from which a pixel is part of the text.
	visible = 64
	// margin is the white border left around the text: Tesseract reads
	// text touching the edges poorly.
	margin = 12
	// lineHeight is the height under which lines of text are upscaled: a
	// DVD's lines are about 25 pixels tall, Tesseract reads best from
	// about 40.
	lineHeight = 40
	// maxScale bounds the upscaling.
	maxScale = 4
	// maxWidth and maxHeight bound the screen a track draws on, whatever
	// its headers declare, UHD video's: what lies past it is not drawn,
	// so that a track allocates no more than that, whatever it declares.
	maxWidth, maxHeight = 3840, 2160
	// maxPrepared bounds the pixels of a picture prepared: upscaling
	// stops short of it.
	maxPrepared = maxWidth * maxHeight
)

const (
	// screenPixels are the pixels of the largest screen a track draws on.
	screenPixels = maxWidth * maxHeight
	// The pixels a track composes and prepares (see pixelBudget) are
	// bounded by its size: those of minTrackScreens whole screens, and
	// pixelsPerByte for each byte of the track, up to those of
	// maxTrackScreens screens whatever its size. A 3840 by 2160 track of
	// a movie's dialogue costs about 16 pixels a byte, 125 with fades drawn
	// as palette updates, 200 with each line shown at the top and at the
	// bottom of the screen. A display set placing a tiny object at both
	// ends of the screen costs hundreds of thousands, as does a subpicture
	// leaving its lines out: a track of them ends within seconds, a few
	// minutes at its largest, not after the hour of its read.
	minTrackScreens = 128
	maxTrackScreens = 4096
	pixelsPerByte   = 1024
)

// errPixels reports a track whose pictures cover more pixels than its
// size allows.
var errPixels = errors.New("subtitle pictures past the pixels their track's size allows")

// pixelBudget is what a track may still compose and prepare, in pixels:
// the canvas of each display set or subpicture, and each picture handed
// to Tesseract. Each is taken from it before it is allocated.
type pixelBudget struct {
	left int64
}

// newPixelBudget is the budget of a track of size bytes.
func newPixelBudget(size int) *pixelBudget {
	return &pixelBudget{left: min(minTrackScreens*screenPixels+pixelsPerByte*int64(size), maxTrackScreens*screenPixels)}
}

// spend takes pixels from the budget, or reports that it holds fewer.
func (b *pixelBudget) spend(pixels int) error {
	if int64(pixels) > b.left {
		return errPixels
	}
	b.left -= int64(pixels)
	return nil
}

// blocksSize is the size of a track's blocks.
func blocksSize(blocks []container.Block) int {
	size := 0
	for _, block := range blocks {
		size += len(block.Data)
	}
	return size
}

// prepare turns a bitmap into what Tesseract reads best: cropped to its
// text, flattened into dark text on white, and upscaled when its lines are
// small. Tesseract 4 and later blend transparency with white, which makes
// white text with a dark outline unreadable: text brighter than its
// outline is drawn on black and inverted instead. The picture's pixels
// are taken from budget first. false when the bitmap shows nothing.
func prepare(b bitmap, budget *pixelBudget) (*image.Gray, bool, error) {
	left, top, right, bottom := b.width, b.height, -1, -1
	// The pixels next to the background are the outline, when there is
	// one; the others the text's fill.
	var edge, inner, all struct{ luma, count int }
	opaque := func(x, y int) bool {
		return x >= 0 && y >= 0 && x < b.width && y < b.height && b.at(x, y).alpha >= visible
	}
	for y := range b.height {
		for x := range b.width {
			if !opaque(x, y) {
				continue
			}
			left, right = min(left, x), max(right, x)
			top, bottom = min(top, y), max(bottom, y)
			luma := int(b.at(x, y).luma)
			part := &inner
			if !opaque(x-1, y) || !opaque(x+1, y) || !opaque(x, y-1) || !opaque(x, y+1) {
				part = &edge
			}
			part.luma, part.count = part.luma+luma, part.count+1
			all.luma, all.count = all.luma+luma, all.count+1
		}
	}
	if right < 0 {
		return nil, false, nil
	}
	bright := all.luma >= 100*all.count
	if inner.count > 0 && edge.count > 0 {
		if fill, border := inner.luma/inner.count, edge.luma/edge.count; fill-border > 32 || border-fill > 32 {
			bright = fill > border
		}
	}
	width, height := right-left+1, bottom-top+1
	flatPixels := (width + 2*margin) * (height + 2*margin)
	scale := 1
	if line := lineOf(b, left, top, right, bottom); line > 0 && line < lineHeight {
		scale = min((lineHeight+line-1)/line, maxScale)
	}
	for scale > 1 && flatPixels*scale*scale > maxPrepared {
		scale--
	}
	if err := budget.spend(flatPixels * scale * scale); err != nil {
		return nil, false, err
	}
	flat := image.NewGray(image.Rect(0, 0, width+2*margin, height+2*margin))
	for i := range flat.Pix {
		flat.Pix[i] = 255
	}
	for y := range height {
		for x := range width {
			p := b.at(left+x, top+y)
			var value int
			if bright {
				// On black, then inverted.
				value = 255 - int(p.luma)*int(p.alpha)/255
			} else {
				// On white.
				value = (int(p.luma)*int(p.alpha) + 255*(255-int(p.alpha))) / 255
			}
			flat.Pix[(y+margin)*flat.Stride+x+margin] = uint8(value)
		}
	}
	if scale == 1 {
		return flat, true, nil
	}
	return upscale(flat, scale), true, nil
}

// lineOf is the average height of the lines of text within a bitmap's
// bounds: the rows holding text, by run of such rows.
func lineOf(b bitmap, left, top, right, bottom int) int {
	rows, runs := 0, 0
	inRun := false
	for y := top; y <= bottom; y++ {
		inked := false
		for x := left; x <= right && !inked; x++ {
			inked = b.at(x, y).alpha >= visible
		}
		if inked {
			rows++
			if !inRun {
				runs++
			}
		}
		inRun = inked
	}
	if runs == 0 {
		return 0
	}
	return rows / runs
}

// upscale enlarges a picture by scale with bilinear interpolation.
func upscale(src *image.Gray, scale int) *image.Gray {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dst := image.NewGray(image.Rect(0, 0, w*scale, h*scale))
	for y := range h * scale {
		// Pixel centers: the destination's map back into the source's.
		sy := (float64(y)+0.5)/float64(scale) - 0.5
		y0 := max(int(sy), 0)
		y1 := min(y0+1, h-1)
		fy := max(sy-float64(y0), 0)
		for x := range w * scale {
			sx := (float64(x)+0.5)/float64(scale) - 0.5
			x0 := max(int(sx), 0)
			x1 := min(x0+1, w-1)
			fx := max(sx-float64(x0), 0)
			a := float64(src.Pix[y0*src.Stride+x0])*(1-fx) + float64(src.Pix[y0*src.Stride+x1])*fx
			b := float64(src.Pix[y1*src.Stride+x0])*(1-fx) + float64(src.Pix[y1*src.Stride+x1])*fx
			dst.Pix[y*dst.Stride+x] = uint8(a*(1-fy) + b*fy + 0.5)
		}
	}
	return dst
}
