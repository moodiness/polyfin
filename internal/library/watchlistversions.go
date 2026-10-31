package library

import (
	"context"
	"slices"
	"sync"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// WatchlistVersions lists the versions of a movie or an episode for the
// checks of the watchlists (see package watchlist), as Versions does, a
// list remembered while fresh included, but within the wait rules of the
// addons that list catalogs (see catalogWaits): an addon whose server asked
// to wait is not asked, and an addon that answers "too many requests"
// (429, or 503 with Retry-After) starts its server's wait, as a refused
// catalog page does, so that the checks that follow do not ask it either.
// Only the answers of the addons asked count: complete is false when an
// addon was not asked, failed, or answered only notices, in which case
// versions are those of the others. A movie waiting for its digital
// release has none, and its addons are not asked.
func (s *Service) WatchlistVersions(ctx context.Context, user accounts.User, id accounts.ID) (versions []Version, complete bool, err error) {
	t, v, err := s.target(ctx, user, id)
	if err != nil {
		return nil, false, err
	}
	if t.kind != KindMovie && t.kind != KindEpisode {
		return nil, false, ErrNotFound
	}
	if t.kind == KindMovie {
		if _, held, err := s.upcoming(ctx, v, id); err != nil || held {
			return nil, err == nil, err
		}
	}
	s.embed(ctx, v, &t, true)
	serving := s.streamServing(ctx, v, t)
	if len(t.embedded) > 0 {
		serving = slices.DeleteFunc(serving, func(source streamSource) bool { return source.addon.ID == t.embeddedBy.addon.ID })
	}
	complete = true
	answered := make([]bool, len(serving))
	var wg sync.WaitGroup
	for i, entry := range serving {
		if own(entry.addon) {
			answered[i] = true
			continue
		}
		if _, waits := s.catalogWaitLeft(stremio.HostOf(entry.addon.ManifestURL)); waits {
			continue
		}
		wg.Go(func() {
			_, err := s.streams(ctx, entry.installed, entry.contentType, entry.id)
			if err == nil {
				answered[i] = true
				return
			}
			if ctx.Err() == nil {
				s.catalogRefused(entry.addon.ManifestURL, err)
			}
		})
	}
	wg.Wait()
	var asked []streamSource
	for i, entry := range serving {
		if answered[i] {
			asked = append(asked, entry)
		} else {
			complete = false
		}
	}
	versions, unknown := s.listVersions(ctx, t, asked, knownOnly, s.settings().TorrentsAllowed(user))
	return versions, complete && len(unknown) == 0, ctx.Err()
}
