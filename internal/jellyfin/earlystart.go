package jellyfin

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A play started early. With playback prepared ahead, once a title's
// details opened and the version its Play takes first is analyzed, the
// play is started for the user's device as its PlaybackInfo would start
// it: when the device plays the version repackaged or converted over HLS,
// FFmpeg started in a play session of its own, which reads the bytes of
// the first segment from the source and nothing more, and makes that
// segment (see hls.Manager.Early); else those bytes read into the source
// cache. The PlaybackInfo of that version on that device then answers
// with that play session (see playSession), whose first HLS request
// takes the encoding over. A play with other parameters, as another audio
// track or quality, asks for another encoding, which stops the one
// started early.
//
// Play pressed at once meets the play started early while it starts:
// both wait for the same analysis. The play session is set before FFmpeg
// starts (see reserveEarly), so a PlaybackInfo coming meanwhile goes on
// from it. A PlaybackInfo over HLS from the start that finds none to go
// on from, as one that came before the play started early began, starts
// FFmpeg itself as it answers, in its own play session, as the play
// started early would have (see startWithPlay); its play is kept as the
// device's, so that no play starts early for its title meanwhile. FFmpeg
// then starts once, as the analysis ends, rather than twice, or once the
// app's first HLS request comes.
//
// Each user's device has one at most, for the title it opened last: the
// details of another title end it, and what FFmpeg made is removed if no
// play took it over within about earlyIdle; the title a Play took opened
// again within earlyIdle starts none. It counts toward the conversions at
// once only once a play took it over, and FFmpeg does not start while the
// server converts video. A title with a resume position, or that would
// play as it is, reads its first bytes only as its play would.

// earlyIdle is how long a play started early waits for its Play: past it,
// opening the title again starts it again.
const earlyIdle = 30 * time.Second

// earlyOwner is whose a play started early is: a user's on a device.
type earlyOwner struct{ user, device accounts.ID }

// earlyPlay is a play started early for the title a user's device opened
// (see startEarly), or the play a PlaybackInfo of it went on with (see
// takeEarly).
type earlyPlay struct {
	title, version accounts.ID
	// stop ends the read of its first segment's bytes.
	stop context.CancelFunc
	// at is when it started; grant, the grant of its play session,
	// unsigned, and session that play session, set once FFmpeg is about
	// to start: empty before, or when it did not.
	at      time.Time
	grant   playback.Grant
	session string
	// taken is set once a PlaybackInfo answered with session: the play
	// is the app's, and nothing starts early for its title meanwhile.
	taken bool
}

// earlyOwnerOf is the owner of the plays started early for the caller of
// ctx: false for a caller that is no user's device, as an API key's.
func earlyOwnerOf(ctx context.Context) (earlyOwner, bool) {
	c := callerFrom(ctx)
	if c.APIKey != nil || c.Device.ID == (accounts.ID{}) {
		return earlyOwner{}, false
	}
	return earlyOwner{user: c.User.ID, device: c.Device.ID}, true
}

// earlyFrom is a copy of r, the request that opened a title's details,
// without its body or cancellation: a play started early is asked as from
// its caller, its credentials and its address.
func earlyFrom(r *http.Request) *http.Request {
	from := r.Clone(context.WithoutCancel(r.Context()))
	from.Body, from.GetBody, from.ContentLength = http.NoBody, nil, 0
	return from
}

// claimEarly records play, of owner, in place of the one before, which it
// returns for the caller to end, and the plays started early that ended
// long ago, which no Play took (see forgetEarly); unless the one before is
// of the same title and version, started within earlyIdle, or the play of
// the same title a PlaybackInfo went on with within earlyIdle (see
// takeEarly): false then, and nothing changes.
func (p *preparations) claimEarly(owner earlyOwner, play *earlyPlay) (ended []*earlyPlay, ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	before := p.early[owner]
	if before != nil && before.title == play.title && (before.taken || before.version == play.version) && now.Sub(before.at) < earlyIdle {
		return nil, false
	}
	ended = p.forgetEarly(now)
	if before != nil && now.Sub(before.at) < earlyKept {
		ended = append(ended, before)
	}
	play.at = now
	p.early[owner] = play
	return ended, true
}

// earlyKept is how long a play started early is kept for its Play to
// take over, FFmpeg's having stopped after earlyIdle: the first bytes read
// stay in the source cache.
const earlyKept = 10 * time.Minute

// forgetEarly forgets the plays started early more than earlyKept ago,
// and returns them. The caller holds p.mu.
func (p *preparations) forgetEarly(now time.Time) []*earlyPlay {
	var ended []*earlyPlay
	for owner, play := range p.early {
		if now.Sub(play.at) >= earlyKept {
			ended = append(ended, play)
			delete(p.early, owner)
		}
	}
	return ended
}

// keepsEarly reports whether play is still the one owner started early.
func (p *preparations) keepsEarly(owner earlyOwner, play *earlyPlay) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.early[owner] == play
}

// reserveEarly records the play session of play, of owner, under grant,
// as its FFmpeg is about to start, unless another play took its place
// meanwhile: false then. A PlaybackInfo then goes on from that session
// (see takeEarly), FFmpeg started or not.
func (p *preparations) reserveEarly(owner earlyOwner, play *earlyPlay, grant playback.Grant, session string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.early[owner] != play {
		return false
	}
	grant.Signed = time.Time{}
	play.grant, play.session = grant, session
	return true
}

// leaveEarly forgets the play owner started early for another title than
// title, and returns it, nil when there is none.
func (p *preparations) leaveEarly(owner earlyOwner, title accounts.ID) *earlyPlay {
	p.mu.Lock()
	defer p.mu.Unlock()
	play := p.early[owner]
	if play == nil || play.title == title {
		return nil
	}
	delete(p.early, owner)
	return play
}

// takeEarly returns the play session a PlaybackInfo of title under grant
// answers with. It is that of the play started early for the device and
// user of grant when a play under grant may go on from it: fits, and the
// play's FFmpeg started, or is about to, under the same grant; true then,
// and the play is the PlaybackInfo's. Else it is fresh, and the play
// started early, if any, is returned for the caller to end; when fits and
// keep, as with playback prepared ahead, fresh takes its place, so that
// nothing starts early for title while the app plays it (see claimEarly).
func (p *preparations) takeEarly(title accounts.ID, grant playback.Grant, fits, keep bool, fresh string) (session string, early bool, ended *earlyPlay) {
	p.mu.Lock()
	defer p.mu.Unlock()
	owner := earlyOwner{user: grant.User, device: grant.Device}
	grant.Signed = time.Time{}
	play := p.early[owner]
	if fits && play != nil && !play.taken && play.session != "" && play.grant == grant {
		play.taken = true
		return play.session, true, nil
	}
	delete(p.early, owner)
	if fits && keep {
		p.early[owner] = &earlyPlay{title: title, version: grant.Version, stop: func() {}, at: p.now(), grant: grant, session: fresh, taken: true}
	}
	return fresh, false, play
}

// answeredWith reports whether the play session a PlaybackInfo of owner
// answered with last is session (see takeEarly).
func (p *preparations) answeredWith(owner earlyOwner, session string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	play := p.early[owner]
	return play != nil && play.taken && play.session == session
}

// endEarly ends plays started early that no Play took over: the reads of
// their first segment's bytes, and their FFmpeg.
func (h *Handler) endEarly(plays ...*earlyPlay) {
	for _, play := range plays {
		if play == nil {
			continue
		}
		play.stop()
		if play.session != "" {
			h.Playback.StopEarly(play.session)
		}
	}
}

// leaveEarly ends the play the device of from started early for a title
// other than title, which its user opened instead.
func (h *Handler) leaveEarly(from *http.Request, title accounts.ID) {
	if owner, ok := earlyOwnerOf(from.Context()); ok {
		h.endEarly(h.preparations.leaveEarly(owner, title))
	}
}

// playSession is the play session a PlaybackInfo of title under grant
// answers with: that of the play started early for grant's device and
// user, when it goes on from it as fits, a play over HLS from the start;
// true then. Else a new one, which ends the play started early, if any,
// and, with playback prepared ahead, is kept as the device's play.
func (h *Handler) playSession(title accounts.ID, grant playback.Grant, fits bool) (string, bool) {
	fresh := h.grant(grant)
	if grant.Device == (accounts.ID{}) {
		return fresh, false
	}
	session, early, ended := h.preparations.takeEarly(title, grant, fits, h.Accounts.Settings().PrepareAhead, fresh)
	h.endEarly(ended)
	return session, early
}

// startEarly starts the play of version, the version the title item's Play
// takes first, for the device whose request from opened its details as
// opened: see the top of this file. It returns once FFmpeg started, or
// once it knows it does not start.
func (h *Handler) startEarly(from *http.Request, user accounts.User, item library.Item, version library.Version, opened accounts.ID) {
	defer logPanic(h.Logger, "Starting a play early ended on a panic")
	owner, ok := earlyOwnerOf(from.Context())
	// Only a play reads a torrent, and a replay holds a connection of its
	// source while it plays.
	if !ok || version.Torrent != nil || version.HoldsConnection || h.Playback.Failed(version.ID) {
		return
	}
	ctx := library.Background(stremio.WithOrigin(from.Context(), stremio.OriginPreload))
	analysis, analyzed := h.Playback.Analyzed(ctx, version.ID)
	if !analyzed || h.playsLive(ctx, item, version) {
		return
	}
	var start time.Duration
	if data, err := h.UserData.Get(ctx, user.ID, []accounts.ID{item.ID}); err == nil {
		start = data[item.ID].Position
	}
	warm, stop := context.WithTimeout(ctx, earlyIdle)
	play := &earlyPlay{title: item.ID, version: version.ID, stop: stop}
	ended, claimed := h.preparations.claimEarly(owner, play)
	h.endEarly(ended...)
	if !claimed {
		stop()
		return
	}
	// FFmpeg started early reads the first segment's bytes itself, and no
	// more (see playback.Service.StartEarly).
	if start == 0 && h.encodeEarly(ctx, from, owner, play, user, item, version, analysis, opened) {
		stop()
		return
	}
	// Else, whatever its play, its first bytes: a play over HLS reads them
	// first, and so does a stream of the file from its start.
	go func() {
		defer stop()
		defer logPanic(h.Logger, "Reading a version's first bytes early ended on a panic")
		if err := h.Playback.WarmAhead(warm, version, start, func(int64) bool { return true }); err != nil && warm.Err() == nil {
			h.Logger.Debug("A version's first bytes were not read early", "addon", version.AddonLog, "error", err)
		}
	}()
}

// encodeEarly starts FFmpeg for play, which owner started early from the
// start of version, when the device plays it repackaged or converted over
// HLS, and reports whether it started, or whether a Play or another title
// took its place meanwhile.
func (h *Handler) encodeEarly(ctx context.Context, from *http.Request, owner earlyOwner, play *earlyPlay, user accounts.User, item library.Item,
	version library.Version, analysis media.Analysis, opened accounts.ID,
) bool {
	request, known := h.earlyRequest(ctx, user)
	if !known {
		return false
	}
	r := from.WithContext(ctx)
	p, err := h.playable(ctx, user, item)
	i := slices.IndexFunc(p.versions, func(v library.Version) bool { return v.ID == version.ID })
	if err != nil || i < 0 {
		return false
	}
	// Decided and asked as PlaybackInfo and the app's first HLS request
	// would, in a play session of its own.
	d, plays := h.decide(r, p, i, version, sourceID(opened, version, i == openedIndex(opened, p.versions)), analysis, request,
		h.convertible(h.Accounts.Conversions(user), user, version), false)
	if !plays || !d.decision.HLS {
		return false
	}
	h.deliverable(ctx, d.decision, d.source.MediaStreams, version, analysis, true)
	_, tokenless := integratedPlayer(r, request)
	grant := playback.Grant{Version: version.ID, User: user.ID, Device: owner.device, Relay: tokenless || mustRelay(r, version)}
	session := h.grant(grant)
	address, err := url.Parse(h.playURL(r, p, d, request, session))
	if err != nil {
		return false
	}
	asked := r.Clone(ctx)
	asked.Method, asked.URL, asked.RequestURI = http.MethodGet, address, address.RequestURI()
	remux, ok := h.remuxFor(discardResponse{}, asked, user, item.ID, version.ID, session)
	if !ok || remux.live {
		return false
	}
	started := time.Now()
	// A Play that came meanwhile, or another title opened, took its place;
	// else a PlaybackInfo coming from now on goes on from this session.
	if !h.preparations.reserveEarly(owner, play, grant, session) {
		return true
	}
	if err := h.Playback.StartEarly(ctx, remux.remux); err != nil {
		if errors.Is(err, hls.ErrBusy) {
			h.Logger.Debug("A play was not started early: the server converts video", "addon", version.AddonLog)
		} else {
			h.Logger.Debug("A play could not be started early", "addon", version.AddonLog, "error", err)
		}
		return false
	}
	// Another title opened, or a PlaybackInfo under another grant, ended
	// it meanwhile; a PlaybackInfo that went on from it keeps it.
	if !h.preparations.keepsEarly(owner, play) {
		h.Playback.StopEarly(session)
		return true
	}
	h.Logger.Debug("Started a play early", "addon", version.AddonLog, "converts", remux.remux.ConvertVideo != nil, "fast", remux.remux.FastStart,
		"took", time.Since(started))
	return true
}

// startWithPlay starts FFmpeg for the play over HLS from the start a
// PlaybackInfo r answered with, in its play session, at target, as the
// play started early would have (see startEarly): with playback prepared
// ahead, when none was there to go on from, as for a Play pressed before
// the version's analysis ended. FFmpeg then starts as the app reads the
// answer rather than once its first HLS request comes, which takes it
// over. It starts in the background, and not once another PlaybackInfo
// or title of the device took its place.
func (h *Handler) startWithPlay(r *http.Request, user accounts.User, item library.Item, version library.Version, target, session string) {
	owner, ok := earlyOwnerOf(r.Context())
	if !ok || !h.Accounts.Settings().PrepareAhead || version.Torrent != nil || version.HoldsConnection {
		return
	}
	address, err := url.Parse(target)
	if err != nil {
		return
	}
	from := earlyFrom(r)
	go func() {
		defer logPanic(h.Logger, "Starting a play as its PlaybackInfo answered ended on a panic")
		ctx := from.Context()
		asked := from.Clone(ctx)
		asked.Method, asked.URL, asked.RequestURI = http.MethodGet, address, address.RequestURI()
		remux, ok := h.remuxFor(discardResponse{}, asked, user, item.ID, version.ID, session)
		if !ok || remux.live || !h.preparations.answeredWith(owner, session) {
			return
		}
		if err := h.Playback.StartEarly(ctx, remux.remux); err != nil {
			if !errors.Is(err, hls.ErrBusy) {
				h.Logger.Debug("A play could not start as its PlaybackInfo answered", "addon", version.AddonLog, "error", err)
			}
			return
		}
		if !h.preparations.answeredWith(owner, session) {
			h.Playback.StopEarly(session)
		}
	}()
}

// earlyRequest is the request a PlaybackInfo of the caller of ctx would
// make, from its device profile known (see knownPlay). A device that told
// none yet, as a browser signed in for the first time, is taken to play
// as the device of the same app and name its user played on last, which
// plays alike: false when there is none either.
func (h *Handler) earlyRequest(ctx context.Context, user accounts.User) (playbackInfoRequest, bool) {
	request, _, known := h.knownPlay(ctx, user)
	if !known {
		c := callerFrom(ctx)
		stored, found, err := h.Accounts.LatestDevicePlay(ctx, user.ID, c.Device.Client, c.Device.DeviceName)
		if err != nil || !found {
			return playbackInfoRequest{}, false
		}
		play := storedPlay(stored)
		if play.profile == nil {
			return playbackInfoRequest{}, false
		}
		request = playbackInfoRequest{DeviceProfile: play.profile, MaxStreamingBitrate: play.bitrate}
		limitBitrate(&request, user)
		h.limitHeight(&request, user)
		request.convertSound = user.ConvertSound
	}
	request.progressive = h.downloadsConverted(user)
	return request, true
}

// discardResponse is a response nothing reads: remuxFor's for a play
// started early, which answers no app.
type discardResponse struct{}

func (discardResponse) Header() http.Header         { return http.Header{} }
func (discardResponse) Write(b []byte) (int, error) { return len(b), nil }
func (discardResponse) WriteHeader(int)             {}
