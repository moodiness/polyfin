package library

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// A panic of a read shared by requests, as of an addon's answer decoded,
// is the error of every request waiting for it, logged once with its
// stack; the server goes on, and the next request reads again.
func TestAPanicInASharedReadIsItsWaitersError(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	const waiters = 4
	var joins sync.WaitGroup
	joins.Add(waiters)
	s := &Service{logger: slog.New(slog.NewTextHandler(&logs, nil)), lifetime: context.Background(), joined: func(string) { joins.Done() }}
	release := make(chan struct{})
	read := func(context.Context) (int, error) {
		<-release
		panic("a broken answer")
	}
	errs := make([]error, waiters)
	var wg sync.WaitGroup
	for i := range waiters {
		wg.Go(func() { _, errs[i] = shared(t.Context(), s, "streams", read) })
	}
	// Every request joined the read before it ends.
	joins.Wait()
	close(release)
	wg.Wait()
	for i, err := range errs {
		if !errors.Is(err, errSharedPanicked) || !strings.Contains(err.Error(), "a broken answer") {
			t.Errorf("waiter %d: %v, want the read's panic", i, err)
		}
	}
	if got := strings.Count(logs.String(), "ended on a panic"); got != 1 || !strings.Contains(logs.String(), "stack=") {
		t.Fatalf("the panic was logged %d times, want once with its stack: %s", got, logs.String())
	}
	s.joined = nil
	if got, err := shared(t.Context(), s, "streams", func(context.Context) (int, error) { return 1, nil }); got != 1 || err != nil {
		t.Errorf("the read made again: %v, %v", got, err)
	}
}
