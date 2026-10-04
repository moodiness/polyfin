package jellyfin

import (
	"context"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/playback"
)

// playbackLimitReached reports whether the caller's user already plays on
// as many of their other devices as they may play on at once: the devices
// /Sessions shows playing. The caller's own device does not count, so that
// it can go on to another title.
//
// Jellyfin's MaxActiveSessions counts the user's signed-in sessions and
// refuses signing in once they are reached; Polyfin counts playbacks
// instead, so a user stays signed in on every device and only starting one
// more playback is refused.
func (h *Handler) playbackLimitReached(ctx context.Context, c caller) (bool, error) {
	most := c.User.MaxPlaybacks
	if most <= 0 {
		return false, nil
	}
	devices, err := h.Accounts.Devices(ctx, c.User.ID)
	if err != nil {
		return false, err
	}
	playing := 0
	for _, device := range devices {
		if _, ok := h.sessions.Playing(device.ID); ok && device.ID != c.Device.ID {
			playing++
		}
	}
	return playing >= most, nil
}

// rateLimitExceeded answers a PlaybackInfo refused for the user's limit of
// playbacks at once, with the code jellyfin-web explains as media that
// cannot be played at this time.
func rateLimitExceeded(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, noCompatibleStream{MediaSources: []MediaSourceInfo{}, ErrorCode: "RateLimitExceeded"})
}

// limitBitrate lowers the bitrate limit a PlaybackInfo plays under, the
// app's, else its device profile's, to the user's MaxBitrate, and keeps that
// in userLimit. Jellyfin's RemoteClientBitrateLimit applies to remote
// clients only; Polyfin applies it to all of the user's playback.
func limitBitrate(request *playbackInfoRequest, user accounts.User) {
	limit := int64(user.MaxBitrate)
	if limit <= 0 {
		return
	}
	request.userLimit = limit
	app := request.MaxStreamingBitrate.value
	if app <= 0 && request.DeviceProfile != nil && request.DeviceProfile.MaxStreamingBitrate != nil {
		app = *request.DeviceProfile.MaxStreamingBitrate
	}
	if app <= 0 || app > limit {
		request.MaxStreamingBitrate = looseInt{value: limit, set: true}
	}
}

// beyondUserLimit reports whether a decision would play a source of the
// given bitrate above the user's limit: as it is, or with its video copied.
// Above the limit, only video converted down to it plays.
func beyondUserLimit(request playbackInfoRequest, bitrate int64, decision playback.Decision) bool {
	return overUserLimit(request.userLimit, bitrate) && (!decision.HLS || decision.Video == nil)
}

// overUserLimit reports whether bitrate is known and above limit, a
// user's MaxBitrate, 0 for none.
func overUserLimit(limit, bitrate int64) bool {
	return limit > 0 && bitrate > limit
}

// liveTvAccess refuses Live TV routes to users without Live TV, as
// Jellyfin's LiveTvAccess policy refuses users without EnableLiveTvAccess.
func liveTvAccess(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !callerFrom(r.Context()).User.LiveTv {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// syncPlayNeed is what a SyncPlay route needs of the caller, as Jellyfin's
// SyncPlay policies require it.
type syncPlayNeed int

const (
	// syncPlayAnyAccess: the user may join groups, or is in one (Jellyfin's
	// SyncPlayHasAccess, which every SyncPlay route requires).
	syncPlayAnyAccess syncPlayNeed = iota
	// syncPlayCreate: the user may create groups (SyncPlayCreateGroup).
	syncPlayCreate
	// syncPlayJoinGroups: the user may join groups (SyncPlayJoinGroup).
	syncPlayJoinGroups
	// syncPlayInGroup: the user is in a group (SyncPlayIsInGroup).
	syncPlayInGroup
)

// syncPlayAllowed reports whether user meets need, with what Jellyfin's
// SyncPlay route policies all require on top.
func (h *Handler) syncPlayAllowed(user accounts.User, need syncPlayNeed) bool {
	active := h.syncPlay.isActive(user.ID)
	if !user.SyncPlay.MayJoinGroups() && !active {
		return false
	}
	switch need {
	case syncPlayCreate:
		return user.SyncPlay.MayCreateGroups()
	case syncPlayJoinGroups:
		return user.SyncPlay.MayJoinGroups()
	case syncPlayInGroup:
		return active
	}
	return true
}

// syncPlayAccess refuses a SyncPlay route to a caller who does not meet
// need, with Jellyfin's 403, before the request is read.
func (h *Handler) syncPlayAccess(need syncPlayNeed, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.syncPlayAllowed(callerFrom(r.Context()).User, need) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next(w, r)
	}
}
