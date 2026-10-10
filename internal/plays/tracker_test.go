package plays

import (
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// clock is a time tests move by hand.
type clock struct{ now time.Time }

func (c *clock) at() time.Time           { return c.now }
func (c *clock) advance(d time.Duration) { c.now = c.now.Add(d) }
func newClock() *clock                   { return &clock{now: time.Date(2026, 10, 10, 20, 0, 0, 0, time.UTC)} }
func (c *clock) tracker(t *testing.T) (*Tracker, *[]Change) {
	t.Helper()
	var told []Change
	tracker := NewTracker(func(change Change) { told = append(told, change) })
	tracker.now = c.at
	return tracker, &told
}

var (
	alice  = accounts.ID{0xa1}
	phone  = accounts.ID{0xd1}
	movie  = Title{Item: accounts.ID{0x11}, Kind: Movie, Name: "Night Train"}
	other  = Title{Item: accounts.ID{0x12}, Kind: Movie, Name: "Morning Boat"}
	report = func(kind int, title Title, position time.Duration, paused bool, method string) Report {
		return Report{Kind: kind, Device: phone, User: alice, UserName: "alice", App: "Web", DeviceName: "Phone", Title: title,
			Position: position, PositionKnown: true, Paused: paused, Method: method}
	}
)

// kinds lists the kinds of the changes told.
func kinds(changes []Change) []string {
	var found []string
	for _, c := range changes {
		found = append(found, c.Kind)
	}
	return found
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A playback counts the time it plays, not its pauses, and tells each
// change of state once, however many reports repeat it.
func TestPausesAreNotCountedAndEachChangeIsToldOnce(t *testing.T) {
	c := newClock()
	tracker, told := c.tracker(t)
	tracker.Report(report(ReportStart, movie, 0, false, ""))
	for range 3 {
		c.advance(10 * time.Second)
		tracker.Report(report(ReportProgress, movie, 0, false, ""))
	}
	// Paused for twenty minutes, the app still reporting.
	tracker.Report(report(ReportProgress, movie, 30*time.Second, true, ""))
	for range 120 {
		c.advance(10 * time.Second)
		tracker.Report(report(ReportProgress, movie, 30*time.Second, true, ""))
	}
	tracker.Report(report(ReportProgress, movie, 30*time.Second, false, ""))
	for range 2 {
		c.advance(10 * time.Second)
		tracker.Report(report(ReportProgress, movie, 50*time.Second, false, ""))
	}
	c.advance(5 * time.Second)
	tracker.Report(report(ReportStop, movie, 55*time.Second, false, ""))

	if want := []string{Started, Paused, Resumed, Stopped}; !equal(kinds(*told), want) {
		t.Fatalf("told %v, want %v", kinds(*told), want)
	}
	stopped := (*told)[3].Playback
	if stopped.Played != 55*time.Second {
		t.Errorf("played %v, want 55s: the pause is not played", stopped.Played)
	}
	if stopped.Position != 55*time.Second || !stopped.Ended.Equal(c.now) {
		t.Errorf("stopped at %v, %v; want 55s, %v", stopped.Position, stopped.Ended, c.now)
	}
}

// An app that closes without a stop report leaves its playback ended at
// its last report, once it has not reported for StaleAfter, with nothing
// counted after it.
func TestAStalePlaybackEndsAtItsLastReport(t *testing.T) {
	c := newClock()
	tracker, told := c.tracker(t)
	tracker.Report(report(ReportStart, movie, 0, false, ""))
	c.advance(10 * time.Second)
	tracker.Report(report(ReportProgress, movie, 10*time.Second, false, ""))
	last := c.now

	c.advance(StaleAfter - time.Second)
	tracker.Sweep()
	if len(*told) != 1 {
		t.Fatalf("told %v before the playback went stale", kinds(*told))
	}
	c.advance(time.Second)
	tracker.Sweep()
	if want := []string{Started, Stopped}; !equal(kinds(*told), want) {
		t.Fatalf("told %v, want %v", kinds(*told), want)
	}
	stopped := (*told)[1].Playback
	if !stopped.Ended.Equal(last) || stopped.Played != 10*time.Second {
		t.Errorf("ended at %v having played %v; want %v and 10s", stopped.Ended, stopped.Played, last)
	}

	// The same app reporting again starts a new playback.
	c.advance(time.Minute)
	tracker.Report(report(ReportProgress, movie, 20*time.Second, false, ""))
	if want := []string{Started, Stopped, Started}; !equal(kinds(*told), want) {
		t.Errorf("told %v, want %v", kinds(*told), want)
	}
}

// Time between reports counts up to maxGap: an app put to sleep while
// playing, which reports again hours later, does not count the hours.
func TestGapsBetweenReportsAreCapped(t *testing.T) {
	c := newClock()
	tracker, told := c.tracker(t)
	tracker.Report(report(ReportStart, movie, 0, false, ""))
	c.advance(4 * time.Minute)
	tracker.Report(report(ReportStop, movie, 0, false, ""))
	if got := (*told)[1].Playback.Played; got != maxGap {
		t.Errorf("played %v, want %v", got, maxGap)
	}
}

// A playback keeps the method that converted most, whatever the reports
// that follow, which may leave the method out.
func TestTheMethodIsKept(t *testing.T) {
	c := newClock()
	tracker, told := c.tracker(t)
	tracker.Report(report(ReportStart, movie, 0, false, DirectStream))
	c.advance(10 * time.Second)
	tracker.Report(report(ReportProgress, movie, 10*time.Second, false, ""))
	c.advance(10 * time.Second)
	tracker.Report(report(ReportStop, movie, 20*time.Second, false, ""))
	if got := (*told)[1].Playback.Method; got != DirectStream {
		t.Errorf("method %q, want %q", got, DirectStream)
	}

	tracker.Report(report(ReportStart, other, 0, false, DirectPlay))
	c.advance(10 * time.Second)
	tracker.Report(report(ReportProgress, other, 10*time.Second, false, Conversion))
	c.advance(10 * time.Second)
	tracker.Report(report(ReportProgress, other, 20*time.Second, false, DirectPlay))
	c.advance(10 * time.Second)
	tracker.Report(report(ReportStop, other, 30*time.Second, false, ""))
	if got := (*told)[3].Playback.Method; got != Conversion {
		t.Errorf("method %q, want %q: the playback was converted for a while", got, Conversion)
	}
}

// Another title reported by the same device ends the one it played.
func TestAnotherTitleEndsThePlaybackBefore(t *testing.T) {
	c := newClock()
	tracker, told := c.tracker(t)
	tracker.Report(report(ReportStart, movie, 0, false, ""))
	c.advance(20 * time.Second)
	tracker.Report(report(ReportProgress, other, 0, false, ""))
	if want := []string{Started, Stopped, Started}; !equal(kinds(*told), want) {
		t.Fatalf("told %v, want %v", kinds(*told), want)
	}
	if got := (*told)[1].Playback; got.Title.Item != movie.Item || got.Played != 20*time.Second {
		t.Errorf("stopped %s after %v, want %s after 20s", got.Title.Name, got.Played, movie.Name)
	}
}
