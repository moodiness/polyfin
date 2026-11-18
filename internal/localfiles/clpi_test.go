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
	"testing"
	"time"
)

// writeCLPI writes a clip information file whose EP map lists, after a
// stream of another type, the video's entry points at times, in units of
// 90 kHz: a coarse entry each time the times' bits above the 20th change,
// as discs do.
func writeCLPI(times ...uint64) []byte {
	var coarse, fine []byte
	entries := 0
	for i, at := range times {
		if i == 0 || at>>20 != times[i-1]>>20 {
			coarse = binary.BigEndian.AppendUint32(coarse, uint32(i)<<14|uint32(at>>19&0x3FFF))
			coarse = binary.BigEndian.AppendUint32(coarse, uint32(i*100))
			entries++
		}
		fine = binary.BigEndian.AppendUint32(fine, uint32(at>>9&0x7FF)<<17|uint32(i*100&0x1FFFF))
	}
	const cpi = 40
	data := []byte("HDMV0200")
	data = binary.BigEndian.AppendUint32(data, 0)
	data = binary.BigEndian.AppendUint32(data, 0)
	data = binary.BigEndian.AppendUint32(data, cpi)
	data = append(data, make([]byte, cpi-len(data))...)
	stream := func(pid uint16, kind, coarse, fine int, start uint32) []byte {
		entry := binary.BigEndian.AppendUint16(nil, pid)
		fields := uint64(kind)<<34 | uint64(coarse)<<18 | uint64(fine)
		entry = binary.BigEndian.AppendUint16(entry, uint16(fields>>32))
		entry = binary.BigEndian.AppendUint32(entry, uint32(fields))
		return binary.BigEndian.AppendUint32(entry, start)
	}
	// The EP map's head, two streams' entries, the other stream's (none),
	// then the video's.
	other := uint32(2 + 2*12)
	video := other + 4
	epMap := []byte{0, 2}
	epMap = append(epMap, stream(0x1100, 3, 0, 0, other)...)
	epMap = append(epMap, stream(0x1011, 1, entries, len(times), video)...)
	epMap = binary.BigEndian.AppendUint32(epMap, 4)
	epMap = binary.BigEndian.AppendUint32(epMap, uint32(4+len(coarse)))
	epMap = append(append(epMap, coarse...), fine...)
	data = binary.BigEndian.AppendUint32(data, uint32(2+len(epMap)))
	data = binary.BigEndian.AppendUint16(data, 1)
	return append(data, epMap...)
}

// A clip's EP map tells the times of its video's entry points, to 512
// units of 90 kHz: those of the test disc's clips, an I-frame a second
// from 600 seconds, and those written, across coarse entries.
func TestReadEPMap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	unpackBluRay(t, dir)
	for clip, want := range map[string]struct {
		count       int
		first, last time.Duration
	}{
		"00000": {96, 600 * time.Second, 695 * time.Second},
		"00001": {94, 696 * time.Second, 789 * time.Second},
		"00005": {30, 600 * time.Second, 629 * time.Second},
	} {
		data, err := os.ReadFile(filepath.Join(dir, "BDMV", "CLIPINF", clip+".clpi"))
		if err != nil {
			t.Fatal(err)
		}
		times, err := readEPMap(data)
		if err != nil || len(times) != want.count {
			t.Fatalf("%s: %d entry points, %v", clip, len(times), err)
		}
		at := func(i int) time.Duration { return time.Duration(times[i]) * time.Second / 90000 }
		if first, last := at(0), at(len(times)-1); first > want.first || want.first-first > 6*time.Millisecond ||
			last > want.last || want.last-last > 6*time.Millisecond {
			t.Errorf("%s: from %v to %v", clip, first, last)
		}
	}

	var times []uint64
	for second := uint64(600); second < 640; second += 3 {
		times = append(times, second*90000)
	}
	got, err := readEPMap(writeCLPI(times...))
	if err != nil || len(got) != len(times) {
		t.Fatalf("written: %v %v", got, err)
	}
	for i, at := range got {
		if at > times[i] || times[i]-at >= epPrecision {
			t.Errorf("entry point %d at %d, written at %d", i, at, times[i])
		}
	}
	written := writeCLPI(times...)
	for name, bad := range map[string][]byte{
		"empty":      nil,
		"a playlist": append([]byte("MPLS0200"), make([]byte, 40)...),
		"no CPI":     written[:44],
		"cut short":  written[:len(written)-8],
	} {
		if _, err := readEPMap(bad); !errors.Is(err, errNotClip) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// A corrupt clip information file whose offsets point past its end, by
// up to 2^32 bytes, is no clip: on 32-bit targets such an offset wraps
// negative as an int, and was read before the file's start.
func TestReadEPMapRefusesOffsetsPastTheFile(t *testing.T) {
	t.Parallel()
	// Where writeCLPI puts the offsets of the CPI, of the video's entries
	// from the EP map, and of its fine entries from those.
	const cpi, videoStart, fineStart = 16, 40 + 6 + 2 + 12 + 8, 40 + 6 + 2 + 2*12 + 4
	for _, at := range []int{cpi, videoStart, fineStart} {
		for _, offset := range []uint32{0x7FFFFFFC, 0x80000000, 0xFFFFFFF0, 0xFFFFFFFF} {
			data := writeCLPI(600*90000, 603*90000)
			binary.BigEndian.PutUint32(data[at:], offset)
			if _, err := readEPMap(data); !errors.Is(err, errNotClip) {
				t.Errorf("offset %#x at %d: %v", offset, at, err)
			}
		}
	}
}

// A title's keyframes are its play items' entry points, each item's from
// its in time to its out time, after the items before it: an entry point
// just before an item's in time starts it, one at its out time is the
// next item's.
func TestTitleKeyframesFollowThePlayItems(t *testing.T) {
	t.Parallel()
	ticks := func(seconds float64) uint64 { return uint64(seconds * 90000) }
	p := playlist{clips: []clipPart{
		{clip: "00001", in: 600 * 45000, out: 700 * 45000},
		{clip: "00002", in: 1000 * 45000, out: 1100 * 45000},
	}}
	points := map[string][]uint64{
		"00001": {ticks(599.996), ticks(610), ticks(650), ticks(700)},
		"00002": {ticks(990), ticks(1000), ticks(1050.5)},
	}
	want := []time.Duration{0, 10 * time.Second, 50 * time.Second, 100 * time.Second, 150500 * time.Millisecond}
	if got, err := titleKeyframes(p, points); err != nil || !slices.Equal(got, want) {
		t.Errorf("keyframes: %v %v", got, err)
	}

	// The test disc's main title: a keyframe a second, through both its
	// clips.
	dir := t.TempDir()
	unpackBluRay(t, dir)
	real, _ := filepath.EvalSymlinks(dir)
	got, err := blurayKeyframes(folderReader(real), 0)
	if err != nil || len(got) != 190 {
		t.Fatalf("the main title: %d keyframes, %v", len(got), err)
	}
	for i, at := range got {
		if want := time.Duration(i) * time.Second; at > want || want-at > 6*time.Millisecond {
			t.Errorf("keyframe %d at %v", i, at)
		}
	}
}

// The keyframes of a Blu-ray disc's title are read from its folder, any
// case, or from its image; a DVD's are not, nor those of a disc whose
// clips' information is missing.
//
//nolint:paralleltest // swaps the package's minMovie, which every scan of a disc reads
func TestDiscKeyframes(t *testing.T) {
	// The disc's 30-second playlist is a title of its own.
	defer func(old time.Duration) { minMovie = old }(minMovie)
	minMovie = 20 * time.Second
	e := newEnv(t)
	dir := t.TempDir()
	unpackBluRay(t, filepath.Join(dir, "The General (1926)"))
	if err := os.Rename(filepath.Join(dir, "The General (1926)", "BDMV", "CLIPINF"), filepath.Join(dir, "The General (1926)", "BDMV", "clipinf")); err != nil {
		t.Fatal(err)
	}
	unpackBluRay(t, filepath.Join(dir, "Box", "DISC02"))
	if err := os.Remove(filepath.Join(dir, "Box", "DISC02", "BDMV", "CLIPINF", "00005.clpi")); err != nil {
		t.Fatal(err)
	}
	unpackImage(t, "bluray", filepath.Join(dir, "Metropolis (1927).iso"))
	copyDVD(t, filepath.Join(dir, "Nosferatu (1922)"), false)
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Shelf", Path: dir, Kind: KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	keyframes := func(rel string) ([]time.Duration, error) {
		t.Helper()
		var key string
		if err := e.pool.QueryRow(t.Context(), "SELECT id::text FROM local_files WHERE addon_id = $1 AND path = $2", addon.ID, rel).Scan(&key); err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		return e.service.Keyframes(t.Context(), key)
	}
	for _, rel := range []string{"The General (1926)/BDMV/PLAYLIST/00000.mpls", "Metropolis (1927).iso"} {
		if got, err := keyframes(rel); err != nil || len(got) != 190 || got[96] > 96*time.Second || got[96] < 96*time.Second-6*time.Millisecond {
			t.Errorf("%s: %d keyframes, %v", rel, len(got), err)
		}
	}
	if got, err := keyframes("Box/DISC02/BDMV/PLAYLIST/00000.mpls"); err != nil || len(got) != 190 {
		t.Errorf("a title with its clips' information: %d keyframes, %v", len(got), err)
	}
	if _, err := keyframes("Box/DISC02/BDMV/PLAYLIST/00001.mpls"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a title without its clip's information: %v", err)
	}
	if _, err := keyframes("Nosferatu (1922)/VIDEO_TS/VTS_02_1.VOB"); !errors.Is(err, errNoKeyframes) {
		t.Errorf("a DVD: %v", err)
	}
	if _, err := e.service.Keyframes(t.Context(), "not a key"); !errors.Is(err, ErrGone) {
		t.Errorf("no key: %v", err)
	}
}

// A title whose play items each play a clip's entry points again lists
// more keyframes than a title has: none is read, and it is cut as a title
// without keyframes is. One within the bound has them all.
func TestTitlesOfTooManyKeyframesHaveNoneRead(t *testing.T) {
	t.Parallel()
	const points = 4000
	times := make([]uint64, points)
	for i := range times {
		times[i] = uint64(600+i) * 90000
	}
	clpi := writeCLPI(times...)
	read := func(items int) ([]time.Duration, error) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		writeMPLS(t, filepath.Join(dir, "BDMV", "PLAYLIST", "00001.mpls"), slices.Repeat([]mplsItem{{"00001", points}}, items)...)
		writeFiles(t, dir, map[string][]byte{"BDMV/CLIPINF/00001.clpi": clpi})
		return blurayKeyframes(folderReader(dir), 1)
	}
	within := maxTitleKeyframes / points
	if got, err := read(within); err != nil || len(got) != within*points {
		t.Errorf("%d play items of %d entry points: %d keyframes, %v", within, points, len(got), err)
	}
	if got, err := read(1500); !errors.Is(err, errNoKeyframes) || got != nil {
		t.Errorf("1500 play items of %d entry points: %d keyframes, %v", points, len(got), err)
	}
}

// The entry points read of a title's clips are bounded together as its
// keyframes are, whatever part of each clip the title plays.
func TestTitlesOfClipsOfTooManyEntryPointsHaveNoKeyframesRead(t *testing.T) {
	t.Parallel()
	// An entry point every quarter second, for 50,000 seconds.
	const points = 200_000
	times := make([]uint64, points)
	for i := range times {
		times[i] = 600*90000 + uint64(i)*22500
	}
	clpi := writeCLPI(times...)
	read := func(clips ...string) ([]time.Duration, error) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		var items []mplsItem
		for _, clip := range clips {
			items = append(items, mplsItem{clip, 1})
			writeFiles(t, dir, map[string][]byte{"BDMV/CLIPINF/" + clip + ".clpi": clpi})
		}
		writeMPLS(t, filepath.Join(dir, "BDMV", "PLAYLIST", "00001.mpls"), items...)
		return blurayKeyframes(folderReader(dir), 1)
	}
	if 2*points > maxTitleKeyframes || 3*points <= maxTitleKeyframes {
		t.Fatalf("maxTitleKeyframes is %d: the test's clips list %d entry points each", maxTitleKeyframes, points)
	}
	// A second of each clip: its four entry points, and the one at its
	// end, whose time an EP map tells to epPrecision, just before it.
	if got, err := read("00001", "00002"); err != nil || len(got) != 10 {
		t.Errorf("two clips: %d keyframes, %v", len(got), err)
	}
	if got, err := read("00001", "00002", "00003"); !errors.Is(err, errNoKeyframes) || got != nil {
		t.Errorf("three clips: %d keyframes, %v", len(got), err)
	}
}

// listedRemote is a share holding files, by their path, which counts the
// folders it lists and the files it opens; sizes tells the size of a file
// larger than its bytes here.
type listedRemote struct {
	files        map[string][]byte
	sizes        map[string]int64
	lists, opens int
}

func (r *listedRemote) list(_ context.Context, rel string) ([]remoteEntry, error) {
	r.lists++
	var entries []remoteEntry
	seen := map[string]bool{}
	for p, data := range r.files {
		rest, ok := strings.CutPrefix(p, rel+"/")
		if !ok {
			continue
		}
		name, _, folder := strings.Cut(rest, "/")
		if seen[name] {
			continue
		}
		seen[name] = true
		entry := remoteEntry{name: name, folder: folder}
		if !folder {
			entry.size = max(int64(len(data)), r.sizes[p])
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (r *listedRemote) open(_ context.Context, rel string) (io.ReadSeekCloser, time.Time, error) {
	data, ok := r.files[rel]
	if !ok {
		return nil, time.Time{}, os.ErrNotExist
	}
	r.opens++
	return struct {
		io.ReadSeeker
		io.Closer
	}{bytes.NewReader(data), io.NopCloser(nil)}, time.Time{}, nil
}

func (r *listedRemote) close() {}

// clipStream is the size of the streams of clipsDisc's clips.
const clipStream = 192

// clipsDisc is the files of a Blu-ray disc whose playlist 00000 plays a
// second of each of clips clips, each of two entry points in that second
// and of a stream of clipStream bytes, by their path in the disc's
// folder: the playlist, the clip information of one clip, and the paths
// of its playlist, of its clips' information and of their streams.
func clipsDisc(t *testing.T, clips int) (playlist, clip []byte, files map[string][]byte) {
	t.Helper()
	items := make([]mplsItem, clips)
	for i := range items {
		items[i] = mplsItem{fmt.Sprintf("%05d", i), 1}
	}
	playlist, clip = mplsOf(t, items...), writeCLPI(600*90000, 600*90000+45000)
	files = map[string][]byte{"BDMV/PLAYLIST/00000.mpls": playlist}
	stream := make([]byte, clipStream)
	for i := range clips {
		files[fmt.Sprintf("BDMV/CLIPINF/%05d.clpi", i)] = clip
		files[fmt.Sprintf("BDMV/STREAM/%05d.m2ts", i)] = stream
	}
	return playlist, clip, files
}

// The clip information of a title's clips is opened from its folder,
// listed once, in an image's ISO 9660 or UDF file system and in a disc's
// folder on a share: a title of twice the clips has about twice the bytes
// of an image read, each clip costing its entry in the folder, its own
// bytes and, in a UDF file system, its file entry; on a share, the same
// folders are listed, and each clip's file opened once.
func TestClipInformationIsOpenedFromItsFolder(t *testing.T) {
	t.Parallel()
	const few = 1000
	keyframes := func(name string, clips int, fsys discFS) {
		got, err := blurayKeyframes(discReader(fsys, maxClipInfoSize), 0)
		if err != nil || len(got) != 2*clips {
			t.Fatalf("%s: %d clips: %d keyframes, %v", name, clips, len(got), err)
		}
	}
	for name, image := range map[string]func(clips int) []byte{
		"ISO 9660": func(clips int) []byte {
			playlist, clip, _ := clipsDisc(t, clips)
			return blurayISO(1, playlist, clips, clip)
		},
		"UDF 2.50": func(clips int) []byte {
			_, _, files := clipsDisc(t, clips)
			return buildUDF(files, udfLayout{metadata: true})
		},
		"UDF 2.01": func(clips int) []byte {
			_, _, files := clipsDisc(t, clips)
			return buildUDF(files, udfLayout{})
		},
	} {
		read := func(clips int) int64 {
			counted := &countedImage{r: readerOf(image(clips))}
			fsys, err := openDiscFS(counted)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			counted.bytes = 0
			keyframes(name, clips, fsys)
			return counted.bytes
		}
		small, large := read(few), read(2*few)
		t.Logf("%s: %d bytes read for %d clips, %d for %d", name, small, few, large, 2*few)
		// A sector and a half a clip, and the folders above them.
		if bound := func(clips int64) int64 { return clips*3*isoSector/2 + 32*isoSector }; small > bound(few) || large > bound(2*few) {
			t.Errorf("%s: %d bytes read for %d clips, %d for %d", name, small, few, large, 2*few)
		}
		if large > 2*small+small/10 {
			t.Errorf("%s: %d bytes read for %d clips, %d for twice as many", name, small, few, large)
		}
	}

	// On a share: the disc's folder, BDMV and each of PLAYLIST and CLIPINF
	// are listed to find the two folders, then these once.
	for _, clips := range []int{few, 2 * few} {
		_, _, files := clipsDisc(t, clips)
		share := &listedRemote{files: map[string][]byte{}}
		for rel, data := range files {
			share.files["Movie (2008)/"+rel] = data
		}
		keyframes("a share", clips, &shareFS{ctx: t.Context(), r: share, dir: "Movie (2008)"})
		if share.lists > 6 || share.opens != clips+1 {
			t.Errorf("a share: %d clips: %d folders listed, %d files opened", clips, share.lists, share.opens)
		}
	}
}

// A title of more clips than maxTitleClips has no keyframes read, and
// no clip information opened past the bound; one of as many has them.
func TestTitlesOfTooManyClipsHaveNoKeyframesRead(t *testing.T) {
	t.Parallel()
	read := func(clips int) ([]time.Duration, int, error) {
		_, _, files := clipsDisc(t, clips)
		share := &listedRemote{files: map[string][]byte{}}
		for rel, data := range files {
			share.files["Movie (2008)/"+rel] = data
		}
		got, err := blurayKeyframes(discReader(&shareFS{ctx: t.Context(), r: share, dir: "Movie (2008)"}, maxClipInfoSize), 0)
		return got, share.opens, err
	}
	if got, opened, err := read(maxTitleClips); err != nil || len(got) != 2*maxTitleClips || opened != maxTitleClips+1 {
		t.Errorf("%d clips: %d keyframes, %d files opened, %v", maxTitleClips, len(got), opened, err)
	}
	if got, opened, err := read(maxTitleClips + 1); !errors.Is(err, errNoKeyframes) || got != nil || opened != maxTitleClips+1 {
		t.Errorf("%d clips: %d keyframes, %d files opened, %v", maxTitleClips+1, len(got), opened, err)
	}
}
