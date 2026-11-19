package jellyfin

import (
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A user's own addon moves an episode to one of its series only from a
// series that user's own addons hold: not from a series no one holds,
// such as one no addon of the server lists, nor from another user's. One
// of the server's addons still moves an episode from any series, and a
// user's own addons move their own episodes between their series.
func TestAUsersOwnAddonMovesOnlyItsOwnersEpisodes(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	video := func(id string) stremio.Video {
		return stremio.Video{ID: id, Title: "Episode " + id, Season: 1, Episode: 1, Released: "2020-01-01T00:00:00.000Z"}
	}
	named := func(id, name string, videos ...stremio.Video) stremio.Meta {
		return stremio.Meta{ID: id, Type: "series", Name: name, Videos: videos}
	}
	add := func(scope addons.Scope, manifest string) {
		t.Helper()
		installed, err := s.addons.Install(t.Context(), scope, manifest, false)
		if err != nil {
			t.Fatal(err)
		}
		choices := []addons.LibraryChoice{{AddonID: installed.ID, CatalogType: "series", CatalogID: "shows"}}
		before, err := s.addons.Libraries(t.Context(), scope)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range before {
			if l.Enabled && l.AddonID != installed.ID {
				choices = append(choices, addons.LibraryChoice{AddonID: l.AddonID, CatalogType: l.Catalog.Type, CatalogID: l.Catalog.ID})
			}
		}
		if _, err := s.addons.SetLibraries(t.Context(), scope, choices); err != nil {
			t.Fatal(err)
		}
	}
	// list lists the episodes of the series named so in a library, as the
	// user of token, which records them.
	list := func(token, view, series string) {
		t.Helper()
		var page, listed QueryResult
		if status := s.get(t, "/Items?ParentId="+s.viewIDs(t, token)[view], token, &page); status != http.StatusOK {
			t.Fatalf("the library %s: %d", view, status)
		}
		for _, item := range page.Items {
			if item.Name == series {
				if status := s.get(t, "/Shows/"+item.Id+"/Episodes", token, &listed); status != http.StatusOK || len(listed.Items) == 0 {
					t.Fatalf("the episodes of %s: %d %v", series, status, itemNames(listed.Items))
				}
				return
			}
		}
		t.Fatalf("the library %s lists no %s: %v", view, series, itemNames(page.Items))
	}
	// held tells the series the record of the episode of video names.
	held := func(video string) string {
		t.Helper()
		var series string
		if err := s.pool.QueryRow(t.Context(), "SELECT data->>'seriesId' FROM items WHERE id = $1",
			library.EpisodeItemID(video)).Scan(&series); err != nil {
			t.Fatal(err)
		}
		return series
	}
	admin := func(c *accounts.UserChanges) { c.IsAdministrator = new(true) }

	add(addons.Shared(), manySeriesAddon(t, "shared.shows", "Server Shows", []string{"tt"}, named("tt7000", "The Server's Show", video("tt7000:1:1"))))
	s.user("admin", admin)
	s.user("other", nil)
	other := s.signIn("other", "tv")
	list(other, "Server Shows", "The Server's Show")
	// The series and its episode are now held by no addon, as an import
	// records those of a series no addon of the server lists.
	if _, err := s.pool.Exec(t.Context(), "UPDATE items SET data = data - 'addon' WHERE key IN ('series|tt7000', 'episode|tt7000:1:1')"); err != nil {
		t.Fatal(err)
	}
	// Another user's own series holds an episode of their own.
	third := s.user("third", admin)
	add(addons.Personal(third.ID), manySeriesAddon(t, "third.shows", "Theirs", []string{"tt"}, named("tt7100", "Their Show", video("tt7100:1:1"))))
	theirs := s.signIn("third", "phone")
	list(theirs, "Theirs", "Their Show")

	// A user's own series names both episodes.
	first := s.user("first", admin)
	add(addons.Personal(first.ID), manySeriesAddon(t, "own.shows", "Mine", []string{"own:"},
		named("own:show", "Mine", video("tt7000:1:1"), video("tt7100:1:1"))))
	list(s.signIn("first", "phone"), "Mine", "Mine")
	if got := held("tt7000:1:1"); got != "tt7000" {
		t.Errorf("the episode of a series no one holds, once a user's own series named it: of %s", got)
	}
	if got := held("tt7100:1:1"); got != "tt7100" {
		t.Errorf("another user's own episode, once a user's own series named it: of %s", got)
	}

	// Their own other addon moves their episode between their series.
	add(addons.Personal(third.ID), manySeriesAddon(t, "third.more", "Theirs Too", []string{"more:"},
		named("more:show", "Their Other Show", video("tt7100:1:1"))))
	list(theirs, "Theirs Too", "Their Other Show")
	if got := held("tt7100:1:1"); got != "more:show" {
		t.Errorf("a user's own episode, once their other series named it: of %s", got)
	}
	// One of the server's addons moves it from any series.
	add(addons.Shared(), manySeriesAddon(t, "shared.more", "More Shows", []string{"tt"}, named("tt9000", "A Third Show", video("tt7100:1:1"))))
	list(other, "More Shows", "A Third Show")
	if got := held("tt7100:1:1"); got != "tt9000" {
		t.Errorf("a user's own episode, once a series of the server named it: of %s", got)
	}
}
