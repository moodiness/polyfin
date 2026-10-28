package playback

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// heldStream answers a stream's first bytes at once, then holds its body
// until the test ends, as a live stream sends the rest in real time; size,
// when not negative, is announced.
type heldStream struct {
	head []byte
	size int
	held chan struct{}
}

type heldBody struct {
	head []byte
	held chan struct{}
}

func (b *heldBody) Read(p []byte) (int, error) {
	if len(b.head) > 0 {
		n := copy(p, b.head)
		b.head = b.head[n:]
		return n, nil
	}
	<-b.held
	return 0, io.EOF
}

func (b *heldBody) Close() error { return nil }

func (h heldStream) Open(ctx context.Context, method, target string, _ http.Header, _ bool) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, err
	}
	header, length := http.Header{"Content-Type": {"video/mp2t"}}, int64(-1)
	if h.size >= 0 {
		header.Set("Content-Length", strconv.Itoa(h.size))
		length = int64(h.size)
	}
	return &http.Response{StatusCode: http.StatusOK, Header: header, ContentLength: length, Request: request,
		Body: &heldBody{head: bytes.Clone(h.head), held: h.held}}, nil
}

// A source that announces no size is a stream with no end as soon as it
// answers: its first megabyte, which a live stream sends in real time, is
// not waited for. A playlist that announces its size is told by its first
// bytes.
func TestUnboundedSourcesAreToldAtTheirAnswer(t *testing.T) {
	t.Parallel()
	held := make(chan struct{})
	t.Cleanup(func() { close(held) })
	packet := append([]byte{0x47}, make([]byte, 187)...)
	for _, tc := range []struct {
		name      string
		stream    heldStream
		unbounded bool
	}{
		{"a live MPEG-TS stream", heldStream{head: packet, size: -1, held: held}, true},
		{"a live playlist of a known size", heldStream{head: []byte("#EXTM3U\n#EXTINF:4,\n1.ts\n"), size: 24, held: held}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newService(t, tc.stream, "ffprobe", nil)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			version := library.Version{ID: accounts.ID{1}, URL: "https://93.184.216.34/live/match"}
			if unbounded, err := s.Unbounded(ctx, version); err != nil || unbounded != tc.unbounded {
				t.Errorf("unbounded: %v %v, want %v", unbounded, err, tc.unbounded)
			}
		})
	}
}
