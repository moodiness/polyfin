package library

import (
	"context"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// withholds reports whether r is a title out of the user's reach because
// only a server addon they do not use knows it (see addons.UsedBy): its
// addon is one of those, and none of the view's addons reaches it. A title
// another addon of theirs describes stays theirs, as a movie known by its
// IMDb identifier does.
func (v view) withholds(r record) bool {
	return r.Addon != nil && slices.Contains(v.withheld, *r.Addon) && !v.reaches(r)
}

// withholdsHolder reports whether r is out of the user's reach, as
// withholds tells, judged by the title that holds it: an episode or a
// season by its series, a folder's video by its folder.
func (s *Service) withholdsHolder(ctx context.Context, v view, r record) bool {
	if len(v.withheld) == 0 {
		return false
	}
	var holder accounts.ID
	switch {
	case r.Kind == KindSeason || r.Kind == KindEpisode:
		holder = r.seriesItemID()
	case r.folderVideo():
		holder = r.folderItemID()
	default:
		return v.withholds(r)
	}
	title, err := s.load(ctx, holder)
	return err == nil && v.withholds(title)
}

// withholdsAny reports whether one of the server's enabled addons is one
// user does not use, from the addons held in memory, with no view built.
// An error reads as yes: the caller then checks the title.
func (s *Service) withholdsAny(ctx context.Context, user accounts.User) bool {
	list, err := s.addons.Addons(ctx, addons.Shared())
	return err != nil || slices.ContainsFunc(list, func(addon addons.Addon) bool { return addon.Enabled && !addon.UsedBy.Uses(user) })
}

// WithheldAddons lists the server's enabled addons user does not use (see
// addons.UsedBy), in order: what their view leaves out, which what is
// kept for their listings is to be told apart by, as their view changes.
func (s *Service) WithheldAddons(ctx context.Context, user accounts.User) ([]accounts.ID, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	return v.withheld, nil
}
