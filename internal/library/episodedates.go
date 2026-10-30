package library

import (
	"context"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// The dates of series' episodes, which smart lists sort by (see
// sortSmart) and Jellyfin apps' "Date Episode Added" sort orders a
// library's series by (DateLastContentAdded, see LatestEpisodes).

// episodeDates are when a series' latest regular episode aired, and its
// latest season premiere, each zero when none aired.
type episodeDates struct {
	latest, premiere time.Time
}

// episodeDatesOf reads the episodes of a series' description: specials,
// in season 0, and episodes not aired by now are left out. A season's
// premiere is its earliest episode.
func episodeDatesOf(videos []stremio.Video, now time.Time) episodeDates {
	var dates episodeDates
	premieres := map[int]time.Time{}
	for _, video := range videos {
		released := exactDay(video.Released)
		if video.Season <= 0 || released == nil || released.After(now) {
			continue
		}
		if released.After(dates.latest) {
			dates.latest = *released
		}
		if first, ok := premieres[int(video.Season)]; !ok || released.Before(first) {
			premieres[int(video.Season)] = *released
		}
	}
	for _, premiere := range premieres {
		if premiere.After(dates.premiere) {
			dates.premiere = premiere
		}
	}
	return dates
}

// LatestEpisodes returns when the latest regular episode of each series
// of items aired, by item, as the descriptions kept of them give it: the
// series' own addon's, else another of the user's. Specials and episodes
// not aired yet are left out. Series whose description is not kept are
// missing, and settled false: theirs are read in the background, at most
// smartLookups at a time, so that the next listing knows them.
func (s *Service) LatestEpisodes(ctx context.Context, user accounts.User, items []Item) (dates map[accounts.ID]time.Time, settled bool,
	err error) {
	var ids []accounts.ID
	for _, item := range items {
		if item.Kind == KindSeries {
			ids = append(ids, item.ID)
		}
	}
	if len(ids) == 0 {
		return nil, true, nil
	}
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, false, err
	}
	records, err := s.loadAll(ctx, ids)
	if err != nil {
		return nil, false, err
	}
	var series []record
	for _, r := range records {
		if r.Kind == KindSeries && r.Addon != nil && r.Meta != nil {
			series = append(series, r)
		}
	}
	descriptions, err := s.keptDescriptions(ctx, v, false, series, true)
	if err != nil {
		return nil, false, err
	}
	now := s.now()
	dates = make(map[accounts.ID]time.Time, len(series))
	var undescribed []record
	for _, r := range series {
		meta, ok := descriptions[metaKey{*r.Addon, r.Meta.Type, r.Meta.ID}]
		if !ok {
			undescribed = append(undescribed, r)
			continue
		}
		if latest := episodeDatesOf(meta.Videos, now).latest; !latest.IsZero() {
			dates[r.ID] = latest
		}
	}
	s.describeLater(ctx, v, false, undescribed)
	return dates, len(undescribed) == 0, nil
}
