package jellyfin

import (
	"cmp"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/plays"
)

// refusePlay answers a PlaybackInfo that plays nothing with code, as
// Jellyfin does (NoCompatibleStream, or RateLimitExceeded, which
// jellyfin-web tells as media that cannot be played at this time), and
// tells PlayFailed of it for reason. item is the title opened, empty when
// it is not known.
func (h *Handler) refusePlay(w http.ResponseWriter, r *http.Request, user accounts.User, item library.Item, code, reason string) {
	writeJSON(w, http.StatusOK, noCompatibleStream{MediaSources: []MediaSourceInfo{}, ErrorCode: code})
	h.playFailed(r, user, callerFrom(r.Context()).Device, item, reason)
}

// streamFailed tells PlayFailed that the stream of item broke for user,
// on the app of the device whose identifier deviceID is, as the stream's
// address carries it: stream requests carry no other credentials.
func (h *Handler) streamFailed(r *http.Request, user accounts.User, item library.Item, deviceID string) {
	if h.PlayFailed == nil {
		return
	}
	device := callerFrom(r.Context()).Device
	if device.DeviceID == "" && deviceID != "" {
		if devices, err := h.Accounts.Devices(r.Context(), user.ID); err == nil {
			for _, d := range devices {
				if d.DeviceID == deviceID {
					device = d
					break
				}
			}
		}
	}
	h.playFailed(r, user, device, item, plays.FailedStream)
}

// playFailed tells PlayFailed, when set, that user's play of item on
// device failed for reason. The notifications keep one per user and title
// an hour.
func (h *Handler) playFailed(r *http.Request, user accounts.User, device accounts.Device, item library.Item, reason string) {
	if h.PlayFailed == nil || r.Context().Err() != nil {
		return
	}
	h.PlayFailed(plays.Failure{User: user.ID, UserName: user.Name, App: device.Client, DeviceName: device.DeviceName, Title: playedTitle(item),
		Reason: reason, At: h.now()})
}

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
		Sent:          h.sentSoFar(device.ID.String()),
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
	case library.KindVideo:
		t.Kind = plays.HomeVideo
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
	case len(item.AlbumArtists) > 0:
		t.Artist = item.AlbumArtists[0].Name
	}
	return t
}
