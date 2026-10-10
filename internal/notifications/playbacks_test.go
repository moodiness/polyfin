package notifications

import (
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/plays"
)

// Each change of a playback is told once, however many reports repeat it,
// to the targets of the user who plays and the server's that chose it; not
// to other users' targets, nor to targets that did not choose it, as
// targets saved before these events existed, until they do.
func TestPlaybacksAreToldOnceToTheirUserAndTheServer(t *testing.T) {
	h := newHarness(t)
	all := []string{PlaybackStarted, PlaybackPaused, PlaybackResumed, PlaybackStopped}
	h.add(t, &h.member, h.webhook("/member", all...))
	h.add(t, &h.admin, h.webhook("/admin", all...))
	h.add(t, nil, h.webhook("/server", all...))
	existing := h.add(t, nil, h.webhook("/existing", NewEpisode, RecordingFinished))

	tracker := plays.NewTracker(h.Playback)
	title := plays.Title{Item: accounts.ID{0x21}, Kind: plays.Episode, Name: "Pilot", SeriesID: accounts.ID{0x20}, SeriesName: "Harbour",
		Season: 1, Episode: 2}
	report := func(kind int, position time.Duration, paused bool) {
		tracker.Report(plays.Report{Kind: kind, Device: accounts.ID{0xd1}, User: h.member.ID, UserName: h.member.Name, App: "Web",
			DeviceName: "Phone", Title: title, Position: position, PositionKnown: true, Paused: paused, Method: plays.Conversion})
	}
	report(plays.ReportStart, 0, false)
	report(plays.ReportProgress, 10*time.Second, false)
	report(plays.ReportProgress, 20*time.Second, false)
	report(plays.ReportProgress, 21*time.Second, true)
	report(plays.ReportProgress, 21*time.Second, true)
	report(plays.ReportProgress, 21*time.Second, false)
	report(plays.ReportProgress, 31*time.Second, false)
	report(plays.ReportStop, 35*time.Second, false)

	for _, path := range []string{"/member", "/server"} {
		got := h.targets.wait(t, path, 4)
		var kinds []string
		for _, r := range got {
			kinds = append(kinds, r.body["type"].(string))
		}
		if !slices.Equal(kinds, all) {
			t.Errorf("%s: %v, want %v", path, kinds, all)
		}
		playback, _ := got[3].body["playback"].(map[string]any)
		user, _ := got[3].body["user"].(map[string]any)
		if user["name"] != "member" || playback["seriesName"] != "Harbour" || playback["position"] != float64(35) ||
			playback["converted"] != true || playback["device"] != "Phone" {
			t.Errorf("%s: the stop says %v of %v, want member's Harbour, converted, stopped at 35 s on Phone", path, playback, user)
		}
	}
	h.idle(t)
	if got := len(h.targets.at("/admin")) + len(h.targets.at("/existing")); got != 0 {
		t.Errorf("%d messages reached another user's target or one that did not choose playbacks", got)
	}

	if _, err := h.Update(t.Context(), nil, existing.ID, Draft{Events: []string{NewEpisode, PlaybackStarted}}); err != nil {
		t.Fatal(err)
	}
	report(plays.ReportStart, 0, false)
	if got := h.targets.wait(t, "/existing", 1); got[0].body["type"] != PlaybackStarted {
		t.Errorf("the target that chose starts received %v", got[0].body["type"])
	}
}
