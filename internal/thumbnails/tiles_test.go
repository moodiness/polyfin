package thumbnails

import (
	"bytes"
	"image"
	"image/jpeg"
	"math"
	"testing"
	"time"
)

// thumbnail is a thumbnail of one gray level.
func thumbnail(width, height int, gray uint8) *image.YCbCr {
	img := blackCanvas(width, height)
	for i := range img.Y {
		img.Y[i] = gray
	}
	return img
}

// grayAt is the gray level of the middle of the thumbnail at place of a
// tile, row by row.
func grayAt(img image.Image, place, width, height int) uint8 {
	x, y := place%10*width+width/2, place/10*height+height/2
	r, _, _, _ := img.At(x, y).RGBA()
	return uint8(r >> 8)
}

// Thumbnails are packed as Jellyfin packs them: ten in a row, ten rows, in
// order; every tile the full grid, the free places of the last black; the
// peak bandwidth the bits of the largest tile over the seconds its 100
// places cover.
func TestTilePacking(t *testing.T) {
	const width, height, count = 240, 136, 105
	tiler := newTiler(count, 10*time.Second)
	for i := range count {
		if err := tiler.add(thumbnail(width, height, uint8(30+i*2))); err != nil {
			t.Fatal(err)
		}
	}
	info, tiles, err := tiler.finish()
	if err != nil {
		t.Fatal(err)
	}
	if len(tiles) != 2 {
		t.Fatalf("%d tiles for %d thumbnails", len(tiles), count)
	}
	largest := max(len(tiles[0]), len(tiles[1]))
	want := Info{Width: width, Height: height, TileWidth: 10, TileHeight: 10, ThumbnailCount: count, Interval: 10000,
		Bandwidth: int(math.Ceil(float64(largest*8) / 10 / 10 / 10))}
	if info != want {
		t.Errorf("info %+v, want %+v", info, want)
	}
	for n, tile := range tiles {
		img, err := jpeg.Decode(bytes.NewReader(tile))
		if err != nil {
			t.Fatal(err)
		}
		if size := img.Bounds().Size(); size != (image.Point{width * 10, height * 10}) {
			t.Errorf("tile %d of %v", n, size)
		}
		for place := range 100 {
			i := n*100 + place
			want := 0
			if i < count {
				want = 30 + i*2
			}
			if got := int(grayAt(img, place, width, height)); got < want-3 || got > want+3 {
				t.Errorf("tile %d, place %d: gray %d, want %d", n, place, got, want)
			}
		}
	}
	// Thumbnails all have the first one's size.
	other := newTiler(2, 10*time.Second)
	if err := other.add(thumbnail(width, height, 100)); err != nil {
		t.Fatal(err)
	}
	if err := other.add(thumbnail(width, height+2, 100)); err == nil {
		t.Error("a thumbnail of another size was packed")
	}
}
