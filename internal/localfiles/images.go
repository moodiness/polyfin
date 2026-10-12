package localfiles

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/media"
)

// A disc kept as an image: an .iso file in a movies folder in the
// container, a DVD's or a Blu-ray disc's, its main title one movie
// matched by its file's name. A scan reads what each new or changed image
// is, and its titles, through its ISO 9660 file system, or its UDF one when
// it has no ISO 9660 one holding the disc's files (see openDiscFS), and
// keeps the answer by the image's size and modification time
// (local_disc_images):
//
//   - an image holding VIDEO_TS/VIDEO_TS.IFO is a DVD's, its titles read
//     from its IFO files as a DVD folder's are;
//   - one holding BDMV is a Blu-ray disc's, its titles its playlists, as a
//     disc folder's are (see discTitles), or libbluray's choice when its
//     playlists cannot be read;
//   - one whose UDF file system is laid out in a way Polyfin does not read,
//     as those of rewritable and write-once media are, is a Blu-ray disc's
//     when FFmpeg's bluray protocol reads it (see UseProbe), one movie,
//     libbluray's choice;
//   - any other is no disc's, and is left out.
//
// Each of an image's other movies is a title of its own, a unit linked by
// hand, kept as the image's path with its number (see discTitlePath).
// FFmpeg plays an image's titles from the image itself (see Input), and
// those of an image on a share from its files' bytes in the image, served
// as one file (see sharedisc.go). A show's image, in a shows folder, is its
// episodes, each kept as another movie is (see episodes.go); one whose
// titles cannot be read is left out.

// Kinds of disc images, as local_disc_images keeps them, and as Unmatched
// tells them.
const (
	imageDVD    = "dvd"
	imageBluRay = "bluray"

	DiscDVDImage    = "dvd_image"
	DiscBluRayImage = "bluray_image"
)

// imageFile reports whether name is a disc image a walk lists.
func imageFile(name string) bool {
	return strings.EqualFold(path.Ext(name), ".iso")
}

// imageKind is the kind of disc Unmatched tells an image of kind is.
func imageKind(kind string) string {
	switch kind {
	case imageDVD:
		return DiscDVDImage
	case imageBluRay:
		return DiscBluRayImage
	}
	return ""
}

// storedImage is what a scan found an image to be: its kind, empty for no
// disc's, its main title, -1 for libbluray's choice and 0 for FFmpeg's
// (see Input), and its other titles, with how long each plays, the main
// title's first, when the image's tables tell it. A show's image, in a
// shows folder, has no main title: title is its first episode, and titles
// its others (see chooseEpisodes).
type storedImage struct {
	file    found
	kind    string
	title   int
	titles  []int
	lengths []time.Duration
}

// entries are the entries of the titles of an image at rel found as f:
// its main title's, rel itself, and each other title's, marked as such;
// for a show's image, episodes set, each title's, rel and its number (see
// discTitlePath). Each has its title's length, when known.
func (image storedImage) entries(rel string, f found, episodes bool) map[string]found {
	length := func(i int) time.Duration {
		if i < len(image.lengths) {
			return image.lengths[i]
		}
		return 0
	}
	main := rel
	if episodes {
		main = discTitlePath(rel, image.title)
	}
	f.length = length(0)
	entries := map[string]found{main: f}
	for i, title := range image.titles {
		f.extra, f.length = !episodes, length(i+1)
		entries[discTitlePath(rel, title)] = f
	}
	return entries
}

// collapseImages keeps, of the files a walk of the folder at root found,
// the disc images of a movies folder, each with an entry per other movie
// it holds, and of a shows folder those of each show's image (see
// showDisc), an entry per episode (see storedImage.entries); it leaves out
// the images of no disc. Each image new or changed since the last scan is
// read again, and the answers kept. A share's images are read on the
// share, by ranges (see shareImage).
func (s *Service) collapseImages(ctx context.Context, id accounts.ID, folder Folder, root string, list listing) error {
	files := list.files
	episodes := folder.Kind == KindShows
	if episodes {
		for rel := range files {
			if imageFile(rel) && !showDisc(rel, true) {
				delete(files, rel)
			}
		}
	}
	rows, err := s.db.Query(ctx, "SELECT path, size, modified, kind, title, titles, lengths FROM local_disc_images WHERE addon_id = $1", id)
	if err != nil {
		return err
	}
	stored := map[string]storedImage{}
	for rows.Next() {
		var rel string
		var image storedImage
		var lengths []int
		if err := rows.Scan(&rel, &image.file.size, &image.file.modified, &image.kind, &image.title, &image.titles, &lengths); err != nil {
			rows.Close()
			return err
		}
		for _, seconds := range lengths {
			image.lengths = append(image.lengths, time.Duration(seconds)*time.Second)
		}
		stored[rel] = image
	}
	if err := rows.Err(); err != nil {
		return err
	}
	read := map[string]storedImage{}
	seen := map[string]storedImage{}
	for rel, f := range files {
		if !imageFile(rel) {
			continue
		}
		image, known := stored[rel]
		if !known || image.file.size != f.size || !image.file.modified.Equal(f.modified) {
			if folder.Share != "" {
				image = shareImage(ctx, list, rel, folder.Kind)
			} else {
				image = s.readImage(ctx, filepath.Join(root, filepath.FromSlash(rel)), folder.Kind)
			}
			image.file = f
			read[rel] = image
		}
		seen[rel] = image
	}
	for rel, image := range seen {
		f := files[rel]
		delete(files, rel)
		if image.kind != "" {
			maps.Copy(files, image.entries(rel, f, episodes))
		}
	}
	var forgotten []string
	for rel := range stored {
		if _, ok := seen[rel]; !ok && !under(rel, list.skipped) {
			forgotten = append(forgotten, rel)
		}
	}
	if len(read) == 0 && len(forgotten) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for rel, image := range read {
		titles := image.titles
		if titles == nil {
			titles = []int{}
		}
		lengths := make([]int, len(image.lengths))
		for i, length := range image.lengths {
			lengths[i] = int(length / time.Second)
		}
		batch.Queue(`INSERT INTO local_disc_images (addon_id, path, size, modified, kind, title, titles, lengths) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (addon_id, path) DO UPDATE SET size = $3, modified = $4, kind = $5, title = $6, titles = $7, lengths = $8`,
			id, rel, image.file.size, image.file.modified, image.kind, image.title, titles, lengths)
	}
	if len(forgotten) > 0 {
		batch.Queue("DELETE FROM local_disc_images WHERE addon_id = $1 AND path = ANY($2)", id, forgotten)
	}
	return s.db.SendBatch(ctx, batch).Close()
}

// UseProbe sets what tells whether FFmpeg reads an input, a Blu-ray image
// read with the bluray protocol (see playback.Service.ProbesInput); it is
// called before the service is used. Without it, the images whose UDF file
// system is laid out in a way Polyfin does not read are left out.
func (s *Service) UseProbe(probe func(ctx context.Context, in media.Input) bool) { s.probe = probe }

// readImage reads what the image at path is, for a folder of kind (see
// storedImage). An image whose UDF file system is laid out in a way
// Polyfin does not read (see errUnsupportedUDF), as those of rewritable
// and write-once media are, is, in a movies folder, a Blu-ray disc's when
// FFmpeg's bluray protocol reads it, one movie, libbluray choosing its
// title; a broken one is no disc's, and neither is, in a shows folder, an
// image whose episodes cannot be read.
func (s *Service) readImage(ctx context.Context, path, kind string) storedImage {
	file, err := os.Open(path)
	if err != nil {
		return storedImage{}
	}
	defer file.Close()
	image, err := readDisc(file, kind)
	if errors.Is(err, errUnsupportedUDF) && kind != KindShows && s.probe != nil &&
		s.probe(ctx, media.Input{Options: []string{"-playlist", "-1"}, Path: "bluray:" + path}) {
		return storedImage{kind: imageBluRay, title: -1}
	}
	return image
}

// shareImage reads what the image rel of a share is, for a folder of
// kind, through the walk's opener, by ranges (see readImage). FFmpeg reads
// a share's disc only as the titles Polyfin lays out: an image whose
// titles cannot be read, a Blu-ray image's playlists or a DVD image's IFO
// files, or whose UDF layout is not read, is no disc's here.
func shareImage(ctx context.Context, list listing, rel, kind string) storedImage {
	file, err := list.open(ctx, rel)
	if err != nil {
		return storedImage{}
	}
	defer file.Close()
	at, err := readerAt(file)
	if err != nil {
		return storedImage{}
	}
	image, _ := readDisc(at, kind)
	if image.kind == "" || image.title < 0 || image.kind == imageDVD && image.title == 0 {
		return storedImage{}
	}
	return image
}

// errNoEpisodes reports a show's disc image whose episodes cannot be read.
var errNoEpisodes = errors.New("the image's episodes cannot be read")

// readDisc reads what the image r is, for a folder of kind (see
// readImage), or why its file systems hold no disc Polyfin reads. A
// movies folder's image whose titles cannot be read is played as FFmpeg
// chooses; a shows folder's is left out.
func readDisc(r io.ReaderAt, kind string) (storedImage, error) {
	fsys, err := openDiscFS(r)
	if err != nil {
		return storedImage{}, err
	}
	if _, err := fsys.names("VIDEO_TS"); err == nil {
		titles, err := dvdTitlesOf(func(name string) (io.ReaderAt, error) { return fsys.file("VIDEO_TS", name) }, titleChooser(kind))
		switch {
		case kind == KindShows && len(titles) == 0:
			return storedImage{}, errNoEpisodes
		case err != nil:
			// FFmpeg plays the DVD's first title.
			return storedImage{kind: imageDVD}, nil
		}
		image := storedImage{kind: imageDVD, title: titles[0].title}
		for i, t := range titles {
			if i > 0 {
				image.titles = append(image.titles, t.title)
			}
			image.lengths = append(image.lengths, t.length)
		}
		return image, nil
	}
	if _, err = fsys.names("BDMV"); err == nil {
		titles, err := imagePlaylists(fsys, titleChooser(kind))
		switch {
		case err != nil && kind == KindShows:
			return storedImage{}, errNoEpisodes
		case err != nil:
			// libbluray chooses the disc's main title.
			return storedImage{kind: imageBluRay, title: -1}, nil
		}
		image := storedImage{kind: imageBluRay, title: titles[0].number}
		for i, t := range titles {
			if i > 0 {
				image.titles = append(image.titles, t.number)
			}
			image.lengths = append(image.lengths, t.length)
		}
		return image, nil
	}
	return storedImage{}, err
}

// maxImagePlaylists bounds the bytes of playlists read in an image, those
// of a disc hiding its movie among hundreds of playlists included: a few
// KB each as a rule.
const maxImagePlaylists = 16 << 20

// imagePlaylists are the titles of a Blu-ray image, its playlists chosen
// by choose (see discTitles), the main title first; an error when none can
// be read.
func imagePlaylists(fsys discFS, choose chooser) ([]playlist, error) {
	streams, err := fsys.names("BDMV", "STREAM")
	if err != nil {
		return nil, err
	}
	clips := map[string]bool{}
	for _, name := range streams {
		clips[strings.ToUpper(strings.TrimSuffix(name, path.Ext(name)))] = true
	}
	names, err := fsys.names("BDMV", "PLAYLIST")
	if err != nil {
		return nil, err
	}
	var playlists []playlist
	budget := int64(maxImagePlaylists)
	for _, name := range names {
		_, number, ok := discPlaylist("BDMV/PLAYLIST/" + name)
		if !ok {
			continue
		}
		file, err := fsys.file("BDMV", "PLAYLIST", name)
		if err != nil || file.Size() > maxPlaylistSize {
			continue
		}
		if budget -= file.Size(); budget < 0 {
			return nil, errors.New("the image's playlists are too large")
		}
		p, err := readPlaylist(file)
		if err != nil {
			continue
		}
		missing := false
		for _, part := range p.clips {
			missing = missing || !clips[part.clip]
		}
		if missing {
			continue
		}
		p.number = number
		playlists = append(playlists, p)
	}
	titles := discTitles(playlists, choose)
	if len(titles) == 0 {
		return nil, errors.New("no playlist of the image can be read")
	}
	return titles, nil
}

// discFS is a disc image's file system, ISO 9660 or UDF: enough of it to
// list a folder and read a file, by their path, any case.
type discFS interface {
	// names lists the names of the folder at path's entries, the folder
	// itself and its parent left out.
	names(path ...string) ([]string, error)
	// file opens the file at path.
	file(path ...string) (*io.SectionReader, error)
}

// errNotDisc reports an image whose file systems hold no disc.
var errNotDisc = errors.New("no disc's file system")

// openDiscFS opens the file system of the image r holding its disc's
// files: its ISO 9660 one when it holds VIDEO_TS or BDMV, as DVD images
// do and many Blu-ray images too, else its UDF one, which Blu-ray images
// often have alone.
func openDiscFS(r io.ReaderAt) (discFS, error) {
	if img, err := openISO(r); err == nil {
		for _, folder := range []string{"VIDEO_TS", "BDMV"} {
			if _, err := img.names(folder); err == nil {
				return img, nil
			}
		}
	}
	if !udfVolume(r) {
		return nil, errNotDisc
	}
	return openUDF(r)
}

// readDiscFile reads a file of a disc image's file system at path, at
// most limit bytes.
func readDiscFile(fsys discFS, limit int64, path ...string) ([]byte, error) {
	file, err := fsys.file(path...)
	if err != nil {
		return nil, err
	}
	if file.Size() > limit {
		return nil, fmt.Errorf("%s is too large", strings.Join(path, "/"))
	}
	return io.ReadAll(file)
}

// udfVolume reports whether the image r has a UDF file system: an NSR
// descriptor in its volume recognition sequence, from sector 16.
func udfVolume(r io.ReaderAt) bool {
	sector := make([]byte, isoSector)
	for n := int64(16); n < 32; n++ {
		if _, err := r.ReadAt(sector, n*isoSector); err != nil {
			return false
		}
		switch id := sector[1:6]; {
		case bytes.Equal(id, []byte("NSR02")), bytes.Equal(id, []byte("NSR03")):
			return true
		case bytes.Equal(id, []byte("TEA01")):
			return false
		}
	}
	return false
}
