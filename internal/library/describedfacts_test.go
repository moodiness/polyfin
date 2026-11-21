package library

import (
	"maps"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A listing and a search tell what the item page does of a series whose
// description is kept: its identifiers, status and end date.
func TestListingsAndSearchesTakeTheKeptDescriptionsFacts(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Types: []string{"series"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs:  []stremio.Catalog{{Type: "series", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "search"}}}}},
		catalogs: map[string][]stremio.Meta{"series/top": {{ID: "tt0100", Type: "series", Name: "Harbour Lights"}}},
		metas: map[string]stremio.Meta{"series/tt0100": {ID: "tt0100", Type: "series", Name: "Harbour Lights",
			ReleaseInfo: "2001-2004", Status: "Ended", TmdbID: "4410"}},
	}
	e.install(addons.Shared(), addon)
	listed := e.children(e.member, "Top", 0, 10)
	if len(listed.Items) != 1 {
		t.Fatalf("listed %v", names(listed.Items))
	}
	page, err := e.service.Item(t.Context(), e.member, listed.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"Imdb": "tt0100", "Tmdb": "4410"}
	if page.Status != "Ended" || page.EndDate == nil || page.EndDate.Year() != 2004 || !maps.Equal(page.ProviderIDs, want) {
		t.Fatalf("item page: %q %v %v", page.Status, page.EndDate, page.ProviderIDs)
	}
	before := len(addon.requests)
	found, err := e.service.Search(t.Context(), e.member, "Harbour", []Kind{KindSeries}, 10)
	if err != nil || len(found) != 1 {
		t.Fatalf("search: %v %v", names(found), err)
	}
	for name, item := range map[string]Item{"listing": e.children(e.member, "Top", 0, 10).Items[0], "search": found[0]} {
		if item.Status != page.Status || item.EndDate == nil || !item.EndDate.Equal(*page.EndDate) || !maps.Equal(item.ProviderIDs, want) {
			t.Errorf("%s: %q %v %v", name, item.Status, item.EndDate, item.ProviderIDs)
		}
	}
	for _, request := range addon.requests[before:] {
		if request != "catalog/series/top/search=Harbour" {
			t.Errorf("asked %s", request)
		}
	}
}

func TestAClosedReleaseRangeEndsASeries(t *testing.T) {
	t.Parallel()
	s := newEnv(t).service
	for releaseInfo, want := range map[string]string{"2001-2004": "Ended", "2001-": "", "2001": ""} {
		item := Item{Kind: KindSeries}
		s.fromMeta(&item, stremio.Meta{ID: "tt1", Type: "series", ReleaseInfo: stremio.Text(releaseInfo)}, "")
		if item.Status != want {
			t.Errorf("%q: status %q", releaseInfo, item.Status)
		}
	}
	item := Item{Kind: KindSeries}
	s.fromMeta(&item, stremio.Meta{ID: "tt1", Type: "series", ReleaseInfo: "2001-2004", Status: "Returning Series"}, "")
	if item.Status != "Continuing" {
		t.Errorf("a status given: %q", item.Status)
	}
}

func TestADateOnlyAirDateIsOutAtTheEndOfItsDay(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		released string
		now      time.Time
		want     bool
	}{
		{"2026-03-05", day, false},
		{"2026-03-05", day.Add(23 * time.Hour), false},
		{"2026-03-05", day.Add(24 * time.Hour), true},
		{"2026-03-05T00:00:00.000Z", day, true},
		{"2026-03-05T20:00:00Z", day.Add(19 * time.Hour), false},
	} {
		if got := released(stremio.Video{Released: tc.released}, tc.now); got != tc.want {
			t.Errorf("%s at %v: released %v", tc.released, tc.now, got)
		}
	}
}
