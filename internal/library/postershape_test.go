package library

import (
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A poster's shape goes with the poster: an item takes the one its addon
// declares, an unknown one counting as a 2:3 poster; a translated or
// enriching description's poster brings its own shape, and a local or
// generated poster in its place has none.
func TestPosterShapeFollowsThePoster(t *testing.T) {
	t.Parallel()
	shapeOf := func(meta stremio.Meta) string {
		var item Item
		fromMeta(&item, meta)
		return item.PosterShape
	}
	for shape, want := range map[string]string{"landscape": stremio.ShapeLandscape, "square": stremio.ShapeSquare, "poster": "", "": "", "round": ""} {
		if got := shapeOf(stremio.Meta{Poster: "p.jpg", PosterShape: shape}); got != want {
			t.Errorf("shape %q: %q, want %q", shape, got, want)
		}
	}

	wide := Item{Images: Images{Primary: "wide.jpg"}, PosterShape: stremio.ShapeLandscape}
	translated := wide
	translateItem(&translated, stremio.Meta{Name: "Translated", Poster: "translated.jpg"})
	if translated.Images.Primary != "translated.jpg" || translated.PosterShape != "" {
		t.Errorf("a translated poster without a shape: %+v", translated)
	}
	translated = wide
	translateItem(&translated, stremio.Meta{Name: "Translated", Poster: "square.jpg", PosterShape: "square"})
	if translated.PosterShape != stremio.ShapeSquare {
		t.Errorf("a translated square poster: %q", translated.PosterShape)
	}
	translated = wide
	translateItem(&translated, stremio.Meta{Name: "Translated"})
	if translated.Images.Primary != "wide.jpg" || translated.PosterShape != stremio.ShapeLandscape {
		t.Errorf("a translation without a poster: %+v", translated)
	}

	own := stremio.Meta{Poster: "own.jpg", PosterShape: "landscape"}
	if got := enriched(own, stremio.Meta{Poster: "addon.jpg"}); got.Poster != "addon.jpg" || got.Shape() != "" {
		t.Errorf("enriched with a poster: %+v", got)
	}
	if got := enriched(own, stremio.Meta{}); got.Poster != "own.jpg" || got.Shape() != stremio.ShapeLandscape {
		t.Errorf("enriched without a poster: %+v", got)
	}

	id := accounts.ID{1}
	local := wide
	local.ID = id
	applyLocal(&local, map[accounts.ID]Images{id: {Backdrop: "backdrop.jpg"}})
	if local.PosterShape != stremio.ShapeLandscape {
		t.Errorf("a local backdrop dropped the shape: %+v", local)
	}
	applyLocal(&local, map[accounts.ID]Images{id: {Primary: "local.jpg"}})
	if local.Images.Primary != "local.jpg" || local.PosterShape != "" {
		t.Errorf("a local poster: %+v", local)
	}
}
