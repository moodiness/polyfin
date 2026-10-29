package jellyfin

import (
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// drawLife is how long a draw of a library's titles at random is kept, so
// that an app paging through one listing at random sees each title once
// (see randomLibraryListing). A listing after that draws anew.
const drawLife = 2 * time.Minute

// drawKey names a draw: the user, with what limits what they see, the
// library, the genre it is narrowed to and the kinds of items the listing
// keeps.
type drawKey struct {
	user   accounts.ID
	limits string
	parent accounts.ID
	genre  string
	kinds  string
}

// draw is the order a listing at random drew a library's titles in, as
// the library listed them, so that its pages need no addon to describe
// them again, and how many it asked for: a new listing, from its start,
// that asks for more draws again; the next pages of a listing keep its
// draw, past whose end they are empty.
type draw struct {
	items []library.Item
	asked int
}

// randomOrder reports whether a listing asks a folder for its titles at
// random: through its folders (see titlesThrough), sorted by Random first,
// as the LumaaGlaass theme's home carousel and jellyfin-web's shuffle ask.
// The keys after Random change nothing.
func randomOrder(r *http.Request) bool {
	sorts := listQuery(r, "sortBy")
	return titlesThrough(r) && len(sorts) > 0 && strings.EqualFold(sorts[0], "Random")
}

// newDrawKey is the key of the draw a listing of r by user reads.
func newDrawKey(r *http.Request, user accounts.User, parent accounts.ID, genre string) drawKey {
	limits, _ := json.Marshal(struct {
		Parental        accounts.ParentalControl
		BlockedGenres   []string
		HiddenLibraries []accounts.ID
		UseSharedAddons bool
	}{user.Parental, user.BlockedGenres, user.HiddenLibraries, user.UseSharedAddons})
	kinds := make([]string, 0)
	for _, kind := range listQuery(r, "includeItemTypes") {
		kinds = append(kinds, strings.ToLower(kind))
	}
	slices.Sort(kinds)
	return drawKey{user: user.ID, limits: string(limits), parent: parent, genre: genre, kinds: strings.Join(kinds, ",")}
}

// randomLibraryListing answers a listing of the titles of one of the
// user's libraries at random (see randomOrder), and reports whether it
// did. Jellyfin sorts the whole library at random; a remote catalog is
// read a page at a time, so Polyfin draws among the library's first
// titles, library.RandomPool of them or as many as the listing reaches,
// read as a whole listing reads them (see library.Service.DrawPool), with
// what the user may see only. The draw is kept for drawLife, so that the
// pages of one listing follow one order; the listing's total is the
// number of titles drawn among. While those titles are still being read,
// a listing draws among those read, and keeps no draw. Other parents, a
// smart list among them, are not answered here.
func (h *Handler) randomLibraryListing(w http.ResponseWriter, r *http.Request, user accounts.User, parent accounts.ID, genre string,
	fields fieldSet, keep func(library.Item) bool, start, limit int) bool {
	start = max(start, 0)
	// A listing that asks for no limit gets the whole draw.
	end, reach := start+limit, start+limit
	if wholeListing(r) {
		end, reach = library.WholeListing, library.WholeListing
	}
	key := newDrawKey(r, user, parent, genre)
	drawn, ok := h.draws.Get(key)
	if !ok || start == 0 && reach > drawn.asked {
		page, isLibrary, err := h.Library.DrawPool(listingContext(r, keep), user, parent, limit, reach, genre)
		if err != nil {
			h.browseError(w, r, err)
			return true
		}
		if !isLibrary {
			return false
		}
		drawn = draw{items: slices.DeleteFunc(slices.Clone(page.Items), func(item library.Item) bool { return !keep(item) })}
		rand.Shuffle(len(drawn.items), func(i, j int) { drawn.items[i], drawn.items[j] = drawn.items[j], drawn.items[i] })
		// A draw among every title of the library answers any listing; one
		// among its first ones, those it asked for. A pool still being read
		// would only be drawn among in part, and is not kept.
		switch pool := min(max(reach, library.RandomPool), library.WholeListing); {
		case !page.More:
			drawn.asked = library.WholeListing
			h.draws.Put(key, drawn)
		case len(page.Items) >= pool:
			drawn.asked = pool
			h.draws.Put(key, drawn)
		}
	}
	found := drawn.items[min(start, len(drawn.items)):min(end, len(drawn.items))]
	// Titles hidden since the draw leave it at once.
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
	writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: len(drawn.items), StartIndex: start})
	return true
}
