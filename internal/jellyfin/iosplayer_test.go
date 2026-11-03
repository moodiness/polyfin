package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
)

// iosHeader is the Authorization header of the Jellyfin iOS app, its
// client named client, which its native player's PlaybackInfo carries
// (jellyfin-web in the app's web view asks it). Each client is a device
// of its own.
func iosHeader(client, token string) string {
	return `MediaBrowser Client="` + client + `", Device="iPad Pro", DeviceId="866-` + strings.ReplaceAll(client, " ", "-") +
		`", Version="1.8.0", Token="` + token + `"`
}

// The Jellyfin iOS app's native player lists a progressive MP4 taking
// AC-3 and E-AC-3 after its HLS profile taking AAC and MP3. Jellyfin
// streams a Matroska file with E-AC-3 to it as that progressive MP4,
// which AVPlayer refuses (#866). Polyfin gives it what that MP4 would
// carry over HLS, in its HLS profile's container: the E-AC-3 copied, in
// MPEG-TS with "Prefer fMP4 in HLS" off, in fragmented MP4 with it on,
// and never as one file, even with Converted downloads on. The version's
// name says so. jellyfin-web sending the same profile, and the app's
// downloads, are decided as before.
func TestTheIOSNativePlayerGetsEAC3OverHLS(t *testing.T) {
	t.Parallel()
	p := playing(t)
	analysis := media.Analysis{Format: "matroska,webm", Duration: 15 * time.Second, Size: 12_000_000, Bitrate: 6_400_000, Remote: true,
		Streams: []media.Stream{
			{Index: 0, Type: "video", Codec: "h264", Profile: "High", Level: 40, Width: 1920, Height: 1080, FrameRate: 24, AverageRate: 24,
				PixelFormat: "yuv420p", BitDepth: 8},
			{Index: 1, Type: "audio", Codec: "eac3", Default: true, Channels: 6, SampleRate: 48000, ChannelLayout: "5.1(side)", Bitrate: 640_000},
		}}
	data, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", p.versions[0].ID, data); err != nil {
		t.Fatal(err)
	}
	p.keyframed(t, p.versions[0])
	token := p.signInAs(t, "member", iosHeader("Jellyfin iPadOS", ""))
	phoneToken := p.signInAs(t, "member", iosHeader("Jellyfin iOS", ""))
	webToken := p.signInAs(t, "member", iosHeader("Jellyfin Web", ""))
	native, fmp4 := p.profile(t, "jellyfin-expo-native"), p.profile(t, "jellyfin-expo-native-fmp4")
	var download map[string]any
	if err := json.Unmarshal(native, &download); err != nil {
		t.Fatal(err)
	}
	// The app's download profile (features/downloads/utils/profile.ts):
	// renamed, its HLS profiles left out.
	download["Name"] = "iOS 13+ Download Profile"
	var kept []any
	for _, profile := range download["TranscodingProfiles"].([]any) {
		if profile.(map[string]any)["Protocol"] != "hls" {
			kept = append(kept, profile)
		}
	}
	download["TranscodingProfiles"] = kept
	downloadProfile, _ := json.Marshal(download)

	type played struct {
		protocol, container, audio, reasons, name string
	}
	ask := func(authorization string, profile json.RawMessage) played {
		t.Helper()
		status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", authorization,
			map[string]any{"DeviceProfile": profile, "MaxStreamingBitrate": 120_000_000, "MediaSourceId": p.movie, "AutoOpenLiveStream": true})
		var answer playbackInfoResponse
		if err := json.Unmarshal(data, &answer); status != http.StatusOK || err != nil || len(answer.MediaSources) != 1 {
			t.Fatalf("PlaybackInfo: %d %s", status, data)
		}
		source := answer.MediaSources[0]
		target, err := url.Parse(source.TranscodingUrl)
		if err != nil || source.TranscodingUrl == "" {
			t.Fatalf("no TranscodingUrl: %+v", source)
		}
		return played{source.TranscodingSubProtocol, source.TranscodingContainer, target.Query().Get("AudioCodec"),
			target.Query().Get("TranscodeReasons"), source.Name}
	}
	name := "Source 2160p · REMUX · HEVC"
	overTS := played{"hls", "ts", "aac,mp3,ac3,eac3", "ContainerNotSupported", name + " (direct stream)"}
	overFMP4 := played{"hls", "mp4", "aac,mp3,ac3,eac3,flac,alac", "ContainerNotSupported", name + " (direct stream)"}
	aacTS := played{"hls", "ts", "aac,mp3", "ContainerNotSupported,AudioCodecNotSupported", name + " (converted: audio)"}
	progressive := played{"http", "mp4", "aac,mp3,ac3,eac3,flac,alac", "ContainerNotSupported", name + " (direct stream)"}

	check := func(what string, got, want played) {
		t.Helper()
		if got != want {
			t.Errorf("%s:\n got %+v\nwant %+v", what, got, want)
		}
	}
	for _, convertedDownloads := range []bool{false, true} {
		p.setting(t, func(s *accounts.Settings) { s.ConvertedDownloads = convertedDownloads })
		suffix := ""
		if convertedDownloads {
			suffix = ", Converted downloads on"
		}
		check("iPad, fMP4 off"+suffix, ask(iosHeader("Jellyfin iPadOS", token), native), overTS)
		check("iPhone, fMP4 off"+suffix, ask(iosHeader("Jellyfin iOS", phoneToken), native), overTS)
		check("iPad, fMP4 on"+suffix, ask(iosHeader("Jellyfin iPadOS", token), fmp4), overFMP4)
		// The same profile from another client is decided as before.
		web := aacTS
		if convertedDownloads {
			web = progressive
		}
		check("the profile from another client"+suffix, ask(iosHeader("Jellyfin Web", webToken), native), web)
	}
	// The app's downloads keep the progressive MP4 Converted downloads gives.
	check("the app's download", ask(iosHeader("Jellyfin iPadOS", token), downloadProfile), progressive)

	// Item details name the version for the device's last play as it plays.
	p.setting(t, func(s *accounts.Settings) { s.ConvertedDownloads = false })
	ask(iosHeader("Jellyfin iPadOS", token), native)
	var movie BaseItemDto
	if status := p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, token, &movie); status != http.StatusOK || movie.MediaSources == nil {
		t.Fatalf("item details: %d", status)
	}
	if got := (*movie.MediaSources)[0].Name; got != overTS.name {
		t.Errorf("item details: %q, want %q", got, overTS.name)
	}
}

// The native player's HLS profile made of its progressive one keeps the
// app's own HLS container, minimum segments and keyframe breaks, and its
// codecs; the profile sent is left as it is. Other profile names, and the
// right profile from another client, are not the native player.
func TestIOSNativeOverHLS(t *testing.T) {
	t.Parallel()
	read := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	request := func(client string) *http.Request {
		r, _ := http.NewRequest(http.MethodPost, "/Items/x/PlaybackInfo", nil)
		r.Header.Set("Authorization", iosHeader(client, "t"))
		return r
	}
	for _, test := range []struct {
		profile, client string
		native          bool
		container       string
	}{
		{"jellyfin-expo-native", "Jellyfin iPadOS", true, "ts"},
		{"jellyfin-expo-native", "jellyfin ios", true, "ts"},
		{"jellyfin-expo-native-fmp4", "Jellyfin iOS", true, "mp4"},
		{"jellyfin-expo-native", "Jellyfin Web", false, ""},
		{"swiftfin-native", "Jellyfin iOS", false, ""},
		{"jellyfin-web-12.4-chrome", "Jellyfin iPadOS", false, ""},
	} {
		var profile, sent playback.DeviceProfile
		data := read(test.profile)
		if json.Unmarshal(data, &profile) != nil || json.Unmarshal(data, &sent) != nil {
			t.Fatal(test.profile)
		}
		if got := iosNativePlayer(request(test.client), &profile); got != test.native {
			t.Errorf("%s from %s: native %v", test.profile, test.client, got)
		}
		if !test.native {
			continue
		}
		over := iosNativeOverHLS(&profile)
		if !reflect.DeepEqual(profile, sent) {
			t.Errorf("%s: the profile sent was changed", test.profile)
		}
		var streamed int
		for i, transcoding := range over.TranscodingProfiles {
			original := profile.TranscodingProfiles[i]
			if transcoding.Type != "Video" || original.Protocol != "http" || original.Context != "Streaming" {
				if !reflect.DeepEqual(transcoding, original) {
					t.Errorf("%s: profile %d changed: %+v", test.profile, i, transcoding)
				}
				continue
			}
			streamed++
			if transcoding.Protocol != "hls" || transcoding.Container != test.container || transcoding.MinSegments != "2" || !transcoding.BreakOnNonKeyFrames ||
				transcoding.AudioCodec != original.AudioCodec || transcoding.VideoCodec != original.VideoCodec {
				t.Errorf("%s: profile %d: %+v", test.profile, i, transcoding)
			}
		}
		if streamed != 1 {
			t.Errorf("%s: %d progressive profiles turned to HLS", test.profile, streamed)
		}
	}
}
