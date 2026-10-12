// Package speech finds when a version has speech, from the energy of the
// voice band in its audio, and times subtitles to it: a constant shift and
// a speed difference between the usual frame rates, found together, as
// subtitle aligners do.
package speech

import (
	"math"
	"slices"
	"time"
)

// SampleRate is the rate of the samples Detect reads: mono, 16-bit.
const SampleRate = 16000

const (
	// frame is the length speech is told in.
	frame        = 10 * time.Millisecond
	frameSamples = SampleRate / 100
	// lowCut and highCut bound the voice band.
	lowCut, highCut = 300, 3000
	// minRange is how much louder than its quietest the voice band must
	// get for a stretch to have speech at all, in dB.
	minRange = 9
	// minLoud is how much louder than the quiet of its stretch the voice
	// band must be for speech, in dB.
	minLoud = 6
	// Speech rises and falls with its syllables, a few times a second,
	// where music and noise hold: the level of the voice band must vary by
	// minSwing dB (a standard deviation) within swingRadius frames either
	// side.
	minSwing    = 4
	swingRadius = 25
	// pause is the longest silence within speech, as between words, and
	// shortest the shortest speech kept.
	pause    = 300 * time.Millisecond
	shortest = 200 * time.Millisecond
)

// Span is a stretch of time.
type Span struct {
	Start, End time.Duration
}

// Reference is when a version has speech: the spans of Speech, by start,
// within the Windows that were looked at, by start too.
type Reference struct {
	Windows []Span
	Speech  []Span
}

// Detect finds the speech in mono 16-bit samples at SampleRate, the first
// at start: the frames whose voice band, between 300 and 3000 Hz, is loud
// compared with the quiet of the samples and rises and falls as syllables
// do, joined over short pauses. Speech shorter than a word is left out.
func Detect(samples []int16, start time.Duration) []Span {
	frames := len(samples) / frameSamples
	if frames == 0 {
		return nil
	}
	band := voiceBand(samples)
	levels := make([]float64, frames)
	for i := range levels {
		var sum float64
		for _, x := range band[i*frameSamples : (i+1)*frameSamples] {
			sum += x * x
		}
		levels[i] = 10 * math.Log10(sum/frameSamples+1e-3)
	}
	sorted := slices.Clone(levels)
	slices.Sort(sorted)
	quiet, loud := sorted[frames/10], sorted[frames*95/100]
	if loud-quiet < minRange {
		return nil
	}
	swings := swing(levels)
	speaks := func(i int) bool { return levels[i] >= quiet+minLoud && swings[i] >= minSwing }
	var spans []Span
	for i := 0; i < frames; {
		if !speaks(i) {
			i++
			continue
		}
		j := i
		for j < frames && speaks(j) {
			j++
		}
		span := Span{start + time.Duration(i)*frame, start + time.Duration(j)*frame}
		if n := len(spans); n > 0 && span.Start-spans[n-1].End <= pause {
			spans[n-1].End = span.End
		} else {
			spans = append(spans, span)
		}
		i = j
	}
	return slices.DeleteFunc(spans, func(s Span) bool { return s.End-s.Start < shortest })
}

// swing is the standard deviation of levels within swingRadius of each.
func swing(levels []float64) []float64 {
	sums, squares := make([]float64, len(levels)+1), make([]float64, len(levels)+1)
	for i, l := range levels {
		sums[i+1], squares[i+1] = sums[i]+l, squares[i]+l*l
	}
	swings := make([]float64, len(levels))
	for i := range levels {
		lo, hi := max(i-swingRadius, 0), min(i+swingRadius+1, len(levels))
		n := float64(hi - lo)
		mean := (sums[hi] - sums[lo]) / n
		swings[i] = math.Sqrt(max((squares[hi]-squares[lo])/n-mean*mean, 0))
	}
	return swings
}

// voiceBand keeps the voice band of samples: second-order high-pass and
// low-pass filters, each run twice.
func voiceBand(samples []int16) []float64 {
	out := make([]float64, len(samples))
	for i, x := range samples {
		out[i] = float64(x)
	}
	for _, f := range []biquad{highPass(lowCut), highPass(lowCut), lowPass(highCut), lowPass(highCut)} {
		f.run(out)
	}
	return out
}

// biquad is a second-order filter, from the Audio EQ Cookbook.
type biquad struct {
	b0, b1, b2, a1, a2 float64
}

func newBiquad(cutoff float64, high bool) biquad {
	w := 2 * math.Pi * cutoff / SampleRate
	cos, alpha := math.Cos(w), math.Sin(w)/math.Sqrt2
	a0 := 1 + alpha
	b0, b1 := (1-cos)/2, 1-cos
	if high {
		b0, b1 = (1+cos)/2, -(1 + cos)
	}
	return biquad{b0 / a0, b1 / a0, b0 / a0, -2 * cos / a0, (1 - alpha) / a0}
}

func highPass(cutoff float64) biquad { return newBiquad(cutoff, true) }
func lowPass(cutoff float64) biquad  { return newBiquad(cutoff, false) }

func (f biquad) run(x []float64) {
	var x1, x2, y1, y2 float64
	for i, x0 := range x {
		y0 := f.b0*x0 + f.b1*x1 + f.b2*x2 - f.a1*y1 - f.a2*y2
		x[i], x2, x1, y2, y1 = y0, x1, x0, y1, y0
	}
}

// Total is how long spans last.
func Total(spans []Span) time.Duration {
	var total time.Duration
	for _, s := range spans {
		total += s.End - s.Start
	}
	return total
}
