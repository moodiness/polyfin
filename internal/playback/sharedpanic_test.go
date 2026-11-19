package playback

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"golang.org/x/sync/singleflight"
)

// A panic of work shared by requests (an analysis, keyframes, a track,
// attached files, an index read ahead) is the error of every request
// waiting for it, logged once with its stack; the server goes on, and the
// next request runs the work again.
func TestAPanicInSharedWorkIsItsWaitersError(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	s := &Service{logger: slog.New(slog.NewTextHandler(&logs, nil))}
	release := make(chan struct{})
	work := func() (any, error) {
		<-release
		panic("a broken index")
	}
	// The waiters join the work before it runs on: DoChan returns at once.
	waiters := make([]<-chan singleflight.Result, 4)
	for i := range waiters {
		waiters[i] = s.doShared("track", work)
	}
	close(release)
	for i, results := range waiters {
		result := <-results
		if !errors.Is(result.Err, errSharedPanicked) || !strings.Contains(result.Err.Error(), "a broken index") {
			t.Errorf("waiter %d: %v, want the work's panic", i, result.Err)
		}
	}
	if got := strings.Count(logs.String(), "ended on a panic"); got != 1 || !strings.Contains(logs.String(), "stack=") {
		t.Fatalf("the panic was logged %d times, want once with its stack: %s", got, logs.String())
	}
	// shared, under which analyses, keyframes and tracks run.
	if _, err := s.shared(t.Context(), "analysis", work); !errors.Is(err, errSharedPanicked) {
		t.Errorf("shared: %v, want the work's panic", err)
	}
	if got, err := s.shared(t.Context(), "track", func() (any, error) { return 1, nil }); got != 1 || err != nil {
		t.Errorf("the work run again: %v, %v", got, err)
	}
}
