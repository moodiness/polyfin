package library

import (
	"context"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A series' seasons and episodes are recorded from its description, which
// another addon than the one that listed it may give: a member's own addon
// describes a series of the server's when no server addon does. The
// thumbnails and season posters it gives are confined as that addon is,
// though the series is not.
func TestEpisodeAndSeasonArtworkFollowsTheDescribingAddon(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	local, hits := localImages(t)

	// The server's addon lists a series and describes none.
	show := stremio.Meta{ID: "tt7000001", Type: "series", Name: "Show", Poster: "https://images.example/show.jpg"}
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "shared", Name: "Shared", Version: "1", Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{{Type: "series", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"series/top": {show}},
	})
	// The member's own addon lists a series of its own, and describes any.
	own := e.ownAddon(stremio.Manifest{ID: "own", Name: "Own", Version: "1", Types: []string{"series"},
		Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
		Catalogs:  []stremio.Catalog{{Type: "series", ID: "mine", Name: "Mine"}}})
	ownShow := stremio.Meta{ID: "own7000002", Type: "series", Name: "Own show", Poster: "https://images.example/own.jpg"}
	now := time.Now()
	e.service.pages.Put(pageKey{addon: own, catalogType: "series", catalogID: "mine"}, fetched[[]stremio.Meta]{[]stremio.Meta{ownShow}, now})
	for _, id := range []string{show.ID, ownShow.ID} {
		e.service.metas.Put(metaKey{own, "series", id}, fetched[stremio.Meta]{stremio.Meta{ID: id, Type: "series", Name: "Show",
			Videos: []stremio.Video{{ID: id + ":1:1", Title: "One", Season: 1, Episode: 1, Thumbnail: local + "/episode.png?of=" + id,
				Released: "2020-01-01T00:00:00.000Z"}},
			Extras: &stremio.Extras{SeasonPosterByNumber: map[string]string{"1": local + "/season.png?of=" + id}}}, now})
	}

	for library, id := range map[string]string{"Mine": ownShow.ID, "Top": show.ID} {
		listed := e.listed(e.member, library)
		if len(listed) != 1 {
			t.Fatalf("%s lists %v", library, listed)
		}
		episodes, err := e.service.Episodes(ctx, e.member, listed[0], nil)
		if err != nil || len(episodes) != 1 {
			t.Fatalf("the episodes of %s: %v %v", library, names(episodes), err)
		}
		seasons, err := e.service.Seasons(ctx, e.member, listed[0])
		if err != nil || len(seasons) != 1 {
			t.Fatalf("the seasons of %s: %v %v", library, names(seasons), err)
		}
		for what, of := range map[string]struct {
			item accounts.ID
			url  string
		}{"episode": {episodes[0].ID, local + "/episode.png?of=" + id}, "season": {seasons[0].ID, local + "/season.png?of=" + id}} {
			art, err := e.service.Artwork(ctx, of.item, "Primary")
			if err != nil || art.URL != of.url {
				t.Fatalf("the %s's image in %s: %+v %v, want the member's addon's", what, library, art, err)
			}
			if !e.refused(art) {
				t.Errorf("the %s's image in %s, which the member's addon gave, is downloaded from a local address", what, library)
			}
		}
	}
	// The series the server's addon listed is still its own.
	if series, err := e.service.load(ctx, itemID(titleKey(KindSeries, show.ID))); err != nil || series.Confined {
		t.Errorf("the server's series: confined %t, %v", series.Confined, err)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("the local server got %d requests", n)
	}
}

// A member who turns the server's addons off for themselves has their own
// addons alone describe a series of the server's. Its episodes then show
// the thumbnails their addon gave, confined; once a server addon describes
// the series again, they show its thumbnails, with its reach. A season
// shows the poster of the description the server's addons keep, with its
// reach, whichever addon its record was last written from.
func TestOwnAddonsDescribingAServersSeriesConfineItsEpisodes(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	local, hits := localImages(t)
	show := stremio.Meta{ID: "tt7000003", Type: "series", Name: "Show", Poster: "https://images.example/show.jpg"}
	full := show
	full.Videos = []stremio.Video{{ID: show.ID + ":1:1", Title: "One", Season: 1, Episode: 1, Thumbnail: "https://images.example/e1.jpg"}}
	full.Extras = &stremio.Extras{SeasonPosterByNumber: map[string]string{"1": "https://images.example/s1.jpg"}}
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "shared", Name: "Shared", Version: "1", Types: []string{"series"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs:  []stremio.Catalog{{Type: "series", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"series/top": {show}},
		metas:    map[string]stremio.Meta{"series/" + show.ID: full},
	})
	listed := e.listed(e.member, "Top")
	if len(listed) != 1 {
		t.Fatalf("Top lists %v", listed)
	}
	// artwork lists the series' episodes and seasons for user, and
	// returns the artwork of its episode and of its season.
	artwork := func(user accounts.User) (episode, season Artwork) {
		t.Helper()
		episodes, err := e.service.Episodes(ctx, user, listed[0], nil)
		if err != nil || len(episodes) != 1 {
			t.Fatalf("the episodes for %s: %v %v", user.Name, names(episodes), err)
		}
		seasons, err := e.service.Seasons(ctx, user, listed[0])
		if err != nil || len(seasons) != 1 {
			t.Fatalf("the seasons for %s: %v %v", user.Name, names(seasons), err)
		}
		if episode, err = e.service.Artwork(ctx, episodes[0].ID, "Primary"); err != nil {
			t.Fatalf("the episode's image for %s: %v", user.Name, err)
		}
		if season, err = e.service.Artwork(ctx, seasons[0].ID, "Primary"); err != nil {
			t.Fatalf("the season's image for %s: %v", user.Name, err)
		}
		return episode, season
	}
	if episode, _ := artwork(e.member); episode.URL != "https://images.example/e1.jpg" || episode.URLConfined() {
		t.Fatalf("the episode's image from the server's addon: %+v", episode)
	}

	// The member adds an addon describing series, and turns the server's
	// addons off for themselves.
	own := e.ownAddon(stremio.Manifest{ID: "own", Name: "Own", Version: "1", Types: []string{"series"}, Resources: []stremio.Resource{{Name: "meta"}}})
	theirs := show
	theirs.Videos = []stremio.Video{{ID: show.ID + ":1:1", Title: "One", Season: 1, Episode: 1, Thumbnail: local + "/episode.png"}}
	theirs.Extras = &stremio.Extras{SeasonPosterByNumber: map[string]string{"1": local + "/season.png"}}
	e.service.metas.Put(metaKey{own, "series", show.ID}, fetched[stremio.Meta]{theirs, time.Now()})
	member, err := e.users.UpdateUser(ctx, e.member.ID, accounts.UserChanges{UseSharedAddons: new(false)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	episode, season := artwork(member)
	if episode.URL != local+"/episode.png" {
		t.Fatalf("the episode's image is not the member's addon's: %+v", episode)
	}
	if !e.refused(episode) {
		t.Error("the thumbnail the member's addon gave is downloaded from a local address")
	}
	if season.URL != "https://images.example/s1.jpg" || season.URLConfined() {
		t.Errorf("the season's image: %+v, want the server's addon's, with its reach", season)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("the local server got %d requests", n)
	}

	// Another user lists the series: the server's addon describes it.
	if episode, _ := artwork(e.admin); episode.URL != "https://images.example/e1.jpg" || episode.URLConfined() {
		t.Errorf("the episode's image once the server's addon described the series again: %+v", episode)
	}
}

// describingIPTV is an IPTV source whose every title is meta, which the
// other addons complete.
type describingIPTV struct {
	enrichingIPTV
	meta stremio.Meta
}

func (d describingIPTV) Meta(context.Context, accounts.ID, string) (stremio.Meta, error) {
	return d.meta, nil
}

// An IPTV source's series is completed with another addon's description:
// its episodes take that addon's thumbnails where the source gives none,
// and its seasons that addon's posters. When that addon is a member's
// own, they are confined as it is, though the source is the server's.
func TestEpisodesAnOwnAddonCompletesForAnIPTVSeriesAreConfined(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	local, hits := localImages(t)
	source := accounts.ID{9}
	box := stremio.Meta{ID: "box:7", Type: "series", Name: "Show", ImdbID: "tt7000009",
		Videos: []stremio.Video{{ID: "box:7:1:1", Title: "One", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00.000Z"}}}
	e.service.iptv = describingIPTV{meta: box}
	own := e.ownAddon(stremio.Manifest{ID: "own", Name: "Own", Version: "1", Types: []string{"series"}, Resources: []stremio.Resource{{Name: "meta"}}})
	e.service.metas.Put(metaKey{own, "series", box.ImdbID}, fetched[stremio.Meta]{stremio.Meta{ID: box.ImdbID, Type: "series", Name: "Show",
		Videos: []stremio.Video{{ID: box.ImdbID + ":1:1", Season: 1, Episode: 1, Thumbnail: local + "/episode.png"}},
		Extras: &stremio.Extras{SeasonPosterByNumber: map[string]string{"1": local + "/season.png"}}}, time.Now()})
	listed := stremio.Meta{ID: box.ID, Type: "series", Name: "Show"}
	series := record{ID: itemID(titleKey(KindSeries, box.ID)), Key: titleKey(KindSeries, box.ID), Kind: KindSeries, Addon: &source, Meta: &listed}
	if err := e.service.save(ctx, []record{series}); err != nil {
		t.Fatal(err)
	}
	// The member's view, with the server's IPTV source.
	v, err := e.service.view(ctx, e.member)
	if err != nil {
		t.Fatal(err)
	}
	v.addons = append(v.addons, installed{shared: true, addon: addons.Addon{ID: source, Kind: addons.KindXtream, Enabled: true,
		Manifest: stremio.Manifest{Types: []string{"series"}, Resources: []stremio.Resource{{Name: "meta"}}}}})
	episodes, records, err := e.service.orderedEpisodes(ctx, v, series.ID, nil)
	if err != nil || len(episodes) != 1 {
		t.Fatalf("the episodes: %v %v", names(episodes), err)
	}
	if err := e.service.save(ctx, records); err != nil {
		t.Fatal(err)
	}
	for what, of := range map[string]struct {
		item accounts.ID
		url  string
	}{"episode": {episodes[0].ID, local + "/episode.png"}, "season": {episodes[0].SeasonID, local + "/season.png"}} {
		art, err := e.service.Artwork(ctx, of.item, "Primary")
		if err != nil || art.URL != of.url {
			t.Fatalf("the %s's image: %+v %v, want the member's addon's", what, art, err)
		}
		if !e.refused(art) {
			t.Errorf("the %s's image, which the member's addon gave, is downloaded from a local address", what)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("the local server got %d requests", n)
	}
}

// An episode whose series is not recorded is confined: nothing tells that
// its thumbnail may be downloaded from a local address.
func TestAnEpisodeWithoutItsSeriesIsConfined(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	video := stremio.Video{ID: "gone:1:1", Season: 1, Episode: 1, Thumbnail: "https://images.example/gone.jpg"}
	episode := record{ID: itemID(episodeKey(video.ID)), Key: episodeKey(video.ID), Kind: KindEpisode, SeriesID: "gone:1", Season: 1, Video: &video}
	if err := e.service.save(ctx, []record{episode}); err != nil {
		t.Fatal(err)
	}
	art, err := e.service.Artwork(ctx, episode.ID, "Primary")
	if err != nil || art.URL != video.Thumbnail || !art.URLConfined() {
		t.Errorf("the image of an episode without its series: %+v %v, want it confined", art, err)
	}
}
