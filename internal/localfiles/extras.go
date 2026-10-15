package localfiles

import (
	"context"
	"errors"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Extras, local trailers and theme media are read as Jellyfin reads them,
// beside a movie in its own folder, in a show's folder or in a season's
// folder, unless the folder's ReadExtras is off: then a scan leaves them
// out, as it did before Polyfin read them.
//
// An extra is a video in one of extraFolders, of that folder's type, or a
// video whose name ends with one of extraSuffixes, or is "trailer" or
// "sample"; a theme song is an audio file named "theme", or one in a
// "theme-music" folder; a theme video, a video in a "backdrops" folder.
// None is a title: it is never matched, listed nor offered as a version,
// unless a link an administrator made names its file (see splitExtras).
// Its name is its file's without its extension or suffix, else its NFO
// file's <title>, else its file's as written. It belongs to the titles of
// the units beside it: the movies directly in its folder, or in a folder
// shared by several movies, those its name begins with; or the show of
// its folder, with the season of a season's folder (see extraOwners). A
// movie's extra named with a suffix whose movie is not beside it is listed
// unmatched, so that it can be linked (see reasonLoneExtra). The library
// serves each extra as an item of its own (see library.Extra), which plays
// its file, with the subtitle files beside it, as a version does.

// extraFolders are the folders whose videos are extras, by their name in
// lower case, with their extra type: Jellyfin's.
var extraFolders = map[string]string{
	"behind the scenes": "BehindTheScenes",
	"deleted scenes":    "DeletedScene",
	"interviews":        "Interview",
	"scenes":            "Scene",
	"samples":           "Sample",
	"shorts":            "Short",
	"featurettes":       "Featurette",
	"clips":             "Clip",
	"other":             "Unknown",
	"extras":            "Unknown",
	"trailers":          library.ExtraTrailer,
	"theme-music":       library.ExtraThemeSong,
	"backdrops":         library.ExtraThemeVideo,
}

// extraSuffixes are the ends of the names of videos that are extras, in
// lower case, with their extra type: Jellyfin's.
var extraSuffixes = []struct{ suffix, extraType string }{
	{"-trailer", library.ExtraTrailer}, {".trailer", library.ExtraTrailer}, {"_trailer", library.ExtraTrailer},
	{" trailer", library.ExtraTrailer}, {"-sample", "Sample"}, {".sample", "Sample"}, {"_sample", "Sample"}, {" sample", "Sample"},
	{"-scene", "Scene"}, {"-clip", "Clip"}, {"-interview", "Interview"}, {"-behindthescenes", "BehindTheScenes"},
	{"-deleted", "DeletedScene"}, {"-deletedscene", "DeletedScene"}, {"-featurette", "Featurette"}, {"-short", "Short"},
	{"-other", "Unknown"}, {"-extra", "Unknown"},
}

// extraNames are the names of videos that are extras, in lower case, with
// their extra type.
var extraNames = map[string]string{"trailer": library.ExtraTrailer, "sample": "Sample"}

// formerlySkipped are the folders scans left out before they read extras,
// by their name in lower case: a scan of a folder whose ReadExtras is
// off, and of a home videos folder, still does.
var formerlySkipped = map[string]bool{
	"extras": true, "featurettes": true, "behind the scenes": true, "deleted scenes": true, "interviews": true, "scenes": true,
	"shorts": true, "trailers": true, "samples": true, "other": true,
}

// sampleVideo reports whether a video's name is a sample's, which scans
// left out before they read extras.
func sampleVideo(name string) bool {
	s := strings.ToLower(stem(name))
	return s == "sample" || strings.HasSuffix(s, "-sample") || strings.HasSuffix(s, ".sample")
}

// themeAudio reports whether rel is a theme song a walk of a movies or
// shows folder lists: an audio file named "theme", or in a "theme-music"
// folder.
func themeAudio(rel string) bool {
	return audioFile(rel) && (strings.EqualFold(stem(rel), "theme") || strings.EqualFold(path.Base(path.Dir(rel)), "theme-music"))
}

// How a file a walk listed is read: as a title's, as an extra, or left
// out, as a file in an extras folder that is no extra.
const (
	readAsTitle = iota
	readAsExtra
	leftOut
)

// extraFile is what an extra's path tells: its type, the folder it is
// beside, "." for the folder's top, and, for a video named with a suffix,
// its name without it, which may name the movie it belongs to.
type extraFile struct {
	file      found
	extraType string
	dir       string
	base      string
	named     bool
}

// readExtra tells how a scan reading extras reads the file rel of a
// movies or shows folder (see the comment at the top of this file).
func readExtra(rel string) (extraFile, int) {
	parts := strings.Split(rel, "/")
	audio := audioFile(rel)
	for i, part := range parts[:len(parts)-1] {
		extraType, ok := extraFolders[strings.ToLower(part)]
		if !ok {
			continue
		}
		// Only the files directly in the folder are extras, a theme music
		// folder's audio files and another's videos.
		if i != len(parts)-2 || audio != (extraType == library.ExtraThemeSong) || !audio && !videoFile(parts[i+1]) {
			return extraFile{}, leftOut
		}
		return extraFile{extraType: extraType, dir: cmpDir(strings.Join(parts[:i], "/"))}, readAsExtra
	}
	dir, name := path.Dir(rel), strings.ToLower(stem(rel))
	if audio {
		if name == "theme" {
			return extraFile{extraType: library.ExtraThemeSong, dir: dir, named: true}, readAsExtra
		}
		return extraFile{}, leftOut
	}
	if !videoFile(path.Base(rel)) {
		return extraFile{}, readAsTitle
	}
	if extraType, ok := extraNames[name]; ok {
		return extraFile{extraType: extraType, dir: dir, named: true}, readAsExtra
	}
	for _, s := range extraSuffixes {
		if strings.HasSuffix(name, s.suffix) {
			base := stem(rel)
			base = strings.TrimRight(base[:len(base)-len(s.suffix)], " -._")
			return extraFile{extraType: s.extraType, dir: dir, base: base, named: true}, readAsExtra
		}
	}
	return extraFile{}, readAsTitle
}

// cmpDir is dir, "." for the folder's top.
func cmpDir(dir string) string {
	if dir == "" {
		return "."
	}
	return dir
}

// formerlyListed reports whether a walk of a folder of kind listed rel
// before scans read extras: none of formerlySkipped's folders, no sample,
// and no audio file outside a music folder.
func formerlyListed(kind, rel string) bool {
	parts := strings.Split(rel, "/")
	if slices.ContainsFunc(parts[:len(parts)-1], func(part string) bool { return formerlySkipped[strings.ToLower(part)] }) {
		return false
	}
	name := parts[len(parts)-1]
	return kind == KindMusic || !audioFile(name) && !(videoExtensions[strings.ToLower(path.Ext(name))] && sampleVideo(name))
}

// splitExtras takes the extras out of what a walk of a movies or shows
// folder found, and returns them, by path, with the files of extras
// folders that are no extras left out. A file linked by hand, a unit of its
// own among links, stays a title whatever its name: the link an
// administrator made wins. Without extras, a home videos folder's walk
// too, the files and side files of formerlySkipped's folders and the
// others it did not list are left out, as before scans read extras.
func splitExtras(kind string, extras bool, links map[string]string, list *listing) map[string]extraFile {
	if kind == KindMusic {
		return nil
	}
	if !extras || kind == KindHomeVideos {
		for _, files := range []map[string]found{list.files, list.sides} {
			for rel := range files {
				if !formerlyListed(kind, rel) {
					delete(files, rel)
				}
			}
		}
		return nil
	}
	taken := map[string]extraFile{}
	for rel := range list.files {
		e, how := readExtra(rel)
		if how == readAsTitle || links[rel] != "" {
			continue
		}
		if how == readAsExtra {
			e.file = list.files[rel]
			taken[rel] = e
		}
		delete(list.files, rel)
	}
	return taken
}

// extraRow is an extra a scan stores: its file, type, name, the units it
// belongs to, and a season's number.
type extraRow struct {
	path      string
	file      found
	extraType string
	name      string
	units     []string
	season    *int
}

// extraOwners finds the units each extra found belongs to, among the
// entries of a folder of kind, and leaves out the extras that belong to
// none. A movie's extras belong to the movies directly in their folder:
// all of them in a folder of their own, else those the name of an extra
// with a suffix begins with, in any case; at the folder's top, only such
// an extra belongs to a movie. A show's extras belong to the show of
// their folder, those of a season's folder to that season. lone lists
// the movies' extras named with a suffix that belong to none, which a
// scan lists as unmatched files (see reasonLoneExtra).
func extraOwners(kind string, extras map[string]extraFile, entries map[string]entry) (rows map[string]extraRow, lone []string) {
	rows = map[string]extraRow{}
	if len(extras) == 0 {
		return rows, nil
	}
	// The units directly in each folder, with their names: a movie's file
	// or disc, or a show's folder.
	type named struct{ unit, name string }
	units := map[string][]named{}
	seen := map[string]bool{}
	for rel, e := range entries {
		if e.file.extra || seen[e.unit] {
			continue
		}
		seen[e.unit] = true
		dir, name := path.Dir(rel), stem(rel)
		switch {
		case kind != KindMovies:
			dir, name = e.unit, e.unit
		case e.unit != rel:
			// A disc: its folder is the movie's own, and its name.
			if _, disc, ok := discEntry(rel); ok {
				dir, name = cmpDir(disc), path.Base(cmpDir(disc))
			}
		}
		units[dir] = append(units[dir], named{e.unit, name})
	}
	for rel, e := range extras {
		row := extraRow{path: rel, file: e.file, extraType: e.extraType}
		switch kind {
		case KindMovies:
			var all, prefixed []string
			for _, u := range units[e.dir] {
				all = append(all, u.unit)
				if e.base != "" && strings.EqualFold(u.name, e.base) {
					prefixed = append(prefixed, u.unit)
				}
			}
			switch {
			case len(prefixed) > 0:
				row.units = prefixed
			case e.dir != ".":
				row.units = all
			}
		case KindShows:
			parts := strings.Split(e.dir, "/")
			if e.dir == "." || len(parts) > 2 || len(units[parts[0]]) == 0 {
				continue
			}
			if len(parts) == 2 {
				season, ok := seasonOf(parts[1])
				if !ok {
					continue
				}
				row.season = &season
			}
			row.units = []string{parts[0]}
		}
		if len(row.units) == 0 {
			if kind == KindMovies && e.base != "" {
				lone = append(lone, rel)
			}
			continue
		}
		slices.Sort(row.units)
		rows[rel] = row
	}
	slices.Sort(lone)
	return rows, lone
}

// seasonOf is the season a season's folder names, false for another
// folder.
func seasonOf(folder string) (int, bool) {
	if m := seasonFolder.FindStringSubmatch(folder); m != nil {
		season, _ := strconv.Atoi(m[1])
		return season, true
	}
	if strings.EqualFold(folder, "specials") {
		return 0, true
	}
	return 0, false
}

// extraName is the name an extra shows: its file's without its extension
// or suffix, else its NFO file's <title>, else its file's as written.
func extraName(rel string, e extraFile, nfo nfoInfo) string {
	if !e.named {
		return stem(rel)
	}
	if e.base != "" {
		return e.base
	}
	if title := strings.TrimSpace(nfo.title); title != "" {
		return title
	}
	return stem(rel)
}

// extraNFO is the NFO file beside an extra that may give its name: the
// one of its name, in any case, for an extra whose file's name tells none
// past its suffix; empty when there is none.
func extraNFO(rel string, e extraFile, sides map[string]found) string {
	if !e.named || e.base != "" {
		return ""
	}
	want := strings.ToLower(strings.TrimSuffix(rel, path.Ext(rel)) + ".nfo")
	best := ""
	for side := range sides {
		if strings.ToLower(side) == want && (best == "" || side < best) {
			best = side
		}
	}
	return best
}

// extraPlace is the key an extra's NFO file has among the places of a
// scan's NFO files (see readNFOs), apart from the units'.
func extraPlace(rel string) string { return "\x00extra:" + rel }

// storeExtras stores the extras a scan found, those new or changed, and
// forgets those gone, but those in the folders the walk skipped. Without
// extras, every one of the folder's is forgotten.
func (s *Service) storeExtras(ctx context.Context, id accounts.ID, rows map[string]extraRow, skipped []string) error {
	type storedExtra struct {
		size      int64
		modified  time.Time
		extraType string
		name      string
		units     []string
		season    *int
	}
	result, err := s.db.Query(ctx, "SELECT path, size, modified, extra_type, name, units, season FROM local_extras WHERE addon_id = $1", id)
	if err != nil {
		return err
	}
	stored := map[string]storedExtra{}
	var rel string
	var row storedExtra
	if _, err := pgx.ForEachRow(result, []any{&rel, &row.size, &row.modified, &row.extraType, &row.name, &row.units, &row.season}, func() error {
		stored[rel] = row
		return nil
	}); err != nil {
		return err
	}
	batch := &pgx.Batch{}
	for _, rel := range slices.Sorted(maps.Keys(rows)) {
		e := rows[rel]
		old, ok := stored[rel]
		if ok && old.size == e.file.size && old.modified.Equal(e.file.modified) && old.extraType == e.extraType && old.name == e.name &&
			slices.Equal(old.units, e.units) && equalSeason(old.season, e.season) {
			continue
		}
		batch.Queue(`INSERT INTO local_extras (id, addon_id, path, size, modified, extra_type, name, units, season)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (addon_id, path) DO UPDATE SET size = $4, modified = $5, extra_type = $6, name = $7, units = $8, season = $9`,
			library.ExtraItemID(id, rel), id, rel, e.file.size, e.file.modified, e.extraType, e.name, e.units, e.season)
	}
	var gone []string
	for rel := range stored {
		if _, ok := rows[rel]; !ok && !under(rel, skipped) {
			gone = append(gone, rel)
		}
	}
	if len(gone) > 0 {
		batch.Queue("DELETE FROM local_extras WHERE addon_id = $1 AND path = ANY($2)", id, gone)
	}
	if batch.Len() == 0 {
		return nil
	}
	return s.db.SendBatch(ctx, batch).Close()
}

func equalSeason(a, b *int) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// folderKinds are the kinds of the folders whose extras belong to titles
// of a library kind.
var folderKinds = map[library.Kind]string{library.KindMovie: KindMovies, library.KindSeries: KindShows}

// Extras lists the extras of folders that belong to owner: a movie's or a
// series', or with a season, the season's (see library.Local).
func (s *Service) Extras(ctx context.Context, folders []accounts.ID, owner library.ExtraOwner) ([]library.Extra, error) {
	kind, ok := folderKinds[owner.Kind]
	if !ok || len(folders) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `SELECT e.id, e.addon_id, e.extra_type, e.name, e.season FROM local_extras e
		JOIN local_folders l ON l.addon_id = e.addon_id
		WHERE e.addon_id = ANY($1) AND l.kind = $2 AND e.season IS NOT DISTINCT FROM $4
			AND EXISTS (SELECT 1 FROM local_files f WHERE f.addon_id = e.addon_id AND f.unit = ANY(e.units) AND f.stremio_id = $3)
		ORDER BY e.name, e.path`, folders, kind, owner.StremioID, owner.Season)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (library.Extra, error) {
		extra := library.Extra{Kind: owner.Kind, Owners: []string{owner.StremioID}}
		err := row.Scan(&extra.ID, &extra.Folder, &extra.Type, &extra.Name, &extra.Season)
		return extra, err
	})
}

// Extra describes an extra by its item, with the titles it belongs to;
// library.ErrNotFound when no folder holds it.
func (s *Service) Extra(ctx context.Context, id accounts.ID) (library.Extra, error) {
	var extra library.Extra
	var kind string
	err := s.db.QueryRow(ctx, `SELECT e.addon_id, e.extra_type, e.name, e.season, l.kind,
			ARRAY(SELECT DISTINCT f.stremio_id FROM local_files f WHERE f.addon_id = e.addon_id AND f.unit = ANY(e.units)
				AND f.stremio_id IS NOT NULL ORDER BY 1)
		FROM local_extras e JOIN local_folders l ON l.addon_id = e.addon_id WHERE e.id = $1`, id).
		Scan(&extra.Folder, &extra.Type, &extra.Name, &extra.Season, &kind, &extra.Owners)
	if errors.Is(err, pgx.ErrNoRows) {
		return library.Extra{}, library.ErrNotFound
	}
	if err != nil {
		return library.Extra{}, err
	}
	extra.ID = id
	for libraryKind, folderKind := range folderKinds {
		if folderKind == kind {
			extra.Kind = libraryKind
		}
	}
	if extra.Kind == "" {
		return library.Extra{}, library.ErrNotFound
	}
	return extra, nil
}

// extraStreams answers the stream of an extra of a folder by its Stremio
// identifier (see library.ExtraStremioID): its file, with the subtitle
// files beside it.
func (s *Service) extraStreams(ctx context.Context, source accounts.ID, id, folderName string) ([]stremio.Stream, error) {
	item, err := accounts.ParseID(strings.TrimPrefix(id, library.ExtraIDPrefix))
	if err != nil {
		return nil, nil
	}
	var rel string
	var size int64
	err = s.db.QueryRow(ctx, "SELECT path, size FROM local_extras WHERE addon_id = $1 AND id = $2", source, item).Scan(&rel, &size)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	stream := stremio.Stream{Name: folderName, Description: formatSize(size), URL: s.files + item.String() + strings.ToLower(path.Ext(rel)),
		BehaviorHints: stremio.StreamBehavior{Filename: path.Base(rel), VideoSize: stremio.Number(size)}}
	subtitles, err := s.fileSubtitles(ctx, []accounts.ID{item})
	if err != nil {
		return nil, err
	}
	stream.Subtitles = subtitles[item]
	return []stremio.Stream{stream}, nil
}
