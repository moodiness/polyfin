// Package userdata keeps what each user did with each item: whether it was
// played and how often, where playback stopped, and whether the user marked
// it as a favorite or rated it. It applies the rules a Jellyfin server
// applies to playback reports and to played marks.
package userdata

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

// tick is Jellyfin's unit of time.
const tick = 100 * time.Nanosecond

// Data is a user's state for one item. The zero value is an item the user
// never touched.
type Data struct {
	Played    bool
	PlayCount int
	// Position is the resume point, zero when there is none; Runtime is the
	// runtime it was measured against, zero when unknown.
	Position time.Duration
	Runtime  time.Duration
	Favorite bool
	// Rating is the user's rating from 0 to 10, nil when the item is not
	// rated.
	Rating     *float64
	LastPlayed *time.Time
}

// Likes tells whether the user likes the item, as Jellyfin reads a
// rating: nil when the item is not rated.
func (d Data) Likes() *bool {
	if d.Rating == nil {
		return nil
	}
	return new(*d.Rating >= 5)
}

// Item names an item whose data changes. Episodes name their series and
// season, which count their played episodes.
type Item struct {
	ID, Series, Season accounts.ID
}

// Entry is the data of one item, as lists return it; Series and Season are
// zero but for episodes.
type Entry struct {
	Item, Series, Season accounts.ID
	Data
}

// Counts are the episodes under a series or season the user played, and
// those with a resume point.
type Counts struct {
	Played, Resumable int
}

// Store keeps user data in PostgreSQL.
type Store struct {
	db *pgxpool.Pool
}

// New returns a store backed by db.
func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

const columns = `item_id, coalesce(series_id, '00000000-0000-0000-0000-000000000000'),
	coalesce(season_id, '00000000-0000-0000-0000-000000000000'),
	played, play_count, position_ticks, runtime_ticks, favorite, rating, last_played_at`

func scanEntry(row pgx.Row) (Entry, error) {
	var e Entry
	var position, runtime int64
	var rating pgtype.Float8
	var lastPlayed pgtype.Timestamptz
	if err := row.Scan(&e.Item, &e.Series, &e.Season, &e.Played, &e.PlayCount, &position, &runtime, &e.Favorite, &rating, &lastPlayed); err != nil {
		return Entry{}, err
	}
	e.Position, e.Runtime = time.Duration(position)*tick, time.Duration(runtime)*tick
	if rating.Valid {
		e.Rating = new(rating.Float64)
	}
	if lastPlayed.Valid {
		e.LastPlayed = new(lastPlayed.Time.UTC())
	}
	return e, nil
}

func (s *Store) entries(ctx context.Context, query string, args ...any) ([]Entry, error) {
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Entry, error) { return scanEntry(row) })
}

// Get returns the user's data for those of ids that have any.
func (s *Store) Get(ctx context.Context, user accounts.ID, ids []accounts.ID) (map[accounts.ID]Data, error) {
	result := make(map[accounts.ID]Data, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	entries, err := s.entries(ctx, "SELECT "+columns+" FROM user_data WHERE user_id = $1 AND item_id = ANY($2)", user, ids)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		result[e.Item] = e.Data
	}
	return result, nil
}

// EpisodeCounts counts, for each series or season in parents, the episodes
// under it the user played and those with a resume point.
func (s *Store) EpisodeCounts(ctx context.Context, user accounts.ID, parents []accounts.ID) (map[accounts.ID]Counts, error) {
	result := make(map[accounts.ID]Counts, len(parents))
	if len(parents) == 0 {
		return result, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT series_id, count(*) FILTER (WHERE played), count(*) FILTER (WHERE position_ticks > 0)
		FROM user_data WHERE user_id = $1 AND series_id = ANY($2) GROUP BY series_id
		UNION ALL
		SELECT season_id, count(*) FILTER (WHERE played), count(*) FILTER (WHERE position_ticks > 0)
		FROM user_data WHERE user_id = $1 AND season_id = ANY($2) GROUP BY season_id`,
		user, parents)
	if err != nil {
		return nil, err
	}
	var parent accounts.ID
	var counts Counts
	_, err = pgx.ForEachRow(rows, []any{&parent, &counts.Played, &counts.Resumable}, func() error {
		result[parent] = counts
		return nil
	})
	return result, err
}

// Resumable lists the items with a resume point, most recently played
// first; those never started come last.
func (s *Store) Resumable(ctx context.Context, user accounts.ID) ([]Entry, error) {
	return s.entries(ctx, "SELECT "+columns+` FROM user_data WHERE user_id = $1 AND position_ticks > 0
		ORDER BY last_played_at DESC NULLS LAST, item_id`, user)
}

// Favorites lists the user's favorite items.
func (s *Store) Favorites(ctx context.Context, user accounts.ID) ([]Entry, error) {
	return s.entries(ctx, "SELECT "+columns+" FROM user_data WHERE user_id = $1 AND favorite ORDER BY item_id", user)
}

// Liked lists the items the user likes.
func (s *Store) Liked(ctx context.Context, user accounts.ID) ([]Entry, error) {
	return s.entries(ctx, "SELECT "+columns+" FROM user_data WHERE user_id = $1 AND rating >= 5 ORDER BY item_id", user)
}

// Played lists the items the user played, most recently played first.
func (s *Store) Played(ctx context.Context, user accounts.ID) ([]Entry, error) {
	return s.entries(ctx, "SELECT "+columns+` FROM user_data WHERE user_id = $1 AND played
		ORDER BY last_played_at DESC NULLS LAST, item_id`, user)
}

// Episodes lists the data of the episodes the user played or started, of
// every series, most recently played first.
func (s *Store) Episodes(ctx context.Context, user accounts.ID) ([]Entry, error) {
	return s.entries(ctx, "SELECT "+columns+` FROM user_data
		WHERE user_id = $1 AND series_id IS NOT NULL AND (played OR position_ticks > 0)
		ORDER BY last_played_at DESC NULLS LAST, item_id`, user)
}

// Change applies change to the data of each item, in one transaction, and
// returns the data of the items afterwards, in order. An item listed twice
// is changed once.
func (s *Store) Change(ctx context.Context, user accounts.ID, items []Item, change func(*Data)) ([]Data, error) {
	seen := make(map[accounts.ID]bool, len(items))
	items = slices.DeleteFunc(slices.Clone(items), func(item Item) bool {
		duplicate := seen[item.ID]
		seen[item.ID] = true
		return duplicate
	})
	if len(items) == 0 {
		return nil, nil
	}
	ids := make([]accounts.ID, len(items))
	series := make([]pgtype.UUID, len(items))
	seasons := make([]pgtype.UUID, len(items))
	for i, item := range items {
		ids[i], series[i], seasons[i] = item.ID, optionalID(item.Series), optionalID(item.Season)
	}
	var result []Data
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		// Rows are created first, so that concurrent changes of a new item
		// wait for each other below.
		if _, err := tx.Exec(ctx, `INSERT INTO user_data (user_id, item_id, series_id, season_id)
			SELECT $1, * FROM unnest($2::uuid[], $3::uuid[], $4::uuid[])
			ON CONFLICT (user_id, item_id) DO UPDATE
			SET series_id = coalesce(excluded.series_id, user_data.series_id), season_id = coalesce(excluded.season_id, user_data.season_id)`,
			user, ids, series, seasons); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, "SELECT "+columns+" FROM user_data WHERE user_id = $1 AND item_id = ANY($2) FOR UPDATE", user, ids)
		if err != nil {
			return err
		}
		current, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Entry, error) { return scanEntry(row) })
		if err != nil {
			return err
		}
		byID := make(map[accounts.ID]Data, len(current))
		for _, e := range current {
			byID[e.Item] = e.Data
		}
		result = make([]Data, len(items))
		played := make([]bool, len(items))
		counts := make([]int32, len(items))
		positions := make([]int64, len(items))
		runtimes := make([]int64, len(items))
		favorites := make([]bool, len(items))
		ratings := make([]pgtype.Float8, len(items))
		lastPlayed := make([]pgtype.Timestamptz, len(items))
		for i, id := range ids {
			data := byID[id]
			change(&data)
			result[i] = data
			played[i], counts[i], favorites[i] = data.Played, int32(max(data.PlayCount, 0)), data.Favorite
			positions[i], runtimes[i] = int64(max(data.Position, 0)/tick), int64(max(data.Runtime, 0)/tick)
			if data.Rating != nil {
				ratings[i] = pgtype.Float8{Float64: min(max(*data.Rating, 0), 10), Valid: true}
			}
			if data.LastPlayed != nil {
				lastPlayed[i] = pgtype.Timestamptz{Time: *data.LastPlayed, Valid: true}
			}
		}
		_, err = tx.Exec(ctx, `UPDATE user_data SET played = c.played, play_count = c.play_count, position_ticks = c.position_ticks,
				runtime_ticks = c.runtime_ticks, favorite = c.favorite, rating = c.rating, last_played_at = c.last_played_at
			FROM unnest($2::uuid[], $3::bool[], $4::int[], $5::bigint[], $6::bigint[], $7::bool[], $8::float8[], $9::timestamptz[])
				AS c (item_id, played, play_count, position_ticks, runtime_ticks, favorite, rating, last_played_at)
			WHERE user_data.user_id = $1 AND user_data.item_id = c.item_id`,
			user, ids, played, counts, positions, runtimes, favorites, ratings, lastPlayed)
		return err
	})
	return result, err
}

func optionalID(id accounts.ID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: id != accounts.ID{}}
}
