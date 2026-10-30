package source

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"sync"
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

// Closing the cache while a block is being written waits for it: nothing
// is written in the cache's folder once Close returns, which a test's
// temporary folder removed after it would find not empty.
func TestCloseWaitsForTheBlockBeingWritten(t *testing.T) {
	_, server := newOrigin(t, 3*blockSize)
	cache := newCache(t, 1<<30)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	cache.storing = func() {
		once.Do(func() {
			close(entered)
			<-release
		})
	}
	s := cache.Open(accounts.ID{8}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	read := make(chan struct{})
	go func() {
		defer close(read)
		_, _ = s.ReadAt(context.Background(), make([]byte, 10), 0)
	}()
	<-entered
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		_ = cache.Close()
	}()
	close(release)
	<-closed
	<-read
	entries, err := os.ReadDir(cache.dir)
	if err != nil || len(entries) != 0 {
		t.Errorf("the cache's folder after Close: %d entries, %v", len(entries), err)
	}
}
