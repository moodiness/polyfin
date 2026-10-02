package jellyfin

import (
	"testing"

	"github.com/moodiness/polyfin/internal/playback"
)

func TestTheDefaultSubtitleAvoidsBurningIn(t *testing.T) {
	subtitle := func(index int, language string, text, forced, external bool) playback.MediaStream {
		return playback.MediaStream{Type: "Subtitle", Index: index, Language: language, IsTextSubtitleStream: text, IsForced: forced, IsExternal: external}
	}
	video := playback.MediaStream{Type: "Video", Index: 1}
	for name, test := range map[string]struct {
		streams []playback.MediaStream
		want    *int
	}{
		"no subtitles":                          {[]playback.MediaStream{video}, nil},
		"none forced":                           {[]playback.MediaStream{video, subtitle(2, "fre", false, false, false)}, new(-1)},
		"a forced file from an addon":           {[]playback.MediaStream{subtitle(0, "fre", true, true, true), video}, new(-1)},
		"a forced image track":                  {[]playback.MediaStream{video, subtitle(2, "fre", false, true, false)}, new(2)},
		"a forced text track after the image":   {[]playback.MediaStream{video, subtitle(2, "fre", false, true, false), subtitle(3, "fre", true, true, false)}, new(3)},
		"a forced text track in another tongue": {[]playback.MediaStream{video, subtitle(2, "fre", false, true, false), subtitle(3, "eng", true, true, false)}, new(2)},
		"a text track not forced":               {[]playback.MediaStream{video, subtitle(2, "fre", false, true, false), subtitle(3, "fre", true, false, false)}, new(2)},
	} {
		got := defaultSubtitle(test.streams)
		if (got == nil) != (test.want == nil) || got != nil && *got != *test.want {
			t.Errorf("%s: %v, want %v", name, deref(got), deref(test.want))
		}
	}
}

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
