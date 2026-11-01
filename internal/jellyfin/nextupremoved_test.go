package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A series taken off Next Up stays off until one of its episodes is
// played after the removal, by a play or a dated mark, or the user puts
// it back; asked for alone, as its page asks, it keeps its next episode.
func TestRemovingFromNextUpHoldsUntilANewerPlay(t *testing.T) {
	t.Parallel()
	tr := newTracking(t)
	nextUp := func(query string) []string { t.Helper(); return tr.list(t, "/Shows/NextUp?"+query) }
	change := func(method, id string) {
		t.Helper()
		if status := tr.send(t, method, "/Polyfin/UserItems/"+id+"/NextUp", nil); status != http.StatusNoContent {
			t.Fatalf("%s %s: %d", method, id, status)
		}
	}
	// The removal is dated, so that marks can be dated around it.
	removedOn := func(date string) {
		t.Helper()
		if _, err := tr.pool.Exec(t.Context(), "UPDATE next_up_removed SET removed_at = $1", date); err != nil {
			t.Fatal(err)
		}
	}
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.episodes[0]+"?datePlayed=2025-01-01T00:00:00Z")
	if got := nextUp(""); !slices.Equal(got, tr.episodes[1:2]) {
		t.Fatalf("before the removal: %v", got)
	}

	// From the episode the card shows: its series goes.
	change(http.MethodDelete, tr.episodes[1])
	if got := nextUp(""); len(got) != 0 {
		t.Errorf("after the removal: %v", got)
	}
	if got := nextUp("seriesId=" + tr.series); !slices.Equal(got, tr.episodes[1:2]) {
		t.Errorf("the series asked for: %v", got)
	}
	// A mark dated before the removal leaves it off; one after brings it
	// back.
	removedOn("2025-06-01T00:00:00Z")
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.episodes[0]+"?datePlayed=2025-03-01T00:00:00Z")
	if got := nextUp(""); len(got) != 0 {
		t.Errorf("after an older mark: %v", got)
	}
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.episodes[0]+"?datePlayed=2025-09-01T00:00:00Z")
	if got := nextUp(""); !slices.Equal(got, tr.episodes[1:2]) {
		t.Errorf("after a newer mark: %v", got)
	}

	// Put back by the user, from the series.
	change(http.MethodDelete, tr.series)
	if got := nextUp(""); len(got) != 0 {
		t.Errorf("after the second removal: %v", got)
	}
	change(http.MethodPost, tr.series)
	if got := nextUp(""); !slices.Equal(got, tr.episodes[1:2]) {
		t.Errorf("put back: %v", got)
	}

	// Brought back by a play.
	change(http.MethodDelete, tr.seasons[0])
	if got := nextUp(""); len(got) != 0 {
		t.Errorf("after the third removal: %v", got)
	}
	tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": tr.episodes[1], "PositionTicks": 0})
	tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": tr.episodes[1], "PositionTicks": 6_000_000_000})
	if got := nextUp(""); !slices.Equal(got, tr.episodes[1:2]) {
		t.Errorf("after a play: %v", got)
	}

	// A movie has no Next Up; an unknown item is not found.
	if status := tr.send(t, http.MethodDelete, "/Polyfin/UserItems/"+tr.movie+"/NextUp", nil); status != http.StatusBadRequest {
		t.Errorf("a movie: %d", status)
	}
	if status := tr.send(t, http.MethodDelete, "/Polyfin/UserItems/0123456789abcdef0123456789abcdef/NextUp", nil); status != http.StatusNotFound {
		t.Errorf("an unknown item: %d", status)
	}
}

// freshAddon serves a catalog of two series, each with a first episode
// out long ago: Fresh, whose second episode came out yesterday and whose
// third comes out next week, and Dateless, whose second has no date.
func freshAddon(t *testing.T) string {
	t.Helper()
	day := func(days int) string {
		return time.Now().UTC().AddDate(0, 0, days).Format(time.DateOnly) + "T00:00:00.000Z"
	}
	shows := []stremio.Meta{
		{ID: "tt0801", Type: "series", Name: "Fresh", Videos: []stremio.Video{
			{ID: "tt0801:1:1", Title: "Fresh 1", Season: 1, Episode: 1, Released: day(-400)},
			{ID: "tt0801:1:2", Title: "Fresh 2", Season: 1, Episode: 2, Released: day(-1)},
			{ID: "tt0801:1:3", Title: "Fresh 3", Season: 1, Episode: 3, Released: day(7)},
		}},
		{ID: "tt0802", Type: "series", Name: "Dateless", Videos: []stremio.Video{
			{ID: "tt0802:1:1", Title: "Dateless 1", Season: 1, Episode: 1, Released: day(-400)},
			{ID: "tt0802:1:2", Title: "Dateless 2", Season: 1, Episode: 2},
		}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "fresh", Name: "Fresh", Version: "1", Types: []string{"series"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "series", ID: "fresh", Name: "Fresh"}}})
		case strings.HasPrefix(path, "/catalog/series/fresh"):
			previews := make([]stremio.Meta, 0, len(shows))
			for _, show := range shows {
				show.Videos = nil
				previews = append(previews, show)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": previews})
		case strings.HasPrefix(path, "/meta/series/"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/meta/series/"), ".json")
			for _, show := range shows {
				if show.ID == id {
					_ = json.NewEncoder(w).Encode(map[string]any{"meta": show})
					return
				}
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// An episode released after the removal brings its series back on Next
// Up; one out before it, one still to come or one without a date does
// not. My account lists only the series still off Next Up.
func TestANewEpisodeBringsARemovedSeriesBack(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), freshAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "fresh"},
	}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	tr := tracking{testServer: s, token: token, user: member.ID.String(), views: map[string]string{}}
	var page QueryResult
	s.get(t, "/UserViews", token, &page)
	for _, view := range page.Items {
		tr.views[view.Name] = view.Id
	}
	series := map[string]string{}
	episodes := map[string]string{}
	s.get(t, "/Items?ParentId="+tr.views["Fresh"], token, &page)
	for _, show := range page.Items {
		series[show.Name] = show.Id
		var listed QueryResult
		s.get(t, "/Shows/"+show.Id+"/Episodes", token, &listed)
		for _, episode := range listed.Items {
			episodes[episode.Name] = episode.Id
		}
	}
	if len(series) != 2 || len(episodes) != 5 {
		t.Fatalf("series %v, episodes %v", series, episodes)
	}
	played := time.Now().UTC().AddDate(0, 0, -300).Format(time.RFC3339)
	for _, name := range []string{"Fresh 1", "Dateless 1"} {
		tr.mark(t, http.MethodPost, "/UserPlayedItems/"+episodes[name]+"?datePlayed="+played)
	}
	nextUp := func() []string { t.Helper(); return tr.list(t, "/Shows/NextUp") }
	if got, want := slices.Sorted(slices.Values(nextUp())), slices.Sorted(slices.Values([]string{episodes["Fresh 2"], episodes["Dateless 2"]})); !slices.Equal(got, want) {
		t.Fatalf("before the removals: %v, want %v", got, want)
	}
	for _, name := range []string{"Fresh 2", "Dateless 2"} {
		if status := tr.send(t, http.MethodDelete, "/Polyfin/UserItems/"+episodes[name]+"/NextUp", nil); status != http.StatusNoContent {
			t.Fatalf("removing %s: %d", name, status)
		}
	}
	// Fresh 2 came out before the removal, Fresh 3 is not out yet.
	if got := nextUp(); len(got) != 0 {
		t.Errorf("after the removals: %v", got)
	}
	removals, err := s.handler.NextUpRemovals(t.Context(), member)
	if err != nil || len(removals) != 2 {
		t.Fatalf("removals: %v %+v", err, removals)
	}

	// Taken off three days ago, Fresh 2 is newer.
	if _, err := s.pool.Exec(t.Context(), "UPDATE next_up_removed SET removed_at = $1", time.Now().AddDate(0, 0, -3)); err != nil {
		t.Fatal(err)
	}
	if got, want := nextUp(), []string{episodes["Fresh 2"]}; !slices.Equal(got, want) {
		t.Errorf("after a new episode: %v, want %v", got, want)
	}
	removals, err = s.handler.NextUpRemovals(t.Context(), member)
	if err != nil || len(removals) != 1 || removals[0].Item.Name != "Dateless" {
		t.Errorf("removals after a new episode: %v %+v", err, removals)
	}
	if got := tr.list(t, "/Shows/NextUp?seriesId="+series["Dateless"]); !slices.Equal(got, []string{episodes["Dateless 2"]}) {
		t.Errorf("Dateless asked for: %v", got)
	}
}

// A series being rewatched, taken off Next Up, leaves out both its next
// unplayed episode and the next one of the rewatch, until a newer play;
// asked for alone, it keeps both.
func TestRemovingFromNextUpTakesARewatchOff(t *testing.T) {
	t.Parallel()
	tr := newTracking(t)
	// The first two episodes played, then the first again.
	for i, episode := range []string{tr.episodes[0], tr.episodes[1], tr.episodes[0]} {
		tr.mark(t, http.MethodPost, "/UserPlayedItems/"+episode+"?datePlayed=2026-01-0"+strconv.Itoa(i+1)+"T20:00:00Z")
	}
	nextUp := func(query string) []string {
		t.Helper()
		return tr.list(t, "/Shows/NextUp?enableRewatching=true"+query)
	}
	both := []string{tr.episodes[1], tr.episodes[2]}
	if got := nextUp(""); !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(both))) {
		t.Fatalf("before the removal: %v", got)
	}
	if status := tr.send(t, http.MethodDelete, "/Polyfin/UserItems/"+tr.series+"/NextUp", nil); status != http.StatusNoContent {
		t.Fatalf("removing: %d", status)
	}
	if got := nextUp(""); len(got) != 0 {
		t.Errorf("after the removal: %v", got)
	}
	if got := nextUp("&seriesId=" + tr.series); len(got) != 2 {
		t.Errorf("the series asked for: %v", got)
	}
	tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": tr.episodes[1], "PositionTicks": 0})
	// Played again past the start: the play is dated now.
	tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": tr.episodes[1], "PositionTicks": 6_000_000_000})
	if got := nextUp(""); len(got) == 0 {
		t.Errorf("after a newer play: %v", got)
	}
}
