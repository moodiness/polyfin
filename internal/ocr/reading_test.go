package ocr

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/testsubtitles"
)

// asked is a context that counts how often it is asked whether it has
// ended, and has ended from the ask numbered end on: never when end is 0.
// The decoders ask it before each subpicture unit, before each display
// set, and as a composition is drawn, every pgsWork runs and pixels.
type asked struct {
	context.Context
	asks, end int
}

func (c *asked) Err() error {
	c.asks++
	if c.end > 0 && c.asks >= c.end {
		return context.Canceled
	}
	return nil
}

// samePictures checks got are the pictures of want: their times and
// their pixels.
func samePictures(t *testing.T, got, want []Picture) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("pictures at %v, want them at %v", times(got), times(want))
	}
	for i, p := range got {
		w := want[i]
		if p.Start != w.Start || p.End != w.End || p.Forced != w.Forced {
			t.Errorf("picture %d shows from %v to %v, forced %t, want from %v to %v, forced %t", i, p.Start, p.End, p.Forced, w.Start, w.End, w.Forced)
		}
		if p.Image.Rect != w.Image.Rect || !bytes.Equal(p.Image.Pix, w.Image.Pix) {
			t.Errorf("picture %d, %v, is not the picture wanted, %v", i, p.Image.Rect, w.Image.Rect)
		}
	}
}

// vobsubPair is a VobSub pair of three captions: the lines of its .idx
// file before its subpictures, the line of each subpicture, and its .sub
// file, each subpicture in packs of its own.
func vobsubPair() (header string, stamps []string, sub []byte) {
	shown := append(slices.Clone(captions), testsubtitles.Caption{Lines: []string{"The harbor lights went out"}, Start: 9 * time.Second, End: 11 * time.Second})
	idx, sub := testsubtitles.Captions{CanvasWidth: 720, CanvasHeight: 480, Size: 36, Shown: shown}.VobSub("en")
	for line := range strings.Lines(idx) {
		if strings.HasPrefix(line, "timestamp:") {
			stamps = append(stamps, line)
		} else {
			header += line
		}
	}
	return header, stamps, sub
}

// filepos is the position a line of a .idx file lists.
func filepos(t *testing.T, stamp string) int {
	t.Helper()
	_, position, _ := strings.Cut(stamp, "filepos:")
	n, err := strconv.ParseInt(strings.TrimSpace(position), 16, 64)
	if err != nil {
		t.Fatalf("%q: %v", stamp, err)
	}
	return int(n)
}

// vobsubWhole decodes a .idx and .sub pair as a reader holding all of it
// does: every subpicture the .idx file lists read first, then decoded.
func vobsubWhole(idx, sub []byte) ([]Picture, error) {
	var units []timedUnit
	index, err := parseIndex(idx, func(entry vobsubEntry) error {
		unit, _, err := subpictureAt(sub, int(entry.position))
		units = append(units, timedUnit{at: entry.at, data: unit})
		return err
	})
	if err != nil {
		return nil, err
	}
	var pictures []Picture
	err = decodeUnits(context.Background(), func(yield func(timedUnit, error) bool) {
		for _, unit := range units {
			if !yield(unit, nil) {
				return
			}
		}
	}, index, newPixelBudget(len(sub)), func(p Picture) error {
		pictures = append(pictures, p)
		return nil
	})
	return pictures, err
}

// The subpictures of a pair are read from the .sub file one at a time, as
// they are decoded: those before one that cannot be read are handed over
// before it is read, and none is held for the ones after it.
func TestVobSubSubpicturesAreReadOneAtATime(t *testing.T) {
	t.Parallel()
	header, stamps, sub := vobsubPair()
	// The third subpicture's pack no longer starts as a pack does.
	sub[filepos(t, stamps[2])+2] = 0xff
	ctx := &asked{Context: context.Background()}
	var asks []int
	err := VobSub(ctx, []byte(header+strings.Join(stamps, "")), sub, func(Picture) error {
		asks = append(asks, ctx.asks)
		return nil
	})
	if !errors.Is(err, errInvalid) {
		t.Fatalf("a pack that does not start as one: %v", err)
	}
	// Each picture is handed over when its own subpicture is the last one
	// read.
	if !slices.Equal(asks, []int{1, 2}) {
		t.Errorf("pictures handed over after %v subpictures were read, want after 1, then 2", asks)
	}
}

// However many lines of a .idx file list a subpicture, it is read and
// decoded once: a .idx file makes no more subpictures than its .sub file
// holds.
func TestVobSubLinesListingASubpictureAgainAreSkipped(t *testing.T) {
	t.Parallel()
	// A subpicture of the most a unit holds, 8 by 2 opaque pixels: its
	// lines from byte 4, read again by each field, and its control
	// sequence at the end.
	const unitSize, lines = 65531, 500
	control := unitSize - 24
	unit := bytes.Repeat([]byte{0xff}, unitSize)
	binary.BigEndian.PutUint16(unit[0:2], unitSize)
	binary.BigEndian.PutUint16(unit[2:4], uint16(control))
	copy(unit[control:], []byte{0, 0, byte(control >> 8), byte(control), // no delay, the last sequence
		0x03, 0x32, 0x10, // colors
		0x04, 0xff, 0xff, // opaque
		0x05, 0x00, 0x00, 0x07, 0x00, 0x00, 0x01, // from 0, 0 to 7, 1
		0x06, 0x00, 0x04, 0x00, 0x04, // lines at 4
		0x01, 0xff})
	// One private stream packet of the most a packet holds: its header's
	// flags and length, the substream's number, then the unit.
	sub := append([]byte{0, 0, 1, 0xbd, 0xff, 0xff, 0x81, 0, 0, 0x20}, unit...)
	idx := bytes.Repeat([]byte("timestamp: 00:00:00:000, filepos: 000000000\n"), lines)
	ctx := &asked{Context: context.Background()}
	pictures := 0
	if err := VobSub(ctx, idx, sub, func(Picture) error { pictures++; return nil }); err != nil {
		t.Fatal(err)
	}
	if pictures != 1 || ctx.asks != 1 {
		t.Errorf("%d lines listing one subpicture: %d pictures, %d subpictures decoded, want 1 and 1", lines, pictures, ctx.asks)
	}

	// The same for the lines of a real pair: listed again, out of order,
	// or at the second pack of a subpicture read already.
	header, stamps, sub := vobsubPair()
	whole, err := vobsubPictures([]byte(header+strings.Join(stamps, "")), sub)
	if err != nil || len(whole) != 3 {
		t.Fatalf("the pair: pictures at %v, %v", times(whole), err)
	}
	within := "timestamp: 00:00:06:000, filepos: " + strconv.FormatInt(int64(filepos(t, stamps[1])+2048), 16) + "\n"
	if filepos(t, stamps[2]) < filepos(t, stamps[1])+2*2048 {
		t.Fatalf("the second subpicture fits in a pack: %q", stamps)
	}
	for _, c := range []struct {
		name  string
		lines []string
		want  []Picture
	}{
		{"each line twice", []string{stamps[0], stamps[0], stamps[1], stamps[1], stamps[2], stamps[2]}, whole},
		{"a line within the subpicture before", []string{stamps[0], stamps[1], within, stamps[2]}, whole},
		{"the second subpicture after the third", []string{stamps[0], stamps[2], stamps[1]}, []Picture{whole[0], whole[2]}},
	} {
		got, err := vobsubPictures([]byte(header+strings.Join(c.lines, "")), sub)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		samePictures(t, got, c.want)
	}
}

// Reading a pair one subpicture at a time changes none of its pictures.
func TestVobSubPairsDecodeAsWhenReadWhole(t *testing.T) {
	t.Parallel()
	header, stamps, sub := vobsubPair()
	rectangle, rectangleSub := testsubtitles.Rectangle{CanvasWidth: 720, CanvasHeight: 480, X: 100, Y: 300, Width: 200, Height: 40,
		Shown: [][2]time.Duration{{time.Second, 3 * time.Second}, {4 * time.Second, 5 * time.Second}}}.VobSub()
	for _, pair := range []struct{ idx, sub []byte }{{[]byte(header + strings.Join(stamps, "")), sub}, {[]byte(rectangle), rectangleSub}} {
		want, err := vobsubWhole(pair.idx, pair.sub)
		if err != nil || len(want) < 2 {
			t.Fatalf("read whole: pictures at %v, %v", times(want), err)
		}
		got, err := vobsubPictures(pair.idx, pair.sub)
		if err != nil {
			t.Fatal(err)
		}
		samePictures(t, got, want)
	}
}

// A context that ends stops the reading of VobSub subpictures at the
// next one, from a pair and from a Matroska track.
func TestVobSubReadingEndsWithItsContext(t *testing.T) {
	t.Parallel()
	header, stamps, sub := vobsubPair()
	var blocks []container.Block
	for _, stamp := range stamps {
		unit, _, err := subpictureAt(sub, filepos(t, stamp))
		if err != nil {
			t.Fatal(err)
		}
		blocks = append(blocks, container.Block{Start: time.Duration(len(blocks)) * time.Minute, Data: unit})
	}
	for name, read := range map[string]func(context.Context, func(Picture) error) error{
		"a pair": func(ctx context.Context, emit func(Picture) error) error {
			return VobSub(ctx, []byte(header+strings.Join(stamps, "")), sub, emit)
		},
		"a track": func(ctx context.Context, emit func(Picture) error) error {
			return VobSubBlocks(ctx, []byte(header), blocks, emit)
		},
	} {
		// The context ends once the first subpicture is decoded.
		ctx := &asked{Context: context.Background(), end: 2}
		pictures := 0
		err := read(ctx, func(Picture) error { pictures++; return nil })
		if !errors.Is(err, context.Canceled) || pictures != 1 || ctx.asks != 2 {
			t.Errorf("%s: %d pictures, the context asked %d times, %v: want 1 picture, 2 asks and the context's end", name, pictures, ctx.asks, err)
		}
	}
}

// supPlaced is an object as a composition places it: where, and the part
// of it shown, when crop is not empty.
type supPlaced struct {
	object, x, y int
	crop         image.Rectangle
}

// supComposition is a composition segment on a video of 1920 by 1080,
// using palette 0, starting an epoch or not, and placing objects.
func supComposition(epoch bool, placed []supPlaced) []byte {
	c := make([]byte, 11)
	binary.BigEndian.PutUint16(c[0:2], 1920)
	binary.BigEndian.PutUint16(c[2:4], 1080)
	if epoch {
		c[7] = 0x80
	}
	c[10] = byte(len(placed))
	for _, p := range placed {
		object := make([]byte, 8)
		binary.BigEndian.PutUint16(object[0:2], uint16(p.object))
		binary.BigEndian.PutUint16(object[4:6], uint16(p.x))
		binary.BigEndian.PutUint16(object[6:8], uint16(p.y))
		if !p.crop.Empty() {
			object[3] = 0x80
			for _, v := range []int{p.crop.Min.X, p.crop.Min.Y, p.crop.Dx(), p.crop.Dy()} {
				object = binary.BigEndian.AppendUint16(object, uint16(v))
			}
		}
		c = append(c, object...)
	}
	return supSegment(segmentComposition, c)
}

// supDefinition is the segments defining an object: its size and its
// run-length data, in as many segments as it takes.
func supDefinition(id, width, height int, rle []byte) []byte {
	first := make([]byte, 11)
	binary.BigEndian.PutUint16(first[0:2], uint16(id))
	first[3] = 0x80
	binary.BigEndian.PutUint16(first[7:9], uint16(width))
	binary.BigEndian.PutUint16(first[9:11], uint16(height))
	var sup []byte
	for head := first; head != nil; {
		n := min(len(rle), 0xffff-len(head))
		sup = append(sup, supSegment(segmentObject, append(head, rle[:n]...))...)
		rle, head = rle[n:], nil
		if len(rle) > 0 {
			head = binary.BigEndian.AppendUint16(nil, uint16(id))
			head = append(head, 0, 0)
		}
	}
	return sup
}

// supRepeated is a SUP file of an epoch defining objects of 1 by 1 pixel,
// each with data bytes of run-length data that end no line, in a palette
// that makes them transparent, then of sets display sets that each place
// them, in turn, placed times in all. It shows nothing.
func supRepeated(objects, data, sets, placed int) []byte {
	var once, often []supPlaced
	for i := range objects {
		once = append(once, supPlaced{object: i})
	}
	for i := range placed {
		often = append(often, supPlaced{object: i % objects})
	}
	sup := supComposition(true, once)
	sup = append(sup, supSegment(segmentPalette, []byte{0, 0, 1, 235, 128, 128, 0})...)
	for i := range objects {
		sup = append(sup, supDefinition(i, 1, 1, bytes.Repeat([]byte{1}, data))...)
	}
	sup = append(sup, supSegment(segmentEnd, nil)...)
	for range sets {
		sup = append(sup, supComposition(false, often)...)
		sup = append(sup, supSegment(segmentEnd, nil)...)
	}
	return sup
}

// What a file costs to decode grows with its size, not with its objects'
// data times the places of its compositions, nor times its display sets:
// a composition draws under pgsWork runs and pixels, so that the context
// is asked for the display sets alone, whatever the data of the objects
// and however often they are placed.
func TestPGSObjectsAreReadOnceHoweverOftenTheyArePlaced(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ objects, data, sets, placed int }{
		{objects: 1, data: 32 << 10, sets: 4, placed: 1},
		{objects: 1, data: 32 << 10, sets: 4, placed: 255},
		{objects: 1, data: 64 << 10, sets: 8, placed: 255},
		{objects: 2, data: 32 << 10, sets: 4, placed: 255},
	} {
		ctx := &asked{Context: context.Background()}
		pictures := 0
		if err := PGS(ctx, supRepeated(c.objects, c.data, c.sets, c.placed), func(Picture) error { pictures++; return nil }); err != nil || pictures != 0 {
			t.Fatalf("%+v: %d pictures, %v", c, pictures, err)
		}
		if want := 1 + c.sets; ctx.asks != want {
			t.Errorf("%+v: the context asked %d times, want %d, once for each display set", c, ctx.asks, want)
		}
	}
}

// pgsDefined is an object given its run-length data in segments of at
// most segment bytes, as a display set defines it.
func pgsDefined(width, height int, rle []byte, segment int) *pgsObject {
	object := &pgsObject{width: width, height: height}
	for len(rle) > 0 {
		n := min(segment, len(rle))
		object.data = append(object.data, rle[:n]...)
		object.trim()
		rle = rle[n:]
	}
	return object
}

// An object keeps of its data what can show, whatever it is given: the
// runs of each line up to its width, the lines up to its height.
func TestAPGSObjectKeepsOfItsDataWhatCanShow(t *testing.T) {
	t.Parallel()
	// A pixel, then data that ends no line: as much as a file holds.
	for _, data := range []int{16 << 10, 32 << 10} {
		if object := pgsDefined(1, 1, bytes.Repeat([]byte{1}, data), 0xffff); len(object.data) != 1 {
			t.Errorf("an object of a pixel given %d bytes keeps %d", data, len(object.data))
		}
	}
	rle := []byte{
		1,          // a pixel
		0, 0x80, 2, // a run of no pixel
		0, 0x82, 3, // two pixels, which fill the line
		5,             // a pixel past the line's width
		0, 0x45, 0x00, // and 1280 more
		0, 0, // the end of the line
		0, 0x7f, 0xff, // 16383 pixels, from within the line
		7,    // a pixel past the width
		0, 0, // the end of the last line
		1, 1, 1, 0, 0, // a line past the object's height
		0, 0x82, // the start of a run
	}
	want := []byte{1, 0, 0x82, 3, 0, 0, 0, 0x7f, 0xff, 0, 0, 0, 0x82}
	for _, segment := range []int{len(rle), 1, 2, 5} {
		if object := pgsDefined(3, 2, rle, segment); !bytes.Equal(object.data, want) {
			t.Errorf("in segments of %d bytes: the object keeps % x, want % x", segment, object.data, want)
		}
	}
}

// A part of an object costs its own runs and pixels to draw, wherever it
// lies in the object and whatever else the object's data holds: for each
// of its lines the runs from a mark, less than pgsMarkGap bytes and a
// run before its left edge, to its right edge.
func TestAPartOfAPGSObjectIsDrawnFromItsOwnRuns(t *testing.T) {
	t.Parallel()
	palette := &[256]pixel{1: {luma: 235, alpha: 255}}
	// lines of width pixels, a byte each.
	lines := func(width, height int) []byte {
		var rle []byte
		for range height {
			rle = append(append(rle, bytes.Repeat([]byte{1}, width)...), 0, 0)
		}
		return rle
	}
	// A pixel behind 20,000 runs of no pixel, on the line after one that
	// 20,000 pixels overflow.
	behind := append(bytes.Repeat([]byte{1}, 20_000), 0, 0)
	behind = append(append(behind, bytes.Repeat([]byte{0, 0x80, 1}, 20_000)...), 1)
	for _, c := range []struct {
		name          string
		width, height int
		rle           []byte
		shown         image.Rectangle
	}{
		{"the last line of 2000", 100, 2000, lines(100, 2000), image.Rect(0, 1999, 100, 2000)},
		{"the right half of a line", 100, 2000, lines(100, 2000), image.Rect(50, 1000, 100, 1001)},
		{"the left half of two lines", 100, 2000, lines(100, 2000), image.Rect(0, 1000, 50, 1002)},
		{"the right edge of wide lines", 20_000, 30, lines(20_000, 30), image.Rect(19_900, 20, 20_000, 30)},
		{"a line among lines too short for a mark each", 2, 2000, lines(2, 2000), image.Rect(0, 1501, 2, 1502)},
		{"a pixel behind runs of none", 1, 2, behind, image.Rect(0, 1, 1, 2)},
	} {
		object := pgsDefined(c.width, c.height, c.rle, 0xffff)
		canvas := newBitmap(c.shown.Dx(), c.shown.Dy())
		work := drawPGSObject(canvas, object, palette, c.shown, image.Pt(0, 0).Sub(c.shown.Min))
		// A line, the runs up to its left edge, then a run and a pixel
		// for each of its pixels.
		if most := c.shown.Dy()*(1+pgsMarkGap+3) + 2*c.shown.Dx()*c.shown.Dy(); work > most {
			t.Errorf("%s: drawn for %d lines, runs and pixels, want %d at most", c.name, work, most)
		}
		if slices.Contains(canvas.pixels, pixel{}) {
			t.Errorf("%s: not all of the part is drawn", c.name)
		}
		// A mark is kept for 64 bytes of data at least.
		if len(object.marks)*pgsMarkGap > len(object.data) {
			t.Errorf("%s: %d marks kept for %d bytes of data", c.name, len(object.marks), len(object.data))
		}
	}
}

// pgsPixels decodes run-length data whole, as its format says: the colors
// of an object of width by height, -1 where the data gives no pixel.
func pgsPixels(width, height int, rle []byte) [][]int {
	pixels := make([][]int, height)
	for y := range pixels {
		pixels[y] = slices.Repeat([]int{-1}, width)
	}
	x, y := 0, 0
	put := func(color byte, n int) {
		for ; n > 0 && x < width; n-- {
			if y < height {
				pixels[y][x] = int(color)
			}
			x++
		}
	}
	for i := 0; i < len(rle); {
		c := rle[i]
		i++
		if c != 0 {
			put(c, 1)
			continue
		}
		if i >= len(rle) {
			break
		}
		flags := rle[i]
		i++
		n := int(flags & 0x3f)
		if flags&0x40 != 0 {
			// The long form: a second byte of length.
			if i >= len(rle) {
				break
			}
			n = n<<8 | int(rle[i])
			i++
		}
		switch {
		case flags == 0:
			x, y = 0, y+1
		case flags&0x80 == 0:
			put(0, n)
		case i >= len(rle):
			return pixels
		default:
			put(rle[i], n)
			i++
		}
	}
	return pixels
}

// The parts of objects are drawn as their data says, whatever the data:
// runs of every form, of no pixel, past the width, lines past the height,
// data cut within a run, given in segments of any size.
func TestPGSObjectsDrawWhatTheirDataSays(t *testing.T) {
	t.Parallel()
	// Color 0 is opaque too in the second palette.
	palettes := []*[256]pixel{{1: {luma: 10, alpha: 255}, 2: {luma: 20, alpha: 255}, 3: {luma: 30, alpha: 1}},
		{0: {luma: 5, alpha: 255}, 1: {luma: 10, alpha: 255}, 2: {luma: 20, alpha: 255}, 3: {luma: 30, alpha: 255}}}
	random := rand.New(rand.NewPCG(7, 11))
	for round := range 300 {
		width, height := 1+random.IntN(40), 1+random.IntN(30)
		if round%10 == 0 {
			width = 300 + random.IntN(400)
		}
		var rle []byte
		for range random.IntN(400) {
			color, n := byte(1+random.IntN(3)), random.IntN(2*width)
			if random.IntN(4) == 0 {
				n = 0
			}
			switch random.IntN(12) {
			case 0, 1, 2:
				rle = append(rle, color)
			case 3, 4:
				rle = append(rle, 0, 0)
			case 5:
				// The short form has no run of no pixel: it is the end of a line.
				rle = append(rle, 0, byte(max(n, 1)&0x3f|1))
			case 6:
				rle = append(rle, 0, 0x40|byte(n>>8), byte(n))
			case 7, 8:
				rle = append(rle, 0, 0x80|byte(n&0x3f), color)
			case 9:
				rle = append(rle, 0, 0xc0|byte(n>>8), byte(n), color)
			default:
				// A short line.
				rle = append(rle, color, 0, 0)
			}
		}
		rle = rle[:len(rle)-random.IntN(min(len(rle), 3)+1)]
		want := pgsPixels(width, height, rle)
		object := pgsDefined(width, height, rle, 1+random.IntN(64))
		if len(object.data) > len(rle) || len(object.data) > height*(4*width+2)+3 {
			t.Fatalf("round %d: an object of %d by %d given %d bytes keeps %d", round, width, height, len(rle), len(object.data))
		}
		if len(object.marks)*pgsMarkGap > len(object.data) {
			t.Fatalf("round %d: %d marks kept for %d bytes of data", round, len(object.marks), len(object.data))
		}
		for range 4 {
			// The whole object, then parts of it.
			shown := image.Rect(0, 0, width, height)
			if round%4 != 0 {
				x, y := random.IntN(width), random.IntN(height)
				shown = image.Rect(x, y, x+1+random.IntN(width-x), y+1+random.IntN(height-y))
			}
			palette := palettes[random.IntN(2)]
			canvas := newBitmap(shown.Dx(), shown.Dy())
			drawPGSObject(canvas, object, palette, shown, image.Pt(0, 0).Sub(shown.Min))
			for y := shown.Min.Y; y < shown.Max.Y; y++ {
				for x := shown.Min.X; x < shown.Max.X; x++ {
					var wanted pixel
					if color := want[y][x]; color >= 0 && palette[color].alpha > 0 {
						wanted = palette[color]
					}
					if got := canvas.at(x-shown.Min.X, y-shown.Min.Y); got != wanted {
						t.Fatalf("round %d, an object of %d by %d from % x: pixel %d, %d of %v is %v, want %v", round, width, height, rle, x, y, shown, got, wanted)
					}
				}
			}
		}
	}
}

// An object placed several times shows at each of its places, whole or
// cropped, as an object of its own would at each.
func TestAPGSObjectPlacedSeveralTimesShowsAtEachPlace(t *testing.T) {
	t.Parallel()
	// 40 by 30 pixels: on each line a longer run of color 1, three clear
	// pixels, then color 2 to the end; nothing ends the last line.
	const width, height = 40, 30
	var rle []byte
	for y := range height {
		rle = append(rle, 0, 0x80|byte(y+2), 1, 0, 3, 0, 0x80|byte(width-y-5), 2)
		if y < height-1 {
			rle = append(rle, 0, 0)
		}
	}
	palette := supSegment(segmentPalette, []byte{0, 0, 1, 235, 128, 128, 255, 2, 16, 128, 128, 255})
	for _, c := range []struct {
		name   string
		places []supPlaced
	}{
		{"whole, three times", []supPlaced{{x: 10, y: 10}, {x: 300, y: 10}, {x: 10, y: 200}}},
		{"whole and cropped", []supPlaced{{x: 10, y: 10}, {x: 300, y: 50, crop: image.Rect(5, 8, 25, 20)}, {x: 600, y: 90, crop: image.Rect(30, 25, 40, 30)}}},
		{"cropped past its first lines", []supPlaced{{x: 10, y: 10, crop: image.Rect(0, 12, 40, 18)}, {x: 300, y: 50, crop: image.Rect(20, 20, 36, 29)}}},
		{"past the screen's edges", []supPlaced{{x: 1900, y: 10}, {x: 10, y: 1070}, {x: 500, y: 500}}},
	} {
		// The same object at every place, then an object for each place.
		same := append(supComposition(true, c.places), palette...)
		same = append(same, supDefinition(0, width, height, rle)...)
		same = append(same, supSegment(segmentEnd, nil)...)
		each := slices.Clone(c.places)
		for i := range each {
			each[i].object = i
		}
		apart := append(supComposition(true, each), palette...)
		for i := range each {
			apart = append(apart, supDefinition(i, width, height, rle)...)
		}
		apart = append(apart, supSegment(segmentEnd, nil)...)
		got, err := pgsPictures(same)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		want, err := pgsPictures(apart)
		if err != nil || len(want) != 1 {
			t.Fatalf("%s, an object for each place: %d pictures, %v", c.name, len(want), err)
		}
		samePictures(t, got, want)
	}
}

// A context that ends stops the reading of PGS display sets at the next
// one, from a SUP file and from a Matroska track, also when nothing they
// show is handed over, and stops a composition that takes long to draw.
func TestPGSReadingEndsWithItsContext(t *testing.T) {
	t.Parallel()
	// Four display sets: each caption shown, then cleared.
	sup := testsubtitles.Captions{CanvasWidth: 1920, CanvasHeight: 1080, Size: 56, Shown: captions}.PGS()
	var blocks []container.Block
	for rest := sup; len(rest) > 0; {
		n := 13 + int(binary.BigEndian.Uint16(rest[11:13]))
		if len(blocks) == 0 || rest[10] == segmentComposition {
			blocks = append(blocks, container.Block{Start: ticks(binary.BigEndian.Uint32(rest[2:6]))})
		}
		block := &blocks[len(blocks)-1]
		block.Data = append(block.Data, rest[10:n]...)
		rest = rest[n:]
	}
	if len(blocks) != 4 {
		t.Fatalf("%d display sets, want 4", len(blocks))
	}
	for name, read := range map[string]func(context.Context, func(Picture) error) error{
		"a file":  func(ctx context.Context, emit func(Picture) error) error { return PGS(ctx, sup, emit) },
		"a track": func(ctx context.Context, emit func(Picture) error) error { return PGSBlocks(ctx, blocks, emit) },
	} {
		// The context is asked for the first display set, for the second,
		// which ends the first picture, and has ended at the third.
		ctx := &asked{Context: context.Background(), end: 3}
		pictures := 0
		err := read(ctx, func(Picture) error { pictures++; return nil })
		if !errors.Is(err, context.Canceled) || pictures != 1 || ctx.asks != 3 {
			t.Errorf("%s: %d pictures, the context asked %d times, %v: want 1 picture, 3 asks and the context's end", name, pictures, ctx.asks, err)
		}
	}
	// A file that shows nothing: of its 101 display sets, the context ends
	// the reading at the third.
	ctx := &asked{Context: context.Background(), end: 3}
	err := PGS(ctx, supRepeated(1, 1<<10, 100, 255), func(Picture) error {
		t.Error("a picture of transparent objects")
		return nil
	})
	if !errors.Is(err, context.Canceled) || ctx.asks != 3 {
		t.Errorf("a file that shows nothing: the context asked %d times, %v: want 3 asks and the context's end", ctx.asks, err)
	}
	// A composition placing an object as large as its video three times
	// draws more than pgsWork pixels: the context is asked once for its
	// display set, and once as it is drawn.
	var screen []byte
	for range 1080 {
		screen = append(screen, 0, 0xc0|1920>>8, 1920&0xff, 1, 0, 0)
	}
	long := supComposition(true, []supPlaced{{}, {}, {}})
	long = append(long, supSegment(segmentPalette, []byte{0, 0, 1, 235, 128, 128, 255})...)
	long = append(long, supDefinition(0, 1920, 1080, screen)...)
	long = append(long, supSegment(segmentEnd, nil)...)
	ctx = &asked{Context: context.Background(), end: 2}
	pictures := 0
	err = PGS(ctx, long, func(Picture) error { pictures++; return nil })
	if !errors.Is(err, context.Canceled) || pictures != 0 || ctx.asks != 2 {
		t.Errorf("a composition long to draw: %d pictures, the context asked %d times, %v: want no picture, 2 asks and the context's end", pictures, ctx.asks, err)
	}
}

// The display sets of a SUP file are decoded where the file holds them:
// reading it copies none of its segments, and holds no list of them.
//
//nolint:paralleltest // counts the process's allocations, which parallel tests would add to
func TestPGSDisplaySetsAreDecodedWhereTheirFileHoldsThem(t *testing.T) {
	// Display sets of an end segment alone, the shortest there is.
	sup := bytes.Repeat(supSegment(segmentEnd, nil), 100_000)
	var err error
	n := allocated(func() {
		err = PGS(context.Background(), sup, func(Picture) error { return errors.New("a picture of nothing") })
	})
	if err != nil {
		t.Fatal(err)
	}
	if n > uint64(len(sup))/4 {
		t.Errorf("a SUP file of %d bytes allocated %d bytes", len(sup), n)
	}
}
