package library

import (
	"context"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
)

// KnownContents gives the series among items that a catalog listed without
// their seasons and episodes the Contents their descriptions tell, from
// the descriptions of the user's addons kept in memory (see keptMeta): a
// listing counts what Polyfin knows, and asks neither an addon nor the
// database. It returns items, copied when it changes one; unchanged when
// the user's addons cannot be read, as the counts are only informational.
func (s *Service) KnownContents(ctx context.Context, user accounts.User, items []Item) []Item {
	if !slices.ContainsFunc(items, func(item Item) bool { return item.Kind == KindSeries && item.Contents == nil }) {
		return items
	}
	v, err := s.view(ctx, user)
	if err != nil {
		return items
	}
	now := s.now()
	result := slices.Clone(items)
	for i := range result {
		item := &result[i]
		if item.Kind != KindSeries || item.Contents != nil || item.StremioID == "" {
			continue
		}
		for _, candidate := range v.addons {
			if !serves(candidate, "meta", item.StremioType, item.StremioID) {
				continue
			}
			if kept, ok := s.metas.Get(metaKey{candidate.addon.ID, item.StremioType, item.StremioID}); ok && len(kept.value.Videos) > 0 {
				item.Contents = contents(kept.value, nil, now)
				break
			}
		}
	}
	return result
}
