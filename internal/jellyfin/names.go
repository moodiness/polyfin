package jellyfin

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// Genres and studios opened by their identifier.
//
// A title gives its genres and studios with the identifier of their name
// (see nameID), and jellyfin-web opens one by asking for that identifier
// as an item (/Users/{userId}/Items/{id}) before it lists its titles: a
// link of a title's page, or a card of /Genres or /Studios. An identifier
// is a hash of the name, which cannot be read back, so Polyfin remembers
// the names it gave an identifier to, the namesKept most recent, in
// memory. A genre is also found among the genre options of the catalogs
// the user reaches, which GenreIds matches too (see nameFilter), then
// among the genres of the titles Polyfin keeps that the user reaches (see
// library.Service.GenreNames), and a studio among those TMDB's details
// named (see library.Service.StudioNames), so that a page opened again
// after a restart still opens. An identifier known none of these ways is
// no item.

const (
	// namesKept bounds the names remembered by their identifier.
	namesKept = 10_000
	// keptNamesFresh is how long the genres of the titles kept and the
	// studios TMDB's details named are used before they are read again,
	// when an identifier is none of them; a user's genres not looked up
	// for as long are forgotten.
	keptNamesFresh = time.Minute
)

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
// titles are those of the catalog options of its name, then more (see
// library.Service.GenreTitles and StudioTitles); a genre of music lists
// no titles.
func (k namedKind) page(name string) namedPage {
	page := namedPage{itemType: k.itemType, name: name, id: nameID(k.kind, name), sortName: strings.ToLower(name), genre: k == genreNamed,
		studio: k == studioNamed}
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

// namedItems remembers the names identifiers were given to, the
// namesKept most recent; its zero value remembers none yet. A genre is
// remembered by the user it was given to: another user's own addon's
// titles give no one else a genre's name, nor its spelling. A studio is
// remembered for everyone, as its logo is asked for without a user (see
// studioArtwork).
type namedItems struct {
	mu   sync.RWMutex
	byID map[readerName]namedItem
	// ring holds the identifiers remembered, next being the place of the
	// oldest once it is full.
	ring []readerName
	next int
	// genres are the names of the genres of the titles Polyfin keeps, as
	// each user reaches them, and studios those of the studios TMDB's
	// details named (see keptName).
	genres  readerNames
	studios keptNames
}

// readerName is an identifier as given to a user, the zero user for a
// name given to no one in particular: a studio's, or a genre of a
// programme of a timer.
type readerName struct {
	reader accounts.ID
	id     string
}

// id returns the identifier of the item of kind named name, and
// remembers its name as given to reader, or to everyone for a studio.
// The first name given an identifier is kept: names differing in letter
// case only have the same one.
func (n *namedItems) id(reader accounts.ID, kind namedKind, name string) string {
	if kind == studioNamed {
		reader = accounts.ID{}
	}
	key := readerName{reader, nameID(kind.kind, name)}
	n.mu.RLock()
	_, known := n.byID[key]
	n.mu.RUnlock()
	if known {
		return key.id
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, known := n.byID[key]; known {
		return key.id
	}
	if n.byID == nil {
		n.byID = map[readerName]namedItem{}
	}
	if len(n.ring) < namesKept {
		n.ring = append(n.ring, key)
	} else {
		delete(n.byID, n.ring[n.next])
		n.ring[n.next] = key
		n.next = (n.next + 1) % namesKept
	}
	n.byID[key] = namedItem{kind, name}
	return key.id
}

// named returns the item an identifier was given to, as given to reader,
// else to no one in particular.
func (n *namedItems) named(reader accounts.ID, id string) (namedItem, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	item, ok := n.byID[readerName{reader, id}]
	if !ok {
		item, ok = n.byID[readerName{id: id}]
	}
	return item, ok
}

// namedPairs returns the names of kind with their identifiers, which it
// remembers as given to reader.
func (h *Handler) namedPairs(reader accounts.ID, kind namedKind, names []string) []NameGuidPair {
	result := make([]NameGuidPair, 0, len(names))
	for _, name := range names {
		result = append(result, NameGuidPair{Name: name, Id: h.names.id(reader, kind, name)})
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

// namedByID returns the item an identifier stands for: a name remembered
// as given to the user, else a genre option of the user's catalogs, else
// a genre of the titles Polyfin keeps that the user reaches, else a
// studio TMDB's details named.
func (h *Handler) namedByID(ctx context.Context, user accounts.User, id string) (namedItem, bool, error) {
	if item, ok := h.names.named(user.ID, id); ok {
		return item, true, nil
	}
	option, ok, err := h.Library.GenreOption(ctx, user, func(option string) bool { return nameID(genreNamed.kind, option) == id })
	if err != nil {
		return namedItem{}, false, err
	}
	if ok {
		return namedItem{genreNamed, option}, true, nil
	}
	if name, ok := h.keptGenre(ctx, user, id); ok {
		return namedItem{genreNamed, name}, true, nil
	}
	if name, ok := h.keptStudio(ctx, id); ok {
		return namedItem{studioNamed, name}, true, nil
	}
	return namedItem{}, false, nil
}

// genreByID returns the name of the genre an identifier stands for: a
// name remembered as given to the user, else a genre of the titles
// Polyfin keeps that the user reaches.
func (h *Handler) genreByID(ctx context.Context, user accounts.User, id string) (string, bool) {
	if item, ok := h.names.named(user.ID, id); ok {
		return item.name, item.kind == genreNamed
	}
	return h.keptGenre(ctx, user, id)
}

// studioByID returns the name of the studio an identifier stands for: a
// name remembered, else one TMDB's details named.
func (h *Handler) studioByID(ctx context.Context, id string) (string, bool) {
	if item, ok := h.names.named(accounts.ID{}, id); ok {
		return item.name, item.kind == studioNamed
	}
	return h.keptStudio(ctx, id)
}

// keptGenre returns the genre whose identifier is id among those of the
// titles Polyfin keeps that the user reaches, which it remembers as given
// to the user.
func (h *Handler) keptGenre(ctx context.Context, user accounts.User, id string) (string, bool) {
	name, ok := keptName(ctx, h.names.genres.of(user.ID), genreNamed, id, func(ctx context.Context) ([]string, error) {
		return h.Library.GenreNames(ctx, user)
	})
	if ok {
		h.names.id(user.ID, genreNamed, name)
	}
	return name, ok
}

// keptStudio returns the studio whose identifier is id among those TMDB's
// details named, which it remembers as given to no one in particular.
func (h *Handler) keptStudio(ctx context.Context, id string) (string, bool) {
	name, ok := keptName(ctx, &h.names.studios, studioNamed, id, h.Library.StudioNames)
	if ok {
		h.names.id(accounts.ID{}, studioNamed, name)
	}
	return name, ok
}

// keptNames are names of a kind Polyfin keeps, by identifier, as read at
// read; the zero value has none yet. used is when a user's were last
// asked for (see readerNames).
type keptNames struct {
	mu   sync.Mutex
	byID map[string]string
	read time.Time
	used time.Time
}

// readerNames are keptNames by user; the zero value has none yet.
type readerNames struct {
	mu     sync.Mutex
	byUser map[accounts.ID]*keptNames
}

// of returns the names kept for user. Those of the users who looked none
// up for a keptNamesFresh are forgotten, so that it holds as many as
// look names up at once.
func (n *readerNames) of(user accounts.ID) *keptNames {
	n.mu.Lock()
	defer n.mu.Unlock()
	if kept, ok := n.byUser[user]; ok {
		kept.used = time.Now()
		return kept
	}
	if n.byUser == nil {
		n.byUser = map[accounts.ID]*keptNames{}
	}
	for other, kept := range n.byUser {
		if time.Since(kept.used) >= keptNamesFresh {
			delete(n.byUser, other)
		}
	}
	kept := &keptNames{used: time.Now()}
	n.byUser[user] = kept
	return kept
}

// keptName returns the name of kind whose identifier is id among the names
// load returns, the first of each identifier. The names are read again
// into kept when id is none of them, once a keptNamesFresh at most.
func keptName(ctx context.Context, kept *keptNames, kind namedKind, id string, load func(context.Context) ([]string, error)) (string, bool) {
	kept.mu.Lock()
	defer kept.mu.Unlock()
	name, ok := kept.byID[id]
	if !ok && time.Since(kept.read) >= keptNamesFresh {
		names, err := load(ctx)
		if err != nil {
			return "", false
		}
		kept.byID, kept.read = make(map[string]string, len(names)), time.Now()
		for _, named := range names {
			if key := nameID(kind.kind, named); kept.byID[key] == "" {
				kept.byID[key] = named
			}
		}
		name, ok = kept.byID[id]
	}
	return name, ok
}
