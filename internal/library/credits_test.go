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
