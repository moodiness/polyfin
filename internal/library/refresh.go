package library

import (
	"context"
	"errors"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Refresh asks the user's addons to describe a title again, and forgets
// what Polyfin keeps of it: the version and subtitle lists of the movie or
// of the series' episodes, stale ones included, the rating and genres
// looked up for parental control, and the ratings MDBList gave, asked for
// again as the title next shows. Its description (a season's or an
// episode's is their series') is replaced only once an addon answered:
// then the other addons' are forgotten, to be asked for again. While the
// addons fail, the title keeps the description it had. Items that are not
// titles have nothing to forget, but the server's libraries (see
// RefreshItem). ErrNotFound is returned for an item the user cannot reach.
func (s *Service) Refresh(ctx context.Context, user accounts.User, id accounts.ID) error {
	return s.RefreshItem(ctx, user, id, true)
}

// RefreshItem refreshes an item as Refresh does. A library of the server's
// that RefreshLibrary refreshes has, when descriptions is set, the
// descriptions of its titles read again in the background, paced, after
// the libraries already waiting, unless it is being refreshed or waits to
// be; it returns at once. Other libraries have nothing to refresh, nor
// has a library without descriptions. A title is refreshed whatever
// descriptions is.
func (s *Service) RefreshItem(ctx context.Context, user accounts.User, id accounts.ID, descriptions bool) error {
	v, err := s.view(ctx, user)
	if err != nil {
		return err
	}
	if _, err := s.item(ctx, v, id); err != nil {
		return err
	}
	if _, ok := v.library(id); ok {
		if !descriptions {
			return nil
		}
		_, err := s.RefreshLibrary(ctx, id)
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrNotRefreshable) {
			// A user's own library, or one whose descriptions are not
			// refreshed: nothing to do.
			return nil
		}
		return err
	}
	r, err := s.load(ctx, id)
	if errors.Is(err, ErrNotFound) {
		// An item Polyfin does not record, such as a smart list.
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
	// What addons are asked for the streams and subtitles of: the movie,
	// home video or channel itself, the episode, or every episode of the
	// series.
	var lists []string
	switch r.Kind {
	case KindMovie, KindVideo, KindChannel:
		lists = []string{title.Meta.ID}
	case KindEpisode:
		if r.Video != nil {
			lists = []string{r.Video.ID}
		}
	}
	if r.Kind == KindMovie || r.Kind == KindVideo || r.Kind == KindEpisode {
		if versions, ok := s.CachedVersions(ctx, user, id); ok {
			for _, version := range versions {
				s.versions.Delete(version.ID)
			}
		}
	}
	// series is the series' complete description, which the lists of its
	// episodes asked for by other identifiers are found by.
	var series *stremio.Meta
	if r.Kind == KindSeries || r.Kind == KindSeason {
		for _, entry := range v.addons {
			key := metaKey{entry.addon.ID, title.Meta.Type, title.Meta.ID}
			kept, ok := s.metas.Get(key)
			if !ok {
				kept, ok = s.loadMeta(ctx, entry, key)
			}
			if ok {
				lists = append(lists, videoIDs(kept.value, r)...)
				if series == nil {
					series = &kept.value
				}
			}
		}
	}
	for _, entry := range v.addons {
		for _, video := range lists {
			// Their follow-ups stop first, and the answers to the
			// requests under way are left out: one answering meanwhile
			// replaces no list, and none is brought back afterwards, not
			// even by a restart. Those requests ask the addon again
			// (see streamAnswer).
			s.forgetLists(ctx, streamKey{entry.addon.ID, title.Meta.Type, video})
		}
	}
	// So are those of the addons asked for them by another identifier
	// (see streamServing).
	for _, key := range s.otherStreamKeys(ctx, v, r, title, series) {
		s.forgetLists(ctx, key)
	}
	if _, err := s.db.Exec(ctx, "UPDATE items SET data = data - 'rating' - 'ratedAt' - 'genres' WHERE id = $1", title.ID); err != nil {
		return err
	}
	if t, ok := ratingsTitle(Item{Kind: title.Kind, ProviderIDs: providerIDs(*title.Meta)}); ok {
		if err := s.ratings.Forget(ctx, t); err != nil {
			return err
		}
	}
	// The description is asked for again now, as apps show the title's
	// page again after a refresh; it gives the rating again too.
	// An addon that fails leaves the title as it was: that is no failure
	// of the refresh.
	described, keptBy, err := s.describeAgain(ctx, v, title)
	if described {
		return err
	}
	s.keptBefore(title, keptBy, err)
	return nil
}

// describeAgain asks the addons for a title's description again (see
// redescribe). Once one answered, the other addons' descriptions are
// forgotten, to be asked for again. It reports whether one answered, and,
// when none did, the addon whose former description the title keeps, if
// any, and the first addon's failure.
func (s *Service) describeAgain(ctx context.Context, v view, title record) (described bool, keptBy string, err error) {
	answered, described, keptBy, failure := s.redescribe(ctx, v, title)
	if !described {
		return false, keptBy, failure
	}
	for _, entry := range v.addons {
		if entry.addon.ID == answered.addon.ID {
			continue
		}
		if err := s.forgetMeta(ctx, metaKey{entry.addon.ID, title.Meta.Type, title.Meta.ID}); err != nil {
			return true, "", err
		}
	}
	return true, "", nil
}

// redescribe asks the addons that describe r for its description again,
// in the order describeBy asks them, until one answers: its answer
// replaces the description it kept. An addon that fails while it keeps a
// description, the one r shows, leaves r described as it was, and keptBy
// names it. It returns the addon that answered, else the first failure.
func (s *Service) redescribe(ctx context.Context, v view, r record) (answered installed, described bool, keptBy string, failure error) {
	for _, candidate := range describers(v, r, false) {
		key := metaKey{candidate.addon.ID, r.Meta.Type, r.Meta.ID}
		var kept fetched[stremio.Meta]
		var err error
		if own(candidate.addon) {
			kept, err = s.keptMeta(ctx, candidate, key.metaType, key.id)
		} else {
			kept, err = s.fetchKeptMeta(ctx, candidate, key)
		}
		if err == nil {
			s.completed(ctx, v, r, candidate, kept, false)
			return candidate, true, "", nil
		}
		if ctx.Err() != nil {
			return installed{}, false, "", ctx.Err()
		}
		if failure == nil {
			failure = err
		}
		if _, ok := s.keptDescription(ctx, candidate, key, inDatabase); ok {
			return installed{}, false, candidate.addon.Manifest.Name, failure
		}
		if !notOwn(candidate, err) {
			s.logger.Debug("An addon could not describe a title", "addon", candidate.addon.Manifest.Name, "error", err)
		}
	}
	if ctx.Err() == nil {
		s.logger.Debug("No addon could describe a refreshed title", "item", r.ID)
	}
	return installed{}, false, "", failure
}

// keptBefore logs that a title could not be refreshed, once it is asked
// no more: it keeps the description the addon keptBy gave before.
func (s *Service) keptBefore(r record, keptBy string, err error) {
	if keptBy != "" {
		s.logger.Warn("A title could not be refreshed: it keeps the description the addon gave before",
			"item", r.ID, "addon", keptBy, "error", err)
	}
}

// videoIDs lists the videos of a series' description that r covers: all of
// them for the series, those of its season for a season (see covers).
func videoIDs(meta stremio.Meta, r record) []string {
	var ids []string
	for _, video := range meta.Videos {
		if r.covers(video) {
			ids = append(ids, video.ID)
		}
	}
	return ids
}
