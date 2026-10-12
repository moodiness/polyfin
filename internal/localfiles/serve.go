package localfiles

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// catalogPage is how many titles a catalog page lists.
const catalogPage = 100

// episodeVideo is an episode's video identifier, "<series>:<season>:<episode>".
var episodeVideo = regexp.MustCompile(`^(.+):(\d{1,4}):(\d{1,5})$`)

// episodeVideoID is the video identifier of a series' episode (see
// episodeVideo).
func episodeVideoID(series string, season, episode int) string {
	return fmt.Sprintf("%s:%d:%d", series, season, episode)
}

// Catalog answers a folder's catalog: its titles, the latest found first,
// those whose name holds search when given, from skip; a home videos
// folder's the latest recorded first (see homeCatalog).
func (s *Service) Catalog(ctx context.Context, source accounts.ID, catalogType, id string, skip int, search string) ([]stremio.Meta, error) {
	var kind string
	err := s.db.QueryRow(ctx, "SELECT kind FROM local_folders WHERE addon_id = $1", source).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && (id != catalogID || catalogType != metaType(kind)) {
		return nil, stremio.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if kind == KindHomeVideos {
		return s.homeCatalog(ctx, source, skip, search)
	}
	rows, err := s.db.Query(ctx, `SELECT stremio_id, max(name), max(poster), max(title_year) FROM local_files
		WHERE addon_id = $1 AND stremio_id IS NOT NULL AND ($2 = '' OR strpos(lower(name), lower($2)) > 0)
		GROUP BY stremio_id ORDER BY max(modified) DESC, stremio_id OFFSET $3 LIMIT $4`, source, strings.TrimSpace(search), max(skip, 0), catalogPage)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (stremio.Meta, error) {
		meta := stremio.Meta{Type: catalogType}
		var year *int
		if err := row.Scan(&meta.ID, &meta.Name, &meta.Poster, &year); err != nil {
			return stremio.Meta{}, err
		}
		if year != nil {
			meta.Year = stremio.Text(strconv.Itoa(*year))
		}
		return meta, nil
	})
}

// Meta describes a title of a folder as its files tell, for when no
// metadata addon describes it: its name, poster and year, and a show's
// episodes found.
func (s *Service) Meta(ctx context.Context, source accounts.ID, id string) (stremio.Meta, error) {
	var kind string
	if err := s.db.QueryRow(ctx, "SELECT kind FROM local_folders WHERE addon_id = $1", source).Scan(&kind); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return stremio.Meta{}, stremio.ErrNotFound
		}
		return stremio.Meta{}, err
	}
	if kind == KindHomeVideos {
		return s.homeMeta(ctx, source, id)
	}
	rows, err := s.db.Query(ctx, `SELECT name, poster, coalesce(title_year, 0), season, episode, last_episode FROM local_files
		WHERE addon_id = $1 AND stremio_id = $2 ORDER BY season, episode`, source, id)
	if err != nil {
		return stremio.Meta{}, err
	}
	defer rows.Close()
	meta := stremio.Meta{ID: id, Type: metaType(kind)}
	seen := map[[2]int]bool{}
	found := false
	for rows.Next() {
		var year int
		var season, episode, last *int
		if err := rows.Scan(&meta.Name, &meta.Poster, &year, &season, &episode, &last); err != nil {
			return stremio.Meta{}, err
		}
		found = true
		if year > 0 {
			meta.Year = stremio.Text(strconv.Itoa(year))
		}
		if kind != KindShows || season == nil {
			continue
		}
		for number := *episode; number <= *last; number++ {
			if seen[[2]int{*season, number}] {
				continue
			}
			seen[[2]int{*season, number}] = true
			meta.Videos = append(meta.Videos, stremio.Video{ID: episodeVideoID(id, *season, number),
				Title: fmt.Sprintf("Episode %d", number), Season: stremio.Number(*season), Episode: stremio.Number(number)})
		}
	}
	if err := rows.Err(); err != nil {
		return stremio.Meta{}, err
	}
	if !found {
		return stremio.Meta{}, stremio.ErrNotFound
	}
	return meta, nil
}

// Streams answers the streams of a folder for a title or an episode: its
// files, named after the folder, with their resolution and size, and the
// subtitle files beside them, which the loopback interface serves (see
// UseFiles). A disc's entry bears its disc's name as its file name (see
// discLabel), a title other than the disc's main one, as each title of a
// show's disc, its number too, as in "Box - DISC02 - title 2.m2ts", or for
// an image's, "Box - title 2.iso"; a Blu-ray disc's playlist, a DVD and a
// disc image have the address of their input as their URL, FFmpeg reading
// them itself (see Input).
func (s *Service) Streams(ctx context.Context, source accounts.ID, id string) ([]stremio.Stream, error) {
	if s.files == "" {
		return nil, nil
	}
	var kind, folderName, location string
	err := s.db.QueryRow(ctx, `SELECT f.kind, a.manifest->>'name', a.manifest_url FROM local_folders f JOIN addons a ON a.id = f.addon_id
		WHERE f.addon_id = $1`, source).Scan(&kind, &folderName, &location)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rows pgx.Rows
	if kind == KindShows {
		m := episodeVideo.FindStringSubmatch(id)
		if m == nil {
			return nil, nil
		}
		season, _ := strconv.Atoi(m[2])
		episode, _ := strconv.Atoi(m[3])
		rows, err = s.db.Query(ctx, `SELECT id, path, unit, size, height FROM local_files
			WHERE addon_id = $1 AND stremio_id = $2 AND season = $3 AND episode <= $4 AND last_episode >= $4 ORDER BY size DESC, path`,
			source, m[1], season, episode)
	} else {
		rows, err = s.db.Query(ctx, `SELECT id, path, unit, size, height FROM local_files
			WHERE addon_id = $1 AND stremio_id = $2 ORDER BY size DESC, path`, source, id)
	}
	if err != nil {
		return nil, err
	}
	var files []accounts.ID
	streams, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (stremio.Stream, error) {
		var file accounts.ID
		var rel, unit string
		var size int64
		var height *int
		if err := row.Scan(&file, &rel, &unit, &size, &height); err != nil {
			return stremio.Stream{}, err
		}
		files = append(files, file)
		details := []string{formatSize(size)}
		if height != nil {
			details = append([]string{strconv.Itoa(*height) + "p"}, details...)
		}
		base, number, titled := titleOf(rel)
		if !titled {
			base = rel
		}
		filename := path.Base(rel)
		if _, folder, ok := discEntry(rel); ok {
			label := discLabel(folder, locationName(location))
			if titled {
				label += " - title " + strconv.Itoa(number)
			}
			filename = label + path.Ext(base)
			if _, number, playlist := discPlaylist(rel); playlist {
				if unit != discUnit(folder) {
					label += " - title " + strconv.Itoa(number)
				}
				// The bluray protocol answers an MPEG-TS stream, as a disc's
				// streams are.
				filename = label + ".m2ts"
			}
		}
		if imageFile(base) && titled {
			ext := path.Ext(base)
			filename = strings.TrimSuffix(path.Base(base), ext) + " - title " + strconv.Itoa(number) + ext
		}
		return stremio.Stream{Name: folderName, Description: strings.Join(details, " · "), URL: s.address(file, rel),
			BehaviorHints: stremio.StreamBehavior{Filename: filename, VideoSize: stremio.Number(size)}}, nil
	})
	if err != nil || len(files) == 0 {
		return streams, err
	}
	subtitles, err := s.fileSubtitles(ctx, files)
	if err != nil {
		return nil, err
	}
	for i := range streams {
		streams[i].Subtitles = subtitles[files[i]]
	}
	return streams, nil
}

// formatSize writes a file's size in decimal units, as players show them.
func formatSize(size int64) string {
	switch {
	case size >= 1e9:
		return strconv.FormatFloat(float64(size)/1e9, 'f', 1, 64) + " GB"
	case size >= 1e6:
		return strconv.FormatFloat(float64(size)/1e6, 'f', 0, 64) + " MB"
	}
	return strconv.FormatFloat(float64(size)/1e3, 'f', 0, 64) + " KB"
}

// address is where a file of a folder is read: through the loopback
// interface as one file, but for a Blu-ray disc's playlist, a DVD and a
// disc image, whose input FFmpeg reads them through (see Input).
func (s *Service) address(file accounts.ID, rel string) string {
	base, _, titled := titleOf(rel)
	if !titled {
		base = rel
	}
	if disc, _, ok := discEntry(rel); ok {
		if _, _, playlist := discPlaylist(rel); playlist || disc == DiscDVD {
			return s.inputs + file.String()
		}
	}
	if imageFile(base) {
		return s.inputs + file.String()
	}
	return s.files + file.String() + strings.ToLower(path.Ext(rel))
}

// ErrGone reports a file a scan found that is no longer where it was, or
// that a symbolic link now leads out of its folder.
var ErrGone = errors.New("the file is no longer in its folder")

// Open opens the file key names, its identifier and extension as Streams
// gives them, a video, a subtitle file, a track, an album's artwork or a
// title's (see Artwork), for reading, with its modification time: within
// its folder, through no symbolic link leading out of it, or from its
// share; a cover a track embeds or a home video's frame, from what its
// scan kept; the title of a disc on a share, as one file (see openTitle).
// A VobSub pair is named by its .idx file, whose key with the .sub
// extension opens its .sub file, as FFmpeg finds it. A share that cannot
// be read fails with why (see shareFailure).
func (s *Service) Open(ctx context.Context, key string) (io.ReadSeekCloser, time.Time, error) {
	raw, extension, _ := strings.Cut(key, ".")
	id, err := accounts.ParseID(raw)
	if err != nil {
		return nil, time.Time{}, ErrGone
	}
	var folder accounts.ID
	var root, rel, user, sealed string
	var data []byte
	var modified *time.Time
	err = s.db.QueryRow(ctx, `SELECT f.addon_id, a.manifest_url, f.path, f.data, f.modified, l.share_user, l.share_password
		FROM (SELECT addon_id, path, NULL::bytea AS data, NULL::timestamptz AS modified FROM local_files WHERE id = $1
			UNION ALL SELECT addon_id, CASE WHEN $2 AND pair <> '' THEN pair ELSE path END, NULL, NULL FROM local_subtitles WHERE id = $1
			UNION ALL SELECT addon_id, path, NULL, NULL FROM local_tracks WHERE id = $1
			UNION ALL SELECT addon_id, path, data, modified FROM local_covers WHERE id = $1
			UNION ALL SELECT addon_id, path, NULL, NULL FROM local_artwork WHERE id = $1
			UNION ALL SELECT addon_id, path, frame, modified FROM local_videos WHERE frame_id = $1 AND frame IS NOT NULL) f
		JOIN addons a ON a.id = f.addon_id JOIN local_folders l ON l.addon_id = f.addon_id`, id, strings.EqualFold(extension, "sub")).
		Scan(&folder, &root, &rel, &data, &modified, &user, &sealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, time.Time{}, ErrGone
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	if data != nil {
		return readSeekNopCloser{bytes.NewReader(data)}, *modified, nil
	}
	if shareKind(root) != "" && shareTitle(rel) {
		return s.openTitle(ctx, id)
	}
	if shareKind(root) != "" {
		r, err := s.folderRemote(folder, root, user, sealed)
		if err != nil {
			return nil, time.Time{}, err
		}
		file, modified, err := r.open(ctx, rel)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, time.Time{}, ErrGone
		}
		return file, modified, err
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, time.Time{}, ErrGone
	}
	target, err := filepath.EvalSymlinks(filepath.Join(real, filepath.FromSlash(rel)))
	if err != nil || !within(real, target) {
		return nil, time.Time{}, ErrGone
	}
	file, err := os.Open(target)
	if err != nil {
		return nil, time.Time{}, ErrGone
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, time.Time{}, ErrGone
	}
	return file, info.ModTime(), nil
}

// Identity tells the file of a folder a video's key names, as Streams
// gives it (see Open and Input), as the folder's last scan found it: path,
// the folder's identifier and the file's path within it, and state, its
// size and modification time, which change with what it holds.
func (s *Service) Identity(ctx context.Context, key string) (path, state string, err error) {
	raw, _, _ := strings.Cut(key, ".")
	id, err := accounts.ParseID(raw)
	if err != nil {
		return "", "", ErrGone
	}
	var folder accounts.ID
	var rel string
	var size int64
	var modified time.Time
	err = s.db.QueryRow(ctx, "SELECT addon_id, path, size, modified FROM local_files WHERE id = $1", id).Scan(&folder, &rel, &size, &modified)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrGone
	}
	if err != nil {
		return "", "", err
	}
	return folder.String() + "/" + rel, strconv.FormatInt(size, 10) + " " + strconv.FormatInt(modified.UnixMicro(), 10), nil
}

// readSeekNopCloser closes nothing.
type readSeekNopCloser struct{ io.ReadSeeker }

func (readSeekNopCloser) Close() error { return nil }
