package library

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// Reading the collections reads every collection of the server's
// collection libraries whole, so that a whole listing then lists all of
// each from what was kept. Read again past the pages' refresh age, it asks
// the addon again and waits for its answers: the pages it leaves are the
// addon's new ones, not the stale ones.
func TestReadingTheCollectionsReadsThemWhole(t *testing.T) {
	e := newEnv(t)
	elapsed := new(atomic.Int64)
	e.service.now = func() time.Time { return time.Now().Add(time.Duration(elapsed.Load())) }
	e.setting(func(s *accounts.Settings) { s.CatalogRefreshMinutes = 60 })
	addon := pairAddon(30)
	e.install(addons.Shared(), addon)
	e.chooseLibraries(libraryOf("collection", "sets", "", 0))
	pair := e.children(e.member, "Sets", 0, 10).Items[0].ID
	kept := func() Page {
		t.Helper()
		page, err := e.service.Children(withKeptOnly(t.Context()), e.member, pair, 0, WholeListing, "")
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	if err := e.service.ReadCollections(t.Context()); err != nil {
		t.Fatal(err)
	}
	if page := kept(); len(page.Items) != 60 || page.More {
		t.Fatalf("kept after reading: %d titles, more %v", len(page.Items), page.More)
	}

	addon.mu.Lock()
	for i := range addon.catalogs["movie/first"] {
		addon.catalogs["movie/first"][i].Name = fmt.Sprintf("renamed %d", i)
	}
	addon.mu.Unlock()
	elapsed.Add(int64(61 * time.Minute))
	// The addon answers late: a read that took the stale pages would not
	// wait for it.
	answer := blockCatalogs(t, addon)
	time.AfterFunc(200*time.Millisecond, answer)
	if err := e.service.ReadCollections(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := names(kept().Items); len(got) != 60 || got[0] != "renamed 0" {
		t.Errorf("kept after reading again: %d titles, first %v", len(got), got[:min(4, len(got))])
	}
}
