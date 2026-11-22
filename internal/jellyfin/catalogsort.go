package jellyfin

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// Addon libraries sorted.
//
// /Items lists the titles of an addon's library through its folders in the
// orders of storedSortKeys but one: by name ascending, which every app
// asks for its default order as for Name, the library keeps its catalog's
// order, a popular or a trending catalog's point, and pages through it as
// it reads it. By name descending, by date added, release date, year,
// rating or length, it sorts the titles Polyfin read of the catalog (see
// library.Service.CatalogPool), as stored libraries are (see
// storedListing), and has the rest read in the background: until the
// catalog is read to its end, or to the most titles the library lists,
// the listing sorts those known, counts them as its total, and tells so
// with partialTotalHeader. A title's date added is when Polyfin first saw
// it in the catalog. The order is kept for the catalog's version (see
// library.Service.CatalogVersion): a page the addon gives anew, an edit,
// or an addon or library changed sorts again.

// catalogListing answers a listing of the titles of parent through its
// folders, sorted as sort asks, when parent is an addon's library that is
// sorted so, and reports whether it did. Other parents, and a listing by
// name ascending, are not answered here.
func (h *Handler) catalogListing(w http.ResponseWriter, r *http.Request, user accounts.User, parent accounts.ID, genre string,
	rating library.RatingFilter, sort titleSort, fields fieldSet, keep func(library.Item) bool, start, limit int) bool {
	if sort.key == "SortName" && !sort.descending {
		return false
	}
	version, ok, err := h.Library.CatalogVersion(r.Context(), user, parent, genre)
	if err != nil {
		h.browseError(w, r, err)
		return true
	}
	if !ok {
		return false
	}
	if wholeListing(r) {
		limit = library.WholeListing
	}
	start = max(start, 0)
	order, err := h.sortedOrder(r.Context(), user, parent, genre, rating, sort, catalogVersion(version), h.sortCatalog(version, start+limit))
	switch {
	case r.Context().Err() != nil:
		// The app gave up waiting.
		return true
	case errors.Is(err, errNotLibrary) || errors.Is(err, errNotStored):
		// The user's libraries changed meanwhile.
		return false
	case err != nil:
		h.browseError(w, r, err)
		return true
	}
	found, total := order.page(keep, start, limit, sort.descending)
	items, err := h.dtos(r, user, found, fields, keep)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	if !order.whole {
		w.Header().Set(partialTotalHeader, "true")
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: total, StartIndex: start})
	return true
}

// catalogVersion names the titles of an addon's catalog at version (see
// library.Service.CatalogVersion) as sortedOrder keeps them apart.
func catalogVersion(version string) string {
	return "catalog " + version + "|"
}

// sortCatalog returns what sorts by name the titles read of the catalog of
// an addon's library at version, its first need titles at least (see
// library.Service.CatalogPool), and keeps the order under its key when it
// holds the whole library and the catalog's version did not change as its
// titles were read: the next listings find it until another page comes.
// An order of part of the catalog is sorted again at each listing, which
// then sorts the titles read since too.
func (h *Handler) sortCatalog(version string, need int) orderBuild {
	return func(ctx context.Context, user accounts.User, parent accounts.ID, genre string, hidden library.HiddenTitles,
		key drawKey) (nameOrder, bool, error) {
		every := func(library.Kind) bool { return true }
		listing := library.WithTitlesThrough(library.WithKinds(ctx, every))
		started := h.now()
		page, read, isCatalog, err := h.Library.CatalogPool(listing, user, parent, genre, need)
		switch {
		case err != nil:
			return nameOrder{}, false, err
		case !isCatalog:
			return nameOrder{}, false, errNotLibrary
		}
		order := sortedByName(hidden.Without(page.Items), !page.More)
		order.added, order.sorted = true, started
		if read != version || page.More {
			return order, false, nil
		}
		h.nameOrders.Put(key, order)
		return order, true, nil
	}
}

// sortedByName is the order by name of pool, titles in their catalog's
// order, whole when they are the whole library.
func sortedByName(pool []library.Item, whole bool) nameOrder {
	keys := make([]string, len(pool))
	for i := range pool {
		keys[i] = library.NameKey(pool[i])
	}
	// The places of the titles in the catalog, in their order by name.
	ranks := make([]int32, len(pool))
	for i := range ranks {
		ranks[i] = int32(i)
	}
	slices.SortFunc(ranks, func(a, b int32) int { return library.CompareNames(&pool[a], &pool[b], keys[a], keys[b]) })
	order := nameOrder{items: make([]library.Item, len(pool)), keys: make([]string, len(pool)), ranks: ranks, whole: whole}
	for i, rank := range ranks {
		order.items[i], order.keys[i] = pool[rank], keys[rank]
	}
	return order
}
