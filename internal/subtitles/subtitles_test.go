package subtitles

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSubRipBecomesWebVTT(t *testing.T) {
	srt := "\ufeff1\r\n00:00:01,500 --> 00:00:03,250\r\n<i>Bonjour</i>\r\nà tous\r\n\r\n2\r\n01:02:03,004 --> 01:02:05,000\r\nFin\r\n"
	cues, err := Parse([]byte(srt))
	if err != nil {
		t.Fatal(err)
	}
	want := "WEBVTT\n\n00:00:01.500 --> 00:00:03.250\n<i>Bonjour</i>\nà tous\n\n01:02:03.004 --> 01:02:05.000\nFin\n"
	if got := string(WebVTT(cues)); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWebVTTBecomesSubRip(t *testing.T) {
	// Addons repeat the header, add notes, identifiers, cue settings and
	// short timestamps without hours.
	vtt := "WEBVTT\n\nWEBVTT\n\nNOTE made by an addon\n\nintro\n00:04.120 --> 00:08.690 align:start line:90%\n- Hello\n- Hi\n\n00:00:09.000 --> 00:00:10.5\nBye\n"
	cues, err := Parse([]byte(vtt))
	if err != nil {
		t.Fatal(err)
	}
	want := "1\n00:00:04,120 --> 00:00:08,690\n- Hello\n- Hi\n\n2\n00:00:09,000 --> 00:00:10,500\nBye\n"
	if got := string(SubRip(cues)); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWindows1252IsDecoded(t *testing.T) {
	srt := []byte("1\n00:00:01,000 --> 00:00:02,000\nD\xe9j\xe0 vu \x85 \x93cool\x94\n")
	cues, err := Parse(srt)
	if err != nil {
		t.Fatal(err)
	}
	if got := cues[0].Lines; !slices.Equal(got, []string{"Déjà vu … “cool”"}) {
		t.Errorf("got %q", got)
	}
}

func TestBrokenCuesAreSkipped(t *testing.T) {
	srt := "1\n00:00:05,000 --> 00:00:04,000\nBackwards\n\n2\nnot a timing\nText\n\n3\n00:00:06,000 --> 00:00:07,000\nKept\n"
	cues, err := Parse([]byte(srt))
	if err != nil || len(cues) != 1 || cues[0].Start != 6*time.Second {
		t.Fatalf("got %+v, %v", cues, err)
	}
}

func TestOtherFilesAreRefused(t *testing.T) {
	for _, data := range []string{"<!DOCTYPE html><html></html>", "", "[Script Info]\nTitle: ASS"} {
		if _, err := Parse([]byte(data)); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%q: got %v", data, err)
		}
	}
	// An empty WebVTT file is valid.
	if cues, err := Parse([]byte("WEBVTT\n")); err != nil || len(cues) != 0 {
		t.Errorf("empty WebVTT: %v %v", cues, err)
	}
}

func TestPlayerFormats(t *testing.T) {
	cues := []Cue{
		{Start: time.Second, End: 4 * time.Second, Lines: []string{"Hello"}},
		{Start: 5 * time.Second, End: 8 * time.Second, Lines: []string{"<i>Second</i>", "line"}},
	}
	wantJSON := `{"TrackEvents":[{"Id":"1","Text":"Hello","StartPositionTicks":10000000,"EndPositionTicks":40000000},` +
		`{"Id":"2","Text":"\u003ci\u003eSecond\u003c/i\u003e\nline","StartPositionTicks":50000000,"EndPositionTicks":80000000}]}`
	if got := string(TrackEvents(cues)); got != wantJSON {
		t.Errorf("track events: got %s", got)
	}
	ass := string(ASS(cues))
	if !strings.HasPrefix(ass, "[Script Info]") || !strings.HasSuffix(ass,
		"Dialogue: 0,0:00:01.00,0:00:04.00,Default,,0,0,0,,Hello\nDialogue: 0,0:00:05.00,0:00:08.00,Default,,0,0,0,,{\\i1}Second{\\i0}\\Nline\n") {
		t.Errorf("ass: got %q", ass)
	}
	// Playback starting at 6 s keeps the second cue, from 0 to 2 s.
	later := From(cues, 6*time.Second)
	if len(later) != 1 || later[0].Start != 0 || later[0].End != 2*time.Second || cues[1].Start != 5*time.Second {
		t.Errorf("from 6 s: %+v (original %+v)", later, cues[1])
	}
}
