package subtitles

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

const scriptHeader = `[Script Info]
; A comment
ScriptType: v4.00+
WrapStyle: 0

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Default,Arial,20,&H00FFFFFF,&H0000FFFF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,1,1,2,10,10,10,1
Style: Sign,Test,18,&H0000FFFF,&H000000FF,&H00000000,&H00000000,1,0,0,0,100,100,0,0,1,1,0,8,10,10,10,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
`

const scriptEvents = `Dialogue: 0,0:00:01.00,0:00:04.00,Default,,0,0,0,,Hello, world\Nagain
Dialogue: 1,0:00:02.50,0:00:05.00,Sign,Bob,0,0,0,,{\pos(320,40)\i1}A SIGN
Dialogue: 0,1:02:03.04,1:02:05.00,Default,,0,0,0,,Late
`

const scriptFooter = `
[Fonts]
fontname: Test_0.ttf
!!!!!!!!

[Graphics]
`

func TestScriptRoundTrip(t *testing.T) {
	script := scriptHeader + scriptEvents + scriptFooter
	crlf := "\ufeff" + strings.ReplaceAll(script, "\n", "\r\n")
	parsed, err := ParseScript([]byte(crlf))
	if err != nil {
		t.Fatal(err)
	}
	if string(parsed.Header) != scriptHeader || string(parsed.Footer) != scriptFooter {
		t.Errorf("header %q\nfooter %q", parsed.Header, parsed.Footer)
	}
	want := []Event{
		{Start: time.Second, End: 4 * time.Second, Layer: "0", Rest: `Default,,0,0,0,,Hello, world\Nagain`},
		{Start: 2500 * time.Millisecond, End: 5 * time.Second, Layer: "1", Rest: `Sign,Bob,0,0,0,,{\pos(320,40)\i1}A SIGN`},
		{Start: time.Hour + 2*time.Minute + 3*time.Second + 40*time.Millisecond, End: time.Hour + 2*time.Minute + 5*time.Second, Layer: "0", Rest: `Default,,0,0,0,,Late`},
	}
	if !slices.Equal(parsed.Events, want) {
		t.Errorf("events %+v", parsed.Events)
	}
	if got := string(parsed.Bytes()); got != script {
		t.Errorf("round trip:\n%s", got)
	}
}

func TestScriptEncodingsAndOtherEventLines(t *testing.T) {
	script := scriptHeader + "Comment: 0,0:00:00.00,0:00:01.00,Default,,0,0,0,,template\n" +
		"Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,D\xe9j\xe0 \x93vu\x94\n" +
		"Dialogue: 0,0:00:03.00,0:00:02.00,Default,,0,0,0,,Backwards\n" +
		"Dialogue: 0,0:00:03.00,bad,Default,,0,0,0,,Bad time\n" +
		"Dialogue: 0,0:00:03.00,0:00:04.00,Default,,0,0\n"
	parsed, err := ParseScript([]byte(script))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Events) != 1 || parsed.Events[0].Rest != "Default,,0,0,0,,Déjà “vu”" {
		t.Errorf("events %+v", parsed.Events)
	}
}

func TestScriptFrom(t *testing.T) {
	parsed, err := ParseScript([]byte(scriptHeader + scriptEvents + scriptFooter))
	if err != nil {
		t.Fatal(err)
	}
	later := parsed.From(3 * time.Second)
	want := []Event{
		{Start: 0, End: time.Second, Layer: "0", Rest: parsed.Events[0].Rest},
		{Start: 0, End: 2 * time.Second, Layer: "1", Rest: parsed.Events[1].Rest},
		{Start: time.Hour + 2*time.Minute + 40*time.Millisecond, End: time.Hour + 2*time.Minute + 2*time.Second, Layer: "0", Rest: parsed.Events[2].Rest},
	}
	if !slices.Equal(later.Events, want) || string(later.Header) != scriptHeader || string(later.Footer) != scriptFooter {
		t.Errorf("from 3 s: %+v", later.Events)
	}
	if parsed.Events[0].Start != time.Second {
		t.Errorf("original changed: %+v", parsed.Events[0])
	}
	if got := parsed.From(4 * time.Second).Events; len(got) != 2 || got[0].Layer != "1" || got[0].End != time.Second {
		t.Errorf("from 4 s: %+v", got)
	}
	if parsed.From(0) != parsed {
		t.Error("from 0 should keep the script")
	}
}

func TestScriptCues(t *testing.T) {
	events := `Dialogue: 0,0:00:05.00,0:00:06.00,Default,,0,0,0,,Later {\b1}bold{\b0}, soft\nspace\hhard
Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,{\i1}Line one\N{\i0}line two\N\N
Dialogue: 0,0:00:01.00,0:00:02.00,Sign,,0,0,0,,{\p1}m 0 0 l 100 0 100 100 0 100{\p0}
Dialogue: 0,0:00:03.00,0:00:04.00,Sign,,0,0,0,,{\pos(10,10)}SIGN
Dialogue: 1,0:00:03.00,0:00:04.00,Sign,,0,0,0,,{\pos(10,10)\bord0}SIGN
Dialogue: 0,0:00:03.00,0:00:04.00,Sign,,0,0,0,,{\pbo0\p2}m 0 0 l 1 1{\p0}Mixed
Dialogue: 0,0:00:07.00,0:00:08.00,Default,,0,0,0,,{ignored comment}{\q2}Wrapped\nhere {open
Dialogue: 0,0:00:09.00,0:00:09.00,Default,,0,0,0,,Never shown
Dialogue: 0,0:00:09.00,0:00:10.00,Default,,0,0,0,,{\an8}
`
	parsed, err := ParseScript([]byte(scriptHeader + events))
	if err != nil {
		t.Fatal(err)
	}
	want := []Cue{
		{Start: time.Second, End: 2 * time.Second, Lines: []string{"Line one", "line two"}},
		{Start: 3 * time.Second, End: 4 * time.Second, Lines: []string{"SIGN"}},
		{Start: 3 * time.Second, End: 4 * time.Second, Lines: []string{"Mixed"}},
		{Start: 5 * time.Second, End: 6 * time.Second, Lines: []string{"Later bold, soft space hard"}},
		{Start: 7 * time.Second, End: 8 * time.Second, Lines: []string{"Wrapped", "here {open"}},
	}
	if got := parsed.Cues(); !slices.EqualFunc(got, want, func(a, b Cue) bool {
		return a.Start == b.Start && a.End == b.End && slices.Equal(a.Lines, b.Lines)
	}) {
		t.Errorf("got %q", got)
	}
	// With WrapStyle 2, \n breaks lines too.
	wrapped := strings.Replace(scriptHeader, "WrapStyle: 0", "WrapStyle: 2", 1) +
		`Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,Soft\nbreak` + "\n"
	parsed, err = ParseScript([]byte(wrapped))
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.Cues(); len(got) != 1 || !slices.Equal(got[0].Lines, []string{"Soft", "break"}) {
		t.Errorf("wrap style 2: %q", got)
	}
}

// Jellyfin 12.2 writes an ASS track as WebVTT with override blocks
// removed, \N as line breaks and \h as a space: Cues gives the same text.
func TestScriptCuesMatchJellyfin(t *testing.T) {
	ass, err := os.ReadFile("testdata/ass-track.ass")
	if err != nil {
		t.Fatal(err)
	}
	vtt, err := os.ReadFile("testdata/ass-track.vtt")
	if err != nil {
		t.Fatal(err)
	}
	script, err := ParseScript(ass)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(string(WebVTT(script.Cues()))), strings.TrimSpace(string(vtt)); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	// Jellyfin serves the script as it is.
	if got := script.Bytes(); string(got) != string(ass) {
		t.Errorf("script changed:\n%s", got)
	}
	// Parse reads ASS files into the same cues.
	cues, err := Parse(ass)
	if err != nil || len(cues) != 3 || !slices.Equal(cues[2].Lines, []string{"Italic words with a hard space"}) {
		t.Errorf("parse: %q, %v", cues, err)
	}
}

func TestScriptsOnly(t *testing.T) {
	for _, data := range []string{"", "WEBVTT\n", "1\n00:00:01,000 --> 00:00:02,000\nHi\n", "<html>[Script Info]</html>", "\n\n[V4+ Styles]\n"} {
		if _, err := ParseScript([]byte(data)); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%q: got %v", data, err)
		}
		if _, err := MatroskaScript([]byte(data), nil); !errors.Is(err, ErrUnsupported) {
			t.Errorf("matroska %q: got %v", data, err)
		}
	}
}

func TestScriptsMissingParts(t *testing.T) {
	standard := "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
	line := "Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,Hi\n"
	for _, test := range []struct{ name, in, header, footer string }{
		{"no events section", "[Script Info]\nTitle: x\n", "[Script Info]\nTitle: x\n\n[Events]\n" + standard, ""},
		{"no format, events", "[Script Info]\n\n[Events]\n" + line, "[Script Info]\n\n[Events]\n" + standard, ""},
		{"no format, no events", "[Script Info]\n\n[Events]", "[Script Info]\n\n[Events]\n" + standard, ""},
		{"no format, footer", "[Script Info]\n\n[Events]\n\n[Fonts]\n", "[Script Info]\n\n[Events]\n" + standard, "\n[Fonts]\n"},
		{"format at the end", "[Script Info]\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text",
			"[Script Info]\n[Events]\n" + standard, ""},
		{"ssa", "[Script Info]\n[V4 Styles]\n[Events]\n" + line, "[Script Info]\n[V4 Styles]\n[Events]\nFormat: Marked, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n", ""},
	} {
		script, err := ParseScript([]byte(test.in))
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		if string(script.Header) != test.header || string(script.Footer) != test.footer {
			t.Errorf("%s: header %q, footer %q", test.name, script.Header, script.Footer)
		}
	}
	script, _ := ParseScript([]byte("[Script Info]\n\n[Events]\n" + line))
	if len(script.Events) != 1 || script.Events[0].Rest != "Default,,0,0,0,,Hi" {
		t.Errorf("events without format: %+v", script.Events)
	}
}

func TestScriptFormatInAnotherOrder(t *testing.T) {
	in := "[Script Info]\n[Events]\nFormat: Start, End, Style, Text\nDialogue: 0:00:01.00,0:00:02.00,Sign,Hi, there\n"
	script, err := ParseScript([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	want := "[Script Info]\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n" +
		"Dialogue: 0,0:00:01.00,0:00:02.00,Sign,,0,0,0,,Hi, there\n"
	if got := string(script.Bytes()); got != want {
		t.Errorf("got %q", got)
	}
	// A Format line without Start is read as the standard one.
	script, _ = ParseScript([]byte("[Script Info]\n[Events]\nFormat: Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,Hi\n"))
	if len(script.Events) != 1 || !strings.HasSuffix(string(script.Header), "Effect, Text\n") {
		t.Errorf("format without start: %q %+v", script.Header, script.Events)
	}
}

func TestHostileScripts(t *testing.T) {
	huge := strings.Repeat("x", 8<<20)
	in := scriptHeader + "Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,," + huge + "\n" +
		"Dialogue: 0,99999999999:00:00.00,99999999999:00:01.00,Default,,0,0,0,,Overflow\n" +
		"Dialogue: 0,0:99999999999:00.00,0:00:01.00,Default,,0,0,0,,Overflow\n" +
		"Dialogue: 0,0:00:00.99999999999,0:00:01.00,Default,,0,0,0,,Overflow\n" +
		"Dialogue: 0,-0:00:01.00,0:00:01.00,Default,,0,0,0,,Negative\n" +
		"Dialogue: " + strings.Repeat(",", 1<<20) + "\n" +
		"Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,{" + huge + "\n" +
		"[" + huge + "]\n"
	script, err := ParseScript([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(script.Events) != 2 {
		t.Fatalf("got %d events", len(script.Events))
	}
	if cues := script.Cues(); len(cues) != 2 || len(cues[0].Lines[0]) != len(huge) {
		t.Errorf("got %d cues", len(cues))
	}
}

func TestMatroskaScript(t *testing.T) {
	private := "\ufeff[Script Info]\r\nScriptType: v4.00+\r\n\r\n[V4+ Styles]\r\nStyle: Default,Arial,20\r\n\r\n[Events]\r\n" +
		"Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\r\n\x00"
	blocks := []MatroskaEvent{
		{Start: 3 * time.Second, Duration: time.Second, Data: []byte(`2,0,Default,,0,0,0,,Third, with commas`)},
		{Start: 1 * time.Second, Duration: 2 * time.Second, Data: []byte(`1,1,Sign,Bob,10,20,30,Fade,{\pos(1,2)}Sign`)},
		{Start: 500 * time.Millisecond, Duration: time.Second, Data: []byte(`1,0,Default,,0,0,0,,Same order, earlier`)},
		{Start: 0, Duration: time.Second, Data: []byte(`0,0,Default,,0,0,0,,Two` + "\r\n" + `lines` + "\r\n")},
		{Start: 0, Duration: time.Second, Data: []byte(`x,0,Default,,0,0,0,,Bad read order`)},
		{Start: 0, Duration: time.Second, Data: []byte(`5,0,Default,Too few`)},
	}
	script, err := MatroskaScript([]byte(private), blocks)
	if err != nil {
		t.Fatal(err)
	}
	want := "[Script Info]\nScriptType: v4.00+\n\n[V4+ Styles]\nStyle: Default,Arial,20\n\n[Events]\n" +
		"Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n" +
		`Dialogue: 0,0:00:00.00,0:00:01.00,Default,,0,0,0,,Two\Nlines` + "\n" +
		`Dialogue: 0,0:00:00.50,0:00:01.50,Default,,0,0,0,,Same order, earlier` + "\n" +
		`Dialogue: 1,0:00:01.00,0:00:03.00,Sign,Bob,10,20,30,Fade,{\pos(1,2)}Sign` + "\n" +
		`Dialogue: 0,0:00:03.00,0:00:04.00,Default,,0,0,0,,Third, with commas` + "\n"
	if got := string(script.Bytes()); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}

	// A CodecPrivate without the Format line, or with another one, gets
	// the standard one, the order of the fields of blocks.
	for _, private := range []string{
		"[Script Info]\n[V4+ Styles]\n[Events]\n",
		"[Script Info]\n[V4+ Styles]\n",
		"[Script Info]\n[V4+ Styles]\n[Events]\nFormat: Layer, Start, End, Style, MarginL, MarginR, MarginV, Effect, Text",
	} {
		script, err := MatroskaScript([]byte(private), blocks[:1])
		if err != nil {
			t.Fatal(err)
		}
		if got := string(script.Bytes()); !strings.HasSuffix(got, "[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"+
			"Dialogue: 0,0:00:03.00,0:00:04.00,Default,,0,0,0,,Third, with commas\n") {
			t.Errorf("%q: got\n%s", private, got)
		}
	}
	ssa, err := MatroskaScript([]byte("[Script Info]\nScriptType: v4.00\n[V4 Styles]\n[Events]\nFormat: Marked, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n[Fonts]\nfontname: a.ttf\n"),
		[]MatroskaEvent{{Start: time.Second, Duration: time.Second, Data: []byte("0,Marked=0,Default,,0,0,0,,SSA")}})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(ssa.Bytes()); !strings.HasSuffix(got, "Format: Marked, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"+
		"Dialogue: Marked=0,0:00:01.00,0:00:02.00,Default,,0,0,0,,SSA\n[Fonts]\nfontname: a.ttf\n") {
		t.Errorf("ssa: got\n%s", got)
	}
}
