package library

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// An addon's collection whose titles wait for their rating is left out of
// a restricted listing, which tells that more may follow, and its own
// pages answer as hidden; once the ratings are known, it shows.
func TestAddonCollectionsWaitForTheirTitlesRatings(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "g", Name: "G", Version: "1", Types: []string{"movie", "collection"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs: []stremio.Catalog{{Type: "collection", ID: "groups", Name: "Groups"},
				{Type: "movie", ID: "family", Name: "Family movies"}}},
		catalogs: map[string][]stremio.Meta{
			"collection/groups": {{ID: "group:family", Type: "collection", Name: "Family"}},
			"movie/family":      {{ID: "tt1", Type: "movie", Name: "Kids"}},
		},
		metas: map[string]stremio.Meta{
			"collection/group:family": {ID: "group:family", Type: "collection", Name: "Family",
				Collection: &stremio.Collection{Sources: []stremio.CollectionSource{{Type: "movie", CatalogID: "family"}}}},
			"movie/tt1": {ID: "tt1", Type: "movie", Name: "Kids", Extras: &stremio.Extras{Certification: "G"}},
		},
		metaGate:     make(chan struct{}),
		metaGateType: "movie",
	}
	e.install(addons.Shared(), addon)
	e.service.ratingWait = 50 * time.Millisecond
	child := e.restrict("child", accounts.ParentalControl{MaxRating: new(13)})
	page := e.children(e.member, "Groups", 0, 10)
	if !slices.Equal(names(page.Items), []string{"Family"}) {
		t.Fatalf("the member's collections: %v", names(page.Items))
	}
	family := page.Items[0].ID
	if page := e.children(child, "Groups", 0, 10); len(page.Items) != 0 || !page.More {
		t.Errorf("the child's collections while ratings wait: %v more=%v", names(page.Items), page.More)
	}
	if _, err := e.service.Item(t.Context(), child, family); !errors.Is(err, ErrHidden) {
		t.Errorf("the child's page of Family while ratings wait: %v", err)
	}
	close(addon.metaGate)
	deadline := time.Now().Add(5 * time.Second)
	for {
		page := e.children(child, "Groups", 0, 10)
		if slices.Equal(names(page.Items), []string{"Family"}) && !page.More {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the child's collections once ratings are known: %v more=%v", names(page.Items), page.More)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := e.service.Item(t.Context(), child, family); err != nil {
		t.Errorf("the child's page of Family: %v", err)
	}
}
