package localfiles

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

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
// FFmpeg plays an image's titles from the image itself (see Input). A
// shows folder leaves images out, and so do shares, FFmpeg reading an
// image from a path.

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
// (see Input), and its other movies' titles.
type storedImage struct {
	file   found
	kind   string
	title  int
	titles []int
}

// collapseImages keeps, of the files a walk of the folder at root found,
// the disc images of a movies folder in the container, each with an entry
// per other movie it holds, and none of a shows folder's or a share's; it
// leaves out the images of no disc. Each image new or changed since the
// last scan is read again, and the answers kept.
func (s *Service) collapseImages(ctx context.Context, id accounts.ID, folder Folder, root string, list listing) error {
	files := list.files
	if folder.Kind == KindShows || folder.Share != "" {
		for rel := range files {
			if imageFile(rel) {
				delete(files, rel)
			}
		}
		return nil
	}
	rows, err := s.db.Query(ctx, "SELECT path, size, modified, kind, title, titles FROM local_disc_images WHERE addon_id = $1", id)
	if err != nil {
		return err
	}
	stored := map[string]storedImage{}
	for rows.Next() {
		var rel string
		var image storedImage
		if err := rows.Scan(&rel, &image.file.size, &image.file.modified, &image.kind, &image.title, &image.titles); err != nil {
			rows.Close()
			return err
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
			image = storedImage{file: f}
			image.kind, image.title, image.titles = s.readImage(ctx, filepath.Join(root, filepath.FromSlash(rel)))
			read[rel] = image
		}
		seen[rel] = image
	}
	for rel, image := range seen {
		if image.kind == "" {
			delete(files, rel)
			continue
		}
		for _, title := range image.titles {
			f := files[rel]
			f.extra = true
			files[discTitlePath(rel, title)] = f
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
		batch.Queue(`INSERT INTO local_disc_images (addon_id, path, size, modified, kind, title, titles) VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (addon_id, path) DO UPDATE SET size = $3, modified = $4, kind = $5, title = $6, titles = $7`,
			id, rel, image.file.size, image.file.modified, image.kind, image.title, titles)
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

// readImage reads what the image at path is: its kind, empty for no
// disc's, its main title, and its other movies' titles (see storedImage).
// An image whose UDF file system is laid out in a way Polyfin does not
// read (see errUnsupportedUDF), as those of rewritable and write-once
// media are, is a Blu-ray disc's when FFmpeg's bluray protocol reads it,
// one movie, libbluray choosing its title; a broken one is no disc's.
func (s *Service) readImage(ctx context.Context, path string) (kind string, title int, others []int) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, nil
	}
	defer file.Close()
	kind, title, others, err = readDisc(file)
	if errors.Is(err, errUnsupportedUDF) && s.probe != nil && s.probe(ctx, media.Input{Options: []string{"-playlist", "-1"}, Path: "bluray:" + path}) {
		return imageBluRay, -1, nil
	}
	return kind, title, others
}

// readDisc reads what the image r is (see readImage), or why its file
// systems hold no disc Polyfin reads.
func readDisc(r io.ReaderAt) (kind string, title int, others []int, err error) {
	fsys, err := openDiscFS(r)
	if err != nil {
		return "", 0, nil, err
	}
	if _, err := fsys.names("VIDEO_TS"); err == nil {
		titles, err := dvdTitlesOf(func(name string) (io.ReaderAt, error) { return fsys.file("VIDEO_TS", name) })
		if err != nil {
			// FFmpeg plays the DVD's first title.
			return imageDVD, 0, nil, nil
		}
		for _, t := range titles[1:] {
			others = append(others, t.title)
		}
		return imageDVD, titles[0].title, others, nil
	}
	if _, err = fsys.names("BDMV"); err == nil {
		titles, err := imagePlaylists(fsys)
		if err != nil {
			// libbluray chooses the disc's main title.
			return imageBluRay, -1, nil, nil
		}
		return imageBluRay, titles[0], titles[1:], nil
	}
	return "", 0, nil, err
}

// maxImagePlaylists bounds the bytes of playlists read in an image, those
// of a disc hiding its movie among hundreds of playlists included: a few
// KB each as a rule.
const maxImagePlaylists = 16 << 20

// imagePlaylists are the titles of a Blu-ray image, its playlists' numbers
// (see discTitles), the main title first; an error when none can be read.
func imagePlaylists(fsys discFS) ([]int, error) {
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
	titles := discTitles(playlists)
	if len(titles) == 0 {
		return nil, errors.New("no playlist of the image can be read")
	}
	numbers := make([]int, len(titles))
	for i, t := range titles {
		numbers[i] = t.number
	}
	return numbers, nil
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
