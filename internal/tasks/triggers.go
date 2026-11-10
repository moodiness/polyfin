package tasks

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

// TriggerType is the kind of a trigger, as Jellyfin's TaskTriggerInfoType.
type TriggerType string

const (
	// DailyTrigger runs the task every day at a time of day.
	DailyTrigger TriggerType = "daily"
	// WeeklyTrigger runs it every week on a day at a time of day.
	WeeklyTrigger TriggerType = "weekly"
	// IntervalTrigger runs it once an interval has passed since its last
	// run started.
	IntervalTrigger TriggerType = "interval"
	// StartupTrigger runs it when the server starts.
	StartupTrigger TriggerType = "startup"
)

// AllTriggerTypes are the four trigger types, which a task's Limits
// allow when they name none.
var AllTriggerTypes = []TriggerType{DailyTrigger, WeeklyTrigger, IntervalTrigger, StartupTrigger}

// Trigger is when the schedule runs a task.
type Trigger struct {
	Type TriggerType
	// TimeOfDay is the time of a daily or weekly trigger, from midnight in
	// the server's time zone.
	TimeOfDay time.Duration
	// Day is the day of a weekly trigger.
	Day time.Weekday
	// Interval is the time between the runs of an interval trigger.
	Interval time.Duration
	// MaxRuntime, when not 0, cancels a run the trigger started once it
	// has run that long.
	MaxRuntime time.Duration
}

// Daily, Weekly, Every and AtStartup make the triggers of task defaults.
func Daily(timeOfDay time.Duration) Trigger { return Trigger{Type: DailyTrigger, TimeOfDay: timeOfDay} }

func Weekly(day time.Weekday, timeOfDay time.Duration) Trigger {
	return Trigger{Type: WeeklyTrigger, Day: day, TimeOfDay: timeOfDay}
}

func Every(interval time.Duration) Trigger { return Trigger{Type: IntervalTrigger, Interval: interval} }

func AtStartup() Trigger { return Trigger{Type: StartupTrigger} }

// The reasons a task's triggers are locked, which the admin app explains.
const (
	// LockedSettings: how often the task runs follows its settings.
	LockedSettings = "settings"
	// LockedRecordings: series recordings rely on the task.
	LockedRecordings = "recordings"
	// LockedNotifications: the task is sent on its own as the notification
	// settings say; running it otherwise sends it again.
	LockedNotifications = "notifications"
	// LockedByHand: the task runs only when an administrator starts it.
	LockedByHand = "byHand"
)

// Limits bound the triggers an administrator may give a task.
type Limits struct {
	// Locked, when not empty, is why the triggers cannot change: one of the
	// Locked constants.
	Locked string
	// Types are the trigger types allowed; none allows all four.
	Types []TriggerType
	// MinInterval is the shortest interval of an interval trigger.
	MinInterval time.Duration
	// KeepOne refuses a task without triggers.
	KeepOne bool
}

// The bounds every task's triggers keep.
const (
	MaxTriggers   = 8
	MaxInterval   = 28 * 24 * time.Hour
	MinMaxRuntime = time.Minute
	MaxMaxRuntime = 7 * 24 * time.Hour
)

// ErrInvalidTriggers reports triggers outside a task's Limits, and
// ErrLocked new triggers for a task whose triggers are locked.
var (
	ErrInvalidTriggers = errors.New("invalid triggers")
	ErrLocked          = errors.New("the task's triggers are locked")
)

// AllowedTypes are the trigger types l allows.
func (l Limits) AllowedTypes() []TriggerType {
	if len(l.Types) == 0 {
		return AllTriggerTypes
	}
	return l.Types
}

// Check reports, as ErrInvalidTriggers, what keeps triggers out of l; a
// locked task's are not checked.
func (l Limits) Check(triggers []Trigger) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidTriggers, fmt.Sprintf(format, args...))
	}
	if len(triggers) > MaxTriggers {
		return invalid("more than %d triggers", MaxTriggers)
	}
	if len(triggers) == 0 && l.KeepOne {
		return invalid("the task needs a trigger")
	}
	for i, t := range triggers {
		if !slices.Contains(AllTriggerTypes, t.Type) {
			return invalid("trigger %d: unknown type %q", i, t.Type)
		}
		if !slices.Contains(l.AllowedTypes(), t.Type) {
			return invalid("trigger %d: the task takes no %s trigger", i, t.Type)
		}
		switch t.Type {
		case WeeklyTrigger:
			if t.Day < time.Sunday || t.Day > time.Saturday {
				return invalid("trigger %d: no day of the week", i)
			}
			fallthrough
		case DailyTrigger:
			if t.TimeOfDay < 0 || t.TimeOfDay >= 24*time.Hour || t.TimeOfDay%time.Minute != 0 {
				return invalid("trigger %d: a time of day in whole minutes is needed", i)
			}
		case IntervalTrigger:
			if t.Interval < max(l.MinInterval, time.Minute) || t.Interval > MaxInterval || t.Interval%time.Minute != 0 {
				return invalid("trigger %d: the interval must be whole minutes from %v to %v", i, max(l.MinInterval, time.Minute), MaxInterval)
			}
		}
		if t.MaxRuntime != 0 && (t.MaxRuntime < MinMaxRuntime || t.MaxRuntime > MaxMaxRuntime || t.MaxRuntime%time.Minute != 0) {
			return invalid("trigger %d: the time limit must be whole minutes from %v to %v", i, MinMaxRuntime, MaxMaxRuntime)
		}
	}
	return nil
}

// triggerJSON is a trigger as stored and as the admin API tells it: times
// in minutes, the day from 0, Sunday.
type triggerJSON struct {
	Type       TriggerType `json:"type"`
	Time       *int64      `json:"time,omitempty"`
	Day        *int        `json:"day,omitempty"`
	Interval   *int64      `json:"interval,omitempty"`
	MaxRuntime *int64      `json:"maxRuntime,omitempty"`
}

// MarshalJSON writes t as triggerJSON.
func (t Trigger) MarshalJSON() ([]byte, error) {
	out := triggerJSON{Type: t.Type}
	switch t.Type {
	case WeeklyTrigger:
		out.Day = new(int(t.Day))
		fallthrough
	case DailyTrigger:
		out.Time = new(int64(t.TimeOfDay / time.Minute))
	case IntervalTrigger:
		out.Interval = new(int64(t.Interval / time.Minute))
	}
	if t.MaxRuntime > 0 {
		out.MaxRuntime = new(int64(t.MaxRuntime / time.Minute))
	}
	return json.Marshal(out)
}

// UnmarshalJSON reads triggerJSON; a field its type needs and that is
// missing is left out of bounds, for Limits.Check to refuse.
func (t *Trigger) UnmarshalJSON(data []byte) error {
	var in triggerJSON
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	*t = Trigger{Type: in.Type, TimeOfDay: -1, Day: -1}
	if in.Time != nil {
		t.TimeOfDay = time.Duration(*in.Time) * time.Minute
	}
	if in.Day != nil {
		t.Day = time.Weekday(*in.Day)
	}
	if in.Interval != nil {
		t.Interval = time.Duration(*in.Interval) * time.Minute
	}
	if in.MaxRuntime != nil {
		t.MaxRuntime = time.Duration(*in.MaxRuntime) * time.Minute
	}
	// Fields its type does not use are dropped, so that a trigger compares
	// equal to the one it was read from.
	switch t.Type {
	case DailyTrigger:
		t.Day = 0
	case WeeklyTrigger:
	default:
		t.TimeOfDay, t.Day = 0, 0
	}
	if t.Type != IntervalTrigger {
		t.Interval = 0
	}
	return nil
}

// NextAt is the first time after after that is timeOfDay past midnight in
// after's time zone. A day whose time is skipped by a change to daylight
// saving time runs at the time the zone gives it instead.
func NextAt(after time.Time, timeOfDay time.Duration) time.Time {
	return nextOn(after, timeOfDay, func(time.Time) bool { return true })
}

// NextWeekly is the first time after after that is timeOfDay past
// midnight on day, in after's time zone.
func NextWeekly(after time.Time, day time.Weekday, timeOfDay time.Duration) time.Time {
	return nextOn(after, timeOfDay, func(at time.Time) bool { return at.Weekday() == day })
}

func nextOn(after time.Time, timeOfDay time.Duration, ok func(time.Time) bool) time.Time {
	year, month, day := after.Date()
	hour, minute := int(timeOfDay/time.Hour), int(timeOfDay%time.Hour/time.Minute)
	for i := 0; ; i++ {
		next := time.Date(year, month, day+i, hour, minute, 0, 0, after.Location())
		if next.After(after) && ok(time.Date(year, month, day+i, 12, 0, 0, 0, after.Location())) {
			return next
		}
	}
}

// LongestGap is the longest time triggers may leave between two runs: the
// shortest interval of their interval triggers, or the longest gap
// between the times of their daily and weekly triggers in a week; 0
// without either.
func LongestGap(triggers []Trigger) time.Duration {
	var gap time.Duration
	var times []time.Duration
	const week = 7 * 24 * time.Hour
	for _, t := range triggers {
		switch t.Type {
		case IntervalTrigger:
			if gap == 0 || t.Interval < gap {
				gap = t.Interval
			}
		case DailyTrigger:
			for day := range 7 {
				times = append(times, time.Duration(day)*24*time.Hour+t.TimeOfDay)
			}
		case WeeklyTrigger:
			times = append(times, time.Duration(t.Day)*24*time.Hour+t.TimeOfDay)
		}
	}
	if len(times) > 0 {
		slices.Sort(times)
		calendar := times[0] + week - times[len(times)-1]
		for i := 1; i < len(times); i++ {
			calendar = max(calendar, times[i]-times[i-1])
		}
		if gap == 0 || calendar < gap {
			gap = calendar
		}
	}
	return gap
}
