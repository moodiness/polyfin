package source

import (
	"bytes"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/bandwidth"
)

// The bytes a source receives are counted under its identifier, once:
// reading its cached blocks again receives nothing, and another source
// counts apart.
func TestSourcesCountTheBytesTheyReceive(t *testing.T) {
	o, server := newOrigin(t, 5*blockSize+12345)
	cache := newCache(t, 1<<30)
	received := bandwidth.New()
	cache.Count(received)
	s := cache.Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	if got := readAll(t, s); !bytes.Equal(got, o.data) {
		t.Fatal("content differs")
	}
	id := accounts.ID{1}.String()
	if got := received.Total(id); got != int64(len(o.data)) {
		t.Fatalf("received %d bytes, not the file's %d", got, len(o.data))
	}
	readAll(t, s)
	if got := received.Total(id); got != int64(len(o.data)) {
		t.Errorf("read again from the cache: %d bytes received", got)
	}
	other := cache.Open(accounts.ID{2}, Location{URL: server.URL + "/other"}, nil)
	defer other.Release()
	head := make([]byte, 1000)
	if _, err := other.ReadAt(t.Context(), head, 0); err != nil {
		t.Fatal(err)
	}
	if got := received.Total(accounts.ID{2}.String()); got == 0 || got > blockSize*int64(cache.readahead+1) {
		t.Errorf("the other source received %d bytes", got)
	}
	if got := received.Total(id); got != int64(len(o.data)) {
		t.Errorf("the first source's count moved with the other's: %d", got)
	}
}
