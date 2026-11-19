package jellyfin

import (
	"bytes"
	"errors"
	"fmt"
	"hash/maphash"
	"image"
	"image/color"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Pictures decoded.
//
// Artwork comes from addons, IPTV sources, TMDB, local folders and
// shares, bounded in bytes only, and a few kilobytes may tell any
// dimensions. Decoding a picture holds 4 bytes a pixel for most, 8 for a
// PNG of 16 bits a channel, and up to 24 while a progressive JPEG is
// read; scaling it down then holds up to 24 more (32 bytes for each
// pixel of the result's width by the source's height), and the scaler's
// weights 32 bytes for each column and row of the source, whatever the
// result. So a picture is decoded, to be resized or drawn into another,
// only when decodable lets it: one of maxDecodedPixels pixels at most,
// whose decoding and drawing hold maxPictureMemory at most, both told
// from its header. One refused is answered as it is where it can be,
// and left out where it would be drawn, as one that is no picture. And
// every caller of decodePicture holds a place among the resizes (see
// imageResizes) from before the decode until it lets the decoded picture
// go, so that those places bound what is held at once: maxPictureMemory
// for each, whether it resizes a picture, draws badges, a studio's plate
// or a DLNA thumbnail, or draws the posters of a generated image, one at
// a time beside the drawing.

const (
	// maxDecodedPixels bounds the pictures decoded whatever they hold: a
	// square of 4,096 pixels a side, twice a 4K backdrop and near three
	// times a poster of 2,000 by 3,000. Decoding and scaling one takes
	// time by its pixels.
	maxDecodedPixels = 4096 * 4096
	// maxPictureMemory bounds what decoding a picture and drawing it may
	// hold (see pictureCost): 256 MiB. The heaviest ordinary artwork, a
	// 4K backdrop of 3,840 by 2,160 resized to 2,560 wide, counts 226 MB
	// as a JPEG and 258 MB as a PNG of 16 bits a channel; a poster of
	// 2,000 by 3,000 resized to 1,280 wide, 158 MB. A larger picture is
	// still resized to the smaller widths: one of 4,000 by 4,000 up to
	// 1,280 wide.
	maxPictureMemory = 256 << 20
	// maxJPEGSegments bounds the segments read before a JPEG picture's
	// frame header to tell whether it is progressive.
	maxJPEGSegments = 1024
	// maxUndecodable bounds the pictures remembered as failing to decode.
	maxUndecodable = 1024
)

// errUndecodable tells a picture whose decoding failed before, which is
// not decoded again.
var errUndecodable = errors.New("the picture could not be decoded before")

// pictureCost is what a picture's header tells of the memory its
// decoding takes: its sides, the most its decoder holds until it is
// decoded, and what the decoded picture takes, in bytes. The counts are
// read from the decoders, Go's image/jpeg and image/png and
// golang.org/x/image/webp. Where the header leaves a doubt, the heavier
// is counted. The tables a decoder builds are not, as they do not grow
// with the picture, but for a WebP picture that may be lossless: its
// prefix codes may hold more than the picture (see
// accounts.WebPPrefixCodes).
type pictureCost struct {
	width, height     int64
	decoding, decoded int64
}

// costOf counts a picture's cost from its header, which DecodeConfig
// read.
func costOf(data []byte, format string, config image.Config) pictureCost {
	cost := pictureCost{width: int64(config.Width), height: int64(config.Height)}
	pixels := cost.width * cost.height
	switch format {
	case "jpeg":
		// The decoder rounds the sides up to whole units, of 32 pixels at
		// most. A grey picture is a byte a pixel. One in colours is
		// decoded as a byte a component, 4 with black, then gathered into
		// 4 bytes a pixel when it is in RGB or CMYK, which a marker after
		// the frame header may still make it.
		pixels = (cost.width + 31) * (cost.height + 31)
		components := int64(1)
		cost.decoding, cost.decoded = pixels, pixels
		if config.ColorModel != color.GrayModel {
			components = 3
			if config.ColorModel == color.CMYKModel {
				components = 4
			}
			cost.decoding, cost.decoded = (components+4)*pixels, 4*pixels
		}
		// A progressive picture keeps every coefficient, of 4 bytes,
		// until its last scan: one for each sample.
		samples, of := progressiveJPEG(data, components)
		cost.decoding += 4 * pixels * samples / of
	case "png":
		// 4 bytes a pixel, 8 at 16 bits a channel, 1 with a palette. A
		// grey picture counts as one in colours, which it is decoded as
		// when it names a transparent grey: the header does not tell.
		each := int64(4)
		if _, paletted := config.ColorModel.(color.Palette); paletted {
			each = 1
		} else if m := config.ColorModel; m == color.Gray16Model || m == color.RGBA64Model || m == color.NRGBA64Model {
			each = 8
		}
		cost.decoded = each * pixels
		// The decoder holds two rows as they are read beside the picture.
		// An interlaced picture, which byte 28 tells, is decoded pass by
		// pass beside the whole, and its last pass is half of it.
		cost.decoding = cost.decoded + 2*(1+each*cost.width)
		if len(data) > 28 && data[28] != 0 {
			cost.decoding += cost.decoded / 2
		}
	default:
		// WebP, whose decoder rounds a lossy picture's sides up to 16. A
		// lossy picture by itself is 12 bits a pixel. Any other may be
		// lossless, or have a lossless transparency: 4 bytes a pixel,
		// and up to 3 more while it is decoded, for the indices of its
		// palette and the pictures of its transforms, and beside them the
		// groups of prefix codes it is decoded with.
		pixels = (cost.width + 15) * (cost.height + 15)
		cost.decoding, cost.decoded = 7*pixels+accounts.WebPPrefixCodes(cost.width, cost.height), 4*pixels
		if len(data) >= 16 && string(data[12:16]) == "VP8 " {
			cost.decoding, cost.decoded = 2*pixels, 2*pixels
		}
	}
	return cost
}

// progressiveJPEG tells the coefficients a JPEG picture of so many
// components keeps while it is decoded: samples of them for each of
// pixels, by its frame header. A sequential frame, of marker 0xC0 or
// 0xC1, keeps none. A progressive one, 0xC2, keeps one for each sample,
// and its header tells how many samples of each component a unit of the
// picture has: 6 for each 4 pixels of most photos, whose colours are
// sampled at half the width and height.
//
// It reads the four bytes that head each segment before the frame
// header and steps over the segment, as Go's decoder does for tables, a
// restart interval, a comment and an application's data. It decodes
// nothing, and reads maxJPEGSegments segments at most. Wherever the
// decoder would not step over a plain segment, past that count, and for
// a frame header it cannot read, the answer is a coefficient for each
// component of each pixel, so that no picture is counted lighter than it
// is decoded.
func progressiveJPEG(data []byte, components int64) (samples, of int64) {
	at := 2
	for range maxJPEGSegments {
		if at+4 > len(data) || data[at] != 0xff {
			break
		}
		marker, length := data[at+1], int(data[at+2])<<8|int(data[at+3])
		plain := marker == 0xc4 || marker == 0xdb || marker == 0xdd || marker == 0xfe || marker >= 0xe0 && marker <= 0xef
		switch {
		case marker == 0xc0 || marker == 0xc1:
			return 0, 1
		case marker == 0xc2:
			// The frame header: 6 bytes, then for each component its
			// name, its samples across and down a unit, and its table.
			frame := data[at+4:]
			if len(frame) < 6+3*int(components) || int64(frame[5]) != components {
				return components, 1
			}
			var across, down int64
			for i := range int(components) {
				h, v := int64(frame[7+3*i]>>4), int64(frame[7+3*i]&0x0f)
				samples += h * v
				across, down = max(across, h), max(down, v)
			}
			if across*down == 0 {
				return components, 1
			}
			return samples, across * down
		case !plain || length < 2:
			return components, 1
		}
		at += 2 + length
	}
	return components, 1
}

// scaling returns what a kernel of golang.org/x/image/draw, of the given
// support (1 for BiLinear, 2 for CatmullRom), holds while it scales sw
// by sh pixels to dw by dh: 32 bytes for each pixel of the result's
// width by the source's height, and its weights, for the columns then
// for the rows: 24 bytes for each of the result, and 16 for each source
// one that each of the result reads, twice the support for each source
// one when scaling down and 2 more for each of the result at most. So a
// strip of millions of pixels by one costs its weights whatever it is
// scaled to.
func scaling(support, sw, sh, dw, dh int64) int64 {
	weights := func(source, result int64) int64 {
		return 56*result + 32*support*max(source, result)
	}
	return 32*dw*sh + weights(sw, dw) + weights(sh, dh)
}

// resized returns the most held while the picture is decoded, then
// scaled down to width as resize does: the decoded picture, the scaler
// and the result, of 4 bytes a pixel. The result is encoded once the
// scaler is done, in less than it held.
func (p pictureCost) resized(width int) int64 {
	dw := int64(width)
	dh := max(1, (p.height*dw+p.width/2)/p.width)
	return max(p.decoding, p.decoded+scaling(1, p.width, p.height, dw, dh)+4*dw*dh)
}

// redrawn returns the most held while the picture is decoded, then drawn
// at its size on a canvas of 4 bytes a pixel, then the canvas encoded.
func (p pictureCost) redrawn() int64 {
	return max(p.decoding, p.decoded+4*p.width*p.height+encoding(p.width, p.height))
}

// encoded returns the most held while the picture is decoded, then
// encoded at its size.
func (p pictureCost) encoded() int64 {
	return max(p.decoding, p.decoded+encoding(p.width, p.height))
}

// encoding returns what encoding a picture of width by height may hold:
// its encoded form, 5 bytes a pixel at the very most, in a buffer that
// holds up to three times that while it grows.
func encoding(width, height int64) int64 {
	return 16 * width * height
}

// fitting returns what the Catmull-Rom scaler holds at most while the
// picture, or a part of it, is scaled to fit or to fill a part of a
// drawing of width by height pixels, keeping its proportions: the result
// by the part's height is no more than the drawing's width by the
// picture's height, nor than the picture's width by the drawing's
// height, but for the rounding of a side.
func (p pictureCost) fitting(width, height int64) int64 {
	return 32*(min(width*p.height, p.width*height)+width+p.height) +
		56*(width+height) + 64*(max(p.width, width)+max(p.height, height))
}

// drawnInto returns the most held while the picture is decoded, then
// scaled into a drawing of width by height pixels, of 4 bytes each, then
// the drawing encoded.
func (p pictureCost) drawnInto(width, height int64) int64 {
	return max(p.decoding, p.decoded+4*width*height+max(p.fitting(width, height), encoding(width, height)))
}

// decodable tells whether a picture may be decoded, from its header
// alone: a JPEG, PNG or WebP picture, the formats resized, of at most
// maxDecodedPixels pixels, when held, which tells the most its caller
// holds while it decodes and draws a picture of that cost, is
// maxPictureMemory at most. Without held, the decoding alone is counted.
func decodable(data []byte, held func(pictureCost) int64) error {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	switch {
	case err != nil:
		return err
	case format != "jpeg" && format != "png" && format != "webp":
		return fmt.Errorf("a %s picture is not decoded", format)
	case config.Width <= 0 || config.Height <= 0:
		return fmt.Errorf("a picture of %d by %d pixels is not decoded", config.Width, config.Height)
	case config.Width > maxDecodedPixels/config.Height:
		return fmt.Errorf("a picture of %d by %d pixels is too large to decode", config.Width, config.Height)
	}
	cost := costOf(data, format, config)
	memory := max(cost.decoding, cost.decoded)
	if held != nil {
		memory = held(cost)
	}
	if memory > maxPictureMemory {
		return fmt.Errorf("a picture of %d by %d pixels would hold %d MiB to decode and draw, too much", config.Width, config.Height, memory>>20)
	}
	return nil
}

// pictureSum tells a picture's bytes from others', within this process.
type pictureSum struct {
	sum  uint64
	size int
}

var pictureSeed = maphash.MakeSeed()

// decodePicture decodes a picture decodable allows for what its caller
// holds (see decodable). One whose decoding fails is remembered by its
// bytes, as long as a failed download is and among maxUndecodable at
// most, and not decoded again meanwhile: a picture cut short costs its
// whole size in memory before it fails.
func (h *Handler) decodePicture(data []byte, held func(pictureCost) int64) (image.Image, string, error) {
	if err := decodable(data, held); err != nil {
		return nil, "", err
	}
	h.images.open(h.imageDir(), h.Logger)
	key := pictureSum{sum: maphash.Bytes(pictureSeed, data), size: len(data)}
	if _, failed := h.images.undecodable.Get(key); failed {
		return nil, "", errUndecodable
	}
	h.images.decoded.Add(1)
	if hook := h.images.decoding.Load(); hook != nil {
		(*hook)(data)
	}
	decoded, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		h.images.undecodable.Put(key, struct{}{})
		return nil, "", err
	}
	return decoded, format, nil
}
