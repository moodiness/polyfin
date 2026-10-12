package library

import (
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A local image replaces the addon's of its type, the others staying; a
// season without a poster of its own shows its series' local one, but
// keeps the addon's own poster unless it has a local one; episodes show
// their series' local backdrop and poster and their season's local poster.
func TestLocalArtworkApplies(t *testing.T) {
	t.Parallel()
	series, season, bare := TitleItemID(KindSeries, "tt1"), SeasonItemID("tt1", 1), SeasonItemID("tt1", 2)
	movie := TitleItemID(KindMovie, "tt2")
	local := map[accounts.ID]Images{
		movie:  {Primary: "local:movie", Disc: "local:disc"},
		series: {Primary: "local:series", Backdrop: "local:backdrop"},
		season: {Primary: "local:season"},
	}
	for _, c := range []struct {
		name       string
		item, want Item
	}{
		{"movie",
			Item{ID: movie, Kind: KindMovie, Images: Images{Primary: "addon:poster", Logo: "addon:logo"}},
			Item{ID: movie, Kind: KindMovie, Images: Images{Primary: "local:movie", Logo: "addon:logo", Disc: "local:disc"}}},
		{"season with a local poster",
			Item{ID: season, Kind: KindSeason, SeriesID: series, SeriesPoster: "addon:series", Images: Images{Primary: "addon:season"}},
			Item{ID: season, Kind: KindSeason, SeriesID: series, SeriesPoster: "local:series",
				Images: Images{Primary: "local:season", Backdrop: "local:backdrop"}}},
		{"season showing its series' poster",
			Item{ID: bare, Kind: KindSeason, SeriesID: series, SeriesPoster: "addon:series", Images: Images{Primary: "addon:series"}},
			Item{ID: bare, Kind: KindSeason, SeriesID: series, SeriesPoster: "local:series",
				Images: Images{Primary: "local:series", Backdrop: "local:backdrop"}}},
		{"season with an addon's poster",
			Item{ID: bare, Kind: KindSeason, SeriesID: series, SeriesPoster: "addon:series", Images: Images{Primary: "addon:season"}},
			Item{ID: bare, Kind: KindSeason, SeriesID: series, SeriesPoster: "local:series",
				Images: Images{Primary: "addon:season", Backdrop: "local:backdrop"}}},
		{"episode",
			Item{Kind: KindEpisode, SeriesID: series, SeasonID: season, SeriesPoster: "addon:series", SeasonPoster: "addon:season",
				Images: Images{Primary: "addon:still", Backdrop: "addon:backdrop"}},
			Item{Kind: KindEpisode, SeriesID: series, SeasonID: season, SeriesPoster: "local:series", SeasonPoster: "local:season",
				Images: Images{Primary: "addon:still", Backdrop: "local:backdrop"}}},
	} {
		item := c.item
		applyLocal(&item, local)
		if item.Images != c.want.Images || item.SeriesPoster != c.want.SeriesPoster || item.SeasonPoster != c.want.SeasonPoster {
			t.Errorf("%s: %+v", c.name, item)
		}
	}
}

// A local image's URL stands for its key, whatever its version.
func TestLocalArtworkKey(t *testing.T) {
	t.Parallel()
	url := LocalArtworkURL("0123.jpg", "a.b")
	if key, ok := LocalArtworkKey(url); !ok || key != "0123.jpg" || ImageTag(url) == ImageTag(LocalArtworkURL("0123.jpg", "a.c")) {
		t.Errorf("%q: %q %v", url, key, ok)
	}
	if _, ok := LocalArtworkKey("https://images.example/poster.jpg"); ok {
		t.Error("an addon's image is a local one")
	}
}
