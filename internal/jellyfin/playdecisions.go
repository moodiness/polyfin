package jellyfin

import (
	"slices"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/logs"
	"github.com/moodiness/polyfin/internal/playback"
)

// The last plays of each movie or episode are kept in memory, for the
// admin app's Versions page: which versions each play tried, in order,
// what came of each, and the one it chose. A restart forgets them.

const (
	// decisionsKept is how many plays of a title are kept, the latest.
	decisionsKept = 5
	// decisionTitles is how many titles' plays are kept, and decisionLife
	// how long after their last play.
	decisionTitles = 500
	decisionLife   = 24 * time.Hour
)

// What came of a version a play tried, as PlayTry tells it.
const (
	// TryPlays: the version plays on the app.
	TryPlays = "plays"
	// TryUnreadable: the version could not be analyzed; Reason says why.
	TryUnreadable = "unreadable"
	// TryUnstarted: the version is a torrent that did not start in time.
	TryUnstarted = "unstarted"
	// TryTorrentCap: as many torrents ran as Torrents at once allows.
	TryTorrentCap = "torrentCap"
	// TryConversionCap: the version needed its video converted while the
	// server converted as many as it may.
	TryConversionCap = "conversionCap"
	// TryUnplayable: the version needs a conversion the user may not
	// have, or is above their bitrate limit or quality group, or cannot be
	// streamed.
	TryUnplayable = "unplayable"
	// TryNotAnalyzed: past the versions a play analyzes, the version was
	// not analyzed before, so it was not tried.
	TryNotAnalyzed = "notAnalyzed"
	// TryPassed: the version was still being read when the play chose.
	TryPassed = "passed"
)

// How a play reaches the app, as PlayDecision tells it.
const (
	DeliveryDirectPlay   = "directPlay"
	DeliveryDirectStream = "directStream"
	DeliveryConverted    = "converted"
)

// PlayDecision is a play of a title: when, by whom and on which device
// and app, whether the app picked the version in its version menu, the
// versions it tried, and the one it chose, zero when none played, with how
// it reaches the app.
type PlayDecision struct {
	At          time.Time
	User        string
	Device, App string
	Picked      bool
	Tries       []PlayTry
	Chosen      accounts.ID
	Delivery    string
}

// PlayTry is a version a play tried, and what came of it (see TryPlays),
// with why for TryUnreadable and TryUnstarted, without the source's
// address.
type PlayTry struct {
	Version accounts.ID
	Name    string
	Outcome string
	Reason  string
}

// playDecisions keeps the last plays of each title; mu orders the changes
// of a title's plays.
type playDecisions struct {
	kept *cache.Cache[accounts.ID, []PlayDecision]
	mu   sync.Mutex
}

func newPlayDecisions() *playDecisions {
	return &playDecisions{kept: cache.New[accounts.ID, []PlayDecision](decisionTitles, decisionLife)}
}

// PlayDecisions returns the last plays of a movie or an episode, the latest
// first.
func (h *Handler) PlayDecisions(item accounts.ID) []PlayDecision {
	decisions, _ := h.decisions.kept.Get(item)
	return slices.Clone(decisions)
}

// recordDecision keeps a play of item, dropping the oldest past
// decisionsKept.
func (h *Handler) recordDecision(item accounts.ID, decision PlayDecision) {
	h.decisions.mu.Lock()
	defer h.decisions.mu.Unlock()
	kept, _ := h.decisions.kept.Get(item)
	kept = slices.Insert(slices.Clone(kept[:min(len(kept), decisionsKept-1)]), 0, decision)
	h.decisions.kept.Put(item, kept)
}

// playTries notes what came of each candidate of a play, by its place in
// the order tried; the tries run at once, and may end after the play
// chose.
type playTries struct {
	mu    sync.Mutex
	tries []PlayTry
}

func newPlayTries(versions []library.Version, order []int) *playTries {
	t := &playTries{tries: make([]PlayTry, len(order))}
	for n, i := range order {
		t.tries[n] = PlayTry{Version: versions[i].ID, Name: versions[i].Name}
	}
	return t
}

// note tells what came of the candidate n; err, for an unreadable one,
// is told without addresses or keys.
func (t *playTries) note(n int, outcome string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tries[n].Outcome = outcome
	if err != nil {
		t.tries[n].Reason = logs.Redact(err.Error())
	}
}

// reached returns the candidates a play reached, the first started of
// them: those whose outcome is not known yet are TryPassed.
func (t *playTries) reached(started int) []PlayTry {
	t.mu.Lock()
	defer t.mu.Unlock()
	var reached []PlayTry
	for n, try := range t.tries {
		switch {
		case try.Outcome != "":
		case n < started:
			try.Outcome = TryPassed
		default:
			continue
		}
		reached = append(reached, try)
	}
	return reached
}

// delivery tells how a decision reaches the app.
func delivery(decision playback.Decision) string {
	switch {
	case decision.DirectPlay:
		return DeliveryDirectPlay
	case decision.Video == nil && decision.Audio == nil:
		return DeliveryDirectStream
	}
	return DeliveryConverted
}
