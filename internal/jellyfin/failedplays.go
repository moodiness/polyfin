package jellyfin

import (
	"fmt"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/playback"
)

// After a direct play fails, an app may ask again with direct play turned
// off, as the Jellyfin app for Android does after two errors within 30
// seconds (QueueManager.kt, restartPlaybackWithFallback, L224-L255 at
// v2.7.3). Its conversion formats may take less than it played: H.264
// alone there, where it played HEVC. Polyfin then first keeps the video
// copied over HLS, when the device profile of the direct play that failed
// played it as it is (see playback.Options.FailedDirectPlay), and converts
// it only when that fails too: the next retry of the same device for the
// same version, within directPlayMemory, converts it as the app's formats
// ask. A direct play that failed at once, as when the device cannot
// decode the video, has the video converted at once: the copy would fail
// the same way, and that app gives up rather than asking a third time
// after a stream Polyfin converts (QueueManager.kt, L242).

const (
	// failedAtOnce is the time within which a direct play the app asks
	// again for, with direct play off, counts as failed at once: a decoder
	// refusing the video fails within a second or two, where the Jellyfin
	// app for Android waits 8 seconds for bytes it cannot read.
	failedAtOnce = 5 * time.Second
	// directPlayMemory is how long a device's last direct play of a
	// version, and the copy a retry after it sent, are remembered after
	// the last of them.
	directPlayMemory = 10 * time.Minute
)

// playKey is a device's play of a version.
type playKey struct{ device, version accounts.ID }

// directPlayed is the direct play last answered for a device's play of a
// version: the device profile it was decided for, and when; copied tells
// that a retry after it failed was sent the video copied.
type directPlayed struct {
	profile *playback.DeviceProfile
	at      time.Time
	copied  bool
}

// failedDirectPlay returns, for a retry of the device with direct play
// turned off, the device profile of its last direct play of version, which
// the retry keeps the video copied for when it played it as it is; else
// nil and why the video is not kept.
func (h *Handler) failedDirectPlay(device, version accounts.ID) (*playback.DeviceProfile, string) {
	played, ok := h.directPlays.Get(playKey{device, version})
	switch {
	case !ok:
		return nil, "no direct play of the version is known for the device"
	case played.copied:
		return nil, "the video copied after a failed direct play failed too"
	case h.now().Sub(played.at) < failedAtOnce:
		return nil, "the direct play failed at once, as when the device cannot decode the video"
	}
	return played.profile, ""
}

// noteDirectPlay remembers the direct play d answers the device asking
// with, for the version it plays, or the copy a retry after a failed one
// is sent, and tells on the log how such a retry plays.
func (h *Handler) noteDirectPlay(r *http.Request, request playbackInfoRequest, item accounts.ID, d decided) {
	c := callerFrom(r.Context())
	if request.DeviceProfile == nil || c.APIKey != nil || c.Device.ID == (accounts.ID{}) {
		return
	}
	key := playKey{c.Device.ID, d.version.ID}
	decision := d.decision
	switch {
	case decision.DirectPlay:
		h.directPlays.Update(key, func(kept directPlayed, ok bool) (directPlayed, bool) {
			return directPlayed{profile: request.DeviceProfile, at: h.now(), copied: ok && kept.copied}, true
		})
	case !request.directPlayOff || !decision.HLS:
	case decision.KeptVideo:
		h.directPlays.Update(key, func(kept directPlayed, ok bool) (directPlayed, bool) {
			kept.copied = true
			return kept, ok
		})
		audio := "copied"
		if a := decision.Audio; a != nil {
			audio = fmt.Sprintf("%s %d ch", a.Codec, a.Channels)
		}
		h.Logger.Info("After a failed direct play, a version plays over HLS with the video it played kept copied",
			"app", c.Device.Client, "item", item, "addon", d.version.Addon, "segments", decision.Transcoding.Container, "audio", audio)
	case decision.Video != nil:
		why := "the direct play that failed did not take the video as it is, or the retry needs it converted"
		if profile, refused := h.failedDirectPlay(c.Device.ID, d.version.ID); profile == nil {
			why = refused
		}
		h.Logger.Info("After a failed direct play, a version plays over HLS with its video converted", "app", c.Device.Client,
			"item", item, "addon", d.version.Addon, "why", why)
	}
}
