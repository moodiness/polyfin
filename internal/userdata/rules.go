package userdata

import "time"

// MinResumeDuration is the runtime under which an item is played as soon
// as it is past its start, as on a Jellyfin server with its default
// configuration (MinResumeDurationSeconds).
const MinResumeDuration = 5 * time.Minute

// Thresholds are how far into an item, in percent of its runtime, a
// position must be to keep a resume point (Resume) and to mark it played
// (Played): Jellyfin's MinResumePct and MaxResumePct, which the server
// settings choose. Resume is below Played.
type Thresholds struct {
	Resume, Played int
}

// Reaches reports whether position, against runtime, is far enough into
// an item for Reach to mark it played.
func (t Thresholds) Reaches(position, runtime time.Duration) bool {
	if runtime <= 0 {
		return false
	}
	percent := float64(position) / float64(runtime) * 100
	return percent >= float64(t.Resume) && (percent > float64(t.Played) || runtime < MinResumeDuration)
}

// Start records that playback began at now: one more play. As in Jellyfin
// 12.2, a played item that keeps a resume point (resumable: a video or an
// audiobook) is not dated by the start, only once a position gets past
// the start of it (see Reached): rewatching an episode and stopping at
// once does not move Next Up on from it.
func (d *Data) Start(now time.Time, resumable bool) {
	d.PlayCount++
	if !d.Played || !resumable {
		d.LastPlayed = &now
	}
}

// Reached dates the play at now after a reported position, as Jellyfin
// 12.2 does, when the position completed the item (completed, as Reach
// and the others report) or left a resume point.
func (d *Data) Reached(completed bool, now time.Time) {
	if completed || d.Position > 0 {
		d.LastPlayed = &now
	}
}

// Reach records the position a player reported while playing or when it
// stopped, against the runtime of what it played. Near the start (before
// the Resume threshold) there is nothing to resume; near the end (past the
// Played threshold), or anywhere past the start of a short item, the item
// is played and there is nothing to resume either. Without a runtime, the
// position is kept as it is. It reports whether the position completed the
// item.
func (d *Data) Reach(position, runtime time.Duration, thresholds Thresholds) bool {
	d.Runtime = runtime
	if runtime <= 0 {
		d.Position = max(position, 0)
		return false
	}
	percent := float64(position) / float64(runtime) * 100
	switch {
	case percent < float64(thresholds.Resume):
		d.Position = 0
	case thresholds.Reaches(position, runtime):
		d.Position = 0
		d.Played = true
		return true
	default:
		d.Position = position
	}
	return false
}

// AudiobookResume is how far into an audiobook a position must be to keep
// a resume point, and how near its end to mark it played, as on a
// Jellyfin server with its default configuration (MinAudiobookResume and
// MaxAudiobookResume, in minutes).
const AudiobookResume = 5 * time.Minute

// ReachSong records the position a player reported in a song, which keeps
// no resume point, as Jellyfin's songs do not: past the start, a song is
// played as an item of its runtime would be (see Reach).
func (d *Data) ReachSong(position, runtime time.Duration, thresholds Thresholds) bool {
	completed := d.Reach(position, runtime, thresholds)
	d.Position = 0
	return completed
}

// ReachAudiobook records the position a player reported in an audiobook,
// with Jellyfin's thresholds for audiobooks, in time rather than percent:
// nothing to resume in its first AudiobookResume, played in its last. It
// reports whether the position completed the book.
func (d *Data) ReachAudiobook(position, runtime time.Duration) bool {
	d.Runtime = runtime
	switch {
	case runtime <= 0:
		d.Position = max(position, 0)
	case position < AudiobookResume:
		d.Position = 0
	case runtime-position < AudiobookResume:
		d.Position = 0
		d.Played = true
		return true
	default:
		d.Position = position
	}
	return false
}

// Finish records a stop reported without a position at now: the item was
// played through, which counts as one more play.
func (d *Data) Finish(now time.Time) {
	d.Position = 0
	d.Played = true
	d.PlayCount++
	d.LastPlayed = &now
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

// ImportPlayed adds a played mark another service's watch history gives,
// watched at at (nil when the history gives no date): the item is played,
// at least once, on the later of its date and at; an earlier resume point
// goes, as the item was watched elsewhere after it was set here. Nothing
// already recorded is taken back. It reports whether the item was not
// played.
func (d *Data) ImportPlayed(at *time.Time) bool {
	was := d.Played
	d.Played = true
	d.PlayCount = max(d.PlayCount, 1)
	if at != nil && (d.LastPlayed == nil || d.LastPlayed.Before(*at)) {
		d.Position = 0
		d.LastPlayed = new(at.UTC())
	}
	return !was
}

// ImportedResume is a resume point another service keeps: at Position of
// Runtime when the service tells them, else at Percent of the item, set at
// At.
type ImportedResume struct {
	Percent           float64
	Position, Runtime time.Duration
	At                time.Time
}

// What ImportResume did with a resume point.
const (
	ResumeKept = iota
	ResumeSet
	ResumePlayed
)

// ImportResume adds a resume point another service keeps, under
// thresholds: none for a played item, nor when Polyfin's is newer; past
// the played threshold the item is played, before the resume one nothing
// is kept. It reports ResumeKept, ResumeSet or ResumePlayed.
func (d *Data) ImportResume(p ImportedResume, thresholds Thresholds) int {
	if d.Played || d.Position > 0 && d.LastPlayed != nil && !d.LastPlayed.Before(p.At) {
		return ResumeKept
	}
	position, runtime := p.Position, p.Runtime
	if runtime <= 0 {
		// The runtime Polyfin measured its own resume point against.
		runtime = d.Runtime
	}
	if position <= 0 && runtime > 0 {
		position = time.Duration(p.Percent / 100 * float64(runtime))
	}
	switch {
	case runtime > 0 && thresholds.Reaches(position, runtime),
		runtime <= 0 && p.Percent > float64(thresholds.Played):
		d.ImportPlayed(&p.At)
		return ResumePlayed
	case runtime <= 0 || float64(position)/float64(runtime)*100 < float64(thresholds.Resume):
		// Without a runtime there is no position to resume from.
		return ResumeKept
	}
	d.Position, d.Runtime = position, runtime
	d.LastPlayed = new(p.At.UTC())
	return ResumeSet
}
