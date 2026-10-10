// Package plays follows what users play, from the reports Jellyfin apps
// send, and keeps a history of the videos played for the statistics.
//
// The Tracker turns the reports into playbacks, which start, pause, resume
// and stop, and tells each change once: the notifications send them, and
// the History keeps the videos that stopped.
package plays

import (
	"context"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The kinds of what is played.
const (
	Movie     = "movie"
	Episode   = "episode"
	Channel   = "channel"
	Recording = "recording"
	Replay    = "replay"
	Song      = "song"
	Audiobook = "audiobook"
)

// Video reports whether kind is a video, which the history keeps; songs
// and audiobooks are only told.
func Video(kind string) bool {
	switch kind {
	case Movie, Episode, Channel, Recording, Replay:
		return true
	}
	return false
}

// How a playback reaches its app, from the least to the most work for the
// server.
const (
	// DirectPlay: the app reads the file as it is.
	DirectPlay = "direct_play"
	// DirectStream: the file is remuxed for the app, its tracks copied.
	DirectStream = "direct_stream"
	// Conversion: the video or the audio is converted.
	Conversion = "conversion"
)

// Methods lists the play methods, from the least to the most work.
var Methods = []string{DirectPlay, DirectStream, Conversion}

// heavier returns the method of a and b that converts more: a playback
// that was converted for a while counts as converted.
func heavier(a, b string) string {
	rank := func(method string) int {
		for i, m := range Methods {
			if m == method {
				return i + 1
			}
		}
		return 0
	}
	if rank(b) > rank(a) {
		return b
	}
	return a
}

// Title is what is played, as it was called when it played: its item, its
// kind (see Movie and the others), its name, its series, season and number
// for an episode, its channel for a channel, a Replay programme or a
// recording, and its artist for a song.
type Title struct {
	Item        accounts.ID
	Kind        string
	Name        string
	SeriesID    accounts.ID
	SeriesName  string
	Season      int
	Episode     int
	ChannelID   accounts.ID
	ChannelName string
	Artist      string
}

// The kinds of reports.
const (
	ReportStart = iota
	ReportProgress
	ReportStop
)

// Report is what an app reported about what it plays.
type Report struct {
	// Kind is ReportStart, ReportProgress or ReportStop.
	Kind int
	// Device is the sign-in the report comes from: one device plays one
	// thing at a time.
	Device   accounts.ID
	User     accounts.ID
	UserName string
	// App and DeviceName name the app and the device, as they signed in.
	App        string
	DeviceName string
	Title      Title
	// Position is where the playback is, when PositionKnown.
	Position      time.Duration
	PositionKnown bool
	Paused        bool
	// Method is how the playback reaches the app, empty when the report
	// does not tell.
	Method string
}

// Playback is one playback of a title on a device, from its start to its
// end.
type Playback struct {
	User       accounts.ID
	UserName   string
	App        string
	DeviceName string
	Title      Title
	Started    time.Time
	// LastReport is when the app last reported it; Ended is when it ended,
	// zero until then.
	LastReport time.Time
	Ended      time.Time
	// Played is how long it played, its pauses left out.
	Played   time.Duration
	Position time.Duration
	Paused   bool
	// Method is the method that converted most while it played (see
	// Methods), empty when no report told.
	Method string
}

// The changes of a playback.
const (
	Started = "started"
	Paused  = "paused"
	Resumed = "resumed"
	Stopped = "stopped"
)

// Change is a playback that started, paused, resumed or stopped, as it was
// then.
type Change struct {
	Kind     string
	Playback Playback
}

// At is when the change happened.
func (c Change) At() time.Time {
	switch c.Kind {
	case Started:
		return c.Playback.Started
	case Stopped:
		return c.Playback.Ended
	}
	return c.Playback.LastReport
}

const (
	// StaleAfter is how long after its last report a playback is ended,
	// at that report, as Jellyfin ends playbacks that stop reporting: an
	// app that closes without a stop report leaves one behind.
	StaleAfter = 5 * time.Minute
	// maxGap bounds the time counted played between two reports: an app
	// that went to sleep while playing does not count the night.
	maxGap = 2 * time.Minute
	// sweepEvery is how often stale playbacks are looked for.
	sweepEvery = 30 * time.Second
)

// Tracker follows the playback of each device.
type Tracker struct {
	now    func() time.Time
	listen func(Change)

	mu      sync.Mutex
	playing map[accounts.ID]*Playback
	// telling keeps the changes told in order: it is taken before mu is
	// released.
	telling sync.Mutex
}

// NewTracker returns a tracker that tells listen each change, one at a
// time, in order.
func NewTracker(listen func(Change)) *Tracker {
	return &Tracker{now: time.Now, listen: listen, playing: map[accounts.ID]*Playback{}}
}

// Report applies an app's report. A start, or a report of another title
// than the one the device plays, ends the one it played; a report while
// the device plays nothing starts a playback, as when Polyfin started
// after the app's start report; a stop of what the device does not play is
// ignored. Reports of a user's position tell nothing; only the changes of
// state do.
func (t *Tracker) Report(r Report) {
	if r.User == (accounts.ID{}) || r.Title.Item == (accounts.ID{}) {
		return
	}
	t.mu.Lock()
	now := t.now()
	var changes []Change
	p := t.playing[r.Device]
	if p != nil && p.Title.Item != r.Title.Item {
		changes = append(changes, t.end(r.Device, p, now))
		p = nil
	}
	switch {
	case r.Kind == ReportStop:
		if p != nil {
			p.apply(r)
			changes = append(changes, t.end(r.Device, p, now))
		}
	case p == nil:
		p = &Playback{User: r.User, UserName: r.UserName, App: r.App, DeviceName: r.DeviceName, Title: r.Title, Started: now,
			LastReport: now, Paused: r.Paused}
		p.apply(r)
		t.playing[r.Device] = p
		changes = append(changes, Change{Kind: Started, Playback: *p})
	default:
		wasPaused := p.Paused
		advance(p, now)
		p.apply(r)
		p.Paused = r.Paused
		switch {
		case p.Paused && !wasPaused:
			changes = append(changes, Change{Kind: Paused, Playback: *p})
		case !p.Paused && wasPaused:
			changes = append(changes, Change{Kind: Resumed, Playback: *p})
		}
	}
	t.tell(changes)
}

// apply takes what a report tells of a playback: its position and method.
func (p *Playback) apply(r Report) {
	if r.PositionKnown {
		p.Position = max(r.Position, 0)
	}
	p.Method = heavier(p.Method, r.Method)
}

// advance counts the time played since the last report, unless paused,
// up to maxGap.
func advance(p *Playback, now time.Time) {
	if !p.Paused {
		p.Played += min(max(now.Sub(p.LastReport), 0), maxGap)
	}
	p.LastReport = now
}

// end ends the playback p of device now, after counting the time played
// since its last report. It is called with t.mu held.
func (t *Tracker) end(device accounts.ID, p *Playback, now time.Time) Change {
	advance(p, now)
	p.Ended = now
	delete(t.playing, device)
	return Change{Kind: Stopped, Playback: *p}
}

// tell releases t.mu and tells changes, in order.
func (t *Tracker) tell(changes []Change) {
	t.telling.Lock()
	t.mu.Unlock()
	defer t.telling.Unlock()
	for _, c := range changes {
		t.listen(c)
	}
}

// Sweep ends the playbacks whose app stopped reporting StaleAfter ago, at
// their last report.
func (t *Tracker) Sweep() {
	t.mu.Lock()
	now := t.now()
	var changes []Change
	for device, p := range t.playing {
		if now.Sub(p.LastReport) >= StaleAfter {
			changes = append(changes, t.endStale(device, p))
		}
	}
	t.tell(changes)
}

// endStale ends p at its last report, counting nothing after it. It is
// called with t.mu held.
func (t *Tracker) endStale(device accounts.ID, p *Playback) Change {
	p.Ended = p.LastReport
	delete(t.playing, device)
	return Change{Kind: Stopped, Playback: *p}
}

// Run sweeps the stale playbacks until ctx ends.
func (t *Tracker) Run(ctx context.Context) {
	ticker := time.NewTicker(sweepEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.Sweep()
		}
	}
}

// Close ends every playback at its last report, as Polyfin stops.
func (t *Tracker) Close() {
	t.mu.Lock()
	var changes []Change
	for device, p := range t.playing {
		changes = append(changes, t.endStale(device, p))
	}
	t.tell(changes)
}
