package accounts

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/testpictures"
)

// blankPNG returns a PNG picture of width by height transparent pixels of
// depth bits a channel, all of them zero, without ever holding its
// pixels: every row is the same deflated block, written once for each.
func blankPNG(t *testing.T, width, height, depth int) []byte {
	t.Helper()
	var out bytes.Buffer
	out.WriteString("\x89PNG\r\n\x1a\n")
	chunk := func(kind string, data []byte) {
		out.Write(binary.BigEndian.AppendUint32(nil, uint32(len(data))))
		sum := crc32.NewIEEE()
		for _, part := range [][]byte{[]byte(kind), data} {
			out.Write(part)
			sum.Write(part)
		}
		out.Write(sum.Sum(nil))
	}
	header := make([]byte, 13)
	binary.BigEndian.PutUint32(header, uint32(width))
	binary.BigEndian.PutUint32(header[4:], uint32(height))
	header[8], header[9] = byte(depth), 6 // red, green, blue and transparency
	chunk("IHDR", header)
	// A row is its filter's byte, none, then its pixels.
	rowSize := 1 + width*depth/2
	var row bytes.Buffer
	deflate, err := flate.NewWriter(&row, flate.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deflate.Write(make([]byte, rowSize)); err != nil {
		t.Fatal(err)
	}
	if err := deflate.Flush(); err != nil {
		t.Fatal(err)
	}
	data := []byte{0x78, 0x9c}
	for range height {
		data = append(data, row.Bytes()...)
	}
	// An empty last block, then the sum of the zeros: one, and their
	// number.
	data = append(data, 0x01, 0x00, 0x00, 0xff, 0xff)
	data = binary.BigEndian.AppendUint32(data, uint32(height*rowSize%65521)<<16|1)
	chunk("IDAT", data)
	chunk("IEND", nil)
	return out.Bytes()
}

// jpegHeader returns the start of a JPEG picture of width by height in
// colours, up to its frame header, of the marker given: enough to tell
// its size, and nothing to decode.
func jpegHeader(width, height int, frame byte) []byte {
	return []byte{
		0xff, 0xd8,
		0xff, 0xe0, 0, 16, 'J', 'F', 'I', 'F', 0, 1, 1, 0, 0, 1, 0, 1, 0, 0,
		0xff, frame, 0, 17, 8, byte(height >> 8), byte(height), byte(width >> 8), byte(width), 3,
		1, 0x22, 0, 2, 0x11, 1, 3, 0x11, 1,
		0xff, 0xd9,
	}
}

// photo returns a JPEG picture of width by height that compresses as a
// photograph does: slow gradients under a fine grain.
func photo(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	grain := uint32(1)
	for y := range height {
		for x := range width {
			grain = grain*1664525 + 1013904223
			g := uint8(grain >> 28)
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(x*200/width) + g,
				G: uint8(y*200/height) + g,
				B: uint8((x+y)*200/(width+height)) + g,
				A: 255,
			})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// What normalizing a picture holds is counted from its header: the
// decoded picture and what its decoder holds beside it, then what scaling
// it down or encoding it does. The numbers are worked out by hand from
// the decoders and the scaler, for the sizes the two bounds are chosen
// by.
func TestTheMemoryAPictureTakesIsCountedFromItsHeader(t *testing.T) {
	t.Parallel()
	const baseline, progressive = 0xc0, 0xc2
	png := func(interlaced byte) []byte { return append(make([]byte, 28), interlaced) }
	for _, c := range []struct {
		name          string
		format        string
		data          []byte
		model         color.Model
		width, height int
		maxSide       int
		want          int64
	}{
		// 4 bytes for each of 4,655 by 3,499 pixels, the sides rounded up,
		// and the scaler: 32 bytes for 512 by 3,468, the result of 512 by
		// 384 at 4 bytes, 64 bytes for 8,092 columns and rows, 56 for 896.
		{"a photo of 16 million pixels", "jpeg", jpegHeader(4624, 3468, baseline), color.YCbCrModel, 4624, 3468, MaxImageSide, 123_325_588},
		{"a photo of 12 million pixels", "jpeg", jpegHeader(4032, 3024, baseline), color.YCbCrModel, 4032, 3024, MaxImageSide, 100_483_268},
		{"a photo of 19 million pixels", "jpeg", jpegHeader(5000, 3750, baseline), color.YCbCrModel, 5000, 3750, MaxImageSide, 138_925_452},
		{"a photo of 20 million pixels", "jpeg", jpegHeader(5472, 3648, baseline), color.YCbCrModel, 5472, 3648, MaxImageSide, 142_080_796},
		{"a photo of 24 million pixels", "jpeg", jpegHeader(6000, 4000, baseline), color.YCbCrModel, 6000, 4000, MaxImageSide, 170_176_727},
		// 3 bytes a pixel, 4 to gather them and 12 of coefficients.
		{"the same photo, progressive", "jpeg", jpegHeader(4624, 3468, progressive), color.YCbCrModel, 4624, 3468, MaxImageSide, 309_469_055},
		{"a progressive photo of 7 million pixels", "jpeg", jpegHeader(3000, 2250, progressive), color.YCbCrModel, 3000, 2250, MaxImageSide, 131_360_509},
		{"a progressive photo of 8 million pixels", "jpeg", jpegHeader(3264, 2448, progressive), color.YCbCrModel, 3264, 2448, MaxImageSide, 155_197_795},
		{"a grey photo", "jpeg", jpegHeader(4000, 3000, baseline), color.GrayModel, 4000, 3000, MaxImageSide, 62_654_569},
		{"a progressive photo in CMYK", "jpeg", jpegHeader(2000, 2000, progressive), color.CMYKModel, 2000, 2000, MaxImageSide, 98_999_064},
		// Kept at its size: 4 bytes a pixel, and 16 for its encoding.
		{"a small photo", "jpeg", jpegHeader(512, 512, baseline), color.YCbCrModel, 512, 512, MaxImageSide, 5_373_700},
		{"50 million pixels at 16 bits a channel", "png", png(0), color.NRGBA64Model, 7071, 7071, MaxImageSide, 521_369_504},
		{"11 million pixels at 16 bits a channel", "png", png(0), color.NRGBA64Model, 3250, 3250, MaxImageSide, 140_889_608},
		{"12 million pixels at 16 bits a channel", "png", png(0), color.NRGBA64Model, 3500, 3500, MaxImageSide, 158_641_608},
		{"14 million pixels at 16 bits a channel", "png", png(0), color.NRGBA64Model, 3700, 3700, MaxImageSide, 173_563_208},
		{"11 million pixels at 8 bits a channel", "png", png(0), color.NRGBAModel, 3250, 3250, MaxImageSide, 97_829_764},
		{"a drawing with a palette", "png", png(0), color.Palette{color.Black}, 4000, 3000, MaxImageSide, 62_654_569},
		{"a grey drawing, counted in colours", "png", png(0), color.GrayModel, 4000, 3000, MaxImageSide, 99_308_452},
		// Half the picture more while its passes are read.
		{"an interlaced strip at 16 bits a channel", "png", png(1), color.RGBA64Model, 6000, 1000, MaxImageSide, 74_615_532},
		{"a lossy WebP picture", "webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), color.YCbCrModel, 4000, 3000, MaxImageSide, 74_872_530},
		// A lossless picture or transparency counts its decoder's prefix
		// codes beside its pixels: one group of 56,568 bytes for each tile
		// of 4 by 4 pixels, 2,600 groups at most.
		{"a lossless WebP picture", "webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8L"), color.NRGBAModel, 4000, 3000, MaxImageSide, 232_602_527},
		{"a lossless WebP of 1,600 by 1,600", "webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8L"), color.NRGBAModel, 1600, 1600, MaxImageSide, 165_697_927},
		{"a lossless WebP of 208 by 200", "webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8L"), color.NRGBAModel, 208, 200, MaxImageSide, 147_463_263},
		{"a lossless WebP of 40 by 40", "webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8L"), color.NRGBAModel, 40, 40, MaxImageSide, 5_692_087},
		{"an extended WebP strip", "webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8X"), color.YCbCrModel, 8000, 600, MaxImageSide, 182_549_727},
		// The scaler keeps weights for each row or column of the picture,
		// however thin the picture is.
		{"a column of 50 million pixels", "png", png(0), color.GrayModel, 1, 50_000_000, MaxImageSide, 11_200_034_808},
		{"a row of 50 million pixels", "png", png(0), color.GrayModel, 50_000_000, 1, MaxImageSide, 9_600_051_192},
		// Uploaded artwork, kept within 3,840 pixels.
		{"an 8K backdrop", "jpeg", jpegHeader(7680, 4320, baseline), color.YCbCrModel, 7680, 4320, 3840, 699_325_444},
		{"an 8K backdrop, progressive", "jpeg", jpegHeader(7680, 4320, progressive), color.YCbCrModel, 7680, 4320, 3840, 699_325_444},
		{"a backdrop of 45 million pixels", "jpeg", jpegHeader(9000, 5000, baseline), color.YCbCrModel, 9000, 5000, 3840, 830_133_212},
		{"a poster of 24 million pixels at 16 bits a channel", "png", png(0), color.NRGBA64Model, 6000, 4000, 3840, 726_327_688},
		{"an 8K backdrop at 16 bits a channel", "png", png(0), color.NRGBA64Model, 7680, 4320, 3840, 833_527_688},
		{"interlaced artwork at 16 bits a channel, kept at its size", "png", png(1), color.NRGBA64Model, 3840, 3840, 3840, 355_806_728},
	} {
		config := image.Config{ColorModel: c.model, Width: c.width, Height: c.height}
		if got := imageMemory(c.data, c.format, config, c.maxSide); got != c.want {
			t.Errorf("%s: counted %d bytes, want %d", c.name, got, c.want)
		}
	}
}

// A WebP picture that may be lossless is counted with the groups of
// prefix codes its decoder builds, which may hold far more than its
// pixels: once every group is built, decoding holds no more than the
// count, for a picture with one group of the largest codes for each of
// its tiles. The count at the decoder's 2,600 groups, kept as a profile
// picture, is in TestTheMemoryAPictureTakesIsCountedFromItsHeader.
//
//nolint:paralleltest // measures the memory the whole test binary holds
func TestALosslessWebPIsCountedWithItsPrefixCodes(t *testing.T) {
	picture, at, err := testpictures.LosslessWebP(128, 128, 1024)
	if err != nil {
		t.Fatal(err)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(picture))
	if err != nil {
		t.Fatal(err)
	}
	counted := imageMemory(picture, format, config, MaxImageSide)
	held, err := testpictures.HeldAt(picture, at, func(r io.Reader) error {
		_, _, err := image.Decode(r)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// The allowance is for what the test binary may hold meanwhile.
	if held > counted+1<<20 {
		t.Errorf("decoding holds %d bytes, counted %d", held, counted)
	}
}

// A JPEG picture is counted with the coefficients a progressive one keeps
// unless its segments, read as the decoder reads them, lead to a frame
// header that says otherwise.
func TestAJPEGCountsAsProgressiveUnlessItsHeaderTellsOtherwise(t *testing.T) {
	t.Parallel()
	encoded := photo(t, 64, 48)
	frame := bytes.Index(encoded, []byte{0xff, 0xc0})
	if frame < 0 {
		t.Fatal("no frame header in the encoded picture")
	}
	insert := func(data []byte, at int, more ...byte) []byte {
		return append(append(append([]byte{}, data[:at]...), more...), data[at:]...)
	}
	relabelled := bytes.Clone(encoded)
	relabelled[frame+1] = 0xc2
	sequential := bytes.Clone(encoded)
	sequential[frame+1] = 0xc1
	for name, c := range map[string]struct {
		data []byte
		want bool
	}{
		"as encoded":                        {encoded, false},
		"an extended sequential frame":      {sequential, false},
		"application data and a comment":    {insert(encoded, 2, 0xff, 0xe1, 0, 6, 'E', 'x', 'i', 'f', 0xff, 0xfe, 0, 4, 'h', 'i'), false},
		"a progressive frame":               {relabelled, true},
		"a fill byte before a marker":       {insert(encoded, 2, 0xff), true},
		"a stray byte between segments":     {insert(encoded, 2, 0x00), true},
		"a restart marker before the frame": {insert(encoded, 2, 0xff, 0xd0), true},
		"a segment the decoder refuses":     {insert(encoded, 2, 0xff, 0xc8, 0, 2), true},
		"cut before its frame header":       {encoded[:frame], true},
		"nothing after its first marker":    {encoded[:2], true},
	} {
		if got := progressiveJPEG(c.data); got != c.want {
			t.Errorf("%s: progressive %v, want %v", name, got, c.want)
		}
	}
	// What the count takes for the decoder's reading is the decoder's.
	for name, data := range map[string][]byte{"as encoded": encoded, "application data and a comment": insert(encoded, 2, 0xff, 0xe1, 0, 6, 'E', 'x', 'i', 'f', 0xff, 0xfe, 0, 4, 'h', 'i')} {
		if config, err := jpeg.DecodeConfig(bytes.NewReader(data)); err != nil || config.Width != 64 || config.Height != 48 {
			t.Errorf("%s: the decoder reads %+v, %v", name, config, err)
		}
	}
}

// A picture is kept or refused by what normalizing it holds, not by its
// pixels alone: the same size is refused at 16 bits a channel and kept at
// 8, and nothing of a refused one is stored.
func TestADeeperPictureIsRefusedWhereTheSameAtEightBitsIsKept(t *testing.T) {
	t.Parallel()
	// 700 by 500 counts 12.2 MB at 16 bits a channel and 10.6 MB at 8.
	const bound = 11 << 20
	if _, err := NormalizeImage(t.Context(), blankPNG(t, 700, 500, 16), MaxImageBytes, MaxImageSide, bound); !errors.Is(err, ErrInvalidImage) {
		t.Errorf("16 bits a channel within %d bytes: %v, want ErrInvalidImage", bound, err)
	}
	kept, err := NormalizeImage(t.Context(), blankPNG(t, 700, 500, 8), MaxImageBytes, MaxImageSide, bound)
	if err != nil {
		t.Fatalf("8 bits a channel within %d bytes: %v", bound, err)
	}
	if config, format, err := image.DecodeConfig(bytes.NewReader(kept.Data)); err != nil || format != "png" || config.Width != MaxImageSide || config.Height != 366 {
		t.Errorf("kept picture: %s %+v %v", format, config, err)
	}
	// With more room, the deeper one is kept too.
	if _, err := NormalizeImage(t.Context(), blankPNG(t, 700, 500, 16), MaxImageBytes, MaxImageSide, 13<<20); err != nil {
		t.Errorf("16 bits a channel within 13 MiB: %v", err)
	}

	// A profile picture of 14 million pixels, a quarter of the pixels a
	// picture may have, is over its bound at 16 bits a channel.
	store := newStore(t)
	user := mustCreate(t, store, NewUser{Name: "member", Password: "correct horse"})
	if _, err := store.SetImage(t.Context(), user.ID, encodedPicture(t, 40, 30, 255)); err != nil {
		t.Fatal(err)
	}
	before, err := store.Image(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetImage(t.Context(), user.ID, blankPNG(t, 3700, 3700, 16)); !errors.Is(err, ErrInvalidImage) {
		t.Errorf("a profile picture of 3700 by 3700 at 16 bits a channel: %v, want ErrInvalidImage", err)
	}
	if after, err := store.Image(t.Context(), user.ID); err != nil || after.Tag != before.Tag {
		t.Errorf("after the refused picture: tag %q, %v, want %q kept", after.Tag, err, before.Tag)
	}
}

// watchDecodes counts the pictures decoded until the test ends.
func watchDecodes(t *testing.T) *int {
	t.Helper()
	decoded := 0
	t.Cleanup(WatchImageDecodesForTests(func() { decoded++ }))
	return &decoded
}

// A picture over its bound is refused from its header: its decoder never
// runs. One within it reaches the decoder.
//
//nolint:paralleltest // watches the pictures the whole test binary decodes
func TestAPictureOverItsMemoryBoundIsNeverDecoded(t *testing.T) {
	decoded := watchDecodes(t)
	for _, c := range []struct {
		name string
		data []byte
		want int
	}{
		{"14 million pixels at 16 bits a channel", blankPNG(t, 3700, 3700, 16), 0},
		{"a photo of 24 million pixels", jpegHeader(6000, 4000, 0xc0), 0},
		{"a progressive photo of 16 million pixels", jpegHeader(4624, 3468, 0xc2), 0},
		// Within the bound, and nothing but a header: its decoder runs,
		// and finds no picture.
		{"a photo of 20 million pixels", jpegHeader(5472, 3648, 0xc0), 1},
	} {
		*decoded = 0
		if _, err := NormalizeImage(t.Context(), c.data, MaxImageBytes, MaxImageSide, MaxImageMemory); !errors.Is(err, ErrInvalidImage) {
			t.Errorf("%s: %v, want ErrInvalidImage", c.name, err)
		}
		if *decoded != c.want {
			t.Errorf("%s: decoded %d times, want %d", c.name, *decoded, c.want)
		}
	}
}

// Ten users send their picture at once: one is decoded at a time, the
// others wait for the turn, and every one is kept.
//
//nolint:paralleltest // replaces the package's picture decoder, which every test decodes with
func TestOnePictureIsNormalizedAtATime(t *testing.T) {
	const uploads = 10
	store := newStore(t)
	users := make([]User, uploads)
	for i := range users {
		users[i] = mustCreate(t, store, NewUser{Name: fmt.Sprintf("member-%d", i), Password: "correct horse"})
	}
	data := encodedPicture(t, 600, 400, 255)
	turns := cap(imageTurns)

	// Each decode is counted from its start to its end. Until the test
	// lets them go, each tells that it started, and is held there.
	var mu sync.Mutex
	running, peak, decoded := 0, 0, 0
	started := make(chan struct{}, uploads)
	release := make(chan struct{})
	decode := decodeImage
	decodeImage = func(r io.Reader) (image.Image, string, error) {
		mu.Lock()
		running++
		decoded++
		peak = max(peak, running)
		mu.Unlock()
		defer func() {
			mu.Lock()
			running--
			mu.Unlock()
		}()
		started <- struct{}{}
		<-release
		return decode(r)
	}
	var wg sync.WaitGroup
	t.Cleanup(func() {
		wg.Wait()
		decodeImage = decode
	})
	for _, user := range users {
		wg.Go(func() {
			if _, err := store.SetImage(t.Context(), user.ID, data); err != nil {
				t.Errorf("%s: %v", user.Name, err)
			}
		})
	}
	for range turns {
		<-started
	}
	// The decodes under way hold every turn, so the other uploads wait
	// for one. Were a turn left, nothing would hold them: the test waits
	// for their decodes to start too, and the count below tells.
	if len(imageTurns) < turns {
		for range uploads - turns {
			<-started
		}
	}
	close(release)
	wg.Wait()

	if peak != turns || decoded != uploads {
		t.Errorf("%d pictures decoded, %d at once, want %d and %d at once", decoded, peak, uploads, turns)
	}
	if running != 0 || len(imageTurns) != 0 {
		t.Errorf("after the uploads: %d decodes under way holding %d turns, want none", running, len(imageTurns))
	}
	for _, user := range users {
		if kept, err := store.Image(t.Context(), user.ID); err != nil || kept.ContentType != "image/jpeg" {
			t.Errorf("%s: kept %q, %v", user.Name, kept.ContentType, err)
		}
	}
}

// awaited is a context that tells when its end is first waited for.
type awaited struct {
	context.Context
	once  sync.Once
	asked chan struct{}
}

func (c *awaited) Done() <-chan struct{} {
	c.once.Do(func() { close(c.asked) })
	return c.Context.Done()
}

// A picture waits for its turn as long as its request lasts: one whose
// request ends meanwhile gives its place up, and is not decoded. Nor is
// one whose request ended before a turn was free.
//
//nolint:paralleltest // replaces the package's picture decoder, which every test decodes with
func TestAPictureWhoseRequestEndsBeforeItsTurnIsNotDecoded(t *testing.T) {
	data := encodedPicture(t, 600, 400, 255)
	normalize := func(ctx context.Context) error {
		_, err := NormalizeImage(ctx, data, MaxImageBytes, MaxImageSide, MaxImageMemory)
		return err
	}
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	decode := decodeImage
	decodeImage = func(r io.Reader) (image.Image, string, error) {
		started <- struct{}{}
		<-release
		return decode(r)
	}
	var wg sync.WaitGroup
	t.Cleanup(func() {
		close(release)
		wg.Wait()
		decodeImage = decode
	})

	// The first picture holds the turn.
	first := make(chan error, 1)
	wg.Go(func() { first <- normalize(t.Context()) })
	<-started
	// The second waits for it, until its request is cancelled.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	waiting := &awaited{Context: ctx, asked: make(chan struct{})}
	second := make(chan error, 1)
	wg.Go(func() { second <- normalize(waiting) })
	select {
	case <-waiting.asked:
	case <-started:
		t.Fatal("a second picture is decoded while the first holds the turn")
	}
	cancel()
	select {
	case err := <-second:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("the picture whose request was cancelled: %v, want context.Canceled", err)
		}
	case <-started:
		t.Fatal("the picture whose request was cancelled is decoded")
	}
	// The first is normalized, and gives the turn back.
	release <- struct{}{}
	if err := <-first; err != nil {
		t.Errorf("the picture holding the turn: %v", err)
	}
	// With the turn free, a request that already ended takes none.
	third := make(chan error, 1)
	wg.Go(func() { third <- normalize(ctx) })
	select {
	case err := <-third:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("a picture whose request had ended: %v, want context.Canceled", err)
		}
	case <-started:
		t.Error("a picture whose request had ended is decoded")
	}
	if len(imageTurns) != 0 {
		t.Errorf("%d turns held after the pictures, want none", len(imageTurns))
	}
}

// The pictures within the bounds are normalized as they were before the
// bounds: the bytes kept for these three are those of the version before.
func TestPicturesAreNormalizedAsTheyWere(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name          string
		data          []byte
		contentType   string
		width, height int
		size          int
		tag           string
	}{
		{"a photo, scaled down", photo(t, 1024, 768), "image/jpeg", 512, 384, 25568, "867feeeb0c339c546f4867fe0c8fc07b"},
		{"a transparent drawing, scaled down", encodedPicture(t, 700, 500, 128), "image/png", 512, 366, 5335, "8424c05c564d856b868c76ce55a13c7d"},
		{"a small drawing, kept at its size", encodedPicture(t, 300, 200, 255), "image/jpeg", 300, 200, 4471, "526ce838ca31240685c67acc2a42edb9"},
	} {
		kept, err := NormalizeImage(t.Context(), c.data, MaxImageBytes, MaxImageSide, MaxImageMemory)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		config, _, err := image.DecodeConfig(bytes.NewReader(kept.Data))
		if err != nil || kept.ContentType != c.contentType || config.Width != c.width || config.Height != c.height ||
			len(kept.Data) != c.size || kept.Tag != c.tag {
			t.Errorf("%s: kept %s of %d by %d, %d bytes, tag %s, %v; want %s of %d by %d, %d bytes, tag %s",
				c.name, kept.ContentType, config.Width, config.Height, len(kept.Data), kept.Tag, err,
				c.contentType, c.width, c.height, c.size, c.tag)
		}
	}
}
