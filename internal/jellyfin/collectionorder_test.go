package jellyfin

import (
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/collections"
)

// A collection lists by premiere date, as Jellyfin's, until an
// administrator chooses its own order in the admin app: then apps, its
// home rows and channels list its entries in that order.
func TestCollectionsListInTheOrderChosenForThem(t *testing.T) {
	t.Parallel()
	s, admin, titles := grouping(t)
	inner := s.createCollection(t, admin, "name=Inner&ids="+titles.adults)
	picks := s.createCollection(t, admin, "name=Picks&ids="+titles.show+","+titles.kids+","+inner+","+titles.teens)
	id, _ := accounts.ParseID(picks)
	ctx := t.Context()
	listed := func(query string) []string {
		t.Helper()
		var page QueryResult
		if status := s.get(t, "/Items?ParentId="+picks+query, admin, &page); status != http.StatusOK {
			t.Fatalf("listing: %d", status)
		}
		return itemNames(page.Items)
	}
	row := func() []string {
		t.Helper()
		user, err := s.store.Authenticate(ctx, "admin", "correct horse")
		if err != nil {
			t.Fatal(err)
		}
		page, err := s.handler.collectionRowTitles(ctx, user, id, 0, 10, false)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, item := range page.Items {
			names = append(names, item.Name)
		}
		return names
	}
	if got := listed(""); !slices.Equal(got, []string{"Teens", "Kids", "Inner", "Show"}) {
		t.Errorf("by premiere date: %v", got)
	}
	if got := row(); !slices.Equal(got, []string{"Teens", "Kids", "Adults", "Show"}) {
		t.Errorf("its row by premiere date: %v", got)
	}

	chosen := collections.OrderChosen
	if err := s.handler.Collections.Change(ctx, id, collections.Changes{DisplayOrder: &chosen}); err != nil {
		t.Fatal(err)
	}
	if got := listed(""); !slices.Equal(got, []string{"Show", "Kids", "Inner", "Teens"}) {
		t.Errorf("in the order added: %v", got)
	}
	c, _ := s.handler.Collections.Get(ctx, id)
	order := []accounts.ID{c.Entries[2], c.Entries[3], c.Entries[0], c.Entries[1]}
	if err := s.handler.Collections.Reorder(ctx, id, order); err != nil {
		t.Fatal(err)
	}
	if got := listed(""); !slices.Equal(got, []string{"Inner", "Teens", "Show", "Kids"}) {
		t.Errorf("in the order chosen: %v", got)
	}
	// Played whole, a collection inside brings its titles after it.
	if got := listed("&Recursive=true&Filters=IsNotFolder&MediaTypes=Video"); !slices.Equal(got, []string{"Adults", "Teens", "Pilot", "Second", "Kids"}) {
		t.Errorf("played in the order chosen: %v", got)
	}
	if got := row(); !slices.Equal(got, []string{"Adults", "Teens", "Show", "Kids"}) {
		t.Errorf("its row in the order chosen: %v", got)
	}
}
