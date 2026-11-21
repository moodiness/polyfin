package source

import (
	"bytes"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// asked is what the origin was asked, in order.
func (o *origin) asked() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.ranges)
}

// Reads that are no stream ask for bounded ranges: a read going on in
// order asks twice as much each time, a read elsewhere, as of a file's
// index at its end, one block. A host then sends nothing past them.
func TestReadsOtherThanAStreamAskBoundedRanges(t *testing.T) {
	t.Parallel()
	o, server := newOrigin(t, 5*blockSize+12345)
	cache := newCache(t, 1<<30)
	s := cache.Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	if got := readAll(t, s); !bytes.Equal(got, o.data) {
		t.Fatal("content differs")
	}
	want := []string{"bytes=0-1048575", "bytes=1048576-2097151", "bytes=2097152-4194303", "bytes=4194304-8388607"}
	if got := o.asked(); !slices.Equal(got, want) {
		t.Errorf("read in order: asked %q, want %q", got, want)
	}

	far, server := newOrigin(t, 48*blockSize)
	s = cache.Open(accounts.ID{2}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	got := make([]byte, 2000)
	off := int64(40*blockSize + 100)
	if _, err := s.ReadAt(t.Context(), got, off); err != nil || !bytes.Equal(got, far.data[off:off+2000]) {
		t.Fatalf("far read: %v", err)
	}
	if got, want := far.asked(), []string{"bytes=41943040-42991615"}; !slices.Equal(got, want) {
		t.Errorf("read far: asked %q, want %q", got, want)
	}
}

// A warm asks for the blocks of its spans, up to their end.
func TestAWarmAsksItsSpansAlone(t *testing.T) {
	t.Parallel()
	o, server := newOrigin(t, 10*blockSize)
	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	if err := s.Warm(t.Context(), Span{Off: 3*blockSize + 10, End: 5*blockSize + 20}); err != nil {
		t.Fatal(err)
	}
	if got, want := o.asked(), []string{"bytes=3145728-6291455"}; !slices.Equal(got, want) {
		t.Errorf("asked %q, want %q", got, want)
	}
}

// A stream, as FFmpeg's or a player's, asks open-ended: it reads on, and
// its window ahead with it.
func TestAStreamAsksOpenEnded(t *testing.T) {
	t.Parallel()
	o, server := newOrigin(t, 5*blockSize+12345)
	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	if got, _ := readToEnd(t, s); !bytes.Equal(got, o.data) {
		t.Fatal("content differs")
	}
	if got, want := o.asked(), []string{"bytes=0-"}; !slices.Equal(got, want) {
		t.Errorf("asked %q, want %q", got, want)
	}
}
