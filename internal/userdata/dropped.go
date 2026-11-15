package userdata

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Dropped is a series the user dropped on a tracking service whose
// history they import: Service lists it as dropped, since At, the date it
// gives for the drop, else when an import first found it dropped (see
// package trackers).
type Dropped struct {
	Series  accounts.ID
	Service string
	At      time.Time
}

// Dropped lists the series the user dropped on a tracking service that
// still hold: a series stays dropped while any service lists it, until
// one of its episodes is played or started after the latest of its
// services' dates, in Polyfin or by an imported later play, as a removal
// from Next Up does. Each series comes once per service listing it, the
// series dropped last first.
func (s *Store) Dropped(ctx context.Context, user accounts.ID) ([]Dropped, error) {
	rows, err := s.db.Query(ctx, `WITH listed AS (
			SELECT series_id, service, coalesce(dropped_at, seen_at) AS at FROM tracker_dropped WHERE user_id = $1
		), held AS (
			SELECT series_id, max(at) AS at FROM listed GROUP BY series_id
		)
		SELECT l.series_id, l.service, l.at FROM listed l JOIN held h ON h.series_id = l.series_id
		WHERE NOT EXISTS (SELECT 1 FROM user_data u WHERE u.user_id = $1 AND u.series_id = h.series_id AND u.last_played_at > h.at)
		ORDER BY h.at DESC, l.series_id, l.service`, user)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Dropped, error) {
		var d Dropped
		err := row.Scan(&d.Series, &d.Service, &d.At)
		d.At = d.At.UTC()
		return d, err
	})
}

// DroppedSeries is the set of the series of Dropped.
func (s *Store) DroppedSeries(ctx context.Context, user accounts.ID) (map[accounts.ID]bool, error) {
	dropped, err := s.Dropped(ctx, user)
	if err != nil {
		return nil, err
	}
	set := make(map[accounts.ID]bool, len(dropped))
	for _, d := range dropped {
		set[d.Series] = true
	}
	return set, nil
}
