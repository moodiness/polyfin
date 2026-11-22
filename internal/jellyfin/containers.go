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
// collection made by users that holds others shows when one of its own
// titles is within reach, or one of the collections in it shows. A
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
	reached, narrows, err := h.reachedTitles(ctx, user, ids)
	if err != nil || !narrows {
		return shown, err
	}
	for i, list := range members {
		shown[i] = len(list) == 0 || slices.ContainsFunc(list, func(id accounts.ID) bool { return reached[id] })
	}
	return shown, nil
}

// reachedTitles reports which of ids, each named once, are titles within
// user's reach under ctx, as containers judge their members (see the
// comment above). narrows is false when nothing is out of their reach,
// and reached then nil.
func (h *Handler) reachedTitles(ctx context.Context, user accounts.User, ids []accounts.ID) (reached map[accounts.ID]bool, narrows bool, err error) {
	ctx = library.WithMembersFilter(ctx)
	if narrows, err = h.Library.Narrows(ctx, user); err != nil || !narrows {
		return nil, false, err
	}
	reached = map[accounts.ID]bool{}
	if len(ids) == 0 {
		return reached, true, nil
	}
	items, err := h.Library.Items(ctx, user, ids)
	if err == nil {
		items, err = h.Library.RatingFiltered(ctx, user, items)
	}
	if err != nil {
		return nil, false, err
	}
	for _, item := range items {
		reached[item.ID] = true
	}
	return reached, true, nil
}

// shownCollections keeps the collections of lists user is shown, in
// order (see the comment above).
func (h *Handler) shownCollections(ctx context.Context, user accounts.User, lists []collections.Collection) ([]collections.Collection, error) {
	tree, err := h.collectionTree(ctx, lists)
	if err != nil {
		return nil, err
	}
	shown, err := h.shownInTree(ctx, user, tree)
	if err != nil {
		return nil, err
	}
	kept := lists[:0:0]
	for _, c := range lists {
		if shown[c.ID] {
			kept = append(kept, c)
		}
	}
	return kept, nil
}

// shownInTree reports, for each collection of tree, whether user is shown
// it under ctx: when it holds nothing, or one of its titles is within
// their reach, or they are shown one of the collections in it.
func (h *Handler) shownInTree(ctx context.Context, user accounts.User, tree collectionTree) (map[accounts.ID]bool, error) {
	reached, narrows, err := h.reachedTitles(ctx, user, tree.titles())
	if err != nil {
		return nil, err
	}
	shown := make(map[accounts.ID]bool, len(tree))
	var visit func(id accounts.ID) bool
	visit = func(id accounts.ID) bool {
		if known, ok := shown[id]; ok {
			return known
		}
		c, ok := tree[id]
		if !ok {
			return false
		}
		// A collection within itself, which nesting refuses, shows
		// nothing more there.
		shown[id] = false
		shown[id] = !narrows || len(c.Entries) == 0 || slices.ContainsFunc(c.Items, func(title accounts.ID) bool { return reached[title] }) ||
			slices.ContainsFunc(c.Collections, visit)
		return shown[id]
	}
	for id := range tree {
		visit(id)
	}
	return shown, nil
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
