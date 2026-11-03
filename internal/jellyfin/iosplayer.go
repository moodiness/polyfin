package jellyfin

import (
	"net/http"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/playback"
)

// iosApps are the client names of the Jellyfin iOS app, on an iPhone and
// on an iPad: "Jellyfin" and the system's name (utils/Device.ts,
// getAppName, at v1.8.0.5). Its web view runs jellyfin-web under the same
// names, with jellyfin-web's own profile.
var iosApps = []string{"Jellyfin iOS", "Jellyfin iPadOS"}

// iosNativeProfiles are the names of the device profiles of the Jellyfin
// iOS app's native player ("Use Native Video Player"), by iOS version and
// whether "Prefer fMP4 in HLS" is on (utils/profiles/ios-11.ts, ios-13.ts
// and ios-13-fmp4.ts at v1.8.0.5; the app needs iOS 12.4). Its downloads
// send the same profile named "Download Profile" instead
// (features/downloads/utils/profile.ts), without its HLS profiles.
var iosNativeProfiles = []string{"iOS 11+ Native Profile", "iOS 13+ Native Profile", "iOS 13+ Native Profile + fMP4"}

// iosNativePlayer reports whether a PlaybackInfo request comes from the
// Jellyfin iOS app's native player: the device profile is one of its
// names, and the request's client is the app. Both are required, as for
// the Android app's integrated player (see integratedPlayer).
func iosNativePlayer(r *http.Request, profile *playback.DeviceProfile) bool {
	if profile == nil || !slices.Contains(iosNativeProfiles, profile.Name) {
		return false
	}
	client := readCredentials(r, true).Client
	return slices.ContainsFunc(iosApps, func(app string) bool { return strings.EqualFold(client, app) })
}

// iosNativeOverHLS returns the native player's profile with its video
// profiles streaming one file turned into HLS profiles, in place.
//
// That player is AVPlayer (expo-av's Video). Its profile lists HLS in
// MPEG-TS, or fragmented MP4 with "Prefer fMP4 in HLS", taking AAC, MP3,
// FLAC and ALAC, and after it a progressive MP4 (Protocol http, Context
// Streaming) taking H.264 with AAC, MP3, AC-3, E-AC-3, FLAC and ALAC.
// Jellyfin 12.4 ranks transcoding profiles without regard to protocol
// (StreamBuilder.cs, GetVideoTranscodeProfile, lines 874 to 927), so a
// source whose audio is E-AC-3 gets that progressive MP4, the E-AC-3
// copied. AVPlayer cannot play it: it refuses a file without byte ranges,
// as Jellyfin serves a conversion (Accept-Ranges none, FileStreamResponse
// Helpers.cs, GetTranscodedFile, line 135), with "The server is not
// correctly configured"; and with byte ranges it reads every fragment of
// the file before it starts, so a conversion made as it plays starts only
// once the whole title is converted.
//
// Polyfin ranks as Jellyfin does, and sends what Jellyfin's progressive
// MP4 would carry over HLS instead, in the container of the app's own
// HLS profile, which carries E-AC-3 either way: the HLS profile made of
// the progressive one keeps its place, codecs and conditions, and takes
// the container, minimum segments and keyframe breaks of the app's first
// HLS video profile. It thus wins only where Jellyfin's progressive MP4
// would: where it keeps audio, or video, the app's HLS profile would
// convert. The native player's plays never stream one file, even with
// Converted downloads on; its downloads, under another profile name,
// are left as they are. The profile sent is not modified.
func iosNativeOverHLS(profile *playback.DeviceProfile) *playback.DeviceProfile {
	hls := playback.TranscodingProfile{Container: "ts"}
	for _, t := range profile.TranscodingProfiles {
		if strings.EqualFold(t.Type, "Video") && strings.EqualFold(t.Protocol, "hls") && streamingContext(t) {
			hls = t
			break
		}
	}
	out := *profile
	out.TranscodingProfiles = slices.Clone(profile.TranscodingProfiles)
	for i := range out.TranscodingProfiles {
		t := &out.TranscodingProfiles[i]
		if !strings.EqualFold(t.Type, "Video") || !strings.EqualFold(t.Protocol, "http") || !streamingContext(*t) {
			continue
		}
		t.Protocol, t.Container, t.MinSegments, t.BreakOnNonKeyFrames = "hls", hls.Container, hls.MinSegments, hls.BreakOnNonKeyFrames
	}
	return &out
}

// streamingContext reports whether t is for streaming, as a profile naming
// no context is: Polyfin decides plays with those alone.
func streamingContext(t playback.TranscodingProfile) bool {
	return t.Context == "" || strings.EqualFold(t.Context, "Streaming")
}
