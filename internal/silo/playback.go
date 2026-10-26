package silo

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/playback"
)

func (s *Server) playbackRoutes() {
	s.mux.HandleFunc("GET /api/v2/playback/capabilities", s.profiled(s.playbackCapabilities))
	s.mux.HandleFunc("POST /api/v2/playback/start", s.profiled(s.startPlayback))
	s.mux.HandleFunc("POST /api/v2/playback/route-events", s.profiled(s.routeEvent))
	s.mux.HandleFunc("POST /api/v2/playback/{session}/replan", s.profiled(s.replan))
	s.mux.HandleFunc("POST /api/v2/playback/{session}/progress", s.profiled(s.progress))
	s.mux.HandleFunc("DELETE /api/v2/playback/{session}", s.profiled(s.stop))
	// The media of a playback session, which the players fetch with the
	// apps' bearer token, as the contract's media addresses are.
	s.mux.HandleFunc("GET /api/v2/stream/{session}", s.signedIn(s.original))
	s.mux.HandleFunc("GET /api/v2/stream/{session}/subtitles/{file}", s.signedIn(s.subtitle))
	s.mux.HandleFunc("GET /api/v2/playback/transcode/{session}/{path...}", s.signedIn(s.converted))
}

// The protocol of playback plans Polyfin speaks.
const (
	planProtocol       = 3
	sequencedProgress  = "sequenced_progress_v1"
	headerAuthenticate = "header_authenticated_media_v1"
)

// planFeatures are the features of the protocol Polyfin's plans honor,
// which each decision names: the plan itself; its neutral contract, plan
// attempt keys the server mints and replans for a change of tracks or
// quality apart from those after a failure; and media fetched with the
// apps' bearer token, no credential in their addresses.
var planFeatures = []string{"playback_plan_v3", "neutral_playback_v3_contract_v1", headerAuthenticate}

// capabilityFeatures are what the capabilities document advertises, the
// apps' condition to start a playback: the plans' features, and progress
// and stops numbered per installation. Nothing Polyfin does not do is
// advertised (see docs/silo-apps.md).
var capabilityFeatures = append(slices.Clone(planFeatures), sequencedProgress)

// The deliveries of a plan: the file as it is, repackaged or converted to
// HLS by Polyfin's conversions.
const (
	originalHTTP = "original_http"
	repackaged   = "server_remux_hls"
	converted    = "server_transcode_hls"
)

// playbackCapabilities tells the apps playback is available, and the
// installation their progress is numbered for: their Polyfin device.
func (s *Server) playbackCapabilities(w http.ResponseWriter, _ *http.Request, v viewer) {
	writeJSON(w, http.StatusOK, struct {
		State            string   `json:"state"`
		Allowed          bool     `json:"allowed"`
		InstallationID   string   `json:"installation_id"`
		ProtocolVersions []int    `json:"protocol_versions"`
		Features         []string `json:"features"`
		Deliveries       []string `json:"deliveries"`
		Revision         string   `json:"revision"`
	}{"available", true, v.device.ID.String(), []int{planProtocol}, capabilityFeatures,
		[]string{originalHTTP, repackaged, converted}, "polyfin-2"})
}

// The client's capabilities, as the start and replan bodies give them.
type clientCapabilities struct {
	CodecsAudio   []string `json:"codecs_audio"`
	CodecsVideo   []string `json:"codecs_video"`
	Containers    []string `json:"containers"`
	MaxResolution string   `json:"max_resolution"`
}

type deliveryCapability struct {
	Enabled           bool     `json:"enabled"`
	SupportedOnDevice bool     `json:"supported_on_device"`
	Containers        []string `json:"containers"`
	VideoCodecs       []string `json:"video_codecs"`
	AudioDecode       []string `json:"audio_decode_codecs"`
	AudioPassthrough  []string `json:"audio_passthrough_codecs"`
	Subtitles         struct {
		SidecarText bool `json:"sidecar_text"`
	} `json:"subtitles"`
}

type playbackContext struct {
	Deliveries map[string]deliveryCapability `json:"deliveries"`
}

type trackIdentity struct {
	ID    string `json:"id"`
	Index *int   `json:"index,omitempty"`
}

type selectedTracks struct {
	Audio    *trackIdentity `json:"audio,omitempty"`
	Subtitle *trackIdentity `json:"subtitle,omitempty"`
}

type startBody struct {
	ProtocolVersion    int                `json:"protocol_version"`
	FileID             string             `json:"file_id"`
	ProfileID          string             `json:"profile_id"`
	InstallationID     string             `json:"installation_id"`
	PlaybackAttemptID  string             `json:"playback_attempt_id"`
	QualityPreference  string             `json:"quality_preference"`
	StartPosition      *float64           `json:"start_position"`
	AudioTrackID       *string            `json:"audio_track_id"`
	AudioTrackIndex    *int               `json:"audio_track_index"`
	SubtitleTrackID    *string            `json:"subtitle_track_id"`
	SubtitleTrackIndex *int               `json:"subtitle_track_index"`
	BandwidthCapKbps   *int64             `json:"bandwidth_cap_kbps"`
	BandwidthEstimate  *int64             `json:"bandwidth_estimate_kbps"`
	Capabilities       clientCapabilities `json:"client_capabilities"`
	Context            playbackContext    `json:"client_playback_context"`
}

type replanBody struct {
	InstallationID    string             `json:"installation_id"`
	PlaybackAttemptID string             `json:"playback_attempt_id"`
	ReplanRequestID   string             `json:"replan_request_id"`
	Operation         string             `json:"operation"`
	QualityPreference string             `json:"quality_preference"`
	PositionSeconds   float64            `json:"position_seconds"`
	SelectedTracks    selectedTracks     `json:"selected_tracks"`
	BandwidthCapKbps  *int64             `json:"bandwidth_cap_kbps"`
	BandwidthEstimate *int64             `json:"bandwidth_estimate_kbps"`
	Capabilities      clientCapabilities `json:"client_capabilities"`
	Context           playbackContext    `json:"client_playback_context"`
}

// choice is what a plan is decided from: the client, the quality and
// tracks asked, and whether the file may play as it is.
type choice struct {
	capabilities clientCapabilities
	context      playbackContext
	quality      string
	bitrateKbps  int64
	// audio is the audio track asked, by its place among the version's
	// audio tracks, nil for the server's choice; subtitle is the subtitle
	// asked, by its place among the version's subtitles (see
	// subtitleOrder), nil for none, as the apps mean them.
	audio, subtitle *int
	original        bool
}

// session is a playback the apps started, with the Jellyfin API's play
// session that plays it.
type session struct {
	// mu guards the session's fields but id, user and seen, which the
	// store's lock guards.
	mu           sync.Mutex
	id           string
	user         accounts.ID
	device       accounts.DeviceInfo
	token        string
	installation string
	attempt      string
	file         string
	version      string
	playState
	// replans are the answers of the last replans, by their identifier,
	// in the order of replanOrder.
	replans     map[string][]byte
	replanOrder []string
	// The last progress sample applied, and the stop received.
	sequence int64
	sample   sample
	stopID   string
	seen     time.Time
}

// playState is what a plan is decided from, and the current plan: the
// Jellyfin play session and version, how it plays, and where its stream
// and subtitles are.
type playState struct {
	choice      choice
	position    float64
	playSession string
	source      mediaSource
	method      string
	stream      string
	subtitles   map[int]string
	audio       *int
	subtitle    *int
}

type sample struct {
	Sequence int64   `json:"sequence"`
	Position float64 `json:"position"`
	IsPaused bool    `json:"is_paused"`
}

// sessionLifetime is how long a playback nobody stopped is kept unused.
const sessionLifetime = 24 * time.Hour

type sessions struct {
	mu   sync.Mutex
	byID map[string]*session
}

func newSessions() *sessions { return &sessions{byID: map[string]*session{}} }

// sweep forgets the sessions unused for a day; s.mu is held.
func (s *sessions) sweep(now time.Time) {
	for key, other := range s.byID {
		if now.Sub(other.seen) > sessionLifetime {
			delete(s.byID, key)
		}
	}
}

// get is a session of user, locked: done unlocks it.
func (s *sessions) get(id string, user accounts.ID, now time.Time) (*session, func(), bool) {
	s.mu.Lock()
	s.sweep(now)
	found, ok := s.byID[id]
	if ok && found.user == user {
		found.seen = now
	}
	s.mu.Unlock()
	if !ok || found.user != user {
		return nil, nil, false
	}
	found.mu.Lock()
	return found, found.mu.Unlock, true
}

// add keeps a session started at its seen time.
func (s *sessions) add(found *session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(found.seen)
	s.byID[found.id] = found
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// newSessionID is a playback session's identifier: a random UUID, written
// in lower case with its hyphens, the only form the apps take.
func newSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// containerNames are the Jellyfin API's names of the containers the apps
// name otherwise.
var containerNames = map[string]string{"matroska": "mkv", "mpegts": "ts", "fmp4": "mp4", "quicktime": "mov"}

func joined(values []string) string {
	var names []string
	for _, value := range values {
		name := strings.ToLower(strings.TrimSpace(value))
		name = cmp.Or(containerNames[name], name)
		if name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return strings.Join(names, ",")
}

// heights are the heights of the qualities the apps may ask.
var heights = map[string]int{"2160p": 2160, "4k": 2160, "1440p": 1440, "1080p": 1080, "720p": 720, "480p": 480, "360p": 360}

// rung is a quality a plan offers below its source: a conversion to its
// height, at its bitrate at most.
type rung struct {
	label  string
	height int
	kbps   int64
}

// rungs are the qualities plans offer, as many as a source is taller than.
var rungs = []rung{{"1080p", 1080, 8000}, {"720p", 720, 4000}, {"480p", 480, 1500}}

// rungOf is the rung a quality names.
func rungOf(quality string) (rung, bool) {
	i := slices.IndexFunc(rungs, func(r rung) bool { return strings.EqualFold(r.label, quality) })
	if i < 0 {
		return rung{}, false
	}
	return rungs[i], true
}

// deviceProfile is the Jellyfin device profile that stands for the
// client: the files it plays as they are, the HLS it plays converted, and
// text subtitles as WebVTT files beside the video.
func deviceProfile(c choice) playback.DeviceProfile {
	containers, video, audio := c.capabilities.Containers, c.capabilities.CodecsVideo, c.capabilities.CodecsAudio
	if original, ok := c.context.Deliveries[originalHTTP]; ok && original.Enabled && original.SupportedOnDevice {
		if len(original.Containers) > 0 {
			containers = original.Containers
		}
		if len(original.VideoCodecs) > 0 {
			video = original.VideoCodecs
		}
		if decoded := slices.Concat(original.AudioDecode, original.AudioPassthrough); len(decoded) > 0 {
			audio = decoded
		}
	}
	profile := playback.DeviceProfile{Name: appName}
	if c.original {
		for _, container := range strings.Split(joined(containers), ",") {
			if container != "" {
				profile.DirectPlayProfiles = append(profile.DirectPlayProfiles, playback.DirectPlayProfile{Container: container,
					VideoCodec: joined(video), AudioCodec: joined(audio), Type: "Video"})
			}
		}
	}
	hlsVideo, hlsAudio, segments := []string{"h264"}, []string{"aac"}, "ts"
	if hls, ok := c.context.Deliveries["hls"]; ok && hls.Enabled {
		for _, codec := range []string{"hevc", "av1"} {
			if slices.Contains(hls.VideoCodecs, codec) {
				hlsVideo = append(hlsVideo, codec)
			}
		}
		for _, codec := range []string{"ac3", "eac3", "mp3", "opus", "flac"} {
			if slices.Contains(hls.AudioDecode, codec) || slices.Contains(hls.AudioPassthrough, codec) {
				hlsAudio = append(hlsAudio, codec)
			}
		}
		if slices.ContainsFunc(hls.Containers, func(c string) bool { return c == "mp4" || c == "fmp4" }) {
			segments = "mp4"
		}
	}
	profile.TranscodingProfiles = []playback.TranscodingProfile{{Container: segments, Type: "Video", VideoCodec: strings.Join(hlsVideo, ","),
		AudioCodec: strings.Join(hlsAudio, ","), Protocol: "hls", Context: "Streaming", MinSegments: "1", BreakOnNonKeyFrames: true}}
	height := heights[strings.ToLower(c.quality)]
	if limit := heights[strings.ToLower(c.capabilities.MaxResolution)]; limit > 0 && (height == 0 || limit < height) {
		height = limit
	}
	if height > 0 {
		profile.CodecProfiles = []playback.CodecProfile{{Type: "Video", Conditions: []playback.ProfileCondition{
			{Condition: "LessThanEqual", Property: "Height", Value: strconv.Itoa(height)}}}}
	}
	profile.SubtitleProfiles = []playback.SubtitleProfile{{Format: "vtt", Method: "External"},
		{Format: "pgssub", Method: "Encode"}, {Format: "dvdsub", Method: "Encode"}, {Format: "dvbsub", Method: "Encode"}}
	return profile
}

type playbackInfo struct {
	MediaSources  []mediaSource
	PlaySessionId string
	ErrorCode     string
}

// The parts of a decision, as the contract names them.
type (
	streamPlan struct {
		URL           string            `json:"url"`
		Protocol      string            `json:"protocol"`
		Container     string            `json:"container,omitempty"`
		MimeType      string            `json:"mime_type,omitempty"`
		Headers       map[string]string `json:"headers"`
		HeaderRefresh string            `json:"header_refresh"`
	}
	timelinePlan struct {
		SourceStart     float64 `json:"source_start_seconds"`
		StreamOrigin    float64 `json:"stream_origin_seconds"`
		PlayerStart     float64 `json:"player_start_seconds"`
		TimelineOffset  float64 `json:"timeline_offset_seconds"`
		CanSeekAnywhere bool    `json:"can_seek_anywhere"`
		SeekRestoration string  `json:"seek_restoration"`
	}
	recipePlan struct {
		VideoCodec    string `json:"video_codec,omitempty"`
		AudioCodec    string `json:"audio_codec,omitempty"`
		Width         int    `json:"width,omitempty"`
		Height        int    `json:"height,omitempty"`
		BitrateKbps   int64  `json:"bitrate_kbps,omitempty"`
		AudioChannels int    `json:"audio_channels,omitempty"`
	}
	claimsPlan struct {
		Audio struct {
			AtmosPreserved bool `json:"atmos_preserved"`
			Passthrough    bool `json:"passthrough"`
		} `json:"audio"`
		Subtitles struct {
			AssStylingPreserved bool `json:"ass_styling_preserved"`
			BitmapOverlay       bool `json:"bitmap_overlay"`
			BitmapSidecar       bool `json:"bitmap_sidecar"`
		} `json:"subtitles"`
		Video struct {
			DolbyVision bool `json:"dolby_vision"`
			HDR10       bool `json:"hdr10"`
			HDR10Plus   bool `json:"hdr10_plus"`
			HLG         bool `json:"hlg"`
		} `json:"video"`
	}
	subtitleArtifact struct {
		URL          string  `json:"url"`
		MimeType     string  `json:"mime_type"`
		Format       string  `json:"format"`
		TimingOrigin float64 `json:"timing_origin_seconds"`
	}
	subtitleRow struct {
		TrackID         string `json:"track_id"`
		CombinedIndex   int    `json:"combined_index"`
		Source          string `json:"source"`
		Codec           string `json:"codec,omitempty"`
		Language        string `json:"language,omitempty"`
		Label           string `json:"label,omitempty"`
		Forced          bool   `json:"forced"`
		Default         bool   `json:"default"`
		HearingImpaired bool   `json:"hearing_impaired"`
		Delivery        string `json:"delivery"`
		URL             string `json:"url,omitempty"`
	}
	subtitlePlan struct {
		Mode      string            `json:"mode"`
		TrackID   string            `json:"track_id,omitempty"`
		Artifact  *subtitleArtifact `json:"artifact,omitempty"`
		Inventory []subtitleRow     `json:"inventory"`
	}
	qualityRung struct {
		Label           string `json:"label"`
		Height          int    `json:"height,omitempty"`
		BitrateKbps     int64  `json:"bitrate_kbps"`
		PreservesSource bool   `json:"preserves_source"`
	}
	sourcePlan struct {
		MediaFileID        string  `json:"media_file_id"`
		Container          string  `json:"container,omitempty"`
		VideoCodec         string  `json:"video_codec,omitempty"`
		AudioCodec         string  `json:"audio_codec,omitempty"`
		Width              int     `json:"width,omitempty"`
		Height             int     `json:"height,omitempty"`
		BitrateKbps        int64   `json:"bitrate_kbps,omitempty"`
		DurationSeconds    float64 `json:"duration_seconds,omitempty"`
		DynamicRange       string  `json:"dynamic_range,omitempty"`
		DVEnhancementLayer string  `json:"dv_enhancement_layer"`
		HDR10Plus          bool    `json:"hdr10_plus"`
	}
	plan struct {
		ProtocolVersion        int            `json:"protocol_version"`
		PlanID                 string         `json:"plan_id"`
		PlanAttemptKey         string         `json:"plan_attempt_key"`
		SessionID              string         `json:"session_id"`
		Delivery               string         `json:"delivery"`
		Stream                 streamPlan     `json:"stream"`
		Timeline               timelinePlan   `json:"timeline"`
		SelectedTracks         selectedTracks `json:"selected_tracks"`
		EffectiveRecipe        recipePlan     `json:"effective_recipe"`
		Claims                 claimsPlan     `json:"claims"`
		Subtitle               subtitlePlan   `json:"subtitle"`
		SubtitleFidelityPolicy string         `json:"subtitle_fidelity_policy"`
		Transformations        []struct{}     `json:"transformations"`
		AppliedQuirks          []struct{}     `json:"applied_quirks"`
		RuntimeCorrections     []string       `json:"runtime_corrections"`
		AvailableQualities     []qualityRung  `json:"available_qualities"`
		ActiveQuality          string         `json:"active_quality,omitempty"`
		DegradationWarnings    []struct{}     `json:"degradation_warnings"`
		DecisionReason         string         `json:"decision_reason"`
		RequestedMediaFileID   string         `json:"requested_media_file_id"`
		EffectiveMediaFileID   string         `json:"effective_media_file_id"`
		Source                 sourcePlan     `json:"source"`
	}
	terminal struct {
		Reason    string `json:"reason"`
		Message   string `json:"message"`
		Retryable bool   `json:"retryable"`
	}
	decision struct {
		ProtocolVersion int       `json:"protocol_version"`
		ServerFeatures  []string  `json:"server_features"`
		Outcome         string    `json:"outcome"`
		SessionID       string    `json:"session_id,omitempty"`
		Plan            *plan     `json:"playback_plan,omitempty"`
		Terminal        *terminal `json:"terminal,omitempty"`
	}
)

// unavailable is the decision when nothing of the title plays for the
// client under the user's limits.
func unavailable(reason, message string, retryable bool) decision {
	return decision{ProtocolVersion: planProtocol, ServerFeatures: planFeatures, Outcome: "adaptation_unavailable",
		Terminal: &terminal{Reason: reason, Message: message, Retryable: retryable}}
}

// tracks are the tracks a play session plays, as the Jellyfin API numbers
// streams: audio nil for the version's default, subtitle -1 for none.
type tracks struct {
	audio    *int
	subtitle int
}

// ask asks the Jellyfin API how the session's version plays for its
// client with the tracks asked, as PlaybackInfo decides for Jellyfin apps:
// the version rules, the user's quality group, bitrate limit and
// conversion permissions apply. It answers the decision to give when
// nothing plays.
func (s *Server) ask(r *http.Request, found *session, asked tracks) (playbackInfo, *decision, error) {
	body := map[string]any{"UserId": found.user.String(), "MediaSourceId": found.version, "DeviceProfile": deviceProfile(found.choice),
		"EnableDirectPlay": found.choice.original, "EnableDirectStream": found.choice.original, "AutoOpenLiveStream": true,
		"SubtitleStreamIndex": asked.subtitle}
	kbps := found.choice.bitrateKbps
	if q, ok := rungOf(found.choice.quality); ok && (kbps == 0 || q.kbps < kbps) {
		kbps = q.kbps
	}
	if kbps > 0 {
		body["MaxStreamingBitrate"] = kbps * 1000
	}
	if asked.audio != nil {
		body["AudioStreamIndex"] = *asked.audio
	}
	var info playbackInfo
	a, err := s.callAs(r, found.device, found.token, http.MethodPost, "/Items/"+found.version+"/PlaybackInfo", body, &info)
	if err != nil {
		return info, nil, err
	}
	var d *decision
	switch {
	case a.status == http.StatusNotFound:
		d = new(unavailable("not_found", "This title is no longer available.", false))
	case a.status == http.StatusForbidden:
		d = new(unavailable("not_allowed", "This account may not play now.", false))
	case !a.ok():
		d = new(unavailable("unavailable", "This title could not be prepared.", true))
	case len(info.MediaSources) == 0 || info.ErrorCode != "" ||
		!info.MediaSources[0].SupportsDirectPlay && info.MediaSources[0].TranscodingUrl == "":
		d = new(unavailable("no_compatible_stream", "No version of this title plays on this device for this account.", false))
	}
	if d != nil {
		// Nothing plays: what PlaybackInfo prepared for it is stopped.
		s.stopPlaySession(r, found, info.PlaySessionId)
	}
	return info, d, nil
}

// stopPlaySession stops the conversions of a Jellyfin play session of the
// session's device, as a Jellyfin app does when it gives a play session up.
func (s *Server) stopPlaySession(r *http.Request, found *session, playSession string) {
	if playSession == "" {
		return
	}
	_, _ = s.callAs(r, found.device, found.token, http.MethodDelete, "/Videos/ActiveEncodings?"+url.Values{
		"deviceId": {found.device.DeviceID}, "playSessionId": {playSession}}.Encode(), nil, nil)
}

// subtitleOrder are a version's subtitles in the order the apps count
// them: the files beside the video first, then the file's own tracks,
// each in the Jellyfin API's order.
func subtitleOrder(source mediaSource) []playback.MediaStream {
	subtitles := streamsOf(source, "Subtitle")
	slices.SortStableFunc(subtitles, func(a, b playback.MediaStream) int {
		switch {
		case a.IsExternal == b.IsExternal:
			return 0
		case a.IsExternal:
			return -1
		}
		return 1
	})
	return subtitles
}

// trackID names a track of a file as the apps name one: its kind and its
// place, among the file's audio tracks or among its subtitles.
func trackID(file, kind string, place int) string {
	return "file:" + file + ":" + kind + ":" + strconv.Itoa(place)
}

// trackPlace is the place of the track the apps ask by its place, or by
// an identifier of trackID's form; nil when they ask none.
func trackPlace(id *string, index *int, kind string) *int {
	if index != nil {
		if *index < 0 {
			return nil
		}
		return index
	}
	if id == nil {
		return nil
	}
	parts := strings.Split(*id, ":")
	if len(parts) != 4 || parts[0] != "file" || parts[2] != kind {
		return nil
	}
	place, err := strconv.Atoi(parts[3])
	if err != nil || place < 0 {
		return nil
	}
	return &place
}

// tracksAt are the Jellyfin streams of a version at the places c asks: a
// place the version lacks is left to the version's default audio, or
// plays no subtitle.
func tracksAt(source mediaSource, c choice) tracks {
	asked := tracks{subtitle: -1}
	if audio := streamsOf(source, "Audio"); c.audio != nil && *c.audio < len(audio) {
		asked.audio = &audio[*c.audio].Index
	}
	if subtitles := subtitleOrder(source); c.subtitle != nil && *c.subtitle < len(subtitles) {
		asked.subtitle = subtitles[*c.subtitle].Index
	}
	return asked
}

// withoutKey drops the access token from a Jellyfin address, keeping its
// other parameters as they are, well formed or not: the apps send their
// token in a header.
func withoutKey(target string) string {
	path, query, _ := strings.Cut(target, "?")
	var kept []string
	for _, part := range strings.FieldsFunc(query, func(r rune) bool { return r == '&' || r == ';' }) {
		name, _, _ := strings.Cut(part, "=")
		if unescaped, err := url.QueryUnescape(name); err == nil {
			name = unescaped
		}
		if !strings.EqualFold(name, "ApiKey") && !strings.EqualFold(name, "api_key") {
			kept = append(kept, part)
		}
	}
	if len(kept) == 0 {
		return path
	}
	return path + "?" + strings.Join(kept, "&")
}

// codecsAllowed reads a codec list of a conversion's address.
func codecsAllowed(values url.Values, name string) []string {
	for key, value := range values {
		if strings.EqualFold(key, name) && len(value) > 0 {
			return strings.Split(strings.ToLower(value[0]), ",")
		}
	}
	return nil
}

// decide makes the plan of the session's play session, which plays the
// tracks asked.
func (s *Server) decide(ctx context.Context, found *session, info playbackInfo, asked tracks) (decision, error) {
	source := info.MediaSources[0]
	found.playSession, found.source = info.PlaySessionId, source
	video := streamsOf(source, "Video")
	audio := streamsOf(source, "Audio")
	audioTrack, hasAudio := defaultAudio(source)
	if asked.audio != nil {
		if i := slices.IndexFunc(audio, func(m playback.MediaStream) bool { return m.Index == *asked.audio }); i >= 0 {
			audioTrack, hasAudio = audio[i], true
		}
	}
	p := plan{ProtocolVersion: planProtocol, PlanID: randomID(), PlanAttemptKey: randomID(), SessionID: found.id,
		Stream:   streamPlan{Headers: map[string]string{}, HeaderRefresh: "session"},
		Timeline: timelinePlan{PlayerStart: found.position, CanSeekAnywhere: true, SeekRestoration: "player_position"},
		Subtitle: subtitlePlan{Mode: "off", Inventory: []subtitleRow{}}, SubtitleFidelityPolicy: "compatible",
		Transformations: []struct{}{}, AppliedQuirks: []struct{}{}, RuntimeCorrections: []string{}, AvailableQualities: []qualityRung{},
		DegradationWarnings: []struct{}{}, RequestedMediaFileID: found.file, EffectiveMediaFileID: found.file}
	if id, err := accounts.ParseID(source.Id); err == nil {
		number, err := s.numbers.number(ctx, id)
		if err != nil {
			return decision{}, err
		}
		p.EffectiveMediaFileID = number
	}
	p.Source = sourcePlan{MediaFileID: p.EffectiveMediaFileID, Container: strings.Split(source.Container, ",")[0],
		DurationSeconds: seconds(source.RunTimeTicks), DVEnhancementLayer: "none", DynamicRange: "sdr"}
	if source.Bitrate != nil {
		p.Source.BitrateKbps = *source.Bitrate / 1000
	}
	height := 0
	if len(video) > 0 {
		p.Source.VideoCodec, p.Source.Width, p.Source.Height = video[0].Codec, intOf(video[0].Width), intOf(video[0].Height)
		height = p.Source.Height
		if video[0].VideoRange == "HDR" {
			p.Source.DynamicRange = strings.ToLower(cmp.Or(video[0].VideoRangeType, "hdr10"))
		}
	}
	found.audio = nil
	if hasAudio {
		p.Source.AudioCodec = audioTrack.Codec
		place := slices.IndexFunc(audio, func(m playback.MediaStream) bool { return m.Index == audioTrack.Index })
		p.SelectedTracks.Audio = &trackIdentity{ID: trackID(p.EffectiveMediaFileID, "audio", place), Index: &place}
		found.audio = &audioTrack.Index
	}
	p.EffectiveRecipe = recipePlan{VideoCodec: p.Source.VideoCodec, AudioCodec: p.Source.AudioCodec, Width: p.Source.Width,
		Height: p.Source.Height, BitrateKbps: p.Source.BitrateKbps}
	if hasAudio {
		p.EffectiveRecipe.AudioChannels = intOf(audioTrack.Channels)
	}
	for _, q := range rungs {
		if height > q.height+q.height/4 {
			p.AvailableQualities = append(p.AvailableQualities, qualityRung{Label: q.label, Height: q.height, BitrateKbps: q.kbps})
		}
	}
	if source.SupportsDirectPlay {
		found.method = "DirectPlay"
		found.stream = "/Videos/" + found.version + "/stream?" + url.Values{"static": {"true"}, "mediaSourceId": {source.Id},
			"PlaySessionId": {info.PlaySessionId}}.Encode()
		p.Delivery, p.DecisionReason = originalHTTP, "direct_play"
		p.Stream.URL, p.Stream.Protocol, p.Stream.Container = "/api/v2/stream/"+found.id, "http_progressive", p.Source.Container
		p.Stream.MimeType = mimeTypes[p.Source.Container]
		p.Claims.Video.HDR10 = p.Source.DynamicRange == "hdr10"
		p.Claims.Video.HLG = p.Source.DynamicRange == "hlg"
	} else {
		found.stream = withoutKey(source.TranscodingUrl)
		_, query, _ := strings.Cut(found.stream, "?")
		values, _ := url.ParseQuery(query)
		videoCodecs, audioCodecs := codecsAllowed(values, "VideoCodec"), codecsAllowed(values, "AudioCodec")
		capped := heights[strings.ToLower(found.choice.quality)]
		if slices.Contains(videoCodecs, strings.ToLower(p.Source.VideoCodec)) && slices.Contains(audioCodecs, strings.ToLower(p.Source.AudioCodec)) &&
			(capped == 0 || height <= capped) {
			// The codecs are copied: the file is only repackaged.
			found.method, p.Delivery, p.DecisionReason = "DirectStream", repackaged, "container_not_supported"
		} else {
			found.method, p.Delivery, p.DecisionReason = "Transcode", converted, "conversion_required"
			if len(videoCodecs) > 0 && !slices.Contains(videoCodecs, strings.ToLower(p.Source.VideoCodec)) {
				p.EffectiveRecipe.VideoCodec = videoCodecs[0]
			}
			if len(audioCodecs) > 0 && !slices.Contains(audioCodecs, strings.ToLower(p.Source.AudioCodec)) {
				p.EffectiveRecipe.AudioCodec = audioCodecs[0]
			}
		}
		p.Stream.URL, p.Stream.Protocol = "/api/v2/playback/transcode/"+found.id+"/master.m3u8", "hls"
		p.Stream.Container, p.Stream.MimeType = cmp.Or(source.TranscodingContainer, "ts"), "application/vnd.apple.mpegurl"
	}
	s.subtitlePlan(found, source, &p, asked.subtitle)
	if p.Subtitle.Mode == "burn_in" {
		// A subtitle burned in converts the video.
		found.method, p.Delivery, p.DecisionReason = "Transcode", converted, "subtitle_burn_in"
	}
	if q, ok := rungOf(found.choice.quality); ok && p.Delivery == converted && height > q.height {
		// The conversion keeps to the rung asked, which the plan delivers.
		p.ActiveQuality = q.label
	}
	d := decision{ProtocolVersion: planProtocol, ServerFeatures: planFeatures, Outcome: "playable", SessionID: found.id, Plan: &p}
	return d, nil
}

// mimeTypes are the MIME types of the containers played as they are.
var mimeTypes = map[string]string{"mp4": "video/mp4", "m4v": "video/mp4", "mov": "video/quicktime", "mkv": "video/x-matroska",
	"webm": "video/webm", "ts": "video/mp2t", "avi": "video/x-msvideo"}

// subtitlePlan lists the version's subtitles, in the order the apps count
// them: text ones as WebVTT files beside the video, picture ones burned
// in when chosen, as the Jellyfin API delivers them. selected is the
// Jellyfin index of the one the play session shows, -1 for none.
func (s *Server) subtitlePlan(found *session, source mediaSource, p *plan, selected int) {
	found.subtitles = map[int]string{}
	found.subtitle = nil
	for n, stream := range subtitleOrder(source) {
		row := subtitleRow{TrackID: trackID(p.EffectiveMediaFileID, "subtitle", n), CombinedIndex: n, Source: "embedded",
			Codec: stream.Codec, Language: languageTag(stream.Language), Label: cmp.Or(stream.Title, stream.DisplayTitle), Forced: stream.IsForced,
			Default: stream.IsDefault, HearingImpaired: stream.IsHearingImpaired, Delivery: "burn_in_only"}
		if stream.IsExternal {
			row.Source = "external"
		}
		if stream.IsTextSubtitleStream && stream.DeliveryUrl != "" && strings.EqualFold(stream.DeliveryMethod, "External") {
			found.subtitles[n] = withoutKey(stream.DeliveryUrl)
			row.Delivery, row.URL = "sidecar", "/api/v2/stream/"+found.id+"/subtitles/"+strconv.Itoa(n)+".vtt"
		}
		p.Subtitle.Inventory = append(p.Subtitle.Inventory, row)
		if stream.Index != selected {
			continue
		}
		index := n
		p.SelectedTracks.Subtitle = &trackIdentity{ID: row.TrackID, Index: &index}
		p.Subtitle.TrackID = row.TrackID
		found.subtitle = &stream.Index
		switch {
		case row.Delivery == "sidecar":
			p.Subtitle.Mode = "render"
			p.Subtitle.Artifact = &subtitleArtifact{URL: row.URL, MimeType: "text/vtt", Format: "vtt"}
		case p.Delivery != originalHTTP:
			p.Subtitle.Mode = "burn_in"
		default:
			// A picture subtitle played as it is: the file's own player
			// shows it, or the app replans with conversion.
			p.Subtitle.Mode = "off"
			p.SelectedTracks.Subtitle, p.Subtitle.TrackID, found.subtitle = nil, "", nil
		}
	}
}

// report tells the Jellyfin API of the session's playback, as a Jellyfin
// app reports its own: the dashboard shows it, and its position becomes
// the user's resume point or played state.
func (s *Server) report(r *http.Request, found *session, path string, position float64, paused bool) error {
	body := map[string]any{"ItemId": found.version, "MediaSourceId": found.source.Id, "PlaySessionId": found.playSession,
		"PositionTicks": int64(position * 1e7), "IsPaused": paused, "PlayMethod": found.method, "CanSeek": true}
	if found.audio != nil {
		body["AudioStreamIndex"] = *found.audio
	}
	if found.subtitle != nil {
		body["SubtitleStreamIndex"] = *found.subtitle
	} else {
		body["SubtitleStreamIndex"] = -1
	}
	_, err := s.callAs(r, found.device, found.token, http.MethodPost, path, body, nil)
	return err
}

// startPlayback starts a playback of a file for the client the body
// describes, and answers its plan.
func (s *Server) startPlayback(w http.ResponseWriter, r *http.Request, v viewer) {
	var body startBody
	if !decode(w, r, &body) {
		return
	}
	switch {
	case body.ProtocolVersion != planProtocol:
		validationProblem(w, "body.protocol_version", "unsupported", "This server plans protocol 3.")
		return
	case !sameID(body.ProfileID, v.user.ID):
		problem(w, http.StatusForbidden, "permission_denied", "This profile is not the account's.")
		return
	case body.InstallationID != v.device.ID.String():
		problem(w, http.StatusConflict, "installation_changed", "This installation is not the session's.")
		return
	}
	version, err := s.numbers.id(r.Context(), body.FileID)
	if err == errUnknownNumber {
		problem(w, http.StatusNotFound, "not_found", "File not found.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	found := &session{id: newSessionID(), user: v.user.ID, device: v.device.DeviceInfo, token: v.token, installation: body.InstallationID,
		attempt: body.PlaybackAttemptID, file: body.FileID, version: version.String(), replans: map[string][]byte{}, seen: s.options.now(),
		playState: playState{choice: choice{capabilities: body.Capabilities, context: body.Context, quality: body.QualityPreference, original: true,
			bitrateKbps: bitrateOf(body.BandwidthCapKbps, body.BandwidthEstimate),
			audio:       trackPlace(body.AudioTrackID, body.AudioTrackIndex, "audio"),
			subtitle:    trackPlace(body.SubtitleTrackID, body.SubtitleTrackIndex, "subtitle")}}}
	if body.StartPosition != nil && *body.StartPosition > 0 {
		found.position = *body.StartPosition
	}
	d, err := s.plan(r, found)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if d.Plan != nil {
		if err := s.report(r, found, "/Sessions/Playing", found.position, false); err != nil {
			s.internalError(w, r, err)
			return
		}
		s.sessions.add(found)
	}
	writeJSON(w, http.StatusCreated, d)
}

// bitrateOf is the bitrate a plan keeps under, in kb/s, 0 for none: the
// client's cap, and 80% of its measured throughput.
func bitrateOf(capKbps, estimateKbps *int64) int64 {
	var limit int64
	if capKbps != nil && *capKbps > 0 {
		limit = *capKbps
	}
	if estimateKbps != nil && *estimateKbps > 0 {
		if estimate := *estimateKbps * 8 / 10; limit == 0 || estimate < limit {
			limit = estimate
		}
	}
	return limit
}

// sameID reports whether an identifier the apps sent names id, in any of
// the forms of an identifier.
func sameID(sent string, id accounts.ID) bool {
	parsed, err := accounts.ParseID(sent)
	return err == nil && parsed == id
}

// plan asks the Jellyfin API how the session plays the tracks the app
// asked, and makes its plan. The tracks the app asks by their place are
// found among the version's streams: those of the session's last plan,
// or of a first answer with the version's own audio and no subtitle,
// given up when the app asked other tracks.
func (s *Server) plan(r *http.Request, found *session) (decision, error) {
	asked := tracks{subtitle: -1}
	if c := found.choice; c.audio != nil || c.subtitle != nil {
		if len(found.source.MediaStreams) > 0 {
			asked = tracksAt(found.source, c)
		} else {
			info, refused, err := s.ask(r, found, asked)
			if err != nil || refused != nil {
				return refusedOr(refused, err)
			}
			source := info.MediaSources[0]
			wanted := tracksAt(source, c)
			if audio, ok := defaultAudio(source); wanted.subtitle == -1 && (wanted.audio == nil || ok && *wanted.audio == audio.Index) {
				return s.decide(r.Context(), found, info, wanted)
			}
			s.stopPlaySession(r, found, info.PlaySessionId)
			asked = wanted
		}
	}
	info, refused, err := s.ask(r, found, asked)
	if err != nil || refused != nil {
		return refusedOr(refused, err)
	}
	return s.decide(r.Context(), found, info, asked)
}

// refusedOr is the decision refused, or the error.
func refusedOr(refused *decision, err error) (decision, error) {
	if err != nil {
		return decision{}, err
	}
	return *refused, nil
}

// replan plans the session again: other tracks, another quality, or a
// conversion once the file failed to play as it is. A replan asked again
// answers the decision it got.
func (s *Server) replan(w http.ResponseWriter, r *http.Request, v viewer) {
	var body replanBody
	if !decode(w, r, &body) {
		return
	}
	found, done, ok := s.sessions.get(r.PathValue("session"), v.user.ID, s.options.now())
	if !ok {
		problem(w, http.StatusNotFound, "not_found", "No such playback.")
		return
	}
	defer done()
	switch {
	case body.InstallationID != found.installation:
		problem(w, http.StatusConflict, "installation_changed", "This installation is not the session's.")
		return
	case body.PlaybackAttemptID != found.attempt:
		problem(w, http.StatusConflict, "conflict", "This playback attempt is not the session's.")
		return
	}
	if previous, ok := found.replans[body.ReplanRequestID]; ok {
		writeRaw(w, http.StatusOK, previous)
		return
	}
	// The replan is planned on the session's state, which it keeps only
	// once a plan plays: a failed or refused replan changes nothing.
	saved := found.playState
	previous := found.playSession
	found.position = body.PositionSeconds
	c := &found.choice
	c.capabilities, c.context = body.Capabilities, body.Context
	c.quality, c.bitrateKbps = body.QualityPreference, bitrateOf(body.BandwidthCapKbps, body.BandwidthEstimate)
	if selected := body.SelectedTracks.Audio; selected != nil {
		c.audio = trackPlace(&selected.ID, selected.Index, "audio")
	}
	c.subtitle = nil
	if selected := body.SelectedTracks.Subtitle; selected != nil {
		c.subtitle = trackPlace(&selected.ID, selected.Index, "subtitle")
	}
	if body.Operation == "failure_recovery" || body.Operation == "seek_failure_recovery" {
		// The file failed to play as it is: it is converted from now on.
		c.original = false
	}
	d, err := s.plan(r, found)
	if err != nil || d.Plan == nil {
		found.playState = saved
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if d.Plan != nil && previous != found.playSession {
		// The previous play session's conversion is no longer needed.
		s.stopPlaySession(r, found, previous)
	}
	data, _ := json.Marshal(d)
	if body.ReplanRequestID != "" {
		found.keepReplan(body.ReplanRequestID, data)
	}
	writeRaw(w, http.StatusOK, data)
}

// maxReplans is how many replans a session keeps the answers of, for
// their replays.
const maxReplans = 32

// keepReplan keeps the answer of a replan, forgetting the oldest past
// maxReplans.
func (found *session) keepReplan(id string, data []byte) {
	if len(found.replanOrder) >= maxReplans {
		delete(found.replans, found.replanOrder[0])
		found.replanOrder = found.replanOrder[1:]
	}
	found.replans[id] = data
	found.replanOrder = append(found.replanOrder, id)
}

// writeRaw answers a JSON body already encoded, which no cache keeps.
func writeRaw(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

type mutation struct {
	Outcome  string  `json:"outcome"`
	Accepted *sample `json:"accepted,omitempty"`
	StopID   string  `json:"stop_id,omitempty"`
}

// progress applies a numbered progress sample: one older than the last
// applied is stale, the last again a replay.
func (s *Server) progress(w http.ResponseWriter, r *http.Request, v viewer) {
	var body struct {
		InstallationID string  `json:"installation_id"`
		Sequence       int64   `json:"sequence"`
		Position       float64 `json:"position"`
		IsPaused       bool    `json:"is_paused"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Sequence < 1 || body.Position < 0 {
		validationProblem(w, "body.sequence", "out_of_range", "A sample has a sequence of 1 or more and a position of 0 or more.")
		return
	}
	found, done, ok := s.sessions.get(r.PathValue("session"), v.user.ID, s.options.now())
	if !ok {
		problem(w, http.StatusNotFound, "not_found", "No such playback.")
		return
	}
	defer done()
	received := sample{Sequence: body.Sequence, Position: body.Position, IsPaused: body.IsPaused}
	switch {
	case body.InstallationID != found.installation:
		problem(w, http.StatusConflict, "installation_changed", "This installation is not the session's.")
		return
	case found.stopID != "":
		problem(w, http.StatusConflict, "conflict", "This playback stopped.")
		return
	case body.Sequence == found.sequence && received == found.sample:
		writeJSON(w, http.StatusOK, mutation{Outcome: "replayed", Accepted: &found.sample})
		return
	case body.Sequence <= found.sequence:
		writeJSON(w, http.StatusOK, mutation{Outcome: "stale_sample", Accepted: &found.sample})
		return
	}
	if err := s.report(r, found, "/Sessions/Playing/Progress", body.Position, body.IsPaused); err != nil {
		s.internalError(w, r, err)
		return
	}
	found.sequence, found.sample, found.position = body.Sequence, received, body.Position
	writeJSON(w, http.StatusOK, mutation{Outcome: "applied", Accepted: &received})
}

// stop ends a playback at its last position, or at the one the stop
// gives: the Jellyfin API records the user's resume point or played state
// as it does for Jellyfin apps. A stop of a stopped playback is a replay,
// answered with the stop that ended it.
func (s *Server) stop(w http.ResponseWriter, r *http.Request, v viewer) {
	var body struct {
		InstallationID string   `json:"installation_id"`
		StopID         string   `json:"stop_id"`
		Sequence       *int64   `json:"sequence"`
		Position       *float64 `json:"position"`
		IsPaused       *bool    `json:"is_paused"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.StopID == "" {
		validationProblem(w, "body.stop_id", "required", "A stop has an identifier.")
		return
	}
	found, done, ok := s.sessions.get(r.PathValue("session"), v.user.ID, s.options.now())
	if !ok {
		problem(w, http.StatusNotFound, "not_found", "No such playback.")
		return
	}
	defer done()
	switch {
	case body.InstallationID != found.installation:
		problem(w, http.StatusConflict, "installation_changed", "This installation is not the session's.")
		return
	case found.stopID != "":
		writeJSON(w, http.StatusOK, mutation{Outcome: "replayed", StopID: found.stopID})
		return
	}
	position := found.position
	// A position goes with a sample newer than the last applied; without a
	// sequence, it cannot be told newer, and the last sample stands.
	if body.Position != nil && body.Sequence != nil && *body.Sequence > found.sequence {
		position = *body.Position
	}
	if err := s.report(r, found, "/Sessions/Playing/Stopped", position, false); err != nil {
		s.internalError(w, r, err)
		return
	}
	found.stopID, found.position = body.StopID, position
	// The stop is kept a while, for a replay: the media are no longer
	// served.
	found.stream, found.subtitles = "", nil
	writeJSON(w, http.StatusOK, mutation{Outcome: "stopped", StopID: body.StopID})
}

// routeEvent takes a diagnostic the apps send of a playback's plan:
// Polyfin keeps none, and logs the failures they report, the rest only
// when debugging.
func (s *Server) routeEvent(w http.ResponseWriter, r *http.Request, v viewer) {
	var body struct {
		EventID               string `json:"event_id"`
		Event                 string `json:"event"`
		InstallationID        string `json:"installation_id"`
		SessionID             string `json:"session_id"`
		FailureClassification string `json:"failure_classification"`
		FallbackReason        string `json:"fallback_reason"`
	}
	if !decode(w, r, &body) {
		return
	}
	switch {
	case body.EventID == "":
		validationProblem(w, "body.event_id", "required", "An event has an identifier.")
		return
	case body.InstallationID != v.device.ID.String():
		problem(w, http.StatusConflict, "installation_changed", "This installation is not the session's.")
		return
	}
	level := slog.LevelDebug
	if body.FailureClassification != "" {
		level = slog.LevelInfo
	}
	s.options.Logger.Log(r.Context(), level, "A Silo app told of its playback", "event", truncate(body.Event),
		"session", truncate(body.SessionID), "failure", truncate(body.FailureClassification), "fallback", truncate(body.FallbackReason))
	writeJSON(w, http.StatusAccepted, struct {
		EventID string `json:"event_id"`
		Outcome string `json:"outcome"`
	}{body.EventID, "accepted"})
}

// mediaView is what a session's media are served from.
type mediaView struct {
	device    accounts.DeviceInfo
	token     string
	method    string
	stream    string
	subtitles map[int]string
}

// media finds the session whose media a request fetches.
func (s *Server) media(w http.ResponseWriter, r *http.Request, v viewer) (mediaView, bool) {
	found, done, ok := s.sessions.get(r.PathValue("session"), v.user.ID, s.options.now())
	if !ok {
		problem(w, http.StatusNotFound, "not_found", "No such playback.")
		return mediaView{}, false
	}
	view := mediaView{device: found.device, token: found.token, method: found.method, stream: found.stream, subtitles: found.subtitles}
	done()
	if view.stream == "" {
		problem(w, http.StatusNotFound, "not_found", "This playback stopped.")
		return mediaView{}, false
	}
	return view, true
}

// original serves the file a plan plays as it is.
func (s *Server) original(w http.ResponseWriter, r *http.Request, v viewer) {
	found, ok := s.media(w, r, v)
	if !ok {
		return
	}
	if found.method != "DirectPlay" {
		problem(w, http.StatusNotFound, "not_found", "This playback is converted.")
		return
	}
	s.relay(private(w), r, found.stream, authorization(found.device, found.token))
}

// converted serves the playlists and segments of a converted plan, which
// the playlists name below the master playlist.
func (s *Server) converted(w http.ResponseWriter, r *http.Request, v viewer) {
	found, ok := s.media(w, r, v)
	if !ok {
		return
	}
	if found.method == "DirectPlay" {
		problem(w, http.StatusNotFound, "not_found", "This playback is not converted.")
		return
	}
	path := r.PathValue("path")
	if strings.Contains(path, "..") {
		problem(w, http.StatusNotFound, "not_found", "Not found.")
		return
	}
	base, query, _ := strings.Cut(found.stream, "?")
	folder := base[:strings.LastIndex(base, "/")+1]
	target := folder + path + "?" + r.URL.RawQuery
	if path == "master.m3u8" {
		target = base + "?" + query
	}
	s.relay(private(w), r, target, authorization(found.device, found.token))
}

// subtitle serves a text subtitle of a plan as WebVTT.
func (s *Server) subtitle(w http.ResponseWriter, r *http.Request, v viewer) {
	found, ok := s.media(w, r, v)
	if !ok {
		return
	}
	name, ok := strings.CutSuffix(r.PathValue("file"), ".vtt")
	n, err := strconv.Atoi(name)
	target, known := found.subtitles[n]
	if !ok || err != nil || !known {
		problem(w, http.StatusNotFound, "not_found", "No such subtitle.")
		return
	}
	s.relay(private(w), r, target, authorization(found.device, found.token))
}

// privateWriter answers a session's media as the user's alone: no cache
// keeps them, whatever the Jellyfin API's answer says.
type privateWriter struct {
	http.ResponseWriter
	wrote bool
}

func private(w http.ResponseWriter) http.ResponseWriter { return &privateWriter{ResponseWriter: w} }

func (p *privateWriter) WriteHeader(status int) {
	if !p.wrote {
		p.wrote = true
		p.Header().Set("Cache-Control", "private, no-store")
	}
	p.ResponseWriter.WriteHeader(status)
}

func (p *privateWriter) Write(data []byte) (int, error) {
	if !p.wrote {
		p.WriteHeader(http.StatusOK)
	}
	return p.ResponseWriter.Write(data)
}

// Unwrap lets http.ResponseController reach the connection, to flush.
func (p *privateWriter) Unwrap() http.ResponseWriter { return p.ResponseWriter }

func (p *privateWriter) Flush() {
	if flusher, ok := p.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
