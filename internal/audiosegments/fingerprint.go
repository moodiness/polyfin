package audiosegments

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/cmplx"
	"time"
)

// The audio is fingerprinted at sampleRate, mono, in windows of window
// samples, one every hop: a frame every 64 ms, each over 256 ms, so that
// the same sound read from two files whose frames fall a little apart
// still gives nearly the same frames.
const (
	sampleRate = 8000
	window     = 2048
	hop        = 512
	// frameTime is the time between two frames.
	frameTime = time.Duration(hop) * time.Second / sampleRate
)

// bandEdges are the frequencies bounding the bands whose energies make a
// frame: 33 bands, as wide as one another on a logarithmic scale, between
// lowest and highest, where voices and music are loudest and lossy codecs
// keep them best. Each frame has one bit per pair of neighbouring bands.
var bandEdges = func() [34]int {
	const lowest, highest = 250.0, 2500.0
	var edges [34]int
	for i := range edges {
		frequency := lowest * math.Pow(highest/lowest, float64(i)/float64(len(edges)-1))
		edges[i] = int(math.Round(frequency * window / sampleRate))
	}
	return edges
}()

// silence is the mean square of the samples of a window, in 16-bit
// units, under which it counts as silent: below -55 dBFS. A silent frame
// is 0, which matches no frame (see sameFrame): every silence sounds
// alike.
var silence = math.Pow(32768*math.Pow(10, -55.0/20), 2)

// Fingerprint is what the audio of a part of a file sounds like: a frame
// every frameTime from Start, 32 bits each, which tell, for each pair of
// neighbouring frequency bands, whether the difference between their
// energies grew since the previous frame. Two recordings of the same sound
// give frames that differ by a few bits, others by about half of them.
type Fingerprint struct {
	// Start is where in its file the fingerprint starts.
	Start  time.Duration
	Frames []uint32
}

// at is the time in the file of the middle of frame i.
func (f Fingerprint) at(i int) time.Duration {
	return f.Start + (time.Duration(i*hop+hop/2+window/2)*time.Second)/sampleRate
}

// fingerprint reads 16-bit little-endian mono samples at sampleRate, as
// FFmpeg decodes them for s16le, and fingerprints them.
func fingerprint(samples io.Reader, start time.Duration) (Fingerprint, error) {
	in := bufio.NewReaderSize(samples, 64<<10)
	f := Fingerprint{Start: start}
	var shape [window]float64
	for i := range shape {
		shape[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/window)
	}
	var (
		buffer   [window]float64
		spectrum [window]complex128
		raw      [2]byte
		previous [33]float64
		filled   int
		first    = true
	)
	for {
		// The window moves by hop samples, after its first fill.
		for filled < window {
			if _, err := io.ReadFull(in, raw[:]); err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
					return f, nil
				}
				return f, err
			}
			buffer[filled] = float64(int16(binary.LittleEndian.Uint16(raw[:])))
			filled++
		}
		power := 0.0
		for i, sample := range buffer {
			power += sample * sample
			spectrum[i] = complex(sample*shape[i], 0)
		}
		transform(spectrum[:])
		var energies [33]float64
		for band := range energies {
			for bin := bandEdges[band]; bin < bandEdges[band+1]; bin++ {
				magnitude := cmplx.Abs(spectrum[bin])
				energies[band] += magnitude * magnitude
			}
		}
		if !first {
			var frame uint32
			for bit := range 32 {
				if energies[bit]-energies[bit+1]-(previous[bit]-previous[bit+1]) > 0 {
					frame |= 1 << bit
				}
			}
			switch {
			case power/window < silence:
				frame = 0
			case frame == 0:
				frame = 1
			}
			f.Frames = append(f.Frames, frame)
		}
		first, previous = false, energies
		copy(buffer[:], buffer[hop:])
		filled = window - hop
	}
}

// twiddles are the roots of unity transform uses.
var twiddles = func() [window / 2]complex128 {
	var roots [window / 2]complex128
	for i := range roots {
		roots[i] = cmplx.Rect(1, -2*math.Pi*float64(i)/window)
	}
	return roots
}()

// transform replaces x, of window values, by its discrete Fourier
// transform, in place: an iterative radix-2 FFT.
func transform(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j |= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		step := n / size
		for start := 0; start < n; start += size {
			for k := range size / 2 {
				w := twiddles[k*step]
				even, odd := x[start+k], w*x[start+k+size/2]
				x[start+k], x[start+k+size/2] = even+odd, even-odd
			}
		}
	}
}

// encode and decode store a fingerprint's frames, 4 bytes each.
func encode(frames []uint32) []byte {
	data := make([]byte, 4*len(frames))
	for i, frame := range frames {
		binary.LittleEndian.PutUint32(data[4*i:], frame)
	}
	return data
}

func decode(data []byte) []uint32 {
	frames := make([]uint32, len(data)/4)
	for i := range frames {
		frames[i] = binary.LittleEndian.Uint32(data[4*i:])
	}
	return frames
}
