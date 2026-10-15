package source

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A source whose file changed is read anew once discarded; one still read
// is kept.
func TestADiscardedSourceIsReadAnew(t *testing.T) {
	o, server := newOrigin(t, 3*blockSize+100)
	cache := newCache(t, 1<<30)
	id := accounts.ID{7}
	s := cache.Open(id, Location{URL: server.URL + "/file"}, nil)
	readAll(t, s)
	if !cache.Read(id) || cache.Discard(id) {
		t.Fatal("a source still read was discarded")
	}
	s.Release()
	if cache.Read(id) {
		t.Error("a released source is still read")
	}
	changed := make([]byte, len(o.data))
	_, _ = rand.Read(changed)
	o.mu.Lock()
	o.data = changed
	o.mu.Unlock()
	if !cache.Discard(id) {
		t.Fatal("a released source was kept")
	}
	if used, sources, _ := cache.Usage(); used != 0 || sources != 0 {
		t.Errorf("after the discard: %d bytes, %d sources", used, sources)
	}
	s = cache.Open(id, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	if got := readAll(t, s); !bytes.Equal(got, changed) {
		t.Error("the file read is the one it replaced")
	}
}
