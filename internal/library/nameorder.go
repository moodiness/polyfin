package library

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Listings by name, and the letters an alphabetical bar files them under.
//
// Titles sort by their sort name, the one an administrator set or else
// their name, without regard to case or accents, as playlists sort (see
// foldName in package jellyfin): "Élan" sorts among the E. Articles count:
// "The Hobbit" sorts under T unless its sort name is "Hobbit, The".

// FoldName is what a title's sort name sorts by: lower case, without
// accents or the spaces around it.
func FoldName(name string) string {
	decomposed := norm.NFD.String(strings.ToLower(strings.TrimSpace(name)))
	return norm.NFC.String(strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, decomposed))
}

// NameKey is what item sorts by in a listing by name.
func NameKey(item Item) string {
	return FoldName(cmp.Or(strings.TrimSpace(item.SortName), item.Name))
}

// NameLetter is the letter a title whose sort name folds to key files
// under: A to Z, or # for one that starts with a digit, a symbol or a
// letter of another alphabet.
func NameLetter(key string) string {
	if key != "" && key[0] >= 'a' && key[0] <= 'z' {
		return string(rune(key[0] - 'a' + 'A'))
	}
	return "#"
}

// SortByName sorts items by name (see NameKey), descending when asked; of
// two of the same name, by their sort names as set, then by ID, so that
// the order is the same at every listing (see CompareNames).
func SortByName(items []Item, descending bool) {
	type keyed struct {
		key  string
		item Item
	}
	sorted := make([]keyed, len(items))
	for i, item := range items {
		sorted[i] = keyed{NameKey(item), item}
	}
	slices.SortFunc(sorted, func(a, b keyed) int {
		result := CompareNames(&a.item, &b.item, a.key, b.key)
		if descending {
			return -result
		}
		return result
	})
	for i := range sorted {
		items[i] = sorted[i].item
	}
}

// CompareNames orders a and b, whose names fold to keyA and keyB (see
// NameKey), as SortByName sorts them ascending. Their IDs compare as
// their bytes, which their hexadecimal strings follow.
func CompareNames(a, b *Item, keyA, keyB string) int {
	if c := strings.Compare(keyA, keyB); c != 0 {
		return c
	}
	if c := strings.Compare(a.SortName, b.SortName); c != 0 {
		return c
	}
	return bytes.Compare(a.ID[:], b.ID[:])
}

// Stores reports whether Polyfin stores the titles of parent, one of
// user's libraries, as NamePool reads them: false for another parent.
func (s *Service) Stores(ctx context.Context, user accounts.User, parent accounts.ID) (bool, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return false, err
	}
	l, ok := v.library(parent)
	return ok && l.stored(), nil
}

// SortedLibraries lists those of the server's libraries user sees whose
// titles a sorted listing sorts, in order: those Polyfin stores (see
// Stores) and those of an addon's catalog (see CatalogVersion). A user's
// own libraries serve that user alone.
func (s *Service) SortedLibraries(ctx context.Context, user accounts.User) ([]accounts.ID, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	var sorted []accounts.ID
	for _, l := range v.libraries {
		if _, _, catalog := catalogLibrary(v, l.item.ID, ""); l.addon.shared && (l.stored() || catalog) {
			sorted = append(sorted, l.item.ID)
		}
	}
	return sorted, nil
}

// ListingPrint names the way user sees the titles of their libraries:
// users of one print list the same titles of a library, in the same
// words, but for the titles each hid (see HiddenTitles) and a request's
// rating filter (see WithRatingFilter), which it leaves out. An order of a
// library's titles sorted for one of them serves them all.
func (s *Service) ListingPrint(ctx context.Context, user accounts.User) (string, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return "", err
	}
	return v.listingPrint(), nil
}

// listingPrint is ListingPrint of v: every field of the view that shapes
// the titles a library lists. A field added to the view that does must be
// named here (see unprintedViewFields).
func (v view) listingPrint() string {
	var b strings.Builder
	for _, entry := range v.addons {
		fmt.Fprintf(&b, "addon %s %t %t\n", entry.addon.ID, entry.confined, entry.shared)
	}
	for _, l := range v.libraries {
		fmt.Fprintf(&b, "library %s %s %t %t %q %q %q %d %t %t\n", l.item.ID, l.addon.addon.ID, l.addon.confined, l.addon.shared,
			l.catalog.Type, l.catalog.ID, l.genre, l.maxItems, l.skipLocal, l.mixed)
	}
	for _, kind := range slices.Sorted(maps.Keys(v.listedFolders)) {
		fmt.Fprintf(&b, "folders %s %v\n", kind, v.listedFolders[kind])
	}
	parental, _ := json.Marshal(v.parental)
	fmt.Fprintf(&b, "parental %s\ngenres %q\nwithheld %v\n", parental, v.genres, v.withheld)
	fmt.Fprintf(&b, "language %q\n", v.language)
	for _, entry := range v.translators {
		fmt.Fprintf(&b, "translator %s\n", entry.addon.ID)
	}
	fmt.Fprintf(&b, "words %+v\n", v.words)
	fmt.Fprintf(&b, "others %v\n", slices.SortedFunc(maps.Keys(v.others), func(a, b accounts.ID) int { return bytes.Compare(a[:], b[:]) }))
	fmt.Fprintf(&b, "country %q release %t %t limits %d %t\n", v.country, v.waitRelease, v.upcoming, v.catalogLimit, v.members)
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:16])
}

// StoredChanges counts the changes to what the libraries Polyfin stores
// list, those NamePool reads: their local folders' scans, links and
// options (see Local.Changes), their IPTV sources' lists and options (see
// LineupChanged), the addons and their libraries (see
// addons.Store.Changes), and administrators' edits (see changeOverrides).
// It is a sum of counts that only grow: it moves whenever one does, so
// that an order of a stored library's titles kept at one count is sorted
// again once it moved.
func (s *Service) StoredChanges() uint64 {
	changes := s.storedChanges.Load() + s.editChanges.Load() + s.addons.Changes()
	if s.local != nil {
		changes += s.local.Changes()
	}
	return changes
}

// WithLengths gives the movies of items, titles of parent, one of user's
// libraries, that have no length the one their file plays, when parent
// is a local folder (see localLengths): a listing by length sorts them
// by it. Its catalog lists none. items are changed in place.
func (s *Service) WithLengths(ctx context.Context, user accounts.User, parent accounts.ID, items []Item) error {
	v, err := s.view(ctx, user)
	if err != nil {
		return err
	}
	if l, ok := v.library(parent); !ok || !l.addon.addon.Local() {
		return nil
	}
	titles := make([]*Item, len(items))
	for i := range items {
		titles[i] = &items[i]
	}
	s.localLengths(ctx, titles)
	return nil
}

// Letter is a letter of a listing by name: how many titles file under it,
// and the position of the first, from 0.
type Letter struct {
	Letter   string
	Count    int
	Position int
}

// Run is a block of consecutive titles of a listing by name that file
// under one letter, from Position to the next run's, or to the end.
type Run struct {
	Letter   string
	Position int
}

// Letters files the titles of a listing by name, whose names fold to keys
// in their order, under their letters (see NameLetter): each letter with
// a title, in the order of its first, and the runs that cover them. A
// letter has one run, unless a letter of another alphabet or a symbol
// sorting among the letters splits #.
func Letters(keys []string) ([]Letter, []Run) {
	letters, runs := []Letter{}, []Run{}
	at := map[string]int{}
	for position, key := range keys {
		letter := NameLetter(key)
		i, seen := at[letter]
		if !seen {
			i = len(letters)
			at[letter] = i
			letters = append(letters, Letter{Letter: letter, Position: position})
		}
		letters[i].Count++
		if len(runs) == 0 || runs[len(runs)-1].Letter != letter {
			runs = append(runs, Run{Letter: letter, Position: position})
		}
	}
	return letters, runs
}
