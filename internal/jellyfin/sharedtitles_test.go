package jellyfin

import (
	"bytes"
	"encoding/json"
	"image"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// sharedTitles is a server whose addon lists, describes and streams
// R-rated movies and a TV-MA series, with posters 60 pixels wide:
//
//	Top Movies   tt0111161 The Real Movie, tt0000002 The Unopened Movie
//	Top Shows    tt0903747 The Real Show
//	Late Movies  tt0000003 The Late Movie, a library no one lists at first
//
// and a user's own addon that names the same identifiers with other
// names, texts, genres and posters (30 pixels wide): its search catalogs
// answer tt0111161, tt0000002 and tt0903747, and its catalog Mine lists
// tt0000003. It gives them the types the test chooses: "movie" and
// "series", or the anime types, which make the same items.
//
// No test server is on a public address, which is all a member's own
// addon may reach: the owner is an administrator while their addon is
// read, and a member (see demote) when the others look. What is recorded
// of an addon follows who installed it, not what they may do.
type sharedTitles struct {
	testServer
	admin, other, child, owner string
	ownerID                    accounts.ID
	ids                        map[string]string
}

func newSharedTitles(t *testing.T, movieType, seriesType string) *sharedTitles {
	t.Helper()
	a := &sharedTitles{testServer: newTestServer(t, 10), ids: map[string]string{}}
	s := a.testServer
	pictures := func(side int) *httptest.Server {
		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngOf(t, side, side*3/2))
		}))
		t.Cleanup(host.Close)
		return host
	}
	real, changed := pictures(60), pictures(30)
	movie := func(id, name, poster string) stremio.Meta {
		return stremio.Meta{ID: id, Type: "movie", Name: name, Description: "The real overview.", Year: "1994", Genres: []string{"Crime"},
			Poster: real.URL + poster, Extras: &stremio.Extras{Certification: "R"}}
	}
	described := map[string]stremio.Meta{
		"tt0111161": movie("tt0111161", "The Real Movie", "/movie.png"),
		"tt0000002": movie("tt0000002", "The Unopened Movie", "/unopened.png"),
		"tt0000003": movie("tt0000003", "The Late Movie", "/late.png"),
		"tt0903747": {ID: "tt0903747", Type: "series", Name: "The Real Show", Description: "The real overview.", Year: "2008",
			Genres: []string{"Crime"}, Poster: real.URL + "/show.png", Extras: &stremio.Extras{Certification: "TV-MA"},
			Videos: []stremio.Video{{ID: "tt0903747:1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2008-01-20T00:00:00.000Z"}}},
	}
	rows := func(ids ...string) map[string]any {
		var metas []stremio.Meta
		for _, id := range ids {
			m := described[id]
			m.Extras, m.Videos = nil, nil
			metas = append(metas, m)
		}
		return map[string]any{"metas": metas}
	}
	var shared *httptest.Server
	shared = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.Path, ".json")
		id := path[strings.LastIndex(path, "/")+1:]
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "shared.titles", Name: "Titles", Version: "1", Types: []string{"movie", "series"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top Movies"}, {Type: "series", ID: "shows", Name: "Top Shows"},
					{Type: "movie", ID: "late", Name: "Late Movies"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(rows("tt0111161", "tt0000002"))
		case strings.HasPrefix(path, "/catalog/movie/late"):
			_ = json.NewEncoder(w).Encode(rows("tt0000003"))
		case strings.HasPrefix(path, "/catalog/series/shows"):
			_ = json.NewEncoder(w).Encode(rows("tt0903747"))
		case strings.HasPrefix(path, "/meta/"):
			meta, ok := described[id]
			if !ok || !strings.HasPrefix(path, "/meta/"+meta.Type+"/") {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
		case strings.HasPrefix(path, "/stream/movie/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{{Name: "Source 1080p", URL: shared.URL + "/files/" + id + ".mkv",
				BehaviorHints: stremio.StreamBehavior{Filename: id + ".mkv", VideoSize: 4e9}}}})
		case strings.HasPrefix(path, "/files/"):
			_, _ = w.Write([]byte("\x1a\x45\xdf\xa3 the R-rated bytes of " + id))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(shared.Close)
	// The user's own addon lists and finds; it describes and streams
	// nothing.
	find := []stremio.Extra{{Name: "search", IsRequired: true}}
	answer := func(id, metaType, name string) stremio.Meta {
		return stremio.Meta{ID: id, Type: metaType, Name: name, Description: "Changed by a member.", Year: "2031",
			Genres: []string{"Family"}, Poster: changed.URL + "/changed.png"}
	}
	own := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.Path, ".json")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "own.titles", Name: "Mine", Version: "1", Types: []string{"movie", "series"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "find", Name: "Find", Extra: find}, {Type: "series", ID: "find", Name: "Find", Extra: find},
					{Type: "movie", ID: "mine", Name: "Mine"}}})
		case strings.HasPrefix(path, "/catalog/movie/find/search="):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{
				answer("tt0111161", movieType, "Changed Movie"), answer("tt0000002", movieType, "Changed Unopened")}})
		case strings.HasPrefix(path, "/catalog/series/find/search="):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{answer("tt0903747", seriesType, "Changed Show")}})
		case strings.HasPrefix(path, "/catalog/movie/mine"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{answer("tt0000003", movieType, "Changed Late")}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(own.Close)

	installed, err := s.addons.Install(t.Context(), addons.Shared(), shared.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: installed.ID, CatalogType: "movie", CatalogID: "top"}, {AddonID: installed.ID, CatalogType: "series", CatalogID: "shows"},
		{AddonID: installed.ID, CatalogType: "movie", CatalogID: "late"}}); err != nil {
		t.Fatal(err)
	}
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.user("other", nil)
	s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(13)} })
	owner := s.user("owner", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	mine, err := s.addons.Install(t.Context(), addons.Personal(owner.ID), own.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Personal(owner.ID), []addons.LibraryChoice{
		{AddonID: mine.ID, CatalogType: "movie", CatalogID: "mine"}}); err != nil {
		t.Fatal(err)
	}
	a.admin, a.other, a.child, a.owner = s.signIn("admin", "dashboard"), s.signIn("other", "tv"), s.signIn("child", "tablet"), s.signIn("owner", "phone")
	a.ownerID = owner.ID
	// Another user lists Top Movies and Top Shows, and opens the first movie
	// and the series: their ratings are then kept with their records.
	a.list(t, a.other, "Top Movies", "Top Shows")
	for _, name := range []string{"The Real Movie", "The Unopened Movie", "The Real Show"} {
		if a.ids[name] == "" {
			t.Fatalf("titles: %v", a.ids)
		}
	}
	for _, name := range []string{"The Real Movie", "The Real Show"} {
		if status, _ := a.details(t, a.other, a.ids[name]); status != http.StatusOK {
			t.Fatalf("the other user's details of %s: %d", name, status)
		}
	}
	return a
}

// list lists libraries as the user of token, and returns the names listed.
func (a *sharedTitles) list(t *testing.T, token string, libraries ...string) []string {
	t.Helper()
	names := []string{}
	views := a.viewIDs(t, token)
	for _, view := range libraries {
		var page QueryResult
		if status := a.get(t, "/Items?ParentId="+views[view], token, &page); status != http.StatusOK {
			t.Fatalf("the listing of %s: %d", view, status)
		}
		for _, item := range page.Items {
			a.ids[item.Name] = item.Id
			names = append(names, item.Name)
		}
	}
	return names
}

// details answers a title's details to the user of token.
func (a *sharedTitles) details(t *testing.T, token, id string) (int, BaseItemDto) {
	t.Helper()
	var dto BaseItemDto
	status := a.get(t, "/Items/"+id, token, &dto)
	return status, dto
}

// shown tells what a title's details show the user of token: their status,
// then the name, overview, genres and rating.
func (a *sharedTitles) shown(t *testing.T, token, id string) string {
	t.Helper()
	status, dto := a.details(t, token, id)
	genres, overview := "", ""
	if dto.Genres != nil {
		genres = strings.Join(*dto.Genres, ",")
	}
	if dto.Overview != nil {
		overview = *dto.Overview
	}
	return strings.Join([]string{http.StatusText(status), dto.Name, overview, genres, dto.OfficialRating}, " | ")
}

// search has the owner search their own addon's catalogs, and returns the
// names found, sorted.
func (a *sharedTitles) search(t *testing.T, term string) []string {
	t.Helper()
	var found QueryResult
	if status := a.get(t, "/Items?Recursive=true&IncludeItemTypes=Movie,Series&SearchTerm="+url.QueryEscape(term), a.owner, &found); status != http.StatusOK {
		t.Fatalf("the owner's search: %d", status)
	}
	names := itemNames(found.Items)
	slices.Sort(names)
	return names
}

// poster asks for a title's poster with no credentials, by tag when one
// is given, and returns the status and the picture's width.
func (a *sharedTitles) poster(t *testing.T, id, tag string) (int, int) {
	t.Helper()
	path := "/Items/" + id + "/Images/Primary"
	if tag != "" {
		path += "?tag=" + tag
	}
	status, _, body := a.send(http.MethodGet, path, "", "", "")
	config, _, _ := image.DecodeConfig(bytes.NewReader(body))
	return status, config.Width
}

// plays tells what playing a title gives the user of token: how many
// versions its details list, and the status and bytes of its stream.
func (a *sharedTitles) plays(t *testing.T, token, id string) (versions, status int, body string) {
	t.Helper()
	_, dto := a.details(t, token, id)
	if dto.MediaSources != nil {
		for _, source := range *dto.MediaSources {
			if source.Path != "" || source.Size != nil {
				versions++
			}
		}
	}
	status, data := a.call(http.MethodGet, "/Videos/"+id+"/stream.mkv?static=true&api_key="+token, "", nil)
	return versions, status, string(data)
}

// demote makes the owner a member, once their addon was read.
func (a *sharedTitles) demote(t *testing.T) {
	t.Helper()
	if _, err := a.store.UpdateUser(t.Context(), a.ownerID, accounts.UserChanges{IsAdministrator: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
}

// The texts and ratings the server's titles show.
const (
	sharedRealMovie = "OK | The Real Movie | The real overview. | Crime | R"
	sharedRealShow  = "OK | The Real Show | The real overview. | Crime | TV-MA"
	sharedLateMovie = "OK | The Late Movie | The real overview. | Crime | R"
	sharedNotFound  = "Not Found |  |  |  | "
)

// Items are one record for every user. A user's own addon whose search
// answers a movie and a series the server's addons list, under types of
// its own that make the same items, changes nothing of them for anyone
// else: the texts, the listing by identifier and the poster, asked with
// or without its tag, stay the server's, for another user and for an
// administrator, and the kept rating still hides the movie.
func TestAUsersOwnAddonLeavesTheServersTitlesAsTheyAre(t *testing.T) {
	t.Parallel()
	a := newSharedTitles(t, "anime.movie", "anime.series")
	movie, show := a.ids["The Real Movie"], a.ids["The Real Show"]
	if movie != library.TitleItemID(library.KindMovie, "tt0111161").String() || show != library.TitleItemID(library.KindSeries, "tt0903747").String() {
		t.Fatalf("items %s and %s are not derived from their identifiers", movie, show)
	}
	_, before := a.details(t, a.other, movie)
	tag := before.ImageTags["Primary"]
	if status, width := a.poster(t, movie, tag); status != http.StatusOK || width != 60 {
		t.Fatalf("the movie's poster before: %d, %d wide", status, width)
	}

	// The owner searches their own addon, which answers both titles and
	// shows them its own names. They are a member from here.
	if found := a.search(t, "changed"); !slices.Equal(found, []string{"Changed Movie", "Changed Show", "Changed Unopened"}) {
		t.Fatalf("the owner's search: %v", found)
	}
	a.demote(t)

	for who, token := range map[string]string{"another user": a.other, "the administrator": a.admin} {
		if got := a.shown(t, token, movie); got != sharedRealMovie {
			t.Errorf("%s's movie: %q", who, got)
		}
		if got := a.shown(t, token, show); got != sharedRealShow {
			t.Errorf("%s's series: %q", who, got)
		}
	}
	var byIDs QueryResult
	if status := a.get(t, "/Items?Ids="+movie+","+show, a.other, &byIDs); status != http.StatusOK ||
		strings.Join(itemNames(byIDs.Items), ",") != "The Real Movie,The Real Show" {
		t.Errorf("another user's /Items?Ids=: %d %v", status, itemNames(byIDs.Items))
	}
	_, after := a.details(t, a.other, movie)
	_, untagged := a.poster(t, movie, "")
	_, tagged := a.poster(t, movie, tag)
	if untagged != 60 || tagged != 60 || after.ImageTags["Primary"] != tag {
		t.Errorf("the movie's poster: %d wide, %d wide by its tag; tag %s, was %s", untagged, tagged, after.ImageTags["Primary"], tag)
	}
	if got := a.shown(t, a.child, movie); got != sharedNotFound {
		t.Errorf("the restricted user's movie: %q", got)
	}
}

// A title the server's addons rate stays hidden from a user under
// parental control whatever a user's own addon records of it: one whose
// rating was never kept, which the own addon's search answers under a
// type no server addon describes, and one the own addon's catalog listed
// before any of the server's did, which it then holds. The restricted
// user opens neither by its identifier, and streams neither.
//
// The title only the own addon holds is its owner's: they read its
// texts, as an administrator, who edits every item, still does, and no
// other user reads anything of it. The first listing of the server's
// library that holds it gives it to the server's addon, for everyone,
// its owner included.
func TestAUsersOwnAddonOpensNoRatedTitleToARestrictedUser(t *testing.T) {
	t.Parallel()
	a := newSharedTitles(t, "anime.movie", "anime.series")
	unopened, late := a.ids["The Unopened Movie"], library.TitleItemID(library.KindMovie, "tt0000003").String()
	// The owner lists their library, whose catalog names the late movie
	// before the server's library was ever listed, and searches their
	// addon, which answers the movie no one opened. They are a member
	// from here.
	if listed := a.list(t, a.owner, "Mine"); !slices.Equal(listed, []string{"Changed Late"}) {
		t.Fatalf("the owner's library: %v", listed)
	}
	a.search(t, "changed")
	a.demote(t)

	for name, id := range map[string]string{"tt0000002": unopened, "tt0000003": late} {
		if got := a.shown(t, a.child, id); got != sharedNotFound {
			t.Errorf("the restricted user's %s: %q", name, got)
		}
		if versions, status, body := a.plays(t, a.child, id); versions != 0 || status == http.StatusOK || strings.Contains(body, "the R-rated bytes") {
			t.Errorf("the restricted user's play of %s: %d versions, stream %d %q", name, versions, status, body)
		}
	}
	// The late movie is its owner's own for now.
	const own = "OK | Changed Late | Changed by a member. | Family | "
	for who, token := range map[string]string{"the owner": a.owner, "the administrator": a.admin} {
		if got := a.shown(t, token, late); got != own {
			t.Errorf("%s's read of the owner's own title: %q", who, got)
		}
	}
	if got := a.shown(t, a.other, late); got != sharedNotFound {
		t.Errorf("another user's read of the owner's own title: %q", got)
	}
	var byIDs QueryResult
	if status := a.get(t, "/Items?Ids="+late, a.other, &byIDs); status != http.StatusOK || len(byIDs.Items) != 0 {
		t.Errorf("another user's /Items?Ids= of the owner's own title: %d %v", status, itemNames(byIDs.Items))
	}

	// The server's library lists it: the title is the server's from then
	// on, as often as the library is listed.
	for range 2 {
		if listed := a.list(t, a.other, "Late Movies"); !slices.Equal(listed, []string{"The Late Movie"}) {
			t.Fatalf("the listing of Late Movies: %v", listed)
		}
		for who, token := range map[string]string{"another user": a.other, "the administrator": a.admin, "the owner": a.owner} {
			if got := a.shown(t, token, late); got != sharedLateMovie {
				t.Errorf("%s's late movie after the server's listing: %q", who, got)
			}
		}
	}
	if _, width := a.poster(t, late, ""); width != 60 {
		t.Errorf("the late movie's poster: %d wide", width)
	}
	if listed := a.list(t, a.child, "Late Movies"); len(listed) != 0 {
		t.Errorf("the restricted user's listing of Late Movies: %v", listed)
	}
	if got := a.shown(t, a.child, late); got != sharedNotFound {
		t.Errorf("the restricted user's late movie: %q", got)
	}
}

// A user's own addon that names the server's titles with their own
// types, "movie" and "series", changes nothing of them either: the
// texts, the ratings and the poster, asked by its tag or without one.
func TestAUsersOwnAddonOfTheSameTypesLeavesTheServersPosters(t *testing.T) {
	t.Parallel()
	a := newSharedTitles(t, "movie", "series")
	movie, show, unopened, late := a.ids["The Real Movie"], a.ids["The Real Show"], a.ids["The Unopened Movie"],
		library.TitleItemID(library.KindMovie, "tt0000003").String()
	_, before := a.details(t, a.other, movie)
	tag := before.ImageTags["Primary"]
	a.list(t, a.owner, "Mine")
	a.search(t, "changed")
	a.demote(t)
	a.list(t, a.other, "Late Movies")
	for who, token := range map[string]string{"another user": a.other, "the administrator": a.admin} {
		if got := a.shown(t, token, movie); got != sharedRealMovie {
			t.Errorf("%s's movie: %q", who, got)
		}
		if got := a.shown(t, token, show); got != sharedRealShow {
			t.Errorf("%s's series: %q", who, got)
		}
		if got := a.shown(t, token, late); got != sharedLateMovie {
			t.Errorf("%s's late movie: %q", who, got)
		}
	}
	for name, id := range map[string]string{"the movie": movie, "the unopened movie": unopened, "the late movie": late} {
		if got := a.shown(t, a.child, id); got != sharedNotFound {
			t.Errorf("the restricted user's details of %s: %q", name, got)
		}
	}
	for name, id := range map[string]string{"the movie": movie, "the late movie": late} {
		if _, untagged := a.poster(t, id, ""); untagged != 60 {
			t.Errorf("the poster of %s without a tag: %d wide", name, untagged)
		}
	}
	if _, tagged := a.poster(t, movie, tag); tagged != 60 {
		t.Errorf("the movie's poster by its tag: %d wide", tagged)
	}
}
