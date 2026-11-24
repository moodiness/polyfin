package jellyfin

import (
	"errors"
	"net/http"
	"slices"

	"github.com/moodiness/polyfin/internal/collections"
	"github.com/moodiness/polyfin/internal/library"
)

// CollectionTypes is what /Polyfin/Items/{itemId}/CollectionTypes answers:
// the Jellyfin types of the entries a collection lists for the user, each
// once, by name. The web player's script offers a type filter on the
// collection's page when there are several, and then has jellyfin-web ask
// the collection's listing for the type chosen alone (IncludeItemTypes).
// An item that is not a collection holds none.
type CollectionTypes struct {
	Types []string
}

// collectionTypes answers /Polyfin/Items/{itemId}/CollectionTypes. A
// collection made by users tells the types of all its entries the user
// sees, the collections in it included, as its listing narrowed by type
// keeps them before its bound. A smart list shown as a collection, or an
// addon's collection, tells the types its listing without a limit lists,
// as jellyfin-web asks it; when that listing is bound short of the whole,
// a movie or a series past the bound counts too, found by the listing
// narrowed to its type, which reads only the catalogs that list it (see
// library.WithKinds).
func (h *Handler) collectionTypes(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	ctx := r.Context()
	var kinds []library.Kind
	add := func(kind library.Kind) {
		if !slices.Contains(kinds, kind) {
			kinds = append(kinds, kind)
		}
	}
	c, err := h.Collections.Get(ctx, id)
	switch {
	case err == nil:
		shown, err := h.shownCollections(ctx, user, []collections.Collection{c})
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		if len(shown) == 0 {
			notFoundProblem(w)
			return
		}
		view, err := h.viewCollections(ctx, user, c)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		for _, entry := range view.entries(c) {
			add(entry.Kind)
		}
	case !errors.Is(err, collections.ErrNotFound):
		h.internalError(w, r, err)
		return
	default:
		item, err := h.Library.Item(ctx, user, id)
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		if item.Kind != library.KindCollection {
			break
		}
		page, err := h.Library.Whole(ctx, user, id, 0, library.WholeListing, "")
		if errors.Is(err, library.ErrHidden) {
			notFoundProblem(w)
			return
		}
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		for _, title := range page.Items {
			add(title.Kind)
		}
		bound := page.More || page.Total > len(page.Items)
		for _, kind := range []library.Kind{library.KindMovie, library.KindSeries} {
			if !bound || slices.Contains(kinds, kind) {
				continue
			}
			only := library.WithKinds(ctx, func(listed library.Kind) bool { return listed == kind })
			narrowed, err := h.Library.Whole(only, user, id, 0, library.WholeListing, "")
			if err != nil {
				h.browseError(w, r, err)
				return
			}
			if slices.ContainsFunc(narrowed.Items, func(title library.Item) bool { return title.Kind == kind }) {
				add(kind)
			}
		}
	}
	types := []string{}
	for _, kind := range kinds {
		if name := itemTypes[kind]; name != "" && !slices.Contains(types, name) {
			types = append(types, name)
		}
	}
	slices.Sort(types)
	writeJSON(w, http.StatusOK, CollectionTypes{Types: types})
}
