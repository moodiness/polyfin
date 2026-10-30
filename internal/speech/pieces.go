package speech

import (
	"cmp"
	"slices"
	"sort"
	"time"

	"github.com/moodiness/polyfin/internal/subtitles"
)

// A subtitle timed on another cut of a title, one with a scene more or
// less, fits the version at one speed, but with a shift that changes at
// each such scene: one shift for all its lines leaves those on one side of
// the scene early or late. Against a reference of the whole version, as a
// text track inside it gives, the shift of each stretch of the subtitle
// can be found, and the subtitle fitted piece by piece. Against three
// stretches of its audio, a scene between them cannot be placed.

const (
	// sceneShift bounds the shift of each piece, either way: a scene added
	// or removed lasts up to about that.
	sceneShift  = 10 * time.Minute
	sceneShifts = int64(sceneShift / unit)
	// chunk is the length of the stretches of a subtitle whose shifts are
	// found apart, one every chunkStep, first in steps of coarse units.
	chunk     = 2 * time.Minute
	chunkStep = time.Minute
	coarse    = 10
	// A piece holds at least minPieceCues lines, over at least minPiece,
	// and its lines on speech start within maxResidual of a line of the
	// reference, by their median.
	minPieceCues = 10
	minPiece     = 2 * time.Minute
	maxResidual  = 500 * time.Millisecond
)

// line is a subtitle's cue: its start in the subtitle's time, and its
// span in units at a speed.
type line struct {
	start time.Duration
	span  interval
}

// run is the lines first to end whose chunks fit at one shift: lastChunk
// is the first line of its last chunk, and firstEnd the end of its first.
type run struct {
	first, end, lastChunk, firstEnd int
	shift                           int64
}

// piece is the lines first to end, shifted by shift.
type piece struct {
	first, end int
	shift      int64
}

// alignPieces fits a subtitle's cues to speech within windows, those of a
// Whole reference, in pieces, at the one of ratios, the speeds tried,
// whose pieces fit best: a shift for each piece, of up to sceneShift either way,
// and a cut between two, where the lines of a scene the version lacks are
// left out. The shift of each chunk of the subtitle is found as Align
// finds a subtitle's, clear when it stands as far above the others; the
// chunks of one shift make a piece. It fits only when there are two
// pieces or more, each clear: long enough, with enough lines on speech
// close to the reference's, and with much more of its time on speech than
// at the shifts of the pieces next to it.
func alignPieces(cues []subtitles.Cue, reference Reference, ratios []float64, speech, windows []interval) (Timing, bool) {
	starts := make([]int64, 0, len(reference.Speech))
	for _, span := range reference.Speech {
		starts = append(starts, int64(span.Start/unit))
	}
	slices.Sort(starts)
	shifter := newFitter(coarsened(speech), coarsened(windows), sceneShifts/coarse)
	refiner := newFitter(speech, windows, int64(apart/unit))
	best, found := Unchanged, false
	var bestScore int64
	for _, ratio := range ratios {
		if timing, score, ok := alignPiecesAt(cues, ratio, shifter, refiner, starts); ok && (!found || score > bestScore) {
			best, bestScore, found = timing, score, true
		}
	}
	return best, found
}

// alignPiecesAt fits cues in pieces at speed ratio (see alignPieces):
// shifter finds the shifts of chunks, coarsely, and refiner refines them;
// starts are those of the reference's lines, in units, sorted. score is
// how well the pieces fit (see fit.score).
func alignPiecesAt(cues []subtitles.Cue, ratio float64, shifter, refiner *fitter, starts []int64) (timing Timing, score int64, ok bool) {
	speech, windows := refiner.speech, refiner.windows
	lines := linesOf(cues, ratio)
	runs := chunkRuns(lines, shifter)
	if len(runs) < 2 {
		return Unchanged, 0, false
	}
	for i := range runs {
		runs[i].shift = refine(refiner, lines[runs[i].first:runs[i].end], runs[i].shift)
	}
	timing = Timing{Ratio: ratio, Offset: time.Duration(runs[0].shift) * unit}
	pieces := make([]piece, 0, len(runs))
	from := 0
	for k := 0; k+1 < len(runs); k++ {
		cut, left, right, ok := placeCut(lines, max(runs[k].lastChunk, from), runs[k+1].firstEnd, runs[k].shift, runs[k+1].shift, ratio, speech, windows)
		if !ok {
			return Unchanged, 0, false
		}
		pieces = append(pieces, piece{from, left, runs[k].shift})
		timing.Cuts = append(timing.Cuts, cut)
		from = right
	}
	pieces = append(pieces, piece{from, len(lines), runs[len(runs)-1].shift})
	for i, p := range pieces {
		var others []int64
		if i > 0 {
			others = append(others, pieces[i-1].shift)
		}
		if i+1 < len(pieces) {
			others = append(others, pieces[i+1].shift)
		}
		own, ok := clearPiece(lines[p.first:p.end], p.shift, others, speech, windows, starts)
		if !ok {
			return Unchanged, 0, false
		}
		score += own.score()
	}
	return timing, score, true
}

// linesOf are the cues at speed ratio, by start; those that last no unit
// are left out.
func linesOf(cues []subtitles.Cue, ratio float64) []line {
	lines := make([]line, 0, len(cues))
	for _, cue := range cues {
		if a, b := unitsAt(cue.Start, ratio), unitsAt(cue.End, ratio); b > a {
			lines = append(lines, line{cue.Start, interval{a, b}})
		}
	}
	slices.SortStableFunc(lines, func(a, b line) int { return cmp.Compare(a.start, b.start) })
	return lines
}

func spansOf(lines []line, shift int64) []interval {
	spans := make([]interval, len(lines))
	for i, l := range lines {
		spans[i] = interval{l.span.start + shift, l.span.end + shift}
	}
	return spans
}

// chunkRuns finds the shift of each chunk of lines, of minPieceCues lines
// at least, that stands clearly above the others, with shifter, coarsely,
// and joins into runs the chunks of one shift, give or take apart, that
// follow each other, or have only chunks of no clear shift between them.
func chunkRuns(lines []line, shifter *fitter) []run {
	if len(lines) == 0 {
		return nil
	}
	reach := sceneShifts / coarse
	var runs []run
	for from := lines[0].start; ; from += chunkStep {
		first := sort.Search(len(lines), func(i int) bool { return lines[i].start >= from })
		if first == len(lines) {
			return runs
		}
		end := sort.Search(len(lines), func(i int) bool { return lines[i].start >= from+chunk })
		if end-first < minPieceCues {
			continue
		}
		// The shifts that take some of the chunk out of the version, where
		// it is all on silence, are not looked at.
		spans := coarsened(spansOf(lines[first:end], 0))
		var length int64
		for _, s := range spans {
			length += s.end - s.start
		}
		p, ok := peakOf(shifter.fitsAt(spans), reach, int64(apart/unit)/coarse, length)
		if !ok || p.peak < minPeak || p.apart < minApart {
			continue
		}
		shift := (p.top - reach) * coarse
		if n := len(runs); n > 0 && abs(runs[n-1].shift-shift) <= int64(apart/unit) {
			runs[n-1].end, runs[n-1].lastChunk = end, first
			continue
		}
		runs = append(runs, run{first: first, end: end, lastChunk: first, firstEnd: end, shift: shift})
	}
}

// coarsened are spans, sorted, in steps of coarse units, those that
// overlap joined.
func coarsened(spans []interval) []interval {
	result := make([]interval, 0, len(spans))
	for _, s := range spans {
		c := interval{s.start / coarse, s.end / coarse}
		if c.end <= c.start {
			continue
		}
		if n := len(result); n > 0 && c.start <= result[n-1].end {
			result[n-1].end = max(result[n-1].end, c.end)
			continue
		}
		result = append(result, c)
	}
	return result
}

// refine is the shift, within apart of shift, at which lines fit best;
// of shifts that fit as well, the nearest to shift.
func refine(refiner *fitter, lines []line, shift int64) int64 {
	fits := refiner.fitsAt(spansOf(lines, shift))
	reach := int64(apart / unit)
	top := reach
	for i, f := range fits {
		if f.score() > fits[top].score() || f.score() == fits[top].score() && abs(int64(i)-reach) < abs(top-reach) {
			top = int64(i)
		}
	}
	return shift + top - reach
}

// placeCut places the cut between lines shifted by before and lines
// shifted by after, among lines lo to hi: the lines before it are shifted
// by before, and those after it by after. When after is the lesser, the
// version lacks a scene of before-after, and the lines that start within
// it are left out. The cut is placed where the lines fit best; of places
// that fit as well, where the fewest lines are left out, then the first.
// left is the end of the lines shifted by before, and right the first of
// those shifted by after.
func placeCut(lines []line, lo, hi int, before, after int64, ratio float64, speech, windows []interval) (cut Cut, left, right int, ok bool) {
	hi = min(hi, len(lines))
	if hi-lo < 2 {
		return Cut{}, 0, 0, false
	}
	// leftScore[k] is the score of lines lo to k shifted by before, and
	// rightScore[k] that of lines k to hi shifted by after.
	leftScore := make([]int64, hi-lo+1)
	rightScore := make([]int64, hi-lo+1)
	for i := lo; i < hi; i++ {
		leftScore[i-lo+1] = leftScore[i-lo] + scoreAt(lines[i].span, before, speech, windows)
	}
	for i := hi - 1; i >= lo; i-- {
		rightScore[i-lo] = rightScore[i-lo+1] + scoreAt(lines[i].span, after, speech, windows)
	}
	// scene is how long the scene the version lacks lasts in the
	// subtitle's time, to the millisecond, as cuts are kept.
	var scene time.Duration
	if after < before {
		scene = time.Duration(float64(time.Duration(before-after)*unit) / ratio).Round(time.Millisecond)
	}
	firstAt := func(at time.Duration) int {
		return sort.Search(len(lines), func(i int) bool { return lines[i].start >= at })
	}
	found := false
	var best int64
	try := func(start time.Duration) {
		k, m := firstAt(start), firstAt(start+scene)
		if k <= lo || m > hi {
			return
		}
		score := leftScore[k-lo] + rightScore[m-lo]
		if !found || score > best || score == best && (m-k < right-left || m-k == right-left && start < cut.Start) {
			cut, left, right, best, found = Cut{Start: start, End: start + scene, Offset: time.Duration(after) * unit}, k, m, score, true
		}
	}
	for i := lo + 1; i < hi; i++ {
		start := lines[i].start.Truncate(time.Millisecond)
		try(start)
		if scene > 0 {
			try(start - scene)
		}
	}
	return cut, left, right, found
}

// scoreAt is the score of a span shifted by shift (see fit.score).
func scoreAt(span interval, shift int64, speech, windows []interval) int64 {
	return fitAt(span, shift, speech, windows).score()
}

func fitAt(span interval, shift int64, speech, windows []interval) fit {
	a, b := span.start+shift, span.end+shift
	return fit{on: overlap(a, b, speech), covered: overlap(a, b, windows)}
}

// clearPiece reports whether lines fit clearly at shift, as a piece, and
// how they fit (own): there are enough of them, over long enough; enough
// of them fall on speech, where they start within maxResidual of a line
// of the reference, of starts, by their median; and much more of their
// time falls on speech than at each of others, the shifts of the pieces
// next to them.
func clearPiece(lines []line, shift int64, others []int64, speech, windows []interval, starts []int64) (own fit, ok bool) {
	if len(lines) < minPieceCues || lines[len(lines)-1].start-lines[0].start < minPiece {
		return own, false
	}
	var residuals []int64
	for _, l := range lines {
		f := fitAt(l.span, shift, speech, windows)
		own.on += f.on
		own.covered += f.covered
		if f.on > 0 {
			residuals = append(residuals, nearest(starts, l.span.start+shift))
		}
	}
	if len(residuals) < minPieceCues {
		return own, false
	}
	slices.Sort(residuals)
	if time.Duration(residuals[len(residuals)/2])*unit > maxResidual {
		return own, false
	}
	for _, other := range others {
		var f fit
		for _, l := range lines {
			g := fitAt(l.span, other, speech, windows)
			f.on += g.on
			f.covered += g.covered
		}
		if own.share()-f.share() < minGain {
			return own, false
		}
	}
	return own, true
}

// nearest is how far at is from the nearest of sorted.
func nearest(sorted []int64, at int64) int64 {
	i := sort.Search(len(sorted), func(i int) bool { return sorted[i] >= at })
	distance := int64(-1)
	if i < len(sorted) {
		distance = sorted[i] - at
	}
	if i > 0 && (distance < 0 || at-sorted[i-1] < distance) {
		distance = at - sorted[i-1]
	}
	return distance
}
