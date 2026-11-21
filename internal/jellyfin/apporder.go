package jellyfin

import (
	"context"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/forwarded"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/playback"
)

// With Settings › Playback › Prefer versions the app plays without
// conversion, a title's versions are ordered for the app asking when its
// device profile is known (see knownPlay): the versions Polyfin knows
// cost the app's play least (see playCost), among those in the user's
// languages as well as the first version of the order known (see
// playedLanguages), come before the others, after the other steps of the
// order but Play local files first, whose local files stay first for its
// users (see ordered). Item details, PlaybackInfo and every request
// naming the title's own identifier for its first version follow it, so
// that the version menu shows first the version Play prefers, and the
// identifier the app sends for it picks it (see playbackInfo). Polyfin
// knows how a version plays as its name says it (see listedDecision),
// reading nothing: a version neither analyzed nor described by RemuxDB
// ranks with those whose video is converted within the app's bitrate
// cap. Without the profile, before a device's first play since the server
// started, the order is the server's, and Play prefers a version among
// them as it does for a play picking none.

// playCost ranks how a version plays on an app, the cheapest first: as it
// is, then repackaged with its tracks copied, then with its audio alone
// converted, then with its video converted, from a source within the
// app's bitrate cap, then from one above it, which the conversion must
// bring down. A version known not to play on the app, or to play other
// languages than the first known, ranks with the last.
type playCost int

const (
	costDirect playCost = iota
	costCopied
	costAudioConverted
	costVideoConverted
	costOverCap
)

// unknownCost is the rank of a version whose play on the app is not known:
// it may need anything, and comes after those known to need no video
// conversion, before those known to need one above the app's cap.
const unknownCost = costVideoConverted

// playCostOf ranks how decision plays a source of the given bitrate for
// request. A source above the user's bitrate limit plays only with its
// video converted down to it; for a user who has their sound converted,
// every version's audio is, and a version whose video is copied counts as
// copied.
func playCostOf(request playbackInfoRequest, bitrate int64, decision playback.Decision) playCost {
	switch {
	case beyondUserLimit(request, bitrate, decision):
		return costOverCap
	case decision.DirectPlay:
		return costDirect
	case !decision.Streamed():
		return costOverCap
	case decision.Video == nil && (decision.Audio == nil || request.convertSound):
		return costCopied
	case decision.Video == nil:
		return costAudioConverted
	case overAppCap(request, bitrate):
		return costOverCap
	}
	return costVideoConverted
}

// overAppCap reports whether a source's bitrate is known and above the
// most the app takes: its PlaybackInfo's MaxStreamingBitrate, lowered to
// the user's limit (see limitBitrate), else its device profile's.
func overAppCap(request playbackInfoRequest, bitrate int64) bool {
	limit := request.MaxStreamingBitrate.value
	if limit <= 0 && request.DeviceProfile != nil && request.DeviceProfile.MaxStreamingBitrate != nil {
		limit = *request.DeviceProfile.MaxStreamingBitrate
	}
	return limit > 0 && bitrate > limit
}

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

// appPlay is how the app plays a version, as far as Polyfin knows it: at
// which cost, and in which languages.
type appPlay struct {
	cost      playCost
	languages playedLanguages
}

// appPlays tells how the app plays those of item's versions whose
// analysis or RemuxDB's description Polyfin has, with the user's track
// preferences and the subtitle files listed lately: at which cost, last
// for a version it does not play, and the languages of the tracks it
// plays. The others are left out.
func (h *Handler) appPlays(ctx context.Context, a *appOrder, user accounts.User, item library.Item, versions []library.Version) map[accounts.ID]appPlay {
	p := playable{item: item, tracks: h.titleTracks(ctx, user, item)}
	p.subtitles, _ = h.keptSubtitles(user, item.ID)
	result := make(map[accounts.ID]appPlay, len(versions))
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
		cost := costOverCap
		if plays {
			cost = playCostOf(a.request, analysis.Bitrate, decision)
		}
		result[version.ID] = appPlay{cost: cost, languages: languagesPlayed(p.tracks, streams, decision)}
	}
	return result
}

// playedLanguages are the languages of the tracks a version plays for the
// user: its audio track's, ranked for their preferred language and region
// (see trackPreferences.audioRank), and its subtitle's when one shows.
//
// Prefer versions the app plays without conversion never trades them for
// a conversion saved: a version cheaper for the app to play comes before
// the one the order would play only when its languages are as good (see
// asGoodAs). The user's languages, the server's default ones and the
// choices remembered on the title thus come first, then the cheapest play.
type playedLanguages struct {
	audio     string
	audioRank int
	subtitled bool
	subtitle  string
	forced    bool
}

// languagesPlayed returns the languages of the tracks decision plays among
// streams, with the user's track preferences.
func languagesPlayed(preferences trackPreferences, streams []playback.MediaStream, decision playback.Decision) playedLanguages {
	var played playedLanguages
	for _, stream := range streams {
		switch {
		case stream.Type == "Audio" && stream.Index == decision.AudioStreamIndex:
			played.audio, played.audioRank = stream.Language, preferences.audioRank(stream)
		case stream.Type == "Subtitle" && stream.Index == decision.SubtitleStreamIndex:
			played.subtitled, played.subtitle, played.forced = true, stream.Language, stream.IsForced
		}
	}
	return played
}

// asGoodAs reports whether a version playing l plays the user's languages
// as well as one playing first: its audio is in a language they prefer
// more, or in first's language, and, when first shows a subtitle, it
// shows one too, in its language and as forced or full (see keeps).
func (l playedLanguages) asGoodAs(first playedLanguages) bool {
	switch {
	case l.audioRank > first.audioRank:
		return true
	case l.audioRank < first.audioRank || !keeps(l.audio, first.audio):
		return false
	case !first.subtitled:
		return true
	}
	return l.subtitled && l.forced == first.forced && keeps(l.subtitle, first.subtitle)
}

// keeps reports whether a track in language keeps a track in first's: the
// same language, or any when first states none, which tells nothing of
// what the user would get. A track of no stated language keeps only
// another of none: a version labeled MULTI whose tracks state none may
// play another language than first's.
func keeps(language, first string) bool {
	switch {
	case localization.Undetermined(first):
		return true
	case localization.Undetermined(language):
		return false
	}
	return localization.SameLanguage(language, first)
}
