package localfiles

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Added is a title files were added for to the local folders: a movie, or
// a show with the files of its episodes, by the name of the title it was
// matched to, or else the name its files tell. Kind is the kind of its
// folder, KindMovies or KindShows.
type Added struct {
	Name  string
	Kind  string
	Files int
}

// Added lists the titles whose files scans first found from since until
// before until, those with the most files first, up to limit of them, and
// how many files were added in all.
func (s *Service) Added(ctx context.Context, since, until time.Time, limit int) ([]Added, int, error) {
	var total int
	if err := s.db.QueryRow(ctx, "SELECT count(*) FROM local_files WHERE added_at >= $1 AND added_at < $2", since, until).Scan(&total); err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}
	rows, err := s.db.Query(ctx, `SELECT coalesce(nullif(f.name, ''), nullif(f.title, ''), f.unit) AS added_name, d.kind, count(*)
		FROM local_files f JOIN local_folders d ON d.addon_id = f.addon_id
		WHERE f.added_at >= $1 AND f.added_at < $2
		GROUP BY added_name, d.kind ORDER BY count(*) DESC, added_name LIMIT $3`, since, until, limit)
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
