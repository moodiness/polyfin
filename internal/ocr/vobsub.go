package ocr

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/container"
)

// VobSub subtitles, a DVD's, are subpicture units: run-length pictures of
// two bits a pixel, whose four values a control sequence maps to four of
// the 16 colors of the disc's palette and to four opacities, and which
// tell when the picture shows and when it goes. A .sub file holds them in
// MPEG program stream packets, at the positions its .idx file lists with
// their times, after the palette; a Matroska track holds one a block, the
// .idx file's header as its CodecPrivate.

// spuTick is the unit of the delays of a subpicture's control sequences.
const spuTick = 1024 * time.Second / 90000

// vobsubIndex is what a .idx file, or a Matroska track's CodecPrivate,
// tells: the palette, as luminances, and for a .idx file the subpictures of
// its first track, by time and position in the .sub file. A track copied
// from a DVD's program stream, without its IFO files, has no palette.
type vobsubIndex struct {
	palette *[16]uint8
	entries []vobsubEntry
}

type vobsubEntry struct {
	at       time.Duration
	position int64
}

// parseIndex reads a .idx file or a CodecPrivate.
func parseIndex(data []byte) (vobsubIndex, error) {
	var index vobsubIndex
	tracks := 0
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		name, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(strings.ToLower(name)) {
		case "palette":
			palette := &[16]uint8{}
			colors := strings.Split(value, ",")
			for i := 0; i < len(colors) && i < 16; i++ {
				rgb, err := strconv.ParseUint(strings.TrimSpace(colors[i]), 16, 32)
				if err != nil {
					return vobsubIndex{}, fmt.Errorf("%w: palette %q", errInvalid, colors[i])
				}
				palette[i] = luma(uint8(rgb>>16), uint8(rgb>>8), uint8(rgb))
			}
			index.palette = palette
		case "id":
			tracks++
		case "timestamp":
			if tracks > 1 {
				continue
			}
			stamp, position, ok := strings.Cut(value, ",")
			_, position, _ = strings.Cut(position, ":")
			at, okTime := idxTime(strings.TrimSpace(stamp))
			offset, err := strconv.ParseInt(strings.TrimSpace(position), 16, 64)
			if !ok || !okTime || err != nil {
				return vobsubIndex{}, fmt.Errorf("%w: timestamp %q", errInvalid, value)
			}
			index.entries = append(index.entries, vobsubEntry{at: at, position: offset})
		}
	}
	return index, nil
}

// conventional are the luminances DVDs usually give the four values of a
// subpicture: a clear background, white text, a black outline and grey
// antialiasing. They stand in for a missing palette: the text is told
// from its outline by where each lies (see prepare), not by its color.
var conventional = [4]uint8{0, 255, 0, 128}

// idxTime reads a .idx time, hh:mm:ss:ms.
func idxTime(text string) (time.Duration, bool) {
	parts := strings.Split(text, ":")
	if len(parts) != 4 {
		return 0, false
	}
	var values [4]int
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return 0, false
		}
		values[i] = n
	}
	return time.Duration(values[0])*time.Hour + time.Duration(values[1])*time.Minute + time.Duration(values[2])*time.Second +
		time.Duration(values[3])*time.Millisecond, true
}

// luma is the luminance of a color, as BT.601 weighs it.
func luma(r, g, b uint8) uint8 {
	return uint8((299*int(r) + 587*int(g) + 114*int(b) + 500) / 1000)
}

// VobSub reads the pictures of a .idx and .sub pair: those of the first
// track the .idx file lists.
func VobSub(idx, sub []byte) ([]Picture, error) {
	index, err := parseIndex(idx)
	if err != nil {
		return nil, err
	}
	units := make([]timedUnit, 0, len(index.entries))
	for _, entry := range index.entries {
		if entry.position < 0 || entry.position >= int64(len(sub)) {
			return nil, fmt.Errorf("%w: position past the .sub file", errInvalid)
		}
		unit, err := subpictureAt(sub, int(entry.position))
		if err != nil {
			return nil, err
		}
		units = append(units, timedUnit{at: entry.at, data: unit})
	}
	return decodeUnits(units, index.palette)
}

// VobSubBlocks reads the pictures of a Matroska VobSub track, from its
// CodecPrivate and blocks.
func VobSubBlocks(private []byte, blocks []container.Block) ([]Picture, error) {
	index, err := parseIndex(private)
	if err != nil {
		return nil, err
	}
	units := make([]timedUnit, len(blocks))
	for i, block := range blocks {
		units[i] = timedUnit{at: block.Start, duration: block.Duration, data: block.Data}
	}
	return decodeUnits(units, index.palette)
}

// timedUnit is a subpicture unit with its time, and its block's duration
// when it has one.
type timedUnit struct {
	at, duration time.Duration
	data         []byte
}

// decodeUnits decodes subpicture units: each shows from its start delay
// to its stop delay, its block's end, or the next one.
func decodeUnits(units []timedUnit, palette *[16]uint8) ([]Picture, error) {
	var pictures []Picture
	open := false
	for _, unit := range units {
		spu, err := decodeSubpicture(unit.data, palette)
		if err != nil {
			return nil, err
		}
		start := unit.at + spu.start
		if open && pictures[len(pictures)-1].End == 0 {
			pictures[len(pictures)-1].End = start
		}
		open = false
		image, ok := prepare(spu.bitmap)
		if !ok {
			continue
		}
		picture := Picture{Start: start, Forced: spu.forced, Image: image}
		switch {
		case spu.stops:
			picture.End = unit.at + spu.stop
		case unit.duration > 0:
			picture.End = unit.at + unit.duration
		default:
			open = true
		}
		pictures = append(pictures, picture)
	}
	if open {
		last := &pictures[len(pictures)-1]
		last.End = last.Start + lastShown
	}
	return pictures, nil
}

// subpictureAt reads the subpicture unit starting in the program stream
// packet at position: its private stream packets' payloads, past their
// substream number, until the unit's size.
func subpictureAt(sub []byte, position int) ([]byte, error) {
	var unit []byte
	size := -1
	substream := -1
	for position+6 <= len(sub) {
		if sub[position] != 0 || sub[position+1] != 0 || sub[position+2] != 1 {
			return nil, fmt.Errorf("%w: no packet start at %d", errInvalid, position)
		}
		code := sub[position+3]
		switch {
		case code == 0xba:
			// A pack header: MPEG-2's, with its stuffing, or MPEG-1's.
			if sub[position+4]&0xc0 == 0x40 {
				if position+14 > len(sub) {
					return nil, fmt.Errorf("%w: short pack header", errInvalid)
				}
				position += 14 + int(sub[position+13]&7)
			} else {
				position += 12
			}
			continue
		case code == 0xb9:
			return nil, fmt.Errorf("%w: the program stream ends within a subpicture", errInvalid)
		}
		length := int(binary.BigEndian.Uint16(sub[position+4 : position+6]))
		end := position + 6 + length
		if end > len(sub) {
			return nil, fmt.Errorf("%w: packet past the end", errInvalid)
		}
		if code == 0xbd {
			payload := sub[position+6 : end]
			if len(payload) < 3 || 3+int(payload[2]) >= len(payload) {
				return nil, fmt.Errorf("%w: short packet", errInvalid)
			}
			data := payload[3+int(payload[2]):]
			if number := int(data[0]); substream < 0 || number == substream {
				substream = number
				unit = append(unit, data[1:]...)
				if size < 0 && len(unit) >= 2 {
					size = int(binary.BigEndian.Uint16(unit))
				}
				if size >= 0 && len(unit) >= size {
					return unit[:size], nil
				}
			}
		}
		position = end
	}
	return nil, fmt.Errorf("%w: the .sub file ends within a subpicture", errInvalid)
}

// subpicture is a decoded subpicture unit.
type subpicture struct {
	bitmap      bitmap
	start, stop time.Duration
	stops       bool
	forced      bool
}

// decodeSubpicture decodes a subpicture unit through the palette's
// luminances.
func decodeSubpicture(data []byte, palette *[16]uint8) (subpicture, error) {
	if len(data) < 4 {
		return subpicture{}, fmt.Errorf("%w: short subpicture", errInvalid)
	}
	var spu subpicture
	var colors, alphas [4]uint8
	var x1, x2, y1, y2 int
	top, bottom := -1, -1
	started := false
	offset := int(binary.BigEndian.Uint16(data[2:4]))
	for range 64 {
		if offset+4 > len(data) {
			return subpicture{}, fmt.Errorf("%w: control sequence past the end", errInvalid)
		}
		delay := time.Duration(binary.BigEndian.Uint16(data[offset:])) * spuTick
		next := int(binary.BigEndian.Uint16(data[offset+2:]))
		i := offset + 4
	commands:
		for i < len(data) {
			command := data[i]
			i++
			need := map[byte]int{0x03: 2, 0x04: 2, 0x05: 6, 0x06: 4}[command]
			if i+need > len(data) {
				return subpicture{}, fmt.Errorf("%w: short command", errInvalid)
			}
			args := data[i : i+need]
			i += need
			switch command {
			case 0x00:
				spu.forced = true
				fallthrough
			case 0x01:
				if !started {
					spu.start, started = delay, true
				}
			case 0x02:
				spu.stop, spu.stops = delay, true
			case 0x03:
				colors = [4]uint8{args[1] & 0xf, args[1] >> 4, args[0] & 0xf, args[0] >> 4}
			case 0x04:
				alphas = [4]uint8{args[1] & 0xf, args[1] >> 4, args[0] & 0xf, args[0] >> 4}
			case 0x05:
				x1, x2 = int(args[0])<<4|int(args[1])>>4, int(args[1]&0xf)<<8|int(args[2])
				y1, y2 = int(args[3])<<4|int(args[4])>>4, int(args[4]&0xf)<<8|int(args[5])
			case 0x06:
				top, bottom = int(binary.BigEndian.Uint16(args[0:2])), int(binary.BigEndian.Uint16(args[2:4]))
			case 0xff:
				break commands
			default:
				return subpicture{}, fmt.Errorf("%w: control command %#x", errInvalid, command)
			}
		}
		if next == offset || next+4 > len(data) {
			break
		}
		offset = next
	}
	width, height := x2-x1+1, y2-y1+1
	if top < 0 || width <= 0 || height <= 0 {
		return spu, nil
	}
	var values [4]pixel
	for i := range values {
		values[i] = pixel{luma: conventional[i], alpha: alphas[i] * 17}
		if palette != nil {
			values[i].luma = palette[colors[i]]
		}
	}
	spu.bitmap = newBitmap(width, height)
	fields := [2]int{top * 2, bottom * 2}
	for y := range height {
		field := &fields[y%2]
		*field = decodeSPULine(data, *field, width, func(x, n int, value uint8) {
			for ; n > 0 && x < width; n-- {
				spu.bitmap.set(x, y, values[value])
				x++
			}
		})
	}
	return spu, nil
}

// decodeSPULine decodes a line of a subpicture from the nibble at
// position: runs of 4 to 16 bits, a length and a two-bit value, length 0
// running to the end of the line. Lines start on a byte. It returns the
// next line's position.
func decodeSPULine(data []byte, position, width int, put func(x, n int, value uint8)) int {
	nibble := func() int {
		if position/2 >= len(data) {
			position++
			return 0
		}
		b := data[position/2]
		position++
		if position%2 == 1 {
			return int(b >> 4)
		}
		return int(b & 0xf)
	}
	for x := 0; x < width && position/2 < len(data); {
		v := nibble()
		if v < 0x4 {
			v = v<<4 | nibble()
			if v < 0x10 {
				v = v<<4 | nibble()
				if v < 0x40 {
					v = v<<4 | nibble()
				}
			}
		}
		run := v >> 2
		if run == 0 {
			run = width - x
		}
		put(x, run, uint8(v&3))
		x += run
	}
	if position%2 == 1 {
		position++
	}
	return position
}
