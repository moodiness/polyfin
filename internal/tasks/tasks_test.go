package tasks

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func waitFor(t *testing.T, done func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !done(); {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestTasksRunAtStartOnTheirIntervalAndByHand(t *testing.T) {
	registry := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	var periodic, manual atomic.Int32
	registry.Register(Task{Key: "Sweep", Category: CategoryMaintenance, AtStart: true, Interval: 20 * time.Millisecond,
		Text: map[string]Text{"en": {Name: "Sweep"}, "fr": {Name: "Balayer"}},
		Run: func(ctx context.Context) error {
			if ByHand(ctx) {
				return errors.New("a scheduled run counts as by hand")
			}
			periodic.Add(1)
			return nil
		}})
	registry.Register(Task{Key: "Refresh", Category: CategoryLibrary,
		Text: map[string]Text{"en": {Name: "Refresh"}},
		Run: func(ctx context.Context) error {
			if ByHand(ctx) {
				manual.Add(1)
			}
			return errors.New("addon down")
		}})
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	registry.Start(ctx)
	waitFor(t, func() bool { return periodic.Load() >= 3 })
	if manual.Load() != 0 {
		t.Fatal("a task without schedule ran by itself")
	}
	if err := registry.Run(ID("refresh")); !errors.Is(err, ErrUnknown) {
		t.Errorf("a key is not an identifier: %v", err)
	}
	if err := registry.Run(ID("Refresh")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { info, _ := registry.Task(ID("Refresh"), "fr"); return info.Last != nil })
	info, _ := registry.Task(ID("Refresh"), "fr")
	if manual.Load() != 1 || info.Last.Status != Failed || info.Last.Error != "addon down" || info.Name != "Refresh" || info.Category != "Médiathèque" {
		t.Errorf("failed run: %+v %+v", info, info.Last)
	}
	if sweep, _ := registry.Task(ID("Sweep"), "fr"); sweep.Name != "Balayer" {
		t.Errorf("French name: %q", sweep.Name)
	}
}

func TestATaskRunsOnceAtATimeAndStopsWhenCancelled(t *testing.T) {
	registry := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	var runs atomic.Int32
	registry.Register(Task{Key: "Wait", Text: map[string]Text{"en": {Name: "Wait"}},
		Run: func(ctx context.Context) error { runs.Add(1); <-ctx.Done(); return ctx.Err() }})
	registry.Start(t.Context())
	id := ID("Wait")
	_ = registry.Run(id)
	_ = registry.Run(id)
	waitFor(t, func() bool { info, _ := registry.Task(id, "en"); return info.State == Running })
	_ = registry.Cancel(id)
	waitFor(t, func() bool { info, _ := registry.Task(id, "en"); return info.State == Idle })
	if info, _ := registry.Task(id, "en"); runs.Load() != 1 || info.Last.Status != Cancelled {
		t.Errorf("runs %d, last %+v", runs.Load(), info.Last)
	}
}
