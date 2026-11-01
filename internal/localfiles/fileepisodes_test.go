package localfiles

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// shelfSeries lists the series of the folder named Shelf as user sees
// them, by name.
func (e env) shelfSeries(user accounts.User) map[string]library.Item {
	e.t.Helper()
	libraries, err := e.library.Libraries(e.t.Context(), user)
	if err != nil {
		e.t.Fatal(err)
	}
	series := map[string]library.Item{}
	for _, l := range libraries {
		if l.Name != "Shelf" {
			continue
		}
		page, err := e.library.Children(e.t.Context(), user, l.ID, 0, 100, "")
		if err != nil {
			e.t.Fatal(err)
		}
		for _, item := range page.Items {
			series[item.Name] = item
		}
	}
	return series
}

// numbered names episodes by their season and number.
func numbered(episodes []library.Item) map[[2]int]string {
	names := map[[2]int]string{}
	for _, episode := range episodes {
		names[[2]int{episode.ParentIndexNumber, episode.IndexNumber}] = episode.Name
	}
	return names
}

// A show its metadata addon describes without an episode list lists the
// episodes its files hold, named after their files, as Jellyfin makes an
// episode of every file; the folder counts them as episodes the addon
// does not list, once the show is described.
func TestShowsListTheEpisodesTheirFilesHold(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	admin, err := e.users.CreateUser(ctx, accounts.NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true})
	if err != nil {
		t.Fatal(err)
	}
	addon, _ := e.folder(KindShows, map[string]int{
		"The Lone Ranger (1949)/Season 01/The Lone Ranger S01E01.mkv":     10,
		"The Lone Ranger (1949)/Season 01/The Lone Ranger S01E02-E03.mkv": 10,
		"The Lone Ranger (1949)/Season 02/The Lone Ranger S02E01.mkv":     10,
	})
	if got := e.files(addon.ID)["The Lone Ranger (1949)/Season 01/The Lone Ranger S01E01.mkv"]; got != "tt0041038" {
		t.Fatalf("the show's match: %s", got)
	}
	show, ok := e.shelfSeries(admin)["The Lone Ranger"]
	if !ok {
		t.Fatalf("the folder's series: %v", e.shelfSeries(admin))
	}
	episodes, err := e.library.Episodes(ctx, admin, show.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[[2]int]string{{1, 1}: "The Lone Ranger S01E01", {1, 2}: "The Lone Ranger S01E02-E03", {1, 3}: "The Lone Ranger S01E02-E03",
		{2, 1}: "The Lone Ranger S02E01"}
	if got := numbered(episodes); len(got) != len(want) || !maps.Equal(got, want) {
		t.Errorf("the show's episodes: %v, want %v", got, want)
	}
	seasons, err := e.library.Seasons(ctx, admin, show.ID)
	if err != nil || len(seasons) != 2 {
		t.Errorf("the show's seasons: %d %v", len(seasons), err)
	}
	// Each plays the folder's file.
	for _, episode := range episodes {
		streams, err := e.service.Streams(ctx, addon.ID, episode.StremioID)
		if err != nil || len(streams) != 1 {
			t.Errorf("the streams of %s: %+v %v", episode.Name, streams, err)
		}
	}
	folder, err := e.service.Folder(ctx, addon.ID)
	if err != nil || folder.Unlisted != 4 {
		t.Errorf("the episodes the addon does not list: %d %v", folder.Unlisted, err)
	}
}

// A show linked to a series whose metadata addon lists fewer episodes
// than its files hold is listed with the files' numbers next to the
// addon's, once the series is described: those numbered after the
// addon's last episode of a season beside the season's numbers, a season
// the addon has not beside nothing but its range, specials left out. The
// folder still counts them.
func TestUnlistedEpisodesShowBothNumberings(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	admin, err := e.users.CreateUser(ctx, accounts.NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true})
	if err != nil {
		t.Fatal(err)
	}
	addon, _ := e.folder(KindShows, map[string]int{
		"Cisco Kid Files/Season 01/Cisco Kid S01E01.mkv": 10,
		"Cisco Kid Files/Season 01/Cisco Kid S01E04.mkv": 10,
		"Cisco Kid Files/Season 01/Cisco Kid S01E05.mkv": 10,
		"Cisco Kid Files/Season 03/Cisco Kid S03E01.mkv": 10,
	})
	if err := e.service.Link(ctx, addon.ID, "Cisco Kid Files", "tt0042084"); err != nil {
		t.Fatal(err)
	}
	show, ok := e.shelfSeries(admin)["The Cisco Kid"]
	if !ok {
		t.Fatalf("the folder's series: %v", e.shelfSeries(admin))
	}
	if _, err := e.library.Episodes(ctx, admin, show.ID, nil); err != nil {
		t.Fatal(err)
	}
	shows, err := e.service.Unlisted(ctx, addon.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []UnlistedShow{{StremioID: "tt0042084", Name: "The Cisco Kid", Unit: "Cisco Kid Files", Described: true,
		First: &[2]int{1, 1}, Last: &[2]int{2, 2}, Seasons: []UnlistedSeason{
			{Season: 1, Files: []Range{{4, 5}}, Listed: []Range{{1, 3}}},
			{Season: 3, Files: []Range{{1, 1}}},
		}}}
	if !reflect.DeepEqual(shows, want) {
		t.Errorf("the shows with episodes only in the files:\n%s\nwant\n%s", printed(shows), printed(want))
	}
	folder, err := e.service.Folder(ctx, addon.ID)
	if err != nil || folder.Unlisted != 3 {
		t.Errorf("the episodes the addon does not list: %d %v", folder.Unlisted, err)
	}
}

// printed is a value as JSON, its pointers followed.
func printed(v any) string {
	text, _ := json.Marshal(v)
	return string(text)
}

// The files of a matched show whose names tell no episode are unmatched
// with why: an episode's number outside a season's folder, an air date,
// or no number at all. Names telling an absolute number are read, never
// unmatched (see absolute.go).
func TestUnreadEpisodeNamesTellWhy(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	addon, _ := e.folder(KindShows, map[string]int{
		"Cisco Kid Files/Season 01/Cisco Kid S01E01.mkv": 10,
		"Cisco Kid Files/Season 02/Cisco Kid - 14.mkv":   10,
		"Cisco Kid Files/Cisco Kid 15 [720p].mkv":        10,
		"Cisco Kid Files/Cisco Kid E07.mkv":              10,
		"Cisco Kid Files/Cisco Kid 1951-03-04.mkv":       10,
		"Cisco Kid Files/Season 01/Cisco Kid Finale.mkv": 10,
	})
	if err := e.service.Link(ctx, addon.ID, "Cisco Kid Files", "tt0042084"); err != nil {
		t.Fatal(err)
	}
	files, _, err := e.service.Unmatched(ctx, addon.ID)
	if err != nil {
		t.Fatal(err)
	}
	reasons := map[string]string{}
	for _, file := range files {
		reasons[file.Path] = fmt.Sprintf("%s %d", file.Reason, 0)
	}
	want := map[string]string{
		"Cisco Kid Files/Cisco Kid E07.mkv":              "no_season 0",
		"Cisco Kid Files/Cisco Kid 1951-03-04.mkv":       "air_date 0",
		"Cisco Kid Files/Season 01/Cisco Kid Finale.mkv": "no_episode_number 0",
	}
	if !maps.Equal(reasons, want) {
		t.Errorf("the unmatched files: %v, want %v", reasons, want)
	}
}

// A link to an identifier no metadata addon of the server describes as
// the folder's kind, such as an episode's typed for its series', is
// flagged; one to an identifier an addon describes is not.
func TestLinksTellWhetherAnAddonDescribesTheirTitle(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	addon, _ := e.folder(KindShows, map[string]int{
		"Harbor Coast/Season 01/Harbor Coast S01E01.mkv": 10,
		"Gull Point/Season 01/Gull Point S01E01.mkv":     10,
	})
	if err := e.service.Link(ctx, addon.ID, "Harbor Coast/Season 01/Harbor Coast S01E01.mkv", "tt9999999"); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Link(ctx, addon.ID, "Gull Point", "tt0041038"); err != nil {
		t.Fatal(err)
	}
	folder, err := e.service.Folder(ctx, addon.ID)
	if err != nil {
		t.Fatal(err)
	}
	described := map[string]bool{}
	for _, link := range folder.Links {
		described[link.Unit] = link.Described
	}
	if !maps.Equal(described, map[string]bool{"Harbor Coast": false, "Gull Point": true}) {
		t.Errorf("the links: %+v", folder.Links)
	}
}
