package jellyfin

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/watchlist"
)

func TestRemovingFromContinueWatchingKeepsThePlayedState(t *testing.T) {
	t.Parallel()
	tr := newTracking(t)
	resume := func() []string { t.Helper(); return tr.list(t, "/UserItems/Resume") }
	// A movie watched once, then started again: played, with a resume
	// point, and one left half watched.
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.movie+"?datePlayed=2026-01-02T03:04:05Z")
	for _, id := range []string{tr.movie, tr.episodes[0]} {
		tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": id, "PositionTicks": 0})
		tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": id, "PositionTicks": 12_000_000_000})
	}
	before := tr.userData(t, tr.movie)
	if !before.Played || before.PlayCount != 2 || before.PlaybackPositionTicks == 0 || before.LastPlayedDate == nil {
		t.Fatalf("before: %+v", before)
	}
	if got := resume(); !slices.Equal(got, []string{tr.episodes[0], tr.movie}) {
		t.Fatalf("resume before: %v", got)
	}

	data := tr.mark(t, http.MethodDelete, "/Polyfin/UserItems/"+tr.movie+"/Resume")
	if !data.Played || data.PlayCount != 2 || data.PlaybackPositionTicks != 0 || data.LastPlayedDate == nil ||
		*data.LastPlayedDate != *before.LastPlayedDate {
		t.Errorf("after the removal: %+v", data)
	}
	if got := resume(); !slices.Equal(got, []string{tr.episodes[0]}) {
		t.Errorf("resume after the removal: %v", got)
	}
	// The episode, never played through, stays unplayed with its play.
	if data := tr.mark(t, http.MethodDelete, "/Polyfin/UserItems/"+tr.series+"/Resume"); data.ItemId != tr.series {
		t.Errorf("series answer: %+v", data)
	}
	if got := resume(); len(got) != 0 {
		t.Errorf("resume after the series' removal: %v", got)
	}
	if episode := tr.userData(t, tr.episodes[0]); episode.Played || episode.PlayCount != 1 || episode.LastPlayedDate == nil {
		t.Errorf("episode after the removal: %+v", episode)
	}

	// Playing it again sets a resume point as usual.
	tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": tr.movie, "PositionTicks": 0})
	tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": tr.movie, "PositionTicks": 20_000_000_000})
	if got := resume(); !slices.Equal(got, []string{tr.movie}) {
		t.Errorf("resume after playing again: %v", got)
	}
	// What keeps no resume point is refused; an unknown item is not found.
	if status := tr.send(t, http.MethodDelete, "/Polyfin/UserItems/"+tr.views["Top"]+"/Resume", nil); status != http.StatusBadRequest {
		t.Errorf("a library: %d", status)
	}
	if status := tr.send(t, http.MethodDelete, "/Polyfin/UserItems/0123456789abcdef0123456789abcdef/Resume", nil); status != http.StatusNotFound {
		t.Errorf("an unknown item: %d", status)
	}
}

func TestAHiddenTitleLeavesTheUsersListsOnly(t *testing.T) {
	t.Parallel()
	tr := newTracking(t)
	// Listings answer a page; Latest answers its items alone.
	names := func(token, path string) []string {
		t.Helper()
		var answer json.RawMessage
		if status := tr.get(t, path, token, &answer); status != http.StatusOK {
			t.Fatalf("%s: %d", path, status)
		}
		var page QueryResult
		if err := json.Unmarshal(answer, &page.Items); err != nil {
			if err := json.Unmarshal(answer, &page); err != nil {
				t.Fatalf("%s: %v", path, err)
			}
		}
		return itemNames(page.Items)
	}
	var movie BaseItemDto
	if status := tr.get(t, "/Items/"+tr.movie, tr.token, &movie); status != http.StatusOK || movie.Name != "Movie 0" || movie.People == nil {
		t.Fatalf("movie: %d %+v", status, movie)
	}
	actor := (*movie.People)[0].Id
	var groups QueryResult
	tr.get(t, "/Items?ParentId="+tr.views["Groups"], tr.token, &groups)
	group := groups.Items[0].Id
	var others QueryResult
	tr.get(t, "/Items?ParentId="+tr.views["Top"], tr.token, &others)
	similarTo := others.Items[2].Id
	member, err := accounts.ParseID(tr.user)
	if err != nil {
		t.Fatal(err)
	}
	movieID, _ := accounts.ParseID(tr.movie)
	if _, err := watchlist.New(tr.pool).Save(t.Context(), member, watchlist.Import,
		[]watchlist.Title{{ID: movieID, AddedAt: new(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))}}, false); err != nil {
		t.Fatal(err)
	}
	watchlistID := tr.viewIDs(t, tr.token)["Watchlist"]
	lists := map[string]string{
		"library":     "/Items?ParentId=" + tr.views["Top"],
		"latest":      "/Items/Latest?ParentId=" + tr.views["Top"],
		"search":      "/Items?searchTerm=Movie&IncludeItemTypes=Movie&Recursive=true",
		"similar":     "/Items/" + similarTo + "/Similar",
		"person":      "/Items?personIds=" + actor + "&recursive=true&includeItemTypes=Movie",
		"collection":  "/Items?ParentId=" + group,
		"favorites":   "/Items?Filters=IsFavorite&Recursive=true",
		"watchlist":   "/Items?ParentId=" + watchlistID,
		"suggestions": "/Items/Suggestions",
	}
	tr.mark(t, http.MethodPost, "/UserFavoriteItems/"+tr.movie)
	for name, path := range lists {
		if got := names(tr.token, path); !slices.Contains(got, "Movie 0") {
			t.Fatalf("%s before hiding: %v", name, got)
		}
	}

	if status := tr.send(t, http.MethodPost, "/Polyfin/UserItems/"+tr.movie+"/Hidden", nil); status != http.StatusNoContent {
		t.Fatalf("hiding: %d", status)
	}
	for name, path := range lists {
		if got := names(tr.token, path); slices.Contains(got, "Movie 0") {
			t.Errorf("%s while hidden: %v", name, got)
		}
	}
	var page QueryResult
	if tr.get(t, lists["library"], tr.token, &page); page.TotalRecordCount != 2 {
		t.Errorf("library count while hidden: %d", page.TotalRecordCount)
	}
	if got := hintNames(hints(t, tr.testServer, tr.token, "searchTerm=Movie")); slices.Contains(got, "Movie Movie 0") {
		t.Errorf("search hints while hidden: %v", got)
	}
	// It still opens by its identifier, and is listed among the hidden.
	if status := tr.get(t, "/Users/"+tr.user+"/Items/"+tr.movie, tr.token, nil); status != http.StatusOK {
		t.Errorf("opening it: %d", status)
	}
	if got := names(tr.token, "/Items?Ids="+tr.movie); !slices.Equal(got, []string{"Movie 0"}) {
		t.Errorf("by identifier: %v", got)
	}
	if got := names(tr.token, "/Polyfin/UserItems/Hidden"); !slices.Equal(got, []string{"Movie 0"}) {
		t.Errorf("hidden titles: %v", got)
	}
	// Another user still sees it.
	tr.testServer.user("other", nil)
	if got := names(tr.signIn("other", "phone"), lists["library"]); !slices.Contains(got, "Movie 0") {
		t.Errorf("another user's library: %v", got)
	}

	if status := tr.send(t, http.MethodDelete, "/Polyfin/UserItems/"+tr.movie+"/Hidden", nil); status != http.StatusNoContent {
		t.Fatalf("showing again: %d", status)
	}
	for name, path := range lists {
		if got := names(tr.token, path); !slices.Contains(got, "Movie 0") {
			t.Errorf("%s shown again: %v", name, got)
		}
	}
	if got := names(tr.token, "/Polyfin/UserItems/Hidden"); len(got) != 0 {
		t.Errorf("hidden titles once shown: %v", got)
	}
	// Only titles, and what is in them, can be hidden.
	if status := tr.send(t, http.MethodPost, "/Polyfin/UserItems/"+tr.views["Top"]+"/Hidden", nil); status != http.StatusBadRequest {
		t.Errorf("hiding a library: %d", status)
	}
}

func TestHidingASeriesLeavesItsEpisodesOutOfTheHomeRows(t *testing.T) {
	t.Parallel()
	tr := newTracking(t)
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.episodes[0])
	tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": tr.episodes[2], "PositionTicks": 0})
	tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": tr.episodes[2], "PositionTicks": 12_000_000_000})
	if got := tr.list(t, "/Shows/NextUp"); !slices.Equal(got, tr.episodes[1:2]) {
		t.Fatalf("next up: %v", got)
	}
	if got := tr.list(t, "/UserItems/Resume"); !slices.Equal(got, tr.episodes[2:3]) {
		t.Fatalf("resume: %v", got)
	}
	member, _, err := tr.openSocket(t, tr.token)
	if err != nil {
		t.Fatal(err)
	}
	member.next(t)

	// Hidden through one of its episodes, as from a card of Next Up.
	if status := tr.send(t, http.MethodPost, "/Polyfin/UserItems/"+tr.episodes[1]+"/Hidden", nil); status != http.StatusNoContent {
		t.Fatalf("hiding: %d", status)
	}
	if change := member.changed(t); !slices.Equal(ids(change.UserDataList), []string{tr.series}) {
		t.Errorf("socket told %v", ids(change.UserDataList))
	}
	if got := tr.list(t, "/Polyfin/UserItems/Hidden"); !slices.Equal(got, []string{tr.series}) {
		t.Errorf("hidden titles: %v", got)
	}
	for _, path := range []string{"/Shows/NextUp", "/UserItems/Resume", "/Items?ParentId=" + tr.views["Shows"]} {
		if got := tr.list(t, path); len(got) != 0 {
			t.Errorf("%s while hidden: %v", path, got)
		}
	}
	// Its page still lists its episodes and its next one.
	if got := tr.list(t, "/Shows/"+tr.series+"/Episodes"); !slices.Equal(got, tr.episodes) {
		t.Errorf("its episodes: %v", got)
	}
	if got := tr.list(t, "/Shows/NextUp?seriesId="+tr.series); !slices.Equal(got, tr.episodes[1:2]) {
		t.Errorf("its next up: %v", got)
	}

	if status := tr.send(t, http.MethodDelete, "/Polyfin/UserItems/"+tr.series+"/Hidden", nil); status != http.StatusNoContent {
		t.Fatalf("showing again: %d", status)
	}
	if got := tr.list(t, "/Shows/NextUp"); !slices.Equal(got, tr.episodes[1:2]) {
		t.Errorf("next up shown again: %v", got)
	}
}
