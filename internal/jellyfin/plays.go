package jellyfin

import (
	"cmp"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/plays"
)

// playsReports are the kinds of tracker reports, by the kind of playback
// report.
var playsReports = map[playbackEvent]int{
	playbackStarted:    plays.ReportStart,
	playbackProgressed: plays.ReportProgress,
	playbackStopped:    plays.ReportStop,
}

// followPlayback tells the playbacks Polyfin follows (see Options.Plays)
// of a report about item. playing is what the device was known to play
// before it.
func (h *Handler) followPlayback(user accounts.User, device accounts.Device, event playbackEvent, state playback.PlayState, positionKnown bool,
	playing playback.NowPlaying, item library.Item) {
	if h.Plays == nil {
		return
	}
	if playing.Item == state.Item || state.Item == (accounts.ID{}) {
		state.PlayMethod = cmp.Or(state.PlayMethod, playing.PlayMethod)
		state.PlaySessionID = cmp.Or(state.PlaySessionID, playing.PlaySessionID)
	}
	h.Plays.Report(plays.Report{
		Kind:          playsReports[event],
		Device:        device.ID,
		User:          user.ID,
		UserName:      user.Name,
		App:           device.Client,
		DeviceName:    device.DeviceName,
		Title:         playedTitle(item),
		Position:      state.Position,
		PositionKnown: positionKnown,
		Paused:        state.Paused,
		Method:        h.playMethod(state),
	})
}

// playMethod tells how a playback reaches its app: as the app reports it,
// except that apps report the streams Polyfin sends over HLS as converted,
// even those it only remuxes: the encodings of the play session tell
// which, while they run. It is empty when neither tells.
func (h *Handler) playMethod(state playback.PlayState) string {
	switch state.PlayMethod {
	case "DirectPlay":
		return plays.DirectPlay
	case "DirectStream":
		return plays.DirectStream
	case "Transcode":
	default:
		return ""
	}
	if state.PlaySessionID == "" || h.Playback == nil {
		return ""
	}
	method := ""
	for _, running := range h.Playback.Encodings() {
		if running.Key.Session != state.PlaySessionID || !running.Opened {
			continue
		}
		if running.Video != nil || running.AudioCodec != "" {
			return plays.Conversion
		}
		method = plays.DirectStream
	}
	return method
}

// playedTitle describes item for the playbacks Polyfin follows.
func playedTitle(item library.Item) plays.Title {
	t := plays.Title{Item: item.ID, Name: item.Name}
	switch item.Kind {
	case library.KindMovie:
		t.Kind = plays.Movie
	case library.KindEpisode:
		t.Kind = plays.Episode
		t.SeriesID, t.SeriesName, t.Season, t.Episode = item.SeriesID, item.SeriesName, item.ParentIndexNumber, item.IndexNumber
	case library.KindChannel:
		t.Kind = plays.Channel
		t.ChannelID, t.ChannelName = item.ID, item.Name
	case library.KindRecording:
		t.Kind = plays.Recording
	case library.KindReplay:
		t.Kind = plays.Replay
	case library.KindTrack:
		t.Kind = plays.Song
	case library.KindAudiobook:
		t.Kind = plays.Audiobook
	}
	if item.Channel != nil {
		t.ChannelID, t.ChannelName = item.Channel.ID, item.Channel.Name
	}
	switch {
	case len(item.Artists) > 0:
		t.Artist = item.Artists[0].Name
	case item.AlbumArtist != nil:
		t.Artist = item.AlbumArtist.Name
	}
	return t
}
