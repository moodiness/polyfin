package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// The playlist requests of Jellyfin's official apps, as each sends them
// (jellyfin-web, the Android TV app, the Kodi add-on and the Android app's
// own browser), answered for every playlist, the Watchlist among them.

// watchlistOf returns the identifier of the Watchlist playlist of token's
// user, empty once they deleted it.
func (s testServer) watchlistOf(t *testing.T, token string) string {
	t.Helper()
	var held WatchlistDto
	if status := s.get(t, "/Polyfin/UserItems/Watchlist", token, &held); status != http.StatusOK {
		t.Fatalf("watchlist: %d", status)
	}
	return held.PlaylistId
}

// listedNames answers the names a listing gives, in order; with entries, each
// item must carry its PlaylistItemId and UserData.Played, which the Kodi
// add-on reads without a guard.
func (s testServer) listedNames(t *testing.T, token, path string, entries bool) []string {
	t.Helper()
	status, body := s.call(http.MethodGet, path, app("tv", token), nil)
	var page struct {
		Items []map[string]any
	}
	if status != http.StatusOK || json.Unmarshal(body, &page) != nil {
		t.Fatalf("%s: %d %s", path, status, body)
	}
	names := make([]string, len(page.Items))
	for i, item := range page.Items {
		names[i], _ = item["Name"].(string)
		data, _ := item["UserData"].(map[string]any)
		if _, played := data["Played"]; entries && (item["PlaylistItemId"] == nil || !played) {
			t.Errorf("%s: %s without PlaylistItemId or UserData.Played: %v", path, names[i], item)
		}
	}
	return names
}

// TestShuffleFollowsSortByOnAPlaylistsChildren is #1050: Shuffle, which
// asks a playlist's children with SortBy=Random, plays them in a new order;
// Play, without SortBy, and the Kodi add-on, with SortBy=None, in the
// playlist's; /Playlists/{id}/Items always lists that order. The same holds
// for the Watchlist and for any playlist.
func TestShuffleFollowsSortByOnAPlaylistsChildren(t *testing.T) {
	t.Parallel()
	s, token, views := browsing(t)
	titles := titlesForPlaylists(t, s, token, views)
	member, _ := s.store.Authenticate(t.Context(), "member", "correct horse")
	user := member.ID.String()
	mix := s.createPlaylist(t, token, map[string]any{"Name": "Mix", "Ids": []string{titles.movies[2], titles.series, titles.movies[0]}})
	watchlist := s.watchlistOf(t, token)
	s.expectStatus(t, http.StatusNoContent, http.MethodPost,
		"/Playlists/"+watchlist+"/Items?ids="+titles.movies[0]+"&ids="+titles.series+"&ids="+titles.movies[2]+"&userId="+user, token, nil)
	tvFields := "fields=CanDelete&fields=ChildCount&fields=MediaSources&fields=PrimaryImageAspectRatio"
	for id, order := range map[string][]string{
		mix:       {"Movie 2", "Pilot", "Second", "Return", "Movie 0"},
		watchlist: {"Movie 0", "Pilot", "Second", "Return", "Movie 2"},
	} {
		play := "/Users/" + user + "/Items?ParentId=" + id + "&Limit=300&Fields=Chapters,Trickplay&ExcludeLocationTypes=Virtual&EnableTotalRecordCount=false&CollapseBoxSetItems=false"
		for _, path := range []string{
			// jellyfin-web's playlist page and Play.
			"/Playlists/" + id + "/Items?Fields=PrimaryImageAspectRatio,MediaSourceCount,Chapters,Trickplay&EnableImageTypes=Primary,Backdrop,Banner,Thumb&UserId=" + user,
			play,
			"/Items?userId=" + user + "&parentId=" + id + "&fields=Chapters,MediaSources,Trickplay&excludeLocationTypes=Virtual&enableTotalRecordCount=false&collapseBoxSetItems=false",
			// The Android TV app's list and Play, without userId.
			"/Playlists/" + id + "/Items?limit=150&" + tvFields,
			"/Items?parentId=" + id + "&isMissing=false&recursive=true&limit=150&" + tvFields,
			// The Kodi add-on, the identifier lowercased.
			"/Users/" + user + "/Items?ParentId=" + strings.ToLower(id) + "&IsMissing=false&Recursive=false&SortBy=None&SortOrder=Ascending&ImageTypeLimit=1&IsVirtualUnaired=false&Fields=DateCreated,Etag,ChildCount",
			// The playlist route takes no sort.
			"/Playlists/" + id + "/Items?sortBy=Random",
		} {
			if got := s.listedNames(t, token, path, true); !slices.Equal(got, order) {
				t.Errorf("%s: %v, want the playlist's order %v", path, got, order)
			}
		}
		for _, path := range []string{
			play + "&SortBy=Random",
			"/Items?parentId=" + id + "&isMissing=false&sortBy=Random&recursive=true&limit=150&" + tvFields,
		} {
			shuffled := false
			for range 30 {
				got := s.listedNames(t, token, path, true)
				if !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(order))) {
					t.Fatalf("%s: %v, want the entries of %v", path, got, order)
				}
				if !slices.Equal(got, order) {
					shuffled = true
					break
				}
			}
			if !shuffled {
				t.Errorf("%s: the playlist's order every time", path)
			}
		}
		// The Android app's own browser asks for names.
		if got := s.listedNames(t, token, "/Items?parentId="+id+"&sortBy=SortName&imageTypeLimit=1&enableImageTypes=Primary&startIndex=1&limit=2", true); !slices.Equal(got, []string{"Movie 2", "Pilot"}) {
			t.Errorf("by name, from the second: %v", got)
		}
	}
}

// specialsAddon serves a series whose first season has an episode not
// aired yet, with a special before it, and a second season.
func specialsAddon(t *testing.T) string {
	t.Helper()
	show := stremio.Meta{ID: "tt0200", Type: "series", Name: "Specials", ReleaseInfo: "2010-", Released: "2010-01-01T00:00:00.000Z", Status: "Continuing",
		Videos: []stremio.Video{
			{ID: "tt0200:0:1", Title: "Special", Season: 0, Episode: 1, Released: "2010-01-01T00:00:00.000Z"},
			{ID: "tt0200:1:1", Title: "One", Season: 1, Episode: 1, Released: "2010-06-01T00:00:00.000Z"},
			{ID: "tt0200:1:2", Title: "Two", Season: 1, Episode: 2, Released: "2010-06-08T00:00:00.000Z"},
			{ID: "tt0200:1:3", Title: "Later", Season: 1, Episode: 3, Released: "2099-06-15T00:00:00.000Z"},
			{ID: "tt0200:2:1", Title: "Three", Season: 2, Episode: 1, Released: "2011-06-01T00:00:00.000Z"},
		}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "specials", Name: "Specials", Version: "1", Types: []string{"series"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "series", ID: "shows", Name: "Shows"}}})
		case strings.HasPrefix(path, "/catalog/series/shows"):
			preview := show
			preview.Videos = nil
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{preview}})
		case path == "/meta/series/tt0200.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": show})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// TestPlaylistsTakeAiredEpisodesOfRegularSeasons checks the rule every
// playlist follows, the Watchlist as any other: a series becomes its aired
// episodes of regular seasons, in order, without its specials or the
// episodes not aired yet; a season, its specials included, its aired
// episodes.
func TestPlaylistsTakeAiredEpisodesOfRegularSeasons(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), specialsAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"}}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views, shows, seasons QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &shows)
	series := shows.Items[0].Id
	s.get(t, "/Shows/"+series+"/Seasons", token, &seasons)
	season := map[int]string{}
	for _, item := range seasons.Items {
		season[*item.IndexNumber] = item.Id
	}
	if season[0] == "" || season[1] == "" {
		t.Fatalf("seasons: %+v", seasons.Items)
	}
	regular := []string{"One", "Two", "Three"}

	mix := s.createPlaylist(t, token, map[string]any{"Name": "Mix", "Ids": []string{series}})
	if got := itemNames(s.playlistEntries(t, token, mix)); !slices.Equal(got, regular) {
		t.Errorf("a series created into a playlist: %v, want %v", got, regular)
	}
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+mix, token, map[string]any{"Ids": []string{season[1], season[0]}})
	if got := itemNames(s.playlistEntries(t, token, mix)); !slices.Equal(got, []string{"One", "Two", "Special"}) {
		t.Errorf("seasons set as a playlist's titles: %v", got)
	}

	watchlist := s.watchlistOf(t, token)
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist+"/Items?ids="+series, token, nil)
	if got := itemNames(s.playlistEntries(t, token, watchlist)); !slices.Equal(got, regular) {
		t.Errorf("a series added to the Watchlist: %v, want %v", got, regular)
	}
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist, token, map[string]any{"Ids": []string{season[1]}})
	if got := itemNames(s.playlistEntries(t, token, watchlist)); !slices.Equal(got, []string{"One", "Two"}) {
		t.Errorf("a season set as the Watchlist's titles: %v", got)
	}
}

// TestTheWatchlistIsAPlaylistAppsChange checks the Watchlist as Jellyfin's
// official apps change it: an add goes at the top unless a position is
// given, and keeps an entry's place; remove, move, replacing through Ids,
// and delete, after which it comes back, empty but for what is added, at
// the next add.
func TestTheWatchlistIsAPlaylistAppsChange(t *testing.T) {
	t.Parallel()
	s, token, views := browsing(t)
	titles := titlesForPlaylists(t, s, token, views)
	watchlist := s.watchlistOf(t, token)
	if watchlist == "" {
		t.Fatal("a member has no Watchlist playlist from the start")
	}
	add := func(query string) {
		t.Helper()
		s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist+"/Items?"+query, token, nil)
	}
	entries := func() []BaseItemDto { t.Helper(); return s.playlistEntries(t, token, watchlist) }
	add("ids=" + titles.movies[0])
	add("ids=" + titles.movies[1])
	if got := itemNames(entries()); !slices.Equal(got, []string{"Movie 1", "Movie 0"}) {
		t.Fatalf("adds: %v, want the latest at the top", got)
	}
	// A position is honored; a title there already keeps its place.
	add("ids=" + titles.movies[2] + "," + titles.movies[0] + "&position=1")
	if got := itemNames(entries()); !slices.Equal(got, []string{"Movie 1", "Movie 2", "Movie 0"}) {
		t.Fatalf("an add at a position: %v", got)
	}
	add("ids=" + titles.series)
	listed := entries()
	if got := itemNames(listed); !slices.Equal(got, []string{"Pilot", "Second", "Return", "Movie 1", "Movie 2", "Movie 0"}) {
		t.Fatalf("a series added: %v", got)
	}

	// jellyfin-web's remove and move, by PlaylistItemId: to the bottom is
	// the count less one.
	s.expectStatus(t, http.StatusNoContent, http.MethodDelete, "/Playlists/"+watchlist+"/Items?EntryIds="+listed[1].PlaylistItemId, token, nil)
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist+"/Items/"+listed[0].PlaylistItemId+"/Move/4", token, nil)
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist+"/Items/"+listed[5].PlaylistItemId+"/Move/0", token, nil)
	if got := itemNames(entries()); !slices.Equal(got, []string{"Movie 0", "Return", "Movie 1", "Movie 2", "Pilot"}) {
		t.Fatalf("after removing and moving: %v", got)
	}

	// Ids replaces every entry.
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist, token, map[string]any{"Ids": []string{titles.movies[2], titles.ep1}})
	if got := itemNames(entries()); !slices.Equal(got, []string{"Movie 2", "Pilot"}) {
		t.Fatalf("replaced through Ids: %v", got)
	}

	// Deleted, it leaves every listing until something is added.
	s.expectStatus(t, http.StatusNoContent, http.MethodDelete, "/Items/"+watchlist, token, nil)
	s.expectStatus(t, http.StatusNotFound, http.MethodGet, "/Playlists/"+watchlist, token, nil)
	if got := s.listedNames(t, token, "/Items?includeItemTypes=Playlist&recursive=true", false); len(got) != 0 {
		t.Errorf("playlists after deleting the Watchlist: %v", got)
	}
	if again := s.watchlistOf(t, token); again != "" {
		t.Errorf("a deleted Watchlist answers %s", again)
	}
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Polyfin/UserItems/"+titles.movies[1]+"/Watchlist", token, nil)
	again := s.watchlistOf(t, token)
	if again == "" || again == watchlist {
		t.Fatalf("the Watchlist after an add: %q", again)
	}
	if got := itemNames(s.playlistEntries(t, token, again)); !slices.Equal(got, []string{"Movie 1"}) {
		t.Errorf("the Watchlist made again: %v", got)
	}
}

// TestTheWatchlistIsNamedAndSharedAsAnyPlaylist checks the Watchlist's
// name, in each viewer's language and with its owner's name for others,
// which jellyfin-web's edit dialog sends back unchanged, and its sharing
// and public access, as any playlist's.
func TestTheWatchlistIsNamedAndSharedAsAnyPlaylist(t *testing.T) {
	t.Parallel()
	s, token, views := browsing(t)
	titles := titlesForPlaylists(t, s, token, views)
	// A French translator makes French the language of the users who
	// ask for it.
	french, err := s.addons.Install(t.Context(), addons.Shared(), catalogAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLanguage(t.Context(), addons.Shared(), french.ID, "fr"); err != nil {
		t.Fatal(err)
	}
	guest := s.user("guest", func(c *accounts.UserChanges) { c.MetadataLanguage = new("fr") })
	guestToken := s.signIn("guest", "phone")
	third := s.user("third", nil)
	thirdToken := s.signIn("third", "laptop")
	watchlist := s.watchlistOf(t, token)
	name := func(token string) string {
		t.Helper()
		var item BaseItemDto
		if status := s.get(t, "/Items/"+watchlist, token, &item); status != http.StatusOK {
			t.Fatalf("the Watchlist: %d", status)
		}
		return item.Name
	}
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist+"/Items?ids="+titles.movies[0], token, nil)

	// Shared to edit, the guest sees it in French, with its owner's name.
	s.expectStatus(t, http.StatusNotFound, http.MethodGet, "/Playlists/"+watchlist+"/Users/"+guest.ID.String(), guestToken, nil)
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist+"/Users/"+guest.ID.String(), token, map[string]any{"CanEdit": true})
	var guestWatchlist string
	for _, playlist := range s.playlistsOf(t, guestToken) {
		if playlist.Id == watchlist {
			guestWatchlist = playlist.Name
		}
	}
	if got := name(token); got != "Watchlist" || guestWatchlist != "À regarder (member)" || name(guestToken) != guestWatchlist {
		t.Errorf("names: %q for its owner, %q and %q for the guest", got, guestWatchlist, name(guestToken))
	}
	var own BaseItemDto
	s.get(t, "/Items/"+s.watchlistOf(t, guestToken), guestToken, &own)
	if own.Name != "À regarder" {
		t.Errorf("the guest's own Watchlist: %q", own.Name)
	}
	// The guest's add is the owner's; the guest may not delete it.
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist+"/Items?ids="+titles.movies[1], guestToken, nil)
	if got := itemNames(s.playlistEntries(t, token, watchlist)); !slices.Equal(got, []string{"Movie 1", "Movie 0"}) {
		t.Errorf("after the guest's add: %v", got)
	}
	var shown BaseItemDto
	s.get(t, "/Users/"+guest.ID.String()+"/Items/"+watchlist, guestToken, &shown)
	if shown.CanDelete == nil || *shown.CanDelete {
		t.Errorf("CanDelete for the guest: %v", shown.CanDelete)
	}
	s.expectStatus(t, http.StatusUnauthorized, http.MethodDelete, "/Items/"+watchlist, guestToken, nil)

	// jellyfin-web's edit dialog sends the name it showed back: the
	// default name stays the default for everyone.
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist, guestToken, map[string]any{"Name": "À regarder (member)", "IsPublic": false})
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist, token, map[string]any{"Name": "Watchlist", "IsPublic": false})
	if got, theirs := name(token), name(guestToken); got != "Watchlist" || theirs != "À regarder (member)" {
		t.Errorf("after saving the edit dialog: %q for its owner, %q for the guest", got, theirs)
	}
	// A name of its own is everyone's, until an empty one.
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist, token, map[string]any{"Name": "Friday", "IsPublic": false})
	if got, theirs := name(token), name(guestToken); got != "Friday" || theirs != "Friday" {
		t.Errorf("renamed: %q for its owner, %q for the guest", got, theirs)
	}
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist, token, map[string]any{"Name": ""})
	if got := name(guestToken); got != "À regarder (member)" {
		t.Errorf("back to the default name: %q for the guest", got)
	}

	// Public, every user sees it, without editing it.
	s.expectStatus(t, http.StatusNotFound, http.MethodGet, "/Playlists/"+watchlist+"/Items", thirdToken, nil)
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist, token, map[string]any{"IsPublic": true})
	var dto PlaylistDto
	s.get(t, "/Playlists/"+watchlist, thirdToken, &dto)
	if !dto.OpenAccess || name(thirdToken) != "Watchlist (member)" {
		t.Errorf("public: %+v, named %q", dto, name(thirdToken))
	}
	s.expectStatus(t, http.StatusForbidden, http.MethodPost, "/Playlists/"+watchlist+"/Items?ids="+titles.movies[2], thirdToken, nil)
	s.expectStatus(t, http.StatusNotFound, http.MethodGet, "/Playlists/"+watchlist+"/Users/"+third.ID.String(), thirdToken, nil)
}

// playlistsOf lists the playlists token's user sees, as the "Add to
// playlist" dialog of jellyfin-web asks.
func (s testServer) playlistsOf(t *testing.T, token string) []BaseItemDto {
	t.Helper()
	var page QueryResult
	if status := s.get(t, "/Items?includeItemTypes=Playlist&sortBy=SortName&recursive=true&enableUserData=false", token, &page); status != http.StatusOK {
		t.Fatalf("playlists: %d", status)
	}
	return page.Items
}

// TestPlaylistsAnswerWhatJellyfinAppsAsk replays the requests of Jellyfin's
// official apps that Polyfin missed: a playlist's favorite and played
// buttons, the Favorites page's and the Android TV app's filters, sorting
// by creation date, searching playlists by name, and the Kodi add-on's
// PlayNow on a playlist.
func TestPlaylistsAnswerWhatJellyfinAppsAsk(t *testing.T) {
	t.Parallel()
	s, token, views := browsing(t)
	titles := titlesForPlaylists(t, s, token, views)
	member, _ := s.store.Authenticate(t.Context(), "member", "correct horse")
	user := member.ID.String()
	watchlist := s.watchlistOf(t, token)
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist+"/Items?ids="+titles.movies[0]+","+titles.movies[1], token, nil)
	mix := s.createPlaylist(t, token, map[string]any{"Name": "Ändern", "Ids": []string{titles.movies[2], titles.ep1}})
	byName := []string{"Ändern", "Watchlist"}
	if got := s.listedNames(t, token, "/Users/"+user+"/Items?SortBy=SortName&SortOrder=Ascending&IncludeItemTypes=Playlist&Recursive=true&Fields=CanDelete&StartIndex=0", false); !slices.Equal(got, byName) {
		t.Errorf("by name, without regard to accents: %v", got)
	}
	if got := s.listedNames(t, token, "/Items?includeItemTypes=Playlist&recursive=true&sortBy=DateCreated&sortOrder=Descending&startIndex=0&limit=60", false); !slices.Equal(got, []string{"Ändern", "Watchlist"}) {
		t.Errorf("latest created first: %v", got)
	}
	if got := s.listedNames(t, token, "/Items?includeItemTypes=Playlist&recursive=true&sortBy=DateCreated", false); !slices.Equal(got, []string{"Watchlist", "Ändern"}) {
		t.Errorf("earliest created first: %v", got)
	}

	// The favorite button of a playlist's page.
	var data UserItemData
	status, body := s.call(http.MethodPost, "/UserFavoriteItems/"+watchlist, app("tv", token), nil)
	if status != http.StatusOK || json.Unmarshal(body, &data) != nil || !data.IsFavorite {
		t.Fatalf("favorite: %d %s", status, body)
	}
	favorites := "/Users/" + user + "/Items?SortBy=SeriesSortName,SortName&SortOrder=Ascending&Filters=IsFavorite&Recursive=true&Fields=PrimaryImageAspectRatio&CollapseBoxSetItems=false&ExcludeLocationTypes=Virtual&EnableTotalRecordCount=false&Limit=20&IncludeItemTypes=Playlist"
	if got := s.listedNames(t, token, favorites, false); !slices.Equal(got, []string{"Watchlist"}) {
		t.Errorf("favorite playlists: %v", got)
	}
	if got := s.listedNames(t, token, "/Users/"+user+"/Items?Filters=IsFavorite&Recursive=true&IncludeItemTypes=Movie", false); len(got) != 0 {
		t.Errorf("favorite movies: %v", got)
	}

	// The played button marks every title, as Jellyfin marks a playlist's
	// children; the Android TV app's filters follow.
	status, body = s.call(http.MethodPost, "/UserPlayedItems/"+mix, app("tv", token), nil)
	if status != http.StatusOK || json.Unmarshal(body, &data) != nil || !data.Played || data.UnplayedItemCount == nil || *data.UnplayedItemCount != 0 {
		t.Fatalf("played: %d %s", status, body)
	}
	var movie BaseItemDto
	s.get(t, "/Items/"+titles.movies[2], token, &movie)
	if !movie.UserData.Played {
		t.Error("a playlist marked played left its movie unplayed")
	}
	grid := "/Items?fields=CanDelete&fields=ChildCount&parentId=" + views["Playlists"] + "&sortBy=SortName&sortOrder=Ascending&startIndex=0&limit=100&filters="
	if played, unplayed := s.listedNames(t, token, grid+"IsPlayed", false), s.listedNames(t, token, grid+"IsUnplayed", false); !slices.Equal(played, []string{"Ändern"}) || !slices.Equal(unplayed, []string{"Watchlist"}) {
		t.Errorf("played %v, unplayed %v", played, unplayed)
	}
	status, body = s.call(http.MethodDelete, "/UserPlayedItems/"+mix, app("tv", token), nil)
	if status != http.StatusOK || json.Unmarshal(body, &data) != nil || data.Played || *data.UnplayedItemCount != 2 {
		t.Errorf("unplayed: %d %s", status, body)
	}

	// Searches find playlists by the name their user sees.
	for term, want := range map[string][]string{"watch": {"Watchlist"}, "andern": {"Ändern"}, "nothing": {}} {
		for _, path := range []string{
			"/Items?userId=" + user + "&recursive=true&includeItemTypes=Playlist&searchTerm=" + term + "&limit=800",
			"/Items?searchTerm=" + term + "&includeItemTypes=Playlist&limit=25&imageTypeLimit=1&fields=CanDelete&fields=ChildCount&recursive=true",
		} {
			if got := s.listedNames(t, token, path, false); !slices.Equal(got, want) {
				t.Errorf("%s: %v, want %v", path, got, want)
			}
		}
	}
	if got := s.listedNames(t, token, "/Items?searchTerm=watch&includeItemTypes=Movie&recursive=true", false); len(got) != 0 {
		t.Errorf("a search for movies: %v", got)
	}

	// The Kodi add-on plays a playlist through PlayNow with its
	// identifier: its session is told the entries.
	kodi := s.remoteApp(t, "member", "kodi", bodyMediaControl, true)
	s.expectStatus(t, http.StatusNoContent, http.MethodPost,
		"/Sessions/"+kodi.session+"/Playing?PlayCommand=PlayNow&ItemIds="+watchlist+"&StartPositionTicks=0", kodi.token, nil)
	var play PlayRequest
	kodi.socket.command(t, "Play", &play)
	if want := []string{titles.movies[0], titles.movies[1]}; !slices.Equal(play.ItemIds, want) {
		t.Errorf("PlayNow on the Watchlist: %v, want %v", play.ItemIds, want)
	}
	if got := s.listedNames(t, token, fmt.Sprintf("/Users/%s/Items?Ids=%s&Fields=Overview", user, strings.Join(play.ItemIds, ",")), false); len(got) != 2 {
		t.Errorf("the entries Kodi then reads: %v", got)
	}
}
