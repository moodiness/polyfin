package taste

import (
	"math"
	"time"
)

const (
	// earlyShare is the share of a title under which a play stopped is
	// abandoned early.
	earlyShare = 0.1
	// abandonedAfter is how long after a play stopped early it counts as
	// abandoned: a user may come back to it before.
	abandonedAfter = 3 * 24 * time.Hour
)

// Activity is what a user did with a movie or series: with a series, with
// its episodes too.
type Activity struct {
	// Series tells a series: Finished then counts its episodes finished.
	Series bool
	// Finished counts the times a movie was watched to its end, or the
	// episodes of a series that were.
	Finished int
	// Early tells a title started and stopped early, before a tenth of it,
	// and never finished: with a series, each episode started was.
	Early    bool
	Favorite bool
	// Rating is the user's rating from 0 to 10, nil if none: a like is 10,
	// a dislike 1 (see package userdata).
	Rating *float64
	Hidden bool
	// At is the user's last activity with the title, which its weight
	// decays from; zero for none known, which does not decay.
	At time.Time
}

// Weight is how much the activity tells the user likes the title at now,
// negative for a title they do not:
//
//   - a movie finished counts 1, and each time it was watched again half
//     as much more, twice at most;
//   - a series of which episodes were finished counts 1, and each further
//     episode a quarter more, six at most;
//   - a title abandoned early counts -1, once 3 days passed;
//   - a favorite counts 1.5 more;
//   - a rating counts from -2 (0) to 2 (10): a like 2, a dislike -1.6;
//   - a hidden title counts -1.
//
// The sum halves every 180 days after the last activity.
func (a Activity) Weight(now time.Time) float64 {
	w := 0.0
	switch {
	case a.Finished > 0 && a.Series:
		w += 1 + 0.25*float64(min(a.Finished-1, 6))
	case a.Finished > 0:
		w += 1 + 0.5*float64(min(a.Finished-1, 2))
	case a.Early && !a.At.IsZero() && now.Sub(a.At) >= abandonedAfter:
		w--
	}
	if a.Favorite {
		w += 1.5
	}
	if a.Rating != nil {
		w += (min(max(*a.Rating, 0), 10) - 5) / 5 * 2
	}
	if a.Hidden {
		w--
	}
	if age := now.Sub(a.At); !a.At.IsZero() && age > 0 {
		w *= math.Exp2(-float64(age) / float64(halfLife))
	}
	return w
}

// Early reports whether a play that stopped at position of runtime, and
// did not finish, stopped early. No position is a play stopped before
// the resume point Jellyfin keeps, which is early too; a position of a
// runtime unknown is not.
func Early(position, runtime time.Duration) bool {
	return position <= 0 || runtime > 0 && float64(position) < earlyShare*float64(runtime)
}
