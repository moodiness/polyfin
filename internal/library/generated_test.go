package library

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// numbered gives the candidates of numbers, each with a poster.
func numbered(numbers ...int) []candidate {
	result := make([]candidate, 0, len(numbers))
	for _, n := range numbers {
		result = append(result, candidate{id: itemID(fmt.Sprint("title ", n)), poster: fmt.Sprint("https://posters.test/", n, ".jpg")})
	}
	return result
}

// The titles an image shows stay while the name stays and fewer than half
// of them, as a poster or as a wide image, left the first titles; past
// that, or renamed, the image shows the first titles again. Titles without
// a poster are skipped, and a choice that shows fewer than it could takes
// those that came since.
func TestGeneratedImagesKeepTheirTitlesUntilHalfAreGone(t *testing.T) {
	t.Parallel()
	first, changed := choose(generatedChoice{}, false, "Saga", numbered(1, 2, 3, 4, 5, 6, 7, 8), true, false)
	if !changed || len(first.titles) != 6 || first.posters[0] != "https://posters.test/1.jpg" {
		t.Fatalf("first choice: %+v", first)
	}
	ids := func(c generatedChoice) []accounts.ID { return c.titles }
	for name, c := range map[string]struct {
		candidates []candidate
		known      bool
		name       string
		changed    bool
	}{
		"the same titles":                       {numbered(1, 2, 3, 4, 5, 6, 7, 8), true, "Saga", false},
		"reordered":                             {numbered(8, 7, 6, 5, 4, 3, 2, 1), true, "Saga", false},
		"one of the poster's gone":              {numbered(1, 3, 4, 5, 6, 7, 8), true, "Saga", false},
		"half of the poster's gone":             {numbered(1, 2, 5, 6, 7, 8, 9, 10), true, "Saga", true},
		"two of the wide image's gone":          {numbered(1, 2, 3, 4, 7, 8, 9, 10), true, "Saga", false},
		"half of the wide image's gone":         {numbered(1, 2, 3, 7, 8, 9, 10, 11), true, "Saga", true},
		"renamed":                               {numbered(1, 2, 3, 4, 5, 6), true, "Saga II", true},
		"unknown titles":                        {nil, false, "Saga", false},
		"titles without posters":                {[]candidate{{id: itemID("title 9")}}, true, "Saga", false},
		"only three left":                       {numbered(1, 2, 3), true, "Saga", true},
		"among the first twenty, past the six":  {numbered(9, 10, 11, 12, 13, 14, 1, 2, 3, 4, 5, 6), true, "Saga", false},
		"four of the six gone, two of the four": {numbered(1, 2, 7, 8, 9, 10), true, "Saga", true},
	} {
		got, changed := choose(first, true, c.name, c.candidates, c.known, false)
		if changed != c.changed || !changed && !slices.Equal(ids(got), ids(first)) || got.name != c.name {
			t.Errorf("%s: changed %t, %+v", name, changed, got)
		}
	}
	// Three of the wide image's gone, one of the poster's, is half.
	if _, changed := choose(first, true, "Saga", numbered(1, 3, 4, 9, 10, 11), true, false); !changed {
		t.Error("half of the wide image gone: kept")
	}
	// Unknown titles keep what is shown under a new name.
	if got, changed := choose(first, true, "Saga II", nil, false, false); !changed || got.name != "Saga II" || !slices.Equal(ids(got), ids(first)) {
		t.Errorf("renamed without titles: %t %+v", changed, got)
	}
	// A choice of two takes the four more that came.
	small, _ := choose(generatedChoice{}, false, "Saga", numbered(1, 2), true, false)
	if grown, changed := choose(small, true, "Saga", numbered(1, 2, 3, 4, 5, 6), true, false); !changed || len(grown.titles) != 6 {
		t.Errorf("a choice of two: %t %+v", changed, grown)
	}
	// Nothing to show gives no image.
	if _, changed := choose(generatedChoice{}, false, "Saga", []candidate{{id: itemID("title 1")}}, true, false); changed {
		t.Error("an image without posters")
	}
}

// A generated image's URL, and so its tag, changes with what it shows.
func TestGeneratedImageURLs(t *testing.T) {
	t.Parallel()
	image := GeneratedImage{Wide: true, Name: "Été & co", Posters: []string{"https://a.test/1.jpg?x=1&y=2", "https://a.test/2.jpg"}, Confined: true}
	tags := map[string]bool{ImageTag(image.URL()): true}
	for _, change := range []func(*GeneratedImage){
		func(g *GeneratedImage) { g.Wide = false },
		func(g *GeneratedImage) { g.Name = "Été" },
		func(g *GeneratedImage) { g.Posters = g.Posters[:1] },
		func(g *GeneratedImage) { g.Confined = false },
	} {
		changed := image
		change(&changed)
		tags[ImageTag(changed.URL())] = true
	}
	if len(tags) != 5 {
		t.Errorf("tags shared by different images: %v", tags)
	}
}

// Artwork in the scheme of generated images from an addon is no image:
// dropped from the pages addons give, and refused when kept from before,
// it is never drawn as a generated image.
func TestAddonsArtworkIsNeverAGeneratedImage(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	generated := GeneratedImage{Name: "x", Posters: []string{"http://127.0.0.1:9/secret.png"}}.URL()
	search := []stremio.Extra{{Name: "search", IsRequired: true}}
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "shared", Name: "Shared", Version: "1", Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{{Type: "movie", ID: "search", Name: "Search", Extra: search}}},
		catalogs: map[string][]stremio.Meta{"movie/search": {{ID: "tt-shared", Type: "movie", Name: "movie shared", Poster: generated,
			Background: generated, Videos: []stremio.Video{{ID: "tt-shared:1", Thumbnail: generated}}}}},
	})
	server := httptest.NewServer(&fakeAddon{manifest: stremio.Manifest{ID: "own", Name: "Own", Version: "1",
		Resources: []stremio.Resource{{Name: "catalog"}}, Catalogs: []stremio.Catalog{{Type: "movie", ID: "mine", Name: "Mine", Extra: search}}}})
	t.Cleanup(server.Close)
	own, err := e.addons.Install(t.Context(), addons.Personal(e.member.ID), server.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	// The member's page is kept as an addon gave it before such artwork
	// was dropped.
	mine := stremio.Meta{ID: "tt-own", Type: "movie", Name: "movie own", Poster: generated}
	e.service.searchPages.Put(pageKey{own.ID, "movie", "mine", "", "movie", "", 0}, fetched[[]stremio.Meta]{[]stremio.Meta{mine}, time.Now()})
	found, err := e.service.Search(t.Context(), e.member, "movie", []Kind{KindMovie}, 10)
	if got := names(found); err != nil || !slices.Equal(got, []string{"movie shared", "movie own"}) {
		t.Fatalf("search: %v %v", got, err)
	}
	if images := found[0].Images; images.Primary != "" || images.Backdrop != "" {
		t.Errorf("an addon's artwork in the scheme of generated images is listed: %+v", images)
	}
	for _, item := range found {
		if art, err := e.service.Artwork(t.Context(), item.ID, "Primary"); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: %+v %v", item.Name, art, err)
		}
	}
}
