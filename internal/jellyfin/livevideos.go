package jellyfin

import (
	"context"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
)

// A video an addon lists among its other titles, such as a sport addon's
// match, may stream with no end: an HLS playlist that does not end, or a
// source that announces no size and gives no duration (see
// playback.Service.Endless). It then plays as a Live TV channel does, from
// its live edge, without a duration or seeking: PlaybackInfo answers an
// infinite stream with no runtime (livePlaybackInfo), its stream is
// relayed or repackaged as it comes (serveChannel, remuxFor), no keyframe
// index is read and no conversion waits for its end. As a channel, it
// keeps no resume point and is never marked played: Jellyfin's live items,
// its LiveTvChannel, support neither. Its playbacks are followed on Home as
// any other.

// liveVideo reports whether item, a video of an addon's other titles,
// plays live: its first version that plays, as PlaybackInfo offers it,
// streams with no end. Telling it may analyze that version as a live
// stream is analyzed, which it then plays as. A video whose versions all
// failed lately plays live when one of them was found live, or failed as
// a live stream does: its PlaybackInfo then tells in the log why it is
// left out (see livePlaybackInfo).
func (h *Handler) liveVideo(ctx context.Context, user accounts.User, item library.Item) bool {
	if !library.AddonVideo(item) {
		return false
	}
	versions, err := h.Library.Versions(ctx, user, item.ID)
	if err != nil {
		return false
	}
	// offered drops versions in place: those it drops are read below.
	if offered := h.offered(ctx, user, slices.Clone(versions)); len(offered) > 0 {
		endless, err := h.Playback.Endless(playback.ForUser(ctx, user.ID), offered[0])
		return err == nil && endless
	}
	return slices.ContainsFunc(versions, func(version library.Version) bool {
		failure := h.Playback.Failure(version.ID)
		if failure == nil {
			return false
		}
		if analysis, ok := h.Playback.Analyzed(ctx, version.ID); ok {
			return playback.Endless(analysis)
		}
		return playback.LiveFailure(failure) != ""
	})
}

// playsLive reports whether item plays version live: a channel, or a video
// of an addon's other titles whose version was found to stream with no end
// (see liveVideo).
func (h *Handler) playsLive(ctx context.Context, item library.Item, version library.Version) bool {
	if item.Kind == library.KindChannel {
		return true
	}
	if !library.AddonVideo(item) {
		return false
	}
	analysis, ok := h.Playback.Analyzed(ctx, version.ID)
	return ok && playback.Endless(analysis)
}

// reportedLive reports whether a playback report of item, whose media
// source is mediaSource, is of a video that plays live (see playsLive),
// which keeps no user data, as channels keep none.
func (h *Handler) reportedLive(ctx context.Context, user accounts.User, item library.Item, mediaSource string) bool {
	if !library.AddonVideo(item) {
		return false
	}
	id, ok := parseGUID(mediaSource)
	if !ok {
		return false
	}
	if id == item.ID {
		id = h.firstWorkingVersion(ctx, user, item)
	}
	version, err := h.version(ctx, user, item, id)
	return err == nil && h.playsLive(ctx, item, version)
}
