package jellyfin

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"golang.org/x/sync/errgroup"
)

// A panic of a page's worker, run in a goroutine of its own as a home
// row or a library's first page is, is that worker's error, logged with
// its stack; the other workers finish and the server goes on.
func TestAPanicOfAPagesWorkerIsItsError(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	rows := make([]string, 3)
	var group errgroup.Group
	for i := range rows {
		group.Go(func() (err error) {
			defer recoverInto(logger, &err, "A home row ended on a panic", "row", i)
			if i == 1 {
				panic("a broken listing")
			}
			rows[i] = "read"
			return nil
		})
	}
	if err := group.Wait(); !errors.Is(err, errWorkPanicked) || !strings.Contains(err.Error(), "a broken listing") {
		t.Fatalf("the workers: %v, want the panic", err)
	}
	if rows[0] != "read" || rows[2] != "read" {
		t.Errorf("the other rows: %q", rows)
	}
	if !strings.Contains(logs.String(), "row=1") || !strings.Contains(logs.String(), "stack=") {
		t.Fatalf("the panic was not logged with its stack: %s", logs.String())
	}
}
