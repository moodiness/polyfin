package jellyfin

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/library"
)

// Libraries by name, for Silo's apps (see package silo), and sorted as
// jellyfin-web's Sort menu asks.
//
// A library's titles are read a page at a time, in their catalog's order,
// whatever sort a listing asks (see items). Silo's apps sort a library by
// title, jump to a letter and show where each letter starts: they need the
// order a listing by name pages through, and the letters it files under.
// A library whose titles Polyfin stores, an IPTV source's movies or
// series or a local folder, is read whole from the database: Polyfin
// sorts it as library.NamePool lists it, with what the user may see only,
// and keeps the order orderIdle after its last read, so that the pages of
// one listing, its letters and the next listings of a session follow it,
// unless what the library lists changes meanwhile: a scan, a link, a
// refresh of its source or an edit sorts it again (see storedOrder). An
// order read more than orderFresh after its titles is sorted again in the
// background. An addon's catalog, and a library of collections, keep
// their catalog's order, as in Jellyfin apps: reading one whole could
// take thousands of requests, and Silo's Apple app opens every library in
// title order.
//
// /Items lists a stored library's titles through its folders in the
// orders of storedSortKeys (see storedListing), each sorted from the
// order by name and kept as it is: by its first sortBy key, then by name.
// The order by date added is by the date each title tells as its
// DateCreated (see dateAdded): a stored library's catalog lists the titles
// added last first, an IPTV source's by the date its provider gives, a
// local folder's by its files' dates, and titles of one date keep its
// order. A title its provider gives no date tells its premiere date.
// As in Jellyfin, a title without a value comes first in ascending order
// and last in descending order.
//
// One order holds every kind of title of a library, for the users who see
// it alike (see library.Service.ListingPrint) and hid the same titles, a
// genre and the ratings a listing narrows to: each listing keeps the kinds
// it asks for (includeItemTypes, excludeItemTypes, mediaTypes, filters)
// as it pages, so that listings asking for other kinds share it. At most
// nameSortsAtOnce orders are sorted at once on the server, and a sort
// stops once no listing waits for it.
//
// Reading a library of 10,000 titles whole takes some 200 ms, sorting it
// 5 ms: once Polyfin started and read the libraries' first pages, it
// sorts by name the server's libraries each user sees, one at a time,
// once the apps' first screens are served (see sortAhead), so that the
// first listings after a start find their order.
//
// GET /Polyfin/NameOrder lists a page of the order by name as /Items does,
// from startIndex, limit titles, in ascending or descending order
// (sortOrder). GET /Polyfin/NameOrder/Letters answers its letters (see
// library.Letters) in that order: Total, Letters and Runs, with Whole
// telling whether the order holds the whole library. Both take parentId,
// a library, and includeItemTypes, recursive and genres as /Items does;
// another parent answers 404, and a library whose titles Polyfin does not
// store 501.

// nameOrder is a library's titles in ascending order by name, with the
// keys they sort by, and whether they are the whole library. ranks are
// the places of the titles in their catalog. An order by another key
// holds the same titles, with places, the indexes in items of its titles
// in ascending order; places is nil for the order by name. added is set
// for the titles of an addon's catalog, which tell when they were added
// (see library.Item.Added). sorted is when the titles were read, which an
// order by another key keeps.
type nameOrder struct {
	items  []library.Item
	keys   []string
	ranks  []int32
	places []int32
	whole  bool
	added  bool
	sorted time.Time
}

// keptKeys are the keys of the titles of the order by name that keep
// keeps, in ascending order.
func (o nameOrder) keptKeys(keep func(library.Item) bool) []string {
	keeps := byKind(keep)
	keys := make([]string, 0, len(o.keys))
	for i := range o.items {
		if keeps(o.items[i].Kind) {
			keys = append(keys, o.keys[i])
		}
	}
	return keys
}

// page returns the titles of the order that keep keeps, in the order
// asked, from start, up to limit of them, and how many it keeps in all.
func (o nameOrder) page(keep func(library.Item) bool, start, limit int, descending bool) ([]library.Item, int) {
	keeps := byKind(keep)
	found := make([]library.Item, 0, max(0, min(limit, len(o.items)-start)))
	total := 0
	for position := range o.items {
		item := &o.items[o.at(position, descending)]
		if !keeps(item.Kind) {
			continue
		}
		if total >= start && len(found) < limit {
			found = append(found, *item)
		}
		total++
	}
	return found, total
}

// byKind tells whether keep keeps a title of a kind, asking it once for
// each kind: a listing keeps titles by their kind (see itemTypeFilter),
// and an order may hold 50,000 of them.
func byKind(keep func(library.Item) bool) func(library.Kind) bool {
	kinds := map[library.Kind]bool{}
	return func(kind library.Kind) bool {
		keeps, known := kinds[kind]
		if !known {
			keeps = keep(library.Item{Kind: kind})
			kinds[kind] = keeps
		}
		return keeps
	}
}

// sortedBy is the order of the titles of the order by name o that sort
// asks for, ascending (see titleSort): the order by name itself, for a
// sort by name.
func (o nameOrder) sortedBy(sort titleSort) nameOrder {
	items := o.items
	var compare func(a, b int32) int
	switch sort.key {
	case "DateCreated":
		// Titles added together keep their catalog's order, which lists a
		// stored library's titles added last first.
		tie := func(a, b int32) int { return cmp.Compare(o.ranks[b], o.ranks[a]) }
		if o.added {
			tie = func(a, b int32) int { return cmp.Compare(o.ranks[a], o.ranks[b]) }
		}
		compare = func(a, b int32) int {
			return cmp.Or(compareDates(dateAdded(&items[a]), dateAdded(&items[b])), tie(a, b))
		}
	case "PremiereDate":
		compare = func(a, b int32) int { return compareDates(items[a].PremiereDate, items[b].PremiereDate) }
	case "ProductionYear":
		compare = func(a, b int32) int { return cmp.Compare(productionYear(&items[a]), productionYear(&items[b])) }
	case "CommunityRating":
		compare = func(a, b int32) int { return cmp.Compare(items[a].CommunityRating, items[b].CommunityRating) }
	case "Runtime":
		compare = func(a, b int32) int { return cmp.Compare(items[a].Runtime, items[b].Runtime) }
	default:
		return o
	}
	places := make([]int32, len(items))
	for i := range places {
		places[i] = int32(i)
	}
	if sort.namesAgainst {
		slices.Reverse(places)
	}
	slices.SortStableFunc(places, compare)
	sorted := o
	sorted.places = places
	return sorted
}

// compareDates orders two dates, one missing first.
func compareDates(a, b *time.Time) int {
	if a == nil || b == nil {
		return compareBool(a != nil, b != nil)
	}
	return a.Compare(*b)
}

// dateAdded is the date a title tells as added (DateCreated), and sorts
// by: when it was added (see library.Item.Added), else, as a title is
// added once out, a catalog title's premiere date; nil when neither is
// known.
func dateAdded(item *library.Item) *time.Time {
	if item.Added != nil || !catalogTitle(item.Kind) {
		return item.Added
	}
	return item.PremiereDate
}

// catalogTitle reports whether items of kind are titles of a catalog, or
// their seasons and episodes, which tell no date added they do not know.
func catalogTitle(kind library.Kind) bool {
	switch kind {
	case library.KindMovie, library.KindSeries, library.KindSeason, library.KindEpisode, library.KindVideo, library.KindFolder:
		return true
	}
	return false
}

// productionYear is the year a title came out: its premiere date's when
// it gives none, 0 when neither is known.
func productionYear(item *library.Item) int {
	if item.ProductionYear == 0 && item.PremiereDate != nil {
		return item.PremiereDate.Year()
	}
	return item.ProductionYear
}

// nameOrderTitleSize is what a title of an order by name takes in memory,
// with its key: an order of 50,000 IPTV movies, names of 30 characters
// without posters, held 64.3 MB, 1,286 bytes a title, and posters and
// longer names add some. The cache of orders bounds their total by it.
// An order by another key counts as much: it shares the titles of the
// order by name, and keeps them once that order is gone.
const nameOrderTitleSize = 1536

// nameOrdersBytes bounds the orders kept, as nameOrderTitleSize counts
// them.
const nameOrdersBytes = 128 << 20

// orderIdle is how long an order is kept unread: each listing that reads
// it keeps it as long again, so that a library browsed in a session, or
// found sorted ahead, stays sorted. Its key changes with what versions its
// titles (see sortedOrder).
const orderIdle = 30 * time.Minute

// orderFresh is how old the titles of an order a listing reads may be
// before the order is sorted again in the background: what no version
// tells, such as a name read since in the user's language or a rating,
// shows that late at most, and the listing does not wait.
const orderFresh = 2 * time.Minute

// nameSortsAtOnce bounds the orders by name sorted at once on the server.
const nameSortsAtOnce = 2

// sortAheadQuiet is how long the apps' requests leave the server free
// before the libraries are sorted ahead: the apps that come back after a
// start load their first screens meanwhile, which the sorts would slow.
const sortAheadQuiet = time.Second

// sortAheadWait bounds how long the sorts ahead wait for the apps'
// requests to leave the server free: one never free that long sorts its
// libraries all the same.
const sortAheadWait = time.Minute

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

// storedSortKeys are the sorts of jellyfin-web's Sort menu a listing of a
// stored library is answered in (see storedListing), as sortBy names them.
var storedSortKeys = []string{"SortName", "DateCreated", "PremiereDate", "ProductionYear", "CommunityRating", "Runtime"}

// titleSort is the order a listing of a stored library asks for: by key,
// one of storedSortKeys, ascending or descending, then by name, the other
// way when namesAgainst is set.
type titleSort struct {
	key          string
	descending   bool
	namesAgainst bool
}

// name names the order sort keeps, whichever way it is read.
func (sort titleSort) name() string {
	if sort.key == "SortName" {
		return "name"
	}
	if sort.namesAgainst {
		return strings.ToLower(sort.key) + ",names against"
	}
	return strings.ToLower(sort.key)
}

// storedSortOf returns the order a listing of a folder's titles through
// its folders (see titlesThrough) asks for when its first sortBy key is
// one of storedSortKeys. The keys after it change nothing but the order of
// the titles of one value: by name, the way the listing asks the name, or
// the first key's when it asks none. A sort by name has no titles of one
// value, and one by date added keeps their catalog's order.
func storedSortOf(r *http.Request) (titleSort, bool) {
	keys, orders := listQuery(r, "sortBy"), listQuery(r, "sortOrder")
	if !titlesThrough(r) || len(keys) == 0 {
		return titleSort{}, false
	}
	i := slices.IndexFunc(storedSortKeys, func(key string) bool { return strings.EqualFold(key, keys[0]) })
	if i < 0 {
		return titleSort{}, false
	}
	sort := titleSort{key: storedSortKeys[i], descending: descendingAt(orders, 0)}
	if sort.key == "SortName" || sort.key == "DateCreated" {
		return sort, true
	}
	if n := slices.IndexFunc(keys[1:], func(key string) bool {
		return strings.EqualFold(key, "SortName") || strings.EqualFold(key, "Name")
	}); n >= 0 {
		sort.namesAgainst = descendingAt(orders, n+1) != sort.descending
	}
	return sort, true
}

// storedListing answers a listing of the titles of parent through its
// folders, sorted as sort asks, when Polyfin stores parent's titles, and
// reports whether it did: another parent, an addon's catalog among them,
// is not answered here. The listing pages through the library's order
// (see storedOrder): its total counts the titles keep keeps of it, every
// one of the library's up to library.StoredNamePool. A listing that asks
// for no limit lists library.WholeListing titles, as Jellyfin lists every
// one.
func (h *Handler) storedListing(w http.ResponseWriter, r *http.Request, user accounts.User, parent accounts.ID, genre string,
	rating library.RatingFilter, sort titleSort, fields fieldSet, keep func(library.Item) bool, start, limit int) bool {
	stored, err := h.Library.Stores(r.Context(), user, parent)
	if err != nil {
		h.browseError(w, r, err)
		return true
	}
	if !stored {
		return false
	}
	order, err := h.storedOrder(r.Context(), user, parent, genre, rating, sort)
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
	if wholeListing(r) {
		limit = library.WholeListing
	}
	start = max(start, 0)
	found, total := order.page(keep, start, limit, sort.descending)
	items, err := h.dtos(r, user, found, fields, keep)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: total, StartIndex: start})
	return true
}

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
	order, err := h.storedOrder(r.Context(), user, parent, genre, library.RatingFilter{}, titleSort{key: "SortName"})
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
		return user, order, true
	}
	return accounts.User{}, nameOrder{}, false
}

// storedOrder returns the order sort asks for of every title of library
// parent that user may see, narrowed to genre and by rating, ascending:
// the one kept, else one sorted for the listings that ask for it
// meanwhile (see nameBuilds). An order is kept while what the stored
// libraries list stays as it was (see library.Service.StoredChanges): a
// scan, a link, a refresh of an IPTV source or an edit sorts again.
func (h *Handler) storedOrder(ctx context.Context, user accounts.User, parent accounts.ID, genre string, rating library.RatingFilter,
	sort titleSort) (nameOrder, error) {
	return h.sortedOrder(ctx, user, parent, genre, rating, sort, h.storedVersion(), h.sortByName)
}

// storedVersion names the titles of the stored libraries as they are.
// The count is read before the titles: a change stored while they are
// sorted sorts them again at the next listing.
func (h *Handler) storedVersion() string {
	return "stored " + strconv.FormatUint(h.Library.StoredChanges(), 10) + "|"
}

// orderBuild sorts the titles of a library by name for an order kept
// under key, and tells whether it kept it (see sortByName).
type orderBuild func(ctx context.Context, user accounts.User, parent accounts.ID, genre string, hidden library.HiddenTitles,
	key drawKey) (nameOrder, bool, error)

// sortedOrder returns the order sort asks for of the titles of library
// parent that build sorts by name, as storedOrder tells. version names
// those titles as they are, so that the orders of other titles, or of the
// same titles since changed, are kept apart. An order serves every user
// who sees the library alike (see library.Service.ListingPrint) and hid
// the same titles. An order read more than orderFresh after its titles is
// answered as it is and sorted again in the background.
func (h *Handler) sortedOrder(ctx context.Context, user accounts.User, parent accounts.ID, genre string, rating library.RatingFilter,
	sort titleSort, version string, build orderBuild) (nameOrder, error) {
	alike, err := h.Library.ListingPrint(ctx, user)
	if err != nil {
		return nameOrder{}, err
	}
	hidden, err := h.Library.HiddenTitles(ctx, user)
	if err != nil {
		return nameOrder{}, err
	}
	narrowed, err := json.Marshal(rating)
	if err != nil {
		return nameOrder{}, err
	}
	// One order holds every kind; hiding a title sorts again, so that
	// positions follow at once.
	byName := drawKey{limits: alike, parent: parent, genre: genre,
		order: version + "name|" + hiddenFingerprint(hidden) + "|" + string(narrowed)}
	key := byName
	key.order = version + sort.name() + "|" + hiddenFingerprint(hidden) + "|" + string(narrowed)
	// sortKey sorts the order of key from the order by name kept, which
	// it sorts again when missing, or when refreshing and stale.
	sortKey := func(refresh bool) func(context.Context) (nameOrder, error) {
		return func(ctx context.Context) (nameOrder, error) {
			order, kept := h.nameOrders.Get(byName)
			if !kept || refresh && h.staleOrder(order) {
				var err error
				if order, kept, err = build(ctx, user, parent, genre, hidden, byName); err != nil {
					return nameOrder{}, err
				}
			}
			sorted := order.sortedBy(sort)
			if kept && key != byName {
				h.nameOrders.Put(key, sorted)
			}
			return sorted, nil
		}
	}
	ctx = library.WithRatingFilter(ctx, rating)
	if order, ok := h.nameOrders.Get(key); ok {
		if h.staleOrder(order) {
			go func() { _, _ = h.nameBuilds.wait(context.WithoutCancel(ctx), key, sortKey(true)) }()
		}
		return order, nil
	}
	return h.nameBuilds.wait(ctx, key, sortKey(false))
}

// staleOrder reports whether the titles of order were read more than
// orderFresh ago.
func (h *Handler) staleOrder(order nameOrder) bool {
	return h.now().Sub(order.sorted) > orderFresh
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
// titles through its folders reads them, with the lengths a local
// folder's files play (see library.Service.WithLengths), and keeps the
// order under key once it holds every title it sorts: those of the whole
// library, or as many as library.NamePool reads at most. kept tells
// whether it did.
func (h *Handler) sortByName(ctx context.Context, user accounts.User, parent accounts.ID, genre string, hidden library.HiddenTitles,
	key drawKey) (_ nameOrder, kept bool, _ error) {
	every := func(library.Kind) bool { return true }
	listing := library.WithTitlesThrough(library.WithKinds(library.WithOpening(ctx), every))
	read := h.now()
	page, bound, stored, isLibrary, err := h.Library.NamePool(listing, user, parent, genre)
	switch {
	case err != nil:
		return nameOrder{}, false, err
	case !isLibrary:
		return nameOrder{}, false, errNotLibrary
	case !stored:
		return nameOrder{}, false, errNotStored
	}
	order := sortedByName(hidden.Without(page.Items), !page.More)
	order.sorted = read
	if err := h.Library.WithLengths(ctx, user, parent, order.items); err != nil {
		return nameOrder{}, false, err
	}
	if !page.More || len(page.Items) >= bound {
		h.nameOrders.Put(key, order)
		return order, true, nil
	}
	return order, false, nil
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

// firstPagesRead sorts the libraries ahead the first time Polyfin read
// their first pages since it started (see sortAhead), then warms the
// posters of those pages (see warmPosters). Every time, it drops the
// orders left unread for orderIdle, which an idle server would keep.
func (h *Handler) firstPagesRead(ctx context.Context) {
	h.nameOrders.DropExpired()
	if h.sortedAhead.CompareAndSwap(false, true) {
		h.sortAhead(ctx)
	}
	h.warmPosters(ctx)
}

// sortAhead sorts by name the titles of the server's libraries each user
// sees whose sorted listings sort them (see
// library.Service.SortedLibraries), as their first listing would, and
// keeps their orders as listings do: those of the users who see a library
// alike are sorted once. It sorts one library at a time, leaving the other
// places of nameSortsAtOnce to listings, reads only the catalog pages
// kept, asking no addon for one, and stops once its orders hold half of
// what the orders keep at most. Each user's libraries, and each library,
// wait for the apps' requests to leave the server free for
// sortAheadQuiet, sortAheadWait at most in all (see waitFree).
func (h *Handler) sortAhead(ctx context.Context) {
	deadline := time.Now().Add(sortAheadWait)
	if !h.waitFree(ctx, deadline) {
		return
	}
	started := time.Now()
	users, err := h.Accounts.Users(ctx)
	if err != nil {
		h.Logger.Debug("The libraries could not be sorted ahead", "error", err)
		return
	}
	most := cache.Budget(nameOrdersBytes) / nameOrderTitleSize / 2
	titles := 0
	for _, user := range users {
		if user.Refused(started) {
			continue
		}
		// The user's view is built once for all their libraries.
		ctx := library.PerRequest(ctx)
		libraries, err := h.Library.SortedLibraries(ctx, user)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			h.Logger.Debug("The libraries to sort ahead could not be listed", "user", user.Name, "error", err)
			continue
		}
		for _, parent := range libraries {
			if titles >= most {
				h.Logger.Debug("Libraries sorted ahead up to their bound", "titles", titles, "took", time.Since(started).Round(time.Millisecond))
				return
			}
			if !h.waitFree(ctx, deadline) {
				return
			}
			order, err := h.sortLibraryAhead(ctx, user, parent)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				h.Logger.Debug("A library could not be sorted ahead", "user", user.Name, "library", parent, "error", err)
				continue
			}
			titles += len(order.items)
		}
	}
	h.Logger.Debug("Libraries sorted ahead", "titles", titles, "took", time.Since(started).Round(time.Millisecond))
}

// appRequests counts the apps' requests under way, and tells when the last
// one ended, or when the handler was made.
type appRequests struct {
	mu      sync.Mutex
	running int
	ended   time.Time
}

func (a *appRequests) begin() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.running++
}

func (a *appRequests) end() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.running--
	a.ended = time.Now()
}

// freeIn returns how long the apps' requests are yet to leave the server
// free for quiet: none once they did, quiet while one is under way.
func (a *appRequests) freeIn(quiet time.Duration) time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running > 0 {
		return quiet
	}
	return quiet - time.Since(a.ended)
}

// waitFree waits until the apps' requests left the server free for
// sortAheadQuiet, or until deadline. It reports false once ctx ended.
func (h *Handler) waitFree(ctx context.Context, deadline time.Time) bool {
	for {
		wait := min(h.requests.freeIn(sortAheadQuiet), time.Until(deadline))
		if wait <= 0 {
			return ctx.Err() == nil
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
}

// sortLibraryAhead returns the order by name of the titles of library
// parent that user sees, the one kept or one sorted as a listing of them
// sorts it, from the catalog pages kept for an addon's library.
func (h *Handler) sortLibraryAhead(ctx context.Context, user accounts.User, parent accounts.ID) (nameOrder, error) {
	byName := titleSort{key: "SortName"}
	stored, err := h.Library.Stores(ctx, user, parent)
	if err != nil {
		return nameOrder{}, err
	}
	if stored {
		return h.storedOrder(ctx, user, parent, "", library.RatingFilter{}, byName)
	}
	version, ok, err := h.Library.CatalogVersion(ctx, user, parent, "")
	if err != nil || !ok {
		return nameOrder{}, err
	}
	return h.sortedOrder(library.WithKeptOnly(ctx), user, parent, "", library.RatingFilter{}, byName, catalogVersion(version),
		h.sortCatalog(version, 0))
}

func descendingOrder(r *http.Request) bool {
	orders := listQuery(r, "sortOrder")
	return len(orders) > 0 && strings.EqualFold(orders[0], "Descending")
}

// at is the index in items of the position-th title of an order in the
// order asked: descending reads it from its end.
func (o nameOrder) at(position int, descending bool) int {
	if descending {
		position = len(o.items) - 1 - position
	}
	if o.places != nil {
		return int(o.places[position])
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
	keep := itemTypeFilter(r)
	found, total := order.page(keep, start, limit, descendingOrder(r))
	items, err := h.dtos(r, user, found, requestedFields(r), keep)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: total, StartIndex: start})
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
	keys := order.keptKeys(itemTypeFilter(r))
	if descendingOrder(r) {
		slices.Reverse(keys)
	}
	letters, runs := library.Letters(keys)
	writeJSON(w, http.StatusOK, nameLetters{Total: len(keys), Whole: order.whole, Letters: letters, Runs: runs})
}
