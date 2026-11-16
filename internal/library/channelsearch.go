package library

import (
	"context"

	"github.com/moodiness/polyfin/internal/accounts"
)

// SearchChannels lists the user's live TV channels, as Channels lists and
// numbers them, whose name, as administrators may have changed it, holds
// reports, the first limit of them, all of them when limit is negative. It
// records the channels it finds, which apps open next, and no other: a
// search tests every channel's name but lists a few, and Channels records
// the others as it lists them.
func (s *Service) SearchChannels(ctx context.Context, user accounts.User, holds func(name string) bool, limit int) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	channels, records := s.channels(ctx, v)
	set := s.overridesNow()
	var found []Item
	var kept []record
	for i, channel := range channels {
		if limit >= 0 && len(found) >= limit {
			break
		}
		name := channel.Name
		if o, ok := set.fields[channel.ID]; ok && o.Name != nil {
			name = *o.Name
		}
		if holds(name) {
			found, kept = append(found, channel), append(kept, records[i])
		}
	}
	return s.overridden(found), s.save(ctx, kept)
}
