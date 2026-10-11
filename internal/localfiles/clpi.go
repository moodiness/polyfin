package localfiles

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// A Blu-ray disc's clip, BDMV/STREAM/NNNNN.m2ts, is described by its
// clip information file, BDMV/CLIPINF/NNNNN.clpi, whose EP map lists the
// entry points of its video: the I-frames a player may start from, each
// with its time and its source packet. Their times, mapped through a
// playlist's play items, are the keyframes of the title, on which its
// segments are cut so that its video may be copied (see Keyframes).

// errNotClip reports a file that is not a Blu-ray clip's information.
var errNotClip = errors.New("not a Blu-ray clip's information")

// errNoKeyframes reports a disc whose keyframes Polyfin does not read.
var errNoKeyframes = errors.New("the disc's keyframes are not read")

// maxClipInfoSize bounds the clip information files read: an EP map takes
// a few bytes per entry point, one a second or so.
const maxClipInfoSize = 8 << 20

// epPrecision is the precision of an entry point's time, in units of
// 90 kHz: an EP map leaves out the time's lowest 9 bits.
const epPrecision = 1 << 9

// readEPMap reads a clip information file's EP map: the times of the
// video's entry points, in units of 90 kHz of the clip's timestamps, in
// order, each known to epPrecision.
func readEPMap(data []byte) ([]uint64, error) {
	if len(data) < 20 || string(data[:4]) != "HDMV" {
		return nil, errNotClip
	}
	cpi := int(binary.BigEndian.Uint32(data[16:]))
	if cpi < 20 || cpi+6 > len(data) || binary.BigEndian.Uint32(data[cpi:]) == 0 || data[cpi+5]&0x0F != 1 {
		return nil, fmt.Errorf("%w: no EP map", errNotClip)
	}
	// The EP map: a reserved byte, the number of streams it maps, then
	// for each its PID, 10 reserved bits, its type (1 for video), its
	// numbers of coarse and fine entries, 16 and 18 bits, and where its
	// entries start, from the EP map's start.
	epMap := cpi + 6
	if epMap+2 > len(data) {
		return nil, fmt.Errorf("%w: the EP map is cut short", errNotClip)
	}
	for i := range int(data[epMap+1]) {
		at := epMap + 2 + i*12
		if at+12 > len(data) {
			return nil, fmt.Errorf("%w: the EP map is cut short", errNotClip)
		}
		fields := uint64(binary.BigEndian.Uint16(data[at+2:]))<<32 | uint64(binary.BigEndian.Uint32(data[at+4:]))
		if fields>>34&0x0F != 1 {
			continue
		}
		coarse, fine := int(fields>>18&0xFFFF), int(fields&0x3FFFF)
		return epTimes(data, epMap+int(binary.BigEndian.Uint32(data[at+8:])), coarse, fine)
	}
	return nil, fmt.Errorf("%w: the EP map maps no video", errNotClip)
}

// epTimes reads the times of a stream's entry points, whose entries start
// at start: the offset of its fine entries from start, then its coarse
// entries, 8 bytes each, the first fine entry each covers (18 bits), the
// time's bits 32 to 19 (14 bits) and a source packet; then its fine
// entries, 4 bytes each, a flag and the I-frame's end (4 bits), the time's
// bits 19 to 9 (11 bits) and the low bits of a source packet.
func epTimes(data []byte, start, coarse, fine int) ([]uint64, error) {
	if start+4 > len(data) || start+4+coarse*8 > len(data) || coarse == 0 && fine > 0 {
		return nil, fmt.Errorf("%w: the EP map is cut short", errNotClip)
	}
	fines := start + int(binary.BigEndian.Uint32(data[start:]))
	if fines+fine*4 > len(data) {
		return nil, fmt.Errorf("%w: the EP map is cut short", errNotClip)
	}
	entry := func(c int) (first int, high uint64) {
		word := binary.BigEndian.Uint32(data[start+4+c*8:])
		return int(word >> 14), uint64(word & 0x3FFF)
	}
	times := make([]uint64, 0, fine)
	c := 0
	for i := range fine {
		for c+1 < coarse {
			if next, _ := entry(c + 1); next > i {
				break
			}
			c++
		}
		first, high := entry(c)
		if first > i {
			return nil, fmt.Errorf("%w: entry point %d has no coarse entry", errNotClip, i)
		}
		low := uint64(binary.BigEndian.Uint32(data[fines+i*4:]) >> 17 & 0x7FF)
		// Bit 19 is in both: the fine entry's is kept.
		times = append(times, (high&^1)<<19+low<<9)
	}
	slices.Sort(times)
	return times, nil
}

// titleKeyframes maps the entry points of a playlist's clips, by clip,
// onto its title's timeline: each play item's, from its in time to its
// out time, follow the items before it. An entry point just before an
// item's in time, within epPrecision, starts it.
func titleKeyframes(p playlist, points map[string][]uint64) []time.Duration {
	var times []time.Duration
	var offset uint64
	for _, part := range p.clips {
		in, out := uint64(part.in)*2, uint64(part.out)*2
		for _, pts := range points[part.clip] {
			if pts+epPrecision <= in || pts >= out {
				continue
			}
			at := offset + max(pts, in) - in
			times = append(times, time.Duration(at)*time.Second/90000)
		}
		offset += out - in
	}
	return times
}

// Keyframes reads the keyframe times of the version key names (see
// Input), on its title's timeline: a Blu-ray disc's playlist's, from its
// clips' EP maps, in its folder or in an image's ISO 9660 file system.
// Those of a DVD, of a title libbluray chooses, and of an image with a
// UDF file system only are not read.
func (s *Service) Keyframes(ctx context.Context, key string) ([]time.Duration, error) {
	d, err := s.disc(ctx, key)
	if err != nil {
		return nil, err
	}
	if d.kind != DiscBluRay || d.title < 0 {
		return nil, errNoKeyframes
	}
	if !d.image {
		return blurayKeyframes(folderReader(d.path), d.title)
	}
	file, err := os.Open(d.path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	img, err := openISO(file)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNoKeyframes, err)
	}
	return blurayKeyframes(func(names ...string) ([]byte, error) {
		entry, err := img.lookup(names...)
		if err != nil {
			return nil, err
		}
		if entry.dir || entry.size > maxClipInfoSize {
			return nil, fmt.Errorf("%s is no clip information", strings.Join(names, "/"))
		}
		return io.ReadAll(img.reader(entry))
	}, d.title)
}

// blurayKeyframes reads the keyframe times of a disc's playlist number,
// read reading the disc's files by their path, any case.
func blurayKeyframes(read func(names ...string) ([]byte, error), number int) ([]time.Duration, error) {
	data, err := read("BDMV", "PLAYLIST", fmt.Sprintf("%05d.mpls", number))
	if err != nil {
		return nil, err
	}
	p, err := readPlaylist(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	points := map[string][]uint64{}
	for _, part := range p.clips {
		if _, ok := points[part.clip]; ok {
			continue
		}
		data, err := read("BDMV", "CLIPINF", part.clip+".clpi")
		if err != nil {
			return nil, err
		}
		if points[part.clip], err = readEPMap(data); err != nil {
			return nil, fmt.Errorf("clip %s: %w", part.clip, err)
		}
	}
	return titleKeyframes(p, points), nil
}

// folderReader reads the files of the disc folder dir by their path, any
// case, through no symbolic link leading out of it.
func folderReader(dir string) func(names ...string) ([]byte, error) {
	return func(names ...string) ([]byte, error) {
		at := dir
		for _, name := range names {
			entries, err := os.ReadDir(at)
			if err != nil {
				return nil, err
			}
			i := slices.IndexFunc(entries, func(e os.DirEntry) bool { return e.Name() == name })
			if i < 0 {
				i = slices.IndexFunc(entries, func(e os.DirEntry) bool { return strings.EqualFold(e.Name(), name) })
			}
			if i < 0 {
				return nil, fmt.Errorf("%w: no %s in %s", os.ErrNotExist, name, at)
			}
			at = filepath.Join(at, entries[i].Name())
		}
		real, err := filepath.EvalSymlinks(at)
		if err != nil || !within(dir, real) {
			return nil, fmt.Errorf("%w: %s", os.ErrNotExist, at)
		}
		file, err := os.Open(real)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, maxClipInfoSize+1))
		if err == nil && len(data) > maxClipInfoSize {
			err = fmt.Errorf("%s is too large", at)
		}
		return data, err
	}
}
