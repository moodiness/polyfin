package ocr

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"iter"
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
// tells of every subpicture: the palette, as luminances, and the size of
// the video. A track copied from a DVD's program stream, without its IFO
// files, has no palette, and may have no size.
type vobsubIndex struct {
	palette       *[16]uint8
	width, height int
}

// dvdWidth and dvdHeight are the size of a DVD's PAL video, the largest:
// a track's pictures may lie anywhere within it, whatever smaller size
// its index tells.
const dvdWidth, dvdHeight = 720, 576

// screen is the part of the video a track's pictures are drawn within:
// the size its index tells, a DVD's at least, at most maxWidth by
// maxHeight; maxWidth by maxHeight when it tells none.
func (index vobsubIndex) screen() (width, height int) {
	if index.width <= 0 || index.height <= 0 {
		return maxWidth, maxHeight
	}
	return min(max(index.width, dvdWidth), maxWidth), min(max(index.height, dvdHeight), maxHeight)
}

type vobsubEntry struct {
	at       time.Duration
	position int64
}

// parseIndex reads a .idx file or a CodecPrivate. It hands the
// subpictures a .idx file lists for its first track to entry, when there
// is one, in the file's order, by time and position in the .sub file, and
// keeps none: an error from entry stops the reading.
func parseIndex(data []byte, entry func(vobsubEntry) error) (vobsubIndex, error) {
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
		case "size":
			width, height, _ := strings.Cut(value, "x")
			index.width, _ = strconv.Atoi(strings.TrimSpace(width))
			index.height, _ = strconv.Atoi(strings.TrimSpace(height))
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
			if entry == nil {
				continue
			}
			if err := entry(vobsubEntry{at: at, position: offset}); err != nil {
				return vobsubIndex{}, err
			}
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
// track the .idx file lists. It hands each picture to emit, in order, as
// soon as its end is known (see decodeUnits).
//
// The .idx file is read twice. Whole first, for its palette and size, and
// so that a file with a line that cannot be read, or that lists a
// subpicture past the .sub file, hands over nothing. Then line by line,
// each subpicture read from the .sub file and decoded before the next
// line is: one is held at a time, however many the .idx file lists.
//
// A line listing a position before the end of the subpicture last read
// is skipped: its bytes were read already. The subpictures read are then
// apart from one another in the .sub file, which bounds them all by its
// size, whatever the lines of the .idx file say, and so does it bound
// the pixels they compose (see pixelBudget).
func VobSub(ctx context.Context, idx, sub []byte, emit func(Picture) error) error {
	index, err := parseIndex(idx, func(entry vobsubEntry) error {
		if entry.position < 0 || entry.position >= int64(len(sub)) {
			return fmt.Errorf("%w: position past the .sub file", errInvalid)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return decodeUnits(ctx, pairUnits(idx, sub), index, newPixelBudget(len(sub)), emit)
}

// pairUnits are the subpicture units of a .idx and .sub pair that was
// checked, each read when its line of the .idx file is.
func pairUnits(idx, sub []byte) iter.Seq2[timedUnit, error] {
	return func(yield func(timedUnit, error) bool) {
		stopped := errors.New("stopped")
		end := 0
		_, _ = parseIndex(idx, func(entry vobsubEntry) error {
			if entry.position < int64(end) {
				return nil
			}
			unit, next, err := subpictureAt(sub, int(entry.position))
			end = next
			if !yield(timedUnit{at: entry.at, data: unit}, err) {
				return stopped
			}
			return nil
		})
	}
}

// VobSubBlocks reads the pictures of a Matroska VobSub track, from its
// CodecPrivate and blocks, as VobSub does.
func VobSubBlocks(ctx context.Context, private []byte, blocks []container.Block, emit func(Picture) error) error {
	index, err := parseIndex(private, nil)
	if err != nil {
		return err
	}
	return decodeUnits(ctx, blockUnits(blocks), index, newPixelBudget(blocksSize(blocks)), emit)
}

// blockUnits are the subpicture units of a Matroska VobSub track's blocks.
func blockUnits(blocks []container.Block) iter.Seq2[timedUnit, error] {
	return func(yield func(timedUnit, error) bool) {
		for _, block := range blocks {
			if !yield(timedUnit{at: block.Start, duration: block.Duration, data: block.Data}, nil) {
				return
			}
		}
	}
}

// timedUnit is a subpicture unit with its time, and its block's duration
// when it has one.
type timedUnit struct {
	at, duration time.Duration
	data         []byte
}

// decodeUnits decodes subpicture units as it is given them, each or the
// error that kept it from being read: each shows from its start delay to
// its stop delay, its block's end, or the next one. It hands each picture
// to emit once its end is known, holding only one that waits for the
// next. An error from emit stops the decoding, and so does ctx when it
// ends: it is asked before each unit. So does budget when the pixels
// decoded and prepared run past it.
func decodeUnits(ctx context.Context, units iter.Seq2[timedUnit, error], index vobsubIndex, budget *pixelBudget, emit func(Picture) error) error {
	width, height := index.screen()
	var waiting Picture
	open := false
	for unit, err := range units {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		spu, err := decodeSubpicture(unit.data, index.palette, width, height, budget)
		if err != nil {
			return err
		}
		start := unit.at + spu.start
		if open {
			waiting.End = start
			if err := emit(waiting); err != nil {
				return err
			}
		}
		open = false
		prepared, ok, err := prepare(spu.bitmap, budget)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		picture := Picture{Start: start, Forced: spu.forced, Image: prepared}
		switch {
		case spu.stops:
			picture.End = unit.at + spu.stop
		case unit.duration > 0:
			picture.End = unit.at + unit.duration
		default:
			waiting, open = picture, true
			continue
		}
		if err := emit(picture); err != nil {
			return err
		}
	}
	if open {
		waiting.End = waiting.Start + lastShown
		return emit(waiting)
	}
	return nil
}

// subpictureAt reads the subpicture unit starting in the program stream
// packet at position: its private stream packets' payloads, past their
// substream number, until the unit's size. It returns the unit, and
// where the last packet read for it ends.
func subpictureAt(sub []byte, position int) ([]byte, int, error) {
	var unit []byte
	size := -1
	substream := -1
	for position+6 <= len(sub) {
		if sub[position] != 0 || sub[position+1] != 0 || sub[position+2] != 1 {
			return nil, 0, fmt.Errorf("%w: no packet start at %d", errInvalid, position)
		}
		code := sub[position+3]
		switch {
		case code == 0xba:
			// A pack header: MPEG-2's, with its stuffing, or MPEG-1's.
			if sub[position+4]&0xc0 == 0x40 {
				if position+14 > len(sub) {
					return nil, 0, fmt.Errorf("%w: short pack header", errInvalid)
				}
				position += 14 + int(sub[position+13]&7)
			} else {
				position += 12
			}
			continue
		case code == 0xb9:
			return nil, 0, fmt.Errorf("%w: the program stream ends within a subpicture", errInvalid)
		}
		length := int(binary.BigEndian.Uint16(sub[position+4 : position+6]))
		end := position + 6 + length
		if end > len(sub) {
			return nil, 0, fmt.Errorf("%w: packet past the end", errInvalid)
		}
		if code == 0xbd {
			payload := sub[position+6 : end]
			if len(payload) < 3 || 3+int(payload[2]) >= len(payload) {
				return nil, 0, fmt.Errorf("%w: short packet", errInvalid)
			}
			data := payload[3+int(payload[2]):]
			if number := int(data[0]); substream < 0 || number == substream {
				substream = number
				unit = append(unit, data[1:]...)
				if size < 0 && len(unit) >= 2 {
					size = int(binary.BigEndian.Uint16(unit))
				}
				if size >= 0 && len(unit) >= size {
					return unit[:size], end, nil
				}
			}
		}
		position = end
	}
	return nil, 0, fmt.Errorf("%w: the .sub file ends within a subpicture", errInvalid)
}

// subpicture is a decoded subpicture unit.
type subpicture struct {
	bitmap      bitmap
	start, stop time.Duration
	stops       bool
	forced      bool
}

// decodeSubpicture decodes a subpicture unit through the palette's
// luminances, on a screen of width by height: the part of its picture
// past the screen is neither drawn nor allocated. The pixels drawn are
// taken from budget first.
func decodeSubpicture(data []byte, palette *[16]uint8, screenWidth, screenHeight int, budget *pixelBudget) (subpicture, error) {
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
	// The lines are decoded at their width, the picture's; only the part
	// on screen is drawn.
	width := x2 - x1 + 1
	shownWidth, shownHeight := min(x2+1, screenWidth)-x1, min(y2+1, screenHeight)-y1
	if top < 0 || shownWidth <= 0 || shownHeight <= 0 {
		return spu, nil
	}
	var values [4]pixel
	for i := range values {
		values[i] = pixel{luma: conventional[i], alpha: alphas[i] * 17}
		if palette != nil {
			values[i].luma = palette[colors[i]]
		}
	}
	if err := budget.spend(shownWidth * shownHeight); err != nil {
		return subpicture{}, err
	}
	spu.bitmap = newBitmap(shownWidth, shownHeight)
	fields := [2]int{top * 2, bottom * 2}
	for y := range shownHeight {
		field := &fields[y%2]
		*field = decodeSPULine(data, *field, width, func(x, n int, value uint8) {
			for ; n > 0 && x < shownWidth; n-- {
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
