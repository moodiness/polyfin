package jellyfin

import (
	"context"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/collections"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playlists"
)

// Containers whose titles are all out of reach.
//
// Jellyfin hides a collection or a playlist none of whose titles the user
// may see: "a container is only as visible as its members". Polyfin hides
// those made by users, collections and playlists, from a user none of
// whose titles they may see, whatever hides them: the user's parental
// control, unrated titles blocked, blocked genres, or the highest rating
// a listing asks (maxOfficialRating). One without titles still shows. A
// hidden one is left out of listings before they page and count, and its
// own pages answer as missing. The library hides the collections of
// addons and the smart lists shown as collections the same way (see the
// library package's containers.go).

// shownContainers reports, for each container holding the titles of
// members, whether user is shown it under ctx: when it holds none, or one
// of them is within their reach (see the comment above).
func (h *Handler) shownContainers(ctx context.Context, user accounts.User, members [][]accounts.ID) ([]bool, error) {
	shown := make([]bool, len(members))
	for i := range shown {
		shown[i] = true
	}
	ctx = library.WithMembersFilter(ctx)
	narrows, err := h.Library.Narrows(ctx, user)
	if err != nil || !narrows {
		return shown, err
	}
	var ids []accounts.ID
	seen := map[accounts.ID]bool{}
	for _, list := range members {
		for _, id := range list {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return shown, nil
	}
	items, err := h.Library.Items(ctx, user, ids)
	if err == nil {
		items, err = h.Library.RatingFiltered(ctx, user, items)
	}
	if err != nil {
		return nil, err
	}
	reached := make(map[accounts.ID]bool, len(items))
	for _, item := range items {
		reached[item.ID] = true
	}
	for i, list := range members {
		shown[i] = len(list) == 0 || slices.ContainsFunc(list, func(id accounts.ID) bool { return reached[id] })
	}
	return shown, nil
}

// shownCollections keeps the collections of lists user is shown, in
// order (see shownContainers).
func (h *Handler) shownCollections(ctx context.Context, user accounts.User, lists []collections.Collection) ([]collections.Collection, error) {
	members := make([][]accounts.ID, len(lists))
	for i, c := range lists {
		members[i] = c.Items
	}
	shown, err := h.shownContainers(ctx, user, members)
	if err != nil {
		return nil, err
	}
	kept := lists[:0:0]
	for i, c := range lists {
		if shown[i] {
			kept = append(kept, c)
		}
	}
	return kept, nil
}

// shownPlaylists keeps the playlists of lists user is shown, in order (see
// shownContainers).
func (h *Handler) shownPlaylists(ctx context.Context, user accounts.User, lists []playlists.Playlist) ([]playlists.Playlist, error) {
	members := make([][]accounts.ID, len(lists))
	for i, p := range lists {
		for _, entry := range p.Entries {
			members[i] = append(members[i], entry.Item)
		}
	}
	shown, err := h.shownContainers(ctx, user, members)
	if err != nil {
		return nil, err
	}
	kept := lists[:0:0]
	for i, p := range lists {
		if shown[i] {
			kept = append(kept, p)
		}
	}
	return kept, nil
}
