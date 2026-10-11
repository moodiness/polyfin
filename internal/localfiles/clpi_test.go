package localfiles

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"slices"
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

// A title's keyframes are its play items' entry points, each item's from
// its in time to its out time, after the items before it: an entry point
// just before an item's in time starts it, one at its out time is the
// next item's.
func TestTitleKeyframesFollowThePlayItems(t *testing.T) {
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
	if got := titleKeyframes(p, points); !slices.Equal(got, want) {
		t.Errorf("keyframes: %v", got)
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
