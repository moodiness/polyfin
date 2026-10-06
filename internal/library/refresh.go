package library

import (
	"context"
	"errors"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Refresh forgets what Polyfin keeps of a title for the user's addons, and
// asks them to describe it again: its description (a season's or an
// episode's is their series'), the version and subtitle lists of the movie
// or of the series' episodes, stale ones included, and the rating and
// genres looked up for parental control. Items that are not titles have
// nothing to forget.
// ErrNotFound is returned for an item the user cannot reach.
func (s *Service) Refresh(ctx context.Context, user accounts.User, id accounts.ID) error {
	v, err := s.view(ctx, user)
	if err != nil {
		return err
	}
	if _, err := s.item(ctx, v, id); err != nil {
		return err
	}
	r, err := s.load(ctx, id)
	if errors.Is(err, ErrNotFound) {
		// A library, which Polyfin does not record.
		return nil
	}
	if err != nil {
		return err
	}
	title := r
	if r.Kind == KindSeason || r.Kind == KindEpisode {
		if title, err = s.load(ctx, r.seriesItemID()); err != nil {
			return err
		}
	}
	if title.Meta == nil {
		return nil
	}
	// What addons are asked for the streams and subtitles of: the movie or
	// channel itself, the episode, or every episode of the series.
	var lists []string
	switch r.Kind {
	case KindMovie, KindChannel:
		lists = []string{title.Meta.ID}
	case KindEpisode:
		if r.Video != nil {
			lists = []string{r.Video.ID}
		}
	}
	if r.Kind == KindMovie || r.Kind == KindEpisode {
		if versions, ok := s.CachedVersions(ctx, user, id); ok {
			for _, version := range versions {
				s.versions.Delete(version.ID)
			}
		}
	}
	for _, entry := range v.addons {
		key := metaKey{entry.addon.ID, title.Meta.Type, title.Meta.ID}
		if r.Kind == KindSeries || r.Kind == KindSeason {
			if meta, ok := s.metas.Get(key); ok {
				lists = append(lists, videoIDs(meta, r)...)
			}
		}
		s.metas.Delete(key)
	}
	for _, entry := range v.addons {
		for _, video := range lists {
			// Their follow-ups stop first: one answering meanwhile
			// replaces no list, and none is brought back afterwards,
			// not even by a restart.
			s.forgetLists(ctx, streamKey{entry.addon.ID, title.Meta.Type, video})
		}
	}
	if _, err := s.db.Exec(ctx, "UPDATE items SET data = data - 'rating' - 'ratedAt' - 'genres' WHERE id = $1", title.ID); err != nil {
		return err
	}
	// The description is asked for again now, as apps show the title's
	// page again after a refresh; it gives the rating again too.
	if _, described := s.titleMeta(ctx, v, title); !described && ctx.Err() == nil {
		s.logger.Debug("No addon could describe a refreshed title", "item", title.ID)
	}
	return nil
}

// videoIDs lists the videos of a series' description that r covers: all of
// them for the series, those of its season for a season.
func videoIDs(meta stremio.Meta, r record) []string {
	var ids []string
	for _, video := range meta.Videos {
		if r.Kind == KindSeries || int(video.Season) == r.Season {
			ids = append(ids, video.ID)
		}
	}
	return ids
}
