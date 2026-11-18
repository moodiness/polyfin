package jellyfin

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// blankPNG returns a PNG picture of width by height transparent pixels of
// 16 bits a channel, all of them zero, without ever holding its pixels:
// every row is the same deflated block, written once for each.
func blankPNG(t *testing.T, width, height int) []byte {
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
	header[8], header[9] = 16, 6 // red, green, blue and transparency
	chunk("IHDR", header)
	// A row is its filter's byte, none, then 8 bytes a pixel.
	rowSize := 1 + 8*width
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
// colours, up to its frame header: enough to tell its size, and nothing
// to decode.
func jpegHeader(width, height int) []byte {
	return []byte{
		0xff, 0xd8,
		0xff, 0xe0, 0, 16, 'J', 'F', 'I', 'F', 0, 1, 1, 0, 0, 1, 0, 1, 0, 0,
		0xff, 0xc0, 0, 17, 8, byte(height >> 8), byte(height), byte(width >> 8), byte(width), 3,
		1, 0x22, 0, 2, 0x11, 1, 3, 0x11, 1,
		0xff, 0xd9,
	}
}

// watchDecodes counts the pictures normalized that reach their decoder,
// until the test ends.
func watchDecodes(t *testing.T) *atomic.Int32 {
	t.Helper()
	var decoded atomic.Int32
	t.Cleanup(accounts.WatchImageDecodesForTests(func() { decoded.Add(1) }))
	return &decoded
}

// A member posts a profile picture of 7,071 by 7,071 pixels at 16 bits a
// channel: under the 50 million pixels a picture may have, in half a
// megabyte, and 400 MB once decoded. It is answered as a body that is no
// picture is, without being decoded, and nothing of it is kept.
//
//nolint:paralleltest // watches the pictures the whole test binary decodes
func TestAProfilePictureThatWouldTakeTooMuchMemoryIsRefusedAsAnInvalidOne(t *testing.T) {
	decoded := watchDecodes(t)
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	authorization := app("tv", s.signIn("member", "tv"))
	post := func(data []byte) (int, string) {
		status, _, body := s.send(http.MethodPost, "/UserImage", authorization, "image/png", base64.StdEncoding.EncodeToString(data))
		return status, string(body)
	}
	kept := func() int {
		status, _, _ := s.send(http.MethodGet, "/UserImage?userId="+member.ID.String(), "", "", "")
		return status
	}

	invalid, answer := post([]byte("not a picture"))
	if invalid != http.StatusBadRequest {
		t.Fatalf("a body that is no picture: %d %s", invalid, answer)
	}
	deep := blankPNG(t, 7071, 7071)
	if len(deep) > accounts.MaxImageBytes/4 {
		t.Fatalf("the picture takes %d bytes", len(deep))
	}
	decoded.Store(0)
	if status, body := post(deep); status != invalid || body != answer {
		t.Errorf("the picture: %d %s, want %d %s as for a body that is no picture", status, body, invalid, answer)
	}
	if n := decoded.Load(); n != 0 {
		t.Errorf("the picture was decoded %d times, want none", n)
	}
	if status := kept(); status != http.StatusNotFound {
		t.Errorf("the member's picture after the refusal: %d, want 404", status)
	}

	// An ordinary picture is decoded, and kept.
	decoded.Store(0)
	if status, body := post(picture(t, 1000, 600, 255)); status != http.StatusNoContent {
		t.Fatalf("an ordinary picture: %d %s", status, body)
	}
	if n := decoded.Load(); n != 1 {
		t.Errorf("the ordinary picture was decoded %d times, want once", n)
	}
	if status := kept(); status != http.StatusOK {
		t.Errorf("the member's picture: %d", status)
	}
}

// Artwork uploaded for an item is kept larger than a profile picture, and
// has a larger bound: an 8K backdrop reaches the decoder, which a profile
// picture of that size does not, and one of 45 million pixels does not
// either. The pictures here are a header with nothing after it, so each is
// refused as no picture: the decodes tell where.
//
//nolint:paralleltest // watches the pictures the whole test binary decodes
func TestUploadedArtworkHasALargerMemoryBoundThanAProfilePicture(t *testing.T) {
	decoded := watchDecodes(t)
	e := newEdited(t)
	post := func(path string, data []byte) int {
		status, _, _ := e.send(http.MethodPost, path, app("dashboard", e.admin), "image/jpeg", base64.StdEncoding.EncodeToString(data))
		return status
	}
	backdrop := "/Items/" + e.ids["Allowed"] + "/Images/Backdrop"
	for _, c := range []struct {
		name string
		path string
		data []byte
		want int32
	}{
		{"an 8K backdrop", backdrop, jpegHeader(7680, 4320), 1},
		{"a backdrop of 45 million pixels", backdrop, jpegHeader(9000, 5000), 0},
		{"a profile picture of 8K", "/UserImage", jpegHeader(7680, 4320), 0},
	} {
		decoded.Store(0)
		if status := post(c.path, c.data); status != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", c.name, status)
		}
		if n := decoded.Load(); n != c.want {
			t.Errorf("%s: decoded %d times, want %d", c.name, n, c.want)
		}
	}
}
