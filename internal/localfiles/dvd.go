package localfiles

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"maps"
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
// movie is its main title, the longest; each other title long enough to
// be a movie, and playing cells the titles kept before it do not, is a
// movie of its own, as a Blu-ray disc's playlists are (see chooseTitles);
// the others are extras, trailers and menus' clips. A scan of a movies
// folder in the container keeps an entry per DVD, the first VOB of its
// main title's set, sized as the set's VOBs and matched by the disc
// folder's name, as a Blu-ray disc is, and one per other movie, the same
// VOB of its own set with its title's number (see discTitlePath), a unit
// of its own linked by hand. FFmpeg plays a title from the VIDEO_TS
// folder, with its dvdvideo demuxer (see Input): the version is not one
// file; on a share, from its title's cells, served as one file (see
// sharedisc.go). A show's DVD, in a shows folder, is its episodes, each
// kept as another movie is (see episodes.go).

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

// collapseDVDs keeps, of the files a walk found, the entries of each DVD
// of a folder of kind (see dvdEntries), its titles chosen by titles from
// its VIDEO_TS folder's path: in a movies folder, each DVD's; in a shows
// folder, those of each show's disc (see showDisc), whose titles are its
// episodes. Without titles, DVDs are left out.
func collapseDVDs(kind string, files map[string]found, titles func(videoTS string) ([]dvdChoice, error)) {
	discs := map[string]map[string]found{}
	for rel, f := range files {
		disc, ok := dvdOf(rel)
		if !ok {
			continue
		}
		delete(files, rel)
		if kind == KindShows && !showDisc(disc, false) || titles == nil {
			continue
		}
		if discs[disc] == nil {
			discs[disc] = map[string]found{}
		}
		discs[disc][rel] = f
	}
	for _, parts := range discs {
		maps.Copy(files, dvdEntries(parts, titles, kind == KindShows))
	}
}

// dvdEntries are the entries a scan keeps for a DVD, of its files a walk
// found: for its main title, the first VOB of the title's set, sized as
// the set's VOBs, modified when the last of them or VIDEO_TS.IFO was; for
// each other movie (see dvdTitles), the same of its own set, with its
// title's number (see discTitlePath), marked as such. The largest set
// stands for the main title's when the IFO files cannot be read; a DVD
// without VIDEO_TS.IFO, or without a VOB of that set, is left out, and so
// is another movie without a VOB of its set. A show's DVD, episodes set,
// has no main title: each of its episodes is kept as another movie is,
// sized as its cells when its IFO files tell them, and it is left out
// when its IFO files cannot be read. Each entry has its title's length.
func dvdEntries(parts map[string]found, read func(videoTS string) ([]dvdChoice, error), episodes bool) map[string]found {
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
		return nil
	}
	titles, err := read(path.Dir(ifo))
	if err != nil && episodes {
		return nil
	}
	if err != nil {
		set := 0
		for s, size := range sizes {
			if set == 0 || size > sizes[set] || size == sizes[set] && s < set {
				set = s
			}
		}
		titles = []dvdChoice{{set: set}}
	}
	entries := map[string]found{}
	for i, title := range titles {
		var rel string
		f := found{modified: parts[ifo].modified, extra: i > 0 && !episodes, length: title.length}
		for name, part := range parts {
			if dvdSet(name) != title.set {
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
		switch {
		case rel == "" && i == 0 && !episodes:
			return nil
		case rel == "":
		case i == 0 && !episodes:
			entries[rel] = f
		default:
			if episodes && title.size > 0 {
				f.size = title.size
			}
			entries[discTitlePath(rel, title.title)] = f
		}
	}
	return entries
}

// Input finds what FFmpeg reads the version key names from, its
// identifier as Streams gives it, through no symbolic link leading out of
// its folder: a Blu-ray disc's title, its playlist played from the disc's
// folder with the bluray protocol, a DVD's main title, read from its
// VIDEO_TS folder with the dvdvideo demuxer, or a disc image's main title,
// read from the image the same ways. A Blu-ray disc's title on a share,
// in its folder or an image, is read as one MPEG-TS file the loopback
// interface serves (see openTitle). Only the discs of movies folders, and
// the titles of shows' discs, have one.
func (s *Service) Input(ctx context.Context, key string) (media.Input, error) {
	d, err := s.disc(ctx, key)
	if err != nil {
		return media.Input{}, err
	}
	switch {
	case d.share && d.kind == DiscBluRay && d.title >= 0 && s.files != "":
		return media.Input{Format: "mpegts", Path: s.files + key + ".m2ts"}, nil
	case d.share && d.kind == DiscDVD && s.files != "":
		// The title's cells, an MPEG program stream; its tracks' languages
		// come from its IFO files, which the dvdvideo demuxer would read.
		layout, err := s.layout(ctx, d.id, d)
		if err != nil {
			return media.Input{}, err
		}
		return media.Input{Format: "mpeg", Path: s.files + key + ".vob", Languages: layout.languages, Duration: layout.length}, nil
	case d.share:
		return media.Input{}, ErrGone
	case d.kind == DiscDVD:
		in := media.Input{Format: "dvdvideo", Path: d.path}
		if d.title > 0 {
			in.Options = []string{"-title", strconv.Itoa(d.title)}
		}
		return in, nil
	}
	return media.Input{Options: []string{"-playlist", strconv.Itoa(d.title)}, Path: "bluray:" + d.path}, nil
}

// discInput is the disc a version that is not one file plays: a Blu-ray
// disc's or a DVD's, in its folder or in an image, on the server's disk or
// on a share.
type discInput struct {
	// kind is DiscBluRay or DiscDVD.
	kind string
	// path is the disc's folder, a DVD's VIDEO_TS folder, or the image
	// file, through no symbolic link; on a share, its path in the share's
	// folder, "" for its root.
	path  string
	image bool
	// title is a Blu-ray disc's playlist, -1 for libbluray's choice, or a
	// DVD's title, 0 for FFmpeg's choice.
	title int
	// share marks a disc on a share: folder is its folder's, root the
	// share's address, user and sealed its user and sealed password.
	share bool
	// id is the version's file's identifier.
	id                 accounts.ID
	folder             accounts.ID
	root, user, sealed string
	// size and modified are the entry's, as the last scan found it.
	size     int64
	modified time.Time
}

// disc finds the disc the version key names plays (see Input): a title
// other than a disc's main one, as each title of a show's disc, plays its
// own number (see discTitlePath).
func (s *Service) disc(ctx context.Context, key string) (discInput, error) {
	id, err := accounts.ParseID(key)
	if err != nil {
		return discInput{}, ErrGone
	}
	var root, rel, kind, image string
	var title int
	var d discInput
	err = s.db.QueryRow(ctx, `SELECT a.manifest_url, f.path, l.kind, coalesce(i.kind, ''), coalesce(i.title, 0),
			f.addon_id, l.share_user, l.share_password, f.size, f.modified
		FROM local_files f JOIN addons a ON a.id = f.addon_id JOIN local_folders l ON l.addon_id = f.addon_id
		LEFT JOIN local_disc_images i ON i.addon_id = f.addon_id AND i.path = regexp_replace(f.path, '#[0-9]+$', '')
		WHERE f.id = $1`, id).
		Scan(&root, &rel, &kind, &image, &title, &d.folder, &d.user, &d.sealed, &d.size, &d.modified)
	d.id = id
	if errors.Is(err, pgx.ErrNoRows) {
		return discInput{}, ErrGone
	}
	if err != nil {
		return discInput{}, err
	}
	if _, _, ok := showTitle(rel); kind != KindMovies && (kind != KindShows || !ok) {
		return discInput{}, ErrGone
	}
	folder := func(rel string) (string, error) { return rel, nil }
	if d.share = shareKind(root) != ""; d.share {
		d.root = root
	} else {
		real, err := filepath.EvalSymlinks(root)
		if err != nil {
			return discInput{}, ErrGone
		}
		folder = func(rel string) (string, error) {
			dir, err := filepath.EvalSymlinks(filepath.Join(real, filepath.FromSlash(rel)))
			if err != nil || !within(real, dir) {
				return "", ErrGone
			}
			return dir, nil
		}
	}
	base, number, titled := titleOf(rel)
	if titled {
		rel, title = base, number
	}
	switch image {
	case imageDVD, imageBluRay:
		if d.path, err = folder(rel); err != nil {
			return discInput{}, err
		}
		d.kind, d.image, d.title = DiscBluRay, true, title
		if image == imageDVD {
			d.kind = DiscDVD
		}
		return d, nil
	}
	if disc, number, ok := discPlaylist(rel); ok {
		if d.path, err = folder(disc); err != nil {
			return discInput{}, err
		}
		d.kind, d.title = DiscBluRay, number
		return d, nil
	}
	if _, ok := dvdOf(rel); !ok || dvdSet(rel) == 0 {
		return discInput{}, ErrGone
	}
	if d.path, err = folder(path.Dir(rel)); err != nil {
		return discInput{}, err
	}
	d.kind, d.title = DiscDVD, title
	if !titled {
		var titles []dvdChoice
		if d.share {
			titles, err = s.shareDVDTitles(ctx, d)
		} else {
			titles, err = dvdTitles(d.path, chooseTitles)
		}
		if err != nil {
			return discInput{}, err
		}
		d.title = titles[0].title
	}
	return d, nil
}

// shareDVDTitles chooses the movies of a DVD on a share (see
// dvdTitlesOf), d's path its VIDEO_TS folder.
func (s *Service) shareDVDTitles(ctx context.Context, d discInput) ([]dvdChoice, error) {
	fsys, release, err := s.shareDiscFS(ctx, d)
	if err != nil {
		return nil, err
	}
	defer release()
	return dvdTitlesOf(func(name string) (io.ReaderAt, error) { return fsys.file(name) }, chooseTitles)
}

// errNotDVD reports IFO files that do not describe a DVD's titles.
var errNotDVD = errors.New("not a DVD's IFO file")

// ifoSector is the size of a DVD's sectors, the unit IFO files' tables
// are addressed in.
const ifoSector = 2048

// maxTitles bounds the titles a DVD has: its title search table holds at
// most 99.
const maxTitles = 99

// dvdChoice is a DVD's title chosen as one of its movies or episodes: its
// number on the disc, its title set, how long it plays, and the bytes of
// its set's cells it plays, 0 when its IFO files tell none.
type dvdChoice struct {
	title, set int
	length     time.Duration
	size       int64
}

// dvdTitles chooses with choose the titles of the DVD whose VIDEO_TS
// folder is dir (see dvdTitlesOf).
func dvdTitles(dir string, choose chooser) ([]dvdChoice, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
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
	return dvdTitlesOf(func(name string) (io.ReaderAt, error) {
		real, ok := names[name]
		if !ok {
			return nil, fmt.Errorf("%w: no %s", errNotDVD, name)
		}
		f, err := os.Open(filepath.Join(dir, real))
		if err == nil {
			opened = append(opened, f)
		}
		return f, err
	}, choose)
}

// dvdTitlesOf chooses with choose a DVD's titles among those its IFO
// files list, opened by their name in upper case: for a movies folder
// (see chooseTitles), its main title, the longest, first, then the others
// long enough to be movies, each playing mostly cells the titles before it
// do not; for a show's disc, its episodes (see chooseEpisodes).
func dvdTitlesOf(open func(name string) (io.ReaderAt, error), choose chooser) ([]dvdChoice, error) {
	vmg, err := open("VIDEO_TS.IFO")
	if err != nil {
		return nil, err
	}
	titles, err := readTitles(vmg)
	if err != nil {
		return nil, err
	}
	sets := map[int]io.ReaderAt{}
	candidates := make([]titleCandidate, len(titles))
	for i, t := range titles {
		vts, ok := sets[t.set]
		if !ok {
			if vts, err = open(fmt.Sprintf("VTS_%02d_0.IFO", t.set)); err != nil {
				return nil, err
			}
			sets[t.set] = vts
		}
		if candidates[i], err = readTitle(vts, t); err != nil {
			return nil, err
		}
		candidates[i].number = i + 1
	}
	var chosen []dvdChoice
	for _, i := range choose(candidates) {
		set := strconv.Itoa(titles[i].set)
		chosen = append(chosen, dvdChoice{title: i + 1, set: titles[i].set, length: candidates[i].length,
			size: int64(ranges{set: rangesOf(candidates[i].parts)[set]}.size()) * ifoSector})
	}
	return chosen, nil
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

// readTitle reads what a title of the title set vts describes plays: the
// program chains its chapters are in, how long they play together, and
// their cells, each by its sectors in the set, with how long it plays. A
// chain without cells stands for its own.
func readTitle(vts io.ReaderAt, t dvdTitle) (titleCandidate, error) {
	var c titleCandidate
	chains, err := titleChains(vts, t)
	if err != nil {
		return c, err
	}
	set := strconv.Itoa(t.set)
	for _, chain := range chains {
		length := playbackTime(chain.head[4:])
		c.length += length
		cells, err := readCells(vts, chain.at, chain.head)
		if err != nil {
			return c, err
		}
		if len(cells) == 0 {
			key := "chain " + strconv.Itoa(chain.number)
			cells = []titlePart{{key: key, length: length, source: set + ":" + key, to: 1}}
		}
		for i := range cells {
			cells[i].key = set + ":" + cells[i].key
			if cells[i].source == "" {
				cells[i].source = set
			}
		}
		c.parts = append(c.parts, cells...)
	}
	return c, nil
}

// programChain is a program chain of a title set: its number, where it is
// in the set's IFO file, and its head, the 0xEC bytes before its tables.
type programChain struct {
	number int
	at     int64
	head   []byte
}

// titleChains reads the program chains a title of the title set vts plays,
// in the order of its chapters, each once.
func titleChains(vts io.ReaderAt, t dvdTitle) ([]programChain, error) {
	head := make([]byte, 0xD0)
	if _, err := vts.ReadAt(head, 0); err != nil {
		return nil, fmt.Errorf("%w: %v", errNotDVD, err)
	}
	if string(head[:12]) != "DVDVIDEO-VTS" {
		return nil, errNotDVD
	}
	chapters := int64(binary.BigEndian.Uint32(head[0xC8:])) * ifoSector
	chains := int64(binary.BigEndian.Uint32(head[0xCC:])) * ifoSector
	// The chapter table: its titles' count, then where each title's
	// chapters start in it, each chapter its chain and program.
	table := make([]byte, 8+4*t.number)
	if _, err := vts.ReadAt(table, chapters); err != nil {
		return nil, fmt.Errorf("%w: %v", errNotDVD, err)
	}
	if int(binary.BigEndian.Uint16(table)) < t.number {
		return nil, fmt.Errorf("%w: title %d is not in its set", errNotDVD, t.number)
	}
	start := int64(binary.BigEndian.Uint32(table[8+4*(t.number-1):]))
	entries := make([]byte, 4*max(t.chapters, 1))
	if _, err := vts.ReadAt(entries, chapters+start); err != nil {
		return nil, fmt.Errorf("%w: %v", errNotDVD, err)
	}
	count := make([]byte, 2)
	if _, err := vts.ReadAt(count, chains); err != nil {
		return nil, fmt.Errorf("%w: %v", errNotDVD, err)
	}
	n := int(binary.BigEndian.Uint16(count))
	seen := map[int]bool{}
	var list []programChain
	for i := 0; i < len(entries); i += 4 {
		chain := int(binary.BigEndian.Uint16(entries[i:]))
		if seen[chain] {
			continue
		}
		seen[chain] = true
		if chain == 0 || chain > n {
			return nil, fmt.Errorf("%w: program chain %d", errNotDVD, chain)
		}
		pointer := make([]byte, 4)
		if _, err := vts.ReadAt(pointer, chains+8+8*int64(chain-1)+4); err != nil {
			return nil, fmt.Errorf("%w: %v", errNotDVD, err)
		}
		at := chains + int64(binary.BigEndian.Uint32(pointer))
		pgc := make([]byte, 0xEC)
		if _, err := vts.ReadAt(pgc, at); err != nil {
			return nil, fmt.Errorf("%w: %v", errNotDVD, err)
		}
		list = append(list, programChain{number: chain, at: at, head: pgc})
	}
	return list, nil
}

// readCells reads the cells of the program chain at at, whose head is
// pgc: its cell playback table, each cell 24 bytes, its playback time and
// its sectors, from its first VOBU's start to its last's end.
func readCells(vts io.ReaderAt, at int64, pgc []byte) ([]titlePart, error) {
	count, offset := int(pgc[3]), int64(binary.BigEndian.Uint16(pgc[0xE8:]))
	if count == 0 || offset == 0 {
		return nil, nil
	}
	table := make([]byte, 24*count)
	if _, err := vts.ReadAt(table, at+offset); err != nil {
		return nil, fmt.Errorf("%w: %v", errNotDVD, err)
	}
	cells := make([]titlePart, count)
	for i := range cells {
		cell := table[24*i:]
		first, last := binary.BigEndian.Uint32(cell[8:]), binary.BigEndian.Uint32(cell[20:])
		cells[i] = titlePart{key: fmt.Sprintf("%d-%d", first, last), length: playbackTime(cell[4:]), from: uint64(first), to: uint64(last) + 1}
	}
	return cells, nil
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
