package jellyfin

import (
	"cmp"
	"net/http"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// Tags, and the streaming services they carry.
//
// Jellyfin has no field for the streaming services that offer a title. A
// movie or series carries those TMDB names in the server's rating country
// as its tags, unless an administrator edited its tags (see
// library.Service.TagsShown). Jellyfin apps show a title's tags, and
// narrow listings by them:
//   - jellyfin-web links each tag of a title's page to a listing of the
//     titles of that tag, with no folder, asking for tags and
//     IncludeItemTypes=tag, a type Jellyfin ignores (see itemTypeFilter).
//     That listing is the service's page (see narrowedListing and
//     library.Service.ServiceTitles), as a studio's is;
//   - a library's filter menu offers the tags of its titles
//     (/Items/Filters and /Items/Filters2), and a listing of the library
//     asking for tags its catalog does not offer as a genre option keeps
//     those of its first titles that have one of them, as for genres (see
//     titleGenreListing).
//
// A tag is preferred over a studio-like item for a service: Jellyfin's
// studios are who made the title, which apps show as such, and narrowing
// by a studio would mix the two.

// tagsOnly reports whether the listing is narrowed to tags alone: neither
// genres, studios nor years.
func (f nameFilter) tagsOnly() bool {
	return len(f.tags) > 0 && len(f.genres) == 0 && len(f.genreIDs) == 0 && len(f.studios) == 0 && len(f.studioIDs) == 0 &&
		len(f.years) == 0
}

// titleTagListing lists the titles of library parent narrowed to the tags
// of f among its first titles, by the tags they show, as
// titleGenreListing does by their genres. It reports whether it answered.
func (h *Handler) titleTagListing(w http.ResponseWriter, r *http.Request, user accounts.User, parent accounts.ID, f nameFilter,
	fields fieldSet, keep func(library.Item) bool, start, limit int) bool {
	has := func(items []library.Item) []bool {
		kept := make([]bool, len(items))
		for i, tags := range h.Library.TagsShown(r.Context(), items) {
			kept[i] = slices.ContainsFunc(tags, func(tag string) bool {
				return slices.ContainsFunc(f.tags, func(name string) bool { return strings.EqualFold(strings.TrimSpace(tag), name) })
			})
		}
		return kept
	}
	return h.titleNameListing(w, r, user, parent, has, fields, keep, start, limit)
}

// titleTags lists the tags of the first titles of library parent that the
// request's item types keep, as titleTagListing narrows them: each once,
// in the spelling of the first title that has it, by name. A music
// library and a folder that is none of the user's libraries have none.
func (h *Handler) titleTags(r *http.Request, user accounts.User, parent accounts.ID) ([]string, error) {
	tags := []string{}
	if parent == (accounts.ID{}) {
		return tags, nil
	}
	if music, err := h.Library.MusicFolder(r.Context(), user, parent); err != nil || music {
		return tags, err
	}
	keep := itemTypeFilter(r)
	pool, _, err := h.Library.DrawPool(listingContext(r, keep), user, parent, library.RandomPool, library.RandomPool, "")
	if err != nil {
		return nil, err
	}
	items := slices.DeleteFunc(slices.Clone(pool.Items), func(item library.Item) bool { return !keep(item) })
	seen := map[string]bool{}
	for _, shown := range h.Library.TagsShown(r.Context(), items) {
		for _, tag := range shown {
			tag = strings.TrimSpace(tag)
			if key := strings.ToLower(tag); tag != "" && !seen[key] {
				seen[key] = true
				tags = append(tags, tag)
			}
		}
	}
	slices.SortFunc(tags, func(a, b string) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a), strings.ToLower(b)), cmp.Compare(a, b))
	})
	return tags, nil
}
