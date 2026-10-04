package jellyfin

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
)

// Tracks play as Jellyfin plays audio: PlaybackInfo decides with the
// app's device profile, from what the addon says of the stream (its codec,
// container, sample rate and bit depth), or from its analysis when it says
// nothing; the app then fetches the track as it is from /Audio/{id}/stream,
// redirected to the source or relayed by the same rules as videos, or
// converted by FFmpeg, progressively or in HLS segments, to the codec,
// bitrate, sample rate and channels the app takes. /Audio/{id}/universal
// decides from its parameters instead of a profile. Quality groups, which
// are about video heights, do not apply to audio; the user's conversion
// permission and bitrate limit do.

func (h *Handler) audioRoutes(rt *router) {
	// Players fetch audio without credentials, as Jellyfin allows: see
	// streamAccess.
	rt.handle(http.MethodGet, "/Audio/{itemId}/{file}", http.HandlerFunc(h.audioFile))
	rt.handle(http.MethodGet, "/Audio/{itemId}/universal", http.HandlerFunc(h.universalAudio))
	rt.handle(http.MethodGet, "/Audio/{itemId}/hls1/{playlistId}/{file}", http.HandlerFunc(h.audioSegment))
}

// audioCodec reads an Eclipse codec as Jellyfin names it, with its profile.
func audioCodec(codec string) (string, string) {
	switch codec = strings.ToLower(codec); codec {
	case "he-aac", "heaac":
		return "aac", "HE-AAC"
	case "eac3_joc":
		return "eac3", ""
	}
	return codec, ""
}

// audioContainer reads an Eclipse container as Jellyfin names an audio
// file's: an MP4 file is m4a, as Jellyfin names audio-only MP4; a manifest
// is hls or dash, which no app plays as audio.
func audioContainer(container, manifest string) string {
	switch manifest {
	case "hls", "dash":
		return manifest
	}
	switch container = strings.ToLower(container); container {
	case "mp4", "fmp4", "m4a", "m4b":
		return "m4a"
	}
	return container
}

// describedAudio is what is known of a track's stream: as its addon
// describes it, else as analyzed.
type describedAudio struct {
	source   playback.AudioSource
	analysis media.Analysis
}

// describeAudio describes a track's stream from its addon's routing
// fields when they give its codec and container, and analyzes it
// otherwise.
func (h *Handler) describeAudio(ctx context.Context, version library.Version) (describedAudio, error) {
	if analysis, ok := h.Playback.Analyzed(ctx, version.ID); ok {
		return analyzedAudio(analysis), nil
	}
	if a := version.Audio; a != nil && a.Codec != "" && (a.Container != "" || a.Manifest == "hls" || a.Manifest == "dash") {
		codec, profile := audioCodec(a.Codec)
		container := audioContainer(a.Container, a.Manifest)
		stream := media.Stream{Index: 0, Type: "audio", Codec: codec, Profile: profile, SampleRate: a.SampleRate, BitDepth: a.BitDepth}
		source := playback.AudioSource{Container: container, Codec: codec, Profile: profile, SampleRate: a.SampleRate, BitDepth: a.BitDepth}
		// The reply gives no bitrate: a lossless stream's is taken at most
		// that of its samples in stereo, so that a bitrate limit converts it
		// rather than letting it through. A lossy stream's stays unknown.
		if losslessCodec(codec) && a.SampleRate > 0 && a.BitDepth > 0 {
			source.Bitrate = int64(a.SampleRate) * int64(a.BitDepth) * 2
		}
		return describedAudio{
			source:   source,
			analysis: media.Analysis{Format: container, Duration: version.Runtime, Streams: []media.Stream{stream}, Remote: true},
		}, nil
	}
	analysis, err := h.Playback.Analyze(ctx, version)
	if err != nil {
		return describedAudio{}, err
	}
	return analyzedAudio(analysis), nil
}

func analyzedAudio(analysis media.Analysis) describedAudio {
	d := describedAudio{analysis: analysis, source: playback.AudioSource{Container: playback.Container(analysis), Bitrate: analysis.Bitrate}}
	if d.source.Container == "mov,mp4,m4a,3gp,3g2,mj2" {
		d.source.Container = "m4a"
	}
	for _, s := range analysis.Streams {
		if s.Type == "audio" {
			d.source.Codec, d.source.Profile, d.source.Channels, d.source.SampleRate = s.Codec, s.Profile, s.Channels, s.SampleRate
			d.source.BitDepth = cmp.Or(s.BitDepth, s.SampleBits)
			if d.source.Bitrate == 0 {
				d.source.Bitrate = s.Bitrate
			}
			break
		}
	}
	return d
}

// audioStreams are the MediaStreams of a track: its audio only, cover art
// left out, as Jellyfin lists a song's.
func (h *Handler) audioStreams(d describedAudio) []playback.MediaStream {
	analysis := d.analysis
	streams := analysis.Streams[:0:0]
	for _, s := range analysis.Streams {
		if s.Type == "audio" {
			streams = append(streams, s)
			break
		}
	}
	analysis.Streams = streams
	return playback.MediaStreams(analysis, nil, h.Accounts.Settings().Language)
}

// audioSource describes a track's stream as a media source, played as it
// is from Path.
func (h *Handler) audioSource(r *http.Request, item library.Item, version library.Version, d describedAudio) MediaSourceInfo {
	streams := h.audioStreams(d)
	source := MediaSourceInfo{
		Protocol:               "Http",
		Id:                     item.ID.String(),
		Type:                   "Default",
		Container:              d.source.Container,
		Name:                   item.Name,
		IsRemote:               true,
		ETag:                   version.ID.String(),
		SupportsTranscoding:    true,
		SupportsDirectStream:   true,
		SupportsDirectPlay:     true,
		SupportsProbing:        true,
		MediaStreams:           streams,
		MediaAttachments:       []MediaAttachment{},
		Formats:                []string{},
		RequiredHttpHeaders:    map[string]string{},
		TranscodingSubProtocol: "http",
	}
	if len(streams) > 0 {
		source.DefaultAudioStreamIndex = new(streams[0].Index)
	}
	if d.source.Bitrate > 0 {
		source.Bitrate = new(d.source.Bitrate)
	}
	if d.analysis.Size > 0 {
		source.Size = new(d.analysis.Size)
	}
	if runtime := cmp.Or(d.analysis.Duration, version.Runtime, item.Runtime); runtime > 0 {
		source.RunTimeTicks = new(int64(runtime / 100))
	}
	source.Path = h.audioURL(r, item.ID, version, d.source.Container, mustRelay(r, version))
	return source
}

// audioURL is the address players fetch a track from as it is, with a
// grant signed for the caller (see streamURL).
func (h *Handler) audioURL(r *http.Request, item accounts.ID, version library.Version, container string, relay bool) string {
	user := callerFrom(r.Context()).User
	grant := h.Playback.Signer().Sign(playback.Grant{Version: version.ID, User: user.ID, Relay: relay})
	query := url.Values{"static": {"true"}, "mediaSourceId": {item.String()}, "Tag": {version.ID.String()}, grantParameter: {grant}}
	extension := container
	if extension == "" || strings.Contains(extension, ",") {
		extension = "audio"
	}
	return baseURL(r) + "/Audio/" + item.String() + "/stream." + extension + "?" + query.Encode()
}

// audioPlaybackInfo answers PlaybackInfo for a track: its one version,
// decided for the app's device profile.
func (h *Handler) audioPlaybackInfo(w http.ResponseWriter, r *http.Request, user accounts.User, item library.Item, request playbackInfoRequest) {
	if request.MediaSourceId != "" {
		if requested, ok := parseGUID(request.MediaSourceId); !ok || requested != item.ID {
			if owner, isVersion := h.Library.VersionOwner(requested); !isVersion || owner != item.ID {
				writeJSON(w, http.StatusOK, noCompatibleStream{MediaSources: []MediaSourceInfo{}, ErrorCode: "NoCompatibleStream"})
				return
			}
		}
	}
	version, err := h.version(r.Context(), user, item, item.ID)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	d, err := h.describeAudio(r.Context(), version)
	if err != nil {
		h.Logger.Info("A track could not be analyzed", "addon", version.Addon, "error", err)
		writeJSON(w, http.StatusOK, noCompatibleStream{MediaSources: []MediaSourceInfo{}, ErrorCode: "NoCompatibleStream"})
		return
	}
	session := h.Playback.Signer().Sign(playback.Grant{Version: version.ID, User: user.ID, Relay: mustRelay(r, version)})
	source := h.audioSource(r, item, version, d)
	// Jellyfin names the default track in a song's details, not here.
	source.DefaultAudioStreamIndex = nil
	decision := playback.AudioDecision{DirectPlay: true}
	if request.DeviceProfile != nil {
		decision = playback.DecideAudio(request.DeviceProfile, d.source, playback.AudioOptions{
			MaxStreamingBitrate: request.MaxStreamingBitrate.value,
			EnableDirectPlay:    request.EnableDirectPlay == nil || *request.EnableDirectPlay,
			ConvertAudio:        request.AllowAudioStreamCopy != nil && !*request.AllowAudioStreamCopy,
			Can:                 h.Playback.Capabilities(),
		})
	}
	// Above the user's bitrate limit, a track plays only converted.
	if decision.DirectPlay && overUserLimit(request.userLimit, d.source.Bitrate) {
		h.Logger.Info("A track above the user's bitrate limit is not played as it is", "addon", version.Addon)
		decision = playback.AudioDecision{}
	}
	if !decision.DirectPlay {
		if decision.Transcoding == nil || decision.Target != nil && !h.Accounts.Conversions(user).Audio {
			h.Logger.Info("A track would need a conversion the user may not have, or that the app does not take", "addon", version.Addon)
			writeJSON(w, http.StatusOK, noCompatibleStream{MediaSources: []MediaSourceInfo{}, ErrorCode: "NoCompatibleStream"})
			return
		}
		source.SupportsDirectPlay, source.SupportsDirectStream = false, false
		source.TranscodingUrl = audioTranscodingURL(r, item.ID, version, decision, session)
		source.TranscodingContainer = decision.Transcoding.Container
		source.TranscodingSubProtocol = "http"
		if decision.HLS {
			source.TranscodingSubProtocol = "hls"
		}
	}
	writeJSON(w, http.StatusOK, playbackInfoResponse{MediaSources: []MediaSourceInfo{source}, PlaySessionId: session})
}

// audioTranscodingURL is a track's TranscodingUrl, with Jellyfin's
// parameters in Jellyfin's order: a master playlist for HLS, else the
// converted stream.
func audioTranscodingURL(r *http.Request, item accounts.ID, version library.Version, decision playback.AudioDecision, session string) string {
	t := decision.Transcoding
	var q orderedQuery
	q.add("DeviceId", callerFrom(r.Context()).Device.DeviceID)
	q.add("MediaSourceId", item.String())
	if target := decision.Target; target != nil {
		q.add("AudioCodec", target.Codec)
		if target.Bitrate > 0 {
			q.add("AudioBitrate", strconv.FormatInt(target.Bitrate, 10))
		}
		if target.SampleRate > 0 {
			q.add("AudioSampleRate", strconv.Itoa(target.SampleRate))
		}
		if target.Channels > 0 {
			q.add("AudioChannels", strconv.Itoa(target.Channels))
		}
	} else {
		q.add("AudioCodec", "copy")
	}
	if decision.HLS {
		q.add("SegmentContainer", t.Container)
		if t.MinSegments != "" {
			q.add("MinSegments", t.MinSegments.String())
		}
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
	q.add("TranscodeReasons", strings.Join(decision.Reasons, ","))
	if decision.HLS {
		return "/audio/" + hyphenated(item) + "/master.m3u8?" + q.String()
	}
	return "/audio/" + hyphenated(item) + "/stream." + t.Container + "?" + q.String()
}

// audioRequest is what a request for a track's bytes is about: who plays
// it, the track and its version, and, when it is converted or copied into
// HLS, what to (target nil copies it).
type audioRequest struct {
	user    accounts.User
	grant   playback.Grant
	item    library.Item
	version library.Version
	target  *playback.AudioTarget
	session string
}

// audioOf resolves a request for a track's bytes, answering w when it
// cannot. The conversion the query asks for is held to what the user may
// have: none without the conversion permission, and no more than their
// bitrate limit.
func (h *Handler) audioOf(w http.ResponseWriter, r *http.Request, converted bool) (audioRequest, bool) {
	opened, ok := parseGUID(r.PathValue("itemId"))
	if !ok {
		validationProblem(w, map[string][]string{"itemId": {notValid(r.PathValue("itemId"))}})
		return audioRequest{}, false
	}
	user, grant, ok := h.streamAccess(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return audioRequest{}, false
	}
	item, err := h.played(r.Context(), user, opened)
	if err != nil || !library.AudioKind(item.Kind) {
		processingError(w, http.StatusNotFound)
		return audioRequest{}, false
	}
	wanted := item.ID
	for _, name := range []string{"mediaSourceId", "tag"} {
		if id, ok := parseGUID(query(r, name)); ok {
			wanted = id
		}
	}
	version, err := h.version(r.Context(), user, item, wanted)
	if errors.Is(err, library.ErrNotFound) && wanted != item.ID {
		version, err = h.version(r.Context(), user, item, item.ID)
	}
	if err != nil {
		processingError(w, http.StatusBadRequest)
		return audioRequest{}, false
	}
	req := audioRequest{user: user, grant: grant, item: item, version: version, session: query(r, "playSessionId")}
	if !converted {
		return req, true
	}
	codec := strings.ToLower(query(r, "audioCodec"))
	if codec == "" || codec == "copy" {
		return req, true
	}
	codec, _, _ = strings.Cut(codec, ",")
	if playback.AudioEncoder(codec) == "" {
		processingError(w, http.StatusBadRequest)
		return audioRequest{}, false
	}
	if !h.Accounts.Conversions(user).Audio {
		h.Logger.Info("A conversion the user may not have was refused", "addon", version.Addon)
		w.WriteHeader(http.StatusForbidden)
		return audioRequest{}, false
	}
	target := &playback.AudioTarget{Codec: codec}
	number := func(names ...string) int64 {
		for _, name := range names {
			if n, err := strconv.ParseInt(strings.TrimSpace(query(r, name)), 10, 64); err == nil && n > 0 {
				return n
			}
		}
		return 0
	}
	target.Bitrate = number("audioBitRate", "audioBitrate", "maxStreamingBitrate")
	if target.Bitrate == 0 && !losslessCodec(codec) {
		target.Bitrate = 128_000
	}
	if limit := int64(user.MaxBitrate); limit > 0 && target.Bitrate > limit {
		target.Bitrate = limit
	}
	if losslessCodec(codec) {
		target.Bitrate = 0
	}
	target.SampleRate = int(min(number("audioSampleRate", "maxAudioSampleRate"), 192_000))
	target.Channels = int(min(number("audioChannels", "maxAudioChannels", "transcodingMaxAudioChannels"), 8))
	if codec == "opus" {
		target.SampleRate = 48_000
	}
	req.target = target
	return req, true
}

func losslessCodec(codec string) bool {
	switch codec {
	case "flac", "alac", "wav", "pcm_s16le":
		return true
	}
	return false
}

// audioMimeTypes are the content types of audio containers.
var audioMimeTypes = map[string]string{
	"flac": "audio/flac", "mp3": "audio/mpeg", "m4a": "audio/mp4", "m4b": "audio/mp4", "mp4": "audio/mp4", "aac": "audio/aac",
	"ogg": "audio/ogg", "oga": "audio/ogg", "opus": "audio/ogg", "webm": "audio/webm", "webma": "audio/webm", "wav": "audio/wav",
}

// audioMuxers are the FFmpeg muxers of the containers tracks are
// converted to.
var audioMuxers = map[string]string{
	"mp3": "mp3", "aac": "adts", "m4a": "mp4", "mp4": "mp4", "flac": "flac", "ogg": "ogg", "oga": "ogg", "opus": "ogg",
	"webm": "webm", "webma": "webm", "wav": "wav",
}

// audioFile answers /Audio/{id}/stream, with an optional container
// extension, and the HLS playlists of a converted track.
func (h *Handler) audioFile(w http.ResponseWriter, r *http.Request) {
	name, extension, _ := strings.Cut(strings.ToLower(r.PathValue("file")), ".")
	switch {
	case extension == "m3u8" && (name == "master" || name == "main"):
		h.audioPlaylist(w, r, name == "master")
		return
	case name != "stream":
		processingError(w, http.StatusNotFound)
		return
	}
	static, _ := boolQuery(r, "static")
	req, ok := h.audioOf(w, r, !static)
	if !ok {
		return
	}
	if static || req.target == nil {
		h.serveTrack(w, r, req, extension)
		return
	}
	h.convertTrack(w, r, req, extension)
}

// serveTrack serves a track as it is: redirected to its source, or
// relayed (see playback.Service.Serve).
func (h *Handler) serveTrack(w http.ResponseWriter, r *http.Request, req audioRequest, extension string) {
	relay := req.grant.Relay || strings.Contains(r.Header.Get("Authorization"), "Token=")
	if analysis, known := h.Playback.Analyzed(r.Context(), req.version.ID); known && overUserLimit(int64(req.user.MaxBitrate), analysis.Bitrate) {
		h.Logger.Info("A track above the user's bitrate limit was refused", "addon", req.version.Addon)
		w.WriteHeader(http.StatusForbidden)
		return
	}
	contentType := audioMimeTypes[extension]
	if err := h.Playback.Serve(w, r, req.version, playback.Delivery{Relay: relay, ContentType: contentType}); err != nil && r.Context().Err() == nil {
		h.Logger.Warn("A track could not be served", "addon", req.version.Addon, "error", err)
	}
}

// convertTrack streams a track converted to the container of the
// extension, from startTimeTicks.
func (h *Handler) convertTrack(w http.ResponseWriter, r *http.Request, req audioRequest, extension string) {
	muxer := audioMuxers[extension]
	if muxer == "" {
		muxer, extension = "mp3", "mp3"
	}
	var start time.Duration
	if ticks, err := strconv.ParseInt(query(r, "startTimeTicks"), 10, 64); err == nil && ticks > 0 {
		start = time.Duration(ticks) * 100
	}
	w.Header().Set("Content-Type", audioMimeTypes[extension])
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		return
	}
	if err := h.Playback.ConvertAudio(r.Context(), req.version, req.target, muxer, start, w); err != nil && r.Context().Err() == nil {
		h.Logger.Warn("A track could not be converted", "addon", req.version.Addon, "error", err)
	}
}

// audioRemux is the HLS remux of a request, its segments in format.
func (h *Handler) audioRemux(ctx context.Context, req audioRequest, format hls.Format) (playback.AudioRemux, error) {
	duration, err := h.Playback.AudioDuration(ctx, req.version)
	if err != nil {
		return playback.AudioRemux{}, err
	}
	session := req.session
	if session == "" {
		session = req.user.ID.String() + "|" + req.version.ID.String()
	}
	return playback.AudioRemux{Session: session, Version: req.version, Duration: duration, Target: req.target, Format: format}, nil
}

// segmentFormat reads the segment container a request asks for: fMP4 by
// default, as jellyfin-web asks.
func segmentFormat(r *http.Request) hls.Format {
	if format, ok := hls.ParseFormat(query(r, "segmentContainer")); ok {
		return format
	}
	return hls.FMP4
}

// audioPlaylist answers the master or the media playlist of a track's
// HLS conversion, whose files carry the same query.
func (h *Handler) audioPlaylist(w http.ResponseWriter, r *http.Request, master bool) {
	req, ok := h.audioOf(w, r, true)
	if !ok {
		return
	}
	format := segmentFormat(r)
	remux, err := h.audioRemux(r.Context(), req, format)
	if err != nil {
		h.Logger.Warn("A track could not be analyzed", "addon", req.version.Addon, "error", err)
		processingError(w, http.StatusInternalServerError)
		return
	}
	var b bytes.Buffer
	if master {
		variant := hls.Variant{Bandwidth: 128_000, Codecs: audioCodecString(req.target)}
		if req.target != nil && req.target.Bitrate > 0 {
			variant.Bandwidth = req.target.Bitrate
		}
		err = hls.WriteMaster(&b, variant, "main.m3u8?"+r.URL.RawQuery)
	} else {
		err = hls.WriteMedia(&b, hls.AudioPlan(remux.Duration), format, func(n int) string {
			return "hls1/main/" + strconv.Itoa(n) + "." + format.Extension() + "?" + r.URL.RawQuery
		})
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b.Bytes())
}

// audioCodecString is the RFC 6381 codec of converted audio, empty when
// copied or unknown.
func audioCodecString(target *playback.AudioTarget) string {
	if target == nil {
		return ""
	}
	switch target.Codec {
	case "aac":
		return "mp4a.40.2"
	case "mp3":
		return "mp4a.40.34"
	case "flac":
		return "fLaC"
	case "opus":
		return "Opus"
	case "ac3":
		return "ac-3"
	case "eac3":
		return "ec-3"
	}
	return ""
}

// audioSegment serves a segment of a track's HLS conversion:
// hls1/{playlist}/{n}.{mp4|ts}, -1 being the initialization segment.
func (h *Handler) audioSegment(w http.ResponseWriter, r *http.Request) {
	name, _, _ := strings.Cut(r.PathValue("file"), ".")
	n, err := strconv.Atoi(name)
	if err != nil || n < -1 {
		processingError(w, http.StatusNotFound)
		return
	}
	req, ok := h.audioOf(w, r, true)
	if !ok {
		return
	}
	format := segmentFormat(r)
	remux, err := h.audioRemux(r.Context(), req, format)
	if err != nil {
		processingError(w, http.StatusInternalServerError)
		return
	}
	var file interface {
		Close() error
	}
	open := func() error {
		if n < 0 {
			f, err := h.Playback.AudioInit(r.Context(), remux)
			if err == nil {
				file = f
				w.Header().Set("Content-Type", "video/mp4")
				http.ServeContent(w, r, "", time.Time{}, f)
			}
			return err
		}
		f, err := h.Playback.AudioSegment(r.Context(), remux, n)
		if err == nil {
			file = f
			w.Header().Set("Content-Type", format.ContentType())
			http.ServeContent(w, r, "", time.Time{}, f)
		}
		return err
	}
	if err := open(); err != nil {
		switch {
		case r.Context().Err() != nil:
		case errors.Is(err, hls.ErrNotFound), errors.Is(err, hls.ErrStopped):
			processingError(w, http.StatusNotFound)
		default:
			h.Logger.Warn("An audio segment could not be made", "addon", req.version.Addon, "error", err)
			processingError(w, http.StatusInternalServerError)
		}
		return
	}
	_ = file.Close()
}

// universalAudio answers /Audio/{id}/universal, which music players use:
// it decides from its parameters as PlaybackInfo decides from a device
// profile (Container lists the formats played as they are, as container
// or container|codec), then serves the track as it is, its HLS master
// playlist, or the converted stream.
func (h *Handler) universalAudio(w http.ResponseWriter, r *http.Request) {
	req, ok := h.audioOf(w, r, false)
	if !ok {
		return
	}
	d, err := h.describeAudio(r.Context(), req.version)
	if err != nil {
		h.Logger.Info("A track could not be analyzed", "addon", req.version.Addon, "error", err)
		processingError(w, http.StatusInternalServerError)
		return
	}
	profile := universalProfile(r)
	limit, _ := strconv.ParseInt(query(r, "maxStreamingBitrate"), 10, 64)
	if user := int64(req.user.MaxBitrate); user > 0 && (limit <= 0 || limit > user) {
		limit = user
	}
	decision := playback.DecideAudio(profile, d.source, playback.AudioOptions{MaxStreamingBitrate: limit, EnableDirectPlay: true,
		Can: h.Playback.Capabilities()})
	if decision.DirectPlay {
		extension := d.source.Container
		h.serveTrack(w, r, req, extension)
		return
	}
	if decision.Transcoding == nil || decision.Target != nil && !h.Accounts.Conversions(req.user).Audio {
		h.Logger.Info("A track would need a conversion the user may not have, or that the app does not take", "addon", req.version.Addon)
		processingError(w, http.StatusBadRequest)
		return
	}
	req.target = decision.Target
	if !decision.HLS {
		h.convertTrack(w, r, req, strings.ToLower(decision.Transcoding.Container))
		return
	}
	// The master playlist, whose media playlist is main.m3u8 beside this
	// route, carries what it was decided to.
	values := r.URL.Query()
	values.Set("segmentContainer", decision.Transcoding.Container)
	if req.target != nil {
		values.Set("audioCodec", req.target.Codec)
		values.Set("audioBitrate", strconv.FormatInt(req.target.Bitrate, 10))
		values.Set("audioSampleRate", strconv.Itoa(req.target.SampleRate))
		values.Set("audioChannels", strconv.Itoa(req.target.Channels))
	} else {
		values.Set("audioCodec", "copy")
	}
	variant := hls.Variant{Bandwidth: max(cmp.Or(limit, 128_000), 1), Codecs: audioCodecString(req.target)}
	if req.target != nil && req.target.Bitrate > 0 {
		variant.Bandwidth = req.target.Bitrate
	}
	var b bytes.Buffer
	if err := hls.WriteMaster(&b, variant, "main.m3u8?"+values.Encode()); err != nil {
		h.internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b.Bytes())
}

// universalProfile is the device profile /Audio/{id}/universal describes
// with its parameters.
func universalProfile(r *http.Request) *playback.DeviceProfile {
	profile := &playback.DeviceProfile{Name: "Universal"}
	for _, entry := range listQuery(r, "container") {
		container, codec, _ := strings.Cut(entry, "|")
		profile.DirectPlayProfiles = append(profile.DirectPlayProfiles, playback.DirectPlayProfile{Type: "Audio", Container: container, AudioCodec: codec})
	}
	if container := query(r, "transcodingContainer"); container != "" {
		protocol := cmp.Or(query(r, "transcodingProtocol"), "http")
		codec := cmp.Or(query(r, "audioCodec"), container)
		profile.TranscodingProfiles = []playback.TranscodingProfile{{Type: "Audio", Context: "Streaming", Container: container, Protocol: protocol,
			AudioCodec: codec, MaxAudioChannels: query(r, "transcodingAudioChannels")}}
	}
	var conditions []playback.ProfileCondition
	for name, property := range map[string]string{"maxAudioSampleRate": "AudioSampleRate", "maxAudioBitDepth": "AudioBitDepth", "maxAudioChannels": "AudioChannels"} {
		if value := query(r, name); value != "" {
			conditions = append(conditions, playback.ProfileCondition{Condition: "LessThanEqual", Property: property, Value: value})
		}
	}
	if len(conditions) > 0 {
		profile.CodecProfiles = []playback.CodecProfile{{Type: "Audio", Conditions: conditions}}
	}
	return profile
}

// addAudioSources describes a track's stream in its DTO, as Jellyfin
// describes a song's file: its details ask the addon where it streams
// from, listings only show what is known. An audiobook's chapters are the
// ones its stream comes with.
func (h *Handler) addAudioSources(r *http.Request, user accounts.User, dto *BaseItemDto, item library.Item, detail bool) {
	var versions []library.Version
	if detail {
		versions, _ = h.Library.Versions(r.Context(), user, item.ID)
	} else {
		versions, _ = h.Library.CachedVersions(r.Context(), user, item.ID)
	}
	sources := []MediaSourceInfo{}
	streams := []playback.MediaStream{}
	if len(versions) > 0 {
		version := versions[0]
		d, err := h.describeAudio(r.Context(), version)
		if err == nil || !detail {
			if err != nil {
				d = describedAudio{analysis: media.Analysis{Duration: version.Runtime}}
			}
			source := h.audioSource(r, item, version, d)
			sources, streams = []MediaSourceInfo{source}, source.MediaStreams
			// Jellyfin gives a song's container, not an audiobook's.
			if item.Kind == library.KindTrack {
				dto.Container = source.Container
			}
		}
		if a := version.Audio; detail && a != nil && item.Kind == library.KindAudiobook && len(a.Chapters) > 0 {
			chapters := make([]ChapterInfo, 0, len(a.Chapters))
			for i, c := range a.Chapters {
				name := c.Title
				if name == "" {
					name = library.ChapterName(h.Accounts.Settings().Language, i+1)
				}
				chapters = append(chapters, ChapterInfo{StartPositionTicks: int64(c.Start / 100), Name: name, ImageDateModified: Time(time.Time{})})
			}
			dto.Chapters = &chapters
		}
	}
	dto.MediaSources, dto.MediaStreams = &sources, &streams
}
