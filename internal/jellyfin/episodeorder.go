package jellyfin

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// Series by their episodes.
//
// jellyfin-web's series libraries sort by "Date Episode Added"
// (DateLastContentAdded) and "Date Played" (SeriesDatePlayed), and other
// apps ask for DatePlayed. Jellyfin orders series by when their latest
// episode was added to the library, and when the user last played one of
// their episodes they played through. Addons' episodes are not added: they
// air, so Polyfin orders series by when their latest regular episode
// aired, as their descriptions tell, specials and episodes not aired yet
// left out. A library's titles are read a page at a time: as for a draw at
// random (see randomLibraryListing), the series are sorted among the
// library's first library.RandomPool, or as many as the listing reaches,
// and the order is kept for drawLife, so that the pages of one listing
// follow it. A series whose description is not kept yet has it read in
// the background, and comes as one without episodes meanwhile: an order
// waiting for descriptions is not kept.

// episodeOrderKeys are the sorts episodeOrderListing answers.
var episodeOrderKeys = []string{"DateLastContentAdded", "SeriesDatePlayed", "DatePlayed"}

// episodeOrder returns the sort a listing of series asks for first, when
// it is one of episodeOrderKeys: a listing through the folders (see
// titlesThrough) of series alone.
func episodeOrder(r *http.Request) (string, bool) {
	sorts, include := listQuery(r, "sortBy"), listQuery(r, "includeItemTypes")
	if !titlesThrough(r) || len(sorts) == 0 || slices.ContainsFunc(include, func(t string) bool { return !strings.EqualFold(t, "Series") }) {
		return "", false
	}
	i := slices.IndexFunc(episodeOrderKeys, func(key string) bool { return strings.EqualFold(key, sorts[0]) })
	if i < 0 {
		return "", false
	}
	return episodeOrderKeys[i], true
}

// episodeOrderListing answers a listing of the series of one of the
// user's libraries sorted by key (see episodeOrder), and reports whether
// it did: other parents, a smart list among them, are not answered here.
// The series are those a draw at random would draw among, with what the
// user may see only. As in Jellyfin, a series without a date comes first
// in ascending order and last in descending order; the keys after the
// first, then the name, order the series of one date. The listing's total
// is the number of series sorted.
func (h *Handler) episodeOrderListing(w http.ResponseWriter, r *http.Request, user accounts.User, parent accounts.ID, key, genre string,
	fields fieldSet, keep func(library.Item) bool, start, limit int) bool {
	start = max(start, 0)
	end, reach := start+limit, start+limit
	if wholeListing(r) {
		end, reach = library.WholeListing, library.WholeListing
	}
	drawKey := newDrawKey(r, user, parent, genre)
	drawKey.order = strings.ToLower(strings.Join(listQuery(r, "sortBy"), ",") + "|" + strings.Join(listQuery(r, "sortOrder"), ","))
	sorted, ok := h.draws.Get(drawKey)
	if !ok || start == 0 && reach > sorted.asked {
		page, isLibrary, err := h.Library.DrawPool(listingContext(r, keep), user, parent, limit, reach, genre)
		if err != nil {
			h.browseError(w, r, err)
			return true
		}
		if !isLibrary {
			return false
		}
		sorted = draw{items: slices.DeleteFunc(slices.Clone(page.Items), func(item library.Item) bool { return !keep(item) })}
		dates, settled, err := h.episodeDates(r, user, key, sorted.items)
		if err != nil {
			h.internalError(w, r, err)
			return true
		}
		sortByDates(r, sorted.items, dates)
		switch pool := min(max(reach, library.RandomPool), library.WholeListing); {
		case !settled:
		case !page.More:
			sorted.asked = library.WholeListing
			h.draws.Put(drawKey, sorted)
		case len(page.Items) >= pool:
			sorted.asked = pool
			h.draws.Put(drawKey, sorted)
		}
	}
	found := sorted.items[min(start, len(sorted.items)):min(end, len(sorted.items))]
	// Titles hidden since the order was kept leave it at once.
	hidden, err := h.Library.HiddenTitles(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	items, err := h.dtos(r, user, hidden.Without(found), fields, keep)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: len(sorted.items), StartIndex: start})
	return true
}

// episodeDates returns the date each series of items sorts by for key:
// when its latest episode aired, when the user last played one of its
// episodes they played through, or the series itself. settled is false
// while descriptions the dates need are being read.
func (h *Handler) episodeDates(r *http.Request, user accounts.User, key string, items []library.Item) (dates map[accounts.ID]time.Time,
	settled bool, err error) {
	ids := make([]accounts.ID, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	switch key {
	case "DateLastContentAdded":
		return h.Library.LatestEpisodes(r.Context(), user, items)
	case "SeriesDatePlayed":
		dates, err = h.UserData.SeriesLastPlayed(r.Context(), user.ID, ids)
		return dates, true, err
	}
	data, err := h.UserData.Get(r.Context(), user.ID, ids)
	if err != nil {
		return nil, false, err
	}
	dates = make(map[accounts.ID]time.Time, len(data))
	for id, d := range data {
		if d.LastPlayed != nil {
			dates[id] = *d.LastPlayed
		}
	}
	return dates, true, nil
}

// sortByDates orders items by their dates, as the request's first sort
// key and order ask, then by its other keys, then by name.
func sortByDates(r *http.Request, items []library.Item, dates map[accounts.ID]time.Time) {
	keys, orders := listQuery(r, "sortBy"), listQuery(r, "sortOrder")
	descending := func(i int) bool {
		if i < len(orders) {
			return strings.EqualFold(orders[i], "Descending")
		}
		return len(orders) == 1 && strings.EqualFold(orders[0], "Descending")
	}
	slices.SortStableFunc(items, func(a, b library.Item) int {
		result := dates[a.ID].Compare(dates[b.ID])
		if descending(0) {
			result = -result
		}
		for i := 1; result == 0 && i < len(keys); i++ {
			result = compareBy(keys[i], a, b, userState{})
			if descending(i) {
				result = -result
			}
		}
		if result == 0 {
			result = compareBy("SortName", a, b, userState{})
		}
		return result
	})
}
