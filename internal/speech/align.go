package speech

import (
	"cmp"
	"math"
	"slices"
	"sort"
	"time"

	"github.com/moodiness/polyfin/internal/subtitles"
)

// Timing maps a subtitle's times to a version's: Ratio·t + Offset.
type Timing struct {
	Ratio  float64
	Offset time.Duration
}

// Unchanged leaves times as they are.
var Unchanged = Timing{Ratio: 1}

// Changes reports whether t moves any time.
func (t Timing) Changes() bool {
	return t.Ratio != 1 || t.Offset != 0
}

// At is the version's time for a subtitle's time d.
func (t Timing) At(d time.Duration) time.Duration {
	return time.Duration(math.Round(float64(d)*t.Ratio)) + t.Offset
}

// Apply retimes cues; those ending before the version starts are left out,
// those starting before it start with it.
func (t Timing) Apply(cues []subtitles.Cue) []subtitles.Cue {
	if !t.Changes() {
		return cues
	}
	result := make([]subtitles.Cue, 0, len(cues))
	for _, cue := range cues {
		cue.Start, cue.End = t.At(cue.Start), t.At(cue.End)
		if cue.End <= 0 {
			continue
		}
		cue.Start = max(cue.Start, 0)
		result = append(result, cue)
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
// time, is left so.
func Align(cues []subtitles.Cue, reference Reference, frameRate float64) Timing {
	best, ok := search(cues, reference, frameRate)
	if !ok || !best.timing.Changes() || time.Duration(best.fit.covered)*unit < minCovered ||
		best.peak < minPeak || best.apart < minApart || best.lead < minLead || best.fit.share()-best.identity.share() < minGain {
		return Unchanged
	}
	return best.timing
}

// candidate is the best shift of a speed: its timing, how it fits, and how
// the subtitle's own timing does (identity). peak is how far it stands
// above the other shifts of its speed, apart above the best of them more
// than apart from it, and lead above its own timing, in standard
// deviations of the scores of those shifts.
type candidate struct {
	speed             speed
	timing            Timing
	fit, identity     fit
	deviation         float64
	peak, apart, lead float64
}

// search finds the timing that fits cues best; false when the cues or the
// reference give nothing to compare.
func search(cues []subtitles.Cue, reference Reference, frameRate float64) (candidate, bool) {
	if len(cues) == 0 || len(reference.Speech) == 0 || len(reference.Windows) == 0 {
		return candidate{}, false
	}
	speech, windows := units(reference.Speech), units(reference.Windows)
	var identity fit
	var found []candidate
	for _, s := range speeds {
		fits := fitsAt(cues, s.ratio(), speech, windows)
		if s.version == 0 {
			identity = fits[shifts]
		}
		top := shifts
		var sum, squares float64
		for i, f := range fits {
			score := f.score()
			sum += float64(score)
			squares += float64(score) * float64(score)
			if score > fits[top].score() || score == fits[top].score() && abs(int64(i)-shifts) < abs(top-shifts) {
				top = int64(i)
			}
		}
		n := float64(len(fits))
		mean := sum / n
		deviation := math.Sqrt(max(squares/n-mean*mean, 0))
		if deviation == 0 {
			continue
		}
		second := math.Inf(-1)
		for i, f := range fits {
			if abs(int64(i)-top) > int64(apart/unit) {
				second = max(second, float64(f.score()))
			}
		}
		score := float64(fits[top].score())
		found = append(found, candidate{
			speed:     s,
			timing:    Timing{Ratio: s.ratio(), Offset: time.Duration(top-shifts) * unit},
			fit:       fits[top],
			deviation: deviation,
			peak:      (score - mean) / deviation,
			apart:     (score - second) / deviation,
		})
	}
	if len(found) == 0 {
		return candidate{}, false
	}
	top := slices.MaxFunc(found, func(a, b candidate) int { return cmp.Compare(a.fit.score(), b.fit.score()) })
	best := top
	for _, c := range found {
		if float64(top.fit.score()-c.fit.score()) <= tie*top.deviation &&
			(c.speed.rank(frameRate) < best.speed.rank(frameRate) || c.speed.rank(frameRate) == best.speed.rank(frameRate) && c.fit.score() > best.fit.score()) {
			best = c
		}
	}
	best.identity = identity
	best.lead = float64(best.fit.score()-identity.score()) / best.deviation
	return best, true
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

// fitsAt is how the cues, at speed ratio, fit speech within windows at
// every shift from -maxShift to maxShift, by index (shift+maxShift)/unit.
// The overlap of two intervals, one shifted by d, rises, stays and falls
// linearly with d: each pair adds four changes of slope, which two running
// sums turn into the overlaps at every shift.
func fitsAt(cues []subtitles.Cue, ratio float64, speech, windows []interval) []fit {
	n := 2*shifts + 1
	on := newRamps(n)
	covered := newRamps(n)
	for _, cue := range cues {
		a := int64(math.Round(float64(cue.Start) * ratio / float64(unit)))
		b := int64(math.Round(float64(cue.End) * ratio / float64(unit)))
		if b <= a {
			continue
		}
		on.overlaps(a, b, speech)
		covered.overlaps(a, b, windows)
	}
	onTotals, coveredTotals := on.totals(), covered.totals()
	fits := make([]fit, n)
	for i := range fits {
		fits[i] = fit{onTotals[i], coveredTotals[i]}
	}
	return fits
}

// ramps sums functions of the shift d made of ramps: Σ w·max(0, d-p) for
// changes of slope w at p, over the shifts from -shifts to shifts.
type ramps struct {
	changes []int64
	// value and slope are the sums at -shifts of the changes before it.
	value, slope int64
}

func newRamps(n int64) *ramps {
	return &ramps{changes: make([]int64, n)}
}

func (r *ramps) add(p, w int64) {
	switch {
	case p < -shifts:
		r.value += w * (-shifts - p)
		r.slope += w
	case p <= shifts:
		r.changes[p+shifts] += w
	}
}

// overlaps adds the overlap of [a+d, b+d] with each interval of spans,
// which are sorted and apart, that it reaches at some shift d.
func (r *ramps) overlaps(a, b int64, spans []interval) {
	first := sort.Search(len(spans), func(i int) bool { return spans[i].end > a-shifts })
	for _, s := range spans[first:] {
		if s.start >= b+shifts {
			break
		}
		r.add(s.start-b, 1)
		r.add(min(s.start-a, s.end-b), -1)
		r.add(max(s.start-a, s.end-b), -1)
		r.add(s.end-a, 1)
	}
}

func (r *ramps) totals() []int64 {
	totals := make([]int64, len(r.changes))
	value, slope := r.value, r.slope+r.changes[0]
	totals[0] = value
	for i := 1; i < len(totals); i++ {
		value += slope
		slope += r.changes[i]
		totals[i] = value
	}
	return totals
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
