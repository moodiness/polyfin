package jellyfin

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/forwarded"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/plays"
)

// A share link (see accounts.ShareLink) lets a guest without an account
// watch one movie or one episode of the user who made it, its sharer, at
// /share/{token}: a page of Polyfin's own, in English or French, with the
// title, its artwork, its overview and a player. The token opens nothing
// else: the page's routes below are the only ones that read it, and every
// other route refuses it as any unknown token.
//
//	GET  /share/{token}                    the page;
//	GET  /share/{token}/image/{kind}       its artwork, primary or backdrop;
//	POST /share/{token}/play               starts a guest's playback;
//	GET  /share/{token}/play/{session}/…   its HLS playlists, segments and
//	                                       text subtitles;
//	POST /share/{token}/play/{session}/progress, …/stop
//	                                       its position, and its end;
//	GET  /share/assets/{file}              the page's script, style and
//	                                       hls.js, the same for every link.
//
// A guest plays through Polyfin, never from the source, as stream
// addresses can hold keys, and over HLS only, nothing to download. The
// sharer's rules apply as if they played: their version rules, their
// quality group and Maximum quality, with video never taller than 1080p,
// their Playbacks at once, which guests count toward, their Allowed hours,
// their permissions to have video and audio converted, and the server's
// Video conversions at once. A guest changes nothing in the sharer's watch
// state: no report reaches the user data. The playbacks show on the admin
// app's Home under the sharer's name, with guestApp as the app, and count
// in the sharer's statistics, marked as shared.

const (
	// guestApp and guestDevice name a guest's playback where an app's
	// and a device's names show.
	guestApp    = "Shared link"
	guestDevice = "Web browser"
	// shareMaxHeight is the tallest video a guest is sent.
	shareMaxHeight = 1080
	// guestStale is how long a guest's playback lasts without a request:
	// the page reports its position every 10 seconds, paused or not.
	guestStale = 2 * time.Minute
)

// The reasons a guest's playback does not start, as the page's script
// reads them.
const (
	shareRefusedLimit       = "limit"
	shareRefusedViewers     = "viewers"
	shareRefusedHours       = "hours"
	shareRefusedBusy        = "busy"
	shareRefusedUnavailable = "unavailable"
)

//go:embed share
var shareFiles embed.FS

var sharePage = template.Must(template.ParseFS(shareFiles, "share/page.html"))

// shareAssets are the files of /share/assets/: the page's script and
// style, and hls.js, vendored (see third_party/hls.js).
var shareAssets = func() fs.FS {
	sub, err := fs.Sub(shareFiles, "share/assets")
	if err != nil {
		panic(err)
	}
	return sub
}()

// guestProfile is what a guest's browser is sent: H.264 video and AAC or
// MP3 audio, in fragmented MP4 over HLS, which every browser plays, with
// hls.js where HLS is not native. Nothing plays as it is.
var guestProfile = playback.DeviceProfile{
	Name: "Polyfin shared link",
	TranscodingProfiles: []playback.TranscodingProfile{{Container: "mp4", Type: "Video", VideoCodec: "h264", AudioCodec: "aac,mp3",
		Context: "Streaming", Protocol: "hls", MaxAudioChannels: "2", MinSegments: "1", BreakOnNonKeyFrames: true}},
	CodecProfiles: []playback.CodecProfile{{Type: "Video", Codec: "h264", Conditions: []playback.ProfileCondition{
		{Condition: "EqualsAny", Property: "VideoProfile", Value: "high|main|baseline|constrained baseline"},
		{Condition: "EqualsAny", Property: "VideoRangeType", Value: "SDR"},
		{Condition: "LessThanEqual", Property: "VideoLevel", Value: "51"},
		{Condition: "NotEquals", Property: "IsInterlaced", Value: "true"},
		{Condition: "LessThanEqual", Property: "VideoBitDepth", Value: "8"},
	}}},
	SubtitleProfiles: []playback.SubtitleProfile{{Format: "vtt", Method: "Hls"}, {Format: "srt", Method: "Hls"}, {Format: "subrip", Method: "Hls"},
		{Format: "ass", Method: "Hls"}, {Format: "ssa", Method: "Hls"}},
}

// shareUser is the sharer as a guest plays for them: their quality group,
// lowered to 1080p.
func shareUser(user accounts.User) accounts.User {
	if user.QualityGroup == 0 || user.QualityGroup > shareMaxHeight {
		user.QualityGroup = shareMaxHeight
	}
	return user
}

// guestPlay is a guest's playback through a link.
type guestPlay struct {
	// id is its play session, which names its encodings: random, and
	// signed for no one, so that no other route takes it.
	id string
	// device stands for the guest where a device is counted.
	device  accounts.ID
	link    accounts.ShareLink
	user    accounts.ID
	item    library.Item
	version accounts.ID
	// params are the parameters of its remux, as PlaybackInfo's
	// TranscodingUrl gives them; the guest never sees them.
	params  string
	method  string
	address string
	started time.Time
	// seen is when the guest last asked anything of it.
	seen     time.Time
	position time.Duration
	paused   bool
}

// guestPlays are the guests' playbacks under way.
type guestPlays struct {
	mu    sync.Mutex
	plays map[string]*guestPlay
}

func newGuestPlays() *guestPlays {
	return &guestPlays{plays: map[string]*guestPlay{}}
}

// admit adds play unless its link has as many guests as it allows, or its
// sharer plays on as many devices as they may, playing of them on their
// own devices; it returns why it refused, empty when it did not.
func (g *guestPlays) admit(play *guestPlay, most, playing int) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	viewers, user := 0, 0
	for _, other := range g.plays {
		if other.link.ID == play.link.ID {
			viewers++
		}
		if other.user == play.user {
			user++
		}
	}
	switch {
	case viewers >= play.link.MaxViewers:
		return shareRefusedViewers
	case most > 0 && playing+user >= most:
		return shareRefusedLimit
	}
	g.plays[play.id] = play
	return ""
}

// get returns the playback of session, through link, noting the guest
// asked for it at now.
func (g *guestPlays) get(session string, link accounts.ID, now time.Time) (guestPlay, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	play, ok := g.plays[session]
	if !ok || play.link.ID != link {
		return guestPlay{}, false
	}
	play.seen = now
	return *play, true
}

// progress notes where the playback of session is.
func (g *guestPlays) progress(session string, position time.Duration, paused bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if play, ok := g.plays[session]; ok {
		play.position, play.paused = max(position, 0), paused
	}
}

// remove ends the playbacks keep chooses, and returns them.
func (g *guestPlays) remove(keep func(*guestPlay) bool) []guestPlay {
	g.mu.Lock()
	defer g.mu.Unlock()
	var removed []guestPlay
	for id, play := range g.plays {
		if !keep(play) {
			removed = append(removed, *play)
			delete(g.plays, id)
		}
	}
	return removed
}

// ofUser counts the playbacks of guests of user.
func (g *guestPlays) ofUser(user accounts.ID) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for _, play := range g.plays {
		if play.user == user {
			n++
		}
	}
	return n
}

// all returns the playbacks under way.
func (g *guestPlays) all() []guestPlay {
	g.mu.Lock()
	defer g.mu.Unlock()
	all := make([]guestPlay, 0, len(g.plays))
	for _, play := range g.plays {
		all = append(all, *play)
	}
	return all
}

// endGuests ends the guests' playbacks that keep does not keep: their
// encodings stop, and the playbacks Polyfin follows end.
func (h *Handler) endGuests(keep func(*guestPlay) bool) {
	for _, play := range h.guests.remove(keep) {
		h.Playback.StopRemux(play.id)
		h.followGuest(play, plays.ReportStop)
	}
}

// endStaleGuests ends the playbacks of guests who asked for nothing in
// guestStale.
func (h *Handler) endStaleGuests() {
	now := h.now()
	h.endGuests(func(play *guestPlay) bool { return now.Sub(play.seen) < guestStale })
}

// followGuest tells the playbacks Polyfin follows (see Options.Plays) of a
// guest's.
func (h *Handler) followGuest(play guestPlay, kind int) {
	if h.Plays == nil {
		return
	}
	h.Plays.Report(plays.Report{Kind: kind, Device: play.device, User: play.user, UserName: play.link.User.Name, App: guestApp,
		DeviceName: guestDevice, Title: playedTitle(play.item), Position: play.position, PositionKnown: true, Paused: play.paused,
		Method: h.playMethod(playback.PlayState{PlayMethod: play.method, PlaySessionID: play.id}), Shared: true})
}

// Share serves the share links' routes, under /share/.
func (h *Handler) Share() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /share/assets/{file}", http.StripPrefix("/share/assets/", shareAssetsHandler()))
	mux.HandleFunc("GET /share/{token}", h.sharePage)
	mux.HandleFunc("GET /share/{token}/image/{kind}", h.shareImage)
	mux.HandleFunc("POST /share/{token}/play", h.startGuest)
	mux.HandleFunc("POST /share/{token}/play/{session}/progress", h.guestProgress)
	mux.HandleFunc("POST /share/{token}/play/{session}/stop", h.stopGuest)
	mux.HandleFunc("GET /share/{token}/play/{session}/{file}", h.guestPlaylist)
	mux.HandleFunc("GET /share/{token}/play/{session}/hls1/{playlistId}/{file}", h.guestSegment)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Robots-Tag", "noindex, nofollow")
		header.Set("X-Frame-Options", "DENY")
		mux.ServeHTTP(w, r)
	})
}

// shareAssetsHandler serves the files of /share/assets/: hls.js, which
// never changes under its name, kept for a day, the page's own files
// checked again at each load.
func shareAssetsHandler() http.Handler {
	files := http.FileServerFS(shareAssets)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "hls") {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

// shareError is how a link that does not open its title answers.
type shareError struct {
	status int
	code   string
}

// openShare reads the link of the request's token, while it opens its
// title, with its sharer as they are now. A client that failed too many
// sign-ins is refused first; a token that is no link's counts as a failed
// sign-in of the client.
func (h *Handler) openShare(r *http.Request) (accounts.ShareLink, accounts.User, *shareError) {
	key := forwarded.Client(r)
	if allowed, _ := h.SignIns.Allowed(key); !allowed {
		return accounts.ShareLink{}, accounts.User{}, &shareError{http.StatusTooManyRequests, "too_many_attempts"}
	}
	link, user, err := h.Accounts.ShareLinkByToken(r.Context(), r.PathValue("token"))
	switch {
	case errors.Is(err, accounts.ErrShareLinkUnknown):
		h.SignIns.Fail(key)
		return link, user, &shareError{http.StatusNotFound, "unknown"}
	case errors.Is(err, accounts.ErrShareLinkExpired):
		return link, user, &shareError{http.StatusGone, "expired"}
	case errors.Is(err, accounts.ErrShareLinkRevoked):
		return link, user, &shareError{http.StatusGone, "revoked"}
	case errors.Is(err, accounts.ErrShareLinksOff):
		return link, user, &shareError{http.StatusGone, "off"}
	case err != nil:
		h.Logger.Warn("A share link could not be read", "error", err)
		return link, user, &shareError{http.StatusInternalServerError, "error"}
	}
	return link, user, nil
}

// openTitle reads the link of the request's token and the title it opens,
// which its sharer must still be allowed to see.
func (h *Handler) openTitle(r *http.Request) (accounts.ShareLink, accounts.User, library.Item, *shareError) {
	link, user, failed := h.openShare(r)
	if failed != nil {
		return link, user, library.Item{}, failed
	}
	item, err := h.Library.Shareable(r.Context(), user, link.Item)
	if err != nil {
		if !errors.Is(err, library.ErrNotFound) && !errors.Is(err, library.ErrNotShareable) {
			h.Logger.Warn("The title of a share link could not be read", "error", err)
		}
		return link, user, library.Item{}, &shareError{http.StatusGone, shareRefusedUnavailable}
	}
	return link, user, item, nil
}

// writeShareError answers the page's script.
func writeShareError(w http.ResponseWriter, failed shareError) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, failed.status, map[string]string{"error": failed.code})
}

// shareLanguage is the language of the page: French or English as the
// browser prefers, else the server's.
func (h *Handler) shareLanguage(r *http.Request) string {
	for part := range strings.SplitSeq(r.Header.Get("Accept-Language"), ",") {
		tag, _, _ := strings.Cut(strings.TrimSpace(part), ";")
		base, _, _ := strings.Cut(strings.ToLower(tag), "-")
		switch base {
		case "fr", "en":
			return base
		}
	}
	if h.Accounts.Settings().Language == "fr" {
		return "fr"
	}
	return "en"
}

// sharePageData is what the page shows.
type sharePageData struct {
	Language string
	Text     shareText
	Server   string
	// Title is unset for a link that does not open its title, Message
	// then saying why.
	Title    *shareTitleData
	Message  string
	Messages map[string]string
}

type shareTitleData struct {
	Name     string
	Subtitle string
	Overview string
	Year     int
	Minutes  int
	Expires  time.Time
}

// sharePage serves the page of a link, or says why the link opens
// nothing.
func (h *Handler) sharePage(w http.ResponseWriter, r *http.Request) {
	language := h.shareLanguage(r)
	text := shareTexts[language]
	data := sharePageData{Language: language, Text: text, Server: h.Accounts.Settings().ServerName, Messages: text.messages()}
	link, _, item, failed := h.openTitle(r)
	status := http.StatusOK
	if failed != nil {
		status = failed.status
		data.Message = text.refusal(failed.code)
	} else {
		title := &shareTitleData{Name: item.Name, Overview: item.Overview, Year: item.ProductionYear, Minutes: int(item.Runtime.Minutes()),
			Expires: link.ExpiresAt}
		if item.Kind == library.KindEpisode {
			title.Name = item.SeriesName
			title.Subtitle = text.episode(item)
			title.Year = 0
		}
		data.Title = title
	}
	header := w.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Cache-Control", "no-store")
	// hls.js runs its worker from a blob: address, and plays from blob:
	// addresses through Media Source Extensions.
	header.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; media-src 'self' blob:; "+
		"connect-src 'self'; worker-src 'self' blob:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	w.WriteHeader(status)
	if err := sharePage.Execute(w, data); err != nil && r.Context().Err() == nil {
		h.Logger.Warn("The page of a share link could not be written", "error", err)
	}
}

// shareImage serves the title's artwork, as its image routes would.
func (h *Handler) shareImage(w http.ResponseWriter, r *http.Request) {
	imageType, width := "", ""
	switch r.PathValue("kind") {
	case "primary":
		imageType, width = "Primary", "480"
	case "backdrop":
		imageType, width = "Backdrop", "1600"
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_, _, item, failed := h.openTitle(r)
	if failed != nil {
		w.WriteHeader(failed.status)
		return
	}
	image := r.Clone(r.Context())
	image.URL = &url.URL{Path: r.URL.Path, RawQuery: url.Values{"maxWidth": {width}}.Encode()}
	image.SetPathValue("itemId", item.ID.String())
	image.SetPathValue("imageType", imageType)
	image.SetPathValue("imageIndex", "")
	h.image(w, image)
}

// startGuest starts a guest's playback of the link's title, unless the
// link has as many guests as it allows, the sharer plays on as many
// devices as they may, it is outside their allowed hours, or nothing can
// play: the answer says which.
func (h *Handler) startGuest(w http.ResponseWriter, r *http.Request) {
	link, sharer, item, failed := h.openTitle(r)
	if failed != nil {
		writeShareError(w, *failed)
		return
	}
	if h.outsideHours(sharer) {
		writeShareError(w, shareError{http.StatusForbidden, shareRefusedHours})
		return
	}
	h.endStaleGuests()
	playing, err := h.devicesPlaying(r.Context(), sharer, accounts.ID{})
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	play := &guestPlay{id: "share" + randomHex(16), device: randomID(), link: link, user: sharer.ID, item: item,
		address: forwarded.Client(r), started: h.now(), seen: h.now()}
	// The limits are checked before choosing what plays, which may read
	// the sources, and again as the playback is admitted.
	if most := sharer.MaxPlaybacks; most > 0 && playing+h.guests.ofUser(sharer.ID) >= most {
		writeShareError(w, shareError{http.StatusTooManyRequests, shareRefusedLimit})
		return
	}
	plan, refused := h.planGuest(r, shareUser(sharer), item, play.id)
	if refused != "" {
		status := http.StatusServiceUnavailable
		if refused == shareRefusedUnavailable {
			status = http.StatusNotFound
		}
		writeShareError(w, shareError{status, refused})
		return
	}
	play.version, play.params, play.method = plan.version, plan.params, plan.method
	if refused := h.guests.admit(play, sharer.MaxPlaybacks, playing); refused != "" {
		writeShareError(w, shareError{http.StatusTooManyRequests, refused})
		return
	}
	h.reasons.Put(play.id, plan.reasons)
	ctx := context.WithoutCancel(r.Context())
	if err := h.Accounts.CountSharePlay(ctx, link.ID); err != nil {
		h.Logger.Warn("A playback through a share link could not be counted", "error", err)
	}
	h.Activity.SharedPlayed(ctx, link, play.address)
	if notify, err := h.Accounts.ShareLinkToNotify(ctx, link.ID); err != nil {
		h.Logger.Warn("A playback through a share link could not be noted for its notification", "error", err)
	} else if notify && h.SharePlayed != nil {
		h.SharePlayed(link)
	}
	h.followGuest(*play, plays.ReportStart)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"session": play.id})
}

// devicesPlaying counts the devices of user playing, but except.
func (h *Handler) devicesPlaying(ctx context.Context, user accounts.User, except accounts.ID) (int, error) {
	devices, err := h.Accounts.Devices(ctx, user.ID)
	if err != nil {
		return 0, err
	}
	playing := 0
	for _, device := range devices {
		if _, ok := h.sessions.Playing(device.ID); ok && device.ID != except {
			playing++
		}
	}
	return playing, nil
}

// guestPlan is what a guest plays: a version, and the parameters of its
// remux, with how it plays and why it streams so.
type guestPlan struct {
	version accounts.ID
	params  string
	method  string
	reasons []string
}

// planGuest chooses the version a guest plays and how, as PlaybackInfo
// chooses for an app, for guestProfile: the sharer's version rules, quality
// group (at most 1080p), Maximum quality and permissions apply. No
// subtitle is chosen: the page offers the text ones. It returns why
// nothing plays, empty when something does.
func (h *Handler) planGuest(r *http.Request, user accounts.User, item library.Item, session string) (guestPlan, string) {
	ctx := r.Context()
	request := playbackInfoRequest{DeviceProfile: &guestProfile, EnableDirectPlay: new(false)}
	limitBitrate(&request, user)
	h.limitHeight(&request, user)
	p, complete, err := h.playableToPlay(ctx, user, item)
	if err != nil {
		return guestPlan{}, shareRefusedUnavailable
	}
	p.tracks.subtitleMode = "None"
	settings := h.Accounts.Settings()
	candidatesOf := func(versions []library.Version) candidates {
		c := candidates{order: make([]int, len(versions)), analyzed: settings.VersionAttempts, named: -1}
		for i := range versions {
			c.order[i] = i
		}
		return c
	}
	choice := h.choose(r, user, p, item.ID, request, candidatesOf(p.versions), settings.PreferDirectPlay)
	if choice.decided == nil && !complete && ctx.Err() == nil {
		versions, err := h.Library.Versions(ctx, user, item.ID)
		if err != nil {
			return guestPlan{}, shareRefusedUnavailable
		}
		p.versions = h.offered(ctx, user, versions)
		choice = h.choose(r, user, p, item.ID, request, candidatesOf(p.versions), settings.PreferDirectPlay)
	}
	chosen := choice.decided
	if chosen == nil || !chosen.decision.HLS {
		// At the server's limit of video conversions, a version whose
		// video must be converted is not chosen: the guest may try again.
		if slices.ContainsFunc(p.versions, func(v library.Version) bool { return !h.Playback.MayConvert(user.ID, v.ID) }) {
			return guestPlan{}, shareRefusedBusy
		}
		return guestPlan{}, shareRefusedUnavailable
	}
	source := h.decidedSource(r, p, *chosen, request, session)
	_, params, found := strings.Cut(source.TranscodingUrl, "?")
	if !found {
		return guestPlan{}, shareRefusedUnavailable
	}
	h.Playback.Warm(chosen.version, 0)
	method := "DirectStream"
	if chosen.decision.Video != nil || chosen.decision.Audio != nil {
		method = "Transcode"
	}
	return guestPlan{version: chosen.version.ID, params: params, method: method, reasons: chosen.decision.Reasons}, ""
}

// openGuest reads the guest's playback the request is about, through the
// request's link, while the link opens its title and the sharer's allowed
// hours allow it; a playback refused so ends.
func (h *Handler) openGuest(r *http.Request) (guestPlay, accounts.User, *shareError) {
	link, sharer, failed := h.openShare(r)
	if failed != nil {
		h.endGuests(func(play *guestPlay) bool { return play.link.ID != link.ID })
		return guestPlay{}, sharer, failed
	}
	session := r.PathValue("session")
	play, ok := h.guests.get(session, link.ID, h.now())
	if !ok {
		return guestPlay{}, sharer, &shareError{http.StatusNotFound, "ended"}
	}
	if h.outsideHours(sharer) {
		h.endGuests(func(other *guestPlay) bool { return other.id != session })
		return guestPlay{}, sharer, &shareError{http.StatusForbidden, shareRefusedHours}
	}
	return play, sharer, nil
}

// guestRemuxer reads the remux of a guest's playback from the parameters
// it keeps, whatever the request's query says.
func (h *Handler) guestRemuxer(play guestPlay, sharer accounts.User) remuxer {
	return func(w http.ResponseWriter, r *http.Request) (remuxRequest, bool) {
		params := r.Clone(r.Context())
		params.URL = &url.URL{Path: r.URL.Path, RawQuery: play.params}
		req, ok := h.remuxFor(w, params, shareUser(sharer), play.item.ID, play.version, play.id)
		req.uris, req.renditions = "", true
		return req, ok
	}
}

// guestPlaylist serves the master and media playlists of a guest's
// playback.
func (h *Handler) guestPlaylist(w http.ResponseWriter, r *http.Request) {
	name, ok := strings.CutSuffix(r.PathValue("file"), ".m3u8")
	if !ok || name != "master" && name != "main" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	play, sharer, failed := h.openGuest(r)
	if failed != nil {
		writeShareError(w, *failed)
		return
	}
	h.servePlaylist(w, r, name, h.guestRemuxer(play, sharer))
}

// guestSegment serves the segments and subtitles of a guest's playback.
func (h *Handler) guestSegment(w http.ResponseWriter, r *http.Request) {
	play, sharer, failed := h.openGuest(r)
	if failed != nil {
		writeShareError(w, *failed)
		return
	}
	h.serveSegment(w, r, h.guestRemuxer(play, sharer))
}

// maxGuestReport bounds the body of a guest's report.
const maxGuestReport = 1 << 10

// guestProgress notes where a guest's playback is, and whether it is
// paused, for Home and the statistics; the sharer's watch state is left
// alone.
func (h *Handler) guestProgress(w http.ResponseWriter, r *http.Request) {
	var report struct {
		Position float64 `json:"position"`
		Paused   bool    `json:"paused"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxGuestReport)).Decode(&report); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	play, _, failed := h.openGuest(r)
	if failed != nil {
		writeShareError(w, *failed)
		return
	}
	h.guests.progress(play.id, time.Duration(report.Position*float64(time.Second)), report.Paused)
	play.position, play.paused = time.Duration(report.Position*float64(time.Second)), report.Paused
	h.followGuest(play, plays.ReportProgress)
	w.WriteHeader(http.StatusNoContent)
}

// stopGuest ends a guest's playback.
func (h *Handler) stopGuest(w http.ResponseWriter, r *http.Request) {
	play, _, failed := h.openGuest(r)
	if failed != nil {
		writeShareError(w, *failed)
		return
	}
	h.endGuests(func(other *guestPlay) bool { return other.id != play.id })
	w.WriteHeader(http.StatusNoContent)
}

// guestSessions describes the guests' playbacks for the admin dashboard,
// under their sharers' names.
func (h *Handler) guestSessions(ctx context.Context, encodings map[string][]hls.Running) []LiveSession {
	var sessions []LiveSession
	for _, play := range h.guests.all() {
		user, err := h.Accounts.User(ctx, play.user)
		if err != nil {
			continue
		}
		now := h.now()
		position := play.position
		if !play.paused {
			position += now.Sub(play.seen)
		}
		session := LiveSession{User: shareUser(user), Shared: true,
			Device: accounts.Device{ID: play.device, UserID: play.user,
				DeviceInfo: accounts.DeviceInfo{DeviceName: guestDevice, Client: guestApp, RemoteAddress: play.address}},
			Playing: playback.NowPlaying{PlayState: playback.PlayState{Item: play.item.ID, MediaSourceID: play.version.String(),
				PlaySessionID: play.id, PlayMethod: play.method, Position: position, Paused: play.paused}, User: play.user,
				Started: play.started, CheckedIn: play.seen},
			Item: play.item, Found: true, Encodings: encodings[play.id]}
		session.Reasons, _ = h.reasons.Get(play.id)
		if size, ok := h.sizes.Get(play.id); ok {
			session.Size = &size
		}
		if analysis, ok := h.Playback.Analyzed(ctx, play.version); ok {
			session.Source = &analysis
		}
		sessions = append(sessions, session)
	}
	return sessions
}

// randomHex is n random bytes in hexadecimal.
func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// randomID is a random identifier.
func randomID() accounts.ID {
	var id accounts.ID
	_, _ = rand.Read(id[:])
	return id
}

// mayShare answers GET /Polyfin/Items/{itemId}/Share: whether the caller
// may give the title through a share link, for the web client's script,
// which offers Share then (see internal/server/webclient.js), and the
// admin app's address that makes the link, set only when they may.
func (h *Handler) mayShare(w http.ResponseWriter, r *http.Request) {
	id, ok := parseGUID(r.PathValue("itemId"))
	if !ok {
		validationProblem(w, map[string][]string{"itemId": {notValid(r.PathValue("itemId"))}})
		return
	}
	user := callerFrom(r.Context()).User
	answer := struct {
		Shareable bool
		Url       *string
	}{}
	if user.ID != (accounts.ID{}) && h.Accounts.MayShare(user) {
		if item, err := h.Library.Shareable(r.Context(), user, id); err == nil {
			answer.Shareable = true
			answer.Url = new("/admin/me/account?" + url.Values{"share": {item.ID.String()}}.Encode() + "#shared-links")
		}
	}
	writeJSON(w, http.StatusOK, answer)
}
