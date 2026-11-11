package jellyfin

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// Genres and studios opened by their identifier.
//
// A title gives its genres and studios with the identifier of their name
// (see nameID), and jellyfin-web opens one by asking for that identifier
// as an item (/Users/{userId}/Items/{id}) before it lists its titles: a
// link of a title's page, or a card of /Genres. An identifier is a hash
// of the name, which cannot be read back, so Polyfin remembers the names it
// gave an identifier to, the namesKept most recent, in memory. A genre is
// also found among the genre options of the catalogs the user reaches,
// which GenreIds matches too (see nameFilter), so that a genre's page
// opened again after a restart still opens. An identifier known neither
// way is no item.

// namesKept bounds the names remembered by their identifier.
const namesKept = 10_000

// namedKind is a kind of item that stands for a name: its Jellyfin type,
// and the kind its identifier is made of (see nameID).
type namedKind struct {
	itemType, kind string
}

var (
	genreNamed = namedKind{"Genre", "genre"}
	// A genre of music is an item of its own, as in Jellyfin: apps list
	// its albums rather than titles.
	musicGenreNamed = namedKind{"MusicGenre", "musicgenre"}
	studioNamed     = namedKind{"Studio", "studio"}
)

// page is the page of the item of kind named name. A genre's or studio's
// titles are those of the catalog options of its name; a genre of music
// lists no titles.
func (k namedKind) page(name string) namedPage {
	page := namedPage{itemType: k.itemType, name: name, id: nameID(k.kind, name), sortName: strings.ToLower(name)}
	if k != musicGenreNamed {
		page.matches = func(option string) bool { return strings.EqualFold(strings.TrimSpace(option), strings.TrimSpace(name)) }
	}
	return page
}

// genresOf is the kind of the genres of items of kind: genres of music
// for artists, albums and songs, as in Jellyfin.
func genresOf(kind library.Kind) namedKind {
	switch kind {
	case library.KindArtist, library.KindAlbum, library.KindTrack:
		return musicGenreNamed
	}
	return genreNamed
}

// namedItem is a name an identifier was given to, and its kind.
type namedItem struct {
	kind namedKind
	name string
}

// namedItems remembers the names identifiers were given to, the namesKept
// most recent; its zero value remembers none yet.
type namedItems struct {
	mu   sync.RWMutex
	byID map[string]namedItem
	// ring holds the identifiers remembered, next being the place of the
	// oldest once it is full.
	ring []string
	next int
}

// id returns the identifier of the item of kind named name, and
// remembers its name. The first name given an identifier is kept: names
// differing in letter case only have the same one.
func (n *namedItems) id(kind namedKind, name string) string {
	id := nameID(kind.kind, name)
	n.mu.RLock()
	_, known := n.byID[id]
	n.mu.RUnlock()
	if known {
		return id
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, known := n.byID[id]; known {
		return id
	}
	if n.byID == nil {
		n.byID = map[string]namedItem{}
	}
	if len(n.ring) < namesKept {
		n.ring = append(n.ring, id)
	} else {
		delete(n.byID, n.ring[n.next])
		n.ring[n.next] = id
		n.next = (n.next + 1) % namesKept
	}
	n.byID[id] = namedItem{kind, name}
	return id
}

// named returns the item an identifier was given to.
func (n *namedItems) named(id string) (namedItem, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	item, ok := n.byID[id]
	return item, ok
}

// namedPairs returns the names of kind with their identifiers, which it
// remembers.
func (h *Handler) namedPairs(kind namedKind, names []string) []NameGuidPair {
	result := make([]NameGuidPair, 0, len(names))
	for _, name := range names {
		result = append(result, NameGuidPair{Name: name, Id: h.names.id(kind, name)})
	}
	return result
}

// describeNamed answers the item a genre or studio identifier stands for,
// as its page does (see writeNamedItem). It reports whether id is one.
func (h *Handler) describeNamed(w http.ResponseWriter, r *http.Request, user accounts.User, id accounts.ID) bool {
	item, ok, err := h.namedByID(r.Context(), user, id.String())
	if err != nil {
		h.browseError(w, r, err)
		return true
	}
	if !ok {
		return false
	}
	h.writeNamedItem(w, r, user, item.kind.page(item.name))
	return true
}

// namedByID returns the item an identifier stands for: a name remembered,
// else a genre option of the user's catalogs.
func (h *Handler) namedByID(ctx context.Context, user accounts.User, id string) (namedItem, bool, error) {
	if item, ok := h.names.named(id); ok {
		return item, true, nil
	}
	option, ok, err := h.Library.GenreOption(ctx, user, func(option string) bool { return nameID(genreNamed.kind, option) == id })
	if err != nil || !ok {
		return namedItem{}, false, err
	}
	return namedItem{genreNamed, option}, true, nil
}
