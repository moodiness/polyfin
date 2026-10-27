package library

import (
	"errors"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A title described again credits the people its new description lists:
// someone it no longer lists loses it on their page, as in Jellyfin, where
// a refreshed item's people replace the ones it had.
func TestTitlesDescribedAgainReplaceTheirCredits(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	addon := describedAddon()
	movie := addon.metas["movie/tt1"]
	movie.Cast = stremio.Names{"Ana Actor", "Ben Actor"}
	addon.metas["movie/tt1"] = movie
	e.install(addons.Shared(), addon)
	var heist Item
	for _, item := range e.children(e.admin, "Top (Movies)", 0, 10).Items {
		if item.Name == "The Heist" {
			heist = item
		}
	}
	if _, err := e.service.Item(t.Context(), e.admin, heist.ID); err != nil {
		t.Fatal(err)
	}
	titles := func(name string) []string {
		t.Helper()
		page, err := e.service.PersonTitles(t.Context(), e.admin, PersonID(name), []Kind{KindMovie}, 10)
		// Someone credited in no title the user reaches is no one to them.
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			t.Fatalf("%s's titles: %v", name, err)
		}
		return names(page.Items)
	}
	if got := titles("Ben Actor"); !slices.Equal(got, []string{"The Heist"}) {
		t.Fatalf("before: Ben Actor's titles %v", got)
	}

	// The addon drops Ben Actor and credits someone new.
	addon.mu.Lock()
	movie.Cast = stremio.Names{"Ana Actor", "Cleo Actor"}
	addon.metas["movie/tt1"] = movie
	addon.mu.Unlock()
	if err := e.service.Refresh(t.Context(), e.admin, heist.ID); err != nil {
		t.Fatal(err)
	}
	if got := titles("Ben Actor"); len(got) != 0 {
		t.Errorf("Ben Actor, no longer credited, still lists %v", got)
	}
	for _, name := range []string{"Ana Actor", "Cleo Actor"} {
		if got := titles(name); !slices.Equal(got, []string{"The Heist"}) {
			t.Errorf("%s's titles: %v", name, got)
		}
	}
}

// The cast, directors and writers an addon gives only as Stremio links
// are the title's people, in their order, as Stremio apps show them, and
// open with the title on their page.
func TestPeopleGivenAsLinks(t *testing.T) {
	t.Parallel()
	e := languageEnv{env: newEnv(t)}
	addon := describedAddon()
	addon.rawMetas = map[string]string{"movie/tt1": `{"id": "tt1", "type": "movie", "name": "The Heist", "links": [
		{"name": "Drama", "category": "Genres", "url": "stremio:///discover"},
		{"name": "Ana Actor", "category": "Cast", "url": "stremio:///search?search=Ana"},
		{"name": "Ben Actor", "category": "Cast", "url": "stremio:///search?search=Ben"},
		{"name": "Eve Director", "category": "Directors", "url": "stremio:///search?search=Eve"},
		{"name": "Finn Writer", "category": "Writers", "url": "stremio:///search?search=Finn"}]}`}
	e.install(addons.Shared(), addon)
	id := e.titleIDs("Top (Movies)")["The Heist"]
	item, err := e.service.Item(t.Context(), e.admin, id)
	if err != nil {
		t.Fatal(err)
	}
	var people []string
	for _, person := range item.People {
		people = append(people, person.Name+"/"+person.Type)
	}
	if want := []string{"Ana Actor/Actor", "Ben Actor/Actor", "Eve Director/Director", "Finn Writer/Writer"}; !slices.Equal(people, want) {
		t.Errorf("people %v, want %v", people, want)
	}
	page, err := e.service.PersonTitles(t.Context(), e.admin, PersonID("Eve Director"), []Kind{KindMovie}, 10)
	if err != nil || !slices.Equal(names(page.Items), []string{"The Heist"}) {
		t.Errorf("the director's titles: %v %v", names(page.Items), err)
	}
}
