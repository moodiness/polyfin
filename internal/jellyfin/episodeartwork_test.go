package jellyfin

import (
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// The seasons and episodes of a series are recorded from its description.
// A member's own addon gives them thumbnails and posters on a local
// address, for a series of its own that it describes by another
// identifier than the one asked, and for a series a server addon listed
// that no server addon describes: none is ever downloaded, with or
// without credentials. The description given by another identifier is
// recorded by the one asked, so its episodes open as their series'.
func TestEpisodeAndSeasonImagesOfAMembersAddonAreConfined(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	local := newCountingArt(t)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	owner := s.user("owner", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })

	listed := stremio.Meta{ID: "tt7100001", Type: "series", Name: "Listed", Poster: "https://images.example/listed.jpg"}
	s.withLibrary(t, addons.Shared(), libraryAddon(t, "shared", "series", []stremio.Meta{listed}, nil), "series")
	mine := stremio.Meta{ID: "own:show", Type: "series", Name: "Mine", Poster: "https://images.example/mine.jpg"}
	describe := func(meta stremio.Meta, as, name string) stremio.Meta {
		meta.Videos = []stremio.Video{{ID: meta.ID + ":1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00.000Z",
			Thumbnail: local.URL + "/" + name + "-episode.png"}}
		meta.Extras = &stremio.Extras{SeasonPosterByNumber: map[string]string{"1": local.URL + "/" + name + "-season.png"}}
		meta.ID = as
		return meta
	}
	s.withLibrary(t, addons.Personal(owner.ID), libraryAddon(t, "own", "series", []stremio.Meta{mine}, map[string]stremio.Meta{
		mine.ID:   describe(mine, "own:nowhere", "mine"),
		listed.ID: describe(listed, listed.ID, "listed"),
	}), "series")

	// browse lists each series, its seasons and its episodes, as the
	// owner, and returns their identifiers by series.
	type shown struct{ series, season, episode string }
	browse := func() (string, map[string]shown) {
		t.Helper()
		token := s.signIn("owner", "tv")
		found := map[string]shown{}
		for library, name := range map[string]string{"own library": "Mine", "shared library": "Listed"} {
			series := s.libraryItems(t, token, library)[name]
			var seasons, episodes QueryResult
			if status := s.get(t, "/Shows/"+series+"/Seasons", token, &seasons); status != http.StatusOK || len(seasons.Items) != 1 {
				t.Fatalf("the seasons of %s: %d %+v", name, status, seasons.Items)
			}
			if status := s.get(t, "/Shows/"+series+"/Episodes", token, &episodes); status != http.StatusOK || len(episodes.Items) != 1 {
				t.Fatalf("the episodes of %s: %d %+v", name, status, episodes.Items)
			}
			found[name] = shown{series, seasons.Items[0].Id, episodes.Items[0].Id}
		}
		return token, found
	}
	browse()
	s.becomesMember(t, owner)
	// The member browses again: what is recorded is their confined addon's.
	token, found := browse()

	for name, items := range found {
		for what, id := range map[string]string{"episode": items.episode, "season": items.season} {
			for _, authorization := range []string{"", app("tv", token)} {
				if status, _, _ := s.send(http.MethodGet, "/Items/"+id+"/Images/Primary", authorization, "", ""); status != http.StatusForbidden {
					t.Errorf("the %s's image of %s, with credentials %t: %d, want 403", what, name, authorization != "", status)
				}
			}
		}
		var episode BaseItemDto
		if status := s.get(t, "/Items/"+items.episode, token, &episode); status != http.StatusOK || episode.SeriesId != items.series {
			t.Errorf("the episode of %s: %d, of the series %q, want %q", name, status, episode.SeriesId, items.series)
		}
	}
	for _, path := range []string{"/mine-episode.png", "/mine-season.png", "/listed-episode.png", "/listed-season.png"} {
		if n := local.requests(path); n != 0 {
			t.Errorf("the local server was asked %s %d times", path, n)
		}
	}
}
