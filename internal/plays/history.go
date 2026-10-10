package plays

import (
	"context"
	"encoding/csv"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

// SweepInterval is how often the playbacks older than the settings keep
// are deleted.
const SweepInterval = 24 * time.Hour

// MinPlayed is how long a video must play to be kept: one that fails to
// start, or is closed at once, is not a playback.
const MinPlayed = 10 * time.Second

// maxText bounds the names kept, in characters.
const maxText = 512

// History keeps the videos played, while the settings keep a playback
// history (see accounts.Settings.PlaybackHistory).
type History struct {
	db       *pgxpool.Pool
	settings func() accounts.Settings
	logger   *slog.Logger
	now      func() time.Time
}

// NewHistory returns a history kept in db, following settings.
func NewHistory(db *pgxpool.Pool, settings func() accounts.Settings, logger *slog.Logger) *History {
	return &History{db: db, settings: settings, logger: logger, now: time.Now}
}

// Record keeps a video that stopped, when it played MinPlayed at least and
// the settings keep a history. A failure is logged, never returned: the
// history is a record, not part of the playback.
func (h *History) Record(ctx context.Context, c Change) {
	p := c.Playback
	if c.Kind != Stopped || !Video(p.Title.Kind) || p.Played < MinPlayed || !h.settings().PlaybackHistory {
		return
	}
	t := p.Title
	var series, channel *accounts.ID
	var season, episode *int
	if t.Kind == Episode {
		season, episode = &t.Season, &t.Episode
		if t.SeriesID != (accounts.ID{}) {
			series = &t.SeriesID
		}
	}
	if t.ChannelID != (accounts.ID{}) {
		channel = &t.ChannelID
	}
	ended := p.Ended
	if ended.Before(p.Started) {
		ended = p.Started
	}
	_, err := h.db.Exec(context.WithoutCancel(ctx), `INSERT INTO playback_history (user_id, item_id, kind, name, series_id, series_name, season,
			episode, channel_id, channel_name, app, device, started_at, ended_at, played_seconds, position_seconds, method)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
		p.User, t.Item, t.Kind, clip(t.Name), series, clip(t.SeriesName), season, episode, channel, clip(t.ChannelName), clip(p.App),
		clip(p.DeviceName), p.Started, ended, int(p.Played.Seconds()), int(p.Position.Seconds()), p.Method)
	if err != nil {
		h.logger.Warn("A playback could not be kept in the history", "error", err)
	}
}

// clip shortens s to at most maxText characters.
func clip(s string) string {
	runes := []rune(s)
	if len(runes) <= maxText {
		return s
	}
	return string(runes[:maxText-1]) + "…"
}

// Sweep deletes the playbacks that started more days ago than the settings
// keep, and reports how many.
func (h *History) Sweep(ctx context.Context) (int64, error) {
	days := h.settings().PlaybackHistoryDays
	tag, err := h.db.Exec(ctx, "DELETE FROM playback_history WHERE started_at < $1", h.now().AddDate(0, 0, -days))
	return tag.RowsAffected(), err
}

// Filter selects playbacks: those of User, unless nil, that started at
// Since or later, unless zero, and before Until, unless zero.
type Filter struct {
	User  *accounts.ID
	Since time.Time
	Until time.Time
}

// args are the filter's arguments, $1 to $3 of filterWhere.
func (f Filter) args() []any {
	var since, until *time.Time
	if !f.Since.IsZero() {
		since = &f.Since
	}
	if !f.Until.IsZero() {
		until = &f.Until
	}
	return []any{since, f.User, until}
}

// filterWhere selects the playbacks of the filter's args.
const filterWhere = "($1::timestamptz IS NULL OR h.started_at >= $1) AND ($2::uuid IS NULL OR h.user_id = $2) AND " +
	"($3::timestamptz IS NULL OR h.started_at < $3)"

// Entry is a playback kept in the history, with its user's name.
type Entry struct {
	ID       int64
	User     accounts.ID
	UserName string
	Title    Title
	App      string
	Device   string
	Started  time.Time
	Ended    time.Time
	Played   time.Duration
	Position time.Duration
	Method   string
}

const entryColumns = `h.id, h.user_id, u.name, h.item_id, h.kind, h.name, coalesce(h.series_id, '00000000-0000-0000-0000-000000000000'),
	h.series_name, coalesce(h.season, 0), coalesce(h.episode, 0), coalesce(h.channel_id, '00000000-0000-0000-0000-000000000000'),
	h.channel_name, h.app, h.device, h.started_at, h.ended_at, h.played_seconds, h.position_seconds, h.method`

func scanEntry(row pgx.CollectableRow) (Entry, error) {
	var e Entry
	var played, position int
	err := row.Scan(&e.ID, &e.User, &e.UserName, &e.Title.Item, &e.Title.Kind, &e.Title.Name, &e.Title.SeriesID, &e.Title.SeriesName,
		&e.Title.Season, &e.Title.Episode, &e.Title.ChannelID, &e.Title.ChannelName, &e.App, &e.Device, &e.Started, &e.Ended, &played,
		&position, &e.Method)
	e.Played, e.Position = time.Duration(played)*time.Second, time.Duration(position)*time.Second
	return e, err
}

// Entries lists the playbacks f selects, the latest started first: limit
// of them from offset, and how many f selects in all.
func (h *History) Entries(ctx context.Context, f Filter, offset, limit int) ([]Entry, int, error) {
	var total int
	if err := h.db.QueryRow(ctx, "SELECT count(*) FROM playback_history h WHERE "+filterWhere, f.args()...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := h.db.Query(ctx, "SELECT "+entryColumns+" FROM playback_history h JOIN users u ON u.id = h.user_id WHERE "+filterWhere+
		" ORDER BY h.started_at DESC, h.id DESC OFFSET $4 LIMIT $5", append(f.args(), offset, limit)...)
	if err != nil {
		return nil, 0, err
	}
	entries, err := pgx.CollectRows(rows, scanEntry)
	return entries, total, err
}

// csvHeader names the columns of WriteCSV.
var csvHeader = []string{"started_at", "ended_at", "user", "kind", "title", "series", "season", "episode", "channel", "app", "device",
	"played_seconds", "position_seconds", "method"}

// WriteCSV writes the playbacks f selects to w as CSV, the latest started
// first, with a header line. Times are in UTC, as RFC 3339.
func (h *History) WriteCSV(ctx context.Context, w io.Writer, f Filter) error {
	rows, err := h.db.Query(ctx, "SELECT "+entryColumns+" FROM playback_history h JOIN users u ON u.id = h.user_id WHERE "+filterWhere+
		" ORDER BY h.started_at DESC, h.id DESC", f.args()...)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := csv.NewWriter(w)
	if err := out.Write(csvHeader); err != nil {
		return err
	}
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return err
		}
		season, episode := "", ""
		if e.Title.Kind == Episode {
			season, episode = strconv.Itoa(e.Title.Season), strconv.Itoa(e.Title.Episode)
		}
		record := []string{e.Started.UTC().Format(time.RFC3339), e.Ended.UTC().Format(time.RFC3339), e.UserName, e.Title.Kind, e.Title.Name,
			e.Title.SeriesName, season, episode, e.Title.ChannelName, e.App, e.Device, strconv.Itoa(int(e.Played.Seconds())),
			strconv.Itoa(int(e.Position.Seconds())), e.Method}
		for i := range record {
			record[i] = cell(record[i])
		}
		if err := out.Write(record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	out.Flush()
	return out.Error()
}

// cell keeps a spreadsheet from reading a name as a formula: a name that
// starts as one is written after an apostrophe.
func cell(value string) string {
	if value != "" && strings.ContainsRune("=+-@\t\r", rune(value[0])) {
		return "'" + value
	}
	return value
}
