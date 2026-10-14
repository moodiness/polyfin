// Package reminders tells users, a few minutes before a Live TV programme
// they chose starts, that it is about to: on their own notification
// targets, and on their Jellyfin apps that accept messages when they want
// it. A user sets a reminder for one programme, or for every airing of its
// title on its channel at the same time of day, as a series timer finds
// them. Reminders are kept in the database: a restart neither sends one
// twice nor forgets one.
//
// A reminder is checked against the guide when it is due: a programme
// moved within the hour keeps its reminder at its new start, and one gone
// from the guide sends nothing. A reminder is only ever sent before its
// programme begins, and only to a user who may still see the programme:
// with the Live TV permission, an account neither disabled nor ended, and
// a programme their parental control and blocked genres allow.
package reminders

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/recordings"
)

const (
	// DefaultBefore is how many minutes before its programme a reminder
	// comes, unless the user chose otherwise.
	DefaultBefore = 5
	// moveReach is how far from its start a programme may move in the
	// guide and keep its reminder.
	moveReach = time.Hour
	// lookEvery is how often a reminder not due yet, whose programme could
	// have moved early enough to be due, has the guide looked at.
	lookEvery = 5 * time.Minute
	// guideReach is how far ahead series reminders look in the guide.
	guideReach = 8 * 24 * time.Hour
	// seriesEvery is how often series reminders look for new airings.
	seriesEvery = 30 * time.Minute
	// maxWait bounds how long the service sleeps between looks at the
	// reminders.
	maxWait = time.Minute
	// keepHandled is how long a reminder sent or given up is kept after
	// its programme ended: a series reminder finds it, and makes no other
	// for the same airing.
	keepHandled = 2 * 24 * time.Hour
)

// Befores are the minutes a user may choose to be reminded before a
// programme, the default included.
var Befores = []int{1, 5, 10, 15, 30, 60}

var (
	// ErrNotFound reports a programme that is not in the user's guide, or
	// that their parental control or blocked genres keep from them, and a
	// reminder the user has none of.
	ErrNotFound = errors.New("reminder not found")
	// ErrForbidden reports a user without the Live TV permission.
	ErrForbidden = errors.New("no Live TV permission")
	// ErrStarted reports a programme that already started.
	ErrStarted = errors.New("the programme already started")
	// ErrInvalid reports minutes that are not one of Befores.
	ErrInvalid = errors.New("invalid reminder settings")
)

// Guide finds the programmes and channels of a user's guide;
// library.Service is one.
type Guide interface {
	Guide(ctx context.Context, user accounts.User, q library.GuideQuery) ([]library.Item, error)
	Item(ctx context.Context, user accounts.User, id accounts.ID) (library.Item, error)
	Channels(ctx context.Context, user accounts.User) ([]library.Item, error)
}

// Users are the accounts; accounts.Store is one.
type Users interface {
	User(ctx context.Context, id accounts.ID) (accounts.User, error)
}

// Teller tells a reminder due to the user's own targets, and returns its
// title and message for the user's apps; notifications.Service is one.
type Teller interface {
	ProgrammeStarting(due Due) (title, message string)
}

// Apps shows a message on the user's Jellyfin apps that accept messages;
// jellyfin.Handler is one.
type Apps interface {
	ShowMessage(ctx context.Context, user accounts.ID, header, text string) int
}

// Config are the dependencies of the service.
type Config struct {
	DB     *pgxpool.Pool
	Guide  Guide
	Users  Users
	Teller Teller
	// Apps shows reminders on the apps of the users who want it; nil shows
	// none.
	Apps Apps
	// Reaches tells whether one of the user's own targets would receive a
	// reminder; nil tells none would.
	Reaches func(user accounts.ID) bool
	Logger  *slog.Logger
	// Ready is closed once the Teller can tell: Run sends nothing before.
	// Nil is ready.
	Ready <-chan struct{}
	// Now is the clock; nil is the system's.
	Now func() time.Time
}

// Reminder is a reminder of one programme, set by hand or made by a series
// reminder (Series).
type Reminder struct {
	ID          accounts.ID
	User        accounts.ID
	Series      *accounts.ID
	Program     accounts.ID
	Channel     accounts.ID
	ChannelName string
	Name        string
	Genres      []string
	Rating      string
	Start, End  time.Time
	// Language is the web client's when the reminder was set, "en" or
	// "fr", which its message is written in; empty for the server's.
	Language string
}

// SeriesReminder reminds its user of every airing of a title on a channel
// at the time of day of Start, within ten minutes. Next is the start of
// the next airing it reminds of, nil when none is in the guide.
type SeriesReminder struct {
	ID          accounts.ID
	Channel     accounts.ID
	ChannelName string
	Name        string
	Start       time.Time
	Language    string
	Next        *time.Time
}

// Settings are a user's: how many minutes before its programme a reminder
// comes, one of Befores, and whether it also shows on their apps.
type Settings struct {
	Before int
	OnApps bool
}

// Due is a reminder to tell now: its user, and how long until its
// programme starts.
type Due struct {
	User     accounts.User
	Reminder Reminder
	Left     time.Duration
}

// State is how a programme stands for a user: whether it started, the
// reminder waiting for it, if any, and the series reminder that made it;
// Reaches tells whether a reminder would reach the user, on a target of
// theirs that chose it or on their apps.
type State struct {
	Program  library.Item
	Started  bool
	Reminder *Reminder
	Series   *SeriesReminder
	Reaches  bool
}

// Service keeps the reminders and tells them when due.
type Service struct {
	db      *pgxpool.Pool
	guide   Guide
	users   Users
	teller  Teller
	apps    Apps
	reaches func(accounts.ID) bool
	logger  *slog.Logger
	now     func() time.Time
	ready   <-chan struct{}

	wake chan struct{}
	// refresh asks the series reminders to look at the guide soon.
	refresh chan struct{}
	// checkMu serializes the looks at the reminders due, and guards
	// looked, when each reminder not due yet last had the guide looked at.
	checkMu sync.Mutex
	looked  map[accounts.ID]time.Time
	// seriesMu serializes the looks at the series reminders.
	seriesMu sync.Mutex
}

// New returns the reminders service.
func New(c Config) *Service {
	s := &Service{db: c.DB, guide: c.Guide, users: c.Users, teller: c.Teller, apps: c.Apps, reaches: c.Reaches, logger: c.Logger,
		now: c.Now, ready: c.Ready, wake: make(chan struct{}, 1), refresh: make(chan struct{}, 1), looked: map[accounts.ID]time.Time{}}
	if s.now == nil {
		s.now = time.Now
	}
	if s.reaches == nil {
		s.reaches = func(accounts.ID) bool { return false }
	}
	return s
}

// Run tells the reminders due until ctx ends: those due while Polyfin was
// off first, when their programme has not begun, then each as it comes.
// Series reminders look for new airings at start, after a series reminder
// is set, and every half hour.
func (s *Service) Run(ctx context.Context) {
	if s.ready != nil {
		select {
		case <-ctx.Done():
			return
		case <-s.ready:
		}
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		timer := time.NewTimer(0)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.refresh:
			case <-timer.C:
				timer.Reset(seriesEvery)
			}
			s.RefreshSeries(ctx)
			s.Wake()
		}
	})
	wg.Go(func() {
		timer := time.NewTimer(0)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
			case <-timer.C:
			}
			s.Check(ctx)
			timer.Reset(s.untilNext(ctx))
		}
	})
	wg.Wait()
}

// Wake has the service look at the reminders now.
func (s *Service) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// untilNext is how long until the next reminder is due, or could be if
// its programme moved, at most maxWait.
func (s *Service) untilNext(ctx context.Context) time.Duration {
	var next *time.Time
	err := s.db.QueryRow(ctx, `SELECT min(r.start_at - make_interval(mins => coalesce(p.before_minutes, $1)))
		FROM programme_reminders r LEFT JOIN reminder_settings p ON p.user_id = r.user_id WHERE r.handled_at IS NULL`,
		DefaultBefore).Scan(&next)
	if err != nil || next == nil {
		return maxWait
	}
	return min(max(next.Sub(s.now()), time.Second), maxWait)
}

// language keeps "en" and "fr", from a client's language such as "fr-FR";
// anything else is the server's.
func language(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	for _, known := range []string{"en", "fr"} {
		if raw == known || strings.HasPrefix(raw, known+"-") || strings.HasPrefix(raw, known+"_") {
			return known
		}
	}
	return ""
}

// allowed reports whether user may see programme p, with the Live TV
// permission at now: as scheduled programmes, by parental control and
// blocked genres.
func allowed(user accounts.User, rating string, genres []string) bool {
	return recordings.Allows(user, recordings.ProgramKind, rating, genres)
}

// program finds a programme of user's guide that they may see.
func (s *Service) program(ctx context.Context, user accounts.User, id accounts.ID) (library.Item, error) {
	if !user.LiveTv {
		return library.Item{}, ErrForbidden
	}
	program, err := s.guide.Item(ctx, user, id)
	if errors.Is(err, library.ErrNotFound) {
		return library.Item{}, ErrNotFound
	}
	if err != nil {
		return library.Item{}, err
	}
	if program.Kind != library.KindProgram || program.Channel == nil || program.StartDate == nil || program.EndDate == nil ||
		!allowed(user, program.OfficialRating, program.Genres) {
		return library.Item{}, ErrNotFound
	}
	return program, nil
}

// reminderOf describes the reminder of a programme.
func reminderOf(user accounts.ID, program library.Item, lang string) Reminder {
	return Reminder{User: user, Program: program.ID, Channel: program.Channel.ID, ChannelName: program.Channel.Name,
		Name: strings.TrimSpace(program.Name), Genres: nonNil(program.Genres), Rating: program.OfficialRating, Start: *program.StartDate,
		End: *program.EndDate, Language: lang}
}

func nonNil(genres []string) []string {
	if genres == nil {
		return []string{}
	}
	return slices.Clone(genres)
}

const reminderColumns = `id, user_id, series_id, program_id, channel_id, channel_name, name, genres, rating, start_at, end_at, language`

func scanReminder(row pgx.Row) (Reminder, error) {
	var r Reminder
	err := row.Scan(&r.ID, &r.User, &r.Series, &r.Program, &r.Channel, &r.ChannelName, &r.Name, &r.Genres, &r.Rating, &r.Start, &r.End,
		&r.Language)
	return r, err
}

const seriesColumns = `id, channel_id, channel_name, name, start_at, language`

func scanSeries(row pgx.Row) (SeriesReminder, error) {
	var r SeriesReminder
	err := row.Scan(&r.ID, &r.Channel, &r.ChannelName, &r.Name, &r.Start, &r.Language)
	return r, err
}

// State tells how the programme id stands for user.
func (s *Service) State(ctx context.Context, user accounts.User, id accounts.ID) (State, error) {
	program, err := s.program(ctx, user, id)
	if err != nil {
		return State{}, err
	}
	return s.state(ctx, user, program)
}

func (s *Service) state(ctx context.Context, user accounts.User, program library.Item) (State, error) {
	state := State{Program: program, Started: !program.StartDate.After(s.now())}
	settings, err := s.Settings(ctx, user.ID)
	if err != nil {
		return State{}, err
	}
	state.Reaches = settings.OnApps || s.reaches(user.ID)
	reminder, err := scanReminder(s.db.QueryRow(ctx, "SELECT "+reminderColumns+` FROM programme_reminders
		WHERE user_id = $1 AND program_id = $2 AND handled_at IS NULL`, user.ID, program.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return State{}, err
	}
	state.Reminder = &reminder
	if reminder.Series != nil {
		series, err := scanSeries(s.db.QueryRow(ctx, "SELECT "+seriesColumns+" FROM programme_series_reminders WHERE id = $1", *reminder.Series))
		if err != nil {
			return State{}, err
		}
		series.Next = &reminder.Start
		state.Series = &series
	}
	return state, nil
}

// Create sets a reminder for user of the programme id, which has not
// started, or with series of every airing of its title on its channel at
// its time of day; lang is the language of the client that sets it. A
// programme with a reminder keeps it; one of a series already set joins
// that series.
func (s *Service) Create(ctx context.Context, user accounts.User, id accounts.ID, series bool, lang string) (State, error) {
	program, err := s.program(ctx, user, id)
	if err != nil {
		return State{}, err
	}
	if !program.StartDate.After(s.now()) {
		return State{}, ErrStarted
	}
	r := reminderOf(user.ID, program, language(lang))
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		// The user is locked, so that two series reminders set at once for
		// the same title do not make two.
		if _, err := tx.Exec(ctx, "SELECT 1 FROM users WHERE id = $1 FOR UPDATE", user.ID); err != nil {
			return err
		}
		if series {
			found, err := s.sameSeries(ctx, tx, user.ID, r)
			if err != nil {
				return err
			}
			if found == nil {
				var id accounts.ID
				if err := tx.QueryRow(ctx, `INSERT INTO programme_series_reminders (user_id, channel_id, channel_name, name, start_at, language)
					VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, user.ID, r.Channel, r.ChannelName, r.Name, r.Start, r.Language).Scan(&id); err != nil {
					return err
				}
				found = &id
			}
			r.Series = found
		}
		// A reminder handled already, sent or given up, is set again; one
		// waiting stays, and joins the series set.
		_, err := tx.Exec(ctx, `INSERT INTO programme_reminders (user_id, series_id, program_id, channel_id, channel_name, name, genres, rating,
				start_at, end_at, language)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (user_id, program_id) DO UPDATE SET
				series_id = coalesce(excluded.series_id, CASE WHEN programme_reminders.handled_at IS NULL THEN programme_reminders.series_id END),
				channel_name = excluded.channel_name, name = excluded.name, genres = excluded.genres, rating = excluded.rating,
				start_at = excluded.start_at, end_at = excluded.end_at,
				language = CASE WHEN programme_reminders.handled_at IS NULL THEN programme_reminders.language ELSE excluded.language END,
				handled_at = NULL, sent = false`,
			user.ID, r.Series, r.Program, r.Channel, r.ChannelName, r.Name, r.Genres, r.Rating, r.Start, r.End, r.Language)
		return err
	})
	if err != nil {
		return State{}, err
	}
	s.logger.Info("A programme reminder was set", "user_id", user.ID.String(), "program", program.ID.String(), "series", series)
	if series {
		select {
		case s.refresh <- struct{}{}:
		default:
		}
	}
	s.Wake()
	return s.state(ctx, user, program)
}

// sameSeries finds the series reminder of user that already reminds of
// r's airings: the same title on the same channel at the same time of day,
// within ten minutes.
func (s *Service) sameSeries(ctx context.Context, tx pgx.Tx, user accounts.ID, r Reminder) (*accounts.ID, error) {
	rows, err := tx.Query(ctx, "SELECT "+seriesColumns+" FROM programme_series_reminders WHERE user_id = $1 AND channel_id = $2 ORDER BY created_at, id",
		user, r.Channel)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (SeriesReminder, error) { return scanSeries(row) })
	if err != nil {
		return nil, err
	}
	program := library.Item{Kind: library.KindProgram, Name: r.Name, StartDate: &r.Start, EndDate: &r.End,
		Channel: &library.Item{ID: r.Channel}}
	for _, series := range list {
		if recordings.Matches(seriesTimer(series), program, nil, r.Start) {
			return &series.ID, nil
		}
	}
	return nil, nil
}

// seriesTimer is the series timer that would record the airings a series
// reminder reminds of.
func seriesTimer(series SeriesReminder) recordings.SeriesTimer {
	return recordings.SeriesTimer{Programme: recordings.Programme{Channel: series.Channel, Name: series.Name, Start: series.Start}}
}

// Delete deletes the reminder id of user: a reminder set by hand, or a
// series reminder with the reminders it made. A reminder a series reminder
// made is given up rather than deleted, so that the series does not make
// it again.
func (s *Service) Delete(ctx context.Context, user accounts.ID, id accounts.ID) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM programme_series_reminders WHERE id = $1 AND user_id = $2", id, user)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if tag, err = s.db.Exec(ctx, "DELETE FROM programme_reminders WHERE id = $1 AND user_id = $2 AND series_id IS NULL AND handled_at IS NULL",
			id, user); err != nil {
			return err
		}
	}
	if tag.RowsAffected() == 0 {
		if tag, err = s.db.Exec(ctx, `UPDATE programme_reminders SET handled_at = $3, sent = false
			WHERE id = $1 AND user_id = $2 AND handled_at IS NULL`, id, user, s.now()); err != nil {
			return err
		}
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	s.logger.Info("A programme reminder was deleted", "user_id", user.String(), "reminder", id.String())
	return nil
}

// List lists user's reminders set by hand, waiting, by start, and their
// series reminders, by name, each with its next airing: those of channels
// the user still reaches, and programmes their parental control and
// blocked genres allow.
func (s *Service) List(ctx context.Context, user accounts.User) ([]Reminder, []SeriesReminder, error) {
	if !user.LiveTv {
		return []Reminder{}, []SeriesReminder{}, nil
	}
	channels, err := s.guide.Channels(ctx, user)
	if err != nil {
		return nil, nil, err
	}
	reached := make(map[accounts.ID]bool, len(channels))
	for _, channel := range channels {
		reached[channel.ID] = true
	}
	rows, err := s.db.Query(ctx, "SELECT "+reminderColumns+` FROM programme_reminders
		WHERE user_id = $1 AND handled_at IS NULL AND start_at > $2 ORDER BY start_at, id`, user.ID, s.now())
	if err != nil {
		return nil, nil, err
	}
	waiting, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Reminder, error) { return scanReminder(row) })
	if err != nil {
		return nil, nil, err
	}
	rows, err = s.db.Query(ctx, "SELECT "+seriesColumns+" FROM programme_series_reminders WHERE user_id = $1 ORDER BY lower(name), start_at, id",
		user.ID)
	if err != nil {
		return nil, nil, err
	}
	series, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (SeriesReminder, error) { return scanSeries(row) })
	if err != nil {
		return nil, nil, err
	}
	reminders := []Reminder{}
	next := map[accounts.ID]time.Time{}
	for _, r := range waiting {
		if !reached[r.Channel] || !allowed(user, r.Rating, r.Genres) {
			continue
		}
		if r.Series == nil {
			reminders = append(reminders, r)
		} else if _, ok := next[*r.Series]; !ok {
			next[*r.Series] = r.Start
		}
	}
	kept := []SeriesReminder{}
	for _, sr := range series {
		if !reached[sr.Channel] {
			continue
		}
		if start, ok := next[sr.ID]; ok {
			sr.Next = &start
		}
		kept = append(kept, sr)
	}
	return reminders, kept, nil
}

// Settings reads user's settings, the defaults when they chose none.
func (s *Service) Settings(ctx context.Context, user accounts.ID) (Settings, error) {
	settings := Settings{Before: DefaultBefore}
	err := s.db.QueryRow(ctx, "SELECT before_minutes, on_apps FROM reminder_settings WHERE user_id = $1", user).Scan(&settings.Before,
		&settings.OnApps)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	return settings, err
}

// SetSettings saves user's settings: Before must be one of Befores.
func (s *Service) SetSettings(ctx context.Context, user accounts.ID, settings Settings) error {
	if !slices.Contains(Befores, settings.Before) {
		return ErrInvalid
	}
	_, err := s.db.Exec(ctx, `INSERT INTO reminder_settings (user_id, before_minutes, on_apps) VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET before_minutes = excluded.before_minutes, on_apps = excluded.on_apps`,
		user, settings.Before, settings.OnApps)
	if err == nil {
		s.Wake()
	}
	return err
}

func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
