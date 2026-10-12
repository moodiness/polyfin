package notifications

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/plays"
)

// The bounds of the lists of a summary: the movies and series of the
// server's week or month, of a user's, and of a year, the new episodes,
// the titles added to the local folders, and the users who joined and the
// problems found.
const (
	summaryServerTop = 5
	summaryUserTop   = 10
	summaryYearTop   = 10
	summaryEpisodes  = 20
	summaryAdded     = 10
	summaryList      = 50
)

// The scopes of a summary.
const (
	// ScopeServer is the server's week, month or year, which the server's
	// targets and administrators' own receive.
	ScopeServer = "server"
	// ScopeUser is a user's, which their own targets receive.
	ScopeUser = "user"
)

// summaryEvents are the summaries, in the order they are looked at: the
// weekly summary, and the month and year in review.
var summaryEvents = []string{WeeklySummary, MonthlySummary, YearlySummary}

// summaryColumns are the columns of notification_summary that keep the
// last week, month and year sent: the end of the week, and the first day
// of the month and of the year.
var summaryColumns = map[string]string{WeeklySummary: "week_end", MonthlySummary: "month_sent", YearlySummary: "year_sent"}

// summaryNames name the summaries in the log.
var summaryNames = map[string]string{WeeklySummary: "weekly summary", MonthlySummary: "month in review", YearlySummary: "year in review"}

// SummaryJSON is a summary of a week, a month or a year, from Start until
// before End: the server's (Scope "server") or a user's (Scope "user").
// Plays counts the videos played and Played how long they played, in
// seconds; Movies and Series are those played longest, the first five for
// the server's week or month, ten for a user and for a year. NewEpisodes
// counts the new episodes of the series followed, the first twenty of
// which are in Episodes; a year leaves them out. The server's summary also
// tells the Users who joined through an invite, the titles Added to the
// local folders, AddedFiles counting their files, and, but for a year, the
// Problems System › Health found; a user's leaves them empty. A year tells
// its TopMonth, the month played longest in.
type SummaryJSON struct {
	Scope       string               `json:"scope"`
	Start       time.Time            `json:"start"`
	End         time.Time            `json:"end"`
	Plays       int                  `json:"plays"`
	Played      int64                `json:"played"`
	Movies      []SummaryTitleJSON   `json:"movies"`
	Series      []SummaryTitleJSON   `json:"series"`
	NewEpisodes int                  `json:"newEpisodes"`
	Episodes    []SummaryEpisodeJSON `json:"episodes"`
	Users       []JoinedJSON         `json:"users"`
	AddedFiles  int                  `json:"addedFiles"`
	Added       []AddedJSON          `json:"added"`
	Problems    []FoundProblemJSON   `json:"problems"`
	TopMonth    *SummaryMonthJSON    `json:"topMonth,omitempty"`
}

// SummaryMonthJSON is a month of a year in review, as YYYY-MM in the
// server's time zone, with how many videos played in it and how long, in
// seconds.
type SummaryMonthJSON struct {
	Month  string `json:"month"`
	Plays  int    `json:"plays"`
	Played int64  `json:"played"`
}

// SummaryTitleJSON is a movie or a series played: its item as Jellyfin
// apps know it, its name, how many times and how long it played, in
// seconds.
type SummaryTitleJSON struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Plays  int    `json:"plays"`
	Played int64  `json:"played"`
}

// SummaryEpisodeJSON is a new episode, as it was found: its item and its
// series' as Jellyfin apps know them, its name, season and number.
type SummaryEpisodeJSON struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	SeriesID   string `json:"seriesId"`
	SeriesName string `json:"seriesName"`
	Season     int    `json:"season"`
	Number     int    `json:"number"`
}

// JoinedJSON is a user who joined through an invite: when, and the
// administrator who created the invite, null once either is deleted.
type JoinedJSON struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	JoinedAt  time.Time `json:"joinedAt"`
	InvitedBy *UserJSON `json:"invitedBy"`
}

// AddedJSON is a title files were added for to the local folders: a
// "movie" or a "show", by its name, an "album" of a music folder, by its
// title, or a "video" of a home videos folder, by its name, with how many
// files (an album's tracks).
type AddedJSON struct {
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Files int    `json:"files"`
}

// FoundProblemJSON is a problem System › Health found, as it was told.
type FoundProblemJSON struct {
	Key      string    `json:"key"`
	Severity string    `json:"severity"`
	Text     string    `json:"text"`
	FoundAt  time.Time `json:"foundAt"`
	// page is the page of the admin app that shows it.
	page string
}

// empty reports whether the summary has nothing to tell.
func (sum SummaryJSON) empty() bool {
	return sum.Plays == 0 && sum.NewEpisodes == 0 && len(sum.Users) == 0 && sum.AddedFiles == 0 && len(sum.Problems) == 0
}

// summaryDue is the end of the last week due at now: the latest day and
// hour of the settings, in zone, not after now.
func summaryDue(now time.Time, zone *time.Location, day, hour int) time.Time {
	local := now.In(zone)
	back := (int(local.Weekday()) - day + 7) % 7
	due := time.Date(local.Year(), local.Month(), local.Day()-back, hour, 0, 0, 0, zone)
	if due.After(now) {
		due = time.Date(local.Year(), local.Month(), local.Day()-back-7, hour, 0, 0, 0, zone)
	}
	return due
}

// weekBefore is the start of the week that ends at end: seven days
// earlier, at the same time of day in zone.
func weekBefore(end time.Time, zone *time.Location) time.Time {
	local := end.In(zone)
	return time.Date(local.Year(), local.Month(), local.Day()-7, local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), zone)
}

// review is the month, or for YearlySummary the year, that ends on the 1st
// of month at 0:00 in zone.
func review(event string, year int, month time.Month, zone *time.Location) (start, end time.Time) {
	end = time.Date(year, month, 1, 0, 0, 0, 0, zone)
	if event == YearlySummary {
		return time.Date(year-1, month, 1, 0, 0, 0, 0, zone), end
	}
	return time.Date(year, month-1, 1, 0, 0, 0, 0, zone), end
}

// reviewDue is the month, or for YearlySummary the year, whose review is
// due at now: the last one whose review's day came, the 1st of the month
// after it, or of January, at hour in zone. ok is false once the month
// that day is in ended: a review is sent within it only.
func reviewDue(event string, now time.Time, zone *time.Location, hour int) (start, end time.Time, ok bool) {
	local := now.In(zone)
	year, month := local.Year(), local.Month()
	if event == YearlySummary {
		month = time.January
	}
	if time.Date(year, month, 1, hour, 0, 0, 0, zone).After(now) {
		if event == YearlySummary {
			year--
		} else {
			month--
		}
	}
	start, end = review(event, year, month, zone)
	return start, end, now.Before(time.Date(year, month+1, 1, 0, 0, 0, 0, zone))
}

// reviewLast is the last month, or for YearlySummary the last year, that
// ended at now in zone.
func reviewLast(event string, now time.Time, zone *time.Location) (start, end time.Time) {
	local := now.In(zone)
	month := local.Month()
	if event == YearlySummary {
		month = time.January
	}
	return review(event, local.Year(), month, zone)
}

// due is the last week, month or year whose summary of event is due at
// now, by the settings, from start until before end, and key, which names
// it in notification_summary: the end of a week, the first day of a month
// or a year. ok is false while none may be sent.
func (s *Service) due(event string, now time.Time) (start, end, key time.Time, ok bool) {
	settings := s.accounts.Settings()
	if event == WeeklySummary {
		end = summaryDue(now, s.zone, settings.WeeklySummaryDay, settings.WeeklySummaryHour)
		return weekBefore(end, s.zone), end, end, true
	}
	start, end, ok = reviewDue(event, now, s.zone, settings.WeeklySummaryHour)
	return start, end, time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC), ok
}

// summaryMessage is a summary to send, and to which targets.
type summaryMessage struct {
	event     Event
	recipient func(target) bool
}

// CheckSummary sends each summary when it is due, in the server's time
// zone: the weekly summary once the day and hour of the settings came, for
// the week before; the month in review on the 1st of each month at that
// hour, for the month before; the year in review on the 1st of January at
// that hour, for the year before. Each is sent once: the last week, month
// and year sent are kept in the database, so that a restart does not send
// them again, and a server that was off at the hour sends them when it
// starts again, a month or year in review within the month it was due in
// only. A summary that cannot be made is tried again at the next check.
func (s *Service) CheckSummary(ctx context.Context) {
	s.summaryMu.Lock()
	defer s.summaryMu.Unlock()
	for _, event := range summaryEvents {
		s.checkSummary(ctx, event)
	}
}

// checkSummary sends the summary of event when it is due (see
// CheckSummary).
func (s *Service) checkSummary(ctx context.Context, event string) {
	start, end, key, ok := s.due(event, s.now())
	if !ok || !key.After(s.summarySent[event]) {
		return
	}
	column := summaryColumns[event]
	var messages []summaryMessage
	sent := false
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var last time.Time
		err := tx.QueryRow(ctx, "SELECT "+column+" FROM notification_summary FOR UPDATE").Scan(&last)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if !key.After(last) {
			s.summarySent[event] = last
			return nil
		}
		if messages, err = s.summaries(ctx, event, start, end); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO notification_summary ("+column+") VALUES ($1) "+
			"ON CONFLICT (singleton) DO UPDATE SET "+column+" = excluded."+column, key); err != nil {
			return err
		}
		sent = true
		return nil
	})
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("The "+summaryNames[event]+" could not be made", "error", err)
		}
		return
	}
	if !sent {
		return
	}
	s.summarySent[event] = key
	for _, m := range messages {
		s.dispatch(m.event, m.recipient)
	}
	s.logger.Info("The "+summaryNames[event]+" was sent", "start", start.Format(time.RFC3339), "end", end.Format(time.RFC3339),
		"summaries", len(messages))
	s.pruneSummary(ctx)
}

// SendSummary sends the summary of event now, to every target that chose
// it: for the weekly summary the last seven days, for the month and year
// in review the last month and year that ended. System › Schedule's tasks
// run it. What the schedule sends next stays as it is.
func (s *Service) SendSummary(ctx context.Context, event string) error {
	s.summaryMu.Lock()
	defer s.summaryMu.Unlock()
	now := s.now()
	start, end := weekBefore(now, s.zone), now
	if event != WeeklySummary {
		start, end = reviewLast(event, now, s.zone)
	}
	messages, err := s.summaries(ctx, event, start, end)
	if err != nil {
		return err
	}
	for _, m := range messages {
		s.dispatch(m.event, m.recipient)
	}
	s.logger.Info("The "+summaryNames[event]+" was sent by hand", "start", start.Format(time.RFC3339), "end", end.Format(time.RFC3339),
		"summaries", len(messages))
	return nil
}

// How long the summaries keep what they tell: the new episodes and the
// problems found past a month and the month that follows it, the users
// who joined past a year and the month that follows it.
const (
	keepFound  = 70 * 24 * time.Hour
	keepJoined = 400 * 24 * time.Hour
)

// pruneSummary forgets what the summaries keep once older than they need.
func (s *Service) pruneSummary(ctx context.Context) {
	now := s.now()
	for _, prune := range []struct {
		query string
		keep  time.Duration
	}{
		{"DELETE FROM notification_found_episodes WHERE found_at < $1", keepFound},
		{"DELETE FROM notification_joins WHERE joined_at < $1", keepJoined},
		{"DELETE FROM notification_health_found WHERE found_at < $1", keepFound},
	} {
		if _, err := s.db.Exec(ctx, prune.query, now.Add(-prune.keep)); err != nil && ctx.Err() == nil {
			s.logger.Warn("Old summary records could not be deleted", "error", err)
		}
	}
}

// summaries makes the summaries of event for the week, month or year from
// start until before end: the server's, for the server's targets and
// administrators' own that chose it, and each other user's, for their
// own; none for a recipient with nothing to tell.
func (s *Service) summaries(ctx context.Context, event string, start, end time.Time) ([]summaryMessage, error) {
	if err := s.reload(ctx); err != nil {
		return nil, err
	}
	server := false
	personal := map[accounts.ID]bool{}
	s.mu.Lock()
	for _, t := range s.targets {
		switch {
		case !t.wants(event):
		case t.owner == nil || s.admins[*t.owner]:
			server = true
		default:
			personal[*t.owner] = true
		}
	}
	s.mu.Unlock()
	var messages []summaryMessage
	if server {
		sum, err := s.serverSummary(ctx, event, start, end)
		if err != nil {
			return nil, err
		}
		if !sum.empty() {
			messages = append(messages, summaryMessage{event: s.summaryEvent(event, nil, sum),
				recipient: func(t target) bool { return t.owner == nil || s.admins[*t.owner] }})
		}
	}
	if len(personal) == 0 {
		return messages, nil
	}
	users, err := s.accounts.Users(ctx)
	if err != nil {
		return nil, err
	}
	for _, user := range users {
		if !personal[user.ID] || user.Refused(s.now()) || user.IsAdministrator {
			continue
		}
		sum, err := s.userSummary(ctx, event, user, start, end)
		if err != nil {
			return nil, err
		}
		if !sum.empty() {
			id := user.ID
			messages = append(messages, summaryMessage{event: s.summaryEvent(event, &user, sum),
				recipient: func(t target) bool { return t.owner != nil && *t.owner == id && !s.admins[id] }})
		}
	}
	return messages, nil
}

// serverSummary is the server's week, month or year, as event tells it: a
// year tells neither the new episodes nor the problems found, but the
// month played longest in.
func (s *Service) serverSummary(ctx context.Context, event string, start, end time.Time) (SummaryJSON, error) {
	sum := newSummary(ScopeServer, start, end)
	f, top := plays.Filter{Since: start, Until: end}, summaryServerTop
	if event == YearlySummary {
		top = summaryYearTop
	}
	if err := s.summarizePlays(ctx, &sum, f, top); err != nil {
		return sum, err
	}
	if event == YearlySummary {
		if err := s.summarizeTopMonth(ctx, &sum, f); err != nil {
			return sum, err
		}
	} else if err := s.summarizeEpisodes(ctx, &sum, nil); err != nil {
		return sum, err
	}
	rows, err := s.db.Query(ctx, `SELECT j.user_id, u.name, j.joined_at, c.id, c.name
		FROM notification_joins j JOIN users u ON u.id = j.user_id
		LEFT JOIN invites i ON i.id = j.invite_id LEFT JOIN users c ON c.id = i.created_by
		WHERE j.joined_at >= $1 AND j.joined_at < $2 ORDER BY j.joined_at, u.name LIMIT $3`, start, end, summaryList)
	if err != nil {
		return sum, err
	}
	sum.Users, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (JoinedJSON, error) {
		var j JoinedJSON
		var user accounts.ID
		var creator *accounts.ID
		var creatorName *string
		err := row.Scan(&user, &j.Name, &j.JoinedAt, &creator, &creatorName)
		j.ID, j.JoinedAt = user.String(), j.JoinedAt.UTC()
		if creator != nil && creatorName != nil {
			j.InvitedBy = &UserJSON{ID: creator.String(), Name: *creatorName}
		}
		return j, err
	})
	if err != nil {
		return sum, err
	}
	if s.files != nil {
		added, total, err := s.files.Added(ctx, start, end, summaryAdded)
		if err != nil {
			return sum, err
		}
		sum.AddedFiles = total
		for _, a := range added {
			kind := "movie"
			switch a.Kind {
			case localfiles.KindShows:
				kind = "show"
			case localfiles.KindMusic:
				kind = "album"
			case localfiles.KindHomeVideos:
				kind = "video"
			}
			sum.Added = append(sum.Added, AddedJSON{Name: a.Name, Kind: kind, Files: a.Files})
		}
	}
	if event == YearlySummary {
		return sum, nil
	}
	rows, err = s.db.Query(ctx, `SELECT key, severity, text, page, found_at FROM notification_health_found
		WHERE found_at >= $1 AND found_at < $2 ORDER BY found_at, id LIMIT $3`, start, end, summaryList)
	if err != nil {
		return sum, err
	}
	sum.Problems, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (FoundProblemJSON, error) {
		var p FoundProblemJSON
		err := row.Scan(&p.Key, &p.Severity, &p.Text, &p.page, &p.FoundAt)
		p.FoundAt = p.FoundAt.UTC()
		return p, err
	})
	return sum, err
}

// userSummary is user's week, month or year, as event tells it: a year
// tells the month played longest in rather than the new episodes.
func (s *Service) userSummary(ctx context.Context, event string, user accounts.User, start, end time.Time) (SummaryJSON, error) {
	sum := newSummary(ScopeUser, start, end)
	f := plays.Filter{User: &user.ID, Since: start, Until: end}
	if err := s.summarizePlays(ctx, &sum, f, summaryUserTop); err != nil {
		return sum, err
	}
	if event == YearlySummary {
		return sum, s.summarizeTopMonth(ctx, &sum, f)
	}
	return sum, s.summarizeEpisodes(ctx, &sum, &user.ID)
}

// newSummary is a summary of scope with nothing in it yet: its lists are
// empty, never null.
func newSummary(scope string, start, end time.Time) SummaryJSON {
	return SummaryJSON{Scope: scope, Start: start.UTC(), End: end.UTC(), Movies: []SummaryTitleJSON{}, Series: []SummaryTitleJSON{},
		Episodes: []SummaryEpisodeJSON{}, Users: []JoinedJSON{}, Added: []AddedJSON{}, Problems: []FoundProblemJSON{}}
}

// summarizePlays adds the playbacks f selects to sum, with the top movies
// and series.
func (s *Service) summarizePlays(ctx context.Context, sum *SummaryJSON, f plays.Filter, top int) error {
	if s.history == nil {
		return nil
	}
	played, err := s.history.Summary(ctx, f, top)
	if err != nil {
		return err
	}
	sum.Plays, sum.Played = played.Plays, int64(played.Played.Seconds())
	titles := func(ranked []plays.Ranked) []SummaryTitleJSON {
		result := make([]SummaryTitleJSON, 0, len(ranked))
		for _, r := range ranked {
			result = append(result, SummaryTitleJSON{ID: r.ID.String(), Name: r.Name, Plays: r.Plays, Played: int64(r.Played.Seconds())})
		}
		return result
	}
	sum.Movies, sum.Series = titles(played.Movies), titles(played.Series)
	return nil
}

// summarizeTopMonth adds to sum, a year, the month the playbacks f selects
// played longest in, by the server's time zone: on a tie, the one with
// the most playbacks, then the earliest.
func (s *Service) summarizeTopMonth(ctx context.Context, sum *SummaryJSON, f plays.Filter) error {
	if s.history == nil || sum.Plays == 0 {
		return nil
	}
	start := sum.Start.In(s.zone)
	bounds := make([]time.Time, 13)
	for i := range bounds {
		bounds[i] = time.Date(start.Year(), start.Month()+time.Month(i), 1, 0, 0, 0, 0, s.zone)
	}
	totals, err := s.history.Totals(ctx, f, bounds)
	if err != nil {
		return err
	}
	best := -1
	for i, t := range totals {
		if t.Plays > 0 && (best < 0 || t.Played > totals[best].Played || t.Played == totals[best].Played && t.Plays > totals[best].Plays) {
			best = i
		}
	}
	if best >= 0 {
		sum.TopMonth = &SummaryMonthJSON{Month: bounds[best].Format("2006-01"), Plays: totals[best].Plays,
			Played: int64(totals[best].Played.Seconds())}
	}
	return nil
}

// summarizeEpisodes adds to sum the new episodes found in its week or
// month: for user, or for anyone when nil, each once.
func (s *Service) summarizeEpisodes(ctx context.Context, sum *SummaryJSON, user *accounts.ID) error {
	rows, err := s.db.Query(ctx, `SELECT DISTINCT ON (episode_id) episode_id, series_id, series_name, name, season, number
		FROM notification_found_episodes WHERE found_at >= $1 AND found_at < $2 AND ($3::uuid IS NULL OR user_id = $3)
		ORDER BY episode_id`, sum.Start, sum.End, user)
	if err != nil {
		return err
	}
	episodes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (SummaryEpisodeJSON, error) {
		var e SummaryEpisodeJSON
		var id, series accounts.ID
		err := row.Scan(&id, &series, &e.SeriesName, &e.Name, &e.Season, &e.Number)
		e.ID, e.SeriesID = id.String(), series.String()
		return e, err
	})
	if err != nil {
		return err
	}
	slices.SortFunc(episodes, func(a, b SummaryEpisodeJSON) int {
		return cmp.Or(strings.Compare(a.SeriesName, b.SeriesName), cmp.Compare(a.Season, b.Season), cmp.Compare(a.Number, b.Number),
			strings.Compare(a.ID, b.ID))
	})
	sum.NewEpisodes = len(episodes)
	sum.Episodes = episodes[:min(len(episodes), summaryEpisodes)]
	return nil
}

// summaryEvent tells sum, the week, month or year of event: the server's
// when user is nil, user's otherwise. Its message is short, with the key
// figures, for the kinds that show a text; an email tells it whole (see
// summaryEmail).
func (s *Service) summaryEvent(event string, user *accounts.User, sum SummaryJSON) Event {
	ev := s.newEvent(event)
	ev.Summary = &sum
	if user != nil {
		ev.User = &UserJSON{ID: user.ID.String(), Name: user.Name}
		switch event {
		case MonthlySummary:
			ev.Title = s.phrase("Your month on %s", "Votre mois sur %s", ev.Server.Name)
		case YearlySummary:
			ev.Title = s.phrase("Your year on %s", "Votre année sur %s", ev.Server.Name)
		default:
			ev.Title = s.phrase("Your week on %s", "Votre semaine sur %s", ev.Server.Name)
		}
		ev.URL = s.adminLink("/me/account#statistics")
	} else {
		switch event {
		case MonthlySummary:
			ev.Title = s.phrase("The month on %s", "Le mois sur %s", ev.Server.Name)
		case YearlySummary:
			ev.Title = s.phrase("The year on %s", "L’année sur %s", ev.Server.Name)
		default:
			ev.Title = s.phrase("The week on %s", "La semaine sur %s", ev.Server.Name)
		}
		ev.URL = s.adminLink("/system/statistics")
	}
	lines := []string{s.periodLine(event, sum), s.playedLine(sum)}
	if sum.TopMonth != nil {
		lines = append(lines, s.phrase("Most watched month: %s.", "Mois le plus regardé : %s.", s.topMonthText(*sum.TopMonth, false)))
	}
	if len(sum.Movies) > 0 {
		lines = append(lines, s.phrase("Movies: %s.", "Films : %s.", titleNames(sum.Movies, summaryServerTop)))
	}
	if len(sum.Series) > 0 {
		lines = append(lines, s.phrase("Series: %s.", "Séries : %s.", titleNames(sum.Series, summaryServerTop)))
	}
	if sum.NewEpisodes > 0 {
		lines = append(lines, s.episodesHeading(sum)+".")
	}
	switch len(sum.Users) {
	case 0:
	case 1:
		lines = append(lines, s.phrase("New user: %s.", "Nouvel utilisateur : %s.", sum.Users[0].Name))
	default:
		names := make([]string, 0, len(sum.Users))
		for _, u := range sum.Users {
			names = append(names, u.Name)
		}
		lines = append(lines, s.phrase("New users: %s.", "Nouveaux utilisateurs : %s.", strings.Join(names, ", ")))
	}
	if sum.AddedFiles > 0 {
		lines = append(lines, s.addedHeading(sum)+".")
	}
	if len(sum.Problems) > 0 {
		lines = append(lines, s.problemsHeading(sum)+".")
	}
	ev.Message = strings.Join(lines, "\n")
	return ev
}

// titleNames lists the names of the first n titles.
func titleNames(titles []SummaryTitleJSON, n int) string {
	names := make([]string, 0, min(len(titles), n))
	for _, t := range titles[:min(len(titles), n)] {
		names = append(names, t.Name)
	}
	return strings.Join(names, ", ")
}

// periodLine names the week, month or year of sum, as event tells it, in
// the server's time zone.
func (s *Service) periodLine(event string, sum SummaryJSON) string {
	start := sum.Start.In(s.zone)
	french := s.accounts.Settings().Language == "fr"
	switch event {
	case MonthlySummary:
		if french {
			name := frenchMonths[start.Month()-1]
			if strings.IndexByte("aeiou", name[0]) >= 0 {
				return fmt.Sprintf("Mois d’%s %d.", name, start.Year())
			}
			return fmt.Sprintf("Mois de %s %d.", name, start.Year())
		}
		return fmt.Sprintf("Month of %s %d.", start.Month(), start.Year())
	case YearlySummary:
		return s.phrase("Year %d.", "Année %d.", start.Year())
	}
	return s.weekLine(sum)
}

// topMonthText names the month of a year played longest in, with how
// long, and with its year when withYear.
func (s *Service) topMonthText(m SummaryMonthJSON, withYear bool) string {
	month, err := time.Parse("2006-01", m.Month)
	if err != nil {
		return m.Month
	}
	name := month.Month().String()
	if s.accounts.Settings().Language == "fr" {
		name = frenchMonths[month.Month()-1]
	}
	if withYear {
		name += " " + strconv.Itoa(month.Year())
	}
	return name + ", " + s.duration(m.Played)
}

// weekLine names the week of sum, by its days in the server's time zone.
func (s *Service) weekLine(sum SummaryJSON) string {
	start, end := sum.Start.In(s.zone), sum.End.In(s.zone)
	if s.accounts.Settings().Language == "fr" {
		first := strconv.Itoa(start.Day())
		if start.Day() == 1 {
			first = "1er"
		}
		if start.Month() == end.Month() {
			return fmt.Sprintf("Semaine du %s au %d %s.", first, end.Day(), frenchMonths[end.Month()-1])
		}
		return fmt.Sprintf("Semaine du %s %s au %d %s.", first, frenchMonths[start.Month()-1], end.Day(), frenchMonths[end.Month()-1])
	}
	if start.Month() == end.Month() {
		return fmt.Sprintf("Week of %s %d to %d.", start.Month(), start.Day(), end.Day())
	}
	return fmt.Sprintf("Week of %s %d to %s %d.", start.Month(), start.Day(), end.Month(), end.Day())
}

var frenchMonths = [12]string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre",
	"décembre"}

// playedLine tells how long and how many times videos played.
func (s *Service) playedLine(sum SummaryJSON) string {
	if sum.Plays == 0 {
		return s.phrase("Nothing was watched.", "Rien n’a été regardé.")
	}
	return s.phrase("%s watched, %s.", "%s de visionnage, %s.", s.duration(sum.Played), s.playsText(sum.Plays))
}

// playsText counts playbacks.
func (s *Service) playsText(n int) string {
	return s.count(n, "%d playback", "%d playbacks", "%d lecture", "%d lectures")
}

// episodesHeading counts the new episodes of sum.
func (s *Service) episodesHeading(sum SummaryJSON) string {
	if sum.Scope == ScopeUser {
		return s.count(sum.NewEpisodes, "%d new episode of the series you follow", "%d new episodes of the series you follow",
			"%d nouvel épisode des séries que vous suivez", "%d nouveaux épisodes des séries que vous suivez")
	}
	return s.count(sum.NewEpisodes, "%d new episode of the series users follow", "%d new episodes of the series users follow",
		"%d nouvel épisode des séries suivies", "%d nouveaux épisodes des séries suivies")
}

// addedHeading counts the files added to the local folders.
func (s *Service) addedHeading(sum SummaryJSON) string {
	return s.count(sum.AddedFiles, "%d file added to the local folders", "%d files added to the local folders",
		"%d fichier ajouté aux dossiers locaux", "%d fichiers ajoutés aux dossiers locaux")
}

// problemsHeading counts the problems found.
func (s *Service) problemsHeading(sum SummaryJSON) string {
	return s.count(len(sum.Problems), "%d problem found by System › Health", "%d problems found by System › Health",
		"%d problème détecté par Système › Santé", "%d problèmes détectés par Système › Santé")
}

// count says n with the pattern for one or for several, in the server
// language: French counts 0 as one.
func (s *Service) count(n int, english1, englishN, french1, frenchN string) string {
	if s.accounts.Settings().Language == "fr" {
		if n <= 1 {
			return fmt.Sprintf(french1, n)
		}
		return fmt.Sprintf(frenchN, n)
	}
	if n == 1 {
		return fmt.Sprintf(english1, n)
	}
	return fmt.Sprintf(englishN, n)
}

// duration says a time played in seconds: in minutes under an hour, in
// hours to a tenth otherwise.
func (s *Service) duration(seconds int64) string {
	if seconds < 3600 {
		minutes := int(math.Round(float64(seconds) / 60))
		return s.count(minutes, "%d minute", "%d minutes", "%d minute", "%d minutes")
	}
	hours := math.Round(float64(seconds)/360) / 10
	text := strconv.FormatFloat(hours, 'f', -1, 64)
	if s.accounts.Settings().Language == "fr" {
		text = strings.Replace(text, ".", ",", 1)
		if hours < 2 {
			return text + " heure"
		}
		return text + " heures"
	}
	if hours == 1 {
		return text + " hour"
	}
	return text + " hours"
}
