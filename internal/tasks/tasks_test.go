package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"sync"
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

// clock is a registry's clock the test moves; the registry looks at it
// every few milliseconds.
type clock struct{ at atomic.Pointer[time.Time] }

func newClock(registry *Registry, at time.Time) *clock {
	c := &clock{}
	c.at.Store(&at)
	registry.now = func() time.Time { return *c.at.Load() }
	registry.check = 5 * time.Millisecond
	return c
}

func (c *clock) set(at time.Time)         { c.at.Store(&at) }
func (c *clock) advance(by time.Duration) { c.set(c.at.Load().Add(by)) }

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// memoryStore is a Store in memory, failing its saves when told.
type memoryStore struct {
	mu       sync.Mutex
	triggers map[string][]Trigger
	runs     map[string]Result
	fail     bool
}

func newMemoryStore() *memoryStore {
	return &memoryStore{triggers: map[string][]Trigger{}, runs: map[string]Result{}}
}

func (m *memoryStore) LoadTriggers(context.Context) (map[string][]Trigger, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return maps.Clone(m.triggers), nil
}

func (m *memoryStore) SaveTriggers(_ context.Context, key string, triggers []Trigger) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("database down")
	}
	m.triggers[key] = triggers
	return nil
}

func (m *memoryStore) DeleteTriggers(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.triggers, key)
	return nil
}

func (m *memoryStore) LoadRuns(context.Context) (map[string]Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return maps.Clone(m.runs), nil
}

func (m *memoryStore) SaveRun(_ context.Context, key string, result Result) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[key] = result
	return nil
}

func (m *memoryStore) run(key string) (Result, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[key]
	return run, ok
}

func TestTasksRunAtStartOnTheirIntervalAndByHand(t *testing.T) {
	t.Parallel()
	registry := New(quiet())
	c := newClock(registry, time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC))
	var periodic, manual atomic.Int32
	registry.Register(Task{Key: "Sweep", Category: CategoryMaintenance, Triggers: []Trigger{AtStartup(), Every(time.Minute)},
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
	waitFor(t, func() bool { return periodic.Load() == 1 })
	for want := int32(2); want <= 3; want++ {
		waitFor(t, func() bool {
			info, _ := registry.Task(ID("Sweep"), "en")
			return info.State == Idle && info.Last != nil
		})
		c.advance(2 * time.Minute)
		waitFor(t, func() bool { return periodic.Load() == want })
	}
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
	t.Parallel()
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

// An interval task first runs its stagger after its interval, so that
// tasks of one interval do not run in the same second.
func TestTasksTellWhenTheirScheduleRunsThemNext(t *testing.T) {
	t.Parallel()
	registry := New(quiet())
	start := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	newClock(registry, start)
	for _, key := range []string{"Hourly", "AlsoHourly"} {
		registry.Register(Task{Key: key, Triggers: []Trigger{Every(time.Hour)}, Text: map[string]Text{"en": {Name: key}},
			Run: func(context.Context) error { return nil }})
	}
	registry.Register(Task{Key: "ByHand", Text: map[string]Text{"en": {Name: "By hand"}},
		Run: func(context.Context) error { return nil }})
	if info, _ := registry.Task(ID("Hourly"), "en"); info.Next != nil {
		t.Errorf("next run before the registry starts: %v", info.Next)
	}
	registry.Start(t.Context())
	for _, key := range []string{"Hourly", "AlsoHourly"} {
		want := start.Add(time.Hour + stagger(key, time.Hour))
		if info, _ := registry.Task(ID(key), "en"); info.Next == nil || !info.Next.Equal(want) {
			t.Errorf("next run of %s: %v, want %v", key, info.Next, want)
		}
	}
	if d := stagger("Hourly", time.Hour) - stagger("AlsoHourly", time.Hour); d > -time.Second && d < time.Second {
		t.Error("two hourly tasks run in the same second")
	}
	if manual, _ := registry.Task(ID("ByHand"), "en"); manual.Next != nil || len(manual.Triggers) != 0 || manual.Triggers == nil {
		t.Errorf("a task without schedule: %+v", manual)
	}
}

// A task's stagger depends on its key alone, the same in every run of the
// server, and stays under 5 minutes and under half its interval.
func TestStaggersAreStableAndBounded(t *testing.T) {
	t.Parallel()
	if got, want := stagger("WarmLibraries", time.Hour), time.Duration(267227399570); got != want {
		t.Errorf("stagger of WarmLibraries: %v, want %v", got, want)
	}
	if got, want := stagger("RefreshLiveTvGuides", 5*time.Minute), time.Duration(142099951770); got != want {
		t.Errorf("stagger of RefreshLiveTvGuides: %v, want %v", got, want)
	}
	for i := range 1000 {
		key := "Task" + strconv.Itoa(i)
		for _, interval := range []time.Duration{time.Millisecond, time.Second, 5 * time.Minute, 10 * time.Minute, time.Hour, 24 * time.Hour} {
			if s := stagger(key, interval); s < 0 || s >= min(5*time.Minute, interval/2) {
				t.Fatalf("stagger of %s every %v: %v", key, interval, s)
			}
		}
	}
	if s := stagger("Never", 0); s != 0 {
		t.Errorf("stagger without interval: %v", s)
	}
}

// A daily trigger runs its task when the clock reaches its minute, and
// tells the next day's run.
func TestADailyTriggerRunsAtItsMinute(t *testing.T) {
	t.Parallel()
	zone := time.FixedZone("Server", 2*60*60)
	registry := New(quiet())
	c := newClock(registry, time.Date(2026, 10, 5, 4, 29, 59, 950_000_000, zone))
	var runs atomic.Int32
	var ranAt atomic.Value
	registry.Register(Task{Key: "Backup", Triggers: []Trigger{Daily(4*time.Hour + 30*time.Minute)}, Text: map[string]Text{"en": {Name: "Back up"}},
		Run: func(context.Context) error {
			ranAt.Store(registry.now())
			runs.Add(1)
			return nil
		}})
	registry.Start(t.Context())
	if info, _ := registry.Task(ID("Backup"), "en"); info.Next == nil || !info.Next.Equal(time.Date(2026, 10, 5, 4, 30, 0, 0, zone)) {
		t.Fatalf("before the minute: %+v", info)
	}
	time.Sleep(5 * registry.check)
	if runs.Load() != 0 {
		t.Fatal("the task ran before its minute")
	}
	c.set(time.Date(2026, 10, 5, 4, 30, 0, 0, zone))
	waitFor(t, func() bool { return runs.Load() == 1 })
	if at := ranAt.Load().(time.Time); at.Hour() != 4 || at.Minute() != 30 {
		t.Errorf("ran at %v", at)
	}
	waitFor(t, func() bool {
		info, _ := registry.Task(ID("Backup"), "en")
		return info.Next != nil && info.Next.Equal(time.Date(2026, 10, 6, 4, 30, 0, 0, zone))
	})
	time.Sleep(5 * registry.check)
	if runs.Load() != 1 {
		t.Errorf("runs: %d", runs.Load())
	}
}

// A weekly trigger runs its task on its day only.
func TestAWeeklyTriggerRunsOnItsDay(t *testing.T) {
	t.Parallel()
	registry := New(quiet())
	// Monday 5 October 2026.
	c := newClock(registry, time.Date(2026, 10, 5, 2, 59, 0, 0, time.UTC))
	var runs atomic.Int32
	registry.Register(Task{Key: "Weekly", Triggers: []Trigger{Weekly(time.Wednesday, 3*time.Hour)}, Text: map[string]Text{"en": {Name: "Weekly"}},
		Run: func(context.Context) error { runs.Add(1); return nil }})
	registry.Start(t.Context())
	c.set(time.Date(2026, 10, 5, 3, 0, 0, 0, time.UTC))
	time.Sleep(5 * registry.check)
	if info, _ := registry.Task(ID("Weekly"), "en"); runs.Load() != 0 || info.Next == nil || !info.Next.Equal(time.Date(2026, 10, 7, 3, 0, 0, 0, time.UTC)) {
		t.Fatalf("on Monday: %d runs, %+v", runs.Load(), info)
	}
	c.set(time.Date(2026, 10, 7, 3, 0, 0, 0, time.UTC))
	waitFor(t, func() bool { return runs.Load() == 1 })
	waitFor(t, func() bool {
		info, _ := registry.Task(ID("Weekly"), "en")
		return info.Next != nil && info.Next.Equal(time.Date(2026, 10, 14, 3, 0, 0, 0, time.UTC))
	})
}

func TestNextAtAndNextWeekly(t *testing.T) {
	t.Parallel()
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		after time.Time
		at    time.Duration
		want  time.Time
	}{
		{time.Date(2026, 10, 5, 3, 0, 0, 0, paris), 4 * time.Hour, time.Date(2026, 10, 5, 4, 0, 0, 0, paris)},
		{time.Date(2026, 10, 5, 4, 0, 0, 0, paris), 4 * time.Hour, time.Date(2026, 10, 6, 4, 0, 0, 0, paris)},
		{time.Date(2026, 10, 5, 4, 14, 0, 0, paris), 4*time.Hour + 15*time.Minute, time.Date(2026, 10, 5, 4, 15, 0, 0, paris)},
		{time.Date(2026, 10, 5, 23, 30, 0, 0, paris), 0, time.Date(2026, 10, 6, 0, 0, 0, 0, paris)},
		{time.Date(2026, 12, 31, 23, 0, 0, 0, paris), 4 * time.Hour, time.Date(2027, 1, 1, 4, 0, 0, 0, paris)},
		// 02:30 does not exist on the day summer time starts.
		{time.Date(2026, 3, 28, 12, 0, 0, 0, paris), 2*time.Hour + 30*time.Minute, time.Date(2026, 3, 29, 3, 30, 0, 0, paris)},
		// The day winter time starts has 25 hours: 04:00 is 5 hours after
		// midnight.
		{time.Date(2026, 10, 25, 0, 0, 0, 0, paris), 4 * time.Hour, time.Date(2026, 10, 25, 4, 0, 0, 0, paris)},
	} {
		if got := NextAt(test.after, test.at); !got.Equal(test.want) {
			t.Errorf("after %v at %v: %v, want %v", test.after, test.at, got, test.want)
		}
	}
	for _, test := range []struct {
		after time.Time
		day   time.Weekday
		want  time.Time
	}{
		// Monday 5 October 2026.
		{time.Date(2026, 10, 5, 3, 0, 0, 0, paris), time.Monday, time.Date(2026, 10, 5, 4, 0, 0, 0, paris)},
		{time.Date(2026, 10, 5, 4, 0, 0, 0, paris), time.Monday, time.Date(2026, 10, 12, 4, 0, 0, 0, paris)},
		{time.Date(2026, 10, 5, 4, 0, 0, 0, paris), time.Sunday, time.Date(2026, 10, 11, 4, 0, 0, 0, paris)},
		{time.Date(2026, 12, 30, 12, 0, 0, 0, paris), time.Friday, time.Date(2027, 1, 1, 4, 0, 0, 0, paris)},
	} {
		if got := NextWeekly(test.after, test.day, 4*time.Hour); !got.Equal(test.want) {
			t.Errorf("after %v on %v: %v, want %v", test.after, test.day, got, test.want)
		}
	}
}

// An interval counts from the last run's start, kept across a restart: a
// run due later runs then, and one overdue runs its stagger after the
// start.
func TestAnIntervalCountsFromTheStoredLastRun(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		interval  time.Duration
		lastStart time.Time
		want      time.Time
	}{
		{time.Hour, now.Add(-50 * time.Minute), now.Add(10 * time.Minute)},
		{7 * 24 * time.Hour, now.Add(-8 * 24 * time.Hour), now.Add(stagger("Weekly", 7*24*time.Hour))},
	} {
		store := newMemoryStore()
		store.runs["Weekly"] = Result{Start: test.lastStart, End: test.lastStart.Add(time.Minute), Status: Completed}
		registry := New(quiet())
		newClock(registry, now)
		interval := test.interval
		registry.Register(Task{Key: "Weekly", Triggers: []Trigger{Every(interval)}, Text: map[string]Text{"en": {Name: "Weekly"}},
			Run: func(context.Context) error { return nil }})
		if err := registry.Load(t.Context(), store); err != nil {
			t.Fatal(err)
		}
		registry.Start(t.Context())
		info, _ := registry.Task(ID("Weekly"), "en")
		if info.Next == nil || !info.Next.Equal(test.want) || info.Last == nil || info.Last.Status != Completed {
			t.Errorf("last started %v: %+v, want next %v", test.lastStart, info, test.want)
		}
	}
}

// A run ends with its time limit, and is told cancelled; its result is
// saved for the next start.
func TestATriggerTimeLimitCancelsTheRun(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	registry := New(quiet())
	registry.Register(Task{Key: "Long", Text: map[string]Text{"en": {Name: "Long"}},
		Run: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }})
	if err := registry.Load(t.Context(), store); err != nil {
		t.Fatal(err)
	}
	registry.Start(t.Context())
	registry.mu.Lock()
	registry.start(registry.find(ID("Long")), false, 20*time.Millisecond)
	registry.mu.Unlock()
	waitFor(t, func() bool { _, ok := store.run("Long"); return ok })
	if run, _ := store.run("Long"); run.Status != Cancelled || run.Error != errTimeLimit.Error() {
		t.Errorf("saved run: %+v", run)
	}
	restarted := New(quiet())
	restarted.Register(Task{Key: "Long", Text: map[string]Text{"en": {Name: "Long"}}, Run: func(context.Context) error { return nil }})
	if err := restarted.Load(t.Context(), store); err != nil {
		t.Fatal(err)
	}
	if info, _ := restarted.Task(ID("Long"), "en"); info.Last == nil || info.Last.Status != Cancelled {
		t.Errorf("after a restart: %+v", info.Last)
	}
}

// A disabled task runs by hand only, and tells no next run.
func TestADisabledTaskRunsByHandOnly(t *testing.T) {
	t.Parallel()
	registry := New(quiet())
	newClock(registry, time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC))
	var runs atomic.Int32
	registry.Register(Task{Key: "Off", Triggers: []Trigger{AtStartup(), Daily(0)}, Enabled: func() bool { return false },
		Text: map[string]Text{"en": {Name: "Off"}}, Run: func(context.Context) error { runs.Add(1); return nil }})
	registry.Start(t.Context())
	if info, _ := registry.Task(ID("Off"), "en"); info.Next != nil || runs.Load() != 0 {
		t.Errorf("disabled: %d runs, %+v", runs.Load(), info)
	}
	_ = registry.Run(ID("Off"))
	waitFor(t, func() bool { return runs.Load() == 1 })
}

func TestLimitsCheckTriggers(t *testing.T) {
	t.Parallel()
	limits := Limits{Types: []TriggerType{DailyTrigger, WeeklyTrigger, IntervalTrigger}, MinInterval: 6 * time.Hour}
	for _, test := range []struct {
		name     string
		limits   Limits
		triggers []Trigger
		ok       bool
	}{
		{"none", limits, nil, true},
		{"none kept", Limits{KeepOne: true}, nil, false},
		{"daily to the minute", limits, []Trigger{Daily(23*time.Hour + 59*time.Minute)}, true},
		{"daily at 24:00", limits, []Trigger{Daily(24 * time.Hour)}, false},
		{"daily to the second", limits, []Trigger{Daily(time.Hour + time.Second)}, false},
		{"daily without time", limits, []Trigger{{Type: DailyTrigger, TimeOfDay: -1}}, false},
		{"weekly", limits, []Trigger{Weekly(time.Saturday, 0)}, true},
		{"weekly without day", limits, []Trigger{{Type: WeeklyTrigger, Day: -1}}, false},
		{"weekly day 7", limits, []Trigger{{Type: WeeklyTrigger, Day: 7}}, false},
		{"interval at its minimum", limits, []Trigger{Every(6 * time.Hour)}, true},
		{"interval under its minimum", limits, []Trigger{Every(5 * time.Hour)}, false},
		{"interval of 28 days", limits, []Trigger{Every(MaxInterval)}, true},
		{"interval over 28 days", limits, []Trigger{Every(MaxInterval + time.Minute)}, false},
		{"startup refused", limits, []Trigger{AtStartup()}, false},
		{"startup allowed", Limits{}, []Trigger{AtStartup()}, true},
		{"unknown", Limits{}, []Trigger{{Type: "monthly"}}, false},
		{"time limit", limits, []Trigger{{Type: DailyTrigger, MaxRuntime: 90 * time.Minute}}, true},
		{"time limit too short", limits, []Trigger{{Type: DailyTrigger, MaxRuntime: time.Second}}, false},
		{"time limit too long", limits, []Trigger{{Type: DailyTrigger, MaxRuntime: MaxMaxRuntime + time.Minute}}, false},
		{"eight", limits, slices.Repeat([]Trigger{Daily(0)}, 8), true},
		{"nine", limits, slices.Repeat([]Trigger{Daily(0)}, 9), false},
	} {
		if err := test.limits.Check(test.triggers); (err == nil) != test.ok || err != nil && !errors.Is(err, ErrInvalidTriggers) {
			t.Errorf("%s: %v", test.name, err)
		}
	}
}

// New triggers are saved, scheduled at once and kept across restarts;
// reset, the defaults come back. A locked task takes its own triggers
// only, and refused triggers save nothing.
func TestSetAndResetTriggers(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	start := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	register := func() *Registry {
		registry := New(quiet())
		newClock(registry, start)
		registry.Register(Task{Key: "Read", Triggers: []Trigger{}, Limits: Limits{Types: []TriggerType{DailyTrigger, IntervalTrigger}, MinInterval: 12 * time.Hour},
			Text: map[string]Text{"en": {Name: "Read"}}, Run: func(context.Context) error { return nil }})
		registry.Register(Task{Key: "Poll", Triggers: []Trigger{AtStartup(), Every(5 * time.Minute)}, Limits: Limits{Locked: LockedSettings},
			Text: map[string]Text{"en": {Name: "Poll"}}, Run: func(context.Context) error { return nil }})
		if err := registry.Load(t.Context(), store); err != nil {
			t.Fatal(err)
		}
		registry.Start(t.Context())
		return registry
	}
	registry := register()
	read := ID("Read")
	if info, _ := registry.Task(read, "en"); info.Next != nil || info.Custom {
		t.Fatalf("by default: %+v", info)
	}
	for _, refused := range [][]Trigger{{Every(time.Hour)}, {AtStartup()}} {
		if err := registry.SetTriggers(t.Context(), read, refused); !errors.Is(err, ErrInvalidTriggers) {
			t.Errorf("%v: %v", refused, err)
		}
	}
	if len(store.triggers) != 0 {
		t.Fatalf("refused triggers saved: %v", store.triggers)
	}
	daily := []Trigger{Daily(3*time.Hour + 15*time.Minute)}
	if err := registry.SetTriggers(t.Context(), read, daily); err != nil {
		t.Fatal(err)
	}
	if info, _ := registry.Task(read, "en"); !info.Custom || !slices.Equal(info.Triggers, daily) || info.Next == nil ||
		!info.Next.Equal(time.Date(2026, 10, 6, 3, 15, 0, 0, time.UTC)) {
		t.Errorf("after setting: %+v", info)
	}
	if info, _ := register().Task(read, "en"); !info.Custom || !slices.Equal(info.Triggers, daily) {
		t.Errorf("after a restart: %+v", info)
	}
	if err := registry.SetTriggers(t.Context(), read, nil); err != nil {
		t.Fatal(err)
	}
	if info, _ := registry.Task(read, "en"); !info.Custom || len(info.Triggers) != 0 || info.Next != nil {
		t.Errorf("without triggers: %+v", info)
	}
	if err := registry.ResetTriggers(t.Context(), read); err != nil {
		t.Fatal(err)
	}
	if _, saved := store.triggers["Read"]; saved {
		t.Error("the reset left the triggers saved")
	}
	poll := ID("Poll")
	if err := registry.SetTriggers(t.Context(), poll, []Trigger{AtStartup(), Every(5 * time.Minute)}); err != nil {
		t.Errorf("a locked task's own triggers: %v", err)
	}
	if err := registry.SetTriggers(t.Context(), poll, []Trigger{Every(time.Hour)}); !errors.Is(err, ErrLocked) {
		t.Errorf("a locked task's new triggers: %v", err)
	}
	if err := registry.SetTriggers(t.Context(), ID("Missing"), nil); !errors.Is(err, ErrUnknown) {
		t.Errorf("an unknown task: %v", err)
	}
	store.fail = true
	if err := registry.SetTriggers(t.Context(), read, daily); err == nil {
		t.Error("a failed save went unreported")
	}
	if info, _ := registry.Task(read, "en"); info.Custom {
		t.Errorf("a failed save changed the triggers: %+v", info)
	}
}

// Saved triggers the task's limits refuse leave it on its defaults.
func TestSavedTriggersOutOfLimitsAreIgnored(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	store.triggers["Read"] = []Trigger{Every(time.Minute)}
	registry := New(quiet())
	registry.Register(Task{Key: "Read", Triggers: []Trigger{Daily(0)}, Limits: Limits{MinInterval: time.Hour},
		Text: map[string]Text{"en": {Name: "Read"}}, Run: func(context.Context) error { return nil }})
	if err := registry.Load(t.Context(), store); err != nil {
		t.Fatal(err)
	}
	if info, _ := registry.Task(ID("Read"), "en"); info.Custom || !slices.Equal(info.Triggers, []Trigger{Daily(0)}) {
		t.Errorf("%+v", info)
	}
}

func TestTriggersRoundTripThroughJSON(t *testing.T) {
	t.Parallel()
	triggers := []Trigger{Daily(4 * time.Hour), Weekly(time.Sunday, 90*time.Minute), Every(6 * time.Hour), AtStartup(),
		{Type: IntervalTrigger, Interval: time.Hour, MaxRuntime: 30 * time.Minute}}
	raw, err := json.Marshal(triggers)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"type":"daily","time":240},{"type":"weekly","time":90,"day":0},{"type":"interval","interval":360},{"type":"startup"},` +
		`{"type":"interval","interval":60,"maxRuntime":30}]`
	if string(raw) != want {
		t.Errorf("JSON: %s", raw)
	}
	var read []Trigger
	if err := json.Unmarshal(raw, &read); err != nil || !slices.Equal(read, triggers) {
		t.Errorf("read back: %+v %v", read, err)
	}
	if err := json.Unmarshal([]byte(`[{"type":"weekly","time":60}]`), &read); err != nil || (Limits{}).Check(read) == nil {
		t.Errorf("a weekly trigger without day: %+v %v", read, err)
	}
}

func TestLongestGap(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		triggers []Trigger
		want     time.Duration
	}{
		{nil, 0},
		{[]Trigger{AtStartup()}, 0},
		{[]Trigger{Daily(4 * time.Hour)}, 24 * time.Hour},
		{[]Trigger{Daily(4 * time.Hour), Daily(16 * time.Hour)}, 12 * time.Hour},
		{[]Trigger{Weekly(time.Monday, 0)}, 7 * 24 * time.Hour},
		{[]Trigger{Weekly(time.Monday, 0), Weekly(time.Tuesday, 0)}, 6 * 24 * time.Hour},
		{[]Trigger{Every(6 * time.Hour), Weekly(time.Monday, 0)}, 6 * time.Hour},
		{[]Trigger{Every(14 * 24 * time.Hour), Weekly(time.Monday, 0)}, 7 * 24 * time.Hour},
	} {
		if got := LongestGap(test.triggers); got != test.want {
			t.Errorf("%v: %v, want %v", test.triggers, got, test.want)
		}
	}
}
