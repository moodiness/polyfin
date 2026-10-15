package jellyfin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/forwarded"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/playback"
)

// Converted downloads (accounts.Settings.ConvertedDownloads): an app
// saving a title for offline viewing asks PlaybackInfo with a device
// profile whose video transcoding profile streams one file (Protocol
// http), and the quality chosen as its bitrate. With the setting on, it
// gets a TranscodingUrl at /videos/{id}/stream.mp4, as from Jellyfin,
// which Polyfin serves as FFmpeg writes it (see playback.Service.
// Progressive): the video copied when it fits the app and the bitrate,
// else converted within the user's quality as any conversion, the audio
// copied when the profile lists its codec, a picture subtitle burned in.
// The file is made on the fly, never kept, so it has no length and no
// byte ranges: an interrupted download starts again. While it runs it
// counts toward the server's video conversions at once, when it converts
// the video, and toward the user's playbacks at once, for its device.

// downloadsConverted reports whether user may download titles converted:
// the setting is on, and the user may download. Converting what needs it
// takes the permission to convert too (see permitted).
func (h *Handler) downloadsConverted(user accounts.User) bool {
	return h.Accounts.Settings().ConvertedDownloads && h.Accounts.MayDownload(user)
}

// downloadPlay is a converted download under way.
type downloadPlay struct {
	session string
	// device is the device downloading, zero when its request named none.
	user, device, item, version accounts.ID
	// method is how it reaches the app, as Jellyfin's PlayMethod: Transcode
	// when it converts, DirectStream when it copies.
	method  string
	started time.Time
}

// downloadPlays are the converted downloads under way.
type downloadPlays struct {
	mu    sync.Mutex
	plays map[*downloadPlay]struct{}
}

func newDownloadPlays() *downloadPlays {
	return &downloadPlays{plays: map[*downloadPlay]struct{}{}}
}

// add records a download under way until the function it returns is
// called.
func (d *downloadPlays) add(play *downloadPlay) func() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.plays[play] = struct{}{}
	return func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		delete(d.plays, play)
	}
}

// onDevice reports whether device downloads.
func (d *downloadPlays) onDevice(device accounts.ID) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for play := range d.plays {
		if play.device == device {
			return true
		}
	}
	return false
}

// withoutDevice counts the downloads of user whose device is not known:
// each counts as a device of its own.
func (d *downloadPlays) withoutDevice(user accounts.ID) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for play := range d.plays {
		if play.user == user && play.device == (accounts.ID{}) {
			n++
		}
	}
	return n
}

// all returns the downloads under way.
func (d *downloadPlays) all() []downloadPlay {
	d.mu.Lock()
	defer d.mu.Unlock()
	all := make([]downloadPlay, 0, len(d.plays))
	for play := range d.plays {
		all = append(all, *play)
	}
	return all
}

// convertedRequest reports whether a request for /Videos/{id}/stream.{ext}
// is a converted download's TranscodingUrl: in a container streamed as one
// file, not static, with a play session and the codec to send. Any other
// gets the version as it is.
func convertedRequest(r *http.Request, extension string) bool {
	if _, ok := hls.ParseFormat(extension); !ok {
		return false
	}
	static, _ := boolQuery(r, "static")
	return !static && query(r, "playSessionId") != "" && query(r, "videoCodec") != ""
}

// convertedDownload serves a converted download: the remux its
// TranscodingUrl describes (see remuxOf), streamed as one file in the
// container of extension, from startTimeTicks, without a length or byte
// ranges. The user must still be allowed to download.
func (h *Handler) convertedDownload(w http.ResponseWriter, r *http.Request, extension string) {
	req, ok := h.remuxOf(w, r)
	if !ok {
		return
	}
	if req.live {
		processingError(w, http.StatusNotFound)
		return
	}
	if !h.Accounts.MayDownload(req.user) {
		h.Logger.Info("A converted download was refused: the user may not download")
		w.WriteHeader(http.StatusForbidden)
		return
	}
	// The play session stands for the user it was signed for: credentials
	// of another user do not make it theirs.
	device, ok := h.downloadDevice(r, req.user)
	if !ok {
		h.Logger.Info("A converted download was refused: its credentials are another user's")
		w.WriteHeader(http.StatusForbidden)
		return
	}
	remux := req.remux
	remux.Format, _ = hls.ParseFormat(extension)
	var start time.Duration
	if ticks, err := strconv.ParseInt(query(r, "startTimeTicks"), 10, 64); err == nil && ticks > 0 {
		start = time.Duration(ticks) * 100
	}
	if reasons := query(r, "transcodeReasons"); reasons != "" {
		h.reasons.Put(remux.Session, strings.Split(reasons, ","))
	}
	if video := remux.ConvertVideo; video != nil {
		h.sizes.Put(remux.Session, video.Size)
	}
	w.Header().Set("Content-Type", mimeTypes[strings.ToLower(extension)])
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		return
	}
	method := "DirectStream"
	if remux.ConvertVideo != nil || remux.ConvertAudio != nil {
		method = "Transcode"
	}
	done := h.downloads.add(&downloadPlay{session: remux.Session, user: req.user.ID, device: device,
		item: req.item.ID, version: remux.Version.ID, method: method, started: h.now()})
	defer done()
	out := &countedWriter{w: w}
	err := h.Playback.Progressive(r.Context(), remux, start, out)
	switch {
	case err == nil || r.Context().Err() != nil:
	case out.n == 0:
		h.remuxError(w, r, remux, err)
	case errors.Is(err, hls.ErrStopped):
	default:
		h.Logger.Warn("A converted download was cut short", "addon", remux.Version.Addon, "error", err)
	}
}

// downloadDevice is the device a converted download's request names by
// its token, zero when it names none; false when the token belongs to
// another user than user.
func (h *Handler) downloadDevice(r *http.Request, user accounts.User) (accounts.ID, bool) {
	token := readCredentials(r, h.Accounts.Settings().LegacyAuthorization).Token
	if token == "" {
		return accounts.ID{}, true
	}
	device, owner, err := h.Accounts.DeviceByToken(r.Context(), token, forwarded.Client(r))
	switch {
	case err != nil:
		return accounts.ID{}, true
	case owner.ID != user.ID:
		return accounts.ID{}, false
	}
	return device.ID, true
}

// countedWriter counts what is written through it.
type countedWriter struct {
	w io.Writer
	n int64
}

func (c *countedWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// downloadSessions describes the converted downloads plays for the admin
// dashboard, as devices playing: their position is how far the file has
// been made. encodings are the encodings under way, by play session.
func (h *Handler) downloadSessions(ctx context.Context, plays []downloadPlay, encodings map[string][]hls.Running) []LiveSession {
	sessions := make([]LiveSession, 0, len(plays))
	for _, play := range plays {
		user, err := h.Accounts.User(ctx, play.user)
		if err != nil {
			continue
		}
		var device accounts.Device
		if play.device != (accounts.ID{}) {
			if device, err = h.Accounts.Device(ctx, play.device); err != nil {
				device = accounts.Device{}
			}
		}
		now := playback.NowPlaying{User: play.user, Started: play.started, CheckedIn: h.now(),
			PlayState: playback.PlayState{Item: play.item, MediaSourceID: play.version.String(), PlaySessionID: play.session, PlayMethod: play.method}}
		session := LiveSession{User: user, Device: device, Download: true, Playing: now, Encodings: encodings[play.session]}
		for _, running := range session.Encodings {
			if running.Progressive {
				session.Playing.Position = max(session.Playing.Position, running.Position)
			}
		}
		session.Reasons, _ = h.reasons.Get(play.session)
		if size, ok := h.sizes.Get(play.session); ok {
			session.Size = &size
		}
		if item, err := h.played(ctx, user, play.item); err == nil {
			session.Item, session.Found = item, true
		}
		if analysis, ok := h.Playback.Analyzed(ctx, play.version); ok {
			session.Source = &analysis
		}
		sessions = append(sessions, session)
	}
	return sessions
}
