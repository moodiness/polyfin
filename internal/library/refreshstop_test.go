package library

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A stale catalog page a listing shows is asked for again in the
// background: the listing ending does not stop that, but the server
// stopping does, and once it stopped no refresh reaches the addon.
func TestAStalePagesRefreshEndsWithTheServer(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	server, stop := context.WithCancel(t.Context())
	defer stop()
	e.service.EndWith(server)
	elapsed := new(atomic.Int64)
	e.service.SetClock(func() time.Time { return time.Now().Add(time.Duration(elapsed.Load())) })
	e.setting(func(s *accounts.Settings) { s.CatalogRefreshMinutes = 1 })
	top := &fakeAddon{
		manifest: stremio.Manifest{ID: "top", Name: "Top", Version: "1", Types: []string{"movie"}, Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"movie/top": titles("movie", 3)},
	}
	busy := &slowCatalogs{addon: top, delay: 50 * time.Millisecond, asked: map[string]int{}, served: map[string]int{},
		slow: func(string) bool { return true }}
	e.installServing(addons.Shared(), busy)
	installed, err := e.addons.Addons(t.Context(), addons.Shared())
	if err != nil || len(installed) != 1 {
		t.Fatalf("the addons: %v %v", installed, err)
	}
	refresh := fmt.Sprintf("refresh page %v", pageKey{addon: installed[0].ID, catalogType: "movie", catalogID: "top"})
	library := e.library(e.member, "Top").ID
	list := func(ctx context.Context) {
		t.Helper()
		if page, err := e.service.Children(ctx, e.member, library, 0, 10, ""); err != nil || len(page.Items) != 3 {
			t.Fatalf("the listing: %d titles, %v", len(page.Items), err)
		}
	}
	asked := func() int {
		_, most := busy.answered("/catalog/movie/top")
		return most
	}
	list(t.Context())
	if asked() != 1 {
		t.Fatalf("the page asked %d times", asked())
	}

	// A listing of the page, stale, ends at once: the refresh goes on, to
	// its end.
	elapsed.Add(int64(2 * time.Minute))
	request, leave := context.WithCancel(t.Context())
	list(request)
	leave()
	ended := false
	for deadline := time.Now().Add(5 * time.Second); !ended; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("the stale page's refresh did not end once its listing ended: asked %d times", asked())
		}
		// A flight of the refresh's key runs this one only once the
		// refresh's own ended.
		ended = (<-e.service.flight.DoChan(refresh, func() (any, error) { return true, nil })).Val == true && asked() == 2
	}

	// The server stops; a listing still under way shows the page, stale
	// again, which no refresh asks for.
	elapsed.Add(int64(2 * time.Minute))
	stop()
	stopped := time.Now()
	list(t.Context())
	time.Sleep(10 * busy.delay)
	if late := busy.askedSince("/catalog/movie/top", stopped); len(late) > 0 {
		t.Errorf("pages asked after the server stopped: %v", late)
	}
}
