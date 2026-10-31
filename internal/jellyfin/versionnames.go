package jellyfin

import (
	"context"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
)

// With Settings › Playback › Say in version names how they play, the
// names of a movie's or an episode's versions end with how the app asking
// plays each (see playback.PlayNote): PlaybackInfo names every version it
// lists for the device profile it is sent; item details, where apps read
// names but which know no profile, name them for the last PlaybackInfo
// profile of the device asking, kept in memory until the device signs out
// or the server stops. Only versions whose tracks are known, analyzed or
// described by RemuxDB, are named: naming reads nothing.

// devicePlay is what a device's last PlaybackInfo told of how it plays:
// its device profile and the bitrate it asked for.
type devicePlay struct {
	profile *playback.DeviceProfile
	bitrate looseInt
}

// rememberPlay keeps the device profile and the bitrate of a PlaybackInfo
// for the device asking, as the app sent them.
func (h *Handler) rememberPlay(ctx context.Context, request playbackInfoRequest) {
	c := callerFrom(ctx)
	if request.DeviceProfile == nil || c.APIKey != nil || c.Device.ID == (accounts.ID{}) {
		return
	}
	h.plays.Put(c.Device.ID, devicePlay{profile: request.DeviceProfile, bitrate: request.MaxStreamingBitrate})
}

// versionNamer names versions for an app: its request, as PlaybackInfo
// limits it for the user, the conversions the user may have, and the
// server language names are in.
type versionNamer struct {
	request  playbackInfoRequest
	allowed  accounts.Conversions
	language string
}

// detailsNamer names the versions of item, a movie or an episode, that
// item details or a listing describe, for the last PlaybackInfo of the
// device asking, with its user's limits; nil when the setting is off, or
// the device has not played since the server started.
func (h *Handler) detailsNamer(r *http.Request, item library.Item) *versionNamer {
	c := callerFrom(r.Context())
	if item.Kind != library.KindMovie && item.Kind != library.KindEpisode || c.APIKey != nil {
		return nil
	}
	play, ok := h.plays.Get(c.Device.ID)
	if !ok {
		return nil
	}
	request := playbackInfoRequest{DeviceProfile: play.profile, MaxStreamingBitrate: play.bitrate}
	limitBitrate(&request, c.User)
	h.limitHeight(&request, c.User)
	return h.playbackNamer(item, request, c.User)
}

// playbackNamer names the versions of item a PlaybackInfo lists for the
// profile it was sent, request being limited for user already; nil when the
// setting is off, the app sent no profile, or item is neither a movie nor
// an episode.
func (h *Handler) playbackNamer(item library.Item, request playbackInfoRequest, user accounts.User) *versionNamer {
	settings := h.Accounts.Settings()
	if !settings.NameHowVersionsPlay || request.DeviceProfile == nil || item.Kind != library.KindMovie && item.Kind != library.KindEpisode {
		return nil
	}
	return &versionNamer{request: request, allowed: h.Accounts.Conversions(user), language: settings.Language}
}

// playName is a version's name ending with how n's app plays it, decided
// from its tracks, streams as described from its analysis or RemuxDB's
// description, with the tracks the user's preferences choose, reading
// nothing (see trackDecision). Over HLS, an MPEG-TS file whose keyframes
// are not known has its video converted, as decide plays it (see
// playback.Service.GridPlanned); another file whose index cannot be read
// is found out only once played: its name cannot tell it before.
func (h *Handler) playName(n *versionNamer, p playable, version library.Version, streams []playback.MediaStream, analysis media.Analysis) string {
	decision, options, described := h.trackDecision(p, version, streams, analysis, n.request, n.allowed, false)
	playable := true
	if decision.HLS && decision.Video == nil && h.Playback.GridPlanned(version, analysis) {
		decision, playable = gridDecision(n.request, described, options)
	}
	playable = playable && !unplayable(n.request, analysis, decision, n.allowed)
	return version.Name + " " + playback.PlayNote(decision, playable, n.language)
}
