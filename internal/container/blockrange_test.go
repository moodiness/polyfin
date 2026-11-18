package container

import (
	"context"
	"errors"
	"testing"
)

// A read of a track's blocks is never passed on with a negative length,
// whatever its Fetcher would answer: it is unreadable, nothing is asked,
// and the bytes counted as fetched for the track stay what they were.
func TestBlockReadsAskNoNegativeLength(t *testing.T) {
	t.Parallel()
	f := &clamped{data: make([]byte, 4096)}
	r := &blockReader{f: f, number: 3}
	const counted = 1 << 20
	r.fetched.Store(counted)
	var err error
	cause := panicOf(func() {
		_, err = r.fetch(context.Background(), 16, -1024)
	})
	if cause != nil {
		t.Errorf("fetching a negative length panicked: %v", cause)
	}
	if fetched := r.fetched.Load(); fetched != counted {
		t.Errorf("%d bytes counted as fetched, want the %d before", fetched, counted)
	}
	if cause == nil && (!errors.Is(err, ErrUnreadable) || len(f.asked) != 0) {
		t.Errorf("got %v after asking lengths %v, want ErrUnreadable and no request", err, f.asked)
	}

	// A length of bytes the file holds is read and counted as before.
	data, err := r.fetch(context.Background(), 16, 1024)
	if err != nil || len(data) != 1024 || r.fetched.Load() != counted+1024 {
		t.Errorf("%d bytes, %v, %d counted", len(data), err, r.fetched.Load())
	}
}
