package localfiles

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
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
