package localfiles

import (
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A disc on a network share, a Blu-ray disc's or a DVD's folder or image,
// is read on the share itself: its playlists, clips' information, IFO
// files and file systems, by ranges, through the share's reader (see
// shareFS and openDiscFS). FFmpeg cannot read a disc from a share, its
// bluray protocol and dvdvideo demuxer needing a path: it reads a title
// instead as one file the loopback interface serves (see Open). A Blu-ray
// disc's title is the MPEG-TS bytes FFmpeg's bluray protocol would read of
// a local disc, each play item's clip from its in time to its out time, in
// order (see blurayPieces), its keyframes from the clips' EP maps, as a
// local disc's (see Keyframes). A DVD's title is its cells, an MPEG
// program stream, its tracks' languages and its length from its IFO files,
// which the dvdvideo demuxer would read (see dvdPieces).

// seekReaderAt reads a file a share opened at any offset: it seeks, then
// reads, one read at a time. A share's reader seeking where it already is
// goes on with the request under way, so reading in order stays one
// request.
type seekReaderAt struct {
	mu sync.Mutex
	f  io.ReadSeeker
}

func (s *seekReaderAt) ReadAt(p []byte, off int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.f.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}
	n, err := io.ReadFull(s.f, p)
	if errors.Is(err, io.ErrUnexpectedEOF) {
		err = io.EOF
	}
	return n, err
}

// readerAt reads file at any offset.
func readerAt(file io.Reader) (io.ReaderAt, error) {
	switch f := file.(type) {
	case io.ReaderAt:
		return f, nil
	case io.ReadSeeker:
		return &seekReaderAt{f: f}, nil
	}
	return nil, errors.New("the file cannot be read at an offset")
}

// shareFS reads the files of a disc's folder on a share, by their path,
// any case (see discFS): dir is the folder's path in the share's folder,
// "" for its root. The files it opens stay open until close.
type shareFS struct {
	ctx context.Context
	r   remote
	dir string

	mu     sync.Mutex
	opened []io.Closer
}

// find finds the entry at path, its elements any case, and its path in
// the share's folder.
func (f *shareFS) find(names ...string) (string, remoteEntry, error) {
	rel, entry := f.dir, remoteEntry{folder: true}
	for _, name := range names {
		if !entry.folder {
			return "", remoteEntry{}, fmt.Errorf("%w: %s is no folder", errNotDisc, rel)
		}
		entries, err := f.r.list(f.ctx, rel)
		if err != nil {
			return "", remoteEntry{}, err
		}
		i := slices.IndexFunc(entries, func(e remoteEntry) bool { return e.name == name })
		if i < 0 {
			i = slices.IndexFunc(entries, func(e remoteEntry) bool { return strings.EqualFold(e.name, name) })
		}
		if i < 0 {
			return "", remoteEntry{}, fmt.Errorf("%w: no %s in %q", errNotDisc, name, rel)
		}
		entry, rel = entries[i], path.Join(rel, entries[i].name)
	}
	return rel, entry, nil
}

func (f *shareFS) names(names ...string) ([]string, error) {
	rel, entry, err := f.find(names...)
	if err != nil {
		return nil, err
	}
	if !entry.folder {
		return nil, fmt.Errorf("%w: %s is no folder", errNotDisc, rel)
	}
	entries, err := f.r.list(f.ctx, rel)
	if err != nil {
		return nil, err
	}
	list := make([]string, len(entries))
	for i, e := range entries {
		list[i] = e.name
	}
	return list, nil
}

func (f *shareFS) file(names ...string) (*io.SectionReader, error) {
	rel, entry, err := f.find(names...)
	if err != nil {
		return nil, err
	}
	if entry.folder {
		return nil, fmt.Errorf("%w: %s is a folder", errNotDisc, rel)
	}
	file, _, err := f.r.open(f.ctx, rel)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.opened = append(f.opened, file)
	f.mu.Unlock()
	return io.NewSectionReader(&seekReaderAt{f: file}, 0, entry.size), nil
}

// close closes the files opened.
func (f *shareFS) close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, file := range f.opened {
		file.Close()
	}
	f.opened = nil
}

// shareDiscFS opens the file system of a disc on a share: its folder's,
// or its image's, read by ranges. It holds what it opened until release.
func (s *Service) shareDiscFS(ctx context.Context, d discInput) (fsys discFS, release func(), err error) {
	r, err := s.folderRemote(d.folder, d.root, d.user, d.sealed)
	if err != nil {
		return nil, nil, err
	}
	if !d.image {
		f := &shareFS{ctx: ctx, r: r, dir: d.path}
		return f, f.close, nil
	}
	file, _, err := r.open(ctx, d.path)
	if err != nil {
		return nil, nil, err
	}
	at, err := readerAt(file)
	if err == nil {
		fsys, err = openDiscFS(at)
	}
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	return fsys, func() { file.Close() }, nil
}

// titlePiece is a part of a disc title's bytes: n bytes from at of the
// file rel of the share's folder.
type titlePiece struct {
	rel   string
	at, n int64
}

// maxTitlePieces bounds the parts a title is made of: a play item's, or
// each extent of its clip in an image.
const maxTitlePieces = 1 << 14

// alignedUnit is the unit a Blu-ray disc's clips are read in: 32 source
// packets of 192 bytes.
const alignedUnit = 32 * 192

// blurayPieces lays out the playlist number of a disc as FFmpeg's bluray
// protocol reads it: each play item's clip, from the aligned unit of the
// entry point at or before its in time to that of the first entry point
// at or after its out time, or the clip's end. Each clip is located by
// locate, in a file of the share's folder.
func blurayPieces(fsys discFS, number int, locate func(names []string, start, end int64) ([]titlePiece, error)) ([]titlePiece, error) {
	data, err := readDiscFile(fsys, maxPlaylistSize, "BDMV", "PLAYLIST", fmt.Sprintf("%05d.mpls", number))
	if err != nil {
		return nil, err
	}
	p, err := readPlaylist(strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	entries := map[string][]epEntry{}
	budget := maxTitleClipInfo
	var pieces []titlePiece
	for _, part := range p.clips {
		points, ok := entries[part.clip]
		if !ok {
			data, err := readDiscFile(fsys, maxClipInfoSize, "BDMV", "CLIPINF", part.clip+".clpi")
			if err != nil {
				return nil, err
			}
			if budget -= len(data); budget < 0 {
				return nil, errors.New("the title's clip information is too large")
			}
			if points, err = readEPEntries(data); err != nil {
				return nil, fmt.Errorf("clip %s: %w", part.clip, err)
			}
			entries[part.clip] = points
		}
		names := []string{"BDMV", "STREAM", part.clip + ".m2ts"}
		clip, err := fsys.file(names...)
		if err != nil {
			return nil, err
		}
		start, end := clipRange(points, uint64(part.in)*2, uint64(part.out)*2, clip.Size())
		located, err := locate(names, start, end)
		if err != nil {
			return nil, err
		}
		if pieces = append(pieces, located...); len(pieces) > maxTitlePieces {
			return nil, errors.New("the title is made of too many parts")
		}
	}
	return pieces, nil
}

// clipRange is the bytes of a clip of size bytes a play item plays from
// in to out, in units of 90 kHz (see blurayPieces).
func clipRange(points []epEntry, in, out uint64, size int64) (start, end int64) {
	end = size
	i, _ := slices.BinarySearchFunc(points, in+1, func(e epEntry, t uint64) int { return cmp.Compare(e.pts, t) })
	if i > 0 {
		start = int64(points[i-1].spn) * 192 / alignedUnit * alignedUnit
	}
	if j, _ := slices.BinarySearchFunc(points, out, func(e epEntry, t uint64) int { return cmp.Compare(e.pts, t) }); j < len(points) {
		end = min(end, int64(points[j].spn)*192/alignedUnit*alignedUnit)
	}
	return min(start, size), max(end, min(start, size))
}

// titleLayout is how a title on a share is laid out, and its tracks'
// languages (see media.Input), kept while its entry stays the same.
type titleLayout struct {
	size      int64
	modified  time.Time
	pieces    []titlePiece
	languages map[string]string
	// length is a DVD title's, as its program chains tell it.
	length time.Duration
}

// maxLayouts bounds the layouts kept.
const maxLayouts = 64

// shareTitle reports whether rel is the entry of a disc's title on a share
// that plays as a title stream (see openTitle): a Blu-ray disc's playlist,
// a DVD's VOB, or a disc image, or one of their titles.
func shareTitle(rel string) bool {
	if base, _, ok := titleOf(rel); ok {
		rel = base
	}
	_, _, playlist := discPlaylist(rel)
	_, dvd := dvdOf(rel)
	return playlist || imageFile(rel) || dvd && dvdSet(rel) > 0
}

// layout lays out the title of a disc on a share the version key names,
// d (see blurayPieces and dvdPieces), or returns the layout kept.
func (s *Service) layout(ctx context.Context, key accounts.ID, d discInput) (titleLayout, error) {
	s.mu.Lock()
	kept, ok := s.layouts[key]
	s.mu.Unlock()
	if ok && kept.size == d.size && kept.modified.Equal(d.modified) {
		return kept, nil
	}
	if d.title < 0 {
		return titleLayout{}, ErrGone
	}
	fsys, release, err := s.shareDiscFS(ctx, d)
	if err != nil {
		return titleLayout{}, err
	}
	defer release()
	// A DVD's files are in its VIDEO_TS folder, the folder of a DVD's
	// entry, or the image's.
	var folder []string
	if d.kind == DiscDVD && d.image {
		folder = []string{"VIDEO_TS"}
	}
	locate := func(names []string, start, end int64) ([]titlePiece, error) {
		names = slices.Concat(folder, names)
		rel := path.Join(d.path, path.Join(names...))
		if folder, ok := fsys.(*shareFS); ok {
			found, _, err := folder.find(names...)
			if err != nil {
				return nil, err
			}
			return []titlePiece{{rel: found, at: start, n: end - start}}, nil
		}
		image, ok := fsys.(interface {
			spansOf(path ...string) ([]span, error)
		})
		if !ok {
			return nil, fmt.Errorf("%s: %w", rel, errNotDisc)
		}
		spans, err := image.spansOf(names...)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		return slicePieces(d.path, spans, start, end), nil
	}
	layout := titleLayout{size: d.size, modified: d.modified}
	if d.kind == DiscDVD {
		open := func(name string) (*io.SectionReader, error) {
			return fsys.file(slices.Concat(folder, []string{name})...)
		}
		layout.pieces, layout.languages, err = dvdPieces(open, d.title, func(name string, start, end int64) ([]titlePiece, error) {
			return locate([]string{name}, start, end)
		})
		if err == nil {
			layout.length, err = dvdLength(open, d.title)
		}
	} else {
		layout.pieces, err = blurayPieces(fsys, d.title, locate)
	}
	if err != nil {
		return titleLayout{}, err
	}
	s.mu.Lock()
	if s.layouts == nil || len(s.layouts) >= maxLayouts {
		s.layouts = map[accounts.ID]titleLayout{}
	}
	s.layouts[key] = layout
	s.mu.Unlock()
	return layout, nil
}

// dvdPieces lays out a DVD's title number, 0 for its first, as its
// program chains play it: their cells, in order, each from its first
// VOBU's start sector to its last VOBU's end, in its title set's VOBs read
// one after the other; of an angle block, the first angle's cells only.
// Each VOB is located by locate. It also tells the title's tracks'
// languages, by the identifiers FFmpeg's MPEG program stream demuxer gives
// their streams (see dvdLanguages). The IFO files are opened by their
// name in upper case.
func dvdPieces(open func(name string) (*io.SectionReader, error), number int,
	locate func(name string, start, end int64) ([]titlePiece, error)) ([]titlePiece, map[string]string, error) {
	vmg, err := open("VIDEO_TS.IFO")
	if err != nil {
		return nil, nil, err
	}
	titles, err := readTitles(vmg)
	if err != nil {
		return nil, nil, err
	}
	number = max(number, 1)
	if number > len(titles) {
		return nil, nil, fmt.Errorf("%w: no title %d", errNotDVD, number)
	}
	t := titles[number-1]
	vts, err := open(fmt.Sprintf("VTS_%02d_0.IFO", t.set))
	if err != nil {
		return nil, nil, err
	}
	chains, err := titleChains(vts, t)
	if err != nil {
		return nil, nil, err
	}
	// The set's title VOBs, its sectors one after the other.
	type vob struct {
		name        string
		start, size int64
	}
	var vobs []vob
	var total int64
	for k := 1; k <= 9; k++ {
		name := fmt.Sprintf("VTS_%02d_%d.VOB", t.set, k)
		file, err := open(name)
		if err != nil {
			break
		}
		vobs = append(vobs, vob{name: name, start: total, size: file.Size()})
		total += file.Size()
	}
	if len(vobs) == 0 {
		return nil, nil, fmt.Errorf("%w: no VOB of title set %d", errNotDVD, t.set)
	}
	var pieces []titlePiece
	for _, chain := range chains {
		count, offset := int(chain.head[3]), int64(binary.BigEndian.Uint16(chain.head[0xE8:]))
		if count == 0 || offset == 0 {
			continue
		}
		table := make([]byte, 24*count)
		if _, err := vts.ReadAt(table, chain.at+offset); err != nil {
			return nil, nil, fmt.Errorf("%w: %v", errNotDVD, err)
		}
		for i := range count {
			cell := table[24*i:]
			// Bits 7 and 6 of the category: the cell's place in its block,
			// 1 for its first; bits 5 and 4: the block's type, 1 for
			// angles.
			if cell[0]>>4&3 == 1 && cell[0]>>6 > 1 {
				continue
			}
			first, last := int64(binary.BigEndian.Uint32(cell[8:])), int64(binary.BigEndian.Uint32(cell[20:]))
			if last < first {
				return nil, nil, fmt.Errorf("%w: a cell ends before it starts", errNotDVD)
			}
			start, end := first*ifoSector, (last+1)*ifoSector
			for _, v := range vobs {
				from, to := max(start, v.start), min(end, v.start+v.size)
				if from >= to {
					continue
				}
				located, err := locate(v.name, from-v.start, to-v.start)
				if err != nil {
					return nil, nil, err
				}
				if pieces = append(pieces, located...); len(pieces) > maxTitlePieces {
					return nil, nil, errors.New("the title is made of too many parts")
				}
			}
		}
	}
	if len(pieces) == 0 {
		return nil, nil, fmt.Errorf("%w: title %d plays no cell", errNotDVD, number)
	}
	languages, err := dvdLanguages(vts, chains[0].head)
	if err != nil {
		return nil, nil, err
	}
	return pieces, languages, nil
}

// dvdLanguages reads the languages of the tracks of a title set, by the
// identifiers FFmpeg's MPEG program stream demuxer gives their streams: of
// each audio and subpicture stream the program chain whose head is pgc
// plays, the language the set's attributes give the stream's number, an
// ISO 639-1 code, as FFmpeg's dvdvideo demuxer reads them.
func dvdLanguages(vts io.ReaderAt, pgc []byte) (map[string]string, error) {
	attributes := make([]byte, 0x400)
	if _, err := vts.ReadAt(attributes, 0); err != nil {
		return nil, fmt.Errorf("%w: %v", errNotDVD, err)
	}
	languages := map[string]string{}
	code := func(b []byte) string {
		if b[0] < 'a' || b[0] > 'z' || b[1] < 'a' || b[1] > 'z' {
			return ""
		}
		return string(b[:2])
	}
	for i := range min(int(binary.BigEndian.Uint16(attributes[0x202:])), 8) {
		control := binary.BigEndian.Uint16(pgc[0x0C+2*i:])
		attribute := attributes[0x204+8*i:]
		if control&0x8000 == 0 || attribute[0]>>2&3 != 1 {
			continue
		}
		stream := int(control >> 8 & 7)
		var id int
		switch attribute[0] >> 5 {
		case 0:
			id = 0x80 + stream
		case 2, 3:
			id = 0x1C0 + stream
		case 4:
			id = 0xA0 + stream
		case 6:
			id = 0x88 + stream
		default:
			continue
		}
		if lang := code(attribute[2:4]); lang != "" {
			languages[fmt.Sprintf("0x%x", id)] = lang
		}
	}
	for i := range min(int(binary.BigEndian.Uint16(attributes[0x254:])), 32) {
		control := binary.BigEndian.Uint32(pgc[0x1C+4*i:])
		attribute := attributes[0x256+6*i:]
		if control&0x80000000 == 0 || attribute[0]&3 != 1 {
			continue
		}
		if lang := code(attribute[2:4]); lang != "" {
			languages[fmt.Sprintf("0x%x", 0x20+int(control>>24&0x1F))] = lang
		}
	}
	return languages, nil
}

// slicePieces are the pieces of the image rel holding the bytes from start
// to end of a file whose bytes are spans of the image.
func slicePieces(rel string, spans []span, start, end int64) []titlePiece {
	var pieces []titlePiece
	var at int64
	for _, s := range spans {
		from, to := max(start, at), min(end, at+s.n)
		if from < to {
			if s.at < 0 {
				pieces = append(pieces, titlePiece{at: -1, n: to - from})
			} else {
				pieces = append(pieces, titlePiece{rel: rel, at: s.at + from - at, n: to - from})
			}
		}
		at += s.n
	}
	return pieces
}

// openTitle opens the title of a disc on a share the version key names as
// one file (see blurayPieces), its files opened on the share as reads
// reach them.
func (s *Service) openTitle(ctx context.Context, key accounts.ID) (io.ReadSeekCloser, time.Time, error) {
	d, err := s.disc(ctx, key.String())
	if err != nil {
		return nil, time.Time{}, err
	}
	if !d.share {
		return nil, time.Time{}, ErrGone
	}
	layout, err := s.layout(ctx, key, d)
	if err != nil {
		return nil, time.Time{}, err
	}
	r, err := s.folderRemote(d.folder, d.root, d.user, d.sealed)
	if err != nil {
		return nil, time.Time{}, err
	}
	pieces := layout.pieces
	stream := &titleStream{pieces: pieces, files: map[string]*sharedFile{}, open: func(rel string) (io.ReadSeekCloser, error) {
		file, _, err := r.open(ctx, rel)
		return file, err
	}}
	for _, p := range pieces {
		stream.size += p.n
	}
	return stream, d.modified, nil
}

// titleStream reads a title's pieces one after the other, as one file.
type titleStream struct {
	pieces []titlePiece
	size   int64
	offset int64
	open   func(rel string) (io.ReadSeekCloser, error)
	files  map[string]*sharedFile
}

// sharedFile is a file of a share a title stream reads.
type sharedFile struct {
	file io.ReadSeekCloser
	at   io.ReaderAt
}

func (t *titleStream) Read(p []byte) (int, error) {
	if t.offset >= t.size {
		return 0, io.EOF
	}
	at := int64(0)
	for _, piece := range t.pieces {
		if t.offset >= at+piece.n {
			at += piece.n
			continue
		}
		into := p[:min(int64(len(p)), at+piece.n-t.offset)]
		from := piece.at + t.offset - at
		if piece.at < 0 {
			clear(into)
		} else {
			f, err := t.file(piece.rel)
			if err != nil {
				return 0, err
			}
			n, err := f.at.ReadAt(into, from)
			t.offset += int64(n)
			if err != nil && !(errors.Is(err, io.EOF) && n == len(into)) {
				if errors.Is(err, io.EOF) {
					err = io.ErrUnexpectedEOF
				}
				return n, err
			}
			return n, nil
		}
		t.offset += int64(len(into))
		return len(into), nil
	}
	return 0, io.EOF
}

// file opens a file of the share as reads reach it.
func (t *titleStream) file(rel string) (*sharedFile, error) {
	if f, ok := t.files[rel]; ok {
		return f, nil
	}
	file, err := t.open(rel)
	if err != nil {
		return nil, err
	}
	f := &sharedFile{file: file, at: &seekReaderAt{f: file}}
	t.files[rel] = f
	return f, nil
}

func (t *titleStream) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekCurrent:
		offset += t.offset
	case io.SeekEnd:
		offset += t.size
	}
	if offset < 0 {
		return 0, errors.New("seek before the start of the title")
	}
	t.offset = offset
	return offset, nil
}

func (t *titleStream) Close() error {
	for _, f := range t.files {
		f.file.Close()
	}
	t.files = nil
	return nil
}

// maxIFOSize bounds the IFO files read whole from a share: a few hundred
// KB as a rule.
const maxIFOSize = 16 << 20

// readIFO reads the IFO file rel of a share through a walk's opener, its
// name in upper case, else in lower case, as discs are copied in either.
func readIFO(ctx context.Context, list listing, rel string) (io.ReaderAt, error) {
	file, err := list.open(ctx, rel)
	if err != nil {
		dir, name := path.Split(rel)
		if file, err = list.open(ctx, dir+strings.ToLower(name)); err != nil {
			return nil, err
		}
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxIFOSize+1))
	if err == nil && len(data) > maxIFOSize {
		err = fmt.Errorf("%w: %s is too large", errNotDVD, rel)
	}
	if err != nil {
		return nil, err
	}
	return strings.NewReader(string(data)), nil
}

// dvdLength is how long a DVD's title number, 0 for its first, plays, as
// its program chains tell it and FFmpeg's dvdvideo demuxer reports it.
func dvdLength(open func(name string) (*io.SectionReader, error), number int) (time.Duration, error) {
	vmg, err := open("VIDEO_TS.IFO")
	if err != nil {
		return 0, err
	}
	titles, err := readTitles(vmg)
	if err != nil {
		return 0, err
	}
	number = max(number, 1)
	if number > len(titles) {
		return 0, fmt.Errorf("%w: no title %d", errNotDVD, number)
	}
	vts, err := open(fmt.Sprintf("VTS_%02d_0.IFO", titles[number-1].set))
	if err != nil {
		return 0, err
	}
	c, err := readTitle(vts, titles[number-1])
	return c.length, err
}
