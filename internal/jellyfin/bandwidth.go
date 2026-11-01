package jellyfin

import (
	"cmp"
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/bandwidth"
	"github.com/moodiness/polyfin/internal/forwarded"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/metrics"
)

// The kinds of source a playback reads, as Origin.Kind tells them.
const (
	// OriginAddon: a link a Stremio or Eclipse addon listed.
	OriginAddon = "addon"
	// OriginTorrent: a torrent's file, read from its swarm.
	OriginTorrent = "torrent"
	// OriginLocalFolder: a file of a folder of the server's disks.
	OriginLocalFolder = "localFolder"
	// OriginNetworkShare: a file of an SMB, WebDAV, S3, SFTP or rclone
	// share.
	OriginNetworkShare = "networkShare"
	// OriginIPTV: a channel, or a programme of its archive, from an IPTV
	// source.
	OriginIPTV = "iptv"
)

// Origin is where a playback's bytes come from: Kind, one of the Origin
// constants, empty when not known, as for a recording of the server's;
// Name, the addon, folder or IPTV source as installed; and for a torrent
// the peers it is connected to.
type Origin struct {
	Kind  string
	Name  string
	Peers int
}

// meteredFile counts what a route serving files as they are sends, and
// meteredStream what a route of HLS segments sends (see metered).
func (h *Handler) meteredFile(next http.HandlerFunc) http.HandlerFunc {
	return h.metered(metrics.DeliveryDirectPlay, next)
}

func (h *Handler) meteredStream(next http.HandlerFunc) http.HandlerFunc {
	return h.metered(metrics.DeliveryRemux, next)
}

// metered counts what a media route sends: each write goes to the meter
// of the playback it serves (see meterKey) and to the Prometheus counter
// of its delivery, delivery unless the play session's encodings tell
// another (see sentDelivery).
func (h *Handler) metered(delivery string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		counted := &meteredWriter{ResponseWriter: w, h: h, r: r, delivery: delivery}
		defer counted.release()
		next(counted, r)
	}
}

// meteredDevice counts what w sends for the playback of device, as metered
// does for a file sent as it is: a DLNA device's, whose requests carry no
// token. The caller releases it once the answer is sent.
func (h *Handler) meteredDevice(w http.ResponseWriter, r *http.Request, device accounts.ID) *meteredWriter {
	return &meteredWriter{ResponseWriter: w, h: h, r: r, delivery: metrics.DeliveryDirectPlay, key: device.String()}
}

// meteredWriter counts the bytes of an answer once its status is known:
// those of a success, for its playback and its delivery, and none of an
// error. An answer sending the app to the source marks its playback so.
// It counts in the writes themselves, with atomic additions only: a
// direct play's answer lasts as long as the app reads, and its rate shows
// as it goes.
type meteredWriter struct {
	http.ResponseWriter
	h        *Handler
	r        *http.Request
	delivery string
	// key, when set, is whose playback the answer serves (see meterKey).
	key     string
	started bool
	meter   *bandwidth.Meter
	sent    prometheus.Counter
}

func (w *meteredWriter) WriteHeader(status int) {
	w.start(status)
	w.ResponseWriter.WriteHeader(status)
}

func (w *meteredWriter) Write(p []byte) (int, error) {
	w.start(http.StatusOK)
	n, err := w.ResponseWriter.Write(p)
	if w.meter != nil {
		w.meter.Add(n)
	}
	if w.sent != nil {
		w.sent.Add(float64(n))
	}
	return n, err
}

// Flush sends what is buffered, as the writer wrapped does.
func (w *meteredWriter) Flush() {
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

// Unwrap gives http.ResponseController the writer wrapped.
func (w *meteredWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// start finds, at the answer's status, what it is counted for.
func (w *meteredWriter) start(status int) {
	if w.started {
		return
	}
	w.started = true
	success := status >= 200 && status < 300
	redirected := status >= 300 && status < 400 && w.Header().Get("Location") != ""
	if !success && !redirected {
		return
	}
	if w.h.Sent != nil {
		key := w.key
		if key == "" {
			key = w.h.meterKey(w.r)
		}
		if key != "" {
			// A redirection's own few bytes are not the playback's.
			meter := w.h.Sent.Hold(key)
			meter.Redirect(redirected)
			if success {
				w.meter = meter
			} else {
				w.h.Sent.Release(meter)
			}
		}
	}
	if success {
		session := cmp.Or(w.r.PathValue("session"), query(w.r, "playSessionId"))
		if session == "" && w.key != "" {
			session = w.h.dlnaSession(w.key)
		}
		w.sent = metrics.BytesSent(w.h.sentDelivery(session, w.delivery))
	}
}

// release ends the hold of the answer's meter.
func (w *meteredWriter) release() {
	if w.meter != nil {
		w.h.Sent.Release(w.meter)
	}
}

// meterKey is whose playback a media request serves: the device of the
// token it carries, which LiveSessions finds a device's playback by, else
// its play session, as a guest's or a converted download's; empty when it
// names neither. The token only tells whose bytes these are: streamAccess
// decides what the request gets.
func (h *Handler) meterKey(r *http.Request) string {
	if token := readCredentials(r, true).Token; token != "" {
		if device, _, err := h.Accounts.DeviceByToken(r.Context(), token, forwarded.Client(r)); err == nil {
			return device.ID.String()
		}
	}
	return cmp.Or(r.PathValue("session"), query(r, "playSessionId"))
}

// sentDelivery is how the bytes of a media request of a play session reach
// the app, as the metrics count them: the session's encodings tell, as for
// the playbacks under way (see LiveSession.delivery), else the route's.
func (h *Handler) sentDelivery(session, route string) string {
	if session == "" || h.Playback == nil {
		return route
	}
	delivery := ""
	for _, running := range h.Playback.Encodings() {
		switch {
		case running.Key.Session != session || !running.Opened:
		case running.Video != nil || running.AudioCodec != "":
			return metrics.DeliveryConversion
		default:
			delivery = metrics.DeliveryRemux
		}
	}
	return cmp.Or(delivery, route)
}

// dlnaSession is the play session of the DLNA device of key, as a string,
// that the device read last; empty when it plays nothing.
func (h *Handler) dlnaSession(key string) string {
	session, seen := "", time.Time{}
	for _, play := range h.dlna.all() {
		if play.device.ID.String() == key && play.seen.After(seen) {
			session, seen = play.id, play.seen
		}
	}
	return session
}

// measure tells what a playback moves now: the bytes per second sent to
// its app, by the meters of its device and of its play session, and those
// its version's source sends, and where that source is.
func (h *Handler) measure(ctx context.Context, s *LiveSession) {
	device := ""
	if s.Device.ID != (accounts.ID{}) {
		device = s.Device.ID.String()
	}
	if h.Sent != nil {
		s.Sent = h.Sent.Rate(device, s.Playing.PlaySessionID)
		s.Redirected = s.Sent == 0 && device != "" && h.Sent.Redirected(device)
	}
	version := s.version
	if version == (accounts.ID{}) {
		version, _ = accounts.ParseID(s.Playing.MediaSourceID)
	}
	if version == (accounts.ID{}) || h.Playback == nil {
		return
	}
	s.Received = h.Playback.ReceivedRate(version)
	s.Origin = h.origin(ctx, s, version)
}

// origin tells where the version a session plays is read from: the source
// of the session's user that lists it, as they play it (see
// library.Service.KnownVersionFor), not the addon of another user's
// listing.
func (h *Handler) origin(ctx context.Context, s *LiveSession, id accounts.ID) Origin {
	live := s.Found && slices.Contains([]library.Kind{library.KindChannel, library.KindProgram, library.KindReplay}, s.Item.Kind)
	if h.Library == nil {
		return Origin{}
	}
	version, known := h.Library.KnownVersionFor(ctx, s.User, id)
	if !known {
		if live {
			return Origin{Kind: OriginIPTV}
		}
		return Origin{}
	}
	origin := Origin{Kind: OriginAddon, Name: version.Addon}
	addon, err := h.Library.VersionAddon(ctx, version)
	if err == nil {
		origin.Name = cmp.Or(addon.Manifest.Name, origin.Name)
	}
	switch {
	case version.Torrent != nil:
		origin.Kind = OriginTorrent
		if progress, running := h.Playback.TorrentProgress(version); running {
			origin.Peers = progress.Connected
		}
	case live, err == nil && addon.IPTV():
		origin.Kind = OriginIPTV
	case err == nil && addon.Local():
		// A local folder's location is a path; a share's, an address with
		// its scheme (smb://, https://, s3://, sftp:// or rclone://).
		origin.Kind = OriginLocalFolder
		if strings.Contains(addon.ManifestURL, "://") {
			origin.Kind = OriginNetworkShare
		}
	}
	return origin
}

// sentSoFar is the bytes the meter of key counted, for the playbacks
// Polyfin follows (see plays.Report's Sent); 0 when nothing is counted.
func (h *Handler) sentSoFar(key string) int64 {
	if h.Sent == nil {
		return 0
	}
	return h.Sent.Total(key)
}

// Bandwidth is what every playback moves now, in bytes per second over
// bandwidth.Window: what Polyfin sends to apps, and what it receives from
// sources.
func (h *Handler) Bandwidth() (sent, received float64) {
	if h.Sent != nil {
		sent = h.Sent.TotalRate()
	}
	if h.Playback != nil {
		received = h.Playback.ReceivedTotal()
	}
	return sent, received
}
