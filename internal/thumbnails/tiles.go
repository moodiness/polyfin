package thumbnails

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"math"
	"time"
)

// Info describes a version's thumbnails at one width, as Jellyfin's
// TrickplayInfo does.
type Info struct {
	// Width and Height are a thumbnail's size, in pixels.
	Width, Height int
	// TileWidth and TileHeight are the thumbnails a tile has in a row and
	// in a column.
	TileWidth, TileHeight int
	// ThumbnailCount is how many thumbnails there are, one every
	// Interval milliseconds from the start.
	ThumbnailCount int
	Interval       int
	// Bandwidth is the most a tile takes, in bits per second of the
	// thumbnails it holds.
	Bandwidth int
}

const (
	// tileColumns and tileRows are the thumbnails a tile holds in a row
	// and in a column, as Jellyfin's tiles do.
	tileColumns = 10
	tileRows    = 10
	// jpegQuality is that of Jellyfin's tiles and of chapter images.
	jpegQuality = 90
)

// tiler packs thumbnails into tiles, row by row, as Jellyfin does: every
// tile is a full grid, the last one's free places black.
type tiler struct {
	info   Info
	canvas *image.YCbCr
	placed int
	tiles  [][]byte
}

// newTiler packs count thumbnails, one every interval.
func newTiler(count int, interval time.Duration) *tiler {
	return &tiler{info: Info{TileWidth: tileColumns, TileHeight: tileRows, ThumbnailCount: count, Interval: int(interval / time.Millisecond)}}
}

// errThumbnailSize reports a thumbnail of another size than the first.
var errThumbnailSize = errors.New("the thumbnails differ in size")

// add places the next thumbnail.
func (t *tiler) add(img *image.YCbCr) error {
	size := img.Rect.Size()
	if t.canvas == nil {
		// The first thumbnail sets their height, and their width unless
		// it was set.
		if t.info.Width == 0 {
			t.info.Width = size.X
		}
		t.info.Height = size.Y
		t.canvas = blackCanvas(t.info.Width*tileColumns, t.info.Height*tileRows)
	}
	if size.X != t.info.Width || size.Y != t.info.Height {
		return errThumbnailSize
	}
	x, y := t.placed%tileColumns*t.info.Width, t.placed/tileColumns*t.info.Height
	copyInto(t.canvas, img, x, y)
	t.placed++
	if t.placed == tileColumns*tileRows {
		return t.flush()
	}
	return nil
}

// finish encodes the last tile, and returns the tiles and what they hold.
func (t *tiler) finish() (Info, [][]byte, error) {
	if t.placed > 0 {
		if err := t.flush(); err != nil {
			return Info{}, nil, err
		}
	}
	return t.info, t.tiles, nil
}

func (t *tiler) flush() error {
	tile, err := encodeJPEG(t.canvas)
	if err != nil {
		return err
	}
	t.tiles = append(t.tiles, tile)
	// Jellyfin's peak bandwidth: the tile's bits over the seconds its
	// thumbnails cover, counting every place of the grid.
	seconds := float64(t.info.Interval) / 1000
	bits := float64(len(tile)*8) / tileColumns / tileRows / seconds
	t.info.Bandwidth = max(t.info.Bandwidth, int(math.Ceil(bits)))
	t.canvas, t.placed = nil, 0
	return nil
}

// blackCanvas is a black image of width by height, both even.
func blackCanvas(width, height int) *image.YCbCr {
	canvas := image.NewYCbCr(image.Rect(0, 0, width, height), image.YCbCrSubsampleRatio420)
	for i := range canvas.Cb {
		canvas.Cb[i], canvas.Cr[i] = 128, 128
	}
	return canvas
}

// copyInto copies img into canvas at x, y, both even, as are img's sides.
func copyInto(canvas, img *image.YCbCr, x, y int) {
	size := img.Rect.Size()
	for row := range size.Y {
		copy(canvas.Y[(y+row)*canvas.YStride+x:], img.Y[row*img.YStride:row*img.YStride+size.X])
	}
	for row := range (size.Y + 1) / 2 {
		at := (y/2+row)*canvas.CStride + x/2
		from := row * img.CStride
		copy(canvas.Cb[at:], img.Cb[from:from+(size.X+1)/2])
		copy(canvas.Cr[at:], img.Cr[from:from+(size.X+1)/2])
	}
}

func encodeJPEG(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
