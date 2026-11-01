package jellyfin

import (
	"context"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/forwarded"
	"github.com/moodiness/polyfin/internal/library"
)

// With Settings › Playback › Prefer versions the app plays without
// conversion, a title's versions are ordered for the app asking when its
// device profile is known (see knownPlay): the versions Polyfin knows the
// app plays without conversion, as they are or repackaged with their
// tracks copied, come before the others, after the other steps of the
// order but Play local files first, whose local files stay first for its
// users (see ordered). Item details, PlaybackInfo and every request naming
// the title's own identifier for its first version follow it, so that the
// version menu shows first the version Play prefers, and the identifier
// the app sends for it picks it (see playbackInfo). Polyfin knows how a
// version plays as its name says it (see listedDecision), reading nothing:
// a version neither analyzed nor described by RemuxDB stays among those
// that need a conversion. Without the profile, before a device's first
// play since the server started, the order is the server's, and Play
// prefers a version among them as it does for a play picking none.

// appOrder orders versions for an app: its device, the request a
// PlaybackInfo of it would make, limited for the user, and the conversions
// the user may have.
type appOrder struct {
	device  accounts.ID
	request playbackInfoRequest
	allowed accounts.Conversions
}

// appOrderFor returns the order for the app asking, for user; nil when
// Prefer versions the app plays without conversion is off, or the app's
// device profile is not known.
func (h *Handler) appOrderFor(ctx context.Context, user accounts.User) *appOrder {
	if !h.Accounts.Settings().PreferDirectPlay {
		return nil
	}
	request, device, ok := h.knownPlay(ctx, user)
	if !ok {
		return nil
	}
	return &appOrder{device: device, request: request, allowed: h.Accounts.Conversions(user)}
}

// key names the app an order is made for, in the key of an open page's
// order (see orderKey): its device, empty for none. Two apps of a user may
// see a title's versions in two orders.
func (a *appOrder) key() string {
	if a == nil {
		return ""
	}
	return a.device.String()
}

// mediaContext is the context of a media request, which tells no caller
// (see streamAccess), with the device of the token it carries, if any, for
// user: the title's own identifier then stands for the first version of
// the order made for that app (see firstWorkingVersion), the version its
// menu showed first. A request carrying only a grant names its version.
func (h *Handler) mediaContext(r *http.Request, route mediaRoute, user accounts.User) context.Context {
	ctx := r.Context()
	if callerFrom(ctx).Device.ID != (accounts.ID{}) {
		return ctx
	}
	token := h.mediaToken(r, route)
	if token == "" {
		return ctx
	}
	device, _, err := h.Accounts.DeviceByToken(ctx, token, forwarded.Client(r))
	if err != nil {
		return ctx
	}
	return context.WithValue(ctx, callerKey{}, caller{User: user, Device: device, Token: token})
}

// playsUnconverted tells which of item's versions the app plays without
// conversion, as far as their analysis or RemuxDB's description tells,
// with the user's track preferences and the subtitle files listed lately.
func (h *Handler) playsUnconverted(ctx context.Context, a *appOrder, user accounts.User, item library.Item, versions []library.Version) map[accounts.ID]bool {
	p := playable{item: item, tracks: h.titleTracks(ctx, user, item)}
	p.subtitles, _ = h.keptSubtitles(user, item.ID)
	result := make(map[accounts.ID]bool, len(versions))
	for _, version := range versions {
		analysis, analyzed := h.Playback.Analyzed(ctx, version.ID)
		known := analyzed
		if !known {
			analysis, known = h.RemuxDB.Described(version)
		}
		if !known {
			continue
		}
		streams := h.sourceStreams(ctx, p, version, analysis, true)
		if analyzed {
			streams = h.withTextCopies(ctx, p, version, analysis, streams)
		}
		decision, plays := h.listedDecision(p, version, streams, analysis, a.request, a.allowed)
		result[version.ID] = plays && (decision.DirectPlay || decision.Streamed() && decision.Video == nil && decision.Audio == nil)
	}
	return result
}
