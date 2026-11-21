package library

import (
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
)

// The titles a sorted listing of an addon's library sorts are those read
// of its catalog, asking the addon nothing beyond the first titles the
// listing needs, while the rest of the catalog is read in the background:
// the next pools hold all of it, each title with when it was first seen
// in the catalog, and the catalog's version tells the pools apart.
func TestACatalogPoolGrowsAsTheCatalogIsReadThrough(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.service.readAhead = true
	addon := pagedAddon(300, 100)
	e.install(addons.Shared(), addon)
	top := e.library(e.member, "Top").ID
	gate := make(chan struct{})
	addon.mu.Lock()
	addon.catalogGate = gate
	addon.mu.Unlock()

	first, version, ok, err := e.service.CatalogPool(t.Context(), e.member, top, "", 0)
	if err != nil || !ok {
		t.Fatalf("pool: %v, %v", ok, err)
	}
	if len(first.Items) != 0 || !first.More {
		t.Errorf("first pool, nothing read: %d titles, more %t", len(first.Items), first.More)
	}
	close(gate)
	// A listing of 120 titles reads them first.
	if needed, _, _, err := e.service.CatalogPool(t.Context(), e.member, top, "", 120); err != nil || len(needed.Items) < 120 {
		t.Errorf("a pool for 120 titles: %d titles, %v", len(needed.Items), err)
	}
	eventually(t, "reading the catalog through", func() bool { return requestsTo(addon, "catalog/movie/top/skip=300") == 1 })
	eventually(t, "pooling every title", func() bool {
		page, _, _, err := e.service.CatalogPool(t.Context(), e.member, top, "", 0)
		return err == nil && len(page.Items) == 300 && !page.More
	})
	page, later, _, _ := e.service.CatalogPool(t.Context(), e.member, top, "", 0)
	if later == version {
		t.Errorf("the catalog's version did not change as it was read: %s", later)
	}
	for _, item := range page.Items {
		if item.Added == nil {
			t.Fatalf("%s has no date added", item.Name)
		}
	}
	if again, ok, _ := e.service.CatalogVersion(t.Context(), e.member, top, ""); !ok || again != later {
		t.Errorf("version %s, %t, want %s", again, ok, later)
	}
}
