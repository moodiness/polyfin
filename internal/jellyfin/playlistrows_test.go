package jellyfin

import (
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/homerows"
)

// playlistRowsServer is browsing with the member's id, their Watchlist's
// id and name, and the catalog's titles.
func playlistRowsServer(t *testing.T) (s testServer, member string, memberID accounts.ID, watchlist, watchlistName string, titles playlistTitles) {
	t.Helper()
	s, member, views := browsing(t)
	titles = titlesForPlaylists(t, s, member, views)
	var me UserDto
	s.get(t, "/Users/Me", member, &me)
	memberID, _ = accounts.ParseID(me.Id)
	var held WatchlistDto
	s.get(t, "/Polyfin/UserItems/Watchlist", member, &held)
	var described BaseItemDto
	if status := s.get(t, "/Items/"+held.PlaylistId, member, &described); status != http.StatusOK || described.Name == "" {
		t.Fatalf("the Watchlist: %d %+v", status, described)
	}
	return s, member, memberID, held.PlaylistId, described.Name, titles
}

// rowOf returns the row of rows named name.
func rowOf(t *testing.T, rows []CustomRowDto, name string) CustomRowDto {
	t.Helper()
	i := slices.IndexFunc(rows, func(row CustomRowDto) bool { return row.Name == name })
	if i < 0 {
		t.Fatalf("no row %q among %q", name, rowSummary(rows))
	}
	return rows[i]
}

// A Watchlist row lists the user's own Watchlist in its order, named as
// they see it, episodes included; a playlist row lists the chosen playlist
// to the users who may see it, whether they own it, it is shared with them
// or it is open, among those the row is shown to; an empty Watchlist makes
// no row. A row's folder lists the playlist too.
func TestWatchlistAndPlaylistRowsListTheirPlaylists(t *testing.T) {
	t.Parallel()
	s, member, _, watchlist, watchlistName, titles := playlistRowsServer(t)
	guest := s.user("guest", nil)
	guestToken := s.signIn("guest", "phone")
	s.user("outsider", nil)
	outsiderToken := s.signIn("outsider", "laptop")
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist+"/Items?ids="+titles.movies[1]+","+titles.ep1, member, nil)
	chosen := s.createPlaylist(t, member, map[string]any{"Name": "Friday night", "Ids": []string{titles.movies[2], titles.movies[0]}})
	chosenID, _ := accounts.ParseID(chosen)
	s.addRow(t, homerows.Row{Name: "À regarder", Kind: homerows.KindWatchlist, Place: homerows.PlaceTop, Everyone: true})
	row := s.addRow(t, homerows.Row{Name: "Our pick", Kind: homerows.KindPlaylist, Place: homerows.PlaceBottom, Playlist: chosenID,
		Users: []accounts.ID{guest.ID}, Everyone: false})
	s.addRow(t, homerows.Row{Name: "For all", Kind: homerows.KindPlaylist, Place: homerows.PlaceNextUp, Playlist: chosenID, Everyone: true})

	rows := s.customRowsOf(t, member)
	if got := rowSummary(rows); !slices.Equal(got, []string{
		watchlistName + " @Top: Movie 1, " + itemNames(s.playlistEntries(t, member, watchlist))[1],
		"For all @AfterNextUp: Movie 2, Movie 0",
	}) {
		t.Fatalf("the member's rows: %q", got)
	}
	if rows[0].Kind != "Watchlist" || rows[0].PlaylistId != watchlist || rows[1].Kind != "Playlist" || rows[1].PlaylistId != chosen {
		t.Errorf("the rows' kinds and playlists: %+v", rows)
	}
	if episode := rows[0].Items[1]; episode.Type != "Episode" || episode.SeriesName == "" || episode.IndexNumber == nil || episode.ParentIndexNumber == nil {
		t.Errorf("the Watchlist's episode: %+v", episode)
	}
	var listed QueryResult
	if status := s.get(t, "/Items?ParentId="+rows[1].Id, member, &listed); status != http.StatusOK ||
		!slices.Equal(itemNames(listed.Items), []string{"Movie 2", "Movie 0"}) {
		t.Errorf("the playlist row's list: %d %v", status, itemNames(listed.Items))
	}
	var folder BaseItemDto
	if status := s.get(t, "/Items/"+rows[0].Id, member, &folder); status != http.StatusOK || folder.Name != watchlistName {
		t.Errorf("the Watchlist row's folder: %d %+v", status, folder)
	}

	// The guest's Watchlist is empty, and the playlist private: no row.
	if got := rowSummary(s.customRowsOf(t, guestToken)); len(got) != 0 {
		t.Errorf("the guest's rows: %q", got)
	}
	if status := s.get(t, "/Items?ParentId="+customRowID(row.ID, accounts.ID{}).String(), guestToken, nil); status == http.StatusOK {
		t.Errorf("the private playlist's row listed to the guest: %d", status)
	}
	// Shared with the guest, both rows show to them; the outsider, whom
	// one row is not for, sees the other once the playlist is open.
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+chosen+"/Users/"+guest.ID.String(), member, map[string]any{"CanEdit": false})
	if got := rowSummary(s.customRowsOf(t, guestToken)); !slices.Equal(got, []string{"Our pick @Bottom: Movie 2, Movie 0",
		"For all @AfterNextUp: Movie 2, Movie 0"}) {
		t.Errorf("the guest's rows once shared: %q", got)
	}
	if got := rowSummary(s.customRowsOf(t, outsiderToken)); len(got) != 0 {
		t.Errorf("the outsider's rows: %q", got)
	}
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+chosen, member, map[string]any{"IsPublic": true})
	if got := rowSummary(s.customRowsOf(t, outsiderToken)); !slices.Equal(got, []string{"For all @AfterNextUp: Movie 2, Movie 0"}) {
		t.Errorf("the outsider's rows once open: %q", got)
	}

	// A renamed Watchlist names its row; deleting the playlist deletes
	// its rows.
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist, member, map[string]any{"Name": "Mine"})
	if got := rowSummary(s.customRowsOf(t, member)); len(got) != 2 || got[0] != "Mine @Top: Movie 1, "+itemNames(rows[0].Items)[1] {
		t.Errorf("after the rename: %q", got)
	}
	s.expectStatus(t, http.StatusNoContent, http.MethodDelete, "/Items/"+chosen, member, nil)
	if _, err := homerows.New(s.pool).Row(t.Context(), row.ID); !errors.Is(err, homerows.ErrNotFound) {
		t.Errorf("the row of the deleted playlist: %v", err)
	}
	if got := rowSummary(s.customRowsOf(t, member)); len(got) != 1 {
		t.Errorf("after the deletion: %q", got)
	}
}

// A My playlists row makes a row for each playlist the user may see and
// made a favorite, up to its count, by name accents aside, each named
// after its playlist; the rows follow the hearts and the playlists'
// access, and the Watchlist shows there only while no Watchlist row does.
func TestFavoritePlaylistRowsFollowTheHearts(t *testing.T) {
	t.Parallel()
	s, member, memberID, watchlist, watchlistName, titles := playlistRowsServer(t)
	s.user("guest", nil)
	guestToken := s.signIn("guest", "phone")
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist+"/Items?ids="+titles.movies[0], member, nil)
	one := []string{titles.movies[1]}
	alpha := s.createPlaylist(t, member, map[string]any{"Name": "alpha", "Ids": one})
	ete := s.createPlaylist(t, member, map[string]any{"Name": "Été", "Ids": one})
	zed := s.createPlaylist(t, member, map[string]any{"Name": "Zed", "Ids": one})
	empty := s.createPlaylist(t, member, map[string]any{"Name": "Empty"})
	beta := s.createPlaylist(t, guestToken, map[string]any{"Name": "Beta", "Ids": []string{titles.movies[2]}, "IsPublic": true})
	store := homerows.New(s.pool)
	watchlistRow := s.addRow(t, homerows.Row{Name: "À regarder", Kind: homerows.KindWatchlist, Place: homerows.PlaceTop, Everyone: true})
	s.addRow(t, homerows.Row{Name: "My playlists", Kind: homerows.KindPlaylists, Place: homerows.PlaceBottom, Count: 3, Everyone: true})
	favorites := func() []string {
		var names []string
		for _, row := range s.customRowsOf(t, member) {
			if row.Kind == "FavoritePlaylists" {
				names = append(names, row.Name)
			}
		}
		return names
	}
	if got := favorites(); len(got) != 0 {
		t.Fatalf("before any heart: %q", got)
	}
	for _, id := range []string{alpha, ete, zed, empty, beta, watchlist} {
		s.expectStatus(t, http.StatusOK, http.MethodPost, "/UserFavoriteItems/"+id, member, nil)
	}
	if got := favorites(); !slices.Equal(got, []string{"alpha", "Beta", "Été"}) {
		t.Errorf("with the hearts: %q", got)
	}
	rows := s.customRowsOf(t, member)
	beta0 := rowOf(t, rows, "Beta")
	if beta0.PlaylistId != beta || len(beta0.Items) != 1 || beta0.Items[0].Name != "Movie 2" {
		t.Errorf("the open playlist's row: %+v", beta0)
	}
	var listed QueryResult
	if status := s.get(t, "/Items?ParentId="+beta0.Id, member, &listed); status != http.StatusOK || !slices.Equal(itemNames(listed.Items), []string{"Movie 2"}) {
		t.Errorf("the favorite's row list: %d %v", status, itemNames(listed.Items))
	}
	// Closed again, the guest's playlist leaves the member's rows.
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+beta, guestToken, map[string]any{"IsPublic": false})
	if got := favorites(); !slices.Equal(got, []string{"alpha", "Été", "Zed"}) {
		t.Errorf("once Beta is private: %q", got)
	}
	if status := s.get(t, "/Items?ParentId="+beta0.Id, member, nil); status == http.StatusOK {
		t.Errorf("the closed playlist's row still listed")
	}
	s.expectStatus(t, http.StatusOK, http.MethodDelete, "/UserFavoriteItems/"+alpha, member, nil)
	if got := favorites(); !slices.Equal(got, []string{"Été", "Zed"}) {
		t.Errorf("without the heart on alpha: %q", got)
	}
	// With the Watchlist row turned off, the Watchlist shows among them.
	if err := store.SetOff(t.Context(), memberID, watchlistRow.ID, true); err != nil {
		t.Fatal(err)
	}
	if got := favorites(); !slices.Equal(got, []string{"Été", watchlistName, "Zed"}) {
		t.Errorf("with the Watchlist row off: %q", got)
	}
	// Another user's hearts are their own.
	if got := rowSummary(s.customRowsOf(t, guestToken)); len(got) != 0 {
		t.Errorf("the guest's rows: %q", got)
	}
}

// Rows of playlists list what /Playlists/{id}/Items lists to the user:
// without the titles parental control keeps from them, and, for the
// Watchlist, those they hid.
func TestPlaylistRowsLeaveOutWhatTheUserMayNotSee(t *testing.T) {
	t.Parallel()
	s, member, _, watchlist, _, titles := playlistRowsServer(t)
	s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(13)} })
	child := s.signIn("child", "tablet")
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist+"/Items?ids="+titles.movies[0]+","+titles.movies[1], member, nil)
	open := s.createPlaylist(t, member, map[string]any{"Name": "Open", "Ids": []string{titles.ep1, titles.movies[2]}, "IsPublic": true})
	openID, _ := accounts.ParseID(open)
	s.addRow(t, homerows.Row{Name: "À regarder", Kind: homerows.KindWatchlist, Place: homerows.PlaceTop, Everyone: true})
	s.addRow(t, homerows.Row{Name: "Open", Kind: homerows.KindPlaylist, Place: homerows.PlaceBottom, Playlist: openID, Everyone: true})
	// The series is rated TV-14, above the child's limit.
	if got := itemNames(rowOf(t, s.customRowsOf(t, child), "Open").Items); !slices.Equal(got, []string{"Movie 2"}) ||
		!slices.Equal(got, itemNames(s.playlistEntries(t, child, open))) {
		t.Errorf("the child's row: %v", got)
	}
	if status, _ := s.call(http.MethodPost, "/Polyfin/UserItems/"+titles.movies[0]+"/Hidden", app("tv", member), nil); status != http.StatusNoContent {
		t.Fatalf("hiding: %d", status)
	}
	rows := s.customRowsOf(t, member)
	if got := itemNames(rows[0].Items); !slices.Equal(got, []string{"Movie 1"}) || !slices.Equal(got, itemNames(s.playlistEntries(t, member, watchlist))) {
		t.Errorf("the Watchlist row with Movie 0 hidden: %v", got)
	}
}

// A Watchlist or playlist row also a library is a view of /UserViews for
// the users who get it, listing the playlist, whose first entries are its
// latest items.
func TestPlaylistRowsAlsoLibraries(t *testing.T) {
	t.Parallel()
	s, member, _, watchlist, watchlistName, titles := playlistRowsServer(t)
	s.user("guest", nil)
	guestToken := s.signIn("guest", "phone")
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+watchlist+"/Items?ids="+titles.movies[1]+","+titles.movies[0], member, nil)
	chosen := s.createPlaylist(t, member, map[string]any{"Name": "Friday night", "Ids": []string{titles.movies[2]}})
	chosenID, _ := accounts.ParseID(chosen)
	s.addRow(t, homerows.Row{Name: "À regarder", Kind: homerows.KindWatchlist, Place: homerows.PlaceTop, Everyone: true, Library: true})
	s.addRow(t, homerows.Row{Name: "Friday", Kind: homerows.KindPlaylist, Place: homerows.PlaceTop, Playlist: chosenID, Everyone: true, Library: true})
	var views QueryResult
	s.get(t, "/UserViews", member, &views)
	ids := map[string]string{}
	for _, view := range views.Items {
		ids[view.Name] = view.Id
	}
	if ids[watchlistName] == "" || ids["Friday"] == "" {
		t.Fatalf("the member's views: %v", itemNames(views.Items))
	}
	var listed QueryResult
	if status := s.get(t, "/Items?ParentId="+ids[watchlistName], member, &listed); status != http.StatusOK ||
		!slices.Equal(itemNames(listed.Items), []string{"Movie 1", "Movie 0"}) {
		t.Errorf("the Watchlist library: %d %v", status, itemNames(listed.Items))
	}
	var latest []BaseItemDto
	if status := s.get(t, "/Items/Latest?ParentId="+ids["Friday"], member, &latest); status != http.StatusOK ||
		!slices.Equal(itemNames(latest), []string{"Movie 2"}) {
		t.Errorf("the playlist library's latest: %d %v", status, itemNames(latest))
	}
	// The guest, who may not see the playlist, gets its library no more
	// than its row.
	s.get(t, "/UserViews", guestToken, &views)
	if got := itemNames(views.Items); slices.Contains(got, "Friday") {
		t.Errorf("the guest's views: %v", got)
	}
}
