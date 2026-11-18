package library

import (
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A title none of the server's addons describes has no rating, for good,
// when one of the server's addons listed it. One a user's own addon
// holds is typed by that addon alone, which may name a title the server's
// addons rate under a type they do not describe: its rating stays
// unknown for the others, so that a restricted user is not shown it.
func TestTheRatingOfATitleAUsersOwnAddonHoldsStaysUnknown(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	typed := func(id string) []stremio.Meta { return []stremio.Meta{{ID: id, Type: "anime.movie", Name: id}} }
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "shared", Name: "Shared", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", Types: []string{"movie"}}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"movie/top": typed("tt0000001")},
	})
	// An administrator's own addon reaches the test's local server.
	e.install(addons.Personal(e.admin.ID), &fakeAddon{
		manifest: stremio.Manifest{ID: "own", Name: "Own", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "mine", Name: "Mine"}}},
		catalogs: map[string][]stremio.Meta{"movie/mine": typed("tt0000002")},
	})
	for _, name := range []string{"Top", "Mine"} {
		if page, err := e.service.Children(t.Context(), e.admin, e.library(e.admin, name).ID, 0, 10, ""); err != nil || len(page.Items) != 1 {
			t.Fatalf("the library %s: %+v %v", name, page.Items, err)
		}
	}
	restricted, err := e.users.UpdateUser(t.Context(), e.member.ID, accounts.UserChanges{Parental: &accounts.ParentalControl{MaxRating: new(13)}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.service.view(t.Context(), restricted)
	if err != nil {
		t.Fatal(err)
	}
	known := func(id string) traits {
		t.Helper()
		r, err := e.service.load(t.Context(), itemID(titleKey(KindMovie, id)))
		if err != nil {
			t.Fatal(err)
		}
		got, _ := e.service.knownTraits(t.Context(), v, r)
		return got
	}
	if got := known("tt0000001"); !got.ratingKnown || !got.genresKnown || got.rating != "" {
		t.Errorf("the title one of the server's addons listed: %+v", got)
	}
	if got := known("tt0000002"); got.ratingKnown || got.genresKnown {
		t.Errorf("the title a user's own addon holds: %+v", got)
	}
	if allowed, decided := v.verdict(KindMovie, known("tt0000002")); allowed || decided {
		t.Errorf("the verdict on the title a user's own addon holds: allowed %t, decided %t", allowed, decided)
	}
}
