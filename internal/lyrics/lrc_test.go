package lyrics

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// sungLine is a line of LRC sung times times: as many time tags, then its
// words.
func sungLine(times int, words string) string {
	return strings.Repeat("[0:0]", times) + words
}

// longLine is a line of 8 KiB sung 1638 times, in 16 KiB of LRC: read
// without bounds, its lines would hold 13 MiB of text.
var longLine = sungLine(1638, strings.Repeat("a", 8<<10))

// held is the text lines hold in all, in bytes.
func held(lines []Line) int {
	size := 0
	for _, line := range lines {
		size += len(line.Text)
	}
	return size
}

// Lyrics are read within their bounds, whatever the text: maxLineTags
// lines for one line of LRC, maxLines lines in all, and maxText of text
// held by the lines in all, which stops a long line sung many times.
// Synced lyrics past a bound stay synced, and the time tags past
// maxLineTags are never read as words.
func TestLyricsAreReadWithinTheirBounds(t *testing.T) {
	t.Parallel()
	// repeated is how many lines a line of size bytes sung many times
	// gives.
	repeated := func(size int) int { return min(maxLineTags, maxText/size) }
	for _, tc := range []struct {
		name, text string
		synced     bool
		lines      int
		words      string
	}{
		{"a line of 8 KiB sung 1638 times", longLine, true, repeated(8 << 10), strings.Repeat("a", 8<<10)},
		{"a line of 16 KiB sung 3276 times", sungLine(3276, strings.Repeat("a", 16<<10)), true, repeated(16 << 10), strings.Repeat("a", 16<<10)},
		{"a line of 512 KiB sung 104857 times", sungLine(104857, strings.Repeat("a", 512<<10)), true, repeated(512 << 10),
			strings.Repeat("a", 512<<10)},
		{"a short line sung 3000 times", "[ti:Song]\n" + sungLine(3000, " la la "), true, maxLineTags, "la la"},
		{"a megabyte of time tags", sungLine(1<<20/5, ""), true, maxLineTags, ""},
		{"more timed lines than a song has", strings.Repeat("[00:01.00]word\n", 4*maxLines), true, maxLines, "word"},
		{"a megabyte of empty timed lines", strings.Repeat("[0:0]\n", 1<<20/6), true, maxLines, ""},
		{"more plain lines than a song has", strings.Repeat("word\r\n", 4*maxLines), false, maxLines, "word"},
		{"a megabyte of short plain lines", strings.Repeat("a\n", 1<<19), false, maxLines, "a"},
		{"one plain line of the most text", strings.Repeat("a", maxText), false, 1, strings.Repeat("a", maxText)},
	} {
		found, ok := Parse(tc.text)
		if !ok || found.Synced != tc.synced || len(found.Lines) != tc.lines || held(found.Lines) > maxText {
			t.Errorf("%s: read %v, synced %v, %d lines holding %d bytes; want %d lines holding %d at most", tc.name, ok, found.Synced,
				len(found.Lines), held(found.Lines), tc.lines, maxText)
			continue
		}
		if i := slices.IndexFunc(found.Lines, func(line Line) bool { return line.Text != tc.words }); i >= 0 {
			t.Errorf("%s: line %d reads %.40q", tc.name, i, found.Lines[i].Text)
		}
	}
	// A plain line that alone passes maxText is no lyrics.
	if found, ok := Parse(strings.Repeat("a", maxText+1)); ok || len(found.Lines) != 0 {
		t.Errorf("a plain line over the bound: read %v, %d lines", ok, len(found.Lines))
	}
	// The lines after a line that passes a bound are left out too: what
	// is read is the start of the text.
	found, _ := Parse("[00:01.00]first\n" + longLine + "\n[00:02.00]last\n")
	if want := 1 + min(maxLineTags, (maxText-len("first"))/(8<<10)); len(found.Lines) != want || held(found.Lines) > maxText ||
		slices.ContainsFunc(found.Lines, func(line Line) bool { return line.Text == "last" }) {
		t.Errorf("lines around a long one: %d lines holding %d bytes, want %d", len(found.Lines), held(found.Lines), want)
	}
}

// A song is read as it was before lyrics had bounds: its description
// left out, its choruses once for each of their time tags, pauses kept,
// word timing left out, all sorted by their start. A line sung all along
// a track, far more often than a chorus, is read at each of its times.
func TestSongsWithRepeatedChorusesAreReadWhole(t *testing.T) {
	t.Parallel()
	const song = "[ti:Paper Boats]\r\n[ar:Tone Quartet]\r\n[length: 03:12]\r\n\r\n" +
		"[00:12.00]We fold the morning into paper boats\r\n" +
		"[00:16.50]And send them down the gutter to the sea\r\n" +
		"[00:21.00][01:05.00][02:10.00][02:40.00]Sail on, sail on, little boat\r\n" +
		"[00:25.30][01:09.30][02:14.30][02:44.30]Carry what I could not say\r\n" +
		"[00:30.00][01:14.00][02:19.00]\r\n" +
		"[00:48.00] <00:48.00>The <00:48.40>rain <00:48.90>comes <00:49.30>in\r\n" +
		"[00:52.75]Slow\r\n" +
		"[02:49.00][02:52.00][02:55.00][02:58.00][03:01.00]Sail on\r\n"
	at := func(minutes, seconds, hundredths int) time.Duration {
		return time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second + time.Duration(hundredths)*10*time.Millisecond
	}
	const sail, carry = "Sail on, sail on, little boat", "Carry what I could not say"
	want := []Line{
		{"We fold the morning into paper boats", at(0, 12, 0)}, {"And send them down the gutter to the sea", at(0, 16, 50)},
		{sail, at(0, 21, 0)}, {carry, at(0, 25, 30)}, {"", at(0, 30, 0)}, {"The rain comes in", at(0, 48, 0)}, {"Slow", at(0, 52, 75)},
		{sail, at(1, 5, 0)}, {carry, at(1, 9, 30)}, {"", at(1, 14, 0)},
		{sail, at(2, 10, 0)}, {carry, at(2, 14, 30)}, {"", at(2, 19, 0)},
		{sail, at(2, 40, 0)}, {carry, at(2, 44, 30)},
		{"Sail on", at(2, 49, 0)}, {"Sail on", at(2, 52, 0)}, {"Sail on", at(2, 55, 0)}, {"Sail on", at(2, 58, 0)}, {"Sail on", at(3, 1, 0)},
	}
	if found, ok := Parse(song); !ok || !found.Synced || !slices.Equal(found.Lines, want) {
		t.Errorf("the song: read %v, synced %v, lines %+v", ok, found.Synced, found.Lines)
	}
	if found, ok := Parse("Verse one\nSecond line\n\nVerse two\n"); !ok || found.Synced ||
		!slices.Equal(found.Lines, []Line{{Text: "Verse one"}, {Text: "Second line"}, {}, {Text: "Verse two"}}) {
		t.Errorf("plain lyrics: read %v, synced %v, lines %+v", ok, found.Synced, found.Lines)
	}

	// One line, sung every 3 seconds for 7 minutes.
	var round strings.Builder
	for i := range 144 {
		fmt.Fprintf(&round, "[%02d:%02d.00]", i*3/60, i*3%60)
	}
	round.WriteString("Round and round we go")
	found, ok := Parse(round.String())
	if !ok || len(found.Lines) != 144 || found.Lines[143] != (Line{"Round and round we go", 143 * 3 * time.Second}) {
		t.Errorf("a line sung 144 times: read %v, %d lines", ok, len(found.Lines))
	}
}

// What LRCLIB answers is read within the bounds of a lyrics file, by a
// look-up, by a search and by a choice among its results, as lyrics
// uploaded are.
func TestLyricsFromLRCLIBAreReadWithinTheirBounds(t *testing.T) {
	t.Parallel()
	f := newFakeLRCLIB(t)
	s, _, _ := newService(t, f)
	want := min(maxLineTags, maxText/(8<<10))
	within := func(what string, found Lyrics, ok bool) {
		t.Helper()
		if !ok || !found.Synced || len(found.Lines) != want || held(found.Lines) > maxText {
			t.Errorf("%s: read %v, synced %v, %d lines holding %d bytes; want %d lines holding %d at most", what, ok, found.Synced,
				len(found.Lines), held(found.Lines), want, maxText)
		}
	}
	track := song("Endless Song")
	found, ok := s.Lyrics(t.Context(), track, 5*time.Second)
	within("looked up", found, ok)
	found, ok = s.Lyrics(t.Context(), track, 0)
	within("kept", found, ok)

	other := song("Echo Chamber")
	found, err := s.Choose(t.Context(), other.ID, resultID(88, true))
	within("chosen", found, err == nil)
	found, ok = s.Chosen(t.Context(), other.ID)
	within("chosen and kept", found, ok)

	found, err = s.Upload(t.Context(), other.ID, "endless.lrc", []byte(longLine))
	within("uploaded", found, err == nil)
	found, ok = s.Chosen(t.Context(), other.ID)
	within("uploaded and kept", found, ok)

	// A search lists maxResults tracks with such lyrics: its results hold
	// no more text in all than a search answer may.
	results := s.Search(t.Context(), track)
	total := 0
	for _, result := range results {
		if result.Lyrics.Synced {
			within("listed", result.Lyrics, true)
		}
		total += held(result.Lyrics.Lines)
	}
	if len(results) == 0 || total > maxSearchAnswer {
		t.Errorf("a search: %d results holding %d bytes, want %d at most", len(results), total, maxSearchAnswer)
	}
}
