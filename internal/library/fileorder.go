package library

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// Local files numbered in an episode order.
//
// A local folder's show may have its files numbered in one of the
// series' TMDB episode groups, as Jellyfin's episodes are its files, at
// the season and number they tell in the series' display order
// (MediaBrowser.Providers/Plugins/Tmdb/TmdbClientManager.cs, lines
// 276-297 at v12.4). Polyfin's episodes are the catalog's: such a file is
// a version of the catalog's episode the group holds at its season and
// number, read as Jellyfin reads a group, its group at order N being
// season N and its episode at order N episode N+1.

// errOrderLate reports that TMDB did not tell an episode group within
// orderWait.
var errOrderLate = errors.New("TMDB did not tell the episode group in time")

// OrderPlace is an episode of an episode group: the season and number the
// catalog gives it, and whether the catalog lists it.
type OrderPlace struct {
	Season, Episode int
	Listed          bool
}

// EpisodeGroupsOf lists the TMDB episode groups of the series meta
// describes: tmdb.ErrNoKey without a TMDB key, tmdb.ErrNotFound for a
// series TMDB does not know.
func (s *Service) EpisodeGroupsOf(ctx context.Context, meta stremio.Meta) ([]tmdb.GroupSummary, error) {
	if !s.tmdb.Enabled() {
		return nil, tmdb.ErrNoKey
	}
	_, groups, err := s.seriesGroups(ctx, meta)
	return groups, err
}

// FileOrder returns the episodes of the TMDB episode group group of the
// series meta describes, by their season and number in the group, each
// at the catalog's place, meta's videos being the catalog. It fails with
// tmdb.ErrNoKey without a TMDB key, ErrUnknownGroup when the series has
// no such group, and when TMDB does not tell it within orderWait.
func (s *Service) FileOrder(ctx context.Context, meta stremio.Meta, group string) (map[[2]int]OrderPlace, error) {
	if !s.tmdb.Enabled() {
		return nil, tmdb.ErrNoKey
	}
	type found struct {
		group tmdb.EpisodeGroup
		show  int
		err   error
	}
	done := make(chan found, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		var f found
		var groups []tmdb.GroupSummary
		f.show, groups, f.err = s.seriesGroups(ctx, meta)
		switch {
		case f.err != nil:
		case !slices.ContainsFunc(groups, func(g tmdb.GroupSummary) bool { return g.ID == group }):
			f.err = ErrUnknownGroup
		default:
			f.group, f.err = s.tmdb.EpisodeGroupOf(ctx, group)
		}
		done <- f
	}()
	wait := time.NewTimer(orderWait)
	defer wait.Stop()
	var f found
	select {
	case f = <-done:
	case <-wait.C:
		return nil, errOrderLate
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if f.err != nil {
		return nil, f.err
	}
	return orderPlaces(meta.Videos, f.group, f.show), nil
}

// orderPlaces maps the episodes of group, an episode group of the series
// TMDB identifies as show, by their season and number in the group, to
// their places among videos, the catalog's, matched by TMDB's season and
// number (see tmdbNumbers).
func orderPlaces(videos []stremio.Video, group tmdb.EpisodeGroup, show int) map[[2]int]OrderPlace {
	byNumber := make(map[[2]int]stremio.Video, len(videos))
	for _, video := range videos {
		if numbers := tmdbNumbers(video); byNumber[numbers].ID == "" {
			byNumber[numbers] = video
		}
	}
	places := map[[2]int]OrderPlace{}
	for _, g := range group.Groups {
		for _, e := range g.Episodes {
			if e.Show != 0 && show != 0 && e.Show != show {
				continue
			}
			place := OrderPlace{Season: e.Season, Episode: e.Episode}
			if video, ok := byNumber[[2]int{e.Season, e.Episode}]; ok {
				place = OrderPlace{Season: int(video.Season), Episode: video.EpisodeNumber(), Listed: true}
			}
			if _, taken := places[[2]int{g.Order, e.Order + 1}]; !taken {
				places[[2]int{g.Order, e.Order + 1}] = place
			}
		}
	}
	return places
}
