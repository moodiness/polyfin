package localfiles

import (
	"bytes"
	"cmp"
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
	entries, err := readEPEntries(data)
	if err != nil {
		return nil, err
	}
	times := make([]uint64, len(entries))
	for i, e := range entries {
		times[i] = e.pts
	}
	return times, nil
}

// epEntry is an entry point of a clip's video: its time, in units of
// 90 kHz, and its source packet, the 192-byte packet of the clip's file
// its I-frame starts in.
type epEntry struct {
	pts uint64
	spn uint32
}

// readEPEntries reads a clip information file's EP map: the video's entry
// points, in order (see readEPMap).
func readEPEntries(data []byte) ([]epEntry, error) {
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
		return epEntries(data, epMap+int(binary.BigEndian.Uint32(data[at+8:])), coarse, fine)
	}
	return nil, fmt.Errorf("%w: the EP map maps no video", errNotClip)
}

// epEntries reads a stream's entry points, whose entries start at start:
// the offset of its fine entries from start, then its coarse entries, 8
// bytes each, the first fine entry each covers (18 bits), the time's bits
// 32 to 19 (14 bits) and a source packet; then its fine entries, 4 bytes
// each, a flag and the I-frame's end (4 bits), the time's bits 19 to 9
// (11 bits) and the low 17 bits of a source packet.
func epEntries(data []byte, start, coarse, fine int) ([]epEntry, error) {
	if start+4 > len(data) || start+4+coarse*8 > len(data) || coarse == 0 && fine > 0 {
		return nil, fmt.Errorf("%w: the EP map is cut short", errNotClip)
	}
	fines := start + int(binary.BigEndian.Uint32(data[start:]))
	if fines+fine*4 > len(data) {
		return nil, fmt.Errorf("%w: the EP map is cut short", errNotClip)
	}
	entry := func(c int) (first int, high uint64, spn uint32) {
		word := binary.BigEndian.Uint32(data[start+4+c*8:])
		return int(word >> 14), uint64(word & 0x3FFF), binary.BigEndian.Uint32(data[start+8+c*8:])
	}
	entries := make([]epEntry, 0, fine)
	c := 0
	for i := range fine {
		for c+1 < coarse {
			if next, _, _ := entry(c + 1); next > i {
				break
			}
			c++
		}
		first, high, coarseSPN := entry(c)
		if first > i {
			return nil, fmt.Errorf("%w: entry point %d has no coarse entry", errNotClip, i)
		}
		word := binary.BigEndian.Uint32(data[fines+i*4:])
		low := uint64(word >> 17 & 0x7FF)
		// The fine entry's source packet replaces the coarse one's low 17
		// bits, and follows it.
		spn := coarseSPN&^0x1FFFF | word&0x1FFFF
		if spn < coarseSPN {
			spn += 0x20000
		}
		// Bit 19 of the time is in both: the fine entry's is kept.
		entries = append(entries, epEntry{pts: (high&^1)<<19 + low<<9, spn: spn})
	}
	slices.SortFunc(entries, func(a, b epEntry) int { return cmp.Compare(a.pts, b.pts) })
	return entries, nil
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
// clips' EP maps, in its folder or in an image's file system (see
// openDiscFS). Those of a DVD and of a title libbluray chooses are not
// read.
func (s *Service) Keyframes(ctx context.Context, key string) ([]time.Duration, error) {
	d, err := s.disc(ctx, key)
	if err != nil {
		return nil, err
	}
	if d.kind != DiscBluRay || d.title < 0 {
		return nil, errNoKeyframes
	}
	read := func(fsys discFS) func(names ...string) ([]byte, error) {
		return func(names ...string) ([]byte, error) { return readDiscFile(fsys, maxClipInfoSize, names...) }
	}
	if d.share {
		fsys, release, err := s.shareDiscFS(ctx, d)
		if err != nil {
			return nil, err
		}
		defer release()
		return blurayKeyframes(read(fsys), d.title)
	}
	if !d.image {
		return blurayKeyframes(folderReader(d.path), d.title)
	}
	file, err := os.Open(d.path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	fsys, err := openDiscFS(file)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNoKeyframes, err)
	}
	return blurayKeyframes(read(fsys), d.title)
}

// maxTitleClipInfo bounds the bytes of clip information read for a title.
const maxTitleClipInfo = 64 << 20

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
	budget := maxTitleClipInfo
	for _, part := range p.clips {
		if _, ok := points[part.clip]; ok {
			continue
		}
		data, err := read("BDMV", "CLIPINF", part.clip+".clpi")
		if err != nil {
			return nil, err
		}
		if budget -= len(data); budget < 0 {
			return nil, errors.New("the title's clip information is too large")
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
