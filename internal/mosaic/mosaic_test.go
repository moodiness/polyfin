package mosaic

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

// solid is a poster of one dark color.
func solid(c color.RGBA) image.Image {
	poster := image.NewRGBA(image.Rect(0, 0, 200, 300))
	for i := 0; i < len(poster.Pix); i += 4 {
		poster.Pix[i], poster.Pix[i+1], poster.Pix[i+2], poster.Pix[i+3] = c.R, c.G, c.B, 255
	}
	return poster
}

// bright counts the near-white pixels of an image: posters are dark and
// shaded, so only the name's letters are.
func bright(picture *image.RGBA) int {
	count := 0
	for i := 0; i < len(picture.Pix); i += 4 {
		if picture.Pix[i] > 230 && picture.Pix[i+1] > 230 && picture.Pix[i+2] > 230 {
			count++
		}
	}
	return count
}

// A poster is 1,000 by 1,500 pixels and a wide image 1,920 by 1,080, the
// posters in their cells and the name, accents included, written across
// them; without a name, no letter shows.
func TestDrawShowsThePostersAndTheName(t *testing.T) {
	t.Parallel()
	fonts := NewFonts(t.TempDir())
	posters := []image.Image{solid(color.RGBA{R: 120}), solid(color.RGBA{G: 120}), solid(color.RGBA{B: 120}), solid(color.RGBA{R: 120, G: 120}),
		solid(color.RGBA{G: 120, B: 120}), solid(color.RGBA{R: 120, B: 120})}
	poster := Draw("Été indien", posters, false, fonts)
	if poster.Bounds() != image.Rect(0, 0, PosterWidth, PosterHeight) {
		t.Fatalf("poster: %v", poster.Bounds())
	}
	wide := Draw("Été indien", posters, true, fonts)
	if wide.Bounds() != image.Rect(0, 0, WideWidth, WideHeight) {
		t.Fatalf("wide image: %v", wide.Bounds())
	}
	if bright(poster) == 0 || bright(wide) == 0 {
		t.Error("the name is not written")
	}
	if unnamed := Draw("", posters, false, fonts); bright(unnamed) != 0 {
		t.Errorf("an image without a name shows %d bright pixels", bright(unnamed))
	}
	// Each poster fills its cell: the top-left one is red, the bottom-right
	// one the fourth poster, yellow, for a poster; the sixth, magenta, for
	// a wide image. Shaded, they keep their hue.
	hue := func(picture *image.RGBA, x, y int) (r, g, b uint8) {
		c := picture.RGBAAt(x, y)
		return c.R, c.G, c.B
	}
	if r, g, b := hue(poster, 20, 20); r == 0 || g != 0 || b != 0 {
		t.Errorf("top-left of the poster: %d %d %d", r, g, b)
	}
	if r, g, b := hue(poster, PosterWidth-20, PosterHeight-20); r == 0 || g == 0 || b != 0 {
		t.Errorf("bottom-right of the poster: %d %d %d", r, g, b)
	}
	if r, g, b := hue(wide, WideWidth-20, WideHeight-20); r == 0 || g != 0 || b == 0 {
		t.Errorf("bottom-right of the wide image: %d %d %d", r, g, b)
	}
	// Without posters, the name alone.
	if bare := Draw("Été", nil, false, fonts); bright(bare) == 0 {
		t.Error("an image without posters has no name")
	}
}

// Cells follow how many posters there are.
func TestCellsFitThePosters(t *testing.T) {
	t.Parallel()
	bounds := image.Rect(0, 0, 1000, 1500)
	for n, want := range map[int]int{0: 0, 1: 1, 2: 2, 3: 2, 4: 4, 6: 4} {
		if got := len(cells(n, false, bounds)); got != want {
			t.Errorf("a poster of %d: %d cells, want %d", n, got, want)
		}
	}
	wide := image.Rect(0, 0, 1920, 1080)
	for n, want := range map[int]int{1: 1, 2: 2, 3: 3, 5: 3, 6: 6} {
		if got := len(cells(n, true, wide)); got != want {
			t.Errorf("a wide image of %d: %d cells, want %d", n, got, want)
		}
	}
}

// A name too long for the smallest size is cut, with an ellipsis, on as
// many lines as the image takes.
func TestLongNamesAreCut(t *testing.T) {
	t.Parallel()
	fonts := NewFonts("")
	long := "The very long name of a collection that goes on and on well past what any poster could ever hold on three lines of text"
	l := fit(long, 840, 3, posterSizes, fonts)
	if len(l.lines) != 3 || l.size != posterSizes[len(posterSizes)-1] {
		t.Fatalf("layout: %+v", l)
	}
	if last := []rune(l.lines[2]); last[len(last)-1] != '…' {
		t.Errorf("last line: %q", l.lines[2])
	}
	if short := fit("Saga", 840, 3, posterSizes, fonts); len(short.lines) != 1 || short.size != posterSizes[0] {
		t.Errorf("a short name: %+v", short)
	}
}

// Marks set apart by a space, as French sets them, stay with their word.
func TestMarksStayWithTheirWord(t *testing.T) {
	t.Parallel()
	got := words("Été indien : la saga « complète » ?")
	want := []string{"Été", "indien :", "la", "saga", "« complète » ?"}
	if !slices.Equal(got, want) {
		t.Errorf("words: %q, want %q", got, want)
	}
}

// The folder's fonts come first, bold sans-serif ones before the others,
// and Go Bold writes what none of them has; a folder that cannot be read
// gives Go Bold alone.
func TestFontsComeFromTheFolderThenGoBold(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for name, data := range map[string][]byte{"Sans-Regular.ttf": goregular.TTF, "nested/Sans-Bold.ttf": gobold.TTF, "Broken.ttf": []byte("no font")} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fonts := NewFonts(dir)
	fonts.once.Do(fonts.list)
	if len(fonts.files) != 3 || filepath.Base(fonts.files[0]) != "Sans-Bold.ttf" || filepath.Base(fonts.files[1]) != "Sans-Regular.ttf" {
		t.Fatalf("order: %v", fonts.files)
	}
	if fonts.fontFor('É') == nil || fonts.fontFor('é') != fonts.fontFor('A') {
		t.Error("accented letters are not written with the first font")
	}
	if fonts.fontFor('\U0001F600') != nil {
		t.Error("a character no font has is written")
	}
	if NewFonts(filepath.Join(dir, "missing")).fontFor('Ж') != builtin() {
		t.Error("Go Bold does not write Cyrillic without a folder")
	}
}
