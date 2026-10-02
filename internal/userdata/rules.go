package userdata

import "time"

// The thresholds of a Jellyfin server's default configuration
// (MinResumePct, MaxResumePct and MinResumeDurationSeconds).
const (
	minResumePercent  = 5
	maxResumePercent  = 90
	minResumeDuration = 5 * time.Minute
)

// Start records that playback began at now: one more play.
func (d *Data) Start(now time.Time) {
	d.PlayCount++
	d.LastPlayed = &now
}

// Reach records the position a player reported while playing or when it
// stopped, against the runtime of what it played. Near the start there is
// nothing to resume; near the end, or anywhere past the start of a short
// item, the item is played and there is nothing to resume either. Without
// a runtime, the position is kept as it is.
func (d *Data) Reach(position, runtime time.Duration) {
	d.Runtime = runtime
	if runtime <= 0 {
		d.Position = max(position, 0)
		return
	}
	percent := float64(position) / float64(runtime) * 100
	switch {
	case percent < minResumePercent:
		d.Position = 0
	case percent > maxResumePercent || runtime < minResumeDuration:
		d.Position = 0
		d.Played = true
	default:
		d.Position = position
	}
}

// Finish records a stop reported without a position: the item was played
// through, which counts as one more play.
func (d *Data) Finish() {
	d.Position = 0
	d.Played = true
	d.PlayCount++
}

// MarkPlayed marks the item played, as a user does from an app, and drops
// its resume point. A mark dated when the user says they played the item
// counts one more play on that date. An undated mark counts a play only if
// there was none, and dates it now only if it had no date.
func (d *Data) MarkPlayed(date *time.Time, now time.Time) {
	d.Played = true
	d.Position = 0
	if date != nil {
		d.PlayCount++
		d.LastPlayed = new(date.UTC())
		return
	}
	d.PlayCount = max(d.PlayCount, 1)
	if d.LastPlayed == nil {
		d.LastPlayed = &now
	}
}

// MarkUnplayed forgets that the item was played: its plays, its date and
// its resume point. Whether it is a favorite and its rating stay.
func (d *Data) MarkUnplayed() {
	d.Played = false
	d.PlayCount = 0
	d.Position = 0
	d.LastPlayed = nil
}

// Like rates the item the way a like or a dislike does.
func (d *Data) Like(likes bool) {
	rating := 1.0
	if likes {
		rating = 10
	}
	d.Rating = &rating
}

// PlayedPercentage is how far into the item the resume point is, and false
// when there is none or the runtime is unknown.
func (d Data) PlayedPercentage() (float64, bool) {
	if d.Position <= 0 || d.Runtime <= 0 {
		return 0, false
	}
	return float64(d.Position) / float64(d.Runtime) * 100, true
}
