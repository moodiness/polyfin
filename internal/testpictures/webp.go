// Package testpictures writes pictures for tests that the standard
// library cannot encode, and measures what decoding one holds.
package testpictures

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"runtime"
)

// LosslessWebP returns a lossless WebP picture of width by height grey
// pixels whose decoder builds the most prefix codes it may: a colour
// cache of 2,048 entries, every symbol of each code used, and a meta
// prefix image of 4 by 4 tiles naming groups groups of codes, at most
// one for each tile. pixels is where its pixels start in it: every group
// is read by then. Each pixel is written as a literal, so that a decoder
// reads on past pixels.
func LosslessWebP(width, height, groups int) (picture []byte, pixels int, err error) {
	tilesAcross, tilesDown := (width+3)/4, (height+3)/4
	if groups < 1 || groups > tilesAcross*tilesDown || groups > 1<<16 {
		return nil, 0, fmt.Errorf("%d tiles cannot name %d groups", tilesAcross*tilesDown, groups)
	}
	w := &bitWriter{}
	w.write(0x2f, 8)
	w.write(uint32(width-1), 14)
	w.write(uint32(height-1), 14)
	w.write(0, 1) // no transparency
	w.write(0, 3) // version 0
	w.write(0, 1) // no transform
	w.write(1, 1) // a colour cache of 2^11 entries
	w.write(11, 4)
	w.write(1, 1) // a meta prefix image, of tiles of 2^2 pixels a side
	w.write(0, 3)
	// The meta prefix image holds a group's number in each tile's green,
	// its low byte, and red, written with codes of 8 bits, and blue and
	// alpha with codes of one symbol, which take no bits.
	byteCode := newPrefixCode(completeLengths(256))
	w.write(0, 1) // no colour cache
	putCode(w, append(completeLengths(256), make([]uint32, 24)...))
	putCode(w, byteCode.lengths)
	putOneSymbol(w, 0)
	putOneSymbol(w, 0)
	putOneSymbol(w, 0)
	for tile := range tilesAcross * tilesDown {
		group := uint32(tile % groups)
		byteCode.put(w, group&0xff)
		byteCode.put(w, group>>8)
	}
	// The groups, all alike: green with its 24 lengths and the 2,048
	// entries of the cache, red, blue, alpha, then the 40 distances.
	green := newPrefixCode(completeLengths(256 + 24 + 2048))
	for range groups {
		putCode(w, green.lengths)
		putCode(w, byteCode.lengths)
		putCode(w, byteCode.lengths)
		putCode(w, byteCode.lengths)
		putCode(w, completeLengths(40))
	}
	pixels = len(w.out)
	for range width * height {
		green.put(w, 128)
		byteCode.put(w, 128)
		byteCode.put(w, 128)
		byteCode.put(w, 255)
	}
	data := w.bytes()
	out := make([]byte, 20, 20+len(data)+1)
	copy(out, "RIFF\x00\x00\x00\x00WEBPVP8L")
	binary.LittleEndian.PutUint32(out[16:], uint32(len(data)))
	out = append(out, data...)
	if len(data)%2 == 1 {
		out = append(out, 0)
	}
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
	return out, 20 + pixels, nil
}

// HeldAt returns how many bytes more the heap holds once decode, which
// reads picture, has read it up to at, than before it started: the
// garbage collected first, both times. It must not run beside other
// tests, whose memory it would count.
func HeldAt(picture []byte, at int, decode func(io.Reader) error) (int64, error) {
	var stats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&stats)
	r := &measuring{r: bytes.NewReader(picture), at: at, before: stats.HeapAlloc}
	if err := decode(r); err != nil {
		return 0, err
	}
	if !r.measured {
		return 0, fmt.Errorf("the picture was read whole before byte %d", at)
	}
	return r.held, nil
}

// measuring measures the heap the first time it is read from at or past
// at.
type measuring struct {
	r        io.Reader
	read, at int
	before   uint64
	held     int64
	measured bool
}

func (m *measuring) Read(p []byte) (int, error) {
	if !m.measured && m.read >= m.at {
		m.measured = true
		var stats runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&stats)
		m.held = int64(stats.HeapAlloc) - int64(m.before)
	}
	n, err := m.r.Read(p)
	m.read += n
	return n, err
}

// bitWriter writes a lossless WebP's bit stream, the least significant
// bit of each byte first.
type bitWriter struct {
	out   []byte
	bits  uint64
	count uint
}

func (w *bitWriter) write(value uint32, n uint) {
	w.bits |= uint64(value) << w.count
	for w.count += n; w.count >= 8; w.count -= 8 {
		w.out = append(w.out, byte(w.bits))
		w.bits >>= 8
	}
}

func (w *bitWriter) bytes() []byte {
	if w.count > 0 {
		return append(w.out, byte(w.bits))
	}
	return w.out
}

// prefixCode is a canonical prefix code, given by the length of each
// symbol's code, 0 for a symbol not used.
type prefixCode struct{ lengths, codes []uint32 }

func newPrefixCode(lengths []uint32) prefixCode {
	var counts, next [16]uint32
	for _, l := range lengths {
		counts[l]++
	}
	counts[0] = 0
	code := uint32(0)
	for l := 1; l < len(next); l++ {
		code = (code + counts[l-1]) << 1
		next[l] = code
	}
	codes := make([]uint32, len(lengths))
	for symbol, l := range lengths {
		if l > 0 {
			codes[symbol] = next[l]
			next[l]++
		}
	}
	return prefixCode{lengths, codes}
}

// put writes a symbol's code, its most significant bit first.
func (c prefixCode) put(w *bitWriter, symbol uint32) {
	for l := c.lengths[symbol]; l > 0; l-- {
		w.write(c.codes[symbol]>>(l-1)&1, 1)
	}
}

// completeLengths gives each of n symbols, n at least 2, a code of k or
// k+1 bits that together make a complete code.
func completeLengths(n int) []uint32 {
	k := 0
	for 1<<(k+1) <= n {
		k++
	}
	lengths := make([]uint32, n)
	longer := 2 * (n - 1<<k)
	for i := range lengths {
		lengths[i] = uint32(k)
		if i >= n-longer {
			lengths[i]++
		}
	}
	return lengths
}

// codeLengthOrder is the order the lengths of the code over code lengths
// are written in.
var codeLengthOrder = [19]int{17, 18, 0, 1, 2, 3, 4, 5, 16, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}

// putCode writes a prefix code in the normal form: its lengths, each
// written by a code over the lengths used and 16, which repeats the last
// length that is not 0 three to six times.
func putCode(w *bitWriter, lengths []uint32) {
	w.write(0, 1)
	var lengthLengths [19]uint32
	var seen [19]bool
	used := []uint32{16}
	for _, l := range lengths {
		if !seen[l] {
			seen[l] = true
			used = append(used, l)
		}
	}
	for i, l := range completeLengths(len(used)) {
		lengthLengths[used[i]] = l
	}
	written := 4
	for i, symbol := range codeLengthOrder {
		if lengthLengths[symbol] != 0 {
			written = max(written, i+1)
		}
	}
	w.write(uint32(written-4), 4)
	for _, symbol := range codeLengthOrder[:written] {
		w.write(lengthLengths[symbol], 3)
	}
	lengthCode := newPrefixCode(lengthLengths[:])
	w.write(0, 1) // every symbol's length is written
	// A repeat before any length that is not 0 repeats 8.
	previous := uint32(8)
	for i := 0; i < len(lengths); {
		l := lengths[i]
		run := 1
		for i+run < len(lengths) && lengths[i+run] == l {
			run++
		}
		i += run
		if l == 0 {
			for ; run > 0; run-- {
				lengthCode.put(w, 0)
			}
			continue
		}
		if l != previous {
			lengthCode.put(w, l)
			previous = l
			run--
		}
		for run >= 3 {
			repeat := min(run, 6)
			lengthCode.put(w, 16)
			w.write(uint32(repeat-3), 2)
			run -= repeat
		}
		for ; run > 0; run-- {
			lengthCode.put(w, l)
		}
	}
}

// putOneSymbol writes a prefix code of one symbol of 8 bits.
func putOneSymbol(w *bitWriter, symbol uint32) {
	w.write(1, 1) // the simple form
	w.write(0, 1) // of one symbol
	w.write(1, 1) // of 8 bits
	w.write(symbol, 8)
}
