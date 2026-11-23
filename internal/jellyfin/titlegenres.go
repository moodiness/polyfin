package jellyfin

import (
	"cmp"
	"net/http"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// Genres of a library's own titles.
//
// A library is narrowed to a genre through its catalog's genre filter,
// which reads the whole catalog narrowed by its addon (see genreFilter).
// Many catalogs offer none: lists a user keeps, or catalogs of an addon
// that only pages through them. jellyfin-web 12.4 then had no genre to
// offer, in a library's Genres tab or its filter menu, and a listing
// asking for one, as a title's genre link does, was empty, whereas
// Jellyfin narrows any library by its titles' genres. Such a library's
// genres are those of its first titles, and a listing asking for genres
// its catalog does not offer keeps those of its first titles that have
// one of them: as for titles at random (see randomLibraryListing), the
// library's first library.RandomPool titles, or as many as the listing
// asks for from its first title, up to library.WholeListing, since a
// remote catalog is read a page at a time.

// genresOnly reports whether the listing is narrowed to genres alone, by
// name or by identifier: neither studios, tags nor years, which no
// title's genres stand for.
func (f nameFilter) genresOnly() bool {
	return (len(f.genres) > 0 || len(f.genreIDs) > 0) && len(f.studios) == 0 && len(f.studioIDs) == 0 && len(f.tags) == 0 &&
		len(f.years) == 0
}

// hasGenre reports whether a title has one of the genres the listing asks
// for, by name in any letter case or by identifier (see nameID): titles
// of any of them are listed, as in Jellyfin.
func (f nameFilter) hasGenre(item library.Item) bool {
	return slices.ContainsFunc(item.Genres, func(genre string) bool {
		genre = strings.TrimSpace(genre)
		return slices.ContainsFunc(f.genres, func(name string) bool { return strings.EqualFold(name, genre) }) ||
			slices.Contains(f.genreIDs, nameID(genreNamed.kind, genre))
	})
}

// titleGenreListing lists the titles of library parent narrowed to the
// genres of f among its first titles, by their own genres (see the
// header): in their catalog's order, or sorted as the listing asks, at
// random for Random. TotalRecordCount is the number of titles kept. It
// reports whether it answered: parent is none of the user's libraries
// otherwise.
func (h *Handler) titleGenreListing(w http.ResponseWriter, r *http.Request, user accounts.User, parent accounts.ID, f nameFilter,
	fields fieldSet, keep func(library.Item) bool, start, limit int) bool {
	has := func(items []library.Item) []bool {
		kept := make([]bool, len(items))
		for i, item := range items {
			kept[i] = f.hasGenre(item)
		}
		return kept
	}
	return h.titleNameListing(w, r, user, parent, has, fields, keep, start, limit)
}

// titleNameListing lists the titles of library parent among its first
// titles that has keeps, has telling for each of them, as
// titleGenreListing does.
func (h *Handler) titleNameListing(w http.ResponseWriter, r *http.Request, user accounts.User, parent accounts.ID,
	has func(items []library.Item) []bool, fields fieldSet, keep func(library.Item) bool, start, limit int) bool {
	start = max(start, 0)
	// A listing that asks for no limit gets every title kept.
	end, reach := start+limit, start+limit
	if wholeListing(r) {
		end, reach = library.WholeListing, library.WholeListing
	}
	pool, isLibrary, err := h.Library.DrawPool(listingContext(r, keep), user, parent, limit, reach, "")
	if err != nil {
		h.browseError(w, r, err)
		return true
	}
	if !isLibrary {
		return false
	}
	candidates := slices.DeleteFunc(slices.Clone(pool.Items), func(item library.Item) bool { return !keep(item) })
	var kept []library.Item
	for i, named := range has(candidates) {
		if named {
			kept = append(kept, candidates[i])
		}
	}
	if len(listQuery(r, "sortBy")) > 0 {
		state, err := h.userState(r.Context(), user, kept)
		if err != nil {
			h.internalError(w, r, err)
			return true
		}
		sortItems(r, kept, state)
	}
	items, err := h.dtos(r, user, kept[min(start, len(kept)):min(end, len(kept))], fields, keep)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	if pool.More {
		// For Silo's apps: the library holds more titles than were looked
		// at (see partialTotalHeader).
		w.Header().Set(partialTotalHeader, "true")
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: len(kept), StartIndex: start})
	return true
}

// titleGenres lists the genres of the first titles of library parent
// that the request's item types keep, as titleGenreListing narrows them:
// each once, in the spelling of the first title that has it, by name.
// A music library, whose genres are genres of music, and a folder that
// is none of the user's libraries have none.
func (h *Handler) titleGenres(r *http.Request, user accounts.User, parent accounts.ID) ([]string, error) {
	if music, err := h.Library.MusicFolder(r.Context(), user, parent); err != nil || music {
		return nil, err
	}
	keep := itemTypeFilter(r)
	pool, _, err := h.Library.DrawPool(listingContext(r, keep), user, parent, library.RandomPool, library.RandomPool, "")
	if err != nil {
		return nil, err
	}
	var genres []string
	seen := map[string]bool{}
	for _, item := range pool.Items {
		if !keep(item) || genresOf(item.Kind) != genreNamed {
			continue
		}
		for _, genre := range item.Genres {
			genre = strings.TrimSpace(genre)
			if key := strings.ToLower(genre); genre != "" && !seen[key] {
				seen[key] = true
				genres = append(genres, genre)
			}
		}
	}
	slices.SortFunc(genres, func(a, b string) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a), strings.ToLower(b)), cmp.Compare(a, b))
	})
	return genres, nil
}
