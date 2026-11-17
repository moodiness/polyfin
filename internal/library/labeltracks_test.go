package library

import (
	"slices"
	"testing"
	"time"
)

// The audio codecs labels name, each once, in order: words, words with
// their channels, runs of words and "DD+"; Atmos alone and "DD" without
// channels name none, nor do the title's own words.
func TestReadAudioCodecsNamesEachCodec(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		labels []string
		want   []string
	}{
		{[]string{"Movie.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265.mkv"}, []string{LabelEAC3}},
		{[]string{"🎧 Atmos • DD+ 🔊 5.1"}, []string{LabelEAC3}},
		{[]string{"BluRay TrueHD 7.1 Atmos x265"}, []string{LabelTrueHD}},
		{[]string{"Movie 1080p BluRay DTS-HD MA 5.1 x264"}, []string{LabelDTSMA}},
		{[]string{"Movie DTS HD 5.1"}, []string{LabelDTSHD}},
		{[]string{"Movie.DTS-X.7.1"}, []string{LabelDTSMA}},
		{[]string{"Movie.1080p.MULTi.AAC2.0.AC3.5.1.x264"}, []string{LabelAAC, LabelAC3}},
		{[]string{"Movie 1080p WEB DD 5.1 H264", "E-AC-3"}, []string{LabelAC3, LabelEAC3}},
		{[]string{"Movie.Dolby.Digital.Plus.5.1"}, []string{LabelEAC3}},
		{[]string{"Movie 720p FLAC Opus"}, []string{LabelFLAC, LabelOpus}},
		{[]string{"Movie 2160p Atmos"}, nil},
		{[]string{"Movie DD x264"}, nil},
		{[]string{"Movie 1080p DTS"}, []string{LabelDTS}},
	} {
		if got := ReadAudioCodecs(nil, tc.labels...); !slices.Equal(got, tc.want) {
			t.Errorf("%q: %v, want %v", tc.labels, got, tc.want)
		}
	}
	if got := ReadAudioCodecs([]string{"FLAC Story"}, "FLAC Story 2026 1080p AAC"); !slices.Equal(got, []string{LabelAAC}) {
		t.Errorf("a title naming a codec: %v", got)
	}
}

// Labels naming Dolby Vision with HDR tell an HDR10 base layer; Dolby
// Vision alone tells none.
func TestReadLabelsTellsDolbyVisionOverHDR10(t *testing.T) {
	t.Parallel()
	if labels := ReadLabels("", "Movie.2160p.DV.HDR10.H265"); labels.HDR != "dv" || !labels.HDR10Base {
		t.Errorf("DV HDR10: %+v", labels)
	}
	if labels := ReadLabels("", "Movie.2160p.DoVi.H265"); labels.HDR != "dv" || labels.HDR10Base {
		t.Errorf("DV alone: %+v", labels)
	}
	if labels := ReadLabels("", "Movie.2160p.HDR10.H265"); labels.HDR10Base {
		t.Errorf("HDR10 alone: %+v", labels)
	}
}

// The analyses labels allow: none unless they name the video codec and
// an audio codec; one for each way of filling what they leave open.
func TestLabelAnalysesFillWhatLabelsLeaveOpen(t *testing.T) {
	t.Parallel()
	version := func(filename string, height int, labels Labels) Version {
		return Version{Filename: filename, Height: height, Size: 9_000_000_000, Labels: labels}
	}
	if got := LabelAnalyses(version("Movie.mkv", 1080, Labels{Codec: "h264"}), time.Hour); got != nil {
		t.Errorf("no audio codec: %d analyses", len(got))
	}
	if got := LabelAnalyses(version("Movie.mkv", 1080, Labels{AudioCodecs: []string{LabelAAC}}), time.Hour); got != nil {
		t.Errorf("no video codec: %d analyses", len(got))
	}
	// Everything told: one analysis, of the file's container, its bitrate
	// from its size and the runtime.
	known := LabelAnalyses(version("Movie.mp4", 1080, Labels{Codec: "h264", Range: StandardRange, AudioCodecs: []string{LabelAAC},
		Audio: AudioFormat{Channels: 2}}), time.Hour)
	if len(known) != 1 || known[0].Format != "mov,mp4,m4a,3gp,3g2,mj2" || known[0].Bitrate != 20_000_000 ||
		known[0].Streams[0].Height != 1080 || known[0].Streams[1].Codec != "aac" || known[0].Streams[1].Channels != 2 {
		t.Fatalf("all told: %+v", known)
	}
	// Open: container (2), height (2), channels (2), HEVC's bit depth
	// (2), and two audio codecs.
	open := LabelAnalyses(version("Movie", 0, Labels{Codec: "hevc", AudioCodecs: []string{LabelAAC, LabelDTSMA}}), 0)
	if len(open) != 2*2*2*2*2 {
		t.Errorf("open: %d analyses", len(open))
	}
	if dts := open[len(open)-1].Streams[1]; dts.Codec != "dts" || dts.Profile != "DTS-HD MA" {
		t.Errorf("DTS-HD MA: %+v", dts)
	}
	// Dolby Vision: profiles 5, 7 and 8.1; 7 and 8.1 over HDR10.
	dv := Labels{Codec: "hevc", Range: HighRange, HDR: "dv", AudioCodecs: []string{LabelEAC3}, Audio: AudioFormat{Channels: 6}}
	profiles := func(labels Labels) []int {
		var got []int
		for _, analysis := range LabelAnalyses(version("Movie.mkv", 2160, labels), time.Hour) {
			video := analysis.Streams[0]
			if video.ColorTransfer != "smpte2084" || video.Profile != "Main 10" || video.DolbyVision == nil {
				t.Errorf("Dolby Vision track: %+v", video)
				continue
			}
			got = append(got, video.DolbyVision.Profile)
		}
		return got
	}
	if got := profiles(dv); !slices.Equal(got, []int{5, 7, 8}) {
		t.Errorf("Dolby Vision: profiles %v", got)
	}
	dv.HDR10Base = true
	if got := profiles(dv); !slices.Equal(got, []int{7, 8}) {
		t.Errorf("Dolby Vision over HDR10: profiles %v", got)
	}
	// Too much left open tells nothing.
	many := Labels{Codec: "hevc", Range: HighRange, HDR: "dv", AudioCodecs: []string{LabelAAC, LabelAC3, LabelEAC3, LabelDTS, LabelTrueHD}}
	if got := LabelAnalyses(version("Movie", 0, many), 0); got != nil {
		t.Errorf("too open: %d analyses", len(got))
	}
}
