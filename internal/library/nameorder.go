package library

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
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
// the order is the same at every listing.
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
		result := cmp.Or(strings.Compare(a.key, b.key), strings.Compare(a.item.SortName, b.item.SortName),
			strings.Compare(a.item.ID.String(), b.item.ID.String()))
		if descending {
			return -result
		}
		return result
	})
	for i := range sorted {
		items[i] = sorted[i].item
	}
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
