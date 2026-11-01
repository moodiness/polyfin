package userdata

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// NextUpRemoval is a series a user took off Next Up, and when they did.
// It holds until one of the series' episodes is played after At, or one
// is released after it (see package jellyfin's Next Up).
type NextUpRemoval struct {
	Series accounts.ID
	At     time.Time
}

// RemoveFromNextUp takes series off the user's Next Up at at. A series
// taken off again counts from the latest removal.
func (s *Store) RemoveFromNextUp(ctx context.Context, user, series accounts.ID, at time.Time) error {
	_, err := s.db.Exec(ctx, `INSERT INTO next_up_removed (user_id, series_id, removed_at) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, series_id) DO UPDATE SET removed_at = excluded.removed_at`, user, series, at)
	return err
}

// PutBackOnNextUp forgets that the user took series off Next Up. It
// reports whether they had.
func (s *Store) PutBackOnNextUp(ctx context.Context, user, series accounts.ID) (bool, error) {
	tag, err := s.db.Exec(ctx, "DELETE FROM next_up_removed WHERE user_id = $1 AND series_id = $2", user, series)
	return tag.RowsAffected() > 0, err
}

// NextUpRemovals lists the series the user took off Next Up, the latest
// removed first, whether or not the removal still holds.
func (s *Store) NextUpRemovals(ctx context.Context, user accounts.ID) ([]NextUpRemoval, error) {
	rows, err := s.db.Query(ctx, `SELECT series_id, removed_at FROM next_up_removed WHERE user_id = $1
		ORDER BY removed_at DESC, series_id`, user)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (NextUpRemoval, error) {
		var removal NextUpRemoval
		err := row.Scan(&removal.Series, &removal.At)
		removal.At = removal.At.UTC()
		return removal, err
	})
}
