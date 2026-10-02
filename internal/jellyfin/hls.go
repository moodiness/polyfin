package jellyfin

import (
	"bytes"
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// transcodingURL is a remux's TranscodingUrl: relative, with Jellyfin's
// parameters in Jellyfin's order. Polyfin reads back the play session, the
// audio track and the segment container; the rest describes the stream as
// Jellyfin would.
func transcodingURL(r *http.Request, item, sourceID accounts.ID, version library.Version, analysis media.Analysis, streams []playback.MediaStream,
	decision playback.Decision, limit int64, session string) string {
	t := decision.Transcoding
	var audio *playback.MediaStream
	for i := range streams {
		if streams[i].Type == "Audio" && streams[i].Index == decision.AudioStreamIndex {
			audio = &streams[i]
		}
	}
	var q orderedQuery
	q.add("DeviceId", callerFrom(r.Context()).Device.DeviceID)
	q.add("MediaSourceId", sourceID.String())
	q.add("VideoCodec", t.VideoCodec)
	q.add("AudioCodec", t.AudioCodec)
	if audio != nil {
		q.add("AudioStreamIndex", strconv.Itoa(audio.Index))
	}
	// A subtitle in HLS is the one the master playlist selects.
	hlsSubtitle := decision.SubtitleStreamIndex >= 0 && decision.Subtitles[decision.SubtitleStreamIndex].Method == "Hls"
	if hlsSubtitle {
		q.add("SubtitleStreamIndex", strconv.Itoa(decision.SubtitleStreamIndex))
	}
	audioBitrate := int64(0)
	if audio != nil && audio.BitRate != nil {
		audioBitrate = *audio.BitRate
	}
	if limit > 0 {
		q.add("VideoBitrate", strconv.FormatInt(limit-audioBitrate, 10))
	}
	if audio != nil {
		if audioBitrate > 0 {
			q.add("AudioBitrate", strconv.FormatInt(audioBitrate, 10))
		}
		if audio.SampleRate != nil {
			q.add("AudioSampleRate", strconv.Itoa(*audio.SampleRate))
		}
	}
	if video, ok := videoStream(analysis); ok && video.AverageRate > 0 {
		q.add("MaxFramerate", strconv.FormatFloat(math.Round(video.AverageRate*1000)/1000, 'f', -1, 64))
	}
	q.add("SegmentContainer", t.Container)
	if t.MinSegments != "" {
		q.add("MinSegments", t.MinSegments.String())
	}
	q.add("PlaySessionId", session)
	if token := callerFrom(r.Context()).Token; token != "" {
		q.add("ApiKey", token)
	}
	if t.MaxAudioChannels != "" {
		q.add("TranscodingMaxAudioChannels", t.MaxAudioChannels)
	}
	q.add("RequireAvc", "false")
	q.add("EnableAudioVbrEncoding", "true")
	q.add("Tag", version.ID.String())
	switch {
	case hlsSubtitle:
		q.add("SubtitleMethod", "Hls")
	case decision.SubtitleStreamIndex < 0:
		q.add("SubtitleMethod", "Encode")
	}
	q.add("TranscodeReasons", strings.Join(decision.Reasons, ","))
	return "/videos/" + hyphenated(item) + "/master.m3u8?" + q.String()
}

// orderedQuery writes URL parameters in order, with commas left as they
// are, as Jellyfin writes its lists.
type orderedQuery struct{ parts []string }

func (q *orderedQuery) add(name, value string) {
	q.parts = append(q.parts, name+"="+strings.ReplaceAll(url.QueryEscape(value), "%2C", ","))
}

func (q *orderedQuery) String() string {
	return strings.Join(q.parts, "&")
}

func videoStream(analysis media.Analysis) (media.Stream, bool) {
	for _, stream := range analysis.Streams {
		if stream.Type == "video" && !stream.AttachedPicture {
			return stream, true
		}
	}
	return media.Stream{}, false
}

// remuxRequest is what an HLS request is about.
type remuxRequest struct {
	remux    playback.Remux
	user     accounts.User
	item     library.Item
	analysis media.Analysis
	// files are the subtitle files addons offer: the first of the
	// version's subtitle streams.
	files []library.ExternalSubtitle
}

// remuxOf reads which remux an HLS request is about. HLS requests carry
// no credentials but the play session, signed for the user and version.
func (h *Handler) remuxOf(w http.ResponseWriter, r *http.Request) (remuxRequest, bool) {
	opened, ok := parseGUID(r.PathValue("itemId"))
	if !ok {
		validationProblem(w, map[string][]string{"itemId": {notValid(r.PathValue("itemId"))}})
		return remuxRequest{}, false
	}
	session := query(r, "playSessionId")
	grant, err := h.Playback.Signer().Verify(session)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return remuxRequest{}, false
	}
	user, err := h.Accounts.User(r.Context(), grant.User)
	if err != nil || user.IsDisabled {
		w.WriteHeader(http.StatusUnauthorized)
		return remuxRequest{}, false
	}
	item, err := h.title(r.Context(), user, opened)
	if err != nil {
		processingError(w, http.StatusNotFound)
		return remuxRequest{}, false
	}
	version, err := h.Library.Version(r.Context(), user, item.ID, grant.Version)
	if err != nil {
		processingError(w, http.StatusNotFound)
		return remuxRequest{}, false
	}
	format, ok := hls.ParseFormat(query(r, "segmentContainer"))
	if !ok {
		format = hls.TS
	}
	analysis, err := h.Playback.Analyze(r.Context(), version)
	if err != nil {
		processingError(w, http.StatusNotFound)
		return remuxRequest{}, false
	}
	files, ok := h.subtitleFiles.Get(item.ID)
	if !ok {
		files, _ = h.Library.Subtitles(r.Context(), user, item.ID)
	}
	remux := playback.Remux{Session: session, Version: version, Audio: audioTrack(analysis, len(files), query(r, "audioStreamIndex")), Format: format}
	return remuxRequest{remux: remux, user: user, item: item, analysis: analysis, files: files}, true
}

// audioTrack converts a Jellyfin audio stream index, counted after the
// subtitle files addons offer, to the version's own: the one PlaybackInfo
// described, else the version's default audio track.
func audioTrack(analysis media.Analysis, files int, asked string) int {
	if index, err := strconv.Atoi(asked); err == nil {
		for _, stream := range analysis.Streams {
			if stream.Type == "audio" && stream.Index == index-files {
				return stream.Index
			}
		}
	}
	fallback := -1
	for _, stream := range analysis.Streams {
		if stream.Type != "audio" {
			continue
		}
		if stream.Default {
			return stream.Index
		}
		if fallback < 0 {
			fallback = stream.Index
		}
	}
	return fallback
}

// hlsPlaylist serves master.m3u8 and main.m3u8. Their URIs are relative and
// repeat the query, which carries the play session.
func (h *Handler) hlsPlaylist(w http.ResponseWriter, r *http.Request, name string) {
	req, ok := h.remuxOf(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	var err error
	if strings.EqualFold(name, "master") {
		var variant hls.Variant
		if variant, err = h.Playback.Variant(r.Context(), req.remux); err == nil {
			if strings.EqualFold(query(r, "subtitleMethod"), "Hls") {
				variant.Subtitles = h.renditions(req, query(r, "subtitleStreamIndex"), r.URL.RawQuery)
			}
			err = hls.WriteMaster(w, variant, "main.m3u8?"+r.URL.RawQuery)
		}
	} else {
		var plan hls.Plan
		if plan, err = h.Playback.Plan(r.Context(), req.remux.Version); err == nil {
			extension := req.remux.Format.Extension()
			err = hls.WriteMedia(w, plan, req.remux.Format, func(n int) string {
				return "hls1/main/" + strconv.Itoa(n) + "." + extension + "?" + r.URL.RawQuery
			})
		}
	}
	if err != nil {
		h.remuxError(w, r, req.remux, err)
	}
}

// renditions are the text subtitles a remux offers in HLS, the one the
// app chose shown by default: the addons' files and the embedded tracks
// FFmpeg extracts. The app's player can switch between them.
func (h *Handler) renditions(req remuxRequest, selected, rawQuery string) []hls.Rendition {
	streams := playback.MediaStreams(req.analysis, playable{item: req.item, subtitles: req.files}.externals(), h.Accounts.Settings().Language)
	var renditions []hls.Rendition
	for _, stream := range streams {
		if stream.Type != "Subtitle" || !(stream.IsExternal || playback.ExtractableSubtitle(req.analysis, stream.Index-len(req.files))) {
			continue
		}
		index := strconv.Itoa(stream.Index)
		renditions = append(renditions, hls.Rendition{Name: stream.DisplayTitle, Language: playback.LanguageTag(stream.Language),
			Default: index == selected, Forced: stream.IsForced, URI: "hls1/subtitles" + index + "/main.m3u8?" + rawQuery})
	}
	return renditions
}

// hlsSegment serves the segments of a remux under hls1/main, the
// initialization segment as -1, and its subtitle renditions under
// hls1/subtitles{index}.
func (h *Handler) hlsSegment(w http.ResponseWriter, r *http.Request) {
	playlist := strings.ToLower(r.PathValue("playlistId"))
	if raw, ok := strings.CutPrefix(playlist, "subtitles"); ok {
		if index, err := strconv.Atoi(raw); err == nil && index >= 0 {
			h.hlsSubtitles(w, r, index)
			return
		}
	}
	name, extension, _ := strings.Cut(r.PathValue("file"), ".")
	n, err := strconv.Atoi(name)
	if playlist != "main" || err != nil || n < -1 {
		processingError(w, http.StatusNotFound)
		return
	}
	req, ok := h.remuxOf(w, r)
	if !ok {
		return
	}
	remux := req.remux
	// The initialization segment of fragmented MP4 is -1.mp4.
	if !strings.EqualFold(extension, remux.Format.Extension()) {
		processingError(w, http.StatusNotFound)
		return
	}
	// A segment far ahead of FFmpeg waits for a new run to reach it.
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	var file *os.File
	if n < 0 {
		file, err = h.Playback.RemuxInit(ctx, remux)
	} else {
		file, err = h.Playback.RemuxSegment(ctx, remux, n)
	}
	if err != nil {
		h.remuxError(w, r, remux, err)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", remux.Format.ContentType())
	http.ServeContent(w, r, "", time.Time{}, file)
}

// hlsSubtitles serves the playlist and the WebVTT segments of a subtitle
// rendition, index being the Jellyfin index of its stream. The segments of
// an addon's file are cut from it; those of an embedded track wait for
// FFmpeg to extract them.
func (h *Handler) hlsSubtitles(w http.ResponseWriter, r *http.Request, index int) {
	name, extension, _ := strings.Cut(r.PathValue("file"), ".")
	req, ok := h.remuxOf(w, r)
	if !ok {
		return
	}
	stream := index - len(req.files)
	if index >= len(req.files) && !playback.ExtractableSubtitle(req.analysis, stream) {
		processingError(w, http.StatusNotFound)
		return
	}
	plan, err := h.Playback.Plan(r.Context(), req.remux.Version)
	if err != nil {
		h.remuxError(w, r, req.remux, err)
		return
	}
	if strings.EqualFold(r.PathValue("file"), "main.m3u8") {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache")
		if err := hls.WriteSubtitlePlaylist(w, plan, func(n int) string { return strconv.Itoa(n) + ".vtt?" + r.URL.RawQuery }); err != nil {
			h.remuxError(w, r, req.remux, err)
		}
		return
	}
	n, err := strconv.Atoi(name)
	if err != nil || n < 0 || n >= plan.Len() || !strings.EqualFold(extension, "vtt") {
		processingError(w, http.StatusNotFound)
		return
	}
	var data []byte
	if index < len(req.files) {
		var cues []subtitles.Cue
		if cues, err = h.subtitleCues(r.Context(), req.files[index]); err == nil {
			data = hls.SubtitleSegment(cues, plan, n)
		}
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		data, err = h.Playback.RemuxSubtitle(ctx, req.remux, stream, n)
	}
	if err != nil {
		h.remuxError(w, r, req.remux, err)
		return
	}
	w.Header().Set("Content-Type", "text/vtt")
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
}

func (h *Handler) remuxError(w http.ResponseWriter, r *http.Request, remux playback.Remux, err error) {
	switch {
	case r.Context().Err() != nil:
	case errors.Is(err, hls.ErrNotFound), errors.Is(err, hls.ErrStopped), errors.Is(err, playback.ErrNotRemuxable):
		processingError(w, http.StatusNotFound)
	default:
		h.Logger.Warn("A remux could not be served", "addon", remux.Version.Addon, "error", err)
		processingError(w, http.StatusInternalServerError)
	}
}

// stopEncodings answers DELETE /Videos/ActiveEncodings, which apps send
// when they leave a remux, for instance to play another audio track.
func (h *Handler) stopEncodings(w http.ResponseWriter, r *http.Request) {
	if session := query(r, "playSessionId"); session != "" {
		h.Playback.StopRemux(session)
	}
	w.WriteHeader(http.StatusNoContent)
}
