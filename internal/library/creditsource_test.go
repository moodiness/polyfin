package library

import (
	"errors"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/stremio"
)

// A title's credits are those of the description its page takes its
// people from: the first addon without a language that describes it,
// starting with the one whose catalog listed it. Another such addon's
// description, or a language copy's, read before or after, changes no
// one's titles, so that a person's page agrees with the title's.
func TestCreditsFollowTheDescriptionTheTitleShows(t *testing.T) {
	t.Parallel()
	e := languageEnv{env: newEnv(t)}
	base := describedAddon()
	e.installIn(base, "")
	movie := base.metas["movie/tt1"]
	movie.Cast = stremio.Names{"Bob Other"}
	other := &fakeAddon{manifest: stremio.Manifest{ID: "b", Name: "B", Version: "1", Types: []string{"movie"},
		Resources: []stremio.Resource{{Name: "meta", IDPrefixes: []string{"tt"}}}},
		metas: map[string]stremio.Meta{"movie/tt1": movie}}
	e.installIn(other, "")
	e.installIn(translatedAddon("fr", "Le Casse", "Le Show", "Pilote"), "fr")
	heist := e.titleIDs("Top (Movies)")["The Heist"]
	item, err := e.service.Item(t.Context(), e.admin, heist)
	if err != nil || len(item.People) != 1 || item.People[0].Name != "Ana Actor" {
		t.Fatalf("the title's people: %+v %v", item.People, err)
	}
	v, err := e.service.view(t.Context(), e.admin)
	if err != nil {
		t.Fatal(err)
	}
	entry := func(id string) installed {
		for _, entry := range v.addons {
			if entry.addon.Manifest.ID == id {
				return entry
			}
		}
		t.Fatalf("no addon %s", id)
		return installed{}
	}
	// Each description read anew, the others' between the title's own.
	for _, id := range []string{"b", "a.fr", "a", "b", "a.fr"} {
		if _, err := e.service.fetchKeptMeta(t.Context(), entry(id), metaKey{entry(id).addon.ID, "movie", "tt1"}); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	titles := func(name string) []string {
		t.Helper()
		page, err := e.service.PersonTitles(t.Context(), e.admin, PersonID(name), []Kind{KindMovie}, 10)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			t.Fatalf("%s's titles: %v", name, err)
		}
		return names(page.Items)
	}
	if got := titles("Ana Actor"); !slices.Equal(got, []string{"The Heist"}) {
		t.Errorf("Ana Actor, credited on the title's page: %v", got)
	}
	for _, name := range []string{"Bob Other", "Someone fr"} {
		if got := titles(name); len(got) != 0 {
			t.Errorf("%s, not on the title's page: %v", name, got)
		}
	}
}
