package trackers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Event is the kind of a playback report.
type Event int

const (
	Started Event = iota
	Progressed
	Stopped
)

// Playback is a playback report of a movie or an episode.
type Playback struct {
	Event Event
	// Device is the device playing, Item the title played.
	Device, Item accounts.ID
	// Position is where playback is, when PositionKnown, in a title of
	// Runtime, zero when unknown.
	Position      time.Duration
	PositionKnown bool
	Runtime       time.Duration
	Paused        bool
	// Played tells that Polyfin counts the title played as of this report.
	Played bool
	// Title identifies the title to the services, false for a title they
	// cannot know. It runs in the background.
	Title func(context.Context) (Title, bool)
}

// Scope is what a played mark was put on.
type Scope int

const (
	// ScopeTitle: a movie or an episode.
	ScopeTitle Scope = iota
	ScopeSeason
	ScopeSeries
)

// Mark is a played mark, or its removal, that a user put on titles from
// an app.
type Mark struct {
	Played bool
	// Date is when the user says they watched, nil for now.
	Date  *time.Time
	Scope Scope
	// Titles identifies the movies or episodes the mark changes, those the
	// services can know: for a played mark, those it counts a new play of.
	// It runs in the background.
	Titles func(context.Context) []Title
}

// The kinds of change sent.
const (
	// kindScrobble follows playback (Action: start, pause or stop).
	kindScrobble = "scrobble"
	// kindHistoryAdd and kindHistoryRemove add titles to the history of a
	// scrobbling service, or remove them.
	kindHistoryAdd    = "history_add"
	kindHistoryRemove = "history_remove"
	// kindResume saves a resume point on PublicMetaDB.
	kindResume = "resume"
	// kindWatched and kindUnwatched add a title to PublicMetaDB's history,
	// or remove it: one episode, or every episode of a season or series
	// (Scope).
	kindWatched   = "watched"
	kindUnwatched = "unwatched"
)

// event is what a change sends, as kept in the queue.
type event struct {
	Titles     []Title    `json:"titles"`
	Action     string     `json:"action,omitempty"`
	Progress   float64    `json:"progress,omitempty"`
	PositionMS int64      `json:"positionMs,omitempty"`
	RuntimeMS  int64      `json:"runtimeMs,omitempty"`
	WatchedAt  *time.Time `json:"watchedAt,omitempty"`
	Scope      Scope      `json:"scope,omitempty"`
}

// sessionKey names the playback of one device of a user.
type sessionKey struct {
	user, device accounts.ID
}

// session is a playback under way.
type session struct {
	item   accounts.ID
	paused bool
	// completed is set once the playback made Polyfin count the title
	// played, which PublicMetaDB's history then holds.
	completed bool
}

// intake holds the reports and marks of one user waiting to be looked at,
// in the order they came.
type intake struct {
	jobs    []func(context.Context)
	running bool
}

// later looks at a report or mark of user in the background, after the
// earlier ones.
func (s *Service) later(user accounts.ID, job func(context.Context)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	q := s.intakes[user]
	if q == nil {
		q = &intake{}
		s.intakes[user] = q
	}
	if len(q.jobs) >= maxIntake {
		return
	}
	q.jobs = append(q.jobs, job)
	if !q.running {
		q.running = s.spawn(func() { s.take(user, q) })
	}
}

func (s *Service) take(user accounts.ID, q *intake) {
	for {
		s.mu.Lock()
		if len(q.jobs) == 0 || s.ctx.Err() != nil {
			q.running = false
			delete(s.intakes, user)
			s.mu.Unlock()
			return
		}
		job := q.jobs[0]
		q.jobs = q.jobs[1:]
		s.mu.Unlock()
		job(s.ctx)
	}
}

// Playback sends a playback report of user to their services, in the
// background.
func (s *Service) Playback(user accounts.ID, report Playback) {
	if s == nil {
		return
	}
	s.later(user, func(ctx context.Context) { s.playback(ctx, user, report) })
}

// Mark sends a played mark of user to their services, in the background.
func (s *Service) Mark(user accounts.ID, mark Mark) {
	if s == nil {
		return
	}
	s.later(user, func(ctx context.Context) { s.mark(ctx, user, mark) })
}

// change is one change to send to one service.
type change struct {
	kind  string
	event event
	// durable changes are kept until the service accepts them.
	durable bool
}

func (s *Service) playback(ctx context.Context, user accounts.ID, report Playback) {
	key := sessionKey{user, report.Device}
	services, err := s.active(ctx, user)
	if err != nil || len(services) == 0 {
		if report.Event == Stopped {
			s.mu.Lock()
			delete(s.sessions, key)
			s.mu.Unlock()
		}
		if err != nil {
			s.logger.Warn("A playback report could not be sent to tracking services", "error", err)
		}
		return
	}
	// The progress the scrobbling services decide from: a stop without a
	// position is a title played to its end, as for Polyfin.
	progress, progressKnown := 0.0, false
	switch {
	case report.Event == Stopped && !report.PositionKnown:
		progress, progressKnown = 100, true
	case report.PositionKnown && report.Runtime > 0:
		progress, progressKnown = float64(report.Position)/float64(report.Runtime)*100, true
	}
	scrobble := func(action string) change {
		return change{kind: kindScrobble, event: event{Action: action, Progress: progress}, durable: action == "stop"}
	}
	resumePoint := func() (change, bool) {
		if !report.PositionKnown || report.Runtime <= 0 {
			return change{}, false
		}
		return change{kind: kindResume, durable: true, event: event{PositionMS: report.Position.Milliseconds(), RuntimeMS: report.Runtime.Milliseconds()}}, true
	}
	watched := change{kind: kindWatched, durable: true}
	// Trakt ignores what is under 1%, and the others have nothing to keep
	// of it.
	worthKeeping := progressKnown && progress >= 1

	var scrobbled, kept []change
	s.mu.Lock()
	current := s.sessions[key]
	if current != nil && current.item != report.Item {
		current = nil
	}
	switch report.Event {
	case Started:
		s.sessions[key] = &session{item: report.Item, paused: report.Paused}
		if !report.Paused {
			scrobbled = append(scrobbled, scrobble("start"))
		}
	case Progressed:
		switch {
		case current == nil:
			// A report of a playback whose start was not seen.
			current = &session{item: report.Item, paused: report.Paused}
			s.sessions[key] = current
			if !report.Paused {
				scrobbled = append(scrobbled, scrobble("start"))
			}
		case report.Paused && !current.paused:
			current.paused = true
			if worthKeeping {
				scrobbled = append(scrobbled, scrobble("pause"))
			}
			if point, ok := resumePoint(); ok && !current.completed {
				kept = append(kept, point)
			}
		case !report.Paused && current.paused:
			current.paused = false
			scrobbled = append(scrobbled, scrobble("start"))
		}
		if report.Played && !current.completed {
			current.completed = true
			kept = append(kept, watched)
		}
	case Stopped:
		delete(s.sessions, key)
		if worthKeeping {
			scrobbled = append(scrobbled, scrobble("stop"))
		}
		completed := current != nil && current.completed
		switch point, ok := resumePoint(); {
		case report.Played && !completed:
			kept = append(kept, watched)
		case !report.Played && !completed && ok:
			kept = append(kept, point)
		}
	}
	s.mu.Unlock()
	if len(scrobbled) == 0 && len(kept) == 0 {
		return
	}
	// Only then is the title looked up, which may take the library a
	// while.
	title, ok := report.Title(ctx)
	if !ok {
		return
	}
	for _, changes := range [][]change{scrobbled, kept} {
		for i := range changes {
			changes[i].event.Titles = []Title{title}
		}
	}

	for _, service := range services {
		changes := kept
		if scrobbles(service) {
			changes = scrobbled
		}
		for _, c := range changes {
			s.queue(ctx, user, service, c)
		}
	}
}

func (s *Service) mark(ctx context.Context, user accounts.ID, mark Mark) {
	services, err := s.active(ctx, user)
	if err != nil {
		s.logger.Warn("A played mark could not be sent to tracking services", "error", err)
		return
	}
	if len(services) == 0 {
		return
	}
	titles := mark.Titles(ctx)
	if len(titles) == 0 {
		return
	}
	at := s.now().UTC()
	if mark.Date != nil {
		at = mark.Date.UTC()
	}
	for _, service := range services {
		switch {
		case scrobbles(service) && mark.Played:
			s.queue(ctx, user, service, change{kind: kindHistoryAdd, durable: true, event: event{Titles: titles, WatchedAt: &at}})
		case scrobbles(service):
			s.queue(ctx, user, service, change{kind: kindHistoryRemove, durable: true, event: event{Titles: titles}})
		case mark.Played:
			// PublicMetaDB takes one title at a time.
			for _, title := range titles {
				s.queue(ctx, user, service, change{kind: kindWatched, durable: true, event: event{Titles: []Title{title}, WatchedAt: &at}})
			}
		default:
			// PublicMetaDB removes a whole series or season at once. The
			// first title names it; the others are the episodes it holds.
			for _, group := range scoped(titles, mark.Scope) {
				s.queue(ctx, user, service, change{kind: kindUnwatched, durable: true, event: event{Titles: group, Scope: mark.Scope}})
			}
		}
	}
}

// scoped groups titles by what one removal from PublicMetaDB's history
// covers: each title, each season, or each series.
func scoped(titles []Title, scope Scope) [][]Title {
	var groups [][]Title
	for _, title := range titles {
		i := -1
		for j, group := range groups {
			first := group[0]
			if scope != ScopeTitle && first.Episode && title.Episode && first.ids() == title.ids() &&
				(scope == ScopeSeries || first.Season == title.Season) {
				i = j
			}
		}
		if i < 0 {
			groups = append(groups, []Title{title})
			continue
		}
		groups[i] = append(groups[i], title)
	}
	return groups
}

// active lists the services user connected that changes can be sent to:
// those not waiting for the user to connect again, and whose app the
// settings still hold.
func (s *Service) active(ctx context.Context, user accounts.ID) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT service FROM tracking_connections
		WHERE user_id = $1 AND problem IS DISTINCT FROM 'reconnect' ORDER BY service`, user)
	if err != nil {
		return nil, err
	}
	connected, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	settings := s.settings()
	var services []string
	for _, service := range connected {
		if Available(service, settings) {
			services = append(services, service)
		}
	}
	return services, nil
}

// queue sends c to service for user: durable changes are kept in the
// database until sent.
func (s *Service) queue(ctx context.Context, user accounts.ID, service string, c change) {
	key := laneKey{user, service}
	if !c.durable {
		s.push(key, ticket{kind: c.kind, event: c.event, queued: s.now()})
		return
	}
	payload, err := json.Marshal(c.event)
	if err != nil {
		return
	}
	var id int64
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		title := c.event.Titles[0].Item
		if c.kind == kindResume {
			// A newer resume point replaces the one waiting.
			if _, err := tx.Exec(ctx, "DELETE FROM tracking_events WHERE user_id = $1 AND service = $2 AND kind = $3 AND title = $4",
				user, service, c.kind, title); err != nil {
				return err
			}
		}
		return tx.QueryRow(ctx, `INSERT INTO tracking_events (user_id, service, kind, title, payload, created_at)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, user, service, c.kind, title, payload, s.now()).Scan(&id)
	})
	if err != nil {
		// The connection may have gone meanwhile.
		s.logger.Debug("A change for a tracking service was not queued", "service", service, "error", err)
		return
	}
	s.push(key, ticket{id: id, kind: c.kind, queued: s.now()})
}
