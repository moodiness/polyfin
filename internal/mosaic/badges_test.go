package mosaic

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden images of the badges")

// samplePoster is a 2:3 poster of width, a gradient that shows whether
// the badges read over light and dark parts alike.
func samplePoster(width int) *image.RGBA {
	height := width * 3 / 2
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			canvas.SetRGBA(x, y, color.RGBA{R: uint8(40 + 200*x/width), G: uint8(60 + 150*y/height), B: 140, A: 0xff})
		}
	}
	return canvas
}

// Each size preset draws the ratings and the status where the golden
// image shows them, at a poster's full width and at a small card's.
// Run with -update to draw the golden images again after a change of the
// badges' look, and look at them before committing.
func TestBadgesMatchTheGoldenImages(t *testing.T) {
	full := Badges{Star: 7.84, IMDb: true, Critics: 91, Audience: 85, Status: NewSeason}
	cases := []struct {
		name   string
		size   BadgeSize
		width  int
		badges Badges
	}{
		{"small", BadgesSmall, 400, full},
		{"medium", BadgesMedium, 400, full},
		{"large", BadgesLarge, 400, full},
		{"small-card", BadgesSmall, 160, full},
		{"medium-card", BadgesMedium, 160, full},
		{"large-card", BadgesLarge, 160, full},
		{"addon-star-new-episode", BadgesMedium, 400, Badges{Star: 6.5, Critics: -1, Audience: -1, Status: NewEpisode}},
		{"scores-only", BadgesMedium, 400, Badges{Critics: 100, Audience: 7}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			canvas := samplePoster(c.width)
			DrawBadges(canvas, c.badges, c.size)
			path := filepath.Join("testdata", "badges-"+c.name+".png")
			if *update {
				var out bytes.Buffer
				if err := png.Encode(&out, canvas); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			file, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			golden, err := png.Decode(file)
			if err != nil {
				t.Fatal(err)
			}
			if err := sameImage(golden, canvas); err != nil {
				t.Errorf("%s: %v", path, err)
			}
		})
	}
}

// sameImage compares two images, allowing the slight differences of
// anti-aliasing that floating-point rounding brings from one processor to
// another: no channel off by more than 24, on at most 1 pixel in 500.
func sameImage(want image.Image, got *image.RGBA) error {
	if want.Bounds() != got.Bounds() {
		return fmt.Errorf("bounds %v, want %v", got.Bounds(), want.Bounds())
	}
	bounds := got.Bounds()
	off := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			wr, wg, wb, _ := want.At(x, y).RGBA()
			gr, gg, gb, _ := got.At(x, y).RGBA()
			if far(wr, gr) || far(wg, gg) || far(wb, gb) {
				off++
			}
		}
	}
	if off*500 > bounds.Dx()*bounds.Dy() {
		return fmt.Errorf("%d pixels differ", off)
	}
	return nil
}

func far(a, b uint32) bool {
	a, b = a>>8, b>>8
	return a > b+24 || b > a+24
}

// A poster without anything to tell is left as it is.
func TestNoBadgesLeaveThePosterAlone(t *testing.T) {
	canvas := samplePoster(300)
	before := bytes.Clone(canvas.Pix)
	DrawBadges(canvas, Badges{Critics: -1, Audience: -1}, BadgesLarge)
	if !bytes.Equal(before, canvas.Pix) {
		t.Error("the poster changed")
	}
}
