// Package recordings records Live TV programmes into files, as Jellyfin's
// DVR does: timers record one programme each, series timers make timers for
// the upcoming programmes of a title, and recordings are the files written.
// Everything is kept in the database, so that it survives restarts.
package recordings

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

var (
	// ErrNotFound reports a timer, series timer or recording that does not
	// exist.
	ErrNotFound = errors.New("not found")
	// ErrUnavailable reports recording asked of a server that does not
	// record: it has no recordings folder.
	ErrUnavailable = errors.New("recording is off")
	// ErrScheduled reports a programme that has a timer already, which
	// Jellyfin refuses as an invalid argument.
	ErrScheduled = errors.New("a scheduled recording already exists for this program")
	// ErrInvalid reports values outside their bounds.
	ErrInvalid = errors.New("invalid timer")
)

// Status is where a timer is, Jellyfin's RecordingStatus.
type Status string

const (
	StatusNew        Status = "New"
	StatusInProgress Status = "InProgress"
	StatusCompleted  Status = "Completed"
	StatusCancelled  Status = "Cancelled"
	StatusError      Status = "Error"
)

// KeepUntil values, Jellyfin's names; the first is the default.
var KeepUntil = []string{"UntilDeleted", "UntilSpaceNeeded", "UntilWatched", "UntilDate"}

// Days are the days of the week a series timer names, Jellyfin's
// DayOfWeek names from Sunday.
var Days = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

// MaxPadding bounds a timer's padding, in seconds, as the settings' does.
const MaxPadding = accounts.MaxRecordingPadding

// MaxKeepUpTo bounds how many recordings a series timer keeps.
const MaxKeepUpTo = 1000

// Programme is what a timer keeps of the programme it records.
type Programme struct {
	Channel accounts.ID
	// Program is the programme's item, nil when the timer was made for a
	// time rather than for a programme.
	Program  *accounts.ID
	Name     string
	Overview string
	Genres   []string
	Rating   string
	Image    string
	Start    time.Time
	End      time.Time
}

// ProgrammeOf describes a programme item of the guide.
func ProgrammeOf(item library.Item) Programme {
	id := item.ID
	p := Programme{Program: &id, Name: item.Name, Overview: item.Overview, Genres: slices.Clone(item.Genres),
		Rating: item.OfficialRating, Image: item.Images.Primary}
	if item.Channel != nil {
		p.Channel = item.Channel.ID
	}
	if item.StartDate != nil {
		p.Start = *item.StartDate
	}
	if item.EndDate != nil {
		p.End = *item.EndDate
	}
	return p
}

// Options are how a timer records: the seconds it starts before the
// programme and goes on after it, Jellyfin's KeepUntil and priority.
type Options struct {
	PrePadding  int
	PostPadding int
	KeepUntil   string
	Priority    int
}

func (o Options) valid() bool {
	return o.PrePadding >= 0 && o.PrePadding <= MaxPadding && o.PostPadding >= 0 && o.PostPadding <= MaxPadding &&
		slices.Contains(KeepUntil, o.KeepUntil)
}

// Timer records one programme.
type Timer struct {
	ID accounts.ID
	// User made the timer: the recording reads the channel as they reach
	// it, and counts toward their live streams.
	User        accounts.ID
	SeriesTimer *accounts.ID
	Programme
	Options
	Status Status
	// Manual is set for a timer made or kept by hand, which its series
	// timer does not change.
	Manual  bool
	Created time.Time
}

// From and Until are when the timer records, padding included.
func (t Timer) From() time.Time  { return t.Start.Add(-time.Duration(t.PrePadding) * time.Second) }
func (t Timer) Until() time.Time { return t.End.Add(time.Duration(t.PostPadding) * time.Second) }

// SeriesTimer records every upcoming programme of a title, Jellyfin's
// SeriesTimerInfo.
type SeriesTimer struct {
	ID   accounts.ID
	User accounts.ID
	// Programme is the programme the series timer was made from: its
	// channel and time of day are those it records on, unless
	// RecordAnyChannel or RecordAnyTime.
	Programme
	Options
	SeriesOptions
	Created time.Time
}

// SeriesOptions are what a series timer records, Jellyfin's fields.
type SeriesOptions struct {
	RecordAnyChannel      bool
	RecordAnyTime         bool
	RecordNewOnly         bool
	SkipEpisodesInLibrary bool
	Days                  []string
	KeepUpTo              int
}

func (o SeriesOptions) valid() bool {
	return o.KeepUpTo >= 0 && o.KeepUpTo <= MaxKeepUpTo && !slices.ContainsFunc(o.Days, func(d string) bool { return !slices.Contains(Days, d) })
}

// Recording is a file of the recordings folder.
type Recording struct {
	ID          accounts.ID
	Timer       *accounts.ID
	SeriesTimer *accounts.ID
	// User made the timer, nil once they are deleted.
	User *accounts.ID
	Programme
	Started time.Time
	// Ended is nil while the recording is written.
	Ended *time.Time
	// Partial is set for a recording stopped before its end.
	Partial bool
	// File is the file's name in the recordings folder; Size its size.
	File string
	Size int64
}

// InProgress reports whether the recording is still written.
func (r Recording) InProgress() bool { return r.Ended == nil }

const timerColumns = "id, user_id, series_timer_id, channel_id, program_id, name, overview, genres, rating, image, start_at, end_at, " +
	"pre_padding, post_padding, keep_until, priority, status, is_manual, created_at"

func scanTimer(row pgx.Row) (Timer, error) {
	var t Timer
	err := row.Scan(&t.ID, &t.User, &t.SeriesTimer, &t.Channel, &t.Program, &t.Name, &t.Overview, &t.Genres, &t.Rating, &t.Image,
		&t.Start, &t.End, &t.PrePadding, &t.PostPadding, &t.KeepUntil, &t.Priority, &t.Status, &t.Manual, &t.Created)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

const seriesColumns = "id, user_id, channel_id, program_id, name, overview, start_at, end_at, record_any_channel, record_any_time, " +
	"record_new_only, skip_episodes_in_library, days, keep_up_to, keep_until, priority, pre_padding, post_padding, created_at"

func scanSeries(row pgx.Row) (SeriesTimer, error) {
	var s SeriesTimer
	err := row.Scan(&s.ID, &s.User, &s.Channel, &s.Program, &s.Name, &s.Overview, &s.Start, &s.End, &s.RecordAnyChannel, &s.RecordAnyTime,
		&s.RecordNewOnly, &s.SkipEpisodesInLibrary, &s.Days, &s.KeepUpTo, &s.KeepUntil, &s.Priority, &s.PrePadding, &s.PostPadding, &s.Created)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, ErrNotFound
	}
	return s, err
}

const recordingColumns = "id, timer_id, series_timer_id, user_id, channel_id, program_id, name, overview, genres, rating, image, " +
	"start_at, end_at, started_at, ended_at, partial, file, size"

func scanRecording(row pgx.Row) (Recording, error) {
	var r Recording
	err := row.Scan(&r.ID, &r.Timer, &r.SeriesTimer, &r.User, &r.Channel, &r.Program, &r.Name, &r.Overview, &r.Genres, &r.Rating, &r.Image,
		&r.Start, &r.End, &r.Started, &r.Ended, &r.Partial, &r.File, &r.Size)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// Timers lists the timers not completed, by start, as Jellyfin lists them.
func (s *Service) Timers(ctx context.Context) ([]Timer, error) {
	rows, err := s.db.Query(ctx, "SELECT "+timerColumns+" FROM live_timers WHERE status <> 'Completed' ORDER BY start_at, id")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Timer, error) { return scanTimer(row) })
}

// Timer returns a timer not completed.
func (s *Service) Timer(ctx context.Context, id accounts.ID) (Timer, error) {
	return scanTimer(s.db.QueryRow(ctx, "SELECT "+timerColumns+" FROM live_timers WHERE id = $1 AND status <> 'Completed'", id))
}

// CreateTimer has user's timer record programme p. A programme has one
// timer: one cancelled or completed is scheduled again, as Jellyfin does,
// and any other refused with ErrScheduled.
func (s *Service) CreateTimer(ctx context.Context, user accounts.ID, p Programme, o Options) (Timer, error) {
	if !s.Available() {
		return Timer{}, ErrUnavailable
	}
	if !o.valid() || p.Name == "" || !p.End.After(p.Start) {
		return Timer{}, ErrInvalid
	}
	var t Timer
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if p.Program != nil {
			existing, err := scanTimer(tx.QueryRow(ctx, "SELECT "+timerColumns+" FROM live_timers WHERE program_id = $1 FOR UPDATE", *p.Program))
			switch {
			case err == nil && (existing.Status == StatusCancelled || existing.Status == StatusCompleted):
				t, err = scanTimer(tx.QueryRow(ctx, "UPDATE live_timers SET status = 'New', is_manual = true WHERE id = $1 RETURNING "+timerColumns, existing.ID))
				return err
			case err == nil:
				return ErrScheduled
			case !errors.Is(err, ErrNotFound):
				return err
			}
		}
		var err error
		t, err = scanTimer(tx.QueryRow(ctx, `INSERT INTO live_timers (user_id, channel_id, program_id, name, overview, genres, rating, image,
				start_at, end_at, pre_padding, post_padding, keep_until, priority)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING `+timerColumns,
			user, p.Channel, p.Program, p.Name, p.Overview, nonNil(p.Genres), p.Rating, p.Image, p.Start, p.End,
			o.PrePadding, o.PostPadding, o.KeepUntil, o.Priority))
		return err
	})
	if uniqueViolation(err) {
		err = ErrScheduled
	}
	if err == nil {
		s.Wake()
	}
	return t, err
}

// UpdateTimer changes a timer's padding, unless it records already, as
// Jellyfin does.
func (s *Service) UpdateTimer(ctx context.Context, id accounts.ID, prePadding, postPadding int) error {
	if prePadding < 0 || prePadding > MaxPadding || postPadding < 0 || postPadding > MaxPadding {
		return ErrInvalid
	}
	t, err := s.Timer(ctx, id)
	if err != nil {
		return err
	}
	if t.Status != StatusInProgress {
		if _, err := s.db.Exec(ctx, "UPDATE live_timers SET pre_padding = $2, post_padding = $3 WHERE id = $1 AND status <> 'InProgress'",
			id, prePadding, postPadding); err != nil {
			return err
		}
		s.Wake()
	}
	return nil
}

// CancelTimer cancels a timer, stopping its recording, which is kept. A
// timer of a series timer stays, cancelled, so that the series timer does
// not make it again; any other is deleted.
func (s *Service) CancelTimer(ctx context.Context, id accounts.ID) error {
	t, err := s.Timer(ctx, id)
	if err != nil {
		return err
	}
	if t.SeriesTimer != nil {
		_, err = s.db.Exec(ctx, "UPDATE live_timers SET status = 'Cancelled', is_manual = true WHERE id = $1", id)
	} else {
		_, err = s.db.Exec(ctx, "DELETE FROM live_timers WHERE id = $1", id)
	}
	if err != nil {
		return err
	}
	s.stop(id)
	return nil
}

// ProgramTimers maps programmes to their timers not completed.
func (s *Service) ProgramTimers(ctx context.Context, programs []accounts.ID) (map[accounts.ID]Timer, error) {
	result := map[accounts.ID]Timer{}
	if len(programs) == 0 {
		return result, nil
	}
	rows, err := s.db.Query(ctx, "SELECT "+timerColumns+" FROM live_timers WHERE program_id = ANY($1) AND status <> 'Completed'", programs)
	if err != nil {
		return nil, err
	}
	timers, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Timer, error) { return scanTimer(row) })
	for _, t := range timers {
		result[*t.Program] = t
	}
	return result, err
}

// SeriesTimers lists the series timers by name.
func (s *Service) SeriesTimers(ctx context.Context) ([]SeriesTimer, error) {
	rows, err := s.db.Query(ctx, "SELECT "+seriesColumns+" FROM live_series_timers ORDER BY lower(name), id")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (SeriesTimer, error) { return scanSeries(row) })
}

// SeriesTimer returns a series timer.
func (s *Service) SeriesTimer(ctx context.Context, id accounts.ID) (SeriesTimer, error) {
	return scanSeries(s.db.QueryRow(ctx, "SELECT "+seriesColumns+" FROM live_series_timers WHERE id = $1", id))
}

// CreateSeriesTimer has user's series timer record the programmes of p's
// title. The timers made already for p, or for its title, become its own,
// kept as made by hand; then its timers are made from the guide.
func (s *Service) CreateSeriesTimer(ctx context.Context, user accounts.ID, p Programme, o Options, so SeriesOptions) (SeriesTimer, error) {
	if !s.Available() {
		return SeriesTimer{}, ErrUnavailable
	}
	if !o.valid() || !so.valid() || p.Name == "" {
		return SeriesTimer{}, ErrInvalid
	}
	var st SeriesTimer
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var err error
		st, err = scanSeries(tx.QueryRow(ctx, `INSERT INTO live_series_timers (user_id, channel_id, program_id, name, overview, start_at, end_at,
				record_any_channel, record_any_time, record_new_only, skip_episodes_in_library, days, keep_up_to, keep_until, priority,
				pre_padding, post_padding)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17) RETURNING `+seriesColumns,
			user, p.Channel, p.Program, p.Name, p.Overview, p.Start, p.End, so.RecordAnyChannel, so.RecordAnyTime, so.RecordNewOnly,
			so.SkipEpisodesInLibrary, nonNil(so.Days), so.KeepUpTo, o.KeepUntil, o.Priority, o.PrePadding, o.PostPadding))
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE live_timers SET series_timer_id = $1, is_manual = true
			WHERE status <> 'Completed' AND (program_id = $2 OR lower(name) = lower($3))`, st.ID, p.Program, p.Name)
		return err
	})
	if err != nil {
		return SeriesTimer{}, err
	}
	s.refreshSeries(ctx, st)
	return st, nil
}

// UpdateSeriesTimer changes what a series timer records, and its timers
// with it, but those recording and those kept by hand.
func (s *Service) UpdateSeriesTimer(ctx context.Context, id accounts.ID, o Options, so SeriesOptions) error {
	if !o.valid() || !so.valid() {
		return ErrInvalid
	}
	st, err := scanSeries(s.db.QueryRow(ctx, `UPDATE live_series_timers SET record_any_channel = $2, record_any_time = $3,
			record_new_only = $4, skip_episodes_in_library = $5, days = $6, keep_up_to = $7, keep_until = $8, priority = $9,
			pre_padding = $10, post_padding = $11
		WHERE id = $1 RETURNING `+seriesColumns,
		id, so.RecordAnyChannel, so.RecordAnyTime, so.RecordNewOnly, so.SkipEpisodesInLibrary, nonNil(so.Days), so.KeepUpTo,
		o.KeepUntil, o.Priority, o.PrePadding, o.PostPadding))
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `UPDATE live_timers SET pre_padding = $2, post_padding = $3, keep_until = $4, priority = $5
		WHERE series_timer_id = $1 AND status = 'New'`, id, o.PrePadding, o.PostPadding, o.KeepUntil, o.Priority); err != nil {
		return err
	}
	s.refreshSeries(ctx, st)
	return nil
}

// CancelSeriesTimer deletes a series timer and its timers, stopping their
// recordings, which are kept.
func (s *Service) CancelSeriesTimer(ctx context.Context, id accounts.ID) error {
	rows, err := s.db.Query(ctx, "DELETE FROM live_timers WHERE series_timer_id = $1 RETURNING id", id)
	if err != nil {
		return err
	}
	timers, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil {
		return err
	}
	tag, err := s.db.Exec(ctx, "DELETE FROM live_series_timers WHERE id = $1", id)
	if err != nil {
		return err
	}
	for _, timer := range timers {
		s.stop(timer)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Recordings lists the recordings, the latest first, as Jellyfin lists
// them.
func (s *Service) Recordings(ctx context.Context) ([]Recording, error) {
	rows, err := s.db.Query(ctx, "SELECT "+recordingColumns+" FROM live_recordings ORDER BY started_at DESC, id")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Recording, error) { return scanRecording(row) })
}

// Recording returns a recording.
func (s *Service) Recording(ctx context.Context, id accounts.ID) (Recording, error) {
	if !s.Available() {
		return Recording{}, ErrNotFound
	}
	return scanRecording(s.db.QueryRow(ctx, "SELECT "+recordingColumns+" FROM live_recordings WHERE id = $1", id))
}

// DeleteRecording deletes a recording and its file, stopping it first when
// it is still written.
func (s *Service) DeleteRecording(ctx context.Context, id accounts.ID) error {
	r, err := s.Recording(ctx, id)
	if err != nil {
		return err
	}
	if r.InProgress() && r.Timer != nil {
		s.stopAndWait(*r.Timer)
	}
	return s.delete(ctx, id)
}

// delete removes a recording, its files and what was learned of them.
func (s *Service) delete(ctx context.Context, id accounts.ID) error {
	r, err := scanRecording(s.db.QueryRow(ctx, "DELETE FROM live_recordings WHERE id = $1 RETURNING "+recordingColumns, id))
	if err != nil {
		return err
	}
	s.removeFiles(r)
	s.recorder.ForgetFile(id)
	// The analysis and keyframes of its file, which plays as a version
	// named after it.
	if _, err := s.db.Exec(ctx, "DELETE FROM media_analyses WHERE version_id = $1", id); err != nil {
		s.logger.Warn("The analysis of a deleted recording could not be removed", "error", err)
	}
	if _, err := s.db.Exec(ctx, "DELETE FROM media_keyframes WHERE version_id = $1", id); err != nil {
		s.logger.Warn("The keyframes of a deleted recording could not be removed", "error", err)
	}
	return nil
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Allows reports whether user's parental control and blocked genres let
// them reach an item of kind (one of accounts.UnratedKinds) rated rating,
// of genres, as titles are judged: a blocked genre hides it, and a rating
// above the user's, or none when the kind is hidden unrated.
func Allows(user accounts.User, kind, rating string, genres []string) bool {
	for _, genre := range genres {
		genre = strings.TrimSpace(genre)
		if slices.ContainsFunc(user.BlockedGenres, func(b string) bool { return strings.EqualFold(genre, b) }) {
			return false
		}
	}
	return !user.Parental.Judges(kind) || user.Parental.Allows(kind, rating)
}
