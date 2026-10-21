package library

import (
	"cmp"
	"context"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Each user may hide movies, series and folders of videos from their lists
// (see userdata.HiddenTitle). A hidden title leaves that user's libraries,
// home rows, searches, similar titles, person pages, collections and
// Watchlist, as the genres a user blocks do, and a hidden series takes its
// seasons and episodes with it, a hidden folder its videos. Unlike a
// blocked title, it still opens by its identifier: its page, with its
// seasons and episodes, and its playback. Other users are not affected.

// HiddenTitles is the set of titles a user hid, by item.
type HiddenTitles map[accounts.ID]bool

// Hides reports whether item is one of the titles, or a season or an
// episode of one, or a video of one.
func (h HiddenTitles) Hides(item Item) bool {
	return h[item.ID] || (item.Kind == KindSeason || item.Kind == KindEpisode) && h[item.SeriesID] ||
		item.FolderID != (accounts.ID{}) && h[item.FolderID]
}

// Without returns items without those h hides, in order.
func (h HiddenTitles) Without(items []Item) []Item {
	if len(h) == 0 {
		return items
	}
	return slices.DeleteFunc(slices.Clone(items), h.Hides)
}

// Hideable reports whether a user may hide items of kind: movies, home
// videos, series and folders of videos. A season or an episode stands for
// its series, a folder's video for its folder.
func Hideable(kind Kind) bool {
	return kind == KindMovie || kind == KindVideo || kind == KindSeries || kind == KindFolder
}

// HiddenTitles returns the titles user hid, as their listings leave them
// out. It reads the view of the request, under PerRequest.
func (s *Service) HiddenTitles(ctx context.Context, user accounts.User) (HiddenTitles, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	return v.hidden, nil
}

func (s *Service) hiddenTitles(ctx context.Context, user accounts.ID) (HiddenTitles, error) {
	titles, err := s.userData.Hidden(ctx, user)
	if err != nil {
		return nil, err
	}
	hidden := make(HiddenTitles, len(titles))
	for _, title := range titles {
		hidden[title.ID] = true
	}
	return hidden, nil
}

// unhiddenMetas leaves out of a catalog page of src the titles the user
// hid; other entries, collections among them, stay.
func (v view) unhiddenMetas(src source, metas []stremio.Meta) []stremio.Meta {
	if len(v.hidden) == 0 {
		return metas
	}
	return slices.DeleteFunc(slices.Clone(metas), func(meta stremio.Meta) bool {
		kind, ok := titleKind(cmp.Or(meta.Type, src.catalog.Type))
		return ok && meta.ID != "" && v.hidden[itemID(titleKey(kind, meta.ID))]
	})
}
