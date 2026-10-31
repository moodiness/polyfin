package plays

import (
	"bytes"
	"encoding/csv"
	"testing"
	"time"
)

// Each playback keeps the bytes its device was sent from the end of the
// previous one to its own end: the start of the file fetched before the
// start report counts, and so do the bytes sent before the stop report.
// A playback ended by another title's report hands its device's total to
// the next. A meter that started again counts on.
func TestPlaybacksKeepTheBytesSentForThem(t *testing.T) {
	c := newClock()
	tracker, told := c.tracker(t)
	sent := func(kind int, title Title, total int64) {
		r := report(kind, title, 0, false, DirectPlay)
		r.Sent = total
		tracker.Report(r)
		c.advance(10 * time.Second)
	}
	// stopped is the playback that stopped last.
	stopped := func() Playback {
		t.Helper()
		for i := len(*told) - 1; i >= 0; i-- {
			if (*told)[i].Kind == Stopped {
				return (*told)[i].Playback
			}
		}
		t.Fatal("nothing stopped")
		return Playback{}
	}
	// 4 MB fetched before the start report.
	sent(ReportStart, movie, 4_000_000)
	sent(ReportProgress, movie, 30_000_000)
	sent(ReportStop, movie, 50_000_000)
	if got := stopped().Sent; got != 50_000_000 {
		t.Errorf("first playback: %d bytes", got)
	}
	// The next one counts from there, its prefetch included.
	sent(ReportStart, other, 52_000_000)
	sent(ReportProgress, other, 60_000_000)
	// Another title's report ends it with what it was last told: 2 MB
	// before its start report, 8 MB after.
	sent(ReportStart, movie, 70_000_000)
	if got := stopped().Sent; got != 10_000_000 {
		t.Errorf("playback ended by another: %d bytes", got)
	}
	// The meter, forgotten while paused, starts again from nothing: 15 MB
	// before, 2 MB after.
	sent(ReportProgress, movie, 75_000_000)
	sent(ReportProgress, movie, 1_000_000)
	sent(ReportStop, movie, 3_000_000)
	if got := stopped().Sent; got != 17_000_000 {
		t.Errorf("after the meter started again: %d bytes", got)
	}
}

// A guest's playback counts its own meter from nothing, and leaves no
// total behind for its device.
func TestGuestPlaybacksCountTheirOwnBytes(t *testing.T) {
	c := newClock()
	tracker, told := c.tracker(t)
	r := report(ReportStart, movie, 0, false, DirectStream)
	r.Shared, r.Sent = true, 2_000_000
	tracker.Report(r)
	r.Kind, r.Sent = ReportStop, 9_000_000
	tracker.Report(r)
	if last := (*told)[len(*told)-1]; last.Kind != Stopped || last.Playback.Sent != 9_000_000 {
		t.Errorf("guest: %s with %d bytes", last.Kind, last.Playback.Sent)
	}
	if len(tracker.sentEnded) != 0 {
		t.Errorf("totals kept: %v", tracker.sentEnded)
	}
}

// The history keeps the bytes sent for each playback, none for one whose
// bytes did not pass through Polyfin; the statistics sum them, and the
// export writes them, empty where none were sent.
func TestHistoryKeepsTheBytesSent(t *testing.T) {
	h := newHistoryHarness(t)
	record := func(title Title, ago time.Duration, sent int64) {
		start := h.now.Add(-ago)
		h.Record(t.Context(), Change{Kind: Stopped, Playback: Playback{User: h.alice.ID, UserName: "alice", App: "Web", DeviceName: "Phone",
			Title: title, Started: start, LastReport: start.Add(time.Hour), Ended: start.Add(time.Hour), Played: time.Hour,
			Method: DirectPlay, Sent: sent}})
	}
	record(movie, 3*time.Hour, 1_500_000_000)
	record(pilot, 2*time.Hour, 0)
	record(other, 100*24*time.Hour, 700_000_000)

	week, err := h.Statistics(t.Context(), Filter{Since: h.now.Add(-7 * 24 * time.Hour)}, Day, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if week.Sent != 1_500_000_000 {
		t.Errorf("the week sent %d bytes", week.Sent)
	}
	all, err := h.Statistics(t.Context(), Filter{}, Month, time.UTC)
	if err != nil || all.Sent != 2_200_000_000 {
		t.Errorf("all the history sent %d bytes: %v", all.Sent, err)
	}
	entries, _, err := h.Entries(t.Context(), Filter{}, 0, 10)
	if err != nil || len(entries) != 3 || entries[0].Sent != 0 || entries[1].Sent != 1_500_000_000 {
		t.Fatalf("entries: %+v %v", entries, err)
	}
	var out bytes.Buffer
	if err := h.WriteCSV(t.Context(), &out, Filter{}); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&out).ReadAll()
	if err != nil || len(rows) != 4 {
		t.Fatalf("CSV: %v %v", rows, err)
	}
	last := len(rows[0]) - 1
	if rows[0][last] != "sent_bytes" || rows[1][last] != "" || rows[2][last] != "1500000000" || rows[3][last] != "700000000" {
		t.Errorf("sent_bytes column: %q %q %q %q", rows[0][last], rows[1][last], rows[2][last], rows[3][last])
	}
}
