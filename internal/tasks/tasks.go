// Package tasks runs Polyfin's periodic jobs and the jobs administrators
// start by hand, and tells their state as Jellyfin's scheduled tasks.
//
// A job registers once, with Register, before or after Start:
//
//	registry.Register(tasks.Task{
//		Key:      "RefreshGuide",
//		Category: tasks.CategoryLiveTV,
//		Text:     map[string]tasks.Text{"en": {Name: "Refresh the guide", Description: "…"}, "fr": {…}},
//		Triggers: []tasks.Trigger{tasks.AtStartup(), tasks.Every(12 * time.Hour)},
//		Run:      guide.Refresh,
//	})
//
// Key identifies the task for good: its Jellyfin identifier derives from
// it. Triggers are the task's default schedule, which administrators may
// change within its Limits (see SetTriggers); a task without triggers
// runs only when an administrator starts it. Run gets a context that ends
// when the task is cancelled, its time limit passes or the server stops;
// its error is reported as the task's failure. A task never runs twice at
// once: a run due while one goes on is skipped.
package tasks

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/i18n"
)

// Jellyfin's task categories, which apps group tasks by.
const (
	CategoryMaintenance = "Maintenance"
	CategoryLibrary     = "Library"
	CategoryLiveTV      = "Live TV"
)

// categoryText names the categories in each server language.
var categoryText = map[string]i18n.Text{
	CategoryMaintenance: {"Maintenance", "Maintenance", "Wartung", "Mantenimiento", "Manutenzione", "Manutenção"},
	CategoryLibrary:     {"Library", "Médiathèque", "Bibliothek", "Biblioteca", "Libreria", "Biblioteca"},
	CategoryLiveTV:      {"Live TV", "TV en direct", "Live-TV", "Televisión en vivo", "Diretta TV", "TV ao Vivo"},
}

// Text is a task's name and description in one language.
type Text struct {
	Name        string
	Description string
}

// Task is a job the registry runs.
type Task struct {
	// Key identifies the task for good, as Jellyfin's task keys do.
	Key string
	// Category is one of the Category constants.
	Category string
	// Text gives the name and description by server language; "en" is
	// required and used for languages missing.
	Text map[string]Text
	// Triggers are the task's default triggers, which administrators may
	// replace within Limits.
	Triggers []Trigger
	Limits   Limits
	// Enabled, when set, tells whether the triggers run the task: a task
	// disabled runs by hand only, as Jellyfin's disabled tasks.
	Enabled func() bool
	Run     func(ctx context.Context) error
}

// State is a task's state, as Jellyfin's TaskState.
type State string

const (
	Idle       State = "Idle"
	Running    State = "Running"
	Cancelling State = "Cancelling"
)

// Status is how a run ended, as Jellyfin's TaskCompletionStatus.
type Status string

const (
	Completed Status = "Completed"
	Failed    Status = "Failed"
	Cancelled Status = "Cancelled"
)

// Result is a run that ended.
type Result struct {
	Start, End time.Time
	Status     Status
	// Error is the failure of a failed run, or why a run was cancelled
	// when its time limit passed.
	Error string
}

// Info is what the registry tells of a task, in one language.
type Info struct {
	ID          string
	Key         string
	Name        string
	Description string
	Category    string
	// Triggers are the task's triggers, Defaults those of its code, and
	// Custom tells that an administrator changed them.
	Triggers []Trigger
	Defaults []Trigger
	Custom   bool
	Limits   Limits
	State    State
	// Last is the last run that ended, kept across restarts once the
	// registry is loaded; nil when none did.
	Last *Result
	// Next is when the schedule runs the task next; nil for a task without
	// timed triggers or disabled, or before the registry starts.
	Next *time.Time
}

// Store keeps the triggers administrators gave and the last runs across
// restarts.
type Store interface {
	// LoadTriggers gives the triggers saved, by task key.
	LoadTriggers(ctx context.Context) (map[string][]Trigger, error)
	// SaveTriggers replaces the triggers of a task; DeleteTriggers returns
	// it to its defaults.
	SaveTriggers(ctx context.Context, key string, triggers []Trigger) error
	DeleteTriggers(ctx context.Context, key string) error
	// LoadRuns gives the last run of each task, by key.
	LoadRuns(ctx context.Context) (map[string]Result, error)
	SaveRun(ctx context.Context, key string, result Result) error
}

// Registry runs the registered tasks.
type Registry struct {
	logger *slog.Logger
	mu     sync.Mutex
	tasks  []*entry
	// ctx is the registry's context once started; nil before.
	ctx context.Context
	// store keeps triggers and runs once loaded, and triggers and runs
	// what it gave, by key, for the tasks registered after.
	store    Store
	triggers map[string][]Trigger
	runs     map[string]Result
	// now is the clock triggers are scheduled by, and check how often a
	// schedule looks at it at least.
	now   func() time.Time
	check time.Duration
}

type entry struct {
	Task
	id     string
	cancel context.CancelFunc
	state  State
	last   *Result
	// triggers are those in force, custom when an administrator gave
	// them.
	triggers []Trigger
	custom   bool
	// stop ends the schedule's goroutine; scheduled is when the schedule
	// began, fired when a trigger last fired, and next when one fires
	// next, by trigger, zero without one.
	stop      context.CancelFunc
	scheduled time.Time
	fired     time.Time
	next      time.Time
	trigger   Trigger
}

// New returns an empty registry.
func New(logger *slog.Logger) *Registry {
	return &Registry{logger: logger, now: time.Now, check: time.Minute}
}

// ID is the Jellyfin identifier of the task key names: 32 hexadecimal
// digits, as Jellyfin derives its own from its tasks' types.
func ID(key string) string {
	sum := md5.Sum([]byte("polyfin.task." + key))
	return hex.EncodeToString(sum[:])
}

// Register adds a task; one registered after Start is scheduled at once.
// It panics on a task without Key, English text or Run, with default
// triggers outside its limits, or with a Key already registered:
// registrations are made by code at startup.
func (r *Registry) Register(task Task) {
	if task.Key == "" || task.Text["en"].Name == "" || task.Run == nil {
		panic("tasks: a task needs a key, an English name and a run")
	}
	if task.Limits.Locked == "" {
		if err := task.Limits.Check(task.Triggers); err != nil {
			panic("tasks: " + task.Key + ": " + err.Error())
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if slices.ContainsFunc(r.tasks, func(e *entry) bool { return e.Key == task.Key }) {
		panic("tasks: " + task.Key + " is registered twice")
	}
	e := &entry{Task: task, id: ID(task.Key), state: Idle, triggers: task.Triggers}
	r.apply(e)
	r.tasks = append(r.tasks, e)
	if r.ctx != nil {
		r.schedule(e, true)
	}
}

// Load reads the triggers and last runs store keeps, and keeps those of
// the runs to come there; it is called before Start. Saved triggers a
// task's limits refuse, as after its limits changed, are left for its
// defaults, with a warning.
func (r *Registry) Load(ctx context.Context, store Store) error {
	triggers, err := store.LoadTriggers(ctx)
	if err != nil {
		return err
	}
	runs, err := store.LoadRuns(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store, r.triggers, r.runs = store, triggers, runs
	for _, e := range r.tasks {
		r.apply(e)
	}
	return nil
}

// apply gives e the triggers and last run loaded; r.mu is held.
func (r *Registry) apply(e *entry) {
	if saved, ok := r.triggers[e.Key]; ok && e.Limits.Locked == "" {
		if err := e.Limits.Check(saved); err != nil {
			r.logger.Warn("A task's saved triggers are refused: it runs on its defaults", "task", e.Key, "error", err)
		} else {
			e.triggers, e.custom = saved, true
		}
	}
	if last, ok := r.runs[e.Key]; ok && e.last == nil {
		e.last = &last
	}
}

// Start runs the tasks of a startup trigger and schedules the others until
// ctx ends.
func (r *Registry) Start(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ctx = ctx
	for _, e := range r.tasks {
		r.schedule(e, true)
	}
}

// schedule follows e's triggers from now, in place of the schedule it
// had, and runs it now when atStart and it has a startup trigger; r.mu is
// held.
func (r *Registry) schedule(e *entry, atStart bool) {
	if e.stop != nil {
		e.stop()
	}
	ctx, stop := context.WithCancel(r.ctx)
	e.stop = stop
	e.scheduled, e.fired = r.now(), time.Time{}
	if atStart {
		if i := slices.IndexFunc(e.triggers, func(t Trigger) bool { return t.Type == StartupTrigger }); i >= 0 {
			r.start(e, false, e.triggers[i].MaxRuntime)
		}
	}
	e.next, e.trigger = r.nextRun(e, e.scheduled)
	if !e.next.IsZero() {
		go r.follow(ctx, e)
	}
}

// follow runs e when its triggers fire, until ctx ends. It wakes at the
// next run, or sooner to look at the clock again.
func (r *Registry) follow(ctx context.Context, e *entry) {
	for {
		r.mu.Lock()
		wait := min(e.next.Sub(r.now()), r.check)
		r.mu.Unlock()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		r.mu.Lock()
		if ctx.Err() != nil {
			r.mu.Unlock()
			return
		}
		now := r.now()
		if !now.Before(e.next) {
			e.fired = now
			r.start(e, false, e.trigger.MaxRuntime)
		}
		e.next, e.trigger = r.nextRun(e, now)
		r.mu.Unlock()
	}
}

// nextRun is when e's triggers run it next after after, and the trigger
// that does; zero without timed triggers; r.mu is held.
//
// An interval counts from the start of the last run, kept across
// restarts, or from when a trigger last fired when later, as when the run
// was skipped. Without either, the first run comes the interval and its
// stagger (see stagger) after the schedule began, so that tasks of one
// interval do not all run in the same second; a run overdue at start
// comes its stagger after it.
func (r *Registry) nextRun(e *entry, after time.Time) (time.Time, Trigger) {
	var next time.Time
	var trigger Trigger
	for _, t := range e.triggers {
		var at time.Time
		switch t.Type {
		case DailyTrigger:
			at = NextAt(after, t.TimeOfDay)
		case WeeklyTrigger:
			at = NextWeekly(after, t.Day, t.TimeOfDay)
		case IntervalTrigger:
			from := e.fired
			if e.last != nil && e.last.Start.After(from) {
				from = e.last.Start
			}
			if from.IsZero() {
				at = e.scheduled.Add(t.Interval + stagger(e.Key, t.Interval))
			} else {
				at = from.Add(t.Interval)
				if floor := e.scheduled.Add(stagger(e.Key, t.Interval)); at.Before(floor) {
					at = floor
				}
			}
		default:
			continue
		}
		if next.IsZero() || at.Before(next) {
			next, trigger = at, t
		}
	}
	return next, trigger
}

// maxStagger bounds how long after its interval an interval task first
// runs.
const maxStagger = 5 * time.Minute

// stagger is how long after its interval the task of key first runs: a
// share of maxStagger, or of half the interval when shorter, taken from
// the key's hash, so the same in every run of the server.
func stagger(key string, interval time.Duration) time.Duration {
	bound := min(maxStagger, interval/2)
	if bound <= 0 {
		return 0
	}
	sum := md5.Sum([]byte("polyfin.task.stagger." + key))
	return time.Duration(binary.BigEndian.Uint64(sum[:8]) % uint64(bound))
}

// errTimeLimit cancels a run whose trigger's time limit passed.
var errTimeLimit = errors.New("the time limit passed")

// start runs e unless it is running, byHand when an administrator asked,
// and cancels it after limit when not 0; a task disabled runs by hand
// only. r.mu is held.
func (r *Registry) start(e *entry, byHand bool, limit time.Duration) {
	if e.state != Idle || r.ctx == nil || r.ctx.Err() != nil || !byHand && e.Enabled != nil && !e.Enabled() {
		return
	}
	ctx, cancel := context.WithCancel(context.WithValue(r.ctx, byHandKey{}, byHand))
	if limit > 0 {
		cancelRun := cancel
		limited, cancelLimit := context.WithTimeoutCause(ctx, limit, errTimeLimit)
		ctx, cancel = limited, func() { cancelLimit(); cancelRun() }
	}
	e.state, e.cancel = Running, cancel
	started := r.now()
	go func() {
		err := e.Run(ctx)
		result := Result{Start: started, End: r.now(), Status: Completed}
		switch {
		case errors.Is(context.Cause(ctx), errTimeLimit):
			result.Status, result.Error = Cancelled, errTimeLimit.Error()
		case ctx.Err() != nil:
			result.Status = Cancelled
		case err != nil:
			result.Status, result.Error = Failed, err.Error()
			r.logger.Warn("A task failed", "task", e.Key, "error", err)
		}
		cancel()
		r.mu.Lock()
		e.state, e.cancel, e.last = Idle, nil, &result
		store := r.store
		r.mu.Unlock()
		if store != nil {
			saveCtx, done := context.WithTimeout(context.WithoutCancel(r.ctx), 10*time.Second)
			defer done()
			if err := store.SaveRun(saveCtx, e.Key, result); err != nil {
				r.logger.Warn("A task's last run could not be saved", "task", e.Key, "error", err)
			}
		}
	}()
}

type byHandKey struct{}

// ByHand reports whether the run ctx belongs to was started by an
// administrator rather than by its schedule: a task may then do more, as
// when a guide refresh fetches every guide rather than those due.
func ByHand(ctx context.Context) bool {
	byHand, _ := ctx.Value(byHandKey{}).(bool)
	return byHand
}

// Tasks describes every task in language, by name.
func (r *Registry) Tasks(language string) []Info {
	r.mu.Lock()
	infos := make([]Info, 0, len(r.tasks))
	for _, e := range r.tasks {
		infos = append(infos, e.info(language))
	}
	r.mu.Unlock()
	slices.SortFunc(infos, func(a, b Info) int { return strings.Compare(a.Name, b.Name) })
	return infos
}

// Task describes the task id identifies, matched without regard to case.
func (r *Registry) Task(id, language string) (Info, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.find(id); e != nil {
		return e.info(language), true
	}
	return Info{}, false
}

// ErrUnknown reports a task identifier no task has.
var ErrUnknown = errors.New("unknown task")

// Run starts the task id identifies, unless it is running.
func (r *Registry) Run(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.find(id)
	if e == nil {
		return ErrUnknown
	}
	r.start(e, true, 0)
	return nil
}

// Cancel asks the task id identifies to stop, if it is running.
func (r *Registry) Cancel(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.find(id)
	if e == nil {
		return ErrUnknown
	}
	if e.state == Running {
		e.state = Cancelling
		e.cancel()
	}
	return nil
}

// SetTriggers replaces the triggers of the task id identifies, saved for
// the next starts, and schedules it by them from now. Triggers outside
// the task's limits are refused with ErrInvalidTriggers before anything is
// saved; a locked task's with ErrLocked, but for the triggers it has,
// which change nothing.
func (r *Registry) SetTriggers(ctx context.Context, id string, triggers []Trigger) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.find(id)
	if e == nil {
		return ErrUnknown
	}
	if e.Limits.Locked != "" {
		if slices.Equal(triggers, e.triggers) {
			return nil
		}
		return ErrLocked
	}
	if err := e.Limits.Check(triggers); err != nil {
		return err
	}
	triggers = slices.Clone(triggers)
	if triggers == nil {
		triggers = []Trigger{}
	}
	if r.store != nil {
		if err := r.store.SaveTriggers(ctx, e.Key, triggers); err != nil {
			return err
		}
	}
	e.triggers, e.custom = triggers, true
	if r.ctx != nil {
		r.schedule(e, false)
	}
	return nil
}

// ResetTriggers gives the task id identifies its default triggers again.
func (r *Registry) ResetTriggers(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.find(id)
	if e == nil {
		return ErrUnknown
	}
	if !e.custom {
		return nil
	}
	if r.store != nil {
		if err := r.store.DeleteTriggers(ctx, e.Key); err != nil {
			return err
		}
	}
	e.triggers, e.custom = e.Task.Triggers, false
	if r.ctx != nil {
		r.schedule(e, false)
	}
	return nil
}

func (r *Registry) find(id string) *entry {
	for _, e := range r.tasks {
		if strings.EqualFold(e.id, id) {
			return e
		}
	}
	return nil
}

// info describes e.
func (e *entry) info(language string) Info {
	text, ok := e.Text[language]
	if !ok {
		text = e.Text["en"]
	}
	category := e.Category
	if named, ok := categoryText[category]; ok {
		category = named.In(language)
	}
	info := Info{ID: e.id, Key: e.Key, Name: text.Name, Description: text.Description, Category: category,
		Triggers: slices.Clone(e.triggers), Defaults: slices.Clone(e.Task.Triggers), Custom: e.custom, Limits: e.Limits, State: e.state}
	if info.Triggers == nil {
		info.Triggers = []Trigger{}
	}
	if info.Defaults == nil {
		info.Defaults = []Trigger{}
	}
	if e.last != nil {
		last := *e.last
		info.Last = &last
	}
	if !e.next.IsZero() && (e.Enabled == nil || e.Enabled()) {
		info.Next = new(e.next)
	}
	return info
}
