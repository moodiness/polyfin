package notifications

import (
	"fmt"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/plays"
)

// playbackEvents are the events of the changes of playbacks.
var playbackEvents = map[string]string{
	plays.Started: PlaybackStarted,
	plays.Paused:  PlaybackPaused,
	plays.Resumed: PlaybackResumed,
	plays.Stopped: PlaybackStopped,
}

// Playback tells the targets of the user who plays, and the server's, that
// a playback started, paused, resumed or stopped. The tracker tells each
// change once (see plays.Tracker); it returns at once.
func (s *Service) Playback(c plays.Change) {
	if _, ok := playbackEvents[c.Kind]; !ok {
		return
	}
	ev := s.playbackEvent(c)
	user := c.Playback.User
	s.dispatch(ev, func(t target) bool { return t.owner == nil || *t.owner == user })
}

// playbackEvent tells of a change of a playback, in the server language.
func (s *Service) playbackEvent(c plays.Change) Event {
	ev := s.newEvent(playbackEvents[c.Kind])
	ev.At = c.At().UTC()
	p, t := c.Playback, c.Playback.Title
	ev.User = &UserJSON{ID: p.User.String(), Name: p.UserName}
	j := &PlaybackJSON{ItemID: t.Item.String(), Kind: t.Kind, Name: t.Name, App: p.App, Device: p.DeviceName,
		Position: int64(p.Position.Seconds()), Paused: p.Paused, Converted: p.Method == plays.Conversion, StartedAt: p.Started.UTC(),
		Played: int64(p.Played.Seconds())}
	if t.Kind == plays.Episode {
		j.SeriesName, j.Season, j.Number = &t.SeriesName, &t.Season, &t.Episode
		if t.SeriesID != (accounts.ID{}) {
			j.SeriesID = new(t.SeriesID.String())
		}
	}
	if t.ChannelID != (accounts.ID{}) {
		j.ChannelID, j.ChannelName = new(t.ChannelID.String()), &t.ChannelName
	}
	if t.Artist != "" {
		j.Artist = &t.Artist
	}
	if p.Method != "" {
		j.Method = &p.Method
	}
	ev.Playback = j

	name := playedName(t)
	switch c.Kind {
	case plays.Started:
		if plays.Video(t.Kind) {
			ev.Title = s.phrase("%s is watching %s", "%s regarde %s", p.UserName, name)
		} else {
			ev.Title = s.phrase("%s is listening to %s", "%s écoute %s", p.UserName, name)
		}
	case plays.Paused:
		ev.Title = s.phrase("%s paused %s", "%s a mis %s en pause", p.UserName, name)
	case plays.Resumed:
		ev.Title = s.phrase("%s resumed %s", "%s a repris %s", p.UserName, name)
	case plays.Stopped:
		ev.Title = s.phrase("%s stopped %s", "%s a arrêté %s", p.UserName, name)
	}
	var parts []string
	if t.Kind == plays.Episode && t.Name != "" {
		parts = append(parts, t.Name)
	}
	switch {
	case p.App != "" && p.DeviceName != "":
		parts = append(parts, s.phrase("%s on %s", "%s sur %s", p.App, p.DeviceName))
	case p.App != "" || p.DeviceName != "":
		parts = append(parts, p.App+p.DeviceName)
	}
	if t.Kind != plays.Channel {
		parts = append(parts, clock(p.Position))
	}
	switch p.Method {
	case plays.DirectPlay:
		parts = append(parts, s.phrase("Direct play", "Lecture directe"))
	case plays.DirectStream:
		parts = append(parts, s.phrase("Remux", "Remux"))
	case plays.Conversion:
		parts = append(parts, s.phrase("Conversion", "Conversion"))
	}
	ev.Message = strings.Join(parts, " · ")
	ev.URL = s.webLink(t.Item)
	return ev
}

// playedName names what plays in a message: an episode by its series,
// season and number, a song by its artist and name, anything else by its
// name.
func playedName(t plays.Title) string {
	switch {
	case t.Kind == plays.Episode && t.SeriesName != "":
		return fmt.Sprintf("%s S%02dE%02d", t.SeriesName, t.Season, t.Episode)
	case t.Kind == plays.Song && t.Artist != "":
		return t.Artist + " - " + t.Name
	}
	return t.Name
}

// clock writes a position as 1:02:03, or 2:03 under an hour.
func clock(d time.Duration) string {
	total := int(d.Seconds())
	if total >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", total/3600, total/60%60, total%60)
	}
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}
