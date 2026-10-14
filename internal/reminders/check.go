package reminders

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/recordings"
)

// watched is a reminder waiting, with its user's settings.
type watched struct {
	Reminder
	before time.Duration
	onApps bool
}

// Check tells the reminders due, each once, after looking at the guide:
// a programme moved within the hour keeps its reminder at its new start,
// told then; one gone, or started, or that its user may no longer see,
// is given up. A reminder not due yet, whose programme would be if it had
// moved up to an hour earlier, has the guide looked at every few minutes.
func (s *Service) Check(ctx context.Context) {
	s.checkMu.Lock()
	defer s.checkMu.Unlock()
	now := s.now()
	rows, err := s.db.Query(ctx, "SELECT "+prefixed("r.", reminderColumns)+`, coalesce(p.before_minutes, $1), coalesce(p.on_apps, false)
		FROM programme_reminders r LEFT JOIN reminder_settings p ON p.user_id = r.user_id
		WHERE r.handled_at IS NULL AND r.start_at - make_interval(mins => coalesce(p.before_minutes, $1)) <= $2
		ORDER BY r.start_at, r.id`, DefaultBefore, now.Add(moveReach))
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("The programme reminders could not be read", "error", err)
		}
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (watched, error) {
		var w watched
		var minutes int
		err := row.Scan(&w.ID, &w.User, &w.Series, &w.Program, &w.Channel, &w.ChannelName, &w.Name, &w.Genres, &w.Rating, &w.Start, &w.End,
			&w.Language, &minutes, &w.onApps)
		w.before = time.Duration(minutes) * time.Minute
		return w, err
	})
	if err != nil {
		s.logger.Warn("The programme reminders could not be read", "error", err)
		return
	}
	kept := make(map[accounts.ID]bool, len(list))
	for _, w := range list {
		kept[w.ID] = true
		due := !w.Start.Add(-w.before).After(now)
		if !due && now.Sub(s.looked[w.ID]) < lookEvery {
			continue
		}
		s.looked[w.ID] = now
		s.follow(ctx, w, now)
	}
	for id := range s.looked {
		if !kept[id] {
			delete(s.looked, id)
		}
	}
}

// prefixed qualifies each of a list of columns with prefix.
func prefixed(prefix, columns string) string {
	names := strings.Split(columns, ", ")
	for i, name := range names {
		names[i] = prefix + name
	}
	return strings.Join(names, ", ")
}

// follow looks at w's programme in the guide, moves w with it, and tells
// it when due. Before it is due, the guide is only looked at for a move:
// what would give the reminder up waits until it is due.
func (s *Service) follow(ctx context.Context, w watched, now time.Time) {
	if !w.Start.After(now) {
		s.giveUp(ctx, w.Reminder, "its programme started")
		return
	}
	due := !w.Start.Add(-w.before).After(now)
	user, err := s.users.User(ctx, w.User)
	if err != nil {
		if !errors.Is(err, accounts.ErrNotFound) && ctx.Err() == nil {
			s.logger.Warn("A programme reminder's user could not be read", "error", err)
		}
		return
	}
	if user.Refused(now) || !user.LiveTv {
		if due {
			s.giveUp(ctx, w.Reminder, "its user may not watch Live TV")
		}
		return
	}
	programs, err := s.guide.Guide(ctx, user, library.GuideQuery{From: w.Start.Add(-moveReach), To: w.Start.Add(moveReach),
		Channels: []accounts.ID{w.Channel}})
	if err != nil {
		// Asked again at the next look, until the programme starts.
		if ctx.Err() == nil {
			s.logger.Warn("The guide could not be read for a programme reminder", "error", err)
		}
		return
	}
	program, ok := moved(w.Reminder, programs)
	if !ok || !allowed(user, program.OfficialRating, program.Genres) || !program.StartDate.After(now) {
		if due {
			s.giveUp(ctx, w.Reminder, "its programme is gone from the guide, started or not allowed")
		}
		return
	}
	if program.ID != w.Program || !program.StartDate.Equal(w.Start) || !program.EndDate.Equal(w.End) {
		_, err := s.db.Exec(ctx, `UPDATE programme_reminders SET program_id = $2, start_at = $3, end_at = $4 WHERE id = $1 AND handled_at IS NULL`,
			w.ID, program.ID, *program.StartDate, *program.EndDate)
		if uniqueViolation(err) {
			// Another reminder of the user's has the programme.
			s.giveUp(ctx, w.Reminder, "another reminder has its programme")
			return
		}
		if err != nil {
			s.logger.Warn("A programme reminder could not follow its programme", "error", err)
			return
		}
		if !program.StartDate.Equal(w.Start) {
			s.logger.Info("A programme reminder follows its programme, moved", "reminder", w.ID.String(), "from", w.Start, "to", *program.StartDate)
		}
		w.Program, w.Start, w.End = program.ID, *program.StartDate, *program.EndDate
	}
	if w.Start.Add(-w.before).After(now) {
		return
	}
	s.tell(ctx, user, w, now)
}

// moved finds r's programme among programs, which the guide lists around
// its start on its channel: the same programme, else the programme of the
// same title whose start is nearest, within the hour.
func moved(r Reminder, programs []library.Item) (library.Item, bool) {
	var found library.Item
	best := time.Duration(-1)
	for _, p := range programs {
		if p.Kind != library.KindProgram || p.StartDate == nil || p.EndDate == nil || p.Channel == nil || p.Channel.ID != r.Channel ||
			!strings.EqualFold(strings.TrimSpace(p.Name), strings.TrimSpace(r.Name)) {
			continue
		}
		distance := p.StartDate.Sub(r.Start).Abs()
		if distance > moveReach {
			continue
		}
		if p.ID == r.Program {
			return p, true
		}
		if best < 0 || distance < best {
			found, best = p, distance
		}
	}
	return found, best >= 0
}

// tell marks w sent, so that nothing sends it again, then tells it.
func (s *Service) tell(ctx context.Context, user accounts.User, w watched, now time.Time) {
	tag, err := s.db.Exec(ctx, "UPDATE programme_reminders SET handled_at = $2, sent = true WHERE id = $1 AND handled_at IS NULL", w.ID, now)
	if err != nil {
		s.logger.Warn("A programme reminder could not be marked sent", "error", err)
		return
	}
	if tag.RowsAffected() == 0 {
		return
	}
	title, message := s.teller.ProgrammeStarting(Due{User: user, Reminder: w.Reminder, Left: w.Start.Sub(now)})
	shown := 0
	if w.onApps && s.apps != nil {
		shown = s.apps.ShowMessage(ctx, user.ID, title, message)
	}
	s.logger.Info("A programme reminder was sent", "user_id", user.ID.String(), "reminder", w.ID.String(), "apps", shown)
}

// giveUp marks r handled without telling it.
func (s *Service) giveUp(ctx context.Context, r Reminder, why string) {
	if _, err := s.db.Exec(ctx, "UPDATE programme_reminders SET handled_at = $2, sent = false WHERE id = $1 AND handled_at IS NULL",
		r.ID, s.now()); err != nil {
		s.logger.Warn("A programme reminder could not be given up", "error", err)
		return
	}
	s.logger.Info("A programme reminder was given up", "reminder", r.ID.String(), "reason", why)
}

// RefreshSeries has every series reminder make reminders for the airings
// of its title the guide shows ahead, in the guide of its user, and
// deletes the reminders handled long ago.
func (s *Service) RefreshSeries(ctx context.Context) {
	s.seriesMu.Lock()
	defer s.seriesMu.Unlock()
	now := s.now()
	if _, err := s.db.Exec(ctx, "DELETE FROM programme_reminders WHERE handled_at IS NOT NULL AND end_at < $1", now.Add(-keepHandled)); err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("Old programme reminders could not be deleted", "error", err)
		}
		return
	}
	rows, err := s.db.Query(ctx, "SELECT user_id, "+seriesColumns+" FROM programme_series_reminders ORDER BY user_id, id")
	if err != nil {
		s.logger.Warn("The series reminders could not be read", "error", err)
		return
	}
	type owned struct {
		user accounts.ID
		SeriesReminder
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (owned, error) {
		var o owned
		err := row.Scan(&o.user, &o.ID, &o.Channel, &o.ChannelName, &o.Name, &o.Start, &o.Language)
		return o, err
	})
	if err != nil {
		s.logger.Warn("The series reminders could not be read", "error", err)
		return
	}
	for _, o := range list {
		if ctx.Err() != nil {
			return
		}
		s.refreshSeries(ctx, o.user, o.SeriesReminder, now)
	}
}

// refreshSeries makes reminders for the airings series reminds user of,
// from now: the programmes of its title on its channel at its time of day,
// within ten minutes, that the user may see. Airings that have a reminder,
// waiting or handled, keep it.
func (s *Service) refreshSeries(ctx context.Context, owner accounts.ID, series SeriesReminder, now time.Time) {
	user, err := s.users.User(ctx, owner)
	if err != nil || user.Refused(now) || !user.LiveTv {
		return
	}
	programs, err := s.guide.Guide(ctx, user, library.GuideQuery{From: now, To: now.Add(guideReach), Channels: []accounts.ID{series.Channel}})
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("The guide could not be read for a series reminder", "error", err)
		}
		return
	}
	timer := seriesTimer(series)
	made := 0
	for _, program := range programs {
		if !recordings.Matches(timer, program, nil, now) || !program.StartDate.After(now) || !allowed(user, program.OfficialRating, program.Genres) {
			continue
		}
		r := reminderOf(user.ID, program, series.Language)
		r.Series = &series.ID
		tag, err := s.db.Exec(ctx, `INSERT INTO programme_reminders (user_id, series_id, program_id, channel_id, channel_name, name, genres, rating,
				start_at, end_at, language)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) ON CONFLICT (user_id, program_id) DO NOTHING`,
			user.ID, r.Series, r.Program, r.Channel, r.ChannelName, r.Name, r.Genres, r.Rating, r.Start, r.End, r.Language)
		if err != nil {
			s.logger.Warn("A series reminder's airing could not be kept", "error", err)
			return
		}
		made += int(tag.RowsAffected())
	}
	if made > 0 {
		s.logger.Info("A series reminder found new airings", "series", series.ID.String(), "count", made)
	}
}
