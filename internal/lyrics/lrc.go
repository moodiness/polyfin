package lyrics

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	// timeTag matches the time tag starting a line of LRC, when the line
	// is sung: [mm:ss], [mm:ss.xx] or [mm:ss.xxx], the fraction also
	// after a colon. A line sung several times starts with several.
	timeTag = regexp.MustCompile(`^\[(\d{1,4}):(\d{1,2})(?:[.:](\d{1,3}))?\]`)
	// wordTag matches the time tags of words in enhanced LRC, which line
	// by line lyrics leave out.
	wordTag = regexp.MustCompile(`<\d{1,4}:\d{1,2}(?:[.:]\d{1,3})?>`)
)

// parseLRC reads LRC lyrics into lines, sorted by their start, as Jellyfin
// reads a track's LRC file: each time tag of a line starts a line of its
// text, trimmed, empty ones included (they mark pauses). Lines without a
// time tag, such as the [ar:] or [ti:] tags of the file's description, are
// left out.
func parseLRC(text string) []Line {
	var lines []Line
	for _, raw := range splitLines(text) {
		rest := strings.TrimSpace(raw)
		var starts []time.Duration
		for {
			match := timeTag.FindStringSubmatch(rest)
			if match == nil {
				break
			}
			starts = append(starts, tagTime(match[1], match[2], match[3]))
			rest = rest[len(match[0]):]
		}
		words := strings.TrimSpace(wordTag.ReplaceAllString(rest, ""))
		for _, start := range starts {
			lines = append(lines, Line{Text: words, Start: start})
		}
	}
	slices.SortStableFunc(lines, func(a, b Line) int { return cmp.Compare(a.Start, b.Start) })
	return lines
}

// tagTime is the time of a time tag's minutes, seconds and fraction of a
// second, in hundredths for two digits as LRC usually writes it, tenths
// for one and thousandths for three.
func tagTime(minutes, seconds, fraction string) time.Duration {
	m, _ := strconv.Atoi(minutes)
	s, _ := strconv.Atoi(seconds)
	at := time.Duration(m)*time.Minute + time.Duration(s)*time.Second
	if fraction != "" {
		f, _ := strconv.Atoi(fraction)
		for range 3 - len(fraction) {
			f *= 10
		}
		at += time.Duration(f) * time.Millisecond
	}
	return at
}

// plainLines reads plain lyrics into lines, as Jellyfin reads a text
// file of lyrics: each line trimmed, empty ones between verses kept.
func plainLines(text string) []Line {
	raw := splitLines(strings.TrimSpace(text))
	lines := make([]Line, len(raw))
	for i, line := range raw {
		lines[i] = Line{Text: strings.TrimSpace(line)}
	}
	return lines
}

// splitLines splits text at its line breaks, of any system.
func splitLines(text string) []string {
	return strings.Split(strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(text), "\n")
}
