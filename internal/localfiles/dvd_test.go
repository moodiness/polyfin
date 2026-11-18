package localfiles

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
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

// movieTitles chooses the movies of the DVD whose VIDEO_TS folder is dir
// (see dvdTitles), each by its number and set alone.
func movieTitles(dir string) ([]dvdChoice, error) {
	titles, err := dvdTitles(dir, chooseTitles)
	for i := range titles {
		titles[i] = dvdChoice{title: titles[i].title, set: titles[i].set}
	}
	return titles, err
}

// A DVD's main title is its longest, whatever its number or its set's
// size; the IFO files tell how long each title plays. Each other title as
// long as a movie is one too.
func TestDVDMainTitle(t *testing.T) {
	t.Parallel()
	titles, err := movieTitles(filepath.Join("testdata", "dvd", "VIDEO_TS"))
	if err != nil || !slices.Equal(titles, []dvdChoice{{title: 2, set: 2}}) {
		t.Errorf("the test DVD: %v %v", titles, err)
	}
	dir := t.TempDir()
	writeIFOs(t, dir, ifoTitle{set: 1, seconds: 90}, ifoTitle{set: 2, seconds: 600}, ifoTitle{set: 2, seconds: 5400},
		ifoTitle{set: 3, seconds: 5400}, ifoTitle{set: 1, seconds: 20})
	if titles, err := movieTitles(dir); err != nil || !slices.Equal(titles, []dvdChoice{{title: 3, set: 2}, {title: 4, set: 3}}) {
		t.Errorf("titles: %v %v", titles, err)
	}
	// Any case, as libdvdread reads them.
	if err := os.Rename(filepath.Join(dir, "VTS_02_0.IFO"), filepath.Join(dir, "vts_02_0.ifo")); err != nil {
		t.Fatal(err)
	}
	if titles, err := movieTitles(dir); err != nil || len(titles) == 0 || titles[0].title != 3 {
		t.Errorf("lower case: %v, %v", titles, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "VIDEO_TS.IFO"), []byte("not a DVD"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := movieTitles(dir); err == nil {
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
	t.Parallel()
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
	titles, err := movieTitles(dir)
	if err != nil || !slices.Equal(titles, []dvdChoice{{title: 3, set: 1}, {title: 4, set: 2}}) {
		t.Errorf("titles: %v %v", titles, err)
	}
	var many []ifoTitle
	for set := range 8 {
		many = append(many, ifoTitle{set: set + 1, seconds: 3600 + set, cells: cells(uint32(set+1), 3600+set)})
	}
	dir = t.TempDir()
	writeIFOs(t, dir, many...)
	if titles, err := movieTitles(dir); err != nil || !slices.Equal(titles, []dvdChoice{{title: 8, set: 8}}) {
		t.Errorf("decoys: %v %v", titles, err)
	}
}

// A DVD copied as it is, a folder holding VIDEO_TS, is one movie: the
// first VOB of its main title's set, sized as that set's VOBs, matched by
// the disc folder's name, the folder above's when it only numbers the
// disc. Its other files are left out, its menus' VOBs too; a DVD without
// VIDEO_TS.IFO is left out whole, and one whose VIDEO_TS.IFO cannot be
// read is its largest set. A VOB out of a VIDEO_TS folder is a video of
// its own.
func TestDVDsAreOneMovie(t *testing.T) {
	t.Parallel()
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
		"Loose (2002)/VTS_01_1.VOB":                 "unmatched: not_found",
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
	if err != nil || total != 3 || !maps.Equal(discs, map[string]string{"Other (1999)/Disc 1": DiscDVD, "Broken (2000)": DiscDVD, "Loose (2002)/VTS_01_1.VOB": ""}) {
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
	t.Parallel()
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
	t.Parallel()
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

// chainCells is how many cells each program chain writeChainIFOs writes
// has: as many as a chain holds.
const chainCells = 255

// writeChainIFOs writes in the VIDEO_TS folder dir the IFO files of a DVD
// of titles titles, each the first title of title set 1, of chapters
// chapters, each chapter in the program chain of its number, of
// chainCells cells of a minute: the chains apart from one another, or all
// at one place.
func writeChainIFOs(t *testing.T, dir string, titles, chapters int, apart bool) {
	t.Helper()
	vmg := make([]byte, 2*ifoSector)
	copy(vmg, "DVDVIDEO-VMG")
	binary.BigEndian.PutUint32(vmg[0xC4:], 1)
	binary.BigEndian.PutUint16(vmg[ifoSector:], uint16(titles))
	for i := range titles {
		entry := vmg[ifoSector+8+12*i:]
		binary.BigEndian.PutUint16(entry[2:], uint16(chapters))
		entry[6], entry[7] = 1, 1
	}
	// The chapter table at sector 1, then the chain table, then the chains.
	chainSector := 1 + (12+4*chapters+ifoSector-1)/ifoSector
	first, size, places := 8+8*chapters, 0xEC+24*chainCells, 1
	if apart {
		places = chapters
	}
	vts := make([]byte, chainSector*ifoSector+first+places*size)
	copy(vts, "DVDVIDEO-VTS")
	binary.BigEndian.PutUint32(vts[0xC8:], 1)
	binary.BigEndian.PutUint32(vts[0xCC:], uint32(chainSector))
	table, chains := vts[ifoSector:], vts[chainSector*ifoSector:]
	binary.BigEndian.PutUint16(table, 1)
	binary.BigEndian.PutUint32(table[8:], 12)
	binary.BigEndian.PutUint16(chains, uint16(chapters))
	for i := range chapters {
		binary.BigEndian.PutUint16(table[12+4*i:], uint16(i+1))
		binary.BigEndian.PutUint16(table[12+4*i+2:], 1)
		binary.BigEndian.PutUint32(chains[8+8*i+4:], uint32(first+i%places*size))
	}
	for place := range places {
		chain := chains[first+place*size:]
		chain[3] = chainCells
		copy(chain[4:], []byte{0, 1, 0, 0xC0})
		binary.BigEndian.PutUint16(chain[0xE8:], 0xEC)
		for j := range chainCells {
			cell := chain[0xEC+24*j:]
			copy(cell[4:], []byte{0, 1, 0, 0xC0})
			binary.BigEndian.PutUint32(cell[8:], uint32(place*10000+j*10))
			binary.BigEndian.PutUint32(cell[20:], uint32(place*10000+j*10+9))
		}
	}
	writeFiles(t, dir, map[string][]byte{"VIDEO_TS.IFO": vmg, "VTS_01_0.IFO": vts})
}

// ifoReads opens the IFO files of the VIDEO_TS folder dir for dvdTitlesOf,
// and counts what is read of them: the files opened, in order, the cells
// read, by whole tables of chainCells, and the reads of a file made after
// another was opened.
type ifoReads struct {
	dir          string
	opened       []string
	cells, stale int
}

func (c *ifoReads) open(name string) (io.ReaderAt, error) {
	data, err := os.ReadFile(filepath.Join(c.dir, name))
	c.opened = append(c.opened, name)
	return ifoReader{name: name, r: readerOf(data), counts: c}, err
}

// ifoReader is an IFO file ifoReads opened.
type ifoReader struct {
	name   string
	r      io.ReaderAt
	counts *ifoReads
}

func (r ifoReader) ReadAt(p []byte, off int64) (int, error) {
	if len(p) == 24*chainCells {
		r.counts.cells += chainCells
	}
	if r.name != r.counts.opened[len(r.counts.opened)-1] {
		r.counts.stale++
	}
	return r.r.ReadAt(p, off)
}

// A title of more chapters than a DVD's title has is not a DVD's: its
// IFO files are refused before a title set is opened, and no cell is
// read.
func TestDVDTitlesOfMoreChaptersThanADVDHasAreRefused(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeChainIFOs(t, dir, 2, 1000, false)
	reads := &ifoReads{dir: dir}
	titles, err := dvdTitlesOf(reads.open, chooseTitles)
	if !errors.Is(err, errNotDVD) || titles != nil {
		t.Fatalf("titles of 1000 chapters: %v %v", titles, err)
	}
	if !slices.Equal(reads.opened, []string{"VIDEO_TS.IFO"}) || reads.cells != 0 {
		t.Errorf("read before the refusal: %v, %d cells", reads.opened, reads.cells)
	}
	if _, err := dvdTitles(dir, chooseEpisodes); !errors.Is(err, errNotDVD) {
		t.Errorf("from the folder: %v", err)
	}

	// As many chapters as a title has are read.
	dir = t.TempDir()
	writeChainIFOs(t, dir, 1, maxChapters, true)
	if titles, err := movieTitles(dir); err != nil || !slices.Equal(titles, []dvdChoice{{title: 1, set: 1}}) {
		t.Errorf("a title of %d chapters: %v %v", maxChapters, titles, err)
	}
}

// The program chains of a title that are at one place are one chain,
// whatever their numbers: its cells are read once, and it plays once.
func TestDVDChainsAtOnePlaceAreReadOnce(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeChainIFOs(t, dir, 1, maxChapters, false)
	reads := &ifoReads{dir: dir}
	var read []titleCandidate
	titles, err := dvdTitlesOf(reads.open, func(candidates []titleCandidate) []int {
		read = candidates
		return []int{0}
	})
	if err != nil || len(titles) != 1 || len(read) != 1 {
		t.Fatalf("the title: %v %v", titles, err)
	}
	if len(read[0].parts) != chainCells || read[0].length != time.Minute || reads.cells != chainCells {
		t.Errorf("%d chains at one place: %d parts, %v, %d cells read", maxChapters, len(read[0].parts), read[0].length, reads.cells)
	}
}

// A DVD whose titles play more parts together than maxDVDParts is not
// read as one, and its reading stops at the title that passes the bound:
// a scan keeps its largest set as one movie in a movies folder, and
// leaves it out of a shows folder. One within the bound is read whole.
func TestDVDsOfMorePartsThanADVDHoldsAreNotRead(t *testing.T) {
	t.Parallel()
	const titleParts = maxChapters * chainCells
	dir := t.TempDir()
	writeChainIFOs(t, dir, 2, maxChapters, true)
	reads := &ifoReads{dir: dir}
	parts := 0
	count := func(candidates []titleCandidate) []int {
		for _, c := range candidates {
			parts += len(c.parts)
		}
		return chooseTitles(candidates)
	}
	if titles, err := dvdTitlesOf(reads.open, count); err != nil || len(titles) != 1 || parts != 2*titleParts || reads.cells != parts {
		t.Fatalf("two titles of %d parts: %v %v, %d parts, %d cells read", titleParts, titles, err, parts, reads.cells)
	}

	// Four such titles pass the bound at the third.
	if 2*titleParts > maxDVDParts || 3*titleParts <= maxDVDParts {
		t.Fatalf("maxDVDParts is %d: the test's titles have %d parts each", maxDVDParts, titleParts)
	}
	dir = t.TempDir()
	writeChainIFOs(t, dir, 4, maxChapters, true)
	reads, parts = &ifoReads{dir: dir}, 0
	titles, err := dvdTitlesOf(reads.open, count)
	if !errors.Is(err, errNotDVD) || titles != nil || parts != 0 {
		t.Fatalf("four titles of %d parts: %v %v, %d parts chosen among", titleParts, titles, err, parts)
	}
	if reads.cells != 3*titleParts {
		t.Errorf("%d cells read, want those of 3 titles, %d", reads.cells, 3*titleParts)
	}

	// What a scan keeps of it.
	files := map[string]found{
		"Movie/VIDEO_TS/VIDEO_TS.IFO": {size: 10}, "Movie/VIDEO_TS/VTS_01_1.VOB": {size: 100},
		"Movie/VIDEO_TS/VTS_02_1.VOB": {size: 200}, "Movie/VIDEO_TS/VTS_02_2.VOB": {size: 50},
	}
	read := func(string) ([]dvdChoice, error) { return dvdTitles(dir, chooseTitles) }
	if got := dvdEntries(files, read, false); len(got) != 1 || got["Movie/VIDEO_TS/VTS_02_1.VOB"].size != 250 {
		t.Errorf("in a movies folder: %v", got)
	}
	if got := dvdEntries(files, read, true); len(got) != 0 {
		t.Errorf("in a shows folder: %v", got)
	}
}

// A DVD's titles are read set by set, each set's IFO file opened once and
// not read again once the next is opened, so that one is held at a time;
// each title stays the one of its number.
func TestDVDSetsAreReadOneAtATime(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeIFOs(t, dir, ifoTitle{set: 1, seconds: 5400}, ifoTitle{set: 2, seconds: 600}, ifoTitle{set: 1, seconds: 90},
		ifoTitle{set: 2, seconds: 3000})
	reads := &ifoReads{dir: dir}
	var read []titleCandidate
	_, err := dvdTitlesOf(reads.open, func(candidates []titleCandidate) []int {
		read = candidates
		return nil
	})
	if err != nil || len(read) != 4 {
		t.Fatalf("the titles: %v %v", read, err)
	}
	for i, seconds := range []int{5400, 600, 90, 3000} {
		if read[i].number != i+1 || read[i].length.Truncate(time.Second) != time.Duration(seconds)*time.Second {
			t.Errorf("title %d: number %d, %v", i+1, read[i].number, read[i].length)
		}
	}
	if !slices.Equal(reads.opened, []string{"VIDEO_TS.IFO", "VTS_01_0.IFO", "VTS_02_0.IFO"}) || reads.stale != 0 {
		t.Errorf("opened %v, %d reads of a file after the next was opened", reads.opened, reads.stale)
	}
}

// The title of a DVD on a share is laid out from its folder's listing,
// listed once, whatever its cells: its IFO files and its VOB are opened
// once each, and each cell located in the VOB without another listing.
func TestShareDVDCellsAreLocatedFromTheirFoldersListing(t *testing.T) {
	t.Parallel()
	const folder, chains = "Movie (2008)/VIDEO_TS", 20
	dir := t.TempDir()
	writeChainIFOs(t, dir, 1, chains, true)
	share := &listedRemote{files: map[string][]byte{folder + "/VTS_01_1.VOB": nil},
		sizes: map[string]int64{folder + "/VTS_01_1.VOB": chains * 10000 * ifoSector}}
	for _, name := range []string{"VIDEO_TS.IFO", "VTS_01_0.IFO"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		share.files[folder+"/"+name] = data
	}
	files := listedOf(&shareFS{ctx: t.Context(), r: share, dir: folder})
	locate := locator(files, folder, nil)
	open := func(name string) (*io.SectionReader, error) { return files.open(name) }
	pieces, _, err := dvdPieces(open, 1, func(name string, start, end int64) ([]titlePiece, error) {
		return locate([]string{name}, start, end)
	})
	if err != nil || len(pieces) != chains*chainCells || pieces[1] != (titlePiece{folder + "/VTS_01_1.VOB", 10 * ifoSector, 10 * ifoSector}) {
		t.Fatalf("%d pieces, %v", len(pieces), err)
	}
	if share.lists != 1 || share.opens != 3 {
		t.Errorf("%d folders listed, %d files opened", share.lists, share.opens)
	}
}
