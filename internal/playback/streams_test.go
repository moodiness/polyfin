package playback

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/media"
)

// requestFields depend on the request or on the file, not on the media.
var requestFields = []string{"DeliveryMethod", "DeliveryUrl", "IsExternalUrl", "Path", "Score"}

func parseFixtureProbe(t *testing.T, clip string) media.Analysis {
	t.Helper()
	data, err := os.ReadFile(playbackFixtures + "probes/" + clip + ".json")
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := media.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	// The fixture probes name files, not URLs: the remote clip was probed
	// from a file, where Jellyfin had read it over HTTP.
	analysis.Remote = strings.HasPrefix(clip, "remote-")
	return analysis
}

// fixtureExternals are the sidecars Jellyfin listed beside each clip.
var fixtureExternals = map[string][]ExternalSubtitle{
	"h264-ac3-srt-mkv": {{Language: "fre", Codec: "subrip"}},
}

func TestMediaStreamsMatchJellyfin(t *testing.T) {
	data, err := os.ReadFile(playbackFixtures + "decisions.json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []struct {
		Media        string
		Profile      string
		Options      map[string]any
		MediaStreams []map[string]any
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	probes, err := filepath.Glob(playbackFixtures + "probes/*.json")
	if err != nil || len(probes) != 6 {
		t.Fatalf("probes: %v %v", probes, err)
	}
	for _, probe := range probes {
		clip := strings.TrimSuffix(filepath.Base(probe), ".json")
		t.Run(clip, func(t *testing.T) {
			got := MediaStreams(parseFixtureProbe(t, clip), fixtureExternals[clip], "en")
			compared := 0
			for _, entry := range entries {
				// Every entry of a clip lists the same streams; the
				// request only changes the fields left out.
				if entry.Media != clip {
					continue
				}
				compared++
				compareStreams(t, entry.Profile, got, entry.MediaStreams)
			}
			if compared == 0 {
				t.Fatal("no decision for the clip")
			}
		})
	}
}

func compareStreams(t *testing.T, context string, got []MediaStream, want []map[string]any) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d streams, want %d", context, len(got), len(want))
	}
	for i := range want {
		raw, err := json.Marshal(got[i])
		if err != nil {
			t.Fatal(err)
		}
		var ours map[string]any
		if err := json.Unmarshal(raw, &ours); err != nil {
			t.Fatal(err)
		}
		keys := maps.Clone(ours)
		maps.Copy(keys, want[i])
		for _, key := range slices.Sorted(maps.Keys(keys)) {
			if slices.Contains(requestFields, key) {
				continue
			}
			if !reflect.DeepEqual(ours[key], want[i][key]) {
				t.Errorf("%s: stream %d %s = %v, want %v", context, i, key, ours[key], want[i][key])
			}
		}
	}
}

func TestContainers(t *testing.T) {
	// Oracle §3.4 for the fixture clips, then clips made for the purpose.
	for _, c := range []struct {
		clip, filename, container, display string
	}{
		{"h264-aac-mp4", "Big Buck Bunny (2008).mp4", "mov,mp4,m4a,3gp,3g2,mj2", "mp4"},
		{"av1-opus-webm", "Cosmos Laundromat (2015).webm", "mkv,webm", "webm"},
		{"h264-ac3-srt-mkv", "Sintel (2010).mkv", "mkv", "mkv"},
		{"remote-h264-aac-mkv", "spring.mkv", "mkv", "mkv"},
		{"h264-aac-mp4", "pioneer-s01e03.mp4", "mov,mp4,m4a,3gp,3g2,mj2", "mp4"},
		{"h264-aac-mp4", "Clip.mov", "mov,mp4,m4a,3gp,3g2,mj2", "mov"},
		{"av1-opus-webm", "Clip.mkv", "mkv,webm", "mkv"},
		{"h264-aac-mp4", "", "mov,mp4,m4a,3gp,3g2,mj2", "mov"},
	} {
		analysis := parseFixtureProbe(t, c.clip)
		if got := Container(analysis); got != c.container {
			t.Errorf("%s: Container = %q, want %q", c.filename, got, c.container)
		}
		if got := DisplayContainer(analysis, c.filename); got != c.display {
			t.Errorf("%s: DisplayContainer = %q, want %q", c.filename, got, c.display)
		}
	}
	matroska := func(streams ...media.Stream) media.Analysis {
		return media.Analysis{Format: "matroska,webm", Streams: streams}
	}
	vp9 := media.Stream{Type: "video", Codec: "vp9"}
	opus := media.Stream{Type: "audio", Codec: "opus"}
	for _, c := range []struct {
		name               string
		analysis           media.Analysis
		filename           string
		container, display string
	}{
		{"H.264 in a .webm", matroska(media.Stream{Type: "video", Codec: "h264"}), "x.webm", "mkv", "mkv"},
		{"AAC in a .webm", matroska(vp9, media.Stream{Type: "audio", Codec: "aac"}), "x.webm", "mkv", "mkv"},
		{"SubRip in a .webm", matroska(vp9, opus, media.Stream{Type: "subtitle", Codec: "subrip"}), "x.webm", "mkv", "mkv"},
		{"VP9 and Opus in a .mkv", matroska(vp9, opus), "x.mkv", "mkv,webm", "mkv"},
	} {
		if got := Container(c.analysis); got != c.container {
			t.Errorf("%s: Container = %q, want %q", c.name, got, c.container)
		}
		if got := DisplayContainer(c.analysis, c.filename); got != c.display {
			t.Errorf("%s: DisplayContainer = %q, want %q", c.name, got, c.display)
		}
	}
}

// remoteProbe is ffprobe's output for a remote Matroska source with the
// given streams.
func remoteProbe(t *testing.T, streams ...string) media.Analysis {
	t.Helper()
	data := `{"format":{"filename":"https://debrid.example/Movie.2160p.mkv","format_name":"matroska,webm","bit_rate":"60000000"},"streams":[` +
		strings.Join(streams, ",") + `]}`
	analysis, err := media.Parse([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return analysis
}

func TestDynamicRanges(t *testing.T) {
	dovi := func(profile, compatibility, el, bl int) string {
		return `{"side_data_type":"DOVI configuration record","dv_version_major":1,"dv_version_minor":0,"dv_profile":` +
			strconv.Itoa(profile) + `,"dv_level":6,"rpu_present_flag":1,"el_present_flag":` + strconv.Itoa(el) +
			`,"bl_present_flag":` + strconv.Itoa(bl) + `,"dv_bl_signal_compatibility_id":` + strconv.Itoa(compatibility) + `}`
	}
	const hdr10Plus = `{"side_data_type":"HDR Dynamic Metadata SMPTE2094-40 (HDR10+)"}`
	for _, c := range []struct {
		name, codec, transfer string
		sideData              []string
		videoRange, rangeType string
		displayTitle          string
	}{
		{"Dolby Vision 5", "hevc", "", []string{dovi(5, 0, 0, 1)}, "HDR", "DOVI", "4K HEVC Dolby Vision Profile 5"},
		{"Dolby Vision 7.6 with an enhancement layer", "hevc", "smpte2084", []string{dovi(7, 6, 1, 1)}, "HDR", "DOVIWithEL", "4K HEVC Dolby Vision Profile 7.6 (HDR10)"},
		{"Dolby Vision 8.1", "hevc", "smpte2084", []string{dovi(8, 1, 0, 1)}, "HDR", "DOVIWithHDR10", "4K HEVC Dolby Vision Profile 8.1 (HDR10)"},
		{"Dolby Vision 8.1 with HDR10+", "hevc", "smpte2084", []string{dovi(8, 1, 0, 1), hdr10Plus}, "HDR", "DOVIWithHDR10Plus", "4K HEVC Dolby Vision Profile 8.1 (HDR10)"},
		{"Dolby Vision 8.2", "hevc", "bt709", []string{dovi(8, 2, 0, 1)}, "SDR", "DOVIWithSDR", "4K HEVC Dolby Vision Profile 8.2 (SDR)"},
		{"Dolby Vision 8.4", "hevc", "arib-std-b67", []string{dovi(8, 4, 0, 1)}, "HDR", "DOVIWithHLG", "4K HEVC Dolby Vision Profile 8.4 (HLG)"},
		{"Dolby Vision 10 in AV1", "av1", "", []string{dovi(10, 0, 0, 1)}, "HDR", "DOVI", "4K AV1 Dolby Vision Profile 10"},
		{"Dolby Vision record without base layer", "hevc", "smpte2084", []string{dovi(8, 1, 0, 0)}, "HDR", "HDR10", "4K HEVC HDR"},
		{"HDR10+", "hevc", "smpte2084", []string{hdr10Plus}, "HDR", "HDR10Plus", "4K HEVC HDR"},
		{"HDR10", "hevc", "smpte2084", nil, "HDR", "HDR10", "4K HEVC HDR"},
		{"HLG", "hevc", "arib-std-b67", nil, "HDR", "HLG", "4K HEVC HDR"},
		{"SDR", "hevc", "bt709", nil, "SDR", "SDR", "4K HEVC SDR"},
	} {
		video := `{"index":0,"codec_type":"video","codec_name":"` + c.codec + `","profile":"Main 10","width":3840,"height":2160,` +
			`"pix_fmt":"yuv420p10le","level":153,"color_transfer":"` + c.transfer + `","field_order":"progressive",` +
			`"sample_aspect_ratio":"1:1","display_aspect_ratio":"16:9","r_frame_rate":"24000/1001","avg_frame_rate":"24000/1001",` +
			`"time_base":"1/1000","side_data_list":[` + strings.Join(c.sideData, ",") + `]}`
		stream := MediaStreams(remoteProbe(t, video), nil, "en")[0]
		if stream.VideoRange != c.videoRange || stream.VideoRangeType != c.rangeType || stream.DisplayTitle != c.displayTitle {
			t.Errorf("%s: %s %s %q, want %s %s %q", c.name, stream.VideoRange, stream.VideoRangeType, stream.DisplayTitle,
				c.videoRange, c.rangeType, c.displayTitle)
		}
		if hdr10Plus := slices.Contains(c.sideData, hdr10Plus); hdr10Plus != (stream.Hdr10PlusPresentFlag != nil && *stream.Hdr10PlusPresentFlag) {
			t.Errorf("%s: Hdr10PlusPresentFlag %v", c.name, stream.Hdr10PlusPresentFlag)
		}
		if stream.RefFrames != nil || *stream.AverageFrameRate != 23.976025 || *stream.BitRate != 60000000 {
			t.Errorf("%s: RefFrames %v, AverageFrameRate %v, BitRate %v", c.name, stream.RefFrames, *stream.AverageFrameRate, *stream.BitRate)
		}
	}

	stream := MediaStreams(remoteProbe(t, `{"index":0,"codec_type":"video","codec_name":"hevc","width":3840,"height":2160,"side_data_list":[`+
		dovi(8, 1, 0, 1)+`]}`), nil, "en")[0]
	got := []*int{stream.DvVersionMajor, stream.DvVersionMinor, stream.DvProfile, stream.DvLevel, stream.RpuPresentFlag,
		stream.ElPresentFlag, stream.BlPresentFlag, stream.DvBlSignalCompatibilityId}
	for i, want := range []int{1, 0, 8, 6, 1, 0, 1, 1} {
		if got[i] == nil || *got[i] != want {
			t.Errorf("Dolby Vision field %d: %v, want %d", i, got[i], want)
		}
	}
}

func TestAudioFormats(t *testing.T) {
	for _, c := range []struct {
		codec, profile, layout string
		channels               int
		spatial, displayTitle  string
	}{
		{"truehd", "Dolby TrueHD + Dolby Atmos", "7.1", 8, "DolbyAtmos", "English - Dolby TrueHD + Dolby Atmos - 7.1"},
		{"eac3", "Dolby Digital Plus + Dolby Atmos", "5.1(side)", 6, "DolbyAtmos", "English - Dolby Digital Plus + Dolby Atmos - 5.1"},
		{"dts", "DTS-HD MA + DTS:X", "7.1", 8, "DTSX", "English - DTS-HD MA + DTS:X - 7.1"},
		{"dts", "DTS-HD MA + DTS:X IMAX", "7.1", 8, "DTSX", "English - DTS-HD MA + DTS:X IMAX - 7.1"},
		{"dts", "DTS-HD MA", "5.1(side)", 6, "None", "English - DTS-HD MA - 5.1"},
		{"truehd", "", "5.1(side)", 6, "None", "English - TRUEHD - 5.1"},
		{"eac3", "", "5.1(side)", 6, "None", "English - Dolby Digital+ - 5.1"},
		{"pcm_s16le", "", "", 2, "None", "English - PCM_S16LE - 2 ch"},
	} {
		analysis := media.Analysis{Format: "matroska,webm", Streams: []media.Stream{
			{Index: 0, Type: "video", Codec: "hevc", Width: 3840, Height: 2160},
			{Index: 1, Type: "audio", Codec: c.codec, Profile: c.profile, Language: "eng", ChannelLayout: c.layout, Channels: c.channels},
		}}
		stream := MediaStreams(analysis, nil, "en")[1]
		if stream.AudioSpatialFormat != c.spatial || stream.DisplayTitle != c.displayTitle || stream.Profile != c.profile {
			t.Errorf("%s %q: %s %q %q, want %s %q", c.codec, c.profile, stream.AudioSpatialFormat, stream.DisplayTitle,
				stream.Profile, c.spatial, c.displayTitle)
		}
	}
}

func TestBitrates(t *testing.T) {
	// Matroska tracks muxed by ffmpeg state no bitrate: Jellyfin assumes
	// one per codec and channel count, and gives the video what is left.
	analysis := media.Analysis{Format: "matroska,webm", Bitrate: 10_000_000, Streams: []media.Stream{
		{Index: 0, Type: "video", Codec: "h264"},
		{Index: 1, Type: "audio", Codec: "flac", Channels: 6},
		{Index: 2, Type: "audio", Codec: "truehd", Channels: 2},
		{Index: 3, Type: "audio", Codec: "aac", Channels: 3},
		{Index: 4, Type: "audio", Codec: "aac", Channels: 2},
		{Index: 5, Type: "audio", Codec: "opus", Channels: 8},
		{Index: 6, Type: "audio", Codec: "alac", Channels: 2},
		{Index: 7, Type: "audio", Codec: "vorbis", Channels: 2},
		{Index: 8, Type: "audio", Codec: "ac3", Channels: 6, Bitrate: 640_000},
		{Index: 9, Type: "subtitle", Codec: "subrip"},
	}}
	streams := MediaStreams(analysis, nil, "en")
	for i, want := range []int64{3_352_000, 2_880_000, 1_400_000, 320_000, 192_000, 256_000, 960_000, 0, 640_000, 0} {
		if got := streams[i].BitRate; (got == nil) != (want == 0) || got != nil && *got != want {
			t.Errorf("stream %d (%s): BitRate %v, want %d", i, streams[i].Codec, got, want)
		}
	}
	analysis.Bitrate = 6_000_000
	if got := MediaStreams(analysis, nil, "en")[0].BitRate; got != nil {
		t.Errorf("video BitRate %d, want none when the audio takes more than the source", *got)
	}
}

func TestVideoSizes(t *testing.T) {
	// Observed on Jellyfin 12.2 with clips of each size.
	for _, c := range []struct {
		width, height int
		fieldOrder    string
		dar, sar      string
		displayTitle  string
		aspectRatio   string
		anamorphic    bool
	}{
		{3840, 2160, "progressive", "16:9", "1:1", "4K H264 SDR", "16:9", false},
		{3840, 1600, "progressive", "12:5", "1:1", "4K H264 SDR", "2.40:1", false},
		{3996, 2160, "progressive", "37:20", "1:1", "4K H264 SDR", "1.85:1", false},
		{4096, 3100, "progressive", "", "", "8K H264 SDR", "4:3", false},
		{2560, 1440, "progressive", "16:9", "1:1", "1080p H264 SDR", "16:9", false},
		{2600, 1080, "progressive", "65:27", "1:1", "4K H264 SDR", "2.40:1", false},
		{1920, 818, "progressive", "960:409", "1:1", "1080p H264 SDR", "2.35:1", false},
		{1920, 1040, "progressive", "24:13", "1:1", "1080p H264 SDR", "1.85:1", false},
		{1440, 1080, "progressive", "16:9", "4:3", "1080p H264 SDR", "16:9", true},
		{1300, 720, "progressive", "65:36", "1:1", "1080p H264 SDR", "16:9", false},
		{1280, 720, "bt", "16:9", "1:1", "720i H264 SDR", "16:9", false},
		{1280, 536, "progressive", "160:67", "1:1", "720p H264 SDR", "2.40:1", false},
		{1280, 800, "progressive", "8:5", "1:1", "720p H264 SDR", "1.6:1", false},
		{720, 576, "tt", "16:9", "64:45", "576i H264 SDR", "16:9", true},
		{720, 480, "progressive", "3:2", "1:1", "480p H264 SDR", "1.5:1", false},
		{720, 404, "progressive", "180:101", "1:1", "404p H264 SDR", "16:9", false},
		{698, 438, "progressive", "349:219", "1:1", "480p H264 SDR", "349:219", false},
		{3800, 1000, "progressive", "19:5", "1:1", "4K H264 SDR", "19:5", false},
		{256, 146, "progressive", "128:73", "1:1", "240p H264 SDR", "16:9", false},
		{640, 360, "progressive", "", "", "360p H264 SDR", "16:9", false},
	} {
		analysis := media.Analysis{Format: "matroska,webm", Streams: []media.Stream{{
			Type: "video", Codec: "h264", Width: c.width, Height: c.height, FieldOrder: c.fieldOrder,
			AspectRatio: c.dar, SampleAspect: c.sar,
		}}}
		stream := MediaStreams(analysis, nil, "en")[0]
		if stream.DisplayTitle != c.displayTitle || stream.AspectRatio != c.aspectRatio || *stream.IsAnamorphic != c.anamorphic ||
			stream.IsInterlaced != (c.fieldOrder != "progressive") {
			t.Errorf("%dx%d: %q %q anamorphic %v interlaced %v, want %q %q %v", c.width, c.height, stream.DisplayTitle,
				stream.AspectRatio, *stream.IsAnamorphic, stream.IsInterlaced, c.displayTitle, c.aspectRatio, c.anamorphic)
		}
	}
	titled := media.Analysis{Streams: []media.Stream{{Type: "video", Codec: "h264", Width: 1920, Height: 1080, Title: "Main Video"}}}
	if got := MediaStreams(titled, nil, "en")[0].DisplayTitle; got != "Main Video - 1080p - H264 - SDR" {
		t.Errorf("titled video: %q", got)
	}
}

func TestFrenchStrings(t *testing.T) {
	// Observed with UICulture=fr: the words are French, the language and
	// channel layout names stay English.
	clip := "h264-ac3-srt-mkv"
	streams := MediaStreams(parseFixtureProbe(t, clip), fixtureExternals[clip], "fr")
	for i, want := range []string{
		"French - SUBRIP - Externe",
		"360p H264 SDR",
		"Surround 5.1 - English - Dolby Digital - Par défaut",
		"Commentaire - French - AAC - Stereo",
		"English - SUBRIP",
		"Forced - English - Forcé - SUBRIP",
	} {
		if streams[i].DisplayTitle != want {
			t.Errorf("stream %d: %q, want %q", i, streams[i].DisplayTitle, want)
		}
	}
	subtitle := streams[4]
	if got := []string{subtitle.LocalizedUndefined, subtitle.LocalizedDefault, subtitle.LocalizedForced, subtitle.LocalizedExternal,
		subtitle.LocalizedHearingImpaired, subtitle.LocalizedLanguage}; !slices.Equal(got,
		[]string{"Non défini", "Par défaut", "Forcé", "Externe", "Malentendants", "English"}) {
		t.Errorf("subtitle strings: %q", got)
	}
	audio := streams[2]
	if got := []string{audio.LocalizedDefault, audio.LocalizedExternal, audio.LocalizedOriginal, audio.LocalizedLanguage,
		audio.LocalizedForced}; !slices.Equal(got, []string{"Par défaut", "Externe", "Original", "English", ""}) {
		t.Errorf("audio strings: %q", got)
	}
	flagged := media.Analysis{Streams: []media.Stream{
		{Type: "audio", Codec: "aac", Profile: "LC", Language: "ger", ChannelLayout: "stereo", Default: true, Original: true},
		{Type: "subtitle", Codec: "subrip", Language: "eng", Default: true, Forced: true, HearingImpaired: true},
		{Type: "subtitle", Codec: "subrip", Title: "Signs"},
	}}
	for i, want := range []string{
		"German - AAC - Stereo - Par défaut - Original",
		"English - Malentendants - Par défaut - Forcé - SUBRIP",
		"Signs - Non défini - SUBRIP",
	} {
		if got := MediaStreams(flagged, nil, "fr")[i].DisplayTitle; got != want {
			t.Errorf("flagged %d: %q, want %q", i, got, want)
		}
	}
}

func TestAddonLanguages(t *testing.T) {
	for _, c := range []struct {
		given, language, name, displayTitle string
	}{
		{"fre", "fra", "French", "French - SUBRIP - External"},
		{"fra", "fra", "French", "French - SUBRIP - External"},
		{"fr", "fra", "French", "French - SUBRIP - External"},
		{"FR", "fra", "French", "French - SUBRIP - External"},
		{"fr-FR", "fra", "French", "French - SUBRIP - External"},
		{"French", "fra", "French", "French - SUBRIP - External"},
		{"Français", "fra", "French", "French - SUBRIP - External"},
		{"français", "fra", "French", "French - SUBRIP - External"},
		{"ger", "deu", "German", "German - SUBRIP - External"},
		{"Spanish", "spa", "Spanish", "Spanish - SUBRIP - External"},
		{"espagnol", "spa", "Spanish", "Spanish - SUBRIP - External"},
		{"pt-BR", "por", "Portuguese", "Portuguese - SUBRIP - External"},
		{"gre", "ell", "Greek", "Greek - SUBRIP - External"},
		{"und", "und", "Undetermined", "Undetermined - SUBRIP - External"},
		{"pob", "pob", "", "Pob - SUBRIP - External"},
		{"", "", "", "Undefined - SUBRIP - External"},
	} {
		stream := ExternalStreams([]ExternalSubtitle{{Language: c.given, Codec: "subrip"}}, "en")[0]
		if stream.Language != c.language || stream.LocalizedLanguage != c.name || stream.DisplayTitle != c.displayTitle {
			t.Errorf("%q: %q %q %q, want %q %q %q", c.given, stream.Language, stream.LocalizedLanguage, stream.DisplayTitle,
				c.language, c.name, c.displayTitle)
		}
	}
	titled := ExternalStreams([]ExternalSubtitle{{Language: "eng", Title: "English (SDH)", Codec: "webvtt"}}, "en")[0]
	if titled.DisplayTitle != "English (SDH) - WEBVTT - External" || titled.Index != 0 || !titled.IsExternal {
		t.Errorf("titled: %+v", titled)
	}
}

func TestEmbeddedLanguages(t *testing.T) {
	// Observed: only ISO 639-2/B codes are rewritten; other tags are kept
	// and named when Jellyfin's list knows them by code or full name.
	for _, c := range []struct {
		tag, language, name, audioTitle, subtitleTitle string
	}{
		{"fre", "fra", "French", "French - AAC - Stereo", "French - SUBRIP"},
		{"ger", "deu", "German", "German - AAC - Stereo", "German - SUBRIP"},
		{"fr", "fr", "French", "French - AAC - Stereo", "French - SUBRIP"},
		{"French", "French", "French", "French - AAC - Stereo", "French - SUBRIP"},
		{"Français", "Français", "", "Français - AAC - Stereo", "Français - SUBRIP"},
		{"Spanish", "Spanish", "", "Spanish - AAC - Stereo", "Spanish - SUBRIP"},
		{"spa", "spa", "Spanish", "Spanish - AAC - Stereo", "Spanish - SUBRIP"},
		{"ENG", "ENG", "English", "English - AAC - Stereo", "English - SUBRIP"},
		{"pt-BR", "pt-BR", "Portuguese (Brazil)", "Portuguese (Brazil) - AAC - Stereo", "Portuguese (Brazil) - SUBRIP"},
		{"en-US", "en-US", "", "En-US - AAC - Stereo", "En-US - SUBRIP"},
		{"und", "und", "Undetermined", "AAC - Stereo", "Undetermined - SUBRIP"},
		{"zxx", "zxx", "No linguistic content", "AAC - Stereo", "No linguistic content - SUBRIP"},
	} {
		analysis := media.Analysis{Streams: []media.Stream{
			{Index: 0, Type: "audio", Codec: "aac", Profile: "LC", Language: c.tag, ChannelLayout: "stereo", Channels: 2},
			{Index: 1, Type: "subtitle", Codec: "subrip", Language: c.tag},
		}}
		streams := MediaStreams(analysis, nil, "en")
		for i, title := range []string{c.audioTitle, c.subtitleTitle} {
			if s := streams[i]; s.Language != c.language || s.LocalizedLanguage != c.name || s.DisplayTitle != title {
				t.Errorf("%q %s: %q %q %q, want %q %q %q", c.tag, s.Type, s.Language, s.LocalizedLanguage, s.DisplayTitle,
					c.language, c.name, title)
			}
		}
	}
}
