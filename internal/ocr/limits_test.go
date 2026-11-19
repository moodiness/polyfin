package ocr

import (
	"context"
	"encoding/binary"
	"errors"
	"image"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/testsubtitles"
)

// supSegment is a PGS segment of kind behind its "PG" header, shown at
// zero.
func supSegment(kind byte, payload []byte) []byte {
	header := make([]byte, 13)
	header[0], header[1] = 'P', 'G'
	header[10] = kind
	binary.BigEndian.PutUint16(header[11:13], uint16(len(payload)))
	return append(header, payload...)
}

// supPicture is a SUP file of one display set, on a video of width by
// height, starting an epoch: palette 0, its color 1 opaque white, and
// objects defined by their size and run-length data, placed in order at
// the positions given.
func supPicture(width, height int, objects []supObject) []byte {
	composition := make([]byte, 11)
	binary.BigEndian.PutUint16(composition[0:2], uint16(width))
	binary.BigEndian.PutUint16(composition[2:4], uint16(height))
	composition[7] = 0x80
	composition[10] = byte(len(objects))
	for id, o := range objects {
		placed := make([]byte, 8)
		binary.BigEndian.PutUint16(placed[0:2], uint16(id))
		binary.BigEndian.PutUint16(placed[4:6], uint16(o.x))
		binary.BigEndian.PutUint16(placed[6:8], uint16(o.y))
		composition = append(composition, placed...)
	}
	sup := supSegment(segmentComposition, composition)
	sup = append(sup, supSegment(segmentPalette, []byte{0, 0, 1, 235, 128, 128, 255})...)
	for id, o := range objects {
		object := make([]byte, 11)
		binary.BigEndian.PutUint16(object[0:2], uint16(id))
		object[3] = 0x80
		binary.BigEndian.PutUint16(object[7:9], uint16(o.width))
		binary.BigEndian.PutUint16(object[9:11], uint16(o.height))
		sup = append(sup, supSegment(segmentObject, append(object, o.rle...))...)
	}
	return append(sup, supSegment(segmentEnd, nil)...)
}

// supObject is an object of a SUP file: where it is placed, the size it
// declares and its run-length data.
type supObject struct {
	x, y, width, height int
	rle                 []byte
}

// allocated is how many bytes f allocates.
func allocated(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// A PGS track's pictures are drawn within its video, and at most UHD
// video, whatever its objects declare: what lies past it allocates
// nothing. Pictures upscaled stop short of the bound too.
//
//nolint:paralleltest // counts the process's allocations, which parallel tests would add to
func TestPGSPicturesStayWithinTheScreen(t *testing.T) {
	dot := []byte{1, 0, 0}
	for _, c := range []struct {
		name     string
		sup      []byte
		pictures int
	}{
		{"a 4000 by 4000 object without data", supPicture(1920, 1080, []supObject{{width: 4000, height: 4000}}), 0},
		{"two dots 4000 pixels apart", supPicture(1920, 1080, []supObject{{width: 1, height: 1, rle: dot}, {x: 4000, y: 4000, width: 1, height: 1, rle: dot}}), 1},
	} {
		var pictures []Picture
		var err error
		n := allocated(func() { pictures, err = pgsPictures(c.sup) })
		if err != nil || len(pictures) != c.pictures {
			t.Fatalf("%s: %d pictures, %v", c.name, len(pictures), err)
		}
		// The video's 1920 by 1080 pixels, two bytes each, are 4 MB.
		if n > 16<<20 {
			t.Errorf("%s: %d bytes of SUP allocated %d MB", c.name, len(c.sup), n>>20)
		}
	}
	// A column 8000 pixels tall, on a video declared as tall.
	var column []byte
	for range 8000 {
		column = append(column, 1, 0, 0)
	}
	pictures, err := pgsPictures(supPicture(8000, 8000, []supObject{{width: 1, height: 8000, rle: column}}))
	if err != nil || len(pictures) != 1 {
		t.Fatalf("the column: %d pictures, %v", len(pictures), err)
	}
	if b := pictures[0].Image.Bounds(); b.Dy() > maxHeight+2*margin {
		t.Errorf("the column is drawn %d pixels tall", b.Dy())
	}
	// Lines a pixel tall across a 3000 by 200 object would be upscaled 4
	// times.
	var lines []byte
	for y := range 200 {
		if y%2 == 0 {
			lines = append(lines, 0, 0xc0|byte(3000>>8), byte(3000&0xff), 1, 0, 0)
		} else {
			lines = append(lines, 0, 0)
		}
	}
	pictures, err = pgsPictures(supPicture(3840, 2160, []supObject{{width: 3000, height: 200, rle: lines}}))
	if err != nil || len(pictures) != 1 {
		t.Fatalf("the lines: %d pictures, %v", len(pictures), err)
	}
	if b := pictures[0].Image.Bounds(); b.Dx()*b.Dy() > maxPrepared {
		t.Errorf("the lines are prepared as %v", b)
	}
}

// A VobSub subpicture is drawn within the video its index tells, a DVD's
// at least, whatever area it declares.
//
//nolint:paralleltest // counts the process's allocations, which parallel tests would add to
func TestVobSubPicturesStayWithinTheScreen(t *testing.T) {
	// A transparent subpicture over 4096 by 4096 pixels, its lines empty.
	spu := blankSubpicture()
	var pictures []Picture
	var err error
	n := allocated(func() {
		pictures, err = vobsubBlockPictures([]byte("size: 720x480\n"), []container.Block{{Data: spu}})
	})
	if err != nil || len(pictures) != 0 {
		t.Fatalf("%d pictures, %v", len(pictures), err)
	}
	// A DVD's 720 by 576 pixels, two bytes each, are under 1 MB.
	if n > 8<<20 {
		t.Errorf("%d bytes of subpicture allocated %d MB", len(spu), n>>20)
	}
}

// blankSubpicture is a transparent subpicture over 4096 by 4096 pixels,
// its lines empty: at 4, both fields' lines, then its control sequence at
// 8.
func blankSubpicture() []byte {
	spu := []byte{0, 0, 0, 8, 0, 0, 0, 0,
		0, 0, 0, 8, // no delay, the last sequence
		0x01,             // start
		0x03, 0x01, 0x00, // colors
		0x04, 0x00, 0x00, // transparent
		0x05, 0x00, 0x0f, 0xff, 0x00, 0x0f, 0xff, // from 0, 0 to 4095, 4095
		0x06, 0x00, 0x04, 0x00, 0x06, // lines at 4 and 6
		0xff}
	binary.BigEndian.PutUint16(spu[0:2], uint16(len(spu)))
	return spu
}

// unheaded is SUP segments without their "PG" headers, as a Matroska
// block holds them.
func unheaded(sup []byte) []byte {
	var segments []byte
	for len(sup) > 0 {
		n := 13 + int(binary.BigEndian.Uint16(sup[11:13]))
		segments = append(segments, sup[10:n]...)
		sup = sup[n:]
	}
	return segments
}

// uhdTrack is the blocks of a Matroska PGS track on a 3840 by 2160 video:
// a display set starting an epoch, with palette 0's color 1 opaque white,
// that defines object 0, of width by height pixels drawn from rle, then
// sets display sets. Each display set places the object at each of at.
func uhdTrack(width, height int, rle []byte, sets int, at ...image.Point) []container.Block {
	composition := func(epoch bool) []byte {
		c := make([]byte, 11)
		binary.BigEndian.PutUint16(c[0:2], maxWidth)
		binary.BigEndian.PutUint16(c[2:4], maxHeight)
		if epoch {
			c[7] = 0x80
		}
		c[10] = byte(len(at))
		for _, p := range at {
			c = append(c, 0, 0, 0, 0)
			c = binary.BigEndian.AppendUint16(c, uint16(p.X))
			c = binary.BigEndian.AppendUint16(c, uint16(p.Y))
		}
		return supSegment(segmentComposition, c)
	}
	blocks := []container.Block{{Data: unheaded(slices.Concat(composition(true), supSegment(segmentPalette, []byte{0, 0, 1, 235, 128, 128, 255}),
		supDefinition(0, width, height, rle), supSegment(segmentEnd, nil)))}}
	for i := range sets {
		blocks = append(blocks, container.Block{Start: time.Duration(i+1) * time.Second, Data: unheaded(slices.Concat(composition(false), supSegment(segmentEnd, nil)))})
	}
	return blocks
}

// noLimit is a budget no track reaches.
const noLimit = int64(1) << 62

// budgetedPGS decodes a Matroska PGS track within a budget of pixels. It
// returns how many pictures it handed over.
func budgetedPGS(blocks []container.Block, pixels int64) (int, error) {
	pictures := 0
	err := decodePGS(context.Background(), blockSets(blocks), &pixelBudget{left: pixels}, func(Picture) error {
		pictures++
		return nil
	})
	return pictures, err
}

// A PGS track stops once its display sets have composed and prepared the
// pixels its budget holds, counting what lies between the objects they
// place and what upscaling adds, but not the lines past those an
// object's data reaches.
func TestPGSTracksStopAtTheirPixelBudget(t *testing.T) {
	t.Parallel()
	line := func(width int) []byte { return []byte{0, 0xc0 | byte(width>>8), byte(width), 1, 0, 0} }
	for _, c := range []struct {
		name     string
		blocks   []container.Block
		budget   int64
		pictures int
	}{
		// Each display set composes a whole screen and prepares as much:
		// the third finds too little left.
		{"a dot at both ends of the screen", uhdTrack(1, 1, []byte{1, 0, 0}, 4, image.Pt(0, 0), image.Pt(maxWidth-1, maxHeight-1)), 5 * screenPixels, 2},
		// Each composes 3,000 pixels, prepared upscaled 4 times: 1,209,600.
		{"a line a pixel tall", uhdTrack(3000, 1, line(3000), 4, image.Pt(0, 0)), 3_000_000, 2},
		// Its one line of data is drawn, not the 2,159 lines after it.
		{"an object as large as the screen with one line of data", uhdTrack(maxWidth, maxHeight, line(maxWidth), 0, image.Pt(0, 0)), screenPixels, 1},
	} {
		pictures, err := budgetedPGS(c.blocks, c.budget)
		if pictures != c.pictures || (c.pictures < len(c.blocks)) != errors.Is(err, errPixels) {
			t.Errorf("%s: %d pictures of %d, %v; want %d", c.name, pictures, len(c.blocks), err, c.pictures)
		}
		// Without a budget, every display set is a picture.
		if pictures, err := budgetedPGS(c.blocks, noLimit); pictures != len(c.blocks) || err != nil {
			t.Errorf("%s without a budget: %d pictures of %d, %v", c.name, pictures, len(c.blocks), err)
		}
	}
}

// A VobSub track stops once its subpictures have drawn the pixels its
// budget holds, transparent ones included.
func TestVobSubTracksStopAtTheirPixelBudget(t *testing.T) {
	t.Parallel()
	// On a screen of 3840 by 2160, the index telling no size.
	blocks := slices.Repeat([]container.Block{{Data: blankSubpicture(), Duration: time.Second}}, 5)
	for _, c := range []struct {
		budget int64
		read   int
		err    error
	}{
		{budget: 3*screenPixels + screenPixels/2, read: 4, err: errPixels},
		{budget: noLimit, read: 5},
	} {
		// The context is asked before each unit.
		ctx := &asked{Context: context.Background()}
		err := decodeUnits(ctx, blockUnits(blocks), vobsubIndex{}, &pixelBudget{left: c.budget}, func(Picture) error { return nil })
		if ctx.asks != c.read || !errors.Is(err, c.err) {
			t.Errorf("budget %d: %d subpictures read, %v; want %d, %v", c.budget, ctx.asks, err, c.read, c.err)
		}
	}
}

// A track drawing its pictures from its own data, as subtitles of text
// do, is read whole within pixelsPerByte for each of its bytes, without
// the screens every track is allowed besides.
func TestTextTracksStayWithinTheirBytes(t *testing.T) {
	t.Parallel()
	uhd := testsubtitles.Captions{CanvasWidth: maxWidth, CanvasHeight: maxHeight, Size: 108, Shown: captions}
	sup := uhd.PGS()
	var pictures int
	count := func(Picture) error {
		pictures++
		return nil
	}
	if err := decodePGS(context.Background(), supSets(sup), &pixelBudget{left: pixelsPerByte * int64(len(sup))}, count); err != nil || pictures != len(captions) {
		t.Errorf("3840 by 2160 PGS: %d pictures, %v", pictures, err)
	}
	dvd := testsubtitles.Captions{CanvasWidth: 720, CanvasHeight: 480, Size: 30, Shown: captions}
	idx, sub := dvd.VobSub("en")
	index, err := parseIndex([]byte(idx), nil)
	if err != nil {
		t.Fatal(err)
	}
	pictures = 0
	if err := decodeUnits(context.Background(), pairUnits([]byte(idx), sub), index, &pixelBudget{left: pixelsPerByte * int64(len(sub))}, count); err != nil || pictures != len(captions) {
		t.Errorf("DVD VobSub: %d pictures, %v", pictures, err)
	}
	// And the budget of a track grows with its size, up to its bound.
	for _, c := range []struct {
		size int
		want int64
	}{
		{0, minTrackScreens * screenPixels},
		{1 << 20, minTrackScreens*screenPixels + pixelsPerByte<<20},
		{256 << 20, maxTrackScreens * screenPixels},
	} {
		if got := newPixelBudget(c.size).left; got != c.want {
			t.Errorf("a track of %d bytes may compose %d pixels, want %d", c.size, got, c.want)
		}
	}
}
