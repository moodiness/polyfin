package localfiles

import (
	"maps"
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
