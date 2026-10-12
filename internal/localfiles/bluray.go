package localfiles

import (
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
)

// A Blu-ray disc's movies are its playlists, BDMV/PLAYLIST/*.mpls, each
// playing clips, the streams in BDMV/STREAM, from one time to another. A
// disc in a folder in the container plays its main playlist, the longest,
// whole, through FFmpeg's bluray protocol (see Service.Input), and a disc
// on a share the same bytes, served as one file (see sharedisc.go); each
// other playlist long enough to be a movie, and playing streams the
// titles kept before it do not, is a title of its own, a unit to match or
// link (see discTitles). A disc whose playlists cannot be read or are too
// short for FFmpeg plays its largest stream. A show's disc, in a shows
// folder, is its episodes, its playlists chosen otherwise (see
// episodes.go), and is left out when its playlists cannot be read.

// minMovie is how long a playlist other than the main one plays at least
// to be a title of its own: shorter ones are extras and menus' clips.
var minMovie = 20 * time.Minute

// minPlayable is how long a disc's main playlist plays at least for FFmpeg
// to play the disc: its bluray protocol refuses a disc without a playlist
// of 3 minutes (MIN_PLAYLIST_LENGTH).
const minPlayable = 3 * time.Minute

// maxDiscTitles bounds a disc's titles. Some discs hide their movie among
// hundreds of playlists as long as it, in another order of its streams;
// those mostly reuse the main playlist's streams, and are left out. A disc
// whose playlists still give more titles than a box set's disc holds,
// four movies as a rule, keeps its main title only, rather than titles
// that may be decoys.
const maxDiscTitles = 6

// discPlaylist tells whether rel is a disc's playlist, a file named after
// its number, NNNNN.mpls, directly in BDMV/PLAYLIST, with the disc
// folder's path, "" for a disc at the folder's root.
func discPlaylist(rel string) (disc string, number int, ok bool) {
	parts := strings.Split(rel, "/")
	n := len(parts)
	if n < 3 || !strings.EqualFold(parts[n-3], "BDMV") || !strings.EqualFold(parts[n-2], "PLAYLIST") ||
		!strings.EqualFold(path.Ext(parts[n-1]), ".mpls") {
		return "", 0, false
	}
	stem := strings.TrimSuffix(parts[n-1], path.Ext(parts[n-1]))
	number, err := strconv.Atoi(stem)
	if len(stem) != 5 || err != nil {
		return "", 0, false
	}
	return strings.Join(parts[:n-3], "/"), number, true
}

// playlistFile reports whether rel is a disc's playlist a walk lists (see
// discPlaylist).
func playlistFile(rel string) bool {
	_, _, ok := discPlaylist(rel)
	return ok
}

// titleUnit is the unit of a disc's title other than its main one, whose
// unit is the disc's (see discUnit): the disc's, then # and the title's
// number, a Blu-ray disc's playlist or a DVD's title, as in
// "Box/DISC02#00002". A disc image's titles are the image's, as in
// "Box.iso#00002".
func titleUnit(disc string, number int) string {
	return fmt.Sprintf("%s#%05d", discUnit(disc), number)
}

// playlist is what a disc's playlist plays: its clips, by their name, in
// order, and how long it plays.
type playlist struct {
	number int
	clips  []clipPart
	length time.Duration
}

// clipPart is a part of a clip a playlist plays: from in to out, in units
// of 45 kHz of the clip's timestamps.
type clipPart struct {
	clip    string
	in, out uint32
	length  time.Duration
}

// errNotPlaylist reports a file that is not a Blu-ray playlist.
var errNotPlaylist = errors.New("not a Blu-ray playlist")

// maxPlaylistSize bounds the playlist files read: a few KB as a rule.
const maxPlaylistSize = 1 << 20

// readPlaylist reads a playlist file: its play items, the clip each plays
// (its first angle's), and the times it plays it from and to, in units of
// 45 kHz.
func readPlaylist(r io.Reader) (playlist, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxPlaylistSize))
	if err != nil {
		return playlist{}, err
	}
	if len(data) < 20 || string(data[:4]) != "MPLS" {
		return playlist{}, errNotPlaylist
	}
	at := int(binary.BigEndian.Uint32(data[8:]))
	if at+10 > len(data) {
		return playlist{}, fmt.Errorf("%w: no play list", errNotPlaylist)
	}
	count := int(binary.BigEndian.Uint16(data[at+6:]))
	var p playlist
	at += 10
	for range count {
		if at+22 > len(data) {
			return playlist{}, fmt.Errorf("%w: a play item is cut short", errNotPlaylist)
		}
		size := int(binary.BigEndian.Uint16(data[at:]))
		item := data[at+2:]
		clip := string(item[:5])
		if _, err := strconv.Atoi(clip); err != nil || len(strings.TrimLeft(clip, "0123456789")) > 0 {
			return playlist{}, fmt.Errorf("%w: clip %q", errNotPlaylist, clip)
		}
		in, out := binary.BigEndian.Uint32(item[12:]), binary.BigEndian.Uint32(item[16:])
		if out < in {
			return playlist{}, fmt.Errorf("%w: clip %s ends before it starts", errNotPlaylist, clip)
		}
		part := clipPart{clip: clip, in: in, out: out, length: time.Duration(out-in) * time.Second / 45000}
		p.clips = append(p.clips, part)
		p.length += part.length
		at += 2 + size
	}
	if len(p.clips) == 0 {
		return playlist{}, fmt.Errorf("%w: no play item", errNotPlaylist)
	}
	return p, nil
}

// discTitles chooses a disc's titles among its playlists with choose,
// chooseTitles for a movies folder's discs and chooseEpisodes for a
// show's, each playing its clips, each clip from its in time to its out
// time.
func discTitles(playlists []playlist, choose chooser) []playlist {
	candidates := make([]titleCandidate, len(playlists))
	for i, p := range playlists {
		candidates[i] = titleCandidate{number: p.number, length: p.length}
		for _, part := range p.clips {
			candidates[i].parts = append(candidates[i].parts, titlePart{key: part.clip, length: part.length, source: part.clip,
				from: uint64(part.in), to: uint64(part.out)})
		}
	}
	var titles []playlist
	for _, i := range choose(candidates) {
		titles = append(titles, playlists[i])
	}
	return titles
}

// chooser chooses a disc's titles among its candidates, and returns their
// indexes: chooseTitles, a movies folder's, or chooseEpisodes, a shows
// folder's.
type chooser func(candidates []titleCandidate) []int

// titleChooser is how the discs of a folder of kind have their titles
// chosen.
func titleChooser(kind string) chooser {
	if kind == KindShows {
		return chooseEpisodes
	}
	return chooseTitles
}

// titleCandidate is a title of a disc that may be one of its movies: its
// number, how long it plays, and the parts of the disc it plays, a Blu-ray
// disc's clips or a DVD's cells.
type titleCandidate struct {
	number int
	length time.Duration
	parts  []titlePart
}

// titlePart is a part of a disc a title plays, by what tells it, and how
// long it plays; source holds it, a Blu-ray disc's clip or a DVD's title
// set, from from to to, the clip's times in units of 45 kHz or the set's
// sectors (see chooseEpisodes).
type titlePart struct {
	key      string
	length   time.Duration
	source   string
	from, to uint64
}

// chooseTitles chooses a disc's titles among its candidates, and returns
// their indexes: its main title, the longest, the first by number of
// those as long, then, by length, each other title at least minMovie long
// that plays mostly parts the titles chosen before it do not: another cut,
// angle or language of a movie counts once. A disc that would have more
// than maxDiscTitles titles has its main title only.
func chooseTitles(candidates []titleCandidate) []int {
	if len(candidates) == 0 {
		return nil
	}
	sorted := make([]int, len(candidates))
	for i := range sorted {
		sorted[i] = i
	}
	slices.SortFunc(sorted, func(a, b int) int {
		return cmp.Or(cmp.Compare(candidates[b].length, candidates[a].length), cmp.Compare(candidates[a].number, candidates[b].number))
	})
	titles := []int{sorted[0]}
	played := map[string]bool{}
	for _, part := range candidates[sorted[0]].parts {
		played[part.key] = true
	}
	for _, i := range sorted[1:] {
		c := candidates[i]
		if c.length < minMovie {
			break
		}
		var fresh, total time.Duration
		for _, part := range c.parts {
			total += part.length
			if !played[part.key] {
				fresh += part.length
			}
		}
		if fresh*2 <= total {
			continue
		}
		titles = append(titles, i)
		for _, part := range c.parts {
			played[part.key] = true
		}
	}
	if len(titles) > maxDiscTitles {
		return titles[:1]
	}
	return titles
}

// discFiles are a disc's files a walk listed: its streams and playlists.
type discFiles struct {
	streams, playlists map[string]found
}

// collapseDiscs keeps, of the files a walk found, the entries of each
// Blu-ray disc of a folder of kind: in a movies folder, each disc's; in a
// shows folder, those of each show's disc (see showDisc), whose titles are
// its episodes (see chooseEpisodes). A disc whose playlists the walk
// listed, and open reads, has an entry per title (see playlistEntries);
// another, in a movies folder, its largest stream (the first by path among
// streams of the same size). The other files in a disc's BDMV folder are
// left out; those beside it stay.
func collapseDiscs(kind string, files map[string]found, open func(rel string) (io.ReadCloser, error)) {
	discs := map[string]discFiles{}
	for rel, f := range files {
		disc, ok := discOf(rel)
		if !ok {
			continue
		}
		delete(files, rel)
		if kind == KindShows && !showDisc(disc, false) {
			continue
		}
		d, ok := discs[disc]
		if !ok {
			d = discFiles{streams: map[string]found{}, playlists: map[string]found{}}
			discs[disc] = d
		}
		if _, ok := discStream(rel); ok {
			d.streams[rel] = f
		} else if _, _, ok := discPlaylist(rel); ok {
			d.playlists[rel] = f
		}
	}
	for _, d := range discs {
		if entries := playlistEntries(d, open, kind); len(entries) > 0 {
			maps.Copy(files, entries)
			continue
		}
		if kind == KindShows {
			// A show's disc is its episodes, which its playlists tell.
			continue
		}
		largest := ""
		for rel, f := range d.streams {
			if largest == "" || f.size > d.streams[largest].size || f.size == d.streams[largest].size && rel < largest {
				largest = rel
			}
		}
		if largest != "" {
			files[largest] = d.streams[largest]
		}
	}
}

// playlistEntries are the entries of the titles of a disc of a folder of
// kind (see discTitles and titleChooser), of its files a walk found: each
// its playlist, read through open, sized as the streams it plays, modified
// when the last of them or the playlist was, with how long it plays; the
// titles of a movies folder's disc other than its main one are marked as
// such. A playlist that cannot be read, or plays a stream the walk did not
// find, is left out; none is kept when the first title is too short for
// FFmpeg to play (see minPlayable).
func playlistEntries(d discFiles, open func(rel string) (io.ReadCloser, error), kind string) map[string]found {
	clips := map[string]found{}
	for rel, f := range d.streams {
		clips[strings.ToUpper(strings.TrimSuffix(path.Base(rel), path.Ext(rel)))] = f
	}
	var playlists []playlist
	paths := map[int]string{}
	for rel := range d.playlists {
		_, number, _ := discPlaylist(rel)
		file, err := open(rel)
		if err != nil {
			continue
		}
		p, err := readPlaylist(file)
		file.Close()
		if err != nil || slices.ContainsFunc(p.clips, func(part clipPart) bool { _, ok := clips[part.clip]; return !ok }) {
			continue
		}
		p.number = number
		playlists = append(playlists, p)
		paths[number] = rel
	}
	titles := discTitles(playlists, titleChooser(kind))
	if len(titles) == 0 || titles[0].length < minPlayable {
		return nil
	}
	entries := map[string]found{}
	for i, title := range titles {
		rel := paths[title.number]
		f := found{modified: d.playlists[rel].modified, extra: i > 0 && kind != KindShows, length: title.length}
		counted := map[string]bool{}
		for _, part := range title.clips {
			clip := clips[part.clip]
			if !counted[part.clip] {
				counted[part.clip] = true
				f.size += clip.size
			}
			if clip.modified.After(f.modified) {
				f.modified = clip.modified
			}
		}
		entries[rel] = f
	}
	return entries
}
