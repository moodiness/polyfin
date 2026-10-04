package accounts

import (
	"errors"
	"math"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// ErrInvalidBlockedGenres reports blocked genres beyond MaxBlockedGenres,
// or a genre that is empty, longer than MaxGenreLength or unprintable.
var ErrInvalidBlockedGenres = errors.New("invalid blocked genres")

// ErrInvalidAccessSchedule reports more than MaxAccessSchedules schedules,
// or one whose day is not one of ScheduleDays or whose hours are not
// MinScheduleHour <= start < end <= MaxScheduleHour.
var ErrInvalidAccessSchedule = errors.New("invalid access schedule")

// The bounds of User.BlockedGenres and User.AccessSchedules.
const (
	MaxBlockedGenres   = 100
	MaxGenreLength     = 100
	MaxAccessSchedules = 50
	MinScheduleHour    = 0
	MaxScheduleHour    = 24
)

// ScheduleDays are the days an access schedule applies on, Jellyfin's
// DynamicDayOfWeek names in its order: the days of the week from Sunday,
// then every day, weekdays (Monday to Friday) and weekends.
var ScheduleDays = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday",
	"Everyday", "Weekday", "Weekend"}

// AccessSchedule is a span of hours a user may use the server in, as
// Jellyfin's user policy holds it. Hours count from midnight, in the
// server's local time zone, and may have fractions (9.5 is 9:30).
type AccessSchedule struct {
	// Day is one of ScheduleDays.
	Day       string  `json:"day"`
	StartHour float64 `json:"start"`
	EndHour   float64 `json:"end"`
}

// includes reports whether t falls in the schedule, as Jellyfin decides:
// on one of its days, from its start hour to its end hour, both included.
func (a AccessSchedule) includes(t time.Time) bool {
	local := t.Local()
	weekday := local.Weekday()
	switch a.Day {
	case "Everyday":
	case "Weekday":
		if weekday == time.Sunday || weekday == time.Saturday {
			return false
		}
	case "Weekend":
		if weekday != time.Sunday && weekday != time.Saturday {
			return false
		}
	default:
		if slices.Index(ScheduleDays, a.Day) != int(weekday) {
			return false
		}
	}
	// The hour on the clock, as Jellyfin reads the time of day.
	hour := float64(local.Hour()) + float64(local.Minute())/60 +
		(float64(local.Second())+float64(local.Nanosecond())/1e9)/3600
	return hour >= a.StartHour && hour <= a.EndHour
}

// AllowedAt reports whether the user may use the server at t: they have
// no access schedule, or t falls in one of them.
func (u User) AllowedAt(t time.Time) bool {
	return len(u.AccessSchedules) == 0 || slices.ContainsFunc(u.AccessSchedules, func(a AccessSchedule) bool { return a.includes(t) })
}

// Restricted reports whether titles may be hidden from the user: by their
// parental control, or by the genres they block. Such a user browses the
// server's addons only, which describe the titles' ratings and genres.
func (u User) Restricted() bool {
	return u.Parental.Restricted() || len(u.BlockedGenres) > 0
}

// normalizedGenres trims the blocked genres and keeps each once, compared
// without regard to case, in the order given.
func normalizedGenres(genres []string) ([]string, error) {
	result := []string{}
	for _, genre := range genres {
		genre = strings.TrimSpace(genre)
		if genre == "" || utf8.RuneCountInString(genre) > MaxGenreLength ||
			strings.ContainsFunc(genre, func(r rune) bool { return !unicode.IsPrint(r) }) {
			return nil, ErrInvalidBlockedGenres
		}
		if !slices.ContainsFunc(result, func(kept string) bool { return strings.EqualFold(kept, genre) }) {
			result = append(result, genre)
		}
	}
	if len(result) > MaxBlockedGenres {
		return nil, ErrInvalidBlockedGenres
	}
	return result, nil
}

// normalizedSchedules names the days as ScheduleDays does, in any case on
// input, and checks the hours.
func normalizedSchedules(schedules []AccessSchedule) ([]AccessSchedule, error) {
	if len(schedules) > MaxAccessSchedules {
		return nil, ErrInvalidAccessSchedule
	}
	result := make([]AccessSchedule, 0, len(schedules))
	for _, schedule := range schedules {
		day := slices.IndexFunc(ScheduleDays, func(known string) bool { return strings.EqualFold(known, schedule.Day) })
		start, end := schedule.StartHour, schedule.EndHour
		if day < 0 || math.IsNaN(start) || math.IsNaN(end) || start < MinScheduleHour || end > MaxScheduleHour || start >= end {
			return nil, ErrInvalidAccessSchedule
		}
		result = append(result, AccessSchedule{Day: ScheduleDays[day], StartHour: start, EndHour: end})
	}
	return result, nil
}

// normalizedLibraries keeps each hidden library once, in the order given.
func normalizedLibraries(libraries []ID) []ID {
	result := []ID{}
	for _, library := range libraries {
		if !slices.Contains(result, library) {
			result = append(result, library)
		}
	}
	return result
}
