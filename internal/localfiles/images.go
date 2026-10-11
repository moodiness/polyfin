package localfiles

import (
	"bytes"
	"context"
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
// container, a DVD's or a Blu-ray disc's, one movie matched by its file's
// name. A scan reads what each new or changed image is, and its main
// title, and keeps the answer by the image's size and modification time
// (local_disc_images):
//
//   - an image whose ISO 9660 file system holds VIDEO_TS/VIDEO_TS.IFO is a
//     DVD's, its main title read from its IFO files as a DVD folder's is;
//   - one whose ISO 9660 file system holds BDMV is a Blu-ray disc's, its
//     main title its longest playlist (see discTitles), or libbluray's
//     choice when its playlists cannot be read there;
//   - one with a UDF file system only, as Blu-ray images often are, is a
//     Blu-ray disc's when FFmpeg's bluray protocol reads it (see
//     UseProbe), libbluray choosing its main title;
//   - any other is no disc's, and is left out.
//
// FFmpeg plays an image's main title from the image itself (see Input). A
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

// UseProbe sets what tells whether FFmpeg reads an input, a Blu-ray image
// read with the bluray protocol (see playback.Service.ProbesInput); it is
// called before the service is used. Without it, images with a UDF file
// system only are left out.
func (s *Service) UseProbe(probe func(ctx context.Context, in media.Input) bool) { s.probe = probe }

// storedImage is what a scan found an image to be: its kind, empty for no
// disc's, and its main title, -1 for libbluray's choice (see Input).
type storedImage struct {
	file  found
	kind  string
	title int
}

// collapseImages keeps, of the files a walk of the folder at root found,
// the disc images of a movies folder in the container, and none of a
// shows folder's or a share's; it leaves out the images of no disc. Each
// image new or changed since the last scan is read again, and the answers
// kept.
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
	rows, err := s.db.Query(ctx, "SELECT path, size, modified, kind, title FROM local_disc_images WHERE addon_id = $1", id)
	if err != nil {
		return err
	}
	stored := map[string]storedImage{}
	for rows.Next() {
		var rel string
		var image storedImage
		if err := rows.Scan(&rel, &image.file.size, &image.file.modified, &image.kind, &image.title); err != nil {
			rows.Close()
			return err
		}
		stored[rel] = image
	}
	if err := rows.Err(); err != nil {
		return err
	}
	read := map[string]storedImage{}
	seen := map[string]bool{}
	for rel, f := range files {
		if !imageFile(rel) {
			continue
		}
		seen[rel] = true
		image, known := stored[rel]
		if !known || image.file.size != f.size || !image.file.modified.Equal(f.modified) {
			image = storedImage{file: f}
			image.kind, image.title = s.readImage(ctx, filepath.Join(root, filepath.FromSlash(rel)))
			read[rel] = image
		}
		if image.kind == "" {
			delete(files, rel)
		}
	}
	var forgotten []string
	for rel := range stored {
		if !seen[rel] && !under(rel, list.skipped) {
			forgotten = append(forgotten, rel)
		}
	}
	if len(read) == 0 && len(forgotten) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for rel, image := range read {
		batch.Queue(`INSERT INTO local_disc_images (addon_id, path, size, modified, kind, title) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (addon_id, path) DO UPDATE SET size = $3, modified = $4, kind = $5, title = $6`,
			id, rel, image.file.size, image.file.modified, image.kind, image.title)
	}
	if len(forgotten) > 0 {
		batch.Queue("DELETE FROM local_disc_images WHERE addon_id = $1 AND path = ANY($2)", id, forgotten)
	}
	return s.db.SendBatch(ctx, batch).Close()
}

// readImage reads what the image at path is: its kind, empty for no
// disc's, and its main title (see storedImage).
func (s *Service) readImage(ctx context.Context, path string) (kind string, title int) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0
	}
	defer file.Close()
	if img, err := openISO(file); err == nil {
		if dir, err := img.lookup("VIDEO_TS"); err == nil && dir.dir {
			title, _, err := dvdMainOf(func(name string) (io.ReaderAt, error) {
				entry, err := img.lookup("VIDEO_TS", name)
				if err != nil {
					return nil, err
				}
				return img.reader(entry), nil
			})
			if err != nil {
				// FFmpeg plays the DVD's first title.
				return imageDVD, 0
			}
			return imageDVD, title
		}
		if dir, err := img.lookup("BDMV"); err == nil && dir.dir {
			return imageBluRay, imagePlaylist(img)
		}
	}
	if !udfVolume(file) || s.probe == nil {
		return "", 0
	}
	if !s.probe(ctx, media.Input{Options: []string{"-playlist", "-1"}, Path: "bluray:" + path}) {
		return "", 0
	}
	return imageBluRay, -1
}

// imagePlaylist is the main playlist of a Blu-ray image, read through its
// ISO 9660 file system (see discTitles), -1 for libbluray's choice when its
// playlists cannot be read there.
func imagePlaylist(img *isoImage) int {
	streams, err := img.lookup("BDMV", "STREAM")
	if err != nil {
		return -1
	}
	clips := map[string]bool{}
	entries, err := img.list(streams)
	if err != nil {
		return -1
	}
	for _, e := range entries {
		clips[strings.ToUpper(strings.TrimSuffix(e.name, path.Ext(e.name)))] = true
	}
	folder, err := img.lookup("BDMV", "PLAYLIST")
	if err != nil {
		return -1
	}
	entries, err = img.list(folder)
	if err != nil {
		return -1
	}
	var playlists []playlist
	for _, e := range entries {
		_, number, ok := discPlaylist("BDMV/PLAYLIST/" + e.name)
		if !ok {
			continue
		}
		p, err := readPlaylist(img.reader(e))
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
		return -1
	}
	return titles[0].number
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
