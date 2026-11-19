package library

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/stremio"
)

// manyAddon serves count movie catalogs of 20 titles each, in one page.
func manyAddon(count int) *fakeAddon {
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "many", Name: "Many", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}}},
		catalogs: map[string][]stremio.Meta{},
		metas:    map[string]stremio.Meta{},
	}
	for i := range count {
		id := fmt.Sprintf("c%d", i)
		addon.manifest.Catalogs = append(addon.manifest.Catalogs, stremio.Catalog{Type: "movie", ID: id, Name: id, Extra: []stremio.Extra{{Name: "skip"}}})
		addon.catalogs["movie/"+id] = titles("movie", 20)
	}
	return addon
}

// A listing whose pages are in the database only reads pageReadsAtOnce of
// them at once, however many catalogs it reads at a time: the pool keeps
// connections for the rest of the server.
func TestPagesReadFromTheDatabaseTakeFewConnectionsAtOnce(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.service.smartWait = time.Minute
	const catalogs = 2 * pageReadsAtOnce
	addon := manyAddon(catalogs)
	e.install(addons.Shared(), addon)
	installed, err := e.addons.Addons(t.Context(), addons.Shared())
	if err != nil || len(installed) != 1 {
		t.Fatalf("addons: %v %v", installed, err)
	}
	var ids []string
	for i := range catalogs {
		ids = append(ids, fmt.Sprintf("movie/c%d", i))
	}
	first := e.makeList(nil, "Many", installed[0].ID, ids, group(smartlists.MatchAll), smartlists.SortCatalog, 0)
	// Another list of the same catalogs, whose titles nothing keeps yet,
	// reads their pages again.
	second := e.makeList(nil, "Same", installed[0].ID, ids, group(smartlists.MatchAll), smartlists.SortCatalog, 0)

	// Each read of a page holds its place until pageReadsAtOnce of them
	// hold theirs.
	var (
		armed  atomic.Bool
		mu     sync.Mutex
		held   int
		most   int
		filled = make(chan struct{})
		once   sync.Once
	)
	e.service.pageReadHook = func() {
		if !armed.Load() {
			return
		}
		mu.Lock()
		held++
		most = max(most, held)
		reached := held == pageReadsAtOnce
		mu.Unlock()
		if reached {
			once.Do(func() { close(filled) })
		}
		<-filled
		mu.Lock()
		held--
		mu.Unlock()
	}
	if total := e.listing(e.member, first).Total; total != 20 {
		t.Fatalf("first listing: %d titles", total)
	}
	requests := len(addon.catalogRequests())
	e.service.pages.DeleteFunc(func(pageKey, fetched[[]stremio.Meta]) bool { return true })
	armed.Store(true)
	if total := e.listing(e.member, second).Total; total != 20 {
		t.Errorf("from the database: %d titles", total)
	}
	if asked := len(addon.catalogRequests()) - requests; asked != 0 {
		t.Errorf("%d pages asked of the addon, not read from the database", asked)
	}
	mu.Lock()
	defer mu.Unlock()
	if most != pageReadsAtOnce {
		t.Errorf("%d pages read from the database at once, want %d", most, pageReadsAtOnce)
	}
}
