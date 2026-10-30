package speech

import (
	"cmp"
	"math"
	"slices"
	"sort"
	"time"

	"github.com/moodiness/polyfin/internal/subtitles"
)

// Timing maps a subtitle's times to a version's: Ratio·t + Offset, until
// the first of Cuts, and then each cut's offset instead.
type Timing struct {
	Ratio  float64
	Offset time.Duration
	Cuts   []Cut
}

// Cut is where a subtitle timed on another cut of a title changes its
// shift, at a scene added or removed: its lines that start from Start to
// End are of a scene the version lacks, and are left out, and those from
// End on are shifted by Offset. A scene the version adds has no lines to
// leave out: Start is End.
type Cut struct {
	Start, End, Offset time.Duration
}

// Unchanged leaves times as they are.
var Unchanged = Timing{Ratio: 1}

// Changes reports whether t moves any time.
func (t Timing) Changes() bool {
	return t.Ratio != 1 || t.Offset != 0 || len(t.Cuts) > 0
}

// Map is the version's times for a line of a subtitle shown from start to
// end; false when it is left out: in a scene the version lacks, or ending
// before the version starts. One that starts before the version starts
// with it.
func (t Timing) Map(start, end time.Duration) (time.Duration, time.Duration, bool) {
	offset := t.Offset
	for _, cut := range t.Cuts {
		if start < cut.Start {
			break
		}
		if start < cut.End {
			return 0, 0, false
		}
		offset = cut.Offset
	}
	start = time.Duration(math.Round(float64(start)*t.Ratio)) + offset
	end = time.Duration(math.Round(float64(end)*t.Ratio)) + offset
	if end <= 0 {
		return 0, 0, false
	}
	return max(start, 0), end, true
}

// Apply retimes cues; those Map leaves out are left out.
func (t Timing) Apply(cues []subtitles.Cue) []subtitles.Cue {
	if !t.Changes() {
		return cues
	}
	result := make([]subtitles.Cue, 0, len(cues))
	for _, cue := range cues {
		var ok bool
		if cue.Start, cue.End, ok = t.Map(cue.Start, cue.End); ok {
			result = append(result, cue)
		}
	}
	return result
}

const (
	// unit is the step shifts are tried at.
	unit = 10 * time.Millisecond
	// maxShift bounds the constant shift corrected, either way.
	maxShift = time.Minute
	shifts   = int64(maxShift / unit)
	// minCovered is how long the cues within the reference's windows must
	// last for a subtitle to be judged at all.
	minCovered = 20 * time.Second
	// A subtitle is changed only when it fits clearly better changed, in
	// standard deviations of the scores of the shifts of its speed: its
	// best shift stands minPeak above them, minApart above the best shift
	// more than apart from it, and minLead above the subtitle as it is;
	// and minGain more of its time falls on speech.
	minPeak  = 4
	minApart = 2
	apart    = 2 * time.Second
	minLead  = 2
	minGain  = 0.15
	// tie is how close, in the same deviations, the best shifts of two
	// speeds score as well: 23.976 and 24 fps differ by 0.1%, which
	// subtitles timed to the half second do not tell apart. The speed
	// preferred wins (see speed.rank).
	tie = 0.5
)

// speed is a subtitle timed for a release of subtitle fps played on a
// version of version fps: its times run version/subtitle as fast, and
// are multiplied by subtitle/version. The zero speed leaves them as they
// are.
type speed struct{ subtitle, version float64 }

func (s speed) ratio() float64 {
	if s.version == 0 {
		return 1
	}
	return s.subtitle / s.version
}

// rank orders speeds that fit as well: as it is, and those for a version
// of the version's frame rate, when known, before the others.
func (s speed) rank(frameRate float64) int {
	if s.version == 0 || math.Abs(s.version-frameRate) < 0.01 {
		return 0
	}
	return 1
}

// frameRates are the usual frame rates, whose differences make subtitles
// run faster or slower than a version.
var frameRates = []float64{24000.0 / 1001, 24, 25}

// speeds are the speeds tried: as it is, then those between two of
// frameRates either way.
var speeds = func() []speed {
	result := []speed{{}}
	for _, subtitle := range frameRates {
		for _, version := range frameRates {
			if subtitle != version {
				result = append(result, speed{subtitle, version})
			}
		}
	}
	return result
}()

// fit is how well a subtitle fits a reference at one timing: the time of
// its cues within the reference's windows (covered) and on speech (on),
// in units.
type fit struct {
	on, covered int64
}

// score grows with the time on speech and shrinks with the time on
// silence.
func (f fit) score() int64 { return 2*f.on - f.covered }

func (f fit) share() float64 {
	if f.covered == 0 {
		return 0
	}
	return float64(f.on) / float64(f.covered)
}

// Align finds the timing that fits a subtitle's cues to a reference: a
// constant shift of up to a minute and a speed among those between 23.976,
// 24 and 25 fps, found together, scored by the time the cues spend on
// speech against the time they spend on silence, within the windows.
// frameRate is the version's, 0 when not known: of two speeds that fit as
// well, the subtitle's own, or one for a version of that frame rate, is
// preferred. The subtitle is left Unchanged unless the timing found fits
// clearly better than its own: well above the other shifts of its speed,
// above any shift apart from it, above its own timing, and with much more
// of its time on speech. A subtitle of another title, or one already in
// time, is left so. Against a Whole reference, a subtitle timed on another
// cut gets a new shift after each scene added or removed (see
// alignPieces), at the speed found, or when no shift of it stands out, as
// happens when most of the subtitle is shifted by more than a minute, at
// the speed that fits so best.
func Align(cues []subtitles.Cue, reference Reference, frameRate float64) Timing {
	speech, windows := units(reference.Speech), units(reference.Windows)
	best, found, ok := search(cues, speech, windows, frameRate)
	if !ok {
		return Unchanged
	}
	if reference.Whole {
		ratios := []float64{best.timing.Ratio}
		if best.peak < minPeak || best.apart < minApart {
			for _, c := range found {
				if c.speed != best.speed {
					ratios = append(ratios, c.timing.Ratio)
				}
			}
		}
		if timing, ok := alignPieces(cues, reference, ratios, speech, windows); ok {
			return timing
		}
	}
	if !best.timing.Changes() || time.Duration(best.fit.covered)*unit < minCovered ||
		best.peak < minPeak || best.apart < minApart || best.lead < minLead || best.fit.share()-best.identity.share() < minGain {
		return Unchanged
	}
	return best.timing
}

// candidate is the best shift of a speed: its timing, how it fits, and how
// the subtitle's own timing does (identity), and how it stands above the
// other shifts of its speed (see standout); lead is how far it stands above
// its own timing, in the same standard deviations.
type candidate struct {
	speed         speed
	timing        Timing
	fit, identity fit
	standout
	lead float64
}

// search finds the timing that fits cues best to speech within windows,
// and the best shift of each speed (found); false when the cues or the
// reference give nothing to compare.
func search(cues []subtitles.Cue, speech, windows []interval, frameRate float64) (best candidate, found []candidate, ok bool) {
	if len(cues) == 0 || len(speech) == 0 || len(windows) == 0 {
		return candidate{}, nil, false
	}
	shifter := newFitter(speech, windows, shifts)
	var identity fit
	for _, s := range speeds {
		fits := shifter.fitsAt(mapped(cues, s.ratio()))
		if s.version == 0 {
			identity = fits[shifts]
		}
		p, ok := peakOf(fits, shifts, int64(apart/unit), 0)
		if !ok {
			continue
		}
		found = append(found, candidate{
			speed:    s,
			timing:   Timing{Ratio: s.ratio(), Offset: time.Duration(p.top-shifts) * unit},
			fit:      fits[p.top],
			standout: p,
		})
	}
	if len(found) == 0 {
		return candidate{}, nil, false
	}
	top := slices.MaxFunc(found, func(a, b candidate) int { return cmp.Compare(a.fit.score(), b.fit.score()) })
	best = top
	for _, c := range found {
		if float64(top.fit.score()-c.fit.score()) <= tie*top.deviation &&
			(c.speed.rank(frameRate) < best.speed.rank(frameRate) || c.speed.rank(frameRate) == best.speed.rank(frameRate) && c.fit.score() > best.fit.score()) {
			best = c
		}
	}
	best.identity = identity
	best.lead = float64(best.fit.score()-identity.score()) / best.deviation
	return best, found, true
}

// standout is the best of the fits at the shifts from -reach to reach: top,
// its index, and how far it stands, in standard deviations (deviation) of
// the scores of those shifts, above their mean (peak) and above the best
// of them more than apart from it (apart).
type standout struct {
	top                    int64
	deviation, peak, apart float64
}

// peakOf finds the peak of fits, the shifts from -reach to reach by
// index, near indexes making apart; of shifts that score as well, the
// nearest to none. Only the shifts where the cues are covered for at least
// full are looked at. False when they all score alike.
func peakOf(fits []fit, reach, near, full int64) (standout, bool) {
	top := int64(-1)
	var sum, squares, n float64
	for i, f := range fits {
		if f.covered < full {
			continue
		}
		score := f.score()
		sum += float64(score)
		squares += float64(score) * float64(score)
		n++
		if top < 0 || score > fits[top].score() || score == fits[top].score() && abs(int64(i)-reach) < abs(top-reach) {
			top = int64(i)
		}
	}
	if top < 0 {
		return standout{}, false
	}
	mean := sum / n
	deviation := math.Sqrt(max(squares/n-mean*mean, 0))
	if deviation == 0 {
		return standout{}, false
	}
	second := math.Inf(-1)
	for i, f := range fits {
		if f.covered >= full && abs(int64(i)-top) > near {
			second = max(second, float64(f.score()))
		}
	}
	score := float64(fits[top].score())
	return standout{top: top, deviation: deviation, peak: (score - mean) / deviation, apart: (score - second) / deviation}, true
}

// interval is a span in units.
type interval struct{ start, end int64 }

// units turns spans into intervals, sorted and apart: those that overlap,
// as cues shown together do, are joined.
func units(spans []Span) []interval {
	sorted := make([]interval, 0, len(spans))
	for _, s := range spans {
		if s.End > s.Start {
			sorted = append(sorted, interval{int64(s.Start / unit), int64(s.End / unit)})
		}
	}
	slices.SortFunc(sorted, func(a, b interval) int { return cmp.Compare(a.start, b.start) })
	result := sorted[:0]
	for _, i := range sorted {
		if n := len(result); n > 0 && i.start <= result[n-1].end {
			result[n-1].end = max(result[n-1].end, i.end)
			continue
		}
		result = append(result, i)
	}
	return result
}

// unitsAt is a subtitle's time d at speed ratio, in units.
func unitsAt(d time.Duration, ratio float64) int64 {
	return int64(math.Round(float64(d) * ratio / float64(unit)))
}

// mapped are the cues at speed ratio, in units; those that last no unit
// are left out.
func mapped(cues []subtitles.Cue, ratio float64) []interval {
	result := make([]interval, 0, len(cues))
	for _, cue := range cues {
		if a, b := unitsAt(cue.Start, ratio), unitsAt(cue.End, ratio); b > a {
			result = append(result, interval{a, b})
		}
	}
	return result
}

// fitter finds how cues fit speech within windows at every shift from
// -reach to reach; it keeps its buffers from one set of cues to the next.
type fitter struct {
	speech, windows  []interval
	on, covered      *ramps
	onTotals, covers []int64
	fits             []fit
}

func newFitter(speech, windows []interval, reach int64) *fitter {
	n := 2*reach + 1
	return &fitter{speech: speech, windows: windows, on: newRamps(reach), covered: newRamps(reach),
		onTotals: make([]int64, n), covers: make([]int64, n), fits: make([]fit, n)}
}

// fitsAt is how spans fit at every shift, by index shift+reach in units,
// until the next call. The overlap of two intervals, one shifted by d,
// rises, stays and falls linearly with d: each pair adds four changes of
// slope, which two running sums turn into the overlaps at every shift.
func (f *fitter) fitsAt(spans []interval) []fit {
	f.on.reset()
	f.covered.reset()
	for _, s := range spans {
		f.on.overlaps(s.start, s.end, f.speech)
		f.covered.overlaps(s.start, s.end, f.windows)
	}
	f.on.totals(f.onTotals)
	f.covered.totals(f.covers)
	for i := range f.fits {
		f.fits[i] = fit{f.onTotals[i], f.covers[i]}
	}
	return f.fits
}

// ramps sums functions of the shift d made of ramps: Σ w·max(0, d-p) for
// changes of slope w at p, over the shifts from -reach to reach.
type ramps struct {
	reach   int64
	changes []int64
	// value and slope are the sums at -reach of the changes before it.
	value, slope int64
}

func newRamps(reach int64) *ramps {
	return &ramps{reach: reach, changes: make([]int64, 2*reach+1)}
}

func (r *ramps) reset() {
	clear(r.changes)
	r.value, r.slope = 0, 0
}

func (r *ramps) add(p, w int64) {
	switch {
	case p < -r.reach:
		r.value += w * (-r.reach - p)
		r.slope += w
	case p <= r.reach:
		r.changes[p+r.reach] += w
	}
}

// overlaps adds the overlap of [a+d, b+d] with each interval of spans,
// which are sorted and apart, that it reaches at some shift d.
func (r *ramps) overlaps(a, b int64, spans []interval) {
	first := sort.Search(len(spans), func(i int) bool { return spans[i].end > a-r.reach })
	for _, s := range spans[first:] {
		if s.start >= b+r.reach {
			break
		}
		r.add(s.start-b, 1)
		r.add(min(s.start-a, s.end-b), -1)
		r.add(max(s.start-a, s.end-b), -1)
		r.add(s.end-a, 1)
	}
}

// totals writes the sums at each shift to totals.
func (r *ramps) totals(totals []int64) {
	value, slope := r.value, r.slope+r.changes[0]
	totals[0] = value
	for i := 1; i < len(totals); i++ {
		value += slope
		slope += r.changes[i]
		totals[i] = value
	}
}

// overlap is how much [a, b] overlaps spans, which are sorted and apart.
func overlap(a, b int64, spans []interval) int64 {
	var total int64
	for i := sort.Search(len(spans), func(i int) bool { return spans[i].end > a }); i < len(spans) && spans[i].start < b; i++ {
		total += min(b, spans[i].end) - max(a, spans[i].start)
	}
	return total
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
