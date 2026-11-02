package jellyfin

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/media"
)

// After a failed direct play, the Jellyfin app for Android asks again with
// direct play turned off, and its conversion formats take H.264 alone
// (#838). The retry keeps copied the HEVC its direct play took as it is,
// in MPEG-TS segments, with the E-AC-3 copied too; the next retry, the
// copy having failed as well, converts the video as Jellyfin 12.4 does.
// A direct play that failed at once, as a decoder refusing the video
// does, and one of a device whose profile did not take HEVC, have the
// video converted at once.
func TestRetryAfterAFailedDirectPlayKeepsTheVideoCopied(t *testing.T) {
	t.Parallel()
	if os.Getenv("POLYFIN_TEST_FFMPEG") == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: Polyfin converts with the encoders FFmpeg has")
	}
	p := playing(t)
	// #838's file: HEVC Main in 1080p and E-AC-3 in 5.1, in Matroska.
	analysis := p.remuxable(t)
	analysis.Bitrate = 1_767_650
	analysis.Streams = []media.Stream{
		{Index: 0, Type: "audio", Codec: "eac3", Default: true, Channels: 6, SampleRate: 48000, ChannelLayout: "5.1(side)", Bitrate: 256_000},
		{Index: 1, Type: "video", Codec: "hevc", Profile: "Main", Level: 120, Width: 1920, Height: 1080, FrameRate: 23.976, AverageRate: 23.976,
			PixelFormat: "yuv420p", BitDepth: 8, ColorTransfer: "bt709", ColorPrimaries: "bt709", ColorSpace: "bt709"},
	}
	data, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "UPDATE media_analyses SET analysis = $2 WHERE version_id = $1", p.versions[0].ID, data); err != nil {
		t.Fatal(err)
	}
	advance := idleClock(p.testServer)
	android := p.profile(t, "jellyfin-android-integrated")
	// The same app on a device that does not decode HEVC.
	noHEVC := json.RawMessage(strings.ReplaceAll(strings.ReplaceAll(string(android), "h264,hevc,", "h264,"), `h264,hevc"`, `h264"`))
	ask := func(device, token string, profile json.RawMessage, directPlay bool) MediaSourceInfo {
		t.Helper()
		body := map[string]any{"UserId": p.user.ID.String(), "MediaSourceId": p.movie, "MaxStreamingBitrate": 120_000_000, "DeviceProfile": profile}
		if !directPlay {
			body["EnableDirectPlay"] = false
		}
		status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app(device, token), body)
		var response playbackInfoResponse
		if err := json.Unmarshal(data, &response); status != http.StatusOK || err != nil || len(response.MediaSources) != 1 {
			t.Fatalf("%d %s", status, data)
		}
		return response.MediaSources[0]
	}
	converted := func(what string, source MediaSourceInfo) {
		t.Helper()
		if source.SupportsDirectPlay || !strings.Contains(source.TranscodingUrl, "&VideoCodec=h264&") ||
			!strings.HasSuffix(source.TranscodingUrl, "&allowVideoStreamCopy=false") ||
			transcodeReasons(t, source.TranscodingUrl) != "VideoCodecNotSupported,DirectPlayError" {
			t.Errorf("%s: the video is not converted as the app asks: %s", what, source.TranscodingUrl)
		}
	}

	phone := p.signIn("member", "phone")
	if first := ask("phone", phone, android, true); !first.SupportsDirectPlay {
		t.Fatalf("the integrated player does not play the file as it is: %+v", first)
	}
	// Two read errors of 8 seconds each.
	advance(20 * time.Second)
	kept := ask("phone", phone, android, false)
	if kept.SupportsDirectPlay || kept.TranscodingContainer != "ts" || !strings.Contains(kept.TranscodingUrl, "&VideoCodec=hevc,h264&") ||
		strings.Contains(kept.TranscodingUrl, "allowVideoStreamCopy=false") || strings.Contains(kept.TranscodingUrl, "allowAudioStreamCopy=false") ||
		transcodeReasons(t, kept.TranscodingUrl) != "DirectPlayError" || !strings.HasSuffix(kept.Name, "(direct stream)") {
		t.Fatalf("the retry does not keep the video copied: %q %s", kept.Name, kept.TranscodingUrl)
	}
	// The copy failed too: the app asks again, then again once the user
	// plays the title anew.
	advance(10 * time.Second)
	converted("the retry after the copy", ask("phone", phone, android, false))
	advance(time.Minute)
	if again := ask("phone", phone, android, true); !again.SupportsDirectPlay {
		t.Fatalf("played anew, the file does not play as it is: %+v", again)
	}
	advance(20 * time.Second)
	converted("the retry of a new play", ask("phone", phone, android, false))

	// A decoder refusing the video fails at once.
	tablet := p.signIn("member", "tablet")
	ask("tablet", tablet, android, true)
	advance(time.Second)
	converted("a direct play failed at once", ask("tablet", tablet, android, false))

	// A device whose profile takes no HEVC never played it as it is.
	box := p.signIn("member", "box")
	if first := ask("box", box, noHEVC, true); first.SupportsDirectPlay {
		t.Fatalf("a device without HEVC plays it as it is: %+v", first)
	}
	advance(20 * time.Second)
	converted("a device without HEVC", ask("box", box, android, false))
}
