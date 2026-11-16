package jellyfin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// Libraries by name, for Silo's apps (see package silo).
//
// A library's titles are read a page at a time, in their catalog's order,
// whatever sort a listing asks (see items). Silo's apps sort a library by
// title, jump to a letter and show where each letter starts: they need the
// order a listing by name pages through, and the letters it files under.
// A library whose titles Polyfin stores, an IPTV source's movies or
// series or a local folder, is read whole from the database: Polyfin
// sorts it as library.NamePool lists it, with what the user may see only,
// and keeps the order for drawLife, so that the pages of one listing, and
// its letters, follow it. An addon's catalog, and a library of
// collections, keep their catalog's order, as in Jellyfin apps: reading
// one whole could take thousands of requests, and Silo's Apple app opens
// every library in title order. The Jellyfin API's own listings keep
// their order.
//
// GET /Polyfin/NameOrder lists a page of that order as /Items does, from
// startIndex, limit titles, in ascending or descending order (sortOrder).
// GET /Polyfin/NameOrder/Letters answers its letters (see library.Letters)
// in that order: Total, Letters and Runs, with Whole telling whether the
// order holds the whole library. Both take parentId, a library, and
// includeItemTypes, recursive and genres as /Items does; another parent
// answers 404, and a library whose titles Polyfin does not store 501.

// nameOrder is a library's titles in ascending order by name, with the
// keys they sort by, and whether they are the whole library.
type nameOrder struct {
	items []library.Item
	keys  []string
	whole bool
}

// nameOrderTitleSize is about what a title of an order by name takes in
// memory, with its key: the cache of orders bounds their total by it.
const nameOrderTitleSize = 2048

// nameOrderOf returns the order by name of the listing r asks for, or
// answers r and returns false.
func (h *Handler) nameOrderOf(w http.ResponseWriter, r *http.Request, b bindErrors) (accounts.User, nameOrder, bool) {
	parent, hasParent := b.guid(r, "parentId")
	user, ok := h.viewer(w, r, b, unknownListingUser)
	if !ok {
		return accounts.User{}, nameOrder{}, false
	}
	if !hasParent {
		notFoundProblem(w)
		return accounts.User{}, nameOrder{}, false
	}
	if h.hiddenLibrary(w, r, user, parent) {
		return accounts.User{}, nameOrder{}, false
	}
	genre, ok := h.genreFilter(r, user, parent)
	if !ok {
		// The library cannot be narrowed to the requested genre.
		return user, nameOrder{whole: true}, true
	}
	key, err := h.newDrawKey(r, user, parent, genre)
	if err != nil {
		h.browseError(w, r, err)
		return accounts.User{}, nameOrder{}, false
	}
	hidden, err := h.Library.HiddenTitles(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return accounts.User{}, nameOrder{}, false
	}
	// Hiding a title sorts again, so that positions follow at once.
	key.order = "name|" + hiddenFingerprint(hidden)
	if order, ok := h.nameOrders.Get(key); ok {
		return user, order, true
	}
	// The apps ask a list's first page and its letters at once: one of
	// them sorts, the other waits for it, even when the app gives up on
	// the one that sorts.
	shared := r.WithContext(context.WithoutCancel(r.Context()))
	built, err, _ := h.nameFlight.Do(fmt.Sprint(key), func() (any, error) {
		return h.sortByName(shared, user, parent, genre, hidden, key)
	})
	switch {
	case errors.Is(err, errNotLibrary):
		notFoundProblem(w)
	case errors.Is(err, errNotStored):
		writeJSON(w, http.StatusNotImplemented, "Polyfin sorts by name only the libraries whose titles it stores.")
	case err != nil:
		h.browseError(w, r, err)
	default:
		return user, built.(nameOrder), true
	}
	return accounts.User{}, nameOrder{}, false
}

var (
	// errNotLibrary is a listing by name of a parent that is not one of
	// the user's libraries.
	errNotLibrary = errors.New("not a library")
	// errNotStored is a listing by name of a library whose titles an
	// addon's catalog pages through.
	errNotStored = errors.New("not a stored library")
)

// sortByName sorts the titles of a library r lists by name, and keeps the
// order under key once it holds every title it sorts: those of the whole
// library, or as many as library.NamePool reads at most.
func (h *Handler) sortByName(r *http.Request, user accounts.User, parent accounts.ID, genre string, hidden library.HiddenTitles,
	key drawKey) (nameOrder, error) {
	keep := itemTypeFilter(r)
	page, bound, stored, isLibrary, err := h.Library.NamePool(listingContext(r, keep), user, parent, genre)
	switch {
	case err != nil:
		return nameOrder{}, err
	case !isLibrary:
		return nameOrder{}, errNotLibrary
	case !stored:
		return nameOrder{}, errNotStored
	}
	items := hidden.Without(slices.DeleteFunc(slices.Clone(page.Items), func(item library.Item) bool { return !keep(item) }))
	library.SortByName(items, false)
	order := nameOrder{items: items, keys: make([]string, len(items)), whole: !page.More}
	for i, item := range items {
		order.keys[i] = library.NameKey(item)
	}
	if !page.More || len(page.Items) >= bound {
		h.nameOrders.Put(key, order)
	}
	return order, nil
}

// hiddenFingerprint names the titles a user hid.
func hiddenFingerprint(hidden library.HiddenTitles) string {
	ids := make([]string, 0, len(hidden))
	for id, on := range hidden {
		if on {
			ids = append(ids, id.String())
		}
	}
	slices.Sort(ids)
	sum := sha256.Sum256([]byte(strings.Join(ids, ",")))
	return hex.EncodeToString(sum[:8])
}

func descendingOrder(r *http.Request) bool {
	orders := listQuery(r, "sortOrder")
	return len(orders) > 0 && strings.EqualFold(orders[0], "Descending")
}

// at is the position-th title of an order in the order asked: descending
// reads it from its end.
func (o nameOrder) at(position int, descending bool) int {
	if descending {
		return len(o.items) - 1 - position
	}
	return position
}

func (h *Handler) nameOrderPage(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	start, limit := b.paging(r, defaultPageSize)
	start = max(start, 0)
	user, order, ok := h.nameOrderOf(w, r, b)
	if !ok {
		return
	}
	descending := descendingOrder(r)
	found := make([]library.Item, 0, max(0, min(limit, len(order.items)-start)))
	for position := start; position < min(start+limit, len(order.items)); position++ {
		found = append(found, order.items[order.at(position, descending)])
	}
	keep := itemTypeFilter(r)
	items, err := h.dtos(r, user, found, requestedFields(r), keep)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: len(order.items), StartIndex: start})
}

// nameLetters is the answer of GET /Polyfin/NameOrder/Letters.
type nameLetters struct {
	Total   int
	Whole   bool
	Letters []library.Letter
	Runs    []library.Run
}

func (h *Handler) nameOrderLetters(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	_, order, ok := h.nameOrderOf(w, r, b)
	if !ok {
		return
	}
	keys := order.keys
	if descendingOrder(r) {
		keys = slices.Clone(keys)
		slices.Reverse(keys)
	}
	letters, runs := library.Letters(keys)
	writeJSON(w, http.StatusOK, nameLetters{Total: len(keys), Whole: order.whole, Letters: letters, Runs: runs})
}
