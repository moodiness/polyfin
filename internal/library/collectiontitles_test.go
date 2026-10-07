package library

import (
	"fmt"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// nestedAddon serves a collection library, Genres, whose collection
// Animation groups two collections, Movies and Series, each grouping a
// catalog of three titles: an addon's genre collections are often made so,
// as are a franchise's sagas. Movies came out in 2010, 2012 and 2014,
// series in 2009, 2011 and 2013.
func nestedAddon() *fakeAddon {
	movies, series := titles("movie", 3), titles("series", 3)
	for i := range 3 {
		movies[i].Released = fmt.Sprintf("%d-06-01T00:00:00.000Z", 2010+2*i)
		series[i].ID = fmt.Sprintf("tts%03d", i)
		series[i].Released = fmt.Sprintf("%d-06-01T00:00:00.000Z", 2009+2*i)
	}
	collection := func(id, name string, items []stremio.Meta, sources ...stremio.CollectionSource) stremio.Meta {
		return stremio.Meta{ID: id, Type: "collection", Name: name, Collection: &stremio.Collection{Items: items, Sources: sources}}
	}
	return &fakeAddon{
		manifest: stremio.Manifest{ID: "nested", Name: "Nested", Version: "1", Types: []string{"movie", "series", "collection"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs: []stremio.Catalog{
				{Type: "collection", ID: "genres", Name: "Genres"},
				{Type: "movie", ID: "top", Name: "Top"},
				{Type: "series", ID: "shows", Name: "Shows"},
			}},
		catalogs: map[string][]stremio.Meta{
			"collection/genres": {{ID: "col:anim", Type: "collection", Name: "Animation"}},
			"movie/top":         movies,
			"series/shows":      series,
		},
		metas: map[string]stremio.Meta{
			"collection/col:anim": collection("col:anim", "Animation", []stremio.Meta{
				{ID: "col:anim:movies", Name: "Movies"}, {ID: "col:anim:series", Name: "Series"}}),
			"collection/col:anim:movies": collection("col:anim:movies", "Movies", nil, stremio.CollectionSource{Type: "movie", CatalogID: "top"}),
			"collection/col:anim:series": collection("col:anim:series", "Series", nil, stremio.CollectionSource{Type: "series", CatalogID: "shows"}),
		},
	}
}

// A collection that groups other collections lists their titles, never
// those collections: whole, by release date, as Jellyfin orders a
// collection; in pages, one title of each catalog in turn.
func TestCollectionsOfCollectionsListTheirTitles(t *testing.T) {
	e := newEnv(t)
	e.install(addons.Shared(), nestedAddon())
	genres := e.children(e.member, "Genres", 0, 10)
	if !slices.Equal(names(genres.Items), []string{"Animation"}) {
		t.Fatalf("the genres: %v", names(genres.Items))
	}
	animation := genres.Items[0].ID
	whole, err := e.service.Children(t.Context(), e.member, animation, 0, 10, "")
	want := []string{"series 0", "movie 0", "series 1", "movie 1", "series 2", "movie 2"}
	if err != nil || !slices.Equal(names(whole.Items), want) || whole.Total != 6 || whole.More {
		t.Errorf("Animation whole: %v, total %d, more %v, %v; want %v", names(whole.Items), whole.Total, whole.More, err, want)
	}
	merged := []string{"movie 0", "series 0", "movie 1", "series 1", "movie 2", "series 2"}
	for _, start := range []int{0, 4} {
		page, err := e.service.Children(t.Context(), e.member, animation, start, 4, "")
		if want := merged[start:min(start+4, 6)]; err != nil || !slices.Equal(names(page.Items), want) || page.Total != 6 {
			t.Errorf("Animation from %d in pages of 4: %v, total %d, %v; want %v", start, names(page.Items), page.Total, err, want)
		}
	}
}
