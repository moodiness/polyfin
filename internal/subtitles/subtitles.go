// Package subtitles reads text subtitles in the SubRip (SRT) and WebVTT
// formats addons serve, and writes them in the formats Jellyfin apps ask
// for: SubRip, WebVTT, Advanced SubStation Alpha and jellyfin-web's JSON.
package subtitles

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrUnsupported reports a file that is neither SubRip nor WebVTT.
var ErrUnsupported = errors.New("unsupported subtitle format")

// Cue is a text shown between two instants.
type Cue struct {
	Start, End time.Duration
	// Lines hold the text, with the inline tags both formats share (<i>,
	// <b>, <u>) kept as they are.
	Lines []string
}

// Parse reads a SubRip or WebVTT file. Text that is not valid UTF-8 is read
// as Windows-1252, the usual encoding of subtitles that are not UTF-8.
func Parse(data []byte) ([]Cue, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(data) {
		data = fromWindows1252(data)
	}
	text := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
	var cues []Cue
	for block := range strings.SplitSeq(text, "\n\n") {
		lines := strings.Split(strings.Trim(block, "\n"), "\n")
		timing := -1
		for i, line := range lines {
			if strings.Contains(line, "-->") {
				timing = i
				break
			}
		}
		// Blocks without timing are headers, notes, styles or indexes on
		// their own; a cue identifier may precede the timing.
		if timing < 0 || timing > 1 {
			continue
		}
		start, end, ok := parseTiming(lines[timing])
		if !ok {
			continue
		}
		var text []string
		for _, line := range lines[timing+1:] {
			if line = strings.TrimRight(line, " \t"); line != "" {
				text = append(text, line)
			}
		}
		if len(text) > 0 {
			cues = append(cues, Cue{Start: start, End: end, Lines: text})
		}
	}
	if len(cues) == 0 && !strings.HasPrefix(strings.TrimSpace(text), "WEBVTT") {
		return nil, ErrUnsupported
	}
	return cues, nil
}

// parseTiming reads "start --> end", with SubRip commas or WebVTT dots
// before milliseconds, optional hours, and WebVTT cue settings after.
func parseTiming(line string) (time.Duration, time.Duration, bool) {
	left, right, _ := strings.Cut(line, "-->")
	fields := strings.Fields(right)
	if len(fields) == 0 {
		return 0, 0, false
	}
	start, ok := parseTimestamp(strings.TrimSpace(left))
	if !ok {
		return 0, 0, false
	}
	end, ok := parseTimestamp(fields[0])
	return start, end, ok && end >= start
}

func parseTimestamp(value string) (time.Duration, bool) {
	value = strings.Replace(value, ",", ".", 1)
	clock, fraction, _ := strings.Cut(value, ".")
	parts := strings.Split(clock, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	var total time.Duration
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return 0, false
		}
		total = total*60 + time.Duration(n)
	}
	total *= time.Second
	if fraction != "" {
		// Milliseconds, whatever the number of digits given.
		fraction = (fraction + "000")[:3]
		ms, err := strconv.Atoi(fraction)
		if err != nil {
			return 0, false
		}
		total += time.Duration(ms) * time.Millisecond
	}
	return total, true
}

// WebVTT writes cues as a WebVTT file.
func WebVTT(cues []Cue) []byte {
	var out bytes.Buffer
	out.WriteString("WEBVTT\n")
	for _, cue := range cues {
		fmt.Fprintf(&out, "\n%s --> %s\n", timestamp(cue.Start, '.'), timestamp(cue.End, '.'))
		for _, line := range cue.Lines {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	return out.Bytes()
}

func timestamp(d time.Duration, separator byte) string {
	ms := d.Milliseconds()
	return fmt.Sprintf("%02d:%02d:%02d%c%03d", ms/3_600_000, ms/60_000%60, ms/1000%60, separator, ms%1000)
}

// From keeps the cues that end after start, shifted so that start becomes
// zero, as players ask when they start playback later in a video.
func From(cues []Cue, start time.Duration) []Cue {
	if start <= 0 {
		return cues
	}
	result := make([]Cue, 0, len(cues))
	for _, cue := range cues {
		if cue.End <= start {
			continue
		}
		cue.Start, cue.End = max(cue.Start-start, 0), cue.End-start
		result = append(result, cue)
	}
	return result
}

// TrackEvents writes cues as the JSON jellyfin-web draws subtitles from:
// times in ticks of 100 ns, text with its inline tags.
func TrackEvents(cues []Cue) []byte {
	type event struct {
		Id                 string
		Text               string
		StartPositionTicks int64
		EndPositionTicks   int64
	}
	events := make([]event, len(cues))
	for i, cue := range cues {
		events[i] = event{Id: strconv.Itoa(i + 1), Text: strings.Join(cue.Lines, "\n"),
			StartPositionTicks: int64(cue.Start / 100), EndPositionTicks: int64(cue.End / 100)}
	}
	data, _ := json.Marshal(struct{ TrackEvents []event }{events})
	return data
}

// assHeader is a minimal Advanced SubStation Alpha script with one default
// style.
const assHeader = `[Script Info]
ScriptType: v4.00+
ScaledBorderAndShadow: Yes

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Default,Arial,20,&H00FFFFFF,&H0000FFFF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,1,1,2,10,10,10,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
`

// assTags turns the inline tags SubRip and WebVTT share into override tags.
var assTags = strings.NewReplacer("<i>", `{\i1}`, "</i>", `{\i0}`, "<b>", `{\b1}`, "</b>", `{\b0}`, "<u>", `{\u1}`, "</u>", `{\u0}`)

// ASS writes cues as an Advanced SubStation Alpha script.
func ASS(cues []Cue) []byte {
	var out bytes.Buffer
	out.WriteString(assHeader)
	for _, cue := range cues {
		fmt.Fprintf(&out, "Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n", assTime(cue.Start), assTime(cue.End),
			assTags.Replace(strings.Join(cue.Lines, `\N`)))
	}
	return out.Bytes()
}

func assTime(d time.Duration) string {
	cs := d.Milliseconds() / 10
	return fmt.Sprintf("%d:%02d:%02d.%02d", cs/360_000, cs/6000%60, cs/100%60, cs%100)
}

// SubRip writes cues as a SubRip file.
func SubRip(cues []Cue) []byte {
	var out bytes.Buffer
	for i, cue := range cues {
		if i > 0 {
			out.WriteByte('\n')
		}
		fmt.Fprintf(&out, "%d\n%s --> %s\n", i+1, timestamp(cue.Start, ','), timestamp(cue.End, ','))
		for _, line := range cue.Lines {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	return out.Bytes()
}

// windows1252 maps the bytes 0x80 to 0x9F, where Windows-1252 differs from
// Latin-1; zero marks bytes it leaves undefined.
var windows1252 = [32]rune{
	'€', 0, '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', 0, 'Ž', 0,
	0, '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', 0, 'ž', 'Ÿ',
}

func fromWindows1252(data []byte) []byte {
	out := make([]byte, 0, len(data)+len(data)/8)
	for _, b := range data {
		r := rune(b)
		if b >= 0x80 && b < 0xa0 {
			if r = windows1252[b-0x80]; r == 0 {
				r = utf8.RuneError
			}
		}
		out = utf8.AppendRune(out, r)
	}
	return out
}
