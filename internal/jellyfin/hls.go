package jellyfin

import (
	"bytes"
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/source"
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
	// A subtitle in HLS is the one the master playlist selects; one burned
	// in, the one FFmpeg draws onto the video.
	method := ""
	if decision.SubtitleStreamIndex >= 0 {
		method = decision.Subtitles[decision.SubtitleStreamIndex].Method
	}
	if method == "Hls" || method == "Encode" {
		q.add("SubtitleStreamIndex", strconv.Itoa(decision.SubtitleStreamIndex))
	}
	audioBitrate := int64(0)
	if audio != nil && audio.BitRate != nil {
		audioBitrate = *audio.BitRate
	}
	if limit > 0 {
		q.add("VideoBitrate", strconv.FormatInt(playback.VideoLimit(limit, audio), 10))
	}
	if audio != nil {
		if audioBitrate > 0 {
			q.add("AudioBitrate", strconv.FormatInt(audioBitrate, 10))
		}
		// Jellyfin gives the sample rate of the audio it keeps.
		if audio.SampleRate != nil && decision.Audio == nil {
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
	case method == "Hls" || method == "Encode":
		q.add("SubtitleMethod", method)
	case decision.SubtitleStreamIndex < 0:
		q.add("SubtitleMethod", "Encode")
	}
	q.add("TranscodeReasons", strings.Join(decision.Reasons, ","))
	// Video and audio converted rather than copied are asked as Jellyfin's
	// own parameters do it, after the reasons.
	if decision.Video != nil {
		q.add("allowVideoStreamCopy", "false")
	}
	if decision.Audio != nil {
		q.add("allowAudioStreamCopy", "false")
	}
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
	// live is set for a channel, whose stream FFmpeg converts as it comes.
	live bool
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
	// Like Jellyfin's HLS endpoints, the user's allowed hours apply.
	if err != nil || user.IsDisabled || h.outsideHours(user) {
		w.WriteHeader(http.StatusUnauthorized)
		return remuxRequest{}, false
	}
	// A conversion the user may not have is refused, as Jellyfin refuses
	// it once FFmpeg is about to start. PlaybackInfo plans none: this
	// guards URLs kept from before a permission was taken away, or made up.
	allowed := h.Accounts.Conversions(user)
	if !allowed.Video && strings.EqualFold(query(r, "allowVideoStreamCopy"), "false") ||
		!allowed.Audio && strings.EqualFold(query(r, "allowAudioStreamCopy"), "false") {
		h.Logger.Info("A conversion the user may not have was refused")
		processingError(w, http.StatusBadRequest)
		return remuxRequest{}, false
	}
	item, err := h.played(r.Context(), user, opened)
	live := item.Kind == library.KindChannel
	if err != nil {
		processingError(w, http.StatusNotFound)
		return remuxRequest{}, false
	}
	version, err := h.version(r.Context(), user, item, grant.Version)
	if err != nil {
		processingError(w, http.StatusNotFound)
		return remuxRequest{}, false
	}
	format, ok := hls.ParseFormat(query(r, "segmentContainer"))
	if !ok {
		format = hls.TS
	}
	var analysis media.Analysis
	var files []library.ExternalSubtitle
	if live {
		analysis, err = h.Playback.AnalyzeLive(r.Context(), version)
	} else {
		analysis, err = h.Playback.Analyze(r.Context(), version)
		if cached, ok := h.subtitleFiles.Get(item.ID); ok {
			files = cached
		} else {
			files, _ = h.Library.Subtitles(r.Context(), user, item.ID)
		}
	}
	if err != nil {
		processingError(w, http.StatusNotFound)
		return remuxRequest{}, false
	}
	remux := playback.Remux{Session: session, User: user.ID, Version: version, Audio: audioTrack(analysis, len(files), query(r, "audioStreamIndex")), Format: format}
	// The conversions PlaybackInfo chose follow from the URL, as they
	// would for Jellyfin, through the same functions. Converted video is
	// scaled down to the user's quality group and to the height the
	// settings allow, the lower winning, as the encoding starts: settings
	// Jellyfin does not have.
	if strings.EqualFold(query(r, "allowVideoStreamCopy"), "false") {
		streams := playback.MediaStreams(analysis, playable{item: item, subtitles: files}.externals(), h.Accounts.Settings().Language)
		if i := slices.IndexFunc(streams, func(s playback.MediaStream) bool { return s.Type == "Video" }); i >= 0 {
			remux.ConvertVideo = playback.ConvertVideo(query(r, "videoCodec"), h.conversionLimits(r, user), streams[i], h.Playback.Capabilities())
		}
	}
	// A subtitle burned in is an image track inside the file, counted after
	// the addons' files.
	if index, err := strconv.Atoi(query(r, "subtitleStreamIndex")); err == nil && remux.ConvertVideo != nil &&
		strings.EqualFold(query(r, "subtitleMethod"), "Encode") && playback.BurnableSubtitle(analysis, index-len(files)) {
		remux.Burn = new(index - len(files))
	}
	if strings.EqualFold(query(r, "allowAudioStreamCopy"), "false") {
		channels, layout := 0, ""
		for _, stream := range analysis.Streams {
			if stream.Index == remux.Audio {
				channels, layout = stream.Channels, stream.ChannelLayout
			}
		}
		remux.ConvertAudio = playback.ConvertAudio(query(r, "audioCodec"), query(r, "transcodingMaxAudioChannels"), channels, layout,
			h.Playback.Capabilities().Tuning)
	}
	// PlaybackInfo copies no video above the user's bitrate limit, nor
	// taller than their quality group: this guards URLs kept from before
	// the limit or the group was set, or made up.
	if remux.ConvertVideo == nil && overUserLimit(int64(user.MaxBitrate), analysis.Bitrate) {
		h.Logger.Info("A remux above the user's bitrate limit was refused")
		processingError(w, http.StatusBadRequest)
		return remuxRequest{}, false
	}
	if remux.ConvertVideo == nil && !user.FitsGroup(videoHeight(analysis)) {
		h.Logger.Info("A remux taller than the user's quality group was refused")
		processingError(w, http.StatusBadRequest)
		return remuxRequest{}, false
	}
	return remuxRequest{remux: remux, user: user, item: item, analysis: analysis, files: files, live: live}, true
}

// conversionLimits are what video converted for an HLS request is scaled
// down within: the bitrate its URL gives the video, which with the audio's
// is the limit PlaybackInfo played under, never above the user's limit
// whatever the URL says, and the heights the user's group and the settings
// cap it at. The limit is the user's when the app's was not lower.
func (h *Handler) conversionLimits(r *http.Request, user accounts.User) playback.Limits {
	var limits playback.Limits
	limits.Group, limits.Server = h.Accounts.ConversionCaps(user)
	limits.Video, _ = strconv.ParseInt(query(r, "videoBitrate"), 10, 64)
	if limits.Video > 0 {
		audio, _ := strconv.ParseInt(query(r, "audioBitrate"), 10, 64)
		limits.Bitrate = limits.Video + max(audio, 0)
	}
	if most := int64(user.MaxBitrate); most > 0 {
		if limits.Video <= 0 || limits.Video > most {
			limits.Video = most
		}
		if limits.Video == most || limits.Bitrate >= most {
			limits.Bitrate, limits.ByUser = most, true
		}
	}
	return limits
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

// hlsPlaylist serves master.m3u8 and main.m3u8, or live.m3u8 for a
// channel. Their URIs are relative and repeat the query, which carries the
// play session.
func (h *Handler) hlsPlaylist(w http.ResponseWriter, r *http.Request, name string) {
	req, ok := h.remuxOf(w, r)
	if !ok {
		return
	}
	if reasons := query(r, "transcodeReasons"); reasons != "" {
		h.reasons.Put(req.remux.Session, strings.Split(reasons, ","))
	}
	if video := req.remux.ConvertVideo; video != nil {
		h.sizes.Put(req.remux.Session, video.Size)
	}
	if req.live {
		h.livePlaylist(w, r, req.remux, name)
		return
	}
	if strings.EqualFold(name, "live") {
		processingError(w, http.StatusNotFound)
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
	if req.live {
		processingError(w, http.StatusNotFound)
		return
	}
	remux := req.remux
	// The initialization segment of fragmented MP4 is -1.mp4.
	if !strings.EqualFold(extension, remux.Format.Extension()) {
		processingError(w, http.StatusNotFound)
		return
	}
	// A segment far ahead of FFmpeg waits for a new run to reach it, as
	// long as FFmpeg makes progress: one that made none for 20 s, or did
	// not reach the segment in 2 min, is answered 503 (see remuxError).
	ctx, cancel := context.WithTimeout(r.Context(), segmentWait)
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
	if req.live {
		processingError(w, http.StatusNotFound)
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
		var text subtitleText
		if text, err = h.subtitleFile(r.Context(), req.files[index]); err == nil {
			data = hls.SubtitleSegment(text.cues, plan, n)
		}
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), segmentWait)
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

// segmentWait bounds the wait for a segment whose FFmpeg makes progress.
const segmentWait = 2 * time.Minute

func (h *Handler) remuxError(w http.ResponseWriter, r *http.Request, remux playback.Remux, err error) {
	switch {
	case r.Context().Err() != nil:
	case errors.Is(err, hls.ErrNotFound), errors.Is(err, hls.ErrStopped), errors.Is(err, playback.ErrNotRemuxable):
		processingError(w, http.StatusNotFound)
	case errors.Is(err, hls.ErrBusy):
		// The server runs as many live encodings, or converts the video of
		// as many playbacks, as it may. Jellyfin has no such limits: its
		// nearest, a tuner's stream limit, fails with a 500. A 503 says the
		// same to apps, which take both as a server error, and that the
		// refusal lasts only while the server is busy.
		h.Logger.Info("An encoding was refused: the server runs as many as it may", "addon", remux.Version.Addon)
		processingError(w, http.StatusServiceUnavailable)
	case errors.Is(err, hls.ErrStalled), errors.Is(err, context.DeadlineExceeded), errors.Is(err, source.ErrUnavailable):
		// The source is slow, stopped answering, or failed: the app hears
		// it within seconds, rather than after minutes, and may ask again,
		// which waits for the same encoding, or reopens one once the
		// source failed.
		h.Logger.Warn("A segment could not be served in time: its source is slow or failed", "addon", remux.Version.Addon, "error", err)
		processingError(w, http.StatusServiceUnavailable)
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
