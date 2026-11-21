package library

import (
	"context"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
)

// KnownSeries returns those of series whose episodes are known without
// asking any addon, in their order: a local folder's or an IPTV source's,
// which their database describes, and the others whose description by one
// of the user's addons is kept. The descriptions of the others are read in
// the background, paced as background work, at most smartLookups of them,
// smartFetches at a time (see describeLater), so that a later listing
// knows them. A library's Upcoming tab lists the coming episodes of its
// series this way: it answers at once, and fills as descriptions arrive.
func (s *Service) KnownSeries(ctx context.Context, user accounts.User, series []accounts.ID) ([]accounts.ID, error) {
	if len(series) == 0 {
		return nil, nil
	}
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	records, err := s.loadAll(ctx, series)
	if err != nil {
		return nil, err
	}
	known := map[accounts.ID]bool{}
	var remote []record
	for _, r := range records {
		if r.Kind != KindSeries || r.Addon == nil || r.Meta == nil {
			continue
		}
		if entry, ok := v.addon(*r.Addon); ok && own(entry.addon) {
			known[r.ID] = true
			continue
		}
		remote = append(remote, r)
	}
	descriptions, err := s.keptDescriptions(ctx, v, false, remote, false)
	if err != nil {
		return nil, err
	}
	var undescribed []record
	for _, r := range remote {
		if _, ok := descriptions[metaKey{*r.Addon, r.Meta.Type, r.Meta.ID}]; ok {
			known[r.ID] = true
		} else {
			undescribed = append(undescribed, r)
		}
	}
	s.describeLater(ctx, v, false, undescribed)
	return slices.DeleteFunc(slices.Clone(series), func(id accounts.ID) bool { return !known[id] }), nil
}
