package localfiles

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/media"
)

// A DVD copied as it is: a disc folder holding a VIDEO_TS folder, any
// case. Its VIDEO_TS.IFO lists the disc's titles, each in a title set nn,
// which VTS_nn_0.IFO describes and VTS_nn_1.VOB to VTS_nn_9.VOB hold. Its
// movie is its main title, the longest; the others are extras, trailers
// and menus' clips. A scan of a movies folder in the container keeps one
// entry per DVD, the first VOB of its main title's set, sized as the set's
// VOBs and matched by the disc folder's name, as a Blu-ray disc is. FFmpeg
// plays the main title from the VIDEO_TS folder, with its dvdvideo demuxer
// (see Input): the version is not one file. A shows folder leaves DVDs
// out, and so do shares, FFmpeg reading a DVD from a path.

// dvdVOB is the name of a title set's VOB, VTS_nn_1.VOB to VTS_nn_9.VOB,
// any case: VTS_nn_0.VOB holds the set's menus.
var dvdVOB = regexp.MustCompile(`(?i)^VTS_(\d\d)_[1-9]\.VOB$`)

// dvdFile reports whether name, a file in a folder named folder, is one of
// a DVD's that a walk lists: its VIDEO_TS.IFO or a title set's VOB.
func dvdFile(folder, name string) bool {
	return strings.EqualFold(folder, "VIDEO_TS") && (strings.EqualFold(name, "VIDEO_TS.IFO") || dvdVOB.MatchString(name))
}

// dvdOf tells whether rel is one of a DVD's files a walk lists, and the
// disc folder's path, "" for a disc at the folder's root.
func dvdOf(rel string) (disc string, ok bool) {
	dir, name := path.Split(rel)
	dir = strings.TrimSuffix(dir, "/")
	if !dvdFile(path.Base(dir), name) {
		return "", false
	}
	if disc = path.Dir(dir); disc == "." {
		disc = ""
	}
	return disc, true
}

// dvdSet is the title set of a VOB's path, 0 for another file.
func dvdSet(rel string) int {
	m := dvdVOB.FindStringSubmatch(path.Base(rel))
	if m == nil {
		return 0
	}
	set, _ := strconv.Atoi(m[1])
	return set
}

// collapseDVDs keeps, of the files a walk of the folder at root found, one
// per DVD of a movies folder (see dvdEntry), and none of a shows folder's:
// a DVD of episodes is no one episode.
func collapseDVDs(kind, root string, files map[string]found) {
	discs := map[string]map[string]found{}
	for rel, f := range files {
		disc, ok := dvdOf(rel)
		if !ok {
			continue
		}
		delete(files, rel)
		if kind == KindShows {
			continue
		}
		if discs[disc] == nil {
			discs[disc] = map[string]found{}
		}
		discs[disc][rel] = f
	}
	for _, parts := range discs {
		if rel, f, ok := dvdEntry(root, parts); ok {
			files[rel] = f
		}
	}
}

// dvdEntry is the entry a scan keeps for a DVD, of its files a walk of
// the folder at root found: the first VOB of its main title's set, sized
// as the set's VOBs, modified when the last of them or VIDEO_TS.IFO was.
// The largest set stands for the main title's when VIDEO_TS.IFO cannot be
// read; a DVD without one, or without a VOB of that set, is left out.
func dvdEntry(root string, parts map[string]found) (rel string, f found, ok bool) {
	var ifo string
	sizes := map[int]int64{}
	for name, part := range parts {
		if set := dvdSet(name); set > 0 {
			sizes[set] += part.size
		} else {
			ifo = name
		}
	}
	if ifo == "" || len(sizes) == 0 {
		return "", found{}, false
	}
	set := 0
	if _, main, err := dvdMain(filepath.Join(root, filepath.FromSlash(path.Dir(ifo)))); err == nil {
		set = main
	} else {
		for s, size := range sizes {
			if set == 0 || size > sizes[set] || size == sizes[set] && s < set {
				set = s
			}
		}
	}
	f.modified = parts[ifo].modified
	for name, part := range parts {
		if dvdSet(name) != set {
			continue
		}
		if rel == "" || name < rel {
			rel = name
		}
		f.size += part.size
		if part.modified.After(f.modified) {
			f.modified = part.modified
		}
	}
	return rel, f, rel != ""
}

// Input finds what FFmpeg reads the version key names from, its
// identifier as Streams gives it, through no symbolic link leading out of
// its folder: a Blu-ray disc's title, its playlist played from the disc's
// folder with the bluray protocol, a DVD's main title, read from its
// VIDEO_TS folder with the dvdvideo demuxer, or a disc image's main title,
// read from the image the same ways. Only the discs of a movies folder in
// the container have one.
func (s *Service) Input(ctx context.Context, key string) (media.Input, error) {
	id, err := accounts.ParseID(key)
	if err != nil {
		return media.Input{}, ErrGone
	}
	var root, rel, kind, image string
	var title int
	err = s.db.QueryRow(ctx, `SELECT a.manifest_url, f.path, l.kind, coalesce(i.kind, ''), coalesce(i.title, 0)
		FROM local_files f JOIN addons a ON a.id = f.addon_id JOIN local_folders l ON l.addon_id = f.addon_id
		LEFT JOIN local_disc_images i ON i.addon_id = f.addon_id AND i.path = f.path WHERE f.id = $1`, id).
		Scan(&root, &rel, &kind, &image, &title)
	if errors.Is(err, pgx.ErrNoRows) {
		return media.Input{}, ErrGone
	}
	if err != nil {
		return media.Input{}, err
	}
	if kind != KindMovies || shareKind(root) != "" {
		return media.Input{}, ErrGone
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return media.Input{}, ErrGone
	}
	folder := func(rel string) (string, error) {
		dir, err := filepath.EvalSymlinks(filepath.Join(real, filepath.FromSlash(rel)))
		if err != nil || !within(real, dir) {
			return "", ErrGone
		}
		return dir, nil
	}
	switch image {
	case imageDVD:
		file, err := folder(rel)
		if err != nil {
			return media.Input{}, err
		}
		in := media.Input{Format: "dvdvideo", Path: file}
		if title > 0 {
			in.Options = []string{"-title", strconv.Itoa(title)}
		}
		return in, nil
	case imageBluRay:
		file, err := folder(rel)
		if err != nil {
			return media.Input{}, err
		}
		return media.Input{Options: []string{"-playlist", strconv.Itoa(title)}, Path: "bluray:" + file}, nil
	}
	if disc, number, ok := discPlaylist(rel); ok {
		dir, err := folder(disc)
		if err != nil {
			return media.Input{}, err
		}
		return media.Input{Options: []string{"-playlist", strconv.Itoa(number)}, Path: "bluray:" + dir}, nil
	}
	if _, ok := dvdOf(rel); !ok || dvdSet(rel) == 0 {
		return media.Input{}, ErrGone
	}
	dir, err := folder(path.Dir(rel))
	if err != nil {
		return media.Input{}, err
	}
	title, _, err = dvdMain(dir)
	if err != nil {
		return media.Input{}, err
	}
	return media.Input{Format: "dvdvideo", Options: []string{"-title", strconv.Itoa(title)}, Path: dir}, nil
}

// errNotDVD reports IFO files that do not describe a DVD's titles.
var errNotDVD = errors.New("not a DVD's IFO file")

// ifoSector is the size of a DVD's sectors, the unit IFO files' tables
// are addressed in.
const ifoSector = 2048

// maxTitles bounds the titles a DVD has: its title search table holds at
// most 99.
const maxTitles = 99

// dvdMain finds the main title of the DVD whose VIDEO_TS folder is dir:
// its longest title, the first of those as long, by its number on the
// disc, and the title set it is in.
func dvdMain(dir string) (title, set int, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, err
	}
	names := map[string]string{}
	for _, e := range entries {
		names[strings.ToUpper(e.Name())] = e.Name()
	}
	var opened []*os.File
	defer func() {
		for _, f := range opened {
			f.Close()
		}
	}()
	return dvdMainOf(func(name string) (io.ReaderAt, error) {
		real, ok := names[name]
		if !ok {
			return nil, fmt.Errorf("%w: no %s", errNotDVD, name)
		}
		f, err := os.Open(filepath.Join(dir, real))
		if err == nil {
			opened = append(opened, f)
		}
		return f, err
	})
}

// dvdMainOf is dvdMain, the DVD's IFO files opened by their name in upper
// case.
func dvdMainOf(open func(name string) (io.ReaderAt, error)) (title, set int, err error) {
	vmg, err := open("VIDEO_TS.IFO")
	if err != nil {
		return 0, 0, err
	}
	titles, err := readTitles(vmg)
	if err != nil {
		return 0, 0, err
	}
	sets := map[int]io.ReaderAt{}
	var longest time.Duration
	for i, t := range titles {
		vts, ok := sets[t.set]
		if !ok {
			if vts, err = open(fmt.Sprintf("VTS_%02d_0.IFO", t.set)); err != nil {
				return 0, 0, err
			}
			sets[t.set] = vts
		}
		length, err := titleLength(vts, t)
		if err != nil {
			return 0, 0, err
		}
		if title == 0 || length > longest {
			title, set, longest = i+1, t.set, length
		}
	}
	return title, set, nil
}

// dvdTitle is a title VIDEO_TS.IFO lists: its title set, its number in
// the set, and its chapters.
type dvdTitle struct {
	set, number, chapters int
}

// readTitles reads the titles of VIDEO_TS.IFO, in their order on the disc.
func readTitles(vmg io.ReaderAt) ([]dvdTitle, error) {
	head := make([]byte, 0xC8)
	if _, err := vmg.ReadAt(head, 0); err != nil {
		return nil, fmt.Errorf("%w: %v", errNotDVD, err)
	}
	if string(head[:12]) != "DVDVIDEO-VMG" {
		return nil, errNotDVD
	}
	table := int64(binary.BigEndian.Uint32(head[0xC4:])) * ifoSector
	count := make([]byte, 2)
	if _, err := vmg.ReadAt(count, table); err != nil {
		return nil, fmt.Errorf("%w: %v", errNotDVD, err)
	}
	n := int(binary.BigEndian.Uint16(count))
	if n == 0 || n > maxTitles {
		return nil, fmt.Errorf("%w: %d titles", errNotDVD, n)
	}
	data := make([]byte, 12*n)
	if _, err := vmg.ReadAt(data, table+8); err != nil {
		return nil, fmt.Errorf("%w: %v", errNotDVD, err)
	}
	titles := make([]dvdTitle, n)
	for i := range titles {
		entry := data[12*i:]
		titles[i] = dvdTitle{chapters: int(binary.BigEndian.Uint16(entry[2:])), set: int(entry[6]), number: int(entry[7])}
		if titles[i].set == 0 || titles[i].set > maxTitles || titles[i].number == 0 {
			return nil, fmt.Errorf("%w: title %d", errNotDVD, i+1)
		}
	}
	return titles, nil
}

// titleLength reads how long a title of the title set vts describes
// plays: the playback times of the program chains its chapters are in.
func titleLength(vts io.ReaderAt, t dvdTitle) (time.Duration, error) {
	head := make([]byte, 0xD0)
	if _, err := vts.ReadAt(head, 0); err != nil {
		return 0, fmt.Errorf("%w: %v", errNotDVD, err)
	}
	if string(head[:12]) != "DVDVIDEO-VTS" {
		return 0, errNotDVD
	}
	chapters := int64(binary.BigEndian.Uint32(head[0xC8:])) * ifoSector
	chains := int64(binary.BigEndian.Uint32(head[0xCC:])) * ifoSector
	// The chapter table: its titles' count, then where each title's
	// chapters start in it, each chapter its chain and program.
	table := make([]byte, 8+4*t.number)
	if _, err := vts.ReadAt(table, chapters); err != nil {
		return 0, fmt.Errorf("%w: %v", errNotDVD, err)
	}
	if int(binary.BigEndian.Uint16(table)) < t.number {
		return 0, fmt.Errorf("%w: title %d is not in its set", errNotDVD, t.number)
	}
	start := int64(binary.BigEndian.Uint32(table[8+4*(t.number-1):]))
	entries := make([]byte, 4*max(t.chapters, 1))
	if _, err := vts.ReadAt(entries, chapters+start); err != nil {
		return 0, fmt.Errorf("%w: %v", errNotDVD, err)
	}
	count := make([]byte, 2)
	if _, err := vts.ReadAt(count, chains); err != nil {
		return 0, fmt.Errorf("%w: %v", errNotDVD, err)
	}
	n := int(binary.BigEndian.Uint16(count))
	var length time.Duration
	seen := map[int]bool{}
	for i := 0; i < len(entries); i += 4 {
		chain := int(binary.BigEndian.Uint16(entries[i:]))
		if seen[chain] {
			continue
		}
		seen[chain] = true
		if chain == 0 || chain > n {
			return 0, fmt.Errorf("%w: program chain %d", errNotDVD, chain)
		}
		pointer := make([]byte, 4)
		if _, err := vts.ReadAt(pointer, chains+8+8*int64(chain-1)+4); err != nil {
			return 0, fmt.Errorf("%w: %v", errNotDVD, err)
		}
		at := chains + int64(binary.BigEndian.Uint32(pointer))
		played := make([]byte, 4)
		if _, err := vts.ReadAt(played, at+4); err != nil {
			return 0, fmt.Errorf("%w: %v", errNotDVD, err)
		}
		length += playbackTime(played)
	}
	return length, nil
}

// playbackTime reads a program chain's playback time: hours, minutes,
// seconds and frames in BCD, the two high bits of the frames telling the
// frame rate, 25 or 30 frames a second.
func playbackTime(b []byte) time.Duration {
	bcd := func(v byte) time.Duration { return time.Duration(v>>4*10 + v&0x0F) }
	rate := time.Duration(30)
	if b[3]>>6 == 1 {
		rate = 25
	}
	return bcd(b[0])*time.Hour + bcd(b[1])*time.Minute + bcd(b[2])*time.Second + bcd(b[3]&0x3F)*time.Second/rate
}
