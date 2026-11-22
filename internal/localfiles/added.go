package localfiles

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Added is a title files were added for to the local folders: a movie, or
// a show with the files of its episodes, by the name of the title it was
// matched to, or else the name its files tell; or an album of a music
// folder with its tracks, by its title. Kind is the kind of its folder,
// KindMovies, KindShows or KindMusic.
type Added struct {
	Name  string
	Kind  string
	Files int
}

// Added lists the titles and albums whose files scans first found from
// since until before until, those with the most files first, up to limit
// of them, and how many files were added in all: the tracks a music
// folder's scan left out of its albums count, but are in none.
func (s *Service) Added(ctx context.Context, since, until time.Time, limit int) ([]Added, int, error) {
	var total int
	if err := s.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM local_files WHERE added_at >= $1 AND added_at < $2) +
		(SELECT count(*) FROM local_tracks WHERE added_at >= $1 AND added_at < $2)`, since, until).Scan(&total); err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}
	rows, err := s.db.Query(ctx, `SELECT added_name, kind, files FROM (
			SELECT coalesce(nullif(f.name, ''), nullif(f.title, ''), f.unit) AS added_name, d.kind, count(*) AS files
			FROM local_files f JOIN local_folders d ON d.addon_id = f.addon_id
			WHERE f.added_at >= $1 AND f.added_at < $2
			GROUP BY added_name, d.kind
			UNION ALL
			SELECT (array_agg(t.album ORDER BY t.disc, t.track, t.path))[1], d.kind, count(*)
			FROM local_tracks t JOIN local_folders d ON d.addon_id = t.addon_id
			WHERE t.added_at >= $1 AND t.added_at < $2 AND t.reason = ''
			GROUP BY t.addon_id, t.album_id, d.kind) a
		ORDER BY files DESC, added_name LIMIT $3`, since, until, limit)
	if err != nil {
		return nil, 0, err
	}
	added, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Added, error) {
		var a Added
		err := row.Scan(&a.Name, &a.Kind, &a.Files)
		return a, err
	})
	return added, total, err
}

// maxAddedTold bounds the titles one scan tells of as added: a folder
// that gains a whole library at once is not told title by title.
const maxAddedTold = 100

// tellAdded tells what UseAdded set of the movies and episodes the scan of
// the folder id, of kind, that started at found new: those of matched
// files stored for the first time since, that no file of any folder gave
// before, whatever their versions, .strm files included. A title whose
// files were stored before, matched late, or another version of it, is
// not new; nor is an unmatched file. At most maxAddedTold are told, the
// first by identifier. A failure is only logged: the scan itself went
// through.
func (s *Service) tellAdded(ctx context.Context, id accounts.ID, kind string, at time.Time) {
	s.mu.Lock()
	added := s.added
	s.mu.Unlock()
	if added == nil || kind != KindMovies && kind != KindShows {
		return
	}
	rows, err := s.db.Query(ctx, `SELECT DISTINCT f.stremio_id, f.season, f.episode, f.last_episode FROM local_files f
		WHERE f.addon_id = $1 AND f.added_at >= $2 AND f.stremio_id IS NOT NULL
			AND NOT EXISTS (SELECT 1 FROM local_files o WHERE o.stremio_id = f.stremio_id AND o.season IS NOT DISTINCT FROM f.season
				AND o.episode IS NOT DISTINCT FROM f.episode AND (o.added_at IS NULL OR o.added_at < $2))
		ORDER BY 1, 2, 3`, id, at)
	if err != nil {
		s.logger.Warn("The titles a scan added could not be listed", "error", err)
		return
	}
	type found struct {
		stremioID             string
		season, episode, last *int
	}
	titles, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (found, error) {
		var f found
		err := row.Scan(&f.stremioID, &f.season, &f.episode, &f.last)
		return f, err
	})
	if err != nil {
		s.logger.Warn("The titles a scan added could not be listed", "error", err)
		return
	}
	var items []accounts.ID
	for _, t := range titles {
		for _, item := range titleItems(kind, t.stremioID, t.season, t.episode, t.last) {
			if !slices.Contains(items, item.ID) {
				items = append(items, item.ID)
			}
		}
	}
	if len(items) > maxAddedTold {
		s.logger.Info("A scan added more titles than are told one by one", "titles", len(items), "told", maxAddedTold)
		items = items[:maxAddedTold]
	}
	if len(items) > 0 {
		added(items)
	}
}
