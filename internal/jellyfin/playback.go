package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
)

// maxAttempts bounds how many versions PlaybackInfo analyzes before giving
// up, when the app did not choose one.
const maxAttempts = 3

func (h *Handler) playbackRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	signedIn(http.MethodGet, "/Items/{itemId}/PlaybackInfo", h.playbackInfo)
	signedIn(http.MethodPost, "/Items/{itemId}/PlaybackInfo", h.playbackInfo)
	// Players fetch media, subtitles and the fonts they use without
	// credentials, as Jellyfin allows: see streamAccess.
	rt.handle(http.MethodGet, "/Videos/{itemId}/{file}", http.HandlerFunc(h.stream))
	rt.handle(http.MethodGet, "/Videos/{itemId}/hls1/{playlistId}/{file}", http.HandlerFunc(h.hlsSegment))
	rt.handle(http.MethodGet, "/Videos/{itemId}/{mediaSourceId}/Subtitles/{index}/{file}", http.HandlerFunc(h.subtitle))
	rt.handle(http.MethodGet, "/Videos/{itemId}/{mediaSourceId}/Subtitles/{index}/{startTicks}/{file}", http.HandlerFunc(h.subtitle))
	rt.handle(http.MethodGet, "/Videos/{itemId}/{mediaSourceId}/Attachments/{index}", http.HandlerFunc(h.attachment))
	signedIn(http.MethodDelete, "/Videos/ActiveEncodings", h.stopEncodings)

	signedIn(http.MethodPost, "/Sessions/Playing", h.reportPlayback("playbackStartInfo", playbackStarted))
	signedIn(http.MethodPost, "/Sessions/Playing/Progress", h.reportPlayback("playbackProgressInfo", playbackProgressed))
	signedIn(http.MethodPost, "/Sessions/Playing/Stopped", h.reportPlayback("playbackStopInfo", playbackStopped))
	signedIn(http.MethodPost, "/Sessions/Playing/Ping", h.pingPlayback)
	signedIn(http.MethodGet, "/Sessions", h.listSessions)
	for _, prefix := range []string{"/PlayingItems/{itemId}", "/Users/{userId}/PlayingItems/{itemId}"} {
		signedIn(http.MethodPost, prefix, h.legacyReport(playbackStarted))
		signedIn(http.MethodPost, prefix+"/Progress", h.legacyReport(playbackProgressed))
		signedIn(http.MethodDelete, prefix, h.legacyReport(playbackStopped))
	}
}

// record applies a playback report to the device's session and to the
// user's data.
func (h *Handler) record(r *http.Request, event playbackEvent, state playback.PlayState, positionKnown bool) {
	c := callerFrom(r.Context())
	before, _ := h.sessions.Playing(c.Device.ID)
	switch event {
	case playbackStarted:
		h.sessions.Start(c.Device.ID, c.User.ID, state)
	case playbackProgressed:
		h.sessions.Progress(c.Device.ID, c.User.ID, state)
	case playbackStopped:
		h.sessions.Stop(c.Device.ID)
		if state.PlaySessionID != "" {
			h.Playback.StopRemux(state.PlaySessionID)
		}
	}
	// The report has arrived: it is recorded even when the app hangs up
	// right after sending it, as when it closes once playback stops.
	h.track(context.WithoutCancel(r.Context()), c.User, event, state, positionKnown, before)
}

func (h *Handler) noContent(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// sessionWindow is how recently a device must have been active to count as
// a session, unless the request says otherwise.
const sessionWindow = 10 * time.Minute

// listSessions lists the signed-in devices active lately, with what they
// play: the caller's own, or everyone's for an administrator. A device
// holding a socket is active. With controllableByUserId, only the sessions
// that user may send commands to are listed.
func (h *Handler) listSessions(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	seconds, limited := b.int32(r, "activeWithinSeconds")
	controllerID, controlled := b.guid(r, "controllableByUserId")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	window := sessionWindow
	if limited {
		window = time.Duration(seconds) * time.Second
	}
	deviceID := query(r, "deviceId")
	caller := callerFrom(r.Context())
	var controller accounts.User
	if controlled {
		var ok bool
		if controller, ok = h.targetUser(w, r, controllerID, true, unknownListingUser); !ok {
			return
		}
	}
	users := []accounts.User{caller.User}
	if caller.User.IsAdministrator {
		all, err := h.Accounts.Users(r.Context())
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		users = all
	}
	sessions := []SessionInfo{}
	for _, user := range users {
		if controlled && !mayControl(controller, user.ID) {
			continue
		}
		devices, err := h.Accounts.Devices(r.Context(), user.ID)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		for _, device := range devices {
			playing, isPlaying := h.sessions.Playing(device.ID)
			connected := h.sockets.latest(device.ID) != nil
			if deviceID != "" && device.DeviceID != deviceID || !isPlaying && !connected && time.Since(device.LastActivityAt) > window {
				continue
			}
			controllable := h.controllable(device)
			if controlled && !controllable {
				continue
			}
			info := newSessionInfo(device, user, h.ServerID, controllable)
			if isPlaying {
				h.describePlaying(r, user, &info, playing)
			}
			sessions = append(sessions, info)
		}
	}
	writeJSON(w, http.StatusOK, sessions)
}

// describePlaying adds what a device plays to its session.
func (h *Handler) describePlaying(r *http.Request, user accounts.User, info *SessionInfo, playing playback.NowPlaying) {
	info.LastPlaybackCheckIn = Time(playing.CheckedIn)
	if !playing.PausedAt.IsZero() {
		info.LastPausedDate = new(Time(playing.PausedAt))
	}
	info.PlayState = PlayerStateInfo{
		PositionTicks:       new(int64(playing.Position / 100)),
		CanSeek:             playing.CanSeek,
		IsPaused:            playing.Paused,
		IsMuted:             playing.Muted,
		VolumeLevel:         playing.VolumeLevel,
		AudioStreamIndex:    playing.AudioStreamIndex,
		SubtitleStreamIndex: playing.SubtitleStreamIndex,
		MediaSourceId:       playing.MediaSourceID,
		PlayMethod:          playing.PlayMethod,
		RepeatMode:          playing.RepeatMode,
		PlaybackOrder:       playing.PlaybackOrder,
	}
	item, err := h.title(r.Context(), user, playing.Item)
	if err != nil {
		return
	}
	// Jellyfin describes the item as on its page, without what concerns
	// the user or managing the item.
	dto := h.newItemDto(item, nil, true, userState{})
	h.addMediaSources(r, user, &dto, item, playing.Item, false)
	// The chapters are those of the version playing, which the app may
	// have picked among the item's.
	if source, err := accounts.ParseID(playing.MediaSourceID); err == nil && source != playing.Item {
		h.setChapters(r.Context(), &dto, h.cachedPlayable(r.Context(), user, item).ordered(source))
	}
	dto.MediaSources, dto.HasSubtitles, dto.People, dto.RemoteTrailers = nil, nil, nil, nil
	dto.CanDelete, dto.CanDownload, dto.LockData, dto.LockedFields, dto.Tags = nil, nil, nil, nil, nil
	dto.Etag, dto.SortName, dto.PlayAccess, dto.DisplayPreferencesId = "", "", "", ""
	dto.UserData, dto.Trickplay = UserItemData{}, &struct{}{}
	info.NowPlayingItem = &dto
}

// title resolves the item a player opens: a movie or an episode, by its own
// identifier or by one of its versions'.
func (h *Handler) title(ctx context.Context, user accounts.User, opened accounts.ID) (library.Item, error) {
	item, err := h.Library.Item(ctx, user, opened)
	if errors.Is(err, library.ErrNotFound) {
		if owner, ok := h.Library.VersionOwner(opened); ok {
			item, err = h.Library.Item(ctx, user, owner)
		}
	}
	if err != nil {
		return library.Item{}, err
	}
	if item.Kind != library.KindMovie && item.Kind != library.KindEpisode {
		return library.Item{}, library.ErrNotFound
	}
	return item, nil
}

// looseInt reads a number that apps send as a JSON number or string.
type looseInt struct {
	value int64
	set   bool
}

func (n *looseInt) UnmarshalJSON(data []byte) error {
	text := strings.Trim(string(data), `"`)
	if text == "null" || strings.TrimSpace(text) == "" {
		return nil
	}
	value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil {
		return err
	}
	*n = looseInt{value: value, set: true}
	return nil
}

// playbackInfoRequest is the PlaybackInfoDto apps post.
type playbackInfoRequest struct {
	MaxStreamingBitrate  looseInt
	AudioStreamIndex     looseInt
	SubtitleStreamIndex  looseInt
	MediaSourceId        string
	DeviceProfile        *playback.DeviceProfile
	EnableDirectPlay     *bool
	EnableDirectStream   *bool
	AllowAudioStreamCopy *bool
	AllowVideoStreamCopy *bool
}

// playbackInfo answers what an app needs to play an item: its version's
// description, whether the app can play it as it is, and a play session.
func (h *Handler) playbackInfo(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	opened := b.pathID(r, "itemId")
	var request playbackInfoRequest
	if bitrate, ok := b.int32(r, "maxStreamingBitrate"); ok {
		request.MaxStreamingBitrate = looseInt{int64(bitrate), true}
	}
	if index, ok := b.int32(r, "audioStreamIndex"); ok {
		request.AudioStreamIndex = looseInt{int64(index), true}
	}
	if index, ok := b.int32(r, "subtitleStreamIndex"); ok {
		request.SubtitleStreamIndex = looseInt{int64(index), true}
	}
	if value, ok := b.bool(r, "enableDirectPlay"); ok {
		request.EnableDirectPlay = new(value)
	}
	if value, ok := b.bool(r, "enableDirectStream"); ok {
		request.EnableDirectStream = new(value)
	}
	if value, ok := b.bool(r, "allowAudioStreamCopy"); ok {
		request.AllowAudioStreamCopy = new(value)
	}
	if value, ok := b.bool(r, "allowVideoStreamCopy"); ok {
		request.AllowVideoStreamCopy = new(value)
	}
	request.MediaSourceId = query(r, "mediaSourceId")
	if r.Method == http.MethodPost && !h.readPlaybackInfoBody(w, r, b, &request) {
		return
	}
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	item, err := h.title(r.Context(), user, opened)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	p, err := h.playable(r.Context(), user, item)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	versions := p.ordered(opened)

	candidates := make([]int, 0, len(versions))
	if request.MediaSourceId != "" {
		requested, _ := parseGUID(request.MediaSourceId)
		for i, version := range versions {
			if sourceID(opened, version, i == 0) == requested {
				candidates = append(candidates, i)
			}
		}
		if len(candidates) == 0 {
			writeJSON(w, http.StatusOK, noCompatibleStream{MediaSources: []MediaSourceInfo{}, ErrorCode: "NoCompatibleStream"})
			return
		}
	} else {
		for i := range min(len(versions), maxAttempts) {
			candidates = append(candidates, i)
		}
	}
	unreadable := map[int]bool{}
	for _, i := range candidates {
		version := versions[i]
		analysis, err := h.Playback.Analyze(r.Context(), version)
		if err != nil {
			h.Logger.Info("A version could not be analyzed", "addon", version.Addon, "error", err)
			unreadable[i] = true
			continue
		}
		session := h.Playback.Signer().Sign(playback.Grant{Version: version.ID, User: user.ID, Relay: mustRelay(r, version)})
		sources := []MediaSourceInfo{h.decidedSource(r, p, version, sourceID(opened, version, i == 0), analysis, request, session)}
		// An app that asks for no version gets every version, as from
		// Jellyfin: some, such as Strand, list them for the user to pick.
		// The one decided comes first, which apps play unless the user picks
		// another; the others are described as item details describe them,
		// and decided when an app asks for one. Those found unreadable just
		// now are left out.
		if request.MediaSourceId == "" {
			for j, other := range versions {
				if j != i && !unreadable[j] {
					sources = append(sources, h.describedSource(r, p, other, sourceID(opened, other, j == 0)))
				}
			}
		}
		writeJSON(w, http.StatusOK, playbackInfoResponse{MediaSources: sources, PlaySessionId: session})
		return
	}
	writeJSON(w, http.StatusOK, noCompatibleStream{MediaSources: []MediaSourceInfo{}, ErrorCode: "NoCompatibleStream"})
}

type playbackInfoResponse struct {
	MediaSources  []MediaSourceInfo
	PlaySessionId string
}

type noCompatibleStream struct {
	MediaSources []MediaSourceInfo
	ErrorCode    string
}

// readPlaybackInfoBody reads a posted PlaybackInfoDto over the query's
// values. An empty body is allowed; a body without a JSON content type is
// not.
func (h *Handler) readPlaybackInfoBody(w http.ResponseWriter, r *http.Request, b bindErrors, request *playbackInfoRequest) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		processingError(w, http.StatusRequestEntityTooLarge)
		return false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		if r.Header.Get("Content-Type") == "" {
			unsupportedMediaTypeProblem(w)
			return false
		}
		return true
	}
	if !jsonContent(r.Header.Get("Content-Type")) {
		unsupportedMediaTypeProblem(w)
		return false
	}
	var posted playbackInfoRequest
	if err := json.Unmarshal(body, &posted); err != nil {
		b.add("$", "The JSON value could not be converted.")
		validationProblem(w, b)
		return false
	}
	if posted.MaxStreamingBitrate.set {
		request.MaxStreamingBitrate = posted.MaxStreamingBitrate
	}
	if posted.AudioStreamIndex.set {
		request.AudioStreamIndex = posted.AudioStreamIndex
	}
	if posted.SubtitleStreamIndex.set {
		request.SubtitleStreamIndex = posted.SubtitleStreamIndex
	}
	if posted.MediaSourceId != "" {
		request.MediaSourceId = posted.MediaSourceId
	}
	if posted.DeviceProfile != nil {
		request.DeviceProfile = posted.DeviceProfile
	}
	if posted.EnableDirectPlay != nil {
		request.EnableDirectPlay = posted.EnableDirectPlay
	}
	if posted.EnableDirectStream != nil {
		request.EnableDirectStream = posted.EnableDirectStream
	}
	if posted.AllowAudioStreamCopy != nil {
		request.AllowAudioStreamCopy = posted.AllowAudioStreamCopy
	}
	if posted.AllowVideoStreamCopy != nil {
		request.AllowVideoStreamCopy = posted.AllowVideoStreamCopy
	}
	return true
}

// decidedSource describes the version an app is about to play, with the
// decision for its device profile; session is the play session a remux
// belongs to.
func (h *Handler) decidedSource(r *http.Request, p playable, version library.Version, id accounts.ID, analysis media.Analysis, request playbackInfoRequest, session string) MediaSourceInfo {
	source := h.baseSource(r, p, version, id, analysis, true)
	streams := source.MediaStreams
	options := playback.Options{
		MaxStreamingBitrate: request.MaxStreamingBitrate.value,
		EnableDirectPlay:    request.EnableDirectPlay == nil || *request.EnableDirectPlay,
		EnableDirectStream:  request.EnableDirectStream == nil || *request.EnableDirectStream,
		ConvertAudio:        request.AllowAudioStreamCopy != nil && !*request.AllowAudioStreamCopy,
		ConvertVideo:        request.AllowVideoStreamCopy != nil && !*request.AllowVideoStreamCopy,
		Can:                 h.Playback.Capabilities(),
	}
	// Like Jellyfin, chosen tracks only count with the version they belong
	// to. The user's preferences choose the others. A decision plays the
	// audio track flagged default, else the first, by itself: another
	// preferred one is passed as if asked, so that the decision checks it.
	audio := p.tracks.audio(streams)
	if request.MediaSourceId != "" && request.AudioStreamIndex.set {
		audio = new(int(request.AudioStreamIndex.value))
		options.AudioStreamIndex = audio
	} else if audio != nil && *audio != *defaultAudio(streams) {
		options.AudioStreamIndex = audio
	}
	if request.MediaSourceId != "" && request.SubtitleStreamIndex.set {
		options.SubtitleStreamIndex = new(int(request.SubtitleStreamIndex.value))
	} else {
		options.SubtitleStreamIndex = p.tracks.subtitle(streams, audio)
	}
	container := playback.Container(analysis)
	decision := playback.Decision{DirectPlay: true, Container: container, AudioStreamIndex: -1, SubtitleStreamIndex: -1}
	if request.DeviceProfile != nil {
		described := playback.MediaSource{Container: container, Bitrate: analysis.Bitrate, Streams: streams}
		decision = playback.Decide(request.DeviceProfile, described, options)
		// A subtitle the app can only take burned into the video is burned
		// in when it is an image track inside the file and the video can be
		// converted, as burning it in requires. Any other is left out
		// rather than preventing playback, and so is one the app would take
		// in HLS when the version cannot be streamed so.
		if selected := options.SubtitleStreamIndex; !decision.DirectPlay && selected != nil && *selected >= 0 {
			method := decision.Subtitles[*selected].Method
			if method == "Encode" && burnable(streams, analysis, *selected) {
				burn := options
				burn.ConvertVideo = true
				if burned := playback.Decide(request.DeviceProfile, described, burn); burned.HLS && burned.Video != nil {
					decision, method = burned, ""
				}
			}
			if method == "Encode" || (method == "Hls" && !decision.HLS) {
				without := options
				without.SubtitleStreamIndex = new(-1)
				if retry := playback.Decide(request.DeviceProfile, described, without); retry.DirectPlay || (retry.HLS && !decision.HLS) {
					decision = retry
				}
			}
		}
	} else {
		if audio != nil {
			decision.AudioStreamIndex = *audio
		}
		if options.SubtitleStreamIndex != nil {
			decision.SubtitleStreamIndex = *options.SubtitleStreamIndex
		}
	}
	// Streaming over HLS needs the keyframe index: a version without one
	// is not offered for it.
	streamed := decision.HLS
	if streamed {
		if _, err := h.Playback.Plan(r.Context(), version); err != nil {
			h.Logger.Info("A version cannot be streamed over HLS", "addon", version.Addon, "error", err)
			streamed = false
		}
	}
	h.deliverable(r.Context(), decision, streams, version, analysis, streamed)
	h.prefetchSubtitle(r.Context(), decision, streams, version, analysis)

	source.Container = decision.Container
	source.SupportsDirectPlay, source.SupportsDirectStream = decision.DirectPlay, decision.DirectPlay
	if streamed {
		limit := request.MaxStreamingBitrate.value
		if limit <= 0 && request.DeviceProfile.MaxStreamingBitrate != nil {
			limit = *request.DeviceProfile.MaxStreamingBitrate
		}
		source.SupportsTranscoding = true
		source.TranscodingUrl = transcodingURL(r, p.item.ID, id, version, analysis, streams, decision, limit, session)
		source.TranscodingSubProtocol = "hls"
		source.TranscodingContainer = decision.Transcoding.Container
	}
	if decision.AudioStreamIndex >= 0 {
		source.DefaultAudioStreamIndex = new(decision.AudioStreamIndex)
	}
	if slices.ContainsFunc(streams, func(stream playback.MediaStream) bool { return stream.Type == "Subtitle" }) {
		source.DefaultSubtitleStreamIndex = new(decision.SubtitleStreamIndex)
	}
	for i := range source.MediaStreams {
		stream := &source.MediaStreams[i]
		delivery, ok := decision.Subtitles[stream.Index]
		if stream.Type != "Subtitle" || !ok {
			continue
		}
		stream.DeliveryMethod = delivery.Method
		if delivery.Method == "External" {
			stream.DeliveryUrl = subtitleURL(r, p.item.ID, id, stream.Index, delivery.Format)
			stream.IsExternalUrl = new(false)
		}
	}
	for i := range source.MediaAttachments {
		attachment := &source.MediaAttachments[i]
		attachment.DeliveryUrl = attachmentURL(r, p.item.ID, id, attachment.Index)
	}
	return source
}

// deliverable adapts a Jellyfin decision's subtitle deliveries to what
// Polyfin delivers: subtitles in the container reach the app embedded in
// what it plays as it is; subtitle files, and the embedded text tracks that
// remuxes extracted whole or that the version's index lists block by block,
// as external files; text subtitles as HLS renditions; and the image track
// chosen burned into converted video. Any other is left out.
func (h *Handler) deliverable(ctx context.Context, decision playback.Decision, streams []playback.MediaStream, version library.Version, analysis media.Analysis, streamed bool) {
	files := subtitleFiles(streams)
	whole := h.Playback.SubtitlesExtracted(ctx, version.ID, analysis.Duration)
	// The index is read only when an embedded track would go out as a file.
	var located map[int]bool
	locate := func(stream int) bool {
		if located == nil {
			located = h.Playback.SubtitlesLocated(ctx, version, analysis)
			if located == nil {
				located = map[int]bool{}
			}
		}
		return located[stream]
	}
	for _, stream := range streams {
		delivery, ok := decision.Subtitles[stream.Index]
		if stream.Type != "Subtitle" || !ok {
			continue
		}
		extractable := !stream.IsExternal && playback.ExtractableSubtitle(analysis, stream.Index-files)
		switch {
		case delivery.Method == "Embed" && !stream.IsExternal && decision.DirectPlay:
		case delivery.Method == "External" && (stream.IsExternal || (extractable && (whole || locate(stream.Index-files)))):
		case delivery.Method == "Hls" && streamed && (stream.IsExternal || extractable):
		case delivery.Method == "Encode" && streamed && decision.Video != nil && stream.Index == decision.SubtitleStreamIndex &&
			burnable(streams, analysis, stream.Index):
		default:
			decision.Subtitles[stream.Index] = playback.SubtitleDelivery{Method: "Drop"}
		}
	}
}

// subtitleFiles counts a version's subtitle files, which come first among
// its streams.
func subtitleFiles(streams []playback.MediaStream) int {
	files := 0
	for _, stream := range streams {
		if stream.Type == "Subtitle" && stream.IsExternal {
			files++
		}
	}
	return files
}

// prefetchSubtitle starts reading whole the embedded track the app shows
// by default when it goes out as a file, so that it is ready when the app
// asks for it a moment later.
func (h *Handler) prefetchSubtitle(ctx context.Context, decision playback.Decision, streams []playback.MediaStream, version library.Version, analysis media.Analysis) {
	files := subtitleFiles(streams)
	delivery, ok := decision.Subtitles[decision.SubtitleStreamIndex]
	if !ok || delivery.Method != "External" || decision.SubtitleStreamIndex < files ||
		h.Playback.SubtitlesExtracted(ctx, version.ID, analysis.Duration) {
		return
	}
	if stream := decision.SubtitleStreamIndex - files; h.Playback.SubtitlesLocated(ctx, version, analysis)[stream] {
		h.Playback.PrefetchSubtitle(version, analysis, stream)
	}
}

// burnable reports whether the subtitle stream of Jellyfin index index can
// be burned into converted video: an image track inside the file.
func burnable(streams []playback.MediaStream, analysis media.Analysis, index int) bool {
	files := subtitleFiles(streams)
	return index >= files && playback.BurnableSubtitle(analysis, index-files)
}

// streamAccess finds who a media request plays for. Players send no
// credentials with media URLs: a signed grant in Path URLs, a signed play
// session, or the caller's token stands for the user.
func (h *Handler) streamAccess(r *http.Request) (accounts.User, playback.Grant, bool) {
	for _, name := range []string{grantParameter, "playSessionId"} {
		token := query(r, name)
		if token == "" {
			continue
		}
		grant, err := h.Playback.Signer().Verify(token)
		if err != nil {
			continue
		}
		user, err := h.Accounts.User(r.Context(), grant.User)
		if err == nil && !user.IsDisabled {
			return user, grant, true
		}
	}
	credentials := readCredentials(r, h.Accounts.Settings().LegacyAuthorization)
	if credentials.Token != "" {
		if _, user, err := h.Accounts.DeviceByToken(r.Context(), credentials.Token, remoteAddress(r)); err == nil {
			return user, playback.Grant{}, true
		}
	}
	return accounts.User{}, playback.Grant{}, false
}

// stream serves a version's bytes: /Videos/{itemId}/stream, with an
// optional container extension.
func (h *Handler) stream(w http.ResponseWriter, r *http.Request) {
	name, extension, _ := strings.Cut(r.PathValue("file"), ".")
	if strings.EqualFold(extension, "m3u8") && (strings.EqualFold(name, "master") || strings.EqualFold(name, "main")) {
		h.hlsPlaylist(w, r, name)
		return
	}
	if !strings.EqualFold(name, "stream") {
		processingError(w, http.StatusNotFound)
		return
	}
	opened, ok := parseGUID(r.PathValue("itemId"))
	if !ok {
		validationProblem(w, map[string][]string{"itemId": {notValid(r.PathValue("itemId"))}})
		return
	}
	user, grant, ok := h.streamAccess(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	item, err := h.title(r.Context(), user, opened)
	if err != nil {
		processingError(w, http.StatusNotFound)
		return
	}
	// The version: the one the app names by its ETag or media source id,
	// else the grant's, else the item's first. The app's choice wins over
	// the play session's: PlaybackInfo lists every version under one
	// session, and apps play the one the user picks with it, as Jellyfin
	// lets them. The grant only stands for the user, who may play any
	// version of the title.
	wanted := grant.Version
	if wanted == (accounts.ID{}) {
		wanted = opened
	}
	for _, name := range []string{"mediaSourceId", "tag"} {
		if id, ok := parseGUID(query(r, name)); ok {
			wanted = id
		}
	}
	version, err := h.Library.Version(r.Context(), user, item.ID, wanted)
	if err != nil {
		processingError(w, http.StatusBadRequest)
		return
	}
	// A player that authenticates with a header might send it to the
	// source too: relay instead of redirecting it.
	relay := grant.Relay || strings.Contains(r.Header.Get("Authorization"), "Token=")
	contentType := mimeTypes[strings.ToLower(extension)]
	if contentType == "" {
		contentType = mimeTypes[containerOfName(version.Filename)]
	}
	if err := h.Playback.Serve(w, r, version, relay, contentType); err != nil && r.Context().Err() == nil {
		h.Logger.Warn("A stream could not be served", "addon", version.Addon, "error", err)
	}
}

// mimeTypes are the content types of media containers.
var mimeTypes = map[string]string{
	"mkv":  "video/x-matroska",
	"mp4":  "video/mp4",
	"m4v":  "video/mp4",
	"webm": "video/webm",
	"mov":  "video/quicktime",
	"avi":  "video/x-msvideo",
	"ts":   "video/mp2t",
	"m3u8": "application/vnd.apple.mpegurl",
	"hls":  "application/vnd.apple.mpegurl",
}

// subtitle serves a version's subtitle, an addon's file or a track inside
// the version, in the format the player asks, optionally from a start
// position.
func (h *Handler) subtitle(w http.ResponseWriter, r *http.Request) {
	name, format, _ := strings.Cut(r.PathValue("file"), ".")
	opened, okItem := parseGUID(r.PathValue("itemId"))
	index, errIndex := strconv.Atoi(r.PathValue("index"))
	if !strings.EqualFold(name, "stream") || !okItem || errIndex != nil {
		processingError(w, http.StatusNotFound)
		return
	}
	var start time.Duration
	if raw := r.PathValue("startTicks"); raw != "" {
		ticks, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			processingError(w, http.StatusNotFound)
			return
		}
		start = time.Duration(ticks) * 100
	}
	item := opened
	if owner, ok := h.Library.VersionOwner(opened); ok {
		item = owner
	}
	files, ok := h.subtitleFiles.Get(item)
	if !ok {
		// Not described since the server started: describe the item again
		// for a signed-in caller.
		user, _, signedIn := h.streamAccess(r)
		if !signedIn {
			processingError(w, http.StatusNotFound)
			return
		}
		title, err := h.title(r.Context(), user, item)
		if err != nil {
			processingError(w, http.StatusNotFound)
			return
		}
		p, _ := h.playable(r.Context(), user, title)
		files = p.subtitles
	}
	// External subtitles come first among a version's streams; embedded
	// tracks follow, served whole (see embeddedTrack).
	var text subtitleText
	switch {
	case index < 0:
		processingError(w, http.StatusInternalServerError)
		return
	case index >= len(files):
		var found bool
		// Jellyfin answers 500 for a subtitle it cannot serve.
		if text, found = h.embeddedTrack(r, item, index-len(files)); !found {
			processingError(w, http.StatusInternalServerError)
			return
		}
	default:
		var err error
		if text, err = h.subtitleFile(r.Context(), files[index]); err != nil {
			if r.Context().Err() == nil {
				h.Logger.Warn("A subtitle could not be read", "addon", files[index].Addon, "error", err)
			}
			processingError(w, http.StatusInternalServerError)
			return
		}
	}
	data, contentType, ok := text.write(format, start)
	if !ok {
		processingError(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
}

// trackWait bounds how long a subtitle request waits for its track to be
// read whole through the version's index: seconds usually, longer when the
// host slows Polyfin down.
const trackWait = 2 * time.Minute

// embeddedTrack returns an embedded text track of the version a subtitle
// URL names, stream being its FFmpeg index. A track the version's index
// lists is read whole through it, now if it was not before: its file keeps
// what the WebVTT remuxes extract loses, ASS styles first. Else it is the
// track remuxes extracted whole, if they did. The URL carries the caller's
// token, as Jellyfin writes DeliveryUrls.
func (h *Handler) embeddedTrack(r *http.Request, item accounts.ID, stream int) (subtitleText, bool) {
	user, _, signedIn := h.streamAccess(r)
	wanted, ok := parseGUID(r.PathValue("mediaSourceId"))
	if !signedIn || !ok {
		return subtitleText{}, false
	}
	version, err := h.Library.Version(r.Context(), user, item, wanted)
	if err != nil {
		return subtitleText{}, false
	}
	analysis, ok := h.Playback.Analyzed(r.Context(), version.ID)
	if !ok || !playback.ExtractableSubtitle(analysis, stream) {
		return subtitleText{}, false
	}
	if h.Playback.SubtitlesLocated(r.Context(), version, analysis)[stream] {
		ctx, cancel := context.WithTimeout(r.Context(), trackWait)
		defer cancel()
		if track, err := h.Playback.SubtitleTrack(ctx, version, analysis, stream); err == nil {
			if text, err := readSubtitleText(track.Data); err == nil {
				return text, true
			}
		}
	}
	cues, ok := h.Playback.ExtractedTrack(r.Context(), version.ID, analysis.Duration, stream)
	return subtitleText{cues: cues}, ok
}

// maxSubtitleBytes bounds subtitle downloads.
const maxSubtitleBytes = 8 << 20

// subtitleFile downloads and reads an addon's subtitle file, keeping it for
// a while.
func (h *Handler) subtitleFile(ctx context.Context, file library.ExternalSubtitle) (subtitleText, error) {
	if text, ok := h.subtitleCache.Get(file.ID); ok {
		return text, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	response, err := h.Stremio.Open(ctx, http.MethodGet, file.URL, nil, file.Confined)
	if err != nil {
		return subtitleText{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return subtitleText{}, errors.New("subtitle unavailable: HTTP " + strconv.Itoa(response.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxSubtitleBytes))
	if err != nil {
		return subtitleText{}, err
	}
	text, err := readSubtitleText(data)
	if err != nil {
		return subtitleText{}, err
	}
	h.subtitleCache.Put(file.ID, text)
	return text, nil
}

// playbackReport is what apps report about playback: Jellyfin's
// PlaybackStartInfo, PlaybackProgressInfo and PlaybackStopInfo.
type playbackReport struct {
	ItemId              string
	MediaSourceId       string
	PlaySessionId       string
	PlayMethod          string
	PositionTicks       looseInt
	IsPaused            bool
	IsMuted             bool
	CanSeek             bool
	AudioStreamIndex    looseInt
	SubtitleStreamIndex looseInt
	VolumeLevel         looseInt
	RepeatMode          string
	PlaybackOrder       string
}

func (report playbackReport) state() playback.PlayState {
	state := playback.PlayState{
		MediaSourceID: report.MediaSourceId,
		PlaySessionID: report.PlaySessionId,
		PlayMethod:    report.PlayMethod,
		Position:      time.Duration(report.PositionTicks.value) * 100,
		Paused:        report.IsPaused,
		Muted:         report.IsMuted,
		CanSeek:       report.CanSeek,
		RepeatMode:    report.RepeatMode,
		PlaybackOrder: report.PlaybackOrder,
	}
	state.Item, _ = parseGUID(report.ItemId)
	if report.AudioStreamIndex.set {
		state.AudioStreamIndex = new(int(report.AudioStreamIndex.value))
	}
	if report.SubtitleStreamIndex.set {
		state.SubtitleStreamIndex = new(int(report.SubtitleStreamIndex.value))
	}
	if report.VolumeLevel.set {
		state.VolumeLevel = new(int(report.VolumeLevel.value))
	}
	return state
}

// reportPlayback records a playback report posted as JSON. Like Jellyfin, it
// accepts reports for unknown items and answers nothing.
func (h *Handler) reportPlayback(parameter string, event playbackEvent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !jsonContent(r.Header.Get("Content-Type")) {
			unsupportedMediaTypeProblem(w)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
		if err != nil {
			processingError(w, http.StatusRequestEntityTooLarge)
			return
		}
		if len(bytes.TrimSpace(body)) == 0 {
			validationProblem(w, map[string][]string{
				"":        {"A non-empty request body is required."},
				parameter: {"The " + parameter + " field is required."},
			})
			return
		}
		var report playbackReport
		if err := json.Unmarshal(body, &report); err != nil {
			validationProblem(w, map[string][]string{"$": {"The JSON value could not be converted."}, parameter: {"The " + parameter + " field is required."}})
			return
		}
		h.record(r, event, report.state(), report.PositionTicks.set)
		w.WriteHeader(http.StatusNoContent)
	}
}

// pingPlayback keeps a play session alive; Polyfin's sessions need no
// keeping.
func (h *Handler) pingPlayback(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(query(r, "playSessionId")) == "" {
		validationProblem(w, map[string][]string{"playSessionId": {"The playSessionId field is required."}})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// legacyReport records a report sent the older way, as query parameters on
// /PlayingItems/{itemId}.
func (h *Handler) legacyReport(event playbackEvent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b := bindErrors{}
		item := b.pathID(r, "itemId")
		// Positions in ticks overflow 32 bits after four minutes.
		var position int64
		positionKnown := false
		if raw := strings.TrimSpace(query(r, "positionTicks")); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				b.add("positionTicks", notValid(raw))
			}
			position, positionKnown = value, true
		}
		paused, _ := b.bool(r, "isPaused")
		if len(b) > 0 {
			validationProblem(w, b)
			return
		}
		h.record(r, event, playback.PlayState{
			Item:          item,
			MediaSourceID: query(r, "mediaSourceId"),
			PlaySessionID: query(r, "playSessionId"),
			Position:      time.Duration(position) * 100,
			Paused:        paused,
		}, positionKnown)
		w.WriteHeader(http.StatusNoContent)
	}
}
