package jellyfin

import (
	"image"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
