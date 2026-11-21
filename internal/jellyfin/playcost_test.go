package jellyfin

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// With prefer, pick takes the candidate that plays at the least cost among
// those playing the user's languages as well as the first that plays, the
// first on a tie; it waits for a candidate still being tried while the
// cheapest found needs a conversion, not once one needs none.
func TestPickRanksTheVersionsByCost(t *testing.T) {
	t.Parallel()
	french := playedLanguages{audio: "fre", audioRank: 2}
	english := playedLanguages{audio: "eng", audioRank: 1}
	plays := func(cost playCost) *tried { return &tried{plays: true, cost: cost, languages: french} }
	fails := &tried{unreadable: true}
	for _, tc := range []struct {
		name    string
		states  []*tried
		sides   []int
		prefer  bool
		patient bool
		want    int
		done    bool
	}{
		{"audio conversion before video conversion", []*tried{plays(costVideoConverted), plays(costAudioConverted)}, nil, true, true, 1, true},
		{"within the cap before over it", []*tried{plays(costOverCap), plays(costVideoConverted)}, nil, true, true, 1, true},
		{"as it is before repackaged", []*tried{plays(costCopied), plays(costDirect)}, nil, true, true, 1, true},
		{"a copy before an audio conversion", []*tried{plays(costAudioConverted), plays(costCopied), plays(costDirect)}, nil, true, true, 2, true},
		{"ties keep the order", []*tried{plays(costAudioConverted), plays(costAudioConverted)}, nil, true, true, 0, true},
		{"a failed version passed", []*tried{fails, plays(costOverCap), plays(costCopied)}, nil, true, true, 2, true},
		{"waits while the cheapest converts", []*tried{plays(costVideoConverted), nil, plays(costAudioConverted)}, nil, true, true, -1, false},
		{"then passes those still read", []*tried{plays(costVideoConverted), nil, plays(costAudioConverted)}, nil, true, false, 2, true},
		{"no wait once one needs no conversion", []*tried{plays(costCopied), nil}, nil, true, true, 0, true},
		{"waits for the first", []*tried{nil, plays(costDirect)}, nil, true, true, -1, false},
		{"the user's languages first", []*tried{plays(costOverCap), {plays: true, cost: costDirect, languages: english}}, nil, true, true, 0, true},
		{"local files first", []*tried{plays(costOverCap), plays(costDirect)}, []int{0, 1}, true, true, 0, true},
		{"without prefer, the first that plays", []*tried{fails, plays(costOverCap), plays(costDirect)}, nil, false, true, 1, true},
		{"none plays", []*tried{fails, fails}, nil, true, true, -1, true},
	} {
		sides := tc.sides
		if sides == nil {
			sides = make([]int, len(tc.states))
		}
		if got, done := pick(tc.states, sides, tc.prefer, tc.patient); got != tc.want || done != tc.done {
			t.Errorf("%s: %d %t, want %d %t", tc.name, got, done, tc.want, tc.done)
		}
	}
}

// convertible stores for a version the analysis of a Matroska file of the
// given video and audio codecs, height and overall bitrate, with the
// keyframe index streaming it over HLS needs.
func (p playbackSetup) convertible(t *testing.T, version library.Version, video, audio string, height int, bitrate int64) {
	t.Helper()
	p.storeAnalysis(t, version, media.Analysis{Format: "matroska,webm", Duration: 15008 * time.Millisecond, Size: 1_000_000, Bitrate: bitrate,
		Remote: true, Streams: []media.Stream{
			{Index: 0, Type: "video", Codec: video, Profile: "Main", Level: 40, Width: height * 16 / 9, Height: height, FrameRate: 24,
				AverageRate: 24, PixelFormat: "yuv420p", BitDepth: 8},
			{Index: 1, Type: "audio", Codec: audio, Default: true, Channels: 6, SampleRate: 48000, ChannelLayout: "5.1(side)"},
		}})
	p.keyframed(t, version)
}

// With jellyfin-web's Chrome profile and two analyzed versions, Play
// takes the one cheapest to play: the movie's second version, whose audio
// alone is converted, rather than its first, a 2160p version in a video
// codec Chrome does not play. When both versions need their video
// converted, as the other title's do, the one within the app's bitrate
// cap comes before the one above it. The menu, once the device's profile
// is known, lists the versions in that order too.
func TestPlayTakesTheVersionCheapestToPlay(t *testing.T) {
	t.Parallel()
	if os.Getenv("POLYFIN_TEST_FFMPEG") == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: Polyfin converts video with the encoders FFmpeg has")
	}
	p := playing(t)
	chrome := p.profile(t, "jellyfin-web-chrome")
	p.setting(t, func(settings *accounts.Settings) { settings.PreferDirectPlay = true })
	p.convertible(t, p.versions[0], "vc1", "aac", 2160, 60_000_000)
	p.convertible(t, p.versions[1], "h264", "eac3", 1080, 10_000_000)
	remote := p.versionsOf(t, p.remote)
	p.convertible(t, remote[0], "vc1", "aac", 2160, 60_000_000)
	p.convertible(t, remote[1], "mpeg2video", "aac", 1080, 10_000_000)

	// The version the app names plays, however it costs.
	asked := firstSource(t, p.ask(t, p.token, p.movie, chrome, map[string]any{"MediaSourceId": p.versions[0].ID.String()}))
	if asked.ETag != p.versions[0].ID.String() || !strings.Contains(asked.TranscodingUrl, "allowVideoStreamCopy=false") {
		t.Fatalf("the first version, asked for: %s %s", asked.ETag, asked.TranscodingUrl)
	}
	// Devices whose profile is not known yet get the server's order, the
	// first version first.
	onDevice := func(device, item string, bitrate int) MediaSourceInfo {
		t.Helper()
		return firstSource(t, p.playbackInfo(t, device, p.signIn("member", device), item,
			map[string]any{"MaxStreamingBitrate": bitrate, "DeviceProfile": chrome}))
	}
	source := onDevice("web", p.movie, 120_000_000)
	if source.ETag != p.versions[1].ID.String() || !strings.Contains(source.TranscodingUrl, "allowAudioStreamCopy=false") ||
		strings.Contains(source.TranscodingUrl, "allowVideoStreamCopy=false") {
		t.Errorf("an audio conversion before a video conversion: %s %s", source.ETag, source.TranscodingUrl)
	}
	if got, want := etags(p.detailsOn(t, "web", p.signIn("member", "web"), p.movie)), versionIDs(p.versions[1], p.versions[0]); !slices.Equal(got, want) {
		t.Errorf("the menu: %v, want %v", got, want)
	}

	// The 60 Mbit/s source is above the app's 20 Mbit/s, the 10 Mbit/s
	// one within it.
	source = onDevice("phone", p.remote, 20_000_000)
	if source.ETag != remote[1].ID.String() || !strings.Contains(source.TranscodingUrl, "allowVideoStreamCopy=false") {
		t.Errorf("a conversion within the cap before one above it: %s %s", source.ETag, source.TranscodingUrl)
	}
	// Within the cap, both cost alike: the addon's order decides.
	if source = onDevice("tablet", p.remote, 120_000_000); source.ETag != remote[0].ID.String() {
		t.Errorf("two conversions within the cap: %s", source.ETag)
	}
}
