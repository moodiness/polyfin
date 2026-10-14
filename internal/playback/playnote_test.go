package playback

import (
	"testing"
	"unicode/utf8"
)

// A name's ending tells how a decision plays a version, with the first
// reason that applies, in English or French, 30 characters at most.
func TestPlayNoteNamesTheFirstReason(t *testing.T) {
	video, audio := &VideoConversion{}, &AudioConversion{}
	burned := map[int]SubtitleDelivery{2: {Method: "Encode"}}
	for _, tc := range []struct {
		decision     Decision
		playable     bool
		english, fre string
	}{
		{Decision{DirectPlay: true}, true, "(direct play)", "(lecture directe)"},
		{Decision{HLS: true, Reasons: []string{"ContainerNotSupported"}}, true, "(direct stream)", "(flux direct)"},
		{Decision{HLS: true, Video: video, Reasons: []string{"VideoCodecNotSupported", "VideoRangeTypeNotSupported"}}, true,
			"(converted: HDR)", "(converti\u00a0: HDR)"},
		{Decision{HLS: true, Video: video, Reasons: []string{"VideoCodecNotSupported", "VideoResolutionNotSupported"}}, true,
			"(converted: quality limit)", "(converti\u00a0: limite de qualité)"},
		{Decision{HLS: true, Video: video, Reasons: []string{"ContainerBitrateExceedsLimit"}}, true,
			"(converted: quality limit)", "(converti\u00a0: limite de qualité)"},
		{Decision{HLS: true, Video: video, Reasons: []string{"SubtitleCodecNotSupported"}, SubtitleStreamIndex: 2, Subtitles: burned}, true,
			"(converted: subtitles)", "(converti\u00a0: sous-titres)"},
		{Decision{HLS: true, Video: video, Reasons: []string{"VideoProfileNotSupported"}, SubtitleStreamIndex: -1}, true,
			"(converted: video)", "(converti\u00a0: vidéo)"},
		{Decision{HLS: true, Audio: audio, Reasons: []string{"AudioCodecNotSupported"}, SubtitleStreamIndex: -1}, true,
			"(converted: audio)", "(converti\u00a0: audio)"},
		{Decision{HLS: true, Video: video, Reasons: []string{"ContainerNotSupported"}, SubtitleStreamIndex: -1}, true, "(converted)", "(converti)"},
		{Decision{HLS: true, Video: video}, false, "(cannot play here)", "(illisible ici)"},
		{Decision{Reasons: []string{"VideoCodecNotSupported"}}, true, "(cannot play here)", "(illisible ici)"},
	} {
		if got := PlayNote(tc.decision, tc.playable, "en"); got != tc.english {
			t.Errorf("%+v: %q, want %q", tc.decision, got, tc.english)
		}
		got := PlayNote(tc.decision, tc.playable, "fr")
		if got != tc.fre {
			t.Errorf("%+v in French: %q, want %q", tc.decision, got, tc.fre)
		}
		if utf8.RuneCountInString(got) > 30 || utf8.RuneCountInString(tc.english) > 30 {
			t.Errorf("%q or %q is longer than 30 characters", got, tc.english)
		}
	}
}
