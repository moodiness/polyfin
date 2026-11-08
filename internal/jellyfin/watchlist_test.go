package jellyfin

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// watchlistStore is a watchlist store of s's database whose titles become
// entries as the server's do: a movie or an episode itself, a series its
// aired episodes of regular seasons.
func (s testServer) watchlistStore() *watchlist.Store {
	return watchlist.New(s.pool).WithEntries(func(ctx context.Context, id, title accounts.ID) (watchlist.Made, error) {
		user, err := s.store.User(ctx, id)
		if err != nil {
			return watchlist.Made{}, err
		}
		return s.library.WatchlistEntries(ctx, user, title)
	})
}

// libraryViews leaves out of views the Playlists view, which every user
// gets for their watchlist playlist.
func libraryViews(views []BaseItemDto) []BaseItemDto {
	return slices.DeleteFunc(slices.Clone(views), func(v BaseItemDto) bool { return v.CollectionType == "playlists" })
}

// airingWatchlist is a member signed in to a server of airingAddon's
// titles: its series by name, their episodes by name, and the movie.
type airingWatchlist struct {
	testServer
	token, movie     string
	series, episodes map[string]string
	member           accounts.User
}

func watchingAiring(t *testing.T) airingWatchlist {
	t.Helper()
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), airingAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "movies"},
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "tonight"},
	}); err != nil {
		t.Fatal(err)
	}
	a := airingWatchlist{testServer: s, token: s.signIn("member", "tv"), member: member, series: map[string]string{}, episodes: map[string]string{}}
	views := s.viewIDs(t, a.token)
	var page QueryResult
	s.get(t, "/Items?ParentId="+views["Movies"], a.token, &page)
	if len(page.Items) != 1 {
		t.Fatalf("movies: %v", itemNames(page.Items))
	}
	a.movie = page.Items[0].Id
	s.get(t, "/Items?ParentId="+views["Tonight"], a.token, &page)
	for _, show := range page.Items {
		a.series[show.Name] = show.Id
	}
	for _, id := range a.series {
		s.get(t, "/Shows/"+id+"/Episodes", a.token, &page)
		for _, episode := range page.Items {
			a.episodes[episode.Name] = episode.Id
		}
	}
	if len(a.series) != 3 || a.episodes["Bravo special"] == "" {
		t.Fatalf("series %v, episodes %v", a.series, a.episodes)
	}
	return a
}

// change posts or deletes id on the member's watchlist through Polyfin's
// route, which answers 204.
func (a airingWatchlist) change(t *testing.T, method, id string) {
	t.Helper()
	if status, body := a.call(method, "/Polyfin/UserItems/"+id+"/Watchlist", app("tv", a.token), nil); status != http.StatusNoContent {
		t.Fatalf("%s %s: %d %s", method, id, status, body)
	}
}

// held answers GET /Polyfin/UserItems/Watchlist.
func (a airingWatchlist) held(t *testing.T) WatchlistDto {
	t.Helper()
	var answer WatchlistDto
	if status := a.get(t, "/Polyfin/UserItems/Watchlist", a.token, &answer); status != http.StatusOK || answer.Items == nil {
		t.Fatalf("watchlist: %d %+v", status, answer)
	}
	return answer
}

// names are the names of entries' ids, by the series' and episodes' maps,
// a pending title's followed by " (pending)".
func (a airingWatchlist) names(entries []WatchlistTitleDto) []string {
	byID := map[string]string{a.movie: "Feature"}
	for name, id := range a.episodes {
		byID[id] = name
	}
	for name, id := range a.series {
		byID[id] = name + " (pending)"
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = byID[e.Id]
	}
	return names
}

// Polyfin's route puts a movie or an episode on the playlist itself, a
// series its aired episodes of regular seasons, a season its aired
// episodes, each at the top; it takes a movie or an episode off itself,
// a series all its entries, a season its own. What it lists is what apps
// list of the playlist, an episode with its SeriesId and SeasonId.
func TestTheWatchlistRouteAddsMoviesEpisodesAndWholeSeries(t *testing.T) {
	t.Parallel()
	a := watchingAiring(t)
	bravo := a.series["Bravo"]
	// Every member has a watchlist playlist from the start, empty.
	start := a.held(t)
	if start.PlaylistId == "" || len(start.Items) != 0 {
		t.Fatalf("a new member's watchlist: %+v", start)
	}
	playlist := func() []string {
		t.Helper()
		var page QueryResult
		if status := a.get(t, "/Items?ParentId="+start.PlaylistId, a.token, &page); status != http.StatusOK {
			t.Fatalf("listing the playlist: %d", status)
		}
		return itemNames(page.Items)
	}

	a.change(t, http.MethodPost, a.movie)
	// A series: its aired episodes of regular seasons, in order, at the
	// top; not its special, nor those to come.
	a.change(t, http.MethodPost, bravo)
	aired := []string{"Bravo long ago", "Bravo two days ago", "Bravo hours ago"}
	held := a.held(t)
	if got, want := a.names(held.Items), append(slices.Clone(aired), "Feature"); held.PlaylistId != start.PlaylistId || !slices.Equal(got, want) {
		t.Fatalf("after adding the series: %v, want %v (%s)", got, want, held.PlaylistId)
	}
	for _, e := range held.Items {
		if want := map[bool]string{true: "", false: bravo}[e.Id == a.movie]; e.SeriesId != want || e.AddedAt.IsZero() {
			t.Errorf("entry %s: SeriesId %q, want %q, added %v", e.Id, e.SeriesId, want, e.AddedAt)
		}
	}
	if got := playlist(); !slices.Equal(got, append(slices.Clone(aired), "Feature")) {
		t.Errorf("the playlist as apps list it: %v", got)
	}
	// Added again, the series changes nothing.
	a.change(t, http.MethodPost, bravo)
	if got := a.names(a.held(t).Items); len(got) != 4 {
		t.Errorf("after adding the series again: %v", got)
	}
	// An episode comes off alone.
	a.change(t, http.MethodDelete, a.episodes["Bravo two days ago"])
	if got := a.names(a.held(t).Items); !slices.Equal(got, []string{"Bravo long ago", "Bravo hours ago", "Feature"}) {
		t.Errorf("after taking an episode off: %v", got)
	}
	// The series comes off whole.
	a.change(t, http.MethodDelete, bravo)
	if got := a.names(a.held(t).Items); !slices.Equal(got, []string{"Feature"}) {
		t.Errorf("after taking the series off: %v", got)
	}
	// A season: its aired episodes.
	var seasons QueryResult
	a.get(t, "/Shows/"+bravo+"/Seasons", a.token, &seasons)
	season := ""
	for _, s := range seasons.Items {
		if s.IndexNumber != nil && *s.IndexNumber == 1 {
			season = s.Id
		}
	}
	if season == "" {
		t.Fatalf("seasons: %v", itemNames(seasons.Items))
	}
	a.change(t, http.MethodPost, season)
	if got := a.names(a.held(t).Items); !slices.Equal(got, append(slices.Clone(aired), "Feature")) {
		t.Errorf("after adding the season: %v", got)
	}
	// An episode adds itself, with its series and season.
	a.change(t, http.MethodPost, a.episodes["Bravo special"])
	held = a.held(t)
	if got := a.names(held.Items); len(got) != 5 || got[0] != "Bravo special" || held.Items[0].SeriesId != bravo ||
		held.Items[0].SeasonId == "" || held.Items[0].SeasonId == season {
		t.Errorf("after adding an episode: %v %+v", got, held.Items[0])
	}
	for _, e := range held.Items[1:4] {
		if e.SeasonId != season {
			t.Errorf("entry %s: SeasonId %q, want %q", e.Id, e.SeasonId, season)
		}
	}
	if movie := held.Items[4]; movie.SeasonId != "" || movie.SeriesId != "" {
		t.Errorf("the movie: %+v", movie)
	}
	// A season comes off alone: the special, of another season, stays.
	a.change(t, http.MethodDelete, season)
	if got := a.names(a.held(t).Items); !slices.Equal(got, []string{"Bravo special", "Feature"}) {
		t.Errorf("after taking the season off: %v", got)
	}
	// A series adds the episodes aired or undated, not those to come.
	a.change(t, http.MethodPost, a.series["Alpha"])
	if got := a.names(a.held(t).Items); !slices.Equal(got, []string{"Alpha undated", "Bravo special", "Feature"}) {
		t.Errorf("after adding a series mostly to come: %v", got)
	}
	// Charlie, none of whose episodes aired, adds no entry yet: it is
	// listed pending, after the entries.
	a.change(t, http.MethodPost, a.series["Charlie"])
	held = a.held(t)
	if got := a.names(held.Items); !slices.Equal(got, []string{"Alpha undated", "Bravo special", "Feature", "Charlie (pending)"}) {
		t.Fatalf("after adding a series not aired: %v", got)
	}
	if p := held.Items[3]; !p.Pending || p.SeriesId != "" || p.SeasonId != "" || p.AddedAt.IsZero() || held.Items[0].Pending {
		t.Errorf("pending: %+v, entry %+v", p, held.Items[0])
	}
	if got := playlist(); !slices.Equal(got, []string{"Alpha undated", "Bravo special", "Feature"}) {
		t.Errorf("the playlist with a pending series: %v", got)
	}
	// Taken off, a pending title waits no more.
	a.change(t, http.MethodDelete, a.series["Charlie"])
	if got := a.names(a.held(t).Items); !slices.Equal(got, []string{"Alpha undated", "Bravo special", "Feature"}) {
		t.Errorf("after taking the pending series off: %v", got)
	}
	// The movie comes off.
	a.change(t, http.MethodDelete, a.movie)
	if got := a.names(a.held(t).Items); !slices.Equal(got, []string{"Alpha undated", "Bravo special"}) {
		t.Errorf("after taking the movie off: %v", got)
	}

	// Deleted, the playlist is gone until something is added again.
	if status, body := a.call(http.MethodDelete, "/Items/"+start.PlaylistId, app("tv", a.token), nil); status != http.StatusNoContent {
		t.Fatalf("deleting the playlist: %d %s", status, body)
	}
	if deleted := a.held(t); deleted.PlaylistId != "" || len(deleted.Items) != 0 {
		t.Errorf("once deleted: %+v", deleted)
	}
	a.change(t, http.MethodPost, a.movie)
	if back := a.held(t); back.PlaylistId == "" || back.PlaylistId == start.PlaylistId || !slices.Equal(a.names(back.Items), []string{"Feature"}) {
		t.Errorf("added to again: %+v", back)
	}
}

// An import adds only what is left to watch: not a movie the member
// played, and of a series only the aired episodes they have not played;
// nothing waits for the movie. Added by hand, through Polyfin's route,
// both come whole, as Jellyfin's playlists take them.
func TestAnImportAddsWhatIsLeftToWatchAndAHandAddEverything(t *testing.T) {
	t.Parallel()
	a := watchingAiring(t)
	for _, id := range []string{a.movie, a.episodes["Bravo long ago"]} {
		if status, body := a.call(http.MethodPost, "/UserPlayedItems/"+id, app("tv", a.token), nil); status != http.StatusOK {
			t.Fatalf("marking %s played: %d %s", id, status, body)
		}
	}
	var titles []watchlist.Title
	for _, id := range []string{a.movie, a.series["Bravo"]} {
		parsed, err := accounts.ParseID(id)
		if err != nil {
			t.Fatal(err)
		}
		titles = append(titles, watchlist.Title{ID: parsed})
	}
	if _, err := a.watchlistStore().Save(t.Context(), a.member.ID, watchlist.Import, titles, false); err != nil {
		t.Fatal(err)
	}
	sorted := func(names []string) []string {
		slices.Sort(names)
		return names
	}
	if got, want := sorted(a.names(a.held(t).Items)), []string{"Bravo hours ago", "Bravo two days ago"}; !slices.Equal(got, want) {
		t.Errorf("imported: %v, want %v", got, want)
	}
	a.change(t, http.MethodPost, a.movie)
	a.change(t, http.MethodPost, a.series["Bravo"])
	if got, want := sorted(a.names(a.held(t).Items)), []string{"Bravo hours ago", "Bravo long ago", "Bravo two days ago", "Feature"}; !slices.Equal(got, want) {
		t.Errorf("added by hand: %v, want %v", got, want)
	}
}

// The route answers 400 for an item that is neither a movie, a series, a
// season nor an episode, and 404 for an unknown one.
func TestTheWatchlistRouteRefusesOtherItems(t *testing.T) {
	t.Parallel()
	a := watchingAiring(t)
	views := a.viewIDs(t, a.token)
	for method, want := range map[string]int{http.MethodPost: http.StatusBadRequest, http.MethodDelete: http.StatusBadRequest} {
		if status, body := a.call(method, "/Polyfin/UserItems/"+views["Movies"]+"/Watchlist", app("tv", a.token), nil); status != want {
			t.Errorf("%s a library: %d %s", method, status, body)
		}
	}
	unknown := accounts.ID{0x7e}.String()
	if status, _ := a.call(http.MethodPost, "/Polyfin/UserItems/"+unknown+"/Watchlist", app("tv", a.token), nil); status != http.StatusNotFound {
		t.Errorf("an unknown item: %d", status)
	}
	if got := a.held(t); len(got.Items) != 0 {
		t.Errorf("watchlist: %+v", got)
	}
}

// /UserViews lists no Watchlist view: the watchlist is a playlist, found
// with the user's playlists.
func TestUserViewsListNoWatchlistView(t *testing.T) {
	t.Parallel()
	a := watchingAiring(t)
	a.change(t, http.MethodPost, a.movie)
	held := a.held(t)
	var views QueryResult
	a.get(t, "/UserViews", a.token, &views)
	for _, view := range views.Items {
		if view.Name == "Watchlist" || view.Id == held.PlaylistId {
			t.Errorf("a Watchlist view: %+v", view)
		}
	}
	var playlists QueryResult
	a.get(t, "/Items?IncludeItemTypes=Playlist&Recursive=true", a.token, &playlists)
	if got := itemIDs(playlists.Items); !slices.Contains(got, held.PlaylistId) {
		t.Errorf("playlists: %v, want %s among them", itemNames(playlists.Items), held.PlaylistId)
	}
}
