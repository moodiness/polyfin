package library

import (
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// limitsAddon serves a paged film catalog of films titles and a paged live
// TV catalog of channels channels.
func limitsAddon(films, channels int) *fakeAddon {
	paged := []stremio.Extra{{Name: "skip"}}
	return &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{
				{Type: "movie", ID: "top", Name: "Top", Extra: paged},
				{Type: "tv", ID: "channels", Name: "Channels", Extra: paged},
			}},
		catalogs: map[string][]stremio.Meta{"movie/top": titles("movie", films), "tv/channels": titles("tv", channels)},
		pageSize: 20,
	}
}

func TestCatalogLimitStopsFilmCatalogs(t *testing.T) {
	e := newEnv(t)
	e.install(addons.Shared(), limitsAddon(250, 0))
	e.setting(func(settings *accounts.Settings) { settings.CatalogLimit = accounts.MinCatalogLimit })
	top := e.library(e.member, "Top")
	children := func(start, count int) Page {
		t.Helper()
		page, err := e.service.Children(t.Context(), e.member, top.ID, start, count, "")
		if err != nil {
			t.Fatal(err)
		}
		return page
	}

	// Up to the limit, more may follow.
	page := children(0, 100)
	if len(page.Items) != 100 || page.Items[99].Name != "movie 99" || !page.More || page.Total != 101 {
		t.Fatalf("up to the limit: %d items, more=%v, total=%d", len(page.Items), page.More, page.Total)
	}
	// The catalog ends at the limit.
	page = children(80, 50)
	if len(page.Items) != 20 || page.Items[19].Name != "movie 99" || page.More || page.Total != 100 {
		t.Fatalf("across the limit: %d items, more=%v, total=%d", len(page.Items), page.More, page.Total)
	}
	if page := children(100, 50); len(page.Items) != 0 || page.More || page.Total != 100 {
		t.Fatalf("past the limit: %v, more=%v, total=%d", names(page.Items), page.More, page.Total)
	}

	// A higher limit applies at once.
	e.setting(func(settings *accounts.Settings) { settings.CatalogLimit = 200 })
	if page := children(100, 50); len(page.Items) != 50 || page.Items[0].Name != "movie 100" || !page.More {
		t.Fatalf("past the old limit: %d items, more=%v, total=%d", len(page.Items), page.More, page.Total)
	}
	if page := children(200, 50); len(page.Items) != 0 || page.More || page.Total != 200 {
		t.Fatalf("past the new limit: %v, more=%v, total=%d", names(page.Items), page.More, page.Total)
	}
}

func TestChannelLimitStopsLiveTVCatalogs(t *testing.T) {
	e := newEnv(t)
	e.install(addons.Shared(), limitsAddon(0, 250))
	channels := func() []Item {
		t.Helper()
		items, err := e.service.Channels(t.Context(), e.member)
		if err != nil {
			t.Fatal(err)
		}
		return items
	}

	e.setting(func(settings *accounts.Settings) { settings.ChannelLimit = 150 })
	if items := channels(); len(items) != 150 || items[149].Name != "tv 149" || items[149].Number != "150" {
		t.Fatalf("channels under a limit of 150: %d", len(items))
	}
	e.setting(func(settings *accounts.Settings) { settings.ChannelLimit = accounts.MaxChannelLimit })
	if items := channels(); len(items) != 250 || items[249].Name != "tv 249" {
		t.Fatalf("channels under the highest limit: %d", len(items))
	}
}

func TestCatalogAndChannelLimitsAreIndependent(t *testing.T) {
	e := newEnv(t)
	e.install(addons.Shared(), limitsAddon(250, 250))
	top := e.library(e.member, "Top")
	listings := func() (films, channels int) {
		t.Helper()
		page, err := e.service.Children(t.Context(), e.member, top.ID, 0, 300, "")
		if err != nil {
			t.Fatal(err)
		}
		items, err := e.service.Channels(t.Context(), e.member)
		if err != nil {
			t.Fatal(err)
		}
		return len(page.Items), len(items)
	}

	// A low channel limit leaves film catalogs whole.
	e.setting(func(settings *accounts.Settings) {
		settings.CatalogLimit, settings.ChannelLimit = accounts.MaxCatalogLimit, accounts.MinChannelLimit
	})
	if films, channels := listings(); films != 250 || channels != 100 {
		t.Errorf("low channel limit: %d films, %d channels", films, channels)
	}
	// And a low catalog limit leaves live TV catalogs whole.
	e.setting(func(settings *accounts.Settings) {
		settings.CatalogLimit, settings.ChannelLimit = accounts.MinCatalogLimit, accounts.MaxChannelLimit
	})
	if films, channels := listings(); films != 100 || channels != 250 {
		t.Errorf("low catalog limit: %d films, %d channels", films, channels)
	}
}
