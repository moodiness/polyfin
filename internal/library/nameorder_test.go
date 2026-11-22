package library

import (
	"reflect"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// unprintedViewFields are the fields of a view that its listing print
// leaves out (see view.listingPrint): who the user is, the titles they
// hid and a request's rating filter, which orders of titles are kept by
// apart, what a request's lookups count, and what lists no titles of a
// library: channels, smart lists and people's biographies.
var unprintedViewFields = []string{"user", "channels", "virtual", "channelFilter", "smart", "deadline", "lookups", "held", "hidden",
	"query", "channelLimit", "peopleLanguage"}

// printedViewFields are the fields of a view that its listing print names.
var printedViewFields = []string{"addons", "libraries", "country", "listedFolders", "parental", "genres", "members", "withheld",
	"waitRelease", "upcoming", "catalogLimit", "language", "translators", "words", "others"}

// Users who see their libraries alike share a listing print, whoever they
// are, whatever they hid and whatever ratings a request narrows to; one
// whose limits, language or addons differ does not. Every field of a view
// is printed or left out on purpose.
func TestListingPrintsNameHowUsersSeeTheirTitles(t *testing.T) {
	t.Parallel()
	var fields []string
	for field := range reflect.TypeFor[view]().Fields() {
		fields = append(fields, field.Name)
	}
	known := append(slices.Clone(unprintedViewFields), printedViewFields...)
	slices.Sort(fields)
	slices.Sort(known)
	if !slices.Equal(fields, known) {
		t.Fatalf("view fields %q, printed or not %q", fields, known)
	}
	base := view{catalogLimit: 2000, others: map[accounts.ID]bool{{7}: true}}
	other := base
	other.user, other.hidden, other.query = accounts.ID{1}, HiddenTitles{{2}: true}, ratingQuery{active: true}
	if base.listingPrint() != other.listingPrint() {
		t.Errorf("another user, their hidden titles or a rating filter change the print")
	}
	maxRating := 10
	for name, change := range map[string]func(*view){
		"parental": func(v *view) { v.parental.MaxRating = &maxRating },
		"genres":   func(v *view) { v.genres = []string{"Horror"} },
		"language": func(v *view) { v.language = "fr" },
		"others":   func(v *view) { v.others = nil },
		"withheld": func(v *view) { v.withheld = []accounts.ID{{3}} },
		"limit":    func(v *view) { v.catalogLimit = 5000 },
	} {
		changed := base
		change(&changed)
		if changed.listingPrint() == base.listingPrint() {
			t.Errorf("%s: the print does not change", name)
		}
	}
}

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
