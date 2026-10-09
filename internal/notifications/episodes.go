package notifications

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

const (
	// followedLimit bounds the series looked at among those a user played,
	// the most recently played first, as the Upcoming row does: each needs
	// its description from its addon.
	followedLimit = 50
	// episodeWindow is how long after its release date an episode that
	// becomes available is still told of: an older one, added late by its
	// addon, is not news.
	episodeWindow = 7 * 24 * time.Hour
	// keepSeen is how long the episodes told of are remembered, well past
	// episodeWindow.
	keepSeen = 60 * 24 * time.Hour
	// describing bounds the series described at once for one user.
	describing = 2
)

// CheckEpisodes looks at the series each user follows, those they played
// an episode of or marked favorite, as the Upcoming row does, for episodes
// that became available since the last look, and tells them: to the
// user's targets, and once to the server's, whoever follows the series.
// Only the users who have a target for new episodes, or all of them when
// the server has one, are looked at. A series seen for the first time has
// the episodes it already has recorded without a message; episodes
// released more than a week ago, or without a release date, are never
// told of. The descriptions of the series are those the library keeps,
// which it refreshes in the background when they are old.
func (s *Service) CheckEpisodes(ctx context.Context) {
	s.episodesMu.Lock()
	defer s.episodesMu.Unlock()
	if err := s.reload(ctx); err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("The notification targets could not be read", "error", err)
		}
		return
	}
	server := false
	personal := map[accounts.ID]bool{}
	s.mu.Lock()
	for _, t := range s.targets {
		switch {
		case !t.wants(NewEpisode):
		case t.owner == nil:
			server = true
		default:
			personal[*t.owner] = true
		}
	}
	s.mu.Unlock()
	if !server && len(personal) == 0 {
		return
	}
	users, err := s.accounts.Users(ctx)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("The users could not be listed for new episodes", "error", err)
		}
		return
	}
	for _, user := range users {
		if user.IsDisabled || !server && !personal[user.ID] {
			continue
		}
		if err := s.checkUser(ctx, user, personal[user.ID], server); err != nil && ctx.Err() == nil {
			s.logger.Warn("A user's series could not be looked at for new episodes", "user_id", user.ID.String(), "error", err)
		}
		if ctx.Err() != nil {
			return
		}
	}
	old := s.now().Add(-keepSeen)
	if _, err := s.db.Exec(ctx, "DELETE FROM notification_episodes WHERE seen_at < $1", old); err != nil && ctx.Err() == nil {
		s.logger.Warn("Old new-episode records could not be deleted", "error", err)
	}
	if _, err := s.db.Exec(ctx, "DELETE FROM notification_server_episodes WHERE seen_at < $1", old); err != nil && ctx.Err() == nil {
		s.logger.Warn("Old new-episode records could not be deleted", "error", err)
	}
}

// followed lists the series user follows: those they played an episode
// of, the most recently played first, up to followedLimit, then their
// favorites that are not episodes, some of which are series.
func (s *Service) followed(ctx context.Context, user accounts.ID) ([]accounts.ID, error) {
	watched, err := s.userData.Episodes(ctx, user)
	if err != nil {
		return nil, err
	}
	favorites, err := s.userData.Favorites(ctx, user)
	if err != nil {
		return nil, err
	}
	var series []accounts.ID
	seen := map[accounts.ID]bool{}
	for _, e := range watched {
		if e.Played && !seen[e.Series] && len(series) < followedLimit {
			seen[e.Series] = true
			series = append(series, e.Series)
		}
	}
	for _, e := range favorites {
		if e.Series == (accounts.ID{}) && !seen[e.Item] {
			seen[e.Item] = true
			series = append(series, e.Item)
		}
	}
	return series, nil
}

// checkUser looks at the series user follows, telling the new episodes to
// their targets when toUser, and to the server's when toServer.
func (s *Service) checkUser(ctx context.Context, user accounts.User, toUser, toServer bool) error {
	series, err := s.followed(ctx, user.ID)
	if err != nil || len(series) == 0 {
		return err
	}
	rows, err := s.db.Query(ctx, "SELECT series_id FROM notification_series WHERE user_id = $1", user.ID)
	if err != nil {
		return err
	}
	known, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil {
		return err
	}
	seen := make(map[accounts.ID]bool, len(known))
	for _, id := range known {
		seen[id] = true
	}

	// Favorites that are not series, and series the user may no longer
	// see, list no episode: they are left out.
	found := make([][]library.Item, len(series))
	listed := make([]bool, len(series))
	var wg sync.WaitGroup
	limiter := make(chan struct{}, describing)
	for i, id := range series {
		wg.Go(func() {
			limiter <- struct{}{}
			defer func() { <-limiter }()
			if ctx.Err() != nil {
				return
			}
			episodes, err := s.library.Episodes(ctx, user, id, nil)
			if err == nil {
				found[i], listed[i] = episodes, true
			}
		})
	}
	wg.Wait()

	now := s.now()
	recent := now.Add(-episodeWindow)
	for i, id := range series {
		if !listed[i] {
			continue
		}
		var available []library.Item
		for _, episode := range found[i] {
			if episode.Available && episode.PremiereDate != nil && !episode.PremiereDate.Before(recent) {
				available = append(available, episode)
			}
		}
		first := !seen[id]
		if first {
			if _, err := s.db.Exec(ctx, "INSERT INTO notification_series (user_id, series_id, seen_at) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING",
				user.ID, id, now); err != nil {
				return err
			}
		}
		for _, episode := range available {
			tag, err := s.db.Exec(ctx, "INSERT INTO notification_episodes (user_id, episode_id, seen_at) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING",
				user.ID, episode.ID, now)
			if err != nil {
				return err
			}
			if first || tag.RowsAffected() == 0 {
				// Available when the series was first seen, or told already.
				continue
			}
			if toUser {
				s.dispatch(s.episodeEvent(&user, episode), func(t target) bool { return t.owner != nil && *t.owner == user.ID })
			}
			if toServer {
				tag, err := s.db.Exec(ctx, "INSERT INTO notification_server_episodes (episode_id, seen_at) VALUES ($1, $2) ON CONFLICT DO NOTHING",
					episode.ID, now)
				if err != nil {
					return err
				}
				if tag.RowsAffected() > 0 {
					s.dispatch(s.episodeEvent(nil, episode), func(t target) bool { return t.owner == nil })
				}
			}
		}
	}
	return nil
}
