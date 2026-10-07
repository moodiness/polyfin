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

func TestTasksTellWhenTheirScheduleRunsThemNext(t *testing.T) {
	registry := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	registry.Register(Task{Key: "Hourly", Interval: time.Hour, Text: map[string]Text{"en": {Name: "Hourly"}},
		Run: func(context.Context) error { return nil }})
	registry.Register(Task{Key: "ByHand", Text: map[string]Text{"en": {Name: "By hand"}},
		Run: func(context.Context) error { return nil }})
	if info, _ := registry.Task(ID("Hourly"), "en"); info.Next != nil {
		t.Errorf("next run before the registry starts: %v", info.Next)
	}
	before := time.Now()
	registry.Start(t.Context())
	hourly, _ := registry.Task(ID("Hourly"), "en")
	if hourly.Next == nil || hourly.Next.Before(before.Add(time.Hour)) || hourly.Next.After(time.Now().Add(time.Hour)) {
		t.Errorf("next run of an hourly task: %v", hourly.Next)
	}
	if manual, _ := registry.Task(ID("ByHand"), "en"); manual.Next != nil {
		t.Errorf("next run of a task without schedule: %v", manual.Next)
	}
}

// A daily task runs when the clock reaches its hour, and tells the next
// day's run; an hour changed to one already past today runs tomorrow,
// without running now.
func TestADailyTaskRunsAtItsHour(t *testing.T) {
	zone := time.FixedZone("Server", 2*60*60)
	// The registry's clock is 50 ms before 04:00 when the test starts.
	start, fake := time.Now(), time.Date(2026, 10, 5, 3, 59, 59, 950_000_000, zone)
	registry := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	registry.now = func() time.Time { return fake.Add(time.Since(start)) }
	registry.dailyCheck = 10 * time.Millisecond
	var hour atomic.Int32
	hour.Store(4)
	var runs atomic.Int32
	var ranAt atomic.Value
	registry.Register(Task{Key: "Backup", Daily: func() int { return int(hour.Load()) }, Text: map[string]Text{"en": {Name: "Back up"}},
		Run: func(context.Context) error {
			ranAt.Store(registry.now())
			runs.Add(1)
			return nil
		}})
	registry.Start(t.Context())
	if info, _ := registry.Task(ID("Backup"), "en"); info.Daily == nil || *info.Daily != 4 || info.Interval != 0 || info.Next == nil ||
		!info.Next.Equal(time.Date(2026, 10, 5, 4, 0, 0, 0, zone)) {
		t.Fatalf("before the hour: %+v", info)
	}
	if runs.Load() != 0 {
		t.Fatal("the task ran before its hour")
	}
	waitFor(t, func() bool { return runs.Load() == 1 })
	if at := ranAt.Load().(time.Time); at.Hour() != 4 || at.Minute() != 0 {
		t.Errorf("ran at %v", at)
	}
	waitFor(t, func() bool {
		info, _ := registry.Task(ID("Backup"), "en")
		return info.Next != nil && info.Next.Equal(time.Date(2026, 10, 6, 4, 0, 0, 0, zone))
	})

	hour.Store(5)
	waitFor(t, func() bool {
		info, _ := registry.Task(ID("Backup"), "en")
		return info.Next != nil && info.Next.Equal(time.Date(2026, 10, 5, 5, 0, 0, 0, zone)) && *info.Daily == 5
	})
	hour.Store(3)
	waitFor(t, func() bool {
		info, _ := registry.Task(ID("Backup"), "en")
		return info.Next != nil && info.Next.Equal(time.Date(2026, 10, 6, 3, 0, 0, 0, zone))
	})
	time.Sleep(5 * registry.dailyCheck)
	if runs.Load() != 1 {
		t.Errorf("runs after the hour changed: %d", runs.Load())
	}
}

// A daily task whose hour is negative runs by hand only, and tells no hour
// nor next run; once given an hour, it runs at it.
func TestADailyTaskWithoutAnHourRunsByHandOnly(t *testing.T) {
	zone := time.FixedZone("Server", 2*60*60)
	// The registry's clock is 50 ms before 23:00, the hour -1 would be
	// taken for, a day's -1st hour.
	start, fake := time.Now(), time.Date(2026, 10, 5, 22, 59, 59, 950_000_000, zone)
	registry := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	registry.now = func() time.Time { return fake.Add(time.Since(start)) }
	registry.dailyCheck = 10 * time.Millisecond
	var hour atomic.Int32
	hour.Store(-1)
	var runs atomic.Int32
	registry.Register(Task{Key: "Read", Daily: func() int { return int(hour.Load()) }, Text: map[string]Text{"en": {Name: "Read"}},
		Run: func(context.Context) error {
			runs.Add(1)
			return nil
		}})
	registry.Start(t.Context())
	if info, _ := registry.Task(ID("Read"), "en"); info.Daily != nil || info.Next != nil {
		t.Errorf("without an hour: %+v", info)
	}
	time.Sleep(10 * registry.dailyCheck)
	if runs.Load() != 0 {
		t.Errorf("ran %d times without an hour", runs.Load())
	}
	if err := registry.Run(ID("Read")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return runs.Load() == 1 })
	// Given an hour, it tells it and when it runs next.
	hour.Store(5)
	waitFor(t, func() bool {
		info, _ := registry.Task(ID("Read"), "en")
		return info.Daily != nil && *info.Daily == 5 && info.Next != nil && info.Next.Equal(time.Date(2026, 10, 6, 5, 0, 0, 0, zone))
	})
}

func TestNextDaily(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		after time.Time
		hour  int
		want  time.Time
	}{
		{time.Date(2026, 10, 5, 3, 0, 0, 0, paris), 4, time.Date(2026, 10, 5, 4, 0, 0, 0, paris)},
		{time.Date(2026, 10, 5, 4, 0, 0, 0, paris), 4, time.Date(2026, 10, 6, 4, 0, 0, 0, paris)},
		{time.Date(2026, 10, 5, 23, 30, 0, 0, paris), 0, time.Date(2026, 10, 6, 0, 0, 0, 0, paris)},
		{time.Date(2026, 12, 31, 23, 0, 0, 0, paris), 4, time.Date(2027, 1, 1, 4, 0, 0, 0, paris)},
		// 02:00 does not exist on the day summer time starts.
		{time.Date(2026, 3, 28, 12, 0, 0, 0, paris), 2, time.Date(2026, 3, 29, 3, 0, 0, 0, paris)},
	} {
		if got := NextDaily(test.after, test.hour); !got.Equal(test.want) {
			t.Errorf("after %v at %d: %v, want %v", test.after, test.hour, got, test.want)
		}
	}
}

func TestATaskCannotHaveAnIntervalAndADailyHour(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	New(slog.New(slog.NewTextHandler(io.Discard, nil))).Register(Task{Key: "Both", Interval: time.Hour, Daily: func() int { return 4 },
		Text: map[string]Text{"en": {Name: "Both"}}, Run: func(context.Context) error { return nil }})
}
