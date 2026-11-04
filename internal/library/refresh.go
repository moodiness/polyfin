package library

import (
	"bytes"
	"context"
	"errors"
	"slices"

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
// addons fail, the title keeps the description it had. The files of local
// folders matched to the title, or a plain video's, are matched again
// first. The failed downloads of its artwork and of its people's photos
// are forgotten, and its people without a working photo are looked up on
// TMDB again (see refreshPeople). A person is looked up on TMDB again, as
// Jellyfin's refresh of a person runs its TmdbPersonProvider again. Other
// items have nothing to forget, but the server's libraries (see
// RefreshItem). ErrNotFound is returned for an item the user cannot
// reach.
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
	if r.Kind == KindPerson && r.Person != nil {
		return s.refreshPeople(ctx, user, []record{r}, nil, true)
	}
	if title.Meta == nil {
		return nil
	}
	// The local files of the title are matched again, as Jellyfin reads an
	// item's files again (ItemRefreshController.cs, RefreshItem): a plain
	// video may be found now, or a title its folder's name, NFO file or
	// link gives (see Local.Rematch).
	if kind, stremioID, ok := title.titleOf(); ok && s.local != nil {
		if err := s.local.Rematch(ctx, kind, stremioID); err != nil {
			return err
		}
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
	if !described {
		s.keptBefore(title, keptBy, err)
		err = nil
	}
	if refreshErr := s.refreshArtwork(ctx, user, v, title); err == nil {
		err = refreshErr
	}
	return err
}

// ArtworkFailures remembers the artwork downloads that failed (the image
// relay, jellyfin.Handler): ArtworkFailing reports one that failed last,
// or was found dead, and ForgetArtwork forgets those of urls, each then
// downloaded at its next request.
type ArtworkFailures interface {
	ArtworkFailing(url string) bool
	ForgetArtwork(urls []string)
}

// UseArtworkFailures sets what remembers the artwork downloads that
// failed, which Refresh forgets; it is called before the service is used.
// Without it, Refresh forgets none.
func (s *Service) UseArtworkFailures(f ArtworkFailures) { s.artwork = f }

// refreshArtwork forgets the failed downloads of a title's artwork, as its
// listing and its description give it, and refreshes its people (see
// refreshPeople): Jellyfin's refresh downloads an item's images again.
func (s *Service) refreshArtwork(ctx context.Context, user accounts.User, v view, title record) error {
	urls := metaArtwork(*title.Meta)
	if full, _, ok := s.describedMeta(ctx, v, title, false, inDatabase); ok {
		urls = append(urls, metaArtwork(full)...)
	}
	var item Item
	fromMeta(&item, *title.Meta)
	if poster, ok := PosterAddress(s.settings().PosterTemplate, title.Kind, item.ProviderIDs); ok {
		urls = append(urls, poster)
	}
	people, err := s.loadWhere(ctx, "kind = 'person' AND data->'credits' ? $1", title.ID.String())
	if err != nil {
		return err
	}
	return s.refreshPeople(ctx, user, people, urls, false)
}

// metaArtwork lists the addresses of the artwork a description gives: the
// title's and its episodes'.
func metaArtwork(meta stremio.Meta) []string {
	urls := []string{meta.Poster, meta.Background, meta.Logo, meta.LandscapePoster}
	for _, video := range meta.Videos {
		urls = append(urls, video.Thumbnail)
	}
	return urls
}

// refreshPeople forgets the failed downloads of people's photos, and of
// the artwork at urls, and the photos found dead (see PhotoFailed): each
// is downloaded again at its next request. The people without a working
// photo, or all of them when all is set, are due for a TMDB lookup, which
// starts now, paced as others are (see lookUp), TMDB's answers kept about
// them forgotten.
func (s *Service) refreshPeople(ctx context.Context, user accounts.User, people []record, urls []string, all bool) error {
	now := s.now()
	var due, changed []accounts.ID
	for _, r := range people {
		if r.Person == nil {
			continue
		}
		urls = append(urls, r.Person.Image)
		if r.TMDB != nil {
			urls = append(urls, r.TMDB.Photo)
		}
		for photo := range r.DeadPhotos {
			urls = append(urls, photo)
		}
		photo := r.personPhoto(now)
		working := photo != "" && (s.artwork == nil || !s.artwork.ArtworkFailing(photo))
		if all || !working {
			due = append(due, r.ID)
		}
		if all || !working || len(r.DeadPhotos) > 0 {
			changed = append(changed, r.ID)
		}
	}
	// The rows are taken in the order of their keys, as other writers of
	// several people take them.
	slices.SortFunc(changed, func(a, b accounts.ID) int { return bytes.Compare(a[:], b[:]) })
	if len(changed) > 0 {
		if _, err := s.db.Exec(ctx, `WITH locked AS (SELECT id FROM items WHERE id = ANY($1::uuid[]) ORDER BY id FOR UPDATE)
			UPDATE items SET data = CASE WHEN items.id = ANY($2::uuid[])
					THEN jsonb_set(items.data - 'deadPhotos', '{tmdb}', (COALESCE(items.data->'tmdb', '{}'::jsonb) - 'failedAt' - 'failures')
						|| '{"due": true}'::jsonb)
					ELSE items.data - 'deadPhotos' END,
				updated_at = now()
			FROM locked WHERE items.id = locked.id`, changed, due); err != nil {
			return err
		}
	}
	if s.artwork != nil {
		urls = slices.DeleteFunc(urls, func(url string) bool { return url == "" })
		s.artwork.ForgetArtwork(urls)
	}
	if !s.tmdb.Enabled() {
		return nil
	}
	language := peopleLanguage(user, s.settings())
	for _, r := range people {
		if !slices.Contains(due, r.ID) {
			continue
		}
		id := 0
		if r.TMDB != nil {
			id = r.TMDB.ID
		}
		s.tmdb.ForgetPerson(id, r.Person.Name)
		s.lookUp(r.ID, language)
	}
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
