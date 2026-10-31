package jellyfin

import (
	"context"
	"errors"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/source"
)

// With next-episode preloading on (Settings.PreloadNextEpisodes), Polyfin
// reads ahead into the source cache the files of the episodes the users
// who watch series lately would play next, while nothing plays, so that
// they start from the disk, after a restart too, as the cache keeps them.

const (
	// preloadFirst is how long after the start the first preload pass
	// comes, and preloadEvery how often the next ones do.
	preloadFirst = 2 * time.Minute
	preloadEvery = 15 * time.Minute
	// preloadRecent is how lately a user must have played or started an
	// episode of a series for its next episodes to be preloaded, and
	// preloadSeries how many of a user's series are, the latest first.
	preloadRecent = 14 * 24 * time.Hour
	preloadSeries = 5
)

// preloadLoop runs the preload passes until ctx is done.
func (h *Handler) preloadLoop(ctx context.Context) {
	timer := time.NewTimer(preloadFirst)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		h.preloadNext(ctx)
		timer.Reset(preloadEvery)
	}
}

// preloadNext reads ahead, for each user who played or started an episode
// within preloadRecent, the next PreloadEpisodes episodes of their
// preloadSeries latest series of Next Up, as they would play them: the
// first version a play would pick, under the user's parental control,
// hidden titles, version rules and quality group. It reads at most a
// quarter of the cache's size a pass, so that what was played stays, and
// stops as soon as something plays, or the disk reaches its reserve (see
// source.Source.Preload).
func (h *Handler) preloadNext(ctx context.Context) {
	settings := h.Accounts.Settings()
	idle := func() bool { return len(h.sessions.All()) == 0 }
	if !settings.PreloadNextEpisodes || !idle() {
		return
	}
	users, err := h.Accounts.Users(ctx)
	if err != nil {
		h.Logger.Debug("The users could not be listed to preload their next episodes", "error", err)
		return
	}
	limits := source.Preloading{Rate: int64(settings.PreloadMbps) * 1e6 / 8, Reserve: int64(settings.PreloadReserveGB) * 1e9, Idle: idle}
	budget := int64(settings.CacheSizeGB) * 1e9 / 4
	cutoff := h.now().Add(-preloadRecent)
	for _, user := range users {
		if user.IsDisabled {
			continue
		}
		for _, episode := range h.episodesToPreload(ctx, user, cutoff, settings.PreloadEpisodes) {
			if budget <= 0 {
				return
			}
			limits.Budget = budget
			fetched, err := h.preloadEpisode(ctx, user, episode, settings.PreloadWholeFiles, limits)
			budget -= fetched
			if errors.Is(err, source.ErrPreloadHeld) || ctx.Err() != nil {
				h.Logger.Debug("Preloading the next episodes stopped", "reason", err)
				return
			}
			if err != nil {
				h.Logger.Debug("An episode could not be preloaded", "user", user.Name, "error", err)
			}
		}
	}
}

// episodesToPreload lists the episodes to preload for user: of the first
// preloadSeries series of Next Up played or started since cutoff, the
// episode Next Up offers and the count-1 after it.
func (h *Handler) episodesToPreload(ctx context.Context, user accounts.User, cutoff time.Time, count int) []library.Item {
	next, err := h.nextUpItems(ctx, user, nil, nil, &cutoff, true)
	if err != nil {
		h.Logger.Debug("Next Up could not be listed to preload the next episodes", "user", user.Name, "error", err)
		return nil
	}
	var episodes []library.Item
	for _, first := range next[:min(len(next), preloadSeries)] {
		chain := []library.Item{first}
		if count > 1 {
			if all, err := h.Library.Episodes(ctx, user, first.SeriesID, nil); err == nil {
				for len(chain) < count {
					after, ok := episodeAfter(all, chain[len(chain)-1].ID)
					if !ok {
						break
					}
					chain = append(chain, after)
				}
			}
		}
		episodes = append(episodes, chain...)
	}
	return episodes
}

// preloadEpisode preloads the first version a play of episode would pick
// for user, within limits: none when it is a torrent's file, or failed
// lately. It returns the bytes fetched.
func (h *Handler) preloadEpisode(ctx context.Context, user accounts.User, episode library.Item, whole bool, limits source.Preloading) (int64, error) {
	// Through Library.Item, as details, for parental control.
	item, err := h.Library.Item(ctx, user, episode.ID)
	if err != nil {
		return 0, err
	}
	p, err := h.playable(ctx, user, item)
	if len(p.versions) == 0 {
		return 0, err
	}
	return h.Playback.Preload(ctx, p.versions[0], whole, limits)
}
