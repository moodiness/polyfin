package jellyfin

import (
	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/playback"
)

// jellyfin-web offers its Quality menu during a play only when the media
// source says SupportsTranscoding (jellyfin-web 12.4,
// apps/legacy/controllers/playback/video/index.js line 973, and
// components/playback/playersettingsmenu.js line 206). Picking a quality
// asks PlaybackInfo again with that MaxStreamingBitrate, which converts a
// source above it. Jellyfin 12.4 says SupportsTranscoding whenever the
// app's profile has a transcoding profile for streaming, a source played
// as it is included, unless the user may convert nothing
// (MediaInfoHelper.SetDeviceSpecificData, lines 285-305), and gives a
// TranscodingUrl only to a source not played as it is (line 322).
// Polyfin says it of a source played as it is when a lower quality would
// play converted (see lowerable).

// lowerable reports whether a source the app plays as it is would play
// with its video converted at a lower quality: the app sent a profile,
// and its video can be converted for the app with the conversions
// allowed, under options with the play as it is ruled out. A lower limit
// converts a source above it, one whose bitrate is not known counting as
// the usual bitrate of its height (see playback.Decide). overHLS tells
// whether that conversion goes out over HLS, which needs the version to
// be cut into segments (see playback.Service.Segmented).
func lowerable(profile *playback.DeviceProfile, source playback.MediaSource, options playback.Options, allowed accounts.Conversions) (ok, overHLS bool) {
	if profile == nil || !allowed.Video {
		return false, false
	}
	options.EnableDirectPlay, options.ConvertVideo = false, true
	converted := playback.Decide(profile, source, options)
	if !converted.Streamed() || converted.Video == nil || !permitted(allowed, converted) {
		return false, false
	}
	return true, converted.HLS
}
