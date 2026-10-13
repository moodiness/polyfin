package accounts

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The apps told collection libraries hold collections are kept trimmed,
// each once whatever its letter case with its first spelling, saved, read
// again by a new start, matched whatever the case, and emptied; an empty,
// too long or unprintable name, or too many, are refused.
func TestCollectionLibraryAppsAreSaved(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	if got := store.Settings().CollectionLibraryApps; got == nil || len(got) != 0 {
		t.Errorf("default: %#v", got)
	}
	changed := store.Settings()
	changed.CollectionLibraryApps = []string{" Shelf TV ", "Example Player", "shelf tv", "Écran du salon", strings.Repeat("é", MaxCollectionLibraryAppLength)}
	want := []string{"Shelf TV", "Example Player", "Écran du salon", strings.Repeat("é", MaxCollectionLibraryAppLength)}
	saved, err := store.UpdateSettings(ctx, changed)
	if err != nil || !slices.Equal(saved.CollectionLibraryApps, want) {
		t.Fatalf("saved: %q %v", saved.CollectionLibraryApps, err)
	}
	if !store.CollectionLibraryApp("SHELF TV") || !store.CollectionLibraryApp("écran du salon") || store.CollectionLibraryApp("Shelf") ||
		store.CollectionLibraryApp("") {
		t.Error("apps matched wrongly")
	}
	reopened, err := Open(ctx, store.db)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Settings().CollectionLibraryApps; !slices.Equal(got, want) {
		t.Errorf("read again: %q", got)
	}
	for _, wrong := range []string{"", "   ", "Shelf\tTV", "Shelf\x7fTV", "Shelf\u0085TV", "\xff", strings.Repeat("a", MaxCollectionLibraryAppLength+1)} {
		refused := changed
		refused.CollectionLibraryApps = []string{"Example Player", wrong}
		if _, err := store.UpdateSettings(ctx, refused); !errors.Is(err, ErrInvalidCollectionLibraryApps) {
			t.Errorf("%q: %v", wrong, err)
		}
	}
	many := make([]string, MaxCollectionLibraryApps+1)
	for i := range many {
		many[i] = "App " + strconv.Itoa(i)
	}
	changed.CollectionLibraryApps = many[:MaxCollectionLibraryApps]
	if _, err := store.UpdateSettings(ctx, changed); err != nil {
		t.Errorf("%d apps: %v", MaxCollectionLibraryApps, err)
	}
	changed.CollectionLibraryApps = many
	if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, ErrInvalidCollectionLibraryApps) {
		t.Errorf("%d apps: %v", len(many), err)
	}
	if _, err := store.db.Exec(ctx, "UPDATE settings SET collection_library_apps = $1", many); err == nil {
		t.Errorf("the database took %d apps", len(many))
	}
	changed.CollectionLibraryApps = nil
	if saved, err := store.UpdateSettings(ctx, changed); err != nil || saved.CollectionLibraryApps == nil || len(saved.CollectionLibraryApps) != 0 ||
		store.CollectionLibraryApp("Shelf TV") {
		t.Errorf("emptied: %#v %v", saved.CollectionLibraryApps, err)
	}
}
