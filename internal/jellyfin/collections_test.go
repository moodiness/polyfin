package jellyfin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// groupedAddon serves three movies, each with a poster of its own and a
// rating — Kids (G, animation, 2001), Teens (PG-13, action, 1999) and Adults
// (R, drama, 2005) — and Show, a TV-14 drama series of two episodes from
// 2010. Each poster's bytes name it.
func groupedAddon(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	meta := func(id string) stremio.Meta {
		m := map[string]stremio.Meta{
			"tt1": {Name: "Kids", Genres: []string{"Animation"}, ReleaseInfo: "2001", Released: "2001-06-01T00:00:00.000Z",
				Extras: &stremio.Extras{Certification: "G"}},
			"tt2": {Name: "Teens", Genres: []string{"Action"}, ReleaseInfo: "1999", Released: "1999-03-01T00:00:00.000Z",
				Extras: &stremio.Extras{Certification: "PG-13"}},
			"tt3": {Name: "Adults", Genres: []string{"Drama"}, ReleaseInfo: "2005", Released: "2005-09-01T00:00:00.000Z",
				Extras: &stremio.Extras{Certification: "R"}},
			"tt4": {Type: "series", Name: "Show", Genres: []string{"Drama"}, ReleaseInfo: "2010", Released: "2010-01-01T00:00:00.000Z",
				Extras: &stremio.Extras{Certification: "TV-14"}, Videos: []stremio.Video{
					{ID: "tt4:1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2010-01-01T00:00:00.000Z"},
					{ID: "tt4:1:2", Title: "Second", Season: 1, Episode: 2, Released: "2010-01-08T00:00:00.000Z"},
				}},
		}[id]
		m.ID, m.Poster = id, server.URL+"/poster/"+id+".jpg"
		if m.Type == "" {
			m.Type = "movie"
		}
		return m
	}
	row := func(id string) stremio.Meta {
		m := meta(id)
		m.Extras, m.Videos = nil, nil
		return m
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "grouped", Name: "Grouped", Version: "1",
				Types: []string{"movie", "series"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}, {Type: "series", ID: "shows", Name: "Shows"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{row("tt1"), row("tt2"), row("tt3")}})
		case strings.HasPrefix(path, "/catalog/series/shows"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{row("tt4")}})
		case len(parts) == 3 && parts[0] == "meta":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta(parts[2])})
		case len(parts) == 2 && parts[0] == "poster":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("poster of " + strings.TrimSuffix(parts[1], ".jpg")))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// groupedTitles are the identifiers of the titles of groupedAddon, and of
// its libraries.
type groupedTitles struct {
	kids, teens, adults, show string
	movies, shows             string
}

// grouping installs collectionsAddon and creates admin, an administrator
// who may manage collections, as Jellyfin's first administrator may.
func grouping(t *testing.T) (testServer, string, groupedTitles) {
	t.Helper()
	s := newTestServer(t, 10)
	if _, err := s.store.CreateUser(t.Context(), accounts.NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true}); err != nil {
		t.Fatal(err)
	}
	addon, err := s.addons.Install(t.Context(), addons.Shared(), groupedAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "top"},
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"},
	}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("admin", "tv")
	var titles groupedTitles
	views := s.viewIDs(t, token)
	titles.movies, titles.shows = views["Top"], views["Shows"]
	var page QueryResult
	s.get(t, "/Items?ParentId="+titles.movies, token, &page)
	for _, item := range page.Items {
		switch item.Name {
		case "Kids":
			titles.kids = item.Id
		case "Teens":
			titles.teens = item.Id
		case "Adults":
			titles.adults = item.Id
		}
	}
	s.get(t, "/Items?ParentId="+titles.shows, token, &page)
	if titles.kids == "" || titles.teens == "" || titles.adults == "" || len(page.Items) != 1 {
		t.Fatalf("titles: %+v %v", titles, itemNames(page.Items))
	}
	titles.show = page.Items[0].Id
	return s, token, titles
}

// viewIDs maps the names of the views of the user token signs in to their
// identifiers.
func (s testServer) viewIDs(t *testing.T, token string) map[string]string {
	t.Helper()
	var views QueryResult
	if status := s.get(t, "/UserViews", token, &views); status != http.StatusOK {
		t.Fatalf("views: %d", status)
	}
	ids := map[string]string{}
	for _, view := range views.Items {
		ids[view.Name] = view.Id
	}
	return ids
}

// createCollection creates a collection and returns its identifier.
func (s testServer) createCollection(t *testing.T, token, query string) string {
	t.Helper()
	status, body := s.call(http.MethodPost, "/Collections?"+query, app("tv", token), nil)
	var created map[string]any
	if status != http.StatusOK || json.Unmarshal(body, &created) != nil || len(created) != 1 {
		t.Fatalf("collection creation: %d %s", status, body)
	}
	id, _ := created["Id"].(string)
	if len(id) != 32 {
		t.Fatalf("collection identifier: %s", body)
	}
	return id
}

// collectionShapes lists what Polyfin cannot match in the collection
// fixtures, recorded from a collection of three movies without artwork.
var collectionShapes = shapeRules{
	dynamic: []string{"ImageTags", "ImageBlurHashes", "ProviderIds"},
	absent: map[string][]string{
		// Polyfin has no files; the test addon has no streams, and its
		// titles no runtime, community rating or language.
		"Path": {"boxset"}, "Container": {"boxset-children"}, "OriginalLanguage": {"boxset-children"},
		"RunTimeTicks": {"boxset-children"}, "CommunityRating": {"boxset-children"},
	},
	returned: map[string][]string{
		// The recorded collection had no artwork; Polyfin's shows the
		// poster of its first title.
		"PrimaryImageAspectRatio": {"boxsets", "boxset"},
	},
}

func TestCollectionsAreMadeChangedAndDeletedAsInJellyfin(t *testing.T) {
	s, admin, titles := grouping(t)
	member := s.user("member", nil)
	memberToken := s.signIn("member", "phone")
	if _, ok := s.viewIDs(t, memberToken)["Collections"]; ok {
		t.Fatal("the Collections view shows before any collection exists")
	}

	// jellyfin-web creates a collection from the titles picked, with a
	// parentId Jellyfin ignores.
	id := s.createCollection(t, admin, "Name=Picks&IsLocked=true&Ids="+titles.teens+","+titles.kids+"&parentId="+randomID().String())
	view, ok := s.viewIDs(t, memberToken)["Collections"]
	if !ok {
		t.Fatal("no Collections view for another user once a collection exists")
	}
	var folder BaseItemDto
	if s.get(t, "/Items/"+view, memberToken, &folder); folder.CollectionType != "boxsets" || folder.Type != "CollectionFolder" || !folder.IsFolder {
		t.Errorf("Collections view: %+v", folder)
	}

	user := member.ID.String()
	counts := "Fields=ItemCounts,PrimaryImageAspectRatio,CanDelete,MediaSourceCount"
	for fixture, path := range map[string]string{
		"boxsets":         "/Users/" + user + "/Items?ParentId=" + view + "&" + counts,
		"boxset":          "/Users/" + user + "/Items/" + id,
		"boxset-children": "/Users/" + user + "/Items?ParentId=" + id + "&" + counts,
	} {
		status, body := s.call(http.MethodGet, path, app("phone", memberToken), nil)
		if status != http.StatusOK {
			t.Fatalf("%s: %d %s", fixture, status, body)
		}
		raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.2", fixture+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var want, got any
		if err := json.Unmarshal(raw, &want); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		for _, difference := range compareShapes(fixture, want, got, collectionShapes) {
			t.Error(difference)
		}
	}

	children := func(query string) []string {
		t.Helper()
		var page QueryResult
		if status := s.get(t, "/Items?ParentId="+id+query, memberToken, &page); status != http.StatusOK || page.TotalRecordCount != len(page.Items) {
			t.Fatalf("children: %d, %d of %d", status, len(page.Items), page.TotalRecordCount)
		}
		return itemNames(page.Items)
	}
	// Like Jellyfin's, a collection lists its titles by premiere date.
	if got := children(""); !slices.Equal(got, []string{"Teens", "Kids"}) {
		t.Errorf("created with: %v", got)
	}
	// Titles already in the collection keep their place.
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Collections/"+id+"/Items?ids="+titles.show+","+titles.kids, admin, nil)
	if got := children(""); !slices.Equal(got, []string{"Teens", "Kids", "Show"}) {
		t.Errorf("after adding: %v", got)
	}
	// Played as apps play a collection, a series brings its episodes.
	if got := children("&Recursive=true&Filters=IsNotFolder&MediaTypes=Video"); !slices.Equal(got, []string{"Teens", "Kids", "Pilot", "Second"}) {
		t.Errorf("played: %v", got)
	}
	if got := children("&IncludeItemTypes=Series"); !slices.Equal(got, []string{"Show"}) {
		t.Errorf("series only: %v", got)
	}
	var detail BaseItemDto
	s.get(t, "/Items/"+id, memberToken, &detail)
	if detail.Type != "BoxSet" || detail.Name != "Picks" || !detail.IsFolder || detail.ChildCount == nil || *detail.ChildCount != 3 ||
		detail.RecursiveItemCount == nil || *detail.RecursiveItemCount != 4 || detail.CanDelete == nil || *detail.CanDelete ||
		detail.ParentId != view || detail.OfficialRating != "G" || detail.ProductionYear == nil || *detail.ProductionYear != 1999 ||
		detail.Genres == nil || !slices.Equal(*detail.Genres, []string{"Action", "Animation", "Drama"}) || detail.DisplayOrder != "PremiereDate" {
		t.Errorf("collection: %+v", detail)
	}
	if s.get(t, "/Items/"+id, admin, &detail); detail.CanDelete == nil || !*detail.CanDelete {
		t.Errorf("an administrator cannot delete it: %v", detail.CanDelete)
	}
	var ancestors []BaseItemDto
	if s.get(t, "/Items/"+id+"/Ancestors", memberToken, &ancestors); len(ancestors) != 1 || ancestors[0].Id != view {
		t.Errorf("ancestors: %v", itemNames(ancestors))
	}

	// Jellyfin answers an unknown collection or item with a 400 and adds
	// nothing; values that are not identifiers are left out.
	refused := func(method, path string) {
		t.Helper()
		status, body := s.call(method, path, app("tv", admin), nil)
		if status != http.StatusBadRequest || string(body) != "Error processing request." {
			t.Errorf("%s %s: %d %s", method, path, status, body)
		}
	}
	refused(http.MethodPost, "/Collections/"+id+"/Items?ids="+titles.adults+","+randomID().String())
	refused(http.MethodPost, "/Collections/"+randomID().String()+"/Items?ids="+titles.adults)
	refused(http.MethodDelete, "/Collections/"+randomID().String()+"/Items?ids="+titles.adults)
	refused(http.MethodPost, "/Collections?ids="+titles.adults)
	if got := children(""); len(got) != 3 {
		t.Errorf("refused changes changed the collection: %v", got)
	}
	if status, body := s.call(http.MethodPost, "/Collections/not-an-id/Items?ids="+titles.adults, app("tv", admin), nil); status != http.StatusBadRequest ||
		!strings.Contains(string(body), "collectionId") {
		t.Errorf("collection that is not an identifier: %d %s", status, body)
	}
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Collections/"+id+"/Items?ids=nope", admin, nil)

	// Titles the collection does not have are ignored.
	s.expectStatus(t, http.StatusNoContent, http.MethodDelete, "/Collections/"+id+"/Items?ids="+titles.teens+","+titles.adults, admin, nil)
	if got := children(""); !slices.Equal(got, []string{"Kids", "Show"}) {
		t.Errorf("after removing: %v", got)
	}

	// jellyfin-web lists the collections to add to across the server; its
	// libraries' Collections tab lists those with titles from them.
	other := s.createCollection(t, admin, "name=Animated&ids="+titles.kids)
	listed := func(query string) []string {
		t.Helper()
		var page QueryResult
		if status := s.get(t, "/Users/"+user+"/Items?Recursive=true&IncludeItemTypes=BoxSet&SortBy=SortName"+query, memberToken, &page); status != http.StatusOK {
			t.Fatalf("collections: %d", status)
		}
		return itemNames(page.Items)
	}
	if got := listed(""); !slices.Equal(got, []string{"Animated", "Picks"}) {
		t.Errorf("collections: %v", got)
	}
	if got := listed("&ParentId=" + titles.shows); !slices.Equal(got, []string{"Picks"}) {
		t.Errorf("collections with shows: %v", got)
	}
	if got := listed("&ParentId=" + titles.movies); !slices.Equal(got, []string{"Animated", "Picks"}) {
		t.Errorf("collections with movies: %v", got)
	}
	if got := listed("&ParentId=" + view + "&StartIndex=1&Limit=1"); !slices.Equal(got, []string{"Picks"}) {
		t.Errorf("a page of the Collections view: %v", got)
	}

	// Jellyfin deletes a collection as an item.
	s.expectStatus(t, http.StatusNoContent, http.MethodDelete, "/Items/"+id, admin, nil)
	if status := s.get(t, "/Items/"+id, memberToken, nil); status != http.StatusNotFound {
		t.Errorf("deleted collection: %d", status)
	}
	if _, ok := s.viewIDs(t, memberToken)["Collections"]; !ok {
		t.Error("the Collections view left while a collection remains")
	}
	s.expectStatus(t, http.StatusNoContent, http.MethodDelete, "/Items/"+other, admin, nil)
	if _, ok := s.viewIDs(t, memberToken)["Collections"]; ok {
		t.Error("the Collections view stays once no collection is left")
	}
}

func TestCollectionsNeedThePermissionToManageThem(t *testing.T) {
	s, admin, titles := grouping(t)
	id := s.createCollection(t, admin, "name=Picks&ids="+titles.kids)
	member := s.user("member", nil)
	memberToken := s.signIn("member", "phone")

	// Like Jellyfin's CollectionManagement policy, an empty 403.
	forbidden := func(token, method, path string) {
		t.Helper()
		if status, body := s.call(method, path, app("phone", token), nil); status != http.StatusForbidden || len(body) != 0 {
			t.Errorf("%s %s: %d %q", method, path, status, body)
		}
	}
	forbidden(memberToken, http.MethodPost, "/Collections?name=Mine&ids="+titles.kids)
	forbidden(memberToken, http.MethodPost, "/Collections/"+id+"/Items?ids="+titles.teens)
	forbidden(memberToken, http.MethodDelete, "/Collections/"+id+"/Items?ids="+titles.kids)
	// Deleting an item is Jellyfin's DELETE /Items, which answers 401.
	if status, body := s.call(http.MethodDelete, "/Items/"+id, app("phone", memberToken), nil); status != http.StatusUnauthorized ||
		string(body) != "\"Unauthorized access\"\n" {
		t.Errorf("member deleting: %d %s", status, body)
	}

	// An administrator's app grants it in the user's policy.
	var dto map[string]any
	s.get(t, "/Users/"+member.ID.String(), admin, &dto)
	policy := dto["Policy"].(map[string]any)
	if policy["EnableCollectionManagement"] != false {
		t.Fatalf("a new user's policy: %v", policy["EnableCollectionManagement"])
	}
	policy["EnableCollectionManagement"] = true
	encoded, _ := json.Marshal(policy)
	if status, body := s.postRaw("/Users/"+member.ID.String()+"/Policy", app("tv", admin), string(encoded)); status != http.StatusNoContent {
		t.Fatalf("policy: %d %s", status, body)
	}
	if stored, _ := s.store.User(t.Context(), member.ID); !stored.CollectionManagement {
		t.Error("the policy did not grant it")
	}
	s.get(t, "/Users/Me", memberToken, &dto)
	if dto["Policy"].(map[string]any)["EnableCollectionManagement"] != true {
		t.Error("the user's own policy does not show it")
	}
	s.createCollection(t, memberToken, "name=Mine&ids="+titles.teens)
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Collections/"+id+"/Items?ids="+titles.teens, memberToken, nil)

	// Like Jellyfin, an administrator without it is refused the
	// collection routes, but may still delete one.
	administrator, _ := s.store.Authenticate(t.Context(), "admin", "correct horse")
	s.limit(t, administrator, func(c *accounts.UserChanges) { c.CollectionManagement = new(false) })
	forbidden(admin, http.MethodPost, "/Collections?name=Other")
	s.expectStatus(t, http.StatusNoContent, http.MethodDelete, "/Items/"+id, admin, nil)
}

func TestCollectionsShowEachUserTheTitlesTheyMaySee(t *testing.T) {
	s, admin, titles := grouping(t)
	id := s.createCollection(t, admin, "name=Picks&ids="+strings.Join([]string{titles.teens, titles.kids, titles.adults, titles.show}, ","))
	s.user("member", nil)
	s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(10)} })
	s.user("calm", func(c *accounts.UserChanges) { c.BlockedGenres = &[]string{"action"} })

	poster := func(tag string) string {
		t.Helper()
		response, err := http.Get(s.url + "/Items/" + id + "/Images/Primary?tag=" + tag)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("poster: %d %s", response.StatusCode, body)
		}
		return string(body)
	}
	for _, tc := range []struct {
		user, device string
		titles       []string
		episodes     int
		poster       string
	}{
		{"member", "phone", []string{"Teens", "Kids", "Adults", "Show"}, 2, "poster of tt2"},
		// Parental control and blocked genres hide titles in collections as
		// everywhere: they neither show nor count.
		{"child", "tablet", []string{"Kids"}, 0, "poster of tt1"},
		{"calm", "laptop", []string{"Kids", "Adults", "Show"}, 2, "poster of tt1"},
	} {
		token := s.signIn(tc.user, tc.device)
		view, ok := s.viewIDs(t, token)["Collections"]
		if !ok {
			t.Errorf("%s: no Collections view", tc.user)
			continue
		}
		var page QueryResult
		s.get(t, "/Items?ParentId="+view+"&Fields=ChildCount,RecursiveItemCount", token, &page)
		if len(page.Items) != 1 || page.Items[0].Id != id {
			t.Errorf("%s: collections %v", tc.user, itemNames(page.Items))
			continue
		}
		listed := page.Items[0]
		movies := len(tc.titles) - min(tc.episodes, 1)
		if listed.ChildCount == nil || *listed.ChildCount != len(tc.titles) || listed.RecursiveItemCount == nil ||
			*listed.RecursiveItemCount != movies+tc.episodes || listed.UserData.UnplayedItemCount == nil ||
			*listed.UserData.UnplayedItemCount != movies+tc.episodes {
			t.Errorf("%s: counts %v %v %v", tc.user, listed.ChildCount, listed.RecursiveItemCount, listed.UserData.UnplayedItemCount)
		}
		// By premiere date: Teens came out first.
		s.get(t, "/Items?ParentId="+id, token, &page)
		if got := itemNames(page.Items); !slices.Equal(got, tc.titles) || page.TotalRecordCount != len(tc.titles) {
			t.Errorf("%s: titles %v of %d", tc.user, got, page.TotalRecordCount)
		}
		// The poster is that of the first title the user sees.
		if got := poster(listed.ImageTags["Primary"]); got != tc.poster {
			t.Errorf("%s: poster %q", tc.user, got)
		}
	}
	// Without a tag, the collection shows its first title's poster.
	if got := poster(""); got != "poster of tt2" {
		t.Errorf("untagged poster: %q", got)
	}

	// Played titles count as Jellyfin counts a folder's.
	member, _ := s.store.Authenticate(t.Context(), "member", "correct horse")
	token := s.signIn("member", "phone")
	s.expectStatus(t, http.StatusOK, http.MethodPost, "/UserPlayedItems/"+titles.kids, token, nil)
	var detail BaseItemDto
	s.get(t, "/Users/"+member.ID.String()+"/Items/"+id, token, &detail)
	if data := detail.UserData; data.UnplayedItemCount == nil || *data.UnplayedItemCount != 4 || data.PlayedPercentage == nil ||
		*data.PlayedPercentage != 20 || data.Played {
		t.Errorf("after playing a title: %+v", data)
	}
}

// jellyfin-web opens a playlist or a collection like any item: its page
// asks for its ancestors, theme media, similar titles and collections, and
// its filters, as the Playlists and Collections views' pages ask theirs.
// Jellyfin answers each, empty where it has nothing.
func TestOwnFoldersAnswerItemPages(t *testing.T) {
	s, token, titles := grouping(t)
	playlist := s.createPlaylist(t, token, map[string]any{"Name": "Mix", "Ids": []string{titles.kids}})
	collection := s.createCollection(t, token, "name=Saga&ids="+titles.teens)
	views := s.viewIDs(t, token)
	if views["Playlists"] == "" || views["Collections"] == "" {
		t.Fatalf("views: %v", views)
	}
	for _, id := range []string{playlist, collection} {
		for _, path := range []string{"/Items/" + id + "/ThemeMedia?inheritFromParent=true&sortBy=Random",
			"/Items/" + id + "/Similar?limit=12&fields=PrimaryImageAspectRatio,CanDelete",
			"/Items/" + id + "/Collections?fields=PrimaryImageAspectRatio"} {
			if status, body := s.call(http.MethodGet, path, app("web", token), nil); status != http.StatusOK {
				t.Errorf("%s: %d %s", path, status, body)
			}
		}
	}
	for id, want := range map[string]string{playlist: "Playlists", collection: "Collections"} {
		var ancestors []BaseItemDto
		if status := s.get(t, "/Items/"+id+"/Ancestors", token, &ancestors); status != http.StatusOK ||
			len(ancestors) != 1 || ancestors[0].Name != want {
			t.Errorf("ancestors of a %s item: %d %+v", want, status, ancestors)
		}
	}
	for _, id := range []string{playlist, collection, views["Playlists"], views["Collections"]} {
		for _, path := range []string{"/Items/Filters?parentId=" + id, "/Items/Filters2?parentId=" + id + "&includeItemTypes=BoxSet"} {
			if status, body := s.call(http.MethodGet, path, app("web", token), nil); status != http.StatusOK {
				t.Errorf("%s: %d %s", path, status, body)
			}
		}
	}
	// What does not exist is still refused.
	unknown := randomID().String()
	s.expectStatus(t, http.StatusNotFound, http.MethodGet, "/Items/"+unknown+"/ThemeMedia", token, nil)
	s.expectStatus(t, http.StatusBadRequest, http.MethodGet, "/Items/Filters?parentId="+unknown, token, nil)
}
