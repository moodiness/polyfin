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

// watchlist puts movie n on token's user's Watchlist, or takes it off.
func (g generating) watchlist(t *testing.T, token string, add bool, n int) {
	t.Helper()
	method := http.MethodDelete
	if add {
		method = http.MethodPost
	}
	id := library.TitleItemID(library.KindMovie, movieID(n)).String()
	if status, _, body := g.send(method, "/Polyfin/UserItems/"+id+"/Watchlist", app("tv", token), "", ""); status != http.StatusNoContent {
		t.Fatalf("%s movie %d: %d %s", method, n, status, body)
	}
}

// Each user's Watchlist view, and each smart list shown as a library, gets
// a wide image of the titles it lists for them, found by its tag, a
// request after a restart included, but under no other view. A title
// taken off the Watchlist leaves its image at once. A user blocking genres
// gets none.
func TestTheWatchlistAndSmartLibrariesShowEachUsersTitles(t *testing.T) {
	t.Parallel()
	g := newGenerating(t)
	g.titlesOf(t, g.views["Top"])
	g.makeSmartList(t, nil, "Picks", smartlists.ShownAsLibrary, []string{"movie/top"}, smartlists.SortCatalog)
	g.watchlist(t, g.member, true, 1)
	g.watchlist(t, g.member, true, 2)
	g.watchlist(t, g.administrator, true, 3)
	g.watchlist(t, g.kid, true, 1)

	member := g.viewDtos(t, g.member)
	watchlist, picks := member["Watchlist"], member["Picks"]
	if watchlist.ImageTags["Primary"] == "" || picks.ImageTags["Primary"] == "" {
		t.Fatalf("the member's images: Watchlist %v, Picks %v", watchlist.ImageTags, picks.ImageTags)
	}
	for _, view := range []BaseItemDto{watchlist, picks} {
		if ratio := aspectRatio(view); ratio != 16.0/9.0 {
			t.Errorf("%s's ratio: %v", view.Name, ratio)
		}
	}
	var item BaseItemDto
	if g.get(t, "/Items/"+watchlist.Id, g.member, &item); item.ImageTags["Primary"] != watchlist.ImageTags["Primary"] {
		t.Errorf("the Watchlist as an item: %v", item.ImageTags)
	}
	admin := g.viewDtos(t, g.administrator)
	if tag := admin["Watchlist"].ImageTags["Primary"]; tag == "" || tag == watchlist.ImageTags["Primary"] {
		t.Errorf("the administrator's Watchlist: %q, the member's %q", tag, watchlist.ImageTags["Primary"])
	}
	if kid := g.viewDtos(t, g.kid); kid["Watchlist"].Id == "" || kid["Watchlist"].ImageTags["Primary"] != "" || kid["Picks"].ImageTags["Primary"] != "" {
		t.Errorf("a user blocking genres: %+v", kid)
	}

	path := "/Items/" + watchlist.Id + "/Images/Primary?tag=" + watchlist.ImageTags["Primary"]
	status, picture, etag := g.download(t, path)
	if status != http.StatusOK || picture.Bounds() != image.Rect(0, 0, 1920, 1080) || etag != `"`+watchlist.ImageTags["Primary"]+`"` || !named(picture) {
		t.Fatalf("the member's Watchlist image: %d %v %s", status, picture, etag)
	}
	if status, _, _ := g.download(t, "/Items/"+picks.Id+"/Images/Primary?tag="+picks.ImageTags["Primary"]); status != http.StatusOK {
		t.Errorf("the member's Picks image: %d", status)
	}
	// Images requests name no user: without its tag, or under another
	// view, the image is not found.
	for _, wrong := range []string{"/Items/" + watchlist.Id + "/Images/Primary", "/Items/" + picks.Id + "/Images/Primary?tag=" + watchlist.ImageTags["Primary"]} {
		if status, _, _ := g.send(http.MethodGet, wrong, "", "", ""); status != http.StatusNotFound {
			t.Errorf("%s: %d", wrong, status)
		}
	}

	// Taken off, a title shown leaves the image as the view next shows,
	// with no time passing.
	g.watchlist(t, g.member, false, 2)
	changed := g.viewDtos(t, g.member)["Watchlist"].ImageTags["Primary"]
	if changed == "" || changed == watchlist.ImageTags["Primary"] {
		t.Fatalf("after taking a title off: %q, was %q", changed, watchlist.ImageTags["Primary"])
	}
	// Apps that keep the user's views for a while ask for the image before
	// the change: it is still found.
	if status, _, _ := g.download(t, path); status != http.StatusOK {
		t.Errorf("the image before the change: %d", status)
	}

	// A restart: the tags given before are found.
	options := g.handler.Options
	options.Library = library.New(g.pool, g.addons, options.Stremio, options.Logger, g.store.Settings)
	restarted := httptest.NewServer(New(options))
	t.Cleanup(restarted.Close)
	for _, view := range []struct{ id, tag string }{{watchlist.Id, changed}, {picks.Id, picks.ImageTags["Primary"]}} {
		response, err := http.Get(restarted.URL + "/Items/" + view.id + "/Images/Primary?tag=" + view.tag)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "image/") {
			t.Errorf("after a restart, %s: %d %s", view.id, response.StatusCode, response.Header.Get("Content-Type"))
		}
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
