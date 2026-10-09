package notifications

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// healthState is a problem the health check found: since when, in how
// many checks in a row, missing from how many since, and whether its
// message was sent.
type healthState struct {
	problem       Problem
	since         time.Time
	seen, missing int
	announced     bool
}

// loadHealth reads the problems an earlier run found. Without them, the
// problems found before a restart would be told again, and those solved
// meanwhile never.
func (s *Service) loadHealth(ctx context.Context) {
	s.health = map[string]*healthState{}
	rows, err := s.db.Query(ctx, "SELECT key, severity, text, since, seen, missing, announced FROM notification_health")
	if err == nil {
		var states []*healthState
		states, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (*healthState, error) {
			var st healthState
			err := row.Scan(&st.problem.Key, &st.problem.Severity, &st.problem.Text, &st.since, &st.seen, &st.missing, &st.announced)
			return &st, err
		})
		for _, st := range states {
			s.health[st.problem.Key] = st
		}
	}
	if err != nil && ctx.Err() == nil {
		s.logger.Warn("The health problems found earlier could not be read", "error", err)
	}
}

// CheckHealth looks at the problems System › Health shows and tells those
// found, and those solved, to the server's targets and to administrators'
// own. A problem is told once healthConfirm checks in a row found it, and
// solved once as many no longer found it, so that a brief failure does not
// send two messages. The problems are kept in memory, and in the database
// for the next run: an unreachable database is a problem told too.
func (s *Service) CheckHealth(ctx context.Context) {
	if s.problems == nil {
		return
	}
	s.healthMu.Lock()
	defer s.healthMu.Unlock()
	if s.health == nil {
		s.loadHealth(ctx)
	}
	// The targets read last stay in use when the database does not answer.
	if err := s.reload(ctx); err != nil && ctx.Err() == nil {
		s.logger.Debug("The notification targets could not be read again", "error", err)
	}
	problems, err := s.problems(ctx)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("The health problems could not be looked at", "error", err)
		}
		return
	}
	now := s.now()
	current := make(map[string]bool, len(problems))
	for _, problem := range problems {
		current[problem.Key] = true
		st := s.health[problem.Key]
		if st == nil {
			st = &healthState{problem: problem, since: now, seen: 1}
			s.health[problem.Key] = st
		} else {
			st.seen++
			st.missing = 0
			st.problem = problem
		}
		if !st.announced && st.seen >= s.timing.healthConfirm {
			st.announced = true
			s.dispatchHealth(s.healthEventOf(st.problem, st.since, false))
		}
		s.saveHealth(ctx, st)
	}
	for key, st := range s.health {
		if current[key] {
			continue
		}
		st.missing++
		if st.announced && st.missing < s.timing.healthConfirm {
			s.saveHealth(ctx, st)
			continue
		}
		if st.announced {
			s.dispatchHealth(s.healthEventOf(st.problem, st.since, true))
		}
		delete(s.health, key)
		if _, err := s.db.Exec(ctx, "DELETE FROM notification_health WHERE key = $1", key); err != nil && ctx.Err() == nil {
			s.logger.Debug("A solved health problem could not be forgotten", "error", err)
		}
	}
}

// saveHealth keeps st for the next run.
func (s *Service) saveHealth(ctx context.Context, st *healthState) {
	_, err := s.db.Exec(ctx, `INSERT INTO notification_health (key, severity, text, since, seen, missing, announced)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (key) DO UPDATE SET severity = excluded.severity, text = excluded.text, seen = excluded.seen,
			missing = excluded.missing, announced = excluded.announced`,
		st.problem.Key, st.problem.Severity, st.problem.Text, st.since, st.seen, st.missing, st.announced)
	if err != nil && ctx.Err() == nil {
		s.logger.Debug("A health problem could not be kept", "error", err)
	}
}

// dispatchHealth sends ev to the server's targets and administrators' own.
// It reads s.admins while dispatch holds s.mu.
func (s *Service) dispatchHealth(ev Event) {
	s.dispatch(ev, func(t target) bool { return t.owner == nil || s.admins[*t.owner] })
}
