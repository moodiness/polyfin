package jellyfin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
)

// transcodingInfo describes what Polyfin sends of what device plays, as
// Jellyfin reports it on the device's session (TranscodeManager.cs
// L343-L366 at v12.4): nil when the app reads the file as it is, or before
// the play session's first encoding opened its input. It is read from the
// play session's encoding under way, and kept for the play session: an
// encoding stops once its player is idle and starts again at its next
// request, and jellyfin-web, which keeps a session 10 s, would otherwise
// lose its Playback Info section between them. Jellyfin clears it when its
// transcode ends; the play stopping ends the session's NowPlaying here.
func (h *Handler) transcodingInfo(ctx context.Context, device accounts.ID, playing playback.NowPlaying) *TranscodingInfo {
	session := playing.PlaySessionID
	if session == "" || playing.PlayMethod == "DirectPlay" {
		return nil
	}
	running, ok := sessionEncoding(h.Playback.Encodings(), session)
	if !ok {
		if kept, ok := h.transcodings.Get(session); ok {
			return &kept
		}
		return nil
	}
	var analysis media.Analysis
	if version, ok := h.playedVersion(device, playing); ok {
		analysis, _ = h.Playback.Analyzed(ctx, version)
	}
	reasons, _ := h.reasons.Get(session)
	info := newTranscodingInfo(running, analysis, reasons)
	h.transcodings.Put(session, info)
	return &info
}

// sessionEncoding picks the encoding of a play session that describes
// what it sends, as the admin dashboard does: of those whose input is
// open, the quality the player plays now, and one converting the video
// over one copying it, as an app switching audio track runs both a moment.
func sessionEncoding(encodings []hls.Running, session string) (hls.Running, bool) {
	var chosen hls.Running
	found := false
	for _, running := range encodings {
		if running.Key.Session != session || !running.Opened {
			continue
		}
		if !found || running.Current && !chosen.Current ||
			running.Current == chosen.Current && running.Video != nil && chosen.Video == nil {
			chosen, found = running, true
		}
	}
	return chosen, found
}

// newTranscodingInfo describes what an encoding sends of the version
// analysis describes, for reasons: the conversion's codec and size, or
// the source's where it copies. Its bitrate is the video's and the
// audio's, or the source's when the copied video's is not known.
func newTranscodingInfo(running hls.Running, analysis media.Analysis, reasons []string) TranscodingInfo {
	info := TranscodingInfo{Container: running.Key.Format.Extension(), IsVideoDirect: running.Video == nil,
		IsAudioDirect: running.AudioCodec == "", TranscodeReasons: append([]string{}, reasons...)}
	var videoBitrate, audioBitrate int64
	if v := running.Video; v != nil {
		info.VideoCodec, info.Width, info.Height = hls.VideoCodecOf(v.Encoder), new(v.Width), new(v.Height)
		info.HardwareAccelerationType = accelerationType(v.Hardware)
		videoBitrate = v.Bitrate
	} else if video, ok := videoStream(analysis); ok {
		info.VideoCodec, info.Width, info.Height = video.Codec, new(video.Width), new(video.Height)
		videoBitrate = video.Bitrate
	}
	channels := 0
	if running.AudioCodec != "" {
		info.AudioCodec, channels, audioBitrate = hls.AudioCodecOf(running.AudioCodec), running.AudioChannels, running.AudioBitrate
	} else if audio, ok := audioStream(analysis, running.Key.Audio); ok {
		info.AudioCodec, channels, audioBitrate = audio.Codec, audio.Channels, audio.Bitrate
	}
	if channels > 0 {
		info.AudioChannels = new(channels)
	}
	switch {
	case videoBitrate > 0:
		info.Bitrate = new(videoBitrate + audioBitrate)
	case running.Video == nil && analysis.Bitrate > 0:
		info.Bitrate = new(analysis.Bitrate)
	}
	return info
}

// audioStream finds the audio stream of ffprobe's index among analysis'
// streams; -1 finds none.
func audioStream(analysis media.Analysis, index int) (media.Stream, bool) {
	for _, stream := range analysis.Streams {
		if stream.Type == "audio" && stream.Index == index {
			return stream, true
		}
	}
	return media.Stream{}, false
}

// accelerationType names the GPU converting video as Jellyfin's
// HardwareAccelerationType does, none for the processor.
func accelerationType(hardware *hls.Hardware) string {
	switch {
	case hardware == nil:
		return "none"
	case hardware.Method == "cuda":
		return "nvenc"
	}
	// VAAPI, VideoToolbox, QSV and AMF have Jellyfin's names.
	return hardware.Method
}

// logPlay tells on the detailed log how the version an app is about to
// play reaches it, as Jellyfin logs its StreamBuilder's decision
// (StreamBuilder.cs L833-L841 at v12.4): as it is, or streamed with its
// video and audio copied or converted, and why. It names no address.
func (h *Handler) logPlay(r *http.Request, item accounts.ID, version library.Version, decision playback.Decision, streamed bool) {
	if !h.Logger.Enabled(r.Context(), slog.LevelDebug) {
		return
	}
	attrs := []any{"app", callerFrom(r.Context()).Device.Client, "item", item, "addon", version.Addon}
	switch {
	case decision.DirectPlay:
		h.Logger.Debug("A version plays as it is", attrs...)
		return
	case !streamed:
		return
	}
	video := "copied"
	if v := decision.Video; v != nil {
		video = v.Codec + " with " + v.Encoder
	}
	audio := "copied"
	switch a := decision.Audio; {
	case decision.AudioStreamIndex < 0:
		audio = "none"
	case a != nil:
		audio = fmt.Sprintf("%s %d ch", a.Codec, a.Channels)
	}
	attrs = append(attrs, "video", video, "audio", audio, "reasons", strings.Join(decision.Reasons, ","))
	if decision.Progressive {
		h.Logger.Debug("A version plays as one stream", attrs...)
		return
	}
	h.Logger.Debug("A version plays over HLS", attrs...)
}
