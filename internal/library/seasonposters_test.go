package library

import (
	"maps"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A metadata addon lists season posters without season numbers, from its
// seasons in order, specials included or not: the list is read only when
// its length tells which poster is which season's.
func TestSeasonPostersMapToTheirSeasons(t *testing.T) {
	show := func(seasons []int, posters stremio.Posters, byNumber map[string]string) stremio.Meta {
		meta := stremio.Meta{ID: "tt1", Type: "series", Extras: &stremio.Extras{SeasonPosters: posters, SeasonPosterByNumber: byNumber}}
		for _, season := range seasons {
			meta.Videos = append(meta.Videos, stremio.Video{ID: "v", Season: stremio.Number(season), Episode: 1})
		}
		return meta
	}
	for _, tc := range []struct {
		name     string
		seasons  []int
		posters  stremio.Posters
		byNumber map[string]string
		want     map[int]string
	}{
		{"one per season", []int{1, 2, 3}, stremio.Posters{"a", "b", "c"}, nil, map[int]string{1: "a", 2: "b", 3: "c"}},
		{"by number, whatever the videos' order", []int{3, 1, 2, 1}, stremio.Posters{"a", "b", "c"}, nil, map[int]string{1: "a", 2: "b", 3: "c"}},
		{"numbers with gaps", []int{1, 4}, stremio.Posters{"a", "b"}, nil, map[int]string{1: "a", 4: "b"}},
		{"specials listed", []int{0, 1, 2}, stremio.Posters{"s", "a", "b"}, nil, map[int]string{0: "s", 1: "a", 2: "b"}},
		{"specials not listed", []int{0, 1, 2}, stremio.Posters{"a", "b"}, nil, map[int]string{1: "a", 2: "b"}},
		{"missing posters", []int{0, 1, 2, 3}, stremio.Posters{"a", "", "c"}, nil, map[int]string{1: "a", 3: "c"}},
		{"one fewer without specials", []int{1, 2, 3}, stremio.Posters{"a", "b"}, nil, map[int]string{}},
		{"a season the videos lack", []int{1, 2}, stremio.Posters{"a", "b", "c"}, nil, map[int]string{}},
		{"two fewer with specials", []int{0, 1, 2, 3}, stremio.Posters{"a", "b"}, nil, map[int]string{}},
		{"none", []int{1, 2}, nil, nil, map[int]string{}},
		{"by number first", []int{1, 2}, stremio.Posters{"a", "b"}, map[string]string{"2": "x", "3": "y"}, map[int]string{1: "a", 2: "x", 3: "y"}},
	} {
		if got := seasonPosters(show(tc.seasons, tc.posters, tc.byNumber)); !maps.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
	if got := seasonPosters(stremio.Meta{Videos: []stremio.Video{{Season: 1}}}); len(got) != 0 {
		t.Errorf("without extras: %v", got)
	}
}

// Seasons show their own poster where the addon gives one, their series'
// otherwise; so do their episodes as parents, and the image route, from
// what Polyfin already knows, without asking the addon again.
func TestSeasonsShowTheirOwnPosters(t *testing.T) {
	e := newEnv(t)
	const series = "https://images.example/show.jpg"
	show := stremio.Meta{ID: "tt100", Type: "series", Name: "Show", Poster: series,
		Extras: &stremio.Extras{SeasonPosters: stremio.Posters{"https://images.example/s1.jpg", "", "https://images.example/s3.jpg"}},
		Videos: []stremio.Video{
			{ID: "tt100:0:1", Name: "Special", Season: 0, Episode: 1, Released: "2019-12-01T00:00:00Z"},
			{ID: "tt100:1:1", Name: "Pilot", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00Z"},
			{ID: "tt100:2:1", Name: "Second", Season: 2, Episode: 1, Released: "2021-01-01T00:00:00Z"},
			{ID: "tt100:3:1", Name: "Third", Season: 3, Episode: 1, Released: "2022-01-01T00:00:00Z"},
		}}
	preview := show
	preview.Videos, preview.Extras = nil, nil
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Types: []string{"series"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", Types: []string{"series"}, IDPrefixes: []string{"tt"}}},
			Catalogs:  []stremio.Catalog{{Type: "series", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"series/top": {preview}},
		metas:    map[string]stremio.Meta{"series/tt100": show},
	}
	e.install(addons.Shared(), addon)
	page, _ := e.service.Children(t.Context(), e.member, e.library(e.member, "Top").ID, 0, 10, "")
	seasons, err := e.service.Seasons(t.Context(), e.member, page.Items[0].ID)
	if err != nil || len(seasons) != 4 {
		t.Fatalf("seasons: %+v %v", seasons, err)
	}
	want := []string{series, "https://images.example/s1.jpg", series, "https://images.example/s3.jpg"}
	for i, season := range seasons {
		if season.Images.Primary != want[i] || season.SeriesPoster != series {
			t.Errorf("season %d: poster %q, series poster %q", season.IndexNumber, season.Images.Primary, season.SeriesPoster)
		}
	}
	episodes, err := e.service.Episodes(t.Context(), e.member, page.Items[0].ID, nil)
	if err != nil || len(episodes) != 4 {
		t.Fatalf("episodes: %+v %v", episodes, err)
	}
	for i, episode := range episodes {
		if own := want[i]; (own == series) != (episode.SeasonPoster == "") || own != series && episode.SeasonPoster != own {
			t.Errorf("episode of season %d: season poster %q", episode.ParentIndexNumber, episode.SeasonPoster)
		}
	}
	metaRequests := func() int {
		addon.mu.Lock()
		defer addon.mu.Unlock()
		count := 0
		for _, request := range addon.requests {
			if strings.HasPrefix(request, "meta/") {
				count++
			}
		}
		return count
	}
	asked := metaRequests()
	artwork := func(what string) {
		t.Helper()
		for i, season := range seasons {
			if url, _, err := e.service.Artwork(t.Context(), season.ID, "Primary"); err != nil || url != want[i] {
				t.Errorf("%s, season %d: %q %v", what, season.IndexNumber, url, err)
			}
		}
	}
	artwork("images")
	// Once the series' metadata is no longer cached, as after a restart,
	// the season keeps the image apps were given its tag for.
	e.service.metas = cache.New[metaKey, stremio.Meta](4000, metaTTL)
	artwork("images after the cache")
	if metaRequests() != asked {
		t.Errorf("images asked the addon for metadata: %d requests, %d before", metaRequests(), asked)
	}
}
