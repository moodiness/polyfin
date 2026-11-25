package library

import (
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testdb"
)

// What addons answered is kept in caches whose writes do not wait for the
// disk: the catalog pages and their sizes, the titles' descriptions, the
// stream lists. The items listings save, which nothing fetches again,
// still do.
func TestCachesAreWrittenWithoutWaitingForTheDisk(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	modes := testdb.WatchCommitModes(t, e.service.db, "catalog_pages", "catalog_sizes", "metas", "stream_lists", "items")
	// A collection of two paged catalogs: its description, the pages of
	// both catalogs, and their size.
	e.install(addons.Shared(), pairAddon(6))
	e.chooseLibraries(libraryOf("collection", "sets", "", 0))
	pair := e.children(e.member, "Sets", 0, 10).Items[0].ID
	if _, err := e.service.Children(t.Context(), e.member, pair, 0, 12, ""); err != nil {
		t.Fatal(err)
	}
	shared, err := e.addons.Addons(t.Context(), addons.Shared())
	if err != nil || len(shared) != 1 {
		t.Fatalf("addons: %v %v", shared, err)
	}
	key := streamKey{addon: shared[0].ID, contentType: "movie", id: "tt1"}
	e.service.streamLists.Put(key, list[stremio.Stream]{items: []stremio.Stream{{Name: "One", URL: "https://streams.example/one"}}, answer: 1,
		at: time.Now()})
	if saved, err := e.service.writeStreams(key); !saved || err != nil {
		t.Fatalf("the stream list: saved %v, %v", saved, err)
	}

	for _, table := range []string{"catalog_pages", "catalog_sizes", "metas", "stream_lists"} {
		if got := modes(table); len(got) == 0 || slices.ContainsFunc(got, func(mode string) bool { return mode != "off" }) {
			t.Errorf("%s written with synchronous_commit %v, want off", table, got)
		}
	}
	if got := modes("items"); len(got) == 0 || slices.ContainsFunc(got, func(mode string) bool { return mode != "on" }) {
		t.Errorf("items written with synchronous_commit %v, want on", got)
	}
}
