package jellyfin

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/dlna"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/plays"
	"github.com/moodiness/polyfin/internal/trackers"
	"github.com/moodiness/polyfin/internal/userdata"
)

// The Jellyfin API's handler plays for DLNA devices as it does for its
// apps (see package dlna): the version PlaybackInfo would pick for the
// device's user and profile, sent as it is or converted to MPEG-TS, under
// the same limits, with the playbacks shown in Sessions and counted in
// the statistics as the DLNA app's.

const (
	// dlnaApp names the app of DLNA playbacks in sessions and statistics.
	dlnaApp = "DLNA"
	// dlnaStale is how long a DLNA playback lasts once its device asks for
	// nothing more: devices read ahead, pause, and read again.
	dlnaStale = 2 * time.Minute
	// dlnaReport is how often a playback under way is reported.
	dlnaReport = 10 * time.Second
)

// dlnaPlay is a DLNA device's playback of a title: the requests for its
// bytes, from the first on, until the device stops asking.
type dlnaPlay struct {
	// id is its play session, which names its conversion.
	id      string
	device  dlna.Device
	user    accounts.User
	item    library.Item
	version accounts.ID
	method  string
	started time.Time
	seen    time.Time
	// position is how far the device read, by the bytes of the file sent
	// for a file, by time for a conversion; runtime the title's length.
	position time.Duration
	runtime  time.Duration
	// sent are the bytes of the version's file sent during the playback,
	// wherever they are in it: devices read its end first to find its
	// index, and read some parts again.
	sent byteRanges
	// requests is how many of its requests are being served.
	requests int
	// marked tells the title was marked played.
	marked bool
}

// dlnaPlays are the DLNA playbacks under way, by device and title.
type dlnaPlays struct {
	mu    sync.Mutex
	plays map[string]*dlnaPlay
}

func newDLNAPlays() *dlnaPlays { return &dlnaPlays{plays: map[string]*dlnaPlay{}} }

func dlnaKey(device, item accounts.ID) string { return device.String() + item.String() }

// ofUser counts the DLNA playbacks of user.
func (d *dlnaPlays) ofUser(user accounts.ID) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, play := range d.plays {
		if play.user.ID == user {
			n++
		}
	}
	return n
}

// all returns copies of the playbacks under way.
func (d *dlnaPlays) all() []dlnaPlay {
	d.mu.Lock()
	defer d.mu.Unlock()
	all := make([]dlnaPlay, 0, len(d.plays))
	for _, play := range d.plays {
		all = append(all, *play)
	}
	return all
}

// endStaleDLNA ends the DLNA playbacks whose devices asked for nothing in
// dlnaStale, and those of other titles on the device of keep, which moved
// on.
func (h *Handler) endStaleDLNA(device accounts.ID, keep string) {
	now := h.now()
	h.dlna.mu.Lock()
	var ended []dlnaPlay
	for key, play := range h.dlna.plays {
		stale := play.requests == 0 && now.Sub(play.seen) >= dlnaStale
		if stale || play.device.ID == device && key != keep && play.requests == 0 {
			ended = append(ended, *play)
			delete(h.dlna.plays, key)
		}
	}
	h.dlna.mu.Unlock()
	for _, play := range ended {
		h.Playback.StopRemux(play.id)
		h.followDLNA(play, plays.ReportStop)
	}
}

// followDLNA tells the playbacks Polyfin follows of a DLNA playback.
func (h *Handler) followDLNA(play dlnaPlay, kind int) {
	if h.Plays == nil {
		return
	}
	h.Plays.Report(plays.Report{Kind: kind, Device: play.device.ID, User: play.user.ID, UserName: play.user.Name, App: dlnaApp,
		DeviceName: play.device.DisplayName(), Title: playedTitle(play.item), Position: play.position, PositionKnown: true,
		Method: play.method, Sent: h.sentSoFar(play.device.ID.String())})
}

// errDLNALimit reports a playback refused for the user's limit of
// playbacks at once.
var errDLNALimit = errors.New("playbacks at once")

// beginDLNA starts or continues the playback of a request, unless its user
// already plays on as many devices as they may; done ends the request.
func (h *Handler) beginDLNA(ctx context.Context, play dlna.Play, version accounts.ID, method string, runtime time.Duration) (*dlnaPlay, func(), error) {
	key := dlnaKey(play.Device.ID, play.Item.ID)
	h.endStaleDLNA(play.Device.ID, key)
	h.dlna.mu.Lock()
	current, ok := h.dlna.plays[key]
	h.dlna.mu.Unlock()
	if !ok {
		if most := play.User.MaxPlaybacks; most > 0 {
			playing, err := h.devicesPlaying(ctx, play.User, accounts.ID{})
			if err != nil {
				return nil, nil, err
			}
			h.endStaleGuests()
			if playing+h.guests.ofUser(play.User.ID) >= most {
				return nil, nil, errDLNALimit
			}
		}
		now := h.now()
		current = &dlnaPlay{id: "dlna" + randomHex(16), device: play.Device, user: play.User, item: play.Item, version: version, method: method,
			started: now, seen: now, runtime: runtime}
		h.dlna.mu.Lock()
		if existing, raced := h.dlna.plays[key]; raced {
			current = existing
		} else {
			h.dlna.plays[key] = current
			ok = false
		}
		h.dlna.mu.Unlock()
		if !ok {
			h.followDLNA(*current, plays.ReportStart)
		}
	}
	h.dlna.mu.Lock()
	current.requests++
	current.seen = h.now()
	if current.version != version {
		current.sent = nil
	}
	current.version, current.method = version, method
	h.dlna.mu.Unlock()
	return current, func() {
		h.dlna.mu.Lock()
		current.requests--
		current.seen = h.now()
		h.dlna.mu.Unlock()
	}, nil
}

// reachedDLNA notes how far the device read a playback, reports it, and
// marks the title played once past Marked played after (%), while Mark
// played what DLNA devices play is on.
func (h *Handler) reachedDLNA(ctx context.Context, current *dlnaPlay, position time.Duration) {
	settings := h.Accounts.Settings()
	h.dlna.mu.Lock()
	current.position = max(current.position, min(position, max(current.runtime, position)))
	mark := settings.DLNAMarkPlayed && !current.marked && current.runtime > 0 &&
		userdata.Thresholds{Resume: settings.ResumePercent, Played: settings.PlayedPercent}.Reaches(current.position, current.runtime)
	if mark {
		current.marked = true
	}
	snapshot := *current
	h.dlna.mu.Unlock()
	h.followDLNA(snapshot, plays.ReportProgress)
	if mark {
		h.markDLNAPlayed(context.WithoutCancel(ctx), snapshot.user, snapshot.item)
	}
}

// markDLNAPlayed marks a title played, as a played mark from an app does,
// and tells the user's tracking services. Resume points are left alone:
// devices send no position.
func (h *Handler) markDLNAPlayed(ctx context.Context, user accounts.User, item library.Item) {
	if item.Kind == library.KindChannel {
		return
	}
	now := time.Now().UTC()
	if _, err := h.UserData.Change(ctx, user.ID, []userdata.Item{stored(item)}, func(d *userdata.Data) { d.MarkPlayed(nil, now) }); err != nil {
		h.Logger.Warn("A title a DLNA device played could not be marked played", "error", err)
		return
	}
	h.userDataChanged(user, []library.Item{item})
	if h.Trackers != nil && (item.Kind == library.KindMovie || item.Kind == library.KindEpisode) {
		h.Trackers.Mark(user.ID, trackers.Mark{Played: true, Scope: markScope(item.Kind), Titles: h.trackedTitles(user, []library.Item{item})})
	}
}

// Allowed reports whether a DLNA device may browse and play for user now.
func (h *Handler) Allowed(user accounts.User) bool {
	return !user.Refused(h.now()) && !h.outsideHours(user)
}

// ContinueWatching lists what Continue Watching lists user.
func (h *Handler) ContinueWatching(ctx context.Context, user accounts.User) ([]library.Item, error) {
	entries, err := h.UserData.Resumable(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return h.resumeItems(ctx, user, entries, resumeKinds)
}

// NextUp lists what Next Up lists user.
func (h *Handler) NextUp(ctx context.Context, user accounts.User) ([]library.Item, error) {
	return h.nextUpItems(ctx, user, nil, nil, nil, true)
}

// dlnaRequest is the PlaybackInfo request a DLNA device stands for: its
// profile, under the user's limits.
func (h *Handler) dlnaRequest(user accounts.User, profile dlna.Profile, converted bool) playbackInfoRequest {
	request := playbackInfoRequest{DeviceProfile: profile.Playback()}
	if converted {
		request.EnableDirectPlay = new(false)
	}
	limitBitrate(&request, user)
	h.limitHeight(&request, user)
	return request
}

// Resource describes what a DLNA device is offered for an item, from what
// is known of its versions: the first offered whose analysis is known, as
// it is when the profile takes it, else converted to MPEG-TS. A title
// whose first version was never analyzed is offered converted, which plays
// whatever it is. A song is offered as it is when the profile takes it,
// else as MP3, and a channel as MPEG-TS.
func (h *Handler) Resource(ctx context.Context, user accounts.User, item library.Item, profile dlna.Profile) (dlna.Resource, bool) {
	switch {
	case item.Kind == library.KindChannel:
		return dlna.Resource{Converted: true, Extension: "ts", MimeType: "video/mpeg"}, true
	case audioItem(item.Kind):
		return h.songResource(ctx, user, item, profile), true
	case !versioned(item.Kind):
		return dlna.Resource{}, false
	}
	converted := dlna.Resource{Converted: true, Extension: "ts", MimeType: "video/mpeg", Duration: item.Runtime}
	converted.MimeType, converted.OrgPn = profile.MimeType("Video", "ts", "video/mpeg")
	p := h.cachedPlayable(ctx, user, item)
	if len(p.versions) == 0 {
		return converted, true
	}
	version := p.versions[0]
	analysis, known := h.Playback.Analyzed(ctx, version.ID)
	if !known {
		return converted, true
	}
	request := h.dlnaRequest(user, profile, false)
	streams := playback.MediaStreams(analysis, h.externals(ctx, p, version), h.Accounts.Settings().Language)
	allowed := h.convertible(h.Accounts.Conversions(user), user, version)
	decision, _, _ := h.trackDecision(p, version, streams, analysis, request, allowed, false)
	converted.Duration = analysis.Duration
	if video, ok := playback.LiveVideo(analysis); ok {
		converted.Width, converted.Height = video.Width, video.Height
	}
	converted.Captions = captioned(decision, streams)
	if !decision.DirectPlay || unplayable(request, analysis, decision, allowed) {
		return converted, true
	}
	container := containerOfName(version.Filename)
	if container == "" {
		container = strings.Split(playback.Container(analysis), ",")[0]
	}
	resource := converted
	resource.Converted, resource.Extension, resource.Size, resource.Bitrate = false, container, analysis.Size, analysis.Bitrate
	resource.MimeType, resource.OrgPn = profile.MimeType("Video", container, cmpOr(mimeTypes[container], "video/"+container))
	return resource, true
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// captioned reports whether a decision sends a text subtitle as a file
// beside the video, which DLNA devices take through CaptionInfo.sec.
func captioned(decision playback.Decision, streams []playback.MediaStream) bool {
	delivery, ok := decision.Subtitles[decision.SubtitleStreamIndex]
	if !ok || delivery.Method != "External" {
		return false
	}
	i := slices.IndexFunc(streams, func(s playback.MediaStream) bool {
		return s.Type == "Subtitle" && s.Index == decision.SubtitleStreamIndex
	})
	return i >= 0 && streams[i].IsTextSubtitleStream
}

// songResource describes a song: as it is when the profile takes its
// format, else as MP3.
func (h *Handler) songResource(ctx context.Context, user accounts.User, item library.Item, profile dlna.Profile) dlna.Resource {
	mp3 := dlna.Resource{Converted: true, Extension: "mp3", MimeType: "audio/mpeg", Duration: item.Runtime, OrgPn: "MP3"}
	if versions, ok := h.Library.CachedVersions(ctx, user, item.ID); ok && len(versions) > 0 && versions[0].Audio != nil {
		audio := versions[0].Audio
		if profile.TakesAudio(audio.Container, audio.Codec) {
			return dlna.Resource{Extension: audio.Container, MimeType: audioMime(audio.Container), Duration: item.Runtime, Size: versions[0].Size,
				Sampling: audio.SampleRate}
		}
	}
	return mp3
}

// audioMime is the MIME type of an audio container.
func audioMime(container string) string {
	switch container {
	case "mp3":
		return "audio/mpeg"
	case "flac":
		return "audio/flac"
	case "m4a", "mp4", "aac":
		return "audio/mp4"
	case "ogg":
		return "audio/ogg"
	}
	return "audio/" + container
}

// Play serves a DLNA device's request for an item's bytes, counted for the
// device's playback (see meteredDevice).
func (h *Handler) Play(w http.ResponseWriter, r *http.Request, play dlna.Play) {
	counted := h.meteredDevice(w, r, play.Device.ID)
	defer counted.release()
	w = counted
	switch {
	case play.Item.Kind == library.KindChannel:
		h.playDLNAChannel(w, r, play)
	case audioItem(play.Item.Kind):
		h.playDLNASong(w, r, play)
	case versioned(play.Item.Kind):
		h.playDLNAVideo(w, r, play)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// dlnaChosen is the version a DLNA play plays, and how.
type dlnaChosen struct {
	p       playable
	chosen  decided
	request playbackInfoRequest
}

// chooseDLNA chooses the version a DLNA device plays and how, as
// PlaybackInfo chooses for an app (see planGuest). It answers the request
// itself when nothing plays.
func (h *Handler) chooseDLNA(w http.ResponseWriter, r *http.Request, play dlna.Play) (dlnaChosen, bool) {
	ctx := r.Context()
	request := h.dlnaRequest(play.User, play.Profile, play.Converted)
	p, complete, err := h.playableToPlay(ctx, play.User, play.Item)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return dlnaChosen{}, false
	}
	settings := h.Accounts.Settings()
	candidatesOf := func(versions []library.Version) candidates {
		c := candidates{order: make([]int, len(versions)), analyzed: settings.VersionAttempts, named: -1}
		for i := range versions {
			c.order[i] = i
		}
		return c
	}
	choice := h.choose(r, play.User, p, play.Item.ID, request, candidatesOf(p.versions), settings.PreferDirectPlay)
	if choice.decided == nil && !complete && ctx.Err() == nil {
		versions, err := h.Library.Versions(ctx, play.User, play.Item.ID)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return dlnaChosen{}, false
		}
		p.versions = h.offered(ctx, play.User, versions)
		choice = h.choose(r, play.User, p, play.Item.ID, request, candidatesOf(p.versions), settings.PreferDirectPlay)
	}
	if choice.decided == nil || !choice.decided.decision.DirectPlay && !choice.decided.decision.HLS {
		// At the server's limit of video conversions, a version whose video
		// must be converted is not chosen: the device may try again.
		if slices.ContainsFunc(p.versions, func(v library.Version) bool { return !h.Playback.MayConvert(play.User.ID, v.ID) }) {
			w.WriteHeader(http.StatusServiceUnavailable)
			return dlnaChosen{}, false
		}
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return dlnaChosen{}, false
	}
	return dlnaChosen{p: p, chosen: *choice.decided, request: request}, true
}

// playDLNAVideo serves a movie or an episode: the file as it is, with byte
// ranges, when the device's profile takes it, else converted to MPEG-TS
// from the time the device asks.
func (h *Handler) playDLNAVideo(w http.ResponseWriter, r *http.Request, play dlna.Play) {
	choice, ok := h.chooseDLNA(w, r, play)
	if !ok {
		return
	}
	chosen := choice.chosen
	method := "DirectPlay"
	if !chosen.decision.DirectPlay {
		method = "DirectStream"
		if chosen.decision.Video != nil || chosen.decision.Audio != nil {
			method = "Transcode"
		}
	}
	runtime := cmpDuration(chosen.analysis.Duration, play.Item.Runtime)
	current, done, err := h.beginDLNA(r.Context(), play, chosen.version.ID, h.playMethod(playback.PlayState{PlayMethod: method}), runtime)
	if errors.Is(err, errDLNALimit) {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	defer done()
	header := w.Header()
	if chosen.decision.DirectPlay && !play.Seek {
		container := containerOfName(chosen.version.Filename)
		resource := dlna.Resource{}
		resource.MimeType, resource.OrgPn = play.Profile.MimeType("Video", container, cmpOr(mimeTypes[container], "application/octet-stream"))
		header.Set("ContentFeatures.DLNA.ORG", dlna.ContentFeatures(resource))
		size := chosen.analysis.Size
		counter := &sentCounter{ResponseWriter: w, plays: h.dlna, play: current}
		stop := h.reportWhile(r.Context(), current, func() time.Duration { return fraction(h.dlna.sent(current), size, runtime) })
		err := h.Playback.Serve(counter, r, chosen.version, playback.Delivery{Relay: true, ContentType: resource.MimeType})
		stop()
		if err != nil && r.Context().Err() == nil {
			h.Logger.Warn("A DLNA device's file could not be served", "addon", chosen.version.Addon, "error", err)
		}
		h.reachedDLNA(r.Context(), current, fraction(h.dlna.sent(current), size, runtime))
		return
	}
	if !chosen.decision.HLS {
		// A file sent as it is takes no time seek: it is converted from the
		// time asked.
		request := choice.request
		request.EnableDirectPlay = new(false)
		retry, ok := h.chooseDLNA(w, r, dlna.Play{Device: play.Device, User: play.User, Item: play.Item, Profile: play.Profile, Converted: true,
			Start: play.Start, Seek: play.Seek})
		if !ok {
			return
		}
		choice, chosen = retry, retry.chosen
	}
	source := h.decidedSource(r, choice.p, chosen, choice.request, current.id)
	_, params, found := strings.Cut(source.TranscodingUrl, "?")
	if !found {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}
	paramsRequest := r.Clone(r.Context())
	paramsRequest.URL = &url.URL{Path: r.URL.Path, RawQuery: params}
	remux, ok := h.remuxFor(w, paramsRequest, play.User, play.Item.ID, chosen.version.ID, current.id)
	if !ok {
		return
	}
	remux.remux.Format = hls.TS
	start := min(play.Start, max(runtime-time.Second, 0))
	resource := dlna.Resource{Converted: true}
	resource.MimeType, resource.OrgPn = play.Profile.MimeType("Video", "ts", "video/mpeg")
	header.Set("Content-Type", resource.MimeType)
	header.Set("ContentFeatures.DLNA.ORG", dlna.ContentFeatures(resource))
	if play.Seek && runtime > 0 {
		header.Set("TimeSeekRange.DLNA.ORG", "npt="+dlna.NPT(start)+"-"+dlna.NPT(runtime)+"/"+dlna.NPT(runtime))
	}
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	began := h.now()
	position := func() time.Duration { return start + h.now().Sub(began) }
	stop := h.reportWhile(r.Context(), current, position)
	err = h.Playback.Progressive(playback.ForUser(r.Context(), play.User.ID), remux.remux, start, w)
	stop()
	h.reachedDLNA(r.Context(), current, position())
	switch {
	case errors.Is(err, hls.ErrBusy):
		w.WriteHeader(http.StatusServiceUnavailable)
	case err != nil && r.Context().Err() == nil && !errors.Is(err, hls.ErrStopped):
		h.Logger.Warn("A DLNA device's conversion failed", "addon", chosen.version.Addon, "error", err)
	}
}

func cmpDuration(a, b time.Duration) time.Duration {
	if a > 0 {
		return a
	}
	return b
}

// reportWhile reports a playback every dlnaReport, at the position
// position gives, until stop.
func (h *Handler) reportWhile(ctx context.Context, current *dlnaPlay, position func() time.Duration) (stop func()) {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(dlnaReport)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.reachedDLNA(ctx, current, position())
			}
		}
	}()
	return func() { close(done) }
}

// fraction is the time of a title of runtime that sent bytes of a file of
// size stand for.
func fraction(sent, size int64, runtime time.Duration) time.Duration {
	if size <= 0 || runtime <= 0 {
		return 0
	}
	return time.Duration(float64(runtime) * min(float64(sent)/float64(size), 1))
}

// byteRange is the bytes [from, to) of a file.
type byteRange struct{ from, to int64 }

// byteRanges are bytes of a file, in order, none overlapping or touching
// another.
type byteRanges []byteRange

// add returns s with the bytes [from, to).
func (s byteRanges) add(from, to int64) byteRanges {
	if from >= to {
		return s
	}
	// s[i:j] are the ranges [from, to) overlaps or touches.
	i, _ := slices.BinarySearchFunc(s, from, func(have byteRange, from int64) int { return cmp.Compare(have.to, from) })
	j, _ := slices.BinarySearchFunc(s, to, func(have byteRange, to int64) int {
		if have.from <= to {
			return -1
		}
		return 1
	})
	if i < j {
		from, to = min(from, s[i].from), max(to, s[j-1].to)
	}
	return slices.Replace(s, i, j, byteRange{from, to})
}

// total counts the bytes of s.
func (s byteRanges) total() int64 {
	var n int64
	for _, have := range s {
		n += have.to - have.from
	}
	return n
}

// sent counts the bytes of its file a playback sent.
func (d *dlnaPlays) sent(play *dlnaPlay) int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return play.sent.total()
}

// sentCounter notes in a playback the bytes of its file an answer sends:
// from where its Content-Range starts, else from the file's start.
type sentCounter struct {
	http.ResponseWriter
	plays *dlnaPlays
	play  *dlnaPlay
	// offset is where the next byte written is in the file, once the
	// header is written.
	offset      int64
	wroteHeader bool
}

func (c *sentCounter) WriteHeader(status int) {
	if !c.wroteHeader {
		c.wroteHeader = true
		if status == http.StatusPartialContent {
			c.offset = contentRangeStart(c.Header().Get("Content-Range"))
		}
	}
	c.ResponseWriter.WriteHeader(status)
}

func (c *sentCounter) Write(p []byte) (int, error) {
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}
	n, err := c.ResponseWriter.Write(p)
	c.plays.mu.Lock()
	c.play.sent = c.play.sent.add(c.offset, c.offset+int64(n))
	c.plays.mu.Unlock()
	c.offset += int64(n)
	return n, err
}

func (c *sentCounter) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (c *sentCounter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

// contentRangeStart is the first byte a Content-Range tells, as in
// bytes 100-199/1000, 0 for none.
func contentRangeStart(header string) int64 {
	spec, ok := strings.CutPrefix(header, "bytes ")
	if !ok {
		return 0
	}
	first, _, _ := strings.Cut(spec, "-")
	start, err := strconv.ParseInt(strings.TrimSpace(first), 10, 64)
	if err != nil || start < 0 {
		return 0
	}
	return start
}

// playDLNASong serves a song as it is when the device's profile takes it,
// else converted to MP3 from the time asked.
func (h *Handler) playDLNASong(w http.ResponseWriter, r *http.Request, play dlna.Play) {
	ctx := r.Context()
	versions, err := h.Library.Versions(ctx, play.User, play.Item.ID)
	if err != nil || len(versions) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	version := versions[0]
	asIs := !play.Converted && version.Audio != nil && play.Profile.TakesAudio(version.Audio.Container, version.Audio.Codec)
	method := "DirectPlay"
	if !asIs {
		if !h.Accounts.Conversions(play.User).Audio {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}
		method = "Transcode"
	}
	current, done, err := h.beginDLNA(ctx, play, version.ID, h.playMethod(playback.PlayState{PlayMethod: method}), play.Item.Runtime)
	if errors.Is(err, errDLNALimit) {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	defer done()
	header := w.Header()
	if asIs {
		mime := audioMime(version.Audio.Container)
		header.Set("ContentFeatures.DLNA.ORG", dlna.ContentFeatures(dlna.Resource{}))
		counter := &sentCounter{ResponseWriter: w, plays: h.dlna, play: current}
		err := h.Playback.Serve(counter, r, version, playback.Delivery{Relay: true, ContentType: mime})
		if err != nil && ctx.Err() == nil {
			h.Logger.Warn("A DLNA device's song could not be served", "error", err)
		}
		h.reachedDLNA(ctx, current, fraction(h.dlna.sent(current), version.Size, play.Item.Runtime))
		return
	}
	header.Set("Content-Type", "audio/mpeg")
	header.Set("ContentFeatures.DLNA.ORG", dlna.ContentFeatures(dlna.Resource{Converted: true, OrgPn: "MP3"}))
	if play.Seek && play.Item.Runtime > 0 {
		header.Set("TimeSeekRange.DLNA.ORG", "npt="+dlna.NPT(play.Start)+"-"+dlna.NPT(play.Item.Runtime)+"/"+dlna.NPT(play.Item.Runtime))
	}
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	target := &playback.AudioTarget{Codec: "mp3", Bitrate: 192_000, Channels: 2}
	began := h.now()
	err = h.Playback.ConvertAudio(playback.ForUser(ctx, play.User.ID), version, target, "mp3", play.Start, w)
	h.reachedDLNA(ctx, current, play.Start+h.now().Sub(began))
	if err != nil && ctx.Err() == nil {
		h.Logger.Warn("A DLNA device's song could not be converted", "error", err)
	}
}

// playDLNAChannel serves a channel's MPEG-TS stream, relayed as it comes.
// A channel whose stream is an HLS playlist does not play over DLNA.
func (h *Handler) playDLNAChannel(w http.ResponseWriter, r *http.Request, play dlna.Play) {
	ctx := playback.ForUser(r.Context(), play.User.ID)
	version, err := h.version(ctx, play.User, play.Item, play.Item.ID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	analysis, err := h.Playback.AnalyzeLive(ctx, version)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	if playback.Manifest(analysis) || !play.User.FitsGroup(videoHeight(analysis)) {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}
	current, done, err := h.beginDLNA(ctx, play, version.ID, plays.DirectStream, 0)
	if errors.Is(err, errDLNALimit) {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	defer done()
	w.Header().Set("ContentFeatures.DLNA.ORG", "DLNA.ORG_OP=00;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=8D100000000000000000000000000000")
	began := h.now()
	stop := h.reportWhile(ctx, current, func() time.Duration { return h.now().Sub(began) })
	err = h.Playback.ServeChannel(w, r.WithContext(ctx), version, true)
	stop()
	if err != nil && r.Context().Err() == nil {
		h.Logger.Warn("A DLNA device's channel could not be served", "addon", version.Addon, "error", err)
	}
}

// Captions serves the text subtitle a play offers beside its video, as
// SubRip, as the subtitle route serves it to apps.
func (h *Handler) Captions(w http.ResponseWriter, r *http.Request, play dlna.Play) {
	if !versioned(play.Item.Kind) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	p := h.cachedPlayable(r.Context(), play.User, play.Item)
	if len(p.versions) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	version := p.versions[0]
	analysis, known := h.Playback.Analyzed(r.Context(), version.ID)
	if !known {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	streams := playback.MediaStreams(analysis, h.externals(r.Context(), p, version), h.Accounts.Settings().Language)
	request := h.dlnaRequest(play.User, play.Profile, false)
	decision, _, _ := h.trackDecision(p, version, streams, analysis, request, h.Accounts.Conversions(play.User), false)
	if !captioned(decision, streams) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := version.ID
	if len(p.versions) > 0 && p.versions[0].ID == version.ID {
		id = play.Item.ID
	}
	subtitle := r.Clone(r.Context())
	grant := h.Playback.Signer().Sign(playback.Grant{Version: version.ID, User: play.User.ID, Relay: true})
	subtitle.URL = &url.URL{Path: r.URL.Path, RawQuery: url.Values{grantParameter: {grant}}.Encode()}
	subtitle.SetPathValue("itemId", play.Item.ID.String())
	subtitle.SetPathValue("mediaSourceId", id.String())
	subtitle.SetPathValue("index", strconv.Itoa(decision.SubtitleStreamIndex))
	subtitle.SetPathValue("startTicks", "")
	subtitle.SetPathValue("file", "Stream.srt")
	h.subtitle(w, subtitle)
}

// Art serves an item's poster as a JPEG at most width wide: its own, else
// its series' or album's.
func (h *Handler) Art(w http.ResponseWriter, r *http.Request, id accounts.ID, width int) {
	image := r.Clone(r.Context())
	image.URL = &url.URL{Path: r.URL.Path, RawQuery: url.Values{"maxWidth": {strconv.Itoa(width)}}.Encode()}
	image.Header.Del("If-None-Match")
	image.SetPathValue("itemId", id.String())
	image.SetPathValue("imageType", "Primary")
	image.SetPathValue("imageIndex", "")
	captured := &capturedImage{header: http.Header{}, status: http.StatusOK}
	h.taggedArtwork(captured, image, "", false)
	if captured.status != http.StatusOK || captured.body.Len() == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	body := captured.body.Bytes()
	if !strings.HasPrefix(captured.header.Get("Content-Type"), "image/jpeg") {
		converted, err := toJPEG(body)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body = converted
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("ContentFeatures.DLNA.ORG", "DLNA.ORG_PN=JPEG_TN;DLNA.ORG_OP=01;DLNA.ORG_CI=1")
	w.Header().Set("Cache-Control", "max-age=86400")
	_, _ = w.Write(body)
}

// toJPEG encodes an image as a JPEG.
func toJPEG(data []byte) ([]byte, error) {
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, decoded, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// capturedImage keeps an image answer to send it again.
type capturedImage struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (c *capturedImage) Header() http.Header         { return c.header }
func (c *capturedImage) WriteHeader(status int)      { c.status = status }
func (c *capturedImage) Write(p []byte) (int, error) { return c.body.Write(p) }

// dlnaSessions describes the DLNA playbacks for the admin dashboard.
func (h *Handler) dlnaSessions(ctx context.Context, encodings map[string][]hls.Running) []LiveSession {
	h.endStaleDLNA(accounts.ID{}, "")
	var sessions []LiveSession
	for _, play := range h.dlna.all() {
		session := LiveSession{User: play.user, Device: dlnaDevice(play),
			Playing: playback.NowPlaying{PlayState: playback.PlayState{Item: play.item.ID, MediaSourceID: play.version.String(),
				PlaySessionID: play.id, PlayMethod: dlnaPlayMethod(play.method), Position: play.position}, User: play.user.ID,
				Started: play.started, CheckedIn: play.seen},
			Item: play.item, Found: true, Encodings: encodings[play.id]}
		session.Reasons, _ = h.reasons.Get(play.id)
		if analysis, ok := h.Playback.Analyzed(ctx, play.version); ok {
			session.Source = &analysis
		}
		sessions = append(sessions, session)
	}
	return sessions
}

// dlnaPlayMethod is how a session says a DLNA playback reaches its device.
func dlnaPlayMethod(method string) string {
	switch method {
	case plays.DirectPlay:
		return "DirectPlay"
	case plays.DirectStream:
		return "DirectStream"
	}
	return "Transcode"
}

// dlnaDevice is a DLNA playback's device as sessions describe devices.
func dlnaDevice(play dlnaPlay) accounts.Device {
	return accounts.Device{ID: play.device.ID, UserID: play.user.ID, LastActivityAt: play.seen,
		DeviceInfo: accounts.DeviceInfo{DeviceName: play.device.DisplayName(), Client: dlnaApp, DeviceID: "dlna-" + play.device.ID.String(),
			RemoteAddress: play.device.Address}}
}

// dlnaSessionInfos describes user's DLNA playbacks as /Sessions does:
// Polyfin does not control DLNA devices.
func (h *Handler) dlnaSessionInfos(r *http.Request, user accounts.User) []SessionInfo {
	h.endStaleDLNA(accounts.ID{}, "")
	var infos []SessionInfo
	for _, play := range h.dlna.all() {
		if play.user.ID != user.ID {
			continue
		}
		info := newSessionInfo(dlnaDevice(play), user, h.ServerID, false)
		h.describePlaying(r, user, &info, playback.NowPlaying{PlayState: playback.PlayState{Item: play.item.ID,
			MediaSourceID: play.version.String(), PlaySessionID: play.id, PlayMethod: dlnaPlayMethod(play.method), Position: play.position},
			User: user.ID, Started: play.started, CheckedIn: play.seen})
		infos = append(infos, info)
	}
	return infos
}

var _ dlna.Player = (*Handler)(nil)
