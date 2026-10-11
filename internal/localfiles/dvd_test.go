package localfiles

import (
	"bytes"
	"encoding/binary"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/secrets"
)

// copyDVD copies the test DVD, testdata/dvd, into the disc folder dir,
// its files' names in lower case when lower is set.
func copyDVD(t *testing.T, dir string, lower bool) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join("testdata", "dvd", "VIDEO_TS"))
	if err != nil {
		t.Fatal(err)
	}
	folder, name := "VIDEO_TS", func(s string) string { return s }
	if lower {
		folder, name = "video_ts", strings.ToLower
	}
	if err := os.MkdirAll(filepath.Join(dir, folder), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join("testdata", "dvd", "VIDEO_TS", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, folder, name(e.Name())), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// ifoTitle is a title the IFO files writeIFOs writes list: its title set
// and how long it plays, and the cells it plays, when given.
type ifoTitle struct {
	set     int
	seconds int
	cells   []ifoCell
}

// ifoCell is a cell of a title: its sectors in its set, and how long it
// plays.
type ifoCell struct {
	first, last uint32
	seconds     int
}

// writeIFOs writes in the VIDEO_TS folder dir the IFO files of a DVD of
// titles, each its set's next, in one program chain, at 30 frames a
// second, with its cells.
func writeIFOs(t *testing.T, dir string, titles ...ifoTitle) {
	t.Helper()
	vmg := make([]byte, 2*ifoSector)
	copy(vmg, "DVDVIDEO-VMG")
	binary.BigEndian.PutUint32(vmg[0xC4:], 1)
	binary.BigEndian.PutUint16(vmg[ifoSector:], uint16(len(titles)))
	sets := map[int][]ifoTitle{}
	for i, title := range titles {
		sets[title.set] = append(sets[title.set], title)
		entry := vmg[ifoSector+8+12*i:]
		binary.BigEndian.PutUint16(entry[2:], 1)
		entry[6], entry[7] = byte(title.set), byte(len(sets[title.set]))
	}
	write(t, filepath.Join(dir, "VIDEO_TS.IFO"), 0)
	if err := os.WriteFile(filepath.Join(dir, "VIDEO_TS.IFO"), vmg, 0o644); err != nil {
		t.Fatal(err)
	}
	bcd := func(v int) byte { return byte(v/10<<4 | v%10) }
	played := func(seconds int) []byte {
		return []byte{bcd(seconds / 3600), bcd(seconds / 60 % 60), bcd(seconds % 60), 0xC0 | bcd(15)}
	}
	for set, titles := range sets {
		vts := make([]byte, 2*ifoSector+8+520*len(titles))
		copy(vts, "DVDVIDEO-VTS")
		binary.BigEndian.PutUint32(vts[0xC8:], 1)
		binary.BigEndian.PutUint32(vts[0xCC:], 2)
		chapters, chains := vts[ifoSector:], vts[2*ifoSector:]
		binary.BigEndian.PutUint16(chapters, uint16(len(titles)))
		binary.BigEndian.PutUint16(chains, uint16(len(titles)))
		for i, title := range titles {
			// Title i+1's one chapter, in chain i+1, after the offsets.
			at := 8 + 4*len(titles) + 4*i
			binary.BigEndian.PutUint32(chapters[8+4*i:], uint32(at))
			binary.BigEndian.PutUint16(chapters[at:], uint16(i+1))
			binary.BigEndian.PutUint16(chapters[at+2:], 1)
			chain := 8 + 8*len(titles) + 512*i
			chains[8+8*i] = 0x80 | byte(i+1)
			binary.BigEndian.PutUint32(chains[8+8*i+4:], uint32(chain))
			copy(chains[chain+4:], played(title.seconds))
			if len(title.cells) > 0 {
				chains[chain+3] = byte(len(title.cells))
				binary.BigEndian.PutUint16(chains[chain+0xE8:], 0xEC)
				for j, cell := range title.cells {
					entry := chains[chain+0xEC+24*j:]
					copy(entry[4:], played(cell.seconds))
					binary.BigEndian.PutUint32(entry[8:], cell.first)
					binary.BigEndian.PutUint32(entry[20:], cell.last)
				}
			}
		}
		if err := os.WriteFile(filepath.Join(dir, "VTS_"+twoDigits(set)+"_0.IFO"), vts, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func twoDigits(n int) string {
	return string([]byte{byte('0' + n/10), byte('0' + n%10)})
}

// A DVD's main title is its longest, whatever its number or its set's
// size; the IFO files tell how long each title plays. Each other title as
// long as a movie is one too.
func TestDVDMainTitle(t *testing.T) {
	titles, err := dvdTitles(filepath.Join("testdata", "dvd", "VIDEO_TS"))
	if err != nil || !slices.Equal(titles, []dvdChoice{{title: 2, set: 2}}) {
		t.Errorf("the test DVD: %v %v", titles, err)
	}
	dir := t.TempDir()
	writeIFOs(t, dir, ifoTitle{set: 1, seconds: 90}, ifoTitle{set: 2, seconds: 600}, ifoTitle{set: 2, seconds: 5400},
		ifoTitle{set: 3, seconds: 5400}, ifoTitle{set: 1, seconds: 20})
	if titles, err := dvdTitles(dir); err != nil || !slices.Equal(titles, []dvdChoice{{title: 3, set: 2}, {title: 4, set: 3}}) {
		t.Errorf("titles: %v %v", titles, err)
	}
	// Any case, as libdvdread reads them.
	if err := os.Rename(filepath.Join(dir, "VTS_02_0.IFO"), filepath.Join(dir, "vts_02_0.ifo")); err != nil {
		t.Fatal(err)
	}
	if titles, err := dvdTitles(dir); err != nil || len(titles) == 0 || titles[0].title != 3 {
		t.Errorf("lower case: %v, %v", titles, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "VIDEO_TS.IFO"), []byte("not a DVD"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := dvdTitles(dir); err == nil {
		t.Error("an unreadable VIDEO_TS.IFO was read")
	}
	if got := playbackTime([]byte{0x01, 0x32, 0x05, 0x40 | 0x12}); got != time.Hour+32*time.Minute+5*time.Second+12*time.Second/25 {
		t.Errorf("a PAL chain's time: %s", got)
	}
}

// A DVD's movies are its titles at least minMovie long that play mostly
// cells the longer ones do not: a title replaying the main one's cells,
// as another cut of the movie does, counts once, whatever its set, and a
// short extra is left out. A disc of more movies than maxDiscTitles keeps
// its main title only.
func TestDVDTitlesAreItsMovies(t *testing.T) {
	cells := func(set uint32, seconds ...int) []ifoCell {
		var list []ifoCell
		for i, s := range seconds {
			list = append(list, ifoCell{first: set*100000 + uint32(i)*1000, last: set*100000 + uint32(i)*1000 + 999, seconds: s})
		}
		return list
	}
	main := cells(1, 3000, 2400)
	// The other cut plays the main title's cells and a short scene.
	cut := append(slices.Clone(main), ifoCell{first: 90000, last: 90999, seconds: 300})
	dir := t.TempDir()
	writeIFOs(t, dir,
		ifoTitle{set: 1, seconds: 120, cells: cells(9, 120)},
		ifoTitle{set: 1, seconds: 5400, cells: main},
		ifoTitle{set: 1, seconds: 5700, cells: cut},
		ifoTitle{set: 2, seconds: 5000, cells: cells(2, 2500, 2500)},
		ifoTitle{set: 3, seconds: 600, cells: cells(3, 600)})
	titles, err := dvdTitles(dir)
	if err != nil || !slices.Equal(titles, []dvdChoice{{title: 3, set: 1}, {title: 4, set: 2}}) {
		t.Errorf("titles: %v %v", titles, err)
	}
	var many []ifoTitle
	for set := range 8 {
		many = append(many, ifoTitle{set: set + 1, seconds: 3600 + set, cells: cells(uint32(set+1), 3600+set)})
	}
	dir = t.TempDir()
	writeIFOs(t, dir, many...)
	if titles, err := dvdTitles(dir); err != nil || !slices.Equal(titles, []dvdChoice{{title: 8, set: 8}}) {
		t.Errorf("decoys: %v %v", titles, err)
	}
}

// A DVD copied as it is, a folder holding VIDEO_TS, is one movie: the
// first VOB of its main title's set, sized as that set's VOBs, matched by
// the disc folder's name, the folder above's when it only numbers the
// disc. Its other files are left out; a DVD without VIDEO_TS.IFO is left
// out whole, and one whose VIDEO_TS.IFO cannot be read is its largest set.
func TestDVDsAreOneMovie(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	copyDVD(t, filepath.Join(dir, "The General (1926)"), false)
	copyDVD(t, filepath.Join(dir, "Other (1999)", "Disc 1"), true)
	nosferatu := filepath.Join(dir, "Nosferatu (1922)", "VIDEO_TS")
	writeIFOs(t, nosferatu, ifoTitle{set: 1, seconds: 5000}, ifoTitle{set: 2, seconds: 300})
	for rel, size := range map[string]int{
		"Nosferatu (1922)/VIDEO_TS/VTS_01_0.VOB":   1000,
		"Nosferatu (1922)/VIDEO_TS/VTS_01_1.VOB":   400,
		"Nosferatu (1922)/VIDEO_TS/VTS_01_2.VOB":   300,
		"Nosferatu (1922)/VIDEO_TS/VTS_02_1.VOB":   900,
		"Nosferatu (1922)/VIDEO_TS/VIDEO_TS.VOB":   50,
		"Nosferatu (1922)/Nosferatu (1922).mkv":    10,
		"Broken (2000)/VIDEO_TS/VIDEO_TS.IFO":      10,
		"Broken (2000)/VIDEO_TS/VTS_01_1.VOB":      10,
		"Broken (2000)/VIDEO_TS/VTS_02_1.VOB":      30,
		"No Menu (2001)/VIDEO_TS/VTS_01_1.VOB":     30,
		"Loose (2002)/VTS_01_1.VOB":                30,
		"Loose (2002)/VIDEO_TS.IFO":                30,
		"The General (1926)/AUDIO_TS/AUDIO_TS.IFO": 5,
	} {
		write(t, filepath.Join(dir, rel), size)
	}
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Shelf", Path: dir, Kind: KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	want := map[string]string{
		"The General (1926)/VIDEO_TS/VTS_02_1.VOB":  "tt0017925",
		"Other (1999)/Disc 1/video_ts/vts_02_1.vob": "unmatched: not_found",
		"Nosferatu (1922)/VIDEO_TS/VTS_01_1.VOB":    "tt0013442",
		"Nosferatu (1922)/Nosferatu (1922).mkv":     "tt0013442",
		"Broken (2000)/VIDEO_TS/VTS_02_1.VOB":       "unmatched: not_found",
	}
	if got := e.files(addon.ID); !maps.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	reads := e.reads(addon.ID)
	for rel, r := range map[string]read{
		"The General (1926)/VIDEO_TS/VTS_02_1.VOB":  {unit: "The General (1926)", title: "The General", year: 1926},
		"Other (1999)/Disc 1/video_ts/vts_02_1.vob": {unit: "Other (1999)/Disc 1", title: "Other", year: 1999},
		"Nosferatu (1922)/VIDEO_TS/VTS_01_1.VOB":    {unit: "Nosferatu (1922)", title: "Nosferatu", year: 1922},
	} {
		if reads[rel] != r {
			t.Errorf("%s: %+v", rel, reads[rel])
		}
	}
	var size int64
	if err := e.pool.QueryRow(t.Context(), "SELECT size FROM local_files WHERE addon_id = $1 AND path = $2", addon.ID,
		"Nosferatu (1922)/VIDEO_TS/VTS_01_1.VOB").Scan(&size); err != nil || size != 700 {
		t.Errorf("the main set's size: %d %v", size, err)
	}
	unmatched, total, err := e.service.Unmatched(t.Context(), addon.ID)
	discs := map[string]string{}
	for _, u := range unmatched {
		discs[u.Unit] = u.Disc
	}
	if err != nil || total != 2 || !maps.Equal(discs, map[string]string{"Other (1999)/Disc 1": DiscDVD, "Broken (2000)": DiscDVD}) {
		t.Errorf("unmatched: %d %+v %v", total, unmatched, err)
	}

	// The DVD's version is named after its folder, and is read from its
	// input, its main title.
	streams, err := e.service.Streams(t.Context(), addon.ID, "tt0017925")
	if err != nil || len(streams) != 1 || streams[0].BehaviorHints.Filename != "The General (1926).VOB" ||
		streams[0].BehaviorHints.VideoSize != 520192 || !strings.HasPrefix(streams[0].URL, "http://inputs.example/") {
		t.Fatalf("streams: %+v %v", streams, err)
	}
	in, err := e.service.Input(t.Context(), strings.TrimPrefix(streams[0].URL, "http://inputs.example/"))
	real, _ := filepath.EvalSymlinks(filepath.Join(dir, "The General (1926)", "VIDEO_TS"))
	if err != nil || in.Format != "dvdvideo" || !slices.Equal(in.Options, []string{"-title", "2"}) || in.Path != real {
		t.Errorf("input: %+v %v", in, err)
	}
	if !slices.Equal(in.Args(), []string{"-f", "dvdvideo", "-title", "2"}) {
		t.Errorf("args: %v", in.Args())
	}
	// A file's key has no input.
	streams, _ = e.service.Streams(t.Context(), addon.ID, "tt0013442")
	for _, stream := range streams {
		key, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(stream.URL, "http://files.example/"), "http://inputs.example/"), ".")
		_, err := e.service.Input(t.Context(), key)
		if dvd := strings.HasSuffix(stream.BehaviorHints.Filename, ".VOB"); dvd != (err == nil) {
			t.Errorf("the input of %s: %v", stream.BehaviorHints.Filename, err)
		}
	}

	// Linked by hand, a DVD is linked whole, and stays so when another set
	// becomes its main title's.
	if err := e.service.Link(t.Context(), addon.ID, "Broken (2000)/VIDEO_TS/VTS_02_1.VOB", "tt0063350"); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "Broken (2000)/VIDEO_TS/VTS_01_2.VOB"), 40)
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	files := e.files(addon.ID)
	if _, old := files["Broken (2000)/VIDEO_TS/VTS_02_1.VOB"]; old || files["Broken (2000)/VIDEO_TS/VTS_01_1.VOB"] != "tt0063350" {
		t.Errorf("another main set: %v", files)
	}
}

// A DVD at a folder's root is named after the folder; a DVD in a shows
// folder is left out; one on a share is read as a local one is, its IFO
// files read on the share.
func TestDVDsAtTheRootInShowsAndOnShares(t *testing.T) {
	e := newEnv(t)
	dir := filepath.Join(t.TempDir(), "The General (1926)")
	copyDVD(t, dir, false)
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Disc", Path: dir, Kind: KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	if got := e.files(addon.ID); !maps.Equal(got, map[string]string{"VIDEO_TS/VTS_02_1.VOB": "tt0017925"}) {
		t.Errorf("files: %v", got)
	}
	if got := e.reads(addon.ID)["VIDEO_TS/VTS_02_1.VOB"]; got != (read{unit: ".", title: "The General", year: 1926}) {
		t.Errorf("read: %+v", got)
	}
	streams, err := e.service.Streams(t.Context(), addon.ID, "tt0017925")
	if err != nil || len(streams) != 1 || streams[0].BehaviorHints.Filename != "The General (1926).VOB" {
		t.Errorf("streams: %+v %v", streams, err)
	}

	shows := t.TempDir()
	copyDVD(t, filepath.Join(shows, "The Lone Ranger", "Season 01"), false)
	write(t, filepath.Join(shows, "The Lone Ranger", "Season 01", "The Lone Ranger - S01E01.mkv"), 10)
	showsFolder, err := e.service.Add(t.Context(), NewFolder{Name: "Shows", Path: shows, Kind: KindShows})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(showsFolder.ID)
	if got := e.files(showsFolder.ID); !maps.Equal(got, map[string]string{"The Lone Ranger/Season 01/The Lone Ranger - S01E01.mkv": "tt0041038"}) {
		t.Errorf("shows: %v", got)
	}

	box, err := secrets.New(bytes.Repeat([]byte{7}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	e.service.box = box
	shared := t.TempDir()
	copyDVD(t, filepath.Join(shared, "The General (1926)"), false)
	write(t, filepath.Join(shared, "Nosferatu (1922).mkv"), 10)
	server := serveWebDAV(t, shared, "reader", "pass phrase")
	share, err := e.service.Add(t.Context(), NewFolder{Name: "NAS", Path: server.URL + "/", Kind: KindMovies, User: "reader",
		Password: "pass phrase"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(share.ID)
	if got := e.files(share.ID); !maps.Equal(got, map[string]string{"Nosferatu (1922).mkv": "tt0013442",
		"The General (1926)/VIDEO_TS/VTS_02_1.VOB": "tt0017925"}) {
		t.Errorf("a share: %v", got)
	}
}

// Each other movie of a DVD is a title of its own: an entry with its
// title's number on the first VOB of its set, sized as that set's VOBs, a
// unit of its own, unmatched until linked by hand, named after the disc
// and its number, and played from the disc with -title. The main title's
// entry stays the one a DVD of one movie has, and the titles keep their
// links through scans.
func TestDVDMoviesAreTitles(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	disc := filepath.Join(dir, "Nosferatu (1922)", "VIDEO_TS")
	writeIFOs(t, disc, ifoTitle{set: 1, seconds: 5400}, ifoTitle{set: 2, seconds: 5000}, ifoTitle{set: 3, seconds: 300},
		ifoTitle{set: 1, seconds: 4000, cells: []ifoCell{{first: 5000, last: 9000, seconds: 4000}}})
	for rel, size := range map[string]int{
		"VTS_01_1.VOB": 700,
		"VTS_01_2.VOB": 300,
		"VTS_02_1.VOB": 600,
		"VTS_03_1.VOB": 50,
	} {
		write(t, filepath.Join(disc, rel), size)
	}
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Shelf", Path: dir, Kind: KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	want := map[string]string{
		"Nosferatu (1922)/VIDEO_TS/VTS_01_1.VOB":   "tt0013442",
		"Nosferatu (1922)/VIDEO_TS/VTS_02_1.VOB#2": "unmatched: disc_title",
		"Nosferatu (1922)/VIDEO_TS/VTS_01_1.VOB#4": "unmatched: disc_title",
	}
	if got := e.files(addon.ID); !maps.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	reads := e.reads(addon.ID)
	for rel, r := range map[string]read{
		"Nosferatu (1922)/VIDEO_TS/VTS_01_1.VOB":   {unit: "Nosferatu (1922)", title: "Nosferatu", year: 1922},
		"Nosferatu (1922)/VIDEO_TS/VTS_02_1.VOB#2": {unit: "Nosferatu (1922)#00002"},
		"Nosferatu (1922)/VIDEO_TS/VTS_01_1.VOB#4": {unit: "Nosferatu (1922)#00004"},
	} {
		if reads[rel] != r {
			t.Errorf("%s: %+v", rel, reads[rel])
		}
	}
	unmatched, total, err := e.service.Unmatched(t.Context(), addon.ID)
	discs := map[string]string{}
	for _, u := range unmatched {
		discs[u.Unit] = u.Disc
	}
	if err != nil || total != 2 || !maps.Equal(discs, map[string]string{"Nosferatu (1922)#00002": DiscDVD, "Nosferatu (1922)#00004": DiscDVD}) {
		t.Errorf("unmatched: %d %+v %v", total, unmatched, err)
	}
	real, _ := filepath.EvalSymlinks(disc)
	streams, _ := e.service.Streams(t.Context(), addon.ID, "tt0013442")
	if len(streams) != 1 || streams[0].BehaviorHints.Filename != "Nosferatu (1922).VOB" || streams[0].BehaviorHints.VideoSize != 1000 {
		t.Fatalf("the main title: %+v", streams)
	}
	if in, err := e.service.Input(t.Context(), strings.TrimPrefix(streams[0].URL, "http://inputs.example/")); err != nil ||
		!slices.Equal(in.Args(), []string{"-f", "dvdvideo", "-title", "1"}) || in.Path != real {
		t.Errorf("the main title's input: %+v %v", in, err)
	}

	if err := e.service.Link(t.Context(), addon.ID, "Nosferatu (1922)/VIDEO_TS/VTS_02_1.VOB#2", "tt0063350"); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.files(addon.ID)["Nosferatu (1922)/VIDEO_TS/VTS_02_1.VOB#2"]; got != "tt0063350" {
		t.Errorf("linked: %s", got)
	}
	streams, err = e.service.Streams(t.Context(), addon.ID, "tt0063350")
	if err != nil || len(streams) != 1 || streams[0].BehaviorHints.Filename != "Nosferatu (1922) - title 2.VOB" ||
		streams[0].BehaviorHints.VideoSize != 600 || !strings.HasPrefix(streams[0].URL, "http://inputs.example/") {
		t.Fatalf("the other movie: %+v %v", streams, err)
	}
	key := strings.TrimPrefix(streams[0].URL, "http://inputs.example/")
	if in, err := e.service.Input(t.Context(), key); err != nil || !slices.Equal(in.Args(), []string{"-f", "dvdvideo", "-title", "2"}) || in.Path != real {
		t.Errorf("the other movie's input: %+v %v", in, err)
	}
	if _, err := e.service.Keyframes(t.Context(), key); !errors.Is(err, errNoKeyframes) {
		t.Errorf("the other movie's keyframes: %v", err)
	}
}
