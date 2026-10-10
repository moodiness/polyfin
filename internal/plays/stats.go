package plays

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// topCount is how many movies, series and channels the statistics rank.
const topCount = 10

// Statistics sum the playbacks a filter selects. Times are played times,
// pauses left out.
type Statistics struct {
	Plays  int
	Played time.Duration
	// Users are the users who played, the one who played longest first.
	Users []UserFigures
	// Buckets are the days, or months, with playbacks, in order, by the
	// time zone of the query: when they started.
	Buckets []Bucket
	// Movies, Series and Channels rank what played longest; Channels
	// count Live TV channels and the Replay programmes of each.
	Movies, Series, Channels []Ranked
	// Apps, Devices and Methods group the playbacks by app, by device
	// (with its app), and by play method ("" when none was told).
	Apps, Devices, Methods []Group
	// Hours spread each playback over the hours of the week it lasted,
	// by the time zone of the query: Hours[0] is Monday from 0:00 to 1:00,
	// Hours[167] Sunday from 23:00.
	Hours [7 * 24]time.Duration
}

// UserFigures are what one user played.
type UserFigures struct {
	User   accounts.ID
	Name   string
	Plays  int
	Played time.Duration
}

// Bucket is a day, or a month, and what each user played in it.
type Bucket struct {
	// Start is its first day, as YYYY-MM-DD.
	Start  string
	Played time.Duration
	Users  map[accounts.ID]time.Duration
}

// Ranked is a movie, a series or a channel, by its last name, with how
// many played it.
type Ranked struct {
	ID     accounts.ID
	Name   string
	Plays  int
	Users  int
	Played time.Duration
}

// Group is an app, a device or a play method.
type Group struct {
	Name   string
	App    string
	Plays  int
	Played time.Duration
}

// Units of the buckets.
const (
	Day   = "day"
	Month = "month"
)

// Statistics sums the playbacks f selects; unit is Day or Month, and zone
// the time zone days, months and hours are counted in, by its IANA name.
func (h *History) Statistics(ctx context.Context, f Filter, unit string, zone *time.Location) (Statistics, error) {
	var st Statistics
	args := append(f.args(), zone.String())
	from := " FROM playback_history h WHERE " + filterWhere
	var played int64
	if err := h.db.QueryRow(ctx, "SELECT count(*), coalesce(sum(h.played_seconds), 0)"+from, args[:3]...).Scan(&st.Plays, &played); err != nil {
		return st, err
	}
	st.Played = seconds(played)

	rows, err := h.db.Query(ctx, `SELECT h.user_id, u.name, count(*), sum(h.played_seconds)
		FROM playback_history h JOIN users u ON u.id = h.user_id WHERE `+filterWhere+`
		GROUP BY h.user_id, u.name ORDER BY sum(h.played_seconds) DESC, u.name`, args[:3]...)
	if err == nil {
		st.Users, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (UserFigures, error) {
			var u UserFigures
			err := row.Scan(&u.User, &u.Name, &u.Plays, &played)
			u.Played = seconds(played)
			return u, err
		})
	}
	if err != nil {
		return st, err
	}

	if unit != Month {
		unit = Day
	}
	rows, err = h.db.Query(ctx, `SELECT to_char(date_trunc('`+unit+`', h.started_at AT TIME ZONE $4), 'YYYY-MM-DD') AS start, h.user_id,
			sum(h.played_seconds)`+from+` GROUP BY start, h.user_id ORDER BY start`, args...)
	if err != nil {
		return st, err
	}
	var start string
	var user accounts.ID
	if _, err := pgx.ForEachRow(rows, []any{&start, &user, &played}, func() error {
		if n := len(st.Buckets); n == 0 || st.Buckets[n-1].Start != start {
			st.Buckets = append(st.Buckets, Bucket{Start: start, Users: map[accounts.ID]time.Duration{}})
		}
		b := &st.Buckets[len(st.Buckets)-1]
		b.Played += seconds(played)
		b.Users[user] += seconds(played)
		return nil
	}); err != nil {
		return st, err
	}

	if st.Movies, err = h.rank(ctx, f, movieRanking, topCount); err != nil {
		return st, err
	}
	if st.Series, err = h.rank(ctx, f, seriesRanking, topCount); err != nil {
		return st, err
	}
	if st.Channels, err = h.rank(ctx, f, channelRanking, topCount); err != nil {
		return st, err
	}

	grouped := func(columns string) ([]Group, error) {
		rows, err := h.db.Query(ctx, `SELECT `+columns+`, count(*), sum(h.played_seconds)`+from+`
			GROUP BY `+columns+` ORDER BY sum(h.played_seconds) DESC, count(*) DESC`, args[:3]...)
		if err != nil {
			return nil, err
		}
		return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Group, error) {
			var g Group
			targets := []any{&g.Name}
			if columns == "h.device, h.app" {
				targets = append(targets, &g.App)
			}
			err := row.Scan(append(targets, &g.Plays, &played)...)
			g.Played = seconds(played)
			return g, err
		})
	}
	if st.Apps, err = grouped("h.app"); err != nil {
		return st, err
	}
	if st.Devices, err = grouped("h.device, h.app"); err != nil {
		return st, err
	}
	if st.Methods, err = grouped("h.method"); err != nil {
		return st, err
	}

	// Each playback's time played is spread over the hours of the week it
	// lasted, in proportion to the part of each hour it covered.
	rows, err = h.db.Query(ctx, `WITH p AS (
			SELECT h.started_at AT TIME ZONE $4 AS s, h.ended_at AT TIME ZONE $4 AS e, h.played_seconds AS played
			FROM playback_history h WHERE `+filterWhere+` AND h.played_seconds > 0)
		SELECT (extract(isodow FROM hour)::int - 1) * 24 + extract(hour FROM hour)::int AS slot,
			sum(p.played * extract(epoch FROM least(p.e, hour + interval '1 hour') - greatest(p.s, hour))
				/ greatest(extract(epoch FROM p.e - p.s), 1))
		FROM p, generate_series(date_trunc('hour', p.s), p.e, interval '1 hour') AS hour
		GROUP BY slot`, args...)
	if err != nil {
		return st, err
	}
	var slot int
	var share float64
	if _, err := pgx.ForEachRow(rows, []any{&slot, &share}, func() error {
		if slot >= 0 && slot < len(st.Hours) {
			st.Hours[slot] += time.Duration(share * float64(time.Second))
		}
		return nil
	}); err != nil {
		return st, err
	}
	return st, nil
}

// rankQuery ranks what id groups, among the playbacks where selects, by
// time played: its last name, how many played it, by how many users, and
// how long, the $4 first.
func rankQuery(id, name, where string) string {
	return `SELECT ` + id + `, (array_agg(` + name + ` ORDER BY h.ended_at DESC))[1], count(*), count(DISTINCT h.user_id),
		sum(h.played_seconds) FROM playback_history h WHERE ` + filterWhere + ` AND ` + where + `
		GROUP BY ` + id + ` ORDER BY sum(h.played_seconds) DESC, count(*) DESC LIMIT $4`
}

// The rankings of movies, series, and Live TV channels with their Replay
// programmes.
var (
	movieRanking   = rankQuery("h.item_id", "h.name", "h.kind = 'movie'")
	seriesRanking  = rankQuery("h.series_id", "h.series_name", "h.kind = 'episode' AND h.series_id IS NOT NULL")
	channelRanking = rankQuery("h.channel_id", "h.channel_name", "h.kind IN ('channel', 'replay') AND h.channel_id IS NOT NULL")
)

// rank runs a ranking over the playbacks f selects, keeping the limit
// first.
func (h *History) rank(ctx context.Context, f Filter, query string, limit int) ([]Ranked, error) {
	rows, err := h.db.Query(ctx, query, append(f.args(), limit)...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Ranked, error) {
		var r Ranked
		var played int64
		err := row.Scan(&r.ID, &r.Name, &r.Plays, &r.Users, &played)
		r.Played = seconds(played)
		return r, err
	})
}

// Summary is what a weekly summary tells of the playbacks a filter
// selects: how many, how long they played, and the movies and series that
// played longest.
type Summary struct {
	Plays          int
	Played         time.Duration
	Movies, Series []Ranked
}

// Summary sums the playbacks f selects, and ranks the top movies and
// series that played longest.
func (h *History) Summary(ctx context.Context, f Filter, top int) (Summary, error) {
	var sum Summary
	var played int64
	if err := h.db.QueryRow(ctx, "SELECT count(*), coalesce(sum(h.played_seconds), 0) FROM playback_history h WHERE "+filterWhere,
		f.args()...).Scan(&sum.Plays, &played); err != nil {
		return sum, err
	}
	sum.Played = seconds(played)
	var err error
	if sum.Movies, err = h.rank(ctx, f, movieRanking, top); err != nil {
		return sum, err
	}
	sum.Series, err = h.rank(ctx, f, seriesRanking, top)
	return sum, err
}

func seconds(n int64) time.Duration {
	return time.Duration(n) * time.Second
}
