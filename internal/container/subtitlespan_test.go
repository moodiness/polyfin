package container

import (
	"context"
	"sync"
	"testing"
)

// spanFetcher reads data, recording the ranges read.
type spanFetcher struct {
	data  []byte
	mu    sync.Mutex
	reads []Range
}

func (f *spanFetcher) Fetch(_ context.Context, off int64, n int) ([]byte, error) {
	f.mu.Lock()
	f.reads = append(f.reads, Range{Off: off, N: n})
	f.mu.Unlock()
	return f.data[off:min(off+int64(n), int64(len(f.data)))], nil
}

// A track's reads stay within its span: a cache holding the span serves
// them all, and the requests estimated for a host serving a range per
// request count only the ranges it does not hold.
func TestSubtitleSpansHoldEveryReadOfTheirTrack(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	data := fixture(t, "subtitles.mkv")
	m := openMemory(t, data)
	for _, number := range []uint64{2, 3} {
		start, end, err := m.SubtitleSpan(ctx, number)
		if err != nil || start < 0 || end > int64(len(data)) || start >= end {
			t.Fatalf("track %d: span %d-%d, %v", number, start, end, err)
		}
		f := &spanFetcher{data: data}
		if _, err := m.SubtitleBlocks(ctx, f, number); err != nil {
			t.Fatalf("track %d: %v", number, err)
		}
		for _, read := range f.reads {
			if read.Off < start || read.Off+int64(read.N) > end {
				t.Errorf("track %d: %d bytes read at %d, past its span %d-%d", number, read.N, read.Off, start, end)
			}
		}
		held := func(from, to int64) bool { return from >= start && to <= end }
		if requests, err := m.SingleRangeRequests(ctx, number, held); err != nil || requests != 0 {
			t.Errorf("track %d, its span held: %d requests, %v", number, requests, err)
		}
		// A Cluster's header, the Cluster checked, and the blocks, close
		// together.
		none := func(int64, int64) bool { return false }
		if requests, err := m.SingleRangeRequests(ctx, number, none); err != nil || requests != 3 {
			t.Errorf("track %d, nothing held: %d requests, %v", number, requests, err)
		}
	}
	if _, _, err := m.SubtitleSpan(ctx, 1); err == nil {
		t.Error("a span for a track the Cues do not list")
	}
}
