package jellyfin

import (
	"slices"
	"sync"

	"github.com/moodiness/polyfin/internal/accounts"
)

// maxTitleSessions bounds the play sessions remembered for a device's
// title: an app replaying a title, or changing its audio track, starts a
// new one each time. maxSeenSessions bounds those remembered of every
// title, to know a late report of one the device left.
const (
	maxTitleSessions = 8
	maxSeenSessions  = 16
)

// titleSessions remembers, by device, the title it plays and the play
// sessions it reported for it, so that a device moving on to another title
// without stopping the one before ends the repackaging and conversions of
// that playback (see leaveTitle).
type titleSessions struct {
	mu    sync.Mutex
	plays map[accounts.ID]titlePlay
}

// titlePlay is the title a device plays, and the play sessions it
// reported for it, the latest last; latest is the play session of its
// latest playback, and seen the play sessions it reported, of any title,
// the latest last.
type titlePlay struct {
	title    accounts.ID
	sessions []string
	latest   string
	seen     []string
}

func newTitleSessions() *titleSessions {
	return &titleSessions{plays: map[accounts.ID]titlePlay{}}
}

// leaveTitle notes, from a report of the device's playback, the play
// session it reports for the title it plays. A device that reports another
// title than the one before, without having stopped it, left that title:
// the repackaging and conversions of the play sessions it reported for it
// stop, as the stop it did not send would have stopped them, rather than
// reading their source until they are idle. A replay of the same title, as
// for another audio track, stops nothing: the app stops what it leaves.
// Only the device's own playbacks are concerned, not those of its other
// devices, of DLNA devices, of guests or of converted downloads.
//
// Only a report of the device's latest playback may change its title: a
// start, a report of its latest play session, or of one it never reported,
// as from an app that sends no start. A late report of a play session the
// device reported before, of a title it left since, changes nothing and
// stops nothing: current is false then. switched tells that the report
// changed the device's title.
func (h *Handler) leaveTitle(device accounts.ID, event playbackEvent, session string) (current, switched bool) {
	h.titles.mu.Lock()
	if event == playbackStopped {
		// The stop stopped its conversions.
		if played, ok := h.titles.plays[device]; ok {
			played.sessions = slices.DeleteFunc(played.sessions, func(s string) bool { return s == session })
			h.titles.plays[device] = played
		}
		h.titles.mu.Unlock()
		return true, false
	}
	h.titles.mu.Unlock()
	playing, ok := h.sessions.Playing(device)
	if !ok {
		return true, false
	}
	title := h.titleOf(playing.Item)
	h.titles.mu.Lock()
	played, known := h.titles.plays[device]
	sameTitle := known && played.title == title
	switch {
	case session == played.latest, sameTitle:
	case session == "":
		// A start, or an app that names no play session.
		if event != playbackStarted && played.latest != "" {
			h.titles.mu.Unlock()
			return false, false
		}
	case slices.Contains(played.seen, session):
		// A late report of a title the device left.
		h.titles.mu.Unlock()
		return false, false
	}
	fresh := session != "" && !slices.Contains(played.seen, session)
	if fresh {
		played.seen = append(played.seen, session)
		if len(played.seen) > maxSeenSessions {
			played.seen = slices.Delete(played.seen, 0, len(played.seen)-maxSeenSessions)
		}
	}
	if !sameTitle || fresh || event == playbackStarted {
		played.latest = session
	}
	var left []string
	if !sameTitle {
		left = slices.DeleteFunc(played.sessions, func(s string) bool { return s == session })
		played.title, played.sessions = title, nil
	}
	if session != "" && !slices.Contains(played.sessions, session) {
		played.sessions = append(played.sessions, session)
		if len(played.sessions) > maxTitleSessions {
			played.sessions = slices.Delete(played.sessions, 0, len(played.sessions)-maxTitleSessions)
		}
	}
	h.titles.plays[device] = played
	h.titles.mu.Unlock()
	for _, session := range left {
		h.Playback.StopRemux(session)
	}
	return true, !sameTitle
}
