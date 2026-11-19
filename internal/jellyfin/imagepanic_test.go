package jellyfin

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// A panic of what makes an image is the error of every request waiting
// for it, logged once a run with its stack; the server goes on, and the
// next request makes the image.
func TestAPanicMakingAnImageIsItsWaitersError(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	c := &imageCache{logger: slog.New(slog.NewTextHandler(&logs, nil)), memory: map[string]*list.Element{}, order: list.New()}
	release := make(chan struct{})
	var runs atomic.Int32
	produce := func(context.Context) (artwork, error) {
		runs.Add(1)
		<-release
		panic("a broken picture")
	}
	const waiters = 4
	errs := make([]error, waiters)
	var wg sync.WaitGroup
	for i := range waiters {
		wg.Go(func() {
			_, errs[i] = c.fetch(t.Context(), "poster", produce)
		})
	}
	close(release)
	wg.Wait()
	for i, err := range errs {
		if !errors.Is(err, errImagePanicked) || !strings.Contains(err.Error(), "a broken picture") {
			t.Errorf("waiter %d: %v, want the panic", i, err)
		}
	}
	if got := strings.Count(logs.String(), "ended on a panic"); got != int(runs.Load()) || !strings.Contains(logs.String(), "stack=") {
		t.Fatalf("the panic was logged %d times for %d runs, want once a run with its stack: %s", got, runs.Load(), logs.String())
	}
	image, err := c.fetch(t.Context(), "poster", func(context.Context) (artwork, error) {
		return artwork{contentType: "image/png", body: []byte("png")}, nil
	})
	if err != nil || string(image.body) != "png" {
		t.Fatalf("the image made again: %v", err)
	}
}
