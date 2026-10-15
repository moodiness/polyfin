package ocr

import (
	"encoding/binary"
	"runtime"
	"testing"

	"github.com/moodiness/polyfin/internal/container"
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
func TestVobSubPicturesStayWithinTheScreen(t *testing.T) {
	// A transparent subpicture over 4096 by 4096 pixels, its lines empty:
	// at 4, both fields' lines, then its control sequence at 8.
	spu := []byte{0, 0, 0, 8, 0, 0, 0, 0,
		0, 0, 0, 8, // no delay, the last sequence
		0x01,             // start
		0x03, 0x01, 0x00, // colors
		0x04, 0x00, 0x00, // transparent
		0x05, 0x00, 0x0f, 0xff, 0x00, 0x0f, 0xff, // from 0, 0 to 4095, 4095
		0x06, 0x00, 0x04, 0x00, 0x06, // lines at 4 and 6
		0xff}
	binary.BigEndian.PutUint16(spu[0:2], uint16(len(spu)))
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
