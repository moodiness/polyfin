package mosaic

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
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

// measuring returns Go Bold alone, counting the characters measured with
// it and failing the test past most of them: what a name costs is counted,
// not timed.
func measuring(t *testing.T, most int) (*Fonts, *int) {
	t.Helper()
	fonts, count := NewFonts(""), new(int)
	fonts.measured = func() {
		if *count++; *count > most {
			t.Fatalf("more than %d characters measured", most)
		}
	}
	return fonts, count
}

// Wrapping a word without a space measures each of its characters twice,
// once to see it is wider than a line and once to break it, and Draw does
// so at each size it tries: twice the characters cost twice the measures.
func TestWrappingANameCostsNoMoreThanItsLength(t *testing.T) {
	t.Parallel()
	var wrapped, drawn []int
	for _, n := range []int{200, 400} {
		name := strings.Repeat("W", n)
		fonts, count := measuring(t, 2*n+1)
		w := &writer{fonts: fonts, size: wideSizes[0], faces: map[*sfnt.Font]font.Face{}}
		if lines := w.wrap(name, WideWidth*0.84); len(lines) < 2 || strings.Join(lines, "") != name {
			t.Fatalf("a word of %d characters: %d lines, %q", n, len(lines), lines)
		}
		if *count != 2*n+1 {
			t.Errorf("wrapping a word of %d characters measured %d, want %d", n, *count, 2*n+1)
		}
		wrapped = append(wrapped, *count)
		// The sizes, each wrapping the word, then the last line's cut and
		// the lines drawn.
		fonts, count = measuring(t, (2*len(wideSizes)+1)*n)
		Draw(name, nil, true, fonts)
		if *count < len(wideSizes)*n {
			t.Errorf("drawing a word of %d characters measured %d only", n, *count)
		}
		drawn = append(drawn, *count)
	}
	if wrapped[1] > 2*wrapped[0] || drawn[1] > 2*drawn[0] {
		t.Errorf("twice the characters: %v measured by wrap, %v by Draw", wrapped, drawn)
	}
}

// Draw cuts a name to its first characters before it measures any: a word
// of 20,000 characters costs what one of 2,000 does, at most two measures
// of what is kept at each size, then the last line's cut and the lines
// drawn.
func TestDrawMeasuresNoMoreOfALongerName(t *testing.T) {
	t.Parallel()
	for _, wide := range []bool{false, true} {
		steps := posterSizes
		if wide {
			steps = wideSizes
		}
		most := (2*len(steps) + 4) * (longestName + 2)
		var counts []int
		for _, n := range []int{2_000, 20_000} {
			fonts, count := measuring(t, most)
			if bright(Draw(strings.Repeat("W", n), nil, wide, fonts)) == 0 {
				t.Errorf("a word of %d characters is not written", n)
			}
			counts = append(counts, *count)
		}
		if counts[0] == 0 || counts[0] != counts[1] {
			t.Errorf("wide %v: %d characters measured for 2,000, %d for 20,000", wide, counts[0], counts[1])
		}
		t.Logf("wide %v: %d characters measured for a word of any length, %d allowed", wide, counts[0], most)
	}
}

// The last line is cut by measuring it once, though hundreds of its
// characters take no room: the third line of a poster holds fourteen
// letters and the 400 characters Go Bold lacks, and loses them all and a
// letter to the ellipsis.
func TestTheLastLinesCutMeasuresItOnce(t *testing.T) {
	t.Parallel()
	name := strings.Repeat("W", 42) + strings.Repeat("東", 400) + "W"
	fonts, _ := measuring(t, (2*len(posterSizes)+1)*443)
	l := fit(name, PosterWidth*0.84, 3, posterSizes, fonts)
	if want := []string{strings.Repeat("W", 14), strings.Repeat("W", 14), strings.Repeat("W", 13) + "…"}; !slices.Equal(l.lines, want) {
		t.Errorf("lines: %q, want %q", l.lines, want)
	}
}

// A name longer than the longest written is drawn as its first characters
// followed by an ellipsis, and one just as long whole. The characters Go
// Bold lacks take no room, so that the letters after them show.
func TestANameTooLongIsDrawnCutWithAnEllipsis(t *testing.T) {
	t.Parallel()
	fonts := NewFonts("")
	head := strings.Repeat("東", longestName-2)
	if long, want := Draw(head+"Abcd", nil, true, fonts), Draw("Ab…", nil, true, fonts); !bytes.Equal(long.Pix, want.Pix) {
		t.Error("a name of two characters too many is not drawn as its first ones and an ellipsis")
	}
	if whole, want := Draw(head+"Ab", nil, true, fonts), Draw("Ab", nil, true, fonts); !bytes.Equal(whole.Pix, want.Pix) {
		t.Error("a name as long as the longest is not drawn whole")
	}
}

// A character wider than the line takes a line of its own, at any size,
// and the words after it follow on theirs.
func TestACharacterWiderThanTheLineTakesALineOfItsOwn(t *testing.T) {
	t.Parallel()
	fonts, _ := measuring(t, 100)
	w := &writer{fonts: fonts, size: posterSizes[0], faces: map[*sfnt.Font]font.Face{}}
	if got, want := w.wrap("Wide load", 20), []string{"W", "i", "d", "e", "l", "o", "a", "d"}; !slices.Equal(got, want) {
		t.Errorf("lines: %q, want %q", got, want)
	}
	if l := fit("W", 20, 3, posterSizes, fonts); !slices.Equal(l.lines, []string{"W"}) || l.size != posterSizes[0] {
		t.Errorf("a letter wider than the image: %+v", l)
	}
}

// Names wrap as they always did: at spaces, a word wider than a line
// broken where it reaches it, marks with their word, the characters no
// font has taking no room, and the last line cut with an ellipsis when the
// smallest size does not hold the name.
func TestNamesWrapOnTheSameLines(t *testing.T) {
	t.Parallel()
	repeat := strings.Repeat
	fonts := NewFonts("")
	for _, test := range []struct {
		name string
		// The size and the lines of a poster, then of a wide image.
		posterSize float64
		poster     []string
		wideSize   float64
		wide       []string
		// The lines at a poster's largest size.
		largest []string
	}{
		{"Saga",
			132, []string{"Saga"},
			150, []string{"Saga"},
			[]string{"Saga"}},
		{"Été indien",
			132, []string{"Été indien"},
			150, []string{"Été indien"},
			[]string{"Été indien"}},
		{"Le Seigneur des anneaux : la trilogie « complète » ?",
			87, []string{"Le Seigneur des", "anneaux : la trilogie", "« complète » ?"},
			127, []string{"Le Seigneur des anneaux :", "la trilogie « complète » ?"},
			[]string{"Le Seigneur", "des", "anneaux : la", "trilogie", "« complète » ", "?"}},
		{"The very long name of a collection that goes on and on well past what any poster could ever hold on three lines of text",
			60, []string{"The very long name of a", "collection that goes on and", "on well past what any post…"},
			68, []string{"The very long name of a collection that goes on", "and on well past what any poster could ever ho…"},
			[]string{"The very", "long name of", "a collection", "that goes on", "and on well", "past what", "any poster", "could ever", "hold on three", "lines of text"}},
		{"Marvel Cinematic Universe: The Infinity Saga Collection",
			80, []string{"Marvel Cinematic", "Universe: The Infinity", "Saga Collection"},
			117, []string{"Marvel Cinematic Universe:", "The Infinity Saga Collection"},
			[]string{"Marvel", "Cinematic", "Universe:", "The Infinity", "Saga", "Collection"}},
		{"The Supercalifragilisticexpialidociousness of it all",
			74, []string{"The", "Supercalifragilisticexpi", "alidociousness of it all"},
			77, []string{"The Supercalifragilisticexpialidociousness", "of it all"},
			[]string{"The", "Supercalifra", "gilisticexpial", "idociousnes", "s of it all"}},
		{"Rindfleischetikettierungsüberwachungsaufgabenübertragungsgesetz und mehr",
			62, []string{"Rindfleischetikettierungsüb", "erwachungsaufgabenübertr", "agungsgesetz und mehr"},
			84, []string{"Rindfleischetikettierungsüberwachungs", "aufgabenübertragungsgesetz und mehr"},
			[]string{"Rindfleischet", "ikettierungsü", "berwachung", "saufgabenüb", "ertragungsg", "esetz und", "mehr"}},
		{"Pneumonoultramicroscopicsilicovolcanoconiosis and friends of the very long words club",
			60, []string{"Pneumonoultramicroscopics", "ilicovolcanoconiosis and", "friends of the very long…"},
			71, []string{"Pneumonoultramicroscopicsilicovolcanoconio", "sis and friends of the very long words club"},
			[]string{"Pneumonoul", "tramicroscop", "icsilicovolca", "noconiosis", "and friends", "of the very", "long words", "club"}},
		// Combining marks, which Go Bold lacks.
		{"Vie\u0323\u0302t Nam: nhu\u031b\u0303ng bo\u0323\u0302 phim q\u0303 hay nha\u0302\u0301t",
			112, []string{"Vie\u0323\u0302t Nam:", "nhu\u031b\u0303ng bo\u0323\u0302 phim", "q\u0303 hay nha\u0302\u0301t"},
			150, []string{"Vie\u0323\u0302t Nam: nhu\u031b\u0303ng bo\u0323\u0302", "phim q\u0303 hay nha\u0302\u0301t"},
			[]string{"Vie\u0323\u0302t Nam:", "nhu\u031b\u0303ng bo\u0323\u0302", "phim q\u0303 hay", "nha\u0302\u0301t"}},
		// Its widest letters, and marks alone between words: three dashes,
		// U+2014, which stay with the word before them.
		{"WWW MMM @@@ \u2014\u2014\u2014 ѪѪѪ ΩΩΩ WMWMWMWMWMWMWMWMWMWMWMWMWM",
			60, []string{"WWW MMM @@@ \u2014\u2014\u2014 ѪѪѪ", "ΩΩΩ", "WMWMWMWMWMWMWM…"},
			68, []string{"WWW MMM @@@ \u2014\u2014\u2014 ѪѪѪ ΩΩΩ", "WMWMWMWMWMWMWMWMWMWMWMWMWM"},
			[]string{"WWW", "MMM @@@ ", "\u2014\u2014\u2014 ѪѪѪ ΩΩΩ", "WMWMWMW", "MWMWMWM", "WMWMWMW", "MWMWM"}},
		// Characters it lacks, among words.
		{"東京物語 and 七人の侍: the Criterion editions",
			132, []string{"東京物語 and 七人の侍: the", "Criterion", "editions"},
			150, []string{"東京物語 and 七人の侍: the Criterion", "editions"},
			[]string{"東京物語 and 七人の侍: the", "Criterion", "editions"}},
		{"Властелин колец: Братство кольца \u2014 режиссёрская версия",
			74, []string{"Властелин колец:", "Братство кольца \u2014", "режиссёрская версия"},
			99, []string{"Властелин колец: Братство", "кольца \u2014 режиссёрская версия"},
			[]string{"Властелин", "колец:", "Братство", "кольца \u2014", "режиссёрск", "ая версия"}},
		{"a b c d e f g h i j k l m n o p q r s t u v w x y z a b c d e f g h i j k l m n o p q r s t u v w x y z",
			60, []string{"a b c d e f g h i j k l m n o p q", "r s t u v w x y z a b c d e f g h", "i j k l m n o p q r s t u v w x…"},
			71, []string{"a b c d e f g h i j k l m n o p q r s t u v w x y z a b", "c d e f g h i j k l m n o p q r s t u v w x y z"},
			[]string{"a b c d e f g", "h i j k l m n o", "p q r s t u v", "w x y z a b c", "d e f g h i j k", "l m n o p q r", "s t u v w x y", "z"}},
		{"( « Les Misérables » ) !",
			132, []string{"( « Les", "Misérables » ", ") !"},
			150, []string{"( « Les Misérables » ) !"},
			[]string{"( « Les", "Misérables » ", ") !"}},
		// Words without a space, of wide letters and of narrow ones.
		{repeat("W", 150),
			60, []string{repeat("W", 14), repeat("W", 14), repeat("W", 13) + "…"},
			68, []string{repeat("W", 25), repeat("W", 24) + "…"},
			slices.Repeat([]string{"WWWWWW"}, 25)},
		{repeat("il.", 60),
			60, []string{repeat("il.", 16), repeat("il.", 16), repeat("il.", 15) + "…"},
			68, []string{repeat("il.", 27) + "i", "l." + repeat("il.", 25) + "i…"},
			append(slices.Repeat([]string{repeat("il.", 7) + "i", "l." + repeat("il.", 6) + "il", "." + repeat("il.", 7)}, 3)[:8], ".il.")},
		// About a collection's longest name, with its spaces.
		{strings.TrimSpace(repeat("Collection of movies ", 24)),
			60, []string{"Collection of movies", "Collection of movies", "Collection of movies…"},
			68, []string{"Collection of movies Collection of movies", "Collection of movies Collection of movies…"},
			slices.Repeat([]string{"Collection of", "movies"}, 24)},
	} {
		if l := fit(test.name, PosterWidth*0.84, 3, posterSizes, fonts); l.size != test.posterSize || !slices.Equal(l.lines, test.poster) {
			t.Errorf("%q on a poster: %v %q, want %v %q", test.name, l.size, l.lines, test.posterSize, test.poster)
		}
		if l := fit(test.name, WideWidth*0.84, 2, wideSizes, fonts); l.size != test.wideSize || !slices.Equal(l.lines, test.wide) {
			t.Errorf("%q on a wide image: %v %q, want %v %q", test.name, l.size, l.lines, test.wideSize, test.wide)
		}
		w := &writer{fonts: fonts, size: posterSizes[0], faces: map[*sfnt.Font]font.Face{}}
		if lines := w.wrap(test.name, PosterWidth*0.84); !slices.Equal(lines, test.largest) {
			t.Errorf("%q at a poster's largest size: %q, want %q", test.name, lines, test.largest)
		}
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
