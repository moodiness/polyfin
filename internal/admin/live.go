package admin

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// Sessions are the playbacks under way, and the commands administrators
// send to the apps playing: the Jellyfin handler's.
type Sessions interface {
	LiveSessions(ctx context.Context) ([]jellyfin.LiveSession, error)
	StopPlayback(ctx context.Context, by accounts.User, session accounts.ID) error
	SendMessage(ctx context.Context, by accounts.User, session accounts.ID, header, text string, timeout time.Duration) error
}

// How a playback reaches its app, as liveSessionJSON.Delivery tells it.
const (
	// deliveryDirectPlay: the app reads the file as it is.
	deliveryDirectPlay = "directPlay"
	// deliveryRemux: Polyfin streams the file over HLS, copying its tracks.
	deliveryRemux = "remux"
	// deliveryConversion: Polyfin converts the video or the audio.
	deliveryConversion = "conversion"
	// deliveryStream: Polyfin streams over HLS, its encoding stopped while
	// the app pauses or between requests: how is not known now.
	deliveryStream = "stream"
)

type liveSessionJSON struct {
	// ID is the session's, as Jellyfin apps know it: the device's sign-in.
	ID           string         `json:"id"`
	User         liveUserJSON   `json:"user"`
	Device       liveDeviceJSON `json:"device"`
	Controllable bool           `json:"controllable"`
	Item         *liveItemJSON  `json:"item"`
	// Position is in seconds.
	Position  float64   `json:"position"`
	Paused    bool      `json:"paused"`
	StartedAt time.Time `json:"startedAt"`
	// PlayMethod is what the app reported: DirectPlay, DirectStream or
	// Transcode.
	PlayMethod string `json:"playMethod"`
	Delivery   string `json:"delivery"`
	// Reasons are Jellyfin's TranscodeReasons, when the session streams
	// over HLS for some.
	Reasons []string `json:"reasons"`
	// Source is the version played, as analyzed; Sent, what reaches the
	// app. Either is null when not known.
	Source *streamJSON `json:"source"`
	Sent   *streamJSON `json:"sent"`
	// Video and Audio tell what is converted, null when copied.
	Video *videoConversionJSON `json:"video"`
	Audio *audioConversionJSON `json:"audio"`
}

type liveUserJSON struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	ImageTag *string `json:"imageTag"`
	// QualityGroup is the tallest video the user is offered, 0 for any.
	QualityGroup int `json:"qualityGroup"`
}

type liveDeviceJSON struct {
	Name       string `json:"name"`
	App        string `json:"app"`
	AppVersion string `json:"appVersion"`
	Address    string `json:"address"`
}

type liveItemJSON struct {
	ID string `json:"id"`
	// Kind is movie, episode, channel or recording.
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	SeriesName string `json:"seriesName"`
	Season     int    `json:"season"`
	Episode    int    `json:"episode"`
	Year       int    `json:"year"`
	Runtime    int64  `json:"runtime"`
	// PosterID is the item whose Primary image is the poster: the series
	// of an episode.
	PosterID string `json:"posterId"`
}

// streamJSON describes a video: its size, codecs and bitrate in bits per
// second, any of them 0 or empty when not known.
type streamJSON struct {
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	VideoCodec string `json:"videoCodec"`
	AudioCodec string `json:"audioCodec"`
	Bitrate    int64  `json:"bitrate"`
}

type videoConversionJSON struct {
	Encoder string `json:"encoder"`
	// Hardware is the GPU method, cuda or vaapi; empty for the CPU.
	Hardware string `json:"hardware"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Bitrate  int64  `json:"bitrate"`
	ToneMap  bool   `json:"toneMap"`
	Burn     bool   `json:"burnSubtitles"`
}

type audioConversionJSON struct {
	Codec    string `json:"codec"`
	Channels int    `json:"channels"`
	Bitrate  int64  `json:"bitrate"`
}

// liveSessions lists the playbacks under way.
func (h *handler) liveSessions(w http.ResponseWriter, r *http.Request) {
	result := []liveSessionJSON{}
	if h.Sessions == nil {
		writeJSON(w, http.StatusOK, result)
		return
	}
	sessions, err := h.Sessions.LiveSessions(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	for _, s := range sessions {
		result = append(result, newLiveSessionJSON(s))
	}
	writeJSON(w, http.StatusOK, result)
}

func newLiveSessionJSON(s jellyfin.LiveSession) liveSessionJSON {
	result := liveSessionJSON{
		ID:           s.Device.ID.String(),
		User:         liveUserJSON{ID: s.User.ID.String(), Name: s.User.Name, ImageTag: imageTag(s.User), QualityGroup: s.User.QualityGroup},
		Device:       liveDeviceJSON{Name: s.Device.DeviceName, App: s.Device.Client, AppVersion: s.Device.ClientVersion, Address: s.Device.RemoteAddress},
		Controllable: s.Controllable,
		Position:     s.Playing.Position.Seconds(),
		Paused:       s.Playing.Paused,
		StartedAt:    s.Playing.Started,
		PlayMethod:   s.Playing.PlayMethod,
		Reasons:      s.Reasons,
	}
	if result.Reasons == nil {
		result.Reasons = []string{}
	}
	if s.Found {
		result.Item = newLiveItemJSON(s.Item)
	}
	if s.Source != nil {
		result.Source = sourceStream(*s.Source)
	}
	result.Delivery = deliveryStream
	switch s.Playing.PlayMethod {
	case "DirectPlay":
		result.Delivery = deliveryDirectPlay
	case "DirectStream":
		result.Delivery = deliveryRemux
	}
	// The encoding of the play session tells best; one converting the
	// video describes it over one copying it, as an app switching audio
	// track runs both a moment.
	for _, running := range s.Encodings {
		if !running.Opened || result.Video != nil {
			continue
		}
		result.Delivery = deliveryRemux
		if running.Video != nil {
			v := running.Video
			result.Video = &videoConversionJSON{Encoder: v.Encoder, Width: v.Width, Height: v.Height, Bitrate: v.Bitrate,
				ToneMap: v.ToneMap, Burn: v.Burn != nil}
			if v.Hardware != nil {
				result.Video.Hardware = v.Hardware.Method
			}
		}
		result.Audio = nil
		if running.AudioCodec != "" {
			result.Audio = &audioConversionJSON{Codec: running.AudioCodec, Channels: running.AudioChannels, Bitrate: running.AudioBitrate}
		}
		if result.Video != nil || result.Audio != nil {
			result.Delivery = deliveryConversion
		}
	}
	result.Sent = sentStream(result)
	return result
}

func newLiveItemJSON(item library.Item) *liveItemJSON {
	result := &liveItemJSON{ID: item.ID.String(), Kind: string(item.Kind), Name: item.Name, Year: item.ProductionYear,
		Runtime: int64(item.Runtime.Seconds()), PosterID: item.ID.String()}
	if item.Kind == library.KindEpisode {
		result.SeriesName, result.Season, result.Episode = item.SeriesName, item.ParentIndexNumber, item.IndexNumber
		if item.SeriesID != (accounts.ID{}) {
			result.PosterID = item.SeriesID.String()
		}
	}
	return result
}

// sourceStream describes an analyzed version: its first video and audio
// tracks.
func sourceStream(analysis media.Analysis) *streamJSON {
	result := &streamJSON{Bitrate: analysis.Bitrate}
	for _, stream := range analysis.Streams {
		switch {
		case stream.Type == "video" && !stream.AttachedPicture && result.VideoCodec == "":
			result.Width, result.Height, result.VideoCodec = stream.Width, stream.Height, stream.Codec
		case stream.Type == "audio" && result.AudioCodec == "":
			result.AudioCodec = stream.Codec
		}
	}
	return result
}

// sentStream describes what reaches the app: the source, with what is
// converted replaced. Its bitrate is the converted video's and audio's,
// or the source's.
func sentStream(s liveSessionJSON) *streamJSON {
	if s.Source == nil && s.Video == nil {
		return nil
	}
	sent := &streamJSON{}
	if s.Source != nil {
		*sent = *s.Source
	}
	if s.Video != nil {
		sent.Width, sent.Height, sent.VideoCodec = s.Video.Width, s.Video.Height, videoCodecOf(s.Video.Encoder)
		sent.Bitrate = s.Video.Bitrate
		if s.Audio != nil {
			sent.Bitrate += s.Audio.Bitrate
		}
	}
	if s.Audio != nil {
		sent.AudioCodec = audioCodecOf(s.Audio.Codec)
	}
	return sent
}

// videoCodecOf names the codec an FFmpeg video encoder makes.
func videoCodecOf(encoder string) string {
	switch {
	case strings.Contains(encoder, "265"), strings.HasPrefix(encoder, "hevc"):
		return "hevc"
	case strings.Contains(encoder, "264"), strings.HasPrefix(encoder, "h264"):
		return "h264"
	}
	return encoder
}

// audioCodecOf names the codec an FFmpeg audio encoder makes.
func audioCodecOf(encoder string) string {
	return strings.TrimPrefix(encoder, "lib")
}

// maxMessageLength bounds the messages administrators send to apps.
const maxMessageLength = 500

// stopSession asks the app of a session to stop playing.
func (h *handler) stopSession(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if h.Sessions == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	h.answerCommand(w, r, h.Sessions.StopPlayback(r.Context(), sessionFrom(r.Context()).User, id))
}

// messageSession has the app of a session show a message.
func (h *handler) messageSession(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Header string `json:"header"`
		Text   string `json:"text"`
		// Seconds the message shows for; 0 until it is dismissed.
		Timeout int `json:"timeout"`
	}
	if !decode(w, r, &body) {
		return
	}
	body.Text, body.Header = strings.TrimSpace(body.Text), strings.TrimSpace(body.Header)
	if body.Text == "" || utf8.RuneCountInString(body.Text) > maxMessageLength || utf8.RuneCountInString(body.Header) > 100 ||
		body.Timeout < 0 || body.Timeout > 3600 {
		writeError(w, http.StatusBadRequest, "invalid_message")
		return
	}
	if h.Sessions == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	h.answerCommand(w, r, h.Sessions.SendMessage(r.Context(), sessionFrom(r.Context()).User, id, body.Header, body.Text,
		time.Duration(body.Timeout)*time.Second))
}

// answerCommand answers a command sent to a session: 204 once sent.
func (h *handler) answerCommand(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, accounts.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, jellyfin.ErrControlRefused):
		writeError(w, http.StatusForbidden, "remote_control_refused")
	case errors.Is(err, jellyfin.ErrNotControllable):
		writeError(w, http.StatusConflict, "not_controllable")
	default:
		h.internalError(w, r, err)
	}
}
