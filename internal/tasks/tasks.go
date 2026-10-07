// Package tasks runs Polyfin's periodic jobs and the jobs administrators
// start by hand, and tells their state as Jellyfin's scheduled tasks.
//
// A job registers once, with Register, before or after Start:
//
//	registry.Register(tasks.Task{
//		Key:      "RefreshGuide",
//		Category: tasks.CategoryLiveTV,
//		Text:     map[string]tasks.Text{"en": {Name: "Refresh the guide", Description: "…"}, "fr": {…}},
//		Interval: 12 * time.Hour,
//		Run:      guide.Refresh,
//	})
//
// Key identifies the task for good: its Jellyfin identifier derives from
// it. Interval runs it periodically, Daily every day at an hour, AtStart
// once when the registry starts; a task with none runs only when an
// administrator starts it. Run gets a context that ends when the task is
// cancelled or the server stops; its error is reported as the task's
// failure. A task never runs twice at once: a run due while one goes on is
// skipped.
package tasks

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
)

// Jellyfin's task categories, which apps group tasks by.
const (
	CategoryMaintenance = "Maintenance"
	CategoryLibrary     = "Library"
	CategoryLiveTV      = "Live TV"
)

// categoryText names the categories in the server languages other than
// English.
var categoryText = map[string]map[string]string{
	"fr": {CategoryMaintenance: "Maintenance", CategoryLibrary: "Médiathèque", CategoryLiveTV: "TV en direct"},
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
	// Interval runs the task that often; 0 never runs it on a timer.
	Interval time.Duration
	// Daily, when set, runs the task every day at the hour of the server's
	// time zone it returns, 0 to 23, or never for a negative hour: the task
	// then runs by hand only. It is asked again every minute, so that a new
	// hour applies without a restart. A task has an Interval or Daily, not
	// both.
	Daily func() int
	// AtStart runs the task once when the registry starts.
	AtStart bool
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
	// Error is the failure of a failed run.
	Error string
}

// Info is what the registry tells of a task, in one language.
type Info struct {
	ID          string
	Key         string
	Name        string
	Description string
	Category    string
	Interval    time.Duration
	// Daily is the hour a daily task runs at, nil for other tasks and for
	// a daily task without one.
	Daily   *int
	AtStart bool
	State   State
	// Last is the last run that ended since the server started; nil when
	// none did.
	Last *Result
	// Next is when the schedule runs the task next; nil for a task without
	// interval or daily hour, or before the registry starts.
	Next *time.Time
}

// Registry runs the registered tasks.
type Registry struct {
	logger *slog.Logger
	mu     sync.Mutex
	tasks  []*entry
	// ctx is the registry's context once started; nil before.
	ctx context.Context
	// now is the clock daily tasks are scheduled by, and dailyCheck how
	// often their hour is asked again.
	now        func() time.Time
	dailyCheck time.Duration
}

type entry struct {
	Task
	id     string
	cancel context.CancelFunc
	state  State
	last   *Result
	// next is when the ticker or the daily schedule fires next, zero
	// without one.
	next time.Time
}

// New returns an empty registry.
func New(logger *slog.Logger) *Registry {
	return &Registry{logger: logger, now: time.Now, dailyCheck: time.Minute}
}

// ID is the Jellyfin identifier of the task key names: 32 hexadecimal
// digits, as Jellyfin derives its own from its tasks' types.
func ID(key string) string {
	sum := md5.Sum([]byte("polyfin.task." + key))
	return hex.EncodeToString(sum[:])
}

// Register adds a task; one registered after Start is scheduled at once.
// It panics on a task without Key, English text or Run, with both an
// Interval and Daily, or with a Key already registered: registrations are
// made by code at startup.
func (r *Registry) Register(task Task) {
	if task.Key == "" || task.Text["en"].Name == "" || task.Run == nil {
		panic("tasks: a task needs a key, an English name and a run")
	}
	if task.Interval > 0 && task.Daily != nil {
		panic("tasks: " + task.Key + " has both an interval and a daily hour")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if slices.ContainsFunc(r.tasks, func(e *entry) bool { return e.Key == task.Key }) {
		panic("tasks: " + task.Key + " is registered twice")
	}
	e := &entry{Task: task, id: ID(task.Key), state: Idle}
	r.tasks = append(r.tasks, e)
	if r.ctx != nil {
		r.schedule(e)
	}
}

// Start runs the tasks due at start and schedules the periodic ones until
// ctx ends.
func (r *Registry) Start(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ctx = ctx
	for _, e := range r.tasks {
		r.schedule(e)
	}
}

// schedule runs e at start and on its interval or daily hour; r.mu is
// held.
func (r *Registry) schedule(e *entry) {
	if e.AtStart {
		r.start(e, false)
	}
	if e.Daily != nil {
		e.next = scheduledDaily(r.now(), e.Daily())
		go r.daily(e, e.next)
		return
	}
	if e.Interval <= 0 {
		return
	}
	e.next = time.Now().Add(e.Interval)
	go func() {
		ticker := time.NewTicker(e.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-r.ctx.Done():
				return
			case at := <-ticker.C:
				r.mu.Lock()
				e.next = at.Add(e.Interval)
				r.start(e, false)
				r.mu.Unlock()
			}
		}
	}()
}

// daily runs e every day at the hour e.Daily gives, next the first time,
// until the registry's context ends. It wakes at the next run, or sooner
// to ask the hour again: an hour changed to one already past today runs
// tomorrow. A negative hour runs it never, until an hour is given.
func (r *Registry) daily(e *entry, next time.Time) {
	for {
		wait := r.dailyCheck
		if !next.IsZero() {
			wait = min(next.Sub(r.now()), r.dailyCheck)
		}
		timer := time.NewTimer(wait)
		select {
		case <-r.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		checked := r.now()
		r.mu.Lock()
		if !next.IsZero() && !checked.Before(next) {
			r.start(e, false)
		}
		next = scheduledDaily(checked, e.Daily())
		e.next = next
		r.mu.Unlock()
	}
}

// scheduledDaily is when a daily task of hour runs next after after (see
// NextDaily), zero for a negative hour: none.
func scheduledDaily(after time.Time, hour int) time.Time {
	if hour < 0 {
		return time.Time{}
	}
	return NextDaily(after, hour)
}

// NextDaily is the first time after after that is hour o'clock in after's
// time zone. A day whose hour is skipped by a change to daylight saving
// time runs at the time the zone gives it instead.
func NextDaily(after time.Time, hour int) time.Time {
	year, month, day := after.Date()
	next := time.Date(year, month, day, hour, 0, 0, 0, after.Location())
	if !next.After(after) {
		next = time.Date(year, month, day+1, hour, 0, 0, 0, after.Location())
	}
	return next
}

// start runs e unless it is running, byHand when an administrator asked;
// r.mu is held.
func (r *Registry) start(e *entry, byHand bool) {
	if e.state != Idle || r.ctx == nil || r.ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.WithValue(r.ctx, byHandKey{}, byHand))
	e.state, e.cancel = Running, cancel
	started := time.Now()
	go func() {
		err := e.Run(ctx)
		result := &Result{Start: started, End: time.Now(), Status: Completed}
		switch {
		case ctx.Err() != nil:
			result.Status = Cancelled
		case err != nil:
			result.Status, result.Error = Failed, err.Error()
			r.logger.Warn("A task failed", "task", e.Key, "error", err)
		}
		cancel()
		r.mu.Lock()
		e.state, e.cancel, e.last = Idle, nil, result
		r.mu.Unlock()
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
		infos = append(infos, e.info(language, r.now()))
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
		return e.info(language, r.now()), true
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
	r.start(e, true)
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

func (r *Registry) find(id string) *entry {
	for _, e := range r.tasks {
		if strings.EqualFold(e.id, id) {
			return e
		}
	}
	return nil
}

// info describes e at now. A daily task's next run is that of its hour at
// now, which a new hour changes at once.
func (e *entry) info(language string, now time.Time) Info {
	text, ok := e.Text[language]
	if !ok {
		text = e.Text["en"]
	}
	category := e.Category
	if named, ok := categoryText[language][category]; ok {
		category = named
	}
	info := Info{ID: e.id, Key: e.Key, Name: text.Name, Description: text.Description, Category: category,
		Interval: e.Interval, AtStart: e.AtStart, State: e.state}
	if e.last != nil {
		last := *e.last
		info.Last = &last
	}
	switch {
	case e.Daily != nil:
		if hour := e.Daily(); hour >= 0 {
			info.Daily = &hour
			if !e.next.IsZero() {
				info.Next = new(NextDaily(now, hour))
			}
		}
	case !e.next.IsZero():
		info.Next = new(e.next)
	}
	return info
}
