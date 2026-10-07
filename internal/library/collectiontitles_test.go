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
// catalog of three titles: an addon's genre collections are often made so.
func nestedAddon() *fakeAddon {
	series := titles("series", 3)
	for i := range series {
		series[i].ID = fmt.Sprintf("tts%03d", i)
	}
	collection := func(id, name string, items []stremio.Meta, sources ...stremio.CollectionSource) stremio.Meta {
		return stremio.Meta{ID: id, Type: "collection", Name: name, Collection: &stremio.Collection{Items: items, Sources: sources}}
	}
	return &fakeAddon{
		manifest: stremio.Manifest{ID: "nested", Name: "Nested", Version: "1", Types: []string{"movie", "series", "collection"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs: []stremio.Catalog{
				{Type: "collection", ID: "genres", Name: "Genres"},
				{Type: "movie", ID: "films", Name: "Films"},
				{Type: "series", ID: "shows", Name: "Shows"},
			}},
		catalogs: map[string][]stremio.Meta{
			"collection/genres": {{ID: "col:anim", Type: "collection", Name: "Animation"}},
			"movie/films":       titles("movie", 3),
			"series/shows":      series,
		},
		metas: map[string]stremio.Meta{
			"collection/col:anim": collection("col:anim", "Animation", []stremio.Meta{
				{ID: "col:anim:movies", Name: "Movies"}, {ID: "col:anim:series", Name: "Series"}}),
			"collection/col:anim:movies": collection("col:anim:movies", "Movies", nil, stremio.CollectionSource{Type: "movie", CatalogID: "films"}),
			"collection/col:anim:series": collection("col:anim:series", "Series", nil, stremio.CollectionSource{Type: "series", CatalogID: "shows"}),
		},
	}
}

// A listing of a collection's titles, as Streamyfin asks for a collection's
// movies and series, lists the titles of the catalogs that the collections
// within it group, merged; its children are still those collections.
func TestTitlesGoThroughNestedCollections(t *testing.T) {
	e := newEnv(t)
	e.install(addons.Shared(), nestedAddon())
	genres := e.children(e.member, "Genres", 0, 10)
	if len(genres.Items) != 1 {
		t.Fatalf("the genres: %v", names(genres.Items))
	}
	animation := genres.Items[0].ID
	children, err := e.service.Children(t.Context(), e.member, animation, 0, 10, "")
	if err != nil || !slices.Equal(names(children.Items), []string{"Movies", "Series"}) {
		t.Errorf("Animation's children: %v %v", names(children.Items), err)
	}
	listed, err := e.service.Titles(t.Context(), e.member, animation, 0, 10, "")
	want := []string{"movie 0", "series 0", "movie 1", "series 1", "movie 2", "series 2"}
	if err != nil || !slices.Equal(names(listed.Items), want) || listed.Total != 6 || listed.More {
		t.Errorf("Animation's titles: %v, total %d, more %v, %v; want %v", names(listed.Items), listed.Total, listed.More, err, want)
	}
	if page, err := e.service.Titles(t.Context(), e.member, animation, 4, 10, ""); err != nil || !slices.Equal(names(page.Items), want[4:]) {
		t.Errorf("from the fifth title: %v %v", names(page.Items), err)
	}
	// Any other folder lists its children.
	if page, err := e.service.Titles(t.Context(), e.member, e.library(e.member, "Genres").ID, 0, 10, ""); err != nil || !slices.Equal(names(page.Items), []string{"Animation"}) {
		t.Errorf("the library's titles: %v %v", names(page.Items), err)
	}
}
