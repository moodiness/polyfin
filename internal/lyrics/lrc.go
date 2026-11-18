package lyrics

import (
	"cmp"
	"iter"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	// maxLines bounds the lines lyrics are read into, synced or plain. A
	// long song has a few hundred: at a line every two seconds, maxLines
	// are sung for nearly three hours.
	maxLines = 5000
	// maxLineTags bounds the time tags read of one line, the times it is
	// sung. A chorus has a few, and a line repeated all along a track
	// many: one sung every three seconds for ten minutes has 200.
	maxLineTags = 200
	// maxText bounds the text the lines hold in all, in bytes: that of
	// the largest lyrics file taken. A line holds its text once for each
	// of its time tags, so that without it the lines of a file could
	// hold many times the file.
	maxText = MaxUpload
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

// Parse reads a track's own lyrics, those of a lyrics file or tag beside
// it: synced when they hold a timed line (LRC), else plain. Lyrics without
// a line of text are none. They are read within maxLines and maxText,
// as all lyrics are: what a text holds past them is left out.
func Parse(text string) (Lyrics, bool) {
	if lines := parseLRC(text); len(lines) > 0 {
		return Lyrics{Synced: true, Lines: lines}, true
	}
	if strings.TrimSpace(text) == "" {
		return Lyrics{}, false
	}
	lines := plainLines(text)
	return Lyrics{Lines: lines}, len(lines) > 0
}

// parseLRC reads LRC lyrics into lines, sorted by their start, as Jellyfin
// reads a track's LRC file: each time tag of a line starts a line of its
// text, trimmed, empty ones included (they mark pauses). Lines without a
// time tag, such as the [ar:] or [ti:] tags of the file's description, are
// left out. A line starts maxLineTags lines at most, at the first of its
// time tags, and reading stops at the line that would pass maxLines or
// maxText: the lyrics read until then stay synced.
func parseLRC(text string) []Line {
	var lines []Line
	held := 0
read:
	for raw := range splitLines(text) {
		rest := strings.TrimSpace(raw)
		var starts []time.Duration
		for {
			match := timeTag.FindStringSubmatch(rest)
			if match == nil {
				break
			}
			// The tags past the bound are read too: left in the line,
			// they would show as its words.
			if len(starts) < maxLineTags {
				starts = append(starts, tagTime(match[1], match[2], match[3]))
			}
			rest = rest[len(match[0]):]
		}
		if len(starts) == 0 {
			continue
		}
		words := strings.TrimSpace(wordTag.ReplaceAllString(rest, ""))
		for _, start := range starts {
			if !fits(lines, held, words) {
				break read
			}
			held += len(words)
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
// Reading stops at the line that would pass maxLines or maxText.
func plainLines(text string) []Line {
	var lines []Line
	held := 0
	for raw := range splitLines(strings.TrimSpace(text)) {
		words := strings.TrimSpace(raw)
		if !fits(lines, held, words) {
			break
		}
		held += len(words)
		lines = append(lines, Line{Text: words})
	}
	return lines
}

// fits reports whether lines, holding held bytes of text, may hold one
// more line of words within maxLines and maxText.
func fits(lines []Line, held int, words string) bool {
	return len(lines) < maxLines && held+len(words) <= maxText
}

// splitLines splits text at its line breaks, of any system, one line
// after the other, so that a reader that stops early splits no more.
func splitLines(text string) iter.Seq[string] {
	return strings.SplitSeq(strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(text), "\n")
}
