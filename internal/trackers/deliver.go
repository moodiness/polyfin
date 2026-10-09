package trackers

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// laneKey names what one user sends to one service.
type laneKey struct {
	user    accounts.ID
	service string
}

// ticket is a change waiting in a lane: a durable one by its row in
// tracking_events, another with its event.
type ticket struct {
	seq    int64
	id     int64
	kind   string
	event  event
	queued time.Time
}

// lane sends one user's changes to one service, one at a time, in order.
type lane struct {
	tickets []ticket
	running bool
	// last is when the lane last sent; notBefore holds it back after the
	// service asked to wait.
	last, notBefore time.Time
}

// pace spaces requests shared by every lane of a service, as limits per
// address ask.
type pace struct {
	mu        sync.Mutex
	last      time.Time
	notBefore time.Time
}

// push queues t in the lane of key, and starts the lane.
func (s *Service) push(key laneKey, t ticket) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	l := s.lanes[key]
	if l == nil {
		l = &lane{}
		s.lanes[key] = l
	}
	s.seq++
	t.seq = s.seq
	l.tickets = append(l.tickets, t)
	if !l.running {
		l.running = s.spawn(func() { s.drive(key, l) })
	}
}

// clearLane forgets what waits in the lane of key.
func (s *Service) clearLane(key laneKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l := s.lanes[key]; l != nil {
		l.tickets = nil
	}
}

// drive sends the changes of a lane until none waits.
func (s *Service) drive(key laneKey, l *lane) {
	for {
		s.mu.Lock()
		if len(l.tickets) == 0 || s.ctx.Err() != nil {
			l.running = false
			if len(l.tickets) == 0 && s.lanes[key] == l {
				delete(s.lanes, key)
			}
			s.mu.Unlock()
			return
		}
		t := l.tickets[0]
		s.mu.Unlock()
		wait := s.deliver(key, l, t)
		if wait == 0 {
			s.mu.Lock()
			if len(l.tickets) > 0 && l.tickets[0].seq == t.seq {
				l.tickets = l.tickets[1:]
			}
			s.mu.Unlock()
			continue
		}
		if !s.sleep(wait) {
			s.mu.Lock()
			l.running = false
			s.mu.Unlock()
			return
		}
	}
}

// deliver sends t, and returns how long to wait before sending it again,
// zero once it is done with.
func (s *Service) deliver(key laneKey, l *lane, t ticket) time.Duration {
	ctx := s.ctx
	kind, ev := t.kind, t.event
	attempts := 0
	if t.id != 0 {
		var payload []byte
		var created time.Time
		err := s.db.QueryRow(ctx, "SELECT kind, payload, created_at, attempts FROM tracking_events WHERE id = $1", t.id).
			Scan(&kind, &payload, &created, &attempts)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// Sent already, replaced, or the connection is gone.
			return 0
		case err != nil:
			return s.timing.retryFirst
		}
		if json.Unmarshal(payload, &ev) != nil || !ev.complete() {
			s.forget(t.id)
			return 0
		}
		if s.now().Sub(created) > s.timing.giveUp {
			s.forget(t.id)
			s.logger.Info("A change could not be sent to a tracking service in time and was dropped",
				"user_id", key.user.String(), "service", key.service, "change", kind)
			return 0
		}
	} else if s.now().Sub(t.queued) > s.timing.stale {
		return 0
	}

	settings := s.settings()
	conn, ok, err := s.connection(ctx, key.user, key.service)
	switch {
	case err != nil:
		return s.timing.retryFirst
	case !ok || conn.problem == ProblemReconnect || !Available(key.service, settings):
		s.forget(t.id)
		return 0
	}
	if kind == kindHistoryAdd || kind == kindWatched {
		// A title the service counted watched lately, from the same
		// viewing, is not added again.
		if ev.Titles, err = s.notWatchedLately(ctx, key, ev.Titles, ev.WatchedAt); err != nil {
			return s.timing.retryFirst
		}
		if len(ev.Titles) == 0 {
			s.forget(t.id)
			return 0
		}
	}
	if !s.waitTurn(key, l) {
		return time.Second
	}
	res := s.send(ctx, key, conn, settings, kind, ev)
	s.mu.Lock()
	l.last = time.Now()
	if res.after > 0 {
		l.notBefore = l.last.Add(res.after)
	}
	s.mu.Unlock()
	if p := s.paces[key.service]; p != nil && res.after > 0 {
		p.mu.Lock()
		p.notBefore = time.Now().Add(res.after)
		p.mu.Unlock()
	}

	switch res.outcome {
	case delivered:
		s.forget(t.id)
		s.delivered(ctx, key, kind, ev, res.watched)
		return 0
	case reconnect:
		s.refusedToken(ctx, key)
		return 0
	case refused:
		s.forget(t.id)
		if t.id != 0 {
			s.logger.Info("A tracking service refused a change, which was dropped",
				"user_id", key.user.String(), "service", key.service, "change", kind, "status", res.status)
		}
		return 0
	}
	if t.id == 0 {
		// A start or pause is not worth sending late.
		return 0
	}
	attempts++
	if _, err := s.db.Exec(ctx, "UPDATE tracking_events SET attempts = $2 WHERE id = $1", t.id, attempts); err != nil {
		return s.timing.retryFirst
	}
	s.failed(ctx, key, res.status)
	backoff := s.timing.retryFirst
	for range min(attempts-1, 20) {
		backoff *= 2
		if backoff >= s.timing.retryMax {
			backoff = s.timing.retryMax
			break
		}
	}
	return max(backoff, res.after)
}

// waitTurn waits until the lane may send again: the gap its service asks
// for since its last request, and the wait it asked for. It reports false
// if the service stopped meanwhile.
func (s *Service) waitTurn(key laneKey, l *lane) bool {
	s.mu.Lock()
	next := latest(l.last.Add(s.timing.gaps[key.service]), l.notBefore)
	s.mu.Unlock()
	if !s.sleep(time.Until(next)) {
		return false
	}
	p := s.paces[key.service]
	if p == nil {
		return true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !s.sleep(time.Until(latest(p.last.Add(s.timing.sharedGaps[key.service]), p.notBefore))) {
		return false
	}
	p.last = time.Now()
	return true
}

// latest is the later of a and b.
func latest(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// send sends a change with a fresh token, refreshing it once more if the
// service refuses it.
func (s *Service) send(ctx context.Context, key laneKey, conn connection, settings accounts.Settings, kind string, ev event) result {
	if !ByCode(key.service) {
		return s.sendTo(ctx, key.service, conn.token, settings, kind, ev)
	}
	conn, err := s.fresh(ctx, key, "")
	switch {
	case errors.Is(err, errTokenRefused):
		return result{outcome: reconnect}
	case err != nil:
		return result{outcome: retry}
	}
	res := s.sendTo(ctx, key.service, conn.token, settings, kind, ev)
	if res.outcome != reconnect {
		return res
	}
	conn, err = s.fresh(ctx, key, conn.token)
	switch {
	case errors.Is(err, errTokenRefused):
		return result{outcome: reconnect}
	case err != nil:
		return result{outcome: retry}
	}
	return s.sendTo(ctx, key.service, conn.token, settings, kind, ev)
}

// forget deletes the queued change id, if any.
func (s *Service) forget(id int64) {
	if id == 0 {
		return
	}
	if _, err := s.db.Exec(s.ctx, "DELETE FROM tracking_events WHERE id = $1", id); err != nil {
		s.logger.Warn("A sent change to a tracking service could not be dropped from the queue", "error", err)
	}
}

// notWatchedLately leaves out of titles those the service of key counted
// watched near at, nil for now.
func (s *Service) notWatchedLately(ctx context.Context, key laneKey, titles []Title, at *time.Time) ([]Title, error) {
	when := s.now()
	if at != nil {
		when = *at
	}
	items := make([]accounts.ID, len(titles))
	for i, title := range titles {
		items[i] = title.Item
	}
	rows, err := s.db.Query(ctx, `SELECT title FROM tracking_watched WHERE user_id = $1 AND service = $2 AND title = ANY($3)
		AND watched_at BETWEEN $4 AND $5`, key.user, key.service, items,
		when.Add(-s.timing.watchedWindow), when.Add(s.timing.watchedWindow))
	if err != nil {
		return nil, err
	}
	watched, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(titles, func(t Title) bool { return slices.Contains(watched, t.Item) }), nil
}

// delivered records that the service of key accepted a change: when, and
// which titles it now counts watched, or no longer.
func (s *Service) delivered(ctx context.Context, key laneKey, kind string, ev event, watched []accounts.ID) {
	var problem *string
	err := s.db.QueryRow(ctx, `UPDATE tracking_connections AS c SET last_sent_at = $3, failures = 0,
			problem = CASE WHEN c.problem = 'unreachable' THEN NULL ELSE c.problem END
		FROM tracking_connections AS old WHERE c.user_id = $1 AND c.service = $2 AND old.user_id = c.user_id AND old.service = c.service
		RETURNING old.problem`, key.user, key.service, s.now()).Scan(&problem)
	if err == nil && problem != nil && *problem == ProblemUnreachable {
		s.logger.Info("A tracking service is reachable again", "user_id", key.user.String(), "service", key.service)
	}
	at := s.now()
	if ev.WatchedAt != nil && kind != kindScrobble {
		at = *ev.WatchedAt
	}
	if len(watched) > 0 {
		if _, err := s.db.Exec(ctx, `INSERT INTO tracking_watched (user_id, service, title, watched_at)
			SELECT $1, $2, unnest($3::uuid[]), $4
			ON CONFLICT (user_id, service, title) DO UPDATE SET watched_at = excluded.watched_at`,
			key.user, key.service, watched, at); err != nil {
			s.logger.Debug("A watched title could not be recorded", "error", err)
		}
	}
	if kind == kindHistoryRemove || kind == kindUnwatched {
		items := make([]accounts.ID, len(ev.Titles))
		for i, title := range ev.Titles {
			items[i] = title.Item
		}
		if _, err := s.db.Exec(ctx, "DELETE FROM tracking_watched WHERE user_id = $1 AND service = $2 AND title = ANY($3)",
			key.user, key.service, items); err != nil {
			s.logger.Debug("Unwatched titles could not be recorded", "error", err)
		}
	}
}

// failed counts a failed send, and shows the service as unreachable once
// enough failed in a row.
func (s *Service) failed(ctx context.Context, key laneKey, status int) {
	var became bool
	err := s.db.QueryRow(ctx, `UPDATE tracking_connections AS c SET failures = c.failures + 1,
			problem = CASE WHEN c.problem IS NULL AND c.failures + 1 >= $3 THEN 'unreachable' ELSE c.problem END
		WHERE user_id = $1 AND service = $2
		RETURNING coalesce(c.problem = 'unreachable', false) AND c.failures = $3`, key.user, key.service, s.timing.unreachableAfter).Scan(&became)
	if err == nil && became {
		s.logger.Info("A tracking service cannot be reached: changes are sent again later",
			"user_id", key.user.String(), "service", key.service, "status", status)
	}
}

// refusedToken records that the service of key refused the user's token or
// key: nothing more is sent until they connect again.
func (s *Service) refusedToken(ctx context.Context, key laneKey) {
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "UPDATE tracking_connections SET problem = 'reconnect' WHERE user_id = $1 AND service = $2",
			key.user, key.service); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "DELETE FROM tracking_events WHERE user_id = $1 AND service = $2", key.user, key.service)
		return err
	})
	if err != nil {
		s.logger.Warn("A refused tracking connection could not be recorded", "error", err)
		return
	}
	s.clearLane(key)
	s.logger.Info("A tracking service refused the user's token or key: nothing is sent until they connect it again",
		"user_id", key.user.String(), "service", key.service)
}

// resumeQueued starts sending what an earlier run left queued.
func (s *Service) resumeQueued() {
	rows, err := s.db.Query(s.ctx, "SELECT id, user_id, service, kind, created_at FROM tracking_events ORDER BY id")
	if err != nil {
		s.logger.Warn("The changes queued for tracking services could not be read", "error", err)
		return
	}
	type queued struct {
		id      int64
		user    accounts.ID
		service string
		kind    string
		created time.Time
	}
	all, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (queued, error) {
		var q queued
		return q, row.Scan(&q.id, &q.user, &q.service, &q.kind, &q.created)
	})
	if err != nil {
		s.logger.Warn("The changes queued for tracking services could not be read", "error", err)
		return
	}
	for _, q := range all {
		s.push(laneKey{q.user, q.service}, ticket{id: q.id, kind: q.kind, queued: q.created})
	}
}
