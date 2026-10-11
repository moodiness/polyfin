package localfiles

import (
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// A Blu-ray disc copied as it is: a disc folder holding a BDMV folder,
// whose streams are the .m2ts files directly in BDMV/STREAM, any case. Its
// movie is one of them, the largest as a rule; the others are menus,
// extras and pieces. A scan keeps one entry per disc, its largest stream,
// matched by the disc folder's name.

// discMarker is a disc folder's name that only numbers the disc, as in
// "Disc 1", "DISC01", "Disk 2", "CD1", "BD 1" or "Part 2": the title is the
// folder above's.
var discMarker = regexp.MustCompile(`(?i)^(?:disc|disk|cd|bd|part)[ ._-]*\d{1,3}$`)

// discOf tells whether rel lies in a disc's BDMV folder, and the disc
// folder's path, "" for a disc at the folder's root.
func discOf(rel string) (disc string, ok bool) {
	parts := strings.Split(rel, "/")
	for i, part := range parts[:len(parts)-1] {
		if strings.EqualFold(part, "BDMV") {
			return strings.Join(parts[:i], "/"), true
		}
	}
	return "", false
}

// discStream tells whether rel is a disc's stream, a .m2ts file directly
// in BDMV/STREAM, and the disc folder's path, "" for a disc at the
// folder's root.
func discStream(rel string) (disc string, ok bool) {
	parts := strings.Split(rel, "/")
	n := len(parts)
	if n < 3 || !strings.EqualFold(parts[n-3], "BDMV") || !strings.EqualFold(parts[n-2], "STREAM") ||
		!strings.EqualFold(path.Ext(parts[n-1]), ".m2ts") {
		return "", false
	}
	return strings.Join(parts[:n-3], "/"), true
}

// discUnit is the unit of a disc: its folder, "." for a disc at the
// folder's root.
func discUnit(disc string) string {
	if disc == "" {
		return "."
	}
	return disc
}

// collapseDiscs keeps, of the video files a walk found, one per disc of a
// movies folder, its largest stream (the first by path among streams of
// the same size), and none of a shows folder's discs: a disc of episodes
// is no one episode. The other files in a disc's BDMV folder are left
// out; those beside it stay.
func collapseDiscs(kind string, files map[string]found) {
	largest := map[string]string{}
	streams := map[string]found{}
	for rel, f := range files {
		disc, ok := discOf(rel)
		if !ok {
			continue
		}
		delete(files, rel)
		if _, stream := discStream(rel); !stream || kind == KindShows {
			continue
		}
		streams[rel] = f
		best, seen := largest[disc]
		if !seen || f.size > streams[best].size || f.size == streams[best].size && rel < best {
			largest[disc] = rel
		}
	}
	for _, rel := range largest {
		files[rel] = streams[rel]
	}
}

// discName is what a disc's folder tells, as a movie file's name does: its
// title and year, the year of the folder above when its own name gives
// none. A folder named only after its disc number gives the folder above's
// title; a disc at the root, the folder's own (root, see locationName).
func discName(disc, root string) name {
	own, dir := discTitleFolder(disc, root)
	return completedName(parseName(own), dir)
}

// discLabel names a disc's version, as a file's name names a file's: its
// folder's name, the folder above's joined to a disc number, the folder's
// own for a disc at the root.
func discLabel(disc, root string) string {
	if disc == "" {
		return root
	}
	own := path.Base(disc)
	if !discMarker.MatchString(own) {
		return own
	}
	above, _ := discTitleFolder(disc, root)
	if above == "" {
		return own
	}
	return above + " - " + own
}

// discTitleFolder is the name a disc's title is read from, and the path of
// the folder above it, "." when there is none.
func discTitleFolder(disc, root string) (own, dir string) {
	if disc == "" {
		return root, "."
	}
	own, dir = path.Base(disc), path.Dir(disc)
	if discMarker.MatchString(own) {
		if dir == "." {
			return root, "."
		}
		own, dir = path.Base(dir), path.Dir(dir)
	}
	return own, dir
}

// locationName is the last element of a folder's path or of a share's
// address, empty when it has none.
func locationName(location string) string {
	var base string
	if shareKind(location) != "" {
		u, err := url.Parse(location)
		if err != nil {
			return ""
		}
		base = path.Base(u.Path)
	} else {
		base = filepath.Base(location)
	}
	if base == "." || base == "/" || base == string(filepath.Separator) {
		return ""
	}
	return base
}
