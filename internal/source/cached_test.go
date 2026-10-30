package source

import (
	"bytes"
	"errors"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// What a playback read is served from the cache with no request, and
// what it did not read fails instead of being fetched.
func TestCachedReadsWithoutRequests(t *testing.T) {
	o, server := newOrigin(t, 8*blockSize)
	cache := newCache(t, 1<<30)
	id := accounts.ID{7}
	if _, ok := cache.Cached(id); ok {
		t.Fatal("a source never opened is cached")
	}
	src := cache.Open(id, Location{URL: server.URL + "/movie.mkv"}, nil)
	defer src.Release()
	ctx := t.Context()
	read := make([]byte, blockSize)
	if _, err := src.ReadAt(ctx, read, blockSize/2); err != nil {
		t.Fatal(err)
	}
	requests := o.requests.Load()

	cached, ok := cache.Cached(id)
	if !ok {
		t.Fatal("the source read is not cached")
	}
	if size, known := cached.KnownSize(); !known || size != 8*blockSize {
		t.Errorf("size %d, %v", size, known)
	}
	got, err := cached.Fetch(ctx, blockSize/2+10, 1000)
	if err != nil || !bytes.Equal(got, o.data[blockSize/2+10:blockSize/2+1010]) {
		t.Fatalf("cached bytes: %v", err)
	}
	// Across the blocks read, then into one that is not.
	if _, err := cached.Fetch(ctx, blockSize, blockSize+1); !errors.Is(err, ErrNotCached) {
		t.Errorf("a span past the blocks read: %v", err)
	}
	if _, err := cached.Fetch(ctx, 7*blockSize, 10); !errors.Is(err, ErrNotCached) {
		t.Errorf("a block not read: %v", err)
	}
	if after := o.requests.Load(); after != requests {
		t.Errorf("%d requests to the host", after-requests)
	}
}
