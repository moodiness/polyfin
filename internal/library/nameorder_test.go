package library

import (
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Titles sort by their sort name, without regard to case or accents:
// articles count unless a sort name leaves them out, digits and symbols
// come before the letters and other alphabets after, and each files under
// its letter, or # for what is not one of A to Z.
func TestTitlesSortAndFileByName(t *testing.T) {
	t.Parallel()
	title := func(n byte, name, sortName string) Item {
		return Item{ID: accounts.ID{n}, Name: name, SortName: sortName}
	}
	items := []Item{
		title(1, "Zulu", ""), title(2, "The Hobbit", ""), title(3, "The Matrix", "Matrix, The"), title(4, "Éclair", ""),
		title(5, "apple", ""), title(6, "1917", ""), title(7, "Ωmega", ""), title(8, "Echo", ""), title(9, "'Salem's Lot", ""),
		title(10, "Ångström", ""),
	}
	SortByName(items, false)
	var names, keys []string
	for _, item := range items {
		names = append(names, item.Name)
		keys = append(keys, NameKey(item))
	}
	want := []string{"'Salem's Lot", "1917", "Ångström", "apple", "Echo", "Éclair", "The Matrix", "The Hobbit", "Zulu", "Ωmega"}
	if !slices.Equal(names, want) {
		t.Fatalf("by name: %q, want %q", names, want)
	}
	letters, runs := Letters(keys)
	wantLetters := []Letter{{"#", 3, 0}, {"A", 2, 2}, {"E", 2, 4}, {"M", 1, 6}, {"T", 1, 7}, {"Z", 1, 8}}
	if !slices.Equal(letters, wantLetters) {
		t.Errorf("letters: %+v, want %+v", letters, wantLetters)
	}
	wantRuns := []Run{{"#", 0}, {"A", 2}, {"E", 4}, {"M", 6}, {"T", 7}, {"Z", 8}, {"#", 9}}
	if !slices.Equal(runs, wantRuns) {
		t.Errorf("runs: %+v, want %+v", runs, wantRuns)
	}
	SortByName(items, true)
	if items[0].Name != "Ωmega" || items[len(items)-1].Name != "'Salem's Lot" {
		t.Errorf("descending: %s ... %s", items[0].Name, items[len(items)-1].Name)
	}
	if letters, runs := Letters(nil); letters == nil || runs == nil || len(letters)+len(runs) != 0 {
		t.Errorf("no titles: %v %v", letters, runs)
	}
}
