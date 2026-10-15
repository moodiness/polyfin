// Package qrcode draws QR codes (ISO/IEC 18004) of bytes, at error
// correction level M, so that an authenticator app can read a secret from
// the screen without the secret leaving the server for a service that would
// draw it. It follows the construction of Project Nayuki's QR Code
// generator: the smallest version that holds the bytes, then the mask with
// the lowest penalty.
package qrcode

import (
	"errors"
	"strings"
)

// ErrTooLong reports bytes that no version holds.
var ErrTooLong = errors.New("too long for a QR code")

// Code is a QR code: Size modules a side, dark or light.
type Code struct {
	Size    int
	modules []bool
}

// Dark reports whether the module at column x and row y is dark.
func (c Code) Dark(x, y int) bool { return c.modules[y*c.Size+x] }

// Rows writes the code a row a line, 1 for a dark module, 0 for a light one.
func (c Code) Rows() []string {
	rows := make([]string, c.Size)
	var row strings.Builder
	for y := range c.Size {
		row.Reset()
		for x := range c.Size {
			if c.Dark(x, y) {
				row.WriteByte('1')
			} else {
				row.WriteByte('0')
			}
		}
		rows[y] = row.String()
	}
	return rows
}

// The error correction of level M, by version: codewords per block and
// blocks.
var (
	eccPerBlock = [41]int{0, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26, 30, 22, 22, 24, 24, 28, 28, 26, 26, 26,
		26, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28}
	eccBlocks = [41]int{0, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5, 5, 8, 9, 9, 10, 10, 11, 13, 14, 16,
		17, 17, 18, 20, 21, 23, 25, 26, 28, 29, 31, 33, 35, 37, 38, 40, 43, 45, 47, 49}
)

// formatLevelM is level M's two bits of the format information.
const formatLevelM = 0

// Encode draws data in byte mode, in the smallest version that holds it,
// with the mask of the lowest penalty.
func Encode(data []byte) (Code, error) {
	for version := 1; version <= 40; version++ {
		countBits := 8
		if version >= 10 {
			countBits = 16
		}
		if len(data) >= 1<<countBits {
			continue
		}
		if 4+countBits+8*len(data) <= 8*dataCodewords(version) {
			return encode(data, version, -1), nil
		}
	}
	return Code{}, ErrTooLong
}

// encode draws data in version, with mask, or the best one when it is -1.
func encode(data []byte, version, mask int) Code {
	capacity := 8 * dataCodewords(version)
	var bits bitBuffer
	bits.append(0b0100, 4)
	if version >= 10 {
		bits.append(len(data), 16)
	} else {
		bits.append(len(data), 8)
	}
	for _, b := range data {
		bits.append(int(b), 8)
	}
	bits.append(0, min(4, capacity-len(bits)))
	bits.append(0, (8-len(bits)%8)%8)
	for pad := 0xEC; len(bits) < capacity; pad ^= 0xEC ^ 0x11 {
		bits.append(pad, 8)
	}
	codewords := make([]byte, len(bits)/8)
	for i, bit := range bits {
		if bit {
			codewords[i>>3] |= 1 << (7 - i&7)
		}
	}

	q := newMatrix(version)
	q.drawFunctionPatterns()
	q.drawCodewords(withErrorCorrection(codewords, version))
	if mask < 0 {
		best := 0
		for candidate := range 8 {
			q.applyMask(candidate)
			q.drawFormatBits(candidate)
			if penalty := q.penalty(); mask < 0 || penalty < best {
				mask, best = candidate, penalty
			}
			q.applyMask(candidate)
		}
	}
	q.applyMask(mask)
	q.drawFormatBits(mask)
	return Code{Size: q.size, modules: q.modules}
}

type bitBuffer []bool

func (b *bitBuffer) append(value, length int) {
	for i := length - 1; i >= 0; i-- {
		*b = append(*b, value>>i&1 == 1)
	}
}

// rawDataModules counts the modules of version that hold data and error
// correction, the remainder bits included.
func rawDataModules(version int) int {
	result := (16*version+128)*version + 64
	if version >= 2 {
		alignments := version/7 + 2
		result -= (25*alignments-10)*alignments - 55
		if version >= 7 {
			result -= 36
		}
	}
	return result
}

// dataCodewords counts the codewords of version at level M that hold data.
func dataCodewords(version int) int {
	return rawDataModules(version)/8 - eccPerBlock[version]*eccBlocks[version]
}

// withErrorCorrection splits data into the blocks of version, adds their
// Reed-Solomon codewords, and interleaves them.
func withErrorCorrection(data []byte, version int) []byte {
	blocks, eccLength := eccBlocks[version], eccPerBlock[version]
	raw := rawDataModules(version) / 8
	short := blocks - raw%blocks
	shortLength := raw / blocks
	divisor := reedSolomonDivisor(eccLength)
	all := make([][]byte, blocks)
	k := 0
	for i := range blocks {
		length := shortLength - eccLength
		if i >= short {
			length++
		}
		block := append([]byte{}, data[k:k+length]...)
		k += length
		ecc := reedSolomonRemainder(block, divisor)
		if i < short {
			// A placeholder, so that every block is as long; skipped below.
			block = append(block, 0)
		}
		all[i] = append(block, ecc...)
	}
	result := make([]byte, 0, raw)
	for i := range all[0] {
		for j, block := range all {
			if i != shortLength-eccLength || j >= short {
				result = append(result, block[i])
			}
		}
	}
	return result
}

// multiply multiplies in GF(2^8) modulo x^8 + x^4 + x^3 + x^2 + 1.
func multiply(x, y byte) byte {
	z := 0
	for i := 7; i >= 0; i-- {
		z = (z << 1) ^ ((z >> 7) * 0x11D)
		z ^= int(y>>i&1) * int(x)
	}
	return byte(z)
}

func reedSolomonDivisor(degree int) []byte {
	result := make([]byte, degree)
	result[degree-1] = 1
	root := byte(1)
	for range degree {
		for j := range result {
			result[j] = multiply(result[j], root)
			if j+1 < len(result) {
				result[j] ^= result[j+1]
			}
		}
		root = multiply(root, 0x02)
	}
	return result
}

func reedSolomonRemainder(data, divisor []byte) []byte {
	result := make([]byte, len(divisor))
	for _, b := range data {
		factor := b ^ result[0]
		copy(result, result[1:])
		result[len(result)-1] = 0
		for i, coefficient := range divisor {
			result[i] ^= multiply(coefficient, factor)
		}
	}
	return result
}

// matrix is a code being drawn; function marks the modules of its
// patterns, which hold no data and no mask.
type matrix struct {
	version, size int
	modules       []bool
	function      []bool
}

func newMatrix(version int) *matrix {
	size := version*4 + 17
	return &matrix{version: version, size: size, modules: make([]bool, size*size), function: make([]bool, size*size)}
}

func (q *matrix) setFunction(x, y int, dark bool) {
	q.modules[y*q.size+x] = dark
	q.function[y*q.size+x] = true
}

// alignmentPositions are the centers of the alignment patterns, on each
// axis.
func (q *matrix) alignmentPositions() []int {
	if q.version == 1 {
		return nil
	}
	count := q.version/7 + 2
	step := (q.version*8 + count*3 + 5) / (count*4 - 4) * 2
	result := make([]int, count)
	result[0] = 6
	for i, position := count-1, q.size-7; i >= 1; i, position = i-1, position-step {
		result[i] = position
	}
	return result
}

func (q *matrix) drawFunctionPatterns() {
	for i := range q.size {
		q.setFunction(6, i, i%2 == 0)
		q.setFunction(i, 6, i%2 == 0)
	}
	for _, center := range [][2]int{{3, 3}, {q.size - 4, 3}, {3, q.size - 4}} {
		for dy := -4; dy <= 4; dy++ {
			for dx := -4; dx <= 4; dx++ {
				x, y := center[0]+dx, center[1]+dy
				if x >= 0 && x < q.size && y >= 0 && y < q.size {
					distance := max(abs(dx), abs(dy))
					q.setFunction(x, y, distance != 2 && distance != 4)
				}
			}
		}
	}
	positions := q.alignmentPositions()
	last := len(positions) - 1
	for i, x := range positions {
		for j, y := range positions {
			// Not over the finder patterns.
			if (i == 0 && j == 0) || (i == 0 && j == last) || (i == last && j == 0) {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					q.setFunction(x+dx, y+dy, max(abs(dx), abs(dy)) != 1)
				}
			}
		}
	}
	// Reserved now, written once the mask is chosen.
	q.drawFormatBits(0)
	if q.version >= 7 {
		remainder := q.version
		for range 12 {
			remainder = (remainder << 1) ^ ((remainder >> 11) * 0x1F25)
		}
		bits := q.version<<12 | remainder
		for i := range 18 {
			dark := bits>>i&1 == 1
			a, b := q.size-11+i%3, i/3
			q.setFunction(a, b, dark)
			q.setFunction(b, a, dark)
		}
	}
}

func (q *matrix) drawFormatBits(mask int) {
	data := formatLevelM<<3 | mask
	remainder := data
	for range 10 {
		remainder = (remainder << 1) ^ ((remainder >> 9) * 0x537)
	}
	bits := (data<<10 | remainder) ^ 0x5412
	bit := func(i int) bool { return bits>>i&1 == 1 }
	for i := range 6 {
		q.setFunction(8, i, bit(i))
	}
	q.setFunction(8, 7, bit(6))
	q.setFunction(8, 8, bit(7))
	q.setFunction(7, 8, bit(8))
	for i := 9; i < 15; i++ {
		q.setFunction(14-i, 8, bit(i))
	}
	for i := range 8 {
		q.setFunction(q.size-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		q.setFunction(8, q.size-15+i, bit(i))
	}
	q.setFunction(8, q.size-8, true)
}

// drawCodewords lays data in the zigzag of two-module columns, from the
// bottom right corner, around the function patterns.
func (q *matrix) drawCodewords(data []byte) {
	i := 0
	for right := q.size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		upward := (right+1)&2 == 0
		for vertical := range q.size {
			for j := range 2 {
				x, y := right-j, vertical
				if upward {
					y = q.size - 1 - vertical
				}
				if !q.function[y*q.size+x] && i < len(data)*8 {
					q.modules[y*q.size+x] = data[i>>3]>>(7-i&7)&1 == 1
					i++
				}
			}
		}
	}
}

// applyMask inverts the data modules mask selects; applying it twice undoes
// it.
func (q *matrix) applyMask(mask int) {
	for y := range q.size {
		for x := range q.size {
			var invert bool
			switch mask {
			case 0:
				invert = (x+y)%2 == 0
			case 1:
				invert = y%2 == 0
			case 2:
				invert = x%3 == 0
			case 3:
				invert = (x+y)%3 == 0
			case 4:
				invert = (x/3+y/2)%2 == 0
			case 5:
				invert = x*y%2+x*y%3 == 0
			case 6:
				invert = (x*y%2+x*y%3)%2 == 0
			case 7:
				invert = ((x+y)%2+x*y%3)%2 == 0
			}
			if invert && !q.function[y*q.size+x] {
				q.modules[y*q.size+x] = !q.modules[y*q.size+x]
			}
		}
	}
}

// penalty scores what makes a code hard to read: long runs of a color,
// blocks of a color, patterns that look like finders, and an unbalanced
// share of dark modules.
func (q *matrix) penalty() int {
	dark := func(x, y int) bool { return q.modules[y*q.size+x] }
	result := 0
	for _, transpose := range []bool{false, true} {
		at := func(a, b int) bool {
			if transpose {
				return dark(b, a)
			}
			return dark(a, b)
		}
		for b := range q.size {
			run := 1
			for a := 1; a <= q.size; a++ {
				if a < q.size && at(a, b) == at(a-1, b) {
					run++
					continue
				}
				if run >= 5 {
					result += 3 + run - 5
				}
				run = 1
			}
			// Light four modules, then dark-light-dark-dark-dark-light-dark,
			// or the reverse; outside the code counts as light.
			pattern := []bool{true, false, true, true, true, false, true}
			for a := -4; a+11 <= q.size+4; a++ {
				module := func(i int) bool { return i >= 0 && i < q.size && at(i, b) }
				matches := true
				for i, want := range pattern {
					if module(a+4+i) != want {
						matches = false
						break
					}
				}
				if !matches {
					continue
				}
				before, after := true, true
				for i := range 4 {
					before = before && !module(a+i)
					after = after && !module(a+11+i)
				}
				if before {
					result += 40
				}
				if after {
					result += 40
				}
			}
		}
	}
	darkCount := 0
	for y := range q.size {
		for x := range q.size {
			if dark(x, y) {
				darkCount++
			}
			if x+1 < q.size && y+1 < q.size {
				color := dark(x, y)
				if color == dark(x+1, y) && color == dark(x, y+1) && color == dark(x+1, y+1) {
					result += 3
				}
			}
		}
	}
	total := q.size * q.size
	k := (abs(darkCount*20-total*10)+total-1)/total - 1
	return result + k*10
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
