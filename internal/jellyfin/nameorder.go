package jellyfin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"

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
// One order holds every kind of title of a library, for a user, their
// limits and the titles they hid, and a genre: each listing keeps the
// kinds it asks for (includeItemTypes, excludeItemTypes, mediaTypes,
// filters) as it pages, so that listings asking for other kinds share it.
// At most nameSortsAtOnce orders are sorted at once on the server, and a
// sort stops once no listing waits for it.
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

// kept is the order of the titles keep keeps.
func (o nameOrder) kept(keep func(library.Item) bool) nameOrder {
	kept := nameOrder{whole: o.whole}
	for i, item := range o.items {
		if keep(item) {
			kept.items, kept.keys = append(kept.items, item), append(kept.keys, o.keys[i])
		}
	}
	return kept
}

// nameOrderTitleSize is what a title of an order by name takes in memory,
// with its key: an order of 50,000 IPTV movies, names of 30 characters
// without posters, held 64.3 MB, 1,286 bytes a title, and posters and
// longer names add some. The cache of orders bounds their total by it.
const nameOrderTitleSize = 1536

// nameSortsAtOnce bounds the orders by name sorted at once on the server.
const nameSortsAtOnce = 2

// nameBuilds are the orders by name being sorted, one for each key, which
// the listings that ask for it wait for together.
type nameBuilds struct {
	mu     sync.Mutex
	builds map[drawKey]*nameBuild
	// sorts holds a token for each sort under way (see nameSortsAtOnce).
	sorts chan struct{}
}

// nameBuild is an order being sorted: the listings waiting for it, and its
// result once done is closed. Its sort stops when the last waiter leaves.
type nameBuild struct {
	done    chan struct{}
	order   nameOrder
	err     error
	waiters int
	stop    context.CancelFunc
	stopped bool
}

func newNameBuilds() *nameBuilds {
	return &nameBuilds{builds: map[drawKey]*nameBuild{}, sorts: make(chan struct{}, nameSortsAtOnce)}
}

// wait returns the order under key, sorted by sort once for the listings
// that ask for it meanwhile. The sort runs under ctx's values, and stops
// when every listing waiting for it has given up.
func (b *nameBuilds) wait(ctx context.Context, key drawKey, sort func(context.Context) (nameOrder, error)) (nameOrder, error) {
	b.mu.Lock()
	build, ok := b.builds[key]
	if !ok || build.stopped {
		sorting, stop := context.WithCancel(context.WithoutCancel(ctx))
		build = &nameBuild{done: make(chan struct{}), stop: stop}
		b.builds[key] = build
		go b.run(sorting, key, build, sort)
	}
	build.waiters++
	b.mu.Unlock()
	select {
	case <-build.done:
		return build.order, build.err
	case <-ctx.Done():
		b.mu.Lock()
		if build.waiters--; build.waiters == 0 {
			build.stopped = true
			build.stop()
		}
		b.mu.Unlock()
		return nameOrder{}, ctx.Err()
	}
}

func (b *nameBuilds) run(ctx context.Context, key drawKey, build *nameBuild, sort func(context.Context) (nameOrder, error)) {
	defer build.stop()
	select {
	case b.sorts <- struct{}{}:
		build.order, build.err = sort(ctx)
		<-b.sorts
	case <-ctx.Done():
		build.err = ctx.Err()
	}
	b.mu.Lock()
	if b.builds[key] == build {
		delete(b.builds, key)
	}
	b.mu.Unlock()
	close(build.done)
}

// nameOrderOf returns the order by name of the listing r asks for, with
// the kinds of titles it keeps, or answers r and returns false.
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
	// One order holds every kind; hiding a title sorts again, so that
	// positions follow at once.
	key.kinds, key.order = "", "name|"+hiddenFingerprint(hidden)
	keep := itemTypeFilter(r)
	if order, ok := h.nameOrders.Get(key); ok {
		return user, order.kept(keep), true
	}
	order, err := h.nameBuilds.wait(r.Context(), key, func(ctx context.Context) (nameOrder, error) {
		return h.sortByName(ctx, user, parent, genre, hidden, key)
	})
	switch {
	case r.Context().Err() != nil:
		// The app gave up waiting.
	case errors.Is(err, errNotLibrary):
		notFoundProblem(w)
	case errors.Is(err, errNotStored):
		writeJSON(w, http.StatusNotImplemented, "Polyfin sorts by name only the libraries whose titles it stores.")
	case err != nil:
		h.browseError(w, r, err)
	default:
		return user, order.kept(keep), true
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

// sortByName sorts every title of a library by name, as a listing of its
// titles through its folders reads them, and keeps the order under key
// once it holds every title it sorts: those of the whole library, or as
// many as library.NamePool reads at most.
func (h *Handler) sortByName(ctx context.Context, user accounts.User, parent accounts.ID, genre string, hidden library.HiddenTitles,
	key drawKey) (nameOrder, error) {
	every := func(library.Kind) bool { return true }
	listing := library.WithTitlesThrough(library.WithKinds(library.WithOpening(ctx), every))
	page, bound, stored, isLibrary, err := h.Library.NamePool(listing, user, parent, genre)
	switch {
	case err != nil:
		return nameOrder{}, err
	case !isLibrary:
		return nameOrder{}, errNotLibrary
	case !stored:
		return nameOrder{}, errNotStored
	}
	items := hidden.Without(slices.Clone(page.Items))
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
