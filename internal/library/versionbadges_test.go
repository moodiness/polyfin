package library

import (
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/media"
)

// labeled is a version of a movie named "Movie" whose addon labels it
// with name, read as newVersion reads a stream's labels.
func labeled(name string) Version {
	v := Version{Name: name, Height: LabelHeight(name), Labels: ReadLabels("Movie", name)}
	v.Labels.Tier, v.Labels.Audio, v.Labels.Edition = ReadRelease([]string{"Movie"}, name)
	return v
}

// A version's badges come in one order, from its analysis where it tells
// a thing, else from its labels: resolution, kind of HDR, video codec,
// audio format, channels, edition and audio languages, by code. Dolby
// Vision is told apart from the HDR10 it may carry, and Atmos from DTS:X;
// a wide 4K video is 4K; an analysis telling SDR, or audio without a
// brand, wins over labels saying otherwise. Many languages end with MULTI.
func TestVersionBadges(t *testing.T) {
	t.Parallel()
	video := func(codec string, width, height int, transfer string, dv *media.DolbyVision) media.Stream {
		return media.Stream{Type: "video", Codec: codec, Width: width, Height: height, ColorTransfer: transfer, DolbyVision: dv}
	}
	audio := func(codec, profile string, channels int, language string) media.Stream {
		return media.Stream{Type: "audio", Codec: codec, Profile: profile, Channels: channels, Language: language}
	}
	dv81 := &media.DolbyVision{Profile: 8, Compatibility: 1, RPU: true, BL: true}
	for _, tc := range []struct {
		name    string
		version Version
		known   []media.Analysis
		want    string
	}{
		{"labels only, emojis and flags", labeled("⚡ Demo · 4K 🌈 DV · 🎞️ BluRay 🎥 HEVC 🌈 DV HDR10 🎬 IMAX · 🔊 Atmos TrueHD 7.1 · " +
			"📦 58.2 GB 👤 312 · 🌐 🇫🇷 🇬🇧 Multi"), nil, "4K DV HEVC Atmos 7.1 IMAX FR EN"},
		{"labels only, a release's name", labeled("Movie.2008.1080p.BluRay.DTS-HD.MA.5.1.x264.EXTENDED-GROUP.mkv"), nil,
			"1080p H.264 DTS-HD_MA 5.1 Extended"},
		{"labels: HDR10 alone", labeled("Movie 2008 2160p HDR10 HEVC"), nil, "4K HDR10 HEVC"},
		{"labels: HDR of no kind", labeled("Movie 2008 2160p HDR"), nil, "4K HDR"},
		{"labels: DTS:X is not Atmos", labeled("Movie (2008) DTS:X 7.1"), nil, "DTS:X 7.1"},
		{"labels: Dolby Digital Plus", labeled("Movie 2008 720p WEB DD+ 2.0 FRENCH"), nil, "720p DD+ Stereo FR"},
		{"the analysis wins", labeled("Movie 2008 1080p HDR10 x264 DTS-HD MA 5.1 ITA IMAX"),
			[]media.Analysis{{Streams: []media.Stream{video("hevc", 3840, 1600, "smpte2084", dv81),
				audio("eac3", "Dolby Digital Plus + Dolby Atmos", 6, "fre"), audio("aac", "LC", 2, "eng")}}},
			"4K DV HEVC Atmos 5.1 IMAX FR EN"},
		{"the analysis: HDR10 without Dolby Vision", labeled("Movie 2008 2160p DV HDR10"),
			[]media.Analysis{{Streams: []media.Stream{video("hevc", 3840, 2160, "smpte2084", nil)}}}, "4K HDR10 HEVC"},
		{"the analysis: HDR10+", labeled("Movie"),
			[]media.Analysis{{Streams: []media.Stream{{Type: "video", Codec: "hevc", Width: 3840, Height: 2160, ColorTransfer: "smpte2084", HDR10Plus: true}}}},
			"4K HDR10+ HEVC"},
		{"the analysis: SDR wins over labels", labeled("Movie 2008 2160p DV"),
			[]media.Analysis{{Streams: []media.Stream{video("h264", 1920, 800, "bt709", nil)}}}, "1080p H.264"},
		{"the analysis: Atmos over TrueHD", labeled("Movie"),
			[]media.Analysis{{Streams: []media.Stream{audio("truehd", "Dolby TrueHD + Dolby Atmos", 8, "eng"), audio("ac3", "", 6, "eng")}}},
			"Atmos 7.1 EN"},
		{"the analysis: DTS:X is not Atmos", labeled("Movie 2008 Atmos"),
			[]media.Analysis{{Streams: []media.Stream{audio("dts", "DTS-HD MA + DTS:X", 8, "")}}}, "DTS:X 7.1"},
		{"the analysis: audio of no brand wins", labeled("Movie 2008 TrueHD Atmos 7.1"),
			[]media.Analysis{{Streams: []media.Stream{audio("aac", "LC", 2, "jpn")}}}, "Stereo JA"},
		{"the analysis first, then RemuxDB's description, then labels", labeled("Movie 2008 IMAX GERMAN"),
			[]media.Analysis{{Streams: []media.Stream{video("av1", 1280, 720, "", nil)}},
				{Streams: []media.Stream{video("hevc", 3840, 2160, "smpte2084", nil), audio("dts", "DTS-HD MA", 6, "spa")}}},
			"720p AV1 DTS-HD_MA 5.1 IMAX ES"},
		{"many languages", labeled("Movie"),
			[]media.Analysis{{Streams: []media.Stream{audio("aac", "", 2, "fre"), audio("aac", "", 2, "eng"), audio("aac", "", 2, "ger"),
				audio("aac", "", 2, "ita"), audio("aac", "", 2, "spa"), audio("aac", "", 2, "und")}}},
			"Stereo FR EN DE IT"},
		{"MULTI when no language is named", labeled("Movie 2008 MULTi 1080p"), nil, "1080p MULTI"},
		{"MULTI hidden by a language named", labeled("Movie 2008 MULTi VFF 1080p"), nil, "1080p FR"},
		{"nothing known", labeled("Cached"), nil, ""},
	} {
		got := VersionBadges(tc.version, "en", tc.known...)
		texts := make([]string, len(got))
		for i, badge := range got {
			texts[i] = strings.ReplaceAll(badge.Text, " ", "_")
			if badge.Label == "" || badge.Kind == "" {
				t.Errorf("%s: %+v lacks a kind or a label", tc.name, badge)
			}
		}
		if joined := strings.Join(texts, " "); joined != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, joined, tc.want)
		}
		kinds := make([]string, len(got))
		for i, badge := range got {
			kinds[i] = badge.Kind
		}
		order := []string{BadgeResolution, BadgeRange, BadgeCodec, BadgeAudio, BadgeChannels, BadgeEdition, BadgeLanguage}
		if !slices.IsSortedFunc(kinds, func(a, b string) int { return slices.Index(order, a) - slices.Index(order, b) }) {
			t.Errorf("%s: kinds out of order: %q", tc.name, kinds)
		}
	}
}

// Labels say badges in words, in the language asked, English for
// another: brands as they are, languages as the language names them;
// stereo and mono say so in words on the badge too, and MULTI says
// several languages when none is named.
func TestVersionBadgesSayThemInWords(t *testing.T) {
	t.Parallel()
	version := labeled("4K DV HEVC Atmos 7.1 IMAX 🇫🇷 VFF Multi")
	for _, tc := range []struct {
		language string
		want     []string
	}{
		{"en", []string{"4K Ultra HD", "Dolby Vision", "HEVC video", "Dolby Atmos", "7.1 surround sound", "IMAX edition", "Audio: French"}},
		{"fr", []string{"4K Ultra HD", "Dolby Vision", "Vidéo HEVC", "Dolby Atmos", "Son surround 7.1", "Version IMAX", "Audio\u202f: français"}},
		{"de", []string{"4K Ultra HD", "Dolby Vision", "HEVC-Video", "Dolby Atmos", "7.1-Surround-Sound", "IMAX-Fassung", "Audio: Französisch"}},
		{"xx", []string{"4K Ultra HD", "Dolby Vision", "HEVC video", "Dolby Atmos", "7.1 surround sound", "IMAX edition", "Audio: French"}},
	} {
		badges := VersionBadges(version, tc.language)
		labels := make([]string, len(badges))
		for i, badge := range badges {
			labels[i] = badge.Label
		}
		if !slices.Equal(labels, tc.want) {
			t.Errorf("%s: %q, want %q", tc.language, labels, tc.want)
		}
	}
	if got := VersionBadges(labeled("Movie 2008 Director's Cut 1080p"), "fr"); len(got) != 2 || got[1].Text != "Director’s cut" ||
		got[1].Label != "Version du réalisateur" {
		t.Errorf("a French director’s cut: %+v", got)
	}
	for _, tc := range []struct {
		language, label string
		want            []string
	}{
		{"en", "Movie 2008 AAC 2.0 Multi", []string{"Stereo|Stereo", "MULTI|Several audio languages"}},
		{"fr", "Movie 2008 AAC 2.0 Multi", []string{"Stéréo|Stéréo", "MULTI|Plusieurs langues audio"}},
		{"fr", "Movie 2008 AAC 1.0", []string{"Mono|Mono"}},
		{"es", "Movie 2008 AAC 2.0", []string{"Estéreo|Estéreo"}},
	} {
		var got []string
		for _, badge := range VersionBadges(labeled(tc.label), tc.language) {
			got = append(got, badge.Text+"|"+badge.Label)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%q in %s: %q, want %q", tc.label, tc.language, got, tc.want)
		}
	}
}
