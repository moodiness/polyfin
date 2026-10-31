package homerows

import (
	"testing"
	"time"
)

// at is a time of the server's time zone.
func at(month time.Month, day, hour, minute int) time.Time {
	return time.Date(2026, month, day, hour, minute, 0, 0, time.Local)
}

// day is a date as schedules keep them.
func day(month time.Month, d int) time.Time {
	return time.Date(2026, month, d, 0, 0, 0, 0, time.UTC)
}

// A row shows from the start of its first date to the end of its last, on
// its days, in its hours; a span of hours past midnight belongs to the day
// it starts on. A row that is also a library follows its dates only.
func TestRowsShowInsideTheirWindow(t *testing.T) {
	t.Parallel()
	// November 6, 2026 is a Friday.
	fridayNights := Schedule{Days: []time.Weekday{time.Friday}, Hours: &Hours{From: 22, To: 2}}
	for _, test := range []struct {
		name     string
		schedule Schedule
		library  bool
		at       time.Time
		shows    bool
	}{
		{"no schedule", Schedule{}, false, at(time.November, 6, 3, 0), true},
		{"the day before the start", Schedule{Start: day(time.December, 1), End: day(time.December, 3)}, false, at(time.November, 30, 23, 59), false},
		{"the start", Schedule{Start: day(time.December, 1), End: day(time.December, 3)}, false, at(time.December, 1, 0, 0), true},
		{"the end", Schedule{Start: day(time.December, 1), End: day(time.December, 3)}, false, at(time.December, 3, 23, 59), true},
		{"the day after the end", Schedule{Start: day(time.December, 1), End: day(time.December, 3)}, false, at(time.December, 4, 0, 0), false},
		{"no start", Schedule{End: day(time.December, 3)}, false, at(time.January, 1, 12, 0), true},
		{"no end", Schedule{Start: day(time.December, 1)}, false, at(time.December, 31, 12, 0), true},
		{"one of its days", Schedule{Days: []time.Weekday{time.Friday, time.Saturday}}, false, at(time.November, 7, 12, 0), true},
		{"another day", Schedule{Days: []time.Weekday{time.Friday, time.Saturday}}, false, at(time.November, 8, 12, 0), false},
		{"before its hours", Schedule{Hours: &Hours{From: 18, To: 23}}, false, at(time.November, 6, 17, 59), false},
		{"its first hour", Schedule{Hours: &Hours{From: 18, To: 23}}, false, at(time.November, 6, 18, 0), true},
		{"its last hour", Schedule{Hours: &Hours{From: 18, To: 23}}, false, at(time.November, 6, 22, 59), true},
		{"after its hours", Schedule{Hours: &Hours{From: 18, To: 23}}, false, at(time.November, 6, 23, 0), false},
		{"until midnight", Schedule{Hours: &Hours{From: 18, To: 24}}, false, at(time.November, 6, 23, 59), true},
		{"midnight", Schedule{Hours: &Hours{From: 18, To: 24}}, false, at(time.November, 7, 0, 0), false},
		{"Friday night", fridayNights, false, at(time.November, 6, 22, 0), true},
		{"Friday night after midnight", fridayNights, false, at(time.November, 7, 1, 59), true},
		{"Friday night's end", fridayNights, false, at(time.November, 7, 2, 0), false},
		{"Thursday night after midnight", fridayNights, false, at(time.November, 6, 1, 0), false},
		{"the last night after midnight", Schedule{End: day(time.December, 31), Hours: &Hours{From: 22, To: 2}}, false,
			time.Date(2027, time.January, 1, 1, 0, 0, 0, time.Local), true},
		{"the night after the last", Schedule{End: day(time.December, 31), Hours: &Hours{From: 22, To: 2}}, false,
			time.Date(2027, time.January, 1, 22, 0, 0, 0, time.Local), false},
		{"a library outside its days and hours", Schedule{Start: day(time.December, 1), Days: []time.Weekday{time.Saturday},
			Hours: &Hours{From: 9, To: 10}}, true, at(time.December, 2, 15, 0), true},
		{"a library before its start", Schedule{Start: day(time.December, 1), Hours: &Hours{From: 9, To: 10}}, true,
			at(time.November, 30, 9, 30), false},
		{"a library after its end", Schedule{End: day(time.December, 1)}, true, at(time.December, 2, 0, 0), false},
	} {
		row := Row{Library: test.library, Schedule: test.schedule}
		if got := row.ShowsAt(test.at); got != test.shows {
			t.Errorf("%s, at %s: shows %t", test.name, test.at, got)
		}
	}
}

// A schedule is refused unless its dates are dates in order, its days are
// days of the week in order, each once, and its hours are a span.
func TestScheduleValid(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		schedule Schedule
		valid    bool
	}{
		{"none", Schedule{}, true},
		{"every field", Schedule{Start: day(time.December, 1), End: day(time.December, 1), Days: []time.Weekday{time.Sunday, time.Saturday},
			Hours: &Hours{From: 23, To: 1}}, true},
		{"the start after the end", Schedule{Start: day(time.December, 2), End: day(time.December, 1)}, false},
		{"a time of day", Schedule{Start: day(time.December, 1).Add(time.Hour)}, false},
		{"days out of order", Schedule{Days: []time.Weekday{time.Monday, time.Sunday}}, false},
		{"a day twice", Schedule{Days: []time.Weekday{time.Monday, time.Monday}}, false},
		{"no such day", Schedule{Days: []time.Weekday{7}}, false},
		{"the same hours", Schedule{Hours: &Hours{From: 8, To: 8}}, false},
		{"hour 24 as start", Schedule{Hours: &Hours{From: 24, To: 2}}, false},
		{"hour 0 as end", Schedule{Hours: &Hours{From: 20, To: 0}}, false},
		{"all day", Schedule{Hours: &Hours{From: 0, To: 24}}, true},
	} {
		if got := test.schedule.Valid(); got != test.valid {
			t.Errorf("%s: valid %t", test.name, got)
		}
	}
}
