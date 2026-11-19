package jellyfin

import (
	"errors"
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/stremio"
)

// The tests of this file install a user's own addon while its owner is an
// administrator, as sharedrecords_test.go tells why, and make the owner a
// member before the others look.

// describedEpisodes is a server holding the series tt7000 of one of the
// server's addons, with one episode, listed by a member, and a user who
// reads their own addons only, whose own addon describes the same series
// otherwise: another name, overview, thumbnail and a runtime of days.
type describedEpisodes struct {
	testServer
	other, second accounts.User
	otherToken    string
	episode       accounts.ID
}

const (
	serverThumbnail = "https://images.example/server.jpg"
	ownThumbnail    = "https://images.example/own.jpg"
)

func newDescribedEpisodes(t *testing.T, s testServer) describedEpisodes {
	t.Helper()
	add := func(scope addons.Scope, manifest string) {
		t.Helper()
		installed, err := s.addons.Install(t.Context(), scope, manifest, false)
		if err != nil {
			t.Fatal(err)
		}
		choices := []addons.LibraryChoice{{AddonID: installed.ID, CatalogType: "series", CatalogID: "shows"}}
		if _, err := s.addons.SetLibraries(t.Context(), scope, choices); err != nil {
			t.Fatal(err)
		}
	}
	series := func(video stremio.Video) stremio.Meta {
		return stremio.Meta{ID: "tt7000", Type: "series", Name: "The Server's Show", Videos: []stremio.Video{video}}
	}
	add(addons.Shared(), manySeriesAddon(t, "shared.shows", "Server Shows", []string{"tt"},
		series(stremio.Video{ID: "tt7000:1:1", Title: "Pilot", Overview: "How it starts.", Thumbnail: serverThumbnail,
			Runtime: "45 min", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00.000Z"})))
	if _, err := s.addons.Install(t.Context(), addons.Shared(), episodeStreams(t), false); err != nil {
		t.Fatal(err)
	}
	d := describedEpisodes{testServer: s, episode: library.EpisodeItemID("tt7000:1:1")}
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	d.other = s.user("other", nil)
	d.otherToken = s.signIn("other", "tv")
	if listed := d.episodes(t, d.otherToken, "Server Shows"); len(listed) != 1 || listed[0].Name != "Pilot" {
		t.Fatalf("the server's episodes: %+v", listed)
	}
	d.second = s.user("second", func(c *accounts.UserChanges) { c.IsAdministrator, c.UseSharedAddons = new(true), new(false) })
	add(addons.Personal(d.second.ID), manySeriesAddon(t, "own.metadata", "Described", []string{"tt"},
		series(stremio.Video{ID: "tt7000:1:1", Title: "Renamed", Overview: "Told otherwise.", Thumbnail: ownThumbnail,
			Runtime: "100 h", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00.000Z"})))
	// The user reads their own description of the series.
	if own := d.episodes(t, s.signIn("second", "phone"), "Described"); len(own) != 1 || own[0].Id != d.episode.String() ||
		own[0].Name != "Renamed" || own[0].Overview == nil || *own[0].Overview != "Told otherwise." {
		t.Fatalf("the user's own description's episodes: %+v", own)
	}
	if _, err := s.store.UpdateUser(t.Context(), d.second.ID, accounts.UserChanges{IsAdministrator: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	return d
}

// episodes lists the episodes of The Server's Show in the library view,
// as the user of token.
func (d describedEpisodes) episodes(t *testing.T, token, view string) []BaseItemDto {
	t.Helper()
	var page, listed QueryResult
	if status := d.get(t, "/Items?ParentId="+d.viewIDs(t, token)[view], token, &page); status != http.StatusOK {
		t.Fatalf("the library %s: %d", view, status)
	}
	for _, item := range page.Items {
		if item.Name == "The Server's Show" {
			if status := d.get(t, "/Shows/"+item.Id+"/Episodes?Fields=Overview", token, &listed); status != http.StatusOK {
				t.Fatalf("the episodes: %d", status)
			}
			return listed.Items
		}
	}
	t.Fatalf("the library %s lists no series: %v", view, itemNames(page.Items))
	return nil
}

// stored is the episode's record: its video's name, overview, thumbnail
// and runtime.
func (d describedEpisodes) stored(t *testing.T) [4]string {
	t.Helper()
	var got [4]string
	if err := d.pool.QueryRow(t.Context(), `SELECT coalesce(data->'video'->>'title', ''), coalesce(data->'video'->>'overview', ''),
		coalesce(data->'video'->>'thumbnail', ''), coalesce(data->'video'->>'runtime', '') FROM items WHERE id = $1`,
		d.episode).Scan(&got[0], &got[1], &got[2], &got[3]); err != nil {
		t.Fatal(err)
	}
	return got
}

var serverEpisode = [4]string{"Pilot", "How it starts.", serverThumbnail, "45 min"}

// The runtime a play gives the versions of an episode is its record's,
// which decides whether a version is a stand-in, a clip too short to be
// the episode: a user's own description of one of the server's series
// leaves it the server addon's, and the others' plays of its episodes
// take the versions for what they are.
func TestAUsersOwnDescriptionLeavesTheRuntimeOfTheServersEpisodes(t *testing.T) {
	t.Parallel()
	// ffprobe tells every version two hours long.
	probe := newFakeProbe(t, true)
	d := newDescribedEpisodes(t, newProbingServer(t, 10, probe.path))
	versions, err := d.library.Versions(t.Context(), d.other, d.episode)
	if err != nil || len(versions) == 0 {
		t.Fatalf("the episode's versions: %v %v", versions, err)
	}
	if versions[0].Runtime.Minutes() != 45 {
		t.Errorf("the versions' runtime: %v", versions[0].Runtime)
	}
	if _, err := d.handler.Playback.Analyze(t.Context(), versions[0]); errors.Is(err, playback.ErrStandIn) {
		t.Errorf("the episode's first version taken for a stand-in: %v", err)
	}
}

// An episode's record is one for every user. A user's own description of
// one of the server's series, read by that user, leaves the records of its
// episodes the server addon's: the others' name, overview, thumbnail and
// runtime stay its, while the user reads their own description.
func TestAUsersOwnDescriptionLeavesTheServersEpisodesAsTheyAre(t *testing.T) {
	t.Parallel()
	d := newDescribedEpisodes(t, newTestServer(t, 10))
	if got := d.stored(t); got != serverEpisode {
		t.Errorf("the episode's record: %q", got)
	}
	artwork, err := d.library.Artwork(t.Context(), d.episode, "Primary")
	if err != nil || artwork.URL != serverThumbnail {
		t.Errorf("the episode's image: %+v %v", artwork, err)
	}
	var episode BaseItemDto
	if status := d.get(t, "/Users/"+d.other.ID.String()+"/Items/"+d.episode.String(), d.otherToken, &episode); status != http.StatusOK ||
		episode.Name != "Pilot" {
		t.Errorf("the episode for the others: %d %q", status, episode.Name)
	}
	if listed := d.episodes(t, d.otherToken, "Server Shows"); len(listed) != 1 || listed[0].Name != "Pilot" ||
		listed[0].Overview == nil || *listed[0].Overview != "How it starts." {
		t.Errorf("the episodes for the others: %+v", listed)
	}
}

// Records an earlier version kept of a user's own description, which name
// no addon, are the server addon's again at the next listing of the
// series by one of its users.
func TestTheServersEpisodesAreItsAgainAtTheirNextListing(t *testing.T) {
	t.Parallel()
	d := newDescribedEpisodes(t, newTestServer(t, 10))
	if _, err := d.pool.Exec(t.Context(), `UPDATE items SET data = data - 'addon' || jsonb_build_object('video',
		data->'video' || '{"title": "Renamed", "overview": "Told otherwise.", "thumbnail": "`+ownThumbnail+`", "runtime": "100 h"}'::jsonb)
		WHERE id = $1`, d.episode); err != nil {
		t.Fatal(err)
	}
	d.episodes(t, d.otherToken, "Server Shows")
	if got := d.stored(t); got != serverEpisode {
		t.Errorf("the episode's record after the listing: %q", got)
	}
	var named bool
	if err := d.pool.QueryRow(t.Context(), "SELECT data ? 'addon' FROM items WHERE id = $1", d.episode).Scan(&named); err != nil || !named {
		t.Errorf("the record names its addon: %v %v", named, err)
	}
	// The user's own description leaves the healed record as it is.
	if _, err := d.store.UpdateUser(t.Context(), d.second.ID, accounts.UserChanges{IsAdministrator: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	d.episodes(t, d.signIn("second", "laptop"), "Described")
	if got := d.stored(t); got != serverEpisode {
		t.Errorf("the episode's record after the user's own listing: %q", got)
	}
}
