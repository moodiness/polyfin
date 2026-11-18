package accounts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"testing"

	"github.com/jackc/pgx/v5"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // registers WebP with image.Decode
)

const (
	// MaxImageBytes bounds a profile picture as sent, and as kept.
	MaxImageBytes = 5 << 20
	// MaxImageSide is the longest side a profile picture is kept at: apps
	// show it small, and a larger one would only cost space and transfer.
	MaxImageSide = 512
	// MaxImageMemory bounds what normalizing a profile picture may hold
	// (see imageMemory): a photo of 16 million pixels fits, 4,624 by
	// 3,468, and a PNG of 16 bits a channel of 10 million.
	MaxImageMemory = 128 << 20
	// maxImagePixels bounds the pictures decoded whatever they hold:
	// decoding and scaling one takes time by its pixels.
	maxImagePixels = 50_000_000
)

// ErrInvalidImage is returned for a picture that is not a JPEG, PNG or
// WebP image within the sizes allowed: MaxImageBytes and MaxImageMemory
// for a profile picture.
var ErrInvalidImage = errors.New("invalid picture")

// ImageTypes are the formats a picture may be sent in.
var ImageTypes = []string{"image/jpeg", "image/png", "image/webp"}

// UserImage is a picture as kept, a user's profile picture or an item's
// uploaded artwork: a JPEG or PNG image, tagged by its content.
type UserImage struct {
	Data        []byte
	ContentType string
	Tag         string
}

// imageTurns holds the turn of the picture being normalized in the
// process, profile pictures and uploaded artwork together: one at a
// time, so that what one may hold, its maxMemory, is all that they hold.
// The others wait for the turn as long as their request lasts.
var imageTurns = make(chan struct{}, 1)

// decodeImage is image.Decode, which a test replaces to count the
// pictures decoded and to hold them.
var decodeImage = image.Decode

// WatchImageDecodesForTests calls watch before each picture this test
// binary decodes to normalize it, until the function it returns is
// called: a test of another package tells by it that a picture was
// refused from its header, and never decoded. It panics outside a test
// binary.
func WatchImageDecodesForTests(watch func()) (stop func()) {
	if !testing.Testing() {
		panic("accounts: WatchImageDecodesForTests called outside a test binary")
	}
	decode := decodeImage
	decodeImage = func(r io.Reader) (image.Image, string, error) {
		watch()
		return decode(r)
	}
	return func() { decodeImage = decode }
}

// NormalizeImage decodes a JPEG, PNG or WebP picture of at most maxBytes
// and encodes it again within maxSide: as PNG when it has transparency,
// which JPEG would lose, else as JPEG, of at most maxBytes too. Profile
// pictures and the artwork administrators upload for items are kept so.
//
// A picture whose normalizing could hold more than maxMemory bytes (see
// imageMemory) is refused from its header, before anything of it is
// decoded. One picture is normalized at a time (see imageTurns): the
// others wait while ctx lasts, and one whose ctx ends meanwhile decodes
// nothing and returns ctx's error.
func NormalizeImage(ctx context.Context, data []byte, maxBytes, maxSide int, maxMemory int64) (UserImage, error) {
	if len(data) == 0 || len(data) > maxBytes {
		return UserImage{}, ErrInvalidImage
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "jpeg" && format != "png" && format != "webp" ||
		config.Width <= 0 || config.Height <= 0 || config.Width > maxImagePixels/config.Height ||
		imageMemory(data, format, config, maxSide) > maxMemory {
		return UserImage{}, ErrInvalidImage
	}
	select {
	case imageTurns <- struct{}{}:
	case <-ctx.Done():
		return UserImage{}, ctx.Err()
	}
	defer func() { <-imageTurns }()
	// A turn that comes as the request ends is given back.
	if err := ctx.Err(); err != nil {
		return UserImage{}, err
	}
	picture, _, err := decodeImage(bytes.NewReader(data))
	if err != nil {
		return UserImage{}, ErrInvalidImage
	}
	bounds := picture.Bounds()
	if width, height := keptSize(bounds.Dx(), bounds.Dy(), maxSide); width != bounds.Dx() || height != bounds.Dy() {
		target := image.Rect(0, 0, width, height)
		scaled := image.NewRGBA(target)
		xdraw.CatmullRom.Scale(scaled, target, picture, bounds, xdraw.Src, nil)
		picture = scaled
	}
	var out bytes.Buffer
	result := UserImage{ContentType: "image/jpeg"}
	if opaque(picture) {
		err = jpeg.Encode(&out, picture, &jpeg.Options{Quality: 90})
	} else {
		result.ContentType = "image/png"
		err = png.Encode(&out, picture)
	}
	if err != nil || out.Len() > maxBytes {
		return UserImage{}, ErrInvalidImage
	}
	result.Data = out.Bytes()
	sum := sha256.Sum256(result.Data)
	result.Tag = hex.EncodeToString(sum[:16])
	return result, nil
}

// keptSize returns the size a picture of width by height is kept at: its
// own within maxSide, else the one that brings its longest side to
// maxSide.
func keptSize(width, height, maxSide int) (int, int) {
	if width <= maxSide && height <= maxSide {
		return width, height
	}
	scale := float64(maxSide) / float64(max(width, height))
	return max(1, int(float64(width)*scale+0.5)), max(1, int(float64(height)*scale+0.5))
}

// imageMemory returns the most bytes that normalizing a picture within
// maxSide holds at once, told from its header: while it is decoded, then
// while the decoded picture is scaled down or, for one kept at its size,
// encoded. The counts are read from the decoders, Go's image/jpeg and
// image/png and golang.org/x/image/webp, and from the Catmull-Rom scaler
// of golang.org/x/image/draw. Where the header leaves a doubt, the
// heavier is counted. The tables a decoder builds are not: they do not
// grow with the picture.
func imageMemory(data []byte, format string, config image.Config, maxSide int) int64 {
	width, height := int64(config.Width), int64(config.Height)
	// Decoders round the sides up to whole blocks, of 32 pixels at most.
	pixels := (width + 31) * (height + 31)
	// decoded is what the decoded picture takes, and decoding the most
	// its decoder holds until it is.
	var decoded, decoding int64
	switch format {
	case "jpeg":
		// A grey picture is a byte a pixel. One in colours is decoded as
		// a byte a component, then gathered into 4 bytes a pixel when it
		// is in RGB or CMYK, which a marker after the header may still
		// make it.
		components := int64(1)
		decoded, decoding = pixels, pixels
		if config.ColorModel != color.GrayModel {
			components = 3
			if config.ColorModel == color.CMYKModel {
				components = 4
			}
			decoded, decoding = 4*pixels, (components+4)*pixels
		}
		// A progressive picture keeps every coefficient, of 4 bytes,
		// until its last scan.
		if progressiveJPEG(data) {
			decoding += 4 * components * pixels
		}
	case "png":
		// 4 bytes a pixel, 8 at 16 bits a channel, 1 with a palette. A
		// grey picture counts as one in colours, which it is decoded as
		// when it names a transparent grey: the header does not tell.
		decoded = 4 * pixels
		if _, paletted := config.ColorModel.(color.Palette); paletted {
			decoded = pixels
		} else if m := config.ColorModel; m == color.Gray16Model || m == color.RGBA64Model || m == color.NRGBA64Model {
			decoded = 8 * pixels
		}
		decoding = decoded
		// An interlaced picture is decoded pass by pass beside the
		// whole, and its last pass is half of it. The header's last
		// byte tells one.
		if len(data) > 28 && data[28] != 0 {
			decoding += decoded / 2
		}
	default:
		// WebP. A lossy picture by itself is 12 bits a pixel. Any other
		// may be lossless, or have a lossless transparency: 4 bytes a
		// pixel, and up to 3 more while it is decoded, for the indices
		// of its palette and the pictures of its transforms.
		decoded, decoding = 4*pixels, 7*pixels
		if len(data) > 15 && data[15] == ' ' {
			decoded, decoding = 2*pixels, 2*pixels
		}
	}
	keptWidth, keptHeight := keptSize(config.Width, config.Height, maxSide)
	kept := int64(keptWidth) * int64(keptHeight)
	// Kept at its size, the picture is encoded into a buffer: 8 bytes a
	// pixel at the very most, in a buffer of up to twice what it holds.
	beside := 16 * kept
	if keptWidth != config.Width || keptHeight != config.Height {
		// Scaled down, the scaler holds beside the picture 32 bytes for
		// each pixel of the result's width by the picture's height, the
		// result at 4 bytes a pixel, and its weights: 64 bytes for each
		// column and row of the picture, 56 for each of the result. The
		// result is encoded after the picture is let go, in less.
		beside = 32*int64(keptWidth)*height + 4*kept + 64*(width+height) + 56*int64(keptWidth+keptHeight)
	}
	return max(decoding, decoded+beside)
}

// progressiveJPEG tells whether a JPEG picture is progressive, by the
// marker of its frame header. It follows the segments before it as Go's
// decoder does, and says yes wherever that one would read on through
// what is no plain segment, so that no picture is counted lighter than
// it is decoded.
func progressiveJPEG(data []byte) bool {
	for at := 2; at+4 <= len(data); at += 2 + int(data[at+2])<<8 + int(data[at+3]) {
		if data[at] != 0xff {
			return true
		}
		switch marker := data[at+1]; {
		case marker == 0xc0 || marker == 0xc1:
			// A baseline or an extended sequential frame.
			return false
		case marker != 0xc4 && marker != 0xdb && marker != 0xdd && marker != 0xfe && (marker < 0xe0 || marker > 0xef):
			// Neither tables, a restart interval, a comment nor an
			// application's data: a progressive frame, or what the
			// decoder does not skip as a segment.
			return true
		}
	}
	return true
}

// opaque reports whether every pixel of picture is opaque.
func opaque(picture image.Image) bool {
	if o, ok := picture.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	bounds := picture.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if _, _, _, a := picture.At(x, y).RGBA(); a != 0xffff {
				return false
			}
		}
	}
	return true
}

// SetImage replaces a user's profile picture with data, a JPEG, PNG or
// WebP image, normalized (see normalizeImage). It returns the user with
// their new ImageTag.
func (s *Store) SetImage(ctx context.Context, id ID, data []byte) (User, error) {
	// Apps show the picture by the user's ImageTag.
	defer s.forgetSignIns()
	normalized, err := NormalizeImage(ctx, data, MaxImageBytes, MaxImageSide, MaxImageMemory)
	if err != nil {
		return User{}, err
	}
	return scanUser(s.db.QueryRow(ctx, "UPDATE users SET image = $2, image_type = $3, image_tag = $4 WHERE id = $1 RETURNING "+userColumns,
		id, normalized.Data, normalized.ContentType, normalized.Tag))
}

// DeleteImage removes a user's profile picture, if they have one.
func (s *Store) DeleteImage(ctx context.Context, id ID) (User, error) {
	defer s.forgetSignIns()
	return scanUser(s.db.QueryRow(ctx, "UPDATE users SET image = NULL, image_type = NULL, image_tag = '' WHERE id = $1 RETURNING "+userColumns, id))
}

// Image returns a user's profile picture; ErrNotFound when the user does
// not exist or has none.
func (s *Store) Image(ctx context.Context, id ID) (UserImage, error) {
	var picture UserImage
	err := s.db.QueryRow(ctx, "SELECT image, image_type, image_tag FROM users WHERE id = $1 AND image IS NOT NULL", id).
		Scan(&picture.Data, &picture.ContentType, &picture.Tag)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserImage{}, ErrNotFound
	}
	return picture, err
}
