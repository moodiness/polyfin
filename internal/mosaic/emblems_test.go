package mosaic

import (
	"bytes"
	"image"
	"testing"
)

// inked counts the pixels of the emblems' ink in the middle of an image:
// blue well above red and green, as the logo's violet to cyan.
func inked(picture *image.RGBA) int {
	count := 0
	for y := WideHeight/2 - 260; y < WideHeight/2+260; y++ {
		for x := WideWidth/2 - 280; x < WideWidth/2+280; x++ {
			c := picture.RGBAAt(x, y)
			if c.B > 200 && int(c.B) > int(c.G)+10 && c.B > c.R+60 {
				count++
			}
		}
	}
	return count
}

// Each emblem is a wide image, its sign drawn in the middle, unlike the
// others; an unknown one is the background alone.
func TestEmblemsDrawTheirSign(t *testing.T) {
	t.Parallel()
	emblems := []string{EmblemLiveTV, EmblemReplay, EmblemCollections, EmblemPlaylists}
	var drawn []*image.RGBA
	for _, name := range emblems {
		picture := DrawEmblem(name)
		if picture.Bounds() != image.Rect(0, 0, WideWidth, WideHeight) {
			t.Fatalf("%s: %v", name, picture.Bounds())
		}
		if inked(picture) < 10000 {
			t.Errorf("%s: %d pixels of ink", name, inked(picture))
		}
		for i, other := range drawn {
			if bytes.Equal(picture.Pix, other.Pix) {
				t.Errorf("%s draws as %s", name, emblems[i])
			}
		}
		drawn = append(drawn, picture)
	}
	if blank := DrawEmblem("watchlist"); inked(blank) != 0 {
		t.Errorf("an unknown emblem: %d pixels of ink", inked(blank))
	}
}
