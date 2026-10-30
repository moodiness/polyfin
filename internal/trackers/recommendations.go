package trackers

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// A user who connected Trakt sees the movies and series Trakt recommends
// to them in the Recommended on Trakt home row, when the administrator
// adds one (see package homerows). Trakt's recommendations are read with
// the user's connection, at most recommendationsLimit movies and as many
// shows, leaving out those on the user's Trakt collection and watchlist,
// and their titles are found as an imported watchlist's are, by their
// IMDb, TMDB or TVDB identifier: those found by none are left out. The
// items found are kept in the database, best first, movies and shows in
// turn, and read again every recommendationsEvery: by the hourly check of
// the connections, for the users whose row asked for them within
// recommendationsAsked, and when a row finds them due. The first read
// starts when a row first asks: the row lists nothing until it ends. A
// read that fails keeps what was read before, and is tried again after
// recommendationsRetry. Disconnecting, or connecting again, forgets them.

// recommendationsLimit is how many movies, and how many shows, are asked
// of Trakt.
const recommendationsLimit = 50

// maxRecommended bounds the items kept, as the database does: a title
// listed under several identifiers is several items.
const maxRecommended = 500

// Recommends reports whether user has a connection to Trakt that works,
// whose recommendations a home row may list: Trakt is available on the
// server, and Trakt did not refuse the user's token.
func (s *Service) Recommends(ctx context.Context, user accounts.ID) (bool, error) {
	if !Available(Trakt, s.settings()) {
		return false, nil
	}
	conn, ok, err := s.connection(ctx, user, Trakt)
	return ok && conn.problem != ProblemReconnect, err
}

// Recommended lists the items of the movies and series Trakt recommends
// to user, best first, as last read: ErrNotConnected when the user has no
// connection to Trakt that works (see Recommends). The first read, and a
// read due, start in the background: none is listed until the first one
// ends.
func (s *Service) Recommended(ctx context.Context, user accounts.ID) ([]accounts.ID, error) {
	ok, err := s.Recommends(ctx, user)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotConnected
	}
	key := laneKey{user, Trakt}
	now := s.now()
	var ids []accounts.ID
	var due *time.Time
	err = s.db.QueryRow(ctx, `SELECT items, due_at FROM tracking_recommendations WHERE user_id = $1 AND service = $2`,
		user, Trakt).Scan(&ids, &due)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if _, err := s.db.Exec(ctx, `INSERT INTO tracking_recommendations (user_id, service, asked_at) VALUES ($1, $2, $3)
			ON CONFLICT (user_id, service) DO NOTHING`, user, Trakt, now); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	default:
		// The hourly check reads again the recommendations asked for
		// lately: a row asking within the hour need not say so.
		if _, err := s.db.Exec(ctx, `UPDATE tracking_recommendations SET asked_at = $3
			WHERE user_id = $1 AND service = $2 AND asked_at < $4`, user, Trakt, now, now.Add(-s.timing.refreshEvery)); err != nil {
			return nil, err
		}
	}
	if due == nil || !now.Before(*due) {
		s.startRecommendations(key)
	}
	return ids, nil
}

// recommendationsDue starts the reads due of the recommendations a row
// asked for lately, of connections that work.
func (s *Service) recommendationsDue() {
	if !Available(Trakt, s.settings()) {
		return
	}
	now := s.now()
	rows, err := s.db.Query(s.ctx, `SELECT r.user_id, r.service FROM tracking_recommendations r
		JOIN tracking_connections c ON c.user_id = r.user_id AND c.service = r.service
		WHERE c.problem IS DISTINCT FROM 'reconnect' AND (r.due_at IS NULL OR r.due_at <= $1) AND r.asked_at > $2`,
		now, now.Add(-s.timing.recommendationsAsked))
	if err != nil {
		s.logger.Warn("The Trakt recommendations due could not be read", "error", err)
		return
	}
	due, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (laneKey, error) {
		var key laneKey
		return key, row.Scan(&key.user, &key.service)
	})
	if err != nil {
		s.logger.Warn("The Trakt recommendations due could not be read", "error", err)
		return
	}
	for _, key := range due {
		s.startRecommendations(key)
	}
}

// startRecommendations reads the recommendations of key in the
// background, unless they are being read.
func (s *Service) startRecommendations(key laneKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recommending[key] {
		return
	}
	if s.spawn(func() {
		defer func() {
			s.mu.Lock()
			delete(s.recommending, key)
			s.mu.Unlock()
		}()
		s.readRecommendations(s.ctx, key)
	}) {
		s.recommending[key] = true
	}
}

// readRecommendations reads the recommendations of key from Trakt, finds
// their titles and keeps them.
func (s *Service) readRecommendations(ctx context.Context, key laneKey) {
	conn, ok, err := s.connection(ctx, key.user, key.service)
	if err != nil || !ok || conn.problem == ProblemReconnect || !Available(key.service, s.settings()) || s.titles == nil {
		return
	}
	r := &reader{s: s, key: key, token: conn.token, settings: s.settings()}
	refs, unknown, err := r.readTraktRecommendations(ctx)
	var ids []accounts.ID
	if err == nil {
		ids, err = s.recommendedItems(ctx, refs, &unknown)
	}
	if ctx.Err() != nil {
		return
	}
	now := s.now()
	if err != nil {
		s.logger.Info("The Trakt recommendations of a user could not be read", "user_id", key.user.String(), "problem", importProblem(err))
		if _, err := s.db.Exec(ctx, `UPDATE tracking_recommendations SET due_at = $3 WHERE user_id = $1 AND service = $2`,
			key.user, key.service, now.Add(s.timing.recommendationsRetry)); err != nil && ctx.Err() == nil {
			s.logger.Warn("The Trakt recommendations of a user could not be kept", "error", err)
		}
		return
	}
	if ids == nil {
		ids = []accounts.ID{}
	}
	if _, err := s.db.Exec(ctx, `UPDATE tracking_recommendations SET items = $3, due_at = $4 WHERE user_id = $1 AND service = $2`,
		key.user, key.service, ids, now.Add(s.timing.recommendationsEvery)); err != nil && ctx.Err() == nil {
		s.logger.Warn("The Trakt recommendations of a user could not be kept", "error", err)
		return
	}
	s.logger.Debug("Read the Trakt recommendations of a user", "user_id", key.user.String(), "found", len(ids), "unmapped", unknown)
}

// readTraktRecommendations reads the movies and shows Trakt recommends,
// in turn, and counts those known by no identifier Polyfin finds titles
// by, which it leaves out.
func (r *reader) readTraktRecommendations(ctx context.Context) (refs []library.TitleRef, unknown int, err error) {
	if err := r.authorize(ctx); err != nil {
		return nil, 0, err
	}
	query := url.Values{"limit": {strconv.Itoa(recommendationsLimit)}, "ignore_collected": {"true"}, "ignore_watchlisted": {"true"}}
	var lists [2][]library.TitleRef
	for i, kind := range [...]string{"movies", "shows"} {
		answer, err := r.get(ctx, "/recommendations/"+kind, query)
		if err != nil {
			return nil, 0, err
		}
		var items []titleJSON
		if json.Unmarshal(answer.body, &items) != nil {
			return nil, 0, ErrUnreachable
		}
		for _, item := range items[:min(len(items), recommendationsLimit)] {
			ref := item.ref(false, 0, 0)
			if kind == "shows" {
				ref = item.seriesRef()
			}
			if !usable(ref) {
				unknown++
				continue
			}
			lists[i] = append(lists[i], ref)
		}
	}
	for i := range max(len(lists[0]), len(lists[1])) {
		for _, list := range lists {
			if i < len(list) {
				refs = append(refs, list[i])
			}
		}
	}
	return refs, unknown, nil
}

// recommendedItems finds the items of refs, in order, each once, and adds
// to unknown those it finds none for.
func (s *Service) recommendedItems(ctx context.Context, refs []library.TitleRef, unknown *int) ([]accounts.ID, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	targets, err := s.titles.Resolve(ctx, refs)
	if err != nil {
		return nil, err
	}
	var ids []accounts.ID
	seen := map[accounts.ID]bool{}
	for i := range refs {
		if len(targets[i]) == 0 {
			*unknown++
		}
		for _, t := range targets[i] {
			if counts(targets[i], t) && !seen[t.ID] {
				seen[t.ID] = true
				ids = append(ids, t.ID)
			}
		}
	}
	return ids[:min(len(ids), maxRecommended)], nil
}
