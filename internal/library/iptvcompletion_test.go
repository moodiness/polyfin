package library

import (
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A member's own addon that completes the description of a series of one
// of the server's IPTV sources leaves the episodes and seasons the server
// recorded as they are, for everyone: the member reads the completion in
// their own lists, not in the records every user shares.
func TestAnOwnAddonCompletingAServersIPTVSeriesLeavesItsEpisodes(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	// The server's source, installed so that its records name the server.
	e.install(addons.Shared(), &fakeAddon{manifest: stremio.Manifest{ID: "box", Name: "Box", Version: "1", Types: []string{"tv"},
		Resources: []stremio.Resource{{Name: "catalog"}}, Catalogs: []stremio.Catalog{{Type: "tv", ID: "box", Name: "Box"}}}})
	source := e.installedID(addons.Shared(), "box")
	serverThumbnail := "https://images.example/box-one.jpg"
	box := stremio.Meta{ID: "box:7", Type: "series", Name: "Show", ImdbID: "tt7000019",
		Videos: []stremio.Video{{ID: "box:7:1:1", Title: "One", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00.000Z", Thumbnail: serverThumbnail}}}
	e.service.iptv = describingIPTV{meta: box}
	own := e.ownAddon(stremio.Manifest{ID: "own", Name: "Own", Version: "1", Types: []string{"series"}, Resources: []stremio.Resource{{Name: "meta"}}})
	e.service.metas.Put(metaKey{own, "series", box.ImdbID}, fetched[stremio.Meta]{stremio.Meta{ID: box.ImdbID, Type: "series", Name: "Show",
		Videos: []stremio.Video{{ID: box.ImdbID + ":1:1", Season: 1, Episode: 1, Title: "Own title", Overview: "Own overview",
			Thumbnail: "https://images.example/own-one.jpg"}},
		Extras: &stremio.Extras{SeasonPosterByNumber: map[string]string{"1": "https://images.example/own-season.jpg"}}}, time.Now()})
	listed := stremio.Meta{ID: box.ID, Type: "series", Name: "Show"}
	series := record{ID: itemID(titleKey(KindSeries, box.ID)), Key: titleKey(KindSeries, box.ID), Kind: KindSeries, Addon: &source, Meta: &listed}
	if err := e.service.save(ctx, []record{series}); err != nil {
		t.Fatal(err)
	}
	member, err := e.service.view(ctx, e.member)
	if err != nil {
		t.Fatal(err)
	}
	member.addons = append(member.addons, installed{shared: true, addon: addons.Addon{ID: source, Kind: addons.KindXtream, Enabled: true,
		Manifest: stremio.Manifest{Types: []string{"series"}, Resources: []stremio.Resource{{Name: "meta"}}}}})
	// The server's listing first: a view of the server's addons alone.
	server := member
	server.addons = slices.DeleteFunc(slices.Clone(member.addons), func(a installed) bool { return !a.shared })
	list := func(v view) (episode, season Item) {
		t.Helper()
		episodes, records, err := e.service.orderedEpisodes(ctx, v, series.ID, nil)
		if err != nil || len(episodes) != 1 {
			t.Fatalf("the episodes: %v %v", names(episodes), err)
		}
		if err := e.service.save(ctx, records); err != nil {
			t.Fatal(err)
		}
		return episodes[0], Item{ID: episodes[0].SeasonID}
	}
	episode, season := list(server)
	if art, err := e.service.Artwork(ctx, episode.ID, "Primary"); err != nil || art.URL != serverThumbnail {
		t.Fatalf("the episode's image after the server's listing: %+v %v", art, err)
	}
	// The member lists the series with their own addon's completion: they
	// read it, the records keep the server's.
	if mine, _ := list(member); mine.Overview != "Own overview" {
		t.Fatalf("the member's episode: %+v, want their addon's overview", mine)
	}
	if art, err := e.service.Artwork(ctx, episode.ID, "Primary"); err != nil || art.URL != serverThumbnail {
		t.Errorf("the episode's image: %+v %v, want the server's source's", art, err)
	}
	if art, err := e.service.Artwork(ctx, season.ID, "Primary"); err == nil && art.URL == "https://images.example/own-season.jpg" {
		t.Errorf("the season's image is the member's addon's: %+v", art)
	}
	r, err := e.service.load(ctx, episode.ID)
	if err != nil || r.Video == nil || r.Video.Overview != "" || r.Video.Thumbnail != serverThumbnail {
		t.Errorf("the episode's record: %+v %v, want the server's", r.Video, err)
	}
}
