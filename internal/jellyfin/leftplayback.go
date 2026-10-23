package jellyfin

import (
	"slices"
	"sync"

	"github.com/moodiness/polyfin/internal/accounts"
)

// maxTitleSessions bounds the play sessions remembered for a device's
// title: an app replaying a title, or changing its audio track, starts a
// new one each time.
const maxTitleSessions = 8

// titleSessions remembers, by device, the title it plays and the play
// sessions it reported for it, so that a device moving on to another title
// without stopping the one before ends the remuxes and conversions of that
// playback (see leaveTitle).
type titleSessions struct {
	mu    sync.Mutex
	plays map[accounts.ID]titlePlay
}

// titlePlay is the title a device plays, and the play sessions it
// reported for it, the latest last.
type titlePlay struct {
	title    accounts.ID
	sessions []string
}

func newTitleSessions() *titleSessions {
	return &titleSessions{plays: map[accounts.ID]titlePlay{}}
}

// leaveTitle notes, from a report of the device's playback, the play
// session it reports for the title it plays. A device that reports another
// title than the one before, without having stopped it, left that title:
// the remuxes and conversions of the play sessions it reported for it stop,
// as the stop it did not send would have stopped them, rather than reading
// their source until they are idle. A replay of the same title, as for
// another audio track, stops nothing: the app stops what it leaves. Only
// the device's own playbacks are concerned, not those of its other
// devices, of DLNA devices, of guests or of converted downloads.
func (h *Handler) leaveTitle(device accounts.ID, event playbackEvent, session string) {
	h.titles.mu.Lock()
	if event == playbackStopped {
		// The stop stopped its remuxes.
		if played, ok := h.titles.plays[device]; ok {
			played.sessions = slices.DeleteFunc(played.sessions, func(s string) bool { return s == session })
			h.titles.plays[device] = played
		}
		h.titles.mu.Unlock()
		return
	}
	h.titles.mu.Unlock()
	playing, ok := h.sessions.Playing(device)
	if !ok {
		return
	}
	title := h.titleOf(playing.Item)
	h.titles.mu.Lock()
	played := h.titles.plays[device]
	var left []string
	if played.title != title {
		left = slices.DeleteFunc(played.sessions, func(s string) bool { return s == playing.PlaySessionID })
		played = titlePlay{title: title}
	}
	if playing.PlaySessionID != "" && !slices.Contains(played.sessions, playing.PlaySessionID) {
		played.sessions = append(played.sessions, playing.PlaySessionID)
		if len(played.sessions) > maxTitleSessions {
			played.sessions = slices.Delete(played.sessions, 0, len(played.sessions)-maxTitleSessions)
		}
	}
	h.titles.plays[device] = played
	h.titles.mu.Unlock()
	for _, session := range left {
		h.Playback.StopRemux(session)
	}
}
