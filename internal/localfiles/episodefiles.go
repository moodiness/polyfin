package localfiles

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// EpisodeFile is a file of a shows folder matched to episodes of a
// series, which FFmpeg reads as one file: the files whose audio the intros
// and the credits of a season are looked for in (see package
// audiosegments).
type EpisodeFile struct {
	// Path is where the file is: its folder's path in the container, or
	// its share's address, joined with its path in the folder.
	Path     string
	Size     int64
	Modified time.Time
	// URL is the address the file is read at (see UseFiles).
	URL string
	// Folder, Title and Season tell the season the file is of: the
	// folder's, its series' identifier and its number. First and Last are
	// the numbers of the episodes it holds.
	Folder      accounts.ID
	Title       string
	Season      int
	First, Last int
}

// episodeColumns are the columns scanEpisodeFile reads.
const episodeColumns = `f.id, f.addon_id, a.manifest_url, f.path, f.size, f.modified, f.stremio_id, f.season, f.episode, f.last_episode
	FROM local_files f JOIN local_folders l ON l.addon_id = f.addon_id JOIN addons a ON a.id = f.addon_id
	WHERE l.kind = 'shows' AND f.stremio_id IS NOT NULL AND f.season IS NOT NULL`

// scanEpisodeFile reads a file of episodeColumns, ok only when it is read
// as one file.
func (s *Service) scanEpisodeFile(row pgx.Row) (EpisodeFile, bool, error) {
	var id accounts.ID
	var root, rel string
	var file EpisodeFile
	if err := row.Scan(&id, &file.Folder, &root, &rel, &file.Size, &file.Modified, &file.Title, &file.Season, &file.First,
		&file.Last); err != nil {
		return EpisodeFile{}, false, err
	}
	file.Path = strings.TrimSuffix(root, "/") + "/" + rel
	file.URL = s.address(id, rel)
	return file, s.files != "" && strings.HasPrefix(file.URL, s.files), nil
}

// EpisodeFile returns the file of a shows folder a version's address names
// (see Streams), when it is one matched to episodes and read as one file:
// not an addon's stream, nor a disc read through its input.
func (s *Service) EpisodeFile(ctx context.Context, address string) (EpisodeFile, bool, error) {
	key, ok := strings.CutPrefix(address, s.files)
	if !ok || s.files == "" {
		return EpisodeFile{}, false, nil
	}
	raw, _, _ := strings.Cut(key, ".")
	id, err := accounts.ParseID(raw)
	if err != nil {
		return EpisodeFile{}, false, nil
	}
	file, ok, err := s.scanEpisodeFile(s.db.QueryRow(ctx, "SELECT "+episodeColumns+" AND f.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return EpisodeFile{}, false, nil
	}
	return file, ok && err == nil && file.URL == address, err
}

// SeasonFiles lists the files of the season a file of EpisodeFile is of,
// itself included, in the order of their paths.
func (s *Service) SeasonFiles(ctx context.Context, file EpisodeFile) ([]EpisodeFile, error) {
	rows, err := s.db.Query(ctx, "SELECT "+episodeColumns+" AND f.addon_id = $1 AND f.stremio_id = $2 AND f.season = $3 ORDER BY f.path",
		file.Folder, file.Title, file.Season)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var files []EpisodeFile
	for rows.Next() {
		found, ok, err := s.scanEpisodeFile(rows)
		if err != nil {
			return nil, err
		}
		if ok {
			files = append(files, found)
		}
	}
	return files, rows.Err()
}
