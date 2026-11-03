package jellyfin

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
)

// androidApp is the client name of the Jellyfin app for Android, and the
// name of its integrated player's device profile (Constants.kt
// APP_INFO_NAME, DeviceProfileBuilder.kt lines 165 and 166 at v2.7.3).
// Its web view runs jellyfin-web under the same client name, with
// jellyfin-web's own profile, which has no name; its external player's
// profile is named "Jellyfin for Android External Player".
//
// Before v2.7.0 the app was named androidAppBefore27, for its client and
// its profile alike (Constants.kt, line 11, and DeviceProfileBuilder.kt,
// line 140, at v2.6.4).
const (
	androidApp         = "Jellyfin for Android"
	androidAppBefore27 = "Jellyfin Android"
)

// integratedPlayer reports whether a PlaybackInfo request comes from the
// Jellyfin app for Android's integrated player, and whether that player
// sends no token with the address it plays a file from, so that its play
// session must make Polyfin relay the file.
//
// That player reads the Protocol of the source it plays as it is
// (QueueManager.kt, createVideoMediaSource, lines 342 to 367 at v2.7.3):
// for File, it builds /Videos/{id}/stream?static=true with the play
// session, the source's id and its device id, and reads it as one file,
// sending its token in the Authorization header; for Http, it loads the
// source's Path as an HLS playlist, whatever it holds. Polyfin describes
// its versions as Http, its Path being its own stream URL, so every play
// as it is failed there at once, and the app asked again with direct play
// turned off (#838).
//
// The app is told by the request's own Authorization header, not by the
// device's sign-in: an app updated since keeps its token, and the client
// name and version it signed in with.
//
// Versions 2.6.3 to 2.6.4, named androidAppBefore27, play both protocols
// so and send the header too (QueueManager.kt, lines 206 to 229, and
// AppModule.kt, lines 115 to 131, at v2.6.4). Versions 2.4.0 to 2.6.2
// build the same address (MediaQueueManager.kt, lines 146 to 156, at
// v2.4.0; QueueManager.kt, lines 210 to 220, at v2.6.2), but send no
// token with it, in a header or in the address: their data source sends
// a user agent alone (ApplicationModule.kt, line 61, at v2.4.0;
// AppModule.kt, lines 82 to 105, at v2.6.2), and getVideoStreamUrl's
// includeCredentials is false in their SDK, 1.1.0 to 1.4.7. Their play
// session, the only credential that address carries, then asks Polyfin to
// relay (tokenless): a redirect to an addon's link may go from http to
// https, which ExoPlayer's own HTTP stack refuses. The bytes count for
// the device the session was signed for (see mediaDevice). Versions
// before 2.4.0 build the file's address with no play session
// (MediaQueueManager.kt, lines 140 to 149, at v2.3.0), which tells
// Polyfin no user. They keep Http.
func integratedPlayer(r *http.Request, request playbackInfoRequest) (integrated, tokenless bool) {
	if request.DeviceProfile == nil {
		return false, false
	}
	app := readCredentials(r, true)
	switch {
	case request.DeviceProfile.Name == androidApp:
		return strings.EqualFold(app.Client, androidApp), false
	case request.DeviceProfile.Name != androidAppBefore27 || !strings.EqualFold(app.Client, androidAppBefore27):
		return false, false
	case atLeast(app.Version, 2, 6, 3):
		return true, false
	case atLeast(app.Version, 2, 4, 0):
		return true, true
	}
	return false, false
}

// atLeast reports whether version, such as 2.6.3 or 2.6.3-beta.1, is at
// least major.minor.patch. A suffix after the patch is not compared: the
// betas of 2.6.3 send the header as 2.6.3 does.
func atLeast(version string, major, minor, patch int) bool {
	want := []int{major, minor, patch}
	core, _, _ := strings.Cut(version, "-")
	parts := strings.Split(core, ".")
	if len(parts) != len(want) {
		return false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return false
		}
		if n != want[i] {
			return n > want[i]
		}
	}
	return true
}

// asFile describes version to the integrated player as Jellyfin 12.4
// describes a file of its own (BaseItem.cs, line 1231;
// MediaSourceManager.GetPathProtocol, lines 240 to 260, gives Http only
// to http paths): with the File protocol, when the version is one file
// Polyfin serves at /Videos/{id}/stream. A local file, an addon's link, a
// torrent's file and a network share's file are; a disc read from its
// input, an HLS playlist, a DASH manifest and a stream with no end are
// not, and keep Http. Live TV never comes here (see livePlaybackInfo).
// Path stays the signed stream URL: the app does not read it for File.
func (h *Handler) asFile(ctx context.Context, source *MediaSourceInfo, version library.Version) {
	if h.oneFile(ctx, version) {
		source.Protocol = "File"
	}
}

// oneFile reports whether version is one file whose bytes Polyfin serves
// at /Videos/{id}/stream, as far as its name, its address and its
// analysis tell.
func (h *Handler) oneFile(ctx context.Context, version library.Version) bool {
	if h.Playback.ReadsInput(version) || manifestName(version.Filename) {
		return false
	}
	if address, err := url.Parse(version.URL); err == nil && manifestName(address.Path) {
		return false
	}
	analysis, known := h.Playback.Analyzed(ctx, version.ID)
	return !known || !playback.Manifest(analysis) && !playback.Endless(analysis)
}

// manifestName reports whether a file name or path is that of an HLS
// playlist or a DASH manifest.
func manifestName(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".m3u8", ".m3u", ".mpd":
		return true
	}
	return false
}
