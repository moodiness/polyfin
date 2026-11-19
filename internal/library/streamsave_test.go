package library

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
)

// A stream list whose save fails is saved again later, the list as it
// stands then, a few times at most.
func TestAStreamListWhoseSaveFailedIsSavedLater(t *testing.T) {
	t.Parallel()
	e, _, addon := smartEnv(t)
	var (
		mu     sync.Mutex
		waits  []time.Duration
		queued []func()
	)
	e.service.saveLater = func(wait time.Duration, save func()) {
		mu.Lock()
		defer mu.Unlock()
		waits, queued = append(waits, wait), append(queued, save)
	}
	next := func() func() {
		t.Helper()
		mu.Lock()
		defer mu.Unlock()
		if len(queued) == 0 {
			t.Fatal("no save again later")
		}
		save := queued[0]
		queued = queued[1:]
		return save
	}
	saved := func() int {
		t.Helper()
		var n int
		if err := e.service.db.QueryRow(t.Context(), "SELECT count(*) FROM stream_lists").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	key := streamKey{addon: addon, contentType: "movie", id: "tt01"}
	e.service.streamLists.Put(key, list[stremio.Stream]{items: []stremio.Stream{{Name: "One", URL: "https://streams.example/one"}}, answer: 1,
		at: time.Now()})
	if _, err := e.service.db.Exec(t.Context(), "ALTER TABLE stream_lists RENAME TO stream_lists_away"); err != nil {
		t.Fatal(err)
	}
	e.service.saveStreams(key)
	next()()
	if _, err := e.service.db.Exec(t.Context(), "ALTER TABLE stream_lists_away RENAME TO stream_lists"); err != nil {
		t.Fatal(err)
	}
	e.service.streamLists.Put(key, list[stremio.Stream]{items: []stremio.Stream{{Name: "One", URL: "https://streams.example/one"},
		{Name: "Two", URL: "https://streams.example/two"}}, answer: 2, at: time.Now()})
	next()()
	if n := saved(); n != 1 {
		t.Fatalf("%d lists saved", n)
	}
	var streams int
	if err := e.service.db.QueryRow(t.Context(), "SELECT jsonb_array_length(streams->'streams') FROM stream_lists").Scan(&streams); err != nil ||
		streams != 2 {
		t.Errorf("saved: %d streams, %v", streams, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(waits, savedRetries[:2]) || len(queued) != 0 {
		t.Errorf("saved again after %v, %d saves left", waits, len(queued))
	}

	// Past the last wait, the list is no longer saved again.
	failing := streamKey{addon: addon, contentType: "movie", id: "tt02"}
	e.service.streamLists.Put(failing, list[stremio.Stream]{items: []stremio.Stream{{Name: "One", URL: "https://streams.example/one"}}, answer: 1,
		at: time.Now()})
	waits, queued = nil, nil
	mu.Unlock()
	if _, err := e.service.db.Exec(t.Context(), "ALTER TABLE stream_lists RENAME TO stream_lists_away"); err != nil {
		t.Fatal(err)
	}
	e.service.saveStreams(failing)
	for range savedRetries {
		next()()
	}
	mu.Lock()
	if !slices.Equal(waits, savedRetries) || len(queued) != 0 {
		t.Errorf("a list failing every time: saved again after %v, %d saves left", waits, len(queued))
	}
}
