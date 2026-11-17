package library

import (
	"reflect"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// The outlines PickedEpisodes chooses from, and the items it describes of
// those kept, in the order kept, are those Episodes lists: in its order,
// as aired with the specials first, last or hidden, with what
// administrators changed. The records it makes, of the episodes kept or of
// all of them, are those the series' listing makes.
func TestPickedEpisodesListAsEpisodesDo(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	future := time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339)
	show := stremio.Meta{ID: "tt100", Type: "series", Name: "Show", Poster: "https://images.example/show.jpg",
		Extras: &stremio.Extras{SeasonPosterByNumber: map[string]string{"2": "https://images.example/s2.jpg"}},
		Videos: []stremio.Video{
			{ID: "tt100:2:1", Title: "Second one", Season: 2, Episode: 1, Released: future},
			{ID: "tt100:1:2", Title: "Pilot two", Season: 1, Episode: 2, Released: "2020-01-08T00:00:00Z"},
			{ID: "tt100:1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00Z", Runtime: "42min"},
			{ID: "tt100:0:1", Name: "Special", Season: 0, Episode: 1, Released: "2019-12-01T00:00:00Z"},
			{ID: "tt100:1:3", Season: 1, Number: 3},
			{ID: "tt100:2:2", Title: "Second two", Season: 2, Episode: 2, Released: future},
		}}
	preview := show
	preview.Videos, preview.Extras = nil, nil
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Types: []string{"series"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", Types: []string{"series"}, IDPrefixes: []string{"tt"}}},
			Catalogs:  []stremio.Catalog{{Type: "series", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"series/top": {preview}},
		metas:    map[string]stremio.Meta{"series/tt100": show},
	})
	page, err := e.service.Children(ctx, e.member, e.library(e.member, "Top").ID, 0, 10, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("the series: %+v %v", page, err)
	}
	series := page.Items[0]
	// An administrator moves an episode's air date, and renames a season.
	episodes, err := e.service.Episodes(ctx, e.member, series.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	moved := time.Date(2030, 5, 6, 0, 0, 0, 0, time.UTC)
	for _, episode := range episodes {
		if episode.StremioID == "tt100:1:2" {
			if err := e.service.SaveOverrides(ctx, episode, Overrides{PremiereDate: &moved}); err != nil {
				t.Fatal(err)
			}
		}
	}
	seasons, err := e.service.Seasons(ctx, e.member, series.ID)
	if err != nil || len(seasons) != 3 {
		t.Fatalf("seasons: %+v %v", seasons, err)
	}
	renamed := "The Start"
	if err := e.service.SaveOverrides(ctx, seasons[1], Overrides{Name: &renamed}); err != nil {
		t.Fatal(err)
	}

	check := func(order string, listed int) {
		t.Helper()
		v, err := e.service.view(ctx, e.member)
		if err != nil {
			t.Fatal(err)
		}
		items, records, err := e.service.orderedEpisodes(ctx, v, series.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := e.service.overridden(items)
		if len(want) != listed {
			t.Fatalf("%s: %d episodes listed", order, len(want))
		}
		list, err := e.service.listedEpisodes(ctx, v, series.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(list.outlines, Outlines(want)) {
			t.Errorf("%s: outlines\n%+v\nwant\n%+v", order, list.outlines, Outlines(want))
		}
		all := make([]int, len(want))
		for i := range all {
			all[i] = i
		}
		if got := e.service.listedItems(ctx, list, all); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: items\n%+v\nwant\n%+v", order, got, want)
		}
		if got := list.records(nil); !reflect.DeepEqual(got, records) {
			t.Errorf("%s: records\n%+v\nwant\n%+v", order, got, records)
		}
		// Two kept, the second first: their items in that order, and the
		// records of those two only.
		kept := []int{2, 0}
		if got := e.service.listedItems(ctx, list, kept); !reflect.DeepEqual(got, []Item{want[2], want[0]}) {
			t.Errorf("%s: items kept\n%+v\nwant\n%+v", order, got, []Item{want[2], want[0]})
		}
		var wantKept []record
		for _, r := range records {
			if r.Kind != KindEpisode || r.ID == want[0].ID || r.ID == want[2].ID {
				wantKept = append(wantKept, r)
			}
		}
		if got := list.records(func(id accounts.ID) bool { return id == want[0].ID || id == want[2].ID }); !reflect.DeepEqual(got, wantKept) {
			t.Errorf("%s: records kept\n%+v\nwant\n%+v", order, got, wantKept)
		}
	}
	check("as aired", len(show.Videos))
	for _, specials := range []string{SpecialsLast, SpecialsHidden} {
		if err := e.service.SetEpisodeOrder(ctx, e.member, series.ID, "", specials); err != nil {
			t.Fatal(err)
		}
		listed := len(show.Videos)
		if specials == SpecialsHidden {
			listed--
		}
		check("specials "+specials, listed)
	}
}
