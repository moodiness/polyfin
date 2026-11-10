package jellyfin

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
)

// With Settings › Playback › Version names on Short, Polyfin writes the
// names of a movie's or an episode's versions itself, the same for every
// app, from what it knows of each (see library.VersionBadges): its
// badges, its size and its addon, such as "4K · DV · HEVC · Atmos 7.1 ·
// IMAX · FR EN · 58 GB · Addon". The names follow the server language,
// and change as a version is analyzed. Rules on version names still read
// the addon's own (see versionRules).

// versionNames are the short names of a title's versions listed
// together, by version, when the setting has them short; nil otherwise,
// and for an item other than a movie or an episode. Two versions listed
// together never share a name: the later of those that would ends with
// " · 2", " · 3" and so on, as the web player tells versions apart by
// name.
func (h *Handler) versionNames(ctx context.Context, item library.Item, versions []library.Version) map[accounts.ID]string {
	settings := h.Accounts.Settings()
	if settings.VersionNames != "short" || item.Kind != library.KindMovie && item.Kind != library.KindEpisode || len(versions) == 0 {
		return nil
	}
	names := make(map[accounts.ID]string, len(versions))
	taken := make(map[string]bool, len(versions))
	for _, version := range versions {
		known := h.knownAnalyses(ctx, version)
		size := version.Size
		if len(known) > 0 && known[0].Size > 0 {
			size = known[0].Size
		}
		name := shortVersionName(library.VersionBadges(version, settings.Language, known...), size, version.Addon, settings.Language)
		unique := name
		for n := 2; taken[unique]; n++ {
			unique = name + " · " + strconv.Itoa(n)
		}
		taken[unique] = true
		names[version.ID] = unique
	}
	return names
}

// shortVersionName writes a version's short name from its badges, its size in
// bytes, 0 when unknown, and its addon's name, in language: its badges
// apart, but its audio format with its channels ("Atmos 7.1") and its
// languages together ("FR EN"), then its size and its addon, those
// unknown left out.
func shortVersionName(badges []library.Badge, size int64, addon, language string) string {
	var parts []string
	previous := ""
	for _, badge := range badges {
		joined := badge.Kind == library.BadgeChannels && previous == library.BadgeAudio ||
			badge.Kind == library.BadgeLanguage && previous == library.BadgeLanguage
		if joined {
			parts[len(parts)-1] += " " + badge.Text
		} else {
			parts = append(parts, badge.Text)
		}
		previous = badge.Kind
	}
	if size > 0 {
		parts = append(parts, sizeName(size, language))
	}
	if addon = strings.TrimSpace(addon); addon != "" {
		parts = append(parts, addon)
	}
	return strings.Join(parts, " · ")
}

// sizeName writes a size in bytes as language writes it, in decimal units:
// whole gigabytes from 10 GB, gigabytes with one decimal from 1 GB,
// megabytes below; a decimal comma but in English, and "Go" and "Mo" in
// French.
func sizeName(size int64, language string) string {
	unit, value, decimals := "GB", float64(size)/1e9, 0
	switch {
	case value < 1:
		unit, value = "MB", math.Max(1, float64(size)/1e6)
	case value < 10:
		decimals = 1
	}
	text := strconv.FormatFloat(value, 'f', decimals, 64)
	if language != "en" {
		text = strings.Replace(text, ".", ",", 1)
	}
	if language == "fr" {
		unit = strings.Replace(unit, "B", "o", 1)
	}
	return text + " " + unit
}

// With Settings › Playback › Say in version names how they play, the
// names of a movie's or an episode's versions end with how the app asking
// plays each (see playback.PlayNote): PlaybackInfo names every version it
// lists for the device profile it is sent; item details, where apps read
// names but which know no profile, name them for the device profile known
// for the device asking (see knownPlay), kept in memory until the device
// signs out or the server stops. Only versions whose tracks are known,
// analyzed or described by RemuxDB, are named: naming reads nothing.

// devicePlay is what a device told of how it plays: the device profile
// and the bitrate its last PlaybackInfo asked for, else the profile its
// session's capabilities carried, which fromCapabilities tells.
type devicePlay struct {
	profile          *playback.DeviceProfile
	bitrate          looseInt
	fromCapabilities bool
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

// rememberCapabilities keeps the device profile a session's capabilities
// carry, as Jellyfin's ClientCapabilitiesDto may, for the device asking,
// until its next PlaybackInfo: the profile a PlaybackInfo sent, which the
// app plays with, is kept over it.
func (h *Handler) rememberCapabilities(ctx context.Context, profile *playback.DeviceProfile) {
	c := callerFrom(ctx)
	if profile == nil || c.APIKey != nil || c.Device.ID == (accounts.ID{}) {
		return
	}
	if known, ok := h.plays.Get(c.Device.ID); ok && !known.fromCapabilities {
		return
	}
	h.plays.Put(c.Device.ID, devicePlay{profile: profile, fromCapabilities: true})
}

// knownPlay returns the request a PlaybackInfo of the device asking would
// make, from its device profile known (see devicePlay), limited for user as
// PlaybackInfo limits it, and the device; false when the caller is no
// device, or the device has told no profile since the server started.
func (h *Handler) knownPlay(ctx context.Context, user accounts.User) (playbackInfoRequest, accounts.ID, bool) {
	c := callerFrom(ctx)
	if c.APIKey != nil || c.Device.ID == (accounts.ID{}) {
		return playbackInfoRequest{}, accounts.ID{}, false
	}
	play, ok := h.plays.Get(c.Device.ID)
	if !ok {
		return playbackInfoRequest{}, accounts.ID{}, false
	}
	request := playbackInfoRequest{DeviceProfile: play.profile, MaxStreamingBitrate: play.bitrate}
	limitBitrate(&request, user)
	h.limitHeight(&request, user)
	request.convertSound = user.ConvertSound
	return request, c.Device.ID, true
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
// item details or a listing describe, for the device profile known for the
// device asking, with its user's limits; nil when the setting is off, or
// the device has told no profile since the server started.
func (h *Handler) detailsNamer(r *http.Request, item library.Item) *versionNamer {
	if item.Kind != library.KindMovie && item.Kind != library.KindEpisode {
		return nil
	}
	c := callerFrom(r.Context())
	request, _, ok := h.knownPlay(r.Context(), c.User)
	if !ok {
		return nil
	}

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

// playName is a version's name ending with how n's app plays it (see
// listedDecision).
func (h *Handler) playName(n *versionNamer, p playable, version library.Version, streams []playback.MediaStream, analysis media.Analysis) string {
	decision, playable := h.listedDecision(p, version, streams, analysis, n.request, n.allowed)
	return p.name(version) + " " + playback.PlayNote(decision, playable, n.language)
}

// listedDecision decides how an app plays a version, for its request,
// from its tracks, streams as described from its analysis or RemuxDB's
// description, with the tracks the user's preferences choose, reading
// nothing (see trackDecision), and whether it plays at all. Over HLS, an
// MPEG-TS file, an MPEG program stream or an AVI file whose keyframes are
// not known has its video converted, as decide plays it (see
// playback.Service.GridPlanned); another file whose index cannot be read
// is found out only once played: a listing cannot tell it before.
func (h *Handler) listedDecision(p playable, version library.Version, streams []playback.MediaStream, analysis media.Analysis,
	request playbackInfoRequest, allowed accounts.Conversions) (playback.Decision, bool) {
	decision, options, described := h.trackDecision(p, version, streams, analysis, request, allowed, false)
	playable := true
	if decision.HLS && decision.Video == nil && h.Playback.GridPlanned(version, analysis) {
		decision, playable = gridDecision(request, described, options)
	}
	return decision, playable && !unplayable(request, analysis, decision, allowed)
}
