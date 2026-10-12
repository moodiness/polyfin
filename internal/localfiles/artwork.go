package localfiles

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// Artwork beside the files of movies and shows folders, read as Jellyfin
// reads it: images (see imageExtensions) named after a title's file or
// folder, as in "Movie (2001)-poster.jpg", or, in a folder of the title's
// own, with the folder's names, as in "poster.jpg" (see titleImages).
// Names are compared in any letter case. A movie is alone in its folder
// when every video directly in it is matched to the same title, its
// versions included; a disc folder is its disc's own. A show's images are
// in its folder, and a season's in its show's folder ("season01-poster")
// or in its season folder (see seasonImages).
//
// A scan chooses the images of each unit, a movie's file or disc or a
// show's folder, and of its seasons, one per image type, and keeps them
// with their size and modification time (see storeArtwork). Artwork gives
// them to the library as the artwork of the titles the units are matched
// to, which it shows in place of the addons' of the same type (see
// library.Local), unless the folder's UseArtwork is off. When several
// folders hold an image of the same type for a title, the folder listed
// first among the server's sources gives it; in one folder, the first of
// the title's units by path. Images are read from the folder or the share
// by Open, by URLs of their own that change with their size and
// modification time, so that apps see their tag change (see artworkURL).

// Jellyfin's image types a title's local artwork may be of.
const (
	imagePrimary  = "Primary"
	imageBackdrop = "Backdrop"
	imageLogo     = "Logo"
	imageThumb    = "Thumb"
	imageBanner   = "Banner"
	imageArt      = "Art"
	imageDisc     = "Disc"
)

// The names of the images that may be a title's Primary image, in the
// order they are preferred: a movie's, a show's and a season's.
var (
	moviePosterNames  = []string{"poster", "folder", "cover", "default", "movie"}
	showPosterNames   = []string{"poster", "folder", "cover", "default", "show"}
	seasonPosterNames = []string{"poster", "folder", "cover", "default"}
)

// artworkNames are the names of the images of the other types than
// Primary and Backdrop, in the order they are preferred. Only movies have
// a Disc image.
var artworkNames = []struct {
	imageType string
	names     []string
}{
	{imageLogo, []string{"logo", "clearlogo"}},
	{imageArt, []string{"clearart"}},
	{imageDisc, []string{"disc", "cdart", "discart"}},
	{imageBanner, []string{"banner"}},
	{imageThumb, []string{"landscape", "thumb"}},
}

// backdropNames are the names of the images that may be a title's
// backdrop, in the order they are preferred: each name, then numbered from
// 1 to maxBackdropNumber after next, as in "fanart-1" or "backdrop2".
var backdropNames = []struct{ first, next string }{
	{"fanart", "fanart-"}, {"background", "background-"}, {"art", "art-"}, {"backdrop", "backdrop"},
}

const maxBackdropNumber = 20

// sideImages are the images a walk listed, by their path in lower case.
// Empty files are left out, as Jellyfin leaves them out.
type sideImages map[string]string

// listImages finds the images among a listing's side files.
func listImages(sides map[string]found) sideImages {
	images := sideImages{}
	for rel, f := range sides {
		if !imageSide(rel) || f.size == 0 {
			continue
		}
		key := strings.ToLower(rel)
		if other, ok := images[key]; !ok || rel < other {
			images[key] = rel
		}
	}
	return images
}

// find returns the image of name in dir, of the first of imageExtensions
// found, in any letter case.
func (images sideImages) find(dir, name string) (string, bool) {
	for _, extension := range imageExtensions {
		if rel, ok := images[strings.ToLower(path.Join(dir, name+extension))]; ok {
			return rel, true
		}
	}
	return "", false
}

// place is where a title's images are: dir, the folder; stem, the name of
// its file without extension or of its folder, which images named after
// the title begin with ("<stem>-poster"); and whether the folder's own
// names count (whole), when the title is alone in it.
type place struct {
	dir, stem string
	whole     bool
}

// names lists name after the title's, then alone when the folder's own
// names count.
func (p place) names(name string) []string {
	var names []string
	if p.stem != "" {
		names = append(names, p.stem+"-"+name)
	}
	if p.whole {
		names = append(names, name)
	}
	return names
}

// titleImages chooses the images of a title at p, by image type, in the
// order Jellyfin prefers them: for Primary, the image named as the title's
// file or folder ("Movie (2001).jpg"), then each of primary after the
// title's name, then alone; for the other types, each name after the
// title's name, then alone. disc tells a movie, which may have a Disc
// image.
func (images sideImages) titleImages(p place, primary []string, disc bool) map[string]string {
	chosen := map[string]string{}
	first := func(imageType string, names []string) {
		for _, name := range names {
			if rel, ok := images.find(p.dir, name); ok {
				chosen[imageType] = rel
				return
			}
		}
	}
	var names []string
	if p.stem != "" {
		names = append(names, p.stem)
		for _, name := range primary {
			names = append(names, p.stem+"-"+name)
		}
	}
	if p.whole {
		names = append(names, primary...)
	}
	first(imagePrimary, names)
	for _, kind := range artworkNames {
		if kind.imageType == imageDisc && !disc {
			continue
		}
		names = nil
		for _, name := range kind.names {
			names = append(names, p.names(name)...)
		}
		first(kind.imageType, names)
	}
	names = nil
	for _, backdrop := range backdropNames {
		numbered := func(prefix string) {
			names = append(names, prefix+backdrop.first)
			for i := 1; i <= maxBackdropNumber; i++ {
				names = append(names, prefix+backdrop.next+strconv.Itoa(i))
			}
		}
		if p.stem != "" {
			numbered(p.stem + "-")
		}
		if p.whole {
			numbered("")
		}
	}
	first(imageBackdrop, names)
	return chosen
}

// seasonArtwork are the images of a season in its show's folder, by the
// name after the season's (see seasonNames).
var seasonArtwork = []struct{ imageType, name string }{
	{imagePrimary, "poster"}, {imageBackdrop, "fanart"}, {imageBanner, "banner"}, {imageThumb, "landscape"},
}

// seasonImage matches the name of an image of a season in its show's
// folder, without its extension, in lower case.
var seasonImage = regexp.MustCompile(`^(?:season(\d{1,4})|season-specials|specials)-(?:poster|fanart|banner|landscape)$`)

// seasonNames are the names the images of season in its show's folder
// begin with, in the order they are preferred: its name without spaces
// ("season1", "specials"), then "season01" or "season-specials".
func seasonNames(season int) []string {
	if season == 0 {
		return []string{"specials", "season-specials"}
	}
	names := []string{"season" + strconv.Itoa(season)}
	if padded := fmt.Sprintf("season%02d", season); padded != names[0] {
		names = append(names, padded)
	}
	return names
}

// seasonImages chooses the images of a season of the show whose folder is
// show, by image type: those in the show's folder first, then those of its
// season folder, when it has one.
func (images sideImages) seasonImages(show string, season int, folder *place) map[string]string {
	chosen := map[string]string{}
	for _, artwork := range seasonArtwork {
		for _, name := range seasonNames(season) {
			if rel, ok := images.find(show, name+"-"+artwork.name); ok {
				chosen[artwork.imageType] = rel
				break
			}
		}
	}
	if folder != nil {
		for imageType, rel := range images.titleImages(*folder, seasonPosterNames, false) {
			if _, ok := chosen[imageType]; !ok {
				chosen[imageType] = rel
			}
		}
	}
	return chosen
}

// imagedFile is a file of a folder as its scan stored it: its path, unit,
// the Stremio identifier of its title, empty when unmatched, and an
// episode's season.
type imagedFile struct {
	path, unit, title string
	season            *int
}

// artworkKey is an image of a unit, of one of its seasons for a show's,
// -1 for the unit itself, and of an image type.
type artworkKey struct {
	unit      string
	season    int
	imageType string
}

// chooseArtwork chooses the images of the units and seasons of a folder
// of kind, whose name is root (see locationName), among its images, those
// of units left unmatched included: a link makes them a title's.
func chooseArtwork(kind, root string, files []imagedFile, images sideImages) map[artworkKey]string {
	chosen := map[artworkKey]string{}
	keep := func(unit string, season int, found map[string]string) {
		for imageType, rel := range found {
			chosen[artworkKey{unit, season, imageType}] = rel
		}
	}
	if kind == KindShows {
		chooseShowArtwork(files, images, keep)
		return chosen
	}
	// The titles of the videos directly in each folder, an unmatched one
	// counting as a title of its own.
	titles := map[string]map[string]bool{}
	for _, f := range files {
		if f.unit != f.path {
			continue
		}
		dir, title := path.Dir(f.path), f.title
		if title == "" {
			title = "\x00" + f.path
		}
		if titles[dir] == nil {
			titles[dir] = map[string]bool{}
		}
		titles[dir][title] = true
	}
	done := map[string]bool{}
	for _, f := range files {
		if done[f.unit] {
			continue
		}
		var p place
		switch _, disc, ok := discEntry(f.path); {
		case f.unit == f.path:
			dir := path.Dir(f.path)
			// A home video at the folder's top is not the folder's own.
			p = place{dir: dir, stem: stem(f.path), whole: len(titles[dir]) == 1 && (kind != KindHomeVideos || dir != ".")}
		case ok && discUnit(disc) == f.unit:
			p = place{dir: f.unit, stem: path.Base(f.unit), whole: true}
			if f.unit == "." {
				p.stem = root
			}
		default:
			// A disc's title other than its main one has no artwork.
			continue
		}
		done[f.unit] = true
		keep(f.unit, -1, images.titleImages(p, moviePosterNames, true))
	}
	return chosen
}

// chooseShowArtwork chooses the images of a shows folder's shows and their
// seasons (see chooseArtwork). A show's files at the folder's top, without
// a folder of their own, have none.
func chooseShowArtwork(files []imagedFile, images sideImages, keep func(unit string, season int, found map[string]string)) {
	type show struct {
		// seasons are the seasons found, and folders the season folders,
		// the folders directly in the show's holding episodes of one
		// season, -1 for several.
		seasons map[int]bool
		folders map[string]int
	}
	shows := map[string]*show{}
	for _, f := range files {
		if f.unit == f.path {
			continue
		}
		s := shows[f.unit]
		if s == nil {
			s = &show{seasons: map[int]bool{}, folders: map[string]int{}}
			shows[f.unit] = s
		}
		if f.season == nil {
			continue
		}
		s.seasons[*f.season] = true
		if dir := path.Dir(f.path); dir != f.unit && path.Dir(dir) == f.unit {
			if season, ok := s.folders[dir]; ok && season != *f.season {
				s.folders[dir] = -1
			} else {
				s.folders[dir] = *f.season
			}
		}
	}
	// The seasons the images in each show's folder are of count with those
	// found.
	for key, rel := range images {
		s := shows[path.Dir(rel)]
		if s == nil {
			continue
		}
		if m := seasonImage.FindStringSubmatch(stem(key)); m != nil {
			season := 0
			if m[1] != "" {
				season, _ = strconv.Atoi(m[1])
			}
			s.seasons[season] = true
		}
	}
	for unit, s := range shows {
		keep(unit, -1, images.titleImages(place{dir: unit, stem: path.Base(unit), whole: true}, showPosterNames, false))
		folders := map[int][]string{}
		for dir, season := range s.folders {
			if season >= 0 {
				folders[season] = append(folders[season], dir)
			}
		}
		for season := range s.seasons {
			var folder *place
			if dirs := folders[season]; len(dirs) > 0 {
				dir := slices.Min(dirs)
				folder = &place{dir: dir, stem: path.Base(dir), whole: true}
			}
			keep(unit, season, images.seasonImages(unit, season, folder))
		}
	}
}

// storedArtwork is an image local_artwork keeps.
type storedArtwork struct {
	id   accounts.ID
	path string
	file found
}

// storeArtwork stores the images of the units and seasons of a movies or
// shows folder, as its files are stored now and its listing found them
// (see chooseArtwork): those new or changed, by path, size and
// modification time, and forgets those gone, but those in the folders the
// walk skipped. root is the folder's path or address.
func (s *Service) storeArtwork(ctx context.Context, id accounts.ID, kind, root string, list listing) error {
	rows, err := s.db.Query(ctx, "SELECT path, unit, coalesce(stremio_id, ''), season FROM local_files WHERE addon_id = $1", id)
	if err != nil {
		return err
	}
	files, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (imagedFile, error) {
		var f imagedFile
		err := row.Scan(&f.path, &f.unit, &f.title, &f.season)
		return f, err
	})
	if err != nil {
		return err
	}
	desired := chooseArtwork(kind, locationName(root), files, listImages(list.sides))
	rows, err = s.db.Query(ctx, "SELECT id, unit, coalesce(season, -1), image_type, path, size, modified FROM local_artwork WHERE addon_id = $1",
		id)
	if err != nil {
		return err
	}
	stored := map[artworkKey]storedArtwork{}
	var key artworkKey
	var row storedArtwork
	if _, err := pgx.ForEachRow(rows, []any{&row.id, &key.unit, &key.season, &key.imageType, &row.path, &row.file.size, &row.file.modified},
		func() error {
			stored[key] = row
			return nil
		}); err != nil {
		return err
	}
	batch := &pgx.Batch{}
	for key, rel := range desired {
		file := list.sides[rel]
		if old, ok := stored[key]; ok && old.path == rel && old.file.size == file.size && old.file.modified.Equal(file.modified) {
			continue
		}
		var season *int
		if key.season >= 0 {
			season = &key.season
		}
		// Another file is another image: its identifier changes too.
		batch.Queue(`INSERT INTO local_artwork (addon_id, unit, season, image_type, path, size, modified)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (addon_id, unit, season, image_type) DO UPDATE SET path = $5, size = $6, modified = $7,
				id = CASE WHEN local_artwork.path = $5 THEN local_artwork.id ELSE gen_random_uuid() END`,
			id, key.unit, season, key.imageType, rel, file.size, file.modified)
	}
	var gone []accounts.ID
	for key, old := range stored {
		if _, ok := desired[key]; !ok && !under(old.path, list.skipped) {
			gone = append(gone, old.id)
		}
	}
	if len(gone) > 0 {
		batch.Queue("DELETE FROM local_artwork WHERE id = ANY($1)", gone)
	}
	// The scan may have matched units too: what Artwork answers is read
	// again either way.
	defer s.artworkChanged()
	if batch.Len() == 0 {
		return nil
	}
	return s.db.SendBatch(ctx, batch).Close()
}

// artworkChanged counts a change to the artwork kept, to the titles of
// its units or to a folder's UseArtwork: Artwork reads them again.
func (s *Service) artworkChanged() {
	s.artworkChanges.Add(1)
}

// artworkSet is the artwork Artwork answers, as it was read once the
// folders' artwork and the server's addons had changed the counted times.
type artworkSet struct {
	changes, addons uint64
	images          map[accounts.ID]library.Images
}

// Artwork returns the artwork of the titles and seasons of the server's
// enabled movies and shows folders whose UseArtwork is on, and of the
// videos of its home videos folders, by item (see library.TitleItemID and
// library.SeasonItemID):
// images Open opens, by URLs that change with their size and modification
// time (see artworkURL). Of the images of a type several folders hold for
// an item, the folder listed first among the server's sources gives it. A
// home video without a poster beside it shows its frame (see storeFrames).
// It is read from memory, again once the artwork or the addons changed; if
// it cannot be read, titles show the addons' artwork until it can.
func (s *Service) Artwork() map[accounts.ID]library.Images {
	if set := s.artwork.Load(); set != nil && set.changes == s.artworkChanges.Load() && set.addons == s.addons.Changes() {
		return set.images
	}
	s.artworkMu.Lock()
	defer s.artworkMu.Unlock()
	// The counts are read before the database: a change stored meanwhile
	// reads it again.
	changes, addonChanges := s.artworkChanges.Load(), s.addons.Changes()
	if set := s.artwork.Load(); set != nil && set.changes == changes && set.addons == addonChanges {
		return set.images
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	images, err := s.readArtwork(ctx)
	if err != nil {
		s.logger.Error("The artwork of local folders could not be read", "error", err)
		return nil
	}
	s.artwork.Store(&artworkSet{changes: changes, addons: addonChanges, images: images})
	return images
}

// readArtwork reads the artwork Artwork answers. A home videos folder has
// no UseArtwork: no addon gives its titles artwork.
func (s *Service) readArtwork(ctx context.Context) (map[accounts.ID]library.Images, error) {
	images := map[accounts.ID]library.Images{}
	// The title of an image's unit is that of its files matched: a show's
	// files at its top whose name tells no episode are not.
	rows, err := s.db.Query(ctx, `SELECT a.id, l.kind, f.stremio_id, coalesce(a.season, -1), a.image_type, a.path, a.size, a.modified
		FROM local_artwork a JOIN local_folders l ON l.addon_id = a.addon_id JOIN addons d ON d.id = a.addon_id
		CROSS JOIN LATERAL (SELECT stremio_id FROM local_files WHERE addon_id = a.addon_id AND unit = a.unit AND stremio_id IS NOT NULL
			LIMIT 1) f
		WHERE (l.use_artwork AND l.kind IN ('movies', 'shows') OR l.kind = 'homevideos') AND d.enabled
		ORDER BY d.position, d.created_at, d.id, a.unit`)
	if err != nil {
		return nil, err
	}
	var image accounts.ID
	var kind, title, imageType, rel string
	var season int
	var size int64
	var modified time.Time
	_, err = pgx.ForEachRow(rows, []any{&image, &kind, &title, &season, &imageType, &rel, &size, &modified}, func() error {
		item := library.TitleItemID(library.KindMovie, title)
		switch {
		case kind == KindHomeVideos:
			item = library.TitleItemID(library.KindVideo, title)
		case kind == KindShows && season >= 0:
			item = library.SeasonItemID(title, season)
		case kind == KindShows:
			item = library.TitleItemID(library.KindSeries, title)
		}
		set := images[item]
		if !setArtwork(&set, imageType, artworkURL(image, rel, size, modified)) {
			return nil
		}
		images[item] = set
		return nil
	})
	if err != nil {
		return nil, err
	}
	rows, err = s.db.Query(ctx, `SELECT v.frame_id, f.stremio_id, length(v.frame), v.modified FROM local_videos v
		JOIN local_files f ON f.addon_id = v.addon_id AND f.path = v.path JOIN addons d ON d.id = v.addon_id
		WHERE v.frame IS NOT NULL AND f.stremio_id IS NOT NULL AND d.enabled`)
	if err != nil {
		return nil, err
	}
	_, err = pgx.ForEachRow(rows, []any{&image, &title, &size, &modified}, func() error {
		item := library.TitleItemID(library.KindVideo, title)
		set := images[item]
		if setArtwork(&set, imagePrimary, artworkURL(image, "frame.jpg", size, modified)) {
			images[item] = set
		}
		return nil
	})
	return images, err
}

// setArtwork sets the image of imageType in images to url, unless it has
// one already; it reports whether it set it.
func setArtwork(images *library.Images, imageType, url string) bool {
	var field *string
	switch imageType {
	case imagePrimary:
		field = &images.Primary
	case imageBackdrop:
		field = &images.Backdrop
	case imageLogo:
		field = &images.Logo
	case imageThumb:
		field = &images.Thumb
	case imageBanner:
		field = &images.Banner
	case imageArt:
		field = &images.Art
	case imageDisc:
		field = &images.Disc
	default:
		return false
	}
	if *field != "" {
		return false
	}
	*field = url
	return true
}

// artworkURL stands for an image of local_artwork in the library's Images
// (see library.LocalArtworkURL): its key, which Open opens, and its
// version, which tells its size and modification time.
func artworkURL(image accounts.ID, rel string, size int64, modified time.Time) string {
	return library.LocalArtworkURL(image.String()+strings.ToLower(path.Ext(rel)),
		strconv.FormatInt(size, 36)+"."+strconv.FormatInt(modified.UnixMicro(), 36))
}
