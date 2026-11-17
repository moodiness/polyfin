package playback

import (
	"fmt"
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
		// The app turned direct play off for a version it plays as it is:
		// the conversion is its request, not its codec's.
		{Decision{HLS: true, Video: video, Reasons: []string{"VideoCodecNotSupported", "DirectPlayError"}, DirectPlayOff: true, SubtitleStreamIndex: -1}, true,
			"(converted: app’s request)", "(converti\u202f: demande de l’app)"},
		{Decision{HLS: true, Video: video, Reasons: []string{"VideoCodecNotSupported", "VideoRangeTypeNotSupported"}}, true,
			"(converted: HDR)", "(converti\u202f: HDR)"},
		{Decision{HLS: true, Video: video, Reasons: []string{"VideoCodecNotSupported", "VideoResolutionNotSupported"}}, true,
			"(converted: quality limit)", "(converti\u202f: limite de qualité)"},
		{Decision{HLS: true, Video: video, Reasons: []string{"ContainerBitrateExceedsLimit"}}, true,
			"(converted: quality limit)", "(converti\u202f: limite de qualité)"},
		{Decision{HLS: true, Video: video, Reasons: []string{"SubtitleCodecNotSupported"}, SubtitleStreamIndex: 2, Subtitles: burned}, true,
			"(converted: subtitles)", "(converti\u202f: sous-titres)"},
		{Decision{HLS: true, Video: video, Reasons: []string{"VideoProfileNotSupported"}, SubtitleStreamIndex: -1}, true,
			"(converted: video)", "(converti\u202f: vidéo)"},
		{Decision{HLS: true, Audio: audio, Reasons: []string{"AudioCodecNotSupported"}, SubtitleStreamIndex: -1}, true,
			"(converted: audio)", "(converti\u202f: audio)"},
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

// An estimate made from a version's labels says so: the same ending as
// the exact one, worded as likely, in English and French, and at most 45
// characters in every language, whatever the reason.
func TestLikelyNoteWordsTheEstimate(t *testing.T) {
	t.Parallel()
	video, audio := &VideoConversion{}, &AudioConversion{}
	for _, tc := range []struct {
		decision     Decision
		playable     bool
		english, fre string
	}{
		{Decision{DirectPlay: true}, true, "(likely direct play)", "(lecture directe probable)"},
		{Decision{HLS: true, Reasons: []string{"ContainerNotSupported"}}, true, "(likely direct stream)", "(flux direct probable)"},
		{Decision{HLS: true, Video: video, Reasons: []string{"VideoRangeTypeNotSupported"}}, true,
			"(likely converted: HDR)", "(conversion probable\u202f: HDR)"},
		{Decision{HLS: true, Audio: audio, Reasons: []string{"AudioCodecNotSupported"}, SubtitleStreamIndex: -1}, true,
			"(likely converted: audio)", "(conversion probable\u202f: audio)"},
		{Decision{HLS: true, Video: video, Reasons: []string{"ContainerNotSupported"}, SubtitleStreamIndex: -1}, true,
			"(likely converted)", "(conversion probable)"},
		{Decision{HLS: true, Video: video}, false, "(likely cannot play here)", "(probablement illisible ici)"},
	} {
		if got := LikelyNote(tc.decision, tc.playable, "en"); got != tc.english {
			t.Errorf("%+v: %q, want %q", tc.decision, got, tc.english)
		}
		if got := LikelyNote(tc.decision, tc.playable, "fr"); got != tc.fre {
			t.Errorf("%+v in French: %q, want %q", tc.decision, got, tc.fre)
		}
	}
	longest := Decision{HLS: true, Video: video, Reasons: []string{"DirectPlayError"}, DirectPlayOff: true}
	for language, words := range playNotes {
		for _, reason := range []string{words.app, words.hdr, words.quality, words.subtitles, words.video, words.audio} {
			for _, ending := range []string{fmt.Sprintf(likelyNotes[language].convertedFor, reason), likelyNotes[language].cannotPlay} {
				if utf8.RuneCountInString(ending) > 45 {
					t.Errorf("%s: %q is longer than 45 characters", language, ending)
				}
			}
		}
		if got := LikelyNote(longest, true, language); utf8.RuneCountInString(got) > 45 || got == PlayNote(longest, true, language) {
			t.Errorf("%s: %q", language, got)
		}
	}
}
