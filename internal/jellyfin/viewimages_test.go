package jellyfin

import (
	"image"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/homerows"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/mosaic"
	"github.com/moodiness/polyfin/internal/smartlists"
)

// viewDtos lists the views of token's user by name.
func (g generating) viewDtos(t *testing.T, token string) map[string]BaseItemDto {
	t.Helper()
	var page QueryResult
	if status := g.get(t, "/UserViews", token, &page); status != http.StatusOK {
		t.Fatalf("views: %d", status)
	}
	byName := map[string]BaseItemDto{}
	for _, view := range page.Items {
		byName[view.Name] = view
	}
	return byName
}

// Each smart list shown as a library gets a wide image of the titles it
// lists for each user, found by its tag, a request after a restart
// included, but under no other view. A user blocking genres gets none.
func TestSmartLibrariesShowEachUsersTitles(t *testing.T) {
	t.Parallel()
	g := newGenerating(t)
	g.titlesOf(t, g.views["Top"])
	g.makeSmartList(t, nil, "Picks", smartlists.ShownAsLibrary, []string{"movie/top"}, smartlists.SortCatalog)

	member := g.viewDtos(t, g.member)
	picks := member["Picks"]
	if picks.ImageTags["Primary"] == "" {
		t.Fatalf("the member's Picks image: %v", picks.ImageTags)
	}
	if ratio := aspectRatio(picks); ratio != 16.0/9.0 {
		t.Errorf("Picks' ratio: %v", ratio)
	}
	var item BaseItemDto
	if g.get(t, "/Items/"+picks.Id, g.member, &item); item.ImageTags["Primary"] != picks.ImageTags["Primary"] {
		t.Errorf("Picks as an item: %v", item.ImageTags)
	}
	if kid := g.viewDtos(t, g.kid); kid["Picks"].Id == "" || kid["Picks"].ImageTags["Primary"] != "" {
		t.Errorf("a user blocking genres: %+v", kid)
	}

	path := "/Items/" + picks.Id + "/Images/Primary?tag=" + picks.ImageTags["Primary"]
	status, picture, etag := g.download(t, path)
	if status != http.StatusOK || picture.Bounds() != image.Rect(0, 0, 1920, 1080) || etag != `"`+picks.ImageTags["Primary"]+`"` || !named(picture) {
		t.Fatalf("the member's Picks image: %d %v %s", status, picture, etag)
	}
	// Images requests name no user: without its tag, the image is not
	// found.
	if status, _, _ := g.send(http.MethodGet, "/Items/"+picks.Id+"/Images/Primary", "", "", ""); status != http.StatusNotFound {
		t.Errorf("without its tag: %d", status)
	}

	// A restart: the tag given before is found.
	options := g.handler.Options
	options.Library = library.New(g.pool, g.addons, options.Stremio, options.Logger, g.store.Settings)
	restarted := httptest.NewServer(New(options))
	t.Cleanup(restarted.Close)
	response, err := http.Get(restarted.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "image/") {
		t.Errorf("after a restart: %d %s", response.StatusCode, response.Header.Get("Content-Type"))
	}
}

// The Live TV, Replay, Collections and Playlists views show their emblem,
// the same for every user, wide.
func TestPolyfinsViewsShowTheirEmblem(t *testing.T) {
	t.Parallel()
	g := newGenerating(t)
	g.titlesOf(t, g.views["Top"])
	movie := library.TitleItemID(library.KindMovie, movieID(1)).String()
	if status, _, answer := g.send(http.MethodPost, "/Playlists?name=Weekend&ids="+movie, app("tv", g.member), "", ""); status != http.StatusOK {
		t.Fatalf("a playlist: %d %s", status, answer)
	}
	playlists := g.viewDtos(t, g.member)["Playlists"]
	want := library.ImageTag(library.EmblemImage(mosaic.EmblemPlaylists).URL())
	if playlists.ImageTags["Primary"] != want || aspectRatio(playlists) != 16.0/9.0 {
		t.Errorf("the Playlists view: %v %v", playlists.ImageTags, aspectRatio(playlists))
	}
	// A playlist with nothing to draw, such as the member's empty
	// Watchlist, shows that emblem too, wide, rather than a blank tile;
	// one with a title keeps a mosaic of its own.
	var lists QueryResult
	if status := g.get(t, "/Items?ParentId="+playlists.Id, g.member, &lists); status != http.StatusOK {
		t.Fatalf("the member's playlists: %d", status)
	}
	byName := map[string]BaseItemDto{}
	for _, item := range lists.Items {
		byName[item.Name] = item
	}
	if watchlist := byName["Watchlist"]; watchlist.ImageTags["Primary"] != want || aspectRatio(watchlist) != 16.0/9.0 {
		t.Errorf("the empty Watchlist: %v %v", watchlist.ImageTags, aspectRatio(watchlist))
	}
	if status, picture, _ := g.download(t, "/Items/"+byName["Watchlist"].Id+"/Images/Primary?tag="+want); status != http.StatusOK ||
		picture.Bounds() != image.Rect(0, 0, 1920, 1080) {
		t.Errorf("the empty Watchlist's image: %d %v", status, picture)
	}
	if weekend := byName["Weekend"]; weekend.ImageTags["Primary"] == "" || weekend.ImageTags["Primary"] == want {
		t.Errorf("the Weekend playlist: %v", weekend.ImageTags)
	}
	tags := map[string]bool{}
	for id, emblem := range viewEmblems {
		tag := library.ImageTag(library.EmblemImage(emblem).URL())
		tags[tag] = true
		status, picture, etag := g.download(t, "/Items/"+id.String()+"/Images/Primary?tag="+tag)
		if status != http.StatusOK || picture.Bounds() != image.Rect(0, 0, 1920, 1080) || etag != `"`+tag+`"` {
			t.Errorf("%s: %d %v %s", emblem, status, picture, etag)
		}
	}
	if len(tags) != 4 {
		t.Errorf("the emblems share tags: %v", tags)
	}
}

// Home rows shown as libraries get an image, wide, for each user: a row of
// a playlist the playlist's own, the Playlists emblem while it has nothing
// to draw, and another row a mosaic of its first titles. The tag changes
// with the titles, and the images are found by their tag.
func TestRowLibrariesShowTheirImages(t *testing.T) {
	t.Parallel()
	g := newGenerating(t)
	g.titlesOf(t, g.views["Top"])
	movie := func(n int) string { return library.TitleItemID(library.KindMovie, movieID(n)).String() }
	for _, n := range []int{3, 2, 1} {
		g.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Polyfin/UserItems/"+movie(n)+"/Watchlist", g.member, nil)
	}
	var held WatchlistDto
	g.get(t, "/Polyfin/UserItems/Watchlist", g.member, &held)
	empty := g.createPlaylist(t, g.member, map[string]any{"Name": "Nothing yet", "IsPublic": true})
	emptyID, _ := accounts.ParseID(empty)
	g.addRow(t, homerows.Row{Name: "Top movies", Kind: homerows.KindCatalog, Place: homerows.PlaceTop, Addon: g.addonID,
		CatalogType: "movie", CatalogID: "top", Everyone: true, Library: true})
	g.addRow(t, homerows.Row{Name: "To watch", Kind: homerows.KindWatchlist, Place: homerows.PlaceTop, Everyone: true, Library: true})
	g.addRow(t, homerows.Row{Name: "Soon", Kind: homerows.KindPlaylist, Place: homerows.PlaceTop, Playlist: emptyID, Everyone: true, Library: true})
	emblem := library.ImageTag(library.EmblemImage(mosaic.EmblemPlaylists).URL())

	views := g.viewDtos(t, g.member)
	var watchlist BaseItemDto
	g.get(t, "/Items/"+held.PlaylistId, g.member, &watchlist)
	top, mine, soon := views["Top movies"], views[watchlist.Name], views["Soon"]
	if top.ImageTags["Primary"] == "" || aspectRatio(top) != 16.0/9.0 {
		t.Errorf("the catalog row's library: %v %v", top.ImageTags, aspectRatio(top))
	}
	if mine.ImageTags["Primary"] == "" || mine.ImageTags["Primary"] == emblem || mine.ImageTags["Primary"] != watchlist.ImageTags["Primary"] {
		t.Errorf("the Watchlist row's library: %v, the Watchlist's %v", mine.ImageTags, watchlist.ImageTags)
	}
	if soon.ImageTags["Primary"] != emblem || aspectRatio(soon) != 16.0/9.0 {
		t.Errorf("the empty playlist's row: %v", soon.ImageTags)
	}
	var folder BaseItemDto
	if g.get(t, "/Items/"+top.Id, g.member, &folder); folder.ImageTags["Primary"] != top.ImageTags["Primary"] {
		t.Errorf("the catalog row's folder: %v", folder.ImageTags)
	}
	for _, view := range []BaseItemDto{top, mine, soon} {
		status, picture, etag := g.download(t, "/Items/"+view.Id+"/Images/Primary?tag="+view.ImageTags["Primary"])
		if status != http.StatusOK || picture.Bounds() != image.Rect(0, 0, 1920, 1080) || etag != `"`+view.ImageTags["Primary"]+`"` {
			t.Errorf("%s's image: %d %v %s", view.Name, status, picture, etag)
		}
	}

	// The administrator's Watchlist is empty: their row shows the emblem.
	var theirs WatchlistDto
	g.get(t, "/Polyfin/UserItems/Watchlist", g.administrator, &theirs)
	var theirWatchlist BaseItemDto
	g.get(t, "/Items/"+theirs.PlaylistId, g.administrator, &theirWatchlist)
	if got := g.viewDtos(t, g.administrator)[theirWatchlist.Name]; got.Id == "" || got.ImageTags["Primary"] != emblem {
		t.Errorf("the administrator's Watchlist row: %+v", got)
	}

	// A title put on the Watchlist changes its image, and the row's.
	g.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Polyfin/UserItems/"+movie(4)+"/Watchlist", g.member, nil)
	g.later()
	g.get(t, "/Items/"+held.PlaylistId, g.member, &watchlist)
	if after := g.viewDtos(t, g.member)[watchlist.Name]; after.ImageTags["Primary"] == mine.ImageTags["Primary"] ||
		after.ImageTags["Primary"] != watchlist.ImageTags["Primary"] {
		t.Errorf("after a title came: %v, the Watchlist's %v", after.ImageTags, watchlist.ImageTags)
	}
}
